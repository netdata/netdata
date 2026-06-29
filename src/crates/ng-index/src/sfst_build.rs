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
use sfst_indexer::{build_and_write, build_into};
use sfst_indexer::row_index::RowIndex;
use wal_otap::KvSink;

use crate::{Entry, Error, FlattenedRequest, Metrics, NodeId, build_kv, decode_frame, sole_wal_file};

/// Map a flatten [`ng_flatten::Kind`] to the format crate's [`sfst::ValueKind`].
/// Identical variant sets; the conversion keeps `sfst` independent of
/// `ng-flatten` (the format crate owns its own kind enum).
fn to_value_kind(kind: ng_flatten::Kind) -> sfst::ValueKind {
    use ng_flatten::Kind;
    match kind {
        Kind::Null => sfst::ValueKind::Null,
        Kind::Bool => sfst::ValueKind::Bool,
        Kind::Int => sfst::ValueKind::Int,
        Kind::Double => sfst::ValueKind::Double,
        Kind::Str => sfst::ValueKind::Str,
        Kind::Bytes => sfst::ValueKind::Bytes,
        Kind::EmptyKvlist => sfst::ValueKind::EmptyKvlist,
        Kind::EmptyArray => sfst::ValueKind::EmptyArray,
        Kind::Kvlist => sfst::ValueKind::Kvlist,
        Kind::Array => sfst::ValueKind::Array,
    }
}

/// Convert the global flatten [`ng_flatten::SchemaTree`] into the format crate's
/// [`sfst::SchemaTree`] node-by-node (ids are preserved, parents precede
/// children). Leaf stats are left unset here — `sfst-indexer` fills them from
/// the per-field cardinality/tier at build time.
fn to_sfst_tree(tree: &ng_flatten::SchemaTree) -> sfst::SchemaTree {
    let nodes = (0..tree.len() as NodeId)
        .map(|id| sfst::SchemaNode {
            kind: to_value_kind(tree.node(id).kind),
            edge: tree.edge(id).map(|(parent, step)| sfst::SchemaEdge {
                parent,
                step: match step {
                    ng_flatten::Step::Field(name) => sfst::Step::Field(name.clone()),
                    ng_flatten::Step::ArrayElem => sfst::Step::ArrayElem,
                },
            }),
            leaf: None,
        })
        .collect();
    sfst::SchemaTree::from_nodes(nodes)
}

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

