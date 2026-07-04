package engine_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nlink-jp/voice-studio-mcp/internal/engine"
	"github.com/nlink-jp/voice-studio-mcp/internal/engine/enginetest"
	"github.com/nlink-jp/voice-studio-mcp/internal/toolerr"
)

func newClient(t *testing.T) (*engine.Client, *enginetest.Mock) {
	t.Helper()
	mock := enginetest.New()
	t.Cleanup(mock.Close)
	return engine.NewClient(mock.URL(), 5*time.Second), mock
}

func TestVersion(t *testing.T) {
	c, _ := newClient(t)
	v, err := c.Version(context.Background())
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if v != "1.1.0-mock" {
		t.Errorf("version: %q", v)
	}
}

func TestSpeakers(t *testing.T) {
	c, _ := newClient(t)
	sp, err := c.Speakers(context.Background())
	if err != nil {
		t.Fatalf("speakers: %v", err)
	}
	if len(sp) != 2 {
		t.Fatalf("speaker count: %d", len(sp))
	}
	if sp[0].Name != "MockNarrator" || sp[0].SpeakerUUID == "" {
		t.Errorf("speaker[0]: %+v", sp[0])
	}
	if len(sp[0].Styles) != 2 || sp[0].Styles[0].ID != enginetest.StyleNarratorNormal {
		t.Errorf("styles[0]: %+v", sp[0].Styles)
	}
}

func TestAudioQueryPassesParamsAndKeepsUnknownFields(t *testing.T) {
	c, mock := newClient(t)
	q, err := c.AudioQuery(context.Background(), "こんにちは", enginetest.StyleHeroineSad)
	if err != nil {
		t.Fatalf("audio_query: %v", err)
	}
	// Query params reached the engine.
	calls := mock.Calls()
	last := calls[len(calls)-1]
	if last.Path != "/audio_query" || last.Query["text"] != "こんにちは" || last.Query["speaker"] != "201" {
		t.Errorf("recorded call: %+v", last)
	}
	// Engine-specific extension fields survive into the map.
	if _, ok := q["tempoDynamicsScale"]; !ok {
		t.Errorf("tempoDynamicsScale missing from AudioQuery: %v", q)
	}
}

func TestSynthesisPostsQueryAndReturnsWAV(t *testing.T) {
	c, mock := newClient(t)
	q := engine.AudioQuery{
		"kana":               "テスト",
		"speedScale":         1.25,
		"outputSamplingRate": float64(44100),
		"tempoDynamicsScale": 0.9,
	}
	wav, err := c.Synthesis(context.Background(), q, enginetest.StyleNarratorNormal)
	if err != nil {
		t.Fatalf("synthesis: %v", err)
	}
	if len(wav) < 44 || string(wav[0:4]) != "RIFF" {
		t.Errorf("not a WAV: %d bytes", len(wav))
	}
	// The engine received our adjusted fields, including the passthrough one.
	got := mock.SynthesisQueries()
	if len(got) != 1 {
		t.Fatalf("synthesis bodies recorded: %d", len(got))
	}
	if got[0]["speedScale"] != 1.25 || got[0]["tempoDynamicsScale"] != 0.9 {
		t.Errorf("posted query: %v", got[0])
	}
}

func TestAddUserDictWord(t *testing.T) {
	c, mock := newClient(t)
	uuid, err := c.AddUserDictWord(context.Background(), engine.UserDictWord{
		Surface:       "美咲",
		Pronunciation: "ミサキ",
		AccentType:    1,
	})
	if err != nil {
		t.Fatalf("add word: %v", err)
	}
	if uuid == "" {
		t.Errorf("empty uuid")
	}
	words := mock.DictWords()
	if len(words) != 1 || words[0]["surface"] != "美咲" || words[0]["pronunciation"] != "ミサキ" {
		t.Errorf("recorded words: %v", words)
	}
}

func TestHTTPErrorBecomesEngineRequestFailed(t *testing.T) {
	c, mock := newClient(t)
	mock.FailNext("/speakers", 500)
	_, err := c.Speakers(context.Background())
	var te *toolerr.Error
	if !errors.As(err, &te) || te.Code != toolerr.CodeEngineRequest {
		t.Fatalf("expected engine_request_failed, got %v", err)
	}
	if te.Details["status"] != 500 {
		t.Errorf("details: %v", te.Details)
	}
}

func TestUnreachableBecomesEngineUnavailable(t *testing.T) {
	mock := enginetest.New()
	url := mock.URL()
	mock.Close() // now nothing is listening
	c := engine.NewClient(url, 1*time.Second)
	_, err := c.Version(context.Background())
	if !errors.Is(err, toolerr.New(toolerr.CodeEngineUnavailable, "")) {
		t.Fatalf("expected engine_unavailable, got %v", err)
	}
}
