package cmd

import (
	"log/slog"

	"github.com/nlink-jp/voice-studio-mcp/internal/config"
	"github.com/nlink-jp/voice-studio-mcp/internal/engine"
	"github.com/nlink-jp/voice-studio-mcp/internal/mcpserver"
)

// registerTools wires the MCP tools onto the server. Kept in its own file so
// serve.go stays a pure lifecycle description.
func registerTools(srv *mcpserver.Server, cfg *config.Config, client *engine.Client, logger *slog.Logger) {
	// Tools are added incrementally; see internal/tools.
	_ = srv
	_ = cfg
	_ = client
	_ = logger
}
