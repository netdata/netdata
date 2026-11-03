//! Tests for the `otel-traces` wire types in `wire.rs` (this file is its
//! child `mod tests`): the request deserialization contract — which JSON
//! shape selects which `TracesMode`, and every strictness rule that
//! turns a bad request into a transport 400 before the handler runs —
//! plus the `InfoResponse` descriptor and `StatusWire` serialization.
//!
//! Fixtures are inline `json!` literals driven straight through
//! serde_json, so the message fragments asserted here are the strings
//! clients see; raw byte slices exist only where a `json!` literal
//! cannot go (duplicate top-level keys). One test reads the published
//! `FUNCTION_UI_SCHEMA.json` — the only file fixture.
//!
//! Pins:
//!
//! - mode selection: no selector means the implicit Functions view,
//!   each of the seven selectors its own mode, `tenant` riding beside
//!   a selector;
//! - strictness: unknown keys, a selector mixed with Functions
//!   parameters, conflicting selectors (reported even when one is also
//!   malformed), selector values that are not objects, present-but-null
//!   selectors, duplicate top-level keys, and a non-object top level —
//!   all client errors, with the message fragments clients match on;
//! - the implicit view's parameter mapping through
//!   `FunctionsParams::search_params`: defaults, relative-window
//!   resolution, and the duration-filter forwarding;
//! - per-mode param objects: self-contained windows, the overview
//!   grid's selections-but-no-duration-bounds rule, the slowest limit,
//!   and the `overview_facets` knob's `Option<bool>` semantics;
//! - `InfoResponse`'s exact descriptor JSON and the untagged response
//!   envelope;
//! - `StatusWire` serialization: the two status shapes, deterministic
//!   partial-reason order, the round trip, the unrepresentable
//!   `{"complete": false}`, and every reason present in the published
//!   UI schema.
//!
//! Not pinned here: the anchor's format (an opaque string on the wire —
//! cursor parsing and the "malformed anchor" rejections are the
//! adapter's pin, `rpc/traces/adapter.rs`), the selections grammar
//! (also the adapter's), semantic validation beyond deserialization
//! (trace-id shape, limit caps, window bounds — the handler's and
//! engine's job), and data-mode response semantics (the handler tests,
//! end to end).
use super::*;
use serde_json::json;
use sfsq::traces::StatusBuilder;

// ── Request / mode resolution ───────────────────────────────────────

fn req(v: serde_json::Value) -> OtelTracesRequest {
    serde_json::from_value(v).expect("request deserializes")
}

fn req_err(v: serde_json::Value) -> String {
    serde_json::from_value::<OtelTracesRequest>(v)
        .expect_err("request must be rejected")
        .to_string()
}

/// The implicit Functions view: no selector becomes Functions params
/// with their serde defaults — `last` falls to the engine default,
/// windows stay at the `0` sentinel — and `tenant` rides beside. A
/// relative window resolves against `now`; a relative `after`
/// resolves against the already-resolved `before`, not `now` again.
#[test]
fn a_request_without_a_mode_selector_selects_the_functions_search_view() {
    let TracesMode::Functions(p) = req(json!({})).mode else {
        panic!("Functions mode expected");
    };
    assert_eq!((p.after, p.before, p.last), (0, 0, 20));
    assert_eq!(p.anchor, None);
    assert!(p.selections.is_empty());

    let r = req(json!({"tenant": "t1"}));
    assert!(matches!(r.mode, TracesMode::Functions(_)));
    assert_eq!(r.tenant.as_deref(), Some("t1"));

    let TracesMode::Functions(p) = req(json!({
        "after": -3600,
        "before": -60,
        "last": 7,
        "anchor": "cursor",
        "selections": {"root_name": ["GET /api"]},
        "timeout": 120000
    }))
    .mode
    else {
        panic!("Functions mode expected");
    };
    assert_eq!((p.after, p.before, p.last), (-3600, -60, 7));
    assert_eq!(p.anchor.as_deref(), Some("cursor"));
    assert_eq!(p.selections["root_name"], ["GET /api"]);

    let search = p.search_params(10_000).unwrap();
    assert_eq!((search.after, search.before), (6_340, 9_940));
}

