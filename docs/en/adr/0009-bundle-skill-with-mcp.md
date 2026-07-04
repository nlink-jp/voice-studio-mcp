# ADR-0009: Bundle the radio-drama skill in this repository, not skills-series

- **Status**: Accepted (2026-07-04)
- **Supersedes**: the RFP's scope decision "the skill is a separate skills-series project"

## Context

At RFP time the agent-facing production skill (radio-drama) was planned as
a separate skills-series project. After the v0.1.0 release and the pilot
production, reality looked different:

- Most of the skill's content **references this MCP's contracts**: the
  script JSONL schema (canonical in `internal/script`), the six tools'
  arguments and results, the error-code dispatch table, the three-valued
  license status.
- All of those evolve in this repository. A separate repo would need
  **cross-repo synchronization** on every tool addition, schema change, or
  error-code change — a drift factory (ADR-0008's three-valued status
  alone already forced a follow-up edit of the workflow guide).
- The workflow guide (docs/reference/agent-workflow) and sample material
  (samples/) already live here; the skill is just their operational form.
- skills-series is a monorepo of *development-process* skills (rfp, …); a
  product-companion skill is a different animal.

## Decision

Bundle the skill as `skills/radio-drama/SKILL.md` in this repository, with
`make install-skill` deploying it to `~/.claude/skills/`.

- The skill evolves **in the same commits and versions** as the MCP server.
- `skills/skills_test.go` machine-checks skill/server coherence: every tool
  name referenced, every error code in the dispatch table, schema example
  fields matching what the parser accepts. Forgetting to update the skill
  breaks the build.
- docs/reference/agent-workflow (both languages) stays as the design
  document; the skill is its operational form.

## Consequences

- Schema/tool/error changes complete in one commit and one review.
- Skill distribution = clone the repo + `make install-skill`; it is not in
  the release zip (skill users are developers/creators who have the repo;
  zip bundling can be added later if demand appears).
- radio-drama is not added to skills-series; in the org catalog it appears
  as part of voice-studio-mcp.

## Alternatives considered

- **Separate skills-series project** (original plan): contract-sync cost
  and drift risk outweigh centralized skill management. Rejected.
- **Bundling the skill in release zips**: no demand yet; revisit later.
- **Dropping the workflow guide in favor of the skill**: the guide keeps
  its value as the "why"-bearing design document; both stay with explicit
  roles.
