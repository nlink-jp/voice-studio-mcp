# voice-studio-mcp — how to use this server

This server turns a script (台本) into radio-drama / audiobook audio using a
local AivisSpeech Engine. You (the agent) do the creative work — script
conversion, casting, direction; the server does synthesis, caching, retakes,
and mastering. Tools return compact JSON (paths, counts, durations) — never
audio bytes; the produced files are played by the human user on this host.

## Workspace model (read this first)

All production state lives in a workspace: `<workspace_root>/<workspace_id>/`

```
script/          script JSONL files   (you write these)
casting.toml     character → voice    (you write this)
dict/words.json  dictionary record    (server-written)
wav/<id>.wav     per-line audio       (server-written)
cache/index.json synthesis cache      (server-written)
master/          final audio + credits (server-written)
```

- `workspace_id`: `[a-zA-Z0-9_-]{1,64}`, one per work (novel / episode).
- `workspace_root` (optional on every workspace tool): an **absolute path to
  a directory you prepared** — create it with your own file tools wherever
  you are allowed to write (e.g. inside the project directory), then pass
  the same value on every call. Omit it to use the server's default root
  (`~/.voice-studio`), which requires the server and you to share an
  unrestricted filesystem view.
- The server never reads or writes outside the workspace (kernel-enforced;
  symlinks inside the workspace that point outside fail with
  `path_not_allowed`).

## Production flow

1. `list_speakers` — voice catalog. `license.status`: `verified`
   (human-reviewed) / `declared` (terms embedded in the model, unreviewed) /
   `unverified`. Only verified models belong in published audio.
2. Write `casting.toml` and your script JSONL into the workspace
   (get user approval for the casting).
3. `register_dictionary` — proper-noun readings (katakana + accent_type).
4. `synthesize_script` — validates everything first, then runs async;
   returns `job_id`. Poll `check_job` until state leaves "running".
5. `synthesize_line` — single-line retakes (`force: true` after dictionary
   or direction changes; dictionary updates do NOT invalidate the cache).
6. `master` — concat + loudness normalization + mp3/m4b (+ scene chapters)
   + credits file. Warn the user if `unverified_models` is non-empty.

## Script JSONL schema (one utterance per line)

```jsonl
{"id":10,"scene":1,"speaker":"ナレーター","text":"...","speed":0.95,"pause_after_ms":800}
{"id":20,"scene":1,"speaker":"美咲","text":"...","style":"悲しみ","intensity":1.4,"volume":0.8}
```

Fields: `id` (required, >0, unique — output is wav/<id>.wav; never renumber),
`scene` (m4b chapters), `speaker` (required; casting key), `text` (required),
`style` (named style from casting), `intensity` 0–2, `speed` 0.5–2,
`volume` 0–2 (relative balance only; overall loudness is master's loudnorm),
`pause_after_ms` (mastering-time silence; does not invalidate the cache).

## casting.toml

```toml
[characters."ナレーター"]
speaker_uuid = "..."       # from list_speakers
style_id = 888753760       # default style id
credit = "AivisSpeech:モデル名"
license_checked = true     # only after a human confirmed the terms

[characters."美咲".styles]  # script "style" name → style id
"悲しみ" = 888753761
```

## Error recovery

| code | action |
|------|--------|
| invalid_script | fix every entry in details.errors, retry |
| casting_unresolved | add unmapped speakers/styles to casting.toml |
| engine_unavailable | the user must check the AivisSpeech installation |
| engine_request_failed | inspect the line text; transient → retry |
| job_not_found | server restarted; re-run synthesize_script (cache = differential) |
| master_incomplete | synthesize details.missing_line_ids first |
| ffmpeg_not_found | the user must install ffmpeg |
| path_not_allowed | use workspace-relative paths / a valid absolute workspace_root; symlinks out of the workspace are rejected |
| invalid_workspace_id | match [a-zA-Z0-9_-]{1,64} |
