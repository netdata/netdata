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
//!   each of the three selectors its own mode, `tenant` riding beside
//!   a selector;
//! - strictness: unknown keys, a selector mixed with Functions
//!   parameters, conflicting selectors (reported even when one is also
//!   malformed), selector values that are not objects, present-but-null
//!   selectors, duplicate top-level keys, and a non-object top level —
//!   all client errors, with the message fragments clients match on;
//! - the implicit view's parameter mapping through
//!   `FunctionsParams::search_params`: defaults, relative-window
//!   resolution, and the chips and text forwarding;
//! - per-mode param objects: self-contained windows;
//! - `InfoResponse`'s exact descriptor JSON and the untagged response
//!   envelope;
//! - `StatusWire` serialization: the two status shapes, deterministic
//!   partial-reason order, the round trip, the unrepresentable
//!   `{"complete": false}`, and every reason present in the published
//!   UI schema.
//!
//! Not pinned here: the anchor's format (an opaque string on the wire —
//! cursor parsing and the "malformed anchor" rejections are the
//! adapter's pin, `rpc/traces/adapter.rs`), the chip key grammar
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
    assert!(p.conditions.is_empty() && p.text.is_empty());

    let r = req(json!({"tenant": "t1"}));
    assert!(matches!(r.mode, TracesMode::Functions(_)));
    assert_eq!(r.tenant.as_deref(), Some("t1"));

    let TracesMode::Functions(p) = req(json!({
        "after": -3600,
        "before": -60,
        "last": 7,
        "anchor": "cursor",
        "conditions": [{"key": "root_name", "op": "=", "values": ["GET /api"]}],
        "text": ["timeout"],
        "timeout": 120000
    }))
    .mode
    else {
        panic!("Functions mode expected");
    };
    assert_eq!((p.after, p.before, p.last), (-3600, -60, 7));
    assert_eq!(p.anchor.as_deref(), Some("cursor"));
    assert_eq!(p.conditions[0].key, "root_name");
    assert_eq!(p.text, ["timeout"]);

    let search = p.search_params(10_000).unwrap();
    assert_eq!((search.after, search.before), (6_340, 9_940));
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
        req(json!({"suggest": {"key": "kind"}})).mode,
        TracesMode::Suggest(_)
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
        (json!({"suggest": null}), "invalid suggest selector"),
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
        (json!({"suggest": []}), "invalid suggest selector"),
        (json!({"trace": ["00ff", 7]}), "invalid trace selector"),
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
    for body in [
        json!({"info": {}, "bogus": 1}),
        json!({"bogus": 1}),
        json!({"attribute_values": {"key": "kind"}}),
        json!({"selections": {"name": ["GET"]}}),
        json!({"min_trace_duration_ns": 1}),
        json!({"max_trace_duration_ns": 1}),
    ] {
        let err = req_err(body.clone());
        assert!(err.contains("unknown field"), "for {body}: {err}");
    }

    for body in [
        json!({"info": {}, "after": 1, "before": 2}),
        json!({"info": {}, "last": 5}),
        json!({"info": {}, "timeout": 30}),
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
/// unknown fields are client errors; a well-formed body carries `id`
/// and the optional window through untouched — semantic
/// validation is not the wire's job.
#[test]
fn trace_params_reject_malformed_and_parse_bounds() {
    let cases = [
        (json!({"trace": 7}), "invalid trace selector"),
        (json!({"trace": {}}), "missing field `id`"),
        (json!({"trace": {"id": "00", "bogus": 1}}), "unknown field"),
        (
            json!({"trace": {"id": "00", "span_cap": 9}}),
            "unknown field",
        ),
    ];
    for (body, needle) in cases {
        let err = req_err(body.clone());
        assert!(err.contains(needle), "for {body}: {err}");
    }
    let TracesMode::Trace(ok) = req(json!({"trace": {"id": "00ff"}})).mode else {
        panic!("trace mode expected");
    };
    assert_eq!(ok.id, "00ff");
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
    let err = req_err(json!({"trace": {}, "suggest": {}}));
    assert!(
        err.contains("conflicting mode selectors: trace, suggest"),
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
    let err = req_err(json!({"trace": null, "suggest": {}}));
    assert!(
        err.contains("conflicting mode selectors: trace, suggest"),
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
        br#"{"info": {}, "tenant": "a", "tenant": "b"}"#,
    )
    .expect_err("duplicate tenant must be rejected");
    assert!(err.to_string().contains("duplicate field"), "{err}");
}

#[test]
fn windowed_mode_objects_carry_their_own_window() {
    let TracesMode::Suggest(p) =
        req(json!({"suggest": {"key": "kind", "after": 5, "before": 6}})).mode
    else {
        panic!("suggest mode expected");
    };
    assert_eq!((p.after, p.before), (5, 6));

    // Omitted windows keep the 0 = "unspecified" sentinel — the
    // adapter's resolve_window defaults are untouched.
    let TracesMode::Suggest(p) = req(json!({"suggest": {"key": "kind"}})).mode else {
        panic!("suggest mode expected");
    };
    assert_eq!((p.after, p.before), (0, 0));
}

#[test]
fn suggest_params_parse_with_defaults_and_reject_strays() {
    let TracesMode::Suggest(p) = req(json!({"suggest": {}})).mode else {
        panic!("suggest mode expected");
    };
    assert_eq!(
        (p.after, p.before, p.key, p.term.as_str(), p.max_items),
        (0, 0, None, "", None)
    );
    let TracesMode::Suggest(p) =
        req(json!({"suggest": {"key": "name", "term": "ab", "max_items": 7}})).mode
    else {
        panic!("suggest mode expected");
    };
    assert_eq!(
        (p.key.as_deref(), p.term.as_str(), p.max_items),
        (Some("name"), "ab", Some(7))
    );
    let err = req_err(json!({"suggest": {"terms": "ab"}}));
    assert!(err.contains("invalid suggest selector"), "{err}");
    let err = req_err(json!({"info": {}, "suggest": {}}));
    assert!(
        err.contains("conflicting mode selectors: info, suggest"),
        "{err}"
    );
}

#[test]
fn tenant_rides_beside_any_mode() {
    let r = req(json!({"trace": {"id": "00"}, "tenant": "t1"}));
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
                "info", "trace", "suggest", "tenant", "after", "before", "last", "anchor", "conditions", "text"
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
        PartialReason::RemoteUnavailable,
        PartialReason::ByteBudget,
        PartialReason::Busy,
        PartialReason::FileBudget,
        PartialReason::Deadline,
        PartialReason::NoNewMatches,
        PartialReason::TraceFragments,
        PartialReason::DroppedFields,
        PartialReason::StackFieldHighCard,
    ];
    for reason in &all {
        match reason {
            PartialReason::SizeCap
            | PartialReason::SourceFailure
            | PartialReason::WorkCeiling
            | PartialReason::Cancelled
            | PartialReason::OverviewCeiling
            | PartialReason::RollupAbsent
            | PartialReason::RemoteUnavailable
            | PartialReason::ByteBudget
            | PartialReason::Busy
            | PartialReason::FileBudget
            | PartialReason::Deadline
            | PartialReason::NoNewMatches
            | PartialReason::TraceFragments
            | PartialReason::DroppedFields
            | PartialReason::StackFieldHighCard => {}
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
            "overview_ceiling", "rollup_absent", "remote_unavailable", "byte_budget", "busy",
            "file_budget", "deadline", "no_new_matches", "trace_fragments", "dropped_fields",
            "stack_field_high_card"
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
