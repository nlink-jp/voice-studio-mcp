// Package tools implements the MCP tools exposed by voice-studio-mcp.
//
// Every tool returns a compact JSON summary (paths, counts, durations) —
// never audio bytes — to keep token cost low for LLM clients.
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"

	"github.com/nlink-jp/voice-studio-mcp/internal/config"
	"github.com/nlink-jp/voice-studio-mcp/internal/engine"
	"github.com/nlink-jp/voice-studio-mcp/internal/job"
	"github.com/nlink-jp/voice-studio-mcp/internal/mcpserver"
	"github.com/nlink-jp/voice-studio-mcp/internal/synth"
	"github.com/nlink-jp/voice-studio-mcp/internal/toolerr"
	"github.com/nlink-jp/voice-studio-mcp/internal/workspace"
)

// Deps carries the shared dependencies of all tools.
type Deps struct {
	Cfg    *config.Config
	Client *engine.Client
	Synth  *synth.Synthesizer
	WS     *workspace.Manager
	Jobs   *job.Manager
	// JobCtx is the server-lifetime context batch jobs run under; tying jobs
	// to the per-request ctx would abort them the moment the tool call
	// returns.
	JobCtx context.Context
	Logger *slog.Logger
}

// Register attaches all tools to the MCP server.
func Register(srv *mcpserver.Server, d *Deps) {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	if d.Jobs == nil {
		d.Jobs = job.NewManager(d.Cfg.Synthesis.Concurrency)
	}
	if d.JobCtx == nil {
		d.JobCtx = context.Background()
	}
	registerListSpeakers(srv, d)
	registerSynthesizeLine(srv, d)
	registerSynthesizeScript(srv, d)
	registerCheckJob(srv, d)
}

// unmarshalStrict decodes tool arguments, rejecting unknown fields so agent
// typos surface as invalid_arguments instead of being silently ignored.
func unmarshalStrict(args json.RawMessage, into any) error {
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	dec := json.NewDecoder(bytes.NewReader(args))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return toolerr.Newf(toolerr.CodeInvalidArguments, "invalid arguments: %v", err)
	}
	return nil
}
