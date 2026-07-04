# ADR-0002: The MCP server manages the engine lifecycle, attach-first

- **Status**: Accepted (2026-07-04)

## Context

The primary users are autonomous agents (Claude Code / Cowork). Requiring a
human to pre-start the engine blocks agent workflows on manual preparation.
But the AivisSpeech GUI also starts an engine on the same port (10101), so
unconditional spawning risks double-starts and killing the user's engine.

## Decision

With `engine.mode = "managed"` (default):

1. **Attach first**: probe `GET /version` at startup; if something answers,
   connect without taking ownership (Stop is a no-op).
2. Otherwise spawn `engine.command` as a child process and poll `/version`
   until ready (first model load takes minutes; `startup_timeout_seconds`
   defaults to 180).
3. On shutdown: SIGTERM → wait `shutdown_timeout_seconds` → SIGKILL. A
   reaper goroutine started at spawn time handles zombie reaping and
   early-exit detection.

`engine.mode = "external"` connects only (tests, manual starts, remote).

## Consequences

- `serve` works with zero preparation by the agent or user.
- Safe in the presence of a GUI-managed engine or an orphan from a
  previously SIGKILLed serve (it just attaches).
- An attached engine is never reaped, so it may outlive `serve` — correct,
  since this server did not start it.
- External mode doubles as the test seam: unit and e2e tests run fully
  hermetic against an httptest mock engine.

## Alternatives considered

- **Always spawn**: double-start / GUI-kill hazards. Rejected.
- **Require a running engine**: blocks autonomy on manual prep. Rejected.
- **launchd-style resident daemon**: complexity without payoff for v1.
