//! Build an SFST index file from ng-flatten output — the augment-SFST path.
//!
//! Streams the flattened WAL, stringifies each typed entry into a `key=value`
//! pair (collapsed-array paths like `tags[]`, typed values rendered to strings),
//! and feeds an [`sfst::RowIndex`] (interning via its `lookup_hash`/`intern`
//! fast path, one `row` per record) in a single pass, then writes a standard
//! SFST file via [`sfst::IndexWriter`]. This reuses SFST's proven build/query
//! machinery while the keys/values now come from the typed, array-collapsed
//! flattening.

use std::path::Path;

use bumpalo::Bump;
use sfst::{
    DroppedAttributeCounts, Durations, Flags, ObservedTimestamps, ParentSpanIds, SpanId, SpanIds,
    TraceId, TraceIds,
};
use sfst::{IndexWriter, KvSlot, RowIndex};

use crate::{
    Entry, Error, FlattenedLogRequest, Metrics, NodeId, build_kv, decode_log_frame, sole_wal_file,
};

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
/// children). Leaf stats are left unset here — the SFST build fills them from
/// the per-field cardinality/tier.
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
    /// Entries whose precomputed hash hit AND whose interned string matched — the
    /// fast path (no intern-map insert).
    pub hits: u64,
    /// Entries that missed the hash, or hit but whose string differed (a collision):
    /// interned via the string path.
    pub misses: u64,
}

