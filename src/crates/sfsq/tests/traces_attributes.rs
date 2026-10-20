//! Acceptance suite for sfsq's attribute / attribute-value enumeration —
//! the phase-4b `attribute_names` / `attribute_values` operations (engine:
//! src/traces/attributes.rs), driven end to end over real corpora. Tests
//! that need data write fresh traces WALs and offer it in the three source
//! shapes of tests/common/mod.rs — a sealed SFST file, an in-memory chunk,
//! and the WAL tail — plus the two failure shapes (a missing file, an
//! unavailable remote).
//!
//! Pinned contracts (each citing the engine symbol that owns it):
//!
//! - Keys come back as the typed, wire-neutral vocabulary: sorted,
//!   partitioned by owner (each key under exactly one), the full static
//!   builtin set present regardless of data (seeded up front by
//!   `sfsq::traces::attribute_names`), and the internal
//!   facets (`_kind`/`_status_code`) plus the bare `trace_state` field
//!   never surfacing (`sfsq::traces::storage_to_attribute`) — while a
//!   span ATTRIBUTE literally named `trace_state` does.
//! - Values merge the dictionaries of every tier and the tail's pair
//!   table: deduplicated, sorted by value bytes, one exact `truncated`
//!   flag (`sfsq::traces::attribute_values`). Merge-then-limit makes
//!   source order irrelevant (the same rule in both entry points), so
//!   results are identical under permutation.
//! - Cancellation is ALL-OR-EMPTY (pin C2; the cancel checks at the head
//!   of `sfsq::traces::attribute_names` and
//!   `sfsq::traces::attribute_values`): an empty result
//!   with `Cancelled` — even the static builtins are withheld.
//! - The optional window prunes sealed files and unavailable sources by
//!   summary overlap, file-granular and conservative (pin C3; `pruned`
//!   in `src/traces/attributes.rs` guards both source kinds); the tail
//!   is never pruned.
//! - Failure honesty (src/traces/status.rs): a source that fails is a
//!   `SourceFailure` reason, an in-window unavailable remote a distinct
//!   `RemoteUnavailable` — never a silent skip; the rest still serve.
//! - Bad requests (zero limits, pin C4 owner/key pairings, duplicate
//!   source ids) are errors before anything is queried — the validation
//!   heads of `sfsq::traces::attribute_names` and
//!   `sfsq::traces::attribute_values`.
//! - A null-only attribute stays in the vocabulary, its values carrying
//!   `kind: None` (pin C1; `sfsq::traces::storage_to_attribute` keeps
//!   the key, `sfsq::traces::attribute_values` folds `kind` to `None`).
//!
//! Not pinned here: WHICH storage chunks the enumeration reads — the
//! suite sees results, not access paths (sfst pins per-tier dictionary
//! enumeration next to the reader:
//! `field_values_per_tier_prefix_stripped_by_length` in
//! `sfst/src/tests/materialize.rs`);
//! values under the Event/Link owners (only their keys are enumerated).

mod common;

use std::collections::BTreeSet;
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

use tokio_util::sync::CancellationToken;

use common::{
    SpanSpec, kv_double, kv_int, kv_null, kv_str, memory_source, missing_source, req, req_with,
    sealed_source, sp, tail_source, unavailable_source, write_wal,
};
use sfsq::Source;
use sfsq::traces::{
    AttributeKey, AttributeNamesQuery, AttributeOwner, AttributeRequestError, AttributeValuesQuery,
    BuiltinField, PartialReason, QueryStatus, SourceId, TimeWindow, TraceSfstCandidate,
    TraceSource, WalCoverage, attribute_names, attribute_values,
};

