# CLAUDE.md — voice-studio-mcp

**Organization rules (mandatory): https://github.com/nlink-jp/.github/blob/main/CONVENTIONS.md**

## Project overview

MCP stdio server that gives AI agents (Claude Code / Cowork) local
speech-synthesis capabilities for radio-drama / audiobook production.
Wraps AivisSpeech Engine (VOICEVOX-compatible API); the agent does the
intellectual work (script conversion, casting, direction), this server
does the mechanical work (synthesis, caching, retakes, mastering).
Skeleton ported from `data-toolbox-mcp`.

## Non-negotiable rules

- **Tests are mandatory** — write them with the implementation
- **Never `go build` directly** — always `make build` (outputs to `dist/`)
- **Docs in sync** — update `README.md` and `README.ja.md` together
- **Small, typed commits** — `feat:`, `fix:`, `test:`, `chore:`, `docs:`, `refactor:`, `security:`
- **stdout is sacred** — the MCP transport runs over stdout; logs MUST go to stderr
- **Hermetic tests** — `go test ./...` must pass without AivisSpeech or
  ffmpeg installed (mock engine + fake runner); real-engine e2e is opt-in

## Build & test

```sh
make build      # → dist/voice-studio-mcp (auto-codesign on darwin)
make test       # go test ./...
make test-e2e   # build + mock-engine e2e over stdio
make package    # build-all + zip + notarize darwin
```

## Key design points (see AGENTS.md for the full list)

- AudioQuery is `map[string]any` (passthrough of engine-specific fields)
- Output sampling rate forced per config for lossless concat
- `pause_after_ms` excluded from the synthesis cache key
- Jobs are in-memory only; `job_not_found` guides recovery
- Managed engine mode attaches before spawning
- Script JSONL schema in `internal/script` is the canonical contract for
  the future radio-drama skill

## Design references

- `docs/ja/voice-studio-mcp-rfp.ja.md` / `docs/en/voice-studio-mcp-rfp.md`
  — approved RFP; canonical source for scope decisions.
