# ADR-0011: Consolidate the bundled skill into multi-actor-narration; ship it as a separate .skill asset

- **Status**: Accepted (2026-07-05)
- **Amends**: ADR-0009 (the bundled skill's identity and distribution; the in-repo coherence principle is reinforced, not reversed)

## Context

ADR-0009 bundled the agent-facing skill (radio-drama) in this repo so it
evolves in lockstep with the server's contracts. Two things changed since:

- The radio-drama skill carried **novel-works-specific coupling** — it told
  the agent to prefer "novel-works-style 設定資料/キャラクター早見表".
  novel-works is a closed, unpublished project; a publicly-general MCP
  should not assume it.
- A more capable skill matured out-of-repo (`magifd2/claude-skills`):
  **multi-actor-narration**, a self-contained router covering four explainer
  formats (talk-podcast, panel-discussion, news-briefing, lesson-narration)
  over the same voice-studio schema, plus a `.skill` packaging convention
  (a zip whose top-level entry is the skill directory). ADR-0009 itself left
  the door open: "zip bundling can be added later if demand appears".

The story→audio-drama capability of radio-drama is generic and worth
keeping — only its novel-works assumptions were inappropriate.

## Decision

1. **Consolidate to a single bundled skill, `multi-actor-narration`.** The
   former radio-drama skill is removed; its story→朗読劇/audiobook capability
   returns as a **generalized `audio-drama` format** inside
   multi-actor-narration, with the novel-works coupling replaced by a
   format-agnostic "if character reference material exists (setting notes,
   character sheets, first-person/口調/呼称 tables — any form), read it
   first" instruction.
2. **Keep the skill source in-repo (ADR-0009 reinforced).**
   `skills/skills_test.go` now aggregates the whole multi-file skill
   (SKILL.md router + `_shared/*.md` + `<format>/FORMAT.md`) and asserts
   every tool, error code, and schema field appears, plus that exactly one
   frontmatter `SKILL.md` exists (the packaging invariant a nested manifest
   would violate).
3. **Package and release the skill as a separate `.skill` asset.**
   `skills/build.sh` (auto-discovers skills under `skills/`, enforces the
   no-nested-SKILL.md invariant) plus `make package-skill` produce
   `dist/multi-actor-narration.skill`. It ships on the same GitHub release as
   the MCP binary but as a **distinct asset** with a different lifecycle
   (no notarization; a plain skill zip).
4. **novel-works keeps its own specialized skill privately.** Its
   manuscript-specific radio-drama coupling stays out of the public tool's
   scope.

## Consequences

- The public tool carries no novel-works assumptions; one skill covers
  朗読劇 + four explainer formats.
- The skill stays pinned to the server schema — ADR-0009's coherence
  guarantee now holds across the multi-file layout.
- **Breaking**: the `/radio-drama` command name is gone; requests route to
  `/multi-actor-narration` (its description carries the
  朗読/ラジオドラマ/オーディオブック/novel/manuscript triggers). novel-works,
  which invoked `/radio-drama`, must migrate or keep a private copy —
  tracked as a follow-up outside this repo.
- Skill distribution now has two paths: `make install-skill` for repo
  holders, and the `.skill` release asset for everyone else (this realizes
  the "zip bundling later" ADR-0009 left open).

## Alternatives considered

- **Move the skill to skills-series / a standalone repo**: would break the
  in-repo coherence test (cross-repo schema sync = drift). Rejected — same
  reasoning as ADR-0009.
- **Keep radio-drama and add multi-actor-narration side by side**: two
  skills with overlapping synthesis pipelines and duplicated `_shared`
  content; the story use case fits cleanly as one more format. Rejected in
  favor of one router.
- **Bundle the .skill inside the MCP binary zip**: couples two different
  artifacts and lifecycles; a separate asset is cleaner and lets skill-only
  users grab just the skill.
