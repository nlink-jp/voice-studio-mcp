//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestE2E_RealEngine drives an actual AivisSpeech Engine in managed mode.
// It is opt-in because it needs AivisSpeech installed and takes minutes on
// first model load:
//
//	VOICE_STUDIO_TEST_REAL_ENGINE=1 make test-e2e
func TestE2E_RealEngine(t *testing.T) {
	if os.Getenv("VOICE_STUDIO_TEST_REAL_ENGINE") != "1" {
		t.Skip("set VOICE_STUDIO_TEST_REAL_ENGINE=1 to run against a real AivisSpeech Engine")
	}
	binary, ok := requireBinary(t)
	if !ok {
		return
	}

	root := t.TempDir()
	configPath := filepath.Join(root, "config.toml")
	cfg := fmt.Sprintf(`
[workspace]
workspace_dir = %q

[engine]
mode = "managed"
`, filepath.Join(root, "workspaces"))
	if err := os.WriteFile(configPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	h := Start(t, binary, configPath)
	// serve answers initialize only after the engine is ready; a cold engine
	// (first model load) can take minutes.
	if err := h.InitializeWithTimeout(4 * time.Minute); err != nil {
		t.Fatalf("initialize (engine startup can take minutes on first run): %v", err)
	}

	// Discover a real style id.
	body, isErr, err := h.CallTool("list_speakers", map[string]any{}, 60*time.Second)
	if err != nil || isErr {
		t.Fatalf("list_speakers: err=%v isErr=%v body=%s", err, isErr, body)
	}
	var speakers struct {
		Speakers []struct {
			Name   string `json:"name"`
			Styles []struct {
				ID int `json:"id"`
			} `json:"styles"`
		} `json:"speakers"`
	}
	if err := json.Unmarshal(body, &speakers); err != nil {
		t.Fatal(err)
	}
	if len(speakers.Speakers) == 0 || len(speakers.Speakers[0].Styles) == 0 {
		t.Fatalf("no speakers installed: %s", body)
	}
	styleID := speakers.Speakers[0].Styles[0].ID
	t.Logf("using speaker %q style %d", speakers.Speakers[0].Name, styleID)

	// Synthesize one real line via style override (no casting table needed).
	body, isErr, err = h.CallTool("synthesize_line", map[string]any{
		"workspace_id": "real-engine-check",
		"line":         map[string]any{"id": 1, "speaker": "check", "text": "音声合成の動作確認です。"},
		"style_id":     styleID,
	}, 5*time.Minute)
	if err != nil || isErr {
		t.Fatalf("synthesize_line: err=%v isErr=%v body=%s", err, isErr, body)
	}
	var out struct {
		WavPath         string  `json:"wav_path"`
		DurationSeconds float64 `json:"duration_seconds"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	wav, err := os.ReadFile(out.WavPath)
	if err != nil {
		t.Fatalf("read wav: %v", err)
	}
	if len(wav) < 44 || string(wav[0:4]) != "RIFF" {
		t.Errorf("not a WAV (%d bytes)", len(wav))
	}
	if out.DurationSeconds <= 0.2 {
		t.Errorf("suspiciously short synthesis: %vs", out.DurationSeconds)
	}
	t.Logf("synthesized %s (%.2fs)", out.WavPath, out.DurationSeconds)
}
