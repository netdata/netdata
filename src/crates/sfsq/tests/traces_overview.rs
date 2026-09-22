//! Integration suite for the TRACE-level overview: exact
//! trace-density grids over known multi-trace corpora, the cross-source
//! straddle (one trace across sources counts once, envelope merged),
//! the stored-row resend divergence from canonical assembly, the
//! legacy (pre-rollup) exclusion
//! (a pre-rollup file is flagged, never mixed in), shard-merge
//! associativity, ceiling termination, cancellation, and per-source
//! failure honesty.

mod common;

use std::sync::Arc;
use std::sync::atomic::AtomicUsize;

use tokio_util::sync::CancellationToken;

use common::{
    corrupt_chunk, kv_str, req, req_with, sealed_source, sealed_source_at, sp, tail_source,
    write_wal,
};
use sfsq::traces::{
    AttributeOwner, BuiltinField, CompareOp, Condition, DURATION_BIN_COUNT, DurationPercentiles,
    FACET_TOP_K, OverviewData, OverviewQuery, OverviewRequestError, PartialReason, Predicate,
    PredicateTarget, PredicateValue, QueryStatus, SearchQuery, SearchSources, TimeWindow,
    TraceQuery, TraceSource, overview, search, trace_by_id,
};

/// One second-wide bucket per second, 10 buckets from t=0.
fn grid() -> sfst::Grid {
    sfst::Grid::new(0, 1_000_000_000, 10)
}