/// Run `attribute_names` with a live token and a throwaway progress
/// counter; panics on request-validation errors (they have their own
/// test).
fn names(
    sources: Vec<TraceSource>,
    query: AttributeNamesQuery,
) -> sfsq::traces::AttributeNamesData {
    attribute_names(
        sources,
        query,
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
    .expect("valid request")
}

/// Run `attribute_values` under the same terms as `names`.
fn values(
    sources: Vec<TraceSource>,
    query: AttributeValuesQuery,
) -> sfsq::traces::AttributeValuesData {
    attribute_values(
        sources,
        query,
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
    .expect("valid request")
}

/// The value strings of a values result, in result order.
fn value_strings(data: &sfsq::traces::AttributeValuesData) -> Vec<&str> {
    data.values.iter().map(|v| v.value.as_str()).collect()
}

/// An owner's attribute keys as bare strings (builtin keys dropped).
fn owner_attrs(data: &sfsq::traces::AttributeNamesData, owner: AttributeOwner) -> Vec<&str> {
    data.keys
        .iter()
        .filter(|(o, _)| *o == owner)
        .filter_map(|(_, k)| match k {
            AttributeKey::Attribute(a) => Some(a.as_str()),
            AttributeKey::Builtin(_) => None,
        })
        .collect()
}

/// Keys spread across every scope and all three source shapes, written
/// as separate WALs under distinct per-WAL meta_tags (content_meta —
/// the engine must not care): the key set must partition by owner,
/// include the full static builtin set, exclude the internal facets
/// and the bare `trace_state` field, and be identical under source
/// permutations.
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

    let data = names(build(), AttributeNamesQuery::new());
    assert_eq!(data.status, QueryStatus::Complete);
    assert!(!data.truncated);

    assert_eq!(
        owner_attrs(&data, AttributeOwner::Resource),
        ["host", "service.name"]
    );
    // The span attribute named trace_state IS vocabulary: the
    // exclusion drops only the BARE `trace_state` storage field
    // (`storage_to_attribute` in `src/traces/vocab.rs`), so typed keys
    // cannot collide.
    assert_eq!(
        owner_attrs(&data, AttributeOwner::Span),
        ["ratio", "retries", "trace_state"]
    );
    assert_eq!(
        owner_attrs(&data, AttributeOwner::Instrumentation),
        ["lang"]
    );
    assert_eq!(owner_attrs(&data, AttributeOwner::Event), ["attempt"]);
    assert_eq!(owner_attrs(&data, AttributeOwner::Link), ["rel"]);

    // The Builtin owner is the full static set (`BuiltinField::ALL` in
    // `src/traces/vocab.rs`)
    // and holds no attributes; the internal facets never surface
    // anywhere.
    let builtins: Vec<BuiltinField> = data
        .keys
        .iter()
        .filter(|(s, _)| *s == AttributeOwner::Builtin)
        .map(|(_, k)| match k {
            AttributeKey::Builtin(i) => *i,
            AttributeKey::Attribute(a) => panic!("attribute {a:?} under Builtin"),
        })
        .collect();
    let mut want: Vec<BuiltinField> = BuiltinField::ALL.to_vec();
    want.sort();
    assert_eq!(builtins, want);
    for (_, key) in &data.keys {
        if let AttributeKey::Attribute(a) = key {
            assert!(
                a != "_kind" && a != "_status_code",
                "internal facet {a:?} leaked into the key vocabulary"
            );
        }
    }

    // Deterministic under permutation and scope-filterable.
    let mut sources = build();
    sources.rotate_left(1);
    let rotated = names(sources, AttributeNamesQuery::new());
    assert_eq!(data.keys, rotated.keys);
    let only_span = names(
        build(),
        AttributeNamesQuery::new().owner(AttributeOwner::Span),
    );
    assert!(
        only_span
            .keys
            .iter()
            .all(|(s, _)| *s == AttributeOwner::Span)
    );
    assert_eq!(
        owner_attrs(&only_span, AttributeOwner::Span),
        ["ratio", "retries", "trace_state"]
    );
}

/// Values merge across a low-tier file, a mid-tier file (>100 distinct
/// values), a high-tier file (>1000 — the tier cutoffs are cardinality
/// thresholds: `FieldTier` / `DEFAULT_CARDINALITY_THRESHOLD` in
/// `sfst/src/schema.rs`), and the tail's pair table:
/// deduplicated, sorted by value bytes, exact truncation. Also pins the
/// resource `service.name` values — the service-list lookup.
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
    let q = || AttributeValuesQuery::new(AttributeOwner::Span, AttributeKey::Attribute("v".into()));
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

    // High tier enumerates through the HF arena
    // (`IndexReader::field_values` in `sfst/src/index_reader.rs`).
    let hi = values(
        sources(),
        AttributeValuesQuery::new(AttributeOwner::Span, AttributeKey::Attribute("hi".into())),
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

    // The service-list lookup: resource service.name across the whole
    // corpus — four sources all carrying `svc` collapse to one value.
    let svc = values(
        sources(),
        AttributeValuesQuery::new(
            AttributeOwner::Resource,
            AttributeKey::Attribute("service.name".into()),
        ),
    );
    assert_eq!(value_strings(&svc), ["svc"]);
}

/// Kinds are field-coalesced across exactly the contributing sources
/// (Int ⊔ Double → Double, Int ⊔ Str → Str via sfst's shared lattice —
/// `join_value_kinds` in `sfst/src/schema.rs`), and a null-only
/// attribute stays in the vocabulary with `kind: None` (pin C1) — its
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
            AttributeValuesQuery::new(AttributeOwner::Span, AttributeKey::Attribute("port".into())),
        )
    };

    // Int ⊔ Double → Double (sealed + sealed).
    let d = port(vec![
        sealed_source(dir.path(), &wal_int, "int"),
        sealed_source(dir.path(), &wal_dbl, "dbl"),
    ]);
    assert_eq!(value_strings(&d), ["8.5", "80"]);
    assert!(
        d.values
            .iter()
            .all(|v| v.kind == Some(sfst::ValueKind::Double))
    );

    // Int ⊔ Str → Str (sealed + TAIL — the tail folds through the same
    // lattice).
    let s = port(vec![
        sealed_source(dir.path(), &wal_int, "int2"),
        tail_source(&wal_str, "str-tail"),
    ]);
    assert_eq!(value_strings(&s), ["80", "www"]);
    assert!(
        s.values
            .iter()
            .all(|v| v.kind == Some(sfst::ValueKind::Str))
    );

    // Null-only: enumerated as a key, value is the empty rendering,
    // kind None — from a sealed file AND from a tail.
    for source in [
        sealed_source(dir.path(), &wal_null, "null-sealed"),
        tail_source(&wal_null, "null-tail"),
    ] {
        let keys = names(vec![source], AttributeNamesQuery::new());
        // Every call consumes its source vec, so each iteration (and
        // each values check below) builds fresh sources.
        assert!(
            owner_attrs(&keys, AttributeOwner::Span).contains(&"ghost"),
            "null-only attr must stay vocabulary"
        );
    }
    let g = values(
        vec![sealed_source(dir.path(), &wal_null, "null-sealed-2")],
        AttributeValuesQuery::new(
            AttributeOwner::Span,
            AttributeKey::Attribute("ghost".into()),
        ),
    );
    assert_eq!(value_strings(&g), [""]);
    assert_eq!(g.values[0].kind, None);
    let gt = values(
        vec![tail_source(&wal_null, "null-tail-2")],
        AttributeValuesQuery::new(
            AttributeOwner::Span,
            AttributeKey::Attribute("ghost".into()),
        ),
    );
    assert_eq!(value_strings(&gt), [""]);
    assert_eq!(gt.values[0].kind, None);
}

