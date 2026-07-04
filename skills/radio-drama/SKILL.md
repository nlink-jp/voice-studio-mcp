---
name: radio-drama
description: Produce radio-drama / audiobook audio from a novel or story manuscript using the voice-studio MCP server (AivisSpeech Engine). Use when the user asks to turn a manuscript, novel, episode, or script into narrated audio, a radio drama, an audiobook, or 朗読/ラジオドラマ/オーディオブック音声. Requires the voice-studio MCP server to be registered.
argument-hint: "[manuscript-path] [workspace-id]"
---

# Radio Drama Production Skill

You are producing radio-drama / audiobook audio from a manuscript, using the
**voice-studio** MCP server tools: `list_speakers`, `register_dictionary`,
`synthesize_script`, `synthesize_line`, `check_job`, `master`.

Input: **$ARGUMENTS** (manuscript path and optional workspace_id; ask the
user if missing). The workspace root is `~/.voice-studio/<workspace_id>/`
unless the server is configured otherwise — create workspace files there.

This skill ships with voice-studio-mcp and matches its script JSONL schema;
if a tool rejects your script, trust the tool's error details over memory.

## Procedure (10 steps)

### Step 1 — Environment check

Call `list_speakers`. On `engine_unavailable`, stop and tell the user how to
fix their AivisSpeech setup. Note each speaker's styles and `license.status`.

### Step 2 — Analyze the manuscript

- **Scene segmentation**: assign `scene` numbers at scene changes (they
  become m4b chapters).
- **Speaker attribution**: identify who speaks each quoted line. **If the
  project has character reference sheets (e.g. novel-works-style
  設定資料/キャラクター早見表 with 一人称・口調・呼称 tables), read them
  FIRST** — they resolve attribution and voice direction authoritatively.
  Ask the user about attributions you are not confident in.
- **Narration**: descriptive prose becomes the narrator's lines; split long
  passages into listenable chunks (~100–150 chars). Give short internal
  exclamations (――は？) to the character; keep long inner monologue with
  the narrator.

### Step 3 — Casting (human checkpoint — MANDATORY)

Draft a casting plan from `list_speakers` and **present it to the user for
approval before writing casting.toml**:

- Narrator: calm, steady voice; characters: match gender / age / personality.
  Voice model names can mislead — check the model description via
  `voice-studio-mcp licenses --full <uuid>` when unsure, or audition (Step 6).
- If `license.status` is not `verified`, say so; for commercial productions
  exclude models with `commercial_use = false`.

After approval write `casting.toml` in the workspace root:

```toml
[characters."ナレーター"]
speaker_uuid = "..."       # from list_speakers
style_id = 888753760       # default style
credit = "AivisSpeech:モデル名"
license_checked = true     # only after the user confirmed the terms

[characters."美咲".styles]  # script "style" name → style id
"悲しみ" = 888753761
```

### Step 4 — Pronunciation dictionary

Extract words likely to be misread: character names, place names, coined
words. Confirm readings from reference sheets or the user, then call
`register_dictionary` (readings in **katakana**, accent_type = mora index
where pitch falls, 0 = flat).

### Step 5 — Write the script JSONL

One line = one utterance, under `<workspace>/script/`:

```jsonl
{"id":10,"scene":1,"speaker":"ナレーター","text":"...","speed":0.95,"pause_after_ms":800}
{"id":20,"scene":1,"speaker":"美咲","text":"...","style":"悲しみ","intensity":1.4}
```

Direction guidelines:

| Parameter | Guide |
|-----------|-------|
| `intensity` (0–2) | calm 1.0 / restrained 0.7–0.9 / agitated 1.3–1.6 / outburst 1.7–2.0 |
| `speed` (0.5–2) | narration 0.9–1.0 / dialogue 1.0 / urgency 1.05–1.2 / reminiscence 0.85–0.95 |
| `volume` (0–2) | relative balance only (whisper 0.6–0.8, shout 1.2–1.5); overall loudness is master's loudnorm |
| `pause_after_ms` | beat 300–600 / paragraph 700–1000 / scene change 1200–2000 |

Number ids in steps of 10 (insertion room). **Never renumber existing ids**
— they key the synthesis cache and retakes.

### Step 6 — Voice audition (optional)

`synthesize_line` with an explicit `style_id` renders the same text across
candidate voices; let the user listen and choose.

### Step 7 — Batch synthesis

`synthesize_script` → on `invalid_script` / `casting_unresolved` fix
**everything in details at once** and retry. Poll `check_job` until state
leaves `running` (5–10s short works, 30s long ones). Failed lines: fix per
the dispatch table, re-run — the cache re-synthesizes only those lines.

### Step 8 — Listening review (human checkpoint — MANDATORY)

Have the user listen (per-line `wav/<id>.wav` or an interim `master`).
Apply feedback:

- Misreading → extend the dictionary, redo with `synthesize_line force=true`
- Wrong delivery → adjust `intensity`/`speed`/`style`, re-run (differential)
- Pacing → adjust `pause_after_ms` (**no re-synthesis; just re-master**)

### Step 9 — Mastering

`master` with `format: "mp3"` (distribution) or `"m4b"` (audiobook; scenes
become chapters). If `unverified_models` is non-empty, **warn the user
before publishing**.

### Step 10 — Delivery

Report: master path + duration, credits (contents of `credits_path`, must
accompany the release), and any license warnings.

## Error dispatch

| code | action |
|------|--------|
| `invalid_script` | fix every details.errors entry, retry |
| `casting_unresolved` | add unmapped speakers/styles to casting.toml |
| `engine_unavailable` | user checks AivisSpeech install / engine.command |
| `engine_request_failed` | inspect the line text; transient → retry |
| `job_not_found` | server restarted; re-run synthesize_script (cache = differential) |
| `master_incomplete` | synthesize details.missing_line_ids first |
| `ffmpeg_not_found` | user: `brew install ffmpeg` |
| `path_not_allowed` | use workspace-relative paths |

## Never do

- Renumber existing script ids (cache wipe, retake confusion)
- Set `license_checked = true` without the user's confirmation
- Fetch audio bytes (tools return paths; playback is the user's job)
- Adjust overall loudness via `volume` (that is `master.loudnorm_i`)
- Skip the two human checkpoints (casting approval, listening review)
