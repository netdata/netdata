//! Cross-source search acceptance suite (phase 4c, stage A).
//!
//! The core criteria:
//!
//! - **Oracle equivalence under re-layouts:** the same corpus sealed as
//!   one file, split across files, or served partly as an in-memory
//!   chunk / WAL tail returns IDENTICAL results (ids, order, summary
//!   numbers, attached spans, kinds).
//! - **The over-approximation never leaks:** a trace whose only match is
//!   a LOSING resend copy is dropped by phase-2 canonical re-evaluation;
//!   a trace whose canonical copy matches is found however its raw rank
//!   compares (the glm inflated-raw-rank construction — the demand-driven
//!   grow-K regression test).
//! - **Deterministic termination:** ceilings return the gathered, ranked,
//!   trimmed result + WorkCeiling; cancellation is all-or-empty; results
//!   are identical under source-order permutation.

mod common;

use std::path::Path;
use std::sync::Arc;
use std::sync::atomic::AtomicUsize;

use tokio_util::sync::CancellationToken;

use common::{
    SpanSpec, kv_str, memory_source, req_with, sealed_source, sp, tail_source, write_wal,
};
use opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest;
use sfsq::traces::{
    CompareOp, Condition, PartialReason, Predicate, PredicateError, PredicateTarget,
    PredicateValue, QueryStatus, SearchData, SearchQuery, SearchRequestError, SearchSources,
    TagScope, TimeWindow, TraceIntrinsic, TraceSource, search,
};

const NS: u64 = 1_000_000_000;

fn tid(n: u8) -> [u8; 16] {
    let mut b = [0u8; 16];
    b[15] = n;
    b
}

fn text(v: &str) -> PredicateValue {
    PredicateValue::Text(v.to_string())
}

fn cond(target: PredicateTarget, op: CompareOp, values: Vec<PredicateValue>) -> Condition {
    Condition { target, op, values }
}

fn attr_eq(scope: TagScope, key: &str, value: &str) -> Condition {
    cond(
        PredicateTarget::Attribute(scope, key.to_string()),
        CompareOp::Eq,
        vec![text(value)],
    )
}

fn intrinsic(i: TraceIntrinsic, op: CompareOp, values: Vec<PredicateValue>) -> Condition {
    cond(PredicateTarget::Intrinsic(i), op, values)
}

fn pred(conditions: Vec<Condition>) -> Predicate {
    Predicate { conditions }
}

