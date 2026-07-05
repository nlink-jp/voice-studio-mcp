# Changelog

All notable changes to this project are documented here. The format is
based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and
this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.4.1] - 2026-07-05

### Changed

- Documentation and capability strings now describe voice-studio-mcp as a
  general **multi-speaker Japanese** narrated-audio server (radio drama,
  audiobook, podcast, panel, briefing, …), not a radio-drama-only tool, and
  **state the Japanese-only limitation explicitly** — the AivisSpeech Engine's
  voice models are Japanese, so other languages are not supported. Updated
  README/README.ja, the MCP `instructions` field + `get_usage` manual, the CLI
  `--help`, AGENTS/CLAUDE, and the setup/architecture reference docs. (Also
  fixed the stale "Go 1.23+" build requirement in the setup guide → 1.25+.)

## [0.4.0] - 2026-07-05

### Added

- `.skill` packaging (ADR-0011): `skills/build.sh` + `make package-skill`
  produce `dist/multi-actor-narration.skill` — a zip whose top-level entry
  is the skill directory. Attached to releases as a **separate asset** next
  to the MCP binary, so users without the repo can import the skill directly.

### Changed

- **Bundled skill consolidated into `multi-actor-narration`** (ADR-0011),
  adopted from the `magifd2/claude-skills` experiment. One router skill now
  covers five formats over the same voice-studio pipeline: talk-podcast,
  panel-discussion, news-briefing, lesson-narration (document/theme →
  multi-speaker explainer audio) and **audio-drama** — the generalized
  successor of radio-drama (novel/script → narrated audio drama / audiobook,
  with the novel-works-specific coupling removed).
  - **BREAKING**: the skill command name changed —
    `/radio-drama` → `/multi-actor-narration`. Its description carries the
    朗読 / ラジオドラマ / オーディオブック / novel / manuscript triggers so
    equivalent requests still route to it.
- `skills/skills_test.go` now aggregates the whole multi-file skill (SKILL.md
  router + `_shared/*.md` + `<format>/FORMAT.md`) for the tool/error/schema
  coherence checks, and asserts exactly one frontmatter `SKILL.md` (the
  packaging invariant a nested manifest would violate).

### Removed

- The standalone `radio-drama` skill; its capability lives on as the
  `audio-drama` format of `multi-actor-narration`.

## [0.3.0] - 2026-07-05

### Added

- Agent-prepared workspaces (ADR-0010, settled in issue #1): every
  workspace tool accepts an optional `workspace_root` — the absolute
  path of a directory the agent created in its own writable area —
  so sandboxed MCP clients whose writes are restricted to the project
  tree can use the server. Stateless per-call parameter; the default
  root and existing calls are unchanged.
- `get_usage` tool returning the embedded operating manual (workspace
  model, production flow, script schema, recovery table), advertised
  via the MCP `instructions` field on initialize; pinned to the real
  tools/errors/schema by coherence tests.

### Changed

- All server I/O inside workspaces is kernel-contained via `os.Root`:
  symlinks leaving a workspace fail with `path_not_allowed` (the
  confused-deputy defense required once workspaces became
  agent-writable). ffmpeg inputs are re-verified as regular files
  immediately before spawn.
- Go toolchain floor raised from 1.23 to 1.25 (os.Root convenience
  APIs).

## [0.2.0] - 2026-07-04

### Added

- License collection workflow (ADR-0008): the engine's `/aivm_models`
  endpoint exposes author-declared license text embedded in AIVM
  manifests, so collection is now automated while verification stays
  human.
  - `list_speakers` reports a three-valued `license.status`:
    `verified` (human-reviewed config entry) / `declared` (manifest
    text present, name and credit extracted heuristically) /
    `unverified`. Falls back gracefully on engines without
    `/aivm_models`.
  - New `licenses` subcommand: overview table, `--full <speaker_uuid>`
    to read the full declared terms, and `--toml` to generate
    `[[speaker_metadata]]` skeletons (with a REVIEW note) for
    unreviewed speakers.
- Bundled `radio-drama` Claude Code skill (ADR-0009): the operational
  form of the agent workflow, installed via `make install-skill`;
  `skills/skills_test.go` pins skill/server coherence (tool names,
  error codes, schema fields) so they cannot drift.

## [0.1.0] - 2026-07-04

### Added

- Initial implementation: MCP stdio server that gives AI agents
  (Claude Code / Cowork) local speech-synthesis capabilities for
  radio-drama / audiobook production, backed by AivisSpeech Engine
  (VOICEVOX-compatible HTTP API).
- Six tools:
  - `list_speakers` — installed voice models + styles, joined with
    hand-maintained license metadata (`[[speaker_metadata]]` config).
  - `register_dictionary` — work-specific pronunciation dictionary
    (proper nouns), idempotent via a per-workspace record.
  - `synthesize_script` — batch synthesis of a script JSONL (async job,
    content-hash cache re-synthesizes only changed lines).
  - `synthesize_line` — synchronous single-line retakes and voice
    auditioning (`style_id` override).
  - `check_job` — job progress with per-line structured failures.
  - `master` — ffmpeg concat with per-line pauses, one-pass loudnorm,
    mp3/m4b (scene-boundary chapters), credits file generation with
    unverified-license warnings.
- Engine lifecycle supervision: managed mode spawns/reaps the engine
  (attach fallback prevents double-spawn); external mode for tests and
  manually started engines.
- Workspace scoping (one work = one workspace) with path-traversal
  defenses; canonical script JSONL schema
  (`id/scene/speaker/text/style/intensity/speed/pause_after_ms`) and
  casting table (`casting.toml`).
- `doctor` subcommand (config / engine / ffmpeg / workspace checks).
- Hermetic test suite: httptest mock engine + fake ffmpeg runner; e2e
  harness driving the built binary over stdio (`make test-e2e`); opt-in
  real-engine e2e including a full production simulation
  (`VOICE_STUDIO_TEST_REAL_ENGINE=1`).
- Per-line `volume` (engine `volumeScale`) with `synthesis.default_volume`
  config; part of the synthesis cache key.
- Documentation set: architecture reference, setup guide, and seven ADRs
  recording design rationale, mirrored in `docs/{en,ja}/`.
- Agent workflow guide (`docs/{en,ja}/reference/agent-workflow*.md`) — the
  10-step procedure Claude Code / Cowork follows from manuscript to
  mastered audio, with direction guidelines, error dispatch table, and
  human checkpoints — plus `samples/` (manuscript, reference script,
  casting template).

### Changed

- Default mastering loudness target lowered from -16 LUFS to -18 LUFS
  (audiobook range) after listening feedback; configurable via
  `master.loudnorm_i`.

[0.3.0]: https://github.com/nlink-jp/voice-studio-mcp/releases/tag/v0.3.0
[0.2.0]: https://github.com/nlink-jp/voice-studio-mcp/releases/tag/v0.2.0
[0.1.0]: https://github.com/nlink-jp/voice-studio-mcp/releases/tag/v0.1.0
