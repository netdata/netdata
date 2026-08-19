//! Cross-source trace-by-id acceptance suite (phase 4a).
//!
//! The core criterion: a trace whose spans are split across sealed files,
//! an in-memory chunk SFST, and a WAL tail reconstructs IDENTICALLY to
//! the same corpus sealed as one file — under source-order permutations,
//! resends (equal and unequal payloads), shared client/server span ids,
//! UNSET ids, and mixed part_keys. Plus the status-honesty contract:
//! failures, caps, and cancellation are reported, never silent.

mod common;

use std::sync::Arc;
use std::sync::atomic::AtomicUsize;

use tokio_util::sync::CancellationToken;

use common::{
    SpanSpec, TRACE, kv_double, kv_int, kv_str, memory_source, req, sealed_source, sp,
    tail_source, write_wal,
};
use sfsq::Source;
use sfsq::traces::{
    PartialReason, SourceId, TraceQuery, TraceSfstCandidate, TraceSource,
    WalCoverage, trace_by_id,
};

fn run(sources: Vec<TraceSource>) -> sfsq::traces::TraceData {
    trace_by_id(
        sources,
        TraceQuery::new(sfst::TraceId::from(TRACE)),
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
    .expect("valid request")
}

/// A trace rendered comparable: the engine's outputs that must be
/// identical however the spans were physically scattered.
type Fingerprint = (
    Vec<sfst::TraceSpan>,
    Vec<usize>,
    Vec<Vec<usize>>,
    Vec<(String, sfst::ValueKind)>,
);

fn fingerprint(data: &sfsq::traces::TraceData) -> Fingerprint {
    // WHOLE spans: TraceSpan is Eq, so any sealed-vs-tail divergence in
    // any field (ids, timing, flags, dropped counts, fields, events,
    // links) fails the equivalence — not just names and ids.
    let spans = data.trace.spans.clone();
    let mut kinds = data.field_kinds.fields.clone();
    kinds.extend(
        data.field_kinds
            .event_attributes
            .iter()
            .map(|(k, v)| (format!("event:{k}"), *v)),
    );
    kinds.extend(
        data.field_kinds
            .link_attributes
            .iter()
            .map(|(k, v)| (format!("link:{k}"), *v)),
    );
    (
        spans,
        data.trace.roots.clone(),
        data.trace.children.clone(),
        kinds,
    )
}

/// The corpus every equivalence layout serves: a rooted tree, a shared
/// client/server-id pair with a child under it, an equal-payload resend,
/// an UNEQUAL resend (later start, different name — the canonical copy is
/// the earlier), and two UNSET-id spans.
fn corpus() -> Vec<SpanSpec> {
    let mut client = sp(3, 1, 120, "shared-client");
    client.kind = 3;
    let mut server = sp(3, 1, 122, "shared-server");
    server.kind = 2;
    let mut root = sp(1, 0, 100, "root");
    root.events = vec![(
        "exception",
        vec![kv_str("exception.type", "E"), kv_int("exception.line", 42)],
    )];
    root.links = vec![([0x33u8; 16], [0x44u8; 8], vec![kv_str("peer", "other")])];
    vec![
        root,
        sp(2, 1, 110, "child"),
        client,
        server,
        sp(4, 3, 130, "under-shared"),
        sp(2, 1, 110, "child"),          // equal-payload resend of `child`
        sp(2, 1, 500, "child-conflict"), // UNEQUAL resend: later start loses
        sp(0, 9, 140, "ghost-a"),        // UNSET id, orphan parent
        sp(0, 9, 141, "ghost-b"),        // UNSET id, distinct span
    ]
}

/// What the corpus must always reconstruct to.
fn assert_corpus_shape(data: &sfsq::traces::TraceData) {
    let t = &data.trace;
    // 7 spans: root, child (resends collapsed to the canonical
    // earliest copy), client, server (kind splits the shared id), the
    // shared pair's child, and the two never-collapsing UNSET spans.
    assert_eq!(t.spans.len(), 7, "spans: {:#?}", span_names(data));
    let child = t
        .spans
        .iter()
        .find(|s| s.span_id == sfst::SpanId::from([2u8; 8]))
        .expect("child present");
    assert!(
        child.fields.iter().any(|(k, v)| k == "name" && v == "child"),
        "the chronological-first copy is canonical"
    );
    // The shared pair's child hangs off the SERVER-kind node.
    let server_idx = t.spans.iter().position(|s| s.kind == 2).expect("server");
    let under_idx = t
        .spans
        .iter()
        .position(|s| s.span_id == sfst::SpanId::from([4u8; 8]))
        .unwrap();
    assert!(t.children[server_idx].contains(&under_idx));
    // Summary root is the true root — and carries its event and link
    // intact (EVNB/LNKB content merges identically however split).
    let root = t.summary_root().expect("non-empty");
    assert_eq!(t.spans[root].span_id, sfst::SpanId::from([1u8; 8]));
    let root_span = &t.spans[root];
    assert_eq!(root_span.events.len(), 1);
    assert_eq!(root_span.events[0].name, "exception");
    assert!(
        root_span.events[0]
            .attributes
            .iter()
            .any(|(k, v)| k == "exception.type" && v == "E")
    );
    assert_eq!(root_span.links.len(), 1);
    assert_eq!(root_span.links[0].trace_id, sfst::TraceId::from([0x33u8; 16]));
    // Sectioned kind maps use result-exposed names only.
    assert!(
        data.field_kinds
            .event_attributes
            .iter()
            .any(|(k, kind)| k == "exception.line" && *kind == sfst::ValueKind::Int),
        "event attr kinds: {:?}",
        data.field_kinds.event_attributes
    );
    assert!(
        data.field_kinds
            .link_attributes
            .iter()
            .any(|(k, kind)| k == "peer" && *kind == sfst::ValueKind::Str)
    );
    assert!(data.status.is_complete(), "status: {:?}", data.status);
}

fn span_names(data: &sfsq::traces::TraceData) -> Vec<String> {
    data.trace
        .spans
        .iter()
        .map(|s| {
            s.fields
                .iter()
                .find(|(k, _)| k == "name")
                .map(|(_, v)| v.clone())
                .unwrap_or_default()
        })
        .collect()
}

#[test]
fn split_many_ways_equals_single_file_under_source_permutations() {
    let dir = tempfile::tempdir().unwrap();
    let all = corpus();

    // Oracle: everything sealed as ONE file.
    let whole_wal = write_wal(dir.path(), vec![req(&all)], "whole");
    let oracle = run(vec![sealed_source(dir.path(), &whole_wal, "oracle")]);
    assert_corpus_shape(&oracle);

    // Split 4 ways: two sealed files with DIFFERENT part-key blobs, an
    // in-memory chunk, and a WAL tail.
    let wal_a = write_wal(dir.path(), vec![req(&all[0..2])], "stream-a");
    let wal_b = write_wal(dir.path(), vec![req(&all[2..5])], "stream-b");
    let wal_c = write_wal(dir.path(), vec![req(&all[5..7])], "stream-c");
    let wal_d = write_wal(dir.path(), vec![req(&all[7..9])], "stream-d");

    let make = || {
        vec![
            sealed_source(dir.path(), &wal_a, "sealed-a"),
            sealed_source(dir.path(), &wal_b, "sealed-b"),
            memory_source(&wal_c, "chunk-c"),
            tail_source(&wal_d, "tail-d"),
        ]
    };

    let split = run(make());
    assert_corpus_shape(&split);
    assert_eq!(fingerprint(&split), fingerprint(&oracle));

    // Source-order permutations must not change anything.
    let mut permuted = make();
    permuted.reverse();
    let reversed = run(permuted);
    assert_eq!(fingerprint(&reversed), fingerprint(&oracle));

    let mut rotated = make();
    rotated.rotate_left(2);
    let rotated = run(rotated);
    assert_eq!(fingerprint(&rotated), fingerprint(&oracle));
}

#[test]
fn unequal_resends_pick_the_same_canonical_copy_however_split() {
    // The conflicting copies land in DIFFERENT sources, both orders.
    let dir = tempfile::tempdir().unwrap();
    let early = sp(7, 0, 10, "early-copy");
    let mut late = sp(7, 0, 900, "late-copy");
    late.end = 950;

    let wal_early = write_wal(dir.path(), vec![req(std::slice::from_ref(&early))], "e");
    let wal_late = write_wal(dir.path(), vec![req(std::slice::from_ref(&late))], "l");

    let a = run(vec![
        sealed_source(dir.path(), &wal_early, "s-early"),
        sealed_source(dir.path(), &wal_late, "s-late"),
    ]);
    let b = run(vec![
        sealed_source(dir.path(), &wal_late, "s-late"),
        sealed_source(dir.path(), &wal_early, "s-early"),
    ]);
    for out in [&a, &b] {
        assert_eq!(out.trace.spans.len(), 1);
        assert_eq!(span_names(out), ["early-copy"]);
    }
}

#[test]
fn cap_keeps_globally_earliest_and_reports_size_cap() {
    // The LATER spans live in the first source; a cap of 2 must keep the
    // globally earliest two from the second.
    let dir = tempfile::tempdir().unwrap();
    let late = vec![sp(10, 0, 300, "late-1"), sp(11, 0, 400, "late-2")];
    let early = vec![sp(12, 0, 1, "early-1"), sp(13, 0, 2, "early-2")];
    let wal_late = write_wal(dir.path(), vec![req(&late)], "late");
    let wal_early = write_wal(dir.path(), vec![req(&early)], "early");

    let sources = vec![
        sealed_source(dir.path(), &wal_late, "late"),
        sealed_source(dir.path(), &wal_early, "early"),
    ];
    let data = trace_by_id(
        sources,
        TraceQuery::new(sfst::TraceId::from(TRACE)).span_cap(2),
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap();
    assert_eq!(span_names(&data), ["early-1", "early-2"]);
    assert!(data.status.has(PartialReason::SizeCap));

    // Exactly-cap is Complete: 2 unique spans, cap 2.
    let sources = vec![sealed_source(dir.path(), &wal_early, "early-2nd")];
    let data = trace_by_id(
        sources,
        TraceQuery::new(sfst::TraceId::from(TRACE)).span_cap(2),
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap();
    assert_eq!(data.trace.spans.len(), 2);
    assert!(data.status.is_complete());
}

#[test]
fn request_validation_rejects_unset_id_zero_cap_and_bad_source_sets() {
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), vec![req(&[sp(1, 0, 1, "x")])], "v");
    let src = || sealed_source(dir.path(), &wal, "same-id");

    // UNSET trace id.
    let err = trace_by_id(
        vec![src()],
        TraceQuery::new(sfst::TraceId::UNSET),
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap_err();
    assert!(matches!(
        err,
        sfsq::traces::TraceRequestError::UnsetTraceId
    ));

    // Zero cap.
    let err = trace_by_id(
        vec![src()],
        TraceQuery::new(sfst::TraceId::from(TRACE)).span_cap(0),
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap_err();
    assert!(matches!(err, sfsq::traces::TraceRequestError::ZeroSpanCap));

    // Duplicate SourceId.
    let err = trace_by_id(
        vec![src(), src()],
        TraceQuery::new(sfst::TraceId::from(TRACE)),
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap_err();
    assert!(matches!(
        err,
        sfsq::traces::TraceRequestError::SourceSet(_)
    ));

    // Overlapping WAL coverage (chunk and tail over intersecting ranges
    // of the same WAL — both helpers derive wal_id from the path, so the
    // collision is caught exactly as in production).
    let err = trace_by_id(
        vec![memory_source(&wal, "one-wal"), tail_source(&wal, "one-wal-tail")],
        TraceQuery::new(sfst::TraceId::from(TRACE)),
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap_err();
    assert!(matches!(
        err,
        sfsq::traces::TraceRequestError::SourceSet(_)
    ));
}

#[test]
fn failed_sources_are_reported_and_the_rest_served() {
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), vec![req(&[sp(1, 0, 1, "good")])], "ok");

    // A missing file and an unparseable in-memory image beside a good one.
    let missing = TraceSource::Sfst(TraceSfstCandidate {
        source_id: SourceId::new("missing"),
        summary: sfst::Summary {
            min_timestamp_s: 0,
            max_timestamp_s: 0,
            record_count: 0,
            content_meta: Vec::new(),
        },
        source: Source::File(dir.path().join("does-not-exist.sfst")),
        coverage: None,
    });
    let garbage = TraceSource::Sfst(TraceSfstCandidate {
        source_id: SourceId::new("garbage"),
        summary: sfst::Summary {
            min_timestamp_s: 0,
            max_timestamp_s: 0,
            record_count: 0,
            content_meta: Vec::new(),
        },
        source: Source::Memory(Arc::new(vec![0u8; 64])),
        coverage: Some(WalCoverage {
            wal_id: "garbage-wal".into(),
            range: wal::FrameRange::new(0, 64),
        }),
    });

    let data = run(vec![
        missing,
        garbage,
        sealed_source(dir.path(), &wal, "good"),
    ]);
    assert_eq!(span_names(&data), ["good"]);
    assert!(data.status.has(PartialReason::SourceFailure));
    assert!(!data.status.has(PartialReason::SizeCap));
}

#[test]
fn pre_cancelled_returns_empty_with_cancelled() {
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), vec![req(&[sp(1, 0, 1, "x")])], "c");
    let cancel = CancellationToken::new();
    cancel.cancel();
    let data = trace_by_id(
        vec![sealed_source(dir.path(), &wal, "s")],
        TraceQuery::new(sfst::TraceId::from(TRACE)),
        cancel,
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap();
    assert!(data.trace.spans.is_empty());
    assert!(data.status.has(PartialReason::Cancelled));
}

#[test]
fn field_kinds_coalesce_across_sources_and_tail() {
    // The same attribute is an Int in a sealed file, a Double in the
    // tail: the merged kind widens to Double. A string+int mix falls to
    // Str. Kinds merge from CONTRIBUTING sources only.
    let dir = tempfile::tempdir().unwrap();
    let mut a = sp(1, 0, 100, "a");
    a.attrs = vec![kv_int("payload.size", 3), kv_int("mixed", 1)];
    let mut b = sp(2, 1, 110, "b");
    b.attrs = vec![kv_double("payload.size", 1.5), kv_str("mixed", "one")];

    let wal_a = write_wal(dir.path(), vec![req(&[a])], "ka");
    let wal_b = write_wal(dir.path(), vec![req(&[b])], "kb");
    let data = run(vec![
        sealed_source(dir.path(), &wal_a, "ka"),
        tail_source(&wal_b, "kb"),
    ]);
    assert!(data.status.is_complete());
    let kind_of = |name: &str| {
        data.field_kinds
            .fields
            .iter()
            .find(|(k, _)| k == &format!("attributes.{name}"))
            .map(|(_, v)| *v)
            .unwrap_or_else(|| panic!("kind for {name} present: {:?}", data.field_kinds.fields))
    };
    assert_eq!(kind_of("payload.size"), sfst::ValueKind::Double);
    assert_eq!(kind_of("mixed"), sfst::ValueKind::Str);

    // A non-contributing source's kinds must NOT leak in: a file holding
    // only a DIFFERENT trace id contributes nothing.
    let mut other = sp(5, 0, 100, "other");
    other.attrs = vec![kv_str("only.in.other", "x")];
    let mut other_req = req(&[other]);
    for rs in &mut other_req.resource_spans {
        for ss in &mut rs.scope_spans {
            for s in &mut ss.spans {
                s.trace_id = vec![0x11u8; 16];
            }
        }
    }
    let wal_o = write_wal(dir.path(), vec![other_req], "ko");
    let data = run(vec![
        sealed_source(dir.path(), &wal_a, "ka2"),
        sealed_source(dir.path(), &wal_o, "ko"),
    ]);
    assert!(
        !data
            .field_kinds
            .fields
            .iter()
            .any(|(k, _)| k.contains("only.in.other")),
        "non-contributing source leaked kinds: {:?}",
        data.field_kinds.fields
    );
}

#[test]
fn absent_id_is_a_complete_empty_result() {
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), vec![req(&[sp(1, 0, 1, "x")])], "abs");
    let data = trace_by_id(
        vec![sealed_source(dir.path(), &wal, "s")],
        TraceQuery::new(sfst::TraceId::from([0x77u8; 16])),
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap();
    assert!(data.trace.spans.is_empty());
    assert!(data.status.is_complete());
    assert!(data.field_kinds.fields.is_empty());
    assert!(data.field_kinds.event_attributes.is_empty());
    assert!(data.field_kinds.link_attributes.is_empty());
}

#[test]
fn pre_cancelled_with_zero_sources_is_cancelled_not_complete() {
    let cancel = CancellationToken::new();
    cancel.cancel();
    let data = trace_by_id(
        Vec::new(),
        TraceQuery::new(sfst::TraceId::from(TRACE)),
        cancel,
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap();
    assert!(data.trace.spans.is_empty());
    assert!(data.status.has(PartialReason::Cancelled));
    assert!(!data.status.is_complete());
}

#[test]
fn memory_chunk_without_coverage_is_a_request_error() {
    let chunk = TraceSource::Sfst(TraceSfstCandidate {
        source_id: SourceId::new("uncovered-chunk"),
        summary: sfst::Summary {
            min_timestamp_s: 0,
            max_timestamp_s: 0,
            record_count: 0,
            content_meta: Vec::new(),
        },
        source: Source::Memory(Arc::new(Vec::new())),
        coverage: None,
    });
    let err = trace_by_id(
        vec![chunk],
        TraceQuery::new(sfst::TraceId::from(TRACE)),
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap_err();
    assert!(matches!(
        err,
        sfsq::traces::TraceRequestError::SourceSet(_)
    ));
}

#[test]
fn large_same_key_resend_run_collapses_to_one_canonical_span() {
    // 40 copies of one span key — 20 in each of two sources, some with a
    // conflicting later payload — collapse to the single chronological-
    // first canonical copy (the streaming selection path).
    let dir = tempfile::tempdir().unwrap();
    let canonical = sp(9, 0, 5, "canonical");
    let mut copies_a: Vec<SpanSpec> = vec![canonical.clone(); 20];
    let mut copies_b: Vec<SpanSpec> = vec![canonical.clone(); 18];
    let mut late = sp(9, 0, 700, "late-conflict");
    late.end = 780;
    copies_a.push(late.clone());
    copies_b.push(late);
    copies_b.push(sp(10, 9, 30, "child"));
    let wal_a = write_wal(dir.path(), vec![req(&copies_a)], "runa");
    let wal_b = write_wal(dir.path(), vec![req(&copies_b)], "runb");
    let data = run(vec![
        sealed_source(dir.path(), &wal_a, "run-a"),
        sealed_source(dir.path(), &wal_b, "run-b"),
    ]);
    assert!(data.status.is_complete());
    assert_eq!(data.trace.spans.len(), 2, "{:?}", span_names(&data));
    assert_eq!(span_names(&data), ["canonical", "child"]);
}

#[test]
fn coexisting_partial_reasons_accumulate() {
    // A garbage source (SourceFailure) beside a good 2-span source with
    // cap 1 (SizeCap): BOTH reasons must survive in the status set.
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(
        dir.path(),
        vec![req(&[sp(1, 0, 1, "one"), sp(2, 0, 2, "two")])],
        "combo",
    );
    let garbage = TraceSource::Sfst(TraceSfstCandidate {
        source_id: SourceId::new("garbage"),
        summary: sfst::Summary {
            min_timestamp_s: 0,
            max_timestamp_s: 0,
            record_count: 0,
            content_meta: Vec::new(),
        },
        source: Source::Memory(Arc::new(vec![0u8; 64])),
        coverage: Some(WalCoverage {
            wal_id: "garbage-wal-2".into(),
            range: wal::FrameRange::new(0, 64),
        }),
    });
    let data = trace_by_id(
        vec![garbage, sealed_source(dir.path(), &wal, "good")],
        TraceQuery::new(sfst::TraceId::from(TRACE)).span_cap(1),
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap();
    assert_eq!(data.trace.spans.len(), 1);
    assert!(data.status.has(PartialReason::SourceFailure));
    assert!(data.status.has(PartialReason::SizeCap));
}