fn run(sources: SearchSources, query: SearchQuery) -> SearchData {
    search(
        sources,
        query,
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
    .expect("valid request")
}

/// One summary rendered comparable across layouts.
type SummaryPrint = (
    String,         // trace id (hex)
    Option<String>, // root_service
    Option<String>, // root_name
    i64,            // start_ns
    i64,            // duration_ns
    usize,          // span_count
    usize,          // error_count
    usize,          // matched_count
    Vec<(String, i64, i64)>, // attached spans: (name, start, duration)
    bool,           // exact
);

fn norm(data: &SearchData) -> (Vec<SummaryPrint>, QueryStatus) {
    let traces = data
        .traces
        .iter()
        .map(|t| {
            (
                t.trace_id.to_string(),
                t.root_service.clone(),
                t.root_name.clone(),
                t.start_ns,
                t.duration_ns,
                t.span_count,
                t.error_count,
                t.matched_count,
                t.matched_spans
                    .iter()
                    .map(|s| {
                        let name = s
                            .fields
                            .iter()
                            .find(|(k, _)| k == "name")
                            .map(|(_, v)| v.clone())
                            .unwrap_or_default();
                        (name, s.start_ns, s.duration_ns)
                    })
                    .collect(),
                t.exact,
            )
        })
        .collect();
    (traces, data.status.clone())
}

// ── The shared world (six traces, two services, two time halves) ──────

fn span_in(trace: u8, id: u8, parent: u8, start: u64, name: &'static str) -> SpanSpec {
    SpanSpec {
        trace: tid(trace),
        ..sp(id, parent, start, name)
    }
}

/// The requests of the EARLY half (wal1): T1/T2/T3-root on svc-a,
/// T4/T6-canonical on svc-b.
fn early_requests() -> Vec<ExportTraceServiceRequest> {
    let svc_a = vec![kv_str("service.name", "svc-a"), kv_str("env", "prod")];
    let svc_b = vec![kv_str("service.name", "svc-b")];
    vec![
        req_with(svc_a, None, &[
            SpanSpec {
                kind: 2, // SERVER
                status: Some((1, "")),
                end: 1_000 * NS + 500_000,
                attrs: vec![kv_str("http.method", "GET")],
                ..span_in(1, 0x11, 0, 1_000 * NS, "GET /users")
            },
            SpanSpec {
                attrs: vec![kv_str("db.system", "pg")],
                ..span_in(1, 0x12, 0x11, 1_000 * NS + 10, "db.query")
            },
            SpanSpec {
                status: Some((2, "boom")),
                attrs: vec![kv_str("http.method", "POST")],
                ..span_in(2, 0x21, 0, 1_100 * NS, "POST /orders")
            },
            SpanSpec {
                status: Some((1, "")),
                ..span_in(3, 0x31, 0, 900 * NS, "GET /health")
            },
        ]),
        req_with(svc_b, None, &[
            SpanSpec {
                kind: 5, // CONSUMER
                attrs: vec![kv_str("queue", "q1")],
                ..span_in(4, 0x41, 0, 1_200 * NS, "consume")
            },
            // T6's CANONICAL copy (chronologically first).
            SpanSpec {
                attrs: vec![kv_str("flag", "on")],
                ..span_in(6, 0x61, 0, 1_300 * NS, "flagged")
            },
        ]),
    ]
}

/// The requests of the LATE half (wal2): T5 on svc-b, T3's late child on
/// svc-a, T6's LOSING resend (same span id, different content, later).
fn late_requests() -> Vec<ExportTraceServiceRequest> {
    let svc_a = vec![kv_str("service.name", "svc-a"), kv_str("env", "prod")];
    let svc_b = vec![kv_str("service.name", "svc-b")];
    vec![
        req_with(svc_b.clone(), None, &[
            SpanSpec {
                status: Some((2, "err1")),
                ..span_in(5, 0x51, 0, 1_500 * NS, "batch")
            },
            SpanSpec {
                status: Some((2, "err2")),
                end: 1_510 * NS + 2_000_000,
                ..span_in(5, 0x52, 0x51, 1_510 * NS, "slow-step")
            },
            SpanSpec {
                status: Some((2, "err3")),
                ..span_in(5, 0x53, 0x51, 1_520 * NS, "fast-step")
            },
        ]),
        req_with(svc_a, None, &[SpanSpec {
            attrs: vec![kv_str("http.method", "GET")],
            ..span_in(3, 0x32, 0x31, 1_900 * NS, "late-check")
        }]),
        req_with(svc_b, None, &[SpanSpec {
            attrs: vec![kv_str("ghost", "yes")],
            ..span_in(6, 0x61, 0, 1_950 * NS, "flagged")
        }]),
    ]
}

/// Window role = completion role over the same paths (the whole-set dev
/// shape; membership is by SourceId).
fn both_roles(build: impl Fn() -> Vec<TraceSource>) -> SearchSources {
    SearchSources {
        window: build(),
        completion: build(),
    }
}

/// The world's WAL halves plus the whole, written once per test.
struct World {
    wal_all: std::path::PathBuf,
    wal_early: std::path::PathBuf,
    wal_late: std::path::PathBuf,
}

fn world(dir: &Path) -> World {
    let mut all = early_requests();
    all.extend(late_requests());
    World {
        wal_all: write_wal(dir, all, "all"),
        wal_early: write_wal(dir, early_requests(), "early"),
        wal_late: write_wal(dir, late_requests(), "late"),
    }
}

const LAYOUTS: [&str; 4] = [
    "single-sealed",
    "two-sealed",
    "sealed-plus-tail",
    "chunk-plus-tail",
];

/// Build one named layout of the world (sources are consumed per call).
fn layout(dir: &Path, world: &World, name: &str) -> SearchSources {
    match name {
        "single-sealed" => both_roles(|| vec![sealed_source(dir, &world.wal_all, "one")]),
        "two-sealed" => both_roles(|| {
            vec![
                sealed_source(dir, &world.wal_early, "early"),
                sealed_source(dir, &world.wal_late, "late"),
            ]
        }),
        "sealed-plus-tail" => both_roles(|| {
            vec![
                sealed_source(dir, &world.wal_early, "early"),
                tail_source(&world.wal_late, "late-tail"),
            ]
        }),
        "chunk-plus-tail" => both_roles(|| {
            vec![
                memory_source(&world.wal_early, "early-chunk"),
                tail_source(&world.wal_late, "late-tail"),
            ]
        }),
        other => panic!("unknown layout {other}"),
    }
}

/// Ids of the returned traces, hex-rendered, in result order.
fn ids(data: &SearchData) -> Vec<String> {
    data.traces.iter().map(|t| t.trace_id.to_string()).collect()
}

fn hex(n: u8) -> String {
    sfst::TraceId::from(tid(n)).to_string()
}

/// Every layout answers every query identically, and the single-sealed
/// answers match the hand-computed expectations (final ordering by
/// canonical matched-span start; T6 ranks by its CANONICAL 1300s copy,
/// not the 1950s resend).
#[test]
fn oracle_equivalence_under_relayouts() {
    let dir = tempfile::tempdir().unwrap();
    let queries: Vec<(&str, SearchQuery, Vec<String>)> = vec![
        (
            "all",
            SearchQuery::new(Predicate::all()),
            vec![hex(3), hex(5), hex(6), hex(4), hex(2), hex(1)],
        ),
        (
            "service",
            SearchQuery::new(pred(vec![attr_eq(TagScope::Resource, "service.name", "svc-a")])),
            vec![hex(3), hex(2), hex(1)],
        ),
        (
            // T1's "GET /users" (1000s) outranks T3's "GET /health"
            // (900s): only the ROOT of T3 matches this pattern, so T3
            // ranks by 900s, not by its non-matching 1900s child.
            "name-regex",
            SearchQuery::new(pred(vec![intrinsic(
                TraceIntrinsic::Name,
                CompareOp::Regex,
                vec![text("GET .*")],
            )])),
            vec![hex(1), hex(3)],
        ),
        (
            "status-error",
            SearchQuery::new(pred(vec![intrinsic(
                TraceIntrinsic::Status,
                CompareOp::Eq,
                vec![text("ERROR")],
            )])),
            vec![hex(5), hex(2)],
        ),
        (
            "kind-keyword",
            SearchQuery::new(pred(vec![intrinsic(
                TraceIntrinsic::Kind,
                CompareOp::Eq,
                vec![text("CONSUMER")],
            )])),
            vec![hex(4)],
        ),
        (
            // Regex over the kind LABEL set is stage-A evaluable (26A):
            // matches T4's CONSUMER (1200s) and T1's SERVER (1000s).
            "kind-regex",
            SearchQuery::new(pred(vec![intrinsic(
                TraceIntrinsic::Kind,
                CompareOp::Regex,
                vec![text("SERVER|CONSUMER")],
            )])),
            vec![hex(4), hex(1)],
        ),
        (
            "duration-floor",
            SearchQuery::new(pred(vec![intrinsic(
                TraceIntrinsic::Duration,
                CompareOp::Gte,
                vec![PredicateValue::Integer(1_000_000)],
            )])),
            vec![hex(5)],
        ),
        (
            // Spanset semantics: ONE span carries both conditions.
            "conjunction",
            SearchQuery::new(pred(vec![
                attr_eq(TagScope::Resource, "service.name", "svc-a"),
                attr_eq(TagScope::Span, "http.method", "GET"),
            ])),
            vec![hex(3), hex(1)],
        ),
        (
            // Window [1050s, 1600s): T6's canonical (1300s) is inside —
            // ranked by it; T1/T3 have no span starting inside.
            "window",
            SearchQuery::new(Predicate::all())
                .window(TimeWindow::new(1_050 * NS as i64, 1_600 * NS as i64).unwrap()),
            vec![hex(5), hex(6), hex(4), hex(2)],
        ),
        (
            // The losing-resend-only direction: `ghost` exists only on
            // T6's resend copy; the canonical copy wins dedup, fails
            // re-evaluation, and the trace is dropped — never a false
            // positive from raw phase-1.
            "ghost-only-on-loser",
            SearchQuery::new(pred(vec![attr_eq(TagScope::Span, "ghost", "yes")])),
            vec![],
        ),
        (
            // The canonical-matches direction: `flag` exists only on the
            // canonical copy; found regardless of the newer resend.
            "flag-on-canonical",
            SearchQuery::new(pred(vec![attr_eq(TagScope::Span, "flag", "on")])),
            vec![hex(6)],
        ),
        (
            "multi-value-or",
            SearchQuery::new(pred(vec![cond(
                PredicateTarget::Attribute(TagScope::Span, "http.method".into()),
                CompareOp::Eq,
                vec![text("GET"), text("POST")],
            )])),
            vec![hex(3), hex(2), hex(1)],
        ),
    ];

    let world = world(dir.path());
    for (qname, query, expected_ids) in &queries {
        let mut reference: Option<(Vec<SummaryPrint>, QueryStatus)> = None;
        for lname in LAYOUTS {
            // Rebuild sources per query (they are consumed by search).
            let sources = layout(dir.path(), &world, lname);
            let data = run(sources, query.clone());
            assert_eq!(
                &ids(&data),
                expected_ids,
                "query {qname} on layout {lname}: id order"
            );
            assert!(
                data.traces.iter().all(|t| t.exact),
                "query {qname} on layout {lname}: everything is exact"
            );
            let got = norm(&data);
            assert_eq!(got.1, QueryStatus::Complete, "query {qname} on {lname}");
            match &reference {
                None => reference = Some(got),
                Some(want) => {
                    assert_eq!(&got, want, "query {qname}: layout {lname} diverges")
                }
            }
        }
    }
}

/// Summary numbers against ground truth (single layout; equivalence is
/// proven above): roots, envelope, counts, spss trim, combiner order.
#[test]
fn summary_ground_truth() {
    let dir = tempfile::tempdir().unwrap();
    let mut all = early_requests();
    all.extend(late_requests());
    let wal = write_wal(dir.path(), all, "world");
    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);

    let data = run(sources, SearchQuery::new(Predicate::all()).spss(2));
    let by_id = |n: u8| {
        data.traces
            .iter()
            .find(|t| t.trace_id == sfst::TraceId::from(tid(n)))
            .unwrap_or_else(|| panic!("trace {n} returned"))
    };

    // T1: root = GET /users (SERVER), envelope covers the 500µs root.
    let t1 = by_id(1);
    assert_eq!(t1.root_service.as_deref(), Some("svc-a"));
    assert_eq!(t1.root_name.as_deref(), Some("GET /users"));
    assert_eq!(t1.start_ns, (1_000 * NS) as i64);
    assert_eq!(t1.duration_ns, 500_000);
    assert_eq!((t1.span_count, t1.error_count, t1.matched_count), (2, 0, 2));
    // spss(2) attaches both, in combiner (chronological) order.
    assert_eq!(t1.matched_spans.len(), 2);
    assert!(t1.matched_spans[0].start_ns < t1.matched_spans[1].start_ns);

    // T3: spans scattered across the halves; envelope spans both.
    let t3 = by_id(3);
    assert_eq!(t3.root_name.as_deref(), Some("GET /health"));
    assert_eq!(t3.start_ns, (900 * NS) as i64);
    assert_eq!(t3.duration_ns, (1_000 * NS + 50) as i64);
    assert_eq!(t3.span_count, 2);

    // T5: all three spans carry ERROR.
    let t5 = by_id(5);
    assert_eq!((t5.span_count, t5.error_count), (3, 3));
    assert_eq!(t5.matched_spans.len(), 2, "spss trims the attachment");
    assert_eq!(t5.matched_count, 3, "matched_count is not trimmed by spss");

    // T6: the resend collapsed to the canonical copy.
    let t6 = by_id(6);
    assert_eq!((t6.span_count, t6.start_ns), (1, (1_300 * NS) as i64));

    // spss = 0 attaches nothing, counts unaffected.
    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let none = run(sources, SearchQuery::new(Predicate::all()).spss(0));
    assert!(none.traces.iter().all(|t| t.matched_spans.is_empty()));
    assert_eq!(none.traces.iter().map(|t| t.matched_count).sum::<usize>(), 10);
}

