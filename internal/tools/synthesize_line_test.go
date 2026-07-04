package tools

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/nlink-jp/voice-studio-mcp/internal/engine/enginetest"
)

const testCasting = `
[characters."narrator"]
speaker_uuid = "00000000-0000-0000-0000-0000000000aa"
style_id = 100
credit = "AivisSpeech:MockNarrator"
license_checked = true

[characters."美咲"]
speaker_uuid = "00000000-0000-0000-0000-0000000000bb"
style_id = 200

[characters."美咲".styles]
"悲しみ" = 201
`

// seedWorkspace materializes a workspace with the standard casting table and
// returns its id.
func (h *testHarness) seedWorkspace(id string) string {
	h.t.Helper()
	ws, err := h.deps.WS.Ensure(id)
	if err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(ws.Path("casting.toml"), []byte(testCasting), 0o644); err != nil {
		h.t.Fatal(err)
	}
	return id
}

func TestSynthesizeLineViaCasting(t *testing.T) {
	h := newHarness(t)
	id := h.seedWorkspace("ep1")

	body, isErr := h.callTool("synthesize_line", map[string]any{
		"workspace_id": id,
		"line": map[string]any{
			"id": 42, "speaker": "美咲", "text": "……本当に、行くの?", "style": "悲しみ", "intensity": 1.4,
		},
	})
	if isErr {
		t.Fatalf("tool error: %s", body)
	}
	var out struct {
		LineID          int     `json:"line_id"`
		WavPath         string  `json:"wav_path"`
		DurationSeconds float64 `json:"duration_seconds"`
		Cached          bool    `json:"cached"`
		StyleID         int     `json:"style_id"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if out.StyleID != enginetest.StyleHeroineSad || out.Cached || out.DurationSeconds <= 0 {
		t.Errorf("out: %+v", out)
	}
	if _, err := os.Stat(out.WavPath); err != nil {
		t.Errorf("wav not written: %v", err)
	}
	// The engine saw intensity via intonationScale.
	q := h.mock.SynthesisQueries()[0]
	if q["intonationScale"] != 1.4 {
		t.Errorf("intonationScale: %v", q["intonationScale"])
	}
}

func TestSynthesizeLineStyleIDOverride(t *testing.T) {
	h := newHarness(t)
	id := h.seedWorkspace("ep1")

	// No casting entry for this speaker — style_id bypasses resolution.
	body, isErr := h.callTool("synthesize_line", map[string]any{
		"workspace_id": id,
		"line":         map[string]any{"id": 1, "speaker": "試聴", "text": "声のテスト"},
		"style_id":     enginetest.StyleNarratorCalm,
	})
	if isErr {
		t.Fatalf("tool error: %s", body)
	}
	var out struct {
		StyleID int `json:"style_id"`
	}
	_ = json.Unmarshal(body, &out)
	if out.StyleID != enginetest.StyleNarratorCalm {
		t.Errorf("style: %d", out.StyleID)
	}
}

func TestSynthesizeLineUnresolvedSpeaker(t *testing.T) {
	h := newHarness(t)
	id := h.seedWorkspace("ep1")

	body, isErr := h.callTool("synthesize_line", map[string]any{
		"workspace_id": id,
		"line":         map[string]any{"id": 1, "speaker": "謎の男", "text": "誰だ"},
	})
	if !isErr {
		t.Fatalf("expected error, got %s", body)
	}
	if errCode(t, body) != "casting_unresolved" {
		t.Errorf("code: %s", body)
	}
}

func TestSynthesizeLineInvalidLine(t *testing.T) {
	h := newHarness(t)
	id := h.seedWorkspace("ep1")

	body, isErr := h.callTool("synthesize_line", map[string]any{
		"workspace_id": id,
		"line":         map[string]any{"id": 0, "speaker": "narrator", "text": "idゼロ"},
	})
	if !isErr || errCode(t, body) != "invalid_script" {
		t.Errorf("expected invalid_script, got isErr=%v %s", isErr, body)
	}
}

func TestSynthesizeLineBadWorkspaceID(t *testing.T) {
	h := newHarness(t)
	body, isErr := h.callTool("synthesize_line", map[string]any{
		"workspace_id": "../evil",
		"line":         map[string]any{"id": 1, "speaker": "a", "text": "x"},
	})
	if !isErr || errCode(t, body) != "invalid_workspace_id" {
		t.Errorf("expected invalid_workspace_id, got isErr=%v %s", isErr, body)
	}
}

func TestSynthesizeLineCastingPathEscape(t *testing.T) {
	h := newHarness(t)
	id := h.seedWorkspace("ep1")
	body, isErr := h.callTool("synthesize_line", map[string]any{
		"workspace_id": id,
		"line":         map[string]any{"id": 1, "speaker": "narrator", "text": "x"},
		"casting_path": "../../etc/passwd",
	})
	if !isErr || errCode(t, body) != "path_not_allowed" {
		t.Errorf("expected path_not_allowed, got isErr=%v %s", isErr, body)
	}
}
