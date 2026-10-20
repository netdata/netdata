//! Tests for [`IndexReader::trace_by_id`], the single-file entry into the
//! shared trace combiner (`trace_combine.rs`, driven through the per-file
//! session in session.rs) — exercised end to end over in-memory SFST files
//! this module builds with the crate's own writer (see `trace_file`).
//!
//! Pins:
//!
//! - a corrupt `TBLM` bloom degrades to the exact `TIDX` lookup instead of
//!   hiding a findable trace; the raw accessor still surfaces the error;
//! - an id absent from the file resolves to the empty trace, not an error;
//! - dedup: duplicate `(span_id, kind)` rows collapse to one span, UNSET
//!   span ids never do;
//! - graph: unset or absent-from-set parents are roots; `children` is
//!   node-index adjacency parallel to `spans`;
//! - a parent cycle with no external entry stays a walkable forest: a
//!   root is promoted, and walkers must guard revisits (`children` keeps
//!   the cycle edges).
//!
//! Not pinned here: attribute facets, events, and links materialization —
//! fixture files carry empty attribute fields on purpose.

use crate::writer::{ChunkCounts, ChunkWriter, ColumnsPresent};
use crate::{
    BitmapValue, ColumnEntry, ColumnsTable, DroppedAttributeCounts, Durations, Flags, Histogram,
    IdRanges, IndexReader, KvId, Metadata, ParentSpanIds, SpanId, SpanIds, StreamBatch, Summary,
    TraceId, TraceIdIndex, TraceIds,
};

const TRACE: [u8; 16] = [7u8; 16];

fn sid(b: u8) -> SpanId {
    SpanId::from([b; 8])
}

/// Build a minimal traces SFST whose rows all share one `trace_id`, from
/// `(span_id, parent_span_id)` pairs given in row order (ascending
/// timestamps are assigned, so row order == start order). Attribute
/// fields stay empty — the tests exercise ids, parents, and row scalars,
/// not facets.
fn trace_file(rows: &[(SpanId, SpanId)]) -> Vec<u8> {
    trace_file_with_bloom(rows, None)
}

/// Like [`trace_file`], optionally writing a caller-supplied `TBLM`
/// (trace-id bloom) payload — which may be deliberately malformed: the
/// writer packs it as-is, validation happens on read.
fn trace_file_with_bloom(
    rows: &[(SpanId, SpanId)],
    bloom: Option<&crate::TraceIdBloom>,
) -> Vec<u8> {
    let n = rows.len();
    let mut trace = TraceIds::with_capacity(n);
    let mut span = SpanIds::with_capacity(n);
    let mut parent = ParentSpanIds::with_capacity(n);
    for &(s, p) in rows {
        trace.push(TraceId::from(TRACE));
        span.push(s);
        parent.push(p);
    }
    let flags = Flags(vec![0u32; n]);
    let drac = DroppedAttributeCounts(vec![0u32; n]);
    let durations = Durations(vec![0i64; n]);
    let index = TraceIdIndex::build(&trace);

    let columns = ColumnsPresent {
        observed_ts: false,
        trace_id: true,
        span_id: true,
        flags: true,
        dropped_attributes_count: true,
        parent_span_id: true,
        duration: true,
    };
    let counts = ChunkCounts {
        columns,
        trace_id_index: true,
        trace_id_bloom: bloom.is_some(),
        event_index: false,
        link_index: false,
        trace_rollup: false,
        mid_fields: 0,
        high_fields: 0,
        stream_batches: 1,
    };
    let summary = Summary {
        min_timestamp_s: 0,
        max_timestamp_s: n as u32,
        record_count: n as u32,
        content_meta: Vec::new(),
    };
    let metadata = Metadata {
        histogram: Histogram {
            timestamps: vec![0],
            counts: vec![n as u32],
        },
        id_ranges: IdRanges {
            low_end: KvId(0),
            mid_end: KvId(0),
            high_end: KvId(0),
        },
        tree: Default::default(),
        columns: ColumnsTable(vec![
            ColumnEntry {
                name: TraceIds::NAME.into(),
                ty: TraceIds::COLUMN_TYPE,
            },
            ColumnEntry {
                name: SpanIds::NAME.into(),
                ty: SpanIds::COLUMN_TYPE,
            },
            ColumnEntry {
                name: Flags::NAME.into(),
                ty: Flags::COLUMN_TYPE,
            },
            ColumnEntry {
                name: DroppedAttributeCounts::NAME.into(),
                ty: DroppedAttributeCounts::COLUMN_TYPE,
            },
            ColumnEntry {
                name: ParentSpanIds::NAME.into(),
                ty: ParentSpanIds::COLUMN_TYPE,
            },
            ColumnEntry {
                name: Durations::NAME.into(),
                ty: Durations::COLUMN_TYPE,
            },
        ]),
    };
    // One ascending timestamp per row so start-sort order == row order.
    let timestamps: Vec<i64> = (0..n as i64).collect();

    let mut w = ChunkWriter::new(std::io::Cursor::new(Vec::new()), counts).unwrap();
    w.summary(&summary).unwrap();
    w.metadata(&metadata).unwrap();
    w.timestamps(&timestamps).unwrap();
    w.primary(std::iter::empty::<(&str, BitmapValue)>())
        .unwrap();
    w.trace_ids(&trace).unwrap();
    w.span_ids(&span).unwrap();
    w.flags(&flags).unwrap();
    w.dropped_attribute_counts(&drac).unwrap();
    w.parent_span_ids(&parent).unwrap();
    w.durations(&durations).unwrap();
    w.trace_id_index(&index).unwrap();
    if let Some(b) = bloom {
        w.trace_id_bloom(b).unwrap();
    }
    w.add_stream_batch(&StreamBatch::for_write(&vec![Vec::<KvId>::new(); n]))
        .unwrap();
    w.finish().unwrap().into_inner()
}

