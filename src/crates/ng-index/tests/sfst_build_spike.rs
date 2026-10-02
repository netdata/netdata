//! Format-layer spike for ng-index's SFST build: feed plain
//! `(timestamp_ns, [key=value])` rows — the stringified shape the seal hands
//! `sfst` (ng-index/src/sfst_build.rs) — to `RowIndex` + `IndexWriter`, then
//! query the file back through `IndexReader`. Two contracts:
//!
//! - log-shaped rows build from out-of-order timestamps and count correctly
//!   under exact, regex and AND filters;
//! - with the span columns set and `build_trace_id_index` on, every column
//!   lands in the file chronological and row-aligned, and the `TIDX`
//!   resolves trace ids against that same column.
//!
//! Both tests use `RowIndex`'s inherent `intern` + `row` with hash `None`
//! (string-keyed dedup — the producer-hash fast path `sfst_build.rs` uses is
//! not exercised here). The full pipelines live one layer up: logs in
//! `ng-index/src/sfst_build.rs`, traces in `ng-index/tests/traces_seal.rs`.

use bumpalo::Bump;
use sfst::query::Filter;
use sfst::{
    DroppedAttributeCounts, Durations, Flags, IndexReader, ParentSpanIds, SpanId, SpanIds, TraceId,
    TraceIds,
};
use sfst::{IndexWriter, RowIndex};

/// Build an SFST file from `(timestamp_ns, &[key=value])` rows; return its bytes.
fn build_sfst(rows: &[(i64, &[&str])]) -> Vec<u8> {
    let arena = Bump::new();
    let mut ri = RowIndex::new(&arena, 100);
    for &(ts, kvs) in rows {
        // hash=None: the interner dedups by the full string.
        let tokens: Vec<_> = kvs.iter().map(|&kv| ri.intern(None, kv)).collect();
        ri.row(ts, &tokens);
    }
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("spike.sfst");
    // Empty content_meta: an opaque producer blob stored verbatim; never read here.
    IndexWriter::write_file(&ri, &path, Vec::new()).expect("write_file");
    std::fs::read(&path).expect("read sfst")
}

/// Count records matching `filter` across all time.
fn count(reader: &IndexReader, filter: Filter) -> u64 {
    let bf = reader
        .compile_filter(&filter, None)
        .expect("compile_filter");
    reader
        .matched_count(&bf, i64::MIN..i64::MAX)
        .expect("matched_count")
}

#[test]
fn build_sfst_from_our_rows_and_query_back() {
    // Timestamps deliberately out of arrival order: the build's time-sort must
    // make the counts independent of insertion order.
    let rows: &[(i64, &[&str])] = &[
        (
            300,
            &["service=checkout", "http.method=GET", "http.status=200"],
        ),
        (
            100,
            &["service=checkout", "http.method=POST", "http.status=500"],
        ),
        (
            200,
            &["service=billing", "http.method=GET", "http.status=200"],
        ),
    ];

    let bytes = build_sfst(rows);
    let reader = IndexReader::open(&bytes).expect("open sfst");

    // Empty filter = every record.
    assert_eq!(count(&reader, Filter::new()), 3);

    // Exact matches.
    assert_eq!(
        count(&reader, Filter::new().select("service", "checkout")),
        2
    );
    assert_eq!(
        count(&reader, Filter::new().select("http.method", "GET")),
        2
    );
    assert_eq!(
        count(&reader, Filter::new().select("http.status", "500")),
        1
    );

    // Full-value regex (anchored ^(?:5..)$ -> matches "500").
    assert_eq!(
        count(&reader, Filter::new().select_pattern("http.status", "5..")),
        1
    );

    // AND across fields: checkout AND GET -> only the ts=300 row.
    let both = Filter::new()
        .select("service", "checkout")
        .select("http.method", "GET");
    assert_eq!(count(&reader, both), 1);
}

/// Pins the traces wiring on `RowIndex` at the format layer: the span
/// columns (`trace_ids`/`span_ids`/`parent_span_ids`/`durations`/`flags`/
/// `dropped_attribute_counts`) plus the `build_trace_id_index` flag must
/// flow through the build into one valid file — each column written
/// chronologically and row-aligned, and a `TIDX` built from the
/// chronological `trace_id` column. The logs seal leaves all of these
/// dormant. Only what is asserted is pinned: span ids / flags / dropped
/// counts are written but not read back, and the `TBLM` bloom
/// (`build_trace_id_bloom`) stays off. The full traces pipeline has its own
/// oracle in `ng-index/tests/traces_seal.rs`.
#[test]
fn build_into_writes_span_columns_and_trace_id_index() {
    let a = TraceId::from([0xAA; 16]);
    let b = TraceId::from([0xBB; 16]);
    // Timestamps out of arrival order so the build-time time-sort reorders the
    // columns; two rows share trace A, one is trace B.
    let rows: &[(i64, TraceId, [u8; 8], i64)] = &[
        (300, a, [0x01; 8], 30), // insertion row 0
        (100, b, [0x02; 8], 10), // insertion row 1
        (200, a, [0x03; 8], 20), // insertion row 2
    ];

    let arena = Bump::new();
    let mut ri = RowIndex::new(&arena, 100);
    let mut trace_ids = TraceIds::default();
    let mut span_ids = SpanIds::default();
    let mut parents = ParentSpanIds::default();
    let mut durations = Vec::new();
    let mut flags = Vec::new();
    let mut dropped = Vec::new();
    for (i, &(ts, tid, sid, dur)) in rows.iter().enumerate() {
        let tokens = vec![ri.intern(None, "service.name=api")];
        ri.row(ts, &tokens);
        trace_ids.push(tid);
        span_ids.push(SpanId::from(sid));
        parents.push(SpanId::UNSET); // root spans
        durations.push(dur);
        flags.push(i as u32);
        dropped.push(0u32);
    }
    ri.trace_ids = Some(trace_ids);
    ri.span_ids = Some(span_ids);
    ri.parent_span_ids = Some(parents);
    ri.durations = Some(Durations(durations));
    ri.flags = Some(Flags(flags));
    ri.dropped_attribute_counts = Some(DroppedAttributeCounts(dropped));
    ri.build_trace_id_index = true;

    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("spans.sfst");
    IndexWriter::write_file(&ri, &path, Vec::new()).expect("write_file");
    let bytes = std::fs::read(&path).expect("read sfst");

    let reader = IndexReader::open(&bytes).expect("open sfst");

    // Traces-only columns decode back present and row-aligned (len == row count).
    let trace_ids = reader.trace_ids().expect("trace_ids column");
    let durations = reader.durations().expect("durations column");
    let parents = reader.parent_span_ids().expect("parent_span_ids column");
    assert_eq!(trace_ids.len(), 3);
    assert_eq!(parents.len(), 3);
    // Durations come back sorted by timestamp (ts 100/200/300 -> [10, 20, 30]),
    // not insertion order: every column is remapped to chronological position.
    assert_eq!(durations.0, vec![10, 20, 30]);

    // The TIDX is present and its positions resolve rows of the chronological
    // `trace_id` column — the same order every column is written in.
    assert!(reader.has_trace_id_index());
    let idx = reader.trace_id_index().expect("trace_id_index");
    let pa = idx.positions(a, &trace_ids);
    let pb = idx.positions(b, &trace_ids);
    assert_eq!(pa.len(), 2, "trace A spans two rows");
    assert_eq!(pb.len(), 1, "trace B spans one row");
    for &p in pa {
        assert_eq!(trace_ids.get(p as usize), a);
    }
    assert_eq!(trace_ids.get(pb[0] as usize), b);
}