/// Dictionary-backed builtins serve values as the STORAGE labels
/// (mapping them to a wire vocabulary is the adapter's job —
/// `sfsq::traces::BuiltinField::dictionary_field`); virtual builtins
/// are a request error
/// (the virtual/dictionary split is static); the builtin key list never
/// depends on the data — a status-less corpus still lists `Status`.
#[test]
fn builtin_values_serve_and_virtual_builtins_reject() {
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
    let intr = |i: BuiltinField| {
        values(
            src(),
            AttributeValuesQuery::new(AttributeOwner::Builtin, AttributeKey::Builtin(i)),
        )
    };

    assert_eq!(value_strings(&intr(BuiltinField::Name)), ["op-a"]);
    assert_eq!(value_strings(&intr(BuiltinField::Kind)), ["SERVER"]);
    assert_eq!(value_strings(&intr(BuiltinField::Status)), ["ERROR"]);
    assert_eq!(value_strings(&intr(BuiltinField::StatusMessage)), ["boom"]);
    assert_eq!(
        value_strings(&intr(BuiltinField::InstrumentationName)),
        ["mylib"]
    );
    assert_eq!(
        value_strings(&intr(BuiltinField::InstrumentationVersion)),
        ["1.2"]
    );
    assert_eq!(value_strings(&intr(BuiltinField::EventName)), ["retry"]);

    for virt in [
        BuiltinField::Duration,
        BuiltinField::SpanId,
        BuiltinField::ParentSpanId,
        BuiltinField::TraceId,
        BuiltinField::LinkSpanId,
        BuiltinField::LinkTraceId,
        BuiltinField::EventTimeSinceStart,
        BuiltinField::RootName,
        BuiltinField::RootServiceName,
        BuiltinField::TraceDuration,
    ] {
        let err = attribute_values(
            src(),
            AttributeValuesQuery::new(AttributeOwner::Builtin, AttributeKey::Builtin(virt)),
            CancellationToken::new(),
            Arc::new(AtomicUsize::new(0)),
        )
        .unwrap_err();
        assert!(
            matches!(err, AttributeRequestError::NotEnumerable(i) if i == virt),
            "virtual {virt:?} must be NotEnumerable"
        );
    }

    // A corpus with no statuses at all still lists Status (static set).
    let plain = write_wal(dir.path(), vec![req(&[sp(9, 0, 1, "bare")])], "p");
    let keys = names(
        vec![sealed_source(dir.path(), &plain, "bare")],
        AttributeNamesQuery::new().owner(AttributeOwner::Builtin),
    );
    assert!(keys.keys.contains(&(
        AttributeOwner::Builtin,
        AttributeKey::Builtin(BuiltinField::Status)
    )));
    // Its spans were sent without a status or kind: the OTel defaults are
    // stored, so they are the values.
    let sv = values(
        vec![sealed_source(dir.path(), &plain, "bare2")],
        AttributeValuesQuery::new(
            AttributeOwner::Builtin,
            AttributeKey::Builtin(BuiltinField::Status),
        ),
    );
    assert_eq!(value_strings(&sv), ["UNSET"]);
    assert_eq!(sv.status, QueryStatus::Complete);
    let kv = values(
        vec![sealed_source(dir.path(), &plain, "bare3")],
        AttributeValuesQuery::new(
            AttributeOwner::Builtin,
            AttributeKey::Builtin(BuiltinField::Kind),
        ),
    );
    assert_eq!(value_strings(&kv), ["UNSPECIFIED"]);

    // Defaults sit beside explicit values in one sorted list.
    let mut ok = sp(2, 0, 2_000, "op-ok");
    ok.status = Some((1, ""));
    let mixed = write_wal(
        dir.path(),
        vec![req(&[a_error_server(), ok, sp(3, 0, 3_000, "op-default")])],
        "m",
    );
    let status = values(
        vec![sealed_source(dir.path(), &mixed, "mixed-status")],
        AttributeValuesQuery::new(
            AttributeOwner::Builtin,
            AttributeKey::Builtin(BuiltinField::Status),
        ),
    );
    assert_eq!(value_strings(&status), ["ERROR", "OK", "UNSET"]);
    let kind = values(
        vec![sealed_source(dir.path(), &mixed, "mixed-kind")],
        AttributeValuesQuery::new(
            AttributeOwner::Builtin,
            AttributeKey::Builtin(BuiltinField::Kind),
        ),
    );
    assert_eq!(value_strings(&kind), ["SERVER", "UNSPECIFIED"]);
}

