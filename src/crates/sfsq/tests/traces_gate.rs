//! Integration tests for the trace-level pre-assembly gate — the
//! TBLM-bloom + TRSU-rollup candidate prune filter `search` consults
//! at each pool pop (sfsq/src/traces/gate.rs; gate engagement and the
//! pruned-pops-don't-charge-the-ceiling rule live in
//! sfsq/src/traces/search.rs).
//!
//! Fixtures come from tests/common/mod.rs: requests written to a WAL,
//! sealed with `sealed_source` (or `sealed_source_at` after byte
//! surgery on the sealed file), plus `tail_source` for tail presence.
//! Each query runs twice — gate on vs `trace_gate_for_tests(false)` —
//! and superset tests compare the answers through `norm`; the
//! corrupt-file tests assert the designed divergence instead. Pinned
//! contracts:
//!
//! - **Superset:** gate-on answers are byte-identical to the gate-off
//!   truth path on tie-free, corruption-free, single-valued corpora —
//!   the gate only removes work, never results. The DESIGNED
//!   differences live elsewhere: corrupt files (skip-and-surface) and
//!   the status deltas a prune makes unobservable (a would-have-been
//!   `SizeCap`; corruption only assembly would have found).
//! - **The incident fix:** a rare trace-level predicate no longer burns
//!   the assembled ceiling on discards — the gate-off `WorkCeiling`
//!   partial becomes a gate-on `Complete`.
//! - **Skip-and-surface:** a file that proves itself corrupt through any
//!   gate read (TBLM, TRSU) is skipped as a failed source AND surfaced
//!   (`SourceFailure`), never silently downgraded.
//! - **The single-source proof and the token gate** only remove work:
//!   proof on, proof off and gates off agree; a file without a rollup
//!   turns the proof off; the gates' reads are charged to the visited
//!   ceiling.
//!
//! Not pinned here: the no-root prune itself — its verdicts are
//! observable, the prune is not (`GateStats` is crate-internal; see
//! `rootless_traces_never_match_root_filters_either_polarity`) — and
//! search's setup-failure engagement precondition (a completion source
//! that failed to open disengages the gate; no fixture here fails at
//! setup).

mod common;

use std::path::Path;
use std::sync::Arc;
use std::sync::atomic::AtomicUsize;

use tokio_util::sync::CancellationToken;

