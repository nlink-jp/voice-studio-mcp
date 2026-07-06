package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/nlink-jp/voice-studio-mcp/internal/config"
	"github.com/nlink-jp/voice-studio-mcp/internal/engine"
	"github.com/nlink-jp/voice-studio-mcp/internal/enginefix"
)

var doctorFix bool

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose environment (config, engine, ffmpeg, workspace dir)",
	Long: `Diagnose the environment. Read-only by default.

With --fix, repair a managed AivisSpeech Engine that macOS refuses to launch
because its bundle is unsigned / un-notarized with mismatched nested library
signatures: the quarantine flag is stripped and every Mach-O in the engine
subtree is ad-hoc re-signed so signatures are uniform (ADR-0012). This
mutates the installed engine bundle and only touches the engine subtree
(the directory of engine.command), never the wider app.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, used, err := resolveConfig(configPath)
		if err != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "NG config: %v\n", err)
			return fmt.Errorf("doctor: config check failed")
		}
		if doctorFix {
			return runEngineFix(cmd.Context(), cmd.OutOrStdout(), cfg)
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
			checkEngineSignature(ctx, out, cfg, &ok)
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

// checkEngineSignature is informational: a spawn-blocking signature state is
// reported but does not fail doctor, because `doctor --fix` resolves it.
func checkEngineSignature(ctx context.Context, out io.Writer, cfg *config.Config, _ *bool) {
	if runtime.GOOS != "darwin" {
		return
	}
	f := &enginefix.Fixer{Runner: enginefix.ExecRunner{}}
	rep, err := f.Detect(ctx, cfg.Engine.Command)
	if err != nil {
		fmt.Fprintf(out, "ok engine signature: could not inspect (%v)\n", err)
		return
	}
	if rep.NeedsFix() {
		reason := "unsigned / mismatched nested signatures"
		if rep.Quarantined {
			reason = "quarantined + " + reason
		}
		fmt.Fprintf(out, "NG engine signature: %s (%s) — run `voice-studio-mcp doctor --fix` to repair\n", rep.Dir, reason)
	} else {
		fmt.Fprintf(out, "ok engine signature: %d Mach-O files, signature verifies\n", rep.MachOFiles)
	}
}

// runEngineFix performs the mutating repair (doctor --fix).
func runEngineFix(ctx context.Context, out io.Writer, cfg *config.Config) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("--fix is macOS-only")
	}
	if cfg.Engine.Mode != config.EngineModeManaged {
		return fmt.Errorf("--fix only applies to managed engine mode (engine.mode=%q)", cfg.Engine.Mode)
	}
	if _, err := os.Stat(cfg.Engine.Command); err != nil {
		return fmt.Errorf("engine command not found: %s", cfg.Engine.Command)
	}
	f := &enginefix.Fixer{Runner: enginefix.ExecRunner{}}
	fmt.Fprintf(out, "Repairing engine bundle: %s\n", enginefix.BundleDir(cfg.Engine.Command))
	rep, err := f.Repair(ctx, cfg.Engine.Command)
	if err != nil {
		return fmt.Errorf("engine repair: %w", err)
	}
	fmt.Fprintf(out, "ok: stripped quarantine and ad-hoc re-signed %d Mach-O file(s)\n", len(rep.Resigned))
	fmt.Fprintln(out, "The engine should now spawn. Re-run this after any AivisSpeech update.")
	return nil
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorFix, "fix", false,
		"Repair a managed engine bundle macOS refuses to launch (strip quarantine + ad-hoc re-sign; mutates the install)")
	rootCmd.AddCommand(doctorCmd)
}
