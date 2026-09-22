# ADR-0014: Leave path judgement to nlink-jp/pathguard — keep no copy

- Status: Accepted
- Date: 2026-09-22

## Context

Since ADR-0013, `work_dir` validation lived in `internal/workdir`, a copy of voice-scribe's (the
reference implementation of organization ADR-021); seven other servers held the same copy. Every copy
compared places **by name**. APFS is case-insensitive by default, so spellings such as `~/.SSH` and
`/USR/local` named the same places and passed the checks. When the home directory could not be
determined, the credential-location check passed everything.

The organization moved this judgement into one module (`nlink-jp/pathguard`, lib-series). It compares
places by file identity and by names folded the way the disk folds them, and it catches a place that
does not exist yet through the identity of its parent. It holds one list, the same as gem-agent's and
lagent's.

## Decision

- Depend on `github.com/nlink-jp/pathguard` v0.1.0. No code from outside this organization comes
  with it.
- `internal/workdir` becomes a **thin adapter**. It keeps only:
  - taking the request's `_meta` from the context and passing it to `pathguard/workdir`'s `Resolve`,
  - moving that `*workdir.Error` onto `toolerr` with the same code, message and details,
  - `NewResolver(serverDirs...)` — passing this server's own directories (today only its config
    directory, `~/.config/voice-studio-mcp`) as protected places (`pathguard.ServerDir`), and its one
    sentence for `work_dir_required` as `RequiredHint`. An empty path refuses every call rather than
    protecting nothing (the config directory is undetermined only when the home directory is, and
    then pathguard refuses everything anyway).
- It keeps no `Sensitive`. Every file argument of this server (`script_path`, `casting_path`, …) is
  workspace-relative and contained by `os.Root`; nothing outside the work directory is read.
- The call sites (`Resolve`, `Validate`) do not change. What changes is the one line that builds the
  resolver (`workDirResolver` in `cmd/tools_registry.go`) and the tests that built it as a zero value.
- The tests of the judgement itself are in pathguard. What stays here are the adapter's tests (taking
  `_meta`, carrying the error across, the protected place, a zero value refusing) and the existing
  contract and wiring tests.

## Consequences

The `work_dir` check changes (the CHANGELOG says so):

- **Refused now**: the real places under your home from the runtimes' list (`~/.kube`,
  `~/.config/gh`, `~/.azure`, `~/.terraform.d`, `~/.gemini`, `~/.config/mcp-bridge`, `~/.netrc`,
  `~/.npmrc`, `~/.pypirc`, `~/.git-credentials`, `~/.vault-token`, `~/.docker/config.json`,
  `~/.claude.json`, `~/.bash_history`, `~/.zsh_history`); every spelling of any floor place — case
  variants, links, firmlinks; wherever a link directly inside one of those directories points; when
  `$HOME` names another directory than the account's home, both; Linux `/etc`.
- **An unknown home refuses every `work_dir`.** It used to pass them.
- `work_dir_denied` carries `reason` in its `details`.
- One check costs about 2 ms (measured in pathguard) — nothing next to a synthesis.

With no copy here, a fix to the judgement is a pathguard release and a one-line dependency update.

## Amendment (2026-09-22): judge the directory actually used

Only `work_dir` was checked, so `work_dir=~/.config` with `workspace_id=gh` made the workspace
`~/.config/gh` and audio was written into it. The hole dates from ADR-0013; image-forge's independent
review found it. `workspace.NewManager(check)` takes the judgement as a required argument, and
`EnsureUnder` judges `<work_dir>/<workspace_id>` with `workdir.Resolver.CheckBeneath` (pathguard
v0.2.0) before making or using it; `newToolDeps` wires it. A Manager without one refuses every
workspace. pathguard v0.2.0 also refuses a path holding a NUL byte.

## Amendment (2026-09-22, v0.6.1): judge every file the workspace reads before reading it

`script_path` and `casting_path` are workspace-relative and read through the `os.Root`, but the floor
was not applied to them. A workspace passes `CheckBeneath` and can still contain places on the floor (a
`.env`, this server's config directory, the target of a link in `~/.ssh` when the workspace is in
that sync folder). Such a file was read as a script or casting table and its contents came back in
the parse error (`unknown keys: [SECRET]`, `invalid character 'S'`), and the answer differed from the
"not found" for a missing one. It is the existence class the independent reviews of
slack-mcp-extender and chrome-pilot-mcp found, measured here with the home directory redirected to a
temporary one (12 of 20 pairs; the 8 planted links out of the workspace were refused by the `os.Root`
whether or not their targets existed).

