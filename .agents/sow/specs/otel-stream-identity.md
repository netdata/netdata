# OTel log-stream identity contract

Scope: how an OpenTelemetry log "stream" is identified, hashed into the
substrate's opaque partition key, serialized into the substrate's opaque
per-file metadata, partitioned into files, and filtered at query time across the
WAL/SFST logs pipeline.

## Two planes

After the substrate restructure the pipeline is split into two planes:

- **Content plane** (`otel-logs-identity`): owns the OTel-logs notion of "which
  stream a file belongs to" — the `(service.namespace, service.name)` pair — and
  is the *only* place that derives it into the substrate's opaque partition key
  and (de)serializes the display-identity blob.
- **Content-agnostic substrate** (`file-registry`, `wal`, `sfst`,
  `otel-catalog`): carries an opaque partition key `part_key: u64` and an opaque
  `content_meta: Vec<u8>` and **never interprets either**. It selects, ages, and
  accounts for files purely by neutral facts (time range, record count,
  `part_key` membership).

A second signal (e.g. traces) will get its own sibling content-plane crate; the
substrate is reused unchanged.

## Identity (content plane)

- A log stream is the pair `(service.namespace, service.name)`, represented by
  `otel_logs_identity::ServiceStream { namespace: String, name: String }`. This
  is the canonical identifier used by the ingestor (write side) and the query
  layer (display side).
- **Absent equals empty.** A missing attribute and an empty-string attribute
  identify the **same** stream. OpenTelemetry mandates this for `service.namespace`
  ("a zero-length namespace string is assumed equal to an unspecified namespace");
  the same collapse is applied symmetrically to both fields because `ServiceStream`
  stores absent and empty alike as `""` and cannot distinguish them.
- Identity is otherwise byte-exact: no case-folding or trimming, so `("Prod","api")`
  and `("prod","api")` are distinct streams.

## Hashing → partition key

- `ServiceStream::ns_hash(&self) -> u64` is the **canonical identity-layer hash**.
  It maps an empty field to `None` and delegates to the low-level primitive
  `otel_logs_identity::compute_ns_hash(Option<&str>, Option<&str>)`. An all-empty
  stream therefore hashes to the `0` "no attribution" sentinel.
- `compute_ns_hash` is the primitive that distinguishes `None` from `Some("")`.
  It MUST NOT be called with that distinction at the identity layer — every
  stream→hash derivation goes through `ServiceStream::ns_hash`. Calling the
  primitive with `Some("")` for an absent field is exactly the divergence this
  contract removes.
- `otel_logs_identity::part_key(&ServiceStream) -> u64` is the content-plane
  entry point the write side uses to turn a stream into the substrate's opaque
  partition key. For logs it **is** `ServiceStream::ns_hash`; the substrate
  ascribes the result no meaning. The `part_key` names WAL/SFST files
  (`FileId.part_key`) and is embedded in remote object keys via `FileId`.

## Display identity (`content_meta`)

The substrate's opaque `content_meta` blob carries the human-facing identity for
a file. The content plane is its sole codec:

- `otel_logs_identity::encode_content_meta(&ServiceStream) -> Option<Vec<u8>>`
  serializes it; `decode_content_meta` / `decode_content_meta_or_empty` read it
  back. Layout: a 1-byte `CONTENT_META_VERSION` (currently `1`) tag, then each
  field as a little-endian `u16` byte-length prefix followed by its UTF-8 bytes
  (`namespace` then `name`).
- The encoder is **fallible**: a field exceeding `MAX_FIELD_BYTES` (`u16::MAX`)
  returns `None` so an over-long, attacker-controlled identity is rejected at the
  write path rather than panicking. The ingestor drops such a frame.
- The substrate stores these bytes verbatim and never parses them; only the
  content plane does. For the row/record path the query layer decodes them into a
  `StreamId` for display. For the stream selector the substrate folds candidates
  into a neutral `registry::PartitionStat` (carrying the opaque `content_meta`),
  and the logs rpc adapter decodes each into its display-typed `StreamStat` —
  keeping the substrate registry free of the content plane.

## Partition-key authority

