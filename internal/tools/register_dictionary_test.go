package tools

import (
	"encoding/json"
	"os"
	"testing"
)

type dictOut struct {
	Registered     int                 `json:"registered"`
	Skipped        int                 `json:"skipped"`
	DictionaryPath string              `json:"dictionary_path"`
	Failed         []map[string]string `json:"failed"`
}

func TestRegisterDictionary(t *testing.T) {
	h := newHarness(t)
	id := h.seedWorkspace("ep1")

	words := []map[string]any{
		{"surface": "美咲", "pronunciation": "ミサキ", "accent_type": 1, "word_type": "PROPER_NOUN"},
		{"surface": "宵闇通り", "pronunciation": "ヨイヤミドオリ", "accent_type": 3},
	}
	body, isErr := h.callTool("register_dictionary", map[string]any{
		"workspace_id": id,
		"words":        words,
	})
	if isErr {
		t.Fatalf("tool error: %s", body)
	}
	var out dictOut
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if out.Registered != 2 || out.Skipped != 0 || len(out.Failed) != 0 {
		t.Errorf("out: %+v", out)
	}
	if len(h.mock.DictWords()) != 2 {
		t.Errorf("engine registrations: %d", len(h.mock.DictWords()))
	}
	// The record file exists and holds both words.
	b, err := os.ReadFile(out.DictionaryPath)
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	var record map[string]map[string]any
	if err := json.Unmarshal(b, &record); err != nil {
		t.Fatal(err)
	}
	if len(record) != 2 || record["美咲"]["pronunciation"] != "ミサキ" {
		t.Errorf("record: %v", record)
	}

	// Re-registering identical words is a no-op for the engine.
	body, isErr = h.callTool("register_dictionary", map[string]any{
		"workspace_id": id,
		"words":        words,
	})
	if isErr {
		t.Fatalf("tool error: %s", body)
	}
	_ = json.Unmarshal(body, &out)
	if out.Registered != 0 || out.Skipped != 2 {
		t.Errorf("idempotent run: %+v", out)
	}
	if len(h.mock.DictWords()) != 2 {
		t.Errorf("engine must not be called again: %d", len(h.mock.DictWords()))
	}

	// Changing a reading re-registers that word only.
	words[0]["pronunciation"] = "ミサキィ"
	body, _ = h.callTool("register_dictionary", map[string]any{
		"workspace_id": id,
		"words":        words,
	})
	_ = json.Unmarshal(body, &out)
	if out.Registered != 1 || out.Skipped != 1 {
		t.Errorf("changed word run: %+v", out)
	}
}

func TestRegisterDictionaryEngineFailureIsPerWord(t *testing.T) {
	h := newHarness(t)
	id := h.seedWorkspace("ep1")
	h.mock.FailNext("/user_dict_word", 500)

	body, isErr := h.callTool("register_dictionary", map[string]any{
		"workspace_id": id,
		"words": []map[string]any{
			{"surface": "壱", "pronunciation": "イチ", "accent_type": 0},
			{"surface": "弐", "pronunciation": "ニ", "accent_type": 0},
		},
	})
	if isErr {
		t.Fatalf("per-word failures must not fail the tool: %s", body)
	}
	var out dictOut
	_ = json.Unmarshal(body, &out)
	if out.Registered != 1 || len(out.Failed) != 1 || out.Failed[0]["surface"] != "壱" {
		t.Errorf("out: %+v", out)
	}
}

func TestRegisterDictionaryEmptyWords(t *testing.T) {
	h := newHarness(t)
	id := h.seedWorkspace("ep1")
	body, isErr := h.callTool("register_dictionary", map[string]any{
		"workspace_id": id,
		"words":        []map[string]any{},
	})
	if !isErr || errCode(t, body) != "missing_argument" {
		t.Errorf("expected missing_argument, got isErr=%v %s", isErr, body)
	}
}
