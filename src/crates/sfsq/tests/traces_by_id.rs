//! Cross-source trace-by-id acceptance suite (phase 4a).
//!
//! The core criterion: a trace whose spans are split across sealed files,
//! an in-memory chunk SFST, and a WAL tail reconstructs IDENTICALLY to
//! the same corpus sealed as one file — under source-order permutations,
//! resends (equal and unequal payloads), shared client/server span ids,
//! UNSET ids, and mixed part_keys. Plus the status-honesty contract:
//! failures, caps, and cancellation are reported, never silent. Also
//! pinned: request validation, the coalesced field-kind map, the
//! absent-id Complete-empty, and the per-source progress ticks.
//!
//! The op's engine contract lives in `src/traces/by_id.rs`; the sibling
//! suites in this directory pin the other traces query ops.

mod common;

use std::collections::BTreeSet;
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

use tokio_util::sync::CancellationToken;

use common::{
    SpanSpec, TRACE, corrupt_chunk, kv_double, kv_int, kv_str, memory_source, missing_source, req,
    req_with, sealed_source, sealed_source_at, sp, tail_source, unavailable_source, write_wal,
};
use sfsq::Source;
use sfsq::traces::{
    PartialReason, QueryStatus, SourceId, TraceQuery, TraceSfstCandidate, TraceSource, WalCoverage,
    trace_by_id,
};

/// The suite's default runner: `trace_by_id` with the default query
/// (default span cap), a fresh token, and a throwaway progress counter.
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
    Vec<(String, sfst::ValueKinds)>,
);

/// A kind set from its members.
fn kinds_of(list: &[sfst::ValueKind]) -> sfst::ValueKinds {
    list.iter().copied().collect()
}

fn fingerprint(data: &sfsq::traces::TraceData) -> Fingerprint {
    // WHOLE spans: TraceSpan is Eq, so any sealed-vs-tail divergence in
    // any field (ids, timing, kind, flags, dropped counts, fields,
    // events, links) fails the equivalence — not just names and ids.
    // The kind maps join into one vec with event:/link: prefixes, so an
    // event attr and a link attr of the same name compare separately.
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
        sp(0, 9, 141, "ghost-b"),        // UNSET id again — a distinct span
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
        child
            .fields
            .iter()
            .any(|(k, v)| k == "name" && v == "child"),
        "the chronological-first copy is canonical"
    );
    // The shared pair's child hangs off the SERVER-kind node — the
    // forest builder resolves a shared parent id to the server-kind
    // candidate, else the earliest.
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
    assert_eq!(
        root_span.links[0].trace_id,
        sfst::TraceId::from([0x33u8; 16])
    );
    // Sectioned kind maps use result-exposed names only —
    // "exception.line", not its storage-prefixed key.
    assert!(
        data.field_kinds
            .event_attributes
            .iter()
            .any(|(k, kinds)| k == "exception.line" && *kinds == kinds_of(&[sfst::ValueKind::Int])),
        "event attr kinds: {:?}",
        data.field_kinds.event_attributes
    );
    assert!(
        data.field_kinds
            .link_attributes
            .iter()
            .any(|(k, kinds)| k == "peer" && *kinds == kinds_of(&[sfst::ValueKind::Str]))
    );
    // Complete is part of the shape: no layout of this corpus may trip
    // a partial reason.
    assert!(data.status.is_complete(), "status: {:?}", data.status);
}

/// Rendered span names — assertion-message material.
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
    // The conflicting copies land in DIFFERENT sources; both source
    // orders must pick the same canonical (earlier) copy.
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

    // Exactly-cap is Complete — truncation is only detected at the
    // first unique span BEYOND the cap.
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
    assert!(matches!(err, sfsq::traces::TraceRequestError::UnsetTraceId));

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
    assert!(matches!(err, sfsq::traces::TraceRequestError::SourceSet(_)));

    // Overlapping WAL coverage (chunk and tail over intersecting ranges
    // of the same WAL — both helpers derive wal_id from the path, so the
    // collision is caught exactly as in production).
    let err = trace_by_id(
        vec![
            memory_source(&wal, "one-wal"),
            tail_source(&wal, "one-wal-tail"),
        ],
        TraceQuery::new(sfst::TraceId::from(TRACE)),
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap_err();
    assert!(matches!(err, sfsq::traces::TraceRequestError::SourceSet(_)));
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
    // A source failure must not masquerade as a size-cap partial.
    assert!(!data.status.has(PartialReason::SizeCap));
}

