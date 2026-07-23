//! Integration suite for the TRACE-level overview (phase-2 v2): exact
//! trace-density grids over known multi-trace corpora, the D7 straddle
//! (one trace across sources counts once, envelope merged), the D9
//! resend divergence from canonical assembly, the D10 legacy exclusion
//! (a pre-rollup file is flagged, never mixed in), shard-merge
//! associativity, ceiling termination, cancellation, and per-source
//! failure honesty.

mod common;

use std::sync::Arc;
use std::sync::atomic::AtomicUsize;

use tokio_util::sync::CancellationToken;

use common::{req, sp, sealed_source, tail_source, write_wal};
use sfsq::traces::{
    DURATION_BIN_COUNT, OverviewData, OverviewQuery, PartialReason, QueryStatus, TraceQuery,
    TraceSource, overview, trace_by_id,
};

/// One second-wide bucket per second, 10 buckets from t=0.
fn grid() -> sfst::Grid {
    sfst::Grid::new(0, 1_000_000_000, 10)
}

fn run(sources: Vec<TraceSource>, query: OverviewQuery) -> OverviewData {
    overview(
        sources,
        query,
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap()
}

/// A span of trace `t` with an explicit envelope contribution.
fn tspan(t: u8, id: u8, start_ns: u64, end_ns: u64, name: &'static str) -> common::SpanSpec {
    let mut s = sp(id, 0, start_ns, name);
    s.trace = [t; 16];
    s.end = end_ns;
    s
}

/// Three traces with distinct envelopes:
///  - A: spans [1.0s..1.2s] and [1.1s..1.4s] → bucket 1, dur 400ms → bin 3
///  - B: one ERROR span [3.0s..3.000_000_5s] → bucket 3, 500ns → bin 0
///  - C: spans [5.0s..5.1s] and [5.05s..17s] → bucket 5, 12s → bin 5
fn corpus() -> Vec<common::SpanSpec> {
    let mut b = tspan(0xB, 3, 3_000_000_000, 3_000_000_500, "b-err");
    b.status = Some((2, "boom"));
    vec![
        tspan(0xA, 1, 1_000_000_000, 1_200_000_000, "a-1"),
        tspan(0xA, 2, 1_100_000_000, 1_400_000_000, "a-2"),
        b,
        tspan(0xC, 4, 5_000_000_000, 5_100_000_000, "c-1"),
        tspan(0xC, 5, 5_050_000_000, 17_000_000_000, "c-2"),
    ]
}

fn expected_cells() -> Vec<[u64; DURATION_BIN_COUNT]> {
    let mut cells = vec![[0u64; DURATION_BIN_COUNT]; 10];
    cells[1][3] = 1; // A: 400ms
    cells[3][0] = 1; // B: 500ns
    cells[5][5] = 1; // C: 12s
    cells
}

#[test]
fn sealed_grid_counts_traces_by_merged_envelope() {
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), vec![req(&corpus())], "sealed");
    let data = run(
        vec![sealed_source(dir.path(), &wal, "a")],
        OverviewQuery::new(grid()),
    );
    assert_eq!(data.status, QueryStatus::Complete);
    assert_eq!(data.cells, expected_cells());
    assert_eq!(data.total_traces, 3);
    assert_eq!(data.total_spans, 5, "stored spans of the binned traces");
    assert_eq!(data.total_errors, 1);
    let cell_sum: u64 = data.cells.iter().flatten().sum();
    assert_eq!(cell_sum, data.total_traces, "cells count traces");
}

#[test]
fn straddling_trace_counts_once_with_the_merged_envelope() {
    // Trace A's two spans land in DIFFERENT sources (a sealed file and a
    // tail) — D7: one merged trace, envelope spanning both parts.
    let dir = tempfile::tempdir().unwrap();
    let wal_1 = write_wal(
        dir.path(),
        vec![req(&[tspan(0xA, 1, 1_000_000_000, 1_200_000_000, "a-1")])],
        "part1",
    );
    let wal_2 = write_wal(
        dir.path(),
        vec![req(&[tspan(0xA, 2, 1_100_000_000, 1_400_000_000, "a-2")])],
        "part2",
    );
    let data = run(
        vec![
            sealed_source(dir.path(), &wal_1, "s1"),
            tail_source(&wal_2, "t2"),
        ],
        OverviewQuery::new(grid()),
    );
    assert_eq!(data.total_traces, 1, "straddling counts ONCE");
    assert_eq!(data.total_spans, 2);
    // Merged envelope: 1.0s..1.4s → 400ms → bucket 1, bin 3.
    let mut expected = vec![[0u64; DURATION_BIN_COUNT]; 10];
    expected[1][3] = 1;
    assert_eq!(data.cells, expected);
}

