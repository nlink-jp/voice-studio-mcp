# ADR-0015: The skill sits at the repository root and ships as a zip on every release

- **Status**: Accepted (2026-11-04)
- **Amends**: [ADR-0009](0009-bundle-skill-with-mcp.md) (where the skill lives and how it is tested),
  [ADR-0011](0011-consolidate-skill-into-multi-actor-narration.md) (how it is packaged)

## Context

ADR-0011 made the skill a separate release asset, `multi-actor-narration.skill`,
built by `make package-skill`. Nothing in the release made building or attaching
it necessary, so it reached a release twice — v0.4.0 and v0.4.1 — and the seventeen
releases from v0.4.2 to v0.6.4 carry the binary only (nlink-jp/voice-studio-mcp#3). A user who installs
the skill from a release gets the v0.4.1 skill, which still teaches
`workspace_root`, the argument v0.5.0 replaced with a required `work_dir`. Since
mcp-tactics v0.13.0 no longer lists this server, the README and the release are
the only places a user learns the skill exists.

Organization ADR-027 (Decision 2) has since set the shape of a skill bundled
with its tool: the skill at the repository root, the organization's skill
validator in `make test`, tests that hold it to the server in both directions,
and a separate release asset. It does not ask an existing skill to move; this
repository chose to, because the packaging was being reworked anyway.

## Decision

1. **The skill moves from `skills/multi-actor-narration/` to
   `multi-actor-narration/`** at the repository root, where the vendored
   `tests/validate-skill.sh` (organization ADR-006) finds it. `make test` runs
   the validator after `go test ./...`. `skills/build.sh` is removed.
2. **The package is `dist/multi-actor-narration-<version>.zip`**, whose root
   is the skill folder — the layout the app's skill registration accepts and
   the skills-series repositories ship. The `.skill` name is retired.
3. **`make package` builds it**, next to the binary zip, and
   **`make verify-release` refuses a release without it**: its prerequisite
   `verify-skill-zip` fails when the zip for this version is missing, holds
   anything but the tracked files of the skill, or differs from them. The
   release uploads both zips.
4. **`make install-skill` installs from the built zip**, not by copying the
   source tree, so a packaging defect breaks a developer's install before it
   reaches a release.
5. **The tests hold the skill to the server in both directions**
   (`internal/tools/skill_test.go`):
   - every registered tool except `get_usage` is named by the skill;
   - every backticked identifier in the skill is a registered tool, a property
     of a tool's input schema, a `toolerr` code or a json/toml tag of the
     server's own types — read from the server, not copied. The few names
     that are not the server's (fields of the skill's own source cards, the
     calling client's tools) are listed with what they are;
   - retired names (`workspace_root` and its spellings, `~/.voice-studio`,
     `/radio-drama`, the old path) are held absent;
   - every format's script and casting templates parse with the server's own
     parsers, and the script resolves against its casting table.

   The last check found three templates that could not be used as written: two
   scripts used a style their casting table did not map, and one used a
   character the table left commented out.

## Consequences

- A release without the current skill fails `verify-release` instead of
  shipping.
- A name the skill teaches that the server does not have fails `go test`,
  whichever file of the skill it is in.
- The README's installation section leads with registering the zip in the app
  and says where gem-agent and lagent read skills
  (`~/.config/gem-agent/skills/`, `~/.config/lagent/skills/`; copied, not
  linked).
- v0.4.0 and v0.4.1 keep their `.skill` assets; nothing re-uploads to past
  releases.

## Alternatives considered

| Alternative | Why not |
|---|---|
| Fix only the packaging and leave `skills/` | Leaves this repository the one exception to ADR-027's layout and the organization validator out of `make test` |
| Rely on the release checklist to attach the asset | The checklist existed for v0.4.2 to v0.6.4 and the asset was skipped every time; a step that only prose requires gets skipped |
| Re-upload the current skill to past releases | A past release's skill should match that release's server |