#[test]
fn the_functions_view_forwards_the_minimum_trace_duration() {
    let TracesMode::Functions(p) = req(json!({"min_trace_duration_ns": 250_000_000})).mode else {
        panic!("Functions mode expected");
    };
    assert_eq!(
        p.search_params(10_000).unwrap().min_trace_duration_ns,
        Some(250_000_000)
    );

    // Omitted stays unset — the bound is opt-in; a client that never
    // sends it keeps the same engine query as before.
    let TracesMode::Functions(p) = req(json!({})).mode else {
        panic!("Functions mode expected");
    };
    assert_eq!(p.search_params(10_000).unwrap().min_trace_duration_ns, None);
}

#[test]
fn the_functions_view_forwards_the_maximum_trace_duration() {
    let TracesMode::Functions(p) = req(json!({"max_trace_duration_ns": 250_000_000})).mode else {
        panic!("Functions mode expected");
    };
    assert_eq!(
        p.search_params(10_000).unwrap().max_trace_duration_ns,
        Some(250_000_000)
    );

    // Omitted stays unset — the bound is opt-in; a client that never
    // sends it keeps the same engine query as before.
    let TracesMode::Functions(p) = req(json!({})).mode else {
        panic!("Functions mode expected");
    };
    assert_eq!(p.search_params(10_000).unwrap().max_trace_duration_ns, None);
}

#[test]
fn overview_facets_rides_with_the_functions_parameters() {
    // The aggregate's facet opt-in is a Functions parameter — it
    // selects the Functions view, and cannot ride with a mode selector
    // (the response's `overview` key is no request field: that
    // selector still means the legacy mode).
    assert!(matches!(
        req(json!({"after": -900, "overview_facets": true})).mode,
        TracesMode::Functions(_)
    ));
    let err = req_err(json!({"overview": {}, "overview_facets": true}));
    assert!(
        err.contains("cannot mix a mode selector with Functions parameters"),
        "{err}"
    );
}

#[test]
fn info_is_the_strict_empty_object() {
    assert!(matches!(req(json!({"info": {}})).mode, TracesMode::Info));
    // Bools (the old wire's two forms), junk scalars, null, arrays,
    // and non-empty objects are all malformed selectors — a malformed
    // selector must not silently select.
    for body in [
        json!({"info": true}),
        json!({"info": false}),
        json!({"info": 7}),
        json!({"info": null}),
        json!({"info": {"extra": 1}}),
        json!({"info": []}),
    ] {
        let err = req_err(body.clone());
        assert!(err.contains("invalid info selector"), "for {body}: {err}");
    }
}

#[test]
fn each_selector_selects_its_mode() {
    assert!(matches!(
        req(json!({"trace": {"id": "00"}})).mode,
        TracesMode::Trace(_)
    ));
    assert!(matches!(
        req(json!({"attributes": {}})).mode,
        TracesMode::Attributes(_)
    ));
    assert!(matches!(
        req(json!({"attribute_values": {"key": "kind"}})).mode,
        TracesMode::AttributeValues(_)
    ));
    assert!(matches!(
        req(json!({"overview": {}})).mode,
        TracesMode::Overview(_)
    ));
    assert!(matches!(
        req(json!({"slowest": {}})).mode,
        TracesMode::Slowest(_)
    ));
    assert!(matches!(
        req(json!({"search": {}})).mode,
        TracesMode::Search(_)
    ));
    assert!(matches!(req(json!({"info": {}})).mode, TracesMode::Info));
}

#[test]
fn a_present_but_null_selector_selects_then_rejects() {
    // serde's stock Option<Value> would swallow the null into "absent"
    // and surface a missing-mode error; the presence-preserving
    // deserializer keeps the selection so the error names the selector.
    for (body, needle) in [
        (json!({"trace": null}), "invalid trace selector"),
        (json!({"overview": null}), "invalid overview selector"),
        (json!({"slowest": null}), "invalid slowest selector"),
        (json!({"search": null}), "invalid search selector"),
    ] {
        let err = req_err(body.clone());
        assert!(err.contains(needle), "for {body}: {err}");
    }
}