/// A shaper bounds what the merge keeps: past its budget the trace is the
/// earliest spans that fit, reported as `byte_budget`, with the shaper's
/// cut flags beside them.
#[test]
fn a_shaper_budget_cuts_the_trace_and_says_so() {
    struct OneEach;
    impl sfst::trace_combine::SpanShaper for OneEach {
        fn shape(&mut self, span: &mut sfst::TraceSpan) -> sfst::trace_combine::Shaped {
            sfst::trace_combine::Shaped {
                weight: 1,
                cut: span.start_ns == 1,
            }
        }
        fn budget(&self) -> usize {
            2
        }
    }
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(
        dir.path(),
        vec![req(&[sp(1, 0, 1, "a"), sp(2, 1, 2, "b"), sp(3, 1, 3, "c")])],
        "w",
    );
    let data = trace_by_id(
        vec![sealed_source(dir.path(), &wal, "w")],
        TraceQuery::new(sfst::TraceId::from(TRACE)).shaper(Box::new(OneEach)),
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap();
    assert_eq!(span_names(&data), ["a", "b"]);
    assert_eq!(data.span_cut, [true, false]);
    assert_eq!(
        data.status,
        QueryStatus::Partial([PartialReason::ByteBudget].into())
    );
}

/// Bloom first: a file that does not hold the trace is never opened
/// fully, so a corrupt primary dictionary there stays unread — the
/// lookup is complete. The same corruption in the file holding the trace
/// is reported.
#[test]
fn a_corrupt_file_the_trace_does_not_need_stays_unread() {
    let dir = tempfile::tempdir().unwrap();
    let mut other = sp(1, 0, 1, "other");
    other.trace = [0x42; 16];
    let w_other = write_wal(dir.path(), vec![req(&[other])], "other");
    let w_held = write_wal(dir.path(), vec![req(&[sp(2, 0, 2, "held")])], "held");
    for (wal, id) in [(&w_other, "other"), (&w_held, "held")] {
        drop(sealed_source(dir.path(), wal, id));
        corrupt_chunk(&dir.path().join(format!("{id}.sfst")), *b"PRIM");
    }
    let other = || sealed_source_at(&dir.path().join("other.sfst"), "other");
    let held = || sealed_source_at(&dir.path().join("held.sfst"), "held");

    let data = run(vec![other(), sealed_source(dir.path(), &w_held, "fresh")]);
    assert_eq!(span_names(&data), ["held"]);
    assert_eq!(data.status, QueryStatus::Complete);

    let data = run(vec![other(), held()]);
    assert!(data.status.has(PartialReason::SourceFailure));
}

#[test]
fn pre_cancelled_returns_empty_with_cancelled() {
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), vec![req(&[sp(1, 0, 1, "x")])], "c");
    let cancel = CancellationToken::new();
    cancel.cancel();
    // A ready source exists, but cancellation is polled before any
    // source head resolves: empty + Cancelled, never Complete.
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
fn field_kinds_union_across_sources_and_tail() {
    // The same attribute is an Int in a sealed file, a Double in the
    // tail: its kinds are both. A string+int mix keeps both too — nothing
    // coalesces. Kinds merge from CONTRIBUTING sources only.
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
    use sfst::ValueKind::{Double, Int, Str};
    assert_eq!(kind_of("payload.size"), kinds_of(&[Int, Double]));
    assert_eq!(kind_of("mixed"), kinds_of(&[Int, Str]));

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

/// Field kinds against ground truth, identical in every layout: every
/// name the returned spans expose has exactly the kinds the contributing
/// sources stored under it, and no other name has any. A key int in one
/// source and str in another lists both; an event attribute and a link
/// attribute sharing a name keep their own kinds; a key only in a span
/// the byte budget cut, or only in another trace, is absent; a kind only
/// another trace of the same file stored is present (a file records
/// kinds per key, not per value); null and empty-container values have
/// their kinds too.
#[test]
fn field_kinds_are_exact_in_every_layout() {
    use sfst::ValueKind::{Double, EmptyArray, EmptyKvlist, Int, Null, Str};
    let dir = tempfile::tempdir().unwrap();
    let mut a = sp(1, 0, 100, "a");
    let kv_any = |key: &str, value| opentelemetry_proto::tonic::common::v1::KeyValue {
        key: key.into(),
        value,
        key_strindex: 0,
    };
    let any = |v| Some(opentelemetry_proto::tonic::common::v1::AnyValue { value: Some(v) });
    use opentelemetry_proto::tonic::common::v1::{
        ArrayValue, KeyValueList, any_value::Value as Av,
    };
    a.attrs = vec![
        kv_int("k.mixed", 1),
        kv_int("k.int", 2),
        kv_str("k.str", "x"),
        kv_any("k.null", None),
        kv_any(
            "k.obj",
            any(Av::KvlistValue(KeyValueList { values: vec![] })),
        ),
        kv_any("k.arr", any(Av::ArrayValue(ArrayValue { values: vec![] }))),
    ];
    a.events = vec![("ev", vec![kv_int("shared", 1)])];
    a.links = vec![([0x33u8; 16], [0x44u8; 8], vec![kv_str("shared", "x")])];
    let mut other = SpanSpec {
        trace: [0x11u8; 16],
        ..sp(9, 0, 105, "other")
    };
    other.attrs = vec![kv_str("k.int", "text"), kv_str("k.other", "z")];
    let mut b = sp(2, 1, 110, "b");
    b.attrs = vec![kv_str("k.mixed", "one"), kv_double("k.dbl", 0.5)];
    let mut c = sp(3, 1, 120, "c");
    c.attrs = vec![kv_int("k.cut", 1)];
    let early = || vec![req(&[a.clone(), other.clone()])];
    let late = || vec![req(&[b.clone(), c.clone()])];
    let wal_all = write_wal(
        dir.path(),
        early().into_iter().chain(late()).collect(),
        "all",
    );
    let wal_a = write_wal(dir.path(), early(), "early");
    let wal_b = write_wal(dir.path(), late(), "late");
    let layouts: Vec<(&str, Vec<TraceSource>)> = vec![
        (
            "single-sealed",
            vec![sealed_source(dir.path(), &wal_all, "all")],
        ),
        (
            "two-sealed",
            vec![
                sealed_source(dir.path(), &wal_a, "a2"),
                sealed_source(dir.path(), &wal_b, "b2"),
            ],
        ),
        (
            "sealed-plus-tail",
            vec![
                sealed_source(dir.path(), &wal_a, "a3"),
                tail_source(&wal_b, "b3"),
            ],
        ),
        (
            "chunk-plus-tail",
            vec![memory_source(&wal_a, "a4"), tail_source(&wal_b, "b4")],
        ),
    ];
    for (layout, sources) in layouts {
        let data = trace_by_id(
            sources,
            TraceQuery::new(sfst::TraceId::from(TRACE)).shaper(Box::new(Cut4 { budget: 2 })),
            CancellationToken::new(),
            Arc::new(AtomicUsize::new(0)),
        )
        .expect("valid request");
        assert_eq!(
            span_names(&data),
            ["a", "b"],
            "{layout}: c is past the budget"
        );
        let k = &data.field_kinds;
        let get = |section: &[(String, sfst::ValueKinds)], name: &str| {
            section
                .iter()
                .find(|(n, _)| n == name)
                .map(|(_, kinds)| *kinds)
        };
        for (name, want) in [
            ("attributes.k.mixed", Some(kinds_of(&[Int, Str]))),
            ("attributes.k.int", Some(kinds_of(&[Int, Str]))),
            ("attributes.k.str", Some(kinds_of(&[Str]))),
            ("attributes.k.dbl", Some(kinds_of(&[Double]))),
            ("attributes.k.null", Some(kinds_of(&[Null]))),
            ("attributes.k.obj", Some(kinds_of(&[EmptyKvlist]))),
            ("attributes.k.arr", Some(kinds_of(&[EmptyArray]))),
            ("name", Some(kinds_of(&[Str]))),
            ("_kind", Some(kinds_of(&[Int]))),
            ("attributes.k.cut", None),
            ("attributes.k.other", None),
        ] {
            assert_eq!(get(&k.fields, name), want, "{layout}: {name}");
        }
        assert_eq!(
            k.event_attributes,
            [("shared".to_string(), kinds_of(&[Int]))],
            "{layout}"
        );
        assert_eq!(
            k.link_attributes,
            [("shared".to_string(), kinds_of(&[Str]))],
            "{layout}"
        );
        let mut exposed: Vec<&str> = data
            .trace
            .spans
            .iter()
            .flat_map(|s| s.fields.iter().map(|(n, _)| n.as_str()))
            .collect();
        exposed.sort();
        exposed.dedup();
        let named: Vec<&str> = k.fields.iter().map(|(n, _)| n.as_str()).collect();
        assert_eq!(named, exposed, "{layout}: exactly the exposed names");
        assert!(
            k.fields.iter().all(|(_, kinds)| !kinds.is_empty()),
            "{layout}"
        );
    }
}

/// Sources that contribute no retained span add no kinds, whatever the
/// bloom says: a losing unequal resend (`str`) and a span past the byte
/// budget (`double`) each sit alone in their source, and the kept spans'
/// key keeps only its own kind (`int`). Sealed in one file together, the
/// file's schema covers all three — the documented over-approximation.
#[test]
fn non_contributing_sources_add_no_kinds() {
    use sfst::ValueKind::{Double, Int, Str};
    let dir = tempfile::tempdir().unwrap();
    let mut a = sp(1, 0, 100, "a");
    a.attrs = vec![kv_int("k.x", 1)];
    let mut b = sp(2, 1, 150, "b");
    b.attrs = vec![kv_int("k.x", 2)];
    // Same span id and kind as `a`, later: the losing copy.
    let mut resend = sp(1, 0, 120, "a");
    resend.attrs = vec![kv_str("k.x", "late")];
    let mut c = sp(3, 1, 300, "c");
    c.attrs = vec![kv_double("k.x", 0.5)];
    let wal_a = write_wal(dir.path(), vec![req(&[a.clone(), b.clone()])], "ka");
    let wal_b = write_wal(dir.path(), vec![req(&[resend.clone()])], "kb");
    let wal_c = write_wal(dir.path(), vec![req(&[c.clone()])], "kc");
    let wal_all = write_wal(dir.path(), vec![req(&[a, b, resend, c])], "kall");
    let run_cut = |sources: Vec<TraceSource>| {
        trace_by_id(
            sources,
            TraceQuery::new(sfst::TraceId::from(TRACE)).shaper(Box::new(Cut4 { budget: 2 })),
            CancellationToken::new(),
            Arc::new(AtomicUsize::new(0)),
        )
        .expect("valid request")
    };
    let x_kinds = |data: &sfsq::traces::TraceData| {
        data.field_kinds
            .fields
            .iter()
            .find(|(n, _)| n == "attributes.k.x")
            .map(|(_, kinds)| *kinds)
    };
    let layouts: Vec<(&str, Vec<TraceSource>)> = vec![
        (
            "three-sealed",
            vec![
                sealed_source(dir.path(), &wal_a, "a1"),
                sealed_source(dir.path(), &wal_b, "b1"),
                sealed_source(dir.path(), &wal_c, "c1"),
            ],
        ),
        (
            "sealed-plus-tails",
            vec![
                sealed_source(dir.path(), &wal_a, "a2"),
                tail_source(&wal_b, "b2"),
                tail_source(&wal_c, "c2"),
            ],
        ),
        (
            "chunk-sealed-tail",
            vec![
                memory_source(&wal_a, "a3"),
                sealed_source(dir.path(), &wal_b, "b3"),
                tail_source(&wal_c, "c3"),
            ],
        ),
    ];
    for (layout, sources) in layouts {
        let data = run_cut(sources);
        assert_eq!(span_names(&data), ["a", "b"], "{layout}");
        assert_eq!(
            data.trace.spans[0]
                .fields
                .iter()
                .find(|(n, _)| n == "attributes.k.x")
                .map(|(_, v)| v.as_str()),
            Some("1"),
            "{layout}: the canonical copy is kept"
        );
        assert_eq!(x_kinds(&data), Some(kinds_of(&[Int])), "{layout}");
    }
    let data = run_cut(vec![sealed_source(dir.path(), &wal_all, "all")]);
    assert_eq!(span_names(&data), ["a", "b"]);
    assert_eq!(x_kinds(&data), Some(kinds_of(&[Int, Double, Str])));
}

/// Two byte-equal copies of a span — the same text from an `int` in one
/// source and a `str` in another — are one span held by both sources, so
/// both contribute and the key lists both kinds.
#[test]
fn byte_equal_resends_list_both_sources_kinds() {
    use sfst::ValueKind::{Int, Str};
    let dir = tempfile::tempdir().unwrap();
    let mut as_int = sp(1, 0, 100, "a");
    as_int.attrs = vec![kv_int("k.x", 1)];
    let mut as_str = sp(1, 0, 100, "a");
    as_str.attrs = vec![kv_str("k.x", "1")];
    let wal_int = write_wal(dir.path(), vec![req(&[as_int])], "ki");
    let wal_str = write_wal(dir.path(), vec![req(&[as_str])], "ks");
    let data = run(vec![
        sealed_source(dir.path(), &wal_int, "i"),
        sealed_source(dir.path(), &wal_str, "s"),
    ]);
    assert_eq!(span_names(&data), ["a"]);
    let kinds = data
        .field_kinds
        .fields
        .iter()
        .find(|(n, _)| n == "attributes.k.x")
        .map(|(_, kinds)| *kinds);
    assert_eq!(kinds, Some(kinds_of(&[Int, Str])));
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
    // An empty source set would otherwise report a vacuous Complete;
    // the pre-cancelled poll wins.
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
    // An in-memory chunk must declare the WAL range it came from:
    // without coverage the source set is rejected outright.
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
    assert!(matches!(err, sfsq::traces::TraceRequestError::SourceSet(_)));
}

#[test]
fn large_same_key_resend_run_collapses_to_one_canonical_span() {
    // 40 copies of one span key (21 in one source, 19 in the other), two
    // with a conflicting later payload — collapse to the single
    // chronological-first canonical copy (the streaming selection path).
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

/// A source whose bytes could not be obtained reports its own reason
/// beside a missing local file's, while the good source still serves;
/// alone, it is never Complete and never a source failure.
#[test]
fn unavailable_sources_are_reported_with_their_own_reason() {
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), vec![req(&[sp(1, 0, 1, "good")])], "ok");
    let progress = Arc::new(AtomicUsize::new(0));
    let data = trace_by_id(
        vec![
            unavailable_source("remote", 0, 10),
            missing_source(dir.path(), "missing", 0, 10),
            sealed_source(dir.path(), &wal, "good"),
        ],
        TraceQuery::new(sfst::TraceId::from(TRACE)),
        CancellationToken::new(),
        Arc::clone(&progress),
    )
    .unwrap();
    assert_eq!(span_names(&data), ["good"]);
    assert_eq!(
        data.status,
        QueryStatus::Partial(BTreeSet::from([
            PartialReason::SourceFailure,
            PartialReason::RemoteUnavailable,
        ]))
    );
    assert_eq!(progress.load(Ordering::Relaxed), 3, "one tick per source");

    let only = run(vec![unavailable_source("remote", 0, 10)]);
    assert!(only.trace.spans.is_empty());
    assert_eq!(
        only.status,
        QueryStatus::Partial(BTreeSet::from([PartialReason::RemoteUnavailable]))
    );
}