/// Intern a slice of entries into tokens. The `key=value` is always rendered so a
/// `lookup_hash` hit can be verified against the interned string (guarding against a
/// hash collision aliasing distinct strings); a verified hit skips the intern-map
/// insert. Computed once per resource/scope group and reused across its records, so
/// shared attrs aren't re-probed per record.
fn intern_entries(
    row_index: &mut RowIndex<'_>,
    entries: &[Entry],
    paths: &[String],
    kv: &mut String,
    stats: &mut SfstStats,
) -> Vec<KvSlot> {
    entries
        .iter()
        .map(|e| {
            // Render key=value up front so a hash hit can be *verified* against the
            // interned string. Without this re-check, two distinct strings sharing a
            // hash (a genuine xxhash64 collision, or an unfilled hash==0) would alias:
            // `lookup_hash` returns the first string's slot with no string check, and
            // the interner's collision overflow — populated only inside `intern` — is
            // never reached, because a fast-path hit skips `intern` entirely.
            build_kv(&paths[e.node as usize], &e.value, kv);
            let hit = row_index.lookup_hash(e.hash);
            match hit {
                Some(token) if row_index.resolve(token) == kv.as_str() => {
                    stats.hits += 1;
                    token
                }
                // Unknown hash, or a hit whose string differs (a real collision):
                // `intern` compares strings and records the collision correctly.
                _ => {
                    stats.misses += 1;
                    row_index.intern(Some(e.hash), kv)
                }
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
    check_payload_format(reader, ng_flatten::LOG_FRAME_PAYLOAD_FORMAT)?;
    let mut stats = SfstStats::default();
    let mut kv = String::new();
    // Accumulates the global typed schema tree by interning every frame's
    // per-frame tree. Persisted as the on-disk field descriptor (`Metadata.tree`).
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
    // the push sites map each raw id to a typed `TraceId`/`SpanId` (empty/malformed
    // → the all-zero `UNSET`) — the arenas store exactly-width typed values.
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

        let flattened: FlattenedLogRequest = {
            let _t = metrics.scope("deserialize");
            decode_log_frame(frame.data).map_err(|source| Error::BincodeDecode {
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
                    // Convert the flattened id (already a typed, fixed-width
                    // ng_flatten id) to the sfst column id — a byte copy across
                    // the crate boundary (ng-flatten has no sfst dep).
                    trace_ids.push(TraceId::from(*record.trace_id.as_bytes()));
                    span_ids.push(SpanId::from(*record.span_id.as_bytes()));
                    flags.push(record.flags);
                    dropped_attrs.push(record.dropped_attributes_count);
                }

                tokens.truncate(resource_tokens.len());
            }
        }
        stats.records += records;
        metrics.add_records(records);
    }

    // Persist the global typed schema tree as the on-disk field descriptor. The
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

/// Refuse to decode a WAL whose header names a different frame codec, before
/// the first frame decode — a mismatched file fails with the format ids
/// instead of a bincode error or a silent mis-decode.
fn check_payload_format(reader: &wal::Reader, expected: u16) -> Result<(), Error> {
    let found = reader.header().payload_format;
    if found != expected {
        return Err(Error::PayloadFormat { found, expected });
    }
    Ok(())
}

/// Build an SFST index file at `out_path` from the flattened WAL in `flat_dir`.
/// Single pass; phases timed: `read` / `deserialize` / `index` / `build`.
pub fn build_sfst(flat_dir: &Path, out_path: &Path, metrics: &Metrics) -> Result<SfstStats, Error> {
    let path = sole_wal_file(flat_dir)?;
    let mut reader = wal::Reader::open(&path)?;
    // The WAL header carries the authoritative identity blob the ingestor wrote;
    // it becomes the SFST summary's content_meta verbatim.
    let content_meta = reader.header().content_meta.clone();
    let arena = Bump::new();
    let mut row_index = RowIndex::new(&arena, CARDINALITY_THRESHOLD);
    let stats = populate_row_index(&mut reader, &mut row_index, metrics)?;
    let _t = metrics.scope("build");
    IndexWriter::write_file(&row_index, out_path, content_meta)?;
    Ok(stats)
}

/// Build an SFST index file from a single flattened WAL **file** (not a directory),
/// returning the registry-facing [`sfst::Summary`] + the written file size. The
/// production seal-time entry point `otel-ledger` calls with a concrete WAL path.
/// Same typed tree + per-row columns as [`build_sfst`]; it just skips the
/// `sole_wal_file` directory probe.
pub fn build_sfst_file(
    wal_path: &Path,
    out_path: &Path,
    metrics: &Metrics,
) -> Result<(sfst::Summary, u64), Error> {
    let mut reader = wal::Reader::open(wal_path)?;
    let content_meta = reader.header().content_meta.clone();
    let arena = Bump::new();
    let mut row_index = RowIndex::new(&arena, CARDINALITY_THRESHOLD);
    populate_row_index(&mut reader, &mut row_index, metrics)?;
    let _t = metrics.scope("build");
    let (summary, _metadata) = IndexWriter::write_file(&row_index, out_path, content_meta)?;
    let size = std::fs::metadata(out_path)?.len();
    Ok((summary, size))
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
    let content_meta = reader.header().content_meta.clone();
    let arena = Bump::new();
    let mut row_index = RowIndex::new(&arena, CARDINALITY_THRESHOLD);
    populate_row_index(&mut reader, &mut row_index, &Metrics::new())?;
    let cursor = std::io::Cursor::new(Vec::new());
    let (cursor, summary, _metadata) = IndexWriter::write_into(&row_index, cursor, content_meta)?;
    Ok((summary, cursor.into_inner()))
}

/// The traces analog of [`populate_row_index`]: decode `FlattenedTraceRequest` frames,
/// intern each span's entries (resource ++ scope ++ span), feed one row per span keyed
/// on the span's start `ts`, and accumulate the span per-row columns — `trace_id`,
/// `span_id`, `parent_span_id`, `duration`, `flags`, `dropped_attributes_count`. There
/// is no `observed_ts` (spans have none). Sets `build_trace_id_index` so the seal
/// builds the `TIDX` index from the chronological `trace_id` column. Span entries
/// carry ingest-filled hashes (see `ng-ingest::write_trace_request`), so the interner
/// fast path is safe here.
fn populate_trace_row_index(
    reader: &mut wal::Reader,
    row_index: &mut RowIndex<'_>,
    metrics: &Metrics,
) -> Result<SfstStats, Error> {
    check_payload_format(reader, ng_flatten::TRACE_FRAME_PAYLOAD_FORMAT)?;
    let mut stats = SfstStats::default();
    let mut kv = String::new();
    let mut flattener = ng_flatten::Flattener::new();

    let mut trace_ids = TraceIds::default();
    let mut span_ids = SpanIds::default();
    let mut parent_span_ids = ParentSpanIds::default();
    let mut durations: Vec<i64> = Vec::new();
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

        let flattened: ng_flatten::FlattenedTraceRequest = {
            let _t = metrics.scope("deserialize");
            ng_flatten::decode_trace_frame(frame.data).map_err(|source| Error::BincodeDecode {
                frame: stats.frames,
                source,
            })?
        };

        let _t = metrics.scope("index");
        let tree = &flattened.tree;
        let _ = flattener.merge_tree(tree);
        let paths: Vec<String> = (0..tree.len() as NodeId).map(|id| tree.path(id)).collect();

        let mut tokens: Vec<KvSlot> = Vec::new();
        let mut records = 0u64;
        for rg in &flattened.resources {
            let resource_tokens =
                intern_entries(row_index, &rg.resource, &paths, &mut kv, &mut stats);
            tokens.clear();
            tokens.extend_from_slice(&resource_tokens);

            for sg in &rg.scopes {
                let scope_tokens =
                    intern_entries(row_index, &sg.scope, &paths, &mut kv, &mut stats);
                tokens.extend_from_slice(&scope_tokens);

                for span in &sg.spans {
                    records += 1;

                    let span_tokens =
                        intern_entries(row_index, &span.entries, &paths, &mut kv, &mut stats);
                    tokens.truncate(resource_tokens.len() + scope_tokens.len());
                    tokens.extend_from_slice(&span_tokens);

                    // ts = start_time, normalized at ingest (always concrete).
                    row_index.row(span.ts, &tokens);

                    // Per-row span columns, one value per row (parallel to the row
                    // just fed) so they stay aligned for the build-time remap. Ids
                    // are byte-copied from the typed ng_flatten ids to the sfst ones.
                    trace_ids.push(TraceId::from(*span.trace_id.as_bytes()));
                    span_ids.push(SpanId::from(*span.span_id.as_bytes()));
                    parent_span_ids.push(SpanId::from(*span.parent_span_id.as_bytes()));
                    durations.push(span.duration);
                    flags.push(span.flags);
                    dropped_attrs.push(span.dropped_attributes_count);
                }

                tokens.truncate(resource_tokens.len());
            }
        }
        stats.records += records;
        metrics.add_records(records);
    }

    row_index.tree = Some(to_sfst_tree(&flattener.into_tree()));
    row_index.trace_ids = Some(trace_ids);
    row_index.span_ids = Some(span_ids);
    row_index.parent_span_ids = Some(parent_span_ids);
    row_index.durations = Some(Durations(durations));
    row_index.flags = Some(Flags(flags));
    row_index.dropped_attribute_counts = Some(DroppedAttributeCounts(dropped_attrs));
    // Spans turn the index on: the builder builds TIDX from the chronological TRCE.
    row_index.build_trace_id_index = true;

    Ok(stats)
}

/// Build an SFST index file from a single flattened **traces** WAL file — the traces
/// analog of [`build_sfst_file`]. Populates the span per-row columns and builds the
/// `trace_id` index; returns the [`sfst::Summary`] + written file size.
pub fn build_sfst_traces_file(
    wal_path: &Path,
    out_path: &Path,
    metrics: &Metrics,
) -> Result<(sfst::Summary, u64), Error> {
    let mut reader = wal::Reader::open(wal_path)?;
    let content_meta = reader.header().content_meta.clone();
    let arena = Bump::new();
    let mut row_index = RowIndex::new(&arena, CARDINALITY_THRESHOLD);
    populate_trace_row_index(&mut reader, &mut row_index, metrics)?;
    let _t = metrics.scope("build");
    let (summary, _metadata) = IndexWriter::write_file(&row_index, out_path, content_meta)?;
    let size = std::fs::metadata(out_path)?.len();
    Ok((summary, size))
}

#[cfg(test)]
mod tests {
    use super::*;
    use ng_flatten::flatten_log_request;
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
                        values: vec![AnyValue {
                            value: Some(Av::StringValue("a".into())),
                        }],
                    })),
                },
            ],
            ..Default::default()
        };
        ExportLogsServiceRequest {
            resource_logs: vec![ResourceLogs {
                scope_logs: vec![ScopeLogs {
                    log_records: vec![rec],
                    ..Default::default()
                }],
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
        let (flattened, _) = flatten_log_request(nested_request());
        let ng_tree = &flattened.tree;
        let sfst_tree = to_sfst_tree(ng_tree);

        assert_eq!(
            sfst_tree.len(),
            ng_tree.len(),
            "node count must be preserved"
        );
        assert!(
            ng_tree.len() > 3,
            "fixture should produce a non-trivial tree"
        );
        let mut saw_array = false;
        for id in 0..ng_tree.len() as NodeId {
            let expected = ng_tree.path(id);
            saw_array |= expected.contains("[]");
            assert_eq!(sfst_tree.path(id), expected, "path mismatch at node {id}");
        }
        assert!(saw_array, "fixture must exercise an ArrayElem ([]) step");
    }

    /// A hash collision must NOT alias distinct strings. Two entries carry different
    /// values but the same (adversarially equal) precomputed hash; `intern_entries`
    /// must re-check the string on the `lookup_hash` hit and intern them separately.
    /// Without the re-check, the second would silently take the first's slot.
    #[test]
    fn intern_entries_rechecks_string_on_hash_collision() {
        use ng_flatten::Value;

        let arena = Bump::new();
        let mut ri = RowIndex::new(&arena, CARDINALITY_THRESHOLD);
        let paths = vec!["k".to_string()];
        let entries = vec![
            Entry {
                node: 0,
                value: Value::Int(1),
                hash: 42,
            },
            Entry {
                node: 0,
                value: Value::Int(2),
                hash: 42,
            }, // distinct value, same hash
        ];
        let mut kv = String::new();
        let mut stats = SfstStats::default();
        let tokens = intern_entries(&mut ri, &entries, &paths, &mut kv, &mut stats);

        assert_ne!(
            tokens[0], tokens[1],
            "colliding-hash distinct strings must not alias to one slot"
        );
        assert_eq!(ri.resolve(tokens[0]), "k=1");
        assert_eq!(ri.resolve(tokens[1]), "k=2");
        // First is a genuine miss; second hit the hash but failed the string re-check.
        assert_eq!((stats.hits, stats.misses), (0, 2));
    }
}
