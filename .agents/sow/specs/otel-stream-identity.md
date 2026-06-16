# OTel log-stream identity contract

Scope: how an OpenTelemetry log "stream" is identified, hashed, partitioned into
files, and filtered at query time across the WAL/SFST logs pipeline.

## Identity

- A log stream is the pair `(service.namespace, service.name)`, represented by
  `file_registry::ServiceStream { namespace: String, name: String }`. This is the
  canonical identifier used by the ingestor, WAL/SFST registries, catalog,
  indexer, and query planner.
- **Absent equals empty.** A missing attribute and an empty-string attribute
  identify the **same** stream. OpenTelemetry mandates this for `service.namespace`
  ("a zero-length namespace string is assumed equal to an unspecified namespace");
  the same collapse is applied symmetrically to both fields because `ServiceStream`
  stores absent and empty alike as `""` and cannot distinguish them.
- Identity is otherwise byte-exact: no case-folding or trimming, so `("Prod","api")`
  and `("prod","api")` are distinct streams.

## Hashing

- `ServiceStream::ns_hash(&self) -> u64` is the **canonical identity-layer hash**.
  It maps an empty field to `None` and delegates to the low-level primitive
  `file_registry::compute_ns_hash(Option<&str>, Option<&str>)`. An all-empty stream
  therefore hashes to the `0` "no attribution" sentinel.
- `compute_ns_hash` is the primitive that distinguishes `None` from `Some("")`.
  It MUST NOT be called with that distinction at the identity layer — every
  stream→hash derivation goes through `ServiceStream::ns_hash`. Calling the
  primitive with `Some("")` for an absent field is exactly the divergence this
  contract removes.
- The `ns_hash` names WAL/SFST files (`FileId.ns_hash`) and is embedded in remote
  object keys via `FileId`.

## Consumers and consistency

- **Ingestor** (`otel-ingestor::logs_service`): `extract_stream` returns a
  `ServiceStream` (absent→""); file naming and the per-`(tenant, ns_hash)` canonical
  collision table key off `ServiceStream` / `ServiceStream::ns_hash`. A literal-empty
  and an absent field land in one partition.
- **WAL registry** (`wal::Registry::candidates`): filters by
  `q.stream.ns_hash()` against `FileId.ns_hash`.
- **SFST registry** (`sfst::Registry::candidates`): filters by exact `ServiceStream`
  equality against the file's `Summary.stream` (collapse-safe; no hash recomputation).
- **Indexer** (`sfst-indexer::row_index::service_stream`): resolves the file's single
  stream from interned `service.namespace=`/`service.name=` entries, defaulting a
  missing key to `""` — already collapsed.
- These two filter mechanisms (WAL by hash, SFST by `==`) agree because both treat
  absent and empty as one stream.

## Persisted formats (unchanged)

This contract changes no on-disk or wire format. `ServiceStream` is stored as String
pairs everywhere it is persisted — SFST `SUMR` (`sfst` `VERSION = 5`), catalog
(`otel-catalog` `FORMAT_VERSION`), remote object keys (`otel-ledger`
`SCHEMA_VERSION = "v1"`). Ferryboat IPC carries `FileId` only, not stream identity.
No version bump is required.

## Compatibility note

Under the empty→None rule, an absent-namespace file (the common case) keeps the
exact `ns_hash` the ingestor already wrote (`compute_ns_hash(None, name)`), so no
migration is needed for it. Only a file produced from a sender that set a *literal*
empty-string namespace changes hash (it now merges into the no-namespace stream).

The two filter tiers treat a pre-existing literal-empty file differently:

- **WAL tier (hash-based):** an unindexed file still carries the old
  `compute_ns_hash(Some(""), name)` in its `FileId`, which the new query hash
  `compute_ns_hash(None, name)` misses. This window is bounded — it ends when the
  file is compacted into an SFST.
- **SFST tier (equality-based):** once indexed, the file's `summary.stream`
  collapses to `ServiceStream("", name)` — the indexer resolves both a missing
  key and a literal-empty `service.namespace=` interner value to `""` — and the
  equality filter matches it normally.

This is accepted: OTel logs were always experimental, the literal-empty case is
rare, and the production query path does not stream-filter today (see below).
When the deferred stream-selector follow-up makes the filter non-dormant,
re-evaluate this transient WAL-tier window.

## Stream filter is dormant in the live agent function

The live `otel-logs` handler (`otel-ledger::ledger::rpc::handler`) builds its
`file_registry::Query` with `stream: None`, so production reads scan all candidate
files and never exercise the stream filter. A stream selector (request param →
`LogsQuery` → `Query.stream`, plus a stream-enumeration API and UI) is deferred
follow-up feature work. The reachable exercisers of the filter today are the
registry candidate tests (`wal::Registry::candidates`, `sfst::Registry::candidates`,
`otel-catalog::Catalog::find`, `otel-ledger::query`) and the `sfsq` query library
(whose `LogsQuery` does not yet expose a stream field). An offline `sfsq-cli` that
filters by stream is a separate, forthcoming tool (developed outside this branch);
it will consume `ServiceStream::ns_hash` rather than recompute the hash. This
contract makes the filter correct for when the selector and that CLI ship.
