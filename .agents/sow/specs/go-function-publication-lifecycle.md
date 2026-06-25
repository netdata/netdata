# Go Function Publication Lifecycle

## Scope

This spec applies to go.d Functions registered through:

- `collectorapi.Creator.SharedFunctions` for static shared job-backed
  Functions.
- `collectorapi.Creator.AgentFunctions` for static true-agent Functions.
- `collectorapi.Creator.JobMethods` for per-job methods.
- `funcapi.FunctionConfig.Available` for first-publication gating.

It does not define Netdata Cloud discovery behavior beyond the Agent-side
publication stream emitted by go.d.

## Static Functions

- Static shared job-backed Functions MUST be declared through
  `collectorapi.Creator.SharedFunctions`.
- Static true-agent Functions MUST be declared through
  `collectorapi.Creator.AgentFunctions`.
- funcctl owns static Function publication. Collectors MUST NOT publish
  their static Functions by calling `funcctl`, `dyncfg.Responder`,
  `netdataapi`, or plugins.d `FUNCTION` commands directly.
- `Available == nil` means the method is available for publication.
- `Available != nil` gates first publication only.
- funcctl MAY recheck unpublished static Functions while jobs are running.
  jobmgr currently performs this recheck from the running-job tick loop.
- Publication is monotonic:
  - once funcctl publishes a static Function, later `Available == false`
    results MUST NOT withdraw it;
  - the Function remains published until go.d cleanup or restart;
  - withdrawal-on-empty requires a separate explicit lifecycle design.
- Rechecks MUST reuse the normal funcctl publication path so public names,
  aliases, tags, `RequireCloud`, handler wiring, and collision behavior stay
  consistent.

## `Available` Predicate Contract

- `Available` MUST be cheap and non-blocking.
- `Available` MUST NOT perform network I/O, expensive rendering, or long-running
  collection work.
- Collectors that need runtime readiness SHOULD update collector-owned state
  from their normal collection or refresh path. `Available` SHOULD only read
  that state.
- `Available` SHOULD match the Function's user-visible not-ready boundary. For
  example, a topology Function should become available when it can return
  renderable topology data instead of its normal not-ready response.

## Job Methods

- Per-job methods remain tied to job lifecycle.
- `collectorapi.Creator.JobMethods` methods are registered for a job on job
  start and unregistered on job stop.
- This spec does not add late-publication or monotonic behavior to job methods.

## Validation Guidance

- Tests for a static Function with `Available` SHOULD cover:
  - not published at module registration or job start while unavailable;
  - published after a running-job recheck when availability turns true;
  - not duplicated after publication;
  - not withdrawn when availability later turns false.
- Race tests SHOULD cover concurrent running-job rechecks and job lifecycle
  hooks for modules with availability-gated methods.
