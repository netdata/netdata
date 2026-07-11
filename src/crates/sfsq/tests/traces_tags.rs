//! Tag / tag-value enumeration acceptance suite (phase 4b).
//!
//! The core criteria: keys and values come back as the typed neutral
//! vocabulary, partitioned by scope, deterministically under source-order
//! permutation, merged across all dictionary tiers and the tail's pair
//! table, with exact truncation flags — never touching sealed span rows
//! (the access-pattern proof lives in sfst's unit tests, next to the
//! dictionary reader). Plus the status honesty and request-validation
//! contracts (pins C1-C4).

mod common;

use std::sync::Arc;
use std::sync::atomic::AtomicUsize;

use tokio_util::sync::CancellationToken;

use common::{
    kv_double, kv_int, kv_null, kv_str, memory_source, req, req_with, sealed_source, sp,
    tail_source, write_wal,
};
use sfsq::Source;
use sfsq::traces::{
    PartialReason, QueryStatus, SourceId, TagKey, TagNamesQuery, TagRequestError, TagScope,
    TagValuesQuery, TimeWindow, TraceIntrinsic, TraceSfstCandidate, TraceSource, WalCoverage,
    tag_names, tag_values,
};

fn names(sources: Vec<TraceSource>, query: TagNamesQuery) -> sfsq::traces::TagNamesData {
    tag_names(
        sources,
        query,
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
    .expect("valid request")
}

fn values(sources: Vec<TraceSource>, query: TagValuesQuery) -> sfsq::traces::TagValuesData {
    tag_values(
        sources,
        query,
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
    .expect("valid request")
}

fn value_strings(data: &sfsq::traces::TagValuesData) -> Vec<&str> {
    data.values.iter().map(|v| v.value.as_str()).collect()
}

/// Attribute keys of one scope, as bare strings.
fn scope_attrs(data: &sfsq::traces::TagNamesData, scope: TagScope) -> Vec<&str> {
    data.keys
        .iter()
        .filter(|(s, _)| *s == scope)
        .filter_map(|(_, k)| match k {
            TagKey::Attribute(a) => Some(a.as_str()),
            TagKey::Intrinsic(_) => None,
        })
        .collect()
}

/// Keys spread across every scope and all three source shapes, with
/// mixed part_keys (distinct meta tags — D7): the key set must partition
/// by scope, include the full static intrinsic set, exclude the internal
/// facets and `trace_state`, and be identical under source permutations.
#[test]
fn keys_partition_scopes_deterministically_under_permutation() {
    let dir = tempfile::tempdir().unwrap();
    // Sealed A: resource + scope attrs; a span attr; kind/status set; a
    // trace_state AND a span attribute literally named trace_state.
    let mut a1 = sp(1, 0, 1_000, "op-a");
    a1.kind = 2;
    a1.status = Some((2, "boom"));
    a1.trace_state = "ot=th:8";
    a1.attrs = vec![kv_str("trace_state", "user-attr"), kv_int("retries", 3)];
    let wal_a = write_wal(
        dir.path(),
        vec![req_with(
            vec![kv_str("service.name", "svc-a"), kv_str("host", "h1")],
            Some(("mylib", "1.2", vec![kv_str("lang", "rust")])),
            &[a1],
        )],
        "part-a",
    );
    // Memory chunk B: an event with typed attrs and a link with attrs.
    let mut b1 = sp(2, 1, 2_000, "op-b");
    b1.events = vec![("retry", vec![kv_int("attempt", 1)])];
    b1.links = vec![([0xCD; 16], [9; 8], vec![kv_str("rel", "follows")])];
    let wal_b = write_wal(dir.path(), vec![req(&[b1])], "part-b");
    // Tail C: its own span attr.
    let mut c1 = sp(3, 1, 3_000, "op-c");
    c1.attrs = vec![kv_double("ratio", 0.5)];
    let wal_c = write_wal(dir.path(), vec![req(&[c1])], "part-c");

    let build = || {
        vec![
            sealed_source(dir.path(), &wal_a, "sealed-a"),
            memory_source(&wal_b, "chunk-b"),
            tail_source(&wal_c, "tail-c"),
        ]
    };

    let data = names(build(), TagNamesQuery::new());
    assert_eq!(data.status, QueryStatus::Complete);
    assert!(!data.truncated);

    assert_eq!(scope_attrs(&data, TagScope::Resource), ["host", "service.name"]);
    // The span attribute named trace_state IS vocabulary (typed keys
    // cannot collide with the excluded storage intrinsic).
    assert_eq!(
        scope_attrs(&data, TagScope::Span),
        ["ratio", "retries", "trace_state"]
    );
    assert_eq!(scope_attrs(&data, TagScope::Instrumentation), ["lang"]);
    assert_eq!(scope_attrs(&data, TagScope::Event), ["attempt"]);
    assert_eq!(scope_attrs(&data, TagScope::Link), ["rel"]);

    // The Intrinsic scope is the full static set (18B) and holds no
    // attributes; the internal facets never surface anywhere.
    let intrinsics: Vec<TraceIntrinsic> = data
        .keys
        .iter()
        .filter(|(s, _)| *s == TagScope::Intrinsic)
        .map(|(_, k)| match k {
            TagKey::Intrinsic(i) => *i,
            TagKey::Attribute(a) => panic!("attribute {a:?} under Intrinsic"),
        })
        .collect();
    let mut want: Vec<TraceIntrinsic> = TraceIntrinsic::ALL.to_vec();
    want.sort();
    assert_eq!(intrinsics, want);
    for (_, key) in &data.keys {
        if let TagKey::Attribute(a) = key {
            assert!(
                a != "_kind" && a != "_status_code",
                "internal facet {a:?} leaked into tags"
            );
        }
    }

    // Deterministic under permutation and scope-filterable.
    let mut sources = build();
    sources.rotate_left(1);
    let rotated = names(sources, TagNamesQuery::new());
    assert_eq!(data.keys, rotated.keys);
    let only_span = names(build(), TagNamesQuery::new().scope(TagScope::Span));
    assert!(only_span.keys.iter().all(|(s, _)| *s == TagScope::Span));
    assert_eq!(scope_attrs(&only_span, TagScope::Span), ["ratio", "retries", "trace_state"]);
}

/// Values merge across a low-tier file, a mid-tier file (>100 distinct
/// values), a high-tier file (>1000 distinct values), and the tail's
/// pair table — deduplicated, sorted by value bytes, with exact
/// truncation; subsumes list-services (resource `service.name` values).
#[test]
fn values_merge_all_tiers_across_sources_with_exact_truncation() {
    let dir = tempfile::tempdir().unwrap();
    let mk = |vals: std::ops::Range<usize>, field: &'static str| -> Vec<common::SpanSpec> {
        vals.map(|i| {
            let mut s = sp((i % 200) as u8 + 1, 0, 1_000 + i as u64, "op");
            let val = format!("v{i:04}");
            s.attrs = vec![match field {
                "v" => kv_str("v", &val),
                _ => kv_str("hi", &val),
            }];
            s
        })
        .collect()
    };

    // A: low tier (60 distinct v). B: mid tier (120 distinct v,
    // overlapping A). C: high tier (1010 distinct hi). Tail: v1000..v1019
    // (20 more v values).
    let wal_a = write_wal(dir.path(), vec![req(&mk(0..60, "v"))], "a");
    let wal_b = write_wal(dir.path(), vec![req(&mk(0..120, "v"))], "b");
    let wal_c = write_wal(dir.path(), vec![req(&mk(0..1010, "hi"))], "c");
    let wal_t = write_wal(dir.path(), vec![req(&mk(1000..1020, "v"))], "t");

    let sources = || {
        vec![
            sealed_source(dir.path(), &wal_a, "low"),
            sealed_source(dir.path(), &wal_b, "mid"),
            sealed_source(dir.path(), &wal_c, "high"),
            tail_source(&wal_t, "tail"),
        ]
    };

    // Union of v: v0000..v0119 ∪ v1000..v1019 = 140 values, sorted.
    let q = || TagValuesQuery::new(TagScope::Span, TagKey::Attribute("v".into()));
    let data = values(sources(), q());
    assert_eq!(data.status, QueryStatus::Complete);
    assert!(!data.truncated);
    let got = value_strings(&data);
    assert_eq!(got.len(), 140);
    assert_eq!(got[0], "v0000");
    assert_eq!(got[119], "v0119");
    assert_eq!(got[120], "v1000");
    assert_eq!(got[139], "v1019");
    let mut sorted = got.clone();
    sorted.sort_unstable();
    assert_eq!(got, sorted, "values must be sorted by value bytes");

    // High tier enumerates through the arena.
    let hi = values(
        sources(),
        TagValuesQuery::new(TagScope::Span, TagKey::Attribute("hi".into())),
    );
    assert_eq!(hi.values.len(), 1010);

    // Exact truncation: exactly-the-limit is NOT truncated; one less is.
    let exact = values(sources(), q().max_values(140));
    assert!(!exact.truncated);
    assert_eq!(exact.values.len(), 140);
    let cut = values(sources(), q().max_values(139));
    assert!(cut.truncated);
    assert_eq!(cut.values.len(), 139);
    assert_eq!(cut.values.last().unwrap().value, "v1018");

    // list-services: resource service.name values across the corpus.
    let svc = values(
        sources(),
        TagValuesQuery::new(TagScope::Resource, TagKey::Attribute("service.name".into())),
    );
    assert_eq!(value_strings(&svc), ["svc"]);
}

/// Kinds are field-coalesced across exactly the contributing sources
/// (Int ⊔ Double → Double, mixed → Str via the shared lattice), and a
/// null-only attribute is enumerated with `kind: None` (pin C1) — its
/// stored value is the empty rendering.
#[test]
fn kinds_coalesce_and_kindless_values_carry_none() {
    let dir = tempfile::tempdir().unwrap();
    let mk = |attr: opentelemetry_proto::tonic::common::v1::KeyValue, id: u8| {
        let mut s = sp(id, 0, 1_000 + id as u64, "op");
        s.attrs = vec![attr];
        s
    };
    let wal_int = write_wal(dir.path(), vec![req(&[mk(kv_int("port", 80), 1)])], "i");
    let wal_dbl = write_wal(dir.path(), vec![req(&[mk(kv_double("port", 8.5), 2)])], "d");
    let wal_str = write_wal(dir.path(), vec![req(&[mk(kv_str("port", "www"), 3)])], "s");
    let wal_null = write_wal(dir.path(), vec![req(&[mk(kv_null("ghost"), 4)])], "n");

    let port = |wals: Vec<TraceSource>| {
        values(
            wals,
            TagValuesQuery::new(TagScope::Span, TagKey::Attribute("port".into())),
        )
    };

    // Int ⊔ Double → Double (sealed + sealed).
    let d = port(vec![
        sealed_source(dir.path(), &wal_int, "int"),
        sealed_source(dir.path(), &wal_dbl, "dbl"),
    ]);
    assert_eq!(value_strings(&d), ["8.5", "80"]);
    assert!(d.values.iter().all(|v| v.kind == Some(sfst::ValueKind::Double)));

    // Int ⊔ Str → Str (sealed + TAIL — the tail folds through the same
    // lattice).
    let s = port(vec![
        sealed_source(dir.path(), &wal_int, "int2"),
        tail_source(&wal_str, "str-tail"),
    ]);
    assert_eq!(value_strings(&s), ["80", "www"]);
    assert!(s.values.iter().all(|v| v.kind == Some(sfst::ValueKind::Str)));

    // Null-only: enumerated as a key, value is the empty rendering,
    // kind None — from a sealed file AND from a tail.
    for source in [
        sealed_source(dir.path(), &wal_null, "null-sealed"),
        tail_source(&wal_null, "null-tail"),
    ] {
        let keys = names(vec![source], TagNamesQuery::new());
        // Rebuild the consumed source for the values call.
        assert!(
            scope_attrs(&keys, TagScope::Span).contains(&"ghost"),
            "null-only attr must stay vocabulary"
        );
    }
    let g = values(
        vec![sealed_source(dir.path(), &wal_null, "null-sealed-2")],
        TagValuesQuery::new(TagScope::Span, TagKey::Attribute("ghost".into())),
    );
    assert_eq!(value_strings(&g), [""]);
    assert_eq!(g.values[0].kind, None);
    let gt = values(
        vec![tail_source(&wal_null, "null-tail-2")],
        TagValuesQuery::new(TagScope::Span, TagKey::Attribute("ghost".into())),
    );
    assert_eq!(value_strings(&gt), [""]);
    assert_eq!(gt.values[0].kind, None);
}

/// Dictionary-backed intrinsics serve values (storage labels — the wire
/// adapter maps vocabulary, decision 19-dissolved); virtual intrinsics
/// are a request error (18B); the intrinsic key list never depends on
/// the data (a status-less corpus still lists `Status`).
#[test]
fn intrinsic_values_serve_and_virtual_intrinsics_reject() {
    let dir = tempfile::tempdir().unwrap();
    let mut a = sp(1, 0, 1_000, "op-a");
    a.kind = 2; // SERVER
    a.status = Some((2, "boom")); // ERROR
    a.events = vec![("retry", Vec::new())];
    let wal = write_wal(
        dir.path(),
        vec![req_with(
            vec![kv_str("service.name", "svc")],
            Some(("mylib", "1.2", Vec::new())),
            &[a],
        )],
        "i",
    );
    let src = || vec![sealed_source(dir.path(), &wal, "f")];
    let intr = |i: TraceIntrinsic| {
        values(
            src(),
            TagValuesQuery::new(TagScope::Intrinsic, TagKey::Intrinsic(i)),
        )
    };

    assert_eq!(value_strings(&intr(TraceIntrinsic::Name)), ["op-a"]);
    assert_eq!(value_strings(&intr(TraceIntrinsic::Kind)), ["SERVER"]);
    assert_eq!(value_strings(&intr(TraceIntrinsic::Status)), ["ERROR"]);
    assert_eq!(value_strings(&intr(TraceIntrinsic::StatusMessage)), ["boom"]);
    assert_eq!(
        value_strings(&intr(TraceIntrinsic::InstrumentationName)),
        ["mylib"]
    );
    assert_eq!(
        value_strings(&intr(TraceIntrinsic::InstrumentationVersion)),
        ["1.2"]
    );
    assert_eq!(value_strings(&intr(TraceIntrinsic::EventName)), ["retry"]);

    for virt in [
        TraceIntrinsic::Duration,
        TraceIntrinsic::SpanId,
        TraceIntrinsic::ParentSpanId,
        TraceIntrinsic::TraceId,
        TraceIntrinsic::LinkSpanId,
        TraceIntrinsic::LinkTraceId,
        TraceIntrinsic::EventTimeSinceStart,
        TraceIntrinsic::RootName,
        TraceIntrinsic::RootServiceName,
        TraceIntrinsic::TraceDuration,
    ] {
        let err = tag_values(
            src(),
            TagValuesQuery::new(TagScope::Intrinsic, TagKey::Intrinsic(virt)),
            CancellationToken::new(),
            Arc::new(AtomicUsize::new(0)),
        )
        .unwrap_err();
        assert!(
            matches!(err, TagRequestError::NotEnumerable(i) if i == virt),
            "virtual {virt:?} must be NotEnumerable"
        );
    }

    // A corpus with no statuses at all still lists Status (static set).
    let plain = write_wal(dir.path(), vec![req(&[sp(9, 0, 1, "bare")])], "p");
    let keys = names(
        vec![sealed_source(dir.path(), &plain, "bare")],
        TagNamesQuery::new().scope(TagScope::Intrinsic),
    );
    assert!(
        keys.keys
            .contains(&(TagScope::Intrinsic, TagKey::Intrinsic(TraceIntrinsic::Status)))
    );
    // But its VALUES are an empty Complete result — a data condition.
    let sv = values(
        vec![sealed_source(dir.path(), &plain, "bare2")],
        TagValuesQuery::new(TagScope::Intrinsic, TagKey::Intrinsic(TraceIntrinsic::Status)),
    );
    assert!(sv.values.is_empty());
    assert_eq!(sv.status, QueryStatus::Complete);
}

/// The optional window prunes SFST candidates by summary overlap
/// (span-start seconds, file-granular) and never prunes the tail; a
/// sub-second window inside a file's range still takes the whole file
/// (pin C3 conservatism).
#[test]
fn window_prunes_files_but_never_the_tail() {
    let dir = tempfile::tempdir().unwrap();
    const NS: u64 = 1_000_000_000;
    let mk = |start: u64, val: &'static str, id: u8| {
        let mut s = sp(id, 0, start, "op");
        s.attrs = vec![kv_str("who", val)];
        s
    };
    // File A around second 5; file B around second 100; tail C at 200.
    let wal_a = write_wal(dir.path(), vec![req(&[mk(5 * NS, "early", 1)])], "a");
    let wal_b = write_wal(dir.path(), vec![req(&[mk(100 * NS, "late", 2)])], "b");
    let wal_c = write_wal(dir.path(), vec![req(&[mk(200 * NS, "tail", 3)])], "c");
    let sources = || {
        vec![
            sealed_source(dir.path(), &wal_a, "a"),
            sealed_source(dir.path(), &wal_b, "b"),
            tail_source(&wal_c, "c"),
        ]
    };
    let who = |w: TimeWindow| {
        values(
            sources(),
            TagValuesQuery::new(TagScope::Span, TagKey::Attribute("who".into()))
                .window(w),
        )
    };

    // Window covering only file A: B pruned, tail always contributes.
    let early = who(TimeWindow::new(0, 10 * NS as i64).unwrap());
    assert_eq!(value_strings(&early), ["early", "tail"]);
    assert_eq!(early.status, QueryStatus::Complete);

    // Sub-second window inside A's second: still the whole file
    // (file-granular, conservative).
    let sub = who(TimeWindow::new((5 * NS + 10) as i64, (5 * NS + 20) as i64).unwrap());
    assert!(value_strings(&sub).contains(&"early"));

    // Window covering nothing sealed: only the tail contributes.
    let none = who(TimeWindow::new(300 * NS as i64, 400 * NS as i64).unwrap());
    assert_eq!(value_strings(&none), ["tail"]);

    // Keys are windowed the same way.
    let keys = names(
        sources(),
        TagNamesQuery::new()
            .scope(TagScope::Span)
            .window(TimeWindow::new(0, 10 * NS as i64).unwrap()),
    );
    assert_eq!(scope_attrs(&keys, TagScope::Span), ["who"]);
}

