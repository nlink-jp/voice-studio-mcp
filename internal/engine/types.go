// Package engine talks to an AivisSpeech Engine (VOICEVOX-compatible HTTP
// API) and manages its process lifecycle.
package engine

import (
	"regexp"
	"strings"
)

// Speaker is one entry of GET /speakers.
type Speaker struct {
	Name        string  `json:"name"`
	SpeakerUUID string  `json:"speaker_uuid"`
	Styles      []Style `json:"styles"`
	Version     string  `json:"version,omitempty"`
}

// Style is one selectable style of a speaker. ID is the globally unique
// style id passed as the `speaker` query parameter of /audio_query and
// /synthesis.
type Style struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

// UserDictWord is one pronunciation-dictionary entry for POST /user_dict_word.
type UserDictWord struct {
	Surface       string `json:"surface"`
	Pronunciation string `json:"pronunciation"` // katakana
	AccentType    int    `json:"accent_type"`
	WordType      string `json:"word_type,omitempty"`
	Priority      *int   `json:"priority,omitempty"`
}

// AudioQuery is the synthesis query returned by /audio_query and consumed by
// /synthesis. It is deliberately a map, not a struct: the engine adds fields
// beyond the VOICEVOX baseline (e.g. tempoDynamicsScale) and we must pass
// everything through untouched, only overriding the few keys we control.
type AudioQuery map[string]any

// AivmModel is one installed voice model from the AivisSpeech-specific
// GET /aivm_models endpoint. Only the fields we need are declared — the
// real response also embeds base64 icons and per-style assets that would
// bloat memory for nothing.
type AivmModel struct {
	IsPrivateModel bool         `json:"is_private_model"`
	Manifest       AivmManifest `json:"manifest"`
	// Speakers is the engine-format speaker list of this model; its
	// SpeakerUUID values join against GET /speakers.
	Speakers []Speaker `json:"speakers"`
}

// AivmManifest is the subset of the AIVM manifest we consume. License is the
// FULL license text embedded by the model author (often ACML 1.0); it is a
// declaration to read, not a verdict — see ADR-0008.
type AivmManifest struct {
	UUID        string   `json:"uuid"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Creators    []string `json:"creators"`
	License     string   `json:"license"`
}

// LicenseName returns a short human-readable name for the embedded license
// text: the first markdown heading if present, otherwise the first non-empty
// line, truncated to a summary length. Empty when no license text exists.
func (m AivmManifest) LicenseName() string {
	for _, line := range strings.Split(m.License, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#"))
		if line == "" {
			continue
		}
		const max = 80
		r := []rune(line)
		if len(r) > max {
			return string(r[:max]) + "…"
		}
		return line
	}
	return ""
}

// CreditLine extracts the credit notation the author asked for in the model
// description (the common AivisHub pattern: 「AivisSpeech: 名前」をご利用ください).
// Empty when the description declares none.
func (m AivmManifest) CreditLine() string {
	match := creditPattern.FindStringSubmatch(m.Description)
	if len(match) == 2 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

var creditPattern = regexp.MustCompile(`「(AivisSpeech:\s*[^」]+)」`)