/// A SERVER-kind span with ERROR status and a status message — reused
/// by the mixed-defaults corpus below.
fn a_error_server() -> SpanSpec {
    let mut a = sp(1, 0, 1_000, "op-a");
    a.kind = 2; // SERVER
    a.status = Some((2, "boom")); // ERROR
    a
}

/// The optional window prunes SFST candidates by summary overlap
/// (span-start seconds expanded to nanoseconds, file-granular) and
/// never prunes the tail; a sub-second window inside a file's range
/// still takes the whole file (pin C3 conservatism; `pruned` in
/// `src/traces/attributes.rs`).
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
            AttributeValuesQuery::new(AttributeOwner::Span, AttributeKey::Attribute("who".into()))
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
        AttributeNamesQuery::new()
            .owner(AttributeOwner::Span)
            .window(TimeWindow::new(0, 10 * NS as i64).unwrap()),
    );
    assert_eq!(owner_attrs(&keys, AttributeOwner::Span), ["who"]);
}

/// A source whose bytes fail to parse is a `SourceFailure` reason while
/// the healthy source still serves — for both operations. The engine
/// documents exact `truncated` as relative to the observed sources
/// under a Partial status (the `src/traces/attributes.rs` module
/// docs); not pinned here — no limits are set.
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
    // A hand-built candidate over 64 zero bytes: no valid SFST, so
    // IndexReader::open fails → SourceFailure.
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

    let keys = names(broken(), AttributeNamesQuery::new());
    assert!(keys.status.has(PartialReason::SourceFailure));
    assert_eq!(owner_attrs(&keys, AttributeOwner::Span), ["k"]);

    let vals = values(
        broken(),
        AttributeValuesQuery::new(AttributeOwner::Span, AttributeKey::Attribute("k".into())),
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

    let keys = attribute_names(
        vec![sealed_source(dir.path(), &wal, "f")],
        AttributeNamesQuery::new(),
        cancel.clone(),
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap();
    assert!(keys.keys.is_empty(), "no static builtins on cancellation");
    assert!(keys.status.has(PartialReason::Cancelled));

    let vals = attribute_values(
        Vec::new(),
        AttributeValuesQuery::new(AttributeOwner::Span, AttributeKey::Attribute("k".into())),
        cancel,
        Arc::new(AtomicUsize::new(0)),
    )
    .unwrap();
    assert!(vals.values.is_empty());
    assert!(vals.status.has(PartialReason::Cancelled));
}

/// A key absent from every source is a data condition, not an error:
/// empty values, `Complete`, not truncated (the engine skips files
/// without the field — `sfsq::traces::attribute_values`).
#[test]
fn absent_key_is_a_complete_empty() {
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), vec![req(&[sp(1, 0, 1, "x")])], "a");
    let data = values(
        vec![sealed_source(dir.path(), &wal, "f"), tail_source(&wal, "t")],
        AttributeValuesQuery::new(AttributeOwner::Link, AttributeKey::Attribute("nope".into())),
    );
    assert!(data.values.is_empty());
    assert!(!data.truncated);
    assert_eq!(data.status, QueryStatus::Complete);
}

