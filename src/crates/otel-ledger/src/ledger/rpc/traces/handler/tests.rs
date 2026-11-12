//! The `otel-traces` Function handler pinned end to end through
//! `on_call`: every mode's wire contract (info, trace, search, the
//! enumeration pair, and the Functions view's embedded window
//! aggregate), window canonicalization, tie-safe
//! pagination, the scope gates, tenant scoping, and the transport
//! status boundary (shape errors 400, semantic errors 500).
//!
//! Fixtures are real traces: OTLP requests written into real WALs or
//! indexed by the production indexing, installed through the shared
//! `fixtures` module into fresh registries over throwaway dirs. What
//! the lower layers already pin is not re-tested here: wire shapes in
//! `wire/tests.rs`, adapter mappings and window resolution in
//! `adapter/tests.rs`, source capture in `sources/tests.rs`, engine
//! semantics in the `sfsq` traces suites, and the remote read-back
//! path in `handler/remote_tests.rs`.
use super::*;
use crate::ledger::rpc::traces::fixtures::{
    indexed_traces, install_sealed, install_sfst, install_wal, make_registries, otlp_req,
    otlp_req_svc,
};
use file_lifecycle::registry::TenantRegistries;
use otel_bridge::function::ProgressState;
use serde_json::json;
use tokio_util::sync::CancellationToken;

fn make_handler_over(registries: Arc<RwLock<TenantRegistries>>) -> OtelTracesHandler {
    // min_entries 4: the 8-entry search-corpus WAL indexes into two real
    // chunks, a 3-span fixture WAL stays a tail — the end-to-end tests
    // cross the chunk-indexing and tail paths for real.
    OtelTracesHandler::new(
        registries,
        Arc::new(ChunkCache::new(64 * 1024 * 1024)),
        Arc::new(crate::ledger::wal_prefix::WalPrefixes::new(4)),
        None,
    )
}

fn make_handler() -> OtelTracesHandler {
    make_handler_over(make_registries())
}

fn make_ctx() -> FunctionCallContext {
    FunctionCallContext::new(
        "tx-test".to_string(),
        ProgressState::new(),
        CancellationToken::new(),
    )
}

async fn call_on(
    h: &OtelTracesHandler,
    v: serde_json::Value,
) -> netdata_plugin_error::Result<OtelTracesResponse> {
    let req: OtelTracesRequest = serde_json::from_value(v).unwrap();
    h.on_call(make_ctx(), req).await
}

async fn call(v: serde_json::Value) -> netdata_plugin_error::Result<OtelTracesResponse> {
    call_on(&make_handler(), v).await
}

/// `info` returns the capability descriptor: protocol version, and the
/// full accepted-params list — the Function's self-description, with
/// no required parameters.
#[tokio::test]
async fn info_returns_the_descriptor() {
    let resp = call(json!({"info": {}})).await.unwrap();
    let v = serde_json::to_value(&resp).unwrap();
    assert_eq!(v["mode"], "info");
    assert_eq!(v["version"], 1);
    assert_eq!(v["status"], 200);
    assert_eq!(v["type"], "traces");
    assert_eq!(v["has_history"], true);
    assert_eq!(v["v"], 3);
    assert_eq!(
        v["accepted_params"],
        json!([
            "info",
            "trace",
            "suggest",
            "explore",
            "tenant",
            "after",
            "before",
            "last",
            "anchor",
            "conditions",
            "text",
            "skip_overview"
        ])
    );
    assert_eq!(v["required_params"], json!([]));
}

/// A request with no mode selector (top-level Functions params only)
/// IS the Functions view: the search page wrapped as `{type, data}`. On
/// an empty agent that's an empty COMPLETE page, never an error.
#[tokio::test]
async fn a_mode_less_request_wraps_the_search_page_as_traces() {
    let resp = call(json!({"after": -3600, "before": 0, "last": 7}))
        .await
        .unwrap();
    let v = serde_json::to_value(&resp).unwrap();
    assert_eq!(v["status"], 200);
    assert_eq!(v["type"], "traces");
    assert_eq!(v["data"]["mode"], "search");
    assert_eq!(v["data"]["version"], 1);
    assert_eq!(v["data"]["status"], json!({"complete": true}));
    assert_eq!(
        v["data"]["items"],
        json!({"returned": 0, "max_to_return": 7})
    );
    assert_eq!(v["data"]["traces"], json!([]));
    assert!(
        v["data"].get("anchor").is_none(),
        "a short page has no next cursor"
    );
}

#[tokio::test]
async fn every_mode_is_implemented_an_empty_agent_answers_them_all() {
    // The mode catalog is complete: every selector answers cleanly on
    // an empty agent — none is a not-implemented error.
    for body in [
        json!({"trace": {"id": "481451a49451a81bd8807bca88d35e81"}}),
        json!({"suggest": {}}),
        json!({"suggest": {"key": "name", "term": "x"}}),
        json!({"explore": {"histogram": {}, "rows": {}}}),
    ] {
        call(body.clone())
            .await
            .unwrap_or_else(|e| panic!("{body}: {e}"));
    }
}

// Conflicting/malformed selector bodies never reach on_call — they
// fail request DESERIALIZATION (pinned in the wire tests) and the
// bridge maps them to transport 400s (pinned by the raw-transport
// tests at the end of this file).

/// The Function declaration the bridge advertises: global visibility,
/// the `traces` tag, and the SIGNED_ID | SAME_SPACE | COMMERCIAL_SPACE |
/// SENSITIVE_DATA access mask.
#[test]
fn declaration_advertises_otel_traces() {
    let d = make_handler().declaration();
    assert_eq!(d.name, "otel-traces");
    assert!(d.global);
    assert_eq!(d.tags.as_deref(), Some("traces"));
    assert_eq!(
        d.access,
        Some(
            HttpAccess::SIGNED_ID
                | HttpAccess::SAME_SPACE
                | HttpAccess::COMMERCIAL_SPACE
                | HttpAccess::SENSITIVE_DATA
        )
    );
}

// ── The trace mode ──────────────────────────────────────────────────

/// The fixture trace: 3 spans, ids [0x11;16]/[i;8], span 1 the root.
const FIXTURE_TRACE_ID: &str = "11111111111111111111111111111111";

/// A fresh WAL holding the fixture trace — the trace mode's workhorse.
async fn handler_with_fixture_wal() -> OtelTracesHandler {
    let registries = make_registries();
    install_wal(
        &registries,
        "default",
        1,
        vec![otlp_req(0x11, 3, 1_000_000_000)],
    )
    .await;
    make_handler_over(registries)
}

#[tokio::test]
async fn an_indexed_traces_fixture_serves_as_a_local_sealed_file() {
    // The fixture is what traces indexing writes: its summary spans the
    // spans' seconds and counts them, and the file answers a lookup alone.
    let (summary, _) = indexed_traces(vec![otlp_req(0x11, 3, 1_000_000_000)]);
    assert_eq!(
        (
            summary.min_timestamp_s,
            summary.max_timestamp_s,
            summary.record_count
        ),
        (1, 1, 3)
    );

    let registries = make_registries();
    install_sealed(
        &registries,
        "default",
        1,
        vec![otlp_req(0x11, 3, 1_000_000_000)],
    )
    .await;
    let h = make_handler_over(registries);
    let v = serde_json::to_value(
        call_on(&h, json!({"trace": {"id": FIXTURE_TRACE_ID}}))
            .await
            .unwrap(),
    )
    .unwrap();
    assert_eq!(v["status"], json!({"complete": true}));
    assert_eq!(v["items"]["returned"], 3);
}

/// Absent assembly bounds capture full retention, and `coverage`
/// declares the range used (0..u32::MAX) — spans beyond it are
/// unknown, never silently dropped.
#[tokio::test]
async fn trace_coverage_declares_the_full_range_for_absent_bounds() {
    let h = handler_with_fixture_wal().await;
    let resp = call_on(&h, json!({"trace": {"id": FIXTURE_TRACE_ID}}))
        .await
        .unwrap();
    let v = serde_json::to_value(&resp).unwrap();
    assert_eq!(
        v["coverage"],
        json!({"after": 0, "before": 4_294_967_295_u32})
    );
}

/// Present bounds echo in `coverage` and change nothing else: the
/// fixture trace lies inside them, so the assembly is identical to the
/// unbounded fetch.
#[tokio::test]
async fn bounded_trace_fetch_echoes_coverage_and_assembles_identically() {
    let h = handler_with_fixture_wal().await;
    let unbounded = serde_json::to_value(
        call_on(&h, json!({"trace": {"id": FIXTURE_TRACE_ID}}))
            .await
            .unwrap(),
    )
    .unwrap();
    let bounded = serde_json::to_value(
        call_on(
            &h,
            json!({"trace": {"id": FIXTURE_TRACE_ID, "after": 0, "before": 100}}),
        )
        .await
        .unwrap(),
    )
    .unwrap();
    assert_eq!(bounded["coverage"], json!({"after": 0, "before": 100}));
    assert_eq!(bounded["spans"], unbounded["spans"]);
    assert_eq!(bounded["resources"], unbounded["resources"]);
    assert_eq!(bounded["status"], json!({"complete": true}));
}

#[tokio::test]
async fn bounded_trace_fetch_prunes_non_overlapping_sealed_files() {
    let registries = make_registries();
    install_wal(
        &registries,
        "default",
        1,
        vec![otlp_req(0x11, 3, 1_000_000_000)],
    )
    .await;
    // Tracked but never written: probing it fails a source, so partial
    // status is the observable for "this file was captured".
    install_sfst(&registries, "default", 2, 500_000, 500_100).await;
    let h = make_handler_over(registries);

    // Bounds not overlapping the sealed summary: pruned at capture,
    // never probed — the assembly stays complete.
    let outside = serde_json::to_value(
        call_on(
            &h,
            json!({"trace": {"id": FIXTURE_TRACE_ID, "after": 0, "before": 100}}),
        )
        .await
        .unwrap(),
    )
    .unwrap();
    assert_eq!(outside["status"], json!({"complete": true}));

    // Absent bounds capture the full range: the unreadable sealed file
    // is probed and surfaces as a source failure.
    let full = serde_json::to_value(
        call_on(&h, json!({"trace": {"id": FIXTURE_TRACE_ID}}))
            .await
            .unwrap(),
    )
    .unwrap();
    assert_eq!(full["status"], json!({"partial": ["source_failure"]}));
}

#[tokio::test]
async fn bounds_excluding_every_file_yield_a_complete_empty_trace() {
    let registries = make_registries();
    install_sfst(&registries, "default", 1, 1_000, 1_100).await;
    let h = make_handler_over(registries);
    let v = serde_json::to_value(
        call_on(
            &h,
            json!({"trace": {"id": FIXTURE_TRACE_ID, "after": 500_000, "before": 500_100}}),
        )
        .await
        .unwrap(),
    )
    .unwrap();
    assert_eq!(v["status"], json!({"complete": true}));
    assert_eq!(v["items"]["returned"], 0);
    assert_eq!(v["coverage"], json!({"after": 500_000, "before": 500_100}));
}

#[tokio::test]
async fn trace_bounds_of_any_width_are_accepted() {
    // No width cap: a range far wider than any UI formula is served and
    // declared as asked.
    let h = handler_with_fixture_wal().await;
    let v = serde_json::to_value(
        call_on(
            &h,
            json!({"trace": {"id": FIXTURE_TRACE_ID, "after": 1, "before": 4_000_000_000_u32}}),
        )
        .await
        .unwrap(),
    )
    .unwrap();
    assert_eq!(
        v["coverage"],
        json!({"after": 1, "before": 4_000_000_000_u32})
    );
    assert_eq!(v["status"], json!({"complete": true}));
    assert_eq!(v["items"]["returned"], 3);
}

#[tokio::test]
async fn trace_by_id_assembles_the_fixture_trace_end_to_end() {
    let h = handler_with_fixture_wal().await;
    let resp = call_on(&h, json!({"trace": {"id": FIXTURE_TRACE_ID}}))
        .await
        .unwrap();
    let v = serde_json::to_value(&resp).unwrap();

    assert_eq!(v["trace_id"], FIXTURE_TRACE_ID);
    assert_eq!(
        v["coverage"],
        json!({"after": 0, "before": 4_294_967_295_u32})
    );
    assert_eq!(v["status"], json!({"complete": true}));
    assert_eq!(v["items"]["returned"], 3);
    // Span 1 (unset parent) is the root; spans 2 and 3 parent to it.
    assert_eq!(v["summary_root"], 0);
    assert_eq!(v["roots"], json!([0]));
    assert_eq!(v["children"], json!([[1, 2], [], []]));
    let s0 = &v["spans"][0];
    assert_eq!(s0["span_id"], "01".repeat(8));
    assert!(s0.get("parent_span_id").is_none());
    assert_eq!(v["spans"][1]["parent_span_id"], "01".repeat(8));
    // Indexing put the OTLP name into the row facets.
    assert!(
        s0["fields"]
            .as_array()
            .unwrap()
            .iter()
            .any(|kv| kv == &json!(["name", "span-1"])),
        "fields carry the span name: {}",
        s0["fields"]
    );
    assert_eq!(attribute_keys_exposed(&v), kinds_keys(&v));
    assert!(
        v["field_kinds"].get("name").is_none(),
        "builtins carry no kinds"
    );
    assert!(
        v["spans"]
            .as_array()
            .unwrap()
            .iter()
            .all(|s| !pair_keys(&s["fields"]).iter().any(|k| k.starts_with('_'))),
        "no internal copy leaves the agent"
    );
}