/// An overflowing width x count is rejected with ITS OWN reason — not
/// the empty-grid message, which would misdiagnose a valid-looking grid.
#[test]
fn grid_overflow_is_rejected_with_a_distinct_reason() {
    let err = overview(
        Vec::new(),
        OverviewQuery::new(sfst::Grid::new(0, i64::MAX, 2)),
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap_err();
    assert!(matches!(
        err,
        sfsq::traces::OverviewRequestError::GridOverflow
    ));
    assert!(err.to_string().contains("overflows"), "{err}");
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

/// Error counts are index-parallel to `cells` — per time bucket AND per
/// duration bin — and they count STORED ERROR-SPAN rows, the statistic
/// `total_errors` reports, not "traces that failed": the cell that
/// binned ONE trace with two failed spans reads 2.
#[test]
fn error_cells_count_error_spans_per_bucket_and_duration_bin() {
    let dir = tempfile::tempdir().unwrap();
    // D: two failed spans, 3us envelope → bucket 7, bin 0.
    let mut d1 = tspan(0xD, 6, 7_000_000_000, 7_000_001_000, "d-1");
    d1.status = Some((2, "boom"));
    let mut d2 = tspan(0xD, 7, 7_000_002_000, 7_000_003_000, "d-2");
    d2.status = Some((2, "boom"));
    // F: one failed span in D's BUCKET but a 5s envelope → bin 4, so the
    // two failures never share a cell.
    let mut f = tspan(0xF, 9, 7_000_000_000, 12_000_000_000, "f-slow");
    f.status = Some((2, "boom"));
    // Past the grid's end: the envelope-start clipping drops E from the
    // cells, the totals AND the error cells.
    let mut e = tspan(0xE, 8, 12_000_000_000, 12_000_001_000, "e-clipped");
    e.status = Some((2, "boom"));
    let mut spans = corpus();
    spans.extend([d1, d2, f, e]);
    let wal = write_wal(dir.path(), vec![req(&spans)], "errs");
    let data = run(
        vec![sealed_source(dir.path(), &wal, "a")],
        OverviewQuery::new(grid()),
    );
    let mut expected = vec![[0u64; DURATION_BIN_COUNT]; 10];
    expected[3][0] = 1; // B: one failed span, 500ns
    expected[7][0] = 2; // D: two failed spans, 3us
    expected[7][4] = 1; // F: one failed span, 5s
    assert_eq!(data.error_cells, expected);
    assert_eq!(data.total_errors, 4, "E never reached the totals");
    assert_eq!(
        data.error_cells.iter().flatten().sum::<u64>(),
        data.total_errors
    );
    assert_eq!(data.cells[7][0], 1, "one trace, two failed spans");
}

#[test]
fn straddling_trace_counts_once_with_the_merged_envelope() {
    // Trace A's two spans land in DIFFERENT sources (a sealed file and a
    // tail) — one merged trace, envelope spanning both parts.
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
    // The stored-row divergence, shown side by side: the SAME span stored twice
    // counts twice in the overview's stored-row totals, while assembly
    // (trace_by_id over the same source) dedups it canonically.
    let dir = tempfile::tempdir().unwrap();
    let a = tspan(0xA, 1, 1_000_000_000, 1_200_000_000, "a-1");
    let wal = write_wal(dir.path(), vec![req(&[a.clone(), a])], "resend");
    let sources = vec![sealed_source(dir.path(), &wal, "s")];

    let data = run(sources.clone(), OverviewQuery::new(grid()));
    assert_eq!(data.total_traces, 1);
    assert_eq!(data.total_spans, 2, "stored rows — the resend counts");

    let tr = trace_by_id(
        sources,
        TraceQuery::new(sfst::TraceId::from([0xA; 16])),
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap();
    assert_eq!(tr.trace.spans.len(), 1, "assembly dedups the resend");
}

#[test]
fn legacy_file_without_the_rollup_is_excluded_and_flagged_never_mixed() {
    // A hand-built pre-rollup ("legacy") SFST beside a modern sealed
    // file: the legacy file's traces never leak into trace-level
    // numbers; the exclusion is flagged.
    let dir = tempfile::tempdir().unwrap();
    let modern_wal = write_wal(dir.path(), vec![req(&corpus())], "modern");
    let modern = sealed_source(dir.path(), &modern_wal, "modern");

    let legacy = common::legacy_sfst_source(dir.path(), "legacy");

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

// ── Per-bucket duration percentiles ─────────────────────────────────

/// Ten traces in ONE bucket with durations 1ms..10ms, plus a single
/// 42ms trace in another — a population whose nearest-rank answers are
/// hand-checkable.
fn percentile_corpus() -> Vec<common::SpanSpec> {
    let mut spans: Vec<common::SpanSpec> = (1..=10u8)
        .map(|i| {
            tspan(
                0x20 + i,
                i,
                2_000_000_000,
                2_000_000_000 + u64::from(i) * 1_000_000,
                "p",
            )
        })
        .collect();
    spans.push(tspan(0x40, 11, 5_000_000_000, 5_042_000_000, "q"));
    spans
}

#[test]
fn per_bucket_percentiles_are_exact_nearest_rank_durations() {
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), vec![req(&percentile_corpus())], "pct");
    let data = run(
        vec![sealed_source(dir.path(), &wal, "a")],
        OverviewQuery::new(grid()),
    );
    assert_eq!(data.total_traces, 11);
    assert_eq!(data.bucket_percentiles.len(), 10, "one per time bucket");
    // n=10, nearest rank ceil(p/100 x n) - 1: p50 is the 5th smallest,
    // p95 and p99 the largest. Every answer is an OBSERVED duration —
    // no interpolation over the decade-wide "1-10ms" bin could name 5ms.
    assert_eq!(
        data.bucket_percentiles[2],
        Some(DurationPercentiles {
            p50: 5_000_000,
            p95: 10_000_000,
            p99: 10_000_000,
        })
    );
    // A one-trace bucket answers that trace's duration at every rank.
    assert_eq!(
        data.bucket_percentiles[5],
        Some(DurationPercentiles {
            p50: 42_000_000,
            p95: 42_000_000,
            p99: 42_000_000,
        })
    );
    // Every other bucket binned nothing: absent, never zero.
    for (i, p) in data.bucket_percentiles.iter().enumerate() {
        assert_eq!(p.is_some(), i == 2 || i == 5, "bucket {i}");
    }
}

/// The all-or-empty paths keep the array at the grid's shape, so a
/// consumer walking it beside `cells` never runs off its end.
#[test]
fn a_cancelled_call_keeps_the_percentile_array_at_the_grid_shape() {
    let cancel = CancellationToken::new();
    cancel.cancel();
    let data = overview(
        vec![],
        OverviewQuery::new(grid()),
        cancel,
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap();
    assert_eq!(data.bucket_percentiles, vec![None; 10]);
}

// ── Root facets ─────────────────────────────────────────────────────

/// A CHILD span (parent set) of trace `t`.
fn fspan(t: u8, id: u8, parent: u8, start_ns: u64, name: &'static str) -> common::SpanSpec {
    let mut s = sp(id, parent, start_ns, name);
    s.trace = [t; 16];
    s.end = start_ns + 500;
    s
}

/// Five traces across services (brute-force expectations inline):
///  - A (svc-a): root "op-a" + a child          → svc-a / op-a
///  - B (svc-a): root "op-b"                    → svc-a / op-b
///  - C (svc-b): root "op-a"                    → svc-b / op-a
///  - D (svc-b): only a child span (parent set) → Indeterminate
///  - E (no service resource): root "op-e"      → service-less root
fn facet_wal(dir: &std::path::Path) -> std::path::PathBuf {
    use common::{kv_str, req_with};
    write_wal(
        dir,
        vec![
            req_with(
                vec![kv_str("service.name", "svc-a")],
                None,
                &[
                    tspan(0xA, 1, 1_000_000_000, 1_500_000_000, "op-a"),
                    fspan(0xA, 2, 1, 1_100_000_000, "a-child"),
                    tspan(0xB, 3, 2_000_000_000, 2_500_000_000, "op-b"),
                ],
            ),
            req_with(
                vec![kv_str("service.name", "svc-b")],
                None,
                &[
                    tspan(0xC, 4, 3_000_000_000, 3_500_000_000, "op-a"),
                    fspan(0xD, 5, 9, 4_000_000_000, "d-orphan"),
                ],
            ),
            req_with(
                vec![],
                None,
                &[tspan(0xE, 6, 5_000_000_000, 5_500_000_000, "op-e")],
            ),
        ],
        "facets",
    )
}

#[test]
fn facets_are_absent_unless_requested_and_change_nothing_else() {
    let dir = tempfile::tempdir().unwrap();
    let wal = facet_wal(dir.path());
    let plain = run(
        vec![sealed_source(dir.path(), &wal, "s")],
        OverviewQuery::new(grid()),
    );
    assert!(plain.root_facets.is_none(), "opt-in only");

    let with = run(
        vec![sealed_source(dir.path(), &wal, "s")],
        OverviewQuery::new(grid()).root_facets(true),
    );
    assert!(with.root_facets.is_some());
    assert_eq!(with.cells, plain.cells, "the grid is identical");
    assert_eq!(with.total_traces, plain.total_traces);
    assert_eq!(with.total_spans, plain.total_spans);
    assert_eq!(with.status, plain.status);
}

#[test]
fn facet_counts_match_brute_force_with_explicit_unattributed_buckets() {
    let dir = tempfile::tempdir().unwrap();
    let wal = facet_wal(dir.path());
    let data = run(
        vec![sealed_source(dir.path(), &wal, "s")],
        OverviewQuery::new(grid()).root_facets(true),
    );
    assert_eq!(data.total_traces, 5);
    let f = data.root_facets.expect("requested");

    // Services: svc-a×2 (A,B) > svc-b×1 (C); D (Indeterminate) and E
    // (service-less root) are bucketed, never attributed.
    assert_eq!(
        f.services.top,
        vec![("svc-a".to_string(), 2), ("svc-b".to_string(), 1)]
    );
    assert_eq!((f.services.other, f.services.unattributed), (0, 2));

    // Operations: op-a×2 (A,C) then value-ASC among the ×1s; only D
    // lacks a root name.
    assert_eq!(
        f.operations.top,
        vec![
            ("op-a".to_string(), 2),
            ("op-b".to_string(), 1),
            ("op-e".to_string(), 1)
        ]
    );
    assert_eq!((f.operations.other, f.operations.unattributed), (0, 1));

    // The partition identity, both dimensions.
    for list in [&f.services, &f.operations] {
        let sum: u64 = list.top.iter().map(|(_, n)| n).sum();
        assert_eq!(sum + list.other + list.unattributed, data.total_traces);
    }
}

#[test]
fn a_straddling_root_attributes_once_from_the_source_holding_it() {
    // The root lives in the sealed part; the tail holds only a child.
    // The merged trace attributes to the root's service exactly once.
    let dir = tempfile::tempdir().unwrap();
    let wal_1 = write_wal(
        dir.path(),
        vec![common::req_with(
            vec![common::kv_str("service.name", "svc-root")],
            None,
            &[tspan(0xA, 1, 1_000_000_000, 1_500_000_000, "op-root")],
        )],
        "part1",
    );
    let wal_2 = write_wal(
        dir.path(),
        vec![common::req_with(
            vec![common::kv_str("service.name", "svc-child")],
            None,
            &[fspan(0xA, 2, 1, 2_000_000_000, "op-child")],
        )],
        "part2",
    );
    let data = run(
        vec![
            sealed_source(dir.path(), &wal_1, "s1"),
            tail_source(&wal_2, "t2"),
        ],
        OverviewQuery::new(grid()).root_facets(true),
    );
    assert_eq!(data.total_traces, 1);
    let f = data.root_facets.unwrap();
    assert_eq!(f.services.top, vec![("svc-root".to_string(), 1)]);
    assert_eq!(f.operations.top, vec![("op-root".to_string(), 1)]);
    assert_eq!((f.services.unattributed, f.operations.unattributed), (0, 0));
}

#[test]
fn the_top_k_cap_folds_the_tail_into_other_deterministically() {
    // Twelve services, one trace each: all counts tie, so the top 10
    // are the value-ASC prefix and `other` carries the remaining two.
    use common::{kv_str, req_with};
    let dir = tempfile::tempdir().unwrap();
    let reqs: Vec<_> = (0..12u8)
        .map(|i| {
            req_with(
                vec![kv_str("service.name", &format!("svc-{:02}", i))],
                None,
                &[tspan(0x10 + i, 1, 1_000_000_000, 1_500_000_000, "op")],
            )
        })
        .collect();
    let wal = write_wal(dir.path(), reqs, "many");
    let data = run(
        vec![sealed_source(dir.path(), &wal, "s")],
        OverviewQuery::new(grid()).root_facets(true),
    );
    let f = data.root_facets.unwrap();
    assert_eq!(f.services.top.len(), FACET_TOP_K);
    assert_eq!(f.services.top[0].0, "svc-00");
    assert_eq!(f.services.top[FACET_TOP_K - 1].0, "svc-09");
    assert_eq!(f.services.other, 2, "svc-10 and svc-11 fold into other");
    assert_eq!(f.services.unattributed, 0);
}

#[test]
fn the_partition_identity_holds_under_a_partial_and_facets_survive_it() {
    // A legacy (rollup-absent) file beside the facet corpus: the
    // partial fires, and the facet partition still describes the
    // COUNTED population exactly.
    let dir = tempfile::tempdir().unwrap();
    let wal = facet_wal(dir.path());
    let data = run(
        vec![
            sealed_source(dir.path(), &wal, "modern"),
            common::legacy_sfst_source(dir.path(), "legacy"),
        ],
        OverviewQuery::new(grid()).root_facets(true),
    );
    assert!(data.status.has(PartialReason::RollupAbsent));
    let f = data.root_facets.expect("requested facets survive the partial");
    for list in [&f.services, &f.operations] {
        let sum: u64 = list.top.iter().map(|(_, n)| n).sum();
        assert_eq!(sum + list.other + list.unattributed, data.total_traces);
    }
}

#[test]
fn a_cancelled_call_keeps_the_requested_facet_shape_empty() {
    // All-or-empty: the DATA is discarded but the response SHAPE
    // follows the request — requested facets come back as empty lists,
    // not as an absent section.
    let dir = tempfile::tempdir().unwrap();
    let wal = facet_wal(dir.path());
    let cancel = CancellationToken::new();
    cancel.cancel();
    let data = overview(
        vec![sealed_source(dir.path(), &wal, "s")],
        OverviewQuery::new(grid()).root_facets(true),
        cancel,
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap();
    assert!(data.status.has(PartialReason::Cancelled));
    let f = data.root_facets.expect("shape follows the request");
    assert!(f.services.top.is_empty() && f.operations.top.is_empty());
    assert_eq!((f.services.unattributed, f.operations.unattributed), (0, 0));
}

#[test]
fn all_traces_outside_the_grid_yield_empty_facets_with_the_zero_identity() {
    // The merge succeeds but every envelope start misses the grid: the
    // facet lists describe the (empty) binned population — identity
    // 0 == 0 + 0 + 0.
    let dir = tempfile::tempdir().unwrap();
    let wal = facet_wal(dir.path());
    let data = run(
        vec![sealed_source(dir.path(), &wal, "s")],
        // The corpus lives in [1s, 6s); this grid starts at 100s.
        OverviewQuery::new(sfst::Grid::new(100_000_000_000, 1_000_000_000, 10))
            .root_facets(true),
    );
    assert_eq!(data.status, QueryStatus::Complete);
    assert_eq!(data.total_traces, 0);
    let f = data.root_facets.unwrap();
    for list in [&f.services, &f.operations] {
        assert!(list.top.is_empty());
        assert_eq!((list.other, list.unattributed), (0, 0));
    }
}

#[test]
fn the_ceiling_prefix_keeps_the_facet_identity() {
    // Ceiling 0 with facets on: only the first source folds; the facet
    // partition describes exactly that prefix's binned traces.
    let dir = tempfile::tempdir().unwrap();
    let wal_a = facet_wal(dir.path());
    let wal_b = write_wal(
        dir.path(),
        vec![req(&[tspan(0x77, 1, 7_000_000_000, 7_500_000_000, "late")])],
        "b",
    );
    let data = run(
        vec![
            sealed_source(dir.path(), &wal_a, "1-first"),
            sealed_source(dir.path(), &wal_b, "2-second"),
        ],
        OverviewQuery::new(grid())
            .root_facets(true)
            .visited_rows_ceiling_for_tests(0),
    );
    assert!(data.status.has(PartialReason::OverviewCeiling));
    assert_eq!(data.total_traces, 5, "the first source's corpus only");
    let f = data.root_facets.unwrap();
    for list in [&f.services, &f.operations] {
        let sum: u64 = list.top.iter().map(|(_, n)| n).sum();
        assert_eq!(sum + list.other + list.unattributed, data.total_traces);
    }
}

// ── Filtered grid (the page's selections applied to the heatmap) ─────

fn text(v: &str) -> PredicateValue {
    PredicateValue::Text(v.to_string())
}

fn cond(target: PredicateTarget, op: CompareOp, values: &[&str]) -> Condition {
    Condition {
        target,
        op,
        values: values.iter().map(|v| text(v)).collect(),
    }
}

/// `name = <values...>` (the rail's operation filter).
fn name_in(values: &[&str]) -> Predicate {
    Predicate {
        conditions: vec![cond(
            PredicateTarget::Builtin(BuiltinField::Name),
            CompareOp::Eq,
            values,
        )],
    }
}

/// `resource.service.name = <value>` (the rail's service filter).
fn service_eq(value: &str) -> Predicate {
    Predicate {
        conditions: vec![cond(
            PredicateTarget::Attribute(AttributeOwner::Resource, "service.name".to_string()),
            CompareOp::Eq,
            &[value],
        )],
    }
}

fn filtered(sources: Vec<TraceSource>, predicate: Predicate) -> OverviewData {
    run(sources, OverviewQuery::new(grid()).predicate(predicate))
}

/// The filter selects TRACES by a matching stored row and keeps each
/// selected trace whole: envelope, span and error totals are the
/// trace's, not the matching rows'.
#[test]
fn filtered_grid_bins_only_traces_with_a_matching_stored_row() {
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), vec![req(&corpus())], "sealed");
    let src = || vec![sealed_source(dir.path(), &wal, "a")];

    // One matching span of C selects C; C's OTHER span still counts.
    let data = filtered(src(), name_in(&["c-1"]));
    assert_eq!(data.status, QueryStatus::Complete);
    let mut cells = vec![[0u64; DURATION_BIN_COUNT]; 10];
    cells[5][5] = 1;
    assert_eq!(data.cells, cells);
    assert_eq!(data.total_traces, 1);
    assert_eq!(data.total_spans, 2, "the whole trace, not the matched row");
    assert_eq!(data.total_errors, 0);
    assert!(data.bucket_percentiles[5].is_some());
    assert!(data.bucket_percentiles[1].is_none(), "A is not binned");

    // Values OR within a key: A or B.
    let data = filtered(src(), name_in(&["a-2", "b-err"]));
    let mut cells = vec![[0u64; DURATION_BIN_COUNT]; 10];
    cells[1][3] = 1;
    cells[3][0] = 1;
    assert_eq!(data.cells, cells);
    assert_eq!((data.total_traces, data.total_spans, data.total_errors), (2, 3, 1));

    // The match-all predicate IS the unfiltered grid.
    let all = filtered(src(), Predicate::all());
    let plain = run(src(), OverviewQuery::new(grid()));
    assert_eq!(all.cells, plain.cells);
    assert_eq!(all.total_traces, plain.total_traces);

    // Nothing matches: an empty, COMPLETE grid.
    let none = filtered(src(), name_in(&["nope"]));
    assert_eq!(none.status, QueryStatus::Complete);
    assert_eq!(none.total_traces, 0);
    assert!(none.cells.iter().flatten().all(|&c| c == 0));
}

/// A resource attribute filter (the rail's service picker) selects the
/// traces whose stored rows carry it — per request/resource, so two
/// services in one corpus split cleanly.
#[test]
fn filtered_grid_honours_resource_attributes() {
    let dir = tempfile::tempdir().unwrap();
    let c = corpus();
    let wal = write_wal(
        dir.path(),
        vec![
            req_with(vec![kv_str("service.name", "cart")], None, &c[..2]), // A
            req_with(vec![kv_str("service.name", "flagd")], None, &c[2..]), // B, C
        ],
        "svc",
    );
    let src = || vec![sealed_source(dir.path(), &wal, "a")];
    let cart = filtered(src(), service_eq("cart"));
    assert_eq!(cart.total_traces, 1);
    assert_eq!(cart.cells[1][3], 1, "A binned");
    let flagd = filtered(src(), service_eq("flagd"));
    assert_eq!(flagd.total_traces, 2);
    assert_eq!((flagd.cells[3][0], flagd.cells[5][5]), (1, 1), "B and C binned");
}

/// A trace straddling a sealed file and a tail is selected by a match in
/// EITHER source and bins by its MERGED envelope — the sealed side's
/// non-matching span still widens it.
#[test]
fn filtered_grid_selects_straddling_traces_from_either_source() {
    let dir = tempfile::tempdir().unwrap();
    let c = corpus();
    let wal_sealed = write_wal(dir.path(), vec![req(&c[..4])], "sealed"); // A, B, c-1
    let wal_tail = write_wal(dir.path(), vec![req(&c[4..])], "tail"); // c-2
    let src = || {
        vec![
            sealed_source(dir.path(), &wal_sealed, "s"),
            tail_source(&wal_tail, "t"),
        ]
    };
    for name in ["c-1", "c-2"] {
        let data = filtered(src(), name_in(&[name]));
        assert_eq!(data.status, QueryStatus::Complete, "{name}");
        assert_eq!(data.total_traces, 1, "{name}");
        assert_eq!(data.cells[5][5], 1, "{name}: merged 12s envelope, bucket 5");
        assert_eq!(data.total_spans, 2, "{name}: both stored spans of C");
    }
    // A trace held by ONE source only (B, sealed) is selected by its
    // own match; the tail-only counterpart runs in
    // `filtered_grid_requires_the_match_to_start_inside_the_grid`.
    let data = filtered(src(), name_in(&["b-err"]));
    assert_eq!((data.total_traces, data.total_errors), (1, 1));
}

/// The matching row must START inside the grid (the search engine's
/// window rule): a trace binned by its early envelope is NOT selected by
/// a match that lies past the grid's end.
#[test]
fn filtered_grid_requires_the_match_to_start_inside_the_grid() {
    let dir = tempfile::tempdir().unwrap();
    // D: d-1 at 2s (in the grid), d-2 at 12s (past the 10s grid end).
    let d = vec![
        tspan(0xD, 6, 2_000_000_000, 2_100_000_000, "d-1"),
        tspan(0xD, 7, 12_000_000_000, 12_100_000_000, "d-2"),
    ];
    let wal = write_wal(dir.path(), vec![req(&d)], "d");
    for (id, src) in [
        ("sealed", vec![sealed_source(dir.path(), &wal, "s")]),
        ("tail", vec![tail_source(&wal, "t")]),
    ] {
        let plain = run(src.clone(), OverviewQuery::new(grid()));
        assert_eq!(plain.total_traces, 1, "{id}: D bins by its 2s envelope start");
        assert_eq!(plain.cells[2][5], 1, "{id}: 10.1s envelope");
        let by_d1 = filtered(src.clone(), name_in(&["d-1"]));
        assert_eq!(by_d1.total_traces, 1, "{id}: in-grid match selects D");
        assert_eq!(by_d1.cells[2][5], 1, "{id}: still the whole envelope");
        let by_d2 = filtered(src, name_in(&["d-2"]));
        assert_eq!(by_d2.total_traces, 0, "{id}: a match past the grid end selects nothing");
    }
}

/// Stored-row semantics, like the totals: a resent copy that matches
/// selects the trace even if the canonical copy carries another value.
#[test]
fn filtered_grid_selects_by_any_stored_copy() {
    let dir = tempfile::tempdir().unwrap();
    let first = tspan(0xA, 1, 1_000_000_000, 1_200_000_000, "a-1");
    let resent = tspan(0xA, 1, 1_000_000_000, 1_200_000_000, "a-1-resent");
    let wal = write_wal(dir.path(), vec![req(&[first, resent])], "resend");
    let src = || vec![sealed_source(dir.path(), &wal, "s")];
    for name in ["a-1", "a-1-resent"] {
        let data = filtered(src(), name_in(&[name]));
        assert_eq!(data.total_traces, 1, "{name}: either stored copy selects A");
        assert_eq!(data.total_spans, 2, "{name}: both stored rows count");
    }
}

/// The filter's scans share the fold's ceiling: an exhausted budget
/// stops the merge with the overview's own partial, and nothing
/// half-flagged is binned.
#[test]
fn filtered_grid_charges_the_visited_ceiling() {
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), vec![req(&corpus())], "c");
    for src in [
        vec![sealed_source(dir.path(), &wal, "s")],
        vec![tail_source(&wal, "t")],
    ] {
        let data = run(
            src,
            OverviewQuery::new(grid())
                .predicate(name_in(&["c-1"]))
                .visited_rows_ceiling_for_tests(1),
        );
        assert!(data.status.has(PartialReason::OverviewCeiling));
        assert_eq!(data.total_traces, 0);
    }
}

