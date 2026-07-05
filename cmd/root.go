package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var configPath string

var rootCmd = &cobra.Command{
	Use:   "voice-studio-mcp",
	Short: "Local TTS voice studio MCP server",
	Long: `voice-studio-mcp exposes local multi-speaker Japanese speech synthesis
(AivisSpeech Engine) as an MCP server: speaker catalog with license metadata,
pronunciation dictionaries, batch script synthesis, and mastering for narrated
audio (radio drama, audiobook, podcast, briefing, ...). Japanese only.

When invoked with no subcommand, behaves like ` + "`voice-studio-mcp serve`" + ` and reads JSON-RPC messages from stdin.`,
	// Don't dump the usage help on RunE errors; cobra still prints "Error: ..." to stderr.
	SilenceUsage: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "",
		"Path to config.toml (default: search ~/.config/voice-studio-mcp/config.toml then ./config.toml)")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		// cobra has already printed "Error: ..." to stderr.
		_ = err
		os.Exit(1)
	}
}