/// The keys of a list of `[key, value]` pairs.
fn pair_keys(list: &serde_json::Value) -> Vec<String> {
    list.as_array()
        .unwrap()
        .iter()
        .map(|kv| kv[0].as_str().unwrap().to_string())
        .collect()
}

/// Every attribute key a trace answer's spans expose (fields, their
/// resources' attributes, event and link attributes), sorted and
/// distinct.
fn attribute_keys_exposed(v: &serde_json::Value) -> Vec<String> {
    let mut keys = Vec::new();
    for r in v["resources"].as_array().unwrap() {
        keys.extend(pair_keys(&r["attributes"]));
    }
    for s in v["spans"].as_array().unwrap() {
        keys.extend(pair_keys(&s["fields"]));
        for e in s["events"].as_array().unwrap() {
            keys.extend(pair_keys(&e["attributes"]));
        }
        for l in s["links"].as_array().unwrap() {
            keys.extend(pair_keys(&l["attributes"]));
        }
    }
    keys.retain(|k| {
        matches!(
            sfsq::traces::classify_storage(k),
            sfsq::traces::StorageName::Key(_, sfsq::traces::AttributeKey::Attribute(_))
        )
    });
    keys.sort();
    keys.dedup();
    keys
}

/// Span `i`'s full pair list: its resource's attributes, then its
/// fields.
fn expanded_pairs(v: &serde_json::Value, i: usize) -> Vec<(String, String)> {
    let s = &v["spans"][i];
    let mut list = Vec::new();
    if let Some(r) = s.get("resource") {
        list.extend(
            v["resources"][r.as_u64().unwrap() as usize]["attributes"]
                .as_array()
                .unwrap()
                .iter(),
        );
    }
    list.extend(s["fields"].as_array().unwrap().iter());
    list.into_iter()
        .map(|kv| {
            (
                kv[0].as_str().unwrap().into(),
                kv[1].as_str().unwrap().into(),
            )
        })
        .collect()
}

/// The keys of a trace answer's `field_kinds`, sorted.
fn kinds_keys(v: &serde_json::Value) -> Vec<String> {
    v["field_kinds"]
        .as_object()
        .unwrap_or_else(|| panic!("field_kinds is a map: {}", v["field_kinds"]))
        .keys()
        .cloned()
        .collect()
}

/// A span with every OTLP part the trace answer names: resource, scope
/// (name, version, attributes), span attributes, an error status with a
/// message, the trace state, an event and a link with attributes.
fn rich_request() -> opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest {
    use opentelemetry_proto::tonic::common::v1::{
        AnyValue, InstrumentationScope, KeyValue, any_value::Value as Av,
    };
    use opentelemetry_proto::tonic::trace::v1::{Status, span};
    let attr = |key: &str, value: &str| KeyValue {
        key: key.into(),
        value: Some(AnyValue {
            value: Some(Av::StringValue(value.into())),
        }),
        key_strindex: 0,
    };
    let mut req = otlp_req(0x11, 1, 1_000_000_000);
    let scope = &mut req.resource_spans[0].scope_spans[0];
    scope.scope = Some(InstrumentationScope {
        name: "lib".into(),
        version: "1.2".into(),
        attributes: vec![attr("tier", "web")],
        ..Default::default()
    });
    let s = &mut scope.spans[0];
    s.kind = 2;
    s.trace_state = "ot=th:8".into();
    s.status = Some(Status {
        code: 2,
        message: "boom".into(),
    });
    s.attributes = vec![attr("http.request.method", "GET")];
    s.events = vec![span::Event {
        time_unix_nano: s.start_time_unix_nano + 10,
        name: "exception".into(),
        attributes: vec![attr("exception.type", "E")],
        ..Default::default()
    }];
    s.links = vec![span::Link {
        trace_id: vec![0x22; 16],
        span_id: vec![0x33; 8],
        trace_state: "a=b".into(),
        attributes: vec![attr("peer", "p")],
        ..Default::default()
    }];
    req
}

/// One trace split across a sealed file and the live WAL tail: each
/// resource goes out once however its spans were stored, with its pairs
/// in stored order (several keys, an array, characters JSON escapes, a
/// value over the cap), every span names its own, and the spans that
/// carry the cut value are flagged.
#[tokio::test]
async fn a_resource_goes_out_once_across_a_sealed_file_and_the_tail() {
    use opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest;
    use opentelemetry_proto::tonic::common::v1::{
        AnyValue, ArrayValue, KeyValue, any_value::Value as Av,
    };
    use opentelemetry_proto::tonic::resource::v1::Resource;
    use opentelemetry_proto::tonic::trace::v1::{ResourceSpans, ScopeSpans, Span};
    let text = |v: &str| AnyValue {
        value: Some(Av::StringValue(v.into())),
    };
    let attr = |key: &str, value: AnyValue| KeyValue {
        key: key.into(),
        value: Some(value),
        key_strindex: 0,
    };
    let long = "x".repeat(crate::ledger::rpc::traces::adapter::TRACE_VALUE_MAX_BYTES + 10);
    let front = vec![
        attr("service.name", text("front")),
        attr("host.name", text("h\"1\\")),
        attr(
            "tags",
            AnyValue {
                value: Some(Av::ArrayValue(ArrayValue {
                    values: vec![text("a"), text("b")],
                })),
            },
        ),
        attr("note", text("tab\there\u{1}")),
        attr("blob", text(&long)),
    ];
    let back = vec![attr("service.name", text("back"))];
    // A resource and its spans as (span id, parent id); parent 0 = root.
    type Group<'a> = (&'a [KeyValue], &'a [(u8, u8)]);
    let req = |groups: &[Group]| ExportTraceServiceRequest {
        resource_spans: groups
            .iter()
            .map(|&(attributes, spans)| ResourceSpans {
                resource: Some(Resource {
                    attributes: attributes.to_vec(),
                    ..Default::default()
                }),
                scope_spans: vec![ScopeSpans {
                    spans: spans
                        .iter()
                        .map(|&(id, parent)| Span {
                            trace_id: vec![0x11; 16],
                            span_id: vec![id; 8],
                            parent_span_id: if parent == 0 {
                                Vec::new()
                            } else {
                                vec![parent; 8]
                            },
                            name: format!("span-{id}"),
                            start_time_unix_nano: 1_000_000_000 + u64::from(id) * 1_000,
                            end_time_unix_nano: 1_000_000_000 + u64::from(id) * 1_000 + 500,
                            ..Default::default()
                        })
                        .collect(),
                    ..Default::default()
                }],
                ..Default::default()
            })
            .collect(),
    };
    let registries = make_registries();
    install_sealed(
        &registries,
        "default",
        1,
        vec![req(&[(&front, &[(1, 0), (2, 1)]), (&back, &[(3, 1)])])],
    )
    .await;
    // Two spans: below the handler's chunk threshold, so a WAL tail.
    install_wal(
        &registries,
        "default",
        2,
        vec![req(&[(&front, &[(4, 1)]), (&back, &[(5, 3)])])],
    )
    .await;
    let h = make_handler_over(registries);
    let v = serde_json::to_value(
        call_on(&h, json!({"trace": {"id": FIXTURE_TRACE_ID}}))
            .await
            .unwrap(),
    )
    .unwrap();
    assert_eq!(v["status"], json!({"complete": true}));
    assert_eq!(v["items"]["returned"], 5);
    let ids: Vec<&str> = v["spans"]
        .as_array()
        .unwrap()
        .iter()
        .map(|s| s["span_id"].as_str().unwrap())
        .collect();
    let id = |b: u8| format!("{b:02x}").repeat(8);
    assert_eq!(ids, [id(1), id(2), id(3), id(4), id(5)]);
    let indices: Vec<&serde_json::Value> = v["spans"]
        .as_array()
        .unwrap()
        .iter()
        .map(|s| &s["resource"])
        .collect();
    assert_eq!(
        indices,
        [&json!(0), &json!(0), &json!(1), &json!(0), &json!(1)]
    );
    let cut = &long[..crate::ledger::rpc::traces::adapter::TRACE_VALUE_MAX_BYTES];
    assert_eq!(
        v["resources"],
        json!([
            {"attributes": [
                ["resource.attributes.service.name", "front"],
                ["resource.attributes.host.name", "h\"1\\"],
                ["resource.attributes.tags[]", "a"],
                ["resource.attributes.tags[]", "b"],
                ["resource.attributes.note", "tab\there\u{1}"],
                ["resource.attributes.blob", cut],
            ]},
            {"attributes": [["resource.attributes.service.name", "back"]]},
        ])
    );
    let flagged: Vec<bool> = v["spans"]
        .as_array()
        .unwrap()
        .iter()
        .map(|s| s.get("truncated") == Some(&json!(true)))
        .collect();
    assert_eq!(flagged, [true, true, false, true, false]);
    for s in v["spans"].as_array().unwrap() {
        assert!(
            !pair_keys(&s["fields"])
                .iter()
                .any(|k| k.starts_with("resource.")),
            "resource pairs stay out of fields: {}",
            s["fields"]
        );
    }
    assert_eq!(attribute_keys_exposed(&v), kinds_keys(&v));
}

/// The same span answers alike from a sealed file and from a live WAL:
/// every key is a search key, the status sits under `status.code` /
/// `status.message`, the resource goes out in the resource table, event
/// and link attributes carry their full keys, internal copies stay in,
/// and `field_kinds` covers exactly the attribute keys.
#[tokio::test]
async fn a_rich_span_answers_with_keys_sealed_and_tail() {
    let mut answers = Vec::new();
    for sealed in [true, false] {
        let registries = make_registries();
        if sealed {
            install_sealed(&registries, "default", 1, vec![rich_request()]).await;
        } else {
            install_wal(&registries, "default", 1, vec![rich_request()]).await;
        }
        let h = make_handler_over(registries);
        let v = serde_json::to_value(
            call_on(&h, json!({"trace": {"id": FIXTURE_TRACE_ID}}))
                .await
                .unwrap(),
        )
        .unwrap();
        assert_eq!(v["status"], json!({"complete": true}), "sealed={sealed}");
        let s = &v["spans"][0];
        assert_eq!(s["resource"], 0, "sealed={sealed}");
        assert_eq!(
            v["resources"],
            json!([{"attributes": [["resource.attributes.service.name", "svc"]]}]),
            "sealed={sealed}"
        );
        let mut fields = expanded_pairs(&v, 0);
        fields.sort();
        let want: Vec<(String, String)> = [
            ("attributes.http.request.method", "GET"),
            ("kind", "SERVER"),
            ("name", "span-1"),
            ("resource.attributes.service.name", "svc"),
            ("role", "ROOT"),
            ("scope.attributes.tier", "web"),
            ("scope.name", "lib"),
            ("scope.version", "1.2"),
            ("status.code", "ERROR"),
            ("status.message", "boom"),
            ("trace_state", "ot=th:8"),
        ]
        .map(|(k, v)| (k.to_string(), v.to_string()))
        .to_vec();
        assert_eq!(fields, want, "sealed={sealed}");
        assert!(s.get("kind").is_none(), "sealed={sealed}");
        assert_eq!(
            s["events"],
            json!([{
                "time_unix_nano": 1_000_001_010_u64,
                "name": "exception",
                "dropped_attributes_count": 0,
                "attributes": [["events.attributes.exception.type", "E"]],
            }]),
            "sealed={sealed}"
        );
        assert_eq!(
            s["links"][0]["attributes"],
            json!([["links.attributes.peer", "p"]]),
            "sealed={sealed}"
        );
        assert_eq!(
            kinds_keys(&v),
            [
                "attributes.http.request.method",
                "events.attributes.exception.type",
                "links.attributes.peer",
                "resource.attributes.service.name",
                "scope.attributes.tier",
            ],
            "sealed={sealed}"
        );
        assert_eq!(
            attribute_keys_exposed(&v),
            kinds_keys(&v),
            "sealed={sealed}"
        );
        // A key suggestion carries the same key string and, over the same
        // source, the same kinds.
        let suggested = serde_json::to_value(
            call_on(
                &h,
                json!({"suggest": {"after": 1, "before": 10, "max_items": 200}}),
            )
            .await
            .unwrap(),
        )
        .unwrap();
        for key in kinds_keys(&v) {
            let item = suggested["items"]
                .as_array()
                .unwrap()
                .iter()
                .find(|i| i["text"] == key.as_str())
                .unwrap_or_else(|| panic!("{key} is suggested: {suggested}"));
            assert_eq!(
                item["kinds"], v["field_kinds"][&key],
                "{key} sealed={sealed}"
            );
        }
        answers.push(v["field_kinds"].clone());
    }
    assert_eq!(answers[0], answers[1], "sealed and tail kinds agree");
}

