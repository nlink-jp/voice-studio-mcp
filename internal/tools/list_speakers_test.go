package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/nlink-jp/voice-studio-mcp/internal/config"
	"github.com/nlink-jp/voice-studio-mcp/internal/engine"
	"github.com/nlink-jp/voice-studio-mcp/internal/engine/enginetest"
	"github.com/nlink-jp/voice-studio-mcp/internal/mcpserver"
	"github.com/nlink-jp/voice-studio-mcp/internal/synth"
	"github.com/nlink-jp/voice-studio-mcp/internal/transport"
	"github.com/nlink-jp/voice-studio-mcp/internal/workspace"
)

// testHarness spins up a registered server over in-memory pipes and returns
// a call helper plus the mock engine.
type testHarness struct {
	t    *testing.T
	deps *Deps
	mock *enginetest.Mock
}

func newHarness(t *testing.T) *testHarness {
	t.Helper()
	mock := enginetest.New()
	t.Cleanup(mock.Close)

	cfg := config.Default()
	cfg.Workspace.Dir = t.TempDir()
	client := engine.NewClient(mock.URL(), 5*time.Second)
	deps := &Deps{
		Cfg:    cfg,
		Client: client,
		Synth: &synth.Synthesizer{
			Client:        client,
			Cfg:           cfg.Synthesis,
			EngineVersion: "1.1.0-mock",
		},
		WS:     workspace.NewManager(cfg.Workspace.Dir),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	return &testHarness{t: t, deps: deps, mock: mock}
}

// callTool drives a full MCP round-trip for one tools/call and returns the
// first content block plus the isError flag.
func (h *testHarness) callTool(name string, args any) (json.RawMessage, bool) {
	h.t.Helper()
	argJSON, err := json.Marshal(args)
	if err != nil {
		h.t.Fatal(err)
	}
	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + name + `","arguments":` + string(argJSON) + `}}` + "\n"

	var out bytes.Buffer
	tr := transport.NewStdioTransport(strings.NewReader(req), &out)
	srv := mcpserver.New("voice-studio-mcp", "test", tr, slog.New(slog.NewTextHandler(io.Discard, nil)))
	Register(srv, h.deps)
	if err := srv.Serve(context.Background()); err != nil {
		h.t.Fatalf("serve: %v", err)
	}

	var resp struct {
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		h.t.Fatalf("parse response: %v\n%s", err, out.String())
	}
	if resp.Error != nil {
		h.t.Fatalf("jsonrpc error: %+v", resp.Error)
	}
	if len(resp.Result.Content) == 0 {
		h.t.Fatalf("no content in result: %s", out.String())
	}
	return json.RawMessage(resp.Result.Content[0].Text), resp.Result.IsError
}

// errCode extracts the structured code from an isError result body.
func errCode(t *testing.T, body json.RawMessage) string {
	t.Helper()
	var e struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(body, &e); err != nil {
		t.Fatalf("parse error body: %v (%s)", err, body)
	}
	return e.Code
}

func TestListSpeakers(t *testing.T) {
	h := newHarness(t)
	// Register license metadata for one of the two mock speakers.
	h.deps.Cfg.SpeakerMetadata = []config.SpeakerMetadata{{
		SpeakerUUID:   "00000000-0000-0000-0000-0000000000aa",
		Name:          "MockNarrator",
		License:       "ACML 1.0",
		Credit:        "AivisSpeech:MockNarrator",
		CommercialUse: true,
	}}

	body, isErr := h.callTool("list_speakers", map[string]any{})
	if isErr {
		t.Fatalf("unexpected tool error: %s", body)
	}
	var out struct {
		Engine   map[string]string `json:"engine"`
		Speakers []struct {
			Name        string `json:"name"`
			SpeakerUUID string `json:"speaker_uuid"`
			Styles      []struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			} `json:"styles"`
			License struct {
				Status        string `json:"status"`
				Credit        string `json:"credit"`
				CommercialUse bool   `json:"commercial_use"`
			} `json:"license"`
		} `json:"speakers"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("parse: %v\n%s", err, body)
	}
	if out.Engine["version"] != "1.1.0-mock" {
		t.Errorf("engine: %v", out.Engine)
	}
	if len(out.Speakers) != 2 {
		t.Fatalf("speakers: %d", len(out.Speakers))
	}
	// Config entry wins over the manifest declaration.
	if out.Speakers[0].License.Status != "verified" || out.Speakers[0].License.Credit != "AivisSpeech:MockNarrator" {
		t.Errorf("registered speaker license: %+v", out.Speakers[0].License)
	}
	// No config entry, but the mock heroine model declares license text in
	// its AIVM manifest.
	if out.Speakers[1].License.Status != "declared" {
		t.Errorf("unregistered speaker license: %+v", out.Speakers[1].License)
	}
	if len(out.Speakers[0].Styles) != 2 || out.Speakers[0].Styles[0].ID != enginetest.StyleNarratorNormal {
		t.Errorf("styles: %+v", out.Speakers[0].Styles)
	}
}

// TestListSpeakersDeclaredFromManifest checks the declared-status join when
// no config metadata exists at all.
func TestListSpeakersDeclaredFromManifest(t *testing.T) {
	h := newHarness(t)
	body, isErr := h.callTool("list_speakers", map[string]any{})
	if isErr {
		t.Fatalf("unexpected tool error: %s", body)
	}
	var out struct {
		Speakers []struct {
			Name    string `json:"name"`
			License struct {
				Status string `json:"status"`
				Name   string `json:"name"`
				Credit string `json:"credit"`
				Notes  string `json:"notes"`
			} `json:"license"`
		} `json:"speakers"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	narr := out.Speakers[0].License
	if narr.Status != "declared" || narr.Name != "Aivis Common Model License (ACML) 1.0" {
		t.Errorf("narrator declared license: %+v", narr)
	}
	if narr.Credit != "AivisSpeech: MockNarrator" {
		t.Errorf("narrator credit extraction: %q", narr.Credit)
	}
	if !strings.Contains(narr.Notes, "licenses --full") {
		t.Errorf("notes should point at the licenses subcommand: %q", narr.Notes)
	}
	// Free-form terms (no heading) still surface as declared with the first line.
	hero := out.Speakers[1].License
	if hero.Status != "declared" || !strings.Contains(hero.Name, "example.com/terms") {
		t.Errorf("heroine declared license: %+v", hero)
	}
	if hero.Credit != "" {
		t.Errorf("heroine has no credit pattern, got %q", hero.Credit)
	}
}

// TestListSpeakersSurvivesMissingAivmEndpoint pins the best-effort behavior:
// a failing /aivm_models must not fail the tool.
func TestListSpeakersSurvivesMissingAivmEndpoint(t *testing.T) {
	h := newHarness(t)
	h.mock.FailNext("/aivm_models", 404)
	body, isErr := h.callTool("list_speakers", map[string]any{})
	if isErr {
		t.Fatalf("tool must not fail when /aivm_models is unavailable: %s", body)
	}
	var out struct {
		Speakers []struct {
			License struct {
				Status string `json:"status"`
			} `json:"license"`
		} `json:"speakers"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if out.Speakers[0].License.Status != "unverified" {
		t.Errorf("expected unverified fallback: %+v", out.Speakers[0].License)
	}
}

func TestListSpeakersRejectsUnknownArgs(t *testing.T) {
	h := newHarness(t)
	body, isErr := h.callTool("list_speakers", map[string]any{"bogus": 1})
	if !isErr {
		t.Fatalf("expected error, got %s", body)
	}
	if errCode(t, body) != "invalid_arguments" {
		t.Errorf("code: %s", body)
	}
}
