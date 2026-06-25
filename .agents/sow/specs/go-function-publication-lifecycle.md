# Go Function Publication Lifecycle

## Scope

This spec applies to go.d Functions registered through:

- `collectorapi.Creator.Methods` for module/static methods.
- `collectorapi.Creator.JobMethods` for per-job methods.
- `funcapi.MethodConfig.Available` for module/static method availability.

It does not define Netdata Cloud discovery behavior beyond the Agent-side
publication stream emitted by go.d.

## Module And Static Methods

- Module/static methods MUST be declared through `collectorapi.Creator.Methods`.
- funcctl owns module/static Function publication. Collectors MUST NOT publish
  their module/static Functions by calling `funcctl`, `dyncfg.Responder`,
  `netdataapi`, or plugins.d `FUNCTION` commands directly.
- A module/static Function MUST NOT be published until the module has at least
  one running provider job.
- `Available == nil` means the method is available whenever the module has a
  running provider job.
- `Available != nil` is authoritative availability state:
  - funcctl MUST publish the method when the module has a running provider job
    and `Available()` returns true;
  - funcctl MUST withdraw the method when `Available()` later returns false;
  - funcctl MUST withdraw the method when the last running provider job stops.
- funcctl MAY recheck module/static methods while jobs are running. jobmgr
  currently performs this recheck from the running-job tick loop and immediately
  after module job start/stop.
- Rechecks MUST reuse the normal funcctl publication path so public names,
  aliases, tags, `RequireCloud`, `AgentWide`, handler wiring, and collision
  behavior stay consistent.
- Direct Function execution MUST fail with `404 unknown function` for withdrawn
  module/static methods, even if the module's method route is still declared.

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
- This spec does not add late-publication behavior to job methods.

## Validation Guidance

- Tests for a module/static method with `Available` SHOULD cover:
  - not published at module registration or job start while unavailable;
  - published after a running-job recheck when availability turns true;
  - not duplicated after publication;
  - withdrawn when availability later turns false;
  - republished if availability later returns true again;
  - direct execution of a withdrawn Function returns 404.
- Tests for module/static provider lifecycle SHOULD cover:
  - no publication at module registration before any job starts;
  - first running provider job publishes available methods;
  - stopping one provider job does not withdraw while another provider still
    runs;
  - stopping the last provider job withdraws the module/static methods.
- Race tests SHOULD cover concurrent running-job rechecks and job lifecycle
  hooks for modules with availability-gated methods.