#[tokio::test]
async fn absent_trace_id_is_a_complete_empty_trace() {
    // "Nothing stored under this id" is an answer, not an error.
    let h = handler_with_fixture_wal().await;
    let resp = call_on(
        &h,
        json!({"trace": {"id": "99999999999999999999999999999999"}}),
    )
    .await
    .unwrap();
    let v = serde_json::to_value(&resp).unwrap();
    assert_eq!(v["status"], json!({"complete": true}));
    assert_eq!(v["items"]["returned"], 0);
    assert_eq!(v["spans"], json!([]));
    assert_eq!(v["summary_root"], serde_json::Value::Null);
    assert_eq!(v["field_kinds"], empty_field_kinds());
}

/// The field-kind map of a trace answer that exposes no names.
fn empty_field_kinds() -> serde_json::Value {
    json!({})
}

#[tokio::test]
async fn semantically_invalid_trace_requests_are_clean_client_errors() {
    // SHAPE errors (null/array selectors, missing/unknown fields) fail
    // request deserialization and are pinned in the wire tests; what
    // stays here is the SEMANTIC validation the handler owns.
    for (body, needle) in [
        (json!({"trace": {"id": "xyz"}}), "32 hex"),
        // The unset sentinel: rejected pre-capture with the engine
        // check's exact wording.
        (
            json!({"trace": {"id": "00000000000000000000000000000000"}}),
            "all-zero",
        ),
        // Assembly bounds: both-or-neither, ordered.
        (
            json!({"trace": {"id": FIXTURE_TRACE_ID, "after": 100}}),
            "both 'after' and 'before'",
        ),
        (
            json!({"trace": {"id": FIXTURE_TRACE_ID, "before": 100}}),
            "both 'after' and 'before'",
        ),
        (
            json!({"trace": {"id": FIXTURE_TRACE_ID, "after": 200, "before": 100}}),
            "after 200 >= before 100",
        ),
        (
            json!({"trace": {"id": FIXTURE_TRACE_ID, "after": 100, "before": 100}}),
            "after 100 >= before 100",
        ),
    ] {
        let err = call(body.clone())
            .await
            .expect_err("must be a client error");
        let msg = err.to_string();
        assert!(msg.contains(needle), "for {body}: {msg}");
    }
}

// ── The trace list ─────────────────────────────────────────────────

/// Corpus base, unix seconds; the corpus queries below pin explicit
/// windows around it, so nothing depends on the wall clock.
const T_S: u32 = 1_700_000_000;

fn base_ns(offset_s: u64) -> u64 {
    (T_S as u64 + offset_s) * 1_000_000_000
}

/// Five traces: A/C/E on svc-a, B/D on svc-b; C and D share the same
/// start (a tie across a page boundary); E is newest.
async fn handler_with_search_corpus() -> OtelTracesHandler {
    let registries = make_registries();
    install_wal(
        &registries,
        "default",
        1,
        vec![
            otlp_req_svc(0x0A, 1, base_ns(10), "svc-a"),
            otlp_req_svc(0x0B, 2, base_ns(20), "svc-b"),
            otlp_req_svc(0x0C, 1, base_ns(30), "svc-a"),
            otlp_req_svc(0x0D, 1, base_ns(30), "svc-b"),
            otlp_req_svc(0x0E, 3, base_ns(40), "svc-a"),
        ],
    )
    .await;
    make_handler_over(registries)
}

/// The suite's standard explicit window: [T_S, T_S+100), 100 s wide.
fn window_body() -> serde_json::Value {
    json!({"after": T_S, "before": T_S + 100})
}

/// Wrap flat params as the wire's `{mode: params}` shape — tests build
/// params flat and name the mode at the call site.
fn as_mode(mode: &str, params: serde_json::Value) -> serde_json::Value {
    json!({ mode: params })
}

/// The list request (the selector-less Functions view) from flat
/// search-style params — `limit` is spelled `last` there — returning
/// the page (`data`).
async fn list(
    h: &OtelTracesHandler,
    mut params: serde_json::Value,
) -> netdata_plugin_error::Result<serde_json::Value> {
    let obj = params.as_object_mut().unwrap();
    if let Some(limit) = obj.remove("limit") {
        obj.insert("last".to_string(), limit);
    }
    let v = serde_json::to_value(call_on(h, params).await?).unwrap();
    Ok(v["data"].clone())
}

fn merge(base: &mut serde_json::Value, extra: serde_json::Value) {
    base.as_object_mut()
        .unwrap()
        .extend(extra.as_object().unwrap().clone());
}

/// Each returned trace's 2-char id prefix, in page order — the fixture
/// traces differ by their first id byte.
fn ids(v: &serde_json::Value) -> Vec<String> {
    v["traces"]
        .as_array()
        .unwrap()
        .iter()
        .map(|t| t["trace_id"].as_str().unwrap()[..2].to_string())
        .collect()
}

/// A WAL whose chunks will not index is refused for the request: every
/// mode's answer says a source failed, instead of looking complete.
#[tokio::test]
async fn a_refused_wal_is_a_failed_source_in_every_mode() {
    let registries = make_registries();
    let path = install_wal(
        &registries,
        "default",
        1,
        vec![
            otlp_req_svc(0x0A, 3, base_ns(10), "svc-a"),
            otlp_req_svc(0x0B, 3, base_ns(20), "svc-b"),
        ],
    )
    .await;
    let len = std::fs::metadata(&path).unwrap().len();
    {
        use std::io::{Seek, Write};
        let mut f = std::fs::OpenOptions::new().write(true).open(&path).unwrap();
        f.seek(std::io::SeekFrom::Start(wal::HEADER_SIZE as u64))
            .unwrap();
        f.write_all(&vec![0xFFu8; (len - wal::HEADER_SIZE as u64) as usize])
            .unwrap();
    }
    let h = make_handler_over(registries);
    let failed = json!({"partial": ["source_failure"]});

    let page = list(&h, window_body()).await.unwrap();
    assert_eq!(page["status"], failed, "list");
    assert_eq!(page["overview"]["status"], failed, "list aggregate");
    let suggest = serde_json::to_value(
        call_on(
            &h,
            as_mode("suggest", json!({"after": T_S, "before": T_S + 100})),
        )
        .await
        .unwrap(),
    )
    .unwrap();
    assert_eq!(suggest["status"], failed, "suggest");
    let trace = serde_json::to_value(
        call_on(&h, as_mode("trace", json!({"id": "0a".repeat(16)})))
            .await
            .unwrap(),
    )
    .unwrap();
    assert_eq!(trace["status"], failed, "trace");
    let explore = serde_json::to_value(
        call_on(
            &h,
            as_mode(
                "explore",
                json!({"after": T_S, "before": T_S + 100, "histogram": {}}),
            ),
        )
        .await
        .unwrap(),
    )
    .unwrap();
    assert_eq!(explore["status"], failed, "explore");
}

/// `explore` over the search corpus (five traces, eight spans, two
/// services, a start shared by two traces).
async fn explore_on(
    h: &OtelTracesHandler,
    params: serde_json::Value,
) -> netdata_plugin_error::Result<serde_json::Value> {
    let mut body = window_body();
    merge(&mut body, params);
    Ok(serde_json::to_value(call_on(h, as_mode("explore", body)).await?).unwrap())
}

#[tokio::test]
async fn explore_counts_and_lists_the_spans() {
    let h = handler_with_search_corpus().await;
    let v = explore_on(
        &h,
        json!({"histogram": {"stack": "resource.attributes.service.name"}, "rows": {"limit": 3}}),
    )
    .await
    .unwrap();
    assert_eq!(v["mode"], "explore");
    assert_eq!(v["status"], json!({"complete": true}));
    // The window is aligned outward to its grid's buckets.
    let window = &v["window"];
    let (after, before) = (
        window["after"].as_u64().unwrap(),
        window["before"].as_u64().unwrap(),
    );
    assert!(after <= u64::from(T_S) && before >= u64::from(T_S) + 100);
    assert_eq!(
        window["bucket_start_s"].as_u64().unwrap()
            + window["bucket_width_s"].as_u64().unwrap() * window["buckets"].as_u64().unwrap(),
        before
    );
    let histogram = &v["histogram"];
    assert_eq!(histogram["stack"], "resource.attributes.service.name");
    assert_eq!(histogram["dimensions"], json!(["svc-a", "svc-b"]));
    assert_eq!(histogram["total"], 8);
    let mut per_dimension = [0u64; 2];
    for bucket in histogram["buckets"].as_array().unwrap() {
        for (sum, count) in per_dimension
            .iter_mut()
            .zip(bucket["counts"].as_array().unwrap())
        {
            *sum += count.as_u64().unwrap();
        }
        assert_eq!(bucket["durations"].as_array().unwrap().len(), 32);
    }
    assert_eq!(per_dimension, [5, 3]);
    assert_eq!(histogram["duration_rows"].as_array().unwrap().len(), 32);
    assert!(histogram["percentiles"]["p50_ns"].is_i64());
    let rows = &v["rows"];
    assert_eq!(rows["matched"], 8);
    assert_eq!(rows["items"].as_array().unwrap().len(), 3);
    let first = &rows["items"][0];
    assert_eq!(
        first["fields"],
        json!([
            ["resource.attributes.service.name", "svc-a"],
            ["name", "span-3"],
            ["kind", "UNSPECIFIED"],
            ["status.code", "UNSET"]
        ])
    );
    assert!(rows["anchor"]["next"].is_string());
}

/// Walking the anchors lists every span once, newest first, and the
/// last page carries no anchor.
#[tokio::test]
async fn explore_rows_walk_by_anchor() {
    let h = handler_with_search_corpus().await;
    let mut seen: Vec<(i64, String, String)> = Vec::new();
    let mut anchor: Option<String> = None;
    loop {
        let mut params = json!({"rows": {"limit": 3, "columns": []}});
        if let Some(a) = &anchor {
            merge(&mut params, json!({"anchor": a}));
        }
        let v = explore_on(&h, params).await.unwrap();
        let rows = &v["rows"];
        for item in rows["items"].as_array().unwrap() {
            seen.push((
                item["start_ns"].as_i64().unwrap(),
                item["trace_id"].as_str().unwrap().to_string(),
                item["span_id"].as_str().unwrap().to_string(),
            ));
        }
        match rows["anchor"]["next"].as_str() {
            Some(next) => anchor = Some(next.to_string()),
            None => break,
        }
    }
    assert_eq!(seen.len(), 8);
    let mut sorted = seen.clone();
    sorted.sort_by(|a, b| b.cmp(a));
    assert_eq!(seen, sorted, "newest first by (start, trace id, span id)");
    sorted.dedup();
    assert_eq!(sorted.len(), 8, "no span listed twice");
}

/// A window whose span sits just under a bucket-width threshold widens
/// when aligned; every page of its walk still answers over the first
/// page's window and grid.
#[tokio::test]
async fn explore_pages_keep_the_first_page_window() {
    let h = handler_with_search_corpus().await;
    // 3590 s from an odd multiple of 30: 30 s buckets, aligned to 3600 s.
    let after = T_S / 60 * 60 - 30;
    let mut windows = Vec::new();
    let mut listed = 0;
    let mut anchor: Option<String> = None;
    loop {
        let mut params =
            json!({"after": after, "before": after + 3590, "rows": {"limit": 3, "columns": []}});
        if let Some(a) = &anchor {
            merge(&mut params, json!({"anchor": a}));
        }
        let v = explore_on(&h, params).await.unwrap();
        windows.push(v["window"].clone());
        listed += v["rows"]["items"].as_array().unwrap().len();
        match v["rows"]["anchor"]["next"].as_str() {
            Some(next) => anchor = Some(next.to_string()),
            None => break,
        }
    }
    assert_eq!(listed, 8);
    assert_eq!(windows[0]["bucket_width_s"], 30);
    assert!(windows.len() > 1);
    assert!(windows.iter().all(|w| *w == windows[0]), "{windows:?}");
}

