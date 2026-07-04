# voice-studio-mcp

> Local speech synthesis for AI-agent-driven radio-drama / audiobook
> production, exposed as a single-binary MCP server (AivisSpeech Engine
> backend).

## Why this exists

Turning a novel into a radio drama takes two kinds of work: the
*intellectual* part (script conversion, speaker attribution, performance
direction) and the *mechanical* part (speech synthesis, retakes,
mastering). AI agents such as Claude Code and Cowork handle the first
part well — but they have no voice. voice-studio-mcp gives them one:
a fully local MCP server that wraps
[AivisSpeech Engine](https://github.com/Aivis-Project/AivisSpeech-Engine)
(VOICEVOX-compatible API, Style-Bert-VITS2-family models) and turns an
agent-authored script into mastered audio. No cloud APIs, no credentials.

## Features

- **Speaker catalog with license metadata** — the engine API does not
  expose model usage terms, so a hand-maintained config section is joined
  in; unreviewed models are flagged `unverified` before you publish.
- **Pronunciation dictionaries** — register proper-noun readings per work
  so character names are never misread.
- **Batch synthesis with a content-hash cache** — edit three lines of a
  200-line script and only those three lines are re-synthesized.
- **Async jobs** — long scripts synthesize in the background; poll with
  `check_job`.
- **Mastering** — ffmpeg concat with per-line pauses, one-pass loudness
  normalization (-16 LUFS default), mp3 or m4b with scene chapters, and
  an auto-generated credits file.
- **Engine lifecycle management** — spawns the AivisSpeech Engine on
  startup and reaps it on shutdown (or attaches to one that is already
  running).

## Requirements

- macOS on Apple Silicon (v1 target)
- [AivisSpeech](https://aivis-project.com/) installed (bundles the engine)
- `ffmpeg` in PATH (only for the `master` tool): `brew install ffmpeg`

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
| `version` | Print the version |

## Tools

| Tool | Description |
|------|-------------|
| `list_speakers` | Installed voice models + styles + license metadata |
| `register_dictionary` | Register work-specific pronunciations (katakana readings, accent) |
| `synthesize_script` | Batch-synthesize a script JSONL → `wav/<id>.wav` (async; returns `job_id`) |
| `synthesize_line` | Synchronous single-line synthesis (retakes, voice auditioning via `style_id`) |
| `check_job` | Progress and per-line failures of a batch job |
| `master` | Concat + loudnorm + mp3/m4b (+ chapters) + credits file |

Tool results are compact JSON summaries (paths, counts, durations) —
audio bytes are never returned to the client.

### Script JSONL (canonical schema)

One utterance per line; the schema is the contract for agent-side skills:

```jsonl
{"id":1,"scene":1,"speaker":"narrator","text":"夜のとばりが街を包んでいた。","pause_after_ms":800}
{"id":2,"scene":1,"speaker":"美咲","text":"……本当に、行くの?","style":"悲しみ","intensity":1.4,"speed":0.9}
```

- `id` (required, unique, >0) — output is `wav/<id>.wav`; retakes overwrite
- `speaker` (required) — resolved via the casting table
- `style` / `intensity` (0–2) / `speed` (0.5–2) — performance direction
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
from any software license: check each model (AivisHub etc.) before
publishing audio, record the result in `casting.toml`
(`license_checked`) and `[[speaker_metadata]]`, and ship the generated
credits file with your production.

## Documentation

- `docs/en/voice-studio-mcp-rfp.md` — the original RFP (design decisions)
- `docs/ja/voice-studio-mcp-rfp.ja.md` — 日本語版 RFP

## License

MIT — see [LICENSE](LICENSE).
