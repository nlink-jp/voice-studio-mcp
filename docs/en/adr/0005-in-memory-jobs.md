# ADR-0005: Async jobs are in-memory only (no persistence)

- **Status**: Accepted (2026-07-04)

## Context

`synthesize_script` runs long, so the tool returns a job_id immediately and
clients poll `check_job`. The open question was job survival across server
restarts (MCP client reconnects, machine reboots). Persisting jobs brings
real complexity: state-machine recovery, orphan resumption, double-run
prevention.

## Decision

The job table lives in memory only.

- After a restart, `check_job` returns `job_not_found` whose message
  **spells out the recovery**: jobs do not survive restarts; re-run
  `synthesize_script` and the cache (ADR-0004) synthesizes only the
  missing lines.
- Jobs run under the server-lifetime context; on SIGTERM the remaining
  lines are aborted and the job ends state=failed with the same recovery
  guidance in fatal_error.
- At most 32 finished jobs are retained (oldest evicted).
- All jobs share one semaphore (`synthesis.concurrency`, default 1), so
  parallel jobs cannot overload the engine.

## Consequences

- The implementation is simple; recovery bugs are structurally absent.
- "Recovery = the agent re-runs" leans on the callers being autonomous
  agents — the error-message wording is the UX.
- If the server dies mid-epic, only the progress view is lost; synthesized
  lines remain in the cache.

## Alternatives considered

- **Disk-persisted resumable jobs**: little return — with the cache,
  "resume" and "re-run" cost nearly the same. Rejected.
- **Synchronous chunked execution**: bounded by MCP client timeouts;
  collapses on long works. Rejected.
