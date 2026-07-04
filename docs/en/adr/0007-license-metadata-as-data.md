# ADR-0007: Voice-model usage terms are data, not code

- **Status**: Accepted (2026-07-04)

## Context

This server's output is distributable media (radio dramas, audiobooks), so
compliance with each voice model's terms (attribution requirements,
commercial-use conditions) is a precondition for publishing. Yet the engine
API (/speakers) exposes no terms. AIVMX model terms are separate from any
software license and differ per model (AivisHub model pages, ACML, etc.).

## Decision

Terms are recorded in two places as **human-verified data**, which the
tools then carry:

1. **Config `[[speaker_metadata]]`** (machine-wide registry):
   speaker_uuid → license name / URL / credit line / commercial-use flag /
   notes. `list_speakers` joins it and reports unregistered models as
   `license.status = "unverified"` so the agent can weigh terms during
   casting.
2. **`credit` / `license_checked` in casting.toml** (per-work record): a
   human confirmed this model may be used for this production. `master`
   auto-generates `<name>.credits.txt` from the used characters' credits
   and returns characters with `license_checked = false` as
   `unverified_models`, prompting review before publishing.

Interpreting and approving terms is deliberately not automated — that is a
human responsibility.

## Consequences

- "Which models are cleared" lives explicitly in config (per machine) and
  casting (per work); missing credits are structurally prevented.
- Every added model needs manual metadata entry — intentional friction;
  publishing without reading the terms costs more.
- Tracking term changes remains manual (recording a check date in `notes`
  is recommended).

## Alternatives considered

- **Auto-fetching from AivisHub etc.**: terms are prose; automatic
  interpretation is dangerous, and external access contradicts the
  local-only policy. Rejected.
- **Carrying no license data**: no credits generation, no unverified
  warnings — inadequate for a tool that produces distributable media.
  Rejected.
