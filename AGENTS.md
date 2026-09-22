# AGENTS.md — voice-studio-mcp

## Project summary

MCP stdio server that gives AI agents (Claude Code / Cowork) local
multi-speaker **Japanese** speech synthesis for narrated audio (radio drama,
audiobook, podcast, briefing, …). Japanese only — the engine's voice models
are Japanese, so other languages are not supported.
Wraps **AivisSpeech Engine** (VOICEVOX-compatible HTTP API on
`127.0.0.1:10101`, Style-Bert-VITS2-family models, CPU/ONNX inference).
Six tools: `list_speakers`, `register_dictionary`, `synthesize_script`
(async job + content-hash cache), `synthesize_line`, `check_job`,
`master` (ffmpeg concat + loudnorm + mp3/m4b + credits). Fully local;
no cloud, no credentials. Skeleton ported from `data-toolbox-mcp`.
Dependencies: `cobra`, `BurntSushi/toml` only (engine and ffmpeg are
runtime dependencies).

## Build & test

```sh
make build      # → dist/voice-studio-mcp (auto-codesign on darwin)
make test       # go test ./... (hermetic: no AivisSpeech / ffmpeg needed)
make test-e2e   # build + drive the binary over stdio against an
                # in-process mock engine + ffmpeg stub
VOICE_STUDIO_TEST_REAL_ENGINE=1 make test-e2e  # opt-in real-engine e2e
make package    # build-all + zip + notarize darwin (NOTARY_PROFILE)
make verify-release  # gate: .notarized marker + freshness (run before upload)
make package-skill  # bundle skills/ → dist/multi-actor-narration.skill (release asset)
```

Never `go build` directly — always `make build` (outputs to `dist/`).
v1 targets darwin/arm64 only (PLATFORMS in the Makefile).

## Structure

- `cmd/` — cobra commands: `serve` (default), `doctor`, `version` (also
  reachable as the `--version` flag, which the org homebrew formula tests);
  `tools_registry.go` wires Deps after the engine is up
- `internal/jsonrpc/`, `internal/transport/`, `internal/mcpserver/`,
  `internal/toolerr/`, `internal/logging/` — MCP skeleton (ported verbatim
  from data-toolbox-mcp)
- `internal/config/` — sectioned TOML, strict decode (unknown keys fail),
  `[[speaker_metadata]]` = hand-maintained model license registry
- `internal/workspace/` — one work = one workspace under the caller's `work_dir`;
  `ResolveInside` blocks path traversal
- `internal/engine/` — `client.go` (HTTP: /version /speakers /audio_query
  /synthesis /user_dict_word), `supervisor.go` (managed spawn + attach
  fallback + SIGTERM reap), `enginetest/` (httptest mock with fault
  injection — used by unit AND e2e tests)
- `internal/script/` — canonical script JSONL schema (`Line`) + casting
  table; full-pass validation (all errors collected, max 20 reported)
- `internal/synth/` — per-line pipeline: AudioQuery → override owned keys →
  Synthesis → `wav/<id>.wav`; SHA256 cache key; WAV header parser
- `internal/job/` — in-memory async jobs; shared semaphore
  (`synthesis.concurrency`, default 1); NOT persisted (by design)
- `internal/master/` — ffmpeg arg builders (pure functions) + Runner
  interface (fake in tests); silence dedupe, ffmetadata chapters, credits
- `e2e/` — `//go:build e2e` harness spawning the built binary

## Gotchas

- **stdout is sacred** — it carries JSON-RPC; all logs go to stderr/file
  via `log/slog`. The spawned engine's stdout/stderr are piped to slog
  debug, never to our stdout.
- **AudioQuery is `map[string]any` on purpose** — AivisSpeech adds fields
  beyond VOICEVOX (`tempoDynamicsScale` etc.); we override only
  `speedScale`/`intonationScale`/`volumeScale`/`prePhonemeLength`/
  `postPhonemeLength`/`outputSamplingRate`/`outputStereo` and pass the
  rest through. Do not turn it into a struct.