#[tokio::test]
async fn explore_chips_select_spans() {
    let h = handler_with_search_corpus().await;
    let v = explore_on(
        &h,
        json!({
            "conditions": [{"key": "resource.attributes.service.name", "op": "=", "values": ["svc-b"]}],
            "histogram": {}
        }),
    )
    .await
    .unwrap();
    assert_eq!(v["histogram"]["total"], 3);
    assert_eq!(v["histogram"]["dimensions"], json!(["UNSET"]));
}

#[tokio::test]
async fn explore_refuses_what_has_no_span_meaning() {
    let h = handler_with_search_corpus().await;
    for (params, needle) in [
        (
            json!({"conditions": [{"key": "root_name", "op": "=", "values": ["x"]}]}),
            "describes a whole trace",
        ),
        (
            json!({"histogram": {"stack": "duration"}}),
            "no stored values",
        ),
        (json!({"histogram": {"stack": "status"}}), "is not a key"),
        (json!({"rows": {"limit": 0}}), "rows limit"),
        (
            json!({"rows": {"columns": ["trace_id"]}}),
            "no stored values",
        ),
        (json!({"anchor": "t2:1:2:3:4:5"}), "malformed anchor"),
        (json!({}), "asks for nothing"),
    ] {
        let err = explore_on(&h, params.clone())
            .await
            .expect_err("must refuse")
            .to_string();
        assert!(err.contains(needle), "for {params}: {err}");
    }
}

#[tokio::test]
async fn explore_beyond_its_pool_answers_busy() {
    let h = handler_with_search_corpus()
        .await
        .with_explore_concurrency(0);
    let v = explore_on(&h, json!({"histogram": {}, "rows": {}}))
        .await
        .unwrap();
    assert_eq!(v["status"], json!({"partial": ["busy"]}));
    assert!(v.get("histogram").is_none());
}

#[tokio::test]
async fn search_completion_captures_slack_files_and_skips_beyond_slack() {
    let registries = make_registries();
    install_wal(
        &registries,
        "default",
        1,
        vec![otlp_req(0x11, 3, u64::from(T_S) * 1_000_000_000)],
    )
    .await;
    // Tracked, never written: a probe is observable as source_failure.
    // Inside the slack band (30min past the window edge).
    install_sfst(&registries, "default", 2, T_S + 1_800, T_S + 1_900).await;
    let h = make_handler_over(registries);
    let v = list(&h, window_body()).await.unwrap();
    assert_eq!(v["status"], json!({"partial": ["source_failure"]}));

    let registries = make_registries();
    install_wal(
        &registries,
        "default",
        1,
        vec![otlp_req(0x11, 3, u64::from(T_S) * 1_000_000_000)],
    )
    .await;
    // Beyond the slack band (2h past a 100s window's 1h slack).
    install_sfst(&registries, "default", 2, T_S + 7_300, T_S + 7_400).await;
    let h = make_handler_over(registries);
    let v = list(&h, window_body()).await.unwrap();
    assert_eq!(v["status"], json!({"complete": true}));
}

#[tokio::test]
async fn search_returns_most_recent_first_with_deterministic_ties() {
    let h = handler_with_search_corpus().await;
    let v = list(&h, window_body()).await.unwrap();
    assert_eq!(v["status"], json!({"complete": true}));
    // Newest first; the C/D tie breaks by trace_id ascending.
    assert_eq!(ids(&v), vec!["0e", "0c", "0d", "0b", "0a"]);
    let e = &v["traces"][0];
    assert_eq!(e["root_service"], "svc-a");
    assert_eq!(e["root_name"], "span-1");
    assert_eq!(e["span_count"], 3);
    assert_eq!(
        e["service_breakdown"],
        json!({
            "top": [{"value": "svc-a", "spans": 3}],
            "other": 0,
            "unattributed": 0,
        })
    );
    assert_eq!(e["exact"], true);
}

#[tokio::test]
async fn chips_narrow_by_service_operation_and_attributes() {
    let h = handler_with_search_corpus().await;

    let mut body = window_body();
    body["conditions"] =
        json!([{"key": "resource.attributes.service.name", "op": "=", "values": ["svc-a"]}]);
    let v = list(&h, body).await.unwrap();
    assert_eq!(ids(&v), vec!["0e", "0c", "0a"]);

    // Multi-value OR within a chip.
    let mut body = window_body();
    body["conditions"] = json!([{"key": "resource.attributes.service.name", "op": "=", "values": ["svc-a", "svc-b"]}]);
    let v = list(&h, body).await.unwrap();
    assert_eq!(ids(&v).len(), 5);

    // Builtin key: only B and E have a second span.
    let mut body = window_body();
    body["conditions"] = json!([{"key": "name", "op": "=", "values": ["span-2"]}]);
    let v = list(&h, body).await.unwrap();
    assert_eq!(ids(&v), vec!["0e", "0b"]);

    // Chips AND at trace level.
    let mut body = window_body();
    body["conditions"] = json!([{"key": "name", "op": "=", "values": ["span-2"]}, {"key": "resource.attributes.service.name", "op": "=", "values": ["svc-b"]}]);
    let v = list(&h, body).await.unwrap();
    assert_eq!(ids(&v), vec!["0b"]);
}

/// The OTel defaults are selectable by name: `status: UNSET` matches
/// the spans that carry no status, `kind: UNSPECIFIED` the spans with
/// no kind — the proto default IS the stored value.
#[tokio::test]
async fn the_otel_defaults_are_selectable_in_the_functions_view() {
    use crate::ledger::rpc::traces::fixtures::otlp_req_err;
    let registries = make_registries();
    install_wal(
        &registries,
        "default",
        1,
        vec![
            otlp_req_err(0x0A, 1, base_ns(10), "svc"), // its only span is ERROR
            otlp_req_svc(0x0B, 2, base_ns(20), "svc"), // no status, no kind
        ],
    )
    .await;
    let h = make_handler_over(registries);

    let mut body = functions_body(20);
    body["conditions"] = json!([{"key": "status.code", "op": "=", "values": ["UNSET"]}]);
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert_eq!(ids(&v["data"]), ["0b"]);
    let o = &v["data"]["overview"];
    assert_eq!(o["totals"], json!({"traces": 1}));

    let mut body = functions_body(20);
    body["conditions"] = json!([{"key": "kind", "op": "=", "values": ["UNSPECIFIED"]}]);
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert_eq!(ids(&v["data"]), ["0b", "0a"]);
}

/// A kind or status code this build does not know is stored as its
/// decimal number, so it filters and suggests like any other label.
#[tokio::test]
async fn unknown_enum_values_filter_and_suggest_as_numbers() {
    let mut odd = otlp_req_svc(0x0A, 1, base_ns(10), "svc");
    let span = &mut odd.resource_spans[0].scope_spans[0].spans[0];
    span.kind = 9;
    span.status = Some(opentelemetry_proto::tonic::trace::v1::Status {
        code: 7,
        message: String::new(),
    });
    let registries = make_registries();
    install_wal(
        &registries,
        "default",
        1,
        vec![odd, otlp_req_svc(0x0B, 1, base_ns(20), "svc")],
    )
    .await;
    let h = make_handler_over(registries);

    for (key, value) in [("kind", "9"), ("status.code", "7")] {
        let mut body = functions_body(20);
        body["conditions"] = json!([{"key": key, "op": "=", "values": [value]}]);
        let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
        assert_eq!(ids(&v["data"]), ["0a"], "{key} = {value}");

        let mut body = window_body();
        merge(&mut body, json!({"key": key}));
        let v = serde_json::to_value(call_on(&h, as_mode("suggest", body)).await.unwrap()).unwrap();
        let values: Vec<&str> = v["items"]
            .as_array()
            .unwrap()
            .iter()
            .map(|i| i["text"].as_str().unwrap())
            .collect();
        assert!(
            values.contains(&value),
            "{key} suggests {value}: {values:?}"
        );
    }
}

#[tokio::test]
async fn pagination_walks_the_corpus_without_dups_or_gaps_across_the_tie() {
    let h = handler_with_search_corpus().await;

    // Page 1 (limit=2): E, C — the tail C ties with D.
    let mut body = window_body();
    body["limit"] = json!(2);
    let v1 = list(&h, body.clone()).await.unwrap();
    assert_eq!(ids(&v1), vec!["0e", "0c"]);
    let next1 = v1["anchor"]["next"].as_str().unwrap().to_string();

    // Page 2: the tie partner D, then B.
    body["anchor"] = json!(next1);
    let v2 = list(&h, body.clone()).await.unwrap();
    assert_eq!(ids(&v2), vec!["0d", "0b"]);
    let next2 = v2["anchor"]["next"].as_str().unwrap().to_string();

    // Page 3: A alone; short page, walk over.
    body["anchor"] = json!(next2);
    let v3 = list(&h, body.clone()).await.unwrap();
    assert_eq!(ids(&v3), vec!["0a"]);
    assert!(v3.get("anchor").is_none());
}

#[tokio::test]
async fn invalid_list_requests_are_clean_client_errors() {
    let h = handler_with_search_corpus().await;
    for (body, needle) in [
        (json!({"limit": 0}), "zero 'last'"),
        (
            json!({"limit": super::super::wire::SEARCH_LIMIT_MAX + 1}),
            "exceeds the maximum",
        ),
        (json!({"anchor": "junk"}), "malformed anchor"),
        (json!({"after": 500, "before": 400}), "invalid window"),
    ] {
        let err = list(&h, body.clone())
            .await
            .expect_err("must be a client error");
        let msg = err.to_string();
        assert!(msg.contains(needle), "for {body}: {msg}");
    }
    // The boundary itself is legal (locks the cap against an accidental
    // off-by-one `<`).
    list(&h, json!({"limit": super::super::wire::SEARCH_LIMIT_MAX}))
        .await
        .expect("last == max succeeds");
}

#[tokio::test]
async fn pagination_survives_a_trace_whose_envelope_predates_its_rank() {
    // F's spans start at T+15s and T+45s: envelope 15, rank 45 (its
    // newest span). With an envelope-anchored cursor, page 2's window
    // would end at 15s+1 and U (rank 30s) would be gapped out — the
    // live-demo failure this pins.
    use crate::ledger::rpc::traces::fixtures::otlp_req_at;
    let registries = make_registries();
    install_wal(
        &registries,
        "default",
        1,
        vec![
            otlp_req_at(0x0F, &[base_ns(15), base_ns(45)], "svc-f"),
            otlp_req_at(0x1A, &[base_ns(30)], "svc-u"),
            otlp_req_at(0x1B, &[base_ns(50)], "svc-v"),
        ],
    )
    .await;
    let h = make_handler_over(registries);

    let mut body = window_body();
    body["limit"] = json!(2);
    let v1 = list(&h, body.clone()).await.unwrap();
    assert_eq!(ids(&v1), vec!["1b", "0f"], "V (50s) then F (rank 45s)");
    body["anchor"] = v1["anchor"]["next"].clone();
    let v2 = list(&h, body).await.unwrap();
    assert_eq!(ids(&v2), vec!["1a"], "U (30s) must not be gapped out");
    assert!(v2.get("anchor").is_none());
}

#[tokio::test]
async fn straddling_trace_above_the_tail_never_reappears() {
    // A straddling trace must never reappear: F's matched spans (10s,
    // 70s) straddle page 1's tail rank (U at 50s), and a design that
    // narrows the window per page re-admits F at rank 10s — a
    // duplicate. The frozen-window after-key walk keeps F at rank 70s,
    // above the key, already-served, out of every later page.
    use crate::ledger::rpc::traces::fixtures::otlp_req_at;
    let registries = make_registries();
    install_wal(
        &registries,
        "default",
        1,
        vec![
            otlp_req_at(0x0F, &[base_ns(10), base_ns(70)], "svc-f"),
            otlp_req_at(0x1A, &[base_ns(50)], "svc-u"),
            otlp_req_at(0x1B, &[base_ns(30)], "svc-w"),
        ],
    )
    .await;
    let h = make_handler_over(registries);

    let mut body = window_body();
    body["limit"] = json!(2);
    let v1 = list(&h, body.clone()).await.unwrap();
    assert_eq!(ids(&v1), vec!["0f", "1a"], "F (rank 70s) then U (50s)");
    body["anchor"] = v1["anchor"]["next"].clone();
    let v2 = list(&h, body.clone()).await.unwrap();
    assert_eq!(
        ids(&v2),
        vec!["1b"],
        "only W is fresh — F must not duplicate"
    );
    assert!(v2.get("anchor").is_none());
}

