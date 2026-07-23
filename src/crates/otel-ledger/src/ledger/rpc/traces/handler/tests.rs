use super::*;
use bridge::function::ProgressState;
use file_lifecycle::chunk::ChunkCache;
use serde_json::json;
use tokio_util::sync::CancellationToken;

fn make_handler() -> OtelTracesHandler {
    let tr = TenantRegistries::new(
        tempfile::tempdir().unwrap().keep(),
        tempfile::tempdir().unwrap().keep(),
        tempfile::tempdir().unwrap().keep(),
    );
    OtelTracesHandler::new(
        Arc::new(RwLock::new(tr)),
        Arc::new(ChunkCache::new(64 * 1024 * 1024)),
        16_384,
    )
}

fn make_ctx() -> FunctionCallContext {
    FunctionCallContext::new(
        "tx-test".to_string(),
        ProgressState::new(),
        CancellationToken::new(),
    )
}

async fn call(v: serde_json::Value) -> netdata_plugin_error::Result<OtelTracesResponse> {
    let req: OtelTracesRequest = serde_json::from_value(v).unwrap();
    make_handler().on_call(make_ctx(), req).await
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
async fn every_data_mode_errors_with_its_name() {
    for (body, mode) in [
        (json!({"trace": {"id": "00"}}), "trace"),
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
