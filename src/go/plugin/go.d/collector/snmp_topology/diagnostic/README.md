# SNMP topology diagnostic v1

`netdata.snmp-topology-diagnostic/v1` is a storage-neutral, content-addressed graph. It is the diagnostic and replay
contract; a future archive is only one container for the same manifest and members.

## Compatibility

- A member is identified by `(namespace, kind, schema, canonicalization, logical length, SHA-256)`.
- The digest is domain-separated by all identity fields and the payload length. It proves byte identity and graph
  integrity.
- Known member schemas and capability closures are immutable. A semantic or required-field change uses a new member
  schema or capability revision.
- Readers validate the raw integrity of every inventoried member. They may skip an unknown member outside the requested
  capability, but MUST reject an unknown member inside its requested closure.
- A wire graph contains only `ContentRef` values. A process-local `MemberHandle` is never serializable.

## Canonical JSON

`netdata.canonical-json/v1` is the exact compact byte sequence emitted by Go `encoding/json` for the declared DTO. DTOs
MUST avoid floating-point values. Map keys use `encoding/json` ordering; semantically ordered slices MUST already be in
their production order. A reader rejects unknown fields, duplicate object keys, invalid UTF-8, trailing values, and any
byte sequence that does not round-trip to the same canonical bytes.

## Capabilities and inventories

- A manifest inventories every member and immutable capability root.
- Each capability and section has one terminal state: `success`, `empty`, `not_applicable`, `failed`, or `incomplete`.
- A capability root inventories its sections. Requiredness is defined by that immutable root revision, never by a global
  per-member optional flag.
- Deterministic shard coordinates bind the capture/registration owner, section, phase/profile coordinates, shard
  index/count, and exact half-open record range. Requested-capability validation rejects gaps, overlaps, duplicates, and
  coordinate changes.

V1 currently defines these immutable capability closures:

- `semantic_replay@1` inventories the device input, ordered profile identity evidence, ordered semantic shards, and exact
  observation. Replay must reproduce the captured observation exactly.
- `refresh_sweep@1` inventories the complete registration-ordered refresh plan, each registration's selection, target
  resolution, and terminal outcome, plus the publication result. A published sweep owns its resulting generation; a
  canceled or panicked sweep explicitly publishes no generation.
- `graph_replay@1` inventories the generation, normalized query, three-state DNS trace, and ordered OUI trace. Replay
  rebuilds `netdata.topology.v1` without ambient dependencies. Production-shaped tests compare it with the live output.
- `capture_accounting@1` inventories one coalesced capture-gap record for attempts rejected before a capture transaction
  could begin.

Known v1 member kinds are `capability_root`, `capture_gap`, `semantic_device`, `semantic_profile`, `semantic_shard`,
`observation`, `refresh_sweep`, `generation`, `graph_query`, `dns_trace`, and `oui_trace`.
`schema-v1.json` validates each complete top-level member document; capability closures add the cross-member ownership,
ordering, reference, and count rules that JSON Schema cannot express.

## Recorder and content lifetime

- `Recorder.Begin` is non-blocking. A successful begin reserves the terminal queue position needed by exactly one later
  `Commit` or `Abort`.
- `AddOwned` transfers one detached immutable DTO and charges its retained capacity before admission. `AddDerivedOwned`
  runs only on the recorder worker after all dependency handles have sealed.
- `MaxMembers` bounds both admitted transaction members and the final unique transitive manifest inventory, including
  its capability root. The minimum valid value is therefore two. A transitive dependency that would exceed the limit
  fails its handle and makes the capability incomplete; the recorder never publishes an oversized manifest.
- `MaxRetainedBytes` accounts for detached DTO ownership transferred directly by one transaction. It does not claim to
  measure the collector's current live projection, encoded bytes, or content already owned by earlier captures. Across
  admitted recorder work, detached DTO ownership is bounded by `QueueCapacity * MaxRetainedBytes`, plus bounded member,
  reference, and queue bookkeeping. Live collection may additionally hold one current-device diagnostic projection;
  whole-process peak-memory measurement and production defaults belong to the storage stage.
- A `MemberHandle` is process-local and resolves exactly once to `sealed(ContentRef)` or `failed`. Device generations
  retain the observation handle required by later generation capture; unchanged devices are referenced without
  re-projecting their rows.
- A capture result's `Members` map contains bytes newly sealed by that transaction. Its manifest can also inventory
  transitive content references sealed by earlier transactions. A sink therefore owns one archive-wide content-addressed
  store; it must not assume each result is a self-contained byte map.
- Queue saturation is represented by bounded, coalesced `capture_gap` records. A capture admitted successfully but later
  aborted or failed sealing is emitted as `incomplete`; it is never silently downgraded to a weaker replay capability.

This layer deliberately supplies no filesystem sink, retention policy, compression, export command, or operator setting.
Those concerns consume this contract; they do not redefine it.

## Semantic and graph replay

Live collection and semantic replay use the same ordered primitive stream: system uptime, main profile tags, topology
rows, BGP outcomes/rows, and ordered VLAN-context outcomes/tags/rows. Semantic shards preserve executed phase, context,
profile, row, and shard order. The profile member is identity evidence only: its definition digest covers the explicit
topology-collection projection, but replay never executes its transforms or treats captured profile text as authority.

Graph replay is closed over the complete published generation and normalized query. DNS records preserve `miss`,
`positive`, and `cached_negative` as distinct ordered results. OUI records preserve every normalized lookup attempt in
order, including unsuccessful candidates before a winner. Replay injects those traces and the recorded publication time;
it does not consult live DNS, embedded OUI data, the profile catalog, SNMP, configuration, the filesystem, or the clock.

Graph replay uses one optional work limiter across L2 normalization, projection, policies, L3 enrichment, focus shaping,
and v1 rendering. The same limiter covers both strict and probable projections. Before each bounded stage executes, it
charges a checked conservative envelope for data-dependent sorts and multiplicative work. Exhaustion is returned through
the normal graph-build error path. Production passes no limiter and therefore does no accounting work.

When no recorder is installed, the collector does not construct diagnostic DTOs, copy topology rows, wrap DNS/OUI
lookups, or run an extra graph/render pass. The production topology and Function result remain authoritative if capture
admission or asynchronous sealing fails.

## Reader policy and sensitivity

Every reader supplies a fully nonzero `ReaderLimits` policy. Limits cover logical/member bytes, member and reference
counts, decoded topology structure, JSON depth/tokens, and replay work. The graph reader enforces logical/member/reference
limits. Member bytes plus JSON string, token, and depth limits are enforced before typed decoding. Schema-specific device,
profile, row, tag, DNS, and OUI counts are checked after that bounded decode. V1 does not maintain a second type-aware
predecode scanner. Operational defaults are not part of v1 validity.

V1 exact artifacts are labelled `restricted`, `exact`, and `sanitized: false`.

## Sensitive inputs

Known credential sources MUST never enter a member: SNMP communities, v3 usernames/authentication/privacy keys, context
strings, security parameters, request strings, raw packets, absolute paths, and opaque error text. DTOs are constructed
from allowlisted fields; runtime collector structs are not serialized. Observed identities, addresses, labels, and profile
content remain restricted data because sanitization is deliberately deferred.
