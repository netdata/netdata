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
    let v = serde_json::to_value(to_trace_result(&trace_id, data, CoverageWire { after: 0, before: u32::MAX })).unwrap();

    assert_eq!(v["version"], 1);
    assert_eq!(v["trace_id"], "11111111111111111111111111111111");
    assert_eq!(v["coverage"], json!({"after": 0, "before": 4_294_967_295_u32}));
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
fn every_engine_owner_is_deliberately_placed_in_the_wire_grammar() {
    // Exhaustive match: a NEW engine owner fails compilation here until
    // its wire spelling (or deliberate absence) is decided.
    fn wire_spelling(owner: AttributeOwner) -> Option<&'static str> {
        match owner {
            AttributeOwner::Resource => Some("resource"),
            AttributeOwner::Span => Some("span"),
            AttributeOwner::Instrumentation => Some("instrumentation"),
            AttributeOwner::Event => Some("event"),
            AttributeOwner::Link => Some("link"),
            // Bare builtin words, not an owner prefix.
            AttributeOwner::Builtin => None,
            // Not enumerable; selections are always owner-qualified.
            AttributeOwner::Any => None,
        }
    }
    for (word, owner) in OWNER_WORDS {
        assert_eq!(wire_spelling(owner), Some(word));
    }
    assert_eq!(wire_spelling(AttributeOwner::Builtin), None);
    assert_eq!(wire_spelling(AttributeOwner::Any), None);
}

#[test]
fn rendered_keys_round_trip_through_the_selection_grammar() {
    // render ∘ parse = identity: every enumerated key feeds straight
    // back as a selection key or a values request.
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
        let rendered = render_attribute_key(owner, &key);
        assert_eq!(
            parse_enumeration_key(&rendered).unwrap(),
            (owner, key),
            "{rendered}"
        );
    }
    // Every builtin word round-trips too.
    for builtin in BuiltinField::ALL {
        let rendered =
            render_attribute_key(AttributeOwner::Builtin, &AttributeKey::Builtin(builtin));
        assert_eq!(
            parse_enumeration_key(&rendered).unwrap(),
            (AttributeOwner::Builtin, AttributeKey::Builtin(builtin)),
            "{rendered}"
        );
    }
}

#[test]
fn owner_words_parse_including_builtin() {
    assert_eq!(parse_owner_word("resource").unwrap(), AttributeOwner::Resource);
    assert_eq!(parse_owner_word("builtin").unwrap(), AttributeOwner::Builtin);
    assert!(parse_owner_word("any").is_err(), "Any stays un-nameable");
    assert!(parse_owner_word("bogus").is_err());
    assert!(parse_owner_word("Resource").is_err(), "case-sensitive by design");
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
    let p = build_predicate(&selections, Some(5), Some(10), Some(100), Some(200)).unwrap();
    assert_eq!(p.conditions.len(), 6);
    // Sorted keys: resource.service.name, status; then span-duration
    // bounds, then trace-duration bounds.
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
    assert_eq!(
        p.conditions[4].target,
        PredicateTarget::Builtin(BuiltinField::TraceDuration)
    );
    assert_eq!(p.conditions[4].op, CompareOp::Gte);
    assert_eq!(
        p.conditions[5].target,
        PredicateTarget::Builtin(BuiltinField::TraceDuration)
    );
    assert_eq!(p.conditions[5].op, CompareOp::Lte);
}

#[test]
fn inverted_trace_duration_bounds_are_a_client_error() {
    let err = build_predicate(&HashMap::new(), None, None, Some(10), Some(5))
        .expect_err("inverted bounds must reject");
    assert!(err.contains("min_trace_duration_ns"), "{err}");
}

// ── Search: cursor ──────────────────────────────────────────────────

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
        "t2:9000:10000:5:00ff:0",   // served 0
        "t2:10000:9000:5:00ff:1",   // inverted window
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

const WIN: (u32, u32) = (9_000, 10_000);
const WIN_COVERAGE: CoverageWire = CoverageWire {
    after: 5_400,
    before: 13_600,
};

#[test]
fn completion_capture_range_clamps_and_saturates() {
    assert_eq!(completion_capture_range(&(9_000..10_000)), 5_400..13_600);
    assert_eq!(
        completion_capture_range(&(0..20_000)),
        0..40_000
    );
    let day = 86_400;
    assert_eq!(
        completion_capture_range(&(day..day * 3)),
        0..day * 4
    );
    assert_eq!(
        completion_capture_range(&(u32::MAX - 100..u32::MAX)),
        u32::MAX - 100 - 3_600..u32::MAX
    );
}