#[tokio::test]
async fn anchor_pages_ignore_the_requests_own_window() {
    // The cursor freezes page 1's window; a page-2 request carrying a
    // different (or defaulted) window must not shift the walk.
    let h = handler_with_search_corpus().await;
    let mut body = window_body();
    body["limit"] = json!(2);
    let v1 = list(&h, body).await.unwrap();
    let next = v1["anchor"]["next"].clone();

    // Page 2 with a WRONG window — the frozen one must win.
    let v2 = list(
        &h,
        json!({"after": 1, "before": 2, "limit": 2, "anchor": next}),
    )
    .await
    .unwrap();
    assert_eq!(ids(&v2), vec!["0d", "0b"]);
}

#[tokio::test]
async fn late_arrivals_above_the_key_shorten_pages_but_never_duplicate() {
    // The documented live-data caveat: traces arriving INTO the frozen
    // window ABOVE the after-key consume the over-fetch allowance —
    // pages can come up short (here: early walk end) — but nothing
    // duplicates and nothing already-served reappears.
    let registries = make_registries();
    install_wal(
        &registries,
        "default",
        1,
        vec![
            otlp_req_svc(0x0A, 1, base_ns(10), "svc"),
            otlp_req_svc(0x0B, 1, base_ns(20), "svc"),
            otlp_req_svc(0x0C, 1, base_ns(30), "svc"),
            otlp_req_svc(0x0D, 1, base_ns(40), "svc"),
        ],
    )
    .await;
    let h = make_handler_over(registries.clone());

    let mut body = window_body();
    body["limit"] = json!(2);
    let v1 = list(&h, body.clone()).await.unwrap();
    assert_eq!(ids(&v1), vec!["0d", "0c"]);

    // A burst of three late arrivals, all ranked above the key (0c@30).
    install_wal(
        &registries,
        "default",
        2,
        vec![
            otlp_req_svc(0x21, 1, base_ns(70), "svc"),
            otlp_req_svc(0x22, 1, base_ns(80), "svc"),
            otlp_req_svc(0x23, 1, base_ns(90), "svc"),
        ],
    )
    .await;

    // Page 2's over-fetch (2+2=4) is dominated by the burst + served:
    // engine top-4 = [23, 22, 21, 0d]; everything at-or-above the key
    // drops → a short (here empty) page, no duplicates, walk ends.
    body["anchor"] = v1["anchor"]["next"].clone();
    let v2 = list(&h, body).await.unwrap();
    let page2 = ids(&v2);
    assert!(
        !page2.contains(&"0d".to_string()) && !page2.contains(&"0c".to_string()),
        "served traces never reappear: {page2:?}"
    );
    assert!(
        page2.len() < 2,
        "the burst consumed the allowance — a short page: {page2:?}"
    );
}

// ── Suggestion admission ───────────────────────────────────────────

#[tokio::test]
async fn a_finished_suggestion_releases_its_permit() {
    let h = handler_with_search_corpus()
        .await
        .with_suggest_concurrency(1);
    let mut body = window_body();
    merge(
        &mut body,
        json!({"key": "resource.attributes.service.name"}),
    );
    for _ in 0..2 {
        let v = serde_json::to_value(call_on(&h, as_mode("suggest", body.clone())).await.unwrap())
            .unwrap();
        assert_ne!(v["status"], json!({"partial": ["busy"]}), "{v}");
    }
}

// ── The heatmap (the list's embedded window aggregate) ───────────────

/// The list request's embedded aggregate for the corpus window.
async fn heatmap_of(h: &OtelTracesHandler) -> serde_json::Value {
    let v = serde_json::to_value(call_on(h, functions_body(20)).await.unwrap()).unwrap();
    v["data"]["overview"].clone()
}

#[tokio::test]
async fn the_heatmap_matches_the_corpus_distribution() {
    // 100s window → 1s buckets aligned to [T_S, T_S+100). The corpus's
    // 5 TRACES (A=1 span, B=2, C=1, D=1, E=3 — 8 stored spans), every
    // envelope sub-1ms (bin 0), binned at each trace's start second.
    let h = handler_with_search_corpus().await;
    let v = heatmap_of(&h).await;
    assert_eq!(v["unit"], "traces");
    assert_eq!(v["status"], json!({"complete": true}));
    assert_eq!(v["totals"], json!({"traces": 5}));
    assert_eq!(v["grid"]["bucket_start_s"], T_S);
    assert_eq!(v["grid"]["bucket_width_s"], 1);
    assert_eq!(
        v["grid"]["duration_bins"],
        json!(["<1ms", "1-10ms", "10-100ms", "100ms-1s", "1-10s", ">10s"])
    );
    let cells = v["grid"]["cells"].as_array().unwrap();
    assert_eq!(cells.len(), 100);
    let sum: u64 = cells
        .iter()
        .flat_map(|r| r.as_array().unwrap())
        .map(|c| c.as_u64().unwrap())
        .sum();
    assert_eq!(sum, 5, "cell sums equal totals.traces");
    // A starts at second 10, B at 20; the C/D pair share second 30
    // (two traces, one cell); E is ONE trace at second 40.
    assert_eq!(cells[10][0], 1);
    assert_eq!(cells[20][0], 1);
    assert_eq!(cells[30][0], 2);
    assert_eq!(cells[40][0], 1);
}

#[tokio::test]
async fn the_grid_carries_error_spans_per_bucket() {
    use crate::ledger::rpc::traces::fixtures::otlp_req_err;
    let registries = make_registries();
    install_wal(
        &registries,
        "default",
        1,
        vec![
            otlp_req_svc(0x0A, 2, base_ns(10), "svc"),
            otlp_req_err(0x0B, 2, base_ns(20), "svc"), // span 1 = ERROR
            otlp_req_err(0x0C, 2, base_ns(20), "svc"), // a second failing trace, same second
        ],
    )
    .await;
    let h = make_handler_over(registries);
    let v = heatmap_of(&h).await;
    assert_eq!(v["totals"], json!({"traces": 3}));
    let cells = v["grid"]["cells"].as_array().unwrap();
    let errors = v["grid"]["error_spans"].as_array().unwrap();
    assert_eq!(errors.len(), cells.len(), "index-parallel to the buckets");
    // The failures count in the second the traces started in.
    assert_eq!(errors[10], 0, "the healthy trace's bucket");
    assert_eq!(errors[20], 2, "both failing traces");
    let sum: u64 = errors.iter().map(|e| e.as_u64().unwrap()).sum();
    assert_eq!(sum, 2, "one failing span in each failing trace");
}

/// Every bucket of this corpus holds one distinct duration, so the
/// histogram answers it exactly.
#[tokio::test]
async fn the_grid_carries_per_bucket_duration_percentiles() {
    let h = handler_with_search_corpus().await;
    let v = heatmap_of(&h).await;
    let p = &v["grid"]["duration_percentiles_ns"];
    for rank in ["p50", "p95", "p99"] {
        let a = p[rank].as_array().unwrap_or_else(|| panic!("{rank} array"));
        assert_eq!(a.len(), 100, "{rank} is index-parallel to the cell rows");
    }
    // The corpus's envelopes: A 500ns at second 10, B 1500ns at 20, the
    // C/D pair 500ns each at 30, E 2500ns at 40.
    assert_eq!(p["p50"][10], 500);
    assert_eq!(p["p50"][20], 1500);
    assert_eq!(p["p50"][30], 500);
    assert_eq!(p["p99"][30], 500);
    assert_eq!(p["p50"][40], 2500);
    assert_eq!(p["p99"][40], 2500);
    // A bucket that binned nothing is null, never 0 — 0 would render as
    // an instantaneous trace.
    for rank in ["p50", "p95", "p99"] {
        assert!(p[rank][0].is_null(), "{rank} of an empty bucket");
    }
}

/// The explorer's 32 rows, each trace counted once in the row its
/// envelope falls in — the same scale the span heatmap draws.
#[tokio::test]
async fn the_grid_carries_traces_per_heatmap_row() {
    let h = handler_with_search_corpus().await;
    let v = heatmap_of(&h).await;
    let rows = v["grid"]["duration_rows"].as_array().unwrap();
    assert_eq!(rows.len(), 32);
    assert_eq!(rows[0], 1024);
    assert!(rows[31].is_null(), "the last row is unbounded");
    let durations = v["grid"]["durations"].as_array().unwrap();
    assert_eq!(durations.len(), 100, "one per time bucket");
    let sum: u64 = durations
        .iter()
        .flat_map(|r| r.as_array().unwrap())
        .map(|c| c.as_u64().unwrap())
        .sum();
    assert_eq!(sum, 5, "row sums equal totals.traces");
    // Envelopes: A 500ns at second 10 (< 1024), B 1500ns at 20
    // ([1024, 2048)), the C/D pair 500ns at 30, E 2500ns at 40
    // ([2048, 4096)).
    assert_eq!(durations[10][0], 1);
    assert_eq!(durations[20][1], 1);
    assert_eq!(durations[30][0], 2);
    assert_eq!(durations[40][2], 1);
}

// ── The Functions view's window aggregate ────────────────────────────

/// Run `body` with a progress state the test keeps; returns `(done, total)`.
async fn progress_of(h: &OtelTracesHandler, body: serde_json::Value) -> (usize, usize) {
    let progress = ProgressState::new();
    let ctx = FunctionCallContext::new(
        "tx-test".to_string(),
        progress.clone(),
        CancellationToken::new(),
    );
    let req: OtelTracesRequest = serde_json::from_value(body.clone()).unwrap();
    h.on_call(ctx, req)
        .await
        .unwrap_or_else(|e| panic!("{body}: {e}"));
    progress.load()
}

#[tokio::test]
async fn every_mode_sets_its_progress_total_and_completes_it() {
    // The corpus WAL resolves to two chunks at min_entries 4; every pass
    // ticks once per source it walks. The Functions view walks the
    // completion range for its page AND the grid's window for its
    // aggregate.
    let h = handler_with_search_corpus().await;
    let windowed = |mode: &str, extra: serde_json::Value| {
        let mut body = window_body();
        merge(&mut body, extra);
        as_mode(mode, body)
    };
    for (body, expected) in [
        (
            json!({"trace": {"id": "0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a"}}),
            (2, 2),
        ),
        (functions_body(2), (4, 4)),
        (windowed("suggest", json!({"key": "name"})), (2, 2)),
        (windowed("explore", json!({"histogram": {}})), (2, 2)),
    ] {
        assert_eq!(progress_of(&h, body.clone()).await, expected, "{body}");
    }
}

/// The Functions request for the corpus window: the protocol's own
/// top-level fields, no mode selector.
fn functions_body(last: usize) -> serde_json::Value {
    json!({"after": T_S, "before": T_S + 100, "last": last})
}

#[tokio::test]
async fn the_functions_view_aggregates_the_whole_window_beside_the_page() {
    let h = handler_with_search_corpus().await;
    let v = serde_json::to_value(call_on(&h, functions_body(2)).await.unwrap()).unwrap();
    // The page is 2 of the window's 5 traces...
    assert_eq!(
        v["data"]["items"],
        json!({"returned": 2, "max_to_return": 2})
    );
    assert_eq!(ids(&v["data"]), ["0e", "0c"]);
    // ...and the aggregate describes the WINDOW, not the page.
    let o = &v["data"]["overview"];
    assert_eq!(o["version"], 1);
    assert_eq!(o["unit"], "traces");
    assert_eq!(o["status"], json!({"complete": true}));
    assert_eq!(o["totals"], json!({"traces": 5}));
    assert_eq!(o["grid"]["bucket_start_s"], T_S);
    assert_eq!(o["grid"]["bucket_width_s"], 1);
    assert_eq!(
        o["grid"]["duration_bins"],
        json!(["<1ms", "1-10ms", "10-100ms", "100ms-1s", "1-10s", ">10s"])
    );
    let cells = o["grid"]["cells"].as_array().unwrap();
    assert_eq!(cells.len(), 100);
    let sum: u64 = cells
        .iter()
        .flat_map(|r| r.as_array().unwrap())
        .map(|c| c.as_u64().unwrap())
        .sum();
    assert_eq!(sum, 5, "cell sums equal totals.traces");
}

#[tokio::test]
async fn the_aggregates_coverage_is_the_grids_aligned_window() {
    let h = handler_with_search_corpus().await;
    // An 886s window: the grid picks 10s buckets and snaps outward to
    // wall-clock multiples, and reports that aligned window.
    let body = json!({"after": T_S + 7, "before": T_S + 893, "last": 20});
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    let o = &v["data"]["overview"];
    assert_eq!(o["coverage"], json!({"after": T_S, "before": T_S + 900}));
    assert_eq!(o["grid"]["bucket_start_s"], T_S);
    assert_eq!(o["grid"]["bucket_width_s"], 10);
    assert_eq!(o["grid"]["cells"].as_array().unwrap().len(), 90);
    assert_eq!(o["totals"]["traces"], 5);
}

