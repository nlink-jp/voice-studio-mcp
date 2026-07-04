//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nlink-jp/voice-studio-mcp/internal/engine/enginetest"
)

const castingTOML = `
[characters."narrator"]
speaker_uuid = "00000000-0000-0000-0000-0000000000aa"
style_id = 100
credit = "AivisSpeech:MockNarrator"
license_checked = true

[characters."美咲"]
speaker_uuid = "00000000-0000-0000-0000-0000000000bb"
style_id = 200
credit = "AivisSpeech:MockHeroine"
license_checked = true

[characters."美咲".styles]
"悲しみ" = 201
`

const scriptJSONL = `# e2e episode
{"id":1,"scene":1,"speaker":"narrator","text":"夜のとばりが街を包んでいた。","pause_after_ms":800}
{"id":2,"scene":1,"speaker":"美咲","text":"……本当に、行くの?","style":"悲しみ","intensity":1.4,"speed":0.9}
{"id":3,"scene":2,"speaker":"narrator","text":"翌朝。"}
`

// ffmpegStub is a shell script that materializes the output file (the last
// argument), standing in for ffmpeg in e2e runs.
const ffmpegStub = `#!/bin/sh
eval "out=\${$#}"
: > "$out"
exit 0
`

// setupEnv starts a mock engine and writes a full external-mode config plus
// workspace inputs. It returns the config path and the workspace root.
func setupEnv(t *testing.T, engineURL string) (configPath, wsRoot string) {
	t.Helper()
	root := t.TempDir()
	wsRoot = filepath.Join(root, "workspaces")

	stub := filepath.Join(root, "ffmpeg-stub")
	if err := os.WriteFile(stub, []byte(ffmpegStub), 0o755); err != nil {
		t.Fatal(err)
	}

	configPath = filepath.Join(root, "config.toml")
	cfg := fmt.Sprintf(`
[workspace]
workspace_dir = %q

[engine]
mode = "external"
url = %q
request_timeout_seconds = 30

[master]
ffmpeg_path = %q

[[speaker_metadata]]
speaker_uuid = "00000000-0000-0000-0000-0000000000aa"
name = "MockNarrator"
license = "ACML 1.0"
credit = "AivisSpeech:MockNarrator"
commercial_use = true
`, wsRoot, engineURL, stub)
	if err := os.WriteFile(configPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	// Workspace inputs, as the agent would place them.
	wsDir := filepath.Join(wsRoot, "ep1")
	if err := os.MkdirAll(filepath.Join(wsDir, "script"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wsDir, "casting.toml"), []byte(castingTOML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wsDir, "script", "ep1.jsonl"), []byte(scriptJSONL), 0o644); err != nil {
		t.Fatal(err)
	}
	return configPath, wsRoot
}

func TestE2E_FullProductionFlow(t *testing.T) {
	binary, ok := requireBinary(t)
	if !ok {
		return
	}
	mock := enginetest.New()
	defer mock.Close()

	configPath, wsRoot := setupEnv(t, mock.URL())
	h := Start(t, binary, configPath)
	if err := h.Initialize(); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	// tools/list exposes all six tools.
	res, err := h.Call("tools/list", nil, 5*time.Second)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	for _, tool := range []string{"list_speakers", "register_dictionary", "synthesize_script", "synthesize_line", "check_job", "master"} {
		if !strings.Contains(string(res), `"name":"`+tool+`"`) {
			t.Errorf("tools/list missing %s", tool)
		}
	}

	// 1. Speaker catalog with license metadata.
	body, isErr, err := h.CallTool("list_speakers", map[string]any{}, 10*time.Second)
	if err != nil || isErr {
		t.Fatalf("list_speakers: err=%v isErr=%v body=%s", err, isErr, body)
	}
	if !strings.Contains(string(body), `"status":"verified"`) || !strings.Contains(string(body), `"status":"unverified"`) {
		t.Errorf("license statuses missing: %s", body)
	}

	// 2. Pronunciation dictionary.
	body, isErr, err = h.CallTool("register_dictionary", map[string]any{
		"workspace_id": "ep1",
		"words": []map[string]any{
			{"surface": "美咲", "pronunciation": "ミサキ", "accent_type": 1, "word_type": "PROPER_NOUN"},
		},
	}, 10*time.Second)
	if err != nil || isErr {
		t.Fatalf("register_dictionary: err=%v isErr=%v body=%s", err, isErr, body)
	}
	if !strings.Contains(string(body), `"registered":1`) {
		t.Errorf("register_dictionary: %s", body)
	}

	// 3. Batch synthesis (async) + polling.
	body, isErr, err = h.CallTool("synthesize_script", map[string]any{
		"workspace_id": "ep1",
		"script_path":  "script/ep1.jsonl",
	}, 30*time.Second)
	if err != nil || isErr {
		t.Fatalf("synthesize_script: err=%v isErr=%v body=%s", err, isErr, body)
	}
	var so struct {
		JobID  string `json:"job_id"`
		Queued int    `json:"queued"`
	}
	if err := json.Unmarshal(body, &so); err != nil {
		t.Fatal(err)
	}
	if so.JobID == "" || so.Queued != 3 {
		t.Fatalf("script response: %s", body)
	}

	deadline := time.Now().Add(30 * time.Second)
	var jo struct {
		State  string `json:"state"`
		Done   int    `json:"done"`
		Failed int    `json:"failed"`
	}
	for {
		body, isErr, err = h.CallTool("check_job", map[string]any{"job_id": so.JobID}, 10*time.Second)
		if err != nil || isErr {
			t.Fatalf("check_job: err=%v isErr=%v body=%s", err, isErr, body)
		}
		if err := json.Unmarshal(body, &jo); err != nil {
			t.Fatal(err)
		}
		if jo.State != "running" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job stuck: %s", body)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if jo.State != "done" || jo.Done != 3 || jo.Failed != 0 {
		t.Fatalf("job: %+v", jo)
	}
	for _, n := range []string{"1.wav", "2.wav", "3.wav"} {
		if _, err := os.Stat(filepath.Join(wsRoot, "ep1", "wav", n)); err != nil {
			t.Errorf("missing %s: %v", n, err)
		}
	}

	// 4. Retake a single line.
	body, isErr, err = h.CallTool("synthesize_line", map[string]any{
		"workspace_id": "ep1",
		"line":         map[string]any{"id": 2, "speaker": "美咲", "text": "……本当に、行くの?", "style": "悲しみ", "intensity": 1.8},
	}, 15*time.Second)
	if err != nil || isErr {
		t.Fatalf("synthesize_line: err=%v isErr=%v body=%s", err, isErr, body)
	}
	if !strings.Contains(string(body), `"cached":false`) {
		t.Errorf("changed intensity should re-synthesize: %s", body)
	}

	// 5. Master to m4b with chapters + credits.
	body, isErr, err = h.CallTool("master", map[string]any{
		"workspace_id": "ep1",
		"script_path":  "script/ep1.jsonl",
		"format":       "m4b",
	}, 30*time.Second)
	if err != nil || isErr {
		t.Fatalf("master: err=%v isErr=%v body=%s", err, isErr, body)
	}
	var mo struct {
		MasterPath  string   `json:"master_path"`
		Chapters    int      `json:"chapters"`
		CreditsPath string   `json:"credits_path"`
		Credits     []string `json:"credits"`
	}
	if err := json.Unmarshal(body, &mo); err != nil {
		t.Fatal(err)
	}
	if mo.Chapters != 2 || len(mo.Credits) != 2 {
		t.Errorf("master: %s", body)
	}
	if _, err := os.Stat(mo.MasterPath); err != nil {
		t.Errorf("master output: %v", err)
	}
	if _, err := os.Stat(mo.CreditsPath); err != nil {
		t.Errorf("credits output: %v", err)
	}
}

func TestE2E_StructuredErrors(t *testing.T) {
	binary, ok := requireBinary(t)
	if !ok {
		return
	}
	mock := enginetest.New()
	defer mock.Close()

	configPath, wsRoot := setupEnv(t, mock.URL())
	// Corrupt the script: duplicate id + missing text.
	bad := `{"id":1,"speaker":"narrator","text":"ok"}` + "\n" + `{"id":1,"speaker":"narrator"}`
	if err := os.WriteFile(filepath.Join(wsRoot, "ep1", "script", "bad.jsonl"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}

	h := Start(t, binary, configPath)
	if err := h.Initialize(); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	body, isErr, err := h.CallTool("synthesize_script", map[string]any{
		"workspace_id": "ep1",
		"script_path":  "script/bad.jsonl",
	}, 15*time.Second)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !isErr {
		t.Fatalf("expected tool error: %s", body)
	}
	var te struct {
		Code    string         `json:"code"`
		Details map[string]any `json:"details"`
	}
	if err := json.Unmarshal(body, &te); err != nil {
		t.Fatalf("error body not structured JSON: %v (%s)", err, body)
	}
	if te.Code != "invalid_script" || te.Details["error_count"] == nil {
		t.Errorf("structured error: %s", body)
	}
}
