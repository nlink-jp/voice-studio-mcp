package synth_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nlink-jp/voice-studio-mcp/internal/config"
	"github.com/nlink-jp/voice-studio-mcp/internal/engine"
	"github.com/nlink-jp/voice-studio-mcp/internal/engine/enginetest"
	"github.com/nlink-jp/voice-studio-mcp/internal/script"
	"github.com/nlink-jp/voice-studio-mcp/internal/synth"
	"github.com/nlink-jp/voice-studio-mcp/internal/workspace"
)

func f(v float64) *float64 { return &v }

func newSynth(t *testing.T) (*synth.Synthesizer, *enginetest.Mock, *workspace.Workspace, *synth.CacheStore) {
	t.Helper()
	mock := enginetest.New()
	t.Cleanup(mock.Close)
	ws, err := workspace.NewManager(t.TempDir()).Ensure("test")
	if err != nil {
		t.Fatal(err)
	}
	store, err := synth.OpenCacheStore(ws)
	if err != nil {
		t.Fatal(err)
	}
	s := &synth.Synthesizer{
		Client:        engine.NewClient(mock.URL(), 5*time.Second),
		Cfg:           config.Default().Synthesis,
		EngineVersion: "1.1.0-mock",
	}
	return s, mock, ws, store
}

var testResolved = synth.Resolved{StyleID: enginetest.StyleHeroineSad, SpeakerUUID: "uuid-heroine"}

func TestSynthesizeLineWritesWavAndAppliesParams(t *testing.T) {
	s, mock, ws, store := newSynth(t)
	line := script.Line{ID: 42, Speaker: "美咲", Text: "こんにちは", Intensity: f(1.4), Speed: f(0.9), Volume: f(0.8)}

	res, err := s.SynthesizeLine(context.Background(), ws, store, line, testResolved, false)
	if err != nil {
		t.Fatalf("synthesize: %v", err)
	}
	if res.Cached || res.LineID != 42 || res.StyleID != enginetest.StyleHeroineSad {
		t.Errorf("result: %+v", res)
	}
	if res.DurationSeconds <= 0 {
		t.Errorf("duration: %v", res.DurationSeconds)
	}
	// WAV landed at wav/<id>.wav and parses.
	b, err := os.ReadFile(res.WavPath)
	if err != nil {
		t.Fatalf("read wav: %v", err)
	}
	info, err := synth.ParseWAV(b)
	if err != nil {
		t.Fatalf("parse wav: %v", err)
	}
	if info.SampleRate != s.Cfg.OutputSamplingRate {
		t.Errorf("sample rate: %d", info.SampleRate)
	}
	if filepath.Base(res.WavPath) != "42.wav" {
		t.Errorf("wav path: %s", res.WavPath)
	}

	// The engine received our overrides and kept unknown fields.
	qs := mock.SynthesisQueries()
	if len(qs) != 1 {
		t.Fatalf("synthesis calls: %d", len(qs))
	}
	q := qs[0]
	if q["speedScale"] != 0.9 || q["intonationScale"] != 1.4 || q["volumeScale"] != 0.8 {
		t.Errorf("speed/intensity/volume not applied: %v", q)
	}
	if q["outputSamplingRate"] != float64(s.Cfg.OutputSamplingRate) {
		t.Errorf("sampling rate not forced: %v", q["outputSamplingRate"])
	}
	if q["outputStereo"] != false {
		t.Errorf("outputStereo: %v", q["outputStereo"])
	}
	if _, ok := q["tempoDynamicsScale"]; !ok {
		t.Errorf("engine-specific field dropped: %v", q)
	}
}

func TestSynthesizeLineUsesDefaultsWhenUnset(t *testing.T) {
	s, mock, ws, store := newSynth(t)
	line := script.Line{ID: 1, Speaker: "美咲", Text: "デフォルト"}
	if _, err := s.SynthesizeLine(context.Background(), ws, store, line, testResolved, false); err != nil {
		t.Fatal(err)
	}
	q := mock.SynthesisQueries()[0]
	if q["speedScale"] != s.Cfg.DefaultSpeed || q["intonationScale"] != s.Cfg.DefaultIntensity || q["volumeScale"] != s.Cfg.DefaultVolume {
		t.Errorf("defaults not applied: %v", q)
	}
}