#[tokio::test]
async fn the_aggregate_bins_only_the_selected_traces() {
    let h = handler_with_search_corpus().await;

    // A chip active: the aggregate follows it (the svc-a traces A, C,
    // E).
    let mut body = functions_body(20);
    body["conditions"] =
        json!([{"key": "resource.attributes.service.name", "op": "=", "values": ["svc-a"]}]);
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert_eq!(ids(&v["data"]), ["0e", "0c", "0a"]);
    let o = &v["data"]["overview"];
    assert_eq!(o["totals"], json!({"traces": 3}));
    let cells = o["grid"]["cells"].as_array().unwrap();
    let sum: u64 = cells
        .iter()
        .flat_map(|r| r.as_array().unwrap())
        .map(|c| c.as_u64().unwrap())
        .sum();
    assert_eq!(sum, 3, "the grid bins the selected traces only");
    // A at second 10, C at 30 (D, svc-b, no longer shares the cell), E at 40.
    assert_eq!(cells[10][0], 1);
    assert_eq!(cells[20][0], 0);
    assert_eq!(cells[30][0], 1);
    assert_eq!(cells[40][0], 1);
}

/// `trace_start` keeps the traces whose earliest span starts in its
/// range — a heatmap cell's own bucket — whatever their in-window spans.
#[tokio::test]
async fn trace_start_bounds_the_list_by_each_trace_start() {
    let h = handler_with_search_corpus().await;
    let listed = |op: &str, second: u64| {
        let mut body = functions_body(20);
        body["conditions"] = json!([{"key": "trace_start", "op": op, "values": [base_ns(second)]}]);
        let h = &h;
        async move { ids(&serde_json::to_value(call_on(h, body).await.unwrap()).unwrap()["data"]) }
    };
    assert_eq!(listed(">=", 20).await, ["0e", "0c", "0d", "0b"]);
    assert_eq!(listed("<", 30).await, ["0b", "0a"]);
}

/// `skip_overview` keeps the page and drops the section, for a heatmap
/// cell's list beside a section the caller already holds.
#[tokio::test]
async fn skip_overview_drops_the_aggregate() {
    let h = handler_with_search_corpus().await;
    let mut body = functions_body(20);
    body["skip_overview"] = json!(true);
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert_eq!(ids(&v["data"]).len(), 5);
    assert!(v["data"].get("overview").is_none(), "{}", v["data"]);

    let mut body = functions_body(20);
    body["skip_overview"] = json!(false);
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert_eq!(v["data"]["overview"]["totals"], json!({"traces": 5}));
}

#[tokio::test]
async fn the_aggregate_follows_the_chips_but_never_the_duration_ones() {
    let h = handler_with_search_corpus().await;
    // A service chip AND a duration chip: the list honours both (E is
    // the only svc-a trace with a ≥1µs envelope), the grid honours the
    // service chip alone — duration is its own axis.
    let mut body = functions_body(20);
    body["conditions"] = json!([
        {"key": "resource.attributes.service.name", "op": "=", "values": ["svc-a"]},
        {"key": "trace_duration", "op": ">=", "values": [1_000]}
    ]);
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert_eq!(ids(&v["data"]), ["0e"]);
    let o = &v["data"]["overview"];
    assert_eq!(o["totals"], json!({"traces": 3}));

    // Values OR within a chip: both services select everything, and the
    // grid equals the unfiltered one.
    let mut body = functions_body(20);
    body["conditions"] = json!([{"key": "resource.attributes.service.name", "op": "=", "values": ["svc-a", "svc-b"]}]);
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    let o = &v["data"]["overview"];
    assert_eq!(o["totals"], json!({"traces": 5}));
}

/// Every trace-level word the engine owns (`trace_level_target`) but
/// `trace_duration` (the grid's own axis), as the wire spells it, with
/// a value an `=` chip can apply and the ids the list then shows
/// (newest first).
const TRACE_LEVEL_CHIPS: [(&str, &str, &[&str]); 3] = [
    ("root_service_name", "svc-a", &["0e", "0c", "0a"]),
    ("root_name", "span-1", &["0e", "0c", "0d", "0b", "0a"]),
    ("trace_id", "0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a", &["0a"]),
];

#[tokio::test]
async fn trace_level_chips_filter_the_grid_too() {
    let h = handler_with_search_corpus().await;
    // Root words read the roots the sources recorded and `trace_id`
    // pins: the grid bins the traces the list shows.
    for (word, value, listed) in TRACE_LEVEL_CHIPS {
        let mut body = functions_body(20);
        body["conditions"] = json!([{"key": word, "op": "=", "values": [value]}]);
        let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
        assert_eq!(ids(&v["data"]), listed, "{word}: the list applies the word");
        let o = &v["data"]["overview"];
        assert_eq!(o["totals"]["traces"], listed.len(), "{word}");
    }

    // Mixed with a span-level chip, both apply to the grid as to the list.
    let mut body = functions_body(20);
    body["conditions"] = json!([{"key": "root_service_name", "op": "=", "values": ["svc-a"]}, {"key": "resource.attributes.service.name", "op": "=", "values": ["svc-b"]}]);
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert!(ids(&v["data"]).is_empty(), "no trace is on both services");
    assert_eq!(v["data"]["overview"]["totals"]["traces"], 0);
}

#[tokio::test]
async fn duration_chips_narrow_the_list_but_never_the_aggregate() {
    let h = handler_with_search_corpus().await;
    let bound = |op: &str, ns: u64| json!({"key": "trace_duration", "op": op, "values": [ns]});

    // A lower bound narrows the page to the window's two long traces
    // (envelopes 2500ns and 1500ns) while the aggregate keeps counting
    // all five — duration chips never reach the grid.
    let mut body = functions_body(20);
    body["conditions"] = json!([bound(">=", 1_000)]);
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert_eq!(ids(&v["data"]), ["0e", "0b"]);
    let o = &v["data"]["overview"];
    assert_eq!(o["totals"], json!({"traces": 5}));

    // The upper bound behaves identically.
    let mut body = functions_body(20);
    body["conditions"] = json!([bound("<=", 1_000)]);
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert_eq!(ids(&v["data"]), ["0c", "0d", "0a"]);
    let o = &v["data"]["overview"];
    assert_eq!(o["totals"], json!({"traces": 5}));

    // A band, the shape of a heatmap cell click (`>=` floor, `<`
    // ceiling): only B's 1500ns envelope is inside.
    let mut body = functions_body(20);
    body["conditions"] = json!([bound(">=", 1_000), bound("<", 2_000)]);
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert_eq!(ids(&v["data"]), ["0b"]);
    let o = &v["data"]["overview"];
    assert_eq!(o["totals"], json!({"traces": 5}));
}

#[tokio::test]
async fn the_aggregate_captures_the_aligned_window_and_carries_its_own_status() {
    let registries = make_registries();
    install_wal(
        &registries,
        "default",
        1,
        vec![otlp_req(0x11, 3, u64::from(T_S) * 1_000_000_000)],
    )
    .await;
    // Tracked, never written: a probe is observable as source_failure.
    // 30 minutes past the window edge — inside the page's completion
    // slack (1h per side), outside the grid's aligned window.
    install_sfst(&registries, "default", 2, T_S + 1_800, T_S + 1_900).await;
    let h = make_handler_over(registries);
    let v = serde_json::to_value(call_on(&h, functions_body(20)).await.unwrap()).unwrap();
    // The page probed the slack file and says so...
    assert_eq!(v["data"]["status"], json!({"partial": ["source_failure"]}));
    // ...the aggregate never saw it: its capture is the grid's window,
    // so its status is its own.
    assert_eq!(v["data"]["overview"]["status"], json!({"complete": true}));
    assert_eq!(v["data"]["overview"]["totals"], json!({"traces": 1}));
}

#[tokio::test]
async fn an_anchor_page_carries_no_aggregate_and_walks_the_corpus_unchanged() {
    let h = handler_with_search_corpus().await;

    // Page 1 carries no cursor, so the aggregate is composed as ever.
    let v1 = serde_json::to_value(call_on(&h, functions_body(2)).await.unwrap()).unwrap();
    assert_eq!(ids(&v1["data"]), ["0e", "0c"]);
    assert_eq!(v1["data"]["overview"]["totals"], json!({"traces": 5}));
    let next1 = v1["data"]["anchor"]["next"].clone();

    // Page 2: the tie partner D, then B, and its own cursor. Asserted
    // BEFORE the section, so the red run proves the page is what it
    // always was and only the aggregate changed.
    let mut body = functions_body(2);
    body["anchor"] = next1.clone();
    let v2 = serde_json::to_value(call_on(&h, body.clone()).await.unwrap()).unwrap();
    assert_eq!(ids(&v2["data"]), ["0d", "0b"]);
    assert_eq!(
        v2["data"]["items"],
        json!({"returned": 2, "max_to_return": 2})
    );
    let next2 = v2["data"]["anchor"]["next"].clone();
    assert!(
        next2.is_string(),
        "a full anchor page still hands out its own cursor"
    );

    // The same cursor through the legacy mode — which never composed an
    // aggregate — yields the identical page: proof the omission
    // perturbs nothing the client reads off it.
    let mut legacy = window_body();
    legacy["limit"] = json!(2);
    legacy["anchor"] = next1;
    let l2 = list(&h, legacy).await.unwrap();
    assert_eq!(l2["traces"], v2["data"]["traces"]);
    assert_eq!(l2["items"], v2["data"]["items"]);
    assert_eq!(l2["anchor"], v2["data"]["anchor"]);

    // An anchor page reruns the cursor's FROZEN window, so its
    // aggregate would be a byte-for-byte repeat of the one the client
    // already holds: the second engine pass is skipped and the key is
    // ABSENT, not null — `get` returning None distinguishes the two.
    assert!(
        v2["data"].get("overview").is_none(),
        "an anchor page repeats the first page's window: no second pass"
    );

    // Page 3: A alone, short, so no cursor — and still no aggregate.
    body["anchor"] = next2;
    let v3 = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert_eq!(ids(&v3["data"]), ["0a"]);
    assert!(v3["data"].get("anchor").is_none());
    assert!(v3["data"].get("overview").is_none());
}

#[tokio::test]
async fn tenant_scoping_isolates_and_defaults() {
    // The tenant selector scopes every data mode: another tenant's data
    // is invisible, an unknown tenant is empty (never an all-tenant
    // union), and the omitted selector reads the default tenant.
    let registries = make_registries();
    install_wal(
        &registries,
        "tenant-a",
        1,
        vec![otlp_req_svc(0x0A, 1, base_ns(10), "svc")],
    )
    .await;
    let h = make_handler_over(registries);

    let body = window_body();
    // Default tenant: tenant-a's data is invisible.
    let v = list(&h, body.clone()).await.unwrap();
    assert_eq!(v["items"]["returned"], 0);
    // The owning tenant sees it — `tenant` rides at the TOP level.
    let mut wrapped = body.clone();
    wrapped["tenant"] = json!("tenant-a");
    let v = list(&h, wrapped).await.unwrap();
    assert_eq!(v["items"]["returned"], 1);
    // An unknown tenant is empty, not an error and not a union.
    let mut wrapped = body;
    wrapped["tenant"] = json!("nope");
    let v = list(&h, wrapped).await.unwrap();
    assert_eq!(v["items"]["returned"], 0);

    // Tenant routes identically through every data mode (the
    // TenantId::resolve_query sites): trace, the Functions view, and one
    // enumeration path each see tenant-a's data only when scoped.
    let mut values_body = window_body();
    merge(
        &mut values_body,
        json!({"key": "resource.attributes.service.name"}),
    );
    for mut wrapped in [
        json!({"trace": {"id": "0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a"}}),
        functions_body(20),
        as_mode("suggest", values_body),
    ] {
        let unscoped = serde_json::to_value(call_on(&h, wrapped.clone()).await.unwrap()).unwrap();
        wrapped["tenant"] = json!("tenant-a");
        let scoped = serde_json::to_value(call_on(&h, wrapped.clone()).await.unwrap()).unwrap();
        let count = |v: &serde_json::Value| -> usize {
            if let Some(items) = v.get("items").and_then(|i| i.get("returned")) {
                items.as_u64().unwrap() as usize
            } else if let Some(items) = v.get("items").and_then(|i| i.as_array()) {
                items.len()
            } else {
                v["data"]["items"]["returned"].as_u64().unwrap() as usize
            }
        };
        assert_eq!(count(&unscoped), 0, "unscoped sees nothing: {wrapped}");
        assert!(count(&scoped) > 0, "tenant-a sees its data: {wrapped}");
    }
}

