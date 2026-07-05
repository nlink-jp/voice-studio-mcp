# ADR-0010: voice-studio works in the workplace the agent prepared

- **Status**: Accepted (2026-07-05)
- **Origin**: settled from the three-proposal discussion in [Issue #1](https://github.com/nlink-jp/voice-studio-mcp/issues/1)

## Context

The server's contract — inputs (script, casting) as files inside a
workspace, outputs returned as paths — implicitly assumed the agent and the
server share an unrestricted filesystem view. Sandboxed MCP clients break
that assumption:

- **Class A (shared FS, restricted writes)**: e.g. the Claude Code local
  sandbox, where writes are typically limited to the project directory and
  tmp — the agent cannot write scripts into the default `~/.voice-studio`.
- **Class B (no shared FS)**: VM/container-hosted clients; file handoff
  collapses entirely.

Additionally, desktop-class hosts start MCP servers once at app startup
from global config, and **one process may serve several concurrent
conversations** — so the workspace location must travel **through the MCP
session**, not through startup configuration. And a stateful,
file-mediated contract cannot be fully explained by tool descriptions;
clients without the bundled skill (ADR-0009) operate by trial and error.

## Decision

The principle, in one line: **"voice-studio works in the workplace the
agent prepared."** Implemented with minimal machinery:

1. **Optional `workspace_root` parameter** on every workspace tool: the
   absolute path of a directory the agent created in its own writable
   area — creating it IS the preparation; there is no dedicated tool, no
   ownership marker, no GUID. Omitted, the configured default root applies
   (fully backward compatible). Validation: absolute, existing, a
   directory. The `<workspace_root>/<workspace_id>/…` layout is unchanged
   (one root, many works). It is a stateless per-call parameter — one
   process serves several conversations, so no server-side session state
   (the same reasoning as ADR-0005).
2. **Kernel-enforced containment via `os.Root` (mandatory)**: once the
   server works where the agent can write, a symlink planted in the
   workspace becomes a confused-deputy escalation (the old design's hidden
   protection — the agent could NOT write into the workspace — inverts;
   that inversion is the security core of this ADR). All server I/O inside
   a workspace goes through `os.Root`; symlinks leaving the root fail at
   the kernel boundary and map to `path_not_allowed`. `ResolveInside`
   remains as a lexical pre-check for friendly errors.
   - **Residual ffmpeg gap**: external processes cannot inherit os.Root.
     Every input is re-verified as an existing regular file via
     `os.Root.Lstat` immediately before spawn; the verify-to-spawn race is
     accepted under the local single-user threat model.
   - Go floor raised to 1.25 (`os.Root` landed in 1.24; the
     ReadFile/WriteFile/MkdirAll/RemoveAll/Rename APIs used here are 1.25).
3. **Self-description**: the `get_usage` tool returns the workspace model,
   production flow, script schema, and recovery table (go:embed markdown,
   pinned to the real tools/errors/schema by coherence tests), and the
   initialize `instructions` field makes it discoverable.

Class B stays **explicitly out of scope** (the audio consumer is the human
on the host, so the practical damage is input-side only; keeping a single
data path wins).

## Consequences

- Class A is solved: the agent creates a workspace inside its project
  directory and passes the same `workspace_root` on every call. Side
  benefit: scripts and casting become committable project artifacts.
- Tool count 7→8 (get_usage). Existing calls work unchanged.
- Symlink regression tests (script / casting / wav directory replacement →
  all `path_not_allowed`) pin the containment.
- Building from source now requires Go 1.25+.

## Alternatives considered

1. **Server-minted workspaces** (the issue's original proposal):
   `ensure_workspace(project_dir)` creating `.voice-studio-<GUID>/` with an
   ownership marker. Works, but ceremonial: under "the agent prepares the
   workplace", the extra tool, GUID, marker, and allowed-roots list all
   evaporate. Rejected.
2. **Inline inputs** (counter-proposal): `script`/`casting` content-string
   parameters removing the shared-FS requirement for inputs (also solving
   Class B's input side). Costs tokens (~a 30KB script per call) that
   Class A never needs to pay, and doubles the data paths. Recorded as the
   revisit-option if Class B becomes a real requirement. Rejected (for now).
3. **Lexical checks only, no os.Root**: defenseless against TOCTOU and
   symlinks in agent-writable areas. Rejected.
