# Changelog

All notable changes to this project are documented here. The format is
based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and
this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

### Changed

- Default mastering loudness target lowered from -16 LUFS to -18 LUFS
  (audiobook range) after listening feedback; configurable via
  `master.loudnorm_i`.
