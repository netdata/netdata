//! Integration suite for the slowest mode:
//! duration-ranked top-K against brute force on multi-source corpora,
//! the cross-source straddle (merged envelope ranks once), the
//! cross-source root pick (single-root straddles exact; multi-root
//! ties by smallest span id), the stored-row counts, the legacy
//! (pre-rollup) exclusion,
//! truncation, tie-breaks, window clipping, ceiling termination,
//! cancellation, per-source failure honesty, and the request-error
//! boundary.

mod common;

use std::sync::Arc;
use std::sync::atomic::AtomicUsize;

use tokio_util::sync::CancellationToken;

use common::{req, sp, sealed_source, tail_source, write_wal};
use sfsq::traces::{
    PartialReason, QueryStatus, SLOWEST_LIMIT_MAX, SlowestData, SlowestQuery,
    SlowestRequestError, TimeWindow, TraceSource, slowest,
};

/// The suite's default window: [0s, 100s).
fn window() -> TimeWindow {
    TimeWindow::new(0, 100_000_000_000).unwrap()
}

fn run(sources: Vec<TraceSource>, query: SlowestQuery) -> SlowestData {
    slowest(
        sources,
        query,
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap()
}

/// A ROOT span (unset parent) of trace `t` with an explicit envelope.
fn tspan(t: u8, id: u8, start_ns: u64, end_ns: u64, name: &'static str) -> common::SpanSpec {
    let mut s = sp(id, 0, start_ns, name);
    s.trace = [t; 16];
    s.end = end_ns;
    s
}

/// A CHILD span (parent set) of trace `t`.
fn cspan(
    t: u8,
    id: u8,
    parent: u8,
    start_ns: u64,
    end_ns: u64,
    name: &'static str,
) -> common::SpanSpec {
    let mut s = sp(id, parent, start_ns, name);
    s.trace = [t; 16];
    s.end = end_ns;
    s
}

/// Four traces with distinct envelope durations:
///  - A: root [1.0s..1.2s] + child [1.1s..1.4s]  → 400ms
///  - B: one ERROR root [3.0s..3.000_000_5s]     → 500ns
///  - C: root [5.0s..5.1s] + child [5.05s..17s]  → 12s
///  - D: root [8.0s..10.0s]                      → 2s
fn corpus() -> Vec<common::SpanSpec> {
    let mut b = tspan(0xB, 3, 3_000_000_000, 3_000_000_500, "b-err");
    b.status = Some((2, "boom"));
    vec![
        tspan(0xA, 1, 1_000_000_000, 1_200_000_000, "a-root"),
        cspan(0xA, 2, 1, 1_100_000_000, 1_400_000_000, "a-child"),
        b,
        tspan(0xC, 4, 5_000_000_000, 5_100_000_000, "c-root"),
        cspan(0xC, 5, 4, 5_050_000_000, 17_000_000_000, "c-child"),
        tspan(0xD, 6, 8_000_000_000, 10_000_000_000, "d-root"),
    ]
}

#[test]
fn top_k_matches_brute_force_on_the_corpus() {
    // Brute force from the corpus definition: C (12s) > D (2s) >
    // A (400ms) > B (500ns) — durations are merged envelopes.
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), vec![req(&corpus())], "sealed");
    let data = run(
        vec![sealed_source(dir.path(), &wal, "s")],
        SlowestQuery::new(window()),
    );
    assert_eq!(data.status, QueryStatus::Complete);
    let got: Vec<_> = data
        .traces
        .iter()
        .map(|t| (t.trace_id, t.duration_ns))
        .collect();
    let expected = vec![
        (sfst::TraceId::from([0xC; 16]), 12_000_000_000),
        (sfst::TraceId::from([0xD; 16]), 2_000_000_000),
        (sfst::TraceId::from([0xA; 16]), 400_000_000),
        (sfst::TraceId::from([0xB; 16]), 500),
    ];
    assert_eq!(got, expected, "full ranking matches brute force");

    // Row fields carry the stored-row counts and the honest-or-absent roots.
    let c = &data.traces[0];
    assert_eq!(c.min_start_ns, 5_000_000_000);
    assert_eq!((c.span_count, c.error_count), (2, 0));
    let root = c.root.as_ref().expect("C has a true root");
    assert_eq!(root.service.as_deref(), Some("svc"));
    assert_eq!(root.name.as_deref(), Some("c-root"));
    let b = &data.traces[3];
    assert_eq!((b.span_count, b.error_count), (1, 1));
}

#[test]
fn limit_truncates_to_the_top_k() {
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), vec![req(&corpus())], "sealed");
    let data = run(
        vec![sealed_source(dir.path(), &wal, "s")],
        SlowestQuery::new(window()).limit(2),
    );
    assert_eq!(data.status, QueryStatus::Complete, "truncation is not a partial");
    let got: Vec<_> = data.traces.iter().map(|t| t.trace_id).collect();
    assert_eq!(
        got,
        vec![
            sfst::TraceId::from([0xC; 16]),
            sfst::TraceId::from([0xD; 16])
        ]
    );
}

