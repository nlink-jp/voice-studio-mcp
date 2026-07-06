# ADR-0012: Fix the unsigned-engine launch failure with local self-repair (doctor --fix)

- **Status**: Accepted (2026-07-05)

## Context

AivisSpeech ships as an **unsigned, un-notarized** PyInstaller bundle whose
nested native libraries (onnxruntime, numpy, …) carry signatures from
**different TeamIDs**, with the outer executable unsigned. Modern macOS
(especially arm64) requires every loaded Mach-O to have a consistent
signature, so this "signature soup" is **killed at load time** (Library
Validation / cdhash / TeamID mismatch).

Key facts from field reports:

- We spawn the engine's `run` directly in managed mode, bypassing the
  LaunchServices approval (which clears quarantine) that a GUI launch would
  perform — so it fails on machines where that approval never happened.
- **Removing quarantine alone is insufficient on some machines** (level 2):
  the signature inconsistency itself blocks launch even after
  `xattr -dr com.apple.quarantine`.
- The proven manual fix was **`codesign --force --sign -` (ad-hoc) over the
  whole subtree**: making signatures uniform removes the TeamID mismatch and
  clears Library Validation.

## Decision

Rather than owning distribution (fork / notarize / reimplement), provide
**on-machine self-repair** via `doctor --fix` — an automation of the manual
fix users already proved.

- **Detection (read-only)**: bare `doctor` checks the engine subtree for a
  quarantine flag and whether `codesign --verify` passes, and reports
  "needs repair — run `doctor --fix`" (doctor itself does not fail).
- **Repair (`doctor --fix` only, mutating)**:
  1. `xattr -dr com.apple.quarantine <engine-dir>` — strip quarantine
     recursively.
  2. Enumerate Mach-O files by magic bytes and **ad-hoc re-sign inside-out**
     (deepest `.dylib`/`.so` first, `run` last) with
     `codesign --force --sign -`, making all signatures ad-hoc (null TeamID)
     and clearing the mismatch and Library Validation.
- **Scope is the engine subtree only** (the directory of `engine.command`).
  We only spawn the engine; we never touch the wider Electron `.app` and
  never roam outside the bundle.
- `--deep` is not used (Apple-deprecated, signs outside-in incompletely).
- The supervisor's early-exit error points at `doctor --fix`.

## Consequences

- **Minimal-cost fix**: no fork, no notarization, no 1GB distribution, no
  LGPL/model-redistribution homework — right-sized for a local, single-user
  macOS tool's threat model.
- It mutates the user's installed bundle under `/Applications`, so it is
  **opt-in** (`--fix`); bare doctor stays read-only.
- An AivisSpeech update restores quarantine/signatures, so re-run after
  updates (doctor re-detects and guides).
- Detection walks a ~2500-file subtree but only reads magic bytes; fast.
- `xattr`/`codesign` run through a Runner interface; a fake runner pins the
  inside-out order, force-ad-hoc signing, quarantine stripping, idempotency,
  and error surfacing hermetically (no AivisSpeech / macOS required).

## Alternatives considered

1. **Signing-overlay fork** (build AivisSpeech-Engine ourselves → sign
   inside-out under our Developer ID → notarize → engine-only download
   asset): correct-by-construction and never touches the user's machine,
   but carries recurring cost — the PyInstaller-notarization spike, a 1GB
   asset, LGPL corresponding-source provision, model curation, and upstream
   tracking. **Held as the second stage** if upstream never notarizes or
   demand broadens; self-repair suffices for now.
2. **Full bundling** (engine + models in our binary): all of the above plus
   non-commercial model-license conflicts. Rejected.
3. **CGO reimplementation** (the image-forge pattern): SBV2 has no mature
   C++ port, so we would maintain Japanese G2P + BERT + SBV2 ourselves — far
   too indirect a route to solving a signing problem. Rejected.
4. **Guiding users to remove quarantine only**: cannot save level-2
   (signature-mismatch) machines. Rejected.
5. **`codesign --deep`**: Apple-deprecated, signs outside-in incompletely.
   Rejected.

## Posture toward upstream

Consistent with avoiding a fork, the ideal outcome is **upstream notarizing
so this repair becomes unnecessary**. It is worth a courtesy note to
AivisSpeech that their unsigned bundle breaks agent-spawned use.