/// Populate `row_index` from every frame `reader` yields: intern each entry
/// (resource ++ scope ++ record), feed the per-row columns and the row `ts`, and
/// accumulate then attach the global typed schema tree. Shared by the file build
/// ([`build_sfst`]) and the in-memory range build ([`build_sfst_range`]) so both
/// produce byte-identical chunks from the same frames — only the output sink differs.
fn populate_row_index(
    reader: &mut wal::Reader,
    row_index: &mut RowIndex<'_>,
    metrics: &Metrics,
) -> Result<SfstStats, Error> {
    let mut stats = SfstStats::default();
    let mut kv = String::new();
    // Accumulates the global typed schema tree by interning every frame's
    // per-frame tree. Persisted as the v9 field descriptor (`Metadata.tree`).
    // The interner feed still renders `key=value` via the per-frame paths (the
    // rendered string is identical), so this is build-only — the returned
    // local→global map is unused.
    let mut flattener = ng_flatten::Flattener::new();

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
        // Intern this frame's tree into the global tree (build-only; the map is
        // unused — paths below render identically from the per-frame tree).
        let _ = flattener.merge_tree(tree);
        // Resolve each node's path once per frame (records reuse the same nodes).
        let paths: Vec<String> = (0..tree.len() as NodeId).map(|id| tree.path(id)).collect();

        // Collect resource, scope and record tokens
        let mut tokens: Vec<KvSlot> = Vec::new();

        let mut records = 0u64;
        for rg in &flattened.resources {
            // Intern resource attrs once per group and reuse across its records,
            // instead of re-probing them for every record.
            let resource_tokens =
                intern_entries(row_index, &rg.resource, &paths, &mut kv, &mut stats);

            tokens.clear();
            tokens.extend_from_slice(&resource_tokens);

            for sg in &rg.scopes {
                let scope_tokens =
                    intern_entries(row_index, &sg.scope, &paths, &mut kv, &mut stats);
                tokens.extend_from_slice(&scope_tokens);

                for record in &sg.records {
                    records += 1;

                    let record_tokens =
                        intern_entries(row_index, &record.entries, &paths, &mut kv, &mut stats);
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

    // Persist the global typed schema tree as the v9 field descriptor. The
    // builder fills each leaf's cardinality/tier from the per-field stats.
    row_index.tree = Some(to_sfst_tree(&flattener.into_tree()));

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

    Ok(stats)
}

/// Build an SFST index file at `out_path` from the flattened WAL in `flat_dir`.
/// Single pass; phases timed: `read` / `deserialize` / `index` / `build`.
pub fn build_sfst(flat_dir: &Path, out_path: &Path, metrics: &Metrics) -> Result<SfstStats, Error> {
    let path = sole_wal_file(flat_dir)?;
    let mut reader = wal::Reader::open(&path)?;
    let arena = Bump::new();
    let mut row_index = RowIndex::new(&arena, CARDINALITY_THRESHOLD);
    let stats = populate_row_index(&mut reader, &mut row_index, metrics)?;
    let _t = metrics.scope("build");
    build_and_write(&row_index, out_path)?;
    Ok(stats)
}

/// Build an **in-memory** SFST over a frame-aligned `range` of an active flattened
/// WAL, returning its [`sfst::Summary`] plus the serialized bytes (openable with
/// [`sfst::IndexReader::open`]). The in-memory counterpart of [`build_sfst`] — the
/// on-query chunk build over an active WAL's durable-unindexed prefix. Reads only the
/// frames in `range` via [`wal::Reader::open_range`] (see [`wal::FrameRange`] for the
/// frame-boundary / durable-prefix soundness checks); the typed tree + per-row columns
/// are identical to the file build, so the chunks are byte-identical to a file build
/// over the same frames. The caller cross-checks `summary.record_count` against the
/// expected count to detect a truncated prefix (the check `open_range` defers).
pub fn build_sfst_range(
    wal_path: &Path,
    range: wal::FrameRange,
) -> Result<(sfst::Summary, Vec<u8>), Error> {
    let mut reader = wal::Reader::open_range(wal_path, range)?;
    let arena = Bump::new();
    let mut row_index = RowIndex::new(&arena, CARDINALITY_THRESHOLD);
    populate_row_index(&mut reader, &mut row_index, &Metrics::new())?;
    let cursor = std::io::Cursor::new(Vec::new());
    let (cursor, summary, _metadata) = build_into(&row_index, cursor)?;
    Ok((summary, cursor.into_inner()))
}

#[cfg(test)]
mod tests {
    use super::*;
    use ng_flatten::flatten_request;
    use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
    use opentelemetry_proto::tonic::common::v1::{
        AnyValue, ArrayValue, KeyValue, KeyValueList, any_value::Value as Av,
    };
    use opentelemetry_proto::tonic::logs::v1::{LogRecord, ResourceLogs, ScopeLogs};

    /// One record with a nested kvlist (`obj.inner`) and an array (`tags[]`) —
    /// exercises both `Field` and `ArrayElem` steps in the two `path()` renderers.
    fn nested_request() -> ExportLogsServiceRequest {
        let any = |v| Some(AnyValue { value: Some(v) });
        let rec = LogRecord {
            time_unix_nano: 1_700_000_000_000_000_000,
            attributes: vec![
                KeyValue {
                    key: "obj".into(),
                    value: any(Av::KvlistValue(KeyValueList {
                        values: vec![KeyValue {
                            key: "inner".into(),
                            value: any(Av::IntValue(1)),
                        }],
                    })),
                },
                KeyValue {
                    key: "tags".into(),
                    value: any(Av::ArrayValue(ArrayValue {
                        values: vec![AnyValue { value: Some(Av::StringValue("a".into())) }],
                    })),
                },
            ],
            ..Default::default()
        };
        ExportLogsServiceRequest {
            resource_logs: vec![ResourceLogs {
                scope_logs: vec![ScopeLogs { log_records: vec![rec], ..Default::default() }],
                ..Default::default()
            }],
        }
    }

    /// `to_sfst_tree` preserves node ids and structure, so the format crate's
    /// `path()` must render every node identically to `ng_flatten`'s. This pins
    /// the two duplicated renderers against a divergence that would silently
    /// drop a field from the reader's derived field table (review finding D).
    #[test]
    fn sfst_and_ng_flatten_path_renderers_agree() {
        let flattened = flatten_request(&nested_request());
        let ng_tree = &flattened.tree;
        let sfst_tree = to_sfst_tree(ng_tree);

        assert_eq!(sfst_tree.len(), ng_tree.len(), "node count must be preserved");
        assert!(ng_tree.len() > 3, "fixture should produce a non-trivial tree");
        let mut saw_array = false;
        for id in 0..ng_tree.len() as NodeId {
            let expected = ng_tree.path(id);
            saw_array |= expected.contains("[]");
            assert_eq!(sfst_tree.path(id), expected, "path mismatch at node {id}");
        }
        assert!(saw_array, "fixture must exercise an ArrayElem ([]) step");
    }
}