func TestSynthesizeLineCacheHitSkipsEngine(t *testing.T) {
	s, mock, ws, store := newSynth(t)
	line := script.Line{ID: 7, Speaker: "美咲", Text: "キャッシュ確認"}

	first, err := s.SynthesizeLine(context.Background(), ws, store, line, testResolved, false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.SynthesizeLine(context.Background(), ws, store, line, testResolved, false)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Cached {
		t.Errorf("expected cache hit: %+v", second)
	}
	if second.DurationSeconds != first.DurationSeconds {
		t.Errorf("cached duration mismatch: %v vs %v", second.DurationSeconds, first.DurationSeconds)
	}
	if n := mock.CallCount("/synthesis"); n != 1 {
		t.Errorf("engine called %d times, want 1", n)
	}
}

func TestSynthesizeLineForceResynthesizes(t *testing.T) {
	s, mock, ws, store := newSynth(t)
	line := script.Line{ID: 7, Speaker: "美咲", Text: "強制再合成"}
	if _, err := s.SynthesizeLine(context.Background(), ws, store, line, testResolved, false); err != nil {
		t.Fatal(err)
	}
	res, err := s.SynthesizeLine(context.Background(), ws, store, line, testResolved, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Cached {
		t.Errorf("force should bypass cache")
	}
	if n := mock.CallCount("/synthesis"); n != 2 {
		t.Errorf("engine called %d times, want 2", n)
	}
}

func TestCacheMissesWhenInputsChange(t *testing.T) {
	s, mock, ws, store := newSynth(t)
	base := script.Line{ID: 9, Speaker: "美咲", Text: "本文"}
	if _, err := s.SynthesizeLine(context.Background(), ws, store, base, testResolved, false); err != nil {
		t.Fatal(err)
	}

	changed := base
	changed.Speed = f(1.2)
	if _, err := s.SynthesizeLine(context.Background(), ws, store, changed, testResolved, false); err != nil {
		t.Fatal(err)
	}
	if n := mock.CallCount("/synthesis"); n != 2 {
		t.Errorf("speed change should re-synthesize (calls=%d)", n)
	}

	// Pause changes must NOT invalidate the cache.
	paused := base
	paused.PauseAfterMS = new(int)
	*paused.PauseAfterMS = 500
	res, err := s.SynthesizeLine(context.Background(), ws, store, paused, testResolved, false)
	if err != nil {
		t.Fatal(err)
	}
	// base was overwritten by "changed" (same line id) — but the cache entry
	// now holds the changed hash, so the pause variant of the ORIGINAL params
	// misses. Use a fresh line id to assert the pause-neutrality precisely.
	line2 := script.Line{ID: 10, Speaker: "美咲", Text: "ポーズ確認"}
	if _, err := s.SynthesizeLine(context.Background(), ws, store, line2, testResolved, false); err != nil {
		t.Fatal(err)
	}
	callsBefore := mock.CallCount("/synthesis")
	line2.PauseAfterMS = new(int)
	*line2.PauseAfterMS = 800
	res, err = s.SynthesizeLine(context.Background(), ws, store, line2, testResolved, false)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Cached {
		t.Errorf("pause change should be a cache hit")
	}
	if mock.CallCount("/synthesis") != callsBefore {
		t.Errorf("pause change must not call the engine")
	}
}

func TestCacheMissesWhenWavDeleted(t *testing.T) {
	s, _, ws, store := newSynth(t)
	line := script.Line{ID: 11, Speaker: "美咲", Text: "ファイル消失"}
	first, err := s.SynthesizeLine(context.Background(), ws, store, line, testResolved, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(first.WavPath); err != nil {
		t.Fatal(err)
	}
	res, err := s.SynthesizeLine(context.Background(), ws, store, line, testResolved, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Cached {
		t.Errorf("missing wav must be re-synthesized")
	}
}

func TestCacheKeyStability(t *testing.T) {
	k1 := synth.CacheKey("v1", "uuid", 100, 1.0, 1.0, 1.0, 0.1, 0.1, 44100, "text")
	k2 := synth.CacheKey("v1", "uuid", 100, 1.0, 1.0, 1.0, 0.1, 0.1, 44100, "text")
	if k1 != k2 {
		t.Errorf("key not deterministic")
	}
	variants := []string{
		synth.CacheKey("v2", "uuid", 100, 1.0, 1.0, 1.0, 0.1, 0.1, 44100, "text"),
		synth.CacheKey("v1", "other", 100, 1.0, 1.0, 1.0, 0.1, 0.1, 44100, "text"),
		synth.CacheKey("v1", "uuid", 101, 1.0, 1.0, 1.0, 0.1, 0.1, 44100, "text"),
		synth.CacheKey("v1", "uuid", 100, 1.1, 1.0, 1.0, 0.1, 0.1, 44100, "text"),
		synth.CacheKey("v1", "uuid", 100, 1.0, 1.3, 1.0, 0.1, 0.1, 44100, "text"),
		synth.CacheKey("v1", "uuid", 100, 1.0, 1.0, 0.8, 0.1, 0.1, 44100, "text"),
		synth.CacheKey("v1", "uuid", 100, 1.0, 1.0, 1.0, 0.2, 0.1, 44100, "text"),
		synth.CacheKey("v1", "uuid", 100, 1.0, 1.0, 1.0, 0.1, 0.1, 24000, "text"),
		synth.CacheKey("v1", "uuid", 100, 1.0, 1.0, 1.0, 0.1, 0.1, 44100, "other"),
	}
	seen := map[string]bool{k1: true}
	for i, v := range variants {
		if seen[v] {
			t.Errorf("variant %d collided", i)
		}
		seen[v] = true
	}
}

func TestParseWAV(t *testing.T) {
	b := enginetest.WAV(24000, 500)
	info, err := synth.ParseWAV(b)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if info.SampleRate != 24000 || info.Channels != 1 || info.BitsPerSample != 16 {
		t.Errorf("info: %+v", info)
	}
	if info.DurationSeconds < 0.49 || info.DurationSeconds > 0.51 {
		t.Errorf("duration: %v", info.DurationSeconds)
	}

	if _, err := synth.ParseWAV([]byte("junk")); err == nil {
		t.Errorf("junk should not parse")
	}
}

func TestCacheStoreRoundTrip(t *testing.T) {
	ws, err := workspace.NewManager(t.TempDir()).Ensure("cache-rt")
	if err != nil {
		t.Fatal(err)
	}
	s1, err := synth.OpenCacheStore(ws)
	if err != nil {
		t.Fatal(err)
	}
	if err := s1.Put(1, synth.CacheEntry{Hash: "h1", DurationSeconds: 1.5}); err != nil {
		t.Fatal(err)
	}

	// A fresh store sees the persisted entry (wav existence is checked by
	// Lookup, so create wav/1.wav inside the workspace).
	if err := ws.WriteFileAtomic(synth.WavRel(1), []byte("x")); err != nil {
		t.Fatal(err)
	}
	s2, err := synth.OpenCacheStore(ws)
	if err != nil {
		t.Fatal(err)
	}
	e, hit := s2.Lookup(1, "h1")
	if !hit || e.DurationSeconds != 1.5 {
		t.Errorf("lookup: %+v hit=%v", e, hit)
	}
	if _, hit := s2.Lookup(1, "other-hash"); hit {
		t.Errorf("hash mismatch must miss")
	}
	if _, hit := s2.Lookup(2, "h1"); hit {
		t.Errorf("unknown line must miss (no wav/2.wav)")
	}
}

func TestCacheStoreCorruptIndexStartsFresh(t *testing.T) {
	ws, err := workspace.NewManager(t.TempDir()).Ensure("cache-corrupt")
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.WriteFileAtomic("cache/index.json", []byte("{corrupt")); err != nil {
		t.Fatal(err)
	}
	s, err := synth.OpenCacheStore(ws)
	if err != nil {
		t.Fatalf("corrupt index should not fail open: %v", err)
	}
	if _, hit := s.Lookup(1, "h"); hit {
		t.Errorf("fresh store must miss")
	}
}