/// The glm 7-trace construction (pin R2-1): losing resends inflate raw
/// ranks; every inflated candidate PASSES phase 2 (so a fixed refill
/// round count would never fire), yet the true newest-canonical trace
/// sits below the initial K in raw order. The demand-driven frontier
/// must dig it out.
#[test]
fn inflated_raw_ranks_do_not_hide_the_true_top_trace() {
    let dir = tempfile::tempdir().unwrap();
    let svc = vec![kv_str("service.name", "svc")];
    let mut reqs: Vec<ExportTraceServiceRequest> = Vec::new();
    // T1..T5: canonical (matching) at (100+i)s, losing resend (also
    // matching) at (1000+i)s.
    for i in 1..=5u8 {
        reqs.push(req_with(svc.clone(), None, &[SpanSpec {
            attrs: vec![kv_str("m", "yes")],
            ..span_in(i, 0x10 + i, 0, (100 + u64::from(i)) * NS, "canon")
        }]));
    }
    for i in 1..=5u8 {
        reqs.push(req_with(svc.clone(), None, &[SpanSpec {
            attrs: vec![kv_str("m", "yes"), kv_str("resend", "1")],
            ..span_in(i, 0x10 + i, 0, (1_000 + u64::from(i)) * NS, "resend")
        }]));
    }
    // T6: single span at 500s — the TRUE canonical #1 is NOT this one…
    // every resend trace's canonical is at 10x s, so T6 (500s) outranks
    // them all canonically while its raw rank (500s) sits below every
    // inflated raw rank (1000+s).
    reqs.push(req_with(svc.clone(), None, &[SpanSpec {
        attrs: vec![kv_str("m", "yes")],
        ..span_in(6, 0x66, 0, 500 * NS, "hidden-winner")
    }]));
    let wal = write_wal(dir.path(), reqs, "inflated");

    let matching = pred(vec![attr_eq(TagScope::Span, "m", "yes")]);
    // limit 1 → initial K = 3: the raw top-3 are all inflated resends.
    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let top = run(sources, SearchQuery::new(matching.clone()).limit(1));
    assert_eq!(ids(&top), vec![hex(6)], "the hidden winner surfaces");
    assert_eq!(top.status, QueryStatus::Complete);

    // Full ordering: canonical ranks throughout (500, then 105..101).
    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let all = run(sources, SearchQuery::new(matching).limit(10));
    assert_eq!(
        ids(&all),
        vec![hex(6), hex(5), hex(4), hex(3), hex(2), hex(1)]
    );
}

