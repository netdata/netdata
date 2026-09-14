//! The trace-level pre-assembly gate acceptance suite (phase 9).
//!
//! Pins the gate's three contracts:
//!
//! - **Superset:** with the gate on, every query answers byte-identically
//!   to the gate-off truth path on healthy, tie-free corpora — the gate
//!   only removes work, never results. The known divergence is corrupt
//!   corpora, where skip-and-surface is the DESIGNED difference.
//! - **The incident fix:** a rare trace-level predicate no longer burns
//!   the assembled ceiling on discards — the gate-off `WorkCeiling`
//!   partial becomes a gate-on `Complete`.
//! - **Skip-and-surface:** a file that proves itself corrupt through any
//!   gate read (TBLM, TRSU) is skipped as a failed source AND surfaced
//!   (`SourceFailure`), never silently downgraded.

mod common;

use std::path::Path;
use std::sync::Arc;
use std::sync::atomic::AtomicUsize;

use tokio_util::sync::CancellationToken;

use common::{
    SpanSpec, kv_str, req_with, sealed_source, sealed_source_at, sp, tail_source, write_wal,
};
use opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest;
use sfsq::traces::{
    BuiltinField, CompareOp, Condition, PartialReason, Predicate, PredicateTarget,
    PredicateValue, QueryStatus, SearchData, SearchQuery, SearchSources, TraceSource, search,
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

fn run(sources: SearchSources, query: SearchQuery) -> SearchData {
    search(
        sources,
        query,
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
    .expect("valid request")
}

fn both_roles(build: impl Fn() -> Vec<TraceSource>) -> SearchSources {
    SearchSources {
        window: build(),
        completion: build(),
    }
}

fn ids(data: &SearchData) -> Vec<String> {
    data.traces.iter().map(|t| t.trace_id.to_string()).collect()
}

fn hex(n: u8) -> String {
    sfst::TraceId::from(tid(n)).to_string()
}

/// One summary rendered comparable across gate settings — trace-level
/// values AND span-level attachments, so the differential sweep also
/// catches a gate-induced change in which spans ride along.
type NormRow = (
    String,
    Option<String>,
    Option<String>,
    i64,
    i64,
    usize,
    usize,
    usize,
    Vec<(i64, i64)>,
    bool,
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
                    t.matched_count,
                    t.matched_spans
                        .iter()
                        .map(|s| (s.start_ns, s.duration_ns))
                        .collect(),
                    t.exact,
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
    builtin(BuiltinField::RootServiceName, CompareOp::Eq, vec![text(value)])
}

fn min_duration(ns: i64) -> Condition {
    builtin(
        BuiltinField::TraceDuration,
        CompareOp::Gte,
        vec![PredicateValue::Integer(ns)],
    )
}

/// Flip the first payload byte of chunk `id` inside a sealed SFST — a
/// CRC mismatch, the cheapest "corrupt in any way".
fn corrupt_chunk(path: &Path, id: [u8; 4]) {
    let mut bytes = std::fs::read(path).unwrap();
    let offset = {
        let container = chunk_file::container::Container::open(&bytes, b"SFST", 1).unwrap();
        let meta = container.chunk_meta(id).expect("chunk present");
        usize::try_from(meta.offset).unwrap()
    };
    bytes[offset] ^= 0xFF;
    std::fs::write(path, &bytes).unwrap();
}

/// Make chunk `id` ABSENT from a sealed SFST by renaming its TOC entry
/// to an id no reader asks for — the "older file without this chunk"
/// shape, built from a real modern seal (payload bytes untouched).
fn hide_chunk(path: &Path, id: [u8; 4]) {
    let mut bytes = std::fs::read(path).unwrap();
    let num_chunks =
        u32::from_le_bytes(bytes[8..12].try_into().unwrap()) as usize;
    let toc_start = chunk_file::container::HEADER_SIZE;
    let entry_size = 12; // ChunkId(4) + offset u64 LE
    let mut hidden = false;
    for i in 0..num_chunks {
        let at = toc_start + i * entry_size;
        if bytes[at..at + 4] == id {
            // Unique per hidden chunk (the TOC rejects duplicate ids).
            bytes[at] = b'Z';
            hidden = true;
            break;
        }
    }
    assert!(hidden, "chunk {:?} not found in TOC", id);
    std::fs::write(path, &bytes).unwrap();
}

/// The path a [`sealed_source`] wrote (its naming contract).
fn sealed_path(dir: &Path, id: &str) -> std::path::PathBuf {
    dir.join(format!("{id}.sfst"))
}

// ── The incident shape ───────────────────────────────────────────────

/// Many candidates, none matching a rare root selection: gate-off burns
/// the assembled ceiling on discards (`Partial{WorkCeiling}`); gate-on
/// prunes every candidate without assembling and PROVES the empty
/// answer (`Complete`) — the phase-9 fix, pinned.
#[test]
fn rare_root_selection_completes_instead_of_burning_the_ceiling() {
    let dir = tempfile::tempdir().unwrap();
    let hot = vec![kv_str("service.name", "hot")];
    // 70 single-span traces > the 64-assembly floor at limit 1.
    let reqs: Vec<ExportTraceServiceRequest> = (0..70u8)
        .map(|i| {
            req_with(hot.clone(), None, &[root_span(
                i + 1,
                0x10,
                u64::from(i + 1) * NS,
                u64::from(i + 1) * NS + 50,
                "op",
            )])
        })
        .collect();
    let wal = write_wal(dir.path(), reqs, "incident");
    let q = || SearchQuery::new(pred(vec![root_service_eq("cold")])).limit(1);

    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let off = run(sources, q().trace_gate_for_tests(false));
    assert!(off.traces.is_empty());
    assert!(
        off.status.has(PartialReason::WorkCeiling),
        "gate-off burns the ceiling: {:?}",
        off.status
    );

    let sources = both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let on = run(sources, q());
    assert!(on.traces.is_empty());
    assert_eq!(on.status, QueryStatus::Complete, "gate-on proves the empty answer");

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
    let a = req_with(svc.clone(), None, &[root_span(
        1,
        0x11,
        100 * NS,
        100 * NS + 1_000_000,
        "root-op",
    )]);
    let b = req_with(svc.clone(), None, &[SpanSpec {
        end: 110 * NS,
        ..span_in(1, 0x12, 0x11, 109 * NS, "child-op")
    }]);
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
}

/// Tail spans are invisible to the gate's evidence: a candidate with
/// ANY tail presence is unprunable — here the tail extends a duration
/// past the bound that the sealed envelope alone would fail.
#[test]
fn tail_presence_blocks_pruning() {
    let dir = tempfile::tempdir().unwrap();
    let svc = vec![kv_str("service.name", "svc-a")];
    let sealed_req = req_with(svc.clone(), None, &[root_span(
        1,
        0x11,
        100 * NS,
        100 * NS + 1_000_000,
        "root-op",
    )]);
    let tail_req = req_with(svc.clone(), None, &[SpanSpec {
        end: 110 * NS,
        ..span_in(1, 0x12, 0x11, 109 * NS, "tail-op")
    }]);
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
        let reqs = vec![req_with(svc, None, &[root_span(
            1,
            0x11,
            100 * NS,
            100 * NS + 50,
            "op",
        )])];
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
        let hit = run(sources(), SearchQuery::new(pred(vec![root_service_eq("hot")])));
        assert_eq!(ids(&hit), vec![hex(1)], "hide_bloom_too={hide_bloom_too}");
    }
}

/// True-root filter semantics (decision 1D): a trace whose root span
/// was never exported matches NO root condition — either polarity —
/// with gate on and off agreeing, and the trace stays findable by
/// span-level conditions. (That the gate reaches this verdict WITHOUT
/// assembling — the `ROOT_CLAIM_NONE` prune — has no public observable
/// yet: `GateStats` is crate-internal. Mutation-checked 2026-08-15:
/// removing the no-root prune leaves this test green because assembly
/// evaluates the same predicate to the same emptiness. The prune-proof
/// assertion lands when gate stats surface for the tail-aggregates /
/// work-ceiling follow-ups.)
#[test]
fn rootless_traces_never_match_root_filters_either_polarity() {
    let dir = tempfile::tempdir().unwrap();
    let svc = vec![kv_str("service.name", "svc-a")];
    // Parent 0xEE never exported: no unset-parent span exists.
    let reqs = vec![req_with(svc, None, &[span_in(
        1,
        0x11,
        0xEE,
        100 * NS,
        "orphan-op",
    )])];
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
            req_with(svc.clone(), None, &[
                span_in(i + 1, 0x10, 0xEE, u64::from(i + 1) * NS, "eval"),
                span_in(i + 1, 0x11, 0x10, u64::from(i + 1) * NS + 5, "resolve"),
            ])
        })
        .collect();
    let wal = write_wal(dir.path(), reqs, "rootless-heavy");
    let sources = || both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let q = || SearchQuery::new(pred(vec![root_service_eq("flagd")])).limit(1);

    let off = run(sources(), q().trace_gate_for_tests(false));
    assert!(off.traces.is_empty());
    assert!(
        off.status.has(PartialReason::WorkCeiling),
        "gate-off burns the ceiling excluding rootless candidates: {:?}",
        off.status
    );

    let on = run(sources(), q());
    assert!(on.traces.is_empty());
    assert_eq!(on.status, QueryStatus::Complete, "the no-root prune proves it");
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
        req_with(cold, None, &[root_span(2, 0x21, 200 * NS, 206 * NS, "op-b")]),
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
        req_with(svc.clone(), None, &[root_span(1, 0x11, 100 * NS, 103 * NS, "op")]),
        req_with(svc.clone(), None, &[root_span(2, 0x21, 200 * NS, 205 * NS, "op")]),
        req_with(svc.clone(), None, &[root_span(3, 0x31, 300 * NS, 320 * NS, "op")]),
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
        req_with(svc.clone(), None, &[root_span(1, 0x11, 100 * NS, 102 * NS, "get-users")]),
        req_with(svc.clone(), None, &[root_span(2, 0x21, 200 * NS, 220 * NS, "post-orders")]),
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
    let a = req_with(vec![kv_str("service.name", "svc-a")], None, &[root_span(
        1, 0x11, 100 * NS, 101 * NS, "op",
    )]);
    let b = req_with(vec![kv_str("service.name", "svc-b")], None, &[root_span(
        1, 0x11, 100 * NS, 101 * NS, "op",
    )]);
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
    let reqs = vec![req_with(svc, None, &[root_span(
        5, 0x51, 100 * NS, 101 * NS, "op",
    )])];
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
    let reqs = vec![req_with(svc, None, &[root_span(
        1, 0x11, 100 * NS, 101 * NS, "op",
    )])];
    let wal = write_wal(dir.path(), reqs, "corrupt-trsu");
    // Seal once, corrupt in place, then build sources WITHOUT resealing.
    drop(sealed_source(dir.path(), &wal, "one"));
    let path = sealed_path(dir.path(), "one");
    corrupt_chunk(&path, *b"TRSU");
    let build = || both_roles(|| vec![sealed_source_at(&path, "one")]);

    let q = || SearchQuery::new(pred(vec![root_service_eq("hot")]));
    let on = run(build(), q());
    assert!(on.traces.is_empty(), "indeterminate candidates are excluded");
    assert!(
        on.status.has(PartialReason::SourceFailure),
        "the skip is surfaced: {:?}",
        on.status
    );

    // Gate-off never reads TRSU: the corruption goes UNDISCOVERED and
    // the trace returns — the designed v1 scope of the principle.
    let off = run(build(), q().trace_gate_for_tests(false));
    assert_eq!(ids(&off), vec![hex(1)]);
    assert_eq!(off.status, QueryStatus::Complete);
}