// ── The transport status boundary ───────────────────────────────────
//
// Shape errors fail request DESERIALIZATION in the bridge; the
// handler's own validation rejects the rest. Both answer 400 with the
// published `error_response` body, so the message reaches the UI as
// `errorMessage`. These cross `HandlerAdapter::handle_raw` — the same
// path the live bridge runs.

/// The `errorMessage` of a 400 body, which must be exactly
/// `{"status": 400, "errorMessage": …}`.
fn bad_request_message(body: &str) -> String {
    let v: serde_json::Value = serde_json::from_str(body).expect("a JSON body");
    assert_eq!(v["status"], 400, "{body}");
    assert_eq!(v.as_object().unwrap().len(), 2, "{body}");
    v["errorMessage"].as_str().expect("a message").to_string()
}

/// `payload` verbatim through `HandlerAdapter::handle_raw` (None = no
/// payload): the (status, body) the live bridge would transport.
async fn raw_call(payload: Option<&[u8]>) -> (u32, String) {
    use otel_bridge::function::{FunctionContext, HandlerAdapter, RawFunctionHandler};
    let adapter = HandlerAdapter::new(make_handler());
    let (tx, _rx) = tokio::sync::mpsc::unbounded_channel();
    let ctx = Arc::new(FunctionContext {
        function_call: Box::new(netdata_plugin_protocol::FunctionCall {
            transaction: "tx-raw".into(),
            timeout: 10,
            name: "otel-traces".into(),
            args: vec![],
            access: None,
            source: None,
            payload: payload.map(|b| b.to_vec()),
        }),
        cancellation_token: CancellationToken::new(),
        outbound_tx: tx,
    });
    let res = adapter.handle_raw(ctx).await;
    (
        res.status,
        String::from_utf8_lossy(&res.payload).into_owned(),
    )
}

