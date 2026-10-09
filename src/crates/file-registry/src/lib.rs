//! The storage-substrate leaf of the OTel plugin's file stack: file identity
//! and the naming/layout vocabulary every storage crate shares, plus the
//! generic building blocks on top of it — crash-safe atomic writes,
//! directory scanning, the seq-keyed registry, the time/partition query
//! filter, and the strictly increasing wall clock.
//!
//! The crate is deliberately content-agnostic: no codec, no query engine, no
//! signal knowledge. [`FileId::pipeline_id`] and [`FileId::part_key`] are
//! opaque numbers here — the signal↔id mapping is the integration layer's
//! (`bridge::signals`: logs=0, traces=1) and the content plane derives
//! partition keys — so a second signal reuses the whole substrate without
//! this crate learning its name.
//!
//! # Modules
//!
//! - `types` (private; its re-exports below are the API): [`FileId`] and the
//!   filename codec, the identity newtypes ([`MachineId`], [`InstanceId`])
//!   that refuse nil, the [`Identity`] pair carrying them, [`SeqKey`],
//!   [`TenantId`], [`TimestampNs`], [`ByteSize`], and the content-agnostic
//!   [`FileSummary`].
//! - `stem` (public): the shared `{machine}-{instance}` 32-hex filename
//!   prefix. Data files append `{pipeline_id:05}-{seq:010}-{part_key:016x}`
//!   and parse back through [`FileId::parse`]/[`FileId::parse_stem`]; the
//!   remote catalog appends `{max_seq:010}-{min_ts:010}-{max_ts:010}` and
//!   parses the prefix with this codec — the reason the module is public.
//! - `durable` (public): the one temp+fsync+rename+dir-fsync write path and
//!   the reserved `.tmp` suffix with its stale-temp sweep. Every
//!   tmp+rename producer in the otel stack (WAL, SFST, catalogs, the seq
//!   high-water file) writes through it.
//! - `dir` (private): [`FileDir`], a flat directory+extension handle for
//!   path derivation, scanning and per-directory max-seq reads; plus
//!   [`scan_max_sequence_recursive`], the one-level walk that re-seeds the
//!   seq counter across restarts.
//! - `layout` (public): the date-partitioned per-tenant layout
//!   `{base}/{YYYY-MM-DD}/{tenant}/` the remote catalog lives under, with a
//!   strict and a lossy partition walk.
//! - `query` (private): [`Query`] — time range plus optional partition set —
//!   and [`range_overlaps`], the single overlap rule every candidate filter
//!   shares.
//! - `registry` (private): [`FileRegistry`], a `BTreeMap` keyed by each
//!   entry's own seq (`Sequenced`); wal and sfst keep their open files in
//!   one.
//! - `selection` (private): [`SelectedFile`], the engine-neutral sealed-file
//!   candidate a query engine converts at its boundary.
//! - `clock` (private): [`MonotonicClock`], strictly increasing nanosecond
//!   wall timestamps.
//!
//! # Contracts
//!
//! - Data-file name:
//!   `<machine_id>-<instance_id>-<pipeline_id:05>-<seq:010>-<part_key:016x>.<ext>`
//!   — machine/instance as 32-char lowercase hex UUIDs, pipeline_id a 5-digit
//!   zero-padded decimal, seq 10 digits, part_key 16 lowercase hex digits;
//!   the extension is the producer's (`wal`, `sfst`, ...). A nil-bearing
//!   machine or instance never renders: the newtypes refuse nil at
//!   construction and [`FileId::parse_stem`] returns `None` on it, so
//!   recovery routes such a file to warn-and-skip instead of creating a
//!   zero-provenance file.
//! - `seq` is unique within ONE process instance across pipelines, so it
//!   alone identifies a local file only; state that crosses an identity
//!   boundary is keyed by [`SeqKey`]. Startup re-seeds the counter from disk
//!   (WAL and SFST max seqs, bounded by the catalog scan and the persisted
//!   seq high-water file).
//! - `pipeline_id` is the numeric signal axis: always chosen explicitly at
//!   construction (no default), opaque to this crate, and part of the disk
//!   format — the raw `u16` must stay byte-compatible across releases, and
//!   signal names never enter filenames.
//! - Units: [`TimestampNs`] is nanoseconds (WAL frames, the clock);
//!   [`FileSummary`] time ranges are seconds (`u32`), the unit [`Query`]
//!   filters on; [`range_overlaps`] is generic over both. The partition key
//!   lives only in the [`FileId`] — the summary carries an opaque
//!   `content_meta` blob the substrate never interprets.
//!
//! # Failure conventions
//!
//! `durable` annotates every I/O error with the operation and path while
//! preserving its `ErrorKind`; scans treat a missing directory as empty and
//! per-entry failures as warn-and-skip; `layout` exposes the strict walk
//! (errors propagate, for callers that need completeness) next to the lossy
//! one (partial results); unparseable filenames are warn-and-skip.
//!
//! # Concurrency
//!
//! No locks, no globals, no async runtime: every function is synchronous
//! `std::fs`/CPU work and the types are plain values (`FileId` is `Copy`,
//! `FileDir` is `Clone`). The blocking *write* path (`durable`) runs under
//! `spawn_blocking` in async consumers (file-lifecycle's catalog builder and
//! recovery do); the *scan* side (`dir`) runs inline as one-shot
//! startup/offline work on the caller's thread.
//!
//! # Consumers in this tree
//!
//! Direct dependents, grep-verified over `src/crates/*/Cargo.toml`: the
//! data planes `wal` and `sfst` (identity, FileDir/FileRegistry, seq
//! recovery, range rule); `file-lifecycle`, the heaviest user (layout,
//! durable, SeqKey, SelectedFile, Query); `otel-ingestor` (identity, the
//! startup seq seed) and `otel-ledger` (identity, SeqKey/TenantId,
//! SelectedFile/Query/FileSummary accounting); `otel-catalog` (identity,
//! the stem prefix for its own tail, layout walks, durable sweeps);
//! `file-cache` (durable) and `sfsq` (SelectedFile, FileDir/Query;
//! `sfsq-cli` uses FileDir/Query only); `ng-ingest` and `ng-index` (value
//! types and clock; ng-index only from tests); and `otel-plugin`/`bridge`,
//! identity types
//! only (the supervisor builds the per-process `Identity` for the plugin
//! config). The crate does not depend on ferryboat and carries no IPC.
pub mod durable;
pub mod layout;
pub mod stem;

mod types;
pub use types::{
    ByteSize, FileId, FileSummary, Identity, InstanceId, MachineId, NilIdentity, SeqKey, TenantId,
    TimestampNs, test_identity,
};

mod selection;
pub use selection::SelectedFile;

mod clock;
pub use clock::MonotonicClock;

mod dir;
pub use dir::{FileDir, scan_max_sequence_recursive};

mod query;
pub use query::{Query, range_overlaps};

mod registry;
pub use registry::{FileRegistry, Sequenced};
