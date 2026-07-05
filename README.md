# voice-studio-mcp

> Local, multi-speaker **Japanese** speech synthesis for AI agents — turn a
> multi-voice script into mastered audio (radio drama, audiobook, podcast,
> panel, briefing, …). Single-binary MCP server, AivisSpeech Engine backend.
>
> **Language: Japanese only.** The AivisSpeech Engine synthesizes Japanese;
> other languages are not supported.

## Why this exists

Producing scripted, multi-voice spoken audio takes two kinds of work: the
*intellectual* part (script conversion, speaker attribution, performance
direction) and the *mechanical* part (speech synthesis, retakes,
mastering). AI agents such as Claude Code and Cowork handle the first
part well — but they have no voice. voice-studio-mcp gives them one:
a fully local MCP server that wraps
[AivisSpeech Engine](https://github.com/Aivis-Project/AivisSpeech-Engine)
(VOICEVOX-compatible API, Style-Bert-VITS2-family models) and turns an
agent-authored script into mastered audio. Radio drama and audiobooks were
its first use cases; the bundled multi-actor-narration skill adds podcasts,
panels, news briefings, and lessons. No cloud APIs, no credentials.

The engine's voice models are **Japanese**, so voice-studio-mcp produces
Japanese audio only — other languages are out of scope.

## Features

- **Speaker catalog with license metadata** — author-declared license text
  is collected automatically from each model's AIVM manifest (`declared`),
  and human-reviewed config entries mark models `verified`; the `licenses`
  subcommand turns declarations into reviewable config skeletons.
- **Pronunciation dictionaries** — register proper-noun readings per work
  so character names are never misread.
- **Batch synthesis with a content-hash cache** — edit three lines of a
  200-line script and only those three lines are re-synthesized.
- **Async jobs** — long scripts synthesize in the background; poll with
  `check_job`.
- **Mastering** — ffmpeg concat with per-line pauses, one-pass loudness
  normalization (-18 LUFS default; configurable), mp3 or m4b with scene
  chapters, and an auto-generated credits file.
- **Engine lifecycle management** — spawns the AivisSpeech Engine on
  startup and reaps it on shutdown (or attaches to one that is already
  running).

## Requirements

- **Japanese-language content only** — the engine's voice models are Japanese; non-Japanese text is not supported
- macOS on Apple Silicon (v1 target)
- [AivisSpeech](https://aivis-project.com/) installed (bundles the engine)
- `ffmpeg` in PATH (only for the `master` tool): `brew install ffmpeg`
- Go 1.25+ (only when building from source)

## Quick start

```sh
make build                     # → dist/voice-studio-mcp
dist/voice-studio-mcp doctor   # verify engine / ffmpeg / config
```

Register with an MCP client (e.g. Claude Code):

```json
{
  "mcpServers": {
    "voice-studio": {
      "command": "/path/to/dist/voice-studio-mcp",
      "args": ["serve"]
    }
  }
}
```

Configuration lives at `~/.config/voice-studio-mcp/config.toml`
(see [config.example.toml](config.example.toml)); everything has
sensible defaults.

## Subcommands

| Command | Description |
|---------|-------------|
| `serve` | Start the MCP stdio server (default when no subcommand is given) |
| `doctor` | Diagnose the environment (config, engine, ffmpeg, workspace dir) |
| `licenses` | Collect voice-model license declarations for review (`--full <uuid>` full text, `--toml` config skeleton) |
| `version` | Print the version |

## Tools

| Tool | Description |
|------|-------------|
| `get_usage` | This server's operating manual (workspace model, schema, recovery table) — call it first from clients without the bundled skill |
| `list_speakers` | Installed voice models + styles + license metadata |
| `register_dictionary` | Register work-specific pronunciations (katakana readings, accent) |
| `synthesize_script` | Batch-synthesize a script JSONL → `wav/<id>.wav` (async; returns `job_id`) |
| `synthesize_line` | Synchronous single-line synthesis (retakes, voice auditioning via `style_id`) |
| `check_job` | Progress and per-line failures of a batch job |
| `master` | Concat + loudnorm + mp3/m4b (+ chapters) + credits file |

Tool results are compact JSON summaries (paths, counts, durations) —
audio bytes are never returned to the client.

## Claude Code skill (bundled)

The repository bundles the **multi-actor-narration** skill — the operational
form of the agent workflow (input → casting → dictionary → synthesis →
retakes → mastering, with mandatory human checkpoints). One skill routes to
five formats over the same voice-studio pipeline:

- **talk-podcast / panel-discussion / news-briefing / lesson-narration** —
  document or theme → multi-speaker explainer audio
- **audio-drama** — novel / script → narrated audio drama / audiobook
  (the generalized successor of the former radio-drama skill)

```sh
make install-skill      # → ~/.claude/skills/multi-actor-narration
make package-skill      # → dist/multi-actor-narration.skill (portable package)
```

Then ask Claude Code / Cowork:
*"/multi-actor-narration samples/manuscript.ja.md"* (routes to audio-drama),
or point it at a PDF/theme for an explainer format. The skill source ships
here (not in a separate repo) so it always matches this server's schema and
tools — `skills/skills_test.go` enforces that across the multi-file skill
(ADR-0009). Releases attach `multi-actor-narration.skill` as a **separate
asset** next to the MCP binary, so users without the repo can import the
skill directly (ADR-0011).

### Script JSONL (canonical schema)

One utterance per line; the schema is the contract for agent-side skills:

```jsonl
{"id":1,"scene":1,"speaker":"narrator","text":"夜のとばりが街を包んでいた。","pause_after_ms":800}
{"id":2,"scene":1,"speaker":"美咲","text":"……本当に、行くの?","style":"悲しみ","intensity":1.4,"speed":0.9}
```

- `id` (required, unique, >0) — output is `wav/<id>.wav`; retakes overwrite
- `speaker` (required) — resolved via the casting table
- `style` / `intensity` (0–2) / `speed` (0.5–2) / `volume` (0–2) —
  performance direction (`volume` sets relative balance; overall loudness
  is governed by the mastering `loudnorm_i` target)
- `pause_after_ms` — silence inserted at mastering time (does not
  invalidate the synthesis cache)
- `scene` — m4b chapter boundaries

### Casting table (`casting.toml` in the workspace root)

```toml
[characters."narrator"]
speaker_uuid = "..."          # from list_speakers
style_id = 888753760          # default style
credit = "AivisSpeech:Anneli"
license_checked = true        # human confirmed the model's terms

[characters."美咲"]
speaker_uuid = "..."
style_id = 933744512

[characters."美咲".styles]    # script line "style" → style id
"悲しみ" = 933744513
```

### Typical agent flow

1. `list_speakers` → cast characters, write `casting.toml`
2. `register_dictionary` → proper-noun readings
3. Write the script JSONL into `<workspace>/script/`
4. `synthesize_script` → poll `check_job`
5. `synthesize_line` for retakes
6. `master` → mp3/m4b + credits

## Workspace layout

Every workspace tool accepts an optional `workspace_root` — the absolute
path of a directory the agent prepared in its own writable area (e.g.
inside the project directory). This is how sandboxed MCP clients whose
writes are restricted to the project tree use the server; the server never
follows symlinks out of a workspace (kernel-enforced via os.Root,
ADR-0010). Omitted, the default root applies:

```
~/.voice-studio/<workspace_id>/
├── script/          script JSONL files (agent-authored)
├── casting.toml     character → voice mapping
├── dict/words.json  registered dictionary record
├── wav/<id>.wav     per-line synthesized audio
├── cache/index.json synthesis cache index
└── master/          mastered outputs + credits + tmp
```

## Testing

```sh
make test        # unit tests (no AivisSpeech / ffmpeg required)
make test-e2e    # build + drive the binary over stdio against a mock engine
VOICE_STUDIO_TEST_REAL_ENGINE=1 make test-e2e   # opt-in: real engine
```

## License notes

The server code is MIT. **Voice models have their own terms**, separate
from any software license. The review workflow (ADR-0008):

1. `voice-studio-mcp licenses` — see what each installed model declares
2. `voice-studio-mcp licenses --full <speaker_uuid>` — read the full terms
3. `voice-studio-mcp licenses --toml` — generate `[[speaker_metadata]]`
   skeletons; fill `license_url` / `commercial_use` and delete the REVIEW
   note once accepted
4. Set `license_checked = true` in `casting.toml` per work, and ship the
   generated credits file with your production

`list_speakers` reports `verified` (human-reviewed) / `declared` (manifest
text present, unreviewed) / `unverified`; only `verified` models belong in
published audio.

The registry is **user data**: it lives in your user config, reflects the
models installed on *your* machine and *your* review, and must not be
committed to shared repositories. Since speaker_uuid is model-intrinsic
(not machine-local), you may sync it across your own machines via personal
dotfiles.

## Documentation

- [`docs/en/reference/agent-workflow.md`](docs/en/reference/agent-workflow.md) — the agent workflow guide (hand this to Claude Code / Cowork; sample material in [`samples/`](samples/))
- [`docs/en/reference/setup.md`](docs/en/reference/setup.md) — setup guide (install → first production → troubleshooting)
- [`docs/en/reference/architecture.md`](docs/en/reference/architecture.md) — architecture overview and decision index
- [`docs/en/adr/`](docs/en/adr/) — ten ADRs recording the *why* behind non-obvious designs
- [`docs/en/voice-studio-mcp-rfp.md`](docs/en/voice-studio-mcp-rfp.md) — the original RFP
- 日本語版: [`docs/ja/`](docs/ja/) (セットアップ / アーキテクチャ / ADR / RFP)

## License

MIT — see [LICENSE](LICENSE).