/// The filter's scans are charged to the fold's ONE ceiling and
/// accumulate across sources: two files each affordable alone are not
/// affordable together, and the merge stops with the partial reason
/// while the traces the earlier file already selected stay binned.
#[test]
fn filter_work_accumulates_across_sources() {
    let dir = tempfile::tempdir().unwrap();
    let c = corpus();
    let wal_a = write_wal(dir.path(), vec![req(&c[..2])], "a"); // A
    let wal_b = write_wal(dir.path(), vec![req(&c[2..])], "b"); // B, C
    let src_a = || sealed_source(dir.path(), &wal_a, "a");
    let src_b = || sealed_source(dir.path(), &wal_b, "b");
    let predicate = || name_in(&["a-1", "c-1"]);
    let at = |sources: Vec<TraceSource>, ceiling: u64| {
        run(
            sources,
            OverviewQuery::new(grid())
                .predicate(predicate())
                .visited_rows_ceiling_for_tests(ceiling),
        )
    };
    // The smallest ceiling under which one file alone completes: its
    // fold rows plus its filter's scan and emission.
    let minimal = |source: &dyn Fn() -> TraceSource| {
        (0..64)
            .find(|&ceiling| at(vec![source()], ceiling).status == QueryStatus::Complete)
            .expect("a small file completes under a small ceiling")
    };
    let (cost_a, cost_b) = (minimal(&src_a), minimal(&src_b));

    // Together they need the SUM: one unit short trips the ceiling.
    let data = at(vec![src_a(), src_b()], cost_a + cost_b - 1);
    assert!(data.status.has(PartialReason::OverviewCeiling));
    assert_eq!(data.total_traces, 1, "the first file's selection survives the stop");
    assert_eq!(data.cells[1][3], 1, "A");
    assert_eq!(data.cells[5][5], 0, "C was never flagged");

    let data = at(vec![src_a(), src_b()], cost_a + cost_b);
    assert_eq!(data.status, QueryStatus::Complete);
    assert_eq!(data.total_traces, 2);
    assert_eq!((data.cells[1][3], data.cells[5][5]), (1, 1));
}

