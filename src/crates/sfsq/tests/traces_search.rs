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
//!
//! Also pinned: the summary numbers against ground truth (roots,
//! envelope, counts, ranks), the service breakdown's
//! partition of the retained canonical spans, the request-validation
//! matrix, post-validation source vanish, unavailable-source honesty
//! (including the completion slack), the span-cap honesty pair,
//! budget-truncated sources that cannot falsify completeness, the
//! trace:id pin shortcut past discovery, tri-state exclusion of
//! indeterminate trace-level candidates, UNSET trace ids, the empty
//! window set, and the field-kind projection.
//!
//! The op's engine contract lives in `src/traces/search.rs`; the
//! sibling suites in this directory pin the other traces query ops, and
//! `tests/common/mod.rs` owns the shared corpus/source fixtures.

mod common;

use std::collections::BTreeSet;
use std::path::Path;
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

use tokio_util::sync::CancellationToken;

use common::{
    SpanSpec, corrupt_chunk, hide_chunk, kv_str, memory_source, missing_source, req_with,
    sealed_source, sealed_source_at, sp, tail_source, unavailable_source, write_wal,
};
use opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest;
use sfsq::traces::{
    AttributeOwner, BuiltinField, CompareOp, Condition, PartialReason, Predicate, PredicateError,
    PredicateTarget, PredicateValue, QueryStatus, SERVICE_BREAKDOWN_TOP_K, SearchData, SearchQuery,
    SearchRequestError, SearchSources, ServiceBreakdown, TimeWindow, Token, TokenQuery,
    TraceSource, search,
};

/// One second in nanoseconds — the corpus' timestamp unit (`500 * NS`
/// below reads as 500s).
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

fn attr_eq(scope: AttributeOwner, key: &str, value: &str) -> Condition {
    cond(
        PredicateTarget::Attribute(scope, key.to_string()),
        CompareOp::Eq,
        vec![text(value)],
    )
}

fn builtin(i: BuiltinField, op: CompareOp, values: Vec<PredicateValue>) -> Condition {
    cond(PredicateTarget::Builtin(i), op, values)
}

fn pred(conditions: Vec<Condition>) -> Predicate {
    Predicate { conditions }
}

/// The suite's default runner: `search` with a fresh token and a
/// throwaway progress counter, expecting a valid request.
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
    String,                         // trace id (hex)
    Option<String>,                 // root_service
    Option<String>,                 // root_name
    i64,                            // start_ns
    i64,                            // duration_ns
    usize,                          // span_count
    usize,                          // error_count
    i64,                            // newest_matched_start_ns (the rank)
    bool,                           // exact
    (Vec<(String, u64)>, u64, u64), // service breakdown
);

/// The whole result — rows plus status — in comparable form: the
/// equivalence oracle's unit of comparison across layouts.
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
                t.newest_matched_start_ns,
                t.exact,
                (
                    t.service_breakdown.top.clone(),
                    t.service_breakdown.other,
                    t.service_breakdown.unattributed,
                ),
            )
        })
        .collect();
    (traces, data.status.clone())
}

/// Positional expected-value constructor for `ServiceBreakdown` asserts.
fn breakdown(top: &[(&str, u64)], other: u64, unattributed: u64) -> ServiceBreakdown {
    ServiceBreakdown {
        top: top.iter().map(|&(s, n)| (s.to_string(), n)).collect(),
        other,
        unattributed,
    }
}

/// Every returned row's breakdown partitions exactly the spans its
/// `span_count` counts, within the cap.
fn assert_breakdowns_partition(data: &SearchData) {
    for t in &data.traces {
        let b = &t.service_breakdown;
        let mut total = b.other + b.unattributed;
        for (_, spans) in &b.top {
            total += spans;
        }
        assert_eq!(total, t.span_count as u64, "trace {}", t.trace_id);
        assert!(
            b.top.len() <= SERVICE_BREAKDOWN_TOP_K,
            "trace {}",
            t.trace_id
        );
    }
}

// ── The shared world (six traces, two services, two time halves) ──────

/// A `sp()` span placed in shared-world trace `trace` (trace ids are
/// the byte `trace` in the tid's last position).
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
        req_with(
            svc_a,
            None,
            &[
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
                    events: vec![
                        ("exception", vec![kv_str("exception.type", "IOError")]),
                        ("retry", vec![kv_str("attempt", "2")]),
                    ],
                    ..span_in(2, 0x21, 0, 1_100 * NS, "POST /orders")
                },
                SpanSpec {
                    status: Some((1, "")),
                    ..span_in(3, 0x31, 0, 900 * NS, "GET /health")
                },
            ],
        ),
        req_with(
            svc_b,
            None,
            &[
                SpanSpec {
                    kind: 5, // CONSUMER
                    attrs: vec![kv_str("queue", "q1"), kv_str("retries", "3")],
                    links: vec![
                        (tid(1), [0x11; 8], vec![kv_str("rel", "follows")]),
                        (tid(2), [0x22; 8], vec![kv_str("rel", "other")]),
                    ],
                    ..span_in(4, 0x41, 0, 1_200 * NS, "consume")
                },
                // T6's CANONICAL copy (chronologically first).
                SpanSpec {
                    attrs: vec![kv_str("flag", "on")],
                    ..span_in(6, 0x61, 0, 1_300 * NS, "flagged")
                },
            ],
        ),
    ]
}

/// The requests of the LATE half (wal2): T5 on svc-b, T3's late child on
/// svc-a, T6's LOSING resend (same span id, different content, later).
fn late_requests() -> Vec<ExportTraceServiceRequest> {
    let svc_a = vec![kv_str("service.name", "svc-a"), kv_str("env", "prod")];
    let svc_b = vec![kv_str("service.name", "svc-b")];
    vec![
        req_with(
            svc_b.clone(),
            None,
            &[
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
            ],
        ),
        req_with(
            svc_a,
            None,
            &[SpanSpec {
                attrs: vec![kv_str("http.method", "GET")],
                ..span_in(3, 0x32, 0x31, 1_900 * NS, "late-check")
            }],
        ),
        req_with(
            svc_b,
            None,
            &[SpanSpec {
                attrs: vec![kv_str("ghost", "yes")],
                ..span_in(6, 0x61, 0, 1_950 * NS, "flagged")
            }],
        ),
    ]
}

/// Both roles over the same paths, each built fresh per call (sources
/// are consumed by `search`; identical SourceId sets trivially satisfy
/// the window ⊆ completion membership check).
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

/// A query on the assembly path (rows off): for the tests that pin
/// canonical-assembly properties — resend dedup, canonical ranks, the
/// span cap — which the rows path deliberately does not have.
fn canon_query(tokens: impl Into<TokenQuery>) -> SearchQuery {
    SearchQuery::new(tokens).rows_for_tests(false)
}

/// Shared-world trace `n` hex-rendered, for expectation lists.
fn hex(n: u8) -> String {
    sfst::TraceId::from(tid(n)).to_string()
}