use common::{
    SpanSpec, corrupt_chunk, hide_chunk, kv_str, req_with, sealed_source, sealed_source_at, sp,
    tail_source, write_wal,
};
use opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest;
use sfsq::traces::{
    AttributeOwner, BuiltinField, CompareOp, Condition, PartialReason, Predicate, PredicateTarget,
    PredicateValue, QueryStatus, SearchData, SearchQuery, SearchSources, Token, TokenQuery,
    TraceSource, search,
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

fn builtin(field: BuiltinField, op: CompareOp, values: Vec<PredicateValue>) -> Condition {
    Condition {
        target: PredicateTarget::Builtin(field),
        op,
        values,
    }
}

fn pred(conditions: Vec<Condition>) -> Predicate {
    Predicate { conditions }
}

/// One `search` call with a fresh cancellation token and progress
/// counter; invalid requests panic rather than error.
fn run(sources: SearchSources, query: SearchQuery) -> SearchData {
    search(
        sources,
        query,
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
    .expect("valid request")
}

/// Identical source vectors in both roles: `build()` runs once per
/// role because `SearchSources` owns its vectors, and window ⊆
/// completion holds trivially when both sides seal the same WALs.
fn both_roles(build: impl Fn() -> Vec<TraceSource>) -> SearchSources {
    SearchSources {
        window: build(),
        completion: build(),
    }
}

fn ids(data: &SearchData) -> Vec<String> {
    data.traces.iter().map(|t| t.trace_id.to_string()).collect()
}

/// `tid(n)`'s canonical hex rendering — the text form `trace:id =`
/// conditions pin.
fn hex(n: u8) -> String {
    sfst::TraceId::from(tid(n)).to_string()
}

/// One summary rendered comparable across gate settings — trace-level
/// values AND the rank (the newest matching span), so the differential
/// sweep also catches a gate-induced change in which spans match.
type NormRow = (
    String,
    Option<String>,
    Option<String>,
    i64,
    i64,
    usize,
    usize,
    i64,
    bool,
    (Vec<(String, u64)>, u64, u64),
);
type Norm = (Vec<NormRow>, QueryStatus);

fn norm(data: &SearchData) -> Norm {
    (
        data.traces
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
            .collect(),
        data.status.clone(),
    )
}

fn span_in(trace: u8, id: u8, parent: u8, start: u64, name: &'static str) -> SpanSpec {
    SpanSpec {
        trace: tid(trace),
        ..sp(id, parent, start, name)
    }
}

/// A root span (unset parent) with an explicit end.
fn root_span(trace: u8, id: u8, start: u64, end: u64, name: &'static str) -> SpanSpec {
    SpanSpec {
        end,
        ..span_in(trace, id, 0, start, name)
    }
}

fn root_service_eq(value: &str) -> Condition {
    builtin(
        BuiltinField::RootServiceName,
        CompareOp::Eq,
        vec![text(value)],
    )
}

fn min_duration(ns: i64) -> Condition {
    builtin(
        BuiltinField::TraceDuration,
        CompareOp::Gte,
        vec![PredicateValue::Integer(ns)],
    )
}

/// The path a [`sealed_source`] wrote (its naming contract).
fn sealed_path(dir: &Path, id: &str) -> std::path::PathBuf {
    dir.join(format!("{id}.sfst"))
}

// ── The incident shape ───────────────────────────────────────────────

/// Many candidates, none matching a rare root selection: gate-off burns
/// the assembled ceiling on discards (`Partial{WorkCeiling}`); gate-on
/// prunes every candidate without assembling — a pruned pop charges
/// nothing — and PROVES the empty answer (`Complete`). The incident
/// fix, pinned.
#[test]
fn rare_root_selection_completes_instead_of_burning_the_ceiling() {
    let dir = tempfile::tempdir().unwrap();
    let hot = vec![kv_str("service.name", "hot")];
    // 70 single-span traces > the 64-assembly floor at limit 1.
    let reqs: Vec<ExportTraceServiceRequest> = (0..70u8)
        .map(|i| {
            req_with(
                hot.clone(),
                None,
                &[root_span(
                    i + 1,
                    0x10,
                    u64::from(i + 1) * NS,
                    u64::from(i + 1) * NS + 50,
                    "op",
                )],
            )
        })
        .collect();
    let wal = write_wal(dir.path(), reqs, "incident");
    let q = || SearchQuery::new(pred(vec![root_service_eq("cold")])).limit(1);

    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let off = run(
        sources,
        q().trace_gate_for_tests(false).rows_for_tests(false),
    );
    assert!(off.traces.is_empty());
    assert!(
        off.status.has(PartialReason::WorkCeiling),
        "gate-off burns the ceiling: {:?}",
        off.status
    );

    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let on = run(sources, q());
    assert!(on.traces.is_empty());
    assert_eq!(
        on.status,
        QueryStatus::Complete,
        "gate-on proves the empty answer"
    );

    // The same corpus still answers the MATCHING selection.
    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let hit = run(
        sources,
        SearchQuery::new(pred(vec![root_service_eq("hot")])).limit(1),
    );
    assert_eq!(hit.traces.len(), 1);
}

// ── Superset rows ────────────────────────────────────────────────────

/// A trace straddling two sealed files: each file's envelope is short,
/// only the MERGED envelope reaches the duration bound — a per-file
/// reading would false-prune. The root lives in one file only, so root
/// selection must also survive cross-file evidence.
#[test]
fn straddling_trace_survives_merged_envelope_and_split_root() {
    let dir = tempfile::tempdir().unwrap();
    let svc = vec![kv_str("service.name", "svc-a")];
    // File A: the root, [100s, 100s+1ms]. File B: a child,
    // [109s, 110s]. Merged duration: 10s.
    let a = req_with(
        svc.clone(),
        None,
        &[root_span(
            1,
            0x11,
            100 * NS,
            100 * NS + 1_000_000,
            "root-op",
        )],
    );
    let b = req_with(
        svc.clone(),
        None,
        &[SpanSpec {
            end: 110 * NS,
            ..span_in(1, 0x12, 0x11, 109 * NS, "child-op")
        }],
    );
    let wal_a = write_wal(dir.path(), vec![a], "straddle-a");
    let wal_b = write_wal(dir.path(), vec![b], "straddle-b");
    let sources = || {
        both_roles(|| {
            vec![
                sealed_source(dir.path(), &wal_a, "a"),
                sealed_source(dir.path(), &wal_b, "b"),
            ]
        })
    };
    // ≥ 5s: each file's own envelope (1ms / 1s) is below, merged is 10s.
    let q = || SearchQuery::new(pred(vec![min_duration(5 * NS as i64)]));
    let on = run(sources(), q());
    let off = run(sources(), q().trace_gate_for_tests(false));
    assert_eq!(ids(&on), vec![hex(1)], "merged envelope, not per-file");
    assert_eq!(norm(&on), norm(&off));

    // Root selection with the root in file A only.
    let q = || SearchQuery::new(pred(vec![root_service_eq("svc-a")]));
    let on = run(sources(), q());
    let off = run(sources(), q().trace_gate_for_tests(false));
    assert_eq!(ids(&on), vec![hex(1)]);
    assert_eq!(norm(&on), norm(&off));

    // One loaded session at a time: the gate's evidence (rollups, root
    // values) and the assembly reload what a release dropped.
    for q in [
        SearchQuery::new(pred(vec![min_duration(5 * NS as i64)])),
        SearchQuery::new(pred(vec![root_service_eq("svc-a")])),
    ] {
        let capped = run(sources(), q.clone().loaded_sources_cap_for_tests(1));
        assert_eq!(norm(&capped), norm(&run(sources(), q)));
    }
}

/// Tail spans are invisible to the gate's evidence: a candidate with
/// ANY tail presence is unprunable — here the tail extends a duration
/// past the bound that the sealed envelope alone would fail.
#[test]
fn tail_presence_blocks_pruning() {
    let dir = tempfile::tempdir().unwrap();
    let svc = vec![kv_str("service.name", "svc-a")];
    let sealed_req = req_with(
        svc.clone(),
        None,
        &[root_span(
            1,
            0x11,
            100 * NS,
            100 * NS + 1_000_000,
            "root-op",
        )],
    );
    let tail_req = req_with(
        svc.clone(),
        None,
        &[SpanSpec {
            end: 110 * NS,
            ..span_in(1, 0x12, 0x11, 109 * NS, "tail-op")
        }],
    );
    let wal_sealed = write_wal(dir.path(), vec![sealed_req], "tailblock-sealed");
    let wal_tail = write_wal(dir.path(), vec![tail_req], "tailblock-tail");
    let sources = || {
        both_roles(|| {
            vec![
                sealed_source(dir.path(), &wal_sealed, "sealed"),
                tail_source(&wal_tail, "tail"),
            ]
        })
    };
    let q = || SearchQuery::new(pred(vec![min_duration(5 * NS as i64)]));
    let on = run(sources(), q());
    let off = run(sources(), q().trace_gate_for_tests(false));
    assert_eq!(ids(&on), vec![hex(1)], "tail extends the true duration");
    assert_eq!(norm(&on), norm(&off));
}

/// A pre-rollup completion file (a real seal with its TRSU hidden, and
/// in the second variant its TBLM too): every candidate it might hold
/// is uncovered → unprunable; results match gate-off exactly, and NO
/// SourceFailure appears — absence is a version fact, not corruption.
#[test]
fn absent_chunks_block_pruning_without_a_partial() {
    for hide_bloom_too in [false, true] {
        let dir = tempfile::tempdir().unwrap();
        let svc = vec![kv_str("service.name", "hot")];
        let reqs = vec![req_with(
            svc,
            None,
            &[root_span(1, 0x11, 100 * NS, 100 * NS + 50, "op")],
        )];
        let wal = write_wal(dir.path(), reqs, "prerollup");
        drop(sealed_source(dir.path(), &wal, "old"));
        let path = sealed_path(dir.path(), "old");
        hide_chunk(&path, *b"TRSU");
        if hide_bloom_too {
            hide_chunk(&path, *b"TBLM");
        }
        let sources = || both_roles(|| vec![sealed_source_at(&path, "old")]);
        let q = || SearchQuery::new(pred(vec![root_service_eq("cold")]));
        let on = run(sources(), q());
        let off = run(sources(), q().trace_gate_for_tests(false));
        assert_eq!(norm(&on), norm(&off), "hide_bloom_too={hide_bloom_too}");
        assert!(
            !on.status.has(PartialReason::SourceFailure),
            "absence is a version fact, not corruption: {:?}",
            on.status
        );
        // The matching selection still returns through assembly.
        let hit = run(
            sources(),
            SearchQuery::new(pred(vec![root_service_eq("hot")])),
        );
        assert_eq!(ids(&hit), vec![hex(1)], "hide_bloom_too={hide_bloom_too}");
    }
}

/// True-root filter semantics (decision 1D): a trace whose root span
/// was never exported matches NO root condition — either polarity —
/// with gate on and off agreeing, and the trace stays findable by
/// span-level conditions. What this does NOT pin: the gate's no-root
/// prune. Removing it leaves the test green — the post-assembly
/// evaluator reaches the same emptiness — and the prune itself has no
/// public observable (`GateStats` is crate-internal), so only the
/// verdict above is under contract here.
#[test]
fn rootless_traces_never_match_root_filters_either_polarity() {
    let dir = tempfile::tempdir().unwrap();
    let svc = vec![kv_str("service.name", "svc-a")];
    // Parent 0xEE never exported: no unset-parent span exists.
    let reqs = vec![req_with(
        svc,
        None,
        &[span_in(1, 0x11, 0xEE, 100 * NS, "orphan-op")],
    )];
    let wal = write_wal(dir.path(), reqs, "orphan");
    let sources = || both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    // Positive and NEGATED root conditions: both fail on a rootless
    // trace (absent-never-satisfies), gate-on == gate-off.
    for (op, value) in [
        (CompareOp::Eq, "orphan-op"),
        (CompareOp::NotEq, "something-else"),
    ] {
        let q = || {
            SearchQuery::new(pred(vec![builtin(
                BuiltinField::RootName,
                op,
                vec![text(value)],
            )]))
        };
        let on = run(sources(), q());
        let off = run(sources(), q().trace_gate_for_tests(false));
        assert!(on.traces.is_empty(), "rootless never matches ({op:?})");
        assert_eq!(norm(&on), norm(&off));
        assert_eq!(on.status, QueryStatus::Complete);
    }
    // Span-level conditions still find it.
    let by_span = run(
        sources(),
        SearchQuery::new(pred(vec![builtin(
            BuiltinField::Name,
            CompareOp::Eq,
            vec![text("orphan-op")],
        )])),
    );
    assert_eq!(ids(&by_span), vec![hex(1)]);
}

/// The 1D effectiveness win, pinned as the second incident shape: a
/// root selection over a ROOTLESS-heavy corpus. Gate-off assembles
/// every candidate just to exclude it (`Partial{WorkCeiling}`);
/// gate-on proves each one rootless from its `ROOT_CLAIM_NONE` row and
/// answers `Complete` — the no-root prune.
#[test]
fn rootless_heavy_corpus_completes_root_selections() {
    let dir = tempfile::tempdir().unwrap();
    let svc = vec![kv_str("service.name", "flagd")];
    // 70 two-span rootless traces (the parent was never exported).
    let reqs: Vec<ExportTraceServiceRequest> = (0..70u8)
        .map(|i| {
            req_with(
                svc.clone(),
                None,
                &[
                    span_in(i + 1, 0x10, 0xEE, u64::from(i + 1) * NS, "eval"),
                    span_in(i + 1, 0x11, 0x10, u64::from(i + 1) * NS + 5, "resolve"),
                ],
            )
        })
        .collect();
    let wal = write_wal(dir.path(), reqs, "rootless-heavy");
    let sources = || both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let q = || SearchQuery::new(pred(vec![root_service_eq("flagd")])).limit(1);

    let off = run(
        sources(),
        q().trace_gate_for_tests(false).rows_for_tests(false),
    );
    assert!(off.traces.is_empty());
    assert!(
        off.status.has(PartialReason::WorkCeiling),
        "gate-off burns the ceiling excluding rootless candidates: {:?}",
        off.status
    );

    let on = run(sources(), q());
    assert!(on.traces.is_empty());
    assert_eq!(
        on.status,
        QueryStatus::Complete,
        "the no-root prune proves it"
    );
}

/// Negated conditions never prune: a `!=` root selection and a `!=`
/// duration point answer identically with the gate on and off — the
/// duration case is the one place a naive lower-bound extraction would
/// silently corrupt results (Eq-compiled intervals carry a `lo`).
#[test]
fn negated_conditions_never_prune() {
    let dir = tempfile::tempdir().unwrap();
    let hot = vec![kv_str("service.name", "hot")];
    let cold = vec![kv_str("service.name", "cold")];
    // Durations: trace 1 = 2s, trace 2 = 6s.
    let reqs = vec![
        req_with(hot, None, &[root_span(1, 0x11, 100 * NS, 102 * NS, "op-a")]),
        req_with(
            cold,
            None,
            &[root_span(2, 0x21, 200 * NS, 206 * NS, "op-b")],
        ),
    ];
    let wal = write_wal(dir.path(), reqs, "negated");
    let sources = || both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);

    let q = || {
        SearchQuery::new(pred(vec![builtin(
            BuiltinField::RootServiceName,
            CompareOp::NotEq,
            vec![text("hot")],
        )]))
    };
    let on = run(sources(), q());
    let off = run(sources(), q().trace_gate_for_tests(false));
    assert_eq!(ids(&on), vec![hex(2)]);
    assert_eq!(norm(&on), norm(&off));

    // `trace:duration != 6s`: trace 1 (2s) must return — a naive
    // reading of the Eq-compiled interval's lo (6s) would prune it.
    let q = || {
        SearchQuery::new(pred(vec![builtin(
            BuiltinField::TraceDuration,
            CompareOp::NotEq,
            vec![PredicateValue::Integer(6 * NS as i64)],
        )]))
    };
    let on = run(sources(), q());
    let off = run(sources(), q().trace_gate_for_tests(false));
    assert_eq!(ids(&on), vec![hex(1)], "below the != point still returns");
    assert_eq!(norm(&on), norm(&off));
}

/// Multi-value `=` duration (an OR of points): the gate's bound is the
/// MIN point — a trace between points assembles (and is then decided by
/// the truth path), a trace below every point is pruned, a trace AT a
/// point returns.
#[test]
fn multi_value_duration_bound_is_the_min_point() {
    let dir = tempfile::tempdir().unwrap();
    let svc = vec![kv_str("service.name", "svc")];
    // Durations: 3s / 5s / 20s.
    let reqs = vec![
        req_with(
            svc.clone(),
            None,
            &[root_span(1, 0x11, 100 * NS, 103 * NS, "op")],
        ),
        req_with(
            svc.clone(),
            None,
            &[root_span(2, 0x21, 200 * NS, 205 * NS, "op")],
        ),
        req_with(
            svc.clone(),
            None,
            &[root_span(3, 0x31, 300 * NS, 320 * NS, "op")],
        ),
    ];
    let wal = write_wal(dir.path(), reqs, "points");
    let sources = || both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let q = || {
        SearchQuery::new(pred(vec![builtin(
            BuiltinField::TraceDuration,
            CompareOp::Eq,
            vec![
                PredicateValue::Integer(5 * NS as i64),
                PredicateValue::Integer(50 * NS as i64),
            ],
        )]))
    };
    let on = run(sources(), q());
    let off = run(sources(), q().trace_gate_for_tests(false));
    assert_eq!(ids(&on), vec![hex(2)], "only the 5s point matches");
    assert_eq!(norm(&on), norm(&off));
}

/// Upper bounds are never prunable (the envelope over-estimates), and
/// regex root selections flow through the same matcher as the truth
/// path.
#[test]
fn upper_bounds_and_regex_selections_match_gate_off() {
    let dir = tempfile::tempdir().unwrap();
    let svc = vec![kv_str("service.name", "svc-a")];
    let reqs = vec![
        req_with(
            svc.clone(),
            None,
            &[root_span(1, 0x11, 100 * NS, 102 * NS, "get-users")],
        ),
        req_with(
            svc.clone(),
            None,
            &[root_span(2, 0x21, 200 * NS, 220 * NS, "post-orders")],
        ),
    ];
    let wal = write_wal(dir.path(), reqs, "upper-regex");
    let sources = || both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);

    let q = || {
        SearchQuery::new(pred(vec![builtin(
            BuiltinField::TraceDuration,
            CompareOp::Lte,
            vec![PredicateValue::Integer(5 * NS as i64)],
        )]))
    };
    let on = run(sources(), q());
    let off = run(sources(), q().trace_gate_for_tests(false));
    assert_eq!(ids(&on), vec![hex(1)]);
    assert_eq!(norm(&on), norm(&off));

    let q = || {
        SearchQuery::new(pred(vec![builtin(
            BuiltinField::RootName,
            CompareOp::Regex,
            vec![text("get-.*")],
        )]))
    };
    let on = run(sources(), q());
    let off = run(sources(), q().trace_gate_for_tests(false));
    assert_eq!(ids(&on), vec![hex(1)]);
    assert_eq!(norm(&on), norm(&off));
}

