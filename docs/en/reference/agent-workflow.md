# Agent Workflow Guide (Claude Code / Cowork)

The standard procedure an agent follows to turn a novel/story manuscript
into radio-drama or audiobook audio. **This document is written to be read
by the agent directly** — the user only pastes the request template in §1.
The future radio-drama skill will treat this guide as its canonical source.

## 1. For users: request template

With the `voice-studio` MCP server registered, ask Claude Code / Cowork:

> Turn the manuscript in `samples/manuscript.ja.md` into radio-drama audio.
> Follow the procedure in voice-studio-mcp's
> `docs/en/reference/agent-workflow.md`. Use workspace_id `my-drama`.
> Check the casting with me before finalizing it.

## 2. For agents: the 10-step procedure

### Step 1 — Environment check

Call `list_speakers`. On `engine_unavailable`, ask the user about their
AivisSpeech installation (see [setup.md](setup.md)). Note the speakers and
each `license.status`.

### Step 2 — Analyze the manuscript and convert to a screenplay

- **Scene segmentation**: assign `scene` numbers at scene changes (they
  become m4b chapter boundaries)
- **Dialogue extraction and speaker attribution**: identify who speaks each
  quoted line from context. **Ask the user about attributions you are not
  confident in** — a wrong attribution found after production is expensive
- **Narration**: treat descriptive prose as the narrator's lines; split
  long passages into listenable chunks (roughly 100–150 chars per line)

### Step 3 — Casting (human checkpoint ①)

Draft a casting plan from `list_speakers` and **present it to the user for
approval**:

- Narrator: a calm, steady style
- Characters: voices matching gender / age / personality
- If a model has `license.status: "unverified"`, say so explicitly and ask
  the user to review its terms (and suggest recording the result in the
  `[[speaker_metadata]]` config)

After approval, write `casting.toml` in the workspace root (format: see the
README). Set `license_checked = true` only for confirmed characters.

### Step 4 — Pronunciation dictionary

Extract words likely to be misread: personal names, place names, coined
words, unusual readings. Decide the reading (**katakana**) and accent
position and register them with `register_dictionary`. Ask the user when
unsure about a reading.

### Step 5 — Write the script JSONL

Write the script under the workspace's `script/`. One line = one utterance:

```jsonl
{"id":1,"scene":1,"speaker":"Narrator","text":"...","speed":0.95,"pause_after_ms":800}
{"id":2,"scene":1,"speaker":"美咲","text":"...","style":"悲しみ","intensity":1.4}
```

Direction guidelines:

| Parameter | Guide |
|-----------|-------|
| `intensity` (emotion 0–2) | calm 1.0 / restrained 0.7–0.9 / agitated 1.3–1.6 / outburst 1.7–2.0 |
| `speed` (0.5–2) | narration 0.9–1.0 / dialogue 1.0 / urgency 1.05–1.2 / reminiscence 0.85–0.95 |
| `volume` (relative balance 0–2) | normal 1.0 / whisper 0.6–0.8 / shout 1.2–1.5 (overall loudness is normalized by master) |
| `pause_after_ms` | conversational beat 300–600 / paragraph break 700–1000 / scene change 1200–2000 |

Number `id` sequentially from 1. **Never renumber existing ids when
inserting lines later** (cache and retake stability); use gaps (e.g. steps
of 10) or append.

### Step 6 — Voice audition (optional)

When unsure about casting, call `synthesize_line` with an explicit
`style_id` on the same text across candidate voices and let the user
compare.

### Step 7 — Batch synthesis

Call `synthesize_script`. On `invalid_script` / `casting_unresolved`, fix
**everything listed in details at once** and retry. On success, poll
`check_job` until state leaves `running` (5–10s intervals for short works,
30s for long ones).

- `failed > 0`: handle each failure code (table below), fix, and re-run
  `synthesize_script` — the cache re-synthesizes only the failed lines
- `job_not_found`: the server restarted; simply re-run

### Step 8 — Listening review (human checkpoint ②)

Have the user listen to line WAVs (`wav/<id>.wav`) or an interim `master`
output, then apply feedback:

- Misreading → add to the dictionary (Step 4), redo the line with
  `synthesize_line force=true`
- Wrong delivery → adjust the line's `intensity`/`speed`/`style` in the
  script and re-run (only changed lines synthesize)
- Bad pacing → adjust `pause_after_ms` (**no re-synthesis** — just re-run
  master)

### Step 9 — Mastering

Call `master` — `format: "mp3"` for distribution, `format: "m4b"` for
audiobooks (scenes become chapters). If `unverified_models` is non-empty,
**warn the user before publishing**.

### Step 10 — Delivery

Report to the user:

- The master file path and duration
- The credits (contents of `credits_path`) and that they must accompany
  the release
- Any unverified-license warnings

## 3. Error dispatch table

| code | Agent action |
|------|--------------|
| `invalid_script` | Fix every entry in details.errors, retry |
| `casting_unresolved` | Add details' unmapped_speakers/styles to casting.toml |
| `engine_unavailable` | Ask the user to check AivisSpeech ([setup.md](setup.md) §7) |
| `engine_request_failed` | Inspect the line text (empty, control chars); rare transient failures: retry |
| `job_not_found` | Re-run `synthesize_script` (cache makes it differential) |
| `master_incomplete` | Synthesize details.missing_line_ids first, retry |
| `ffmpeg_not_found` | Tell the user: `brew install ffmpeg` |
| `path_not_allowed` | Use workspace-relative paths |

## 4. Never do

- Renumber existing script `id`s (cache wipe + retake confusion)
- Set `license_checked = true` without human confirmation (terms review is
  a human responsibility)
- Try to fetch audio bytes (tools return paths only; playback happens in
  the user's environment)
- Adjust overall loudness with `volume` — that is `master.loudnorm_i`'s job
  ([ADR-0006](../adr/0006-mastering-pipeline.md))

## 5. Sample material

See [`samples/`](../../../samples/) in the repository:

- `manuscript.ja.md` — sample manuscript (this workflow's input)
- `script/yoiyami.jsonl` — its reference script (example output of Steps 2/5)
- `casting.example.toml` — casting table template

For a plumbing-only check, skip Steps 2–5: copy the sample script and
casting into a workspace and start at Step 7.
