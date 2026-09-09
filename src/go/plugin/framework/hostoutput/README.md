# Host publication

`Publisher` owns host metadata publication for one plugin output stream. The process constructs it beside
`lifecycle.FrameOwner`; an activated manager run binds its configured-vnode definition lookup. Configuration
construction and candidate probing do not change active authority.

## Metadata ownership

Jobs reference configured vnodes by name. The runtime resolves that name to a collection snapshot and routing GUID.
Every ordinary output batch targeting that GUID, including generated V2 host scopes, uses the configured host
definition. Job labels remain chart labels; they do not contribute host labels.

Definitions are immutable and pre-encoded outside output admission. `Publication.Build` selects the current
configured definition under the existing exclusive frame lease. It prepends a definition only when needed, without
changing the routing GUID of the prepared collection. `Publication.Commit` records successful publication after
the complete write and chart transaction succeed. Failed writes and failed state commits abort publication state.

Without configured authority, each contributor retains its existing first/changed-metadata behavior. Different
unchanged generated definitions do not alternate on every cycle. Conflict diagnostics share the publication owner
records and retain at most 64 reported metadata states per live GUID.

## Lifetime and removal

An opaque `Owner` belongs to a particular runtime/scope incarnation and GUID. Successful owners stay registered
through quiet cycles and retry periods. Removing configuration drops authority but retains publication provenance
while contributors remain: a generated contributor quiet throughout adoption and removal restores its fallback on
its next ordinary emission. No permanent GUID tombstones or inventory scans are needed.

Scope retirement and terminal job cleanup release their own handles. Old-runtime cleanup cannot release a
replacement runtime's handle. Generation admission spans frame building, writing, and settlement. Accepted cleanup
uses its separate process-lifetime capability; final process fencing rejects late cleanup.

## Cleanup and staleness

Cleanup never publishes a host definition. It obsoletes the job's own charts unless the effective vnode timeout
suppresses that output. Suppression avoids the `HOST` command refreshing host activity. Consequently, a stopped
job's charts are not explicitly retired while another job keeps the vnode active. Global job-status charts remain
independent of vnode cleanup suppression.

Configured `stale_after` is a duration in whole seconds, with a maximum of 4294967295 seconds from the plugins.d
protocol's unsigned 32-bit timeout. Omission preserves legacy label configuration; explicit zero disables the
timeout. An explicit value overrides `_node_stale_after_seconds`, which remains the wire representation consumed
by Netdata. The existing C parser's reset-before-copy defect is a separate fix; Go propagation tests do not prove
end-to-end host staleness until that parser ordering is corrected.

## Collector roles

`VirtualNode()` supplies a collector-generated default host. V2 collectors implementing
`collectorapi.ConfiguredVnodeConsumer` receive an
owned configured snapshot initially before Init/Check, and on changed revisions synchronously before Collect.
Nagios uses this consumer for host macros. Configuration commits never mutate collector state asynchronously.
A collection already in progress retains its original target; the next collection receives the updated snapshot.

## V1 output settlement

V1 collectors retain their desired chart objects, including pending redefinitions and removals. The runtime
renders through a journal of changed fields and commits creation flags, caches, priorities, pruning and output
timing only after the frame is accepted and written. Rejected or failed output leaves collector intent and the
last committed host state intact. Chart pointers remain stable for collectors that retain them.

Cleanup uses an inventory of successfully emitted `CHART` definitions, separate from mutable collector objects.
It applies create/obsolete operations in wire order and removes a definition only when its obsolete command was
emitted. Collector cleanup can mutate the desired charts without changing which definitions are retired. Global
self charts have their own inventory and remain independent of vnode staleness suppression.

## Cost and scope

Steady-state publication uses a GUID lookup and constant-time provenance checks per host batch. Metadata preparation
and encoding happen on changes outside frame admission. Diagnostic scans occur only for changed unconfigured
metadata. Retained publication state scales with live contributors, not all hosts ever seen.

V1 rendering remains O(charts + visited dimensions + bytes). The reusable journal retains only changed fields and
emitted definitions; steady cycles do not copy the chart graph or scan the cleanup inventory. Inventory updates
are O(emitted definitions), storage follows live emitted charts, and terminal cleanup sorts their wire IDs.

This authority covers one plugin process. Other plugin processes or streaming writers can address the same host
in Netdata; agent-wide exclusive metadata ownership would require a separate protocol contract.
