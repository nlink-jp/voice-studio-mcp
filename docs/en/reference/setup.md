# voice-studio-mcp Setup Guide

From zero to your first radio-drama audio.

## 1. Prerequisites

| Requirement | Notes |
|-------------|-------|
| OS | macOS 13+ / Apple Silicon (v1 target) |
| AivisSpeech | Install the .dmg from https://aivis-project.com/ and **launch the GUI once** to complete first-run setup (default voice model download) |
| ffmpeg | `brew install ffmpeg` (used only by the `master` tool) |
| Go 1.23+ | Only when building from source |

> If the AivisSpeech GUI has never been launched, the engine may fail to
> start because no model has been fetched yet. `doctor` will tell you.

## 2. Build and diagnose

```sh
git clone https://github.com/nlink-jp/voice-studio-mcp.git
cd voice-studio-mcp
make build            # → dist/voice-studio-mcp (auto-codesigned on darwin)
dist/voice-studio-mcp doctor
```

Reading `doctor` output:

```
ok config: built-in defaults (no config.toml found)   ← works without config
ok engine command: /Applications/AivisSpeech.app/...  ← AivisSpeech detected
ok engine: not running at http://127.0.0.1:10101 (serve will spawn it)
ok ffmpeg: ffmpeg
ok workspace dir: /Users/you/.voice-studio
```

Any `NG` line includes its remedy (AivisSpeech missing, ffmpeg missing, …).

## 3. Configuration (optional)

Everything has defaults; you can start with no config. To customize:

```sh
mkdir -p ~/.config/voice-studio-mcp
cp config.example.toml ~/.config/voice-studio-mcp/config.toml
```

Commonly touched keys:

- `master.loudnorm_i` — perceived loudness of the final audio (default
  -18 LUFS; go to -19/-20 if it still feels loud)
- `synthesis.concurrency` — parallel synthesis (default 1 for CPU
  inference; worth trying 2 on M3-class machines)
- `[[speaker_metadata]]` — license records for installed models (§6)

Switch between configs with `--config <path>`.

## 4. Registering with an MCP client

### Claude Code

```sh
claude mcp add voice-studio -- /path/to/dist/voice-studio-mcp serve
```

Or in the project's `.mcp.json`:

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

### Claude Desktop / Cowork

Add the same entry under `mcpServers` in `claude_desktop_config.json`.

> The first connection may take tens of seconds to minutes before
> initialize answers (engine model load). Subsequent starts take seconds.

## 5. First production (manual walkthrough)

Skip this section when an agent drives the tools — it's the fastest way to
understand the moving parts, though.

```sh
# 1. Prepare the workspace inputs
mkdir -p ~/.voice-studio/demo/script
cat > ~/.voice-studio/demo/casting.toml <<'EOF'
[characters."Narrator"]
speaker_uuid = "<from list_speakers>"
style_id = 888753760
credit = "AivisSpeech:Anneli"
license_checked = true
EOF

cat > ~/.voice-studio/demo/script/demo.jsonl <<'EOF'
{"id":1,"speaker":"Narrator","text":"これは最初のテストです。","pause_after_ms":500}
{"id":2,"speaker":"Narrator","text":"うまく聞こえていますか。"}
EOF
```

From the MCP client (or as instructions to the agent):

1. `list_speakers` — copy speaker_uuid / style_id into casting.toml
2. `register_dictionary` — register readings for proper nouns, if any
3. `synthesize_script` `{workspace_id: "demo", script_path: "script/demo.jsonl"}`
4. `check_job` — poll until state is done
5. `master` `{workspace_id: "demo", script_path: "script/demo.jsonl", format: "mp3"}`
6. Play `~/.voice-studio/demo/master/demo.mp3`

## 6. Adding voice models and recording their terms

1. Add models from [AivisHub](https://hub.aivis-project.com/) via the
   AivisSpeech GUI (Settings → Manage voice models).
2. **Read the model page's terms** and confirm they allow your use.
3. Record the result in config:

```toml
[[speaker_metadata]]
speaker_uuid = "..."         # from list_speakers
name = "ModelName"
license = "ACML 1.0"
license_url = "https://hub.aivis-project.com/aivm-models/..."
credit = "AivisSpeech:ModelName"
commercial_use = true
```

4. When casting the model in a work, set `license_checked = true` in
   casting.toml — otherwise `master` flags it under `unverified_models`
   (intentional friction).

## 7. Troubleshooting

| Symptom | Remedy |
|---------|--------|
| initialize times out | First model load in progress. Raise `engine.startup_timeout_seconds`; launch the GUI once to finish model setup |
| `engine_unavailable: engine command not found` | AivisSpeech not installed, or non-standard path → set `engine.command` |
| `ffmpeg_not_found` | `brew install ffmpeg`, or set an absolute `master.ffmpeg_path` |
| `job_not_found` | Jobs die with the server. Re-run `synthesize_script` (the cache makes it differential) |
| `master_incomplete` | Synthesize the `missing_line_ids` from details, then retry |
| Final audio too loud / quiet | Tune `master.loudnorm_i` (default -18; lower = quieter). Inter-line balance is the script's `volume` |
| A proper noun is misread | Register the reading (katakana) + accent via `register_dictionary`, then redo the line with `synthesize_line force=true` |
| Conflict with the GUI's engine? | None. A running engine is attached to, never owned (ADR-0002) |

## 8. Uninstall

```sh
rm -rf ~/.voice-studio          # all workspaces (audio, caches)
rm -rf ~/.config/voice-studio-mcp
```

Words registered in the engine's user dictionary stay in AivisSpeech
(removable from the GUI's dictionary settings).
