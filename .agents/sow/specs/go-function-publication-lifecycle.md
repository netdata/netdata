# Go Function Publication Lifecycle

## Scope

This spec applies to go.d Functions registered through:

- `collectorapi.Creator.SharedFunctions` for static shared job-backed
  Functions.
- `collectorapi.Creator.AgentFunctions` for static true-agent Functions.
- `collectorapi.Creator.JobMethods` for per-job methods.
- `funcapi.FunctionConfig.Available` for true-agent Function publication
  gating.
- `collectorapi.FunctionAvailability` for running-job availability of
  job-backed Functions.

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
- `funcapi.FunctionConfig.Available` MUST NOT be used for `SharedFunctions`.
  Shared Functions are job-backed, so their availability is derived from
  running jobs.
- Shared Function publication is derived from available running jobs:
  - publish after reconciliation observes the first available running job;
  - expose only available running jobs in `__job`;
  - withdraw when the last available running job stops or becomes unavailable;
  - reject unavailable explicit job requests before dispatching to collector
    code.
- Job start does not synchronously publish shared Functions. A short-lived job
  that starts and stops before reconciliation may never publish the shared
  Function, and therefore may not emit a matching withdrawal for that Function.
- Function withdrawal is emitted through `FUNCTION_DEL GLOBAL`. Parents that do
  not advertise `STREAM_CAP_FUNCTION_DEL` can retain stale streamed Function
  entries even after the child unregisters them locally.
- Shared single-instance Functions still use `SharedFunctions`.
  `InstancePolicySingle` removes the public `__job` selector but does not make
  the Function true-agent; publication still depends on the canonical singleton
  job being running and available.
- funcctl MAY recheck shared Function availability while jobs are running. jobmgr
  currently performs this recheck from the running-job tick loop and on job
  stop; agent/process-backed Function availability may still be checked at job
  start.
- `AgentFunctions` are true-agent declarations and are not processed by shared
  job availability reconciliation.
- Rechecks MUST reuse the normal funcctl publication path so public names,
  aliases, tags, `RequireCloud`, handler wiring, and collision behavior stay
  consistent.

## Job-Backed Function Availability Contract

- Collectors MAY implement `collectorapi.FunctionAvailability` when a
  running job can serve only some shared Functions, or when a shared Function
  should appear only after collector-owned runtime state is ready.
- `FunctionAvailable(functionID)` MUST be cheap and non-blocking.
- If a collector does not implement `FunctionAvailability`, every running job is
  available for every shared Function declared by the module.
- Availability is pull-based. Collectors update their own state from normal
  runtime paths; funcctl reads that state during reconciliation.
- Availability changes are reflected after the next reconcile pass. Rapidly
  flapping availability can produce repeated `FUNCTION GLOBAL` /
  `FUNCTION_DEL GLOBAL` output and SHOULD be avoided.

## Agent Function `Available` Predicate Contract

- `Available` applies to `AgentFunctions`.
- `Available` MUST be cheap and non-blocking.
- `Available` MUST NOT perform network I/O, expensive rendering, or long-running
  collection work.
- Collectors that need runtime readiness SHOULD update collector-owned state
  from their normal collection or refresh path. `Available` SHOULD only read
  that state.
- `Available` SHOULD match the Function's user-visible not-ready boundary for
  true-agent Functions.

## Job Methods

- Per-job methods remain tied to job lifecycle.
- `collectorapi.Creator.JobMethods` methods are registered for a job on job
  start and unregistered on job stop.
- This spec does not add late-publication or monotonic behavior to job methods.

## Validation Guidance

- Tests for shared Function availability SHOULD
  cover:
  - not published at module registration or job start while no running job is
    available;
  - published after a running-job recheck when a job becomes available;
  - not synchronously published on job start;
  - not duplicated after publication;
  - withdrawn after the last available running job stops or becomes unavailable;
  - re-published with a new generation when availability returns;
  - stale pre-withdrawal handlers return 404;
  - `__job` contains only available running jobs.
- Tests for `AgentFunctions` SHOULD cover that shared availability reconciliation
  does not withdraw true-agent Functions.
- Race tests SHOULD cover concurrent running-job rechecks and job lifecycle
  hooks for modules with availability-gated Functions.