/// A corrupt TBLM chunk: same principle through the gate's other read.
#[test]
fn corrupt_bloom_chunk_is_skipped_and_surfaced() {
    let dir = tempfile::tempdir().unwrap();
    let svc = vec![kv_str("service.name", "hot")];
    let reqs = vec![req_with(svc, None, &[root_span(
        1, 0x11, 100 * NS, 101 * NS, "op",
    )])];
    let wal = write_wal(dir.path(), reqs, "corrupt-tblm");
    drop(sealed_source(dir.path(), &wal, "one"));
    let path = sealed_path(dir.path(), "one");
    corrupt_chunk(&path, *b"TBLM");
    let build = || both_roles(|| vec![sealed_source_at(&path, "one")]);

    let q = || SearchQuery::new(pred(vec![root_service_eq("hot")]));
    let on = run(build(), q());
    assert!(on.traces.is_empty());
    assert!(on.status.has(PartialReason::SourceFailure), "{:?}", on.status);
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
        req_with(cold, None, &[
            root_span(1, 0x11, 100 * NS, 101 * NS, "op"),
            span_in(1, 0x12, 0x11, 100 * NS + 10, "child"),
            span_in(1, 0x13, 0x11, 100 * NS + 20, "child"),
            span_in(1, 0x14, 0x11, 100 * NS + 30, "child"),
        ]),
        req_with(hot, None, &[root_span(2, 0x21, 200 * NS, 201 * NS, "op")]),
    ];
    let wal = write_wal(dir.path(), reqs, "sizecap-delta");
    let sources = || both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);
    let q = || SearchQuery::new(pred(vec![root_service_eq("hot")])).span_cap_for_tests(2);

    let off = run(sources(), q().trace_gate_for_tests(false));
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
    let leaked: Vec<&'static str> =
        names.iter().map(|n| &*Box::leak(n.clone().into_boxed_str())).collect();
    let reqs: Vec<ExportTraceServiceRequest> = (0..150u64)
        .map(|i| {
            req_with(svc.clone(), None, &[root_span(
                (i + 1) as u8,
                0x10,
                (i + 1) * NS,
                (i + 1) * NS + 50,
                leaked[i as usize],
            )])
        })
        .collect();
    let wal = write_wal(dir.path(), reqs, "midtier");
    let sources = || both_roles(|| vec![sealed_source(dir.path(), &wal, "one")]);

    // Trace ids wrap u8 (150 traces), so pick target 42 by name only.
    let q = || {
        SearchQuery::new(pred(vec![builtin(
            BuiltinField::RootName,
            CompareOp::Eq,
            vec![text("op-042")],
        )]))
    };
    let on = run(sources(), q());
    let off = run(sources(), q().trace_gate_for_tests(false));
    assert_eq!(on.traces.len(), 1, "mid-tier dictionary resolves in the gate");
    assert_eq!(norm(&on), norm(&off));

    // The closed rule, on the sealed file directly.
    let bytes = std::fs::read(sealed_path(dir.path(), "one")).unwrap();
    let reader = sfst::IndexReader::open(&bytes).unwrap();
    let rollup = reader.trace_rollup().unwrap();
    let name_ref = rollup.root_name_refs[0];
    let service_ref = rollup.root_service_refs[0];
    assert_ne!(name_ref, sfst::ROLLUP_NO_REF);
    assert_ne!(service_ref, sfst::ROLLUP_NO_REF);
    let mut resolver = sfst::RollupRootResolver::new(&reader);
    assert!(
        matches!(
            resolver.resolve("name", name_ref),
            sfst::RollupRefOutcome::Value(v) if v.starts_with("op-")
        ),
        "a ref in its own field is a proven value"
    );
    assert!(
        matches!(
            resolver.resolve("name", service_ref),
            sfst::RollupRefOutcome::Corrupt
        ),
        "a wrong-field ref is corruption evidence, never absence"
    );
    assert!(
        matches!(
            resolver.resolve("no.such.field", name_ref),
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
    let reqs = vec![req_with(cold, None, &[root_span(
        1, 0x11, 100 * NS, 101 * NS, "op",
    )])];
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

    let off = run(sources(), q().trace_gate_for_tests(false));
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
    let first = req_with(vec![kv_str("service.name", "svc-first")], None, &[SpanSpec {
        kind: 3,
        ..root_span(1, 0x42, 100 * NS, 101 * NS, "op")
    }]);
    let second = req_with(vec![kv_str("service.name", "svc-canon")], None, &[SpanSpec {
        kind: 2,
        ..root_span(1, 0x42, 100 * NS, 101 * NS, "op")
    }]);
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
            req_with(svc.clone(), None, &[SpanSpec {
                trace,
                end: (i + 1) * NS + 50,
                ..sp(0x10, 0, (i + 1) * NS, names[i as usize])
            }])
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
    let off = run(sources(), q().trace_gate_for_tests(false));
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

/// Seeded pseudo-random corpora × trace-level predicates: gate-on and
/// gate-off answers are identical (results AND status) on tie-free,
/// corruption-free inputs. Span-level-only predicates ride along to pin
/// the not-engaged path.
#[test]
fn differential_gate_on_off_sweep() {
    // A tiny deterministic LCG — no external randomness, stable seeds.
    struct Lcg(u64);
    impl Lcg {
        fn next(&mut self, bound: u64) -> u64 {
            self.0 = self.0.wrapping_mul(6364136223846793005).wrapping_add(1442695040888963407);
            (self.0 >> 33) % bound
        }
    }
    let services = ["ad", "cart", "checkout", "front"];
    let names = ["get", "post", "sync", "flush"];
    for seed in 0..4u64 {
        let mut rng = Lcg(seed.wrapping_mul(0x9E3779B97F4A7C15).wrapping_add(1));
        let dir = tempfile::tempdir().unwrap();
        // 24 traces, 1-3 spans each, distinct starts (tie-free), spread
        // across two sealed files by trace parity.
        let mut reqs_a: Vec<ExportTraceServiceRequest> = Vec::new();
        let mut reqs_b: Vec<ExportTraceServiceRequest> = Vec::new();
        for t in 1..=24u8 {
            let svc = services[rng.next(4) as usize];
            let name: &'static str = names[rng.next(4) as usize];
            let start = u64::from(t) * 10 * NS + rng.next(1000);
            let dur = (rng.next(20) + 1) * NS / 2;
            let mut spans = vec![root_span(t, 0x10, start, start + dur, name)];
            for extra in 0..rng.next(3) {
                let id = 0x20 + extra as u8;
                spans.push(SpanSpec {
                    end: start + dur / 2,
                    ..span_in(t, id, 0x10, start + 1 + extra * 7, "child")
                });
            }
            let req = req_with(vec![kv_str("service.name", svc)], None, &spans);
            if t % 2 == 0 { reqs_a.push(req) } else { reqs_b.push(req) }
        }
        let wal_a = write_wal(dir.path(), reqs_a, "diff-a");
        let wal_b = write_wal(dir.path(), reqs_b, "diff-b");
        let sources = || {
            both_roles(|| {
                vec![
                    sealed_source(dir.path(), &wal_a, "a"),
                    sealed_source(dir.path(), &wal_b, "b"),
                ]
            })
        };
        let queries: Vec<Predicate> = vec![
            pred(vec![root_service_eq("ad")]),
            pred(vec![root_service_eq("nosuch")]),
            pred(vec![builtin(
                BuiltinField::RootName,
                CompareOp::Regex,
                vec![text("(get|post)")],
            )]),
            pred(vec![min_duration(5 * NS as i64)]),
            pred(vec![min_duration(NS as i64), root_service_eq("cart")]),
            pred(vec![builtin(
                BuiltinField::TraceDuration,
                CompareOp::Lt,
                vec![PredicateValue::Integer(3 * NS as i64)],
            )]),
            // Span-level-only: the gate is never engaged.
            pred(vec![builtin(
                BuiltinField::Name,
                CompareOp::Eq,
                vec![text("child")],
            )]),
        ];
        for (qi, predicate) in queries.into_iter().enumerate() {
            let on = run(sources(), SearchQuery::new(predicate.clone()).limit(30));
            let off = run(
                sources(),
                SearchQuery::new(predicate).limit(30).trace_gate_for_tests(false),
            );
            assert_eq!(norm(&on), norm(&off), "seed {seed} query {qi}");
        }
    }
}