/// Every layout answers every query identically — id order checked
/// against the hand-computed expectations everywhere, full summaries
/// against the single-sealed reference — with every row exact and the
/// status Complete (final ordering by canonical matched-span start; T6
/// ranks by its CANONICAL 1300s copy, not the 1950s resend).
#[test]
fn oracle_equivalence_under_relayouts() {
    let dir = tempfile::tempdir().unwrap();
    let queries: Vec<(&str, SearchQuery, Vec<String>)> = vec![
        (
            "all",
            canon_query(Predicate::all()),
            vec![hex(3), hex(5), hex(6), hex(4), hex(2), hex(1)],
        ),
        (
            "service",
            canon_query(pred(vec![attr_eq(
                AttributeOwner::Resource,
                "service.name",
                "svc-a",
            )])),
            vec![hex(3), hex(2), hex(1)],
        ),
        (
            // T1's "GET /users" (1000s) outranks T3's "GET /health"
            // (900s): only the ROOT of T3 matches this pattern, so T3
            // ranks by 900s, not by its non-matching 1900s child.
            "name-regex",
            canon_query(pred(vec![builtin(
                BuiltinField::Name,
                CompareOp::Regex,
                vec![text("GET .*")],
            )])),
            vec![hex(1), hex(3)],
        ),
        (
            "status-error",
            canon_query(pred(vec![builtin(
                BuiltinField::Status,
                CompareOp::Eq,
                vec![text("ERROR")],
            )])),
            vec![hex(5), hex(2)],
        ),
        (
            "kind-keyword",
            canon_query(pred(vec![builtin(
                BuiltinField::Kind,
                CompareOp::Eq,
                vec![text("CONSUMER")],
            )])),
            vec![hex(4)],
        ),
        (
            // Regex over the kind LABEL set is stage-A evaluable (26A):
            // matches T4's CONSUMER (1200s) and T1's SERVER (1000s).
            "kind-regex",
            canon_query(pred(vec![builtin(
                BuiltinField::Kind,
                CompareOp::Regex,
                vec![text("SERVER|CONSUMER")],
            )])),
            vec![hex(4), hex(1)],
        ),
        (
            "duration-floor",
            canon_query(pred(vec![builtin(
                BuiltinField::Duration,
                CompareOp::Gte,
                vec![PredicateValue::Integer(1_000_000)],
            )])),
            vec![hex(5)],
        ),
        (
            // Span-group semantics: ONE span carries both conditions.
            "conjunction",
            canon_query(pred(vec![
                attr_eq(AttributeOwner::Resource, "service.name", "svc-a"),
                attr_eq(AttributeOwner::Span, "http.method", "GET"),
            ])),
            vec![hex(3), hex(1)],
        ),
        (
            // Window [1050s, 1600s): T6's canonical (1300s) is inside —
            // ranked by it; T1/T3 have no span starting inside.
            "window",
            canon_query(Predicate::all())
                .window(TimeWindow::new(1_050 * NS as i64, 1_600 * NS as i64).unwrap()),
            vec![hex(5), hex(6), hex(4), hex(2)],
        ),
        (
            // The losing-resend-only direction: `ghost` exists only on
            // T6's resend copy; the canonical copy wins dedup, fails
            // re-evaluation, and the trace is dropped — never a false
            // positive from raw phase-1.
            "ghost-only-on-loser",
            canon_query(pred(vec![attr_eq(AttributeOwner::Span, "ghost", "yes")])),
            vec![],
        ),
        (
            // The canonical-matches direction: `flag` exists only on the
            // canonical copy; found regardless of the newer resend.
            "flag-on-canonical",
            canon_query(pred(vec![attr_eq(AttributeOwner::Span, "flag", "on")])),
            vec![hex(6)],
        ),
        (
            "multi-value-or",
            canon_query(pred(vec![cond(
                PredicateTarget::Attribute(AttributeOwner::Span, "http.method".into()),
                CompareOp::Eq,
                vec![text("GET"), text("POST")],
            )])),
            vec![hex(3), hex(2), hex(1)],
        ),
        (
            // Negation: a span with the attribute PRESENT and ≠ GET —
            // only T2's POST span; spans without http.method never
            // satisfy a negated comparison.
            "negated-attribute",
            canon_query(pred(vec![cond(
                PredicateTarget::Attribute(AttributeOwner::Span, "http.method".into()),
                CompareOp::NotEq,
                vec![text("GET")],
            )])),
            vec![hex(2)],
        ),
        (
            // Negated-absent + resend direction: `ghost` exists ONLY on
            // T6's losing resend, so no CANONICAL span has it present —
            // nothing satisfies `ghost != x`, however the raw copies
            // look.
            "negated-absent-attribute",
            canon_query(pred(vec![cond(
                PredicateTarget::Attribute(AttributeOwner::Span, "ghost".into()),
                CompareOp::NotEq,
                vec![text("x")],
            )])),
            vec![],
        ),
        (
            // Negated builtin: every span stores its kind label, the
            // UNSPECIFIED default included, so `kind != SERVER` matches
            // every trace owning a non-SERVER span — all six (T1 through
            // its UNSPECIFIED child).
            "negated-kind",
            canon_query(pred(vec![builtin(
                BuiltinField::Kind,
                CompareOp::NotEq,
                vec![text("SERVER")],
            )])),
            vec![hex(3), hex(5), hex(6), hex(4), hex(2), hex(1)],
        ),
        (
            // The UNSPECIFIED default is a stored, selectable kind.
            "kind-unspecified",
            canon_query(pred(vec![builtin(
                BuiltinField::Kind,
                CompareOp::Eq,
                vec![text("UNSPECIFIED")],
            )])),
            vec![hex(3), hex(5), hex(6), hex(2), hex(1)],
        ),
        (
            // The UNSET default is a stored, selectable status: every
            // span sent without a status (T1's child, T3's late child,
            // T4, T6's canonical copy).
            "status-unset",
            canon_query(pred(vec![builtin(
                BuiltinField::Status,
                CompareOp::Eq,
                vec![text("UNSET")],
            )])),
            vec![hex(3), hex(6), hex(4), hex(1)],
        ),
        (
            // `status != ERROR` reaches UNSET spans too, not only OK ones
            // (which alone would give T1 and T3).
            "status-not-error",
            canon_query(pred(vec![builtin(
                BuiltinField::Status,
                CompareOp::NotEq,
                vec![text("ERROR")],
            )])),
            vec![hex(3), hex(6), hex(4), hex(1)],
        ),
        (
            // The any-owner disjunction: `env` is a RESOURCE attribute of
            // svc-a; the any-owner lookup reaches it.
            "any-owner-attribute",
            canon_query(pred(vec![cond(
                PredicateTarget::Attribute(AttributeOwner::Any, "env".into()),
                CompareOp::Eq,
                vec![text("prod")],
            )])),
            vec![hex(3), hex(2), hex(1)],
        ),
        (
            // Dictionary numerics: `retries` renders as the token "3".
            "numeric-attribute",
            canon_query(pred(vec![cond(
                PredicateTarget::Attribute(AttributeOwner::Span, "retries".into()),
                CompareOp::Gte,
                vec![PredicateValue::Integer(2)],
            )])),
            vec![hex(4)],
        ),
        (
            // Numeric equality accepts the float spelling of an integer
            // token (the dictionary-numeric rule, not byte equality).
            "numeric-float-eq",
            canon_query(pred(vec![cond(
                PredicateTarget::Attribute(AttributeOwner::Span, "retries".into()),
                CompareOp::Eq,
                vec![PredicateValue::Float(3.0)],
            )])),
            vec![hex(4)],
        ),
        (
            // span:id — the SPAN column scan; T5's middle child.
            "span-id",
            canon_query(pred(vec![builtin(
                BuiltinField::SpanId,
                CompareOp::Eq,
                vec![text("5252525252525252")],
            )])),
            vec![hex(5)],
        ),
        (
            // span:parentID — children of T5's root.
            "parent-id",
            canon_query(pred(vec![builtin(
                BuiltinField::ParentSpanId,
                CompareOp::Eq,
                vec![text("5151515151515151")],
            )])),
            vec![hex(5)],
        ),
        (
            // Negated parent id: parent PRESENT and ≠ T5's root — roots
            // (unset parent = absent) never satisfy it.
            "negated-parent-id",
            canon_query(pred(vec![builtin(
                BuiltinField::ParentSpanId,
                CompareOp::NotEq,
                vec![text("5151515151515151")],
            )])),
            vec![hex(3), hex(1)],
        ),
        (
            // trace:id pins the candidate set; the span-local remainder
            // still applies in phase 2.
            "trace-id-pin",
            canon_query(pred(vec![
                builtin(
                    BuiltinField::TraceId,
                    CompareOp::Eq,
                    vec![text(&hex(5)), text(&hex(2))],
                ),
                builtin(BuiltinField::Status, CompareOp::Eq, vec![text("ERROR")]),
            ])),
            vec![hex(5), hex(2)],
        ),
        (
            // A pin whose remainder fails phase-2 re-evaluation drops.
            "trace-id-pin-no-match",
            canon_query(pred(vec![
                builtin(BuiltinField::TraceId, CompareOp::Eq, vec![text(&hex(1))]),
                builtin(BuiltinField::Status, CompareOp::Eq, vec![text("ERROR")]),
            ])),
            vec![],
        ),
        (
            // trace:id != excludes at pool admission.
            "trace-id-excluded",
            canon_query(pred(vec![
                builtin(BuiltinField::Status, CompareOp::Eq, vec![text("ERROR")]),
                builtin(BuiltinField::TraceId, CompareOp::NotEq, vec![text(&hex(5))]),
            ])),
            vec![hex(2)],
        ),
        (
            // link:spanID / link:traceID — the LNKB row scan; T4 links
            // to T1's root span.
            "link-span-id",
            canon_query(pred(vec![builtin(
                BuiltinField::LinkSpanId,
                CompareOp::Eq,
                vec![text("1111111111111111")],
            )])),
            vec![hex(4)],
        ),
        (
            "link-trace-id",
            canon_query(pred(vec![builtin(
                BuiltinField::LinkTraceId,
                CompareOp::Eq,
                vec![text(&hex(1))],
            )])),
            vec![hex(4)],
        ),
        (
            // event:name rides the flat events.name token; the span-side
            // evaluator gathers from the STRUCTURED event names.
            "event-name",
            canon_query(pred(vec![builtin(
                BuiltinField::EventName,
                CompareOp::Eq,
                vec![text("exception")],
            )])),
            vec![hex(2)],
        ),
        (
            "event-name-regex",
            canon_query(pred(vec![builtin(
                BuiltinField::EventName,
                CompareOp::Regex,
                vec![text("exc.*")],
            )])),
            vec![hex(2)],
        ),
        (
            // Event attribute through the structural refine.
            "event-attr",
            canon_query(pred(vec![attr_eq(
                AttributeOwner::Event,
                "exception.type",
                "IOError",
            )])),
            vec![hex(2)],
        ),
        (
            // Same-event conjunction: name + attribute on ONE event.
            "event-subgroup-same",
            canon_query(pred(vec![
                builtin(BuiltinField::EventName, CompareOp::Eq, vec![text("retry")]),
                attr_eq(AttributeOwner::Event, "attempt", "2"),
            ])),
            vec![hex(2)],
        ),
        (
            // THE refine-conjunction counterexample: both flat tokens
            // exist on T2's span (different events), but NO single event
            // carries both — the mandatory refine rejects what a loose
            // span-level match would leak.
            "event-subgroup-counterexample",
            canon_query(pred(vec![
                attr_eq(AttributeOwner::Event, "exception.type", "IOError"),
                attr_eq(AttributeOwner::Event, "attempt", "2"),
            ])),
            vec![],
        ),
        (
            // Cross-condition on names: no single event is named both.
            "event-two-names-unsatisfiable",
            canon_query(pred(vec![
                builtin(
                    BuiltinField::EventName,
                    CompareOp::Eq,
                    vec![text("exception")],
                ),
                builtin(BuiltinField::EventName, CompareOp::Eq, vec![text("retry")]),
            ])),
            vec![],
        ),
        (
            // event:timeSinceStart computes in the refine (the corpus
            // writes events at start + 1ns).
            "event-time-since-start",
            canon_query(pred(vec![builtin(
                BuiltinField::EventTimeSinceStart,
                CompareOp::Lte,
                vec![PredicateValue::Integer(1)],
            )])),
            vec![hex(2)],
        ),
        (
            "event-time-since-start-none",
            canon_query(pred(vec![builtin(
                BuiltinField::EventTimeSinceStart,
                CompareOp::Gt,
                vec![PredicateValue::Integer(1)],
            )])),
            vec![],
        ),
        (
            // Link attribute + link id on the SAME link.
            "link-subgroup-same",
            canon_query(pred(vec![
                attr_eq(AttributeOwner::Link, "rel", "follows"),
                builtin(
                    BuiltinField::LinkSpanId,
                    CompareOp::Eq,
                    vec![text("1111111111111111")],
                ),
            ])),
            vec![hex(4)],
        ),
        (
            // The LINK counterexample: attribute from one link, id from
            // the other — no single link has both.
            "link-subgroup-counterexample",
            canon_query(pred(vec![
                attr_eq(AttributeOwner::Link, "rel", "other"),
                builtin(
                    BuiltinField::LinkSpanId,
                    CompareOp::Eq,
                    vec![text("1111111111111111")],
                ),
            ])),
            vec![],
        ),
        (
            "link-subgroup-other",
            canon_query(pred(vec![
                attr_eq(AttributeOwner::Link, "rel", "other"),
                builtin(
                    BuiltinField::LinkSpanId,
                    CompareOp::Eq,
                    vec![text("2222222222222222")],
                ),
            ])),
            vec![hex(4)],
        ),
        (
            // Trace-level only: phase-1 candidates come from All over
            // the window (C-4); the root evaluates post-assembly.
            "root-service",
            canon_query(pred(vec![builtin(
                BuiltinField::RootServiceName,
                CompareOp::Eq,
                vec![text("svc-a")],
            )])),
            vec![hex(3), hex(2), hex(1)],
        ),
        (
            "root-name-regex",
            canon_query(pred(vec![builtin(
                BuiltinField::RootName,
                CompareOp::Regex,
                vec![text("GET .*")],
            )])),
            vec![hex(3), hex(1)],
        ),
        (
            "negated-root-name",
            canon_query(pred(vec![builtin(
                BuiltinField::RootName,
                CompareOp::NotEq,
                vec![text("GET /users")],
            )])),
            vec![hex(3), hex(5), hex(6), hex(4), hex(2)],
        ),
        (
            // Envelope duration: only T3 spans a second-scale envelope.
            "trace-duration",
            canon_query(pred(vec![builtin(
                BuiltinField::TraceDuration,
                CompareOp::Gt,
                vec![PredicateValue::Integer((100 * NS) as i64)],
            )])),
            vec![hex(3)],
        ),
        (
            // MIXED: the span-local half lowers to the file plans, the
            // trace-level half applies post-assembly (C-4).
            "mixed-trace-level",
            canon_query(pred(vec![
                builtin(BuiltinField::Status, CompareOp::Eq, vec![text("ERROR")]),
                builtin(
                    BuiltinField::RootServiceName,
                    CompareOp::Eq,
                    vec![text("svc-b")],
                ),
            ])),
            vec![hex(5)],
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
            // One loaded session at a time: every step releases the
            // previous file, and the answer is the same.
            let capped = run(
                layout(dir.path(), &world, lname),
                query.clone().loaded_sources_cap_for_tests(1),
            );
            assert_eq!(
                norm(&capped),
                got,
                "query {qname} on {lname}: releasing sessions changed the answer"
            );
            match &reference {
                None => reference = Some(got),
                Some(want) => {
                    assert_eq!(&got, want, "query {qname}: layout {lname} diverges")
                }
            }
        }
    }
}

