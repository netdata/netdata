use super::*;
use crate::ledger::rpc::traces::fixtures::{install_wal, make_registries, otlp_req};
use bridge::function::ProgressState;
use file_lifecycle::registry::TenantRegistries;
use serde_json::json;
use tokio_util::sync::CancellationToken;

fn make_handler_over(registries: Arc<RwLock<TenantRegistries>>) -> OtelTracesHandler {
    // Small min_entries so the WAL fixtures split into chunks + tail —
    // the end-to-end tests then cross the chunk-build path for real.
    OtelTracesHandler::new(registries, Arc::new(ChunkCache::new(64 * 1024 * 1024)), 4)
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

#[tokio::test]
async fn info_returns_the_descriptor() {
    let resp = call(json!({"info": true})).await.unwrap();
    let v = serde_json::to_value(&resp).unwrap();
    assert_eq!(v["version"], 1);
    assert_eq!(v["status"], 200);
    assert_eq!(v["accepted_params"], json!(["info"]));
    assert_eq!(v["required_params"], json!([]));
}

#[tokio::test]
async fn default_request_is_search_and_not_implemented() {
    // The bridge turns a missing payload into `{}` — the default data
    // mode must answer with a clean, mode-naming error, never a panic
    // or a silent empty result.
    let err = call(json!({})).await.expect_err("search is not implemented yet");
    let msg = err.to_string();
    assert!(msg.contains("search"), "names the mode: {msg}");
    assert!(msg.contains("not implemented"), "states why: {msg}");
}

#[tokio::test]
async fn remaining_data_modes_error_with_their_names() {
    for (body, mode) in [
        (json!({"attributes": {}}), "attributes"),
        (json!({"attribute_values": {}}), "attribute_values"),
        (json!({"overview": {}}), "overview"),
    ] {
        let err = call(body).await.expect_err("data modes are not implemented yet");
        let msg = err.to_string();
        assert!(msg.contains(mode), "names mode {mode}: {msg}");
        assert!(msg.contains("not implemented"), "states why: {msg}");
    }
}

#[tokio::test]
async fn conflicting_selectors_error_lists_them() {
    let err = call(json!({"trace": {}, "attributes": {}}))
        .await
        .expect_err("conflicting selectors are a client error");
    let msg = err.to_string();
    assert!(msg.contains("conflicting mode selectors"), "{msg}");
    assert!(msg.contains("trace") && msg.contains("attributes"), "{msg}");
}

#[test]
fn declaration_advertises_otel_traces() {
    let d = make_handler().declaration();
    assert_eq!(d.name, "otel-traces");
    assert!(d.global);
    assert_eq!(d.tags.as_deref(), Some("traces"));
    assert_eq!(
        d.access,
        Some(HttpAccess::SIGNED_ID | HttpAccess::SAME_SPACE | HttpAccess::SENSITIVE_DATA)
    );
}

// ── The trace mode ──────────────────────────────────────────────────

/// The fixture trace: 3 spans, ids [0x11;16]/[i;8], span 1 the root.
const FIXTURE_TRACE_ID: &str = "11111111111111111111111111111111";

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
async fn trace_by_id_assembles_the_fixture_trace_end_to_end() {
    let h = handler_with_fixture_wal().await;
    let resp = call_on(&h, json!({"trace": {"id": FIXTURE_TRACE_ID}}))
        .await
        .unwrap();
    let v = serde_json::to_value(&resp).unwrap();

    assert_eq!(v["trace_id"], FIXTURE_TRACE_ID);
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
    // The seal indexed the OTLP name into the row facets.
    assert!(
        s0["fields"]
            .as_array()
            .unwrap()
            .iter()
            .any(|kv| kv == &json!(["name", "span-1"])),
        "fields carry the span name: {}",
        s0["fields"]
    );
    // And the typed map describes what came back.
    assert!(
        v["field_kinds"]["fields"]
            .as_array()
            .unwrap()
            .iter()
            .any(|kv| kv[0] == "name"),
        "{}",
        v["field_kinds"]
    );
}

#[tokio::test]
async fn span_cap_returns_the_earliest_spans_and_a_size_cap_partial() {
    let h = handler_with_fixture_wal().await;
    let resp = call_on(
        &h,
        json!({"trace": {"id": FIXTURE_TRACE_ID, "span_cap": 2}}),
    )
    .await
    .unwrap();
    let v = serde_json::to_value(&resp).unwrap();
    assert_eq!(v["status"], json!({"partial": ["size_cap"]}));
    assert_eq!(v["items"]["returned"], 2);
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
}

#[tokio::test]
async fn malformed_trace_selectors_are_clean_client_errors() {
    for (body, needle) in [
        // null must not fall through to another mode NOR panic.
        (json!({"trace": null}), "invalid trace selector"),
        (json!({"trace": {"id": "xyz"}}), "32 hex"),
        (json!({"trace": {}}), "missing field"),
        // A typo'd parameter on a two-field object is an error.
        (
            json!({"trace": {"id": FIXTURE_TRACE_ID, "span_capp": 1}}),
            "unknown field",
        ),
        // The engine's own request validation surfaces verbatim.
        (
            json!({"trace": {"id": "00000000000000000000000000000000"}}),
            "all-zero",
        ),
        (
            json!({"trace": {"id": FIXTURE_TRACE_ID, "span_cap": 0}}),
            "zero span cap",
        ),
    ] {
        let err = call(body.clone()).await.expect_err("must be a client error");
        let msg = err.to_string();
        assert!(msg.contains(needle), "for {body}: {msg}");
    }
}
