# ADR-0013: The work directory is a per-call `work_dir`, with no default root

- **Status**: Accepted (2026-09-13)
- **Amends**: [ADR-0010](0010-agent-prepared-workspaces.md) (agent-prepared workspaces)

## Context

This applies organization ADR-021 (the work-directory contract for file-mediated
MCP servers) here. ADR-0010 settled that the server works in the workplace the
agent prepared, but named the argument `workspace_root` and **fell back to
`~/.voice-studio` when it was omitted** — a directory no calling agent's file
tools can open, so the failure only ever appears as synthesis succeeding and the
returned path being unopenable.

Across the fleet the same argument was spelled three ways, and measurement of the
four calling runtimes showed that neither MCP `roots` nor the environment reaches
half of them. **The per-call argument is the only common channel.**

## Decision

1. **The argument is `work_dir`, required by all four workspace tools**
   (synthesize_script, synthesize_line, register_dictionary, master). It means the
   absolute path of a directory the caller can read back; the workspace is
   `<work_dir>/<workspace_id>/`.
2. **Resolution is argument → `_meta["jp.nlink/work_dir"]` → error.** No default
   root, and the manager's default-root operations (`Ensure`, `Root`, `List`,
   `Delete`) are deleted with it.
3. **Validation is a closed list** (absolute, no `~`, no `..`, exists and is a
   directory, writable, not a system or credential location), with five
   `work_dir_*` codes. The directory is not created.
4. Containment inside the workspace (`os.Root`) is unchanged from ADR-0010.
5. **A media chain shares one `work_dir`**: image-forge renders the page images,
   this server synthesizes the audio, video-studio muxes them — all under one
   directory. Sharing the `<work_dir>/<workspace_id>/` namespace is deliberate.

## Consequences

- **Breaking.** A call sending `workspace_root` is refused with the new name.
- `internal/workdir` is byte-identical to the copies in voice-scribe,
  pcap-analyzer, gem-scribe and image-forge.
- `~/.voice-studio` stops being used; anything already there is left alone.

## References

- Organization ADR-021, voice-scribe ADR-0010 (reference implementation),
  pcap-analyzer-mcp ADR-0008
- [ADR-0010](0010-agent-prepared-workspaces.md) — this record replaces its
  argument name and its default