/// Request validation: zero limits, the invalid owner/key pairings of
/// pin C4, and duplicate source ids are errors before anything is
/// queried (the validation heads of `sfsq::traces::attribute_names` and
/// `sfsq::traces::attribute_values`); an empty or inverted window is
/// rejected earlier still, by `TimeWindow::new`.
#[test]
fn request_validation_rejects_bad_requests() {
    let cancel = CancellationToken::new;
    let counter = || Arc::new(AtomicUsize::new(0));

    assert!(matches!(
        attribute_names(
            Vec::new(),
            AttributeNamesQuery::new().max_keys(0),
            cancel(),
            counter()
        ),
        Err(AttributeRequestError::ZeroLimit)
    ));
    assert!(matches!(
        attribute_values(
            Vec::new(),
            AttributeValuesQuery::new(AttributeOwner::Span, AttributeKey::Attribute("k".into()))
                .max_values(0),
            cancel(),
            counter()
        ),
        Err(AttributeRequestError::ZeroLimit)
    ));
    assert!(matches!(
        TimeWindow::new(5, 5),
        Err(sfsq::traces::WindowError::Invalid { .. })
    ));
    assert!(matches!(
        TimeWindow::new(9, 3),
        Err(sfsq::traces::WindowError::Invalid { .. })
    ));

    // `Any` is a predicate construct; enumeration takes a concrete owner.
    assert!(matches!(
        attribute_names(
            Vec::new(),
            AttributeNamesQuery::new().owner(AttributeOwner::Any),
            cancel(),
            counter()
        ),
        Err(AttributeRequestError::AnyOwnerNotEnumerable)
    ));
    assert!(matches!(
        attribute_values(
            Vec::new(),
            AttributeValuesQuery::new(AttributeOwner::Any, AttributeKey::Attribute("k".into())),
            cancel(),
            counter()
        ),
        Err(AttributeRequestError::AnyOwnerNotEnumerable)
    ));

    // Pin C4: builtin key outside the Builtin owner…
    assert!(matches!(
        attribute_values(
            Vec::new(),
            AttributeValuesQuery::new(
                AttributeOwner::Span,
                AttributeKey::Builtin(BuiltinField::Name)
            ),
            cancel(),
            counter()
        ),
        Err(AttributeRequestError::BuiltinKeyOutsideBuiltinOwner(
            AttributeOwner::Span
        ))
    ));
    // …and attribute keys inside it.
    assert!(matches!(
        attribute_values(
            Vec::new(),
            AttributeValuesQuery::new(AttributeOwner::Builtin, AttributeKey::Attribute("k".into())),
            cancel(),
            counter()
        ),
        Err(AttributeRequestError::AttributeKeyUnderBuiltinOwner(a)) if a == "k"
    ));

    // Source-set hygiene: duplicate source ids are rejected before any
    // source is read.
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
        attribute_names(
            vec![dup("same"), dup("same")],
            AttributeNamesQuery::new(),
            cancel(),
            counter()
        ),
        Err(AttributeRequestError::SourceSet(_))
    ));
}

