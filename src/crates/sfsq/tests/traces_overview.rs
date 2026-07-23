//! Integration suite for the traces overview (span-density grid): exact
//! counts over a known distribution, sealed-vs-tail parity, shard-merge
//! associativity (mixing source shapes changes nothing), ceiling
//! termination, cancellation, and per-source failure honesty.

mod common;

use std::sync::Arc;
use std::sync::atomic::AtomicUsize;

use tokio_util::sync::CancellationToken;

use common::{req, sp, sealed_source, tail_source, write_wal};
use sfsq::traces::{
    DURATION_BIN_COUNT, OverviewData, OverviewQuery, PartialReason, QueryStatus, TraceSource,
    overview,
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

/// The known corpus: spans at (start_s, duration) →
///  - t=1s, 500µs  → bucket 1, bin 0 (<1ms)
///  - t=1s, 5ms    → bucket 1, bin 1 (1-10ms)
///  - t=3s, 50ms   → bucket 3, bin 2 (10-100ms)
///  - t=3s, 2s     → bucket 3, bin 4 (1-10s)  [ERROR status]
///  - t=8s, 20s    → bucket 8, bin 5 (>10s)
///  - t=20s (outside the grid — clipped)
fn corpus() -> Vec<common::SpanSpec> {
    let s = |id: u8, start_ns: u64, dur_ns: u64| {
        let mut s = sp(id, 0, start_ns, "op");
        s.end = start_ns + dur_ns;
        s
    };
    let mut error_span = s(4, 3_000_000_000, 2_000_000_000);
    error_span.status = Some((2, "boom")); // OTLP STATUS_CODE_ERROR
    vec![
        s(1, 1_000_000_000, 500_000),
        s(2, 1_000_000_000, 5_000_000),
        s(3, 3_000_000_000, 50_000_000),
        error_span,
        s(5, 8_000_000_000, 20_000_000_000),
        s(6, 20_000_000_000, 1_000),
    ]
}

fn expected_cells() -> Vec<[u64; DURATION_BIN_COUNT]> {
    let mut cells = vec![[0u64; DURATION_BIN_COUNT]; 10];
    cells[1][0] = 1;
    cells[1][1] = 1;
    cells[3][2] = 1;
    cells[3][4] = 1;
    cells[8][5] = 1;
    cells
}

#[test]
fn sealed_grid_matches_the_known_distribution_exactly() {
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), vec![req(&corpus())], "sealed");
    let data = run(
        vec![sealed_source(dir.path(), &wal, "a")],
        OverviewQuery::new(grid()),
    );
    assert_eq!(data.status, QueryStatus::Complete);
    assert_eq!(data.cells, expected_cells());
    assert_eq!(data.total_spans, 5, "the out-of-grid span is clipped");
    assert_eq!(data.total_errors, 1);
    let cell_sum: u64 = data.cells.iter().flatten().sum();
    assert_eq!(cell_sum, data.total_spans, "cells and totals agree");
}

#[test]
fn tail_parity_with_sealing_the_same_data() {
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), vec![req(&corpus())], "parity");
    let sealed = run(
        vec![sealed_source(dir.path(), &wal, "a")],
        OverviewQuery::new(grid()),
    );
    let tail = run(vec![tail_source(&wal, "t")], OverviewQuery::new(grid()));
    assert_eq!(sealed.cells, tail.cells);
    assert_eq!(sealed.total_spans, tail.total_spans);
    assert_eq!(sealed.total_errors, tail.total_errors);
}

#[test]
fn shard_merge_is_associative_across_source_shapes() {
    // The corpus split across a sealed file AND a tail must equal
    // sealing everything as one source — cell-wise sums, order-free.
    let (half_a, half_b) = {
        let c = corpus();
        (c[..3].to_vec(), c[3..].to_vec())
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
    assert_eq!(mixed.total_spans, whole.total_spans);
    assert_eq!(mixed.total_errors, whole.total_errors);
}

#[test]
fn ceiling_terminates_with_the_deterministic_prefix_and_the_partial() {
    // Two sources; ceiling 1: source 1 (SourceId order) processes and
    // overshoots the budget, source 2 never runs.
    let dir = tempfile::tempdir().unwrap();
    let wal_a = write_wal(dir.path(), vec![req(&corpus()[..3])], "a");
    let wal_b = write_wal(dir.path(), vec![req(&corpus()[3..])], "b");
    let data = run(
        vec![
            sealed_source(dir.path(), &wal_a, "1-first"),
            sealed_source(dir.path(), &wal_b, "2-second"),
        ],
        OverviewQuery::new(grid()).visited_rows_ceiling_for_tests(1),
    );
    assert!(data.status.has(PartialReason::OverviewCeiling));
    assert_eq!(data.total_spans, 3, "only the first source counted");
    let cell_sum: u64 = data.cells.iter().flatten().sum();
    assert_eq!(cell_sum, 3, "the gathered grid holds exactly the prefix");
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
    assert_eq!(data.total_spans, 0);
    assert!(data.cells.iter().flatten().all(|&c| c == 0));
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
    assert_eq!(data.total_spans, 5, "the healthy source still counts");
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

#[test]
fn cancelled_zero_source_call_still_reports_cancelled() {
    // The sibling-engine contract: a zero-source or already-cancelled
    // call can never report Complete — the up-front poll guarantees it
    // even when the source loop never runs.
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