The `part_key` is the **single source of truth in the file's `FileId`** (the
filename) — it is not stored anywhere else (not in the WAL header, the SFST
`SUMR` summary, or the catalog entry). Every reader routes a file by
`id.part_key` parsed from its name and **never re-derives it from contents or
cross-checks it against them at read time**. The producer chain guarantees the
filename matches the contents (the WAL writer stamps the `FileId` from the same
partition key it routes a frame to; the SFST indexer inherits that `FileId`
verbatim), so for self-produced files the label and the content always agree by
construction. The accepted trade-off: a file whose name is altered out-of-band
(manual rename, disk corruption) would be routed by its now-wrong label; there
is intentionally no runtime content-vs-label guard because filenames are
internal and never rewritten externally. See `sfst/FORMAT.md`
("Partition-key authority").

## Consumers and consistency

- **Ingestor** (`otel-ingestor::logs_service`): `extract_stream` returns a
  `ServiceStream` (absent→""); it derives `part_key(&stream)` and
  `encode_content_meta(&stream)` and hands both to the WAL writer. The
  per-`(tenant, part_key)` canonical collision table keys off the stream and its
  `part_key`. A literal-empty and an absent field land in one partition.
- **Query filter** (`file_registry::Query`): the partition filter is a **set of
  `part_key` values** — `partition_keys: Vec<u64>`. An empty set matches every
  partition; a non-empty set keeps files whose `part_key` is a member.
  `Query::matches_partition(part_key)` is the single empty=all membership
  predicate every source's `candidates` shares.
- **WAL registry** (`wal::Registry::candidates`): membership of `f.id.part_key`
  in `partition_keys`.
- **SFST registry** (`sfst::Registry::candidates`): membership of `f.id.part_key`
  (from the filename), identically. Routing by the filename key (not the prior
  content-derived `ServiceStream` equality on the summary) is safe under the
  ingestor's per-`(tenant, part_key)` collision invariant — one stream per key
  within a tenant — and is collapse-safe because `part_key`/`ns_hash` already maps
  absent and empty `service.namespace` together.
- **Catalog** (`otel-catalog::Catalog::find` / `Registry::candidates`): membership
  of `e.id.part_key`, identically.
- **Query planner** (`otel-ledger::query::remote_candidates_from`): membership of
  `e.id.part_key` via `Query::matches_partition`.
- **Indexer** (`sfst-indexer::row_index::service_stream`): resolves the file's
  single stream from interned `service.namespace=`/`service.name=` entries,
  defaulting a missing key to `""` (already collapsed), and encodes it into
  `content_meta`. It does **not** store the `part_key` in the summary.
- All filter tiers (WAL, SFST, catalog, planner) match by `id.part_key`
  membership, so they agree by construction; absent and empty collapse into one
  partition at the hash.

## Persisted formats

The substrate stores the opaque `content_meta` (display identity) but **not** the
`part_key` (which lives only in the `FileId`):

- **SFST** (`sfst` `VERSION = 7`): the `SUMR` summary is the content-agnostic
  `file_registry::FileSummary { min_timestamp_s, max_timestamp_s, record_count,
  content_meta }`. v7 dropped the `part_key` field from the summary; v6 had
  dropped the typed `ServiceStream` in favor of opaque `part_key` + `content_meta`.
- **WAL** (`wal` `FORMAT_VERSION = 4`): the header carries the opaque
  `content_meta` blob (so an unsealed WAL can be named/displayed without decoding
  frames — see the stream selector). v4 dropped the 8-byte `part_key` slot from
  the header (`content_meta_len` shifted to offset 16); v2 had replaced the typed
  stream String pairs with the opaque blob.
- **Catalog** (`otel-catalog` `FORMAT_VERSION = 4`): each `CatalogEntry` carries
  `content_meta`; v3 dropped the top-level `part_key` (it lives in `entry.id`); v4
  marks the per-signal remote-key layout below — a pre-v4 catalog's entries embed
  the old segment-less `remote_key`, so it is rejected on recovery (per the
  no-back-compat rule below) rather than republished with stale keys.
- **Remote object keys** (`otel-ledger` `remote_keys`, `SCHEMA_VERSION = "v1"`):
  signal-scoped — `v1/{signal}/tenants/{tenant}/sfst/{date}/{file}.sfst` and
  `v1/{signal}/catalog/{date}/{tenant}/{file}.catalog`. The `{signal}` segment is
  the per-signal storage separator (the decided design: signals live in distinct
  paths, not disambiguated by the filename); the `part_key` is still embedded via
  the `FileId` in the SFST filename. Each pipeline supplies its own signal name
  (logs = `"logs"`).

