package workspace

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nlink-jp/voice-studio-mcp/internal/toolerr"
)

func TestEnsureCreatesLayout(t *testing.T) {
	m := NewManager(allowAll, noFloor)
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
	m := NewManager(allowAll, noFloor)
	if _, err := m.EnsureUnder(t.TempDir(), "../evil"); !errors.Is(err, ErrInvalidID) {
		t.Errorf("expected invalid_workspace_id, got %v", err)
	}
}

func TestResolveInside(t *testing.T) {
	m := NewManager(allowAll, noFloor)
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
	m := NewManager(allowAll, noFloor)
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
	m := NewManager(allowAll, noFloor)
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

	m := NewManager(allowAll, noFloor)
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

// TestAMissingFileNamesThePathItLookedFor: a workspace-relative name that is
// not there has to come back with the absolute path that was looked at. The
// workspace is a level below the work directory the caller named, which is not
// where an agent naturally puts a file; "openat x: no such file or directory"
// sent a real agent off inventing a directory (voice-scribe, 2026-09-14). The
// scribes and image-forge were fixed then; this server — which shares a workspace with video-studio-mcp — was not.
func TestAMissingFileNamesThePathItLookedFor(t *testing.T) {
	workDir := t.TempDir()
	m := NewManager(allowAll, noFloor)
	w, err := m.EnsureUnder(workDir, "drama")
	if err != nil {
		t.Fatal(err)
	}
	for name, call := range map[string]func() error{
		"ReadFile":      func() error { _, err := w.ReadFile("script.jsonl"); return err },
		"Stat":          func() error { _, err := w.Stat("script.jsonl"); return err },
		"VerifyRegular": func() error { return w.VerifyRegular("script.jsonl") },
	} {
		err := call()
		if err == nil {
			t.Fatalf("%s: a file that is not there was found", name)
		}
		if !strings.Contains(err.Error(), w.Path("script.jsonl")) {
			t.Errorf("%s: the error does not name the absolute path it looked for: %q", name, err)
		}
		if !strings.Contains(err.Error(), "<work_dir>/<workspace_id>/") {
			t.Errorf("%s: the error does not say what the name is relative to: %q", name, err)
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s: the error stopped being fs.ErrNotExist: %v", name, err)
		}
	}
}

// TestEnsureRefusesLinkedWorkspaceDir pins the containment property that
// os.Root alone cannot give: <work_dir>/<id> itself must be a real directory.
// TestSymlinkContainment above covers links planted *inside* a workspace;
// os.Root contains operations within a root but resolves the root path
// normally, so a link pre-planted at <work_dir>/<id> — by any other tool with
// write access to work_dir — made os.OpenRoot(w.BaseDir) anchor on the link's
// target, and every read and write then landed outside work_dir while
// reporting success.
func TestEnsureRefusesLinkedWorkspaceDir(t *testing.T) {
	// EvalSymlinks first: on macOS t.TempDir() sits under /var, which is
	// itself a link to /private/var, so a raw comparison would never match.
	workDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outside, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workDir, "ep1")); err != nil {
		t.Fatal(err)
	}

	w, err := NewManager(allowAll, noFloor).EnsureUnder(workDir, "ep1")
	if err == nil {
		t.Fatalf("a linked workspace dir was accepted: base=%s", w.BaseDir)
	}
	if !strings.Contains(err.Error(), "ep1") {
		t.Errorf("the refusal does not name the workspace id: %q", err)
	}
	if !strings.Contains(err.Error(), outside) {
		t.Errorf("the refusal does not name what the id resolved to: %q", err)
	}

	// Nothing may have been created or written through the link.
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("the link's target was written through: %v", names)
	}
}

// allowAll stands for the server's check in tests of the manager's own
// mechanics; the check itself is workdir.Resolver.CheckBeneath's.
func allowAll(string) error { return nil }

// The directory actually used is judged before it is made: the check sees
// <work_dir>/<workspace_id>, and its refusal is returned as is, with nothing
// created. A Manager without a check refuses every workspace.
func TestEnsureUnderJudgesTheWorkspaceDirectoryBeforeMakingIt(t *testing.T) {
	work := t.TempDir()
	refusal := errors.New("refused")
	var seen string
	m := NewManager(func(dir string) error { seen = dir; return refusal }, noFloor)
	if _, err := m.EnsureUnder(work, "gh"); !errors.Is(err, refusal) {
		t.Fatalf("EnsureUnder = %v, want the check's refusal", err)
	}
	if want := filepath.Join(work, "gh"); seen != want {
		t.Errorf("the check saw %q, want %q", seen, want)
	}
	if _, err := os.Stat(filepath.Join(work, "gh")); !os.IsNotExist(err) {
		t.Errorf("a refused workspace was created (stat: %v)", err)
	}
	for name, m := range map[string]*Manager{"no check": NewManager(nil, noFloor), "zero": {}} {
		if _, err := m.EnsureUnder(work, "ws"); err == nil {
			t.Errorf("%s: EnsureUnder succeeded", name)
		}
	}
}

