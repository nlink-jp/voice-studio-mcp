package cmd

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/nlink-jp/voice-studio-mcp/internal/config"
	"github.com/nlink-jp/voice-studio-mcp/internal/engine"
	"github.com/nlink-jp/voice-studio-mcp/internal/mcpserver"
	"github.com/nlink-jp/voice-studio-mcp/internal/synth"
	"github.com/nlink-jp/voice-studio-mcp/internal/tools"
	"github.com/nlink-jp/voice-studio-mcp/internal/workdir"
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

	tools.Register(srv, newToolDeps(cfg, client, engineVersion, logger))
}

// newToolDeps assembles what every tool shares. It is the only place
// tools.Deps is built, which is what makes the work-directory resolver below
// impossible for a tool added later to forget: no tool constructs a Resolver
// of its own, they all read Deps.WorkDir.
func newToolDeps(cfg *config.Config, client *engine.Client, engineVersion string, logger *slog.Logger) *tools.Deps {
	wd := workDirResolver()
	return &tools.Deps{
		Cfg:    cfg,
		Client: client,
		Synth: &synth.Synthesizer{
			Client:        client,
			Cfg:           cfg.Synthesis,
			EngineVersion: engineVersion,
		},
		WS:      workspace.NewManager(wd.CheckBeneath, wd.LocalPath),
		WorkDir: wd,
		Logger:  logger,
	}
}

// workDirResolver builds the per-call work-directory resolver, denying this
// server's own directories.
//
// A work directory is the caller's, not ours (organization ADR-021 §4: "not a
// system location … and not the server's own config or state directory" →
// `work_dir_denied`). Without the denial a caller could name our config
// directory as its workspace root and have us synthesize into it — reading and
// rewriting our own configuration as if it were a workspace, on a model's
// say-so.
func workDirResolver() workdir.Resolver {
	return workdir.NewResolver(serverOwnedDirs()...)
}

// serverOwnedDirs lists this server's own config and state directories.
//
// There is one: the config directory. This server keeps no state on disk —
// every byte it produces goes under the caller's `work_dir`, and the
// `~/.voice-studio` default root that used to exist was deleted when ADR-021
// was adopted. If a state directory is ever reintroduced it belongs here. An
// empty one (no home) is passed on, and refuses every call rather than
// protecting nothing.
func serverOwnedDirs() []string {
	return []string{configDir()}
}

// configDir is where this server keeps its own config.toml.
//
// resolveConfig searches it and workDirResolver denies it, through this one
// expression: spelling the path twice is how a denial drifts away from the
// location it was meant to protect. Empty when the home directory cannot be
// determined.
func configDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "voice-studio-mcp")
}