There is **no back-compat** at any tier — every reader hard-rejects an older
version (`UnsupportedVersion`) before deserializing the payload, so an unsealed
old-format WAL/SFST/catalog file is lost on upgrade (accepted: the feature is
experimental and files are short-lived). `recover()` reads the `part_key` from
the **filename** (`FileId`) on restart, not the header.

The writer→ledger **ferryboat IPC** carries the opaque identity too:
`FileEvent::Created` holds the `content_meta` blob (not a typed stream). Ferryboat
has no version field, but the writer and ledger are separate worker processes of
the **same `self_exe`**, so they never skew — no wire-compat constraint.

## Compatibility note

Under the empty→None rule, an absent-namespace file (the common case) keeps the
exact `part_key` the ingestor already wrote (`compute_ns_hash(None, name)`), so no
migration is needed for it. Only a file produced from a sender that set a *literal*
empty-string namespace changes hash (it now merges into the no-namespace stream).

The two filter tiers treat a pre-existing literal-empty file differently:

- **WAL tier:** an unindexed file still carries the old
  `compute_ns_hash(Some(""), name)` in its `FileId`, which the new query hash
  `compute_ns_hash(None, name)` misses. This window is bounded — it ends when the
  file is compacted into an SFST.
- **SFST tier:** once indexed, the file's stream collapses to `("", name)` — the
  indexer resolves both a missing key and a literal-empty `service.namespace=`
  interner value to `""` — so its `part_key` is `compute_ns_hash(None, name)`, the
  same key the query derives, and membership matches it normally.

This is accepted: OTel logs were always experimental and the literal-empty case
is rare. The transient WAL-tier window can hide a stale literal-empty file from a
stream-filtered query until it compacts into an SFST — still bounded and rare, not
a correctness regression for the common absent-namespace file.

## Stream selector in the live agent function

The live `otel-logs` handler (`otel-ledger::ledger::rpc::handler`) exposes a
**stream selector** modeled on the systemd-journal `__logs_sources` control:

- **Advertise.** Every data response carries one `required_params` entry — a
  `MultiSelection` with `id = "__streams"` (`STREAM_SELECTION_PARAM`), name
  "Services" — whose options are the tenant's streams. Options are enumerated
  **window-scoped and remote-inclusive** from `TenantRegistries::enumerate_streams`:
  every stream with data overlapping `q.time_range` — local SFST + unsealed WAL +
  remote-only (evicted-but-cataloged) entries, deduped by `part_key` with
  SFST-wins over WAL over remote per seq — so a stream with no in-window data is
  omitted, while a stream that exists only as an unsealed WAL or only remotely is
  listed (this is why the WAL header records `content_meta`). The stream filter
  is ignored during enumeration (the selector lists all streams independent of the
  user's current pick). Each option: `id` = `part_key` as 16-digit hex, `name` =
  `namespace/name` decoded from `content_meta` (absent namespace shown as
  `name • (no namespace)`), `pill` = total size (an active WAL uses `valid_up_to`
  since `File.size` lands only on close), `info` = file count + span, and
  `defaultSelected: true` on **every** option.
- **Default = all.** The UI auto-selects only the first option when none sets
  `defaultSelected`, and the control is mandatory (blocks the query until a
  non-empty selection). Marking every option `defaultSelected` keeps the default
  view spanning all streams (the prior no-selector behavior). A tenant with no
  streams advertises no control (`required_params: []`).
- **Honor.** The handler removes the reserved `__streams` key from `selections`
  **before** building the engine `LogsQuery` (so the engine never treats it as a
  row facet), decodes the picks (`hex → u64`, unparseable skipped) into
  `file_registry::Query::partition_keys`, and that set prunes which files the
  query opens. Empty set = all streams.

The offline `sfsq-cli` (`discover.rs`) applies the same `part_key` filter from its
`--namespace`/`--name` flags (one-element `partition_keys`); it derives the key via
`otel_logs_identity::part_key` / `ServiceStream::ns_hash` rather than recomputing
the hash inline. The registry candidate tests
(`wal`/`sfst`/`otel-catalog`/`otel-ledger::query`) and an `otel-ledger`
adapter/registry test suite exercise the filter and the enumeration directly.