/// Failed sources are reported (`SourceFailure`) while the rest serve —
/// for both operations; the exact-truncated guarantee is then relative
/// to the observed sources (documented).
#[test]
fn failed_sources_reported_and_the_rest_served() {
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(
        dir.path(),
        vec![req(&[{
            let mut s = sp(1, 0, 1_000, "op");
            s.attrs = vec![kv_str("k", "v")];
            s
        }])],
        "ok",
    );
    let broken = || {
        vec![
            TraceSource::Sfst(TraceSfstCandidate {
                source_id: SourceId::new("garbage"),
                summary: sfst::Summary {
                    min_timestamp_s: 0,
                    max_timestamp_s: u32::MAX,
                    record_count: 0,
                    content_meta: Vec::new(),
                },
                source: Source::Memory(Arc::new(vec![0u8; 64])),
                coverage: Some(WalCoverage {
                    wal_id: "garbage-wal".into(),
                    range: wal::FrameRange::new(0, 64),
                }),
            }),
            sealed_source(dir.path(), &wal, "good"),
        ]
    };

    let keys = names(broken(), TagNamesQuery::new());
    assert!(keys.status.has(PartialReason::SourceFailure));
    assert_eq!(scope_attrs(&keys, TagScope::Span), ["k"]);

    let vals = values(
        broken(),
        TagValuesQuery::new(TagScope::Span, TagKey::Attribute("k".into())),
    );
    assert!(vals.status.has(PartialReason::SourceFailure));
    assert_eq!(value_strings(&vals), ["v"]);
}

