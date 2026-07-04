# ADR-0008: Automate license collection from AIVM manifests; keep verification human

- **Status**: Accepted (2026-07-04)
- **Amends**: [ADR-0007](0007-license-metadata-as-data.md) (policy retained; adds a collection mechanism)

## Context

ADR-0007 assumed "the engine API exposes no model terms" and required fully
manual `[[speaker_metadata]]` entry. A live investigation (2026-07-04)
showed:

- The AivisSpeech-specific `GET /aivm_models` returns the **AIVM manifest**
  of every installed model, including **`license` (the full license text)**,
  `description` (often containing the requested credit line), and
  `creators`.
- Of 20 real models: 17 embed the full ACML 1.0 text; CC0, JVNV-corpus
  terms, and URL-only terms appear once each (absent text is also possible).
- So *collection* can be automated locally — while *interpreting and
  approving* the terms (commercial use, fitness for publication) remains
  prose reading that should not be automated (ADR-0007's core stands).

The friction was real: the pilot production warned unverified for all 10
models, each needing manual entry.

## Decision

Split the roles: **collection is automated, verification stays human.**

1. `engine.Client.AivmModels()` reads `/aivm_models` (local only; still no
   AivisHub or other external access).
2. License status becomes three-valued:
   - `verified` — a human read the terms and recorded them in
     `[[speaker_metadata]]` (as before)
   - `declared` — **the manifest embeds license text** (name = first
     heading, credit extracted from the description's
     「AivisSpeech: 名前」 pattern); notes flag it as unreviewed
   - `unverified` — neither declaration nor record
3. `list_speakers` attaches declared info automatically, falling back
   silently when `/aivm_models` is unavailable (e.g. a VOICEVOX engine).
4. A new **`licenses`** subcommand provides the review workflow:
   an overview table; `--full <speaker_uuid>` to read the complete text;
   `--toml` to generate `[[speaker_metadata]]` skeletons for unreviewed
   speakers (with a `REVIEW:` note to delete once the terms are accepted,
   and license_url / commercial_use left for the human).

## Consequences

- Manual work shrinks from transcribing UUIDs/names/licenses to reviewing a
  generated skeleton and filling license_url / commercial_use.
- `declared` means nothing more than "the author says so". Agents may weigh
  it during casting, but `master` keeps warning about everything that is
  not **verified** — the publishing gate is unchanged.
- Name/credit extraction is heuristic (heading line, 「」 pattern); a miss
  only leaves a blank field.
- `/aivm_models` responses are huge (147KB for 20 models, base64 icons);
  the client parses only the needed fields via selective structs.

## Where the registry lives (data boundary)

The verified registry (`[[speaker_metadata]]`) is **user data** and lives
only in the user config (`~/.config/voice-studio-mcp/config.toml`). It is
**never bundled into the product repository**:

1. Installed models differ per environment (environment-specific data)
2. "Verified" is the user's own judgment; shipping it in a repo would
   distribute legal conclusions to third parties
3. Terms change over time (which is why notes record the review date)

Note that speaker_uuid is a **global, model-intrinsic ID**, not a
machine-local one — so the registry is portable as a *personal* asset
(the same model on another machine matches the same UUID). Syncing the
config via personal dotfiles is a reasonable practice.

## Alternatives considered

- **Fetching from the AivisHub API**: violates the local-only policy; the
  embedded full text suffices, leaving only license_url for the human.
  Rejected.
- **LLM interpretation of the terms to auto-set commercial_use**:
  misjudgments carry legal consequences and would erode ADR-0007's
  "approval is a human responsibility". Rejected.
- **Keeping two statuses and folding declarations into verified**: erases
  the distinction between "someone read it" and "it is written somewhere",
  hollowing out the publishing gate. Rejected.
