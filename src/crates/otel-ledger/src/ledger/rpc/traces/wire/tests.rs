use super::*;
use serde_json::json;
use sfsq::traces::StatusBuilder;

// ── Request / mode resolution ───────────────────────────────────────

fn req(v: serde_json::Value) -> OtelTracesRequest {
    serde_json::from_value(v).expect("request deserializes")
}

#[test]
fn empty_request_is_search() {
    // The bridge deserializes a missing payload as `{}`; a request with
    // no selector is the default data mode.
    assert_eq!(req(json!({})).mode(), Ok(RequestMode::Search));
}

#[test]
fn info_selects_info() {
    assert_eq!(req(json!({"info": true})).mode(), Ok(RequestMode::Info));
}

#[test]
fn info_false_from_the_get_shim_is_search() {
    // The rt-level GET shim always synthesizes an `info` field; `false`
    // must fall through to the data modes.
    assert_eq!(
        req(json!({"info": false, "after": 1, "before": 2})).mode(),
        Ok(RequestMode::Search)
    );
}

#[test]
fn info_wins_over_data_selectors() {
    // The logs `info`-over-`files` precedence: discovery works even if a
    // client also sends a data selector.
    assert_eq!(
        req(json!({"info": true, "trace": {"id": "00"}})).mode(),
        Ok(RequestMode::Info)
    );
}

#[test]
fn each_data_selector_selects_its_mode() {
    assert_eq!(
        req(json!({"trace": {"id": "00"}})).mode(),
        Ok(RequestMode::Trace)
    );
    assert_eq!(
        req(json!({"attributes": {}})).mode(),
        Ok(RequestMode::Attributes)
    );
    assert_eq!(
        req(json!({"attribute_values": {}})).mode(),
        Ok(RequestMode::AttributeValues)
    );
    assert_eq!(
        req(json!({"overview": {}})).mode(),
        Ok(RequestMode::Overview)
    );
    assert_eq!(
        req(json!({"slowest": {}})).mode(),
        Ok(RequestMode::Slowest)
    );
}

#[test]
fn a_present_but_null_selector_still_selects_its_mode() {
    // serde's stock Option<Value> would swallow the null into "absent"
    // and silently fall through to search; the presence-preserving
    // deserializer keeps the selection so the typed parse can reject
    // the null loudly.
    assert_eq!(req(json!({"trace": null})).mode(), Ok(RequestMode::Trace));
    assert_eq!(
        req(json!({"overview": null})).mode(),
        Ok(RequestMode::Overview)
    );
    assert_eq!(req(json!({"slowest": null})).mode(), Ok(RequestMode::Slowest));
}

#[test]
fn trace_params_reject_null_and_malformed_selectors() {
    let cases = [
        (json!({"trace": null}), "invalid trace selector"),
        (json!({"trace": 7}), "invalid trace selector"),
        (json!({"trace": {}}), "missing field `id`"),
        (json!({"trace": {"id": "00", "bogus": 1}}), "unknown field"),
    ];
    for (body, needle) in cases {
        let err = req(body.clone()).trace_params().expect_err("must reject");
        assert!(err.contains(needle), "for {body}: {err}");
    }
    let ok = req(json!({"trace": {"id": "00ff", "span_cap": 9}}))
        .trace_params()
        .unwrap();
    assert_eq!(ok.id, "00ff");
    assert_eq!(ok.span_cap, Some(9));
}

#[test]
fn conflicting_data_selectors_are_a_client_error() {
    let err = req(json!({"trace": {}, "overview": {}}))
        .mode()
        .expect_err("two data selectors must conflict");
    assert_eq!(err, ModeConflict(vec!["trace", "overview"]));
    assert_eq!(
        err.to_string(),
        "conflicting mode selectors: trace, overview"
    );
}

#[test]
fn common_fields_deserialize_with_defaults() {
    let r = req(json!({}));
    assert_eq!(r.after, 0);
    assert_eq!(r.before, 0);
    assert_eq!(r.tenant, None);
    assert_eq!(r.timeout, None);

    let r = req(json!({"after": 10, "before": 20, "tenant": "t1", "timeout": 30}));
    assert_eq!((r.after, r.before), (10, 20));
    assert_eq!(r.tenant.as_deref(), Some("t1"));
    assert_eq!(r.timeout, Some(30));
}

// ── Info response ───────────────────────────────────────────────────

#[test]
fn info_response_shape_is_pinned() {
    let v = serde_json::to_value(InfoResponse::default()).unwrap();
    assert_eq!(
        v,
        json!({
            "version": 1,
            "status": 200,
            "accepted_params": [
                "info", "trace", "attributes", "attribute_values", "overview",
                "slowest", "tenant", "after", "before", "last", "anchor"
            ],
            "required_params": [],
            "help": "Query and visualize OpenTelemetry traces.",
        })
    );
}

#[test]
fn response_envelope_is_untagged() {
    // The Info variant serializes as the bare descriptor object, no
    // enum tag wrapper.
    let v = serde_json::to_value(OtelTracesResponse::Info(InfoResponse::default())).unwrap();
    assert!(v.get("version").is_some());
    assert!(v.get("Info").is_none());
}

// ── Status serialization ────────────────────────────────────────────

#[test]
fn complete_status_serializes_as_complete_true() {
    let wire = StatusWire::from(&QueryStatus::Complete);
    assert_eq!(serde_json::to_value(&wire).unwrap(), json!({"complete": true}));
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

#[test]
fn every_partial_reason_wire_name_is_pinned() {
    let mut b = StatusBuilder::new();
    b.add(PartialReason::SizeCap);
    b.add(PartialReason::SourceFailure);
    b.add(PartialReason::WorkCeiling);
    b.add(PartialReason::Cancelled);
    b.add(PartialReason::OverviewCeiling);
    b.add(PartialReason::RollupAbsent);
    b.add(PartialReason::SlowestCeiling);
    let wire = StatusWire::from(&b.finish());
    assert_eq!(
        serde_json::to_value(&wire).unwrap(),
        json!({"partial": [
            "size_cap", "source_failure", "work_ceiling", "cancelled",
            "overview_ceiling", "rollup_absent", "slowest_ceiling"
        ]})
    );
}

#[test]
fn slowest_params_reject_null_junk_and_unknown_fields() {
    for (body, needle) in [
        (json!({"slowest": null}), "invalid slowest selector"),
        (json!({"slowest": 7}), "invalid slowest selector"),
        (json!({"slowest": {"bogus": 1}}), "unknown field"),
    ] {
        let err = req(body.clone()).slowest_params().expect_err("must reject");
        assert!(err.contains(needle), "for {body}: {err}");
    }
    assert_eq!(req(json!({"slowest": {}})).slowest_params().unwrap().limit, None);
    assert_eq!(
        req(json!({"slowest": {"limit": 5}})).slowest_params().unwrap().limit,
        Some(5)
    );
}

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