/// Two files each claiming a true root for the same trace (a resent
/// root with contradictory service values): ANY matching claim
/// survives; only all-claims-fail prunes. Both answers must equal the
/// gate-off truth.
#[test]
fn split_root_claims_survive_on_any_match() {
    let dir = tempfile::tempdir().unwrap();
    // The SAME root span resent through two services (producers
    // contradicting themselves across files; same span id, same start
    // — the canonical copy is content-tie-broken, so assert only
    // equality with gate-off, not a specific value).
    let a = req_with(
        vec![kv_str("service.name", "svc-a")],
        None,
        &[root_span(1, 0x11, 100 * NS, 101 * NS, "op")],
    );
    let b = req_with(
        vec![kv_str("service.name", "svc-b")],
        None,
        &[root_span(1, 0x11, 100 * NS, 101 * NS, "op")],
    );
    let wal_a = write_wal(dir.path(), vec![a], "claims-a");
    let wal_b = write_wal(dir.path(), vec![b], "claims-b");
    let sources = || {
        both_roles(|| {
            vec![
                sealed_source(dir.path(), &wal_a, "a"),
                sealed_source(dir.path(), &wal_b, "b"),
            ]
        })
    };
    for service in ["svc-a", "svc-b", "svc-c"] {
        let q = || SearchQuery::new(pred(vec![root_service_eq(service)]));
        let on = run(sources(), q());
        let off = run(sources(), q().trace_gate_for_tests(false));
        assert_eq!(norm(&on), norm(&off), "root={service}");
    }
}

/// Pinned candidate sets (`trace:id =`) never engage the gate: the
/// lookup must always assemble, trace-level conditions included.
#[test]
fn pinned_lookups_bypass_the_gate() {
    let dir = tempfile::tempdir().unwrap();
    let svc = vec![kv_str("service.name", "hot")];
    let reqs = vec![req_with(
        svc,
        None,
        &[root_span(5, 0x51, 100 * NS, 101 * NS, "op")],
    )];
    let wal = write_wal(dir.path(), reqs, "pinned");
    let sources = || both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let q = || {
        SearchQuery::new(pred(vec![
            builtin(BuiltinField::TraceId, CompareOp::Eq, vec![text(&hex(5))]),
            root_service_eq("hot"),
        ]))
    };
    let on = run(sources(), q());
    let off = run(sources(), q().trace_gate_for_tests(false));
    assert_eq!(ids(&on), vec![hex(5)]);
    assert_eq!(norm(&on), norm(&off));
}

// ── Skip-and-surface (corrupt files) ─────────────────────────────────

/// A corrupt TRSU chunk (CRC flip): the gate skips the whole file as a
/// failed source AND surfaces it — `Partial{SourceFailure}`, the
/// candidate excluded as indeterminate. Never a prune, never silence.
#[test]
fn corrupt_rollup_chunk_is_skipped_and_surfaced() {
    let dir = tempfile::tempdir().unwrap();
    let svc = vec![kv_str("service.name", "hot")];
    let reqs = vec![req_with(
        svc,
        None,
        &[root_span(1, 0x11, 100 * NS, 101 * NS, "op")],
    )];
    let wal = write_wal(dir.path(), reqs, "corrupt-trsu");
    // Seal once, corrupt in place, then build sources WITHOUT resealing.
    drop(sealed_source(dir.path(), &wal, "one"));
    let path = sealed_path(dir.path(), "one");
    corrupt_chunk(&path, *b"TRSU");
    let build = || both_roles(|| vec![sealed_source_at(&path, "one")]);

    let q = || SearchQuery::new(pred(vec![root_service_eq("hot")]));
    let on = run(build(), q());
    assert!(
        on.traces.is_empty(),
        "indeterminate candidates are excluded"
    );
    assert!(
        on.status.has(PartialReason::SourceFailure),
        "the skip is surfaced: {:?}",
        on.status
    );

    // Gate-off never reads TRSU (only the gate does): the corruption
    // goes UNDISCOVERED and the trace returns — discovery laziness
    // cuts both ways.
    let off = run(
        build(),
        q().trace_gate_for_tests(false).rows_for_tests(false),
    );
    assert_eq!(ids(&off), vec![hex(1)]);
    assert_eq!(off.status, QueryStatus::Complete);
}

/// A corrupt TRSU chunk under a token-only query: the single-source
/// proof cannot be built and says so — `SourceFailure` — instead of
/// turning itself off silently. Assembly never reads the rollup, so
/// the result stays exact.
#[test]
fn corrupt_rollup_under_the_proof_is_surfaced() {
    let dir = tempfile::tempdir().unwrap();
    let svc = vec![kv_str("service.name", "hot")];
    let reqs = vec![req_with(
        svc,
        None,
        &[root_span(1, 0x11, 100 * NS, 101 * NS, "op")],
    )];
    let wal = write_wal(dir.path(), reqs, "corrupt-proof");
    drop(sealed_source(dir.path(), &wal, "one"));
    let path = sealed_path(dir.path(), "one");
    corrupt_chunk(&path, *b"TRSU");
    let build = || both_roles(|| vec![sealed_source_at(&path, "one")]);

    let data = run(
        build(),
        SearchQuery::new(tokens(vec![
            Token::positive(pred(vec![service_eq("hot")])),
            Token::positive(pred(vec![name_eq("op")])),
        ])),
    );
    assert_eq!(ids(&data), vec![hex(1)]);
    assert!(data.traces[0].exact, "assembly never read the rollup");
    assert!(
        data.status.has(PartialReason::SourceFailure),
        "the corrupt rollup is surfaced: {:?}",
        data.status
    );
}