/// Windowed direction of the resend rule: the resend STARTS inside the
/// window but the canonical copy does not — the trace must NOT match
/// (the canonical span fails predicate+window re-evaluation).
#[test]
fn window_applies_to_the_canonical_copy() {
    let dir = tempfile::tempdir().unwrap();
    let mut all = early_requests();
    all.extend(late_requests());
    let wal = write_wal(dir.path(), all, "world");

    // [1890s, 2000s): holds T3's late child (1900s) and T6's resend
    // (1950s) — but T6's canonical is at 1300s, outside.
    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let data = run(
        sources,
        SearchQuery::new(Predicate::all())
            .window(TimeWindow::new(1_890 * NS as i64, 2_000 * NS as i64).unwrap()),
    );
    assert_eq!(ids(&data), vec![hex(3)]);
    // Root-outside-window: the summary still derives from the real root
    // and the FULL retained set (envelope reaches back to 900s).
    let t3 = &data.traces[0];
    assert_eq!(t3.root_name.as_deref(), Some("GET /health"));
    assert_eq!(t3.start_ns, (900 * NS) as i64);
    assert_eq!(t3.span_count, 2);
    // …but matched_count/attachment follow predicate + window.
    assert_eq!(t3.matched_count, 1);
    assert_eq!(t3.matched_spans.len(), 1);
    assert_eq!(t3.matched_spans[0].start_ns, (1_900 * NS) as i64);
}