- The workspace judges every read — `ReadFile`, `Stat`, `VerifyRegular` — before it reads or looks
  (`Workspace.judge`, internal/workspace/manager.go), by pathguard's Local policy with this server's
  own directories. `workspace.NewManager(check, floor)` takes the floor as a required argument
  (`workdir.Resolver.LocalPath`; a Manager without one refuses every workspace), and a Workspace built
  without one refuses every read. pathguard follows the links on the path itself, so no separate
  placement is needed; the refusal names the path only as given.
- A first version judged `script_path` and `casting_path` in the tools. Its independent review found a
  third read that skipped it: `master` reads `wav/<id>.wav` for every line, and a link planted there
  reached a place on the floor inside the workspace ("not a RIFF/WAVE stream (36 bytes)" when present,
  `missing_line_ids` when absent; the synth cache's `cached` count likewise). Three reads, one of them
  missed, is a class: the judgement moved into the one place every read goes through. A refused
  `wav/<id>.wav` now counts as missing, the same answer as no file. It also closed a leak nobody had
  named: a link planted at `dict/words.json` or `cache/index.json` to a JSON file on the floor was
  loaded and saved back into the workspace as an ordinary file the caller could read.
- Cost: one judgement is a walk of the credential directories (about 2.5 ms here), and a Workspace
  remembers its answer per path, so a line's WAV read and then verified in one step is judged once. A
  job runs after the call that queued it, possibly behind other jobs, so it takes a `Fresh` workspace
  that remembers nothing: its first memo version reused the call's answers, and a WAV swapped for a
  link while the job waited counted as cached when the file behind it existed (the narrow review of
  the memo found it). `master` over 300 lines went from about 0.10 s to 0.75 s with a fake ffmpeg, and a cached
  `synthesize_script` re-run of 300 lines from 0.09 s to 0.72 s (measured 2026-09-22); a real ffmpeg
  pass over that much audio takes far longer. A check prepared once per call belongs in pathguard.
- ffmpeg reads the concat list and the chapter metadata the server writes just before spawning it;
  those two are not judged, and redirecting them needs a swap in between (the race below).
- `TestExistenceIsNotRevealed` calls `synthesize_script`, `synthesize_line` and `master` with the
  same path while a file is there and after it is removed, compares the whole answer and checks that
  none of the file's contents comes back (it cannot see a read that leaves the answer unchanged).
  `TestMasterDoesNotReadAFloorFileThroughAWav` pins the wav case, and
  `TestEveryReadIsJudgedBeforeItLooks` (internal/workspace) each of the three reads and a Manager or
  Workspace without a floor, `TestAPathIsJudgedOncePerWorkspace` the memo,
  `TestServerNamedFilesAreJudged` the cached count and the dictionary record through a planted link,
  `TestTheServersWorkspacesJudgeEveryRead` (cmd) the server's own wiring through `newToolDeps`, and
  `TestAJobJudgesAfresh` a job that waits while a WAV is swapped. Twelve mutations (no floor, each of
  the three reads unjudged, a Manager without a floor accepted, the floor not handed to the workspace,
  the wiring's floor a no-op or nil, the memo remembering a pass for a refusal or not remembering at
  all, the job reusing the call's workspace, `Fresh` keeping the memory) all fail by assertion.
- Known limits in pathguard, recorded for its next release:
  - `work_dir` is validated by pathguard/workdir in the order organization ADR-022 §4 sets (not found
    before denied), so a `work_dir` naming a credential directory is answered by whether it exists.
  - A link target with a non-ASCII name spelled in another Unicode normalisation is found by identity
    only while it exists (pathguard does not normalise). A hard link made elsewhere is refused only when
    it is to a file that is itself a place on the floor (`~/.netrc`, `~/.docker/config.json`, …), and
    only while it exists; one to a file inside a credential directory (`~/.ssh/id_rsa`) or to a `.env`
    is not refused at all — a directory is compared by its own identity, not by its files'.
- The judgement and the read are two steps, and a link swapped in between them is followed: a
  check-to-use race, not closed here (closing it means judging what was opened, by its descriptor).
  Within one step a path's answer is remembered, so a directory swapped for a link between a line's
  read and its verification before ffmpeg is not judged again.

## References

- Organization ADR-021 (the work-dir contract of the file-mediated MCP servers)
- ADR-0013 (work-dir contract): the closed list of checks — whose implementation this replaces
- nlink-jp/pathguard's RFP (`docs/en/pathguard-rfp.md`)