/// A corrupt TBLM chunk: same principle through the all-file path's
/// other read (its screens). The single-source proof decides from the
/// rollup alone and never reads the bloom, a lookup hint whose
/// corruption the session's lookups already survive (they fall back to
/// the exact index): the trace returns, as with the gate off —
/// discovery laziness again.
#[test]
fn corrupt_bloom_chunk_is_skipped_and_surfaced() {
    let dir = tempfile::tempdir().unwrap();
    let svc = vec![kv_str("service.name", "hot")];
    let reqs = vec![req_with(
        svc,
        None,
        &[root_span(1, 0x11, 100 * NS, 101 * NS, "op")],
    )];
    let wal = write_wal(dir.path(), reqs, "corrupt-tblm");
    drop(sealed_source(dir.path(), &wal, "one"));
    let path = sealed_path(dir.path(), "one");
    corrupt_chunk(&path, *b"TBLM");
    let build = || both_roles(|| vec![sealed_source_at(&path, "one")]);

    let q = || SearchQuery::new(pred(vec![root_service_eq("hot")]));
    let on = run(
        build(),
        q().single_source_proof_ids_for_tests(0)
            .rows_for_tests(false),
    );
    assert!(on.traces.is_empty());
    assert!(
        on.status.has(PartialReason::SourceFailure),
        "{:?}",
        on.status
    );
    // The rows path reads past the hint (exact lookups answer) and
    // still reports it.
    let rows = run(build(), q().single_source_proof_ids_for_tests(0));
    assert_eq!(ids(&rows), vec![hex(1)]);
    assert!(
        rows.status.has(PartialReason::SourceFailure),
        "{:?}",
        rows.status
    );

    let proof = run(build(), q());
    assert_eq!(ids(&proof), vec![hex(1)]);
    assert_eq!(proof.status, QueryStatus::Complete);
}

/// The recorded status delta, pinned: a pruned candidate's would-have-
/// been `SizeCap` partial is never observed — gate-off assembles the
/// non-matching capped trace (Partial{SizeCap}); gate-on prunes it
/// first (Complete). A DESIGNED difference, not a regression.
#[test]
fn pruned_candidates_sizecap_partial_is_never_observed() {
    let dir = tempfile::tempdir().unwrap();
    let cold = vec![kv_str("service.name", "cold")];
    let hot = vec![kv_str("service.name", "hot")];
    let reqs = vec![
        // Trace 1: 4 spans — truncated under a span cap of 2.
        req_with(
            cold,
            None,
            &[
                root_span(1, 0x11, 100 * NS, 101 * NS, "op"),
                span_in(1, 0x12, 0x11, 100 * NS + 10, "child"),
                span_in(1, 0x13, 0x11, 100 * NS + 20, "child"),
                span_in(1, 0x14, 0x11, 100 * NS + 30, "child"),
            ],
        ),
        req_with(hot, None, &[root_span(2, 0x21, 200 * NS, 201 * NS, "op")]),
    ];
    let wal = write_wal(dir.path(), reqs, "sizecap-delta");
    let sources = || both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let q = || SearchQuery::new(pred(vec![root_service_eq("hot")])).span_cap_for_tests(2);

    let off = run(
        sources(),
        q().trace_gate_for_tests(false).rows_for_tests(false),
    );
    assert_eq!(ids(&off), vec![hex(2)]);
    assert!(
        off.status.has(PartialReason::SizeCap),
        "gate-off assembles the capped non-match: {:?}",
        off.status
    );

    let on = run(sources(), q());
    assert_eq!(ids(&on), vec![hex(2)]);
    assert_eq!(on.status, QueryStatus::Complete, "the prune skips the cap");
}

/// Mid-tier root-name dictionaries resolve through the gate (150
/// distinct names push `name` past the low-card threshold), and the
/// resolver's closed rule holds on a healthy file: a ref resolved
/// against the WRONG field — or a field the file does not carry — is
/// Corrupt, never a value and never absence.
#[test]
fn mid_tier_resolution_and_the_resolver_closed_rule() {
    let dir = tempfile::tempdir().unwrap();
    let svc = vec![kv_str("service.name", "svc")];
    let names: Vec<String> = (0..150).map(|i| format!("op-{i:03}")).collect();
    let leaked: Vec<&'static str> = names
        .iter()
        .map(|n| &*Box::leak(n.clone().into_boxed_str()))
        .collect();
    let reqs: Vec<ExportTraceServiceRequest> = (0..150u64)
        .map(|i| {
            req_with(
                svc.clone(),
                None,
                &[root_span(
                    (i + 1) as u8,
                    0x10,
                    (i + 1) * NS,
                    (i + 1) * NS + 50,
                    leaked[i as usize],
                )],
            )
        })
        .collect();
    let wal = write_wal(dir.path(), reqs, "midtier");
    let sources = || both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);

    // One distinct name per trace (ids 1..=150 stay inside u8): the
    // query targets its trace by name, and the assertion pins ONE
    // match — not which trace id returned.
    let q = || {
        SearchQuery::new(pred(vec![builtin(
            BuiltinField::RootName,
            CompareOp::Eq,
            vec![text("op-042")],
        )]))
    };
    let on = run(sources(), q());
    let off = run(sources(), q().trace_gate_for_tests(false));
    assert_eq!(
        on.traces.len(),
        1,
        "mid-tier dictionary resolves in the gate"
    );
    assert_eq!(norm(&on), norm(&off));

    // The closed rule, on the sealed file directly.
    let bytes = std::fs::read(sealed_path(dir.path(), "one")).unwrap();
    let reader = sfst::IndexReader::open(&bytes).unwrap();
    let rollup = reader.trace_rollup().unwrap();
    let name_ref = rollup.root_name_refs[0];
    let service_ref = rollup.root_service_refs[0];
    assert_ne!(name_ref, sfst::ROLLUP_NO_REF);
    assert_ne!(service_ref, sfst::ROLLUP_NO_REF);
    let mut resolver = sfst::RollupRefResolver::new();
    assert!(
        matches!(
            resolver.resolve(&reader, "name", name_ref),
            sfst::RollupRefOutcome::Value(v) if v.starts_with("op-")
        ),
        "a ref in its own field is a proven value"
    );
    assert!(
        matches!(
            resolver.resolve(&reader, "name", service_ref),
            sfst::RollupRefOutcome::Corrupt
        ),
        "a wrong-field ref is corruption evidence, never absence"
    );
    assert!(
        matches!(
            resolver.resolve(&reader, "no.such.field", name_ref),
            sfst::RollupRefOutcome::Corrupt
        ),
        "a ref into an absent field can never be proven"
    );
}

/// The second recorded status delta, pinned (review finding): a pruned
/// candidate's assembly never runs, so corruption living only in
/// assembly-read chunks (here `SPAN`) goes UNDISCOVERED when the gate
/// prunes the candidate — gate-off assembles and surfaces
/// `SourceFailure`. Same designed class as the SizeCap delta.
#[test]
fn pruning_masks_assembly_only_corruption_by_design() {
    let dir = tempfile::tempdir().unwrap();
    let cold = vec![kv_str("service.name", "cold")];
    let reqs = vec![req_with(
        cold,
        None,
        &[root_span(1, 0x11, 100 * NS, 101 * NS, "op")],
    )];
    let wal = write_wal(dir.path(), reqs, "masked");
    drop(sealed_source(dir.path(), &wal, "one"));
    let path = sealed_path(dir.path(), "one");
    // SPAN is read by assembly only (discovery reads TRCE/TIMS; the
    // gate reads TBLM/TRSU) — the shape that separates the two paths.
    corrupt_chunk(&path, *b"SPAN");
    let sources = || both_roles(|| vec![sealed_source_at(&path, "one")]);
    let q = || SearchQuery::new(pred(vec![root_service_eq("hot")]));

    let on = run(sources(), q());
    assert!(on.traces.is_empty());
    assert_eq!(
        on.status,
        QueryStatus::Complete,
        "the prune never reads SPAN: corruption stays undiscovered"
    );

    let off = run(
        sources(),
        q().trace_gate_for_tests(false).rows_for_tests(false),
    );
    assert!(off.traces.is_empty());
    assert!(
        off.status.has(PartialReason::SourceFailure),
        "assembly discovers what the prune skipped: {:?}",
        off.status
    );
}

