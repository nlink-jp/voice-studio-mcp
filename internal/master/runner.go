// Package master concatenates per-line WAVs into a distributable audio file
// (mp3 / m4b) via ffmpeg: silence insertion, loudness normalization,
// chapters, and license credit generation.
package master

import (
	"bytes"
	"context"
	"os/exec"
)

// Runner executes an external command. It exists so tests can fake ffmpeg;
// production uses ExecRunner.
type Runner interface {
	Run(ctx context.Context, name string, args []string) (stdout, stderr []byte, exitCode int, err error)
}

// ExecRunner runs commands with os/exec.
type ExecRunner struct{}

// Run implements Runner.
func (ExecRunner) Run(ctx context.Context, name string, args []string) ([]byte, []byte, int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	} else if err != nil {
		exitCode = -1
	}
	return stdout.Bytes(), stderr.Bytes(), exitCode, err
}
