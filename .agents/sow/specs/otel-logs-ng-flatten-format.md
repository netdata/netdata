# OTel logs ng-flatten WAL frame + SFST v9 format

## Scope

- The on-WAL payload format for OTel **logs** and the SFST **v9** typed
  descriptor it produces, plus the field namespace, ingest normalization, and
  the seal/tail render-parity guarantee.
- **Logs only.** Metrics are a different signal on the same gRPC server and
  storage substrate; they do not use ng-flatten flattening and are out of scope
  here. **Traces update (in-flight):** a span flattening path now lives in
  `ng-flatten` too (Decision 3A — extend the crate, not a sibling), with parallel
  `SpanRecord`/`FlattenedTraceRequest` types that reuse the shared
  `SchemaTree`/`Entry`/`flatten_resource`/`flatten_scope`. It is pre-graduation
  (no producer wired yet); its durable contract will get its own spec when the
  traces pipeline graduates. This spec remains the authority for the **logs**
  frame + v9 descriptor.
- Cross-references (do not duplicate): stream identity →
  [otel-stream-identity.md](otel-stream-identity.md); file lifecycle / substrate
  → [otel-storage-substrate.md](otel-storage-substrate.md); on-disk SFST chunk
  layout → `src/crates/sfst/FORMAT.md`.

## WAL logs payload (the frame contract)

- The logs WAL frame payload is a **bincode-encoded
  `ng_flatten::FlattenedLogRequest`**. The `wal` container is payload-agnostic;
  only logs frames carry this shape.
- **Versioned per file (2026-07, WAL header v5):** the payload is identified by
  `ng_flatten::LOG_FRAME_PAYLOAD_FORMAT` (= 1) in the WAL header's opaque
  `payload_format` tag. Producers stamp it at `wal::Writer::new`; both logs
  decode sites — the seal build (`ng_index`, `check_payload_format`) and the
  tail scan (`sfsq::WalScan::drain_flattened`) — check it before any bincode
  decode and refuse a mismatch with the two ids. bincode is positional (not
  self-describing), so any change to the frame types (`Record`, `Entry`,
  `Value`/`Kind` variants, `SchemaTree`) changes the wire shape and MUST ship
  under a NEW format id; ids are append-only, never reused. By default no
  decoder for the superseded id is kept — the old file becomes a kept, logged
  orphan (the decided policy). The flattened traces codec has its own id
  (`TRACE_FRAME_PAYLOAD_FORMAT` = 3); the traces raw-OTLP proof uses 2.
- Each frame is **self-contained**: a per-frame typed `ng_flatten::SchemaTree`
  plus resource → scope → record groups of typed `Entry` values, plus the
  per-record columns. No cross-frame state — any frame range decodes alone.
- Producers MUST emit it via `ng_flatten::prepare_log_frame` (one normalize
  walk → consuming flatten with emit-time entry hashing → bincode encode):
  the production ingestor (`otel-ingestor::logs_service`) and the `ng-ingest`
  benchmark binary both do.
- The former OTAP/Arrow logs payload (the `wal-otap` crate and
  `otel-ingestor::arrow_bridge`) is **removed**. ng-flatten is the only logs
  format; do not reintroduce a second one.

## Field namespace (the queryable key space)

- Record attributes → `attributes.*`; resource attributes →
  `resource.attributes.*`; scope name/version/attributes → `scope.*`.
- Scalar arrays **collapse** to one multi-valued key, one pair per element:
  `attributes.tags[]`.
- Array-of-structs **collapses, not positional**: `attributes.endpoints[].host`
  (never `endpoints.0.host`).
- Projected scalar fields are queryable: `event_name` (now indexed),
  `severity_text`, `severity_number`, `body`.
- `trace_id` / `span_id` / `flags` / `dropped_attributes_count` /
  `observed_time` are **per-row columns**, NOT indexed entries — retrieved per
  row, not faceted (`trace_id`/`span_id` are near-unique; faceting them is noise).

## Per-frame hashing fast path

- Each `Entry` carries a pre-computed `xxhash64("key=value")`, filled at
  emit time by the flattener (an invariant of flattening — there is no
  separate fill pass); the interner uses an identity hasher so the
  pre-computed `u64` is the bucket key.
