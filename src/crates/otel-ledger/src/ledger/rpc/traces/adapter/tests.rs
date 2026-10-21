//! Tests for the otel-traces wire adapter ([`super::adapter`]) — the
//! pure mapping between the `otel-traces` wire types and the
//! wire-neutral [`sfsq::traces`] engine, consumed by `handler.rs`.
//! Fixtures are synthetic `sfst` spans/summaries and hand-written
//! cursor strings; `now` is always an argument, so nothing here
//! touches the clock, files, or a runtime.
//!
//! Pins:
//!
//! - [`parse_trace_id`]: trim + case-insensitive accept with a
//!   canonical lowercase echo, the "32 hex" rejection message, and
//!   the all-zero id parsing here (its rejection is the handler's
//!   pin);
//! - [`to_trace_result`]: the whole wire envelope — echoed lowercase
//!   id, coverage echo, status, items, summary_root, roots/children,
//!   an ABSENT `parent_span_id` for roots, event/link sub-shapes with
//!   full attribute keys, the one `field_kinds` map over attribute keys
//!   with each set in canonical order — plus the empty-trace degenerate;
//!   the emission rule (keys and `trace_state` only; internal copies
//!   never; names outside the vocabulary dropped with `dropped_fields`);
//!   the resource table (each resource once, by index, in first-use
//!   order, lossless); the byte budget's accounting;
//! - the keys: every key the wire takes, pinned literally (the UI
//!   spells exactly these), and the rejections with their messages;
//! - the cursor codec: encode/parse round-trip, malformed
//!   rejections, over-cap walks advised to narrow;
//! - window canonicalization: the now-derived default window, the
//!   anchor-frozen window, inverted explicit bounds;
//! - search pagination: a cursor only for a full complete page,
//!   after-key filtering with tie runs and no straddling duplicates,
//!   served accumulation + cap, partial statuses ending the walk
//!   with the reason on the wire, the service breakdown, and the
//!   cursor's frozen window.
//!
//! Not pinned here: `validate_trace_bounds` and the overview
//! converter (covered end-to-end by the
//! handler tests) and request deserialization (the wire tests).
use super::*;
use serde_json::json;
use sfsq::traces::{FieldKinds, QueryStatus};

// ── parse_trace_id ──────────────────────────────────────────────────

#[test]
fn parses_hex_ids_case_insensitively_and_trimmed() {
    let id = parse_trace_id(" 0102030405060708090A0b0c0D0e0F10 ").unwrap();
    assert_eq!(
        id.to_string(),
        "0102030405060708090a0b0c0d0e0f10",
        "echo form is canonical lowercase"
    );
}

#[test]
fn rejects_wrong_length_and_non_hex() {
    for bad in [
        "",
        "0102",
        "zz02030405060708090a0b0c0d0e0f10",
        "0102030405060708090a0b0c0d0e0f1",
    ] {
        let err = parse_trace_id(bad).expect_err("must reject");
        assert!(err.contains("32 hex"), "{err}");
    }
}

#[test]
fn unset_id_parses_here_and_is_the_engines_call() {
    // The all-zero sentinel is syntactically valid hex and parses
    // here; rejecting the lookup is the ENGINE's job (its precise
    // message is pinned in the handler tests).
    let id = parse_trace_id("00000000000000000000000000000000").unwrap();
    assert!(id.is_unset());
}

// ── to_trace_result ─────────────────────────────────────────────────

/// A span fixture: the id byte fills the 8-byte span id, with one
/// event and one link attached so the shape test sees every converted
/// sub-object.
fn span(id: u8, parent: Option<u8>, start_ns: i64) -> sfst::TraceSpan {
    sfst::TraceSpan {
        span_id: sfst::SpanId::from([id; 8]),
        parent_span_id: parent
            .map(|p| sfst::SpanId::from([p; 8]))
            .unwrap_or_default(),
        start_ns,
        duration_ns: 500,
        kind: 2,
        flags: 1,
        dropped_attributes_count: 0,
        dropped_events_count: 0,
        dropped_links_count: 0,
        fields: vec![
            ("name".into(), format!("span-{id}")),
            ("_kind".into(), "2".into()),
        ],
        events: vec![sfst::TraceEvent {
            time_unix_nano: 42,
            name: "ev".into(),
            dropped_attributes_count: 0,
            attributes: vec![("k".into(), "v".into())],
        }],
        links: vec![sfst::TraceLink {
            trace_id: sfst::TraceId::from([0xAB; 16]),
            span_id: sfst::SpanId::from([0xCD; 8]),
            trace_state: "ot=th:8".into(),
            flags: 0,
            dropped_attributes_count: 0,
            attributes: vec![],
        }],
    }
}

// A two-span trace (root + child) through `to_trace_result`,
// asserted envelope field by field.
#[test]
fn trace_result_shape_is_pinned() {
    use sfst::ValueKind::{Int, Null, Str};
    let trace_id = parse_trace_id("11111111111111111111111111111111").unwrap();
    let data = TraceData {
        trace: sfst::Trace {
            spans: vec![span(1, None, 1_000), span(2, Some(1), 2_000)],
            roots: vec![0],
            children: vec![vec![1], vec![]],
        },
        status: QueryStatus::Complete,
        field_kinds: FieldKinds {
            fields: vec![
                ("_kind".into(), kinds(&[Int])),
                ("attributes.n".into(), kinds(&[Str, Int, Null])),
                ("name".into(), kinds(&[Str])),
            ],
            event_attributes: vec![("k".into(), kinds(&[Str]))],
            link_attributes: vec![],
        },
        span_cut: Vec::new(),
        matched: Vec::new(),
    };
    let v = serde_json::to_value(to_trace_result(
        &trace_id,
        data,
        ShapedResources::default(),
        CoverageWire {
            after: 0,
            before: u32::MAX,
        },
    ))
    .unwrap();

    assert_eq!(v["version"], 1);
    assert_eq!(v["trace_id"], "11111111111111111111111111111111");
    assert_eq!(
        v["coverage"],
        json!({"after": 0, "before": 4_294_967_295_u32})
    );
    assert_eq!(v["status"], json!({"complete": true}));
    assert_eq!(v["items"], json!({"returned": 2}));
    assert_eq!(v["summary_root"], 0);
    assert_eq!(v["roots"], json!([0]));
    assert_eq!(v["children"], json!([[1], []]));
    assert_eq!(v["resources"], json!([]));

    let s0 = &v["spans"][0];
    assert_eq!(s0["span_id"], "0101010101010101");
    assert!(
        s0.get("parent_span_id").is_none(),
        "an unset parent is ABSENT (OTel root convention), not a zero string"
    );
    assert_eq!(
        s0["fields"],
        json!([["name", "span-1"]]),
        "no internal copy"
    );
    assert!(s0.get("kind").is_none(), "the raw kind int left the wire");
    assert_eq!(s0["events"][0]["name"], "ev");
    assert_eq!(
        s0["events"][0]["attributes"],
        json!([["events.attributes.k", "v"]])
    );
    assert_eq!(s0["links"][0]["trace_id"], "ab".repeat(16));
    assert_eq!(s0["links"][0]["span_id"], "cd".repeat(8));

    let s1 = &v["spans"][1];
    assert_eq!(s1["parent_span_id"], "0101010101010101");

    assert_eq!(
        v["field_kinds"],
        json!({
            "attributes.n": ["null", "int", "str"],
            "events.attributes.k": ["str"],
        }),
        "one map over attribute keys, each set in canonical order"
    );
}

/// The trace by its result, at a complete status: every span shaped as
/// the engine shapes the spans it keeps, none refused.
fn result_of(mut spans: Vec<sfst::TraceSpan>, field_kinds: FieldKinds) -> serde_json::Value {
    use sfst::trace_combine::SpanShaper;
    let trace_id = parse_trace_id("11111111111111111111111111111111").unwrap();
    let n = spans.len();
    let mut shaper = WireShaper::default();
    let resources = shaper.resources();
    let span_cut = spans.iter_mut().map(|s| shaper.shape(s).cut).collect();
    let data = TraceData {
        trace: sfst::Trace {
            spans,
            roots: (0..n).collect(),
            children: vec![Vec::new(); n],
        },
        status: QueryStatus::Complete,
        field_kinds,
        span_cut,
        matched: Vec::new(),
    };
    serde_json::to_value(to_trace_result(
        &trace_id,
        data,
        resources.take(),
        coverage_all(),
    ))
    .unwrap()
}