#[test]
fn cursor_freezes_the_original_window_while_coverage_declares_the_widened_range() {
    let r = to_search_result(
        search_data(vec![summary(1, 300), summary(2, 200)]),
        2,
        None,
        WIN,
        WIN_COVERAGE,
    );
    let v = serde_json::to_value(&r).unwrap();
    assert_eq!(
        v["completion_coverage"],
        serde_json::json!({"after": 5_400, "before": 13_600})
    );
    let cursor = parse_cursor(v["anchor"]["next"].as_str().unwrap()).unwrap();
    assert_eq!((cursor.after_s, cursor.before_s), WIN);
}

#[test]
fn full_page_emits_a_cursor_and_short_page_does_not() {
    let r = to_search_result(
        search_data(vec![summary(1, 300), summary(2, 200)]),
        2,
        None,
        WIN,
        WIN_COVERAGE,
    );
    assert_eq!(r.items.returned, 2);
    assert_eq!(
        r.anchor.as_ref().unwrap().next,
        format!("t2:9000:10000:200:{}:2", "02".repeat(16))
    );

    let r = to_search_result(search_data(vec![summary(1, 300)]), 2, None, WIN, WIN_COVERAGE);
    assert!(r.anchor.is_none(), "a short page ends the walk");
}

#[test]
fn anchor_page_drops_everything_at_or_above_the_after_key() {
    // Served: ranks above 200, plus rank-200 ids ≤ 02. The engine
    // re-emits the whole prefix (same query, larger limit).
    let c = cursor(WIN.0, WIN.1, 200, 0x02, 2);
    let data = search_data(vec![
        summary(1, 300),  // above the key → served
        summary(2, 200),  // the key itself → served
        summary(3, 200),  // rank tie, id above the key → fresh
        summary(4, 100),
    ]);
    let r = to_search_result(data, 2, Some(&c), WIN, WIN_COVERAGE);
    let ids: Vec<&str> = r.traces.iter().map(|t| t.trace_id.as_str()).collect();
    assert_eq!(ids, vec!["03".repeat(16).as_str(), "04".repeat(16).as_str()]);
    // served accumulates: 2 before + 2 this page.
    assert_eq!(
        r.anchor.as_ref().unwrap().next,
        format!("t2:9000:10000:100:{}:4", "04".repeat(16))
    );
}

#[test]
fn straddling_trace_does_not_duplicate_below_the_boundary() {
    // The review-caught flaw: trace F matched spans at 70 and 10; page 1
    // served it at rank 70 with tail U at rank 50. The next page reruns
    // the SAME window, so F still ranks 70 — at-or-above the key — and
    // drops, instead of re-entering at rank 10 as the narrowed-window
    // design allowed.
    let c = cursor(WIN.0, WIN.1, 50, 0x1A, 2); // tail U(50), served F+U
    let data = search_data(vec![
        summary_ranked(0x0F, 10, 70), // F: full-window rank stays 70
        summary(0x1A, 50),            // U: the key
        summary(0x1B, 30),            // W: genuinely fresh
    ]);
    let r = to_search_result(data, 2, Some(&c), WIN, WIN_COVERAGE);
    let ids: Vec<&str> = r.traces.iter().map(|t| t.trace_id.as_str()).collect();
    assert_eq!(ids, vec!["1b".repeat(16).as_str()], "only W is fresh");
}

#[test]
fn a_walk_at_the_served_cap_emits_no_further_cursor() {
    // The over-fetch allowance can't grow past the cap: the final page
    // is full but carries no continuation.
    let c = cursor(WIN.0, WIN.1, 500, 0x01, 9_999);
    let data = search_data(vec![summary(0x02, 400), summary(0x03, 300), summary(0x04, 200)]);
    let r = to_search_result(data, 2, Some(&c), WIN, WIN_COVERAGE);
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
        field_kinds: FieldKinds::default(),
    };
    let r = to_search_result(data, 2, None, WIN, WIN_COVERAGE);
    assert_eq!(r.items.returned, 2, "the page itself is full");
    assert!(r.anchor.is_none());
    assert_eq!(
        serde_json::to_value(&r.status).unwrap(),
        json!({"partial": ["work_ceiling"]})
    );
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
    let r = to_search_result(data, 20, None, WIN, WIN_COVERAGE);
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
    let v = serde_json::to_value(to_trace_result(&trace_id, data, CoverageWire { after: 0, before: u32::MAX })).unwrap();
    assert_eq!(v["status"], json!({"complete": true}));
    assert_eq!(v["items"]["returned"], 0);
    assert_eq!(v["summary_root"], serde_json::Value::Null);
    assert_eq!(v["spans"], json!([]));
}
