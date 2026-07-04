# ADR-0004: Per-line content-hash synthesis cache (pauses excluded)

- **Status**: Accepted (2026-07-04)

## Context

Synthesizing a long script (hundreds of lines) takes minutes to tens of
minutes on CPU. Real production is an edit-and-rerun loop; resynthesizing
everything each time would dominate the experience. Agents also need cheap
recovery from lost jobs (ADR-0005).

## Decision

Hash each line's synthesis inputs and skip on match.

- Key: `SHA256("v2|" + engineVersion|speakerUUID|styleID|speed|intensity|volume|prePhoneme|postPhoneme|samplingRate|text)`
- Storage: `cache/index.json` (line_id → {hash, duration_seconds,
  synthesized_at}), persisted atomically (temp+rename) after every line.
- Hit requires the hash to match **and** `wav/<id>.wav` to exist.
- `force=true` bypasses everything (e.g. after a model update).

**Excluded**: `pause_after_ms`. Pauses are not baked into WAVs — they are
inserted at mastering time — so pacing tweaks (frequent) never trigger
re-synthesis.

**Included**: engine version and speaker_uuid. Engine/model updates and
recasting can change output, so they intentionally invalidate the cache.

## Consequences

- Edit 3 lines, re-run → 3 lines synthesized. Retakes overwrite
  `wav/<id>.wav` deterministically.
- Re-running `synthesize_script` after a lost job is effectively a
  differential run — the precondition that makes ADR-0005's
  no-persistence design viable.
- Changing the key layout invalidates all caches (generation-managed via
  the `v2|` prefix).

## Alternatives considered

- **mtime-based**: regenerating the script file would invalidate every
  line. Rejected.
- **Hashing the output WAV**: requires synthesizing first; useless here.
- **Including pauses in the key**: re-synthesis on every pacing tweak.
  Rejected.
