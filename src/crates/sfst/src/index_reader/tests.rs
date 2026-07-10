//! Tests for `IndexReader::trace_by_id` trace reconstruction: span dedup,
//! root detection, and the cycle -> forest reachability guard.

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

/// Build a minimal traces SFST whose rows all share one `trace_id`, from a list
/// of `(span_id, parent_span_id)` pairs in chronological (row) order. Fields are
/// empty (the tree logic under test reads ids/timestamps, not attributes).
fn trace_file(rows: &[(SpanId, SpanId)]) -> Vec<u8> {
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
    w.add_stream_batch(&StreamBatch::for_write(&vec![Vec::<KvId>::new(); n]))
        .unwrap();
    w.finish().unwrap().into_inner()
}

#[test]
fn absent_trace_yields_empty() {
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

    // The two A rows collapse to one span; B remains → 2 spans, both roots.
    assert_eq!(trace.spans.len(), 2);
    assert_eq!(trace.roots.len(), 2);
    assert!(trace.children.is_empty());
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
    // A(root), B(child of A), C(parent = X which is absent from this file → root).
    let buf = trace_file(&[(sid(1), unset), (sid(2), sid(1)), (sid(3), sid(9))]);
    let reader = IndexReader::open(&buf).unwrap();
    let trace = reader.trace_by_id(TraceId::from(TRACE)).unwrap();

    assert_eq!(trace.spans.len(), 3);
    // A and C are roots; B is A's child.
    assert_eq!(trace.roots.len(), 2);
    let a_children = trace.children.get(&sid(1)).expect("A has children");
    assert_eq!(a_children.len(), 1);
    // The edge points at B.
    let child_idx = a_children[0];
    assert_eq!(trace.spans[child_idx].span_id, sid(2));
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

    // Every span is reachable from some root via a revisit-guarded walk.
    let mut seen = vec![false; trace.spans.len()];
    let mut stack = trace.roots.clone();
    while let Some(i) = stack.pop() {
        if seen[i] {
            continue;
        }
        seen[i] = true;
        if let Some(kids) = trace.children.get(&trace.spans[i].span_id) {
            stack.extend(kids.iter().copied().filter(|&c| !seen[c]));
        }
    }
    assert!(seen.iter().all(|&s| s), "every span reachable from a root");
}
