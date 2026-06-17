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
- **Query filter** (`file_registry::Query`): the stream filter is a **set of
  `ns_hash` values** — `stream_hashes: Vec<u64>`. An empty set matches every
  stream; a non-empty set keeps files whose stream hashes to a member.
  `Query::matches_stream(ns_hash)` is the single empty=all membership predicate
  every source's `candidates` shares.
- **WAL registry** (`wal::Registry::candidates`): membership of `FileId.ns_hash`
  in `stream_hashes`.
- **SFST registry** (`sfst::Registry::candidates`): membership of
  `f.summary.stream.ns_hash()` in `stream_hashes`. Hash membership (not the prior
  `ServiceStream` equality) is safe under the ingestor's per-`(tenant, ns_hash)`
  collision invariant — one stream per hash within a tenant — and is collapse-safe
  because `ns_hash` already maps absent and empty `service.namespace` together.
- **Catalog** (`otel-catalog::Catalog::find` / `Registry::candidates`): membership
  of `e.stream.ns_hash()`, identically.
- **Indexer** (`sfst-indexer::row_index::service_stream`): resolves the file's single
  stream from interned `service.namespace=`/`service.name=` entries, defaulting a
  missing key to `""` — already collapsed.
- All three filter tiers (WAL, SFST, catalog) now match by `ns_hash` membership, so
  they agree by construction; absent and empty collapse into one stream at the hash.

## Persisted formats

`ServiceStream` is stored as String pairs in the SFST `SUMR` (`sfst` `VERSION = 5`),
the catalog (`otel-catalog` `FORMAT_VERSION`), and remote object keys (`otel-ledger`
`SCHEMA_VERSION = "v1"`) — none of those changed.

The **WAL header records the stream** (so an unsealed WAL can be named without
decoding frames — see the stream selector). This bumped `wal` `FORMAT_VERSION` 1→2:
the header carries the `(namespace, name)` as two length-prefixed UTF-8 fields, each
capped at `MAX_STREAM_FIELD_BYTES = 256` and truncated on a char boundary for display
only (the `ns_hash` partition key is unaffected). There is **no v1 back-compat** —
`FileHeader::from_bytes` hard-rejects v1, so an unsealed v1 WAL is lost on upgrade
(accepted: the feature is experimental and WAL files are short-lived). `recover()`
reads the stream from the header on restart; `wal::Registry::File` carries it.

The writer→ledger **ferryboat IPC** also carries the stream: `FileEvent::Created`
gained a `ServiceStream` field. Ferryboat has no version field, but the writer and
ledger are separate worker processes of the **same `self_exe`**, so they never skew
— no wire-compat constraint.

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
- **SFST tier (hash-membership):** once indexed, the file's `summary.stream`
  collapses to `ServiceStream("", name)` — the indexer resolves both a missing
  key and a literal-empty `service.namespace=` interner value to `""` — so
  `summary.stream.ns_hash()` is `compute_ns_hash(None, name)`, the same hash the
  query derives, and membership matches it normally.

This is accepted: OTel logs were always experimental and the literal-empty case
is rare. The stream selector (below) is now live, so this transient WAL-tier
window can hide a stale literal-empty file from a stream-filtered query until it
compacts into an SFST — still bounded and rare, not a correctness regression for
the common absent-namespace file.

## Stream selector in the live agent function

The live `otel-logs` handler (`otel-ledger::ledger::rpc::handler`) exposes a
**stream selector** modeled on the systemd-journal `__logs_sources` control:

- **Advertise.** Every data response carries one `required_params` entry — a
  `MultiSelection` with `id = "__streams"` (`STREAM_SELECTION_PARAM`), name
  "Services" — whose options are the tenant's streams. Options are enumerated
  **window-independent** from `TenantRegistries::enumerate_streams` (SFST
  summaries + WAL `File.stream`, deduped by `ns_hash`, SFST-wins by seq), so a
  stream that exists only as an unsealed WAL is listed too (this is why Stage A
  records the stream in the WAL header). Each option: `id` = `ns_hash` as
  16-digit hex, `name` = `namespace/name` (absent namespace shown as
  `name • (no namespace)`), `pill` = total size (an active WAL uses
  `valid_up_to` since `File.size` lands only on close), `info` = file count +
  span, and `defaultSelected: true` on **every** option.
- **Default = all.** The UI auto-selects only the first option when none sets
  `defaultSelected`, and the control is mandatory (blocks the query until a
  non-empty selection). Marking every option `defaultSelected` keeps the default
  view spanning all streams (the prior no-selector behavior). A tenant with no
  streams advertises no control (`required_params: []`).
- **Honor.** The handler removes the reserved `__streams` key from
  `selections` **before** building the engine `LogsQuery` (so the engine never
  treats it as a row facet), decodes the picks (`hex → u64`, unparseable skipped)
  into `file_registry::Query::stream_hashes`, and that set prunes which files the
  query opens. Empty set = all streams.

The offline `sfsq-cli` (`discover.rs`) applies the same `ns_hash` filter from its
`--namespace`/`--name` flags (one-element `stream_hashes`); it consumes
`ServiceStream::ns_hash` rather than recomputing the hash. The registry candidate
tests (`wal`/`sfst`/`otel-catalog`/`otel-ledger::query`) and an `otel-ledger`
adapter/registry test suite exercise the filter and the enumeration directly.
