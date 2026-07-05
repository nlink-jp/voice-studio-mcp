package script_test

import (
	"errors"
	"testing"

	"github.com/nlink-jp/voice-studio-mcp/internal/script"
	"github.com/nlink-jp/voice-studio-mcp/internal/toolerr"
)

const castingTOML = `
[characters."narrator"]
speaker_uuid = "uuid-narrator"
style_id = 100
credit = "AivisSpeech:MockNarrator"
license_checked = true

[characters."narrator".styles]
"落ち着き" = 101

[characters."美咲"]
speaker_uuid = "uuid-heroine"
style_id = 200

[characters."美咲".styles]
"悲しみ" = 201
`

func parseCasting(t *testing.T, body string) (*script.Casting, error) {
	t.Helper()
	return script.ParseCasting([]byte(body), "casting.toml")
}

func TestLoadCastingAndResolve(t *testing.T) {
	c, err := parseCasting(t, castingTOML)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	// Default style.
	id, entry, err := c.Resolve("narrator", "")
	if err != nil || id != 100 {
		t.Errorf("default style: id=%d err=%v", id, err)
	}
	if entry.Credit != "AivisSpeech:MockNarrator" || !entry.LicenseChecked {
		t.Errorf("entry: %+v", entry)
	}

	// Named style.
	id, _, err = c.Resolve("美咲", "悲しみ")
	if err != nil || id != 201 {
		t.Errorf("named style: id=%d err=%v", id, err)
	}

	sentinel := toolerr.New(toolerr.CodeCastingUnresolved, "")
	if _, _, err := c.Resolve("誰か", ""); !errors.Is(err, sentinel) {
		t.Errorf("unknown speaker: %v", err)
	}
	if _, _, err := c.Resolve("narrator", "怒り"); !errors.Is(err, sentinel) {
		t.Errorf("unknown style: %v", err)
	}
}

func TestLoadCastingRejectsUnknownKeys(t *testing.T) {
	_, err := parseCasting(t, `
[characters."a"]
speaker_uuid = "u"
style_id = 1
volume = 2
`)
	if err == nil {
		t.Fatalf("expected unknown-key error")
	}
}

func TestLoadCastingRequiresSpeakerUUID(t *testing.T) {
	_, err := parseCasting(t, `
[characters."a"]
style_id = 1
`)
	if err == nil {
		t.Fatalf("expected missing speaker_uuid error")
	}
}

func TestValidateCollectsAllUnresolved(t *testing.T) {
	c, err := parseCasting(t, castingTOML)
	if err != nil {
		t.Fatal(err)
	}

	lines := []script.Line{
		{ID: 1, Speaker: "narrator", Text: "ok"},
		{ID: 2, Speaker: "謎の男", Text: "unmapped speaker"},
		{ID: 3, Speaker: "美咲", Style: "怒り", Text: "unmapped style"},
		{ID: 4, Speaker: "謎の女", Text: "another unmapped"},
	}
	err = c.Validate(lines)
	var te *toolerr.Error
	if !errors.As(err, &te) || te.Code != toolerr.CodeCastingUnresolved {
		t.Fatalf("expected casting_unresolved, got %v", err)
	}
	speakers := te.Details["unmapped_speakers"].([]string)
	if len(speakers) != 2 || speakers[0] != "謎の女" && speakers[0] != "謎の男" {
		t.Errorf("unmapped_speakers: %v", speakers)
	}
	styles := te.Details["unmapped_styles"].([]map[string]string)
	if len(styles) != 1 || styles[0]["style"] != "怒り" {
		t.Errorf("unmapped_styles: %v", styles)
	}

	// A fully-resolvable script validates clean.
	if err := c.Validate(lines[:1]); err != nil {
		t.Errorf("expected clean validation, got %v", err)
	}
}