/// Discovery reads its columns through the sessions: with one loaded
/// session at a time, a file's later bands decode its columns again
/// after other files' reads released them, and the answer equals the
/// default limit's — for match-all discovery and for a two-token query
/// (every token compiled per file).
#[test]
fn discovery_survives_session_release() {
    let dir = tempfile::tempdir().unwrap();
    // Four files interleaved in time: file f holds traces f+1, f+5, ….
    let wals: Vec<_> = (0..4u8)
        .map(|f| {
            let spans: Vec<SpanSpec> = (0..6u8)
                .map(|i| {
                    let t = f + 1 + i * 4;
                    let name = if t % 3 == 0 { "hot" } else { "op" };
                    let mut s = sp(1, 0, u64::from(t) * 1_000_000_000, name);
                    s.trace = [0; 16];
                    s.trace[15] = t;
                    s
                })
                .collect();
            let svc = vec![kv_str("service.name", "svc")];
            write_wal(
                dir.path(),
                vec![req_with(svc, None, &spans)],
                &format!("f{f}"),
            )
        })
        .collect();
    let sources = || {
        both_roles(|| {
            wals.iter()
                .enumerate()
                .map(|(f, wal)| sealed_source(dir.path(), wal, &format!("f{f}")))
                .collect()
        })
    };
    let any_name = builtin(BuiltinField::Name, CompareOp::Regex, vec![text("op|hot")]);
    for (query, want) in [
        (
            SearchQuery::new(pred(vec![])).limit(3),
            vec![hex(24), hex(23), hex(22)],
        ),
        (
            SearchQuery::new(tokens(vec![
                Token::positive(pred(vec![any_name])),
                Token::positive(pred(vec![name_eq("op")])),
            ]))
            .limit(3),
            vec![hex(23), hex(22), hex(20)],
        ),
    ] {
        let wide = run(sources(), query.clone());
        assert_eq!(ids(&wide), want);
        let narrow = run(sources(), query.loaded_sources_cap_for_tests(1));
        assert_eq!(norm(&narrow), norm(&wide));
    }
}

/// Summary numbers against ground truth (single layout; equivalence is
/// proven above): roots, envelope, counts, ranks.
#[test]
fn summary_ground_truth() {
    let dir = tempfile::tempdir().unwrap();
    let mut all = early_requests();
    all.extend(late_requests());
    let wal = write_wal(dir.path(), all, "world");
    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);

    let data = run(sources, canon_query(Predicate::all()));
    assert_breakdowns_partition(&data);
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
    assert_eq!((t1.span_count, t1.error_count), (2, 0));
    assert_eq!(t1.service_breakdown, breakdown(&[("svc-a", 2)], 0, 0));
    // Ranked by the newest matching span: the child.
    assert_eq!(t1.newest_matched_start_ns, (1_000 * NS + 10) as i64);

    // T3: spans scattered across the halves; envelope spans both.
    let t3 = by_id(3);
    assert_eq!(t3.root_name.as_deref(), Some("GET /health"));
    assert_eq!(t3.start_ns, (900 * NS) as i64);
    assert_eq!(t3.duration_ns, (1_000 * NS + 50) as i64);
    assert_eq!(t3.span_count, 2);
    assert_eq!(t3.service_breakdown, breakdown(&[("svc-a", 2)], 0, 0));

    // T5: all three spans carry ERROR.
    let t5 = by_id(5);
    assert_eq!((t5.span_count, t5.error_count), (3, 3));
    assert_eq!(t5.service_breakdown, breakdown(&[("svc-b", 3)], 0, 0));
    assert_eq!(t5.newest_matched_start_ns, (1_520 * NS) as i64);

    // T6: the resend collapsed to the canonical copy.
    let t6 = by_id(6);
    assert_eq!((t6.span_count, t6.start_ns), (1, (1_300 * NS) as i64));
    assert_eq!(t6.service_breakdown, breakdown(&[("svc-b", 1)], 0, 0));
}

