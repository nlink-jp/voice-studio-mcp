package tools

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

// fakeFFmpegRunner materializes each ffmpeg output file (last arg).
type fakeFFmpegRunner struct {
	calls int
}

func (f *fakeFFmpegRunner) Run(ctx context.Context, name string, args []string) ([]byte, []byte, int, error) {
	f.calls++
	_ = os.WriteFile(args[len(args)-1], []byte("fake"), 0o644)
	return nil, nil, 0, nil
}

func TestMasterTool(t *testing.T) {
	h := newHarness(t)
	fr := &fakeFFmpegRunner{}
	h.deps.Runner = fr
	h.deps.Cfg.Master.FFmpegPath = "/bin/ls" // exists; fake runner intercepts

	id := h.seedWorkspace("ep1")
	h.seedScript(id, "ep1.jsonl", testScript)

	// Synthesize first (mock engine), then master.
	_, jo := h.runScriptJob(map[string]any{"workspace_id": id, "script_path": "script/ep1.jsonl"})
	if jo.State != "done" || jo.Failed != 0 {
		t.Fatalf("synthesis job: %+v", jo)
	}

	body, isErr := h.callTool("master", map[string]any{
		"workspace_id": id,
		"script_path":  "script/ep1.jsonl",
		"format":       "m4b",
	})
	if isErr {
		t.Fatalf("master error: %s", body)
	}
	var out struct {
		MasterPath       string   `json:"master_path"`
		Format           string   `json:"format"`
		DurationSeconds  float64  `json:"duration_seconds"`
		LinesIncluded    int      `json:"lines_included"`
		Chapters         int      `json:"chapters"`
		CreditsPath      string   `json:"credits_path"`
		Credits          []string `json:"credits"`
		UnverifiedModels []string `json:"unverified_models"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if out.Format != "m4b" || out.LinesIncluded != 3 || out.Chapters != 2 || out.DurationSeconds <= 0 {
		t.Errorf("out: %+v", out)
	}
	if _, err := os.Stat(out.MasterPath); err != nil {
		t.Errorf("master file: %v", err)
	}
	if _, err := os.Stat(out.CreditsPath); err != nil {
		t.Errorf("credits file: %v", err)
	}
	// testCasting: narrator has credit + license_checked, 美咲 has neither.
	if len(out.Credits) != 1 || out.Credits[0] != "AivisSpeech:MockNarrator" {
		t.Errorf("credits: %v", out.Credits)
	}
	if len(out.UnverifiedModels) != 1 || out.UnverifiedModels[0] != "美咲" {
		t.Errorf("unverified: %v", out.UnverifiedModels)
	}
	if fr.calls == 0 {
		t.Errorf("ffmpeg runner never invoked")
	}
}

func TestMasterToolIncompleteSynthesis(t *testing.T) {
	h := newHarness(t)
	h.deps.Runner = &fakeFFmpegRunner{}
	h.deps.Cfg.Master.FFmpegPath = "/bin/ls"

	id := h.seedWorkspace("ep1")
	h.seedScript(id, "ep1.jsonl", testScript)

	body, isErr := h.callTool("master", map[string]any{
		"workspace_id": id,
		"script_path":  "script/ep1.jsonl",
	})
	if !isErr || errCode(t, body) != "master_incomplete" {
		t.Fatalf("expected master_incomplete, got isErr=%v %s", isErr, body)
	}
}

func TestMasterToolRejectsBadFormat(t *testing.T) {
	h := newHarness(t)
	id := h.seedWorkspace("ep1")
	body, isErr := h.callTool("master", map[string]any{
		"workspace_id": id,
		"script_path":  "script/ep1.jsonl",
		"format":       "wav",
	})
	if !isErr || errCode(t, body) != "invalid_arguments" {
		t.Fatalf("expected invalid_arguments, got isErr=%v %s", isErr, body)
	}
}

func TestMasterToolRejectsPathyOutputName(t *testing.T) {
	h := newHarness(t)
	id := h.seedWorkspace("ep1")
	body, isErr := h.callTool("master", map[string]any{
		"workspace_id": id,
		"script_path":  "script/ep1.jsonl",
		"output_name":  "../evil",
	})
	if !isErr || errCode(t, body) != "invalid_arguments" {
		t.Fatalf("expected invalid_arguments, got isErr=%v %s", isErr, body)
	}
}