#[test]
fn corrupt_bloom_degrades_to_the_exact_lookup() {
    // The bloom is a skip hint: a TBLM that decodes but fails validation
    // (absurd hash count here) must NOT make a findable trace unfindable —
    // trace_by_id falls through to TIDX. The accessor itself still errors,
    // so cross-file callers can observe the corruption.
    let hostile = crate::TraceIdBloom::raw_for_tests(
        1,
        fastbloom::BloomFilter::from_vec(vec![0u64; 4])
            .seed(&1)
            .hashes(1_000),
    );
    let buf = trace_file_with_bloom(&[(sid(1), SpanId::from([0; 8]))], Some(&hostile));
    let reader = IndexReader::open(&buf).unwrap();

    assert!(reader.has_trace_id_bloom());
    assert!(
        reader.trace_id_bloom().is_err(),
        "accessor surfaces corruption"
    );
    let trace = reader.trace_by_id(TraceId::from(TRACE)).unwrap();
    assert_eq!(trace.spans.len(), 1, "lookup degraded to TIDX and resolved");
}

#[test]
fn absent_trace_yields_empty() {
    // An id the TIDX doesn't carry is not an error: trace_by_id resolves
    // it to the empty trace (no spans, no roots). No bloom in this
    // fixture, so the miss comes from the exact lookup.
    let buf = trace_file(&[(sid(1), SpanId::from([0; 8]))]);
    let reader = IndexReader::open(&buf).unwrap();
    let trace = reader.trace_by_id(TraceId::from([0xEE; 16])).unwrap();
    assert!(trace.spans.is_empty());
    assert!(trace.roots.is_empty());
}

#[test]
fn duplicate_span_id_is_collapsed_to_first() {
    let unset = SpanId::from([0; 8]);
    // span A sent twice (a resend), then span B — all roots (unset parents).
    let buf = trace_file(&[(sid(1), unset), (sid(1), unset), (sid(2), unset)]);
    let reader = IndexReader::open(&buf).unwrap();
    let trace = reader.trace_by_id(TraceId::from(TRACE)).unwrap();

    // Both A rows share the dedup key (span_id, kind) → one span; B is a
    // distinct key.
    assert_eq!(trace.spans.len(), 2);
    assert_eq!(trace.roots.len(), 2);
    // `children` is node-index adjacency parallel to `spans`.
    assert!(trace.children.iter().all(|kids| kids.is_empty()));
}

#[test]
fn unset_span_ids_are_not_collapsed() {
    let unset = SpanId::from([0; 8]);
    // Two rows both with an UNSET span_id are distinct spans, not one resend.
    let buf = trace_file(&[(unset, unset), (unset, unset)]);
    let reader = IndexReader::open(&buf).unwrap();
    let trace = reader.trace_by_id(TraceId::from(TRACE)).unwrap();
    assert_eq!(trace.spans.len(), 2);
    assert_eq!(trace.roots.len(), 2);
}

#[test]
fn parent_edges_and_missing_parent_root() {
    let unset = SpanId::from([0; 8]);
    // A (unset parent → root), B (child of A), C (parent X not in this
    // file's span set → root: a partial trace still forms a forest).
    let buf = trace_file(&[(sid(1), unset), (sid(2), sid(1)), (sid(3), sid(9))]);
    let reader = IndexReader::open(&buf).unwrap();
    let trace = reader.trace_by_id(TraceId::from(TRACE)).unwrap();

    assert_eq!(trace.spans.len(), 3);
    assert_eq!(trace.roots.len(), 2);
    let a_idx = trace
        .spans
        .iter()
        .position(|s| s.span_id == sid(1))
        .unwrap();
    let a_children = &trace.children[a_idx];
    assert_eq!(a_children.len(), 1);
    assert_eq!(trace.spans[a_children[0]].span_id, sid(2));
}

#[test]
fn parent_cycle_stays_a_forest_and_terminates() {
    // A's parent is B, B's parent is A — a 2-node cycle with no external entry.
    // The reachability guard must promote a root so every span is reachable,
    // and must not loop forever.
    let buf = trace_file(&[(sid(1), sid(2)), (sid(2), sid(1))]);
    let reader = IndexReader::open(&buf).unwrap();
    let trace = reader.trace_by_id(TraceId::from(TRACE)).unwrap();

    assert_eq!(trace.spans.len(), 2);
    // At least one span is promoted to a root so the forest is walkable.
    assert!(!trace.roots.is_empty());

    // The cycle's edges remain in `children`, so the walk guards against
    // revisits — and still reaches every span from the promoted root(s).
    let mut seen = vec![false; trace.spans.len()];
    let mut stack = trace.roots.clone();
    while let Some(i) = stack.pop() {
        if seen[i] {
            continue;
        }
        seen[i] = true;
        stack.extend(trace.children[i].iter().copied().filter(|&c| !seen[c]));
    }
    assert!(seen.iter().all(|&s| s), "every span reachable from a root");
}