#[test]
fn selectors_are_object_only_arrays_reject() {
    // serde-derived struct visitors also accept positional JSON arrays
    // via the seq visitor; the is_object gate before the typed parse
    // closes that hole for every selector value.
    for (body, needle) in [
        (json!({"overview": []}), "invalid overview selector"),
        (json!({"trace": ["00ff", 7]}), "invalid trace selector"),
        (json!({"search": []}), "invalid search selector"),
    ] {
        let err = req_err(body.clone());
        assert!(
            err.contains(needle) && err.contains("expected an object"),
            "for {body}: {err}"
        );
    }
}

#[test]
fn the_top_level_must_be_a_json_object() {
    for body in [json!([]), json!([{}]), json!([1, 2]), json!(7), json!("x")] {
        let err = req_err(body.clone());
        assert!(
            err.contains("otel-traces request object"),
            "for {body}: {err}"
        );
    }
}

/// Both rejection classes are deserialization-time client errors:
/// genuinely unknown keys always, and each legal Functions parameter —
/// even `timeout`, accepted but ignored — beside any selector, as the
/// mixing error rather than an unknown-field error.
#[test]
fn unknown_and_retired_top_level_keys_are_client_errors() {
    for body in [json!({"search": {}, "bogus": 1}), json!({"bogus": 1})] {
        let err = req_err(body.clone());
        assert!(err.contains("unknown field"), "for {body}: {err}");
    }

    for body in [
        json!({"search": {}, "after": 1, "before": 2}),
        json!({"search": {}, "last": 5}),
        json!({"search": {}, "timeout": 30}),
        json!({"search": {}, "min_trace_duration_ns": 1}),
        json!({"search": {}, "max_trace_duration_ns": 1}),
        json!({"trace": {"id": "00"}, "anchor": "x"}),
    ] {
        let err = req_err(body.clone());
        assert!(
            err.contains("cannot mix a mode selector with Functions parameters"),
            "for {body}: {err}"
        );
    }
}

/// The `trace` selector's strictness and parse-through: junk shapes and
/// unknown fields are client errors; a well-formed body carries `id`,
/// `span_cap`, and the optional window through untouched — semantic
/// validation is not the wire's job.
#[test]
fn trace_params_reject_malformed_and_parse_bounds() {
    let cases = [
        (json!({"trace": 7}), "invalid trace selector"),
        (json!({"trace": {}}), "missing field `id`"),
        (json!({"trace": {"id": "00", "bogus": 1}}), "unknown field"),
    ];
    for (body, needle) in cases {
        let err = req_err(body.clone());
        assert!(err.contains(needle), "for {body}: {err}");
    }
    let TracesMode::Trace(ok) = req(json!({"trace": {"id": "00ff", "span_cap": 9}})).mode else {
        panic!("trace mode expected");
    };
    assert_eq!(ok.id, "00ff");
    assert_eq!(ok.span_cap, Some(9));
    assert_eq!(ok.after, None);
    assert_eq!(ok.before, None);

    let TracesMode::Trace(bounded) =
        req(json!({"trace": {"id": "00ff", "after": 100, "before": 200}})).mode
    else {
        panic!("trace mode expected");
    };
    assert_eq!(bounded.after, Some(100));
    assert_eq!(bounded.before, Some(200));
}

#[test]
fn conflicting_selectors_are_a_client_error() {
    let err = req_err(json!({"trace": {}, "overview": {}}));
    assert!(
        err.contains("conflicting mode selectors: trace, overview"),
        "{err}"
    );
    let err = req_err(json!({"overview": {}, "slowest": {}}));
    assert!(
        err.contains("conflicting mode selectors: overview, slowest"),
        "{err}"
    );
    // info is a PEER selector — no precedence.
    let err = req_err(json!({"info": {}, "trace": {"id": "00"}}));
    assert!(
        err.contains("conflicting mode selectors: info, trace"),
        "{err}"
    );
}

