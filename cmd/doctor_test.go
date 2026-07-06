package cmd

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nlink-jp/voice-studio-mcp/internal/config"
	"github.com/nlink-jp/voice-studio-mcp/internal/engine/enginetest"
)

func doctorConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg := config.Default()
	cfg.Workspace.Dir = t.TempDir()
	cfg.Master.FFmpegPath = "/bin/ls" // stands in for an existing executable
	return cfg
}

func TestDoctorAllGreenExternal(t *testing.T) {
	mock := enginetest.New()
	defer mock.Close()

	cfg := doctorConfig(t)
	cfg.Engine.Mode = config.EngineModeExternal
	cfg.Engine.URL = mock.URL()

	var out strings.Builder
	if ok := runDoctorChecks(context.Background(), &out, cfg, "/tmp/config.toml"); !ok {
		t.Fatalf("expected all green, output:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "ok engine: 1.1.0-mock") {
		t.Errorf("engine line missing: %s", out.String())
	}
}

func TestDoctorExternalModeFailsWithoutEngine(t *testing.T) {
	mock := enginetest.New()
	url := mock.URL()
	mock.Close()

	cfg := doctorConfig(t)
	cfg.Engine.Mode = config.EngineModeExternal
	cfg.Engine.URL = url

	var out strings.Builder
	if ok := runDoctorChecks(context.Background(), &out, cfg, ""); ok {
		t.Fatalf("expected failure, output:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "NG engine") {
		t.Errorf("NG engine line missing: %s", out.String())
	}
}

func TestDoctorManagedModeMissingCommandFails(t *testing.T) {
	mock := enginetest.New()
	url := mock.URL()
	mock.Close()

	cfg := doctorConfig(t)
	cfg.Engine.Mode = config.EngineModeManaged
	cfg.Engine.URL = url
	cfg.Engine.Command = filepath.Join(t.TempDir(), "missing-engine")

	var out strings.Builder
	if ok := runDoctorChecks(context.Background(), &out, cfg, ""); ok {
		t.Fatalf("expected failure, output:\n%s", out.String())
	}
	s := out.String()
	if !strings.Contains(s, "NG engine command") {
		t.Errorf("NG engine command line missing: %s", s)
	}
	// Engine-not-running is informational in managed mode.
	if !strings.Contains(s, "serve will spawn it") {
		t.Errorf("managed-mode informational line missing: %s", s)
	}
}

func TestDoctorMissingFFmpegFails(t *testing.T) {
	mock := enginetest.New()
	defer mock.Close()

	cfg := doctorConfig(t)
	cfg.Engine.Mode = config.EngineModeExternal
	cfg.Engine.URL = mock.URL()
	cfg.Master.FFmpegPath = "definitely-no-such-ffmpeg-binary"

	var out strings.Builder
	if ok := runDoctorChecks(context.Background(), &out, cfg, ""); ok {
		t.Fatalf("expected failure, output:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "NG ffmpeg") {
		t.Errorf("NG ffmpeg line missing: %s", out.String())
	}
}

func TestRunEngineFixRejectsExternalMode(t *testing.T) {
	cfg := doctorConfig(t)
	cfg.Engine.Mode = config.EngineModeExternal
	var out strings.Builder
	if err := runEngineFix(context.Background(), &out, cfg); err == nil {
		t.Errorf("--fix must reject external mode")
	}
}

func TestRunEngineFixRejectsMissingCommand(t *testing.T) {
	cfg := doctorConfig(t)
	cfg.Engine.Mode = config.EngineModeManaged
	cfg.Engine.Command = filepath.Join(t.TempDir(), "no-such-engine", "run")
	var out strings.Builder
	if err := runEngineFix(context.Background(), &out, cfg); err == nil {
		t.Errorf("--fix must reject a missing engine command")
	}
}