/// Request validation: everything rejected BEFORE any I/O.
#[test]
fn request_validation_matrix() {
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), early_requests(), "w");
    let sources = || both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let try_search = |sources: SearchSources, query: SearchQuery| {
        search(
            sources,
            query,
            CancellationToken::new(),
            Arc::new(AtomicUsize::new(0)),
        )
    };

    assert!(matches!(
        try_search(sources(), SearchQuery::new(Predicate::all()).limit(0)),
        Err(SearchRequestError::ZeroLimit)
    ));
    assert!(matches!(
        try_search(sources(), SearchQuery::new(Predicate::all()).spss(129)),
        Err(SearchRequestError::SpssBeyondMax { got: 129 })
    ));
    // Structural predicate error.
    assert!(matches!(
        try_search(
            sources(),
            SearchQuery::new(pred(vec![cond(
                PredicateTarget::Attribute(TagScope::Span, "x".into()),
                CompareOp::Regex,
                vec![text("(")],
            )])),
        ),
        Err(SearchRequestError::Predicate(PredicateError::InvalidPattern { .. }))
    ));
    // Stage-B constructs: named not-yet-evaluable request errors.
    for stage_b in [
        cond(
            PredicateTarget::Attribute(TagScope::Span, "x".into()),
            CompareOp::NotEq,
            vec![text("v")],
        ),
        cond(
            PredicateTarget::Attribute(TagScope::Event, "msg".into()),
            CompareOp::Eq,
            vec![text("v")],
        ),
        cond(
            PredicateTarget::UnscopedAttribute("x".into()),
            CompareOp::Eq,
            vec![text("v")],
        ),
        intrinsic(TraceIntrinsic::EventName, CompareOp::Eq, vec![text("e")]),
        intrinsic(
            TraceIntrinsic::TraceDuration,
            CompareOp::Gt,
            vec![PredicateValue::Integer(5)],
        ),
        intrinsic(TraceIntrinsic::SpanId, CompareOp::Eq, vec![text("00f067aa0ba902b7")]),
    ] {
        assert!(
            matches!(
                try_search(sources(), SearchQuery::new(pred(vec![stage_b.clone()]))),
                Err(SearchRequestError::Predicate(PredicateError::NotYetEvaluable { .. }))
            ),
            "stage-B construct must be rejected: {stage_b:?}"
        );
    }
    // Window ⊄ completion (R2-7): membership by SourceId.
    let bad = SearchSources {
        window: vec![sealed_source(dir.path(), &wal, "other-id")],
        completion: vec![sealed_source(dir.path(), &wal, "one")],
    };
    assert!(matches!(
        try_search(bad, SearchQuery::new(Predicate::all())),
        Err(SearchRequestError::WindowNotInCompletion(id)) if id.as_str() == "other-id"
    ));
    // Per-role duplicate ids still rejected.
    let dup = SearchSources {
        window: Vec::new(),
        completion: vec![
            sealed_source(dir.path(), &wal, "one"),
            sealed_source(dir.path(), &wal, "one"),
        ],
    };
    assert!(matches!(
        try_search(dup, SearchQuery::new(Predicate::all())),
        Err(SearchRequestError::SourceSet(_))
    ));
}

