package tools

import (
	"context"
	_ "embed"
	"encoding/json"

	"github.com/nlink-jp/voice-studio-mcp/internal/mcpserver"
)

// usageMarkdown is the client-neutral operating manual returned by get_usage
// (ADR-0010: clients without the bundled multi-actor-narration skill should not have
// to operate this stateful, file-mediated server by trial and error).
// Coherence with the real tools/errors/schema is pinned by usage_test.go.
//
//go:embed usage.md
var usageMarkdown string

// Instructions is the short initialize-time hint that makes get_usage
// discoverable (surfaced via the MCP `instructions` field).
const Instructions = "voice-studio-mcp produces multi-speaker narrated Japanese audio " +
	"(radio drama, audiobook, podcast, briefing, ...) from script JSONL via a local TTS engine. " +
	"Japanese only -- other languages are not supported. It is stateful and file-mediated: " +
	"inputs live in a workspace directory, outputs are returned as file paths (never audio bytes). " +
	"Call the get_usage tool before your first production to learn the workspace model, " +
	"the script schema, and the recovery table."

func registerGetUsage(srv *mcpserver.Server, d *Deps) {
	srv.RegisterTool(mcpserver.Tool{
		Name: "get_usage",
		Description: "Return this server's operating manual (markdown): workspace model and workspace_root, " +
			"production flow, script JSONL schema, casting table format, and the error recovery table. " +
			"Call it once before your first production.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		var in struct{}
		if err := unmarshalStrict(args, &in); err != nil {
			return nil, err
		}
		return mcpserver.RawResult{
			Content: []mcpserver.ContentBlock{{Type: "text", Text: usageMarkdown}},
		}, nil
	})
}