/// Every owner's names go out as keys, with `trace_state`; the internal
/// copies never do; resource attributes go out in the trace's resource
/// table, the span pointing to its entry. Null, empty-map and empty-array
/// values travel as stored, and a flat event facet (the defensive path
/// for a file without event structures) is a key like any other.
#[test]
fn span_fields_are_keys_and_the_trace_state() {
    use sfst::ValueKind::{EmptyArray, EmptyKvlist, Null, Str};
    let mut s = span(1, None, 1_000);
    s.fields = [
        ("resource.attributes.service.name", "svc"),
        ("scope.name", "lib"),
        ("scope.version", "1.0"),
        ("scope.attributes.tier", "web"),
        ("name", "GET /"),
        ("kind", "SERVER"),
        ("_kind", "2"),
        ("trace_state", "ot=th:8"),
        ("status.code", "ERROR"),
        ("_status_code", "2"),
        ("status.message", "boom"),
        ("attributes.none", ""),
        ("attributes.map", "{}"),
        ("attributes.list", "[]"),
        ("events.name", "flat"),
        ("events.attributes.flat", "x"),
    ]
    .map(|(k, v)| (k.to_string(), v.to_string()))
    .to_vec();
    s.links[0].attributes = vec![("peer".into(), "p".into())];
    let mut t = span(2, None, 2_000);
    t.fields = vec![("name".into(), "no-state".into())];
    t.events.clear();
    t.links.clear();
    let field_kinds = FieldKinds {
        fields: vec![
            ("attributes.list".into(), kinds(&[EmptyArray])),
            ("attributes.map".into(), kinds(&[EmptyKvlist])),
            ("attributes.none".into(), kinds(&[Null])),
            ("events.attributes.flat".into(), kinds(&[Str])),
            ("resource.attributes.service.name".into(), kinds(&[Str])),
            ("scope.attributes.tier".into(), kinds(&[Str])),
            ("status.code".into(), kinds(&[Str])),
            ("trace_state".into(), kinds(&[Str])),
        ],
        event_attributes: vec![("k".into(), kinds(&[Str]))],
        link_attributes: vec![("peer".into(), kinds(&[Str]))],
    };
    let v = result_of(vec![s, t], field_kinds);
    assert_eq!(v["status"], json!({"complete": true}));
    assert_eq!(
        v["spans"][0],
        json!({
            "span_id": "0101010101010101",
            "start_ns": 1_000,
            "duration_ns": 500,
            "flags": 1,
            "dropped_attributes_count": 0,
            "dropped_events_count": 0,
            "dropped_links_count": 0,
            "resource": 0,
            "fields": [
                ["scope.name", "lib"],
                ["scope.version", "1.0"],
                ["scope.attributes.tier", "web"],
                ["name", "GET /"],
                ["kind", "SERVER"],
                ["trace_state", "ot=th:8"],
                ["status.code", "ERROR"],
                ["status.message", "boom"],
                ["attributes.none", ""],
                ["attributes.map", "{}"],
                ["attributes.list", "[]"],
                ["events.name", "flat"],
                ["events.attributes.flat", "x"],
            ],
            "events": [{
                "time_unix_nano": 42,
                "name": "ev",
                "dropped_attributes_count": 0,
                "attributes": [["events.attributes.k", "v"]],
            }],
            "links": [{
                "trace_id": "ab".repeat(16),
                "span_id": "cd".repeat(8),
                "trace_state": "ot=th:8",
                "flags": 0,
                "dropped_attributes_count": 0,
                "attributes": [["links.attributes.peer", "p"]],
            }],
        })
    );
    assert_eq!(v["spans"][1]["fields"], json!([["name", "no-state"]]));
    assert!(
        v["spans"][1].get("resource").is_none(),
        "absent without resource attributes"
    );
    assert_eq!(
        v["resources"],
        json!([{"attributes": [["resource.attributes.service.name", "svc"]]}])
    );
    assert_eq!(
        v["field_kinds"],
        json!({
            "attributes.list": ["empty_array"],
            "attributes.map": ["empty_kvlist"],
            "attributes.none": ["null"],
            "events.attributes.flat": ["str"],
            "events.attributes.k": ["str"],
            "links.attributes.peer": ["str"],
            "resource.attributes.service.name": ["str"],
            "scope.attributes.tier": ["str"],
        }),
        "attribute keys only; a flat event facet and the event's own attribute share one map"
    );
}

/// Names outside the key vocabulary — the status names of earlier
/// versions, an attribute prefix with no attribute, an unknown prefix,
/// an unresolvable reference — are left out, and the result says so
/// once, however many spans carry them.
#[test]
fn names_outside_the_vocabulary_are_dropped_and_reported() {
    let mut spans = Vec::new();
    for id in 1..=3 {
        let mut s = span(id, None, 1_000 * i64::from(id));
        s.fields = [
            ("name", "op"),
            ("status_code", "ERROR"),
            ("status_message", "boom"),
            ("attributes.", "x"),
            ("resource.attributes.", "x"),
            ("span.http.method", "GET"),
            ("", ""),
        ]
        .map(|(k, v)| (k.to_string(), v.to_string()))
        .to_vec();
        s.events[0].attributes = vec![("".into(), "".into()), ("k".into(), "v".into())];
        s.links[0].attributes = vec![("".into(), "".into())];
        spans.push(s);
    }
    assert_eq!(
        dropped_names(&spans).into_iter().collect::<Vec<_>>(),
        [
            "",
            "attributes.",
            "events.attributes.",
            "links.attributes.",
            "resource.attributes.",
            "span.http.method",
            "status_code",
            "status_message",
        ]
    );
    let v = result_of(spans, FieldKinds::default());
    assert_eq!(v["status"], json!({"partial": ["dropped_fields"]}));
    assert_eq!(v["resources"], json!([]), "a bare prefix is no resource");
    for s in v["spans"].as_array().unwrap() {
        assert_eq!(s["fields"], json!([["name", "op"]]));
        assert!(s.get("resource").is_none());
        assert_eq!(
            s["events"][0]["attributes"],
            json!([["events.attributes.k", "v"]])
        );
        assert_eq!(s["links"][0]["attributes"], json!([]));
    }
    // An earlier reason stays beside the new one.
    let trace_id = parse_trace_id("11111111111111111111111111111111").unwrap();
    let mut s = span(1, None, 1_000);
    s.fields = vec![("status_code".into(), "OK".into())];
    let data = TraceData {
        trace: sfst::Trace {
            spans: vec![s],
            roots: vec![0],
            children: vec![vec![]],
        },
        status: QueryStatus::Partial([sfsq::traces::PartialReason::SourceFailure].into()),
        field_kinds: FieldKinds::default(),
        span_cut: Vec::new(),
        matched: Vec::new(),
    };
    let v = serde_json::to_value(to_trace_result(
        &trace_id,
        data,
        ShapedResources::default(),
        coverage_all(),
    ))
    .unwrap();
    assert_eq!(
        v["status"],
        json!({"partial": ["source_failure", "dropped_fields"]})
    );
}

/// A kind set from its members.
fn kinds(list: &[sfst::ValueKind]) -> sfst::ValueKinds {
    list.iter().copied().collect()
}

// ── Search: keys ────────────────────────────────────────────────────

