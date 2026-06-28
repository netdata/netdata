//! Build an SFST index file from ng-flatten output — the augment-SFST path.
//!
//! Streams the flattened WAL, stringifies each typed entry into a `key=value`
//! pair (collapsed-array paths like `tags[]`, typed values rendered to strings),
//! and feeds the existing `sfst-indexer` [`RowIndex`] via the `KvSink` interface in
//! a single pass, then `build_and_write`s a standard SFST file. This reuses SFST's
//! proven build/query machinery while the keys/values now come from the typed,
//! array-collapsed flattening. (Types + a schema-tree chunk come in a later step.)

use std::path::Path;

use bumpalo::Bump;
use sfst::{DroppedAttributeCounts, Flags, ObservedTimestamps, SpanIds, TraceIds};
use sfst_indexer::KvSlot;
use sfst_indexer::build_and_write;
use sfst_indexer::row_index::RowIndex;
use wal_otap::KvSink;

use crate::{Entry, Error, FlattenedRequest, Metrics, NodeId, build_kv, decode_frame, sole_wal_file};

/// SFST cardinality-tier threshold (mirrors `sfst`'s default of 100).
const CARDINALITY_THRESHOLD: u32 = 100;

/// Stats from building an SFST from the flattened WAL.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub struct SfstStats {
    pub frames: u64,
    pub records: u64,
    /// Entries that rode the `lookup_hash` fast path (no string formatting).
    pub hits: u64,
    /// Entries that missed (first occurrence) and were formatted + interned.
    pub misses: u64,
}

/// Intern a slice of entries into tokens, riding the interner's `lookup_hash` fast
/// path (format + intern only on a miss). Computed once per resource/scope group
/// and reused across its records, so shared attrs aren't re-probed per record.
fn intern_entries(
    row_index: &mut RowIndex<'_>,
    entries: &[Entry],
    paths: &[String],
    kv: &mut String,
    stats: &mut SfstStats,
) -> Vec<KvSlot> {
    entries
        .iter()
        .map(|e| match row_index.lookup_hash(e.hash) {
            Some(token) => {
                stats.hits += 1;
                token
            }
            None => {
                stats.misses += 1;
                build_kv(&paths[e.node as usize], &e.value, kv);
                row_index.intern(Some(e.hash), kv)
            }
        })
        .collect()
}

/// Build an SFST index file at `out_path` from the flattened WAL in `flat_dir`.
/// Single pass; phases timed: `read` / `deserialize` / `index` / `build`.
pub fn build_sfst(flat_dir: &Path, out_path: &Path, metrics: &Metrics) -> Result<SfstStats, Error> {
    let path = sole_wal_file(flat_dir)?;
    let mut reader = wal::Reader::open(&path)?;
    let arena = Bump::new();
    let mut row_index = RowIndex::new(&arena, CARDINALITY_THRESHOLD);
    let mut stats = SfstStats::default();
    let mut kv = String::new();

    // Per-row columns accumulated in insertion order, parallel to the rows fed to
    // `row_index.row(...)`. Stored as the optional SFST column chunks
    // (`OBTS`/`TRCE`/`SPAN`/`FLAG`/`DRAC`), not facets: identifiers/scalars retrieved
    // per row, not FST-indexed. trace_id/span_id are no longer flattened as entries
    // (see `ng_flatten::flatten_record`), so they never reach the interner — they live
    // only here. The ingest boundary already validated id lengths (16/8 or empty);
    // the arenas zero-fill empty/short ids.
    let mut observed_ts: Vec<i64> = Vec::new();
    let mut trace_ids = TraceIds::default();
    let mut span_ids = SpanIds::default();
    let mut flags: Vec<u32> = Vec::new();
    let mut dropped_attrs: Vec<u32> = Vec::new();

    loop {
        let frame = {
            let _t = metrics.scope("read");
            match reader.next_frame()? {
                Some(frame) => frame,
                None => break,
            }
        };
        stats.frames += 1;
        metrics.add_frames(1);
        metrics.add_bytes(frame.data.len() as u64);

        let flattened: FlattenedRequest = {
            let _t = metrics.scope("deserialize");
            decode_frame(frame.data).map_err(|source| Error::BincodeDecode {
                frame: stats.frames,
                source,
            })?
        };

        let _t = metrics.scope("index");
        let tree = &flattened.tree;
        // Resolve each node's path once per frame (records reuse the same nodes).
        let paths: Vec<String> = (0..tree.len() as NodeId).map(|id| tree.path(id)).collect();

        // Collect resource, scope and record tokens
        let mut tokens: Vec<KvSlot> = Vec::new();

        let mut records = 0u64;
        for rg in &flattened.resources {
            // Intern resource attrs once per group and reuse across its records,
            // instead of re-probing them for every record.
            let resource_tokens =
                intern_entries(&mut row_index, &rg.resource, &paths, &mut kv, &mut stats);

            tokens.clear();
            tokens.extend_from_slice(&resource_tokens);

            for sg in &rg.scopes {
                let scope_tokens =
                    intern_entries(&mut row_index, &sg.scope, &paths, &mut kv, &mut stats);
                tokens.extend_from_slice(&scope_tokens);

                for record in &sg.records {
                    records += 1;

                    let record_tokens =
                        intern_entries(&mut row_index, &record.entries, &paths, &mut kv, &mut stats);
                    tokens.truncate(resource_tokens.len() + scope_tokens.len());
                    tokens.extend_from_slice(&record_tokens);

                    // ts resolved at ingest (time/observed/clock); always concrete.
                    row_index.row(record.ts, &tokens);

                    // Per-row columns, one value pushed per row (parallel to the
                    // row just fed) so they stay aligned for the build-time remap.
                    observed_ts.push(record.observed_ts);
                    trace_ids.push(&record.trace_id);
                    span_ids.push(&record.span_id);
                    flags.push(record.flags);
                    dropped_attrs.push(record.dropped_attributes_count);
                }

                tokens.truncate(resource_tokens.len());
            }
        }
        stats.records += records;
        metrics.add_records(records);
    }

    // Hand the accumulated per-row columns to the builder, which reorders each to
    // chronological order and writes its column chunk. This pipeline supplies all
    // five (each accumulated one value per row, so lengths equal the row count and
    // align with the rows fed to `row.row(...)`); the columns are independently
    // optional at the format level (see `RowIndex`), this caller just fills them all.
    row_index.observed_timestamps = Some(ObservedTimestamps(observed_ts));
    row_index.trace_ids = Some(trace_ids);
    row_index.span_ids = Some(span_ids);
    row_index.flags = Some(Flags(flags));
    row_index.dropped_attribute_counts = Some(DroppedAttributeCounts(dropped_attrs));

    let _t = metrics.scope("build");
    build_and_write(&row_index, out_path)?;
    Ok(stats)
}
