use super::*;
use serde_json::json;
use sfsq::traces::QueryStatus;

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
    for bad in ["", "0102", "zz02030405060708090a0b0c0d0e0f10", "0102030405060708090a0b0c0d0e0f1"] {
        let err = parse_trace_id(bad).expect_err("must reject");
        assert!(err.contains("32 hex"), "{err}");
    }
}

#[test]
fn unset_id_parses_here_and_is_the_engines_call() {
    // The all-zero sentinel is syntactically valid hex; the ENGINE
    // rejects the lookup with its precise message (pinned in the
    // handler test).
    let id = parse_trace_id("00000000000000000000000000000000").unwrap();
    assert!(id.is_unset());
}

// ── to_trace_result ─────────────────────────────────────────────────

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
        fields: vec![("name".into(), format!("span-{id}"))],
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

#[test]
fn trace_result_shape_is_pinned() {
    let trace_id = parse_trace_id("11111111111111111111111111111111").unwrap();
    let data = TraceData {
        trace: sfst::Trace {
            spans: vec![span(1, None, 1_000), span(2, Some(1), 2_000)],
            roots: vec![0],
            children: vec![vec![1], vec![]],
        },
        status: QueryStatus::Complete,
        field_kinds: FieldKinds {
            fields: vec![("name".into(), sfst::ValueKind::Str)],
            event_attributes: vec![("k".into(), sfst::ValueKind::Str)],
            link_attributes: vec![],
        },
    };
    let v = serde_json::to_value(to_trace_result(&trace_id, data)).unwrap();

    assert_eq!(v["version"], 1);
    assert_eq!(v["trace_id"], "11111111111111111111111111111111");
    assert_eq!(v["status"], json!({"complete": true}));
    assert_eq!(v["items"], json!({"returned": 2}));
    assert_eq!(v["summary_root"], 0);
    assert_eq!(v["roots"], json!([0]));
    assert_eq!(v["children"], json!([[1], []]));

    let s0 = &v["spans"][0];
    assert_eq!(s0["span_id"], "0101010101010101");
    assert!(
        s0.get("parent_span_id").is_none(),
        "an unset parent is ABSENT (OTel root convention), not a zero string"
    );
    assert_eq!(s0["fields"], json!([["name", "span-1"]]));
    assert_eq!(s0["events"][0]["name"], "ev");
    assert_eq!(s0["events"][0]["attributes"], json!([["k", "v"]]));
    assert_eq!(s0["links"][0]["trace_id"], "ab".repeat(16));
    assert_eq!(s0["links"][0]["span_id"], "cd".repeat(8));

    let s1 = &v["spans"][1];
    assert_eq!(s1["parent_span_id"], "0101010101010101");

    assert_eq!(
        v["field_kinds"],
        json!({
            "fields": [["name", "str"]],
            "event_attributes": [["k", "str"]],
            "link_attributes": [],
        })
    );
}

// ── Search: selection keys ──────────────────────────────────────────

#[test]
fn every_builtin_has_a_wire_word_and_nothing_stale() {
    // The wire table must stay in lockstep with the engine's builtin
    // set (the CLI's own guard pattern).
    for builtin in BuiltinField::ALL {
        assert!(
            BUILTIN_WORDS.iter().any(|(_, f)| *f == builtin),
            "builtin {builtin:?} has no wire word"
        );
    }
    assert_eq!(BUILTIN_WORDS.len(), BuiltinField::ALL.len());
    let mut words: Vec<&str> = BUILTIN_WORDS.iter().map(|(w, _)| *w).collect();
    words.sort_unstable();
    words.dedup();
    assert_eq!(words.len(), BUILTIN_WORDS.len(), "duplicate wire words");
}

#[test]
fn selection_keys_parse_owners_builtins_and_dotted_attributes() {
    assert_eq!(
        parse_selection_key("resource.service.name").unwrap(),
        PredicateTarget::Attribute(AttributeOwner::Resource, "service.name".into())
    );
    assert_eq!(
        parse_selection_key("span.http.method").unwrap(),
        PredicateTarget::Attribute(AttributeOwner::Span, "http.method".into())
    );
    assert_eq!(
        parse_selection_key("status").unwrap(),
        PredicateTarget::Builtin(BuiltinField::Status)
    );
    for bad in ["resource.", "bogus", "resource", "svc.name"] {
        assert!(parse_selection_key(bad).is_err(), "{bad} must not parse");
    }
}