- The seal builder rides `RowIndex::lookup_hash` — on a hash hit it skips
  `key=value` string formatting. Collision-safe: the interner answers `None` for
  any hash it has seen more than one distinct string for.
- The tail scanner (`ScanSink`) dedups by full string and keeps no hash index —
  a bounded tail gains nothing from the fast path.

## SFST v9 typed descriptor

- The v9 **on-disk** field descriptor is a typed `sfst::SchemaTree` in the `META`
  chunk; it replaces the flat untyped `FieldTable` *as the persisted descriptor* —
  not as the query contract.
- The flat `FieldTable` is **still provided**, derived from the tree on demand
  (`SchemaTree::derive_field_table()`, reproducing the same tiers/cardinality).
  Both query paths yield one — `sfst::IndexReader::field_table()` for a sealed SFST
  and `WalScan`'s `fields` for the tail — and `otel-ledger`'s otel-logs adapter
  builds the Function response's `columns` + `available_histograms` from it. The
  query side stays producer-agnostic: it reads only the derived `FieldTable`.
- A producer that supplies no typed tree emits a flat `Str`-typed tree
  (`SchemaTree::flat`) so every v9 file carries a valid descriptor.

## Ingest normalization (once, at the observation point)

- `ng_flatten::normalize_timestamps(req, fallback_base_ns)`: per record,
  `time_unix_nano` if non-zero, else `observed_time_unix_nano`, else
  `fallback_base_ns + row_offset` (base+offset, computed lock-free), frozen into
  `Record.ts`. Ordering is preserved; the synthetic fallback is deterministic but
  not globally unique across frames (it only affects records lacking both event
  and observed time).
- `ng_flatten::normalize_ids(req)`: drops malformed trace/span ids (not exactly
  16/8 bytes, or empty) so the per-row `TRCE`/`SPAN` columns stay clean
  fixed-stride; returns an aggregate `MalformedIds` count for a single warning
  per request.
- Both run once at ingest. Downstream (seal build, tail scan) reads the frozen
  values; it MUST NOT re-resolve timestamps or ids.

## Identity authority

- The ingestor writes the stream `content_meta` into the WAL header; the
  seal/range build trusts that blob **verbatim** as the file's display identity
  (`sfst::IndexWriter::write_file`/`write_into` take `content_meta: Vec<u8>` as
  a required argument; the build never derives or inspects it). Full contract:
  [otel-stream-identity.md](otel-stream-identity.md).

## Seal/tail render parity (correctness-critical)

- Each frame is consumed by two readers: the seal-time build
  (`ng_index::build_sfst_file` / `build_sfst_range` → an SFST) and the
  active-WAL **tail** scan (`sfsq::logs::WalScan::scan_flattened` /
  `scan_flattened_range`).
- Both MUST render `key=value` through the **same** `ng_flatten::build_kv` from
  the **same** frame, assemble tokens in the **same** order (resource ++ scope ++
  record), and order rows by the **same** frozen `Record.ts`. Parity is therefore
  **structural**, not two decoders happening to agree.
- The query path (`sfsq::logs::run`) folds sealed chunks (read via
  `sfst::IndexReader`) and the tail (row scan) into one result. The guarantee is
  guarded by `src/crates/sfsq/tests/ng_wal_equivalence.rs` (tail vs sealed across
  filter/facet/timeline/materialize, plus the run-level fold).

## Invariants

- Every v9 SFST MUST carry a valid `SchemaTree` descriptor (typed, or flat via
  `SchemaTree::flat`).
- A logs WAL frame MUST be a self-contained bincode `FlattenedLogRequest` with
  no cross-frame dependency.
- Seal and tail MUST produce identical `key=value` strings and MUST order by the
  frozen `Record.ts`.
- The pipeline MUST NOT reintroduce the OTAP/Arrow logs payload or a second logs
  renderer.

## Consumers

`ng-flatten`, `ng-index`, `sfst` (index build), `sfsq`, `otel-ingestor::logs_service`,
`otel-ledger` (indexer + on-query handler); operator field-namespace
documentation / UI field pickers / saved queries before GA (the `attributes.*`
namespace differs from the pre-migration bare keys).
