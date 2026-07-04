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

	// Resolved paths stay under the base dir.
	p, err := w.ResolveInside("script/ep1.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if p != filepath.Join(w.BaseDir, "script", "ep1.jsonl") {
		t.Errorf("resolved path: %s", p)
	}
}