/// The recorder's tie ABSTENTION, pinned end to end: two unset-parent
/// spans sharing `(start_ns, span_id)` with different kinds and
/// per-kind services. The recorder cannot model the combiner's deeper
/// tie-break keys, so it records NO root claim — the gate treats the
/// candidate as unprunable and gate-on answers byte-identically to the
/// truth path for BOTH tie services. (Before the abstention this shape
/// was the ruled kind-tie recall miss; the abstention closed it.)
#[test]
fn ambiguous_root_ties_abstain_and_never_diverge() {
    let dir = tempfile::tempdir().unwrap();
    // Same span id + start; CLIENT(3)/svc-first stored first,
    // SERVER(2)/svc-canon second — the combiner orders by kind, so the
    // canonical root is the SERVER copy whichever was stored first.
    let first = req_with(
        vec![kv_str("service.name", "svc-first")],
        None,
        &[SpanSpec {
            kind: 3,
            ..root_span(1, 0x42, 100 * NS, 101 * NS, "op")
        }],
    );
    let second = req_with(
        vec![kv_str("service.name", "svc-canon")],
        None,
        &[SpanSpec {
            kind: 2,
            ..root_span(1, 0x42, 100 * NS, 101 * NS, "op")
        }],
    );
    let wal = write_wal(dir.path(), vec![first, second], "tie");
    let sources = || both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);

    for service in ["svc-canon", "svc-first"] {
        let q = || SearchQuery::new(pred(vec![root_service_eq(service)]));
        let on = run(sources(), q());
        let off = run(sources(), q().trace_gate_for_tests(false));
        assert_eq!(norm(&on), norm(&off), "root={service}");
    }
    // And the trace is findable by id (pins bypass the gate).
    let by_id = run(
        sources(),
        SearchQuery::new(pred(vec![builtin(
            BuiltinField::TraceId,
            CompareOp::Eq,
            vec![text(&hex(1))],
        )])),
    );
    assert_eq!(ids(&by_id), vec![hex(1)]);
}

/// High-tier root-name dictionaries resolve through the gate: 1100
/// distinct names push `name` past the high-card threshold (10× the
/// default 100), exercising the resolver's random-access + memo path.
#[test]
fn high_tier_resolution_matches_gate_off() {
    let dir = tempfile::tempdir().unwrap();
    let svc = vec![kv_str("service.name", "svc")];
    let names: Vec<&'static str> = (0..1100)
        .map(|i| &*Box::leak(format!("op-{i:04}").into_boxed_str()))
        .collect();
    let reqs: Vec<ExportTraceServiceRequest> = (0..1100u64)
        .map(|i| {
            let mut trace = [0u8; 16];
            trace[14..16].copy_from_slice(&(i as u16 + 1).to_be_bytes());
            req_with(
                svc.clone(),
                None,
                &[SpanSpec {
                    trace,
                    end: (i + 1) * NS + 50,
                    ..sp(0x10, 0, (i + 1) * NS, names[i as usize])
                }],
            )
        })
        .collect();
    let wal = write_wal(dir.path(), reqs, "hightier");
    let sources = || both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let q = || {
        SearchQuery::new(pred(vec![builtin(
            BuiltinField::RootName,
            CompareOp::Eq,
            vec![text("op-1090")],
        )]))
    };
    let on = run(sources(), q());
    let off = run(
        sources(),
        q().trace_gate_for_tests(false).rows_for_tests(false),
    );
    assert_eq!(ids(&on), ids(&off), "same trace found either way");
    assert_eq!(on.traces.len(), 1);
    // 1099 non-matching candidates: gate-off burns the assembled
    // ceiling proving nothing else outranks; gate-on PRUNES them via
    // high-tier ref resolution and proves the answer complete — the
    // resolution path under test is what makes the prunes possible.
    assert_eq!(on.status, QueryStatus::Complete);
    assert!(off.status.has(PartialReason::WorkCeiling));
}

// ── The differential sweep ───────────────────────────────────────────

/// Seeded pseudo-random corpora × trace-level predicates and token
/// queries: every gate mode — the single-source proof on or off (the
/// all-file path), rollup or span discovery — answers exactly as no
/// gate at all (results AND status) on tie-free, corruption-free
/// inputs. Span-level-only predicates ride along to pin the
/// not-engaged path.
#[test]
fn differential_gate_on_off_sweep() {
    // A tiny deterministic LCG — no external randomness, stable seeds.
    struct Lcg(u64);
    impl Lcg {
        fn next(&mut self, bound: u64) -> u64 {
            self.0 = self
                .0
                .wrapping_mul(6364136223846793005)
                .wrapping_add(1442695040888963407);
            (self.0 >> 33) % bound
        }
    }
    let services = ["ad", "cart", "checkout", "front"];
    let names = ["get", "post", "sync", "flush"];
    for seed in 0..4u64 {
        let mut rng = Lcg(seed.wrapping_mul(0x9E3779B97F4A7C15).wrapping_add(1));
        let dir = tempfile::tempdir().unwrap();
        // 24 traces, 1-3 spans each, distinct starts (tie-free), rooted.
        // Each lives in one sealed file by parity, except: every fifth
        // lives in a WAL tail; every seventh keeps its root sealed and its
        // children (service `db`) in the tail; every third (of the rest)
        // puts its children (service `db`) in the other sealed file.
        let mut reqs_a: Vec<ExportTraceServiceRequest> = Vec::new();
        let mut reqs_b: Vec<ExportTraceServiceRequest> = Vec::new();
        let mut reqs_tail: Vec<ExportTraceServiceRequest> = Vec::new();
        for t in 1..=24u8 {
            let svc = services[rng.next(4) as usize];
            let name: &'static str = names[rng.next(4) as usize];
            let start = u64::from(t) * 10 * NS + rng.next(1000);
            let dur = (rng.next(20) + 1) * NS / 2;
            let split = t % 5 != 0 && (t % 7 == 0 || t % 3 == 0);
            let children = if split { rng.next(2) + 1 } else { rng.next(3) };
            let root = root_span(t, 0x10, start, start + dur, name);
            let children: Vec<SpanSpec> = (0..children)
                .map(|extra| SpanSpec {
                    end: start + dur / 2,
                    ..span_in(t, 0x20 + extra as u8, 0x10, start + 1 + extra * 7, "child")
                })
                .collect();
            let req = |svc: &str, spans: &[SpanSpec]| {
                req_with(vec![kv_str("service.name", svc)], None, spans)
            };
            let (home, away) = if t % 2 == 0 {
                (&mut reqs_a, &mut reqs_b)
            } else {
                (&mut reqs_b, &mut reqs_a)
            };
            if t % 5 == 0 {
                let mut spans = vec![root];
                spans.extend(children);
                reqs_tail.push(req(svc, &spans));
            } else if t % 7 == 0 {
                home.push(req(svc, &[root]));
                reqs_tail.push(req("db", &children));
            } else if t % 3 == 0 {
                home.push(req(svc, &[root]));
                away.push(req("db", &children));
            } else {
                let mut spans = vec![root];
                spans.extend(children);
                home.push(req(svc, &spans));
            }
        }
        let wal_a = write_wal(dir.path(), reqs_a, "diff-a");
        let wal_b = write_wal(dir.path(), reqs_b, "diff-b");
        let wal_tail = write_wal(dir.path(), reqs_tail, "diff-tail");
        let sources = || {
            both_roles(|| {
                vec![
                    sealed_source(dir.path(), &wal_a, "a"),
                    sealed_source(dir.path(), &wal_b, "b"),
                    tail_source(&wal_tail, "t"),
                ]
            })
        };
        let queries: Vec<TokenQuery> = vec![
            pred(vec![root_service_eq("ad")]).into(),
            pred(vec![root_service_eq("nosuch")]).into(),
            pred(vec![builtin(
                BuiltinField::RootName,
                CompareOp::Regex,
                vec![text("(get|post)")],
            )])
            .into(),
            pred(vec![min_duration(5 * NS as i64)]).into(),
            pred(vec![min_duration(NS as i64), root_service_eq("cart")]).into(),
            pred(vec![builtin(
                BuiltinField::TraceDuration,
                CompareOp::Lt,
                vec![PredicateValue::Integer(3 * NS as i64)],
            )])
            .into(),
            // Span-level-only: the gate is never engaged.
            pred(vec![builtin(
                BuiltinField::Name,
                CompareOp::Eq,
                vec![text("child")],
            )])
            .into(),
            // Two positive span-local tokens: the token gate.
            tokens(vec![
                Token::positive(pred(vec![name_eq("child")])),
                Token::positive(pred(vec![service_eq("cart")])),
            ]),
            tokens(vec![
                Token::positive(pred(vec![name_eq("child")])),
                Token::positive(pred(vec![name_eq("get")])),
                Token::negated(pred(vec![service_eq("ad")])),
            ]),
            // Tokens met in different holders (files, the tail).
            tokens(vec![
                Token::positive(pred(vec![service_eq("db")])),
                Token::positive(pred(vec![service_eq("cart")])),
            ]),
            tokens(vec![
                Token::positive(pred(vec![service_eq("front")])),
                Token::negated(pred(vec![service_eq("db")])),
            ]),
            // Rollup discovery with a negated span chip.
            tokens(vec![
                Token::positive(pred(vec![min_duration(NS as i64)])),
                Token::negated(pred(vec![name_eq("child")])),
            ]),
        ];
        for (qi, query) in queries.into_iter().enumerate() {
            let q = || SearchQuery::new(query.clone()).limit(30);
            // Both paths: rows (decided first) and assembly with the gates
            // in every mode; on this resend-free, rooted corpus they agree.
            let mut by_path = Vec::new();
            for rows in [true, false] {
                // No gate at all: the trace-level gate off and no proof
                // (the token gate needs it).
                let reference = norm(&run(
                    sources(),
                    q().rows_for_tests(rows)
                        .trace_gate_for_tests(false)
                        .single_source_proof_ids_for_tests(0),
                ));
                for (mode, (proof, rollup)) in
                    [(true, true), (false, true), (true, false), (false, false)]
                        .into_iter()
                        .enumerate()
                {
                    let mut query = q().rows_for_tests(rows).rollup_discovery_for_tests(rollup);
                    if !proof {
                        query = query.single_source_proof_ids_for_tests(0);
                    }
                    assert_eq!(
                        norm(&run(sources(), query)),
                        reference,
                        "seed {seed} query {qi} rows {rows} mode {mode} (proof {proof}, rollup {rollup})"
                    );
                }
                by_path.push(reference);
            }
            assert_eq!(
                by_path[0], by_path[1],
                "seed {seed} query {qi}: rows vs assembly"
            );
        }
    }
}