#[test]
fn predicate_builds_sorted_eq_conditions_plus_duration_bounds() {
    let mut selections = HashMap::new();
    selections.insert(
        "resource.service.name".to_string(),
        vec!["a".to_string(), "b".to_string()],
    );
    selections.insert("status".to_string(), vec!["ERROR".to_string()]);
    selections.insert("span.empty".to_string(), vec![]); // no constraint
    let p = build_predicate(&selections, Some(5), Some(10)).unwrap();
    assert_eq!(p.conditions.len(), 4);
    // Sorted keys: resource.service.name, status; then min, max duration.
    assert_eq!(
        p.conditions[0].target,
        PredicateTarget::Attribute(AttributeOwner::Resource, "service.name".into())
    );
    assert_eq!(p.conditions[0].op, CompareOp::Eq);
    assert_eq!(p.conditions[0].values.len(), 2);
    assert_eq!(
        p.conditions[1].target,
        PredicateTarget::Builtin(BuiltinField::Status)
    );
    assert_eq!(p.conditions[2].op, CompareOp::Gte);
    assert_eq!(p.conditions[3].op, CompareOp::Lte);
}

// ── Search: cursor ──────────────────────────────────────────────────

#[test]
fn cursor_round_trips_and_rejects_malformed() {
    let c = SearchCursor {
        start_ns: 1_700_000_000_000_000_000,
        served_at_start: 3,
    };
    assert_eq!(parse_cursor(&encode_cursor(&c)).unwrap(), c);
    for bad in ["", "t1:1", "t2:1:1", "t1:x:1", "t1:1:0", "t1:1:1:extra"] {
        assert!(parse_cursor(bad).is_err(), "{bad:?} must not parse");
    }
    assert!(
        parse_cursor("t1:1:999999").unwrap_err().contains("narrow"),
        "over-cap tie runs advise narrowing"
    );
}

// ── Search: window canonicalization ─────────────────────────────────

#[test]
fn window_defaults_to_the_recent_span_and_narrows_for_anchors() {
    // Both unspecified → [now-900, now).
    let w = resolve_window(0, 0, 10_000, None).unwrap().unwrap();
    assert_eq!(w.capture, 9_100..10_000);
    assert_eq!(w.start_ns, 9_100_000_000_000);
    assert_eq!(w.end_ns, 10_000_000_000_000);

    // Anchor narrows the END ns-precise; capture stays second-granular.
    let c = SearchCursor {
        start_ns: 9_500_123_456_789,
        served_at_start: 1,
    };
    let w = resolve_window(9_000, 10_000, 10_000, Some(&c)).unwrap().unwrap();
    assert_eq!(w.end_ns, 9_500_123_456_790);
    assert_eq!(w.capture, 9_000..10_000);

    // An anchor at/below the window start ends the walk (empty page).
    let done = SearchCursor {
        start_ns: 8_000_000_000_000,
        served_at_start: 1,
    };
    assert_eq!(resolve_window(9_000, 10_000, 10_000, Some(&done)).unwrap(), None);

    // Inverted explicit windows are client errors.
    assert!(resolve_window(500, 400, 10_000, None).is_err());
}

// ── Search: result mapping + tie-safe pagination ────────────────────

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
        matched_count: 2,
        matched_spans: vec![],
        exact: true,
    }
}

fn search_data(traces: Vec<sfsq::traces::TraceSummary>) -> SearchData {
    SearchData {
        traces,
        status: QueryStatus::Complete,
        field_kinds: FieldKinds::default(),
    }
}

#[test]
fn full_page_emits_a_cursor_and_short_page_does_not() {
    let r = to_search_result(search_data(vec![summary(1, 300), summary(2, 200)]), 2, None);
    assert_eq!(r.items.returned, 2);
    assert_eq!(r.anchor.as_ref().unwrap().next, "t1:200:1");

    let r = to_search_result(search_data(vec![summary(1, 300)]), 2, None);
    assert!(r.anchor.is_none(), "a short page ends the walk");
}