/// Cancellation is ALL-OR-EMPTY (pin C2): a pre-cancelled token — even
/// with zero sources — yields an empty result with `Cancelled`, never a
/// Complete or per-source prefix.
#[test]
fn cancellation_is_all_or_empty() {
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), vec![req(&[sp(1, 0, 1, "x")])], "c");
    let cancel = CancellationToken::new();
    cancel.cancel();

    let keys = tag_names(
        vec![sealed_source(dir.path(), &wal, "f")],
        TagNamesQuery::new(),
        cancel.clone(),
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap();
    assert!(keys.keys.is_empty(), "no static intrinsics on cancellation");
    assert!(keys.status.has(PartialReason::Cancelled));

    let vals = tag_values(
        Vec::new(),
        TagValuesQuery::new(TagScope::Span, TagKey::Attribute("k".into())),
        cancel,
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap();
    assert!(vals.values.is_empty());
    assert!(vals.status.has(PartialReason::Cancelled));
}

/// A tag absent from every source is a data condition: empty values,
/// `Complete`, not truncated (21A).
#[test]
fn absent_key_is_a_complete_empty() {
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), vec![req(&[sp(1, 0, 1, "x")])], "a");
    let data = values(
        vec![
            sealed_source(dir.path(), &wal, "f"),
            tail_source(&wal, "t"),
        ],
        TagValuesQuery::new(TagScope::Link, TagKey::Attribute("nope".into())),
    );
    assert!(data.values.is_empty());
    assert!(!data.truncated);
    assert_eq!(data.status, QueryStatus::Complete);
}

