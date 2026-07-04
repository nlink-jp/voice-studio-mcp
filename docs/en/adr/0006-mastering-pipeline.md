# ADR-0006: Mastering via concat demuxer + one-pass loudnorm; two volume knobs

- **Status**: Accepted (2026-07-04)

## Context

The final audio is produced by concatenating the line WAVs in script order
with per-line pauses, leveling perceived loudness, and encoding for
distribution. Design questions: (1) how to concatenate in ffmpeg, (2) the
loudness-normalization method and target, (3) separating the two different
requirements both called "volume", (4) chapter embedding.

## Decision

1. **Concatenation uses the concat demuxer with silence WAV files.** One
   silence WAV is rendered per distinct pause duration (deduped) and
   `concat.txt` interleaves line WAVs with silences for a single final
   encode. A filtergraph (`aevalsrc`/`adelay` chained per line) is rejected:
   command length grows with script length and debugging becomes hopeless.
2. **One-pass loudnorm, target -18 LUFS (default).** Started at -16 (podcast
   standard); listening feedback called it too loud, so the default moved
   to -18 (audiobook range, ≈0.8× amplitude). Tunable via
   `master.loudnorm_i`. Two-pass (measure→apply) is more accurate but
   deferred.
3. **Two volume knobs**: per-line `volume` (engine volumeScale, part of the
   cache key) sets **relative balance** between lines (whispers, shouts);
   the **perceived loudness** of the master output is governed by loudnorm,
   i.e. `loudnorm_i`. This separation is documented in the README/schema to
   prevent the "raised volumeScale but loudnorm cancelled it" confusion.
4. **m4b uses the ipod muxer with ffmetadata chapters.** Chapter start
   times are precomputed from measured WAV durations + pauses (loudnorm
   does not change duration, so they are exact). `chapters=false` disables
   them if player compatibility bites; mp3 is the first-class output.

## Consequences

- Even long works need only (distinct-pause-count) silence renders plus one
  final ffmpeg invocation.
- Missing synthesized lines abort with `master_incomplete`
  (missing_line_ids attached) — no silent partial masters.
- One-pass loudnorm is less exact than two-pass but fine for narration
  (measured: -18.2 LUFS against the -18 target).

## Alternatives considered

- **Single filtergraph render**: command-length explosion. Rejected.
- **Bake volume at synthesis and skip normalization**: leaves inter-line
  and inter-model level differences. Rejected.
- **Keep -16 LUFS default**: changed to -18 after listening feedback.
