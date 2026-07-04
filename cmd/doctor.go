package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/cobra"

	"github.com/nlink-jp/voice-studio-mcp/internal/config"
	"github.com/nlink-jp/voice-studio-mcp/internal/engine"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose environment (config, engine, ffmpeg, workspace dir)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, used, err := resolveConfig(configPath)
		if err != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "NG config: %v\n", err)
			return fmt.Errorf("doctor: config check failed")
		}
		if ok := runDoctorChecks(cmd.Context(), cmd.OutOrStdout(), cfg, used); !ok {
			return fmt.Errorf("doctor: one or more checks failed")
		}
		return nil
	},
}

// runDoctorChecks writes one line per check and reports overall success.
// Informational conditions (engine not running in managed mode) do not fail
// the run; broken preconditions (missing engine command, missing ffmpeg) do.
func runDoctorChecks(ctx context.Context, out io.Writer, cfg *config.Config, usedConfig string) bool {
	ok := true

	if usedConfig == "" {
		fmt.Fprintln(out, "ok config: built-in defaults (no config.toml found)")
	} else {
		fmt.Fprintf(out, "ok config: %s\n", usedConfig)
	}

	// Engine command (managed mode only).
	if cfg.Engine.Mode == config.EngineModeManaged {
		if _, err := os.Stat(cfg.Engine.Command); err != nil {
			fmt.Fprintf(out, "NG engine command: %s not found — install AivisSpeech or set engine.command\n", cfg.Engine.Command)
			ok = false
		} else {
			fmt.Fprintf(out, "ok engine command: %s\n", cfg.Engine.Command)
		}
	}

	// Engine liveness (informational in managed mode: serve will spawn it).
	client := engine.NewClient(cfg.Engine.URL, 2*time.Second)
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	version, err := client.Version(probeCtx)
	cancel()
	switch {
	case err == nil:
		fmt.Fprintf(out, "ok engine: %s answers at %s\n", version, cfg.Engine.URL)
	case cfg.Engine.Mode == config.EngineModeManaged:
		fmt.Fprintf(out, "ok engine: not running at %s (serve will spawn it)\n", cfg.Engine.URL)
	default:
		fmt.Fprintf(out, "NG engine: nothing answers at %s (external mode requires a running engine)\n", cfg.Engine.URL)
		ok = false
	}

	// ffmpeg (needed by the master tool).
	if _, err := exec.LookPath(cfg.Master.FFmpegPath); err != nil {
		fmt.Fprintf(out, "NG ffmpeg: %q not found in PATH — master will fail (brew install ffmpeg)\n", cfg.Master.FFmpegPath)
		ok = false
	} else {
		fmt.Fprintf(out, "ok ffmpeg: %s\n", cfg.Master.FFmpegPath)
	}

	// Workspace root must be creatable/writable.
	if err := os.MkdirAll(cfg.Workspace.Dir, 0o755); err != nil {
		fmt.Fprintf(out, "NG workspace dir: %v\n", err)
		ok = false
	} else {
		fmt.Fprintf(out, "ok workspace dir: %s\n", cfg.Workspace.Dir)
	}

	return ok
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
