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

use common::{req, sp, sealed_source, tail_source, write_wal};
use sfsq::traces::{
    DURATION_BIN_COUNT, FACET_TOP_K, OverviewData, OverviewQuery, PartialReason, QueryStatus,
    TraceQuery, TraceSource, overview, trace_by_id,
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
