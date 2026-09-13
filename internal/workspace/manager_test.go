package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nlink-jp/voice-studio-mcp/internal/toolerr"
)

func TestEnsureCreatesLayout(t *testing.T) {
	m := NewManager()
	w, err := m.EnsureUnder(t.TempDir(), "ep01")
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	for _, d := range []string{DirScript, DirDict, DirWav, DirCache, DirMaster} {
		if fi, err := os.Stat(w.Path(d)); err != nil || !fi.IsDir() {
			t.Errorf("subdir %s missing: err=%v", d, err)
		}
	}
	// Idempotent.
	if _, err := m.EnsureUnder(t.TempDir(), "ep01"); err != nil {
		t.Errorf("second ensure: %v", err)
	}
}

func TestEnsureRejectsBadID(t *testing.T) {
	m := NewManager()
	if _, err := m.EnsureUnder(t.TempDir(), "../evil"); !errors.Is(err, ErrInvalidID) {
		t.Errorf("expected invalid_workspace_id, got %v", err)
	}
}

func TestResolveInside(t *testing.T) {
	m := NewManager()
	w, err := m.EnsureUnder(t.TempDir(), "ws")
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

// EnsureUnder takes a work directory that internal/workdir has already
// validated; what it still owns is the workspace id and the refusal to invent
// the parent (organization ADR-021 §4).
func TestEnsureUnderValidation(t *testing.T) {
	m := NewManager()
	if _, err := m.EnsureUnder("relative/path", "ep1"); err == nil {
		t.Error("a relative work_dir must be refused")
	}
	missing := filepath.Join(t.TempDir(), "not-there")
	if _, err := m.EnsureUnder(missing, "ep1"); err == nil {
		t.Error("a missing work_dir must be refused, not created")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Errorf("work_dir was created: %v", err)
	}
}

func TestRootHelpersRoundTrip(t *testing.T) {
	m := NewManager()
	w, err := m.EnsureUnder(t.TempDir(), "io")
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

	m := NewManager()
	w, err := m.EnsureUnder(t.TempDir(), "evil")
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
