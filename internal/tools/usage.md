# voice-studio-mcp — how to use this server

This server turns a multi-speaker script (台本) into narrated Japanese audio
(radio drama, audiobook, podcast, briefing, …) using a local AivisSpeech
Engine. It synthesizes **Japanese only** — other languages are not supported.
You (the agent) do the creative work — script conversion, casting, direction;
the server does synthesis, caching, retakes, and mastering. Tools return compact JSON (paths, counts, durations) — never
audio bytes; the produced files are played by the human user on this host.

## Workspace model (read this first)

All production state lives in a workspace: `<work_dir>/<workspace_id>/`

```
script/          script JSONL files   (you write these)
casting.toml     character → voice    (you write this)
dict/words.json  dictionary record    (server-written)
wav/<id>.wav     per-line audio       (server-written)
cache/index.json synthesis cache      (server-written)
master/          final audio + credits (server-written)
```

- `workspace_id`: `[a-zA-Z0-9_-]{1,64}`, one per work (novel / episode).
- `work_dir` (**required** on every workspace tool): the **absolute path of a
  directory you can read back** — your session or working directory. Every
  result is a path under it, so a directory you cannot open leaves you holding
  a path to nothing. There is no default any more: it must already exist, and
  nothing here expands `~` or resolves a relative path. Your runtime may supply
  it by setting `_meta["jp.nlink/work_dir"]` on the call; the argument wins.
- Pass the **same `work_dir` to the other media servers** when they share a
  pipeline: image-forge renders the page images and video-studio muxes them
  with the audio produced here, all under one directory.
- The server never reads or writes outside the workspace (kernel-enforced;
  symlinks inside the workspace that point outside fail with
  `path_not_allowed`). The workspace directory itself must be a real directory:
  if `<work_dir>/<workspace_id>` is a symlink, the call is refused with
  `path_not_allowed` rather than run against the link's target.
- A `script_path` or `casting_path` that is a `.env`, lies in this server's
  config directory, or is where a link inside a credential directory points is
  refused with `path_not_allowed` before it is read, whether or not it exists.

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
| path_not_allowed | use workspace-relative paths; symlinks out of the workspace are rejected, and so is a script or casting table that is a `.env`, lies in this server's config directory, or is where a link inside a credential directory points (whether or not it exists) |
| work_dir_required | no `work_dir` argument and no `_meta` hint — pass the absolute path of a directory you can read back |
| work_dir_invalid | not absolute, started with `~`, or contained `..` |
| work_dir_not_found | not there, or not a directory — it is yours, so this is a typo; the server does not create it |
| work_dir_not_writable | the server cannot write there |
| work_dir_denied | a system location, your home directory itself, a credential or agent-control location (or where a link directly inside one points), this server's own config directory (`~/.config/voice-studio-mcp`) — under any spelling — or the home directory cannot be determined, for `work_dir` and for the workspace directory `<work_dir>/<workspace_id>` it would use; `details.reason` says which: `system_dir`, `home_dir`, `sensitive_path`, `server_dir`, `home_unknown`, `unconfigured`, `unresolvable_path` |
| invalid_workspace_id | match [a-zA-Z0-9_-]{1,64} |