/// An empty source set with a live token is a Complete result: the
/// static builtin vocabulary does not depend on sources existing
/// (`sfsq::traces::attribute_names` seeds it unconditionally), and
/// attribute scopes are simply empty — pinned so a future change
/// cannot gate the static set on "saw a source".
#[test]
fn empty_sources_still_yield_the_static_builtins() {
    let data = names(Vec::new(), AttributeNamesQuery::new());
    assert_eq!(data.status, QueryStatus::Complete);
    assert!(!data.truncated);
    let mut want: Vec<BuiltinField> = BuiltinField::ALL.to_vec();
    want.sort();
    let got: Vec<BuiltinField> = data
        .keys
        .iter()
        .map(|(s, k)| match (s, k) {
            (AttributeOwner::Builtin, AttributeKey::Builtin(i)) => *i,
            other => panic!("only static builtins expected, got {other:?}"),
        })
        .collect();
    assert_eq!(got, want);

    // A non-Builtin owner filter on zero sources: zero keys, Complete.
    let span_only = names(
        Vec::new(),
        AttributeNamesQuery::new().owner(AttributeOwner::Span),
    );
    assert!(span_only.keys.is_empty());
    assert_eq!(span_only.status, QueryStatus::Complete);

    // Values on zero sources: an empty Complete (data condition).
    let vals = values(
        Vec::new(),
        AttributeValuesQuery::new(AttributeOwner::Span, AttributeKey::Attribute("k".into())),
    );
    assert!(vals.values.is_empty());
    assert_eq!(vals.status, QueryStatus::Complete);
}