/// A file whose rows merge but whose filter scan fails (here: a corrupt
/// trace-id column, which the UNFILTERED grid never reads) contributes
/// no selected trace: its matches are unknown, so its traces stay
/// unflagged, the failure is on the result, and the merge goes on to
/// the next source.
#[test]
fn a_failed_filter_scan_leaves_the_files_traces_unflagged() {
    let dir = tempfile::tempdir().unwrap();
    let c = corpus();
    let wal_bad = write_wal(dir.path(), vec![req(&c[2..])], "bad"); // B, C
    let wal_ok = write_wal(dir.path(), vec![req(&c[..2])], "ok"); // A
    // Seal once, corrupt in place, then build sources WITHOUT resealing.
    drop(sealed_source(dir.path(), &wal_bad, "bad"));
    let bad_path = dir.path().join("bad.sfst");
    corrupt_chunk(&bad_path, *b"TRCE");
    let src = || {
        vec![
            sealed_source_at(&bad_path, "bad"),
            sealed_source(dir.path(), &wal_ok, "ok"),
        ]
    };

    // Unfiltered, the column is never read: the file counts, complete.
    let plain = run(src(), OverviewQuery::new(grid()));
    assert_eq!(plain.status, QueryStatus::Complete);
    assert_eq!(plain.total_traces, 3);

    // Filtered, C's match is unknowable: A alone is selected, the
    // failure is flagged, and the healthy file after it still ran.
    let data = filtered(src(), name_in(&["a-1", "c-1"]));
    assert!(data.status.has(PartialReason::SourceFailure));
    assert!(!data.status.has(PartialReason::OverviewCeiling));
    assert_eq!(data.total_traces, 1);
    assert_eq!(data.cells[1][3], 1, "A, from the healthy file");
    assert_eq!(data.cells[5][5], 0, "C stays unflagged");
}

