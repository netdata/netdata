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

Collector and process provider declarations, raw input versus response passthrough, and managed metadata are documented
in [`pkg/funcapi`](../../../pkg/funcapi/README.md).

## Wire field identity

The daemon sanitizes command and source text, then emits it literally inside double quotes. These fields are not Go
or JSON string literals: backslashes do not escape characters or the closing quote. Embedded double quotes are
replaced with apostrophes by the daemon before framing.

Command words are separated only by ASCII whitespace. Unicode whitespace, including NBSP, remains in the argument
so the configuration owner can reject the original invalid name. Literal escapes such as `d\x62` must likewise reach
validation unchanged. Parsing must never turn either into a different configuration's identity. Payload bytes are
independent of command tokenization. Wire test fixtures must use the daemon's literal quoting rather than Go `%q`.

## Payload framing

The existing wire protocol ends a payload at an exact `FUNCTION_PAYLOAD_END` line. Until that marker, all other
lines are body data, including valid `FUNCTION`, `FUNCTION_PAYLOAD`, `FUNCTION_CANCEL`, `FUNCTION_PROGRESS`, and
`QUIT` commands. Commands cannot interleave with a payload. Rejected payload headers with an identifiable UID and
oversized bodies are drained through the marker before reporting their rejection and reading the next command.
An unterminated payload is a protocol error; a command-looking body line cannot resynchronize the parser.

This preserves the existing sentinel framing and LF/CRLF separator handling. It does not make the protocol an
arbitrary-byte transport: a body line equal to the end marker still terminates the payload. Removing that ambiguity
requires a coordinated producer/consumer protocol change.

## Input ownership

`InputCapsule` owns only the payload currently being parsed. A complete call is transferred to `Consumer`; a partial
payload stays process-owned across run rotation until it is drained or discarded by the process ingress protocol.

The capsule:

- bounds command lines, payload bodies, and nesting-independent input storage;
- grows its one process-owned payload geometrically up to the body limit;
- rejects malformed or oversized input without transferring partial data;
- treats cancellation and quit as control events only outside a payload;
- never invokes collector code directly.

## Concurrency

This package imposes no process-wide limit on concurrent Function execution. The Job Manager task supervisor maintains
separate framework-control and generic-Function scheduling classes. Collector implementations may serialize their own
handlers when their internal state requires it.
