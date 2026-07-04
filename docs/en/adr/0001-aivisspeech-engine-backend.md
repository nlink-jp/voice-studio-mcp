# ADR-0001: AivisSpeech Engine as the sole v1 TTS backend

- **Status**: Accepted (2026-07-04)

## Context

The quality requirements are fluent Japanese (accent/intonation), multiple
speakers, fully local operation (macOS / Apple Silicon), and licenses that
exclude commercial packaged products. A comparative study (deep-research,
25 sources / 24 verified claims, 2026-07) established:

- **Style-Bert-VITS2-family** models are the current state of the art for
  Japanese quality, but using the repository directly is unofficial on
  macOS (verified environments are Windows/WSL2/Linux only; MPS produces
  corrupted audio) and the code is AGPL-3.0.
- **AivisSpeech Engine** runs SBV2-family models in AIVMX (ONNX) format on
  CPU, officially supports macOS 13+ (Apple Silicon recommended), inherits
  only the LGPL-3.0 half of the VOICEVOX dual license, and speaks the
  VOICEVOX-compatible HTTP API.
- **Kokoro-family** (MIT/Apache 2.0) is fastest but ships only two Japanese
  preset voices — failing the multi-speaker casting requirement.
- **VOICEVOX** has a rich character roster but no quality advantage over
  SBV2-family, and its Mac build is CPU-only.
- **fish-speech etc.** were excluded for non-commercial model licenses.

## Decision

v1 targets AivisSpeech Engine exclusively. Because the API is
VOICEVOX-compatible, the client in `internal/engine` stays engine-agnostic
so a VOICEVOX ENGINE connection (wider voice roster) can be added later.

## Consequences

- Japanese quality, speaker extensibility (AivisHub models, AIVM-converted
  SBV2 checkpoints), official macOS support, and LGPL are satisfied at once.
- The engine is non-streaming (WAV after full synthesis) — acceptable
  because this project is batch-oriented (real-time narration is out of
  scope).
- Installing AivisSpeech becomes a runtime prerequisite (we do not bundle
  the >1GB engine; model management belongs to AivisSpeech).

## Alternatives considered

- **Direct SBV2 operation**: unofficial on macOS + AGPL. Rejected; the same
  quality is available via AivisSpeech under LGPL.
- **Kokoro (mlx-audio / kokoro-onnx)**: best latency, but two Japanese
  voices fail the requirement.
- **Multi-engine support from v1**: doubles license-metadata curation and
  the verification surface. Deferred to Phase 2+.