// ── The token gate for multi-file and tail candidates ───────────────

/// Two-token query (`op` AND `db`) driven by `op`, so discovery emits
/// what `op` matches.
fn op_and_db() -> SearchQuery {
    SearchQuery::new(tokens(vec![
        Token::positive(pred(vec![name_eq("op")])),
        Token::positive(pred(vec![name_eq("db")])),
    ]))
    .driver_for_tests(0)
}

/// Multi-file candidates that cannot match need no assembly: their
/// in-window rows in every window holder miss a token. With room for
/// one assembly, two such candidates complete (pruned) instead of
/// ending the search at the ceiling.
#[test]
fn zero_hit_straddlers_need_no_assembly() {
    let dir = tempfile::tempdir().unwrap();
    let svc = || vec![kv_str("service.name", "svc")];
    let half = |at: u64| {
        req_with(
            svc(),
            None,
            &[
                root_span(1, 0x11, at * NS, at * NS + 50, "op"),
                root_span(2, 0x21, (at + 1) * NS, (at + 1) * NS + 50, "op"),
            ],
        )
    };
    let wal_a = write_wal(dir.path(), vec![half(100)], "straddle-a");
    let wal_b = write_wal(dir.path(), vec![half(200)], "straddle-b");
    // A third holder whose rows all start after the query window: in
    // the window source set, but it holds no in-window span to probe.
    let wal_c = write_wal(dir.path(), vec![half(500)], "straddle-c");
    let sources = || {
        both_roles(|| {
            vec![
                sealed_source(dir.path(), &wal_a, "a"),
                sealed_source(dir.path(), &wal_b, "b"),
                sealed_source(dir.path(), &wal_c, "c"),
            ]
        })
    };
    let window = sfsq::traces::TimeWindow::new(100 * NS as i64, 300 * NS as i64).unwrap();
    // Both paths: the rows path decides them; without it the token gate
    // prunes them (the path of files sealed before TRSS).
    for rows in [true, false] {
        let data = run(
            sources(),
            op_and_db()
                .window(window)
                .assembled_ceiling_for_tests(1)
                .rows_for_tests(rows),
        );
        assert_eq!(
            data.status,
            QueryStatus::Complete,
            "rows {rows}: {:?}",
            data.status
        );
        assert!(data.traces.is_empty(), "rows {rows}");
    }
}

/// A window holder not compiled yet compiles when a multi-file
/// candidate needs it and then discovers like any other file: the
/// answer equals the no-gate one, including the holder's own trace.
#[test]
fn on_demand_compile_equals_gate_off() {
    let dir = tempfile::tempdir().unwrap();
    let svc = || vec![kv_str("service.name", "svc")];
    // X: `op` in the newest file, `db` in the older one. V: both in the
    // older file. W: `op` alone in the newest file.
    let newest = req_with(
        svc(),
        None,
        &[
            root_span(1, 0x11, 200 * NS, 200 * NS + 50, "op"),
            root_span(3, 0x31, 150 * NS, 150 * NS + 50, "op"),
        ],
    );
    let older = req_with(
        svc(),
        None,
        &[
            span_in(1, 0x12, 0x11, 100 * NS, "db"),
            root_span(2, 0x21, 90 * NS, 90 * NS + 50, "op"),
            span_in(2, 0x22, 0x21, 91 * NS, "db"),
        ],
    );
    let wal_new = write_wal(dir.path(), vec![newest], "ondemand-new");
    let wal_old = write_wal(dir.path(), vec![older], "ondemand-old");
    let sources = || {
        both_roles(|| {
            vec![
                sealed_source(dir.path(), &wal_new, "new"),
                sealed_source(dir.path(), &wal_old, "old"),
            ]
        })
    };
    for rows in [true, false] {
        let q = || op_and_db().limit(2).rows_for_tests(rows);
        let on = run(sources(), q());
        assert_eq!(ids(&on), vec![hex(1), hex(2)], "rows {rows}");
        assert_eq!(
            norm(&on),
            norm(&run(sources(), q().trace_gate_for_tests(false))),
            "rows {rows}"
        );
    }
}

/// A tail-only candidate missing a token is pruned from the tail's own
/// rows, without assembly.
#[test]
fn tail_held_non_match_is_pruned() {
    let dir = tempfile::tempdir().unwrap();
    let svc = vec![kv_str("service.name", "svc")];
    let reqs = vec![req_with(
        svc,
        None,
        &[
            root_span(1, 0x11, 100 * NS, 100 * NS + 50, "op"),
            root_span(2, 0x21, 101 * NS, 101 * NS + 50, "op"),
        ],
    )];
    let wal = write_wal(dir.path(), reqs, "tail-prune");
    let sources = || both_roles(|| vec![tail_source(&wal, "t")]);
    for rows in [true, false] {
        let data = run(
            sources(),
            op_and_db()
                .assembled_ceiling_for_tests(1)
                .rows_for_tests(rows),
        );
        assert_eq!(
            data.status,
            QueryStatus::Complete,
            "rows {rows}: {:?}",
            data.status
        );
        assert!(data.traces.is_empty(), "rows {rows}");
    }
}

// ── The single-source proof and the token gate ──────────────────────

fn name_eq(name: &str) -> Condition {
    builtin(BuiltinField::Name, CompareOp::Eq, vec![text(name)])
}

fn service_eq(service: &str) -> Condition {
    Condition {
        target: PredicateTarget::Attribute(AttributeOwner::Resource, "service.name".into()),
        op: CompareOp::Eq,
        values: vec![text(service)],
    }
}

fn tokens(tokens: Vec<Token>) -> TokenQuery {
    TokenQuery { tokens }
}

/// Twelve traces over three sealed files and a tail: single-file traces,
/// traces straddling two files (their merged envelope reaches the
/// duration bound, a file's alone does not), and traces with tail spans.
fn mixed_corpus(dir: &Path) -> impl Fn() -> SearchSources + '_ {
    let svc = |s: &str| vec![kv_str("service.name", s)];
    let mut files: [Vec<ExportTraceServiceRequest>; 4] = Default::default();
    for t in 1..=12u8 {
        let start = 100 * NS + u64::from(t) * NS;
        let service = if t % 2 == 0 { "svc-a" } else { "svc-b" };
        let home = usize::from(t % 3);
        let root = root_span(t, 0x10, start, start + 1_000_000, "root-op");
        files[home].push(req_with(svc(service), None, &[root]));
        let child_name = if t % 4 == 0 { "db" } else { "cache" };
        let child = SpanSpec {
            end: start + 8 * NS,
            ..span_in(t, 0x11, 0x10, start + 7 * NS, child_name)
        };
        // Traces 1-4 straddle into the next file, 5-6 into the tail,
        // the rest stay home.
        let away = match t {
            1..=4 => (home + 1) % 3,
            5 | 6 => 3,
            _ => home,
        };
        files[away].push(req_with(svc(service), None, &[child]));
    }
    let [a, b, c, tail] = files;
    let wals = [
        write_wal(dir, a, "mixed-a"),
        write_wal(dir, b, "mixed-b"),
        write_wal(dir, c, "mixed-c"),
        write_wal(dir, tail, "mixed-tail"),
    ];
    move || {
        both_roles(|| {
            vec![
                sealed_source(dir, &wals[0], "a"),
                sealed_source(dir, &wals[1], "b"),
                sealed_source(dir, &wals[2], "c"),
                tail_source(&wals[3], "tail"),
            ]
        })
    }
}