// ── Highlight ─────────────────────────────────────────────────────────

fn token(
    target: sfsq::traces::PredicateTarget,
    op: sfsq::traces::CompareOp,
    value: &str,
) -> sfsq::traces::Token {
    sfsq::traces::Token::positive(sfsq::traces::Predicate {
        conditions: vec![sfsq::traces::Condition {
            target,
            op,
            values: vec![sfsq::traces::PredicateValue::Text(value.to_string())],
        }],
    })
}

fn name_token(name: &str) -> sfsq::traces::Token {
    token(
        sfsq::traces::PredicateTarget::Builtin(sfsq::traces::BuiltinField::Name),
        sfsq::traces::CompareOp::Eq,
        name,
    )
}

fn highlight(
    sources: Vec<TraceSource>,
    tokens: Vec<sfsq::traces::Token>,
    shaper: Option<Box<dyn sfst::trace_combine::SpanShaper + Send>>,
) -> sfsq::traces::TraceData {
    let mut query =
        TraceQuery::new(sfst::TraceId::from(TRACE)).highlight(sfsq::traces::TokenQuery { tokens });
    if let Some(shaper) = shaper {
        query = query.shaper(shaper);
    }
    trace_by_id(
        sources,
        query,
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap()
}

/// Cuts every field value to 4 bytes, one unit per span.
struct Cut4 {
    budget: usize,
}

impl sfst::trace_combine::SpanShaper for Cut4 {
    fn shape(&mut self, span: &mut sfst::TraceSpan) -> sfst::trace_combine::Shaped {
        let mut cut = false;
        for (_, value) in &mut span.fields {
            if value.len() > 4 {
                value.truncate(4);
                cut = true;
            }
        }
        sfst::trace_combine::Shaped { weight: 1, cut }
    }
    fn budget(&self) -> usize {
        self.budget
    }
}

/// Highlighting tests the full values: a free-text match that lies past
/// the shaper's cut still marks the span, whose returned value is cut.
#[test]
fn highlight_sees_values_past_the_cut() {
    let dir = tempfile::tempdir().unwrap();
    let mut a = sp(1, 0, 1, "a");
    a.attrs = vec![kv_str("note", "xxxxxxxx-needle")];
    let mut b = sp(2, 1, 2, "b");
    b.attrs = vec![kv_str("note", "plain")];
    let wal = write_wal(dir.path(), vec![req(&[a, b])], "w");
    let text = token(
        sfsq::traces::PredicateTarget::FreeText,
        sfsq::traces::CompareOp::Contains,
        "NEEDLE",
    );
    let data = highlight(
        vec![sealed_source(dir.path(), &wal, "w")],
        vec![text],
        Some(Box::new(Cut4 { budget: 100 })),
    );
    assert_eq!(span_names(&data), ["a", "b"]);
    assert_eq!(data.matched, [0]);
    let note = data.trace.spans[0]
        .fields
        .iter()
        .find(|(k, _)| k == "attributes.note")
        .map(|(_, v)| v.as_str());
    assert_eq!(note, Some("xxxx"), "the returned value is cut");
}

/// Indices name spans, not ids: of a client/server pair sharing a span
/// id only the matching one is marked, and a span with the UNSET id can
/// match too.
#[test]
fn highlight_shared_and_unset_ids() {
    let dir = tempfile::tempdir().unwrap();
    let mut client = sp(1, 0, 1, "client");
    client.kind = 3;
    let mut server = sp(1, 1, 2, "server");
    server.kind = 2;
    let unset = sp(0, 1, 3, "unset");
    let wal = write_wal(dir.path(), vec![req(&[client, server, unset])], "w");
    let src = || vec![sealed_source(dir.path(), &wal, "w")];
    let data = highlight(src(), vec![name_token("server")], None);
    assert_eq!(span_names(&data), ["client", "server", "unset"]);
    assert_eq!(data.matched, [1]);
    let data = highlight(src(), vec![name_token("unset"), name_token("client")], None);
    assert_eq!(data.matched, [0, 2], "any positive token marks a span");
}

/// Integer comparisons are exact past 2^53, in sealed files and in the
/// WAL tail: `= 2^53` does not mark the span holding 2^53 + 1, which an
/// f64 comparison would equate with it.
#[test]
fn highlight_compares_big_integers_exactly() {
    const BIG: i64 = (1 << 53) + 1;
    let dir = tempfile::tempdir().unwrap();
    let mut a = sp(1, 0, 1, "a");
    a.attrs = vec![kv_int("big", BIG)];
    let mut b = sp(2, 1, 2, "b");
    b.attrs = vec![kv_int("big", BIG - 1)];
    let wal = write_wal(dir.path(), vec![req(&[a, b])], "w");
    let big = |op, n| {
        sfsq::traces::Token::positive(sfsq::traces::Predicate {
            conditions: vec![sfsq::traces::Condition {
                target: sfsq::traces::PredicateTarget::Attribute(
                    sfsq::traces::AttributeOwner::Span,
                    "big".into(),
                ),
                op,
                values: vec![sfsq::traces::PredicateValue::Integer(n)],
            }],
        })
    };
    use sfsq::traces::CompareOp::{Eq, Gt, Lt};
    let cases = [
        (big(Eq, BIG), vec![0]),
        (big(Eq, BIG - 1), vec![1]),
        (big(Gt, BIG - 1), vec![0]),
        (big(Lt, BIG), vec![1]),
    ];
    for (i, (token, want)) in cases.into_iter().enumerate() {
        for (layout, src) in [
            ("sealed", sealed_source(dir.path(), &wal, &format!("w{i}"))),
            ("tail", tail_source(&wal, &format!("w{i}-tail"))),
        ] {
            let data = highlight(vec![src], vec![token.clone()], None);
            assert_eq!(span_names(&data), ["a", "b"], "case {i} on {layout}");
            assert_eq!(data.matched, want, "case {i} on {layout}");
        }
    }
}

/// Past the byte budget the trace is a prefix, and so are its marks: a
/// matching span the budget refused is not reported.
#[test]
fn highlight_respects_the_byte_budget_prefix() {
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(
        dir.path(),
        vec![req(&[sp(1, 0, 1, "a"), sp(2, 1, 2, "b"), sp(3, 1, 3, "c")])],
        "w",
    );
    let data = highlight(
        vec![sealed_source(dir.path(), &wal, "w")],
        vec![name_token("b"), name_token("c")],
        Some(Box::new(Cut4 { budget: 2 })),
    );
    assert_eq!(span_names(&data), ["a", "b"]);
    assert_eq!(data.matched, [1]);
    assert!(data.status.has(PartialReason::ByteBudget));
}

/// Without tokens nothing is marked; negated, trace-level and `trace_id`
/// tokens select no span; an invalid token is a request error.
#[test]
fn highlight_without_tokens_is_empty() {
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), vec![req(&[sp(1, 0, 1, "a")])], "w");
    let src = || vec![sealed_source(dir.path(), &wal, "w")];
    assert!(run(src()).matched.is_empty());
    let root = token(
        sfsq::traces::PredicateTarget::Builtin(sfsq::traces::BuiltinField::RootName),
        sfsq::traces::CompareOp::Eq,
        "a",
    );
    let negated = sfsq::traces::Token::negated(name_token("a").group);
    let id_hex: String = TRACE.iter().map(|b| format!("{b:02x}")).collect();
    let by_id = token(
        sfsq::traces::PredicateTarget::Builtin(sfsq::traces::BuiltinField::TraceId),
        sfsq::traces::CompareOp::Eq,
        &id_hex,
    );
    let data = highlight(src(), vec![root, negated, by_id], None);
    assert!(data.matched.is_empty());
    let short = token(
        sfsq::traces::PredicateTarget::FreeText,
        sfsq::traces::CompareOp::Contains,
        "a",
    );
    let err = trace_by_id(
        src(),
        TraceQuery::new(sfst::TraceId::from(TRACE)).highlight(sfsq::traces::TokenQuery {
            tokens: vec![short],
        }),
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap_err();
    assert!(
        matches!(err, sfsq::traces::TraceRequestError::Highlight(_)),
        "{err}"
    );
}

