# Changelog

All notable changes to this project are documented here. The format is
based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and
this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.5.4] - 2026-09-21

### Fixed

- **A file that is not in the workspace is reported with the path that was
  looked at.** The error used to read `openat narration.wav: no such file or
  directory` — a name relative to a directory the message did not mention. The
  workspace is a level below the `work_dir` the caller names, which is not where
  an agent naturally puts a file; the same bare sentence in voice-scribe sent a
  real agent off inventing a directory (2026-09-14). That fix reached the
  scribes and image-forge and not this server, which shares a workspace with
  video-studio-mcp. Every workspace file operation now names the absolute path and says
  what the name is relative to.

### Documentation

- The English architecture and setup references, `samples/README.md` and the
  bundled skill's argument hint still named `~/.voice-studio` and
  `workspace-root`; the Japanese references had been corrected and these had not.

## [0.5.3] - 2026-09-14

### Added

- `TestEveryRequiredNameIsDeclared` — a schema that lists a name in `required`
  without declaring it in `properties` makes a strict client refuse the whole
  tool list (Vertex AI: "schema at top-level requires unspecified property").
  data-toolbox-mcp shipped exactly that and broke a session outright; the
  existing contract test checked declared ⇒ required only, so the fleet is
  pinned in both directions now.

## [0.5.2] - 2026-09-14

### Fixed

- **The bundled `multi-actor-narration` skill still taught `workspace_root`.**
  It is installed into the agent's own skill directory and read as instruction,
  so a stale argument there is not a documentation wart — it told the agent to
  make a call 0.5.0 refuses, described the argument as optional, and named
  `~/.voice-studio` as the fallback. All eight files now teach `work_dir` as
  required with no default.
- **The initialize `instructions` field never mentioned `work_dir`.** It is the
  first thing the model reads about this server — before any tool list — and it
  still described "a workspace directory you prepare" while every tool required
  an argument it did not name. It now states the contract: `work_dir` is the
  absolute path of a directory you can read back, required, with no default.

### Added

- `TestInstructionsNameTheWorkDirContract` — the schema and description tests
  walked `tools/list`; nothing walked what `initialize` returns (ADR-0013).
- `TestSkillNamesNoRetiredArgument` / `TestSkillTeachesWorkDirAsRequired` —
  every existing skill test checked that something is present; none checked
  that a retired thing is absent, which is how the skill drifted.

## [0.5.1] - 2026-09-13

### Fixed

- **The `~/.voice-studio` root was announced as gone in 0.5.0 but was still
  half there**: `[workspace] workspace_dir` still decoded into a field no tool
  read, `doctor` still created and reported the directory, and
  `config.example.toml` still offered the key. It is gone, and a config still
  carrying it now fails to load naming `work_dir` as the replacement.
- The README workspace layout, the architecture reference and the setup
  walkthrough (both languages) still showed `~/.voice-studio/<workspace_id>/`
  and ran `synthesize_script` without a `work_dir` — which 0.5.0 refuses. They
  use a caller-prepared directory throughout.

### Added

- A contract test walking every registered tool: no retired name in a schema or
  a description, and `work_dir` required wherever it is declared. ADR-0013
  asked for this test and the release shipped without it.

## [0.5.0] - 2026-09-13

### Changed

- **Breaking: `workspace_root` is now `work_dir`, required by every workspace
  tool** (`synthesize_script`, `synthesize_line`, `register_dictionary`,
  `master`). It means the absolute path of a directory the caller can read back;
  the workspace is `<work_dir>/<workspace_id>/`. A call still sending
  `workspace_root` (or `workspaceRoot` / `workspace_dir`) is refused with
  `work_dir_required` naming the replacement. See
  [ADR-0013](docs/en/adr/0013-work-dir-contract.md); organization ADR-021.
- **Breaking: the `~/.voice-studio` default root is gone.** Omitting the argument
  used to write there, which no calling agent can open: the synthesis succeeded
  and the path it returned did not. Anything already in `~/.voice-studio` is left
  alone.
- A runtime may supply the directory instead of the model: the server reads
  `_meta["jp.nlink/work_dir"]` when the argument is absent. The argument wins.

### Added

