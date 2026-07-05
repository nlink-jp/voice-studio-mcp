package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/nlink-jp/voice-studio-mcp/internal/config"
	"github.com/nlink-jp/voice-studio-mcp/internal/engine"
	"github.com/nlink-jp/voice-studio-mcp/internal/logging"
	"github.com/nlink-jp/voice-studio-mcp/internal/mcpserver"
	"github.com/nlink-jp/voice-studio-mcp/internal/tools"
	"github.com/nlink-jp/voice-studio-mcp/internal/transport"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the MCP stdio server",
	Long:  "Read JSON-RPC messages from stdin and serve MCP tool calls. This is the default when no subcommand is given.",
	RunE:  runServe,
}

func runServe(cmd *cobra.Command, args []string) error {
	cfg, _, err := resolveConfig(configPath)
	if err != nil {
		return err
	}

	logger, logFile, err := logging.Setup(cfg.Server.LogLevel, cfg.Server.LogFile)
	if err != nil {
		return err
	}
	if logFile != nil {
		defer logFile.Close()
	}

	tr := transport.NewStdioTransport(os.Stdin, os.Stdout)
	srv := mcpserver.New("voice-studio-mcp", Version, tr, logger)
	srv.SetInstructions(tools.Instructions)

	client := engine.NewClient(cfg.Engine.URL,
		time.Duration(cfg.Engine.RequestTimeoutSeconds)*time.Second)
	sup := engine.NewSupervisor(client, logger, engine.SupervisorOpts{
		Mode:            cfg.Engine.Mode,
		Command:         cfg.Engine.Command,
		Args:            cfg.Engine.Args,
		StartupTimeout:  time.Duration(cfg.Engine.StartupTimeoutSeconds) * time.Second,
		ShutdownTimeout: time.Duration(cfg.Engine.ShutdownTimeoutSeconds) * time.Second,
	})

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := sup.Start(ctx); err != nil {
		return fmt.Errorf("engine startup: %w", err)
	}
	// Reap a spawned engine on exit; attached engines are left running.
	defer func() {
		if err := sup.Stop(context.Background()); err != nil {
			logger.Warn("engine shutdown", "err", err)
		}
	}()

	registerTools(srv, cfg, client, logger)

	logger.Info("serving MCP over stdio", "version", Version, "engine", cfg.Engine.URL)
	if err := srv.Serve(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}
	return nil
}

// resolveConfig loads the explicit config path, or searches the default
// locations, or falls back to built-in defaults. The second return value is
// the file actually used ("" for defaults).
func resolveConfig(explicit string) (*config.Config, string, error) {
	if explicit != "" {
		cfg, err := config.Load(explicit)
		return cfg, explicit, err
	}
	home, _ := os.UserHomeDir()
	for _, c := range []string{
		filepath.Join(home, ".config", "voice-studio-mcp", "config.toml"),
		"config.toml",
	} {
		if _, err := os.Stat(c); err == nil {
			cfg, err := config.Load(c)
			return cfg, c, err
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, "", fmt.Errorf("stat %s: %w", c, err)
		}
	}
	return config.Default(), "", nil
}

func init() {
	rootCmd.AddCommand(serveCmd)
	rootCmd.RunE = runServe
}