/// The wire's keys, pinned literally: the UI spells exactly these, so a
/// vocabulary change shows up here. Every builtin key and the five
/// attribute prefixes.
#[test]
fn the_wire_keys_are_pinned() {
    use BuiltinField::*;
    let builtins = [
        ("name", Name),
        ("kind", Kind),
        ("role", Role),
        ("status.code", Status),
        ("status.message", StatusMessage),
        ("scope.name", InstrumentationName),
        ("scope.version", InstrumentationVersion),
        ("events.name", EventName),
        ("duration", Duration),
        ("span_id", SpanId),
        ("parent_span_id", ParentSpanId),
        ("trace_id", TraceId),
        ("links.span_id", LinkSpanId),
        ("links.trace_id", LinkTraceId),
        ("events.time_since_start", EventTimeSinceStart),
        ("root_name", RootName),
        ("root_service_name", RootServiceName),
        ("trace_duration", TraceDuration),
    ];
    assert_eq!(
        builtins.len(),
        BuiltinField::ALL.len(),
        "a builtin is missing"
    );
    for (key, field) in builtins {
        assert_eq!(
            parse_selection_key(key).unwrap(),
            PredicateTarget::Builtin(field),
            "{key}"
        );
    }
    let attributes = [
        (
            "resource.attributes.service.name",
            AttributeOwner::Resource,
            "service.name",
        ),
        (
            "scope.attributes.telemetry.sdk",
            AttributeOwner::Instrumentation,
            "telemetry.sdk",
        ),
        (
            "attributes.http.request.method",
            AttributeOwner::Span,
            "http.request.method",
        ),
        (
            "events.attributes.exception.type",
            AttributeOwner::Event,
            "exception.type",
        ),
        ("links.attributes.peer", AttributeOwner::Link, "peer"),
    ];
    for (key, owner, attribute) in attributes {
        assert_eq!(
            parse_selection_key(key).unwrap(),
            PredicateTarget::Attribute(owner, attribute.into()),
            "{key}"
        );
    }
}

#[test]
fn keys_reject_empty_attributes_and_the_old_spellings() {
    for empty in ["attributes.", "resource.attributes.", "links.attributes."] {
        assert_eq!(
            parse_selection_key(empty).unwrap_err(),
            format!("key {empty:?} names no attribute")
        );
    }
    for bad in [
        "bogus",
        "resource",
        "resource.service.name",
        "span.http.method",
        "instrumentation.lib",
        "status",
        "status_message",
        "instrumentation_name",
        "event_name",
        "link_trace_id",
        "event_time_since_start",
        ".service.name",
    ] {
        let err = parse_selection_key(bad).unwrap_err();
        assert!(
            err.contains("resource.attributes.<key>") && err.contains("status.code"),
            "{bad}: {err}"
        );
    }
}

// ── Search: cursor ──────────────────────────────────────────────────

/// A cursor fixture: the id byte fills the 16-byte trace id.
fn cursor(after: u32, before: u32, rank: i64, id_byte: u8, served: usize) -> SearchCursor {
    SearchCursor {
        after_s: after,
        before_s: before,
        rank_ns: rank,
        trace_id: sfst::TraceId::from([id_byte; 16]),
        served,
    }
}

#[test]
fn cursor_round_trips_and_rejects_malformed() {
    let c = cursor(9_000, 10_000, 1_700_000_000_000_000_000, 0xAB, 40);
    assert_eq!(parse_cursor(&encode_cursor(&c)).unwrap(), c);
    for bad in [
        "",
        "t1:1:1",
        "t2:1:2:3:zz:1",
        "t2:1:2:3:1",
        "t2:9000:10000:5:00ff:0", // served 0
        "t2:10000:9000:5:00ff:1", // inverted window
        &format!("t2:9000:10000:5:{}:1:extra", "ab".repeat(16)),
    ] {
        assert!(parse_cursor(bad).is_err(), "{bad:?} must not parse");
    }
    let deep = format!("t2:9000:10000:5:{}:999999", "ab".repeat(16));
    assert!(
        parse_cursor(&deep).unwrap_err().contains("narrow"),
        "over-cap walks advise narrowing"
    );
}

// ── Search: window canonicalization ─────────────────────────────────

#[test]
fn window_defaults_to_the_recent_span_and_anchors_freeze_it() {
    // Both unspecified → [now-900, now).
    let w = resolve_window(0, 0, 10_000, None).unwrap();
    assert_eq!(w.capture, 9_100..10_000);
    assert_eq!(w.start_ns, 9_100_000_000_000);
    assert_eq!(w.end_ns, 10_000_000_000_000);

    // An anchor page uses the cursor's FROZEN window verbatim — the
    // request's own bounds and the drifted `now` are ignored, so every
    // page of one walk ranks the same corpus.
    let c = cursor(9_000, 9_800, 9_500_000_000_000, 0x01, 2);
    let w = resolve_window(0, 0, 99_999, Some(&c)).unwrap();
    assert_eq!(w.capture, 9_000..9_800);
    assert_eq!(w.end_ns, 9_800_000_000_000, "no ns narrowing — ever");

    // Inverted explicit windows are client errors.
    assert!(resolve_window(500, 400, 10_000, None).is_err());
}

// ── Search: result mapping + after-key pagination ───────────────────

/// A summary whose rank key equals its envelope start.
fn summary(id_byte: u8, start_ns: i64) -> sfsq::traces::TraceSummary {
    summary_ranked(id_byte, start_ns, start_ns)
}

/// A summary whose envelope start and RANK key (newest matched-span
/// start) differ — the shape real multi-span traces have.
fn summary_ranked(id_byte: u8, envelope_ns: i64, rank_ns: i64) -> sfsq::traces::TraceSummary {
    sfsq::traces::TraceSummary {
        trace_id: sfst::TraceId::from([id_byte; 16]),
        root_service: Some("svc".into()),
        root_name: Some("op".into()),
        start_ns: envelope_ns,
        newest_matched_start_ns: rank_ns,
        duration_ns: 100,
        span_count: 2,
        error_count: 0,
        service_breakdown: sfsq::traces::ServiceBreakdown {
            top: vec![("svc".into(), 2)],
            other: 0,
            unattributed: 0,
        },
        exact: true,
    }
}

fn search_data(traces: Vec<sfsq::traces::TraceSummary>) -> SearchData {
    SearchData {
        traces,
        status: QueryStatus::Complete,
    }
}

// The shared fixture window.
const WIN: (u32, u32) = (9_000, 10_000);

// The window widened by clamp(width, 1h, 24h) per side, saturating
// at the u32 bounds: a middle-width window widens by its own width,
// a multi-day one by the 1d cap.
#[test]
fn completion_capture_range_clamps_and_saturates() {
    assert_eq!(completion_capture_range(&(9_000..10_000)), 5_400..13_600);
    assert_eq!(completion_capture_range(&(0..20_000)), 0..40_000);
    let day = 86_400;
    assert_eq!(completion_capture_range(&(day..day * 3)), 0..day * 4);
    assert_eq!(
        completion_capture_range(&(u32::MAX - 100..u32::MAX)),
        u32::MAX - 100 - 3_600..u32::MAX
    );
}

// The page-1 cursor carries the MATCH window, never the widened
// completion range.
#[test]
fn cursor_freezes_the_original_window() {
    let r = to_search_result(
        search_data(vec![summary(1, 300), summary(2, 200)]),
        2,
        None,
        WIN,
    );
    let v = serde_json::to_value(&r).unwrap();
    let cursor = parse_cursor(v["anchor"]["next"].as_str().unwrap()).unwrap();
    assert_eq!((cursor.after_s, cursor.before_s), WIN);
}

// The engine's per-service span counts map onto the wire breakdown:
// `top` as value/spans pairs, then the other/unattributed counters.
#[test]
fn search_rows_carry_the_service_breakdown() {
    let mut t = summary(1, 300);
    t.span_count = 12;
    t.service_breakdown = sfsq::traces::ServiceBreakdown {
        top: vec![("frontend".into(), 6), ("cart".into(), 3)],
        other: 2,
        unattributed: 1,
    };
    let r = to_search_result(search_data(vec![t]), 2, None, WIN);
    let v = serde_json::to_value(&r).unwrap();
    assert_eq!(
        v["traces"][0]["service_breakdown"],
        json!({
            "top": [{"value": "frontend", "spans": 6}, {"value": "cart", "spans": 3}],
            "other": 2,
            "unattributed": 1,
        })
    );
}

