// Package engine talks to an AivisSpeech Engine (VOICEVOX-compatible HTTP
// API) and manages its process lifecycle.
package engine

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
