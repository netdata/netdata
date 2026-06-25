# Go Function Publication Lifecycle

## Scope

This spec applies to go.d Functions registered through:

- `collectorapi.Creator.Methods` for module/static methods.
- `collectorapi.Creator.JobMethods` for per-job methods.
- `funcapi.MethodConfig.Available` for first-publication gating.

It does not define Netdata Cloud discovery behavior beyond the Agent-side
publication stream emitted by go.d.

## Module And Static Methods

- Module/static methods MUST be declared through `collectorapi.Creator.Methods`.
- funcctl owns module/static Function publication. Collectors MUST NOT publish
  their module/static Functions by calling `funcctl`, `dyncfg.Responder`,
  `netdataapi`, or plugins.d `FUNCTION` commands directly.
- `Available == nil` means the method is available for publication.
- `Available != nil` gates first publication only.
- funcctl MAY recheck unpublished module/static methods while jobs are running.
  jobmgr currently performs this recheck from the running-job tick loop.
- Publication is monotonic:
  - once funcctl publishes a module/static Function, later `Available == false`
    results MUST NOT withdraw it;
  - the Function remains published until go.d cleanup or restart;
  - withdrawal-on-empty requires a separate explicit lifecycle design.
- Rechecks MUST reuse the normal funcctl publication path so public names,
  aliases, tags, `RequireCloud`, `MethodScope`, handler wiring, and collision
  behavior stay consistent.

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

- Tests for a module/static method with `Available` SHOULD cover:
  - not published at module registration or job start while unavailable;
  - published after a running-job recheck when availability turns true;
  - not duplicated after publication;
  - not withdrawn when availability later turns false.
- Race tests SHOULD cover concurrent running-job rechecks and job lifecycle
  hooks for modules with availability-gated methods.