#[test]
fn a_conflict_is_reported_before_a_malformed_selector() {
    // All present selectors are counted before any is decoded, so the
    // conflict wins over a malformed selector in the same body.
    let err = req_err(json!({"trace": null, "overview": {}}));
    assert!(
        err.contains("conflicting mode selectors: trace, overview"),
        "{err}"
    );
}

#[test]
fn duplicate_keys_are_rejected_not_last_value_wins() {
    // The manual top-level visitor streams the original map into the
    // derived raw visitor, preserving serde's duplicate-field
    // rejection (a `json!` literal cannot express duplicates — raw
    // bytes only).
    let err = serde_json::from_slice::<OtelTracesRequest>(
        br#"{"trace": {"id": "00"}, "trace": {"id": "ff"}}"#,
    )
    .expect_err("duplicate selector must be rejected");
    assert!(err.to_string().contains("duplicate field"), "{err}");

    let err = serde_json::from_slice::<OtelTracesRequest>(
        br#"{"search": {}, "tenant": "a", "tenant": "b"}"#,
    )
    .expect_err("duplicate tenant must be rejected");
    assert!(err.to_string().contains("duplicate field"), "{err}");
}

#[test]
fn search_params_defaults_and_strictness() {
    let TracesMode::Search(p) = req(json!({"search": {}})).mode else {
        panic!("search mode expected");
    };
    assert_eq!(p.after, 0);
    assert_eq!(p.before, 0);
    assert_eq!(p.limit, sfsq::traces::DEFAULT_SEARCH_LIMIT);
    assert_eq!(p.spans_per_trace, None);
    assert!(p.selections.is_empty());
    assert_eq!(p.anchor, None);

    let TracesMode::Search(p) = req(json!({"search": {
        "after": 10, "before": 20, "limit": 1,
        "selections": {"kind": ["SERVER"]}
    }}))
    .mode
    else {
        panic!("search mode expected");
    };
    assert_eq!((p.after, p.before), (10, 20));
    assert_eq!(p.limit, 1);
    assert_eq!(p.selections["kind"], vec!["SERVER"]);

    let err = req_err(json!({"search": {"last": 5}}));
    assert!(
        err.contains("invalid search selector") && err.contains("unknown field"),
        "the old `last` name is retired: {err}"
    );
}

#[test]
fn windowed_mode_objects_carry_their_own_window() {
    let TracesMode::Overview(p) = req(json!({"overview": {"after": 1, "before": 2}})).mode else {
        panic!("overview mode expected");
    };
    assert_eq!((p.after, p.before), (1, 2));
    let TracesMode::Attributes(p) = req(json!({"attributes": {"after": 3, "before": 4}})).mode
    else {
        panic!("attributes mode expected");
    };
    assert_eq!((p.after, p.before), (3, 4));
    let TracesMode::AttributeValues(p) =
        req(json!({"attribute_values": {"key": "kind", "after": 5, "before": 6}})).mode
    else {
        panic!("attribute_values mode expected");
    };
    assert_eq!((p.after, p.before), (5, 6));
    let TracesMode::Slowest(p) = req(json!({"slowest": {"after": 7, "before": 8}})).mode else {
        panic!("slowest mode expected");
    };
    assert_eq!((p.after, p.before), (7, 8));

    // Omitted windows keep the 0 = "unspecified" sentinel — the
    // adapter's resolve_window defaults are untouched.
    let TracesMode::Overview(p) = req(json!({"overview": {}})).mode else {
        panic!("overview mode expected");
    };
    assert_eq!((p.after, p.before), (0, 0));
}