- **Two volume knobs** — per-line `volume` (volumeScale, in the cache key)
  is for relative balance between lines; the final perceived loudness of
  master output is governed by `master.loudnorm_i` (-18 LUFS default).
- **Sampling rate is forced** to `synthesis.output_sampling_rate` on every
  line — mixed rates would break the concat demuxer in `master`.
- **`pause_after_ms` is not in the cache key** — pauses are applied at
  mastering time; changing them must not trigger re-synthesis.
- **`/user_dict_word` takes query parameters, not a JSON body**
  (VOICEVOX API convention); pronunciation must be katakana.
- **Jobs die with the process** — `check_job` on an unknown id returns
  `job_not_found` with recovery guidance (re-run `synthesize_script`;
  the cache makes it cheap). Do not add persistence casually.
- **Managed mode attaches before spawning** — if something answers at
  `engine.url`, the supervisor never takes ownership (protects the
  GUI-managed engine and survives a prior SIGKILL orphan).
- **Engine version is part of the cache key** — an engine update
  invalidates caches; that is intentional (output may change).
- **The workspace base is verified by real path, because the path is handed to
  ffmpeg, which resolves it outside any root** — `os.Root` contains operations
  *within* a root but resolves the root path itself normally, so a link planted
  at `<work_dir>/<id>` would anchor every read and write on its target while
  reporting success. `makeWorkspaceDir` creates the directory through an
  `os.Root` on `work_dir` **and** compares `filepath.EvalSymlinks` of the base
  against `<real work_dir>/<id>`; a mismatch is refused. The comparison is the
  load-bearing half — the root-based mkdir alone cannot help a path that later
  leaves the process.