/// A source vanishing AFTER validation (retention unlink between
/// phases) is a SourceFailure on the result — not a request error, not
/// a silent skip — and surviving summaries are no longer exact.
#[test]
fn post_validation_vanish_is_a_source_failure() {
    let dir = tempfile::tempdir().unwrap();
    let wal_early = write_wal(dir.path(), early_requests(), "early");
    let wal_late = write_wal(dir.path(), late_requests(), "late");
    let build = || {
        vec![
            sealed_source(dir.path(), &wal_early, "early"),
            sealed_source(dir.path(), &wal_late, "late"),
        ]
    };
    let sources = both_roles(build);
    // The "late" file is unlinked after the source set was built (the
    // sealed_source helper wrote it to dir/late.sfst).
    std::fs::remove_file(dir.path().join("late.sfst")).unwrap();

    let data = run(sources, SearchQuery::new(Predicate::all()));
    assert!(data.status.has(PartialReason::SourceFailure));
    // The early half still answers; nothing is exact (assembly is
    // degraded — the vanished file's spans may be missing anywhere).
    assert!(!data.traces.is_empty());
    assert!(data.traces.iter().all(|t| !t.exact));
}

/// Cancellation is ALL-OR-EMPTY: a token cancelled up front returns the
/// empty result + Cancelled, never a partial prefix.
#[test]
fn cancellation_is_all_or_empty() {
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), early_requests(), "w");
    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let cancel = CancellationToken::new();
    cancel.cancel();
    let data = search(
        sources,
        SearchQuery::new(Predicate::all()),
        cancel,
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap();
    assert!(data.traces.is_empty());
    assert!(data.status.has(PartialReason::Cancelled));
}

/// Results are identical under source-order permutation (the engine
/// iterates snapshots in SourceId order, so ceilings and ordering are
/// functions of the SET, not the Vec).
#[test]
fn determinism_under_source_permutation() {
    let dir = tempfile::tempdir().unwrap();
    let wal_early = write_wal(dir.path(), early_requests(), "early");
    let wal_late = write_wal(dir.path(), late_requests(), "late");
    let forward = || {
        vec![
            sealed_source(dir.path(), &wal_early, "early"),
            tail_source(&wal_late, "late-tail"),
        ]
    };
    let reverse = || {
        vec![
            tail_source(&wal_late, "late-tail"),
            sealed_source(dir.path(), &wal_early, "early"),
        ]
    };
    let q = || SearchQuery::new(pred(vec![attr_eq(TagScope::Resource, "service.name", "svc-b")]));
    let a = run(both_roles(forward), q());
    let b = run(both_roles(reverse), q());
    assert_eq!(norm(&a), norm(&b));
    assert!(!a.traces.is_empty());
}