/// The standalone grid takes the page's `selections` grammar and
/// nothing else of the page's filters: the duration bounds are unknown
/// fields here (the grid never applies them).
#[test]
fn overview_params_take_selections_but_no_duration_bounds() {
    let TracesMode::Overview(p) =
        req(json!({"overview": {"selections": {"name": ["GET", "POST"]}}})).mode
    else {
        panic!("overview mode expected");
    };
    assert_eq!(p.selections["name"], vec!["GET", "POST"]);
    let TracesMode::Overview(p) = req(json!({"overview": {}})).mode else {
        panic!("overview mode expected");
    };
    assert!(p.selections.is_empty());
    for field in [
        "min_trace_duration_ns",
        "max_trace_duration_ns",
        "min_duration_ns",
    ] {
        let msg = req_err(json!({"overview": {field: 1}}));
        assert!(msg.contains("unknown field"), "{field}: {msg}");
    }
}

#[test]
fn tenant_rides_beside_any_mode() {
    let r = req(json!({"search": {}, "tenant": "t1"}));
    assert_eq!(r.tenant.as_deref(), Some("t1"));
    let r = req(json!({"info": {}}));
    assert_eq!(r.tenant, None);
}

// ── Info response ───────────────────────────────────────────────────

/// The capability-discovery descriptor, field for field: the envelope
/// fields, the help string, and `accepted_params` equal to the
/// module's `ACCEPTED_PARAMS` list — the list the UI builds requests
/// from, so a missing entry makes a request field unreachable.
#[test]
fn info_response_shape_is_pinned() {
    let v = serde_json::to_value(InfoResponse::default()).unwrap();
    assert_eq!(
        v,
        json!({
            "mode": "info",
            "version": 1,
            "status": 200,
            "type": "traces",
            "has_history": true,
            "v": 3,
            "accepted_params": [
                "info", "trace", "attributes", "attribute_values", "overview",
                "slowest", "search", "tenant", "after", "before", "last", "anchor", "selections",
                "min_trace_duration_ns", "max_trace_duration_ns", "overview_facets"
            ],
            "required_params": [],
            "help": "Query and visualize OpenTelemetry traces.",
        })
    );
}

#[test]
fn response_envelope_is_untagged() {
    // The Info variant serializes as the bare descriptor object, no
    // enum tag wrapper; the mode field self-describes instead.
    let v = serde_json::to_value(OtelTracesResponse::Info(InfoResponse::default())).unwrap();
    assert!(v.get("version").is_some());
    assert!(v.get("Info").is_none());
    assert_eq!(v.get("mode").and_then(|m| m.as_str()), Some("info"));
}

// ── Status serialization ────────────────────────────────────────────

#[test]
fn complete_status_serializes_as_complete_true() {
    let wire = StatusWire::from(&QueryStatus::Complete);
    assert_eq!(
        serde_json::to_value(&wire).unwrap(),
        json!({"complete": true})
    );
}

#[test]
fn partial_status_serializes_reason_names_deterministically() {
    // Insertion order must not matter — the engine's BTreeSet renders
    // deterministically, and the wire names are pinned snake_case.
    let mut b = StatusBuilder::new();
    b.add(PartialReason::SourceFailure);
    b.add(PartialReason::SizeCap);
    let wire = StatusWire::from(&b.finish());
    assert_eq!(
        serde_json::to_value(&wire).unwrap(),
        json!({"partial": ["size_cap", "source_failure"]})
    );
}

/// Every engine reason. The exhaustive match fails compilation when the
/// engine gains a reason, so the pins below cannot silently miss it.
fn every_partial_reason() -> Vec<PartialReason> {
    let all = vec![
        PartialReason::SizeCap,
        PartialReason::SourceFailure,
        PartialReason::WorkCeiling,
        PartialReason::Cancelled,
        PartialReason::OverviewCeiling,
        PartialReason::RollupAbsent,
        PartialReason::SlowestCeiling,
        PartialReason::RemoteUnavailable,
    ];
    for reason in &all {
        match reason {
            PartialReason::SizeCap
            | PartialReason::SourceFailure
            | PartialReason::WorkCeiling
            | PartialReason::Cancelled
            | PartialReason::OverviewCeiling
            | PartialReason::RollupAbsent
            | PartialReason::SlowestCeiling
            | PartialReason::RemoteUnavailable => {}
        }
    }
    all
}

