package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nlink-jp/voice-studio-mcp/internal/toolerr"
)

func TestEnsureCreatesLayout(t *testing.T) {
	m := NewManager(t.TempDir())
	w, err := m.Ensure("ep01")
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	for _, d := range []string{DirScript, DirDict, DirWav, DirCache, DirMaster} {
		if fi, err := os.Stat(w.Path(d)); err != nil || !fi.IsDir() {
			t.Errorf("subdir %s missing: err=%v", d, err)
		}
	}
	// Idempotent.
	if _, err := m.Ensure("ep01"); err != nil {
		t.Errorf("second ensure: %v", err)
	}
}

func TestEnsureRejectsBadID(t *testing.T) {
	m := NewManager(t.TempDir())
	if _, err := m.Ensure("../evil"); !errors.Is(err, ErrInvalidID) {
		t.Errorf("expected invalid_workspace_id, got %v", err)
	}
}

func TestListReturnsSortedIDs(t *testing.T) {
	root := t.TempDir()
	m := NewManager(root)
	for _, id := range []string{"zeta", "alpha"} {
		if _, err := m.Ensure(id); err != nil {
			t.Fatal(err)
		}
	}
	// Stray non-workspace entries are ignored.
	if err := os.WriteFile(filepath.Join(root, "stray.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ids, err := m.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(ids) != 2 || ids[0] != "alpha" || ids[1] != "zeta" {
		t.Errorf("ids: %v", ids)
	}
}

func TestListMissingRootIsEmpty(t *testing.T) {
	m := NewManager(filepath.Join(t.TempDir(), "does-not-exist"))
	ids, err := m.List()
	if err != nil || len(ids) != 0 {
		t.Errorf("ids=%v err=%v", ids, err)
	}
}

func TestDeleteRemovesWorkspace(t *testing.T) {
	m := NewManager(t.TempDir())
	w, err := m.Ensure("gone")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Delete("gone"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(w.BaseDir); !os.IsNotExist(err) {
		t.Errorf("workspace dir should be removed, err=%v", err)
	}
}

func TestDeleteRejectsBadID(t *testing.T) {
	m := NewManager(t.TempDir())
	if err := m.Delete("a/b"); !errors.Is(err, ErrInvalidID) {
		t.Errorf("expected invalid_workspace_id, got %v", err)
	}
}

func TestResolveInside(t *testing.T) {
	m := NewManager(t.TempDir())
	w, err := m.Ensure("ws")
	if err != nil {
		t.Fatal(err)
	}

	ok := []string{"script/ep1.jsonl", "casting.toml", "wav/1.wav", "a/b/../c"}
	for _, rel := range ok {
		if _, err := w.ResolveInside(rel); err != nil {
			t.Errorf("ResolveInside(%q) unexpected error: %v", rel, err)
		}
	}

	bad := []string{"", "../other", "script/../../other", "/etc/passwd"}
	sentinel := toolerr.New(toolerr.CodePathNotAllowed, "")
	for _, rel := range bad {
		if _, err := w.ResolveInside(rel); !errors.Is(err, sentinel) {
			t.Errorf("ResolveInside(%q) should be path_not_allowed, got %v", rel, err)
		}
	}

	// ResolveInside returns cleaned workspace-relative paths.
	p, err := w.ResolveInside("a/b/../c")
	if err != nil {
		t.Fatal(err)
	}
	if p != filepath.Join("a", "c") {
		t.Errorf("resolved rel path: %s", p)
	}
}

func TestEnsureInValidation(t *testing.T) {
	m := NewManager(t.TempDir())
	sentinel := toolerr.New(toolerr.CodePathNotAllowed, "")

	if _, err := m.EnsureIn("relative/path", "ws"); !errors.Is(err, sentinel) {
		t.Errorf("relative workspace_root should be rejected: %v", err)
	}
	if _, err := m.EnsureIn(filepath.Join(t.TempDir(), "missing"), "ws"); !errors.Is(err, sentinel) {
		t.Errorf("missing workspace_root should be rejected: %v", err)
	}
	file := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := m.EnsureIn(file, "ws"); !errors.Is(err, sentinel) {
		t.Errorf("non-directory workspace_root should be rejected: %v", err)
	}

	// Happy path: an agent-prepared directory works and gets the layout.
	prepared := t.TempDir()
	w, err := m.EnsureIn(prepared, "ep1")
	if err != nil {
		t.Fatalf("EnsureIn: %v", err)
	}
	if w.BaseDir != filepath.Join(prepared, "ep1") {
		t.Errorf("base dir: %s", w.BaseDir)
	}
	for _, d := range []string{DirScript, DirWav, DirMaster} {
		if fi, err := os.Stat(w.Path(d)); err != nil || !fi.IsDir() {
			t.Errorf("subdir %s: %v", d, err)
		}
	}
	// Empty root falls back to the default root.
	w2, err := m.EnsureIn("", "ep1")
	if err != nil || w2.BaseDir != filepath.Join(m.Root(), "ep1") {
		t.Errorf("fallback: %+v err=%v", w2, err)
	}
}

func TestRootHelpersRoundTrip(t *testing.T) {
	m := NewManager(t.TempDir())
	w, err := m.Ensure("io")
	if err != nil {
		t.Fatal(err)
	}
	if err := w.WriteFileAtomic("cache/index.json", []byte(`{"a":1}`)); err != nil {
		t.Fatalf("write: %v", err)
	}
	b, err := w.ReadFile("cache/index.json")
	if err != nil || string(b) != `{"a":1}` {
		t.Fatalf("read: %q err=%v", b, err)
	}
	if _, err := w.Stat("cache/index.json"); err != nil {
		t.Errorf("stat: %v", err)
	}
	if err := w.VerifyRegular("cache/index.json"); err != nil {
		t.Errorf("verify regular: %v", err)
	}
	if err := w.RemoveAll("cache"); err != nil {
		t.Errorf("removeall: %v", err)
	}
	if _, err := w.Stat("cache/index.json"); err == nil {
		t.Errorf("file should be gone")
	}
}

// TestSymlinkContainment pins the ADR-0010 security property: symlinks
// planted inside an (agent-writable) workspace cannot make the server read
// or write outside it.
func TestSymlinkContainment(t *testing.T) {
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("outside data"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := NewManager(t.TempDir())
	w, err := m.Ensure("evil")
	if err != nil {
		t.Fatal(err)
	}
	sentinel := toolerr.New(toolerr.CodePathNotAllowed, "")

	// Symlinked file pointing outside → reads fail with path_not_allowed.
	if err := os.Symlink(secret, w.Path("script", "link.jsonl")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.ReadFile("script/link.jsonl"); !errors.Is(err, sentinel) {
		t.Errorf("read through symlink should be path_not_allowed, got %v", err)
	}
	if err := w.VerifyRegular("script/link.jsonl"); !errors.Is(err, sentinel) {
		t.Errorf("VerifyRegular on symlink should be path_not_allowed, got %v", err)
	}

	// Whole directory replaced by a symlink → writes fail and never land outside.
	if err := os.RemoveAll(w.Path(DirWav)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, w.Path(DirWav)); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteFileAtomic("wav/10.wav", []byte("RIFF")); !errors.Is(err, sentinel) {
		t.Errorf("write through symlinked dir should be path_not_allowed, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "10.wav")); err == nil {
		t.Errorf("write escaped the workspace root")
	}
	if _, err := os.Stat(filepath.Join(outside, "10.wav.tmp")); err == nil {
		t.Errorf("temp write escaped the workspace root")
	}
}