/// A corpus whose every candidate keeps threatening the top-1 (inflated
/// raw ranks, ancient canonicals) drives assembly into its ceiling:
/// termination is deterministic, the gathered top result is right, and
/// the status says WorkCeiling.
#[test]
fn assembled_ceiling_terminates_with_the_gathered_result() {
    let dir = tempfile::tempdir().unwrap();
    let svc = vec![kv_str("service.name", "svc")];
    let mut reqs: Vec<ExportTraceServiceRequest> = Vec::new();
    // 80 traces: canonical (matching) at (100+i)s, resend at (10_000+i)s.
    for i in 1..=80u64 {
        reqs.push(req_with(svc.clone(), None, &[SpanSpec {
            attrs: vec![kv_str("m", "yes")],
            ..span_in(i as u8, 0x10, 0, (100 + i) * NS, "canon")
        }]));
    }
    for i in 1..=80u64 {
        reqs.push(req_with(svc.clone(), None, &[SpanSpec {
            attrs: vec![kv_str("m", "yes"), kv_str("resend", "1")],
            ..span_in(i as u8, 0x10, 0, (10_000 + i) * NS, "resend")
        }]));
    }
    let wal = write_wal(dir.path(), reqs, "ceiling");

    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let data = run(
        sources,
        SearchQuery::new(pred(vec![attr_eq(TagScope::Span, "m", "yes")])).limit(1),
    );
    // Ceiling = max(1 × 16, 64) = 64 < 80 candidates: Partial.
    assert!(data.status.has(PartialReason::WorkCeiling));
    // The gathered result is the best CANONICAL among the examined
    // frontier — the raw frontier descends 10_080, 10_079, …, so the
    // first examined candidate (trace 80, canonical 180s) stays #1.
    assert_eq!(ids(&data), vec![hex(80)]);

    // Determinism: an identical second run returns the identical result.
    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let again = run(
        sources,
        SearchQuery::new(pred(vec![attr_eq(TagScope::Span, "m", "yes")])).limit(1),
    );
    assert_eq!(norm(&data), norm(&again));
}

/// The visited-rows ceiling gates refill: with a tiny test ceiling the
/// grow-K loop stops digging, keeps the gathered ranked result, and
/// reports WorkCeiling — deterministically.
#[test]
fn visited_rows_ceiling_gates_refill() {
    let dir = tempfile::tempdir().unwrap();
    let svc = vec![kv_str("service.name", "svc")];
    let mut reqs: Vec<ExportTraceServiceRequest> = Vec::new();
    // 100 inflated traces (canonicals ancient), so the frontier keeps
    // demanding refills far past a 30-row budget.
    for i in 1..=100u64 {
        reqs.push(req_with(svc.clone(), None, &[SpanSpec {
            attrs: vec![kv_str("m", "yes")],
            ..span_in(i as u8, 0x10, 0, (100 + i) * NS, "canon")
        }]));
    }
    for i in 1..=100u64 {
        reqs.push(req_with(svc.clone(), None, &[SpanSpec {
            attrs: vec![kv_str("m", "yes"), kv_str("resend", "1")],
            ..span_in(i as u8, 0x10, 0, (10_000 + i) * NS, "resend")
        }]));
    }
    let wal = write_wal(dir.path(), reqs, "visited");

    let q = || {
        SearchQuery::new(pred(vec![attr_eq(TagScope::Span, "m", "yes")]))
            .limit(1)
            .visited_rows_ceiling_for_tests(30)
    };
    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let data = run(sources, q());
    assert!(data.status.has(PartialReason::WorkCeiling));
    assert_eq!(data.traces.len(), 1, "gathered, ranked, trimmed");
    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let again = run(sources, q());
    assert_eq!(norm(&data), norm(&again), "ceiling termination is deterministic");
}

