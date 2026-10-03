# Native DEM ownership

The command owns SQLite and the plugin-wide retention service. It closes SQLite only after `agenthost.Result` reports
`Err == nil` and `ExitRequired == false`. An error or recovery requiring process exit may leave consumers alive; process
exit owns their handles. Process-service finalizers run before job retirement and cannot close resources used by jobs.

The framework owns desired configuration, preflight, scheduling, admission, retries, status and DynCfg. The constructor
injects a runtime hub into collectors and a domain-only source into process Functions. The hub contains actual admitted
site registrations and receiver availability. It has no desired config mirror, scheduler or job status.

Receiver `Init` / `Check` validate and prepare without binding. `Run` binds the listener, publishes availability and
signals readiness. Each site initializes its immutable policy, then independently admits its aggregator, route, redactor
and runtime generation. Site readiness does not depend on receiver readiness. Receiver replacement preserves site
aggregation and pending investigation/export queues.

An HTTP request acquires one exact site registration containing both policy and sinks. Bootstrap, preflight, demo and
beacon endpoints use this same admission. Retirement removes future admission, cancels admitted request contexts and
joins their leases. Cancellation interrupts socket reads through a response-controller deadline before closing bodies;
closing a net/http request body alone can wait behind a stalled read. Cancellation callbacks join before leases release.
Function reads lease domain state only while copying snapshots, and historical queries use caller-aware SQL contexts.

A retiring site joins its HTTP and Function readers before cancelling the history/export worker context. Export queues
drain using one shared fixed shutdown budget, and accepted history contributions flush under a detached five-second SQL
budget. A cancelled steady-state SQL batch stays pending and joins the final transaction; successful commits and
unrelated database failures are never replayed. Session contributions are deltas, allowing persistence to accumulate across site generations and session cache
replacement. Publication uses typed metrix snapshots and static chart templates, with no V1 map bridge.

Stock native health templates own alert policy and attach independently to each site's charts. Site jobs publish
measurements only; they do not write health configuration, invoke health reload or recover generated files. The command
creates its history directory as the service account under the existing writable Agent state root.

Reachability probes use an explicitly configured site or receiver public URL first. With neither configured, they probe
the address learned from trusted proxy requests directly; setup Functions advertise it after successful confirmation.
An explicit receiver public URL takes effect immediately, including when an older learned address was confirmed.

Receiver and OTLP TLS preparation use the shared context-aware TLS helper, so native preflight cancellation reaches
credential-file reads. Site redaction covers stored/exported payloads and remote OTLP diagnostic messages.
