# voice-studio-mcp Architecture

> Covers the v0.1.0 line. The "why" of each decision lives in the ADRs
> ([../adr/](../adr/)); this document provides the overall picture and an
> index into those decisions.

## 0. In one sentence

An MCP stdio server that turns an agent-authored script JSONL into finished
radio-drama / audiobook audio using a local AivisSpeech Engine and ffmpeg —
**judgment and creativity stay on the agent side; deterministic heavy
lifting lives here**. That split drives every design below.

```
┌────────────────────┐  stdio (JSON-RPC / MCP)
│ Agent              │◄────────────────────────┐
│ (Claude Code etc.) │                          │
│  · script writing  │   ┌──────────────────────┴──┐
│  · casting decisions│  │ voice-studio-mcp (Go)    │
│  · direction        │  │  tools ── synth ── engine │──► AivisSpeech Engine
│  · license review   │  │    │       │        client│    (HTTP 127.0.0.1:10101,
└────────────────────┘   │    │     cache             │     managed child process)
        │ handoff via files│  job    workspace        │
        ▼                 │  master ────────────────►│──► ffmpeg
  <workspace>/script/     └──────────────────────────┘
  <workspace>/casting.toml
```

## 1. Process boundaries and trust model

| Boundary | Notes |
|----------|-------|
| stdio | JSON-RPC with the MCP client. **stdout is transport-only**; logs go to stderr / file |
| localhost HTTP | AivisSpeech Engine (our child process in managed mode). Zero external network traffic |
| Child processes | The engine (ADR-0002) and ffmpeg (via the Runner interface). Exit codes and stderr tails surface in structured errors |
| Filesystem | Writes stay under the workspace root. Agent-supplied relative paths go through `ResolveInside` (absolute paths and `..` rejected) |

There are no secrets (no cloud APIs, no auth). The main threat is an
agent's bad arguments writing/deleting outside the workspace — countered by
workspace_id regex validation, path confinement, and a direct-child check
on deletion.

## 2. Package layout and dependency direction

```
cmd (cobra: serve/doctor/version)
 └─ tools ──► workspace, engine, script, synth, job, master, toolerr, config
              synth ──► engine, script, workspace
              job   ──► toolerr only (synthesis injected as Run closures)
              master──► script, synth (WAV parsing), workspace
mcpserver ──► jsonrpc, transport, toolerr   (the protocol layer knows no domain)
```

- External Go dependencies: `cobra` and `BurntSushi/toml` only. The engine
  and ffmpeg are **runtime** dependencies, never code dependencies.
- The skeleton (transport/jsonrpc/mcpserver/toolerr/logging/e2e harness) is
  the org-standard pattern ported from data-toolbox-mcp.

## 3. Data flow

### 3.1 synthesize_script (the core)

```
script JSONL ─ Parse (full scan, errors collected) ─ casting.Validate (all refs)
    │  any failure → invalid_script / casting_unresolved (details: first 20)
    ▼
per line: cache key (ADR-0004) ─ hit → skip
    │ miss
    ▼
POST /audio_query → override owned keys only (ADR-0003) → POST /synthesis
    ▼
wav/<id>.wav (temp+rename) + cache/index.json updated atomically per line
```

The key property is that validation completes synchronously before
enqueueing: the agent learns about every problem up front instead of
discovering half-failed output thirty minutes later. Jobs run under the
server-lifetime context with a shared semaphore (ADR-0005).

### 3.2 master

```
verify every line has a WAV (missing → master_incomplete)
 → render one silence WAV per distinct pause value → concat.txt (interleaved)
 → (m4b only) ffmetadata chapters from scene boundaries (precomputed times)
 → single ffmpeg run: concat → loudnorm (-18 LUFS default) → mp3/m4b
 → credits.txt from casting + unverified_models warnings (ADR-0007)
```

### 3.3 Contracts (the agent boundary)

- The **script JSONL schema** (`internal/script.Line`) is canonical; the
  future radio-drama skill (separate project) references it.
- Large inputs (scripts) are passed **by file path**, never inline. Outputs
  return **paths + summaries only** — audio bytes never cross the MCP
  boundary. Both are deliberate token economics.

## 4. Workspace (one work = one workspace)

```
~/.voice-studio/<id>/
├── script/           script JSONL (agent-authored)
├── casting.toml      casting + license-check record (ADR-0007)
├── dict/words.json   dictionary registration record (idempotency basis)
├── wav/<line_id>.wav per-line output (retakes overwrite by id)
├── cache/index.json  synthesis cache (ADR-0004)
└── master/           finished audio + credits + tmp (concat.txt kept for debugging)
```

Scoping dictionaries, caches, and jobs per work makes parallel productions
and per-work deletion safe.

## 5. Error model

Every tool error is structured `{code, message, details}` JSON (a text
block with isError=true). Principles:

- **code is a stable slug for agent branching** (invalid_script,
  casting_unresolved, engine_unavailable, master_incomplete, …).
- **details carry everything needed to fix the input** (invalid-line lists,
  unmapped speakers, missing line ids, ffmpeg exit code + stderr tail),
  bounded at 20 entries + a truncated flag.
- **message contains the recovery procedure** (e.g. job_not_found says
  "re-run and the cache makes it differential"). Since the callers are
  autonomous agents, error wording is the recovery loop's UX.

## 6. Testing strategy

| Layer | How | Notes |
|-------|-----|-------|
| Unit | `go test ./...` | Passes without AivisSpeech / ffmpeg (org rule). Engine mocked by `enginetest` httptest server (unknown-field responses, fault injection); ffmpeg faked via the Runner interface |
| E2E (mock) | `make test-e2e` | Dummy MCP client harness drives the built binary over stdio; external mode + mock engine + ffmpeg stub covers the full flow |
| E2E (real) | `VOICE_STUDIO_TEST_REAL_ENGINE=1` | Opt-in. Real engine spawn→synthesis→reap (real_engine_test.go) and a full production simulation that casts dynamically from live data through to mp3/m4b (real_engine_full_test.go) |

Structurally, "external mode" (ADR-0002) doubles as the mock-connection
test seam.

## 7. Decision index

| ADR | Decision |
|-----|----------|
| [0001](../adr/0001-aivisspeech-engine-backend.md) | AivisSpeech Engine only in v1 (SBV2 quality under LGPL with official macOS support) |
| [0002](../adr/0002-managed-engine-lifecycle.md) | Managed child-process engine, attach-first |
| [0003](../adr/0003-audioquery-passthrough.md) | AudioQuery as a map passthrough; only the sampling rate is forced |
| [0004](../adr/0004-content-hash-cache.md) | Content-hash cache (pauses excluded, engine version included) |
| [0005](../adr/0005-in-memory-jobs.md) | No job persistence (recovery = cache-backed re-run) |
| [0006](../adr/0006-mastering-pipeline.md) | Concat demuxer + one-pass loudnorm (-18 LUFS); two volume knobs |
| [0007](../adr/0007-license-metadata-as-data.md) | Model terms as human-recorded data carried by the tools |

## 8. Out of scope (deliberately, in v1)

Script conversion / casting judgment (the agent's job); BGM/SE mixing
(post-process in a DAW); voice-model training or cloning; cloud TTS;
streaming playback; Windows/Linux (engine management is OS-specific, so v1
focuses on darwin-arm64).