#[test]
fn equal_durations_tie_break_by_ascending_trace_id() {
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(
        dir.path(),
        vec![req(&[
            tspan(0x22, 1, 1_000_000_000, 2_000_000_000, "y"),
            tspan(0x11, 2, 4_000_000_000, 5_000_000_000, "x"),
        ])],
        "ties",
    );
    let data = run(
        vec![sealed_source(dir.path(), &wal, "s")],
        SlowestQuery::new(window()),
    );
    let got: Vec<_> = data.traces.iter().map(|t| t.trace_id).collect();
    assert_eq!(
        got,
        vec![
            sfst::TraceId::from([0x11; 16]),
            sfst::TraceId::from([0x22; 16])
        ],
        "1s == 1s → trace id ASC"
    );
}

#[test]
fn straddling_trace_ranks_once_with_the_merged_envelope_and_root() {
    // Trace A's ROOT lands in the sealed file, its long-running CHILD in
    // the tail: one merged trace whose duration spans both parts;
    // across sources, the root comes from the ONLY source holding a
    // true root — exact, no tie involved.
    let dir = tempfile::tempdir().unwrap();
    let wal_1 = write_wal(
        dir.path(),
        vec![req(&[tspan(0xA, 1, 1_000_000_000, 1_200_000_000, "a-root")])],
        "part1",
    );
    let wal_2 = write_wal(
        dir.path(),
        vec![req(&[cspan(0xA, 2, 1, 1_100_000_000, 9_000_000_000, "a-child")])],
        "part2",
    );
    let data = run(
        vec![
            sealed_source(dir.path(), &wal_1, "s1"),
            tail_source(&wal_2, "t2"),
        ],
        SlowestQuery::new(window()),
    );
    assert_eq!(data.traces.len(), 1, "straddling ranks ONCE");
    let t = &data.traces[0];
    assert_eq!(t.duration_ns, 8_000_000_000, "merged envelope 1s..9s");
    assert_eq!(t.span_count, 2);
    let root = t.root.as_ref().expect("part1 holds the true root");
    assert_eq!(root.name.as_deref(), Some("a-root"));
}

#[test]
fn multi_root_straddle_picks_the_smallest_root_span_id() {
    // Pathological: BOTH sources hold a true (unset-parent) root span of
    // the same trace, with different span ids. The pinned cross-source
    // rule: the smallest root span id wins, deterministically.
    let dir = tempfile::tempdir().unwrap();
    let wal_1 = write_wal(
        dir.path(),
        vec![req(&[tspan(0xA, 7, 1_000_000_000, 2_000_000_000, "late-root")])],
        "part1",
    );
    let wal_2 = write_wal(
        dir.path(),
        vec![req(&[tspan(0xA, 3, 1_500_000_000, 2_500_000_000, "small-root")])],
        "part2",
    );
    // Both orders produce the same pick — the rule is source-order-free.
    for (ids, label) in [(["1-a", "2-b"], "forward"), (["2-a", "1-b"], "reversed")] {
        let data = run(
            vec![
                sealed_source(dir.path(), &wal_1, ids[0]),
                sealed_source(dir.path(), &wal_2, ids[1]),
            ],
            SlowestQuery::new(window()),
        );
        let root = data.traces[0].root.as_ref().expect("a root survives");
        assert_eq!(
            root.name.as_deref(),
            Some("small-root"),
            "{label}: span id 3 < span id 7"
        );
    }
}

#[test]
fn resend_counts_stored_rows() {
    // The SAME span stored twice counts twice in the row's numbers.
    let dir = tempfile::tempdir().unwrap();
    let a = tspan(0xA, 1, 1_000_000_000, 1_200_000_000, "a-root");
    let wal = write_wal(dir.path(), vec![req(&[a.clone(), a])], "resend");
    let data = run(
        vec![sealed_source(dir.path(), &wal, "s")],
        SlowestQuery::new(window()),
    );
    assert_eq!(data.traces.len(), 1);
    assert_eq!(data.traces[0].span_count, 2, "stored rows (resends count)");
}

#[test]
fn a_trace_starting_before_the_window_is_clipped() {
    // The alignment rule shared with the overview: envelope-start
    // clipping. Trace A starts at 1s — outside a [2s, 100s) window —
    // and disappears entirely; D (8s) survives.
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), vec![req(&corpus())], "sealed");
    let data = run(
        vec![sealed_source(dir.path(), &wal, "s")],
        SlowestQuery::new(TimeWindow::new(2_000_000_000, 100_000_000_000).unwrap()),
    );
    let got: Vec<_> = data.traces.iter().map(|t| t.trace_id).collect();
    assert_eq!(
        got,
        vec![
            sfst::TraceId::from([0xC; 16]),
            sfst::TraceId::from([0xD; 16]),
            sfst::TraceId::from([0xB; 16])
        ],
        "A clipped; the rest keep rank order"
    );
}

