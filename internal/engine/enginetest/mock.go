// Package enginetest provides an in-process mock of the AivisSpeech Engine
// HTTP API for unit and e2e tests. It implements the endpoints the client
// uses (/version /speakers /audio_query /synthesis /user_dict_word), records
// every call, and supports fault injection.
package enginetest

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"unicode/utf8"
)

// Fixture style IDs exposed by the mock's /speakers.
const (
	StyleNarratorNormal = 100
	StyleNarratorCalm   = 101
	StyleHeroineNormal  = 200
	StyleHeroineSad     = 201
)

// FixtureSpeakers is the /speakers payload: two speakers with two styles each.
var FixtureSpeakers = []map[string]any{
	{
		"name":         "MockNarrator",
		"speaker_uuid": "00000000-0000-0000-0000-0000000000aa",
		"styles": []map[string]any{
			{"id": StyleNarratorNormal, "name": "ノーマル", "type": "talk"},
			{"id": StyleNarratorCalm, "name": "落ち着き", "type": "talk"},
		},
		"version": "1.0.0",
	},
	{
		"name":         "MockHeroine",
		"speaker_uuid": "00000000-0000-0000-0000-0000000000bb",
		"styles": []map[string]any{
			{"id": StyleHeroineNormal, "name": "ノーマル", "type": "talk"},
			{"id": StyleHeroineSad, "name": "悲しみ", "type": "talk"},
		},
		"version": "1.0.0",
	},
}

// Call is one recorded HTTP request.
type Call struct {
	Method string
	Path   string
	Query  map[string]string
}

// Mock is a fake engine backed by httptest.Server.
type Mock struct {
	Server *httptest.Server

	mu           sync.Mutex
	down         bool
	failNext     map[string][]int // path -> queued status codes
	calls        []Call
	synthQueries []map[string]any // recorded /synthesis bodies
	dictWords    []map[string]string
}

// New starts the mock engine. Callers must Close it.
func New() *Mock {
	m := &Mock{failNext: make(map[string][]int)}
	mux := http.NewServeMux()
	mux.HandleFunc("/version", m.handleVersion)
	mux.HandleFunc("/speakers", m.handleSpeakers)
	mux.HandleFunc("/audio_query", m.handleAudioQuery)
	mux.HandleFunc("/synthesis", m.handleSynthesis)
	mux.HandleFunc("/user_dict_word", m.handleUserDictWord)
	mux.HandleFunc("/aivm_models", m.handleAivmModels)
	m.Server = httptest.NewServer(mux)
	return m
}

// FixtureAivmModels is the /aivm_models payload, mirroring the real engine's
// shape: each model entry's speakers[] wraps the VOICEVOX-style descriptor
// under "speaker" and carries the per-speaker license text as
// speaker_info.policy. The narrator model declares an ACML license (via
// policy) plus a credit line in its description; the heroine model has an
// empty policy so consumers must fall back to the manifest's free-form
// terms (no markdown heading, no credit pattern).
var FixtureAivmModels = map[string]any{
	"10000000-0000-0000-0000-00000000000a": map[string]any{
		"is_private_model": false,
		"manifest": map[string]any{
			"uuid":        "10000000-0000-0000-0000-00000000000a",
			"name":        "MockNarrator",
			"description": "落ち着いた声のモデルです。クレジットして頂ける際は「AivisSpeech: MockNarrator」をご利用ください。",
			"creators":    []string{"Mock Studio <mock@example.com>"},
			"license":     "# Aivis Common Model License (ACML) 1.0\n\nこのライセンスは、AI 音声合成モデルの利用条件と制限を定めるものです。\n",
		},
		"speakers": []map[string]any{{
			"speaker": FixtureSpeakers[0],
			"speaker_info": map[string]any{
				"policy": "# Aivis Common Model License (ACML) 1.0\n\nこのライセンスは、AI 音声合成モデルの利用条件と制限を定めるものです。\n",
			},
		}},
	},
	"10000000-0000-0000-0000-00000000000b": map[string]any{
		"is_private_model": false,
		"manifest": map[string]any{
			"uuid":        "10000000-0000-0000-0000-00000000000b",
			"name":        "MockHeroine",
			"description": "ヒロイン向けのモデルです。",
			"creators":    []string{"Mock Studio <mock@example.com>"},
			"license":     "\n利用規約は https://example.com/terms を遵守すること。\n再配布は禁止。\n",
		},
		"speakers": []map[string]any{{
			"speaker":      FixtureSpeakers[1],
			"speaker_info": map[string]any{"policy": ""},
		}},
	},
}

func (m *Mock) handleAivmModels(w http.ResponseWriter, r *http.Request) {
	if !m.gate(w, r) {
		return
	}
	writeJSON(w, FixtureAivmModels)
}

// Close shuts the mock down.
func (m *Mock) Close() { m.Server.Close() }

// URL returns the mock base URL.
func (m *Mock) URL() string { return m.Server.URL }

// SetDown makes every endpoint return 503 until re-enabled.
func (m *Mock) SetDown(down bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.down = down
}

// FailNext queues an error status for the next call to path.
func (m *Mock) FailNext(path string, status int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failNext[path] = append(m.failNext[path], status)
}

// Calls returns a copy of all recorded calls.
func (m *Mock) Calls() []Call {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Call, len(m.calls))
	copy(out, m.calls)
	return out
}

// CallCount returns how many times path was hit.
func (m *Mock) CallCount(path string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, c := range m.calls {
		if c.Path == path {
			n++
		}
	}
	return n
}