#[tokio::test]
async fn shape_errors_are_transport_400s() {
    for (payload, needle) in [
        (&br#"{"explore": {"bogus": 1}}"#[..], "unknown field"),
        (
            br#"{"info": {}, "after": 1}"#,
            "cannot mix a mode selector with Functions parameters",
        ),
        (
            br#"{"trace": {}, "suggest": {}}"#,
            "conflicting mode selectors",
        ),
        (br#"{"info": true}"#, "invalid info selector"),
        (br#"{"trace": null}"#, "invalid trace selector"),
        (br#"{"suggest": []}"#, "expected an object"),
        (
            br#"{"conditions": [{"key": "name", "op": "=", "values": [true]}]}"#,
            "invalid list request: a chip value is a string or a number",
        ),
        (br#"[]"#, "otel-traces request object"),
        (
            br#"{"info": {}, "tenant": "a", "tenant": "b"}"#,
            "duplicate field",
        ),
    ] {
        let (status, body) = raw_call(Some(payload)).await;
        assert_eq!(
            status,
            400,
            "for {}: {body}",
            String::from_utf8_lossy(payload)
        );
        let message = bad_request_message(&body);
        assert!(
            message.starts_with("Invalid request: ") && message.contains(needle),
            "for {}: {message}",
            String::from_utf8_lossy(payload)
        );
    }
}

/// A capture failure is the caller's only when a narrower request
/// fits; a broken download cache is the agent's.
#[test]
fn capture_failures_split_between_the_caller_and_the_agent() {
    use crate::ledger::rpc::traces::sources::CaptureError;
    use netdata_plugin_error::NetdataPluginError;
    assert!(matches!(
        capture_error(CaptureError::TooLarge {
            at_least: 200,
            capacity: 100
        }),
        NetdataPluginError::BadRequest { .. }
    ));
    assert!(matches!(
        capture_error(CaptureError::EvictionFailed),
        NetdataPluginError::FunctionHandler { .. }
    ));
}

#[tokio::test]
async fn requests_the_handler_rejects_are_400s_too() {
    // The handler's own validation (zero limit, one-sided trace
    // bounds, bad trace id, a bad chip, a short suggest term) answers
    // the same body as a shape error.
    for (payload, needle) in [
        (&br#"{"last": 0}"#[..], "zero 'last'"),
        (
            br#"{"trace": {"id": "11111111111111111111111111111111", "after": 100}}"#,
            "after",
        ),
        (br#"{"trace": {"id": "xyz"}}"#, "trace id"),
        (
            br#"{"conditions": [{"key": "bogus", "op": "=", "values": ["x"]}]}"#,
            "unknown key",
        ),
        (
            br#"{"suggest": {"key": "trace_id", "after": 1, "before": 2}}"#,
            "trace_id",
        ),
    ] {
        let (status, body) = raw_call(Some(payload)).await;
        assert_eq!(
            status,
            400,
            "for {}: {body}",
            String::from_utf8_lossy(payload)
        );
        let message = bad_request_message(&body);
        assert!(
            message.starts_with("invalid otel-traces request: ") && message.contains(needle),
            "for {}: {message}",
            String::from_utf8_lossy(payload)
        );
    }
}

/// The default view: an absent payload or a selector-less `{}` both
/// select the Functions view (200); a zero-length body is a
/// deserialization failure (EOF → 400).
#[tokio::test]
async fn absent_and_empty_object_payloads_use_the_functions_view() {
    let (status, body) = raw_call(None).await;
    assert_eq!(status, 200);
    let v: serde_json::Value = serde_json::from_str(&body).unwrap();
    assert_eq!(v["type"], "traces");
    assert_eq!(v["data"]["mode"], "search");

    let (status, body) = raw_call(Some(b"{}")).await;
    assert_eq!(status, 200);
    let v: serde_json::Value = serde_json::from_str(&body).unwrap();
    assert_eq!(v["type"], "traces");

    let (status, body) = raw_call(Some(b"")).await;
    assert_eq!(status, 400);
    assert!(bad_request_message(&body).contains("EOF"), "{body}");
}

/// Every explicit mode's response serializes its `mode` discriminator,
/// one assertion each.
#[tokio::test]
async fn every_response_shape_declares_its_mode() {
    let h = handler_with_search_corpus().await;
    for (body, mode) in [
        (json!({"info": {}}), "info"),
        (
            json!({"trace": {"id": "0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a"}}),
            "trace",
        ),
        (json!({"suggest": {}}), "suggest"),
    ] {
        let v = serde_json::to_value(call_on(&h, body.clone()).await.unwrap()).unwrap();
        assert_eq!(v["mode"], mode, "for {body}");
    }
}

#[tokio::test]
async fn suggest_lists_matching_values_and_keys() {
    let h = handler_with_search_corpus().await;
    let mut body = window_body();
    merge(
        &mut body,
        json!({"key": "resource.attributes.service.name", "term": "svc-"}),
    );
    let v = serde_json::to_value(call_on(&h, as_mode("suggest", body)).await.unwrap()).unwrap();
    assert_eq!(v["mode"], "suggest");
    assert_eq!(v["key"], "resource.attributes.service.name");
    assert_eq!(
        v["items"],
        json!([
            {"text": "svc-a", "match": "prefix"},
            {"text": "svc-b", "match": "prefix"}
        ])
    );
    assert_eq!(v["status"], json!({"complete": true}));
    assert_eq!(v["truncated"], false);

    // Keys: rendered as OTLP field paths, matched by bare name too.
    let mut body = window_body();
    merge(&mut body, json!({"term": "service"}));
    let v = serde_json::to_value(call_on(&h, as_mode("suggest", body)).await.unwrap()).unwrap();
    assert!(v.get("key").is_none());
    assert_eq!(
        v["items"][0],
        json!({"text": "resource.attributes.service.name", "match": "prefix", "tier": "low", "kinds": ["str"]})
    );

    // A virtual key is flagged: it has no values to ask for.
    let mut body = window_body();
    merge(&mut body, json!({"term": "duration"}));
    let v = serde_json::to_value(call_on(&h, as_mode("suggest", body)).await.unwrap()).unwrap();
    assert_eq!(
        v["items"][0],
        json!({"text": "duration", "match": "exact", "virtual": true})
    );
}

/// A key's stored kinds travel end to end: a key holding numbers lists
/// `int`, a text key `str`.
#[tokio::test]
async fn suggest_lists_the_kinds_a_key_holds() {
    use opentelemetry_proto::tonic::common::v1::{AnyValue, KeyValue, any_value::Value as Av};
    let attr = |key: &str, value| KeyValue {
        key: key.into(),
        value: Some(AnyValue { value: Some(value) }),
        key_strindex: 0,
    };
    let mut req = otlp_req_svc(0x0A, 1, base_ns(10), "svc-a");
    req.resource_spans[0].scope_spans[0].spans[0].attributes = vec![
        attr("http.status_code", Av::IntValue(200)),
        attr("http.route", Av::StringValue("/cart".into())),
    ];
    let registries = make_registries();
    install_wal(&registries, "default", 1, vec![req]).await;
    let h = make_handler_over(registries);

    let mut body = window_body();
    merge(&mut body, json!({"term": "attributes.http."}));
    let v = serde_json::to_value(call_on(&h, as_mode("suggest", body)).await.unwrap()).unwrap();
    assert_eq!(
        v["items"],
        json!([
            {"text": "attributes.http.route", "match": "prefix", "tier": "low", "kinds": ["str"]},
            {"text": "attributes.http.status_code", "match": "prefix", "tier": "low", "kinds": ["int"]},
        ])
    );
}

/// A tier list answers every key of its tier, past the default 50 and
/// never cut; `name` holds 150 values in the window, so it is `mid`.
#[tokio::test]
async fn suggest_lists_every_key_of_a_tier() {
    use opentelemetry_proto::tonic::common::v1::{AnyValue, KeyValue, any_value::Value as Av};
    let mut req = otlp_req_svc(0x0A, 150, base_ns(10), "svc-a");
    req.resource_spans[0].scope_spans[0].spans[0].attributes = (0..60)
        .map(|i| KeyValue {
            key: format!("a{i:02}"),
            value: Some(AnyValue {
                value: Some(Av::StringValue("x".into())),
            }),
            key_strindex: 0,
        })
        .collect();
    let registries = make_registries();
    install_wal(&registries, "default", 1, vec![req]).await;
    let h = make_handler_over(registries);
    let tier = |tier: &str| {
        let mut body = window_body();
        merge(&mut body, json!({ "tier": tier }));
        as_mode("suggest", body)
    };

    let v = serde_json::to_value(call_on(&h, tier("low")).await.unwrap()).unwrap();
    let items = v["items"].as_array().unwrap();
    assert!(items.iter().all(|item| item["tier"] == "low"), "{v}");
    let wide = items
        .iter()
        .filter(|item| item["text"].as_str().unwrap().starts_with("attributes.a"))
        .count();
    assert_eq!(wide, 60, "every low key, past the default 50");
    assert!(!items.iter().any(|item| item["text"] == "name"));
    assert_eq!(v["truncated"], false);
    assert_eq!(v["status"], json!({"complete": true}));

    let v = serde_json::to_value(call_on(&h, tier("mid")).await.unwrap()).unwrap();
    assert_eq!(
        v["items"],
        json!([{"text": "name", "match": "prefix", "tier": "mid"}])
    );
}

#[tokio::test]
async fn suggest_is_validated_before_admission_and_bounded() {
    let h = handler_with_search_corpus()
        .await
        .with_suggest_concurrency(0);
    let v = serde_json::to_value(call_on(&h, json!({"suggest": {}})).await.unwrap()).unwrap();
    assert_eq!(v["status"], json!({"partial": ["busy"]}));
    assert_eq!(v["items"], json!([]));
    for (body, needle) in [
        (
            json!({"suggest": {"after": 500, "before": 400}}),
            "invalid window",
        ),
        (json!({"suggest": {"max_items": 0}}), "zero 'max_items'"),
        (
            json!({"suggest": {"max_items": 201}}),
            "exceeds the maximum 200",
        ),
        (
            json!({"suggest": {"tier": "low", "max_items": 10}}),
            "a 'tier' list is never cut",
        ),
        (
            json!({"suggest": {"tier": "low", "key": "name"}}),
            "'tier' lists keys",
        ),
        (
            json!({"suggest": {"key": "nope.x"}}),
            "invalid otel-traces request",
        ),
        (
            json!({"suggest": {"key": "duration"}}),
            "has no stored values",
        ),
        (
            json!({"suggest": {"key": "trace_id"}}),
            "has no stored values",
        ),
        (
            json!({"suggest": {"term": "x".repeat(1025)}}),
            "'term' is longer than 1024 bytes",
        ),
        (
            json!({"suggest": {"key": format!("attributes.{}", "k".repeat(1014))}}),
            "'key' is longer than 1024 bytes",
        ),
    ] {
        let err = call_on(&h, body.clone()).await.expect_err("a client error");
        assert!(err.to_string().contains(needle), "{body}: {err}");
    }
}

#[tokio::test]
async fn lists_and_trace_lookups_have_their_own_admission() {
    // No list or trace permit: both answer `busy` at once, empty and
    // without a heatmap or a cursor; the keystroke pool is untouched.
    let h = handler_with_search_corpus()
        .await
        .with_list_and_trace_concurrency(0, 0);
    let v = serde_json::to_value(call_on(&h, window_body()).await.unwrap()).unwrap();
    assert_eq!(v["type"], "traces");
    assert_eq!(v["data"]["status"], json!({"partial": ["busy"]}));
    assert_eq!(v["data"]["traces"], json!([]));
    assert!(v["data"].get("overview").is_none());
    assert!(v["data"].get("anchor").is_none());

    let v = serde_json::to_value(
        call_on(
            &h,
            json!({"trace": {"id": "0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a"}}),
        )
        .await
        .unwrap(),
    )
    .unwrap();
    assert_eq!(v["status"], json!({"partial": ["busy"]}));
    assert_eq!(v["items"]["returned"], 0);
    assert_eq!(v["field_kinds"], empty_field_kinds());

    let mut body = window_body();
    merge(
        &mut body,
        json!({"key": "resource.attributes.service.name"}),
    );
    let v = serde_json::to_value(call_on(&h, as_mode("suggest", body)).await.unwrap()).unwrap();
    assert_ne!(v["status"], json!({"partial": ["busy"]}), "{v}");
}

#[tokio::test]
async fn finished_lists_and_lookups_release_their_permits() {
    let h = handler_with_search_corpus()
        .await
        .with_list_and_trace_concurrency(1, 1);
    for _ in 0..2 {
        let v = serde_json::to_value(call_on(&h, window_body()).await.unwrap()).unwrap();
        assert_eq!(v["data"]["status"], json!({"complete": true}));
        let v = serde_json::to_value(
            call_on(
                &h,
                json!({"trace": {"id": "0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a"}}),
            )
            .await
            .unwrap(),
        )
        .unwrap();
        assert_eq!(v["status"], json!({"complete": true}));
    }
}

// ── Search-bar chips and free text ─────────────────────────────────

/// Chips and free text filter the list and the grid alike, each one a
/// token: `!` ops are NOT EXISTS, `^=` is "starts with", and a duration
/// chip narrows the list but never the grid (its own axis).
#[tokio::test]
async fn chips_and_text_filter_the_list_and_the_grid() {
    let h = handler_with_search_corpus().await;
    let run = |conditions: serde_json::Value, text: serde_json::Value| {
        let mut body = functions_body(20);
        body["conditions"] = conditions;
        body["text"] = text;
        let h = &h;
        async move { serde_json::to_value(call_on(h, body).await.unwrap()).unwrap() }
    };
    let chip = |key: &str, op: &str, values: serde_json::Value| json!([{"key": key, "op": op, "values": values}]);

    let v = run(
        chip("resource.attributes.service.name", "=", json!(["svc-a"])),
        json!([]),
    )
    .await;
    assert_eq!(ids(&v["data"]), ["0e", "0c", "0a"]);
    assert_eq!(v["data"]["overview"]["totals"]["traces"], 3);

    let v = run(
        chip("resource.attributes.service.name", "!=", json!(["svc-a"])),
        json!([]),
    )
    .await;
    assert_eq!(ids(&v["data"]), ["0d", "0b"]);
    assert_eq!(v["data"]["overview"]["totals"]["traces"], 2);

    let v = run(
        chip("resource.attributes.service.name", "^=", json!(["svc-"])),
        json!([]),
    )
    .await;
    assert_eq!(ids(&v["data"]).len(), 5);

    let v = run(chip("trace_duration", ">=", json!([1000])), json!([])).await;
    assert_eq!(ids(&v["data"]), ["0e", "0b"]);
    assert_eq!(v["data"]["overview"]["totals"]["traces"], 5, "not the grid");

    let v = run(json!([]), json!(["SVC-B"])).await;
    assert_eq!(
        ids(&v["data"]),
        ["0d", "0b"],
        "free text over resource values"
    );
    assert_eq!(v["data"]["overview"]["totals"]["traces"], 2);
}

/// Every chip op reaches the engine with its wire meaning: negated
/// regex and prefix ops are NOT EXISTS, ordering ops compare numbers,
/// root and `trace_id` chips select whole traces, and all of them
/// filter the grid too.
#[tokio::test]
async fn every_chip_op_reaches_the_engine() {
    let h = handler_with_search_corpus().await;
    let run = |conditions: serde_json::Value| {
        let mut body = functions_body(20);
        body["conditions"] = conditions;
        let h = &h;
        async move { serde_json::to_value(call_on(h, body).await.unwrap()).unwrap() }
    };
    let chip = |key: &str, op: &str, values: serde_json::Value| json!([{"key": key, "op": op, "values": values}]);
    let sorted = |v: &serde_json::Value| {
        let mut ids = ids(&v["data"]);
        ids.sort();
        ids
    };
    let id_0b = "0b".repeat(16);
    for (conditions, want, grid) in [
        // No span named span-2 or span-3: the one-span traces.
        (
            chip("name", "!~", json!(["span-[23]"])),
            vec!["0a", "0c", "0d"],
            3,
        ),
        (
            chip("resource.attributes.service.name", "!^=", json!(["svc-a"])),
            vec!["0b", "0d"],
            2,
        ),
        // Envelopes: 500 ns, 0B 1500 ns, 0E 2500 ns. Duration chips
        // leave the grid alone.
        (
            chip("trace_duration", "<", json!([1000])),
            vec!["0a", "0c", "0d"],
            5,
        ),
        (chip("trace_duration", ">", json!([1500])), vec!["0e"], 5),
        (
            chip("trace_duration", "<=", json!([1500])),
            vec!["0a", "0b", "0c", "0d"],
            5,
        ),
        (
            chip("duration", ">", json!([400])),
            vec!["0a", "0b", "0c", "0d", "0e"],
            5,
        ),
        (chip("duration", "<", json!([400])), vec![], 0),
        (
            chip("root_service_name", "=", json!(["svc-b"])),
            vec!["0b", "0d"],
            2,
        ),
        (chip("root_name", "!=", json!(["span-1"])), vec![], 0),
        (chip("trace_id", "=", json!([id_0b.clone()])), vec!["0b"], 1),
        (
            chip("trace_id", "!=", json!([id_0b.clone()])),
            vec!["0a", "0c", "0d", "0e"],
            4,
        ),
    ] {
        let v = run(conditions.clone()).await;
        assert_eq!(sorted(&v), want, "{conditions}");
        assert_eq!(
            v["data"]["overview"]["totals"]["traces"], grid,
            "{conditions}"
        );
    }
}

/// A malformed chip or text is a client error before admission.
#[tokio::test]
async fn bad_chips_and_text_are_client_errors() {
    let h = handler_with_search_corpus().await;
    let chip = |key: &str, op: &str, values: serde_json::Value| json!([{"key": key, "op": op, "values": values}]);
    let too_many: Vec<serde_json::Value> = (0..65).map(|i| json!(format!("v{i}"))).collect();
    let tokens_33: Vec<serde_json::Value> = (0..33).map(|i| json!(format!("term{i}"))).collect();
    let none = json!([]);
    for (conditions, text, message) in [
        (
            chip("resource.attributes.service.name", "~=", json!(["x"])),
            none.clone(),
            "condition on 'resource.attributes.service.name': unknown op '~='",
        ),
        (
            chip("nope", "=", json!(["x"])),
            none.clone(),
            "unknown key \"nope\": use",
        ),
        (
            chip("resource.attributes.service.name", "=", json!(too_many)),
            none.clone(),
            "condition on 'resource.attributes.service.name' carries 65 values, over the limit of 64",
        ),
        (
            chip("resource.attributes.service.name", "=~", json!(["("])),
            none.clone(),
            "'resource.attributes.service.name =~': invalid pattern \"(\": ",
        ),
        (
            chip("resource.attributes.service.name", "^=", json!([5])),
            none.clone(),
            "condition on 'resource.attributes.service.name': '^=' takes text values",
        ),
        (
            chip("trace_duration", ">=", json!(["long"])),
            none.clone(),
            "'trace_duration >=' takes whole nanoseconds",
        ),
        (
            chip("trace_duration", "=~", json!(["x"])),
            none.clone(),
            "'=~' is not valid on 'trace_duration'",
        ),
        (
            chip("trace_duration", "^=", json!(["1"])),
            none.clone(),
            "'^=' is not valid on 'trace_duration'",
        ),
        (
            chip("name", ">=", json!([5])),
            none.clone(),
            "'>=' is not valid on 'name'",
        ),
        (
            chip("trace_duration", ">=", json!([1, 2])),
            none.clone(),
            "'trace_duration >=' takes exactly one value, got 2",
        ),
        (
            chip("resource.attributes.service.name", "=", json!([])),
            none.clone(),
            "'resource.attributes.service.name =' has no values",
        ),
        (
            chip("trace_id", "=", json!(["xyz"])),
            none.clone(),
            "'trace_id' takes 32-character hex ids, got \"xyz\"",
        ),
        (
            chip("status.code", "=", json!([5])),
            none.clone(),
            "'status.code =' takes text values",
        ),
        (
            none.clone(),
            json!(["ab"]),
            "text \"ab\" is shorter than 3 characters",
        ),
        (
            none.clone(),
            json!(tokens_33),
            "at most 32 conditions and text terms, got 33",
        ),
    ] {
        let mut body = functions_body(20);
        body["conditions"] = conditions.clone();
        body["text"] = text.clone();
        let err = call_on(&h, body).await.unwrap_err().to_string();
        assert!(
            err.contains(&format!("invalid otel-traces request: {message}")),
            "{conditions} {text}: {err}"
        );
    }
}

/// A malformed chip or text in the trace mode is a client error too.
#[tokio::test]
async fn bad_chips_in_the_trace_mode_are_client_errors() {
    let registries = make_registries();
    install_sealed(
        &registries,
        "default",
        1,
        vec![otlp_req(0x11, 3, 1_000_000_000)],
    )
    .await;
    let h = make_handler_over(registries);
    for (params, message) in [
        (
            json!({"conditions": [{"key": "nope", "op": "=", "values": ["x"]}]}),
            "unknown key \"nope\": use",
        ),
        (
            json!({"conditions": [{"key": "name", "op": "~~", "values": ["x"]}]}),
            "condition on 'name': unknown op '~~'",
        ),
        (
            json!({"conditions": [{"key": "name", "op": ">", "values": [1]}]}),
            "'>' is not valid on 'name'",
        ),
        (
            json!({"text": ["ab"]}),
            "text \"ab\" is shorter than 3 characters",
        ),
    ] {
        let mut trace = params.clone();
        trace["id"] = json!(FIXTURE_TRACE_ID);
        let err = call_on(&h, json!({ "trace": trace }))
            .await
            .unwrap_err()
            .to_string();
        assert!(
            err.contains(&format!("invalid otel-traces request: {message}")),
            "{params}: {err}"
        );
    }
}

/// The trace mode marks the spans matching the list's chips; without
/// chips nothing is marked, and the field is always present.
#[tokio::test]
async fn the_trace_marks_spans_matching_the_chips() {
    let registries = make_registries();
    install_sealed(
        &registries,
        "default",
        1,
        vec![otlp_req(0x11, 3, 1_000_000_000)],
    )
    .await;
    let h = make_handler_over(registries);
    let v = serde_json::to_value(
        call_on(&h, json!({"trace": {"id": FIXTURE_TRACE_ID}}))
            .await
            .unwrap(),
    )
    .unwrap();
    assert_eq!(v["matched"], json!([]));
    let v = serde_json::to_value(
        call_on(
            &h,
            json!({"trace": {
                "id": FIXTURE_TRACE_ID,
                "conditions": [{"key": "name", "op": "=", "values": ["span-1"]}],
            }}),
        )
        .await
        .unwrap(),
    )
    .unwrap();
    let matched = v["matched"].as_array().unwrap();
    assert_eq!(matched.len(), 1);
    let span = &v["spans"][matched[0].as_u64().unwrap() as usize];
    assert!(
        span["fields"]
            .as_array()
            .unwrap()
            .iter()
            .any(|f| f == &json!(["name", "span-1"]))
    );
}