/// Request validation: zero limits, an inverted window, the invalid
/// (scope, key) pairs of pin C4, and source-set hygiene are errors —
/// nothing is queried.
#[test]
fn request_validation_rejects_bad_requests() {
    let cancel = CancellationToken::new;
    let counter = || Arc::new(AtomicUsize::new(0));

    assert!(matches!(
        tag_names(Vec::new(), TagNamesQuery::new().max_keys(0), cancel(), counter()),
        Err(TagRequestError::ZeroLimit)
    ));
    assert!(matches!(
        tag_values(
            Vec::new(),
            TagValuesQuery::new(TagScope::Span, TagKey::Attribute("k".into())).max_values(0),
            cancel(),
            counter()
        ),
        Err(TagRequestError::ZeroLimit)
    ));
    assert!(matches!(
        TimeWindow::new(5, 5),
        Err(TagRequestError::InvalidWindow { .. })
    ));
    assert!(matches!(
        TimeWindow::new(9, 3),
        Err(TagRequestError::InvalidWindow { .. })
    ));

    // Pin C4: intrinsic key outside the Intrinsic scope…
    assert!(matches!(
        tag_values(
            Vec::new(),
            TagValuesQuery::new(TagScope::Span, TagKey::Intrinsic(TraceIntrinsic::Name)),
            cancel(),
            counter()
        ),
        Err(TagRequestError::IntrinsicKeyOutsideIntrinsicScope(TagScope::Span))
    ));
    // …and attribute keys inside it.
    assert!(matches!(
        tag_values(
            Vec::new(),
            TagValuesQuery::new(TagScope::Intrinsic, TagKey::Attribute("k".into())),
            cancel(),
            counter()
        ),
        Err(TagRequestError::AttributeKeyInIntrinsicScope(a)) if a == "k"
    ));

    // Source-set hygiene runs on every operation.
    let dup = |id: &str| {
        TraceSource::Sfst(TraceSfstCandidate {
            source_id: SourceId::new(id.to_string()),
            summary: sfst::Summary {
                min_timestamp_s: 0,
                max_timestamp_s: 0,
                record_count: 0,
                content_meta: Vec::new(),
            },
            source: Source::File("/dev/null".into()),
            coverage: None,
        })
    };
    assert!(matches!(
        tag_names(
            vec![dup("same"), dup("same")],
            TagNamesQuery::new(),
            cancel(),
            counter()
        ),
        Err(TagRequestError::SourceSet(_))
    ));
}