// SynthesisQueries returns the recorded /synthesis request bodies.
func (m *Mock) SynthesisQueries() []map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]map[string]any, len(m.synthQueries))
	copy(out, m.synthQueries)
	return out
}

// DictWords returns the recorded /user_dict_word registrations.
func (m *Mock) DictWords() []map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]map[string]string, len(m.dictWords))
	copy(out, m.dictWords)
	return out
}

// gate records the call and applies down / fail-injection state. It reports
// whether the handler should continue.
func (m *Mock) gate(w http.ResponseWriter, r *http.Request) bool {
	m.mu.Lock()
	q := map[string]string{}
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			q[k] = v[0]
		}
	}
	m.calls = append(m.calls, Call{Method: r.Method, Path: r.URL.Path, Query: q})
	if m.down {
		m.mu.Unlock()
		http.Error(w, "engine down", http.StatusServiceUnavailable)
		return false
	}
	if queue := m.failNext[r.URL.Path]; len(queue) > 0 {
		status := queue[0]
		m.failNext[r.URL.Path] = queue[1:]
		m.mu.Unlock()
		http.Error(w, "injected failure", status)
		return false
	}
	m.mu.Unlock()
	return true
}

func (m *Mock) handleVersion(w http.ResponseWriter, r *http.Request) {
	if !m.gate(w, r) {
		return
	}
	writeJSON(w, "1.1.0-mock")
}

func (m *Mock) handleSpeakers(w http.ResponseWriter, r *http.Request) {
	if !m.gate(w, r) {
		return
	}
	writeJSON(w, FixtureSpeakers)
}

func (m *Mock) handleAudioQuery(w http.ResponseWriter, r *http.Request) {
	if !m.gate(w, r) {
		return
	}
	text := r.URL.Query().Get("text")
	speaker := r.URL.Query().Get("speaker")
	if text == "" || speaker == "" {
		http.Error(w, "missing text or speaker", http.StatusUnprocessableEntity)
		return
	}
	// Baseline VOICEVOX fields plus an AivisSpeech-specific extension
	// (tempoDynamicsScale) so client passthrough can be asserted.
	writeJSON(w, map[string]any{
		"accent_phrases":     []any{},
		"speedScale":         1.0,
		"intonationScale":    1.0,
		"tempoDynamicsScale": 1.0,
		"pitchScale":         0.0,
		"volumeScale":        1.0,
		"prePhonemeLength":   0.1,
		"postPhonemeLength":  0.1,
		"outputSamplingRate": 24000,
		"outputStereo":       false,
		"kana":               text,
	})
}

func (m *Mock) handleSynthesis(w http.ResponseWriter, r *http.Request) {
	if !m.gate(w, r) {
		return
	}
	if r.URL.Query().Get("speaker") == "" {
		http.Error(w, "missing speaker", http.StatusUnprocessableEntity)
		return
	}
	var q map[string]any
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		http.Error(w, "invalid audio query: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}
	m.mu.Lock()
	m.synthQueries = append(m.synthQueries, q)
	m.mu.Unlock()

	rate := 24000
	if v, ok := q["outputSamplingRate"].(float64); ok && v > 0 {
		rate = int(v)
	}
	// Fake duration: 50ms per rune of kana, min 100ms — long enough to make
	// per-line durations distinguishable in tests.
	kana, _ := q["kana"].(string)
	ms := 50 * utf8.RuneCountInString(kana)
	if ms < 100 {
		ms = 100
	}
	w.Header().Set("Content-Type", "audio/wav")
	_, _ = w.Write(WAV(rate, ms))
}

func (m *Mock) handleUserDictWord(w http.ResponseWriter, r *http.Request) {
	if !m.gate(w, r) {
		return
	}
	q := r.URL.Query()
	if q.Get("surface") == "" || q.Get("pronunciation") == "" || q.Get("accent_type") == "" {
		http.Error(w, "missing surface/pronunciation/accent_type", http.StatusUnprocessableEntity)
		return
	}
	if _, err := strconv.Atoi(q.Get("accent_type")); err != nil {
		http.Error(w, "accent_type must be an integer", http.StatusUnprocessableEntity)
		return
	}
	m.mu.Lock()
	m.dictWords = append(m.dictWords, map[string]string{
		"surface":       q.Get("surface"),
		"pronunciation": q.Get("pronunciation"),
		"accent_type":   q.Get("accent_type"),
	})
	n := len(m.dictWords)
	m.mu.Unlock()
	writeJSON(w, fmt.Sprintf("mock-word-uuid-%04d", n))
}

// WAV builds a minimal valid mono 16-bit PCM WAV of the given sampling rate
// and duration in milliseconds (silence).
func WAV(rate, durationMS int) []byte {
	nSamples := rate * durationMS / 1000
	dataSize := nSamples * 2 // 16-bit mono
	buf := make([]byte, 44+dataSize)
	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:8], uint32(36+dataSize))
	copy(buf[8:12], "WAVE")
	copy(buf[12:16], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:20], 16)             // fmt chunk size
	binary.LittleEndian.PutUint16(buf[20:22], 1)              // PCM
	binary.LittleEndian.PutUint16(buf[22:24], 1)              // mono
	binary.LittleEndian.PutUint32(buf[24:28], uint32(rate))   // sample rate
	binary.LittleEndian.PutUint32(buf[28:32], uint32(rate*2)) // byte rate
	binary.LittleEndian.PutUint16(buf[32:34], 2)              // block align
	binary.LittleEndian.PutUint16(buf[34:36], 16)             // bits per sample
	copy(buf[36:40], "data")
	binary.LittleEndian.PutUint32(buf[40:44], uint32(dataSize))
	return buf
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