/// The per-service breakdown over the retained canonical spans: ranked
/// by span count then name (ties at the cut included), capped with the
/// tail folded into `other`, service-less spans
/// counted as `unattributed`, a resend from another service counted
/// once under its canonical copy's service, and a capped trace
/// partitioning only its retained spans.
#[test]
fn service_breakdown_partitions_the_retained_canonical_spans() {
    let dir = tempfile::tempdir().unwrap();
    let svc = |name: &str| vec![kv_str("service.name", name)];
    // Trace 21: frontend 3, auth 2, cart 2 (a name tie), 2 spans with no
    // service, and a later resend of a cart span from another service.
    let mut reqs = vec![
        req_with(
            svc("frontend"),
            None,
            &[
                span_in(21, 0xA1, 0, 5_000 * NS, "root"),
                span_in(21, 0xA2, 0xA1, 5_001 * NS, "op"),
                span_in(21, 0xA3, 0xA1, 5_002 * NS, "op"),
            ],
        ),
        req_with(
            svc("cart"),
            None,
            &[
                span_in(21, 0xA4, 0xA1, 5_003 * NS, "op"),
                span_in(21, 0xA5, 0xA1, 5_004 * NS, "op"),
            ],
        ),
        req_with(
            svc("auth"),
            None,
            &[
                span_in(21, 0xA6, 0xA1, 5_005 * NS, "op"),
                span_in(21, 0xA7, 0xA1, 5_006 * NS, "op"),
            ],
        ),
        req_with(
            vec![kv_str("env", "prod")],
            None,
            &[
                span_in(21, 0xA8, 0xA1, 5_007 * NS, "op"),
                span_in(21, 0xA9, 0xA1, 5_008 * NS, "op"),
            ],
        ),
        req_with(
            svc("zz-resend"),
            None,
            &[span_in(21, 0xA4, 0xA1, 5_009 * NS, "op")],
        ),
    ];
    // Trace 22: seven services, more than the cap. foxtrot ties delta and
    // echo at the cut and is seen before them, yet folds by name.
    let services = [
        ("alpha", 4),
        ("foxtrot", 2),
        ("golf", 1),
        ("echo", 2),
        ("delta", 2),
        ("charlie", 3),
        ("bravo", 3),
    ];
    let mut id = 0xB0u8;
    let mut start = 6_000 * NS;
    for (service, spans) in services {
        let mut specs = Vec::new();
        for _ in 0..spans {
            let parent = if id == 0xB0 { 0 } else { 0xB0 };
            specs.push(span_in(22, id, parent, start, "op"));
            id += 1;
            start += NS;
        }
        reqs.push(req_with(svc(service), None, &specs));
    }
    let wal = write_wal(dir.path(), reqs, "breakdown");

    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let data = run(sources, canon_query(Predicate::all()));
    assert_eq!(ids(&data), vec![hex(22), hex(21)]);
    assert_breakdowns_partition(&data);
    let (t22, t21) = (&data.traces[0], &data.traces[1]);
    assert_eq!(t21.span_count, 9);
    assert_eq!(
        t21.service_breakdown,
        breakdown(&[("frontend", 3), ("auth", 2), ("cart", 2)], 0, 2)
    );
    assert_eq!(t22.span_count, 17);
    assert_eq!(
        t22.service_breakdown,
        breakdown(
            &[
                ("alpha", 4),
                ("bravo", 3),
                ("charlie", 3),
                ("delta", 2),
                ("echo", 2)
            ],
            3,
            0
        )
    );

    // Filtered to cart: only trace 21 matches, on two spans, yet its
    // breakdown still covers every retained span, not just the matched ones.
    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let filtered = run(
        sources,
        canon_query(pred(vec![attr_eq(
            AttributeOwner::Resource,
            "service.name",
            "cart",
        )])),
    );
    assert_eq!(ids(&filtered), vec![hex(21)]);
    assert_breakdowns_partition(&filtered);
    let t21 = &filtered.traces[0];
    assert_eq!(t21.span_count, 9);
    // Ranked by its newest cart span, not by the trace's newest span.
    assert_eq!(t21.newest_matched_start_ns, (5_004 * NS) as i64);
    assert_eq!(
        t21.service_breakdown,
        breakdown(&[("frontend", 3), ("auth", 2), ("cart", 2)], 0, 2)
    );

    // Capped at 2 spans: each trace keeps its two earliest, inexact, and
    // the breakdown partitions exactly those.
    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let capped = run(sources, canon_query(Predicate::all()).span_cap_for_tests(2));
    assert!(capped.status.has(PartialReason::SizeCap));
    assert_breakdowns_partition(&capped);
    let (t22, t21) = (&capped.traces[0], &capped.traces[1]);
    assert!(!t21.exact && !t22.exact);
    assert_eq!(t21.service_breakdown, breakdown(&[("frontend", 2)], 0, 0));
    assert_eq!(t22.service_breakdown, breakdown(&[("alpha", 2)], 0, 0));
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
        reqs.push(req_with(
            svc.clone(),
            None,
            &[SpanSpec {
                attrs: vec![kv_str("m", "yes")],
                ..span_in(i, 0x10 + i, 0, (100 + u64::from(i)) * NS, "canon")
            }],
        ));
    }
    for i in 1..=5u8 {
        reqs.push(req_with(
            svc.clone(),
            None,
            &[SpanSpec {
                attrs: vec![kv_str("m", "yes"), kv_str("resend", "1")],
                ..span_in(i, 0x10 + i, 0, (1_000 + u64::from(i)) * NS, "resend")
            }],
        ));
    }
    // T6: a single span at 500s — the TRUE canonical #1 (every other
    // trace's canonical sits at (100+i)s), yet its raw rank (500s)
    // sits below every inflated raw rank (1001+s), outside the initial
    // K: only a refill can find it.
    reqs.push(req_with(
        svc.clone(),
        None,
        &[SpanSpec {
            attrs: vec![kv_str("m", "yes")],
            ..span_in(6, 0x66, 0, 500 * NS, "hidden-winner")
        }],
    ));
    let wal = write_wal(dir.path(), reqs, "inflated");

    let matching = pred(vec![attr_eq(AttributeOwner::Span, "m", "yes")]);
    // limit 1 → initial K = 3: the raw top-3 are all inflated resends.
    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let top = run(sources, canon_query(matching.clone()).limit(1));
    assert_eq!(ids(&top), vec![hex(6)], "the hidden winner surfaces");
    assert_eq!(top.status, QueryStatus::Complete);

    // Full ordering: canonical ranks throughout (500, then 105..101).
    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let all = run(sources, canon_query(matching).limit(10));
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
        canon_query(Predicate::all())
            .window(TimeWindow::new(1_890 * NS as i64, 2_000 * NS as i64).unwrap()),
    );
    assert_eq!(ids(&data), vec![hex(3)]);
    // Root-outside-window: the summary still derives from the real root
    // and the FULL retained set (envelope reaches back to 900s).
    let t3 = &data.traces[0];
    assert_eq!(t3.root_name.as_deref(), Some("GET /health"));
    assert_eq!(t3.start_ns, (900 * NS) as i64);
    assert_eq!(t3.span_count, 2);
    // …but the rank follows predicate + window.
    assert_eq!(t3.newest_matched_start_ns, (1_900 * NS) as i64);
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
    // Structural predicate error.
    assert!(matches!(
        try_search(
            sources(),
            SearchQuery::new(pred(vec![cond(
                PredicateTarget::Attribute(AttributeOwner::Span, "x".into()),
                CompareOp::Regex,
                vec![text("(")],
            )])),
        ),
        Err(SearchRequestError::Predicate(
            PredicateError::InvalidPattern { .. }
        ))
    ));
    // Stage-B constructs: named not-yet-evaluable request errors.
    for stage_b in [
        // Negated event/link subgroup forms stay not-yet-evaluable —
        // their semantics are the recorded open question (flat negation
        // vs single-item subgroup) until the stage boundary relaxes.
        cond(
            PredicateTarget::Attribute(AttributeOwner::Event, "msg".into()),
            CompareOp::NotEq,
            vec![text("v")],
        ),
        builtin(BuiltinField::EventName, CompareOp::NotEq, vec![text("e")]),
        builtin(
            BuiltinField::LinkSpanId,
            CompareOp::NotEq,
            vec![text("00f067aa0ba902b7")],
        ),
    ] {
        assert!(
            matches!(
                try_search(sources(), SearchQuery::new(pred(vec![stage_b.clone()]))),
                Err(SearchRequestError::Predicate(
                    PredicateError::NotYetEvaluable { .. }
                ))
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
    let q = || {
        SearchQuery::new(pred(vec![attr_eq(
            AttributeOwner::Resource,
            "service.name",
            "svc-b",
        )]))
    };
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
        reqs.push(req_with(
            svc.clone(),
            None,
            &[SpanSpec {
                attrs: vec![kv_str("m", "yes")],
                ..span_in(i as u8, 0x10, 0, (100 + i) * NS, "canon")
            }],
        ));
    }
    for i in 1..=80u64 {
        reqs.push(req_with(
            svc.clone(),
            None,
            &[SpanSpec {
                attrs: vec![kv_str("m", "yes"), kv_str("resend", "1")],
                ..span_in(i as u8, 0x10, 0, (10_000 + i) * NS, "resend")
            }],
        ));
    }
    let wal = write_wal(dir.path(), reqs, "ceiling");

    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let data = run(
        sources,
        canon_query(pred(vec![attr_eq(AttributeOwner::Span, "m", "yes")])).limit(1),
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
        canon_query(pred(vec![attr_eq(AttributeOwner::Span, "m", "yes")])).limit(1),
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
        reqs.push(req_with(
            svc.clone(),
            None,
            &[SpanSpec {
                attrs: vec![kv_str("m", "yes")],
                ..span_in(i as u8, 0x10, 0, (100 + i) * NS, "canon")
            }],
        ));
    }
    for i in 1..=100u64 {
        reqs.push(req_with(
            svc.clone(),
            None,
            &[SpanSpec {
                attrs: vec![kv_str("m", "yes"), kv_str("resend", "1")],
                ..span_in(i as u8, 0x10, 0, (10_000 + i) * NS, "resend")
            }],
        ));
    }
    let wal = write_wal(dir.path(), reqs, "visited");

    let q = || {
        canon_query(pred(vec![attr_eq(AttributeOwner::Span, "m", "yes")]))
            .limit(1)
            .visited_rows_ceiling_for_tests(30)
    };
    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let data = run(sources, q());
    assert!(data.status.has(PartialReason::WorkCeiling));
    assert_eq!(data.traces.len(), 1, "gathered, ranked, trimmed");
    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let again = run(sources, q());
    assert_eq!(
        norm(&data),
        norm(&again),
        "ceiling termination is deterministic"
    );
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
    let reqs = vec![req_with(
        svc,
        None,
        &[
            span_in(7, 0x71, 0, 500 * NS, "older-unrelated"),
            SpanSpec {
                attrs: vec![kv_str("m", "yes")],
                ..span_in(9, 0x91, 0, 1_000 * NS, "the-only-match")
            },
            span_in(8, 0x81, 0, 2_000 * NS, "unrelated"),
        ],
    )];
    let wal = write_wal(dir.path(), reqs, "drain");
    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    // Ceiling 0: the clamp allows exactly one emission, which IS the
    // only match — complete, not WorkCeiling.
    let data = run(
        sources,
        SearchQuery::new(pred(vec![attr_eq(AttributeOwner::Span, "m", "yes")]))
            .visited_rows_ceiling_for_tests(0),
    );
    assert_eq!(ids(&data), vec![hex(9)]);
    assert_eq!(data.status, QueryStatus::Complete);
}

/// The span-cap honesty pair: a capped candidate whose RETAINED spans
/// fail the predicate is not a proven non-match (the matching span may
/// live beyond the cap) — dropping it makes the query Partial{SizeCap},
/// never a silent Complete; a capped candidate whose retained spans DO
/// match returns inexact with SizeCap.
#[test]
fn capped_candidates_are_never_silently_dropped() {
    let dir = tempfile::tempdir().unwrap();
    let svc = vec![kv_str("service.name", "svc")];
    let reqs = vec![req_with(
        svc,
        None,
        &[
            // Trace 9: three spans; with cap 2 the retained set is the
            // earliest two (A, B). `m` exists only beyond the cap (C);
            // `a` only on the retained A; `m2` on ALL THREE — so for `m2`
            // the raw rank is 600s (C) while the computed retained rank is
            // only 200s (B).
            SpanSpec {
                attrs: vec![kv_str("a", "yes"), kv_str("m2", "yes")],
                ..span_in(9, 0x91, 0, 100 * NS, "a-span")
            },
            SpanSpec {
                attrs: vec![kv_str("m2", "yes")],
                ..span_in(9, 0x92, 0x91, 200 * NS, "b-span")
            },
            SpanSpec {
                attrs: vec![kv_str("m", "yes"), kv_str("m2", "yes")],
                ..span_in(9, 0x93, 0x91, 600 * NS, "c-span")
            },
            // Trace 8: an independent `m` match proving results still flow.
            SpanSpec {
                attrs: vec![kv_str("m", "yes")],
                ..span_in(8, 0x81, 0, 400 * NS, "other")
            },
            // Trace 7: a single-span match NEWER than trace 9's RETAINED
            // rank but older than its true one — the trimming displacer.
            SpanSpec {
                attrs: vec![kv_str("a", "yes"), kv_str("m2", "yes")],
                ..span_in(7, 0x71, 0, 500 * NS, "displacer")
            },
        ],
    )];
    let wal = write_wal(dir.path(), reqs, "capped");

    // Direction 1: the match is BEYOND the cap → trace 9 drops, but the
    // query is Partial{SizeCap} — its absence is not proven.
    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let dropped = run(
        sources,
        canon_query(pred(vec![attr_eq(AttributeOwner::Span, "m", "yes")])).span_cap_for_tests(2),
    );
    assert_eq!(ids(&dropped), vec![hex(8)]);
    assert!(dropped.status.has(PartialReason::SizeCap));

    // Direction 2: a retained span matches → trace 9 returns, capped:
    // inexact, SizeCap, counts over the retained set only.
    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let kept = run(
        sources,
        canon_query(pred(vec![attr_eq(AttributeOwner::Span, "a", "yes")])).span_cap_for_tests(2),
    );
    assert_eq!(ids(&kept), vec![hex(7), hex(9)]);
    assert!(kept.status.has(PartialReason::SizeCap));
    let t9 = &kept.traces[1];
    assert!(!t9.exact);
    assert_eq!(t9.span_count, 2);

    // Direction 3 (the trimmed-out shape): for `m2`, trace 9's computed
    // rank comes from its RETAINED spans (200s) though its true newest
    // match is beyond the cap (600s). With limit 1 it is trimmed behind
    // trace 7 (500s) — the wrong top-1 — so the query MUST say SizeCap:
    // a trimmed truncated candidate's real rank is unproven.
    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let trimmed = run(
        sources,
        canon_query(pred(vec![attr_eq(AttributeOwner::Span, "m2", "yes")]))
            .span_cap_for_tests(2)
            .limit(1),
    );
    assert_eq!(ids(&trimmed), vec![hex(7)]);
    assert!(trimmed.status.has(PartialReason::SizeCap));
    // …and without the cap the true top-1 is trace 9 (600s), Complete.
    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let true_top = run(
        sources,
        canon_query(pred(vec![attr_eq(AttributeOwner::Span, "m2", "yes")])).limit(1),
    );
    assert_eq!(ids(&true_top), vec![hex(9)]);
    assert_eq!(true_top.status, QueryStatus::Complete);

    // Sanity: without the cap the `m` query is Complete and exact.
    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let uncapped = run(
        sources,
        canon_query(pred(vec![attr_eq(AttributeOwner::Span, "m", "yes")])),
    );
    assert_eq!(ids(&uncapped), vec![hex(9), hex(8)]);
    assert_eq!(uncapped.status, QueryStatus::Complete);
}

/// A budget-truncated setup source whose SUMMARY range cannot outrank
/// the Nth final is no threat: the result stays Complete (the
/// truncation threat is a rank bound, not a bare flag). The budget is
/// sized so the newer file compiles but the older file's high-card
/// stream-batch scan runs out mid-compilation.
#[test]
fn budget_truncated_old_source_cannot_falsify_completeness() {
    let dir = tempfile::tempdir().unwrap();
    let svc = vec![kv_str("service.name", "svc")];
    // 120 distinct values → the `hc` field is high-card (production
    // threshold 100), so compiling `hc =~ .*` scans stream batches.
    let old: Vec<ExportTraceServiceRequest> = (0..120u64)
        .map(|i| {
            req_with(
                svc.clone(),
                None,
                &[SpanSpec {
                    attrs: vec![kv_str("m", "yes"), kv_str("hc", &format!("v{i:03}"))],
                    ..span_in((i + 1) as u8, 0x10, 0, (100 + i) * NS, "old")
                }],
            )
        })
        .collect();
    let new: Vec<ExportTraceServiceRequest> = (0..120u64)
        .map(|i| {
            req_with(
                svc.clone(),
                None,
                &[SpanSpec {
                    attrs: vec![kv_str("m", "yes"), kv_str("hc", &format!("w{i:03}"))],
                    ..span_in((i + 130) as u8, 0x10, 0, (1_000 + i) * NS, "new")
                }],
            )
        })
        .collect();
    let wal_old = write_wal(dir.path(), old, "old");
    let wal_new = write_wal(dir.path(), new, "new");
    // SourceId order decides processing order: "a-new" compiles first
    // (120 rows), "b-old" starts within budget and truncates mid-scan.
    let sources = || {
        both_roles(|| {
            vec![
                sealed_source(dir.path(), &wal_new, "a-new"),
                sealed_source(dir.path(), &wal_old, "b-old"),
            ]
        })
    };
    let q = || {
        SearchQuery::new(pred(vec![
            attr_eq(AttributeOwner::Span, "m", "yes"),
            cond(
                PredicateTarget::Attribute(AttributeOwner::Span, "hc".into()),
                CompareOp::Regex,
                vec![text(".*")],
            ),
        ]))
        .limit(1)
        .visited_rows_ceiling_for_tests(130)
    };
    let data = run(sources(), q());
    // The top-1 comes from the new file (1119s); the truncated old
    // file's bound (~220s) cannot outrank it — provably Complete.
    assert_eq!(ids(&data), vec![hex(249)]);
    assert_eq!(data.status, QueryStatus::Complete);
    // Sanity: the unbudgeted answer is the same top-1.
    let unbudgeted = run(
        sources(),
        SearchQuery::new(pred(vec![attr_eq(AttributeOwner::Span, "m", "yes")])).limit(1),
    );
    assert_eq!(ids(&unbudgeted), vec![hex(249)]);
}

/// `trace:id =` pins bypass phase-1 discovery entirely: the search
/// answers with a ZERO visited-rows budget (assembly is not a
/// discovery cost), proving no scan ran.
#[test]
fn trace_id_pins_skip_discovery() {
    let dir = tempfile::tempdir().unwrap();
    let mut all = early_requests();
    all.extend(late_requests());
    let wal = write_wal(dir.path(), all, "world");
    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let data = run(
        sources,
        SearchQuery::new(pred(vec![builtin(
            BuiltinField::TraceId,
            CompareOp::Eq,
            vec![text(&hex(5))],
        )]))
        .visited_rows_ceiling_for_tests(0),
    );
    assert_eq!(ids(&data), vec![hex(5)]);
    assert_eq!(data.status, QueryStatus::Complete);
    // Two contradictory pins intersect to nothing: Complete empty.
    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let none = run(
        sources,
        SearchQuery::new(pred(vec![
            builtin(BuiltinField::TraceId, CompareOp::Eq, vec![text(&hex(5))]),
            builtin(BuiltinField::TraceId, CompareOp::Eq, vec![text(&hex(4))]),
        ])),
    );
    assert!(none.traces.is_empty());
    assert_eq!(none.status, QueryStatus::Complete);
}

/// Decision 15 tri-state: a CAPPED trace's root/envelope values are
/// unreliable, so a trace-level condition EXCLUDES it as indeterminate
/// (never guessed either way) and the query stays Partial via the
/// examined-truncated SizeCap.
#[test]
fn trace_level_conditions_exclude_indeterminate_candidates() {
    let dir = tempfile::tempdir().unwrap();
    let svc = vec![kv_str("service.name", "svc")];
    let reqs = vec![req_with(
        svc,
        None,
        &[
            span_in(9, 0x91, 0, 100 * NS, "root-9"),
            span_in(9, 0x92, 0x91, 200 * NS, "child"),
            span_in(9, 0x93, 0x91, 300 * NS, "child-2"),
            span_in(8, 0x81, 0, 400 * NS, "root-8"),
        ],
    )];
    let wal = write_wal(dir.path(), reqs, "tristate");
    // Uncapped: both roots match the regex.
    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let full = run(
        sources,
        canon_query(pred(vec![builtin(
            BuiltinField::RootName,
            CompareOp::Regex,
            vec![text("root-.*")],
        )])),
    );
    assert_eq!(ids(&full), vec![hex(8), hex(9)]);
    assert_eq!(full.status, QueryStatus::Complete);
    // Capped at 2: trace 9 assembles truncated → INDETERMINATE for the
    // trace-level condition → excluded; trace 8 (complete) returns;
    // SizeCap marks the exclusion's cause.
    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let capped = run(
        sources,
        canon_query(pred(vec![builtin(
            BuiltinField::RootName,
            CompareOp::Regex,
            vec![text("root-.*")],
        )]))
        .span_cap_for_tests(2),
    );
    assert_eq!(ids(&capped), vec![hex(8)]);
    assert!(capped.status.has(PartialReason::SizeCap));
}

/// The subgroup refine is budget-bounded: a ceiling landing mid-refine
/// truncates discovery deterministically (Partial{WorkCeiling}), never
/// a silent partial refine.
#[test]
fn refine_budget_abort_is_honest() {
    let dir = tempfile::tempdir().unwrap();
    let svc = vec![kv_str("service.name", "svc")];
    // 20 spans, two events each — the refine visits rows AND records.
    let reqs: Vec<ExportTraceServiceRequest> = (0..20u64)
        .map(|i| {
            req_with(
                svc.clone(),
                None,
                &[SpanSpec {
                    events: vec![
                        ("boot", vec![kv_str("seq", "1")]),
                        ("work", vec![kv_str("seq", "2")]),
                    ],
                    ..span_in((i + 1) as u8, 0x10, 0, (100 + i) * NS, "spanned")
                }],
            )
        })
        .collect();
    let wal = write_wal(dir.path(), reqs, "refine-budget");
    let q = |ceiling: u64| {
        SearchQuery::new(pred(vec![attr_eq(AttributeOwner::Event, "seq", "2")]))
            .visited_rows_ceiling_for_tests(ceiling)
    };
    // Unbudgeted: all 20 match through the refine.
    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let full = run(sources, q(u64::MAX).limit(30));
    assert_eq!(full.traces.len(), 20);
    assert_eq!(full.status, QueryStatus::Complete);
    // A ceiling mid-refine: the file is budget-truncated at compile, so
    // discovery finds nothing and the standing threat is WorkCeiling.
    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let cut = run(sources, q(10).limit(30));
    assert!(cut.status.has(PartialReason::WorkCeiling));
    assert!(cut.traces.is_empty());
    // Determinism of the truncated shape.
    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let again = run(sources, q(10).limit(30));
    assert_eq!(norm(&cut), norm(&again));
}

/// UNSET trace ids never become candidates (TRCE carries zeros for
/// them; a by-UNSET group is not a trace).
#[test]
fn unset_trace_ids_are_never_candidates() {
    let dir = tempfile::tempdir().unwrap();
    let svc = vec![kv_str("service.name", "svc")];
    let reqs = vec![req_with(
        svc,
        None,
        &[
            span_in(7, 0x71, 0, 1_000 * NS, "real"),
            SpanSpec {
                trace: [0u8; 16],
                ..sp(0x72, 0, 2_000 * NS, "orphan")
            },
        ],
    )];
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

/// Search across ONLY tails (no sealed file at all): the tail path
/// stands alone.
#[test]
fn tail_only_search_matches_the_sealed_answer() {
    let dir = tempfile::tempdir().unwrap();
    let wal_early = write_wal(dir.path(), early_requests(), "early");
    let wal_late = write_wal(dir.path(), late_requests(), "late");
    let q = || {
        SearchQuery::new(pred(vec![attr_eq(
            AttributeOwner::Resource,
            "service.name",
            "svc-b",
        )]))
    };

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

/// An unavailable completion source reports its own reason beside a
/// missing file's; the reachable data still answers but no summary is
/// exact (the missing bytes may hold spans of any candidate). Alone, it
/// is never Complete and never a source failure.
#[test]
fn unavailable_sources_are_reported_and_degrade_every_summary() {
    let dir = tempfile::tempdir().unwrap();
    let wal_early = write_wal(dir.path(), early_requests(), "early");
    let progress = Arc::new(AtomicUsize::new(0));
    let data = search(
        both_roles(|| {
            vec![
                sealed_source(dir.path(), &wal_early, "early"),
                missing_source(dir.path(), "missing", 1_000, 1_100),
                unavailable_source("remote", 1_000, 1_100),
            ]
        }),
        SearchQuery::new(Predicate::all()),
        CancellationToken::new(),
        Arc::clone(&progress),
    )
    .unwrap();
    assert_eq!(ids(&data), vec![hex(6), hex(4), hex(2), hex(1), hex(3)]);
    assert!(data.traces.iter().all(|t| !t.exact));
    assert_eq!(
        data.status,
        QueryStatus::Partial(BTreeSet::from([
            PartialReason::SourceFailure,
            PartialReason::RemoteUnavailable,
        ]))
    );
    assert_eq!(
        progress.load(Ordering::Relaxed),
        3,
        "one tick per completion source"
    );

    let only = run(
        both_roles(|| vec![unavailable_source("remote", 1_000, 1_100)]),
        SearchQuery::new(Predicate::all()),
    );
    assert!(only.traces.is_empty());
    assert_eq!(
        only.status,
        QueryStatus::Partial(BTreeSet::from([PartialReason::RemoteUnavailable]))
    );
}

/// An unavailable source outside the match window but inside the
/// completion slack still matters: it may hold out-of-window spans of a
/// matched trace, so the reason is reported and no summary is exact.
#[test]
fn an_unavailable_source_in_the_completion_slack_makes_every_summary_inexact() {
    let dir = tempfile::tempdir().unwrap();
    let wal_early = write_wal(dir.path(), early_requests(), "early");
    let data = run(
        SearchSources {
            window: vec![sealed_source(dir.path(), &wal_early, "early")],
            completion: vec![
                sealed_source(dir.path(), &wal_early, "early"),
                unavailable_source("remote-slack", 5_000, 6_000),
            ],
        },
        SearchQuery::new(Predicate::all())
            .window(TimeWindow::new((800 * NS) as i64, (1_400 * NS) as i64).unwrap()),
    );
    assert!(!data.traces.is_empty());
    assert!(data.traces.iter().all(|t| !t.exact));
    assert_eq!(
        data.status,
        QueryStatus::Partial(BTreeSet::from([PartialReason::RemoteUnavailable]))
    );
}

/// Trace-level conditions need an exact assembly; with an unavailable
/// source in the snapshot every candidate is indeterminate and excluded
/// (never guessed), and the reason says why the list is empty.
#[test]
fn an_unavailable_source_makes_trace_level_candidates_indeterminate() {
    let dir = tempfile::tempdir().unwrap();
    let svc = vec![kv_str("service.name", "svc")];
    let reqs = vec![req_with(
        svc,
        None,
        &[
            span_in(9, 0x91, 0, 100 * NS, "root-9"),
            span_in(8, 0x81, 0, 400 * NS, "root-8"),
        ],
    )];
    let wal = write_wal(dir.path(), reqs, "tristate");
    let root_name = || {
        SearchQuery::new(pred(vec![builtin(
            BuiltinField::RootName,
            CompareOp::Regex,
            vec![text("root-.*")],
        )]))
    };
    let whole = run(
        both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]),
        root_name(),
    );
    assert_eq!(ids(&whole), vec![hex(8), hex(9)]);

    let degraded = run(
        both_roles(|| {
            vec![
                sealed_source(dir.path(), &wal, "one"),
                unavailable_source("remote", 0, 1_000),
            ]
        }),
        root_name(),
    );
    assert!(degraded.traces.is_empty());
    assert_eq!(
        degraded.status,
        QueryStatus::Partial(BTreeSet::from([PartialReason::RemoteUnavailable]))
    );
}

/// Files open lazily: a corrupt dictionary in a slack-only file that
/// holds none of the candidates is never read, so the page is complete.
#[test]
fn an_unread_corrupt_slack_file_does_not_degrade_the_page() {
    let dir = tempfile::tempdir().unwrap();
    let svc = || vec![kv_str("service.name", "svc")];
    let mut a = sp(1, 0, 10 * NS, "a");
    a.trace = tid(1);
    let mut b = sp(2, 0, 20 * NS, "b");
    b.trace = tid(2);
    let w_window = write_wal(dir.path(), vec![req_with(svc(), None, &[a])], "window");
    let w_slack = write_wal(dir.path(), vec![req_with(svc(), None, &[b])], "slack");
    drop(sealed_source(dir.path(), &w_slack, "slack"));
    corrupt_chunk(&dir.path().join("slack.sfst"), *b"PRIM");
    let window = || sealed_source(dir.path(), &w_window, "window");
    let data = run(
        SearchSources {
            window: vec![window()],
            completion: vec![
                window(),
                sealed_source_at(&dir.path().join("slack.sfst"), "slack"),
            ],
        },
        SearchQuery::new(Predicate::default()),
    );
    assert_eq!(data.status, QueryStatus::Complete);
    assert_eq!(data.traces.len(), 1);
}

/// Window files compile newest first, only while they can still beat
/// the N-th result: an older file is never compiled (here its corrupt
/// dictionary stays unread and the page is complete).
#[test]
fn a_window_file_that_cannot_beat_the_page_is_never_compiled() {
    let dir = tempfile::tempdir().unwrap();
    let svc = || vec![kv_str("service.name", "svc")];
    let traced = |t: u8, at: u64| {
        let mut s = sp(t, 0, at * NS, "op");
        s.trace = tid(t);
        s
    };
    let w_new = write_wal(
        dir.path(),
        vec![req_with(svc(), None, &[traced(1, 30), traced(2, 31)])],
        "new",
    );
    let w_old = write_wal(
        dir.path(),
        vec![req_with(svc(), None, &[traced(3, 10)])],
        "old",
    );
    drop(sealed_source(dir.path(), &w_old, "old"));
    corrupt_chunk(&dir.path().join("old.sfst"), *b"PRIM");
    let sources = || {
        vec![
            sealed_source(dir.path(), &w_new, "new"),
            sealed_source_at(&dir.path().join("old.sfst"), "old"),
        ]
    };
    let data = run(
        SearchSources {
            window: sources(),
            completion: sources(),
        },
        SearchQuery::new(Predicate::default()).limit(2),
    );
    assert_eq!(data.status, QueryStatus::Complete);
    assert_eq!(data.traces.len(), 2);

    // A page the old file could still change compiles it, and its
    // corruption surfaces.
    let data = run(
        SearchSources {
            window: sources(),
            completion: sources(),
        },
        SearchQuery::new(Predicate::default()).limit(3),
    );
    assert!(data.status.has(PartialReason::SourceFailure));
}

/// A value longer than any defensive cut is evaluated whole, sealed or
/// tail alike: a condition that depends on its last bytes matches, and
/// the attached span carries the full value.
#[test]
fn long_values_are_evaluated_whole_in_every_layout() {
    let dir = tempfile::tempdir().unwrap();
    let big = format!("{}-tail", "x".repeat(70_000));
    let span = SpanSpec {
        attrs: vec![kv_str("big", &big)],
        ..span_in(1, 1, 0, 100 * NS, "op")
    };
    let wal = write_wal(
        dir.path(),
        vec![req_with(vec![kv_str("service.name", "svc")], None, &[span])],
        "long",
    );
    let big_target = || PredicateTarget::Attribute(AttributeOwner::Span, "big".into());
    let queries = [
        (
            "equal",
            cond(big_target(), CompareOp::Eq, vec![text(&big)]),
            vec![hex(1)],
        ),
        (
            "regex past the cut",
            cond(big_target(), CompareOp::Regex, vec![text("x+-tail")]),
            vec![hex(1)],
        ),
        (
            "negated",
            cond(big_target(), CompareOp::NotEq, vec![text(&big)]),
            vec![],
        ),
    ];
    for (layout, sources) in [
        (
            "sealed",
            both_roles(|| vec![sealed_source(dir.path(), &wal, "sealed")]),
        ),
        ("tail", both_roles(|| vec![tail_source(&wal, "tail")])),
    ] {
        let mut sources = Some(sources);
        for (name, condition, want) in &queries {
            let s = sources.take().unwrap_or_else(|| match layout {
                "sealed" => both_roles(|| vec![sealed_source(dir.path(), &wal, "sealed")]),
                _ => both_roles(|| vec![tail_source(&wal, "tail")]),
            });
            let data = run(s, SearchQuery::new(pred(vec![condition.clone()])));
            assert_eq!(&ids(&data), want, "{name} on {layout}");
            assert_eq!(data.status, QueryStatus::Complete, "{name} on {layout}");
        }
        // The trace view returns the value whole too.
        let sources = match layout {
            "sealed" => vec![sealed_source(dir.path(), &wal, "sealed")],
            _ => vec![tail_source(&wal, "tail")],
        };
        let trace = sfsq::traces::trace_by_id(
            sources,
            sfsq::traces::TraceQuery::new(sfst::TraceId::from(tid(1))),
            CancellationToken::new(),
            Arc::new(AtomicUsize::new(0)),
        )
        .expect("valid request");
        let value = trace.trace.spans[0]
            .fields
            .iter()
            .find(|(k, _)| k == "attributes.big")
            .map(|(_, v)| v.len());
        assert_eq!(value, Some(big.len()), "{layout}: value cut");
    }
}

/// A pending window file that may hold a newer candidate than the pool's
/// best compiles before the pool is examined, as eager compile would have
/// ranked it: the tail's older trace (over the span cap) is never
/// assembled, so the page is complete instead of `size_cap`.
#[test]
fn a_newer_pending_file_compiles_before_older_pool_candidates() {
    let dir = tempfile::tempdir().unwrap();
    let svc = || vec![kv_str("service.name", "svc")];
    let older = write_wal(
        dir.path(),
        vec![req_with(
            svc(),
            None,
            &[
                span_in(1, 1, 0, 100 * NS, "a"),
                span_in(1, 2, 1, 101 * NS, "a"),
            ],
        )],
        "older-tail",
    );
    let newer = write_wal(
        dir.path(),
        vec![req_with(svc(), None, &[span_in(2, 1, 0, 150 * NS, "b")])],
        "newer",
    );
    let sources = || {
        both_roles(|| {
            vec![
                tail_source(&older, "older-tail"),
                sealed_source(dir.path(), &newer, "newer"),
            ]
        })
    };
    let data = run(
        sources(),
        SearchQuery::new(Predicate::all())
            .limit(1)
            .span_cap_for_tests(1),
    );
    assert_eq!(ids(&data), vec![hex(2)]);
    assert_eq!(data.status, QueryStatus::Complete);
}

// ── Free text: index and span paths agree ──────────────────────────

/// One span (and trace) of the free-text corpus, with the values free
/// text should search, for the oracle.
struct TextSpan {
    trace: [u8; 16],
    spec: SpanSpec,
    /// Every value in the free-text scope: name, status message, span,
    /// resource and scope attribute values, event names and attribute
    /// values, link attribute values.
    scope_values: Vec<String>,
}

const TEXT_TRACES: usize = 2400;
const TEXT_BATCH: usize = 100;

/// 2400 one-span traces in requests of 100, sized so each half (1200
/// spans) seals `name` and `rq` into the high tier, `route` into the
/// mid tier and the rest into the low tier. Rare values: a low-card
/// flag, status messages, an event (name + attribute), a link
/// attribute, a resource attribute on batch 3 and a scope attribute on
/// batch 5. Status ERROR without a message on others: free text never
/// searches `status_code`.
fn text_corpus() -> (
    Vec<ExportTraceServiceRequest>,
    Vec<ExportTraceServiceRequest>,
    Vec<TextSpan>,
) {
    let mut spans = Vec::new();
    let mut early = Vec::new();
    let mut late = Vec::new();
    for batch in 0..TEXT_TRACES / TEXT_BATCH {
        let mut resource = vec![kv_str("service.name", &format!("svc-{}", batch % 4))];
        if batch == 3 {
            resource.push(kv_str("deployment", "blue-CANARY"));
        }
        let scope_attrs = vec![kv_str("feature", "telescope-mode")];
        let scope = (batch == 5).then(|| ("lib", "1.0", scope_attrs.clone()));
        let mut specs = Vec::new();
        for i in batch * TEXT_BATCH..(batch + 1) * TEXT_BATCH {
            let mut trace = [0u8; 16];
            trace[0] = 1;
            trace[14..].copy_from_slice(&(i as u16).to_be_bytes());
            let name: &'static str = Box::leak(format!("op-{i:04}").into_boxed_str());
            let mut spec = SpanSpec {
                trace,
                ..sp(1, 0, (1_000 + i as u64) * NS, name)
            };
            spec.attrs = vec![
                kv_str("rq", &format!("rq{i:04}x")),
                kv_str("route", &format!("route-{:03}", i % 150)),
                common::kv_int("count", (i % 7) as i64),
            ];
            if i % 500 == 5 {
                spec.attrs.push(kv_str("flag", "Golden-Ticket"));
            }
            if i % 400 == 11 {
                spec.status = Some((2, "Upstream TIMEOUT at gateway"));
            } else if i % 401 == 13 {
                spec.status = Some((2, ""));
            }
            if i % 300 == 7 {
                spec.events = vec![(
                    "checkout.failed",
                    vec![kv_str("error.detail", "Card DECLINED by issuer")],
                )];
            }
            if i % 350 == 9 {
                spec.links = vec![(tid(9), [0x99; 8], vec![kv_str("rel", "follows-from-batch")])];
            }
            let mut scope_values = vec![
                name.to_string(),
                format!("rq{i:04}x"),
                format!("route-{:03}", i % 150),
                (i % 7).to_string(),
                format!("svc-{}", batch % 4),
            ];
            if i % 500 == 5 {
                scope_values.push("Golden-Ticket".into());
            }
            if let Some((_, message)) = spec.status {
                scope_values.push(message.to_string());
            }
            if batch == 3 {
                scope_values.push("blue-CANARY".into());
            }
            if batch == 5 {
                scope_values.push("telescope-mode".into());
            }
            if i % 300 == 7 {
                scope_values.push("checkout.failed".into());
                scope_values.push("Card DECLINED by issuer".into());
            }
            if i % 350 == 9 {
                scope_values.push("follows-from-batch".into());
            }
            specs.push(spec.clone());
            spans.push(TextSpan {
                trace,
                spec,
                scope_values,
            });
        }
        let request = req_with(resource, scope, &specs);
        if batch < TEXT_TRACES / TEXT_BATCH / 2 {
            early.push(request);
        } else {
            late.push(request);
        }
    }
    (early, late, spans)
}

/// Free text finds the same traces whether a span is read through the
/// index (sealed files: low, mid and high tiers, with or without event
/// and link structures) or decoded (WAL tails): span names, status
/// messages, span/resource/scope attribute values, event names and
/// attribute values, and link attribute values, case-insensitively;
/// never field names or `status_code`.
#[test]
fn text_index_span_parity() {
    let dir = tempfile::tempdir().unwrap();
    let (early, late, spans) = text_corpus();
    assert!(spans.iter().all(|s| s.spec.trace == s.trace));
    let mut all = early.clone();
    all.extend(late.clone());
    let wal_all = write_wal(dir.path(), all, "all");
    let wal_early = write_wal(dir.path(), early, "early");
    let wal_late = write_wal(dir.path(), late, "late");
    // The tiers the corpus is sized for, in each sealed half.
    for half in [&wal_early, &wal_late] {
        sealed_source(dir.path(), half, "probe");
        let bytes = std::fs::read(dir.path().join("probe.sfst")).unwrap();
        let meta = sfst::MetaReader::open(&bytes).unwrap();
        let tier = |name: &str| {
            meta.field_table()
                .unwrap()
                .iter()
                .find(|f| f.name == name)
                .map(|f| f.tier)
        };
        assert_eq!(tier("name"), Some(sfst::FieldTier::High));
        assert_eq!(tier("attributes.rq"), Some(sfst::FieldTier::High));
        assert_eq!(tier("attributes.route"), Some(sfst::FieldTier::Mid));
        assert_eq!(tier("attributes.flag"), Some(sfst::FieldTier::Low));
    }
    // A file sealed without its event and link structures (EVNB, LNKB):
    // event names and event/link attributes then reach the spans as row
    // facets under their storage names.
    drop(sealed_source(dir.path(), &wal_all, "flat"));
    let flat = dir.path().join("flat.sfst");
    hide_chunk(&flat, *b"EVNB");
    hide_chunk(&flat, *b"LNKB");
    type Layout<'a> = (&'static str, Box<dyn Fn() -> Vec<TraceSource> + 'a>);
    let layouts: [Layout; 5] = [
        (
            "sealed-without-structures",
            Box::new(|| vec![sealed_source_at(&flat, "flat")]),
        ),
        (
            "sealed",
            Box::new(|| vec![sealed_source(dir.path(), &wal_all, "all")]),
        ),
        (
            "sealed-halves",
            Box::new(|| {
                vec![
                    sealed_source(dir.path(), &wal_early, "early"),
                    sealed_source(dir.path(), &wal_late, "late"),
                ]
            }),
        ),
        ("tail", Box::new(|| vec![tail_source(&wal_all, "all-tail")])),
        (
            "sealed-plus-tail",
            Box::new(|| {
                vec![
                    sealed_source(dir.path(), &wal_early, "early"),
                    tail_source(&wal_late, "late-tail"),
                ]
            }),
        ),
    ];
    let terms = [
        ("RQ0777X", 1),       // high tier, upper case
        ("op-1999", 1),       // span name, high tier
        ("route-042", 16),    // mid tier
        ("golden", 5),        // low tier
        ("timeout", 6),       // status message
        ("checkout.fail", 8), // event name
        ("declined", 8),      // event attribute value
        ("from-batch", 7),    // link attribute value
        ("canary", 100),      // resource attribute value
        ("telescope", 100),   // scope attribute value
        ("error", 0),         // only a field name and `status_code`
        ("rel", 0),           // only a field name
    ];
    for (term, count) in terms {
        let needle = term.to_lowercase();
        let want: BTreeSet<String> = spans
            .iter()
            .filter(|s| {
                s.scope_values
                    .iter()
                    .any(|v| v.to_lowercase().contains(&needle))
            })
            .map(|s| sfst::TraceId::from(s.trace).to_string())
            .collect();
        assert_eq!(want.len(), count, "oracle for {term}");
        for (name, build) in &layouts {
            let query = SearchQuery::new(pred(vec![cond(
                PredicateTarget::FreeText,
                CompareOp::Contains,
                vec![text(term)],
            )]))
            .limit(200);
            let data = run(both_roles(build), query);
            let got: BTreeSet<String> = ids(&data).into_iter().collect();
            assert_eq!(got, want, "{term} on {name}");
            assert_eq!(data.status, QueryStatus::Complete, "{term} on {name}");
        }
    }
}

// ── Tokens: trace-level AND across spans and files, NOT EXISTS ──────

fn pos(conditions: Vec<Condition>) -> Token {
    Token::positive(pred(conditions))
}

fn neg(conditions: Vec<Condition>) -> Token {
    Token::negated(pred(conditions))
}

fn tokens(tokens: Vec<Token>) -> TokenQuery {
    TokenQuery { tokens }
}

fn name_eq(name: &str) -> Condition {
    builtin(BuiltinField::Name, CompareOp::Eq, vec![text(name)])
}

fn span_attr_eq(key: &str, value: &str) -> Condition {
    attr_eq(AttributeOwner::Span, key, value)
}

/// `query` on every layout of the shared world: the expected ids in
/// order, every row exact, Complete, and the same summaries (ranks
/// included) as on the single sealed file. Returns that reference.
fn on_every_layout(
    dir: &Path,
    world: &World,
    name: &str,
    query: &SearchQuery,
    want: &[String],
) -> (Vec<SummaryPrint>, QueryStatus) {
    on_every_layout_with(dir, world, name, query, want, want)
}

/// [`on_every_layout`] on both paths: assembly (canonical spans) answers
/// `want`, the rows path (stored rows, decision 49) `want_rows` — they
/// differ where a losing resend moves a rank (T6's 1950 s copy). Returns
/// the rows path's reference.
fn on_every_layout_with(
    dir: &Path,
    world: &World,
    name: &str,
    query: &SearchQuery,
    want: &[String],
    want_rows: &[String],
) -> (Vec<SummaryPrint>, QueryStatus) {
    layouts_agree(
        dir,
        world,
        &format!("{name} (assembly)"),
        &query.clone().rows_for_tests(false),
        want,
    );
    layouts_agree(dir, world, &format!("{name} (rows)"), query, want_rows)
}

/// Every layout answers `query` with `want`, exactly and completely, and
/// with the same summaries.
fn layouts_agree(
    dir: &Path,
    world: &World,
    name: &str,
    query: &SearchQuery,
    want: &[String],
) -> (Vec<SummaryPrint>, QueryStatus) {
    let mut reference = None;
    for layout_name in LAYOUTS {
        let data = run(layout(dir, world, layout_name), query.clone());
        assert_eq!(ids(&data), want, "{name} on {layout_name}");
        assert!(
            data.traces.iter().all(|t| t.exact),
            "{name} on {layout_name}"
        );
        let got = norm(&data);
        assert_eq!(got.1, QueryStatus::Complete, "{name} on {layout_name}");
        match &reference {
            None => reference = Some(got),
            Some(r) => assert_eq!(&got, r, "{name} on {layout_name}: summaries"),
        }
    }
    reference.expect("layouts")
}

/// Two positive tokens may match different spans of one trace (T1's
/// root and its db child); the same conditions in ONE token need one
/// span holding both. The rank is the older of the two tokens' newest
/// matches.
#[test]
fn cross_span_and() {
    let dir = tempfile::tempdir().unwrap();
    let world = world(dir.path());
    let split = SearchQuery::new(tokens(vec![
        pos(vec![name_eq("GET /users")]),
        pos(vec![span_attr_eq("db.system", "pg")]),
    ]));
    let (rows, _) = on_every_layout(dir.path(), &world, "split", &split, &[hex(1)]);
    assert_eq!(
        rows[0].7,
        1_000 * NS as i64,
        "min of the two newest matches"
    );
    let joined = SearchQuery::new(tokens(vec![pos(vec![
        name_eq("GET /users"),
        span_attr_eq("db.system", "pg"),
    ])]));
    on_every_layout(dir.path(), &world, "joined", &joined, &[]);
}

/// Tokens matched in different files (T3's early root, its late child)
/// still AND: the trace assembles across sources. Its rank is the older
/// match, so a trace whose newest match is recent can rank old.
#[test]
fn cross_file_and() {
    let dir = tempfile::tempdir().unwrap();
    let world = world(dir.path());
    let query = SearchQuery::new(tokens(vec![
        pos(vec![name_eq("GET /health")]),
        pos(vec![name_eq("late-check")]),
    ]));
    let (rows, _) = on_every_layout(dir.path(), &world, "cross-file", &query, &[hex(3)]);
    assert_eq!(rows[0].7, 900 * NS as i64);
}

/// A negated token is NOT EXISTS: no span of the trace matches its
/// positive form — traces without the field pass. An in-token `!=`
/// keeps its one-span meaning: the field present with another value.
#[test]
fn negated_token_is_not_exists() {
    let dir = tempfile::tempdir().unwrap();
    let world = world(dir.path());
    let chip = SearchQuery::new(tokens(vec![neg(vec![span_attr_eq("http.method", "GET")])]));
    on_every_layout_with(
        dir.path(),
        &world,
        "not exists",
        &chip,
        &[hex(5), hex(6), hex(4), hex(2)],
        &[hex(6), hex(5), hex(4), hex(2)],
    );
    let in_token = SearchQuery::new(tokens(vec![pos(vec![cond(
        PredicateTarget::Attribute(AttributeOwner::Span, "http.method".into()),
        CompareOp::NotEq,
        vec![text("GET")],
    )])]));
    on_every_layout(dir.path(), &world, "in-token !=", &in_token, &[hex(2)]);
}

/// A negated token on a field no span carries excludes nothing.
#[test]
fn negated_absent_field_passes() {
    let dir = tempfile::tempdir().unwrap();
    let world = world(dir.path());
    let query = SearchQuery::new(tokens(vec![neg(vec![span_attr_eq("nowhere", "x")])]));
    on_every_layout_with(
        dir.path(),
        &world,
        "absent",
        &query,
        &[hex(3), hex(5), hex(6), hex(4), hex(2), hex(1)],
        &[hex(6), hex(3), hex(5), hex(4), hex(2), hex(1)],
    );
}

/// A negated event token excludes the traces holding a matching event
/// (an in-token negated event condition is not evaluable; the token
/// form is).
#[test]
fn negated_event_token() {
    let dir = tempfile::tempdir().unwrap();
    let world = world(dir.path());
    let exception = || {
        builtin(
            BuiltinField::EventName,
            CompareOp::Eq,
            vec![text("exception")],
        )
    };
    let query = SearchQuery::new(tokens(vec![neg(vec![exception()])]));
    on_every_layout_with(
        dir.path(),
        &world,
        "no exception",
        &query,
        &[hex(3), hex(5), hex(6), hex(4), hex(1)],
        &[hex(6), hex(3), hex(5), hex(4), hex(1)],
    );
    let query = SearchQuery::new(tokens(vec![pos(vec![exception()])]));
    on_every_layout(dir.path(), &world, "exception", &query, &[hex(2)]);
}

/// Negated trace-level and `trace_id` tokens: NOT on the trace's value.
/// A positive root condition never holds without a true root, so a
/// trace whose root was never exported passes the negated token.
#[test]
fn negated_trace_level_and_id_tokens() {
    let dir = tempfile::tempdir().unwrap();
    let world = world(dir.path());
    let not_b = SearchQuery::new(tokens(vec![neg(vec![builtin(
        BuiltinField::RootServiceName,
        CompareOp::Eq,
        vec![text("svc-b")],
    )])]));
    on_every_layout(
        dir.path(),
        &world,
        "root not svc-b",
        &not_b,
        &[hex(3), hex(2), hex(1)],
    );
    let id = |n: u8| sfst::TraceId::from(tid(n)).to_string();
    let not_t1 = SearchQuery::new(tokens(vec![neg(vec![builtin(
        BuiltinField::TraceId,
        CompareOp::Eq,
        vec![text(&id(1)), text(&id(2))],
    )])]));
    on_every_layout_with(
        dir.path(),
        &world,
        "not T1 or T2",
        &not_t1,
        &[hex(3), hex(5), hex(6), hex(4)],
        &[hex(6), hex(3), hex(5), hex(4)],
    );
    // NOT (id ∉ {T4}) pins T4.
    let only_t4 = SearchQuery::new(tokens(vec![neg(vec![builtin(
        BuiltinField::TraceId,
        CompareOp::NotEq,
        vec![text(&id(4))],
    )])]));
    on_every_layout(dir.path(), &world, "pin T4", &only_t4, &[hex(4)]);

    // A rootless trace (child spans only) passes a negated root token
    // and fails the positive one.
    let rootless = write_wal(
        dir.path(),
        vec![req_with(
            vec![kv_str("service.name", "svc-r")],
            None,
            &[span_in(7, 0x72, 0x71, 1_000 * NS, "orphan")],
        )],
        "rootless",
    );
    let sources = || both_roles(|| vec![sealed_source(dir.path(), &rootless, "rootless")]);
    let root = || {
        vec![builtin(
            BuiltinField::RootServiceName,
            CompareOp::Eq,
            vec![text("svc-r")],
        )]
    };
    let data = run(sources(), SearchQuery::new(tokens(vec![neg(root())])));
    assert_eq!(ids(&data), vec![hex(7)]);
    let data = run(sources(), SearchQuery::new(tokens(vec![pos(root())])));
    assert!(ids(&data).is_empty());
}

/// Whichever positive token drives discovery, the answer is the same.
#[test]
fn forced_driver_invariance() {
    let dir = tempfile::tempdir().unwrap();
    let world = world(dir.path());
    let queries = [
        (
            tokens(vec![
                pos(vec![name_eq("GET /users")]),
                pos(vec![span_attr_eq("db.system", "pg")]),
            ]),
            vec![hex(1)],
        ),
        (
            tokens(vec![
                pos(vec![name_eq("GET /health")]),
                pos(vec![name_eq("late-check")]),
            ]),
            vec![hex(3)],
        ),
        (
            tokens(vec![
                pos(vec![attr_eq(
                    AttributeOwner::Resource,
                    "service.name",
                    "svc-b",
                )]),
                pos(vec![builtin(
                    BuiltinField::Status,
                    CompareOp::Eq,
                    vec![text("ERROR")],
                )]),
                neg(vec![name_eq("fast-step")]),
            ]),
            vec![],
        ),
        (
            tokens(vec![
                pos(vec![cond(
                    PredicateTarget::Attribute(AttributeOwner::Resource, "service.name".into()),
                    CompareOp::Regex,
                    vec![text("svc-.*")],
                )]),
                pos(vec![builtin(
                    BuiltinField::Status,
                    CompareOp::Regex,
                    vec![text("OK|UNSET")],
                )]),
            ]),
            vec![hex(3), hex(6), hex(4), hex(1)],
        ),
    ];
    for (i, (query, want)) in queries.iter().enumerate() {
        // T6's stored 1950 s resend ranks it first on the rows path.
        let want_rows = if i == 3 {
            vec![hex(6), hex(3), hex(4), hex(1)]
        } else {
            want.clone()
        };
        let auto = on_every_layout_with(
            dir.path(),
            &world,
            &format!("query {i}"),
            &SearchQuery::new(query.clone()),
            want,
            &want_rows,
        );
        for driver in 0..2 {
            let forced = on_every_layout_with(
                dir.path(),
                &world,
                &format!("query {i}, driver {driver}"),
                &SearchQuery::new(query.clone()).driver_for_tests(driver),
                want,
                &want_rows,
            );
            assert_eq!(forced, auto, "query {i}, driver {driver}");
        }
    }
}

/// A flat predicate and the tokens it means give the same answer:
/// span-local conditions as one token, trace-level and `trace_id`
/// conditions as their own.
#[test]
fn single_token_equals_predicate() {
    let dir = tempfile::tempdir().unwrap();
    let world = world(dir.path());
    let service_a = || attr_eq(AttributeOwner::Resource, "service.name", "svc-a");
    let get = || span_attr_eq("http.method", "GET");
    let root_a = || {
        builtin(
            BuiltinField::RootServiceName,
            CompareOp::Eq,
            vec![text("svc-a")],
        )
    };
    let not_t1 = || {
        builtin(
            BuiltinField::TraceId,
            CompareOp::NotEq,
            vec![text(&sfst::TraceId::from(tid(1)).to_string())],
        )
    };
    let cases = [
        (
            pred(vec![service_a(), get()]),
            vec![pos(vec![service_a(), get()])],
        ),
        (
            pred(vec![get(), root_a(), not_t1()]),
            vec![pos(vec![get()]), pos(vec![root_a()]), pos(vec![not_t1()])],
        ),
        (pred(vec![root_a()]), vec![pos(vec![root_a()])]),
    ];
    for (i, (flat, explicit)) in cases.into_iter().enumerate() {
        for layout_name in LAYOUTS {
            let a = run(
                layout(dir.path(), &world, layout_name),
                SearchQuery::new(flat.clone()),
            );
            let b = run(
                layout(dir.path(), &world, layout_name),
                SearchQuery::new(tokens(explicit.clone())),
            );
            assert_eq!(norm(&a), norm(&b), "case {i} on {layout_name}");
            assert!(!a.traces.is_empty(), "case {i} on {layout_name}");
        }
    }
}

/// A multi-token result pages as prefixes: every limit returns the
/// first rows of the full answer (the ledger's cursor reruns the query
/// with a larger limit and drops the served prefix).
#[test]
fn multi_token_cursor_partition() {
    let dir = tempfile::tempdir().unwrap();
    let world = world(dir.path());
    let query = tokens(vec![
        pos(vec![cond(
            PredicateTarget::Attribute(AttributeOwner::Resource, "service.name".into()),
            CompareOp::Regex,
            vec![text("svc-.*")],
        )]),
        neg(vec![span_attr_eq("http.method", "GET")]),
    ]);
    for layout_name in LAYOUTS {
        let full = run(
            layout(dir.path(), &world, layout_name),
            SearchQuery::new(query.clone()).limit(10),
        );
        // Rows path: T6's stored 1950 s resend ranks it first.
        assert_eq!(
            ids(&full),
            vec![hex(6), hex(5), hex(4), hex(2)],
            "{layout_name}"
        );
        let full = norm(&full).0;
        for limit in 1..=full.len() {
            let page = run(
                layout(dir.path(), &world, layout_name),
                SearchQuery::new(query.clone()).limit(limit),
            );
            assert_eq!(norm(&page).0, full[..limit], "{layout_name}, limit {limit}");
        }
    }
}

/// Token queries stop at the work ceilings: the visited-rows ceiling
/// (tails, compiles and emissions, the driver count included) and the
/// dictionary-bytes ceiling of a free-text token's walks.
#[test]
fn token_ceilings_terminate() {
    let dir = tempfile::tempdir().unwrap();
    let world = world(dir.path());
    let query = tokens(vec![
        pos(vec![cond(
            PredicateTarget::Attribute(AttributeOwner::Resource, "service.name".into()),
            CompareOp::Regex,
            vec![text("svc-.*")],
        )]),
        pos(vec![builtin(
            BuiltinField::Status,
            CompareOp::Regex,
            vec![text("OK|UNSET")],
        )]),
    ]);
    for layout_name in LAYOUTS {
        let data = run(
            layout(dir.path(), &world, layout_name),
            SearchQuery::new(query.clone()).visited_rows_ceiling_for_tests(1),
        );
        assert_eq!(
            data.status,
            QueryStatus::Partial([PartialReason::WorkCeiling].into()),
            "{layout_name}"
        );
    }
    let text_query = tokens(vec![pos(vec![cond(
        PredicateTarget::FreeText,
        CompareOp::Contains,
        vec![text("users")],
    )])]);
    let data = run(
        layout(dir.path(), &world, "single-sealed"),
        SearchQuery::new(text_query.clone()).dict_bytes_ceiling_for_tests(1),
    );
    assert_eq!(
        data.status,
        QueryStatus::Partial([PartialReason::WorkCeiling].into())
    );
    let data = run(
        layout(dir.path(), &world, "single-sealed"),
        SearchQuery::new(text_query),
    );
    assert_eq!(ids(&data), vec![hex(1)]);
    assert_eq!(data.status, QueryStatus::Complete);
}

/// A negated token needs the whole trace: when the assembly is capped,
/// the unseen spans may hold what it forbids, so the trace is excluded
/// as indeterminate (and the query is Partial). A trace assembled whole
/// still returns.
#[test]
fn negated_token_excludes_capped_traces() {
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(
        dir.path(),
        vec![req_with(
            vec![kv_str("service.name", "svc-x")],
            None,
            &[
                // T1: an OK root, then an ERROR child the cap hides.
                SpanSpec {
                    status: Some((1, "")),
                    ..span_in(1, 0x11, 0, 1_000 * NS, "root")
                },
                SpanSpec {
                    status: Some((2, "boom")),
                    ..span_in(1, 0x12, 0x11, 1_001 * NS, "child")
                },
                // T2: one OK span, whole under the cap.
                SpanSpec {
                    status: Some((1, "")),
                    ..span_in(2, 0x21, 0, 1_002 * NS, "solo")
                },
            ],
        )],
        "capped-negation",
    );
    let sources = || both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let query = || {
        canon_query(tokens(vec![
            pos(vec![attr_eq(
                AttributeOwner::Resource,
                "service.name",
                "svc-x",
            )]),
            neg(vec![builtin(
                BuiltinField::Status,
                CompareOp::Eq,
                vec![text("ERROR")],
            )]),
        ]))
    };
    let whole = run(sources(), query());
    assert_eq!(ids(&whole), vec![hex(2)]);
    assert_eq!(whole.status, QueryStatus::Complete);
    let capped = run(sources(), query().span_cap_for_tests(1));
    assert_eq!(ids(&capped), vec![hex(2)]);
    assert_eq!(
        capped.status,
        QueryStatus::Partial([PartialReason::SizeCap].into())
    );
}

// ── The rows path (decision 49) ──────────────────────────────────────

/// The row of trace `n` in `data`.
fn row_of(data: &SearchData, n: u8) -> &sfsq::traces::TraceSummary {
    data.traces
        .iter()
        .find(|t| t.trace_id.to_string() == hex(n))
        .unwrap_or_else(|| panic!("trace {n} in {:?}", ids(data)))
}

/// Rows count stored rows: a resent span counts twice and its later copy
/// sets the rank; assembly keeps one canonical copy.
#[test]
fn stored_rows_count_resends_and_a_losing_copy_matches() {
    let dir = tempfile::tempdir().unwrap();
    let svc = || vec![kv_str("service.name", "svc")];
    let wal = write_wal(
        dir.path(),
        vec![
            req_with(svc(), None, &[span_in(1, 0x11, 0, 100 * NS, "op")]),
            req_with(svc(), None, &[span_in(1, 0x11, 0, 300 * NS, "op")]),
        ],
        "resend",
    );
    let sources = || both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let rows = run(sources(), SearchQuery::new(pred(vec![])));
    let canon = run(sources(), canon_query(pred(vec![])));
    let (row, assembled) = (row_of(&rows, 1), row_of(&canon, 1));
    assert_eq!(
        (row.span_count, row.newest_matched_start_ns),
        (2, 300 * NS as i64)
    );
    assert_eq!(assembled.span_count, 1, "assembly dedups the resend");
}

/// The row's root is the earliest TRUE claim across files; a withheld
/// claim at or before it leaves the root unknown, and a trace without
/// an unset-parent span has none (no display promotion).
#[test]
fn root_is_the_earliest_recorded_claim_across_files() {
    let dir = tempfile::tempdir().unwrap();
    let svc = || vec![kv_str("service.name", "svc")];
    let mut tie_a = span_in(2, 0x22, 0, 15 * NS, "tie");
    tie_a.kind = 1;
    let mut tie_b = tie_a.clone();
    tie_b.kind = 2;
    let wal_a = write_wal(
        dir.path(),
        vec![req_with(
            svc(),
            None,
            &[
                span_in(1, 0x11, 0, 10 * NS, "late"),
                span_in(2, 0x21, 0, 20 * NS, "kept"),
                span_in(3, 0x31, 0x99, 30 * NS, "orphan"),
            ],
        )],
        "a",
    );
    let wal_b = write_wal(
        dir.path(),
        vec![req_with(
            svc(),
            None,
            &[span_in(1, 0x12, 0, 5 * NS, "early"), tie_a, tie_b],
        )],
        "b",
    );
    let sources = || {
        both_roles(|| {
            vec![
                sealed_source(dir.path(), &wal_a, "a"),
                sealed_source(dir.path(), &wal_b, "b"),
            ]
        })
    };
    let rows = run(sources(), SearchQuery::new(pred(vec![])));
    assert_eq!(row_of(&rows, 1).root_name.as_deref(), Some("early"));
    assert_eq!(row_of(&rows, 2).root_name, None, "a withheld claim first");
    assert_eq!(row_of(&rows, 3).root_name, None, "rootless");
}

/// A holder sealed before TRSS sends its candidates to assembly
/// (canonical counts); a trace held only by files with stats keeps
/// stored-row counts. Trace 1 straddles both files, its resend pair in
/// the legacy one.
#[test]
fn legacy_holder_falls_back_to_assembly() {
    let dir = tempfile::tempdir().unwrap();
    let svc = || vec![kv_str("service.name", "svc")];
    let pair = |t: u8, id: u8, at: u64| {
        vec![
            req_with(svc(), None, &[span_in(t, id, 0, at * NS, "op")]),
            req_with(svc(), None, &[span_in(t, id, 0, at * NS, "op")]),
        ]
    };
    let wal_old = write_wal(dir.path(), pair(1, 0x11, 100), "legacy-old");
    let mut new = pair(2, 0x21, 200);
    new.push(req_with(
        svc(),
        None,
        &[span_in(1, 0x12, 0x11, 150 * NS, "child")],
    ));
    let wal_new = write_wal(dir.path(), new, "legacy-new");
    drop(sealed_source(dir.path(), &wal_old, "old"));
    let old = dir.path().join("old.sfst");
    hide_chunk(&old, *b"TRSS");
    let sources = || {
        both_roles(|| {
            vec![
                sealed_source_at(&old, "old"),
                sealed_source(dir.path(), &wal_new, "new"),
            ]
        })
    };
    let data = run(sources(), SearchQuery::new(pred(vec![])));
    assert_eq!(data.status, QueryStatus::Complete);
    assert_eq!(row_of(&data, 1).span_count, 2, "assembled: the resend once");
    assert_eq!(row_of(&data, 2).span_count, 2, "stored rows");
}

/// A corrupt TRSS chunk is reported, and the trace still returns exact
/// through assembly (which never reads it).
#[test]
fn corrupt_stats_chunk_is_skipped_and_surfaced() {
    let dir = tempfile::tempdir().unwrap();
    let svc = vec![kv_str("service.name", "svc")];
    let wal = write_wal(
        dir.path(),
        vec![req_with(svc, None, &[span_in(1, 0x11, 0, 100 * NS, "op")])],
        "corrupt-trss",
    );
    drop(sealed_source(dir.path(), &wal, "one"));
    let path = dir.path().join("one.sfst");
    corrupt_chunk(&path, *b"TRSS");
    let data = run(
        both_roles(|| vec![sealed_source_at(&path, "one")]),
        SearchQuery::new(pred(vec![])),
    );
    assert_eq!(ids(&data), vec![hex(1)]);
    assert!(data.traces[0].exact);
    assert!(
        data.status.has(PartialReason::SourceFailure),
        "{:?}",
        data.status
    );
}

/// A row reads rollups, never spans, so the span cap does not touch it.
#[test]
fn rows_never_hit_the_span_cap() {
    let dir = tempfile::tempdir().unwrap();
    let svc = vec![kv_str("service.name", "svc")];
    let spans: Vec<SpanSpec> = (0..4u8)
        .map(|i| {
            span_in(
                1,
                0x11 + i,
                if i == 0 { 0 } else { 0x11 },
                (100 + u64::from(i)) * NS,
                "op",
            )
        })
        .collect();
    let wal = write_wal(dir.path(), vec![req_with(svc, None, &spans)], "cap");
    let sources = || both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let rows = run(
        sources(),
        SearchQuery::new(pred(vec![])).span_cap_for_tests(2),
    );
    assert_eq!(rows.status, QueryStatus::Complete);
    assert_eq!((rows.traces[0].span_count, rows.traces[0].exact), (4, true));
    let canon = run(sources(), canon_query(pred(vec![])).span_cap_for_tests(2));
    assert!(canon.status.has(PartialReason::SizeCap));
}