// noFloor stands for the server's floor in tests of the manager's own
// mechanics; the floor itself is workdir.Resolver.LocalPath's.
func noFloor(string, string) string { return "" }

// Every read through a workspace — ReadFile, Stat, VerifyRegular — is judged
// by the floor before anything is read or looked for: a path the floor refuses
// gets the same refusal whether or not a file is there, and it names the path
// only as given. A Manager without a floor refuses every workspace.
func TestEveryReadIsJudgedBeforeItLooks(t *testing.T) {
	work := t.TempDir()
	floor := func(raw, _ string) string {
		if strings.HasSuffix(raw, ".secret") {
			return "a test floor refuses it"
		}
		return ""
	}
	w, err := NewManager(allowAll, floor).EnsureUnder(work, "ws")
	if err != nil {
		t.Fatal(err)
	}
	rel := filepath.Join(DirWav, "1.secret")
	reads := map[string]func() error{
		"ReadFile":      func() error { _, err := w.ReadFile(rel); return err },
		"Stat":          func() error { _, err := w.Stat(rel); return err },
		"VerifyRegular": func() error { return w.VerifyRegular(rel) },
	}
	for name, read := range reads {
		if err := os.WriteFile(w.Path(rel), []byte("SECRET"), 0o600); err != nil {
			t.Fatal(err)
		}
		e := read()
		if err := os.Remove(w.Path(rel)); err != nil {
			t.Fatal(err)
		}
		m := read()
		if !errors.Is(e, toolerr.New(toolerr.CodePathNotAllowed, "")) || e.Error() != fmt.Sprint(m) {
			t.Errorf("%s: existing %v, missing %v; want the same path_not_allowed", name, e, m)
		}
	}
	if _, err := w.ReadFile(filepath.Join(DirWav, "ok.wav")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("an ordinary missing file: %v, want not found", err)
	}
	for name, m := range map[string]*Manager{"no floor": NewManager(allowAll, nil)} {
		if _, err := m.EnsureUnder(work, "ws2"); err == nil {
			t.Errorf("%s: EnsureUnder succeeded", name)
		}
	}
	if _, err := (&Workspace{ID: "x", BaseDir: w.BaseDir}).ReadFile("script/a.jsonl"); !errors.Is(err, toolerr.New(toolerr.CodePathNotAllowed, "")) {
		t.Errorf("a Workspace without a floor read: %v", err)
	}
}

// A path is judged once per Workspace — one step of work — however often it is
// read: master reads each WAV and then verifies it, and every judgement walks
// the credential directories.
func TestAPathIsJudgedOncePerWorkspace(t *testing.T) {
	calls := 0
	w, err := NewManager(allowAll, func(string, string) string { calls++; return "" }).EnsureUnder(t.TempDir(), "ws")
	if err != nil {
		t.Fatal(err)
	}
	rel := filepath.Join(DirWav, "1.wav")
	if err := os.WriteFile(w.Path(rel), []byte("RIFF"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := calls
	_, _ = w.ReadFile(rel)
	_ = w.VerifyRegular(rel)
	_, _ = w.Stat(rel)
	if got := calls - before; got != 1 {
		t.Errorf("three reads of one path judged it %d times, want once", got)
	}
}

// PlaceFile writes through a temporary name next to the destination: a link a
// caller plants at that name, or at the destination, is replaced — the file it
// points at, outside the workspace, is never written.
func TestPlaceFileReplacesLinksItFinds(t *testing.T) {
	w, err := NewManager(allowAll, noFloor).EnsureUnder(t.TempDir(), "ep01")
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "rendered.mp3")
	if err := os.WriteFile(src, []byte("RENDERED"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(w.Path(DirMaster), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, planted := range []string{"ep01.mp3.tmp", "ep01.mp3"} {
		victim := filepath.Join(t.TempDir(), "victim")
		if err := os.WriteFile(victim, []byte("keep me"), 0o644); err != nil {
			t.Fatal(err)
		}
		link := w.Path(DirMaster, planted)
		_ = os.Remove(link)
		if err := os.Symlink(victim, link); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		if err := w.PlaceFile(filepath.Join(DirMaster, "ep01.mp3"), src); err != nil {
			t.Fatalf("%s planted: PlaceFile: %v", planted, err)
		}
		if b, _ := os.ReadFile(victim); string(b) != "keep me" {
			t.Errorf("%s planted: the link's target was written: %q", planted, b)
		}
		fi, err := os.Lstat(w.Path(DirMaster, "ep01.mp3"))
		if err != nil || !fi.Mode().IsRegular() {
			t.Fatalf("%s planted: the destination is not a regular file: %v %v", planted, fi, err)
		}
		if b, _ := os.ReadFile(w.Path(DirMaster, "ep01.mp3")); string(b) != "RENDERED" {
			t.Errorf("%s planted: destination holds %q", planted, b)
		}
	}
}
