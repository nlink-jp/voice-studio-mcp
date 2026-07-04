//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestE2E_RealEngineFullProduction simulates the complete agent workflow
// ("test head") against a real AivisSpeech Engine and real ffmpeg:
//
//	list_speakers → casting → register_dictionary → synthesize_script →
//	check_job → retake (synthesize_line) → master (mp3 + m4b)
//
// The casting table is built dynamically from whatever voice models are
// installed, so the test adapts to any AivisSpeech setup.
//
// Opt-in (needs AivisSpeech + ffmpeg; first model load takes minutes):
//
//	VOICE_STUDIO_TEST_REAL_ENGINE=1 make test-e2e
//
// Set VOICE_STUDIO_TEST_WORK_DIR to keep the produced audio for listening;
// otherwise a temp dir is used and cleaned.
func TestE2E_RealEngineFullProduction(t *testing.T) {
	if os.Getenv("VOICE_STUDIO_TEST_REAL_ENGINE") != "1" {
		t.Skip("set VOICE_STUDIO_TEST_REAL_ENGINE=1 to run against a real AivisSpeech Engine")
	}
	binary, ok := requireBinary(t)
	if !ok {
		return
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed; master step needs it")
	}

	workRoot := os.Getenv("VOICE_STUDIO_TEST_WORK_DIR")
	if workRoot == "" {
		workRoot = t.TempDir()
	} else if err := os.MkdirAll(workRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	wsID := "sim-drama"
	wsDir := filepath.Join(workRoot, wsID)
	// Start from a clean workspace so cache hits from prior runs don't mask
	// synthesis problems.
	if err := os.RemoveAll(wsDir); err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(t.TempDir(), "config.toml")
	cfg := fmt.Sprintf("[workspace]\nworkspace_dir = %q\n\n[engine]\nmode = \"managed\"\n", workRoot)
	if err := os.WriteFile(configPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	h := Start(t, binary, configPath)
	if err := h.InitializeWithTimeout(4 * time.Minute); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	// --- 1. Speaker discovery (the agent reads the catalog) ---
	body, isErr, err := h.CallTool("list_speakers", map[string]any{}, 60*time.Second)
	if err != nil || isErr {
		t.Fatalf("list_speakers: err=%v isErr=%v body=%s", err, isErr, body)
	}
	var cat struct {
		Speakers []struct {
			Name        string `json:"name"`
			SpeakerUUID string `json:"speaker_uuid"`
			Styles      []struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			} `json:"styles"`
			License struct {
				Status string `json:"status"`
			} `json:"license"`
		} `json:"speakers"`
	}
	if err := json.Unmarshal(body, &cat); err != nil {
		t.Fatal(err)
	}
	if len(cat.Speakers) == 0 || len(cat.Speakers[0].Styles) == 0 {
		t.Fatalf("no voice models installed: %s", body)
	}
	narrator := cat.Speakers[0]
	heroine := cat.Speakers[len(cat.Speakers)-1] // 2nd speaker if present, else same
	heroineEmphasis := heroine.Styles[len(heroine.Styles)-1].ID
	t.Logf("casting: narrator=%s (style %d), 美咲=%s (default %d, 強め %d)",
		narrator.Name, narrator.Styles[0].ID, heroine.Name, heroine.Styles[0].ID, heroineEmphasis)

	// --- 2. Casting table + script (the agent's deliverables) ---
	if err := os.MkdirAll(filepath.Join(wsDir, "script"), 0o755); err != nil {
		t.Fatal(err)
	}
	casting := fmt.Sprintf(`
[characters."ナレーター"]
speaker_uuid = %q
style_id = %d
credit = "AivisSpeech:%s"
license_checked = true

[characters."美咲"]
speaker_uuid = %q
style_id = %d
credit = "AivisSpeech:%s"
license_checked = true

[characters."美咲".styles]
"強め" = %d
`, narrator.SpeakerUUID, narrator.Styles[0].ID, narrator.Name,
		heroine.SpeakerUUID, heroine.Styles[0].ID, heroine.Name, heroineEmphasis)
	if err := os.WriteFile(filepath.Join(wsDir, "casting.toml"), []byte(casting), 0o644); err != nil {
		t.Fatal(err)
	}

	script := `# シミュレーション: ラジオドラマ「宵闇亭の夜」
{"id":1,"scene":1,"speaker":"ナレーター","text":"夜のとばりが下りるころ、宵闇亭の看板にひとつ、灯りがともった。","speed":0.95,"pause_after_ms":900}
{"id":2,"scene":1,"speaker":"美咲","text":"いらっしゃいませ。……あら、珍しいお客さま。","pause_after_ms":500}
{"id":3,"scene":1,"speaker":"ナレーター","text":"扉の前に立っていたのは、雨に濡れたひとりの旅人だった。","pause_after_ms":800}
{"id":4,"scene":2,"speaker":"美咲","text":"そんなの、聞いてないわ!","style":"強め","intensity":1.5,"speed":1.05,"pause_after_ms":600}
{"id":5,"scene":2,"speaker":"ナレーター","text":"美咲の声が、静かな店内に響いた。","pause_after_ms":700}
{"id":6,"scene":2,"speaker":"美咲","text":"……ごめんなさい。続きを、聞かせてください。","intensity":0.8,"speed":0.9,"pause_after_ms":1000}
`
	if err := os.WriteFile(filepath.Join(wsDir, "script", "yoiyami.jsonl"), []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}

	// --- 3. Pronunciation dictionary (coined word) ---
	body, isErr, err = h.CallTool("register_dictionary", map[string]any{
		"workspace_id": wsID,
		"words": []map[string]any{
			{"surface": "宵闇亭", "pronunciation": "ヨイヤミテイ", "accent_type": 3, "word_type": "PROPER_NOUN"},
			{"surface": "美咲", "pronunciation": "ミサキ", "accent_type": 1, "word_type": "PROPER_NOUN"},
		},
	}, 30*time.Second)
	if err != nil || isErr {
		t.Fatalf("register_dictionary: err=%v isErr=%v body=%s", err, isErr, body)
	}
	t.Logf("register_dictionary: %s", body)

	// --- 4. Batch synthesis ---
	body, isErr, err = h.CallTool("synthesize_script", map[string]any{
		"workspace_id": wsID,
		"script_path":  "script/yoiyami.jsonl",
	}, 60*time.Second)
	if err != nil || isErr {
		t.Fatalf("synthesize_script: err=%v isErr=%v body=%s", err, isErr, body)
	}
	var so struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(body, &so); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(10 * time.Minute)
	var jo struct {
		State    string `json:"state"`
		Done     int    `json:"done"`
		Failed   int    `json:"failed"`
		Failures []any  `json:"failures"`
	}
	for {
		body, isErr, err = h.CallTool("check_job", map[string]any{"job_id": so.JobID}, 30*time.Second)
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
			t.Fatalf("synthesis stuck: %s", body)
		}
		time.Sleep(500 * time.Millisecond)
	}
	if jo.State != "done" || jo.Done != 6 || jo.Failed != 0 {
		t.Fatalf("synthesis job: %s", body)
	}
	t.Logf("synthesis: %d lines done", jo.Done)

	// --- 5. Retake one line with stronger direction ---
	body, isErr, err = h.CallTool("synthesize_line", map[string]any{
		"workspace_id": wsID,
		"line": map[string]any{
			"id": 4, "scene": 2, "speaker": "美咲",
			"text": "そんなの、聞いてないわ!", "style": "強め", "intensity": 1.9, "speed": 1.1,
		},
	}, 5*time.Minute)
	if err != nil || isErr {
		t.Fatalf("retake: err=%v isErr=%v body=%s", err, isErr, body)
	}
	if !strings.Contains(string(body), `"cached":false`) {
		t.Errorf("retake with new direction must re-synthesize: %s", body)
	}

	// --- 6. Master: mp3 and m4b with chapters (real ffmpeg) ---
	for _, format := range []string{"mp3", "m4b"} {
		body, isErr, err = h.CallTool("master", map[string]any{
			"workspace_id": wsID,
			"script_path":  "script/yoiyami.jsonl",
			"format":       format,
		}, 5*time.Minute)
		if err != nil || isErr {
			t.Fatalf("master %s: err=%v isErr=%v body=%s", format, err, isErr, body)
		}
		var mo struct {
			MasterPath      string   `json:"master_path"`
			DurationSeconds float64  `json:"duration_seconds"`
			Chapters        int      `json:"chapters"`
			CreditsPath     string   `json:"credits_path"`
			Credits         []string `json:"credits"`
		}
		if err := json.Unmarshal(body, &mo); err != nil {
			t.Fatal(err)
		}
		fi, err := os.Stat(mo.MasterPath)
		if err != nil {
			t.Fatalf("master %s output: %v", format, err)
		}
		if fi.Size() < 10*1024 {
			t.Errorf("master %s suspiciously small: %d bytes", format, fi.Size())
		}
		if format == "m4b" && mo.Chapters != 2 {
			t.Errorf("m4b chapters: %d", mo.Chapters)
		}
		if len(mo.Credits) == 0 {
			t.Errorf("credits empty")
		}
		t.Logf("master %s: %s (%.1fs, %d bytes, %d chapters)",
			format, mo.MasterPath, mo.DurationSeconds, fi.Size(), mo.Chapters)
		t.Logf("credits: %v", mo.Credits)
	}
}
