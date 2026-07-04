//go:build e2e

// Package e2e drives the built voice-studio-mcp binary end-to-end over its
// stdio JSON-RPC interface, acting as a dummy MCP client.
//
// Run via `make test-e2e`, or manually:
//
//	VOICE_STUDIO_TEST_BINARY=$PWD/dist/voice-studio-mcp go test -tags e2e ./e2e/...
package e2e

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// Harness owns one running server process.
type Harness struct {
	t      *testing.T
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	lines  chan []byte
	nextID atomic.Int64
}

// requireBinary locates the binary under test, skipping the test when it has
// not been built.
func requireBinary(t *testing.T) (string, bool) {
	t.Helper()
	if p := os.Getenv("VOICE_STUDIO_TEST_BINARY"); p != "" {
		return p, true
	}
	p, err := filepath.Abs(filepath.Join("..", "dist", "voice-studio-mcp"))
	if err == nil {
		if _, statErr := os.Stat(p); statErr == nil {
			return p, true
		}
	}
	t.Skip("binary not built; run `make build` or set VOICE_STUDIO_TEST_BINARY")
	return "", false
}

// Start launches `binary serve --config configPath`.
func Start(t *testing.T, binary, configPath string) *Harness {
	t.Helper()
	args := []string{"serve"}
	if configPath != "" {
		args = append(args, "--config", configPath)
	}
	cmd := exec.Command(binary, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", binary, err)
	}

	lines := make(chan []byte, 16)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		for scanner.Scan() {
			b := make([]byte, len(scanner.Bytes()))
			copy(b, scanner.Bytes())
			lines <- b
		}
	}()

	h := &Harness{t: t, cmd: cmd, stdin: stdin, lines: lines}
	t.Cleanup(h.Close)
	return h
}

// Close ends the server (stdin EOF → clean shutdown) and reaps it.
func (h *Harness) Close() {
	_ = h.stdin.Close()
	_ = h.cmd.Wait()
}

// Call sends one JSON-RPC request and waits for its response.
func (h *Harness) Call(method string, params any, timeout time.Duration) (json.RawMessage, error) {
	id := h.nextID.Add(1)
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		req["params"] = params
	}
	if err := json.NewEncoder(h.stdin).Encode(req); err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	deadline := time.After(timeout)
	for {
		select {
		case line, ok := <-h.lines:
			if !ok {
				return nil, fmt.Errorf("server stdout closed before response (method=%s id=%d)", method, id)
			}
			var resp struct {
				ID     json.Number     `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  *struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(line, &resp); err != nil {
				return nil, fmt.Errorf("parse response: %w (line=%q)", err, string(line))
			}
			respID, err := resp.ID.Int64()
			if err != nil || respID != id {
				continue // out-of-order; skip
			}
			if resp.Error != nil {
				return nil, fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
			}
			return resp.Result, nil
		case <-deadline:
			return nil, fmt.Errorf("timeout after %v waiting for response (method=%s id=%d)", timeout, method, id)
		}
	}
}

// Initialize performs the MCP handshake.
func (h *Harness) Initialize() error {
	return h.InitializeWithTimeout(10 * time.Second)
}

// InitializeWithTimeout performs the MCP handshake with a custom timeout.
// The server answers initialize only after the engine is ready, so managed
// mode with a cold engine (first model load) needs minutes, not seconds.
func (h *Harness) InitializeWithTimeout(timeout time.Duration) error {
	_, err := h.Call("initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "e2e", "version": "1"},
	}, timeout)
	if err != nil {
		return err
	}
	return json.NewEncoder(h.stdin).Encode(map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	})
}

// CallTool invokes tools/call and returns the first content block plus the
// isError flag.
func (h *Harness) CallTool(name string, arguments any, timeout time.Duration) (json.RawMessage, bool, error) {
	res, err := h.Call("tools/call", map[string]any{
		"name":      name,
		"arguments": arguments,
	}, timeout)
	if err != nil {
		return nil, false, err
	}
	var wrap struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(res, &wrap); err != nil {
		return nil, false, fmt.Errorf("parse tools/call result: %w", err)
	}
	if len(wrap.Content) == 0 {
		return nil, wrap.IsError, fmt.Errorf("tools/call returned no content")
	}
	return json.RawMessage(wrap.Content[0].Text), wrap.IsError, nil
}