#[test]
fn full_page_emits_a_cursor_and_short_page_does_not() {
    let r = to_search_result(
        search_data(vec![summary(1, 300), summary(2, 200)]),
        2,
        None,
        WIN,
    );
    assert_eq!(r.items.returned, 2);
    assert_eq!(
        r.anchor.as_ref().unwrap().next,
        format!("t2:9000:10000:200:{}:2", "02".repeat(16))
    );

    let r = to_search_result(search_data(vec![summary(1, 300)]), 2, None, WIN);
    assert!(r.anchor.is_none(), "a short page ends the walk");
}

#[test]
fn anchor_page_drops_everything_at_or_above_the_after_key() {
    // Already-served: ranks above 200, plus rank-200 ids ≤ 02 (the
    // engine re-emits the whole prefix — same query, larger limit).
    let c = cursor(WIN.0, WIN.1, 200, 0x02, 2);
    let data = search_data(vec![
        summary(1, 300), // above the key → served
        summary(2, 200), // the key itself → served
        summary(3, 200), // rank tie, id above the key → fresh
        summary(4, 100),
    ]);
    let r = to_search_result(data, 2, Some(&c), WIN);
    let ids: Vec<&str> = r.traces.iter().map(|t| t.trace_id.as_str()).collect();
    assert_eq!(
        ids,
        vec!["03".repeat(16).as_str(), "04".repeat(16).as_str()]
    );
    // served accumulates: 2 before + 2 this page.
    assert_eq!(
        r.anchor.as_ref().unwrap().next,
        format!("t2:9000:10000:100:{}:4", "04".repeat(16))
    );
}

#[test]
fn straddling_trace_does_not_duplicate_below_the_boundary() {
    // The review-caught flaw of the first (window-narrowing) design:
    // trace F matched spans at ranks 70 and 10; page 1 served it at
    // rank 70 with tail U at 50. The next page reruns the SAME
    // window, so F still ranks 70 — at-or-above the key — and drops
    // instead of re-entering at rank 10.
    let c = cursor(WIN.0, WIN.1, 50, 0x1A, 2); // tail U(50), served F+U
    let data = search_data(vec![
        summary_ranked(0x0F, 10, 70), // F: full-window rank stays 70
        summary(0x1A, 50),            // U: the key
        summary(0x1B, 30),            // W: genuinely fresh
    ]);
    let r = to_search_result(data, 2, Some(&c), WIN);
    let ids: Vec<&str> = r.traces.iter().map(|t| t.trace_id.as_str()).collect();
    assert_eq!(ids, vec!["1b".repeat(16).as_str()], "only W is fresh");
}

#[test]
fn a_walk_at_the_served_cap_emits_no_further_cursor() {
    // The over-fetch allowance can't grow past the cap: served 9_999
    // plus a full page exceeds CURSOR_SERVED_CAP (10_000), so the
    // final page fills but carries no continuation.
    let c = cursor(WIN.0, WIN.1, 500, 0x01, 9_999);
    let data = search_data(vec![
        summary(0x02, 400),
        summary(0x03, 300),
        summary(0x04, 200),
    ]);
    let r = to_search_result(data, 2, Some(&c), WIN);
    assert_eq!(r.items.returned, 2, "the page itself still fills");
    assert!(
        r.anchor.is_none(),
        "served would exceed the cap — the walk ends here"
    );
}

#[test]
fn partial_full_page_ends_the_walk_with_the_status_saying_why() {
    // A partial result's continuation is not a stable prefix (work
    // ceilings scale with the limit), so no cursor — the status carries
    // the reason the walk stopped.
    let mut b = sfsq::traces::StatusBuilder::new();
    b.add(sfsq::traces::PartialReason::WorkCeiling);
    let data = SearchData {
        traces: vec![summary(1, 300), summary(2, 200)],
        status: b.finish(),
    };
    let r = to_search_result(data, 2, None, WIN);
    assert_eq!(r.items.returned, 2, "the page itself is full");
    assert!(r.anchor.is_none());
    assert_eq!(
        serde_json::to_value(&r.status).unwrap(),
        json!({"partial": ["work_ceiling"]})
    );
}

#[test]
fn remote_unavailable_full_page_ends_the_walk() {
    // An unavailable remote source may hold better-ranked traces (its
    // spans are simply absent), so the page is not a stable prefix:
    // no cursor, and the status names the reason.
    let mut b = sfsq::traces::StatusBuilder::new();
    b.add(sfsq::traces::PartialReason::RemoteUnavailable);
    let data = SearchData {
        traces: vec![summary(1, 300), summary(2, 200)],
        status: b.finish(),
    };
    let r = to_search_result(data, 2, None, WIN);
    assert_eq!(r.items.returned, 2, "the page itself is full");
    assert!(r.anchor.is_none());
    assert_eq!(
        serde_json::to_value(&r.status).unwrap(),
        json!({"partial": ["remote_unavailable"]})
    );
}

// Partial reasons map onto the wire regardless of pagination — pinned
// on a short page, where the no-cursor rule above isn't in play.
#[test]
fn work_ceiling_partial_reaches_the_wire() {
    let mut b = sfsq::traces::StatusBuilder::new();
    b.add(sfsq::traces::PartialReason::WorkCeiling);
    let data = SearchData {
        traces: vec![summary(1, 300)],
        status: b.finish(),
    };
    let r = to_search_result(data, 20, None, WIN);
    assert_eq!(
        serde_json::to_value(&r.status).unwrap(),
        json!({"partial": ["work_ceiling"]})
    );
}

// An empty trace is still a complete, well-formed result: zero
// spans, a null summary_root.
#[test]
fn empty_trace_maps_to_complete_zero_span_result() {
    let trace_id = parse_trace_id("22222222222222222222222222222222").unwrap();
    let data = TraceData {
        trace: sfst::Trace {
            spans: vec![],
            roots: vec![],
            children: vec![],
        },
        status: QueryStatus::Complete,
        field_kinds: FieldKinds::default(),
        span_cut: Vec::new(),
        matched: Vec::new(),
    };
    let v = serde_json::to_value(to_trace_result(
        &trace_id,
        data,
        ShapedResources::default(),
        CoverageWire {
            after: 0,
            before: u32::MAX,
        },
    ))
    .unwrap();
    assert_eq!(v["status"], json!({"complete": true}));
    assert_eq!(v["items"]["returned"], 0);
    assert_eq!(v["summary_root"], serde_json::Value::Null);
    assert_eq!(v["spans"], json!([]));
    assert_eq!(v["resources"], json!([]));
}

// ── Response bounds ─────────────────────────────────────────────────

fn coverage_all() -> CoverageWire {
    CoverageWire {
        after: 0,
        before: u32::MAX,
    }
}

#[test]
fn rendered_keys_round_trip_through_the_parser() {
    // render ∘ parse = identity, so a suggested key feeds straight back
    // as a condition key or a values request: every attribute owner's
    // prefix, then every builtin key.
    let cases: Vec<(AttributeOwner, AttributeKey)> = vec![
        (
            AttributeOwner::Resource,
            AttributeKey::Attribute("service.name".into()),
        ),
        (
            AttributeOwner::Span,
            AttributeKey::Attribute("http.method".into()),
        ),
        (AttributeOwner::Event, AttributeKey::Attribute("x".into())),
        (AttributeOwner::Link, AttributeKey::Attribute("peer".into())),
        (
            AttributeOwner::Instrumentation,
            AttributeKey::Attribute("telemetry.sdk".into()),
        ),
    ];
    for (owner, key) in cases {
        let rendered = sfsq::traces::render_key(owner, &key);
        assert_eq!(
            parse_enumeration_key(&rendered).unwrap(),
            (owner, key),
            "{rendered}"
        );
    }
    for builtin in BuiltinField::ALL {
        let rendered =
            sfsq::traces::render_key(AttributeOwner::Builtin, &AttributeKey::Builtin(builtin));
        assert_eq!(
            parse_enumeration_key(&rendered).unwrap(),
            (AttributeOwner::Builtin, AttributeKey::Builtin(builtin)),
            "{rendered}"
        );
    }
}