- `work_dir_required`, `work_dir_invalid`, `work_dir_not_found`,
  `work_dir_not_writable`, `work_dir_denied` — five codes that say which part of
  the contract failed. The work directory must already exist (the server does not
  create it), be writable, and not be a system or credential location.

## [0.4.6] - 2026-08-31

### Changed

- The `workspace_root` argument now says plainly that the caller should pass a
  root it can read back: every result is returned as a path under that root, so
  a workspace the caller cannot open leaves it holding a path to nothing. Text
  only — the behaviour is unchanged.

## [0.4.5] - 2026-07-26

### Fixed

- **`--version` now works.** The binary only implemented a `version`
  subcommand, so `voice-studio-mcp --version` failed with "unknown flag" — and
  the shared org homebrew formula template tests exactly that invocation, so
  `brew test voice-studio-mcp` failed. `rootCmd.Version` is now set, which
  makes cobra provide the flag. The `version` subcommand is unchanged, and both
  spellings print the identical string (bare version, no "<name> version "
  prefix).

## [0.4.4] - 2026-07-12

### Changed

- **`LICENSE` is now bundled** in the release archive alongside `README.md`,
  per `nlink-jp/.github` CONVENTIONS.md §Release Archive Standard. The archive
  name (`voice-studio-mcp-vX.Y.Z-darwin-arm64.zip`) and canonical in-archive
  binary name were already compliant. v1 targets macOS Apple Silicon only, so
  it already shipped **darwin/arm64 only** (no Intel / universal / Linux /
  Windows) — unchanged.
- **darwin code-signature identifier** is now explicitly pinned to the
  canonical `voice-studio-mcp` via `codesign -i`.
- **Dropped the `-s -w` linker strip flags** from `LDFLAGS`, aligning with the
  org-standard build flags (darwin-only, so purely a consistency change).

No change to the binary's behaviour — a packaging / build-config release.

## [0.4.3] - 2026-07-05

### Fixed

- `doctor` reported the engine signature state with a static message
  ("unsigned / mismatched nested signatures") even when the signature
  verifies and only the download quarantine flag is set. It now reports
  the conditions actually detected: `NG` when the signature fails to
  verify, `warn` when it verifies but is quarantined, `ok` otherwise.
  (Verified on a real install: `doctor --fix` cleared quarantine on 2520
  files and idempotently re-signed 107 Mach-O files with the signature
  still verifying and the engine unaffected.)

## [0.4.2] - 2026-07-05

### Added

- `doctor --fix` repairs a managed AivisSpeech Engine that macOS refuses to
  launch (ADR-0012). The engine ships as an unsigned/un-notarized PyInstaller
  bundle whose nested libraries carry mismatched TeamID signatures; modern
  macOS kills it at load time, and stripping quarantine alone is not enough on
  some machines. `--fix` strips quarantine and ad-hoc re-signs every Mach-O in
  the engine subtree inside-out so signatures are uniform — automating the
  proven manual fix. Bare `doctor` detects and reports the condition read-only;
  `--fix` mutates the install (re-run after each AivisSpeech update). Chosen
  over forking/notarizing or reimplementing the engine (see ADR-0012).
- The engine supervisor's spawn-early-exit error now points at `doctor --fix`.

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

[0.4.5]: https://github.com/nlink-jp/voice-studio-mcp/releases/tag/v0.4.5
[0.4.4]: https://github.com/nlink-jp/voice-studio-mcp/releases/tag/v0.4.4
[0.4.3]: https://github.com/nlink-jp/voice-studio-mcp/releases/tag/v0.4.3
[0.4.2]: https://github.com/nlink-jp/voice-studio-mcp/releases/tag/v0.4.2
[0.4.1]: https://github.com/nlink-jp/voice-studio-mcp/releases/tag/v0.4.1
[0.4.0]: https://github.com/nlink-jp/voice-studio-mcp/releases/tag/v0.4.0
[0.3.0]: https://github.com/nlink-jp/voice-studio-mcp/releases/tag/v0.3.0
[0.2.0]: https://github.com/nlink-jp/voice-studio-mcp/releases/tag/v0.2.0
[0.1.0]: https://github.com/nlink-jp/voice-studio-mcp/releases/tag/v0.1.0