#[test]
fn a_straddle_whose_merged_start_is_pre_window_is_clipped_whole() {
    // Part 1 starts BEFORE the window, part 2 inside it. The clip
    // tests the MERGED envelope start, not per-source starts — the
    // trace disappears whole, it does not sneak in through part 2.
    let dir = tempfile::tempdir().unwrap();
    let wal_1 = write_wal(
        dir.path(),
        vec![req(&[tspan(0xA, 1, 1_000_000_000, 1_500_000_000, "early")])],
        "part1",
    );
    let wal_2 = write_wal(
        dir.path(),
        vec![req(&[cspan(0xA, 2, 1, 3_000_000_000, 9_000_000_000, "late")])],
        "part2",
    );
    let data = run(
        vec![
            sealed_source(dir.path(), &wal_1, "s1"),
            tail_source(&wal_2, "t2"),
        ],
        SlowestQuery::new(TimeWindow::new(2_000_000_000, 100_000_000_000).unwrap()),
    );
    assert_eq!(data.status, QueryStatus::Complete);
    assert!(data.traces.is_empty(), "merged start 1s < window start 2s");
}

#[test]
fn legacy_file_without_the_rollup_is_excluded_and_flagged() {
    // Identical to the overview: a pre-rollup file contributes
    // nothing and the exclusion is flagged.
    let dir = tempfile::tempdir().unwrap();
    let modern_wal = write_wal(dir.path(), vec![req(&corpus())], "modern");
    let modern = sealed_source(dir.path(), &modern_wal, "modern");
    let legacy = common::legacy_sfst_source(dir.path(), "legacy");

    let data = run(vec![modern, legacy], SlowestQuery::new(window()));
    assert!(data.status.has(PartialReason::RollupAbsent));
    assert_eq!(data.traces.len(), 4, "only the modern file's traces rank");
}

#[test]
fn ceiling_terminates_with_the_deterministic_prefix_and_the_partial() {
    // Ceiling 0: source 1 (SourceId order) processes and overshoots;
    // source 2 never runs — its slower trace is honestly absent.
    let dir = tempfile::tempdir().unwrap();
    let wal_a = write_wal(dir.path(), vec![req(&corpus()[..2])], "a"); // trace A
    let wal_b = write_wal(dir.path(), vec![req(&corpus()[2..])], "b"); // B, C, D
    let data = run(
        vec![
            sealed_source(dir.path(), &wal_a, "1-first"),
            sealed_source(dir.path(), &wal_b, "2-second"),
        ],
        SlowestQuery::new(window()).visited_rows_ceiling_for_tests(0),
    );
    assert!(data.status.has(PartialReason::SlowestCeiling));
    let got: Vec<_> = data.traces.iter().map(|t| t.trace_id).collect();
    assert_eq!(
        got,
        vec![sfst::TraceId::from([0xA; 16])],
        "only the first source's trace — C's 12s is honestly missing"
    );
}

#[test]
fn cancelled_call_returns_empty_with_the_reason() {
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), vec![req(&corpus())], "sealed");
    let cancel = CancellationToken::new();
    cancel.cancel();
    let data = slowest(
        vec![sealed_source(dir.path(), &wal, "s")],
        SlowestQuery::new(window()),
        cancel,
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap();
    assert!(data.status.has(PartialReason::Cancelled));
    assert!(data.traces.is_empty(), "all-or-empty");
}

#[test]
fn cancelled_zero_source_call_still_reports_cancelled() {
    let cancel = CancellationToken::new();
    cancel.cancel();
    let data = slowest(
        Vec::new(),
        SlowestQuery::new(window()),
        cancel,
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap();
    assert!(data.status.has(PartialReason::Cancelled));
}

#[test]
fn a_failed_source_degrades_honestly_while_the_rest_rank() {
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), vec![req(&corpus())], "good");
    let missing = TraceSource::Sfst(sfsq::traces::TraceSfstCandidate {
        source_id: sfsq::traces::SourceId::new("gone"),
        summary: sfst::Summary {
            min_timestamp_s: 0,
            max_timestamp_s: 10,
            record_count: 1,
            content_meta: Vec::new(),
        },
        source: sfsq::Source::File(dir.path().join("no-such-file.sfst")),
        coverage: None,
    });
    let data = run(
        vec![missing, sealed_source(dir.path(), &wal, "good")],
        SlowestQuery::new(window()),
    );
    assert!(data.status.has(PartialReason::SourceFailure));
    assert_eq!(data.traces.len(), 4, "the healthy source still ranks");
}

#[test]
fn zero_and_oversized_limits_are_request_errors() {
    let err = slowest(
        Vec::new(),
        SlowestQuery::new(window()).limit(0),
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
    .expect_err("zero limit");
    assert!(matches!(err, SlowestRequestError::ZeroLimit));

    let err = slowest(
        Vec::new(),
        SlowestQuery::new(window()).limit(SLOWEST_LIMIT_MAX + 1),
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
    .expect_err("oversized limit");
    assert!(matches!(err, SlowestRequestError::LimitTooLarge(n) if n == SLOWEST_LIMIT_MAX + 1));
}