#[test]
fn every_partial_reason_wire_name_is_pinned() {
    let mut b = StatusBuilder::new();
    for reason in every_partial_reason() {
        b.add(reason);
    }
    let wire = StatusWire::from(&b.finish());
    assert_eq!(
        serde_json::to_value(&wire).unwrap(),
        json!({"partial": [
            "size_cap", "source_failure", "work_ceiling", "cancelled",
            "overview_ceiling", "rollup_absent", "slowest_ceiling",
            "remote_unavailable"
        ]})
    );
}

#[test]
fn every_partial_reason_is_in_the_published_schema() {
    // The published Functions schema closes the reason list; a wire name
    // missing there makes a valid response fail schema validation.
    let path = concat!(
        env!("CARGO_MANIFEST_DIR"),
        "/../../plugins.d/FUNCTION_UI_SCHEMA.json"
    );
    let schema: serde_json::Value =
        serde_json::from_str(&std::fs::read_to_string(path).unwrap()).unwrap();
    let published: Vec<&str> = schema["definitions"]["traces_status"]["oneOf"]
        .as_array()
        .unwrap()
        .iter()
        .filter_map(|variant| variant["properties"]["partial"]["items"]["enum"].as_array())
        .flatten()
        .map(|name| name.as_str().unwrap())
        .collect();
    for reason in every_partial_reason() {
        let wire = serde_json::to_value(PartialReasonWire::from(reason)).unwrap();
        let name = wire.as_str().unwrap();
        assert!(
            published.contains(&name),
            "{name} is not in {path}: {published:?}"
        );
    }
}

#[test]
fn slowest_params_reject_junk_and_unknown_fields() {
    for (body, needle) in [
        (json!({"slowest": 7}), "invalid slowest selector"),
        (json!({"slowest": {"bogus": 1}}), "unknown field"),
    ] {
        let err = req_err(body.clone());
        assert!(err.contains(needle), "for {body}: {err}");
    }
    let TracesMode::Slowest(p) = req(json!({"slowest": {}})).mode else {
        panic!("slowest mode expected");
    };
    assert_eq!(p.limit, None);
    let TracesMode::Slowest(p) = req(json!({"slowest": {"limit": 5}})).mode else {
        panic!("slowest mode expected");
    };
    assert_eq!(p.limit, Some(5));
}

#[test]
fn overview_facets_knob_parses_and_junk_is_rejected() {
    let facets = |v: serde_json::Value| -> Option<bool> {
        let TracesMode::Overview(p) = req(v).mode else {
            panic!("overview mode expected");
        };
        p.facets
    };
    assert_eq!(facets(json!({"overview": {}})), None);
    assert_eq!(facets(json!({"overview": {"facets": true}})), Some(true));
    assert_eq!(facets(json!({"overview": {"facets": false}})), Some(false));
    assert_eq!(
        facets(json!({"overview": {"facets": null}})),
        None,
        "null means off, not an error (Option<bool> semantics)"
    );
    for body in [
        json!({"overview": {"facets": "yes"}}),
        json!({"overview": {"bogus": 1}}),
    ] {
        let err = req_err(body.clone());
        assert!(
            err.contains("invalid overview selector"),
            "for {body}: {err}"
        );
    }
}

/// The untagged enum reads back exactly what it writes: the distinct
/// field names (`complete` vs `partial`) make the round trip a real
/// contract even though both directions are derived.
#[test]
fn status_wire_round_trips() {
    for status in [
        StatusWire::Complete {
            complete: CompleteTrue,
        },
        StatusWire::Partial {
            partial: vec![PartialReasonWire::SizeCap, PartialReasonWire::Cancelled],
        },
    ] {
        let v = serde_json::to_value(&status).unwrap();
        let back: StatusWire = serde_json::from_value(v).unwrap();
        assert_eq!(back, status);
    }
}

#[test]
fn complete_false_is_unrepresentable() {
    // `{"complete": false}` means nothing — it must fail to
    // deserialize rather than masquerade as a status.
    assert!(serde_json::from_value::<StatusWire>(json!({"complete": false})).is_err());
}
