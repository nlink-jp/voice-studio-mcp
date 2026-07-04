package engine_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nlink-jp/voice-studio-mcp/internal/engine"
	"github.com/nlink-jp/voice-studio-mcp/internal/engine/enginetest"
)

func TestAivmModels(t *testing.T) {
	mock := enginetest.New()
	t.Cleanup(mock.Close)
	c := engine.NewClient(mock.URL(), 5*time.Second)

	models, err := c.AivmModels(context.Background())
	if err != nil {
		t.Fatalf("aivm_models: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("model count: %d", len(models))
	}
	narr, ok := models["10000000-0000-0000-0000-00000000000a"]
	if !ok {
		t.Fatalf("narrator model missing: %v", models)
	}
	if narr.Manifest.Name != "MockNarrator" || narr.IsPrivateModel {
		t.Errorf("manifest: %+v", narr.Manifest)
	}
	// The join key against /speakers survives parsing.
	if len(narr.Speakers) != 1 || narr.Speakers[0].Speaker.SpeakerUUID != "00000000-0000-0000-0000-0000000000aa" {
		t.Errorf("speakers: %+v", narr.Speakers)
	}
	// Per-speaker policy wins; empty policy falls back to the manifest text.
	if got := narr.LicenseTextFor(narr.Speakers[0]); !strings.Contains(got, "ACML") {
		t.Errorf("narrator license text: %q", got)
	}
	hero := models["10000000-0000-0000-0000-00000000000b"]
	if got := hero.LicenseTextFor(hero.Speakers[0]); !strings.Contains(got, "example.com/terms") {
		t.Errorf("heroine fallback license text: %q", got)
	}
}

func TestManifestLicenseName(t *testing.T) {
	cases := []struct {
		license string
		want    string
	}{
		{"# Aivis Common Model License (ACML) 1.0\n\n本文...", "Aivis Common Model License (ACML) 1.0"},
		{"\n利用規約は https://example.com/terms を遵守すること。\n", "利用規約は https://example.com/terms を遵守すること。"},
		{"Creative Commons CC0 1.0 Universal\n...", "Creative Commons CC0 1.0 Universal"},
		{"", ""},
		{"   \n\n", ""},
	}
	for _, tc := range cases {
		m := engine.AivmManifest{License: tc.license}
		if got := m.LicenseName(); got != tc.want {
			t.Errorf("LicenseName(%q) = %q, want %q", tc.license[:min(20, len(tc.license))], got, tc.want)
		}
	}
	// Long first lines are truncated with an ellipsis.
	long := engine.AivmManifest{License: string(make([]rune, 0)) + "あ" + repeat("い", 100)}
	if got := long.LicenseName(); len([]rune(got)) != 81 {
		t.Errorf("long license name not truncated: %d runes", len([]rune(got)))
	}
}

func TestManifestCreditLine(t *testing.T) {
	withCredit := engine.AivmManifest{
		Description: "クレジットして頂ける際は「AivisSpeech: コハク」をご利用ください。",
	}
	if got := withCredit.CreditLine(); got != "AivisSpeech: コハク" {
		t.Errorf("credit: %q", got)
	}
	without := engine.AivmManifest{Description: "ただの説明文。"}
	if got := without.CreditLine(); got != "" {
		t.Errorf("expected empty credit, got %q", got)
	}
}

func repeat(s string, n int) string {
	out := ""
	for range n {
		out += s
	}
	return out
}