/// The proof only removes work: with it, without it, and with every gate
/// off, the answers are identical — trace-level selections, duration
/// bounds that only straddling or tail-extended traces reach, and
/// multi-token queries the token gate prunes.
#[test]
fn single_file_proof_on_off_identity() {
    let dir = tempfile::tempdir().unwrap();
    let sources = mixed_corpus(dir.path());
    let queries: Vec<(SearchQuery, usize)> = vec![
        (SearchQuery::new(pred(vec![root_service_eq("svc-a")])), 6),
        (
            SearchQuery::new(pred(vec![min_duration(5 * NS as i64)])),
            12,
        ),
        (
            SearchQuery::new(pred(vec![
                root_service_eq("svc-b"),
                min_duration(5 * NS as i64),
            ])),
            6,
        ),
        (
            SearchQuery::new(tokens(vec![
                Token::positive(pred(vec![name_eq("root-op")])),
                Token::positive(pred(vec![name_eq("db")])),
            ]))
            .driver_for_tests(0),
            3,
        ),
        (
            SearchQuery::new(tokens(vec![
                Token::positive(pred(vec![service_eq("svc-a")])),
                Token::positive(pred(vec![name_eq("cache")])),
                Token::negated(pred(vec![name_eq("db")])),
            ])),
            3,
        ),
    ];
    for (i, (q, count)) in queries.into_iter().enumerate() {
        let q = q.limit(20);
        let on = run(sources(), q.clone());
        let no_proof = run(sources(), q.clone().single_source_proof_ids_for_tests(0));
        // A budget that ends after the first file's ids turns the proof
        // off; it is never used cut short.
        let cut = run(sources(), q.clone().single_source_proof_ids_for_tests(4));
        assert_eq!(norm(&on), norm(&cut), "query {i}: proof budget passed");
        let off = run(sources(), q.trace_gate_for_tests(false));
        assert_eq!(on.traces.len(), count, "query {i}");
        assert_eq!(on.status, QueryStatus::Complete, "query {i}");
        assert_eq!(norm(&on), norm(&no_proof), "query {i}: proof off");
        assert_eq!(norm(&on), norm(&off), "query {i}: gates off");
    }
}

/// A trace whose newest span sits in a file with a rollup (so that file
/// emits the candidate) and whose older span sits in a file without one:
/// the proof cannot see the second file's ids, so it is off for the
/// query — read alone, the first file's envelope would prune the trace.
#[test]
fn file_without_rollup_disables_the_proof() {
    let dir = tempfile::tempdir().unwrap();
    let svc = vec![kv_str("service.name", "svc-a")];
    let a = req_with(
        svc.clone(),
        None,
        &[root_span(
            1,
            0x11,
            100 * NS,
            100 * NS + 1_000_000,
            "root-op",
        )],
    );
    let b = req_with(
        svc,
        None,
        &[SpanSpec {
            end: 91 * NS,
            ..span_in(1, 0x12, 0x11, 90 * NS, "early-op")
        }],
    );
    let wal_a = write_wal(dir.path(), vec![a], "norollup-a");
    let wal_b = write_wal(dir.path(), vec![b], "norollup-b");
    drop(sealed_source(dir.path(), &wal_a, "a"));
    drop(sealed_source(dir.path(), &wal_b, "b"));
    hide_chunk(&sealed_path(dir.path(), "b"), *b"TRSU");
    let sources = || {
        both_roles(|| {
            vec![
                sealed_source_at(&sealed_path(dir.path(), "a"), "a"),
                sealed_source_at(&sealed_path(dir.path(), "b"), "b"),
            ]
        })
    };
    let q = || SearchQuery::new(pred(vec![min_duration(5 * NS as i64)]));
    let on = run(sources(), q());
    assert_eq!(ids(&on), vec![hex(1)]);
    assert_eq!(
        norm(&on),
        norm(&run(sources(), q().trace_gate_for_tests(false)))
    );
}

/// The token gate's incident shape: many candidates match the driver
/// token, none the other — gate-off burns the assembled ceiling, the
/// gate prunes each from its single file's rows and proves the empty
/// answer. A trace matching both still returns.
#[test]
fn token_gate_completes_instead_of_burning_the_ceiling() {
    let dir = tempfile::tempdir().unwrap();
    let hot = vec![kv_str("service.name", "hot")];
    let mut reqs: Vec<ExportTraceServiceRequest> = (0..70u8)
        .map(|i| {
            let start = u64::from(i + 1) * NS;
            req_with(
                hot.clone(),
                None,
                &[root_span(i + 1, 0x10, start, start + 50, "op")],
            )
        })
        .collect();
    let wal = write_wal(dir.path(), reqs.clone(), "token-incident");
    let sources = || both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let q = || {
        SearchQuery::new(tokens(vec![
            Token::positive(pred(vec![service_eq("hot")])),
            Token::positive(pred(vec![name_eq("rare")])),
        ]))
        .driver_for_tests(0)
        .limit(1)
    };
    let off = run(
        sources(),
        q().trace_gate_for_tests(false).rows_for_tests(false),
    );
    assert!(off.traces.is_empty());
    assert!(
        off.status.has(PartialReason::WorkCeiling),
        "{:?}",
        off.status
    );
    let on = run(sources(), q());
    assert!(on.traces.is_empty());
    assert_eq!(on.status, QueryStatus::Complete);

    // Trace 30 gains a `rare` child: it returns, gate on and off.
    reqs.push(req_with(
        hot,
        None,
        &[span_in(30, 0x11, 0x10, 30 * NS + 10, "rare")],
    ));
    let wal = write_wal(dir.path(), reqs, "token-hit");
    let sources = || both_roles(|| vec![sealed_source(dir.path(), &wal, "hit")]);
    let on = run(sources(), q());
    assert_eq!(ids(&on), vec![hex(30)]);
    assert_eq!(on.status, QueryStatus::Complete);
}

/// The gates' reads count toward the visited ceiling: a budget the
/// gate-off search fits in is too small once the proof and the checks
/// are charged — the search ends `WorkCeiling` instead of doing
/// uncounted work.
#[test]
fn gate_probes_hit_the_ceiling() {
    let dir = tempfile::tempdir().unwrap();
    let hot = vec![kv_str("service.name", "hot")];
    let reqs: Vec<ExportTraceServiceRequest> = (0..70u8)
        .map(|i| {
            let start = u64::from(i + 1) * NS;
            req_with(
                hot.clone(),
                None,
                &[root_span(i + 1, 0x10, start, start + 50, "op")],
            )
        })
        .collect();
    let wal = write_wal(dir.path(), reqs, "charged");
    let sources = || both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let q = || {
        SearchQuery::new(pred(vec![root_service_eq("cold")]))
            .limit(1)
            .visited_rows_ceiling_for_tests(100)
    };
    // Without the gates: discovery alone fits, and the assembled
    // ceiling ends the discards.
    let off = run(sources(), q().trace_gate_for_tests(false));
    assert!(off.traces.is_empty());
    // A rollup row per check with the proof, a screen and a row per
    // check without it: both exceed the budget.
    for q in [q(), q().single_source_proof_ids_for_tests(0)] {
        let on = run(sources(), q);
        assert!(on.traces.is_empty());
        assert!(on.status.has(PartialReason::WorkCeiling), "{:?}", on.status);
    }
    // With the default ceiling the gate proves the empty answer.
    let on = run(
        sources(),
        SearchQuery::new(pred(vec![root_service_eq("cold")])).limit(1),
    );
    assert_eq!(on.status, QueryStatus::Complete);
}

// ── Rollup discovery ─────────────────────────────────────────────────

/// Trace-level-only queries discover from rollup rows; span discovery
/// (the toggle) and the gates off give the same answers at every limit,
/// with straddling and tail-extended traces, a negated span-local token
/// and a window.
#[test]
fn rollup_and_span_discovery_agree() {
    let dir = tempfile::tempdir().unwrap();
    let sources = mixed_corpus(dir.path());
    let window = sfsq::traces::TimeWindow::new(104 * NS as i64, 110 * NS as i64).unwrap();
    let queries: Vec<SearchQuery> = vec![
        SearchQuery::new(pred(vec![min_duration(5 * NS as i64)])),
        SearchQuery::new(pred(vec![root_service_eq("svc-a")])),
        SearchQuery::new(pred(vec![
            root_service_eq("svc-b"),
            min_duration(5 * NS as i64),
        ])),
        SearchQuery::new(tokens(vec![
            Token::positive(pred(vec![min_duration(5 * NS as i64)])),
            Token::negated(pred(vec![name_eq("db")])),
        ])),
        SearchQuery::new(pred(vec![min_duration(5 * NS as i64)])).window(window),
    ];
    for (i, q) in queries.into_iter().enumerate() {
        for limit in [1, 3, 20] {
            let q = q.clone().limit(limit);
            let rollup = run(sources(), q.clone());
            let spans = run(sources(), q.clone().rollup_discovery_for_tests(false));
            let off = run(sources(), q.trace_gate_for_tests(false));
            assert!(!rollup.traces.is_empty(), "query {i}, limit {limit}");
            assert_eq!(
                rollup.status,
                QueryStatus::Complete,
                "query {i}, limit {limit}"
            );
            assert_eq!(
                norm(&rollup),
                norm(&spans),
                "query {i}, limit {limit}: spans"
            );
            assert_eq!(
                norm(&rollup),
                norm(&off),
                "query {i}, limit {limit}: gates off"
            );
        }
    }
}

