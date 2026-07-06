// Package enginefix repairs the code-signature state of a locally installed
// AivisSpeech Engine so it can be spawned as a child process.
//
// The engine ships as an unsigned/un-notarized PyInstaller bundle whose
// nested native libraries (onnxruntime, numpy, …) carry signatures from
// different TeamIDs. On modern macOS this "signature soup" fails at load
// time (Library Validation / cdhash / TeamID-mismatch), and removing the
// quarantine flag alone is not enough. The proven manual fix is to strip
// quarantine and re-sign the whole subtree ad-hoc so every Mach-O carries
// one uniform (null-TeamID) signature — which is exactly what this package
// automates (ADR-0012).
//
// Scope is deliberately the engine subtree only (the directory that contains
// the `run` executable), never the wider .app; the server only spawns the
// engine, and roaming would be surprising. Everything runs through a Runner
// so tests are hermetic (no real codesign / macOS required).
package enginefix

import (
	"context"
	"encoding/binary"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Runner executes an external command (codesign, xattr). It exists so tests
// can fake macOS tooling; production uses ExecRunner.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (stdout, stderr []byte, exitCode int, err error)
}

// Report describes what a Detect/Repair pass found or did.
type Report struct {
	Dir           string
	MachOFiles    int
	Quarantined   bool     // any quarantine xattr present in the subtree
	SignatureOK   bool     // `codesign --verify` on the executable passed
	Resigned      []string // Mach-O paths ad-hoc re-signed (Repair only)
	QuarantineOff bool     // quarantine stripped (Repair only)
	Executable    string   // the engine executable within Dir
}

// Fixer inspects and repairs an engine bundle subtree.
type Fixer struct {
	Runner Runner
}

// BundleDir returns the engine subtree to operate on: the directory holding
// the engine executable (e.g. .../AivisSpeech-Engine).
func BundleDir(engineCommand string) string {
	return filepath.Dir(engineCommand)
}

// Detect reports whether the engine at engineCommand looks like it would
// fail to spawn for signature reasons: a quarantine flag anywhere in the
// subtree, or a failing signature verification on the executable.
func (f *Fixer) Detect(ctx context.Context, engineCommand string) (Report, error) {
	dir := BundleDir(engineCommand)
	r := Report{Dir: dir, Executable: engineCommand}

	machos, err := findMachO(dir)
	if err != nil {
		return r, err
	}
	r.MachOFiles = len(machos)

	r.Quarantined = f.anyQuarantined(ctx, dir, machos)
	r.SignatureOK = f.verify(ctx, engineCommand)
	return r, nil
}

// NeedsFix reports whether Detect's result indicates a spawn-blocking state.
func (r Report) NeedsFix() bool {
	return r.Quarantined || !r.SignatureOK
}

// Repair strips quarantine from the subtree and ad-hoc re-signs every Mach-O
// inside-out (deepest paths first, executable last) so signatures are
// uniform. Idempotent.
func (f *Fixer) Repair(ctx context.Context, engineCommand string) (Report, error) {
	dir := BundleDir(engineCommand)
	r := Report{Dir: dir, Executable: engineCommand}

	machos, err := findMachO(dir)
	if err != nil {
		return r, err
	}
	r.MachOFiles = len(machos)
	if len(machos) == 0 {
		return r, fmt.Errorf("no Mach-O files under %s — is this the engine bundle?", dir)
	}

	// 1. Strip quarantine recursively.
	if _, stderr, code, err := f.Runner.Run(ctx, "xattr", "-dr", "com.apple.quarantine", dir); err != nil || code != 0 {
		return r, fmt.Errorf("strip quarantine: exit %d: %s: %w", code, strings.TrimSpace(string(stderr)), err)
	}
	r.QuarantineOff = true

	// 2. Ad-hoc re-sign inside-out. Deepest path first so nested libraries
	// are signed before anything that loads them, and the executable last.
	sort.Slice(machos, func(i, j int) bool {
		di, dj := strings.Count(machos[i], string(filepath.Separator)), strings.Count(machos[j], string(filepath.Separator))
		if di != dj {
			return di > dj // deeper first
		}
		return machos[i] > machos[j]
	})
	// Ensure the executable is signed dead last regardless of depth.
	machos = moveToEnd(machos, engineCommand)

	for _, p := range machos {
		// -f: replace any existing signature; -s -: ad-hoc (null identity),
		// which clears TeamID and Library-Validation constraints.
		if _, stderr, code, err := f.Runner.Run(ctx, "codesign", "--force", "--sign", "-", p); err != nil || code != 0 {
			return r, fmt.Errorf("ad-hoc sign %s: exit %d: %s: %w", p, code, strings.TrimSpace(string(stderr)), err)
		}
		r.Resigned = append(r.Resigned, p)
	}
	return r, nil
}

func (f *Fixer) anyQuarantined(ctx context.Context, dir string, machos []string) bool {
	// Probe the directory and the executable(s); quarantine is applied
	// broadly on download, so a hit on any representative path is enough.
	probes := append([]string{dir}, machos...)
	for _, p := range probes {
		stdout, _, code, err := f.Runner.Run(ctx, "xattr", p)
		if err == nil && code == 0 && strings.Contains(string(stdout), "com.apple.quarantine") {
			return true
		}
	}
	return false
}

func (f *Fixer) verify(ctx context.Context, path string) bool {
	_, _, code, err := f.Runner.Run(ctx, "codesign", "--verify", "--deep", "--strict", path)
	return err == nil && code == 0
}

func moveToEnd(paths []string, target string) []string {
	out := make([]string, 0, len(paths))
	found := false
	for _, p := range paths {
		if p == target {
			found = true
			continue
		}
		out = append(out, p)
	}
	if found {
		out = append(out, target)
	}
	return out
}

// Mach-O magic numbers (thin and fat/universal, both byte orders).
var machoMagics = map[uint32]bool{
	0xFEEDFACE: true, // 32-bit
	0xCEFAEDFE: true, // 32-bit swapped
	0xFEEDFACF: true, // 64-bit
	0xCFFAEDFE: true, // 64-bit swapped
	0xCAFEBABE: true, // fat/universal
	0xBEBAFECA: true, // fat swapped
}

// findMachO walks dir and returns every regular file whose leading bytes are
// a Mach-O magic number. Symlinks are not followed.
func findMachO(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if isMachO(path) {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func isMachO(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	var hdr [4]byte
	if _, err := f.Read(hdr[:]); err != nil {
		return false
	}
	return machoMagics[binary.BigEndian.Uint32(hdr[:])]
}
