package engine_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nlink-jp/voice-studio-mcp/internal/engine"
	"github.com/nlink-jp/voice-studio-mcp/internal/engine/enginetest"
	"github.com/nlink-jp/voice-studio-mcp/internal/toolerr"
)

var unavailable = toolerr.New(toolerr.CodeEngineUnavailable, "")

func TestExternalModeAttaches(t *testing.T) {
	mock := enginetest.New()
	defer mock.Close()
	c := engine.NewClient(mock.URL(), 2*time.Second)
	s := engine.NewSupervisor(c, nil, engine.SupervisorOpts{Mode: "external"})

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if s.Spawned() {
		t.Errorf("external mode must not spawn")
	}
	if err := s.Stop(context.Background()); err != nil {
		t.Errorf("stop: %v", err)
	}
}

func TestExternalModeFailsWhenEngineDown(t *testing.T) {
	mock := enginetest.New()
	url := mock.URL()
	mock.Close()
	c := engine.NewClient(url, 1*time.Second)
	s := engine.NewSupervisor(c, nil, engine.SupervisorOpts{Mode: "external"})

	err := s.Start(context.Background())
	if !errors.Is(err, unavailable) {
		t.Fatalf("expected engine_unavailable, got %v", err)
	}
}

func TestManagedModeAttachesToRunningEngine(t *testing.T) {
	mock := enginetest.New()
	defer mock.Close()
	c := engine.NewClient(mock.URL(), 2*time.Second)
	s := engine.NewSupervisor(c, nil, engine.SupervisorOpts{
		Mode:    "managed",
		Command: "/bin/definitely-not-a-real-engine",
	})

	// Engine already answers, so the bogus command must never be needed.
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if s.Spawned() {
		t.Errorf("attach path must not spawn")
	}
}

func TestManagedModeRejectsMissingCommand(t *testing.T) {
	mock := enginetest.New()
	url := mock.URL()
	mock.Close()
	c := engine.NewClient(url, 500*time.Millisecond)
	s := engine.NewSupervisor(c, nil, engine.SupervisorOpts{
		Mode:    "managed",
		Command: filepath.Join(t.TempDir(), "missing"),
	})
	err := s.Start(context.Background())
	if !errors.Is(err, unavailable) {
		t.Fatalf("expected engine_unavailable, got %v", err)
	}
}

// TestManagedModeSpawnsAndReaps exercises the real spawn path: the "engine"
// is /bin/sleep, and readiness is simulated by flipping the mock server back
// up after Start begins polling.
func TestManagedModeSpawnsAndReaps(t *testing.T) {
	mock := enginetest.New()
	defer mock.Close()
	mock.SetDown(true) // force the spawn path (attach probe fails)

	c := engine.NewClient(mock.URL(), 2*time.Second)
	s := engine.NewSupervisor(c, nil, engine.SupervisorOpts{
		Mode:            "managed",
		Command:         "/bin/sleep",
		Args:            []string{"60"},
		StartupTimeout:  10 * time.Second,
		ShutdownTimeout: 2 * time.Second,
		ProbeInterval:   50 * time.Millisecond,
	})

	go func() {
		time.Sleep(300 * time.Millisecond)
		mock.SetDown(false)
	}()

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if !s.Spawned() {
		t.Fatalf("expected spawn path")
	}

	if err := s.Stop(context.Background()); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if s.Spawned() {
		t.Errorf("stop should clear the spawned process")
	}
}

// TestManagedModeStartupTimeout verifies that a never-ready engine is killed
// and reported.
func TestManagedModeStartupTimeout(t *testing.T) {
	mock := enginetest.New()
	defer mock.Close()
	mock.SetDown(true)

	c := engine.NewClient(mock.URL(), 1*time.Second)
	s := engine.NewSupervisor(c, nil, engine.SupervisorOpts{
		Mode:            "managed",
		Command:         "/bin/sleep",
		Args:            []string{"60"},
		StartupTimeout:  300 * time.Millisecond,
		ShutdownTimeout: 2 * time.Second,
		ProbeInterval:   50 * time.Millisecond,
	})

	err := s.Start(context.Background())
	if !errors.Is(err, unavailable) {
		t.Fatalf("expected engine_unavailable, got %v", err)
	}
}

// TestManagedModeDetectsEarlyExit verifies that an engine that dies during
// startup is reported with its exit code instead of waiting out the timeout.
func TestManagedModeDetectsEarlyExit(t *testing.T) {
	mock := enginetest.New()
	defer mock.Close()
	mock.SetDown(true)

	// A command that exits immediately with a failure code.
	script := filepath.Join(t.TempDir(), "dies.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 7\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	c := engine.NewClient(mock.URL(), 1*time.Second)
	s := engine.NewSupervisor(c, nil, engine.SupervisorOpts{
		Mode:            "managed",
		Command:         script,
		StartupTimeout:  5 * time.Second,
		ShutdownTimeout: 1 * time.Second,
		ProbeInterval:   50 * time.Millisecond,
	})

	start := time.Now()
	err := s.Start(context.Background())
	if !errors.Is(err, unavailable) {
		t.Fatalf("expected engine_unavailable, got %v", err)
	}
	var te *toolerr.Error
	if !errors.As(err, &te) || te.Details["exit_code"] != 7 {
		t.Errorf("expected exit_code 7 in details, got %v", te.Details)
	}
	if time.Since(start) > 3*time.Second {
		t.Errorf("early exit should be detected well before the startup timeout")
	}
}