#[test]
fn anchor_page_drops_served_ties_and_accumulates_the_count() {
    // Page 1 ended at start=200 with one trace served there; the anchor
    // page re-includes the ties (engine window end = 201) and the wire
    // drops the served one.
    let c = SearchCursor {
        start_ns: 200,
        served_at_start: 1,
    };
    // Engine returns (limit = 2 + 1): the served tie first (id 2), then
    // the unserved tie (id 3), then older (id 4).
    let data = search_data(vec![summary(2, 200), summary(3, 200), summary(4, 100)]);
    let r = to_search_result(data, 2, Some(&c));
    let ids: Vec<&str> = r.traces.iter().map(|t| t.trace_id.as_str()).collect();
    assert_eq!(ids[0], "03".repeat(16));
    assert_eq!(ids[1], "04".repeat(16));
    // The page tail (id 4, start=100) starts a fresh count.
    assert_eq!(r.anchor.as_ref().unwrap().next, "t1:100:1");
}

#[test]
fn tie_run_spanning_pages_accumulates_served_at_start() {
    // Three ties at start=200, pages of 2. Page 1 serves two of them.
    let r = to_search_result(
        search_data(vec![summary(1, 200), summary(2, 200), summary(3, 200)]),
        2,
        None,
    );
    assert_eq!(r.anchor.as_ref().unwrap().next, "t1:200:2");

    // Page 2 (engine limit 2+2): drops the two served, returns the third.
    let c = parse_cursor("t1:200:2").unwrap();
    let data = search_data(vec![
        summary(1, 200),
        summary(2, 200),
        summary(3, 200),
        summary(4, 100),
    ]);
    let r = to_search_result(data, 2, Some(&c));
    let ids: Vec<&str> = r.traces.iter().map(|t| t.trace_id.as_str()).collect();
    assert_eq!(ids, vec!["03".repeat(16).as_str(), "04".repeat(16).as_str()]);
    assert_eq!(
        r.anchor.as_ref().unwrap().next,
        "t1:100:1",
        "count resets when the tail moves past the tie run"
    );
}

#[test]
fn cursor_anchors_on_the_rank_key_not_the_envelope() {
    // A page tail whose envelope start (100) is far older than its rank
    // (300, its newest matched span). Anchoring on the envelope would
    // gap every trace ranked between 100 and 300 — the cursor must
    // carry the RANK key. Caught live against multi-span demo traces.
    let r = to_search_result(
        search_data(vec![summary(1, 400), summary_ranked(2, 100, 300)]),
        2,
        None,
    );
    assert_eq!(r.anchor.as_ref().unwrap().next, "t1:300:1");

    // And the anchor page drops served ties by RANK, not envelope.
    let c = parse_cursor("t1:300:1").unwrap();
    let data = search_data(vec![
        summary_ranked(2, 100, 300), // the served tail, re-emitted first
        summary(3, 250),
    ]);
    let r = to_search_result(data, 2, Some(&c));
    let ids: Vec<&str> = r.traces.iter().map(|t| t.trace_id.as_str()).collect();
    assert_eq!(ids, vec!["03".repeat(16).as_str()]);
}

#[test]
fn work_ceiling_partial_reaches_the_wire() {
    let mut b = sfsq::traces::StatusBuilder::new();
    b.add(sfsq::traces::PartialReason::WorkCeiling);
    let data = SearchData {
        traces: vec![summary(1, 300)],
        status: b.finish(),
        field_kinds: FieldKinds::default(),
    };
    let r = to_search_result(data, 20, None);
    assert_eq!(
        serde_json::to_value(&r.status).unwrap(),
        json!({"partial": ["work_ceiling"]})
    );
}

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
    };
    let v = serde_json::to_value(to_trace_result(&trace_id, data)).unwrap();
    assert_eq!(v["status"], json!({"complete": true}));
    assert_eq!(v["items"]["returned"], 0);
    assert_eq!(v["summary_root"], serde_json::Value::Null);
    assert_eq!(v["spans"], json!([]));
}