/// A rollup row ranks by its envelope end, which can lie far after the
/// trace's newest span start: an early long span must not outrank a
/// later short one in the answer (the bound only orders the frontier),
/// and a trace whose newest span starts late must rank by it, not by
/// its first start.
#[test]
fn long_span_rank_bound_is_sound() {
    let dir = tempfile::tempdir().unwrap();
    let svc = vec![kv_str("service.name", "svc-a")];
    // Trace 1 starts at 100 s and runs 50 s; trace 2 starts at 140 s
    // and runs 10 s. Both envelopes end at 150 s; trace 2 is newer.
    // Trace 3 starts at 120 s, but its child starts at 145 s, the
    // newest start of all.
    let a = req_with(
        svc.clone(),
        None,
        &[
            root_span(1, 0x11, 100 * NS, 150 * NS, "long"),
            root_span(3, 0x31, 120 * NS, 121 * NS, "early-root"),
            SpanSpec {
                end: 146 * NS,
                ..span_in(3, 0x32, 0x31, 145 * NS, "late-child")
            },
        ],
    );
    let b = req_with(
        svc,
        None,
        &[root_span(2, 0x21, 140 * NS, 150 * NS, "short")],
    );
    let wal_a = write_wal(dir.path(), vec![a], "long-a");
    let wal_b = write_wal(dir.path(), vec![b], "short-b");
    let sources = || {
        both_roles(|| {
            vec![
                sealed_source(dir.path(), &wal_a, "a"),
                sealed_source(dir.path(), &wal_b, "b"),
            ]
        })
    };
    let q = |limit| SearchQuery::new(pred(vec![min_duration(5 * NS as i64)])).limit(limit);
    assert_eq!(ids(&run(sources(), q(1))), vec![hex(3)]);
    assert_eq!(ids(&run(sources(), q(2))), vec![hex(3), hex(2)]);
    assert_eq!(ids(&run(sources(), q(3))), vec![hex(3), hex(2), hex(1)]);
    assert_eq!(
        norm(&run(sources(), q(3))),
        norm(&run(sources(), q(3).trace_gate_for_tests(false)))
    );
}

/// Rollup discovery visits one row per trace and file, not one per span:
/// a span-heavy corpus that span discovery cannot finish inside the
/// visited ceiling completes; a tiny ceiling still ends `WorkCeiling`.
#[test]
fn rollup_discovery_ceiling() {
    let dir = tempfile::tempdir().unwrap();
    let svc = vec![kv_str("service.name", "svc-a")];
    // 40 traces of 40 spans each; every tenth lasts 10 s.
    let reqs: Vec<ExportTraceServiceRequest> = (1..=40u8)
        .map(|t| {
            let start = u64::from(t) * 100 * NS;
            let end = if t % 10 == 0 {
                start + 10 * NS
            } else {
                start + NS
            };
            let mut spans = vec![root_span(t, 1, start, end, "root")];
            for c in 2..=40u8 {
                spans.push(span_in(t, c, 1, start + u64::from(c), "child"));
            }
            req_with(svc.clone(), None, &spans)
        })
        .collect();
    let wal = write_wal(dir.path(), reqs, "span-heavy");
    let sources = || both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let q = || {
        SearchQuery::new(pred(vec![min_duration(5 * NS as i64)]))
            .limit(10)
            .visited_rows_ceiling_for_tests(400)
    };
    let rollup = run(sources(), q());
    assert_eq!(ids(&rollup), vec![hex(40), hex(30), hex(20), hex(10)]);
    assert_eq!(rollup.status, QueryStatus::Complete);
    let spans = run(sources(), q().rollup_discovery_for_tests(false));
    assert!(
        spans.status.has(PartialReason::WorkCeiling),
        "{:?}",
        spans.status
    );
    let tiny = run(sources(), q().visited_rows_ceiling_for_tests(5));
    assert!(
        tiny.status.has(PartialReason::WorkCeiling),
        "{:?}",
        tiny.status
    );
}

/// Rollup discovery prunes single-file short traces from their rollup
/// rows; a negated span chip must not undo that. A multi-file
/// candidate's token evidence compiles the window files it needs for
/// evidence alone — never as span-discovery files, whose every
/// in-window trace would become a candidate that the duration then
/// rejects one by one.
#[test]
fn negated_chip_keeps_rollup_discovery() {
    let dir = tempfile::tempdir().unwrap();
    let svc = || vec![kv_str("service.name", "svc")];
    let files: Vec<_> = (0..6u8)
        .map(|f| {
            let base = u64::from(f) * 100;
            let mut spans: Vec<SpanSpec> = (0..30u8)
                .map(|j| {
                    let at = (base + u64::from(j)) * NS;
                    root_span(2 + f * 30 + j, 0x10, at, at + 50, "op")
                })
                .collect();
            // Trace 1 has a span in every file: 500 s long.
            spans.push(if f == 0 {
                root_span(1, 0x11, 50 * NS, 50 * NS + 50, "op")
            } else {
                span_in(1, 0x11 + f, 0x11, (base + 50) * NS, "op")
            });
            let wal = write_wal(
                dir.path(),
                vec![req_with(svc(), None, &spans)],
                &format!("f{f}"),
            );
            (wal, format!("f{f}"))
        })
        .collect();
    let sources = || {
        both_roles(|| {
            files
                .iter()
                .map(|(wal, id)| sealed_source(dir.path(), wal, id))
                .collect()
        })
    };
    let dur = || Token::positive(pred(vec![min_duration(10 * NS as i64)]));
    let not_zzz = || Token::negated(pred(vec![name_eq("zzz")]));
    // Measured: about 250 rows of work without the flood, 1075 with it
    // (each short trace popped and decided); at most 2 assemblies keep
    // the rows path's breach from finishing the flood by assembly.
    for (name, toks) in [
        ("duration", vec![dur()]),
        ("duration + not", vec![dur(), not_zzz()]),
    ] {
        let q = SearchQuery::new(tokens(toks))
            .visited_rows_ceiling_for_tests(500)
            .assembled_ceiling_for_tests(2);
        let data = run(sources(), q);
        assert_eq!(data.status, QueryStatus::Complete, "{name}");
        assert_eq!(ids(&data), vec![hex(1)], "{name}");
    }
}

/// A window file whose evidence read fails is reported once and not
/// read again: later candidates it may hold fall back without a retry
/// (each retry re-decoded its dictionary and warned again).
#[test]
fn failed_evidence_read_is_reported_once() {
    use std::io::Write;
    use std::sync::Mutex;

    #[derive(Clone, Default)]
    struct Lines(Arc<Mutex<Vec<u8>>>);
    impl Write for Lines {
        fn write(&mut self, buf: &[u8]) -> std::io::Result<usize> {
            self.0.lock().unwrap().extend_from_slice(buf);
            Ok(buf.len())
        }
        fn flush(&mut self) -> std::io::Result<()> {
            Ok(())
        }
    }

    let dir = tempfile::tempdir().unwrap();
    let svc = || vec![kv_str("service.name", "svc")];
    // Five long traces, each with a span in both files; the older
    // file's dictionary is corrupt.
    let file = |f: u8| {
        let spans: Vec<SpanSpec> = (1..=5u8)
            .map(|t| {
                let at = (u64::from(f) * 100 + u64::from(t)) * NS;
                if f == 0 {
                    root_span(t, 0x10, at, at + 50, "op")
                } else {
                    span_in(t, 0x20, 0x10, at, "op")
                }
            })
            .collect();
        write_wal(
            dir.path(),
            vec![req_with(svc(), None, &spans)],
            &format!("e{f}"),
        )
    };
    let (old, new) = (file(0), file(1));
    drop(sealed_source(dir.path(), &old, "e0"));
    corrupt_chunk(&sealed_path(dir.path(), "e0"), *b"PRIM");
    let sources = both_roles(|| {
        vec![
            sealed_source_at(&sealed_path(dir.path(), "e0"), "e0"),
            sealed_source(dir.path(), &new, "e1"),
        ]
    });
    let query = SearchQuery::new(tokens(vec![
        Token::positive(pred(vec![min_duration(10 * NS as i64)])),
        Token::negated(pred(vec![name_eq("zzz")])),
    ]));
    let lines = Lines::default();
    let writer = lines.clone();
    let subscriber = tracing_subscriber::fmt()
        .with_writer(move || writer.clone())
        .with_ansi(false)
        .finish();
    let data = tracing::subscriber::with_default(subscriber, || run(sources, query));
    assert!(
        data.status.has(PartialReason::SourceFailure),
        "{:?}",
        data.status
    );
    let log = String::from_utf8(lines.0.lock().unwrap().clone()).unwrap();
    assert_eq!(
        log.matches("failed a token evidence read").count(),
        1,
        "{log}"
    );
}