/// A ceiling crossed by the very LAST match must not turn a provably
/// complete result Partial: an exactly-full extraction that drained the
/// range marks its file exhausted (the exhaustion probe), so no phantom
/// undiscovered threat survives.
#[test]
fn exact_drain_at_the_ceiling_stays_complete() {
    let dir = tempfile::tempdir().unwrap();
    let svc = vec![kv_str("service.name", "svc")];
    // The match must sit ABOVE the range floor (an older non-matching
    // row below it), else the band collapses to `lo` trivially and the
    // exactly-full-band case never arises.
    let reqs = vec![req_with(svc, None, &[
        span_in(7, 0x71, 0, 500 * NS, "older-unrelated"),
        SpanSpec {
            attrs: vec![kv_str("m", "yes")],
            ..span_in(9, 0x91, 0, 1_000 * NS, "the-only-match")
        },
        span_in(8, 0x81, 0, 2_000 * NS, "unrelated"),
    ])];
    let wal = write_wal(dir.path(), reqs, "drain");
    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    // Ceiling 0: the clamp allows exactly one emission, which IS the
    // only match — complete, not WorkCeiling.
    let data = run(
        sources,
        SearchQuery::new(pred(vec![attr_eq(TagScope::Span, "m", "yes")]))
            .visited_rows_ceiling_for_tests(0),
    );
    assert_eq!(ids(&data), vec![hex(9)]);
    assert_eq!(data.status, QueryStatus::Complete);
}

/// UNSET trace ids never become candidates (TRCE carries zeros for
/// them; a by-UNSET group is not a trace).
#[test]
fn unset_trace_ids_are_never_candidates() {
    let dir = tempfile::tempdir().unwrap();
    let svc = vec![kv_str("service.name", "svc")];
    let reqs = vec![req_with(svc, None, &[
        span_in(7, 0x71, 0, 1_000 * NS, "real"),
        SpanSpec {
            trace: [0u8; 16],
            ..sp(0x72, 0, 2_000 * NS, "orphan")
        },
    ])];
    let wal = write_wal(dir.path(), reqs, "unset");
    for build in [
        both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]),
        both_roles(|| vec![tail_source(&wal, "tail")]),
    ] {
        let data = run(build, SearchQuery::new(Predicate::all()));
        assert_eq!(ids(&data), vec![hex(7)], "only the real trace returns");
        assert_eq!(data.status, QueryStatus::Complete);
    }
}

/// An empty window set discovers nothing: a Complete empty result (the
/// caller offered nowhere to look — a data condition, not an error).
#[test]
fn empty_window_set_is_a_complete_empty() {
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), early_requests(), "w");
    let sources = SearchSources {
        window: Vec::new(),
        completion: vec![sealed_source(dir.path(), &wal, "one")],
    };
    let data = run(sources, SearchQuery::new(Predicate::all()));
    assert!(data.traces.is_empty());
    assert_eq!(data.status, QueryStatus::Complete);
}

/// FieldKinds cover exactly the returned traces' attached spans: names
/// the attachment exposes, kinds from the contributing sources.
#[test]
fn field_kinds_describe_the_returned_data() {
    let dir = tempfile::tempdir().unwrap();
    let mut all = early_requests();
    all.extend(late_requests());
    let wal = write_wal(dir.path(), all, "world");
    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let data = run(
        sources,
        SearchQuery::new(pred(vec![attr_eq(TagScope::Span, "queue", "q1")])),
    );
    assert_eq!(ids(&data), vec![hex(4)]);
    let names: Vec<&str> = data.field_kinds.fields.iter().map(|(k, _)| k.as_str()).collect();
    assert!(names.contains(&"attributes.queue"), "{names:?}");
    assert!(names.contains(&"resource.attributes.service.name"), "{names:?}");
    // svc-a's attributes exist in the (single, contributing) file but on
    // no attached span — the projection excludes them.
    assert!(!names.contains(&"attributes.http.method"), "{names:?}");
}

/// Search across ONLY tails (no sealed file at all): the tail path
/// stands alone.
#[test]
fn tail_only_search_matches_the_sealed_answer() {
    let dir = tempfile::tempdir().unwrap();
    let wal_early = write_wal(dir.path(), early_requests(), "early");
    let wal_late = write_wal(dir.path(), late_requests(), "late");
    let q = || SearchQuery::new(pred(vec![attr_eq(TagScope::Resource, "service.name", "svc-b")]));

    let tails = both_roles(|| {
        vec![
            tail_source(&wal_early, "early-tail"),
            tail_source(&wal_late, "late-tail"),
        ]
    });
    let sealed = both_roles(|| {
        vec![
            sealed_source(dir.path(), &wal_early, "early"),
            sealed_source(dir.path(), &wal_late, "late"),
        ]
    });
    assert_eq!(norm(&run(tails, q())), norm(&run(sealed, q())));
}