/// A key carries the engine's kinds in canonical order, none for a key
/// without kinds (a builtin). Keys are cut at the request-key limit, not
/// the value cap: a key past 256 bytes stays whole; two keys cut onto one
/// text list the union of their kinds, in either order, and the higher
/// tier; a key whose escapes pass the limit is flagged, so every
/// unflagged key can be sent back.
#[test]
fn key_suggestions_carry_their_kinds() {
    use sfsq::traces::{SuggestData, Suggestion};
    use sfst::ValueKind::{Double, Int, Str};
    let key = |text: String, list: &[sfst::ValueKind]| Suggestion {
        text,
        truncated: false,
        class: sfst::MatchClass::Prefix,
        tier: Some(sfst::FieldTier::Low),
        enumerable: true,
        kinds: kinds(list),
    };
    let max = super::super::wire::REQUEST_TEXT_MAX_BYTES;
    let long = format!("attributes.{}", "l".repeat(300));
    let ctl = "\u{1}".repeat(max / 6 + 1);
    let p = "a".repeat(max - 2);
    let q = "b".repeat(max - 2);
    let data = SuggestData {
        items: vec![
            key("attributes.code".into(), &[Int]),
            key("name".into(), &[]),
            key(long.clone(), &[Str]),
            key(ctl.clone(), &[Str]),
            key(p.clone(), &[Str]),
            key(format!("{p}€x"), &[Double, Int]),
            key(q.clone(), &[Int]),
            Suggestion {
                tier: Some(sfst::FieldTier::High),
                ..key(format!("{q}€x"), &[])
            },
        ],
        truncated: false,
        status: QueryStatus::Complete,
        complete_after: None,
    };
    let v = serde_json::to_value(to_suggest_result(data, None)).unwrap();
    assert_eq!(
        v["items"],
        json!([
            {"text": "attributes.code", "match": "prefix", "tier": "low", "kinds": ["int"]},
            {"text": "name", "match": "prefix", "tier": "low"},
            {"text": long, "match": "prefix", "tier": "low", "kinds": ["str"]},
            {"text": &ctl[..max / 6], "match": "prefix", "truncated": true, "tier": "low", "kinds": ["str"]},
            {"text": p, "match": "prefix", "truncated": true, "tier": "low", "kinds": ["int", "double", "str"]},
            {"text": q, "match": "prefix", "truncated": true, "tier": "high", "kinds": ["int"]},
        ])
    );
}

#[test]
fn suggestions_are_capped_flagged_and_listed_once() {
    use sfsq::traces::{SuggestData, Suggestion};
    let item = |text: String, class, truncated| Suggestion {
        text,
        truncated,
        class,
        tier: None,
        enumerable: true,
        kinds: sfst::ValueKinds::default(),
    };
    // Control characters: 256 raw bytes the engine kept are 1,536 on
    // the wire; the adapter cuts them to the cap, and two texts that
    // differ past it become one flagged item.
    let ctl = "\u{1}".repeat(LIST_VALUE_MAX_BYTES);
    let data = SuggestData {
        items: vec![
            item(ctl.clone(), sfst::MatchClass::Prefix, false),
            item(
                format!("{}x", &ctl[..LIST_VALUE_MAX_BYTES - 1]),
                sfst::MatchClass::Prefix,
                false,
            ),
            item("b".into(), sfst::MatchClass::Fuzzy2, false),
        ],
        truncated: true,
        status: QueryStatus::Complete,
        complete_after: Some(7),
    };
    let v = serde_json::to_value(to_suggest_result(data, Some("name".into()))).unwrap();
    let kept = "\u{1}".repeat(LIST_VALUE_MAX_BYTES / 6);
    assert_eq!(
        v,
        json!({
            "mode": "suggest", "version": 1, "status": {"complete": true}, "key": "name",
            "items": [
                {"text": kept, "match": "prefix", "truncated": true},
                {"text": "b", "match": "fuzzy"},
            ],
            "truncated": true, "complete_after": 7,
        })
    );

    // A text equal to another's cut prefix need not be its neighbour:
    // `p` < `p + "bb"` (uncut) < `p + "€x"` (cut back to `p`).
    let p = "a".repeat(LIST_VALUE_MAX_BYTES - 2);
    let data = SuggestData {
        items: [p.clone(), format!("{p}bb"), format!("{p}€x")]
            .into_iter()
            .map(|text| item(text, sfst::MatchClass::Prefix, false))
            .collect(),
        truncated: false,
        status: QueryStatus::Complete,
        complete_after: None,
    };
    let v = serde_json::to_value(to_suggest_result(data, Some("name".into()))).unwrap();
    assert_eq!(
        v["items"],
        json!([
            {"text": p, "match": "prefix", "truncated": true},
            {"text": format!("{p}bb"), "match": "prefix"}
        ])
    );
}

#[test]
fn values_are_cut_at_a_character_boundary() {
    // "é" is two bytes: a cut landing inside it backs off to before it.
    let mut s = "aé".to_string();
    assert!(truncate_value(&mut s, 2));
    assert_eq!(s, "a");
    let mut s = "abc".to_string();
    assert!(!truncate_value(&mut s, 3), "at the cap: untouched");
    assert_eq!(s, "abc");
}

#[test]
fn caps_count_the_escaped_json_length() {
    // Every ASCII character, and a few wider ones, as serde_json writes them.
    for c in (0u8..0x80).map(char::from).chain(['é', '€', '😀']) {
        let wire = serde_json::to_string(&c.to_string()).unwrap();
        assert_eq!(json_escaped_len(c), wire.len() - 2, "{c:?}");
    }
    // A control character takes six bytes on the wire: ten fit in 60.
    let mut s = "\u{1}".repeat(100);
    assert!(truncate_value(&mut s, 60));
    assert_eq!(s.chars().count(), 10);
    assert!(serde_json::to_string(&s).unwrap().len() - 2 <= 60);
}

/// Spans served as they are, positions = indices.
struct VecSource(Vec<sfst::TraceSpan>);

impl sfst::trace_combine::SpanSource for VecSource {
    fn span_refs(
        &mut self,
        _trace_id: sfst::TraceId,
    ) -> Result<Vec<sfst::trace_combine::SpanRef>, sfst::Error> {
        Ok(self
            .0
            .iter()
            .enumerate()
            .map(|(i, s)| sfst::trace_combine::SpanRef {
                position: i as u32,
                start_ns: s.start_ns,
                span_id: s.span_id,
                kind: s.kind,
            })
            .collect())
    }

    fn materialize(&mut self, position: u32) -> Result<sfst::TraceSpan, sfst::Error> {
        Ok(self.0[position as usize].clone())
    }
}

/// Merge `spans` as the trace handler does (the wire shaper and its
/// budget), then shape the engine's result into the wire.
fn shaped_trace(spans: Vec<sfst::TraceSpan>) -> TraceResult {
    let trace_id = parse_trace_id("11111111111111111111111111111111").unwrap();
    let mut source = VecSource(spans);
    let mut shaper = WireShaper::default();
    let resources = shaper.resources();
    let out = sfst::trace_combine::combine(
        &mut [&mut source as &mut dyn sfst::trace_combine::SpanSource],
        trace_id,
        Some(10_000),
        None,
        Some(&mut shaper),
        &|| false,
    );
    let data = TraceData {
        status: if out.budget_hit {
            QueryStatus::Partial([sfsq::traces::PartialReason::ByteBudget].into())
        } else {
            QueryStatus::Complete
        },
        trace: out.trace,
        field_kinds: FieldKinds::default(),
        span_cut: out.cut,
        matched: Vec::new(),
    };
    to_trace_result(&trace_id, data, resources.take(), coverage_all())
}

