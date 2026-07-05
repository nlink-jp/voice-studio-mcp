package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedProjectWorkspace prepares an agent-style workspace under an arbitrary
// (project) directory and returns its root.
func (h *testHarness) seedProjectWorkspace(t *testing.T, wsID string) string {
	t.Helper()
	root := t.TempDir() // stands in for a project directory the agent can write
	if err := os.MkdirAll(filepath.Join(root, wsID, "script"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, wsID, "casting.toml"), []byte(testCasting), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, wsID, "script", "ep1.jsonl"), []byte(testScript), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestWorkspaceRootEndToEnd pins ADR-0010's core: the server works in the
// workplace the agent prepared.
func TestWorkspaceRootEndToEnd(t *testing.T) {
	h := newHarness(t)
	root := h.seedProjectWorkspace(t, "ep1")

	so, jo := h.runScriptJob(map[string]any{
		"workspace_id":   "ep1",
		"workspace_root": root,
		"script_path":    "script/ep1.jsonl",
	})
	if jo.State != "done" || jo.Done != 3 || jo.Failed != 0 {
		t.Fatalf("job: %+v", jo)
	}
	// Outputs landed inside the agent-prepared root, not the default root.
	if !strings.HasPrefix(so.OutputDir, root) {
		t.Errorf("output dir %q not under workspace_root %q", so.OutputDir, root)
	}
	for _, n := range []string{"1.wav", "2.wav", "3.wav"} {
		if _, err := os.Stat(filepath.Join(root, "ep1", "wav", n)); err != nil {
			t.Errorf("missing %s under workspace_root: %v", n, err)
		}
	}
	if entries, _ := os.ReadDir(filepath.Join(h.deps.WS.Root(), "ep1", "wav")); len(entries) != 0 {
		t.Errorf("default root must stay untouched, found %d files", len(entries))
	}

	// Retake and mastering follow the same root.
	body, isErr := h.callTool("synthesize_line", map[string]any{
		"workspace_id":   "ep1",
		"workspace_root": root,
		"line":           map[string]any{"id": 2, "speaker": "美咲", "text": "……本当に、行くの?", "style": "悲しみ", "intensity": 1.8},
	})
	if isErr {
		t.Fatalf("retake: %s", body)
	}
	var lr struct {
		WavPath string `json:"wav_path"`
	}
	_ = json.Unmarshal(body, &lr)
	if !strings.HasPrefix(lr.WavPath, root) {
		t.Errorf("retake wav %q not under workspace_root", lr.WavPath)
	}
}

func TestWorkspaceRootValidation(t *testing.T) {
	h := newHarness(t)
	for _, tc := range []struct {
		name string
		root string
	}{
		{"relative", "relative/path"},
		{"missing", filepath.Join(t.TempDir(), "nope")},
	} {
		body, isErr := h.callTool("synthesize_script", map[string]any{
			"workspace_id":   "ep1",
			"workspace_root": tc.root,
			"script_path":    "script/ep1.jsonl",
		})
		if !isErr || errCode(t, body) != "path_not_allowed" {
			t.Errorf("%s: expected path_not_allowed, got isErr=%v %s", tc.name, isErr, body)
		}
	}
}

// TestWorkspaceRootSymlinkRegression pins the confused-deputy defense: an
// agent-writable workspace whose files are symlinks out of the workspace
// must be rejected, not followed.
func TestWorkspaceRootSymlinkRegression(t *testing.T) {
	h := newHarness(t)
	root := h.seedProjectWorkspace(t, "ep1")

	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.jsonl"), []byte(testScript), 0o644); err != nil {
		t.Fatal(err)
	}

	// Script replaced by a symlink pointing outside the workspace.
	link := filepath.Join(root, "ep1", "script", "evil.jsonl")
	if err := os.Symlink(filepath.Join(outside, "secret.jsonl"), link); err != nil {
		t.Fatal(err)
	}
	body, isErr := h.callTool("synthesize_script", map[string]any{
		"workspace_id":   "ep1",
		"workspace_root": root,
		"script_path":    "script/evil.jsonl",
	})
	if !isErr || errCode(t, body) != "path_not_allowed" {
		t.Fatalf("symlinked script: expected path_not_allowed, got isErr=%v %s", isErr, body)
	}

	// casting.toml replaced by an out-of-workspace symlink.
	if err := os.Remove(filepath.Join(root, "ep1", "casting.toml")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "casting.toml"), []byte(testCasting), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "casting.toml"), filepath.Join(root, "ep1", "casting.toml")); err != nil {
		t.Fatal(err)
	}
	body, isErr = h.callTool("synthesize_script", map[string]any{
		"workspace_id":   "ep1",
		"workspace_root": root,
		"script_path":    "script/ep1.jsonl",
	})
	if !isErr || errCode(t, body) != "path_not_allowed" {
		t.Fatalf("symlinked casting: expected path_not_allowed, got isErr=%v %s", isErr, body)
	}
}

func TestGetUsage(t *testing.T) {
	h := newHarness(t)
	body, isErr := h.callTool("get_usage", map[string]any{})
	if isErr {
		t.Fatalf("get_usage error: %s", body)
	}
	// RawResult returns the markdown directly (not JSON-wrapped).
	s := string(body)
	for _, want := range []string{"workspace_root", "Production flow", "Error recovery", "casting.toml"} {
		if !strings.Contains(s, want) {
			t.Errorf("usage missing %q", want)
		}
	}
}

// TestUsageCoherence pins usage.md against the real server surface, the
// same way skills_test.go pins the bundled skill (ADR-0009 mechanism).
func TestUsageCoherence(t *testing.T) {
	for _, tool := range []string{
		"get_usage", "list_speakers", "register_dictionary",
		"synthesize_script", "synthesize_line", "check_job", "master",
	} {
		if tool == "get_usage" {
			continue // the manual need not reference itself
		}
		if !strings.Contains(usageMarkdown, "`"+tool+"`") {
			t.Errorf("usage.md does not reference tool %q", tool)
		}
	}
	for _, code := range []string{
		"invalid_script", "casting_unresolved", "engine_unavailable",
		"engine_request_failed", "job_not_found", "master_incomplete",
		"ffmpeg_not_found", "path_not_allowed", "invalid_workspace_id",
	} {
		if !strings.Contains(usageMarkdown, code) {
			t.Errorf("usage.md recovery table missing %q", code)
		}
	}
	for _, field := range []string{`"id"`, `"scene"`, `"speaker"`, `"text"`, `"style"`, `"intensity"`, `"speed"`, `"volume"`, `"pause_after_ms"`} {
		if !strings.Contains(usageMarkdown, field) {
			t.Errorf("usage.md schema example missing %s", field)
		}
	}
	if !strings.Contains(Instructions, "get_usage") {
		t.Errorf("initialize instructions must point at get_usage")
	}
}