- **The workspace judges every read.** `Workspace.ReadFile`, `Stat` and
  `VerifyRegular` call `judge` first — pathguard's Local policy with this
  server's own directories, the `floor` that `workspace.NewManager(check,
  floor)` requires (`workdir.Resolver.LocalPath`; nil refuses every workspace,
  and a Workspace literal without one refuses every read). A workspace passes
  `CheckBeneath` but can still contain a `.env`, this server's config directory
  or the file a link in `~/.ssh` leads to; a first fix judged only
  `script_path` and `casting_path`, and its review found `master` reading
  `wav/<id>.wav` through a planted link (ADR-0014, amendment v0.6.1). Do not
  add a read that bypasses these three, and do not judge at call sites instead.
  `TestEveryReadIsJudgedBeforeItLooks`, `TestExistenceIsNotRevealed` and
  `TestMasterDoesNotReadAFloorFileThroughAWav` pin it; six mutations are
  caught by assertion. Writes are not judged (a planted directory link can
  steer one inside the workspace).
- **The mastered file is cleared through the root before the spawn** — ffmpeg
  opens the output path itself and would follow a symlink planted at
  `master/<name>.<format>`, overwriting the link's target. `Build` calls
  `ws.RemoveAll(outRel)` first (a root-based remove unlinks the link, never
  what it points at) so ffmpeg always creates the file fresh.
- **Voice-model licenses are data, not code** — `[[speaker_metadata]]`
  in config and `license_checked` in casting.toml; `master` warns on
  unverified models and generates the credits file.
- **The path judgement is nlink-jp/pathguard's, not this repository's.**
  `internal/workdir` only takes `_meta` from the context and carries
  pathguard's errors onto `toolerr` (ADR-0014). Do not add a location list or a
  name comparison here; a fix to the judgement is a pathguard release and a
  dependency bump.
- **`workdir.Resolver` is built in exactly one place** — `workDirResolver()`
  in `cmd/tools_registry.go`, reached only through `newToolDeps`, with
  `workdir.NewResolver(serverOwnedDirs()...)`. Those are this server's own
  directories (organization ADR-021 §4), which today is
  `configDir()` = `~/.config/voice-studio-mcp`; `resolveConfig`
  searches that same expression so the denial cannot drift from the location.
  There is no state directory — every byte produced goes under the caller's
  `work_dir`. Add one and it belongs in `serverOwnedDirs()`. A zero
  `workdir.Resolver{}` refuses every call, and so does an empty server
  directory — tests build one with `workdir.NewResolver(t.TempDir())`.
- **The workspace directory is judged too.** `workspace.NewManager(check)`
  takes `workdir.Resolver.CheckBeneath`, and `EnsureUnder` judges
  `<work_dir>/<workspace_id>` before making it — `work_dir=~/.config` with
  `workspace_id=gh` is `~/.config/gh`. `newToolDeps` wires both; a Manager
  without a check refuses every workspace.

## ADR cheat sheet

- **ADR-0001**: AivisSpeech Engine only in v1 (SBV2 quality, LGPL, official macOS)
- **ADR-0002**: managed engine child process, attach-first
- **ADR-0003**: AudioQuery = map passthrough; only sampling rate forced
- **ADR-0004**: content-hash cache; pauses excluded, engine version included
- **ADR-0005**: jobs in-memory only; recovery = cache-backed re-run
- **ADR-0006**: concat demuxer + 1-pass loudnorm (-18 LUFS); two volume knobs
- **ADR-0007**: voice-model terms as human-recorded data
- **ADR-0008**: license collection automated from AIVM manifests
  (`/aivm_models` → status "declared"); verification stays human
  ("verified" via `[[speaker_metadata]]`; `licenses` subcommand)
- **ADR-0009**: the Claude Code skill is bundled in `skills/` (installed via
  `make install-skill`), not a separate repo; `skills/skills_test.go` pins
  skill/server coherence — update the skill when tools, error codes, or the
  schema change, or the build breaks
- **ADR-0010**: "the server works in the workplace the agent prepared" —
  optional stateless `work_dir` param on all workspace tools;
  ALL workspace I/O goes through os.Root (Go 1.25 floor); ffmpeg inputs
  re-verified via Lstat pre-spawn; `get_usage` + initialize instructions
  for skill-less clients (`internal/tools/usage.md`, coherence-tested)
- **ADR-0011**: the bundled skill is the single **multi-actor-narration**
  skill — five formats over one pipeline (talk-podcast / panel-discussion /
  news-briefing / lesson-narration + generalized **audio-drama**, the former
  radio-drama). Built by `skills/build.sh` + `make package-skill` →
  `dist/multi-actor-narration.skill`, shipped as a separate release asset;
  the coherence test aggregates the multi-file skill (SKILL.md + `_shared/`
  + `<format>/FORMAT.md`). Breaking: `/radio-drama` → `/multi-actor-narration`
- **ADR-0012**: unsigned AivisSpeech bundle (mismatched nested signatures)
  is killed by macOS on spawn; `doctor --fix` strips quarantine + ad-hoc
  re-signs the engine subtree inside-out (`internal/enginefix`, Runner-faked
  tests). Bare `doctor` detects read-only; `--fix` mutates. NOT a
  fork/notarize/reimplement — self-repair on the user's machine

Full texts: [`docs/en/adr/`](docs/en/adr/) / [`docs/ja/adr/`](docs/ja/adr/).

## Design references

- [`docs/en/reference/agent-workflow.md`](docs/en/reference/agent-workflow.md) /
  [`docs/ja/reference/agent-workflow.ja.md`](docs/ja/reference/agent-workflow.ja.md)
  — the 10-step production procedure agents follow (precursor of the
  multi-actor-narration skill's audio-drama format); sample material in
  `samples/`.
- [`docs/en/reference/architecture.md`](docs/en/reference/architecture.md) /
  [`docs/ja/reference/architecture.ja.md`](docs/ja/reference/architecture.ja.md)
  — module map, data flow, error model, testing strategy.
- [`docs/en/reference/setup.md`](docs/en/reference/setup.md) /
  [`docs/ja/reference/setup.ja.md`](docs/ja/reference/setup.ja.md) — setup guide.
- [`docs/ja/voice-studio-mcp-rfp.ja.md`](docs/ja/voice-studio-mcp-rfp.ja.md) /
  [`docs/en/voice-studio-mcp-rfp.md`](docs/en/voice-studio-mcp-rfp.md) —
  approved RFP; canonical source for scope decisions (v1: AivisSpeech only,
  macOS only, BGM/SE out of scope, skill = separate project).