/// A span's resource attributes lead its fields, in the request's order,
/// from a sealed file and from the WAL tail alike (resource, then scope,
/// then span entries). The `otel-traces` answer sends each resource once
/// and a client rebuilds a span's fields by putting its resource's pairs
/// first, so this order is what makes that rebuild exact.
#[test]
fn resource_pairs_lead_a_spans_fields_on_every_source() {
    let dir = tempfile::tempdir().unwrap();
    let resource = vec![
        kv_str("service.name", "svc"),
        kv_str("host.name", "h"),
        kv_int("process.pid", 7),
        kv_str("deployment.environment", "prod"),
    ];
    let mut root = sp(1, 0, 1_000, "root");
    root.attrs = vec![kv_str("http.method", "GET")];
    let mut child = sp(2, 1, 2_000, "child");
    child.attrs = vec![kv_str("db.system", "pg")];
    let request = || {
        req_with(
            resource.clone(),
            Some(("lib", "1.0", vec![kv_str("tier", "web")])),
            &[root.clone(), child.clone()],
        )
    };
    let sealed = write_wal(dir.path(), vec![request()], "sealed");
    let tail = write_wal(dir.path(), vec![request()], "tail");
    let want = [
        "resource.attributes.service.name",
        "resource.attributes.host.name",
        "resource.attributes.process.pid",
        "resource.attributes.deployment.environment",
    ];
    for (label, source) in [
        ("sealed", sealed_source(dir.path(), &sealed, "sealed")),
        ("tail", tail_source(&tail, "tail")),
    ] {
        let data = run(vec![source]);
        assert_eq!(data.trace.spans.len(), 2, "{label}");
        for span in &data.trace.spans {
            let names: Vec<&str> = span.fields.iter().map(|(k, _)| k.as_str()).collect();
            assert_eq!(names[..want.len()], want, "{label}: {names:?}");
            assert!(
                !names[want.len()..]
                    .iter()
                    .any(|name| name.starts_with("resource.")),
                "{label}: {names:?}"
            );
        }
    }
}