#[test]
fn resend_counts_stored_rows_where_assembly_dedups() {
    // The D9 divergence, shown side by side: the SAME span stored twice
    // counts twice in the overview's stored-row totals, while assembly
    // (trace_by_id over the same source) dedups it canonically.
    let dir = tempfile::tempdir().unwrap();
    let a = tspan(0xA, 1, 1_000_000_000, 1_200_000_000, "a-1");
    let wal = write_wal(dir.path(), vec![req(&[a.clone(), a])], "resend");
    let sources = vec![sealed_source(dir.path(), &wal, "s")];

    let data = run(sources.clone_sources(), OverviewQuery::new(grid()));
    assert_eq!(data.total_traces, 1);
    assert_eq!(data.total_spans, 2, "stored rows — the resend counts (D9)");

    let tr = trace_by_id(
        sources,
        TraceQuery::new(sfst::TraceId::from([0xA; 16])),
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap();
    assert_eq!(tr.trace.spans.len(), 1, "assembly dedups the resend");
}

/// The overview consumes sources by value; tests that need two runs
/// rebuild the same set. (Sealed sources are cheap descriptors.)
trait CloneSources {
    fn clone_sources(&self) -> Vec<TraceSource>;
}
impl CloneSources for Vec<TraceSource> {
    fn clone_sources(&self) -> Vec<TraceSource> {
        self.iter()
            .map(|s| match s {
                TraceSource::Sfst(c) => TraceSource::Sfst(sfsq::traces::TraceSfstCandidate {
                    source_id: c.source_id.clone(),
                    summary: c.summary.clone(),
                    source: c.source.clone(),
                    coverage: c.coverage.clone(),
                }),
                TraceSource::Tail(t) => TraceSource::Tail(sfsq::traces::TraceWalTail {
                    source_id: t.source_id.clone(),
                    path: t.path.clone(),
                    coverage: t.coverage.clone(),
                }),
            })
            .collect()
    }
}

#[test]
fn legacy_file_without_the_rollup_is_excluded_and_flagged_never_mixed() {
    // A hand-built pre-rollup ("legacy") SFST beside a modern sealed
    // file: D10 — the legacy file's traces never leak into trace-level
    // numbers; the exclusion is flagged.
    let dir = tempfile::tempdir().unwrap();
    let modern_wal = write_wal(dir.path(), vec![req(&corpus())], "modern");
    let modern = sealed_source(dir.path(), &modern_wal, "modern");

    // Minimal valid SFST WITHOUT a TRSU chunk (the raw chunk writer).
    let legacy_path = dir.path().join("legacy.sfst");
    {
        let counts = sfst::ChunkCounts {
            columns: sfst::ColumnsPresent::default(),
            trace_id_index: false,
            trace_id_bloom: false,
            event_index: false,
            link_index: false,
            trace_rollup: false,
            mid_fields: 0,
            high_fields: 0,
            stream_batches: 1,
        };
        let summary = sfst::Summary {
            min_timestamp_s: 0,
            max_timestamp_s: 10,
            record_count: 1,
            content_meta: Vec::new(),
        };
        let metadata = sfst::Metadata {
            histogram: sfst::Histogram {
                timestamps: vec![0],
                counts: vec![1],
            },
            id_ranges: sfst::IdRanges {
                low_end: sfst::KvId(1),
                mid_end: sfst::KvId(1),
                high_end: sfst::KvId(1),
            },
            tree: sfst::SchemaTree::flat(
                &vec![sfst::FieldEntry {
                    name: "name".into(),
                    cardinality: 1,
                    tier: sfst::FieldTier::Low,
                }]
                .into(),
            ),
            columns: sfst::ColumnsTable::default(),
        };
        let mut w =
            sfst::ChunkWriter::new(std::io::Cursor::new(Vec::new()), counts).unwrap();
        w.summary(&summary).unwrap();
        w.metadata(&metadata).unwrap();
        w.timestamps(&[1_000_000_000]).unwrap();
        w.primary(vec![("name=legacy", {
            let mut data = Vec::new();
            let desc = treight::Bitmap::from_sorted_iter([0u32].into_iter(), 1, &mut data);
            sfst::BitmapValue { desc, data }
        })])
        .unwrap();
        w.add_stream_batch(&sfst::StreamBatch::for_write(&[vec![sfst::KvId(0)]]))
            .unwrap();
        let bytes = w.finish().unwrap().into_inner();
        std::fs::write(&legacy_path, &bytes).unwrap();
    }
    let legacy_summary = sfst::read_summary_path(&legacy_path).unwrap();
    let legacy = TraceSource::Sfst(sfsq::traces::TraceSfstCandidate {
        source_id: sfsq::traces::SourceId::new("legacy"),
        summary: legacy_summary,
        source: sfsq::Source::File(legacy_path),
        coverage: None,
    });

    let data = run(vec![modern, legacy], OverviewQuery::new(grid()));
    assert!(data.status.has(PartialReason::RollupAbsent));
    assert_eq!(data.total_traces, 3, "only the modern file's traces count");
    assert_eq!(data.cells, expected_cells(), "nothing leaked from legacy");
}

#[test]
fn shard_merge_is_associative_across_source_shapes() {
    let (half_a, half_b) = {
        let c = corpus();
        (c[..2].to_vec(), c[2..].to_vec())
    };
    let dir = tempfile::tempdir().unwrap();
    let wal_a = write_wal(dir.path(), vec![req(&half_a)], "a");
    let wal_b = write_wal(dir.path(), vec![req(&half_b)], "b");
    let wal_all = write_wal(dir.path(), vec![req(&corpus())], "all");

    let mixed = run(
        vec![
            sealed_source(dir.path(), &wal_a, "sa"),
            tail_source(&wal_b, "tb"),
        ],
        OverviewQuery::new(grid()),
    );
    let whole = run(
        vec![sealed_source(dir.path(), &wal_all, "whole")],
        OverviewQuery::new(grid()),
    );
    assert_eq!(mixed.cells, whole.cells);
    assert_eq!(mixed.total_traces, whole.total_traces);
    assert_eq!(mixed.total_spans, whole.total_spans);
    assert_eq!(mixed.total_errors, whole.total_errors);
}

#[test]
fn ceiling_terminates_with_the_deterministic_prefix_and_the_partial() {
    // Ceiling 0: source 1 (SourceId order) processes and overshoots;
    // source 2 never runs.
    let dir = tempfile::tempdir().unwrap();
    let wal_a = write_wal(dir.path(), vec![req(&corpus()[..2])], "a"); // trace A
    let wal_b = write_wal(dir.path(), vec![req(&corpus()[2..])], "b"); // traces B, C
    let data = run(
        vec![
            sealed_source(dir.path(), &wal_a, "1-first"),
            sealed_source(dir.path(), &wal_b, "2-second"),
        ],
        OverviewQuery::new(grid()).visited_rows_ceiling_for_tests(0),
    );
    assert!(data.status.has(PartialReason::OverviewCeiling));
    assert_eq!(data.total_traces, 1, "only the first source's trace");
}

#[test]
fn cancelled_call_returns_the_empty_grid_with_the_reason() {
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), vec![req(&corpus())], "c");
    let cancel = CancellationToken::new();
    cancel.cancel();
    let data = overview(
        vec![sealed_source(dir.path(), &wal, "a")],
        OverviewQuery::new(grid()),
        cancel,
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap();
    assert!(data.status.has(PartialReason::Cancelled));
    assert_eq!(data.total_traces, 0);
    assert!(data.cells.iter().flatten().all(|&c| c == 0));
}

