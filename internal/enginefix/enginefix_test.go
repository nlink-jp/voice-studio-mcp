package enginefix

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRunner records commands and returns programmed results.
type fakeRunner struct {
	calls        [][]string
	quarantined  map[string]bool // paths reported as quarantined by `xattr`
	verifyOK     bool            // codesign --verify result
	failCodesign string          // path whose codesign should fail ("" = none)
}

func (f *fakeRunner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, int, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	switch name {
	case "xattr":
		if len(args) == 1 { // `xattr <path>` — list attrs
			if f.quarantined[args[0]] {
				return []byte("com.apple.quarantine\n"), nil, 0, nil
			}
			return nil, nil, 0, nil
		}
		return nil, nil, 0, nil // `xattr -dr ...`
	case "codesign":
		if args[0] == "--verify" {
			if f.verifyOK {
				return nil, nil, 0, nil
			}
			return nil, []byte("code object is not signed at all"), 1, nil
		}
		// --force --sign - <path>
		p := args[len(args)-1]
		if f.failCodesign != "" && p == f.failCodesign {
			return nil, []byte("errSecInternalComponent"), 1, nil
		}
		return nil, nil, 0, nil
	}
	return nil, nil, 0, nil
}

// writeMachO writes a file with a Mach-O 64-bit magic header.
func writeMachO(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	var b [8]byte
	binary.BigEndian.PutUint32(b[:4], 0xFEEDFACF)
	if err := os.WriteFile(path, b[:], 0o755); err != nil {
		t.Fatal(err)
	}
}

// seedBundle builds an engine subtree: run executable + nested dylibs + a
// non-Mach-O text file that must be ignored.
func seedBundle(t *testing.T) (engineCmd string, dir string) {
	t.Helper()
	dir = filepath.Join(t.TempDir(), "AivisSpeech-Engine")
	engineCmd = filepath.Join(dir, "run")
	writeMachO(t, engineCmd)
	writeMachO(t, filepath.Join(dir, "_internal", "libpython.dylib"))
	writeMachO(t, filepath.Join(dir, "_internal", "onnxruntime", "libonnxruntime.dylib"))
	if err := os.WriteFile(filepath.Join(dir, "README.txt"), []byte("not mach-o"), 0o644); err != nil {
		t.Fatal(err)
	}
	return engineCmd, dir
}

func TestFindMachOIgnoresNonBinaries(t *testing.T) {
	_, dir := seedBundle(t)
	machos, err := findMachO(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(machos) != 3 {
		t.Fatalf("expected 3 Mach-O files, got %d: %v", len(machos), machos)
	}
	for _, p := range machos {
		if strings.HasSuffix(p, "README.txt") {
			t.Errorf("text file misdetected as Mach-O")
		}
	}
}

func TestDetectNeedsFix(t *testing.T) {
	engineCmd, dir := seedBundle(t)

	// Quarantined + unsigned → needs fix.
	fr := &fakeRunner{quarantined: map[string]bool{dir: true}, verifyOK: false}
	f := &Fixer{Runner: fr}
	rep, err := f.Detect(context.Background(), engineCmd)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Quarantined || rep.SignatureOK || !rep.NeedsFix() {
		t.Errorf("report: %+v", rep)
	}
	if rep.MachOFiles != 3 {
		t.Errorf("macho count: %d", rep.MachOFiles)
	}

	// Clean: no quarantine, verify passes → no fix.
	fr = &fakeRunner{verifyOK: true}
	f = &Fixer{Runner: fr}
	rep, _ = f.Detect(context.Background(), engineCmd)
	if rep.NeedsFix() {
		t.Errorf("clean bundle should not need fix: %+v", rep)
	}

	// Verify fails even without quarantine (the level-2 signature soup) → fix.
	fr = &fakeRunner{verifyOK: false}
	f = &Fixer{Runner: fr}
	rep, _ = f.Detect(context.Background(), engineCmd)
	if !rep.NeedsFix() {
		t.Errorf("bad signature must need fix: %+v", rep)
	}
}

func TestRepairStripsQuarantineAndSignsInsideOut(t *testing.T) {
	engineCmd, dir := seedBundle(t)
	fr := &fakeRunner{}
	f := &Fixer{Runner: fr}

	rep, err := f.Repair(context.Background(), engineCmd)
	if err != nil {
		t.Fatalf("repair: %v", err)
	}
	if !rep.QuarantineOff || len(rep.Resigned) != 3 {
		t.Errorf("report: %+v", rep)
	}

	// Quarantine stripped recursively over the whole dir.
	foundStrip := false
	var signOrder []string
	for _, c := range fr.calls {
		if c[0] == "xattr" && len(c) == 4 && c[1] == "-dr" && c[2] == "com.apple.quarantine" && c[3] == dir {
			foundStrip = true
		}
		if c[0] == "codesign" && c[1] == "--force" {
			signOrder = append(signOrder, c[len(c)-1])
		}
	}
	if !foundStrip {
		t.Errorf("quarantine not stripped recursively over %s: %v", dir, fr.calls)
	}
	// Deeper libs signed before shallower ones; the executable signed last.
	if len(signOrder) != 3 {
		t.Fatalf("sign calls: %v", signOrder)
	}
	if signOrder[len(signOrder)-1] != engineCmd {
		t.Errorf("executable must be signed last, order: %v", signOrder)
	}
	deepest := filepath.Join(dir, "_internal", "onnxruntime", "libonnxruntime.dylib")
	if signOrder[0] != deepest {
		t.Errorf("deepest lib must be signed first, order: %v", signOrder)
	}
	// Ad-hoc identity.
	for _, c := range fr.calls {
		if c[0] == "codesign" && c[1] == "--force" {
			if c[2] != "--sign" || c[3] != "-" {
				t.Errorf("must sign ad-hoc (--sign -): %v", c)
			}
		}
	}
}

func TestRepairSurfacesCodesignFailure(t *testing.T) {
	engineCmd, dir := seedBundle(t)
	fr := &fakeRunner{failCodesign: filepath.Join(dir, "_internal", "libpython.dylib")}
	f := &Fixer{Runner: fr}
	if _, err := f.Repair(context.Background(), engineCmd); err == nil {
		t.Errorf("codesign failure must surface")
	}
}

func TestRepairEmptyBundle(t *testing.T) {
	dir := t.TempDir()
	f := &Fixer{Runner: &fakeRunner{}}
	if _, err := f.Repair(context.Background(), filepath.Join(dir, "run")); err == nil {
		t.Errorf("empty bundle (no Mach-O) should error")
	}
}
