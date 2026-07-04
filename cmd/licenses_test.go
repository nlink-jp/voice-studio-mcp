package cmd

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nlink-jp/voice-studio-mcp/internal/config"
	"github.com/nlink-jp/voice-studio-mcp/internal/engine"
	"github.com/nlink-jp/voice-studio-mcp/internal/engine/enginetest"
)

func licensesEnv(t *testing.T) (*engine.Client, *config.Config) {
	t.Helper()
	mock := enginetest.New()
	t.Cleanup(mock.Close)
	cfg := config.Default()
	cfg.Engine.URL = mock.URL()
	return engine.NewClient(mock.URL(), 5*time.Second), cfg
}

func TestLicensesTable(t *testing.T) {
	client, cfg := licensesEnv(t)
	// One speaker already reviewed.
	cfg.SpeakerMetadata = []config.SpeakerMetadata{{
		SpeakerUUID: "00000000-0000-0000-0000-0000000000aa",
		Name:        "MockNarrator",
		License:     "ACML 1.0 (reviewed)",
	}}

	var out strings.Builder
	if err := licensesTable(context.Background(), &out, client, cfg); err != nil {
		t.Fatalf("table: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "verified") || !strings.Contains(s, "ACML 1.0 (reviewed)") {
		t.Errorf("verified row missing:\n%s", s)
	}
	if !strings.Contains(s, "declared") || !strings.Contains(s, "example.com/terms") {
		t.Errorf("declared row missing:\n%s", s)
	}
	if !strings.Contains(s, "1 verified / 1 declared / 0 unverified") {
		t.Errorf("summary line:\n%s", s)
	}
}

func TestLicensesSkeletonSkipsVerified(t *testing.T) {
	client, cfg := licensesEnv(t)
	cfg.SpeakerMetadata = []config.SpeakerMetadata{{
		SpeakerUUID: "00000000-0000-0000-0000-0000000000aa",
	}}

	var out strings.Builder
	if err := licensesSkeleton(context.Background(), &out, client, cfg); err != nil {
		t.Fatalf("skeleton: %v", err)
	}
	s := out.String()
	// Only the unreviewed heroine is emitted (the header comment also
	// mentions the table name, so count entry lines only).
	if strings.Count(s, "\n[[speaker_metadata]]\n") != 1 {
		t.Errorf("expected exactly one entry:\n%s", s)
	}
	if !strings.Contains(s, `speaker_uuid = "00000000-0000-0000-0000-0000000000bb"`) {
		t.Errorf("heroine entry missing:\n%s", s)
	}
	// Credit falls back to the AivisSpeech convention when undeclared.
	if !strings.Contains(s, `credit = "AivisSpeech:MockHeroine"`) {
		t.Errorf("credit fallback missing:\n%s", s)
	}
	if !strings.Contains(s, "REVIEW: auto-collected") {
		t.Errorf("REVIEW note missing:\n%s", s)
	}
	// The declared free-form first line lands in license.
	if !strings.Contains(s, "example.com/terms") {
		t.Errorf("declared license missing:\n%s", s)
	}
}

func TestLicensesFullText(t *testing.T) {
	client, _ := licensesEnv(t)
	var out strings.Builder
	if err := licensesFullText(context.Background(), &out, client, "00000000-0000-0000-0000-0000000000aa"); err != nil {
		t.Fatalf("full: %v", err)
	}
	s := out.String()
	for _, want := range []string{"MockNarrator", "Mock Studio", "--- license (full text) ---", "Aivis Common Model License"} {
		if !strings.Contains(s, want) {
			t.Errorf("full text missing %q:\n%s", want, s)
		}
	}

	if err := licensesFullText(context.Background(), &out, client, "no-such-uuid"); err == nil {
		t.Errorf("unknown uuid should error")
	}
}