#[test]
fn cancelled_zero_source_call_still_reports_cancelled() {
    let cancel = CancellationToken::new();
    cancel.cancel();
    let data = overview(
        vec![],
        OverviewQuery::new(grid()),
        cancel,
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap();
    assert!(data.status.has(PartialReason::Cancelled));
}

#[test]
fn a_failed_source_degrades_honestly_while_the_rest_count() {
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), vec![req(&corpus())], "ok");
    let missing = dir.path().join("gone.sfst");
    let sources = vec![
        sealed_source(dir.path(), &wal, "good"),
        TraceSource::Sfst(sfsq::traces::TraceSfstCandidate {
            source_id: sfsq::traces::SourceId::new("missing"),
            summary: sfst::Summary {
                min_timestamp_s: 0,
                max_timestamp_s: 10,
                record_count: 1,
                content_meta: Vec::new(),
            },
            source: sfsq::Source::File(missing),
            coverage: None,
        }),
    ];
    let data = run(sources, OverviewQuery::new(grid()));
    assert!(data.status.has(PartialReason::SourceFailure));
    assert_eq!(data.total_traces, 3, "the healthy source still counts");
}

#[test]
fn empty_grid_is_a_request_error() {
    let err = overview(
        vec![],
        OverviewQuery::new(sfst::Grid::new(0, 1_000_000_000, 0)),
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap_err();
    assert!(err.to_string().contains("empty"), "{err}");
}
