package engine

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/nlink-jp/voice-studio-mcp/internal/toolerr"
)

// Supervisor owns the engine process lifecycle.
//
// mode "managed": if the engine already answers at the configured URL the
// supervisor attaches to it (and never kills it); otherwise it spawns the
// configured command and waits until /version responds. mode "external":
// attach only — Start fails when nothing is listening.
type Supervisor struct {
	client *Client
	logger *slog.Logger

	mode    string // config.EngineModeManaged / EngineModeExternal
	command string
	args    []string

	startupTimeout  time.Duration
	shutdownTimeout time.Duration
	probeInterval   time.Duration

	// Set only when we spawned the process ourselves. waitCh is closed by the
	// reaper goroutine once the process has been Wait()ed; exitCode is written
	// before the close (happens-before for readers that observe the close).
	cmd      *exec.Cmd
	waitCh   chan struct{}
	exitCode int
}

// SupervisorOpts configures a Supervisor.
type SupervisorOpts struct {
	Mode            string
	Command         string
	Args            []string
	StartupTimeout  time.Duration
	ShutdownTimeout time.Duration
	// ProbeInterval is how often /version is polled while waiting for the
	// engine to come up. Zero means 500ms.
	ProbeInterval time.Duration
}

// NewSupervisor builds a Supervisor around an engine client.
func NewSupervisor(client *Client, logger *slog.Logger, opts SupervisorOpts) *Supervisor {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	probe := opts.ProbeInterval
	if probe <= 0 {
		probe = 500 * time.Millisecond
	}
	return &Supervisor{
		client:          client,
		logger:          logger,
		mode:            opts.Mode,
		command:         opts.Command,
		args:            opts.Args,
		startupTimeout:  opts.StartupTimeout,
		shutdownTimeout: opts.ShutdownTimeout,
		probeInterval:   probe,
	}
}

// Spawned reports whether the supervisor owns a child process.
func (s *Supervisor) Spawned() bool { return s.cmd != nil }

// Start makes the engine reachable, spawning it when allowed and necessary.
func (s *Supervisor) Start(ctx context.Context) error {
	// Attach path: something is already answering (a manually started engine
	// or the AivisSpeech GUI). Never take ownership of it — killing the
	// user's GUI-managed engine on shutdown would be hostile, and this also
	// prevents double-spawn after a previous serve was SIGKILLed.
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	_, err := s.client.Version(probeCtx)
	cancel()
	if err == nil {
		s.logger.Info("attached to running engine", "url", s.client.BaseURL())
		return nil
	}

	if s.mode != "managed" {
		return toolerr.Newf(toolerr.CodeEngineUnavailable,
			"engine.mode is %q and no engine answers at %s — start it manually or switch to managed mode",
			s.mode, s.client.BaseURL())
	}

	if _, err := os.Stat(s.command); err != nil {
		return toolerr.Newf(toolerr.CodeEngineUnavailable,
			"engine command not found: %s (install AivisSpeech or fix engine.command)", s.command).
			WithDetails(map[string]any{"command": s.command})
	}

	cmd := exec.Command(s.command, s.args...)
	if stderr, err := cmd.StderrPipe(); err == nil {
		go s.pipeToLog(stderr, "engine.stderr")
	}
	if stdout, err := cmd.StdoutPipe(); err == nil {
		go s.pipeToLog(stdout, "engine.stdout")
	}
	if err := cmd.Start(); err != nil {
		return toolerr.Newf(toolerr.CodeEngineUnavailable, "spawn engine: %v", err)
	}
	s.cmd = cmd
	s.waitCh = make(chan struct{})
	go func() {
		err := cmd.Wait()
		s.exitCode = -1
		if cmd.ProcessState != nil {
			s.exitCode = cmd.ProcessState.ExitCode()
		}
		_ = err
		close(s.waitCh)
	}()
	s.logger.Info("spawned engine", "command", s.command, "pid", cmd.Process.Pid)

	// Wait for /version to answer. Model loading can take minutes on first run.
	deadline := time.Now().Add(s.startupTimeout)
	for {
		probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		_, err := s.client.Version(probeCtx)
		cancel()
		if err == nil {
			s.logger.Info("engine is ready", "url", s.client.BaseURL())
			return nil
		}
		select {
		case <-s.waitCh:
			code := s.exitCode
			s.cmd = nil
			return toolerr.Newf(toolerr.CodeEngineUnavailable,
				"engine process exited during startup (exit code %d) — see server log for engine output", code).
				WithDetails(map[string]any{"exit_code": code})
		default:
		}
		if time.Now().After(deadline) {
			_ = s.Stop(context.Background())
			return toolerr.Newf(toolerr.CodeEngineUnavailable,
				"engine did not become ready within %s", s.startupTimeout).
				WithDetails(map[string]any{"startup_timeout": s.startupTimeout.String()})
		}
		select {
		case <-ctx.Done():
			_ = s.Stop(context.Background())
			return ctx.Err()
		case <-time.After(s.probeInterval):
		}
	}
}

// Stop terminates a spawned engine (SIGTERM, then SIGKILL after the shutdown
// timeout) and reaps it. No-op when the supervisor only attached.
func (s *Supervisor) Stop(ctx context.Context) error {
	if s.cmd == nil || s.cmd.Process == nil {
		return nil
	}
	cmd := s.cmd
	waitCh := s.waitCh
	s.cmd = nil

	select {
	case <-waitCh:
		return nil // already exited and reaped
	default:
	}

	s.logger.Info("stopping engine", "pid", cmd.Process.Pid)
	_ = cmd.Process.Signal(syscall.SIGTERM)

	select {
	case <-waitCh:
		return nil
	case <-time.After(s.shutdownTimeout):
		s.logger.Warn("engine ignored SIGTERM; killing", "pid", cmd.Process.Pid)
		_ = cmd.Process.Kill()
		<-waitCh
		return nil
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		<-waitCh
		return ctx.Err()
	}
}

func (s *Supervisor) pipeToLog(r io.Reader, tag string) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 64*1024)
	for sc.Scan() {
		s.logger.Debug(tag, "line", sc.Text())
	}
}
