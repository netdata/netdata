# Function framework boundary

`plugin/framework/functions` contains the process-facing Function protocol boundary shared by Job Manager and service
discovery. It does not own a dispatcher, worker pool, scheduler, invocation ledger, or runtime component.

## Retained contracts

- `Function` is the compatibility value used by framework registries and handlers.
- `Registry` publishes and withdraws prefix Function handlers for service discovery.
- `InputCapsule` is the process-fixed bounded parser for `FUNCTION`, `FUNCTION_PAYLOAD`, `FUNCTION_CANCEL`, and `QUIT`
  input.
- `Consumer` receives immutable parsed calls and control events.
- `TerminalFinalizer` is the terminal-response callback contract.
- `BuildJSONPayload` is a passive helper.

The active Job Manager generation owns routing, UID admission, cancellation, deadlines, ordering, task execution,
terminal-once behavior, and runtime metrics. Those responsibilities live under `plugin/agent/jobmgr`.

## Input ownership

`InputCapsule` owns only the payload currently being parsed. A complete call is transferred to `Consumer`; a partial
payload stays process-owned across run rotation until it is drained or discarded by the process ingress protocol.

The capsule:

- bounds command lines, payload bodies, and nesting-independent input storage;
- grows its one process-owned payload geometrically up to the body limit;
- rejects malformed or oversized input without transferring partial data;
- treats cancellation and quit as control events;
- never invokes collector code directly.

## Concurrency

This package imposes no process-wide limit on concurrent Function execution. The Job Manager task supervisor maintains
separate framework-control and generic-Function scheduling classes. Collector implementations may serialize their own
handlers when their internal state requires it.
