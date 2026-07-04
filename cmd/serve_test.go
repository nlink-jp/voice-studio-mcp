package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveConfigExplicitPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[engine]\nmode = \"external\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, used, err := resolveConfig(path)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if used != path {
		t.Errorf("used: %q", used)
	}
	if cfg.Engine.Mode != "external" {
		t.Errorf("mode: %q", cfg.Engine.Mode)
	}
}

func TestResolveConfigExplicitMissingFails(t *testing.T) {
	_, _, err := resolveConfig(filepath.Join(t.TempDir(), "nope.toml"))
	if err == nil {
		t.Fatalf("expected error for missing explicit config")
	}
}

func TestResolveConfigFallsBackToDefaults(t *testing.T) {
	// Run from a directory without a config.toml so the cwd probe misses.
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
	cfg, used, rerr := resolveConfig("")
	if rerr != nil {
		t.Fatalf("resolve: %v", rerr)
	}
	// The home config may exist on a developer machine; accept either the
	// defaults or that file, but never a cwd file.
	if used != "" && !strings.Contains(used, ".config") {
		t.Errorf("unexpected config source: %q", used)
	}
	if cfg == nil || cfg.Engine.URL == "" {
		t.Errorf("config not populated: %+v", cfg)
	}
}