#[test]
fn an_oversized_span_value_is_cut_and_flagged() {
    let mut root = span(1, None, 1_000);
    root.fields = vec![(
        "attributes.stack".into(),
        "x".repeat(TRACE_VALUE_MAX_BYTES + 10),
    )];
    let v = serde_json::to_value(shaped_trace(vec![root, span(2, Some(1), 2_000)])).unwrap();
    assert_eq!(
        v["spans"][0]["fields"][0][1].as_str().unwrap().len(),
        TRACE_VALUE_MAX_BYTES
    );
    assert_eq!(v["spans"][0]["truncated"], true);
    assert!(
        v["spans"][1].get("truncated").is_none(),
        "absent when nothing was cut"
    );
    assert_eq!(
        v["status"],
        json!({"complete": true}),
        "a cut value is not a cut trace"
    );
}

#[test]
fn a_trace_over_the_byte_budget_keeps_its_earliest_spans() {
    // A chain of spans each carrying one capped-size value under its own
    // key: the budget holds only a prefix. The kept spans are the input
    // prefix in order with their own graph, and the spans stay within
    // the budget.
    let n = TRACE_SPANS_MAX_BYTES / TRACE_VALUE_MAX_BYTES + 50;
    let id = |i: usize| sfst::SpanId::from((i as u64 + 1).to_be_bytes());
    let spans: Vec<sfst::TraceSpan> = (0..n)
        .map(|i| {
            let mut s = span(1, None, 1_000 + i as i64);
            s.span_id = id(i);
            if i > 0 {
                s.parent_span_id = id(i - 1);
            }
            s.fields = vec![(
                format!("attributes.v{i}"),
                "x".repeat(TRACE_VALUE_MAX_BYTES),
            )];
            s
        })
        .collect();
    let input = spans.clone();
    let r = shaped_trace(spans);
    let kept = r.spans.len();
    assert!(kept > 0 && kept < n, "kept {kept} of {n}");
    assert_eq!(r.items.returned, kept);
    for (got, want) in r.spans.iter().zip(&input) {
        assert_eq!(got.span_id, want.span_id.to_string());
        assert_eq!(got.start_ns, want.start_ns);
    }
    let (roots, children) = sfst::trace_combine::build_forest(&input[..kept]);
    assert_eq!((&r.roots, &r.children), (&roots, &children));
    let spans_bytes: usize = r.spans.iter().map(json_len).sum();
    assert!(spans_bytes <= TRACE_SPANS_MAX_BYTES);
    let v = serde_json::to_value(&r).unwrap();
    assert_eq!(v["status"], json!({"partial": ["byte_budget"]}));
}

/// A cut trace carries only the resources of the spans it keeps: with a
/// resource of its own on every span, the table holds one entry per kept
/// span, in span order, so the refused span's resource is not in it, and
/// `spans` and `resources` together stay within the budget.
#[test]
fn a_cut_trace_carries_only_the_resources_of_its_kept_spans() {
    let n = TRACE_SPANS_MAX_BYTES / TRACE_VALUE_MAX_BYTES + 50;
    let id = |i: usize| sfst::SpanId::from((i as u64 + 1).to_be_bytes());
    let spans: Vec<sfst::TraceSpan> = (0..n)
        .map(|i| {
            let mut s = span(1, None, 1_000 + i as i64);
            s.span_id = id(i);
            s.fields = vec![
                (
                    "resource.attributes.host".into(),
                    format!("{i}-{}", "h".repeat(TRACE_VALUE_MAX_BYTES - 16)),
                ),
                ("name".into(), "op".into()),
            ];
            s
        })
        .collect();
    let r = shaped_trace(spans);
    let kept = r.spans.len();
    assert!(kept > 0 && kept < n, "kept {kept} of {n}");
    assert_eq!(r.resources.len(), kept);
    for (i, s) in r.spans.iter().enumerate() {
        assert_eq!(s.resource, Some(i as u32));
        assert!(r.resources[i].attributes[0].1.starts_with(&format!("{i}-")));
    }
    assert!(json_len(&r.spans) + json_len(&r.resources) <= TRACE_SPANS_MAX_BYTES);
    let v = serde_json::to_value(&r).unwrap();
    assert_eq!(v["status"], json!({"partial": ["byte_budget"]}));
}

/// The pairs of `list`, owned.
fn pairs(list: &[(&str, &str)]) -> Vec<(String, String)> {
    list.iter()
        .map(|(k, v)| (k.to_string(), v.to_string()))
        .collect()
}

/// Each distinct resource goes out once, numbered in first-use order.
/// Spans share an entry only when their resource pairs are equal in
/// order: one more pair, or the same pairs in another order, is another
/// entry. A span without resource attributes names none. Expanding a span
/// (its entry's pairs, then its fields) gives back exactly the pairs the
/// emission rule left on it before resources were shared, since every
/// source stores a span's resource pairs first.
#[test]
fn each_resource_goes_out_once_and_expands_losslessly() {
    let front = pairs(&[
        ("resource.attributes.service.name", "front"),
        ("resource.attributes.tags[]", "a"),
        ("resource.attributes.tags[]", "b"),
    ]);
    let back = pairs(&[("resource.attributes.service.name", "back")]);
    let back_v2 = pairs(&[
        ("resource.attributes.service.name", "back"),
        ("resource.attributes.service.version", "2"),
    ]);
    let reordered = pairs(&[
        ("resource.attributes.tags[]", "b"),
        ("resource.attributes.tags[]", "a"),
        ("resource.attributes.service.name", "front"),
    ]);
    let own = pairs(&[
        ("scope.name", "lib"),
        ("name", "op"),
        ("_kind", "2"),
        ("attributes.k", "v"),
    ]);
    let layouts = [
        [front.clone(), own.clone()].concat(),
        [back.clone(), own.clone()].concat(),
        own.clone(),
        [front.clone(), own.clone()].concat(),
        [back_v2.clone(), own.clone()].concat(),
        [reordered.clone(), own.clone()].concat(),
        [back.clone(), own.clone()].concat(),
    ];
    let spans: Vec<sfst::TraceSpan> = layouts
        .iter()
        .enumerate()
        .map(|(i, fields)| {
            let mut s = span(i as u8 + 1, None, 1_000 + i as i64);
            s.fields = fields.clone();
            s
        })
        .collect();
    let input = spans.clone();
    let r = shaped_trace(spans);
    let indices: Vec<Option<u32>> = r.spans.iter().map(|s| s.resource).collect();
    assert_eq!(
        indices,
        [Some(0), Some(1), None, Some(0), Some(2), Some(3), Some(1)]
    );
    let entries: Vec<&Vec<(String, String)>> = r.resources.iter().map(|e| &e.attributes).collect();
    assert_eq!(entries, [&front, &back, &back_v2, &reordered]);
    for (got, want) in r.spans.iter().zip(&input) {
        let mut expanded = got
            .resource
            .map(|i| r.resources[i as usize].attributes.clone())
            .unwrap_or_default();
        expanded.extend(got.fields.iter().cloned());
        let emitted: Vec<(String, String)> = want
            .fields
            .iter()
            .filter(|(name, _)| matches!(field_use(name), FieldUse::Attribute | FieldUse::Plain))
            .cloned()
            .collect();
        assert_eq!(expanded, emitted, "span {}", got.span_id);
    }
}

/// A layout no source writes, resource pairs after other pairs, still
/// shares by the pairs in order and expands with its resource pairs
/// first.
#[test]
fn resource_pairs_out_of_front_still_share_by_their_order() {
    let mut first = span(1, None, 1_000);
    first.fields = pairs(&[
        ("resource.attributes.service.name", "svc"),
        ("resource.attributes.host", "h"),
        ("name", "a"),
    ]);
    let mut scattered = span(2, None, 2_000);
    scattered.fields = pairs(&[
        ("scope.name", "lib"),
        ("resource.attributes.service.name", "svc"),
        ("name", "b"),
        ("resource.attributes.host", "h"),
    ]);
    let r = shaped_trace(vec![first, scattered]);
    assert_eq!(r.resources.len(), 1);
    assert_eq!(r.spans[1].resource, Some(0));
    assert_eq!(
        r.spans[1].fields,
        pairs(&[("scope.name", "lib"), ("name", "b")])
    );
}