/// An empty source set with a live token is a Complete result: the
/// static intrinsic vocabulary (18B) does not depend on sources
/// existing, and attribute scopes are simply empty — pinned so a future
/// change cannot gate the static set on "saw a source".
#[test]
fn empty_sources_still_yield_the_static_intrinsics() {
    let data = names(Vec::new(), TagNamesQuery::new());
    assert_eq!(data.status, QueryStatus::Complete);
    assert!(!data.truncated);
    let mut want: Vec<TraceIntrinsic> = TraceIntrinsic::ALL.to_vec();
    want.sort();
    let got: Vec<TraceIntrinsic> = data
        .keys
        .iter()
        .map(|(s, k)| match (s, k) {
            (TagScope::Intrinsic, TagKey::Intrinsic(i)) => *i,
            other => panic!("only static intrinsics expected, got {other:?}"),
        })
        .collect();
    assert_eq!(got, want);

    // A non-Intrinsic scope filter on zero sources: zero keys, Complete.
    let span_only = names(Vec::new(), TagNamesQuery::new().scope(TagScope::Span));
    assert!(span_only.keys.is_empty());
    assert_eq!(span_only.status, QueryStatus::Complete);

    // Values on zero sources: an empty Complete (data condition).
    let vals = values(
        Vec::new(),
        TagValuesQuery::new(TagScope::Span, TagKey::Attribute("k".into())),
    );
    assert!(vals.values.is_empty());
    assert_eq!(vals.status, QueryStatus::Complete);
}
