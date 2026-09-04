# Mutating And Stateful Collectors

Load this when a collector writes or deletes remote objects, or persists state across cycles or restarts. A collector
that does neither records "no durable remote ownership, no durable local state" in its design note and stops here; it
needs no journal, queue, lock, or recovery analysis. A collector with durable local state but no remote mutation (for
example a receiver that persists its own protocol state) reads §3 and §5 only. Rules use the format When / Do / Don't /
Evidence / Boundary.

## 1. Mutation Ownership

**When:** the collector creates, modifies, or deletes anything outside its process. **Do:** name every remote object
the collector owns and the identity that owns it: the stable job identity (`pluginconfig.RegistryUniqueID()` plus the
job name), never display labels, credentials, or endpoints. Decide which concurrent owners exist: the same job's old
and new runtime (protect with a per-owner lock) versus different jobs (independent by default). State what a renamed
job inherits: nothing automatically; draining old work requires restoring the old identity and configuration.
**Don't:** let mutable labels join the durable identity or safety fingerprint (correcting a display label must not
strand owned state); scan other owners' state to decide whether this owner may run (Architecture Gate). **Evidence:**
the identity construction in code and the fingerprint fields listed in the note. **Boundary:** deliberate writers under
the reserved namespace and root or same-UID tampering with a private state directory are outside the supported
operating model; state that boundary instead of defending against it.

## 2. Failure Boundaries

**When:** designing any mutation sequence. **Do:** fill one row per mutation before building the engine: state before,
action, ambiguous outcome and how it is retained, what is persisted before and after, the permitted next action, and
the behavior after a restart at that point. Order recovery correctly: acquire the owner lock, then load the
authoritative state (a snapshot loaded before the lock is stale once a successor took ownership); retain uncertainty
explicitly (an ambiguous PUT or an absence observed too soon is not resolved by forgetting); retire an entry durably
before its cleanup deletion (a DELETE that is itself part of the measurement happens while the probe is still active);
after a failed publication, restore the last authoritative state rather than continuing from rolled-back memory.
**Don't:** move an entry to cleanup in memory while its persistence failed and then delete the remote object; enter
exact cleanup with identities (version or marker IDs) known only in memory. **Evidence:** a test per row that reaches
the state through real transitions and asserts the next action. **Boundary:** "save before and after the operation" is
too vague to count as a design; ordinary read-only collectors have no rows.

## 3. Persistence Semantics

**When:** the collector writes state to disk. **Do:** treat persistence as more than success or failure: creating a
directory without durably recording its parent leaves a first-use crash gap; a second owner can observe a directory
before the first finished syncing it; rename or unlink can become visible before a directory sync fails. A failed
publication is fail-closed: stop all remote mutation in the same collection or shutdown call, and keep next-call
telemetry truthful (early returns must not clear backlog or backpressure signals). **Don't:** describe a telemetry
defect as data loss when remote mutations are already stopped. **Evidence:** tests for failed publication in the same
call and the next call. **Boundary:** host or power-loss durability on a platform without the needed primitives is an
accepted, documented limitation, not something to claim silently.

## 4. Cleanup Versus Measurement

**When:** the collector must clean up what it created. **Do:** separate active probing from unfinished cleanup so
housekeeping never creates a monitoring blind period; bound cleanup work per call and prove progress separately (a
persistently failing front entry must not starve a recoverable later one: persisted rotation, or a stated strict
dependency); make backlog and backpressure visible as explicit state, not as forgotten ownership. Shutdown cleanup
follows the fixed-budget rule in the V2 skill. **Don't:** shorten a safety interval to hide a stall, backdate quarantine
to creation time, or use extreme chart-lifetime settings as a substitute for explicit result state. **Evidence:** a
trace with a blocked item ahead of a recoverable one; a test that measurement continues while cleanup is pending.
**Boundary:** strict ordering is legitimate when it is a real dependency; say so instead of adding round-robin.

## 5. Preconditions And Identities

**When:** a dangerous operation depends on a provider setting or an identity. **Do:** decide which assumptions are
stable for the job's lifetime and which authorize dangerous work and must be re-checked at the operation (bucket
versioning, replication policy scope on the actual key, not on an approximation of it); use response metadata to keep
ownership when the state changed underneath; canonicalize identities before comparing them (endpoint spellings) and
take runtime paths from the runtime configuration (the varlib path), not from assumptions. **Don't:** cache a safety
precondition forever because the endpoint did not change; approximate an authorization input and tighten the
approximation repeatedly. **Evidence:** the precondition list in the note with "checked at" for each. **Boundary:**
re-checking everything every cycle is not required; only what authorizes mutation.

## 6. Tests Specific To Mutating Collectors

Reach every state through real construction and transitions; do not populate private state and call the result a
recovery test. Cover: a crash between each persisted step; restart with a stale in-memory snapshot; an ambiguous
provider response; a second unrelated job on the same agent (it must be unaffected); a persistently blocked backlog
item ahead of a recoverable one; failed publication in the same call and the next; and, only when durable ownership is
in scope, a renamed owner. The oracle rules for fakes and real-path construction are in the V2 skill's Tests section and
apply here in full.