/// Merge `spans` as the trace handler does: the engine with the wire
/// shaper and its budget.
fn merged(spans: Vec<sfst::TraceSpan>) -> sfst::trace_combine::CombineOutcome {
    let trace_id = parse_trace_id("11111111111111111111111111111111").unwrap();
    let mut source = VecSource(spans);
    let mut shaper = WireShaper::default();
    sfst::trace_combine::combine(
        &mut [&mut source as &mut dyn sfst::trace_combine::SpanSource],
        trace_id,
        Some(10_000),
        None,
        Some(&mut shaper),
        &|| false,
    )
}

/// What kept spans' fields hold in memory: their pair slots and their
/// strings' allocations.
fn held_bytes(spans: &[sfst::TraceSpan]) -> usize {
    spans
        .iter()
        .map(|s| {
            s.fields.capacity() * std::mem::size_of::<(String, String)>()
                + s.fields
                    .iter()
                    .map(|(k, v)| k.capacity() + v.capacity())
                    .sum::<usize>()
        })
        .sum()
}

/// `n` spans of one trace, in start order, each with `fields`.
fn spans_with(n: usize, fields: &[(String, String)]) -> Vec<sfst::TraceSpan> {
    let id = |i: usize| sfst::SpanId::from((i as u64 + 1).to_be_bytes());
    (0..n)
        .map(|i| {
            let mut s = span(1, None, 1_000 + i as i64);
            s.span_id = id(i);
            s.fields = fields.to_vec();
            s
        })
        .collect()
}

/// The merge holds what the answer sends: a kept span gives its resource
/// pairs up when it is shaped, so a resource shared by every span is held
/// once, not once per span, and the kept spans stay within the budget.
#[test]
fn kept_spans_hold_no_resource_copies() {
    let resource: Vec<(String, String)> = (0..4)
        .map(|k| {
            (
                format!("resource.attributes.k{k}"),
                "r".repeat(TRACE_VALUE_MAX_BYTES),
            )
        })
        .collect();
    let n = 500;
    let out = merged(spans_with(
        n,
        &[resource, pairs(&[("name", "op")])].concat(),
    ));
    assert_eq!(
        out.trace.spans.len(),
        n,
        "the shared resource is charged once"
    );
    let held = held_bytes(&out.trace.spans);
    assert!(
        held <= TRACE_SPANS_MAX_BYTES,
        "kept spans hold {held} bytes"
    );
}

/// A resource of many small pairs leaves nothing behind in the spans that
/// give it up: not even the slots its pairs took.
#[test]
fn many_small_resource_pairs_leave_no_slots_behind() {
    let resource: Vec<(String, String)> = (0..1_000)
        .map(|k| (format!("resource.attributes.k{k}"), "v".to_string()))
        .collect();
    let n = 300;
    let out = merged(spans_with(
        n,
        &[resource, pairs(&[("name", "op")])].concat(),
    ));
    assert_eq!(
        out.trace.spans.len(),
        n,
        "the shared resource is charged once"
    );
    let held = held_bytes(&out.trace.spans);
    assert!(
        held <= TRACE_SPANS_MAX_BYTES,
        "kept spans hold {held} bytes"
    );
}

/// A value cut to the cap gives the rest of its allocation back: a kept
/// span holds the cut value, not the stored one.
#[test]
fn a_cut_value_gives_its_memory_back() {
    let n = 20;
    let fields = pairs(&[("name", "op")]);
    let mut spans = spans_with(n, &fields);
    for s in &mut spans {
        s.fields
            .push(("attributes.blob".into(), "x".repeat(256 * 1024)));
    }
    let out = merged(spans);
    assert_eq!(out.trace.spans.len(), n);
    assert!(out.cut.iter().all(|&cut| cut));
    let held = held_bytes(&out.trace.spans);
    assert!(
        held <= TRACE_SPANS_MAX_BYTES,
        "kept spans hold {held} bytes"
    );
}

/// A resource value over the cap is cut once, in the shared entry, and
/// flags the spans that carried it, as the cut flagged them before
/// resources were shared.
#[test]
fn a_cut_resource_value_is_cut_once_and_flags_its_spans() {
    let long = "x".repeat(TRACE_VALUE_MAX_BYTES + 10);
    let spans = (1..=2u8)
        .map(|i| {
            let mut s = span(i, None, 1_000 * i64::from(i));
            s.fields = vec![
                ("resource.attributes.blob".into(), long.clone()),
                ("name".into(), "op".into()),
            ];
            s
        })
        .collect();
    let r = shaped_trace(spans);
    assert_eq!(r.resources.len(), 1);
    assert_eq!(r.resources[0].attributes[0].1.len(), TRACE_VALUE_MAX_BYTES);
    assert!(
        r.spans.iter().all(|s| s.resource == Some(0) && s.truncated),
        "both spans carried the cut value"
    );
}

/// The engine's field kinds for `spans` as storage names (span fields)
/// and bare names (event and link attributes), every name holding `of`.
fn kinds_of_input(spans: &[sfst::TraceSpan], of: sfst::ValueKinds) -> FieldKinds {
    let mut k = FieldKinds::default();
    for s in spans {
        k.fields
            .extend(s.fields.iter().map(|(n, _)| (n.clone(), of)));
        for e in &s.events {
            k.event_attributes
                .extend(e.attributes.iter().map(|(n, _)| (n.clone(), of)));
        }
        for l in &s.links {
            k.link_attributes
                .extend(l.attributes.iter().map(|(n, _)| (n.clone(), of)));
        }
    }
    k
}

/// The field kinds and the resource table count against the byte budget,
/// and the shaper charges exactly the keys `field_kinds` holds and the
/// resources the spans share, each once. A budget-bound trace whose names
/// and values need escaping still fits. With every key holding every leaf
/// kind (the entry the shaper charges), the charge is exact up to the
/// three commas the last span, the last resource and the last entry do
/// not take; with ordinary kind sets it is an upper bound. Builtins,
/// `trace_state` and internal copies are charged nothing.
#[test]
fn field_kinds_and_resources_count_against_the_byte_budget() {
    use sfst::ValueKind::{Int, Str};
    let n = TRACE_SPANS_MAX_BYTES / 512;
    let id = |i: usize| sfst::SpanId::from((i as u64 + 1).to_be_bytes());
    let spans: Vec<sfst::TraceSpan> = (0..n)
        .map(|i| {
            let mut s = span(1, None, 1_000 + i as i64);
            s.span_id = id(i);
            s.fields = vec![
                (
                    format!("resource.attributes.r\"{}", i % 3),
                    format!("v\u{1}{}", i % 3),
                ),
                (format!("attributes.f\"{i}\u{1}"), "x".repeat(100)),
                ("name".into(), "op".into()),
                ("status.code".into(), "OK".into()),
                ("trace_state".into(), "ot=th:8".into()),
                ("_kind".into(), "2".into()),
            ];
            s.events[0].attributes = vec![(format!("e\\{i}"), "v".into())];
            s.links[0].attributes = vec![(format!("l\"{i}"), "v".into())];
            s
        })
        .collect();
    let input = spans.clone();
    let r = shaped_trace(spans);
    let kept = r.spans.len();
    assert!(kept > 0 && kept < n, "the budget binds: kept {kept} of {n}");
    let mut shaper = WireShaper::default();
    let charged: usize = input[..kept]
        .iter()
        .map(|s| {
            use sfst::trace_combine::SpanShaper;
            shaper.shape(&mut s.clone()).weight
        })
        .sum();
    assert_eq!(r.resources.len(), 3, "three resources, shared");
    let spans_bytes = json_len(&r.spans) + json_len(&r.resources);
    let every: sfst::ValueKinds = sfst::ValueKinds::LEAF_ORDER.into_iter().collect();
    let real = spans_bytes + json_len(&field_kinds_wire(kinds_of_input(&input[..kept], every)));
    assert!(
        real <= TRACE_SPANS_MAX_BYTES,
        "{real} > {TRACE_SPANS_MAX_BYTES}"
    );
    assert!(
        (0..=3).contains(&(charged - real)),
        "charged {charged}, real {real}"
    );
    for of in [kinds(&[Str]), kinds(&[Int, Str])] {
        let real = spans_bytes + json_len(&field_kinds_wire(kinds_of_input(&input[..kept], of)));
        assert!(charged >= real, "charged {charged} < real {real}");
    }
}

