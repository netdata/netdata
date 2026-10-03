# Native DEM ownership

The command owns the investigation journal and the plugin-wide retention service. It closes it only after `agenthost.Result` reports
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
Function reads lease domain state only while copying snapshots, and historical queries own independent journal snapshots and check caller cancellation between files and rows.

A retiring site joins its HTTP and Function readers before cancelling the history/export worker context. Export queues
drain using one shared fixed shutdown budget, and accepted history events flush under one detached five-second
admission budget. Every event is self-contained: there are no session/group parent records or cumulative deltas.
A cancelled admission leaves only unattempted entries pending for final drain; attempted appends are never replayed,
including when an error or cancellation races them. Appends, sync, snapshot construction and retention serialize SDK
Log access; the SDK disk operation itself is not interruptible. A non-quiescent exit uses the existing host fail-stop
boundary. Snapshot scans release writer admission and copy SDK payloads before their borrowed lifetime ends.

History filters select saved time; session timelines and retained activity spans use original observation time.
Retained counts are derived from selected events, not reconstructed lifetime totals. Error fingerprint overview is
linear in matched retained events and fingerprints; selected fingerprint details additionally retain distinct session,
page and browser sets for that group. Queries materialize SDK entry offsets but no second complete event list.
Query failures return errors instead of silently labeling corrupt or quarantined history as complete. Damaged
`.journal~` files remain for operator investigation and are outside the SDK retention policy.

One process worker archives idle history, reopens lazily and applies the plugin-wide age/committed-byte policy even
when all sites are disabled. Failed sweeps retry after five seconds; the store owns the reopened lazy Log before
enforcing retention, and the next append creates its active file. Whole archives expire; the active file and filesystem
preallocation can exceed the configured committed-byte target. Neither retention nor history scans hold a live site lease across disk work. Publication uses
typed metrix snapshots and static chart templates, with no V1 map bridge.

Stock native health templates own alert policy and attach independently to each site's charts. Site jobs publish
measurements only; they do not write health configuration, invoke health reload or recover generated files. The command
creates its history directory as the service account under the existing writable Agent state root.

Reachability probes use an explicitly configured site or receiver public URL first. With neither configured, they probe
the address learned from trusted proxy requests directly; setup Functions advertise it after successful confirmation.
An explicit receiver public URL takes effect immediately, including when an older learned address was confirmed.

Receiver and OTLP TLS preparation use the shared context-aware TLS helper, so native preflight cancellation reaches
credential-file reads. Site redaction covers stored/exported payloads and remote OTLP diagnostic messages.