/// Trace-level and trace-id conditions cannot be answered from stored
/// rows: a clean request error names the offending target, never a
/// silently unfiltered grid.
#[test]
fn filtered_grid_rejects_trace_level_conditions() {
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), vec![req(&corpus())], "c");
    for (field, value) in [
        (BuiltinField::RootName, text("a-1")),
        (BuiltinField::RootServiceName, text("svc")),
        (BuiltinField::TraceDuration, PredicateValue::Integer(1000)),
        (
            BuiltinField::TraceId,
            text("0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a"),
        ),
    ] {
        let predicate = Predicate {
            conditions: vec![Condition {
                target: PredicateTarget::Builtin(field),
                op: CompareOp::Eq,
                values: vec![value],
            }],
        };
        let err = overview(
            vec![sealed_source(dir.path(), &wal, "a")],
            OverviewQuery::new(grid()).predicate(predicate),
            CancellationToken::new(),
            Arc::new(AtomicUsize::new(0)),
        )
        .unwrap_err();
        assert!(
            matches!(err, OverviewRequestError::TraceLevelCondition { .. }),
            "{field:?}: {err}"
        );
    }
}

/// Cross-check against the list: on a resend-free corpus the filtered
/// grid's trace count equals the number of traces `search` returns for
/// the same predicate over the same window and sources.
#[test]
fn filtered_grid_agrees_with_search_on_a_canonical_corpus() {
    let dir = tempfile::tempdir().unwrap();
    let c = corpus();
    let wal_sealed = write_wal(dir.path(), vec![req(&c[..3])], "sealed"); // A, B
    let wal_tail = write_wal(dir.path(), vec![req(&c[3..])], "tail"); // C
    let src = || {
        vec![
            sealed_source(dir.path(), &wal_sealed, "s"),
            tail_source(&wal_tail, "t"),
        ]
    };
    let window = TimeWindow::new(0, 10_000_000_000).unwrap();
    let predicates = vec![
        name_in(&["c-1"]),
        name_in(&["a-1", "b-err"]),
        name_in(&["nope"]),
        Predicate {
            conditions: vec![cond(
                PredicateTarget::Builtin(BuiltinField::Name),
                CompareOp::Regex,
                &["a-.*"],
            )],
        },
        service_eq("svc"),
        Predicate::all(),
    ];
    for predicate in predicates {
        let grid_data = filtered(src(), predicate.clone());
        let list = search(
            SearchSources {
                window: src(),
                completion: src(),
            },
            SearchQuery::new(predicate.clone()).window(window).limit(100),
            CancellationToken::new(),
            Arc::new(AtomicUsize::new(0)),
        )
        .unwrap();
        assert_eq!(
            grid_data.total_traces,
            list.traces.len() as u64,
            "{predicate:?}: heatmap population = list population"
        );
    }
}