#[test]
fn a_list_rows_long_strings_are_cut_and_flagged() {
    let mut t = summary(1, 300);
    t.root_name = Some("n".repeat(LIST_VALUE_MAX_BYTES + 1));
    let r = to_search_result(search_data(vec![t, summary(2, 200)]), 2, None, WIN);
    let v = serde_json::to_value(&r).unwrap();
    assert_eq!(
        v["traces"][0]["root_name"].as_str().unwrap().len(),
        LIST_VALUE_MAX_BYTES
    );
    assert_eq!(v["traces"][0]["truncated"], true);
    assert!(v["traces"][1].get("truncated").is_none());
}

// ── Guard: every stored name is in the vocabulary ───────────────────

/// Every name the flattener writes for an OTLP request is a key,
/// `trace_state` or an internal copy — never unknown, so the emission
/// rule drops nothing from data this build stores — and every key renders
/// back to the name it came from. The OTLP messages are exhaustive
/// literals with non-default values (unknown enum values included), so a
/// proto upgrade that adds a field fails to compile here until the
/// flattener and the vocabulary have a place for it.
#[test]
fn every_stored_name_is_a_key() {
    use opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest;
    use opentelemetry_proto::tonic::common::v1::{
        AnyValue, ArrayValue, InstrumentationScope, KeyValue, KeyValueList, any_value::Value as Av,
    };
    use opentelemetry_proto::tonic::resource::v1::Resource;
    use opentelemetry_proto::tonic::trace::v1::{ResourceSpans, ScopeSpans, Span, Status, span};
    let text = |v: &str| AnyValue {
        value: Some(Av::StringValue(v.into())),
    };
    let kv = |key: &str, value: AnyValue| KeyValue {
        key: key.into(),
        value: Some(value),
        key_strindex: 0,
    };
    let nested = || AnyValue {
        value: Some(Av::KvlistValue(KeyValueList {
            values: vec![
                kv("inner", text("x")),
                kv(
                    "list",
                    AnyValue {
                        value: Some(Av::ArrayValue(ArrayValue {
                            values: vec![text("a"), text("b")],
                        })),
                    },
                ),
            ],
        })),
    };
    let attrs = || {
        vec![
            kv("plain", text("v")),
            kv("map", nested()),
            kv("", text("e")),
            kv("a=b", text("q")),
        ]
    };
    let req = ExportTraceServiceRequest {
        resource_spans: vec![ResourceSpans {
            resource: Some(Resource {
                attributes: attrs(),
                dropped_attributes_count: 1,
                entity_refs: Vec::new(),
            }),
            scope_spans: vec![ScopeSpans {
                scope: Some(InstrumentationScope {
                    name: "lib".into(),
                    version: "1".into(),
                    attributes: attrs(),
                    dropped_attributes_count: 1,
                }),
                spans: vec![Span {
                    trace_id: vec![1; 16],
                    span_id: vec![2; 8],
                    trace_state: "ot=th:8".into(),
                    parent_span_id: vec![3; 8],
                    flags: 1,
                    name: "op".into(),
                    kind: 9,
                    start_time_unix_nano: 1_000_000_000,
                    end_time_unix_nano: 1_000_000_500,
                    attributes: attrs(),
                    dropped_attributes_count: 1,
                    events: vec![span::Event {
                        time_unix_nano: 1_000_000_100,
                        name: "ev".into(),
                        attributes: attrs(),
                        dropped_attributes_count: 1,
                    }],
                    dropped_events_count: 1,
                    links: vec![span::Link {
                        trace_id: vec![4; 16],
                        span_id: vec![5; 8],
                        trace_state: "a=b".into(),
                        attributes: attrs(),
                        dropped_attributes_count: 1,
                        flags: 1,
                    }],
                    dropped_links_count: 1,
                    status: Some(Status {
                        message: "boom".into(),
                        code: 7,
                    }),
                }],
                schema_url: "s".into(),
            }],
            schema_url: "s".into(),
        }],
    };
    let (flat, _) = ng_flatten::flatten_trace_request(req);
    let mut names: Vec<String> = Vec::new();
    let mut add = |entries: &[ng_flatten::Entry]| {
        names.extend(entries.iter().map(|e| flat.tree.path(e.node)));
    };
    for rg in &flat.resources {
        add(&rg.resource);
        for sg in &rg.scopes {
            add(&sg.scope);
            for s in &sg.spans {
                add(&s.entries);
                for e in &s.events {
                    add(std::slice::from_ref(&e.name));
                    add(&e.attributes);
                }
                for l in &s.links {
                    add(&l.attributes);
                }
            }
        }
    }
    names.sort();
    names.dedup();
    assert!(names.len() > 20, "the request flattened: {names:?}");
    let mut uses = std::collections::BTreeSet::new();
    for name in &names {
        match sfsq::traces::classify_storage(name) {
            StorageName::Key(owner, key) => {
                assert_eq!(&sfsq::traces::render_key(owner, &key), name, "{name}");
                uses.insert(match key {
                    AttributeKey::Attribute(_) => format!("{owner:?} attribute"),
                    AttributeKey::Builtin(b) => format!("{b:?}"),
                });
            }
            StorageName::TraceState => {
                uses.insert("trace_state".into());
            }
            StorageName::Internal => {
                uses.insert(format!("internal {name}"));
            }
            StorageName::Unknown => panic!("{name} is stored but not in the vocabulary"),
        }
    }
    assert_eq!(
        uses.into_iter().collect::<Vec<_>>(),
        [
            "Event attribute",
            "EventName",
            "Instrumentation attribute",
            "InstrumentationName",
            "InstrumentationVersion",
            "Kind",
            "Link attribute",
            "Name",
            "Resource attribute",
            "Role",
            "Span attribute",
            "Status",
            "StatusMessage",
            "internal _kind",
            "internal _status_code",
            "trace_state",
        ],
        "every kind of stored name is covered"
    );
}

/// One explore page of one listed span with older spans below it, under
/// `status`, as the wire shapes it.
fn explore_page(status: QueryStatus) -> serde_json::Value {
    let params: ExploreParams =
        serde_json::from_value(json!({"rows": {"limit": 1, "columns": ["name"]}})).unwrap();
    let grid = sfst::Grid::new(0, 1_000_000_000, 10);
    let request = explore_request(&params, grid, None).unwrap();
    let data = ExploreData {
        status,
        histogram: None,
        rows: Some(sfsq::traces::RowsData {
            matched: 2,
            items: vec![sfsq::traces::SpanRow {
                key: RowKey {
                    start_ns: 5_000_000_000,
                    trace_id: sfst::TraceId::from([1; 16]),
                    span_id: sfst::SpanId::from([2; 8]),
                },
                duration_ns: 1,
                columns: vec![vec!["op".to_string()]],
            }],
            more: true,
        }),
    };
    serde_json::to_value(to_explore_result(data, &request, grid, (0, 10), (0, 10))).unwrap()
}

#[test]
fn an_explore_walk_continues_only_from_complete_rows() {
    use sfsq::traces::PartialReason::*;
    for (status, continues) in [
        (QueryStatus::Complete, true),
        (QueryStatus::Complete.with(StackFieldHighCard), true),
        (QueryStatus::Complete.with(SourceFailure), false),
        (QueryStatus::Complete.with(WorkCeiling), false),
        (QueryStatus::Complete.with(RemoteUnavailable), false),
        (
            QueryStatus::Complete
                .with(StackFieldHighCard)
                .with(WorkCeiling),
            false,
        ),
    ] {
        let page = explore_page(status.clone());
        assert_eq!(
            page["rows"]["anchor"]["next"].is_string(),
            continues,
            "{status:?}: {page}"
        );
    }
}
