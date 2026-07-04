# ADR-0003: AudioQuery is a map[string]any passthrough, not a struct

- **Status**: Accepted (2026-07-04)

## Context

Synthesis uses the VOICEVOX-compatible two-step API (`POST /audio_query` →
adjust parameters → `POST /synthesis`). Beyond the VOICEVOX baseline fields
(`speedScale`, `intonationScale`, `prePhonemeLength`, …), AivisSpeech adds
its own (`tempoDynamicsScale` etc.) and will likely add more. A fixed Go
struct would **silently drop** unknown fields on the round trip, turning
them into audio-quality bugs (deviations from engine defaults) that are
very hard to trace.

## Decision

`engine.AudioQuery` is `map[string]any`. The server overrides only the keys
it owns:

- `speedScale` / `intonationScale` / `volumeScale` — per-line direction
- `prePhonemeLength` / `postPhonemeLength` — config defaults
- `outputSamplingRate` / `outputStereo` — **forced to one config value on
  every line**

Everything else passes through untouched.

## Consequences

- The server tracks engine schema drift with zero code changes (this was
  the top implementation risk).
- Unified sampling rates let master's concat demuxer join WAVs losslessly,
  eliminating per-model native-rate mismatches at the root.
- Type safety is traded away, but only six keys are touched and tests pin
  the behavior (the mock engine returns an AudioQuery containing unknown
  fields and asserts they survive).

## Alternatives considered

- **Complete struct definition**: field-loss bugs on every engine update.
  Rejected.
- **Struct + raw remainder**: Go has no standard inline-remainder JSON
  support; extra complexity for nothing. Rejected.