/// An in-window unavailable source reports its own reason
/// (`RemoteUnavailable`) beside a missing file's (`SourceFailure`), for
/// both operations, while the healthy source still serves; alone, it is
/// never Complete and never a source failure — the two reasons are
/// distinct (`sfsq::traces::PartialReason::RemoteUnavailable`).
#[test]
fn unavailable_sources_are_reported_and_the_rest_served() {
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
    // Healthy + missing-file + remote-unavailable: both failure
    // reasons coexist in one status set.
    let mixed = || {
        vec![
            sealed_source(dir.path(), &wal, "good"),
            missing_source(dir.path(), "missing", 0, 10),
            unavailable_source("remote", 0, 10),
        ]
    };
    let both = QueryStatus::Partial(BTreeSet::from([
        PartialReason::SourceFailure,
        PartialReason::RemoteUnavailable,
    ]));
    let only_remote = QueryStatus::Partial(BTreeSet::from([PartialReason::RemoteUnavailable]));
    let k = || AttributeValuesQuery::new(AttributeOwner::Span, AttributeKey::Attribute("k".into()));

    let progress = Arc::new(AtomicUsize::new(0));
    let keys = attribute_names(
        mixed(),
        AttributeNamesQuery::new(),
        CancellationToken::new(),
        Arc::clone(&progress),
    )
    .unwrap();
    assert_eq!(owner_attrs(&keys, AttributeOwner::Span), ["k"]);
    assert_eq!(keys.status, both);
    assert_eq!(progress.load(Ordering::Relaxed), 3, "one tick per source");

    let progress = Arc::new(AtomicUsize::new(0));
    let vals = attribute_values(
        mixed(),
        k(),
        CancellationToken::new(),
        Arc::clone(&progress),
    )
    .unwrap();
    assert_eq!(value_strings(&vals), ["v"]);
    assert_eq!(vals.status, both);
    assert_eq!(progress.load(Ordering::Relaxed), 3, "one tick per source");

    let alone = || vec![unavailable_source("remote", 0, 10)];
    assert_eq!(
        names(alone(), AttributeNamesQuery::new()).status,
        only_remote
    );
    let vals = values(alone(), k());
    assert!(vals.values.is_empty());
    assert_eq!(vals.status, only_remote);
}

/// The window prunes an unavailable source by its summary, exactly like
/// a sealed file (`pruned` in `src/traces/attributes.rs` guards both
/// source kinds): out of window it
/// is irrelevant (Complete), in window it is missing data (the reason).
#[test]
fn window_prunes_unavailable_sources_by_their_summary() {
    const NS: i64 = 1_000_000_000;
    let remote = || vec![unavailable_source("remote", 100, 110)];
    let k = || AttributeValuesQuery::new(AttributeOwner::Span, AttributeKey::Attribute("k".into()));
    let inside = TimeWindow::new(90 * NS, 120 * NS).unwrap();
    let outside = TimeWindow::new(0, 10 * NS).unwrap();
    let only_remote = QueryStatus::Partial(BTreeSet::from([PartialReason::RemoteUnavailable]));

    assert_eq!(values(remote(), k().window(inside)).status, only_remote);
    assert_eq!(
        values(remote(), k().window(outside)).status,
        QueryStatus::Complete
    );
    assert_eq!(
        names(remote(), AttributeNamesQuery::new().window(inside)).status,
        only_remote
    );
    assert_eq!(
        names(remote(), AttributeNamesQuery::new().window(outside)).status,
        QueryStatus::Complete
    );
}
