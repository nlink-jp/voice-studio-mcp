package cmd

import (
	"context"
	"log/slog"
	"time"

	"github.com/nlink-jp/voice-studio-mcp/internal/config"
	"github.com/nlink-jp/voice-studio-mcp/internal/engine"
	"github.com/nlink-jp/voice-studio-mcp/internal/mcpserver"
	"github.com/nlink-jp/voice-studio-mcp/internal/synth"
	"github.com/nlink-jp/voice-studio-mcp/internal/tools"
	"github.com/nlink-jp/voice-studio-mcp/internal/workspace"
)

// registerTools wires the MCP tools onto the server. Called after the engine
// is reachable, so the engine version (part of every cache key) can be read
// synchronously here.
func registerTools(srv *mcpserver.Server, cfg *config.Config, client *engine.Client, logger *slog.Logger) {
	engineVersion := "unknown"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if v, err := client.Version(ctx); err == nil {
		engineVersion = v
	} else {
		logger.Warn("could not read engine version for cache keys", "err", err)
	}
	cancel()

	tools.Register(srv, &tools.Deps{
		Cfg:    cfg,
		Client: client,
		Synth: &synth.Synthesizer{
			Client:        client,
			Cfg:           cfg.Synthesis,
			EngineVersion: engineVersion,
		},
		WS:     workspace.NewManager(cfg.Workspace.Dir),
		Logger: logger,
	})
}
