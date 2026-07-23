use super::*;
use crate::ledger::rpc::traces::fixtures::{install_wal, make_registries, otlp_req, otlp_req_svc};
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
    assert_eq!(
        v["accepted_params"],
        json!(["info", "trace", "tenant", "after", "before", "last", "anchor"])
    );
    assert_eq!(v["required_params"], json!([]));
}

#[tokio::test]
async fn default_request_is_an_empty_complete_search() {
    // The bridge turns a missing payload into `{}` — the default data
    // mode is a search over the default recent window; on an empty
    // agent that's an empty COMPLETE page, never a panic or an error.
    let resp = call(json!({})).await.unwrap();
    let v = serde_json::to_value(&resp).unwrap();
    assert_eq!(v["status"], json!({"complete": true}));
    assert_eq!(v["items"], json!({"returned": 0, "max_to_return": 20}));
    assert_eq!(v["traces"], json!([]));
    assert!(v.get("anchor").is_none(), "a short page has no next cursor");
}

#[tokio::test]
async fn remaining_data_modes_error_with_their_names() {
    let err = call(json!({"overview": {}}))
        .await
        .expect_err("overview is not implemented yet");
    let msg = err.to_string();
    assert!(msg.contains("overview"), "names the mode: {msg}");
    assert!(msg.contains("not implemented"), "states why: {msg}");
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

// ── The search mode ─────────────────────────────────────────────────

/// Corpus base, unix seconds (chosen inside an explicit query window).
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

fn window_body() -> serde_json::Value {
    json!({"after": T_S, "before": T_S + 100})
}

fn ids(v: &serde_json::Value) -> Vec<String> {
    v["traces"]
        .as_array()
        .unwrap()
        .iter()
        .map(|t| t["trace_id"].as_str().unwrap()[..2].to_string())
        .collect()
}

#[tokio::test]
async fn search_returns_most_recent_first_with_deterministic_ties() {
    let h = handler_with_search_corpus().await;
    let mut body = window_body();
    body["spans_per_trace"] = json!(1);
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert_eq!(v["status"], json!({"complete": true}));
    // Newest first; the C/D tie breaks by trace_id ascending.
    assert_eq!(ids(&v), vec!["0e", "0c", "0d", "0b", "0a"]);
    let e = &v["traces"][0];
    assert_eq!(e["root_service"], "svc-a");
    assert_eq!(e["root_name"], "span-1");
    assert_eq!(e["span_count"], 3);
    assert_eq!(e["exact"], true);
    assert_eq!(e["matched_spans"].as_array().unwrap().len(), 1);
}

#[tokio::test]
async fn selections_narrow_by_service_operation_and_attributes() {
    let h = handler_with_search_corpus().await;

    let mut body = window_body();
    body["selections"] = json!({"resource.service.name": ["svc-a"]});
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert_eq!(ids(&v), vec!["0e", "0c", "0a"]);

    // Multi-value OR within a key.
    let mut body = window_body();
    body["selections"] = json!({"resource.service.name": ["svc-a", "svc-b"]});
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert_eq!(ids(&v).len(), 5);

    // Builtin word: only B and E have a second span.
    let mut body = window_body();
    body["selections"] = json!({"name": ["span-2"]});
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert_eq!(ids(&v), vec!["0e", "0b"]);

    // Keys AND across selections.
    let mut body = window_body();
    body["selections"] = json!({"name": ["span-2"], "resource.service.name": ["svc-b"]});
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert_eq!(ids(&v), vec!["0b"]);
}

#[tokio::test]
async fn duration_bounds_filter_spans() {
    let h = handler_with_search_corpus().await;
    // Fixture spans last 500 ns each.
    let mut body = window_body();
    body["min_duration_ns"] = json!(501);
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert_eq!(ids(&v).len(), 0);

    let mut body = window_body();
    body["min_duration_ns"] = json!(100);
    body["max_duration_ns"] = json!(500);
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert_eq!(ids(&v).len(), 5);
}

#[tokio::test]
async fn pagination_walks_the_corpus_without_dups_or_gaps_across_the_tie() {
    let h = handler_with_search_corpus().await;

    // Page 1 (last=2): E, C — the tail C ties with D.
    let mut body = window_body();
    body["last"] = json!(2);
    let v1 = serde_json::to_value(call_on(&h, body.clone()).await.unwrap()).unwrap();
    assert_eq!(ids(&v1), vec!["0e", "0c"]);
    let next1 = v1["anchor"]["next"].as_str().unwrap().to_string();

    // Page 2: the tie partner D, then B.
    body["anchor"] = json!(next1);
    let v2 = serde_json::to_value(call_on(&h, body.clone()).await.unwrap()).unwrap();
    assert_eq!(ids(&v2), vec!["0d", "0b"]);
    let next2 = v2["anchor"]["next"].as_str().unwrap().to_string();

    // Page 3: A alone; short page, walk over.
    body["anchor"] = json!(next2);
    let v3 = serde_json::to_value(call_on(&h, body.clone()).await.unwrap()).unwrap();
    assert_eq!(ids(&v3), vec!["0a"]);
    assert!(v3.get("anchor").is_none());
}

#[tokio::test]
async fn spans_per_trace_zero_attaches_no_spans() {
    let h = handler_with_search_corpus().await;
    let mut body = window_body();
    body["spans_per_trace"] = json!(0);
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert!(
        v["traces"]
            .as_array()
            .unwrap()
            .iter()
            .all(|t| t["matched_spans"].as_array().unwrap().is_empty())
    );
    // The counts still tell the story.
    assert_eq!(v["traces"][0]["matched_count"], 3);
}

#[tokio::test]
async fn invalid_search_requests_are_clean_client_errors() {
    let h = handler_with_search_corpus().await;
    for (body, needle) in [
        (json!({"last": 0}), "zero 'last'"),
        (json!({"spans_per_trace": 200}), "exceeds the library maximum"),
        (json!({"selections": {"bogus": ["x"]}}), "unknown selection key"),
        (json!({"anchor": "junk"}), "malformed anchor"),
        (json!({"after": 500, "before": 400}), "invalid window"),
    ] {
        let err = call_on(&h, body.clone())
            .await
            .expect_err("must be a client error");
        let msg = err.to_string();
        assert!(msg.contains(needle), "for {body}: {msg}");
    }
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
    body["last"] = json!(2);
    body["spans_per_trace"] = json!(0);
    let v1 = serde_json::to_value(call_on(&h, body.clone()).await.unwrap()).unwrap();
    assert_eq!(ids(&v1), vec!["1b", "0f"], "V (50s) then F (rank 45s)");
    body["anchor"] = v1["anchor"]["next"].clone();
    let v2 = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert_eq!(ids(&v2), vec!["1a"], "U (30s) must not be gapped out");
    assert!(v2.get("anchor").is_none());
}

#[tokio::test]
async fn straddling_trace_above_the_tail_never_reappears() {
    // The round-1 review's HIGH: F's matched spans (10s, 70s) straddle
    // page 1's tail rank (U at 50s). The narrowed-window design let F
    // re-enter page 2 at rank 10s — a duplicate. The frozen-window
    // after-key walk keeps F at rank 70s (at-or-above the key) forever.
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
    body["last"] = json!(2);
    body["spans_per_trace"] = json!(0);
    let v1 = serde_json::to_value(call_on(&h, body.clone()).await.unwrap()).unwrap();
    assert_eq!(ids(&v1), vec!["0f", "1a"], "F (rank 70s) then U (50s)");
    body["anchor"] = v1["anchor"]["next"].clone();
    let v2 = serde_json::to_value(call_on(&h, body.clone()).await.unwrap()).unwrap();
    assert_eq!(ids(&v2), vec!["1b"], "only W is fresh — F must not duplicate");
    assert!(v2.get("anchor").is_none());
}

#[tokio::test]
async fn anchor_pages_ignore_the_requests_own_window() {
    // The cursor freezes page 1's window; a page-2 request carrying a
    // different (or defaulted) window must not shift the walk.
    let h = handler_with_search_corpus().await;
    let mut body = window_body();
    body["last"] = json!(2);
    body["spans_per_trace"] = json!(0);
    let v1 = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    let next = v1["anchor"]["next"].clone();

    // Page 2 with a WRONG window — the frozen one must win.
    let body2 = json!({"after": 1, "before": 2, "last": 2, "spans_per_trace": 0, "anchor": next});
    let v2 = serde_json::to_value(call_on(&h, body2).await.unwrap()).unwrap();
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
    body["last"] = json!(2);
    body["spans_per_trace"] = json!(0);
    let v1 = serde_json::to_value(call_on(&h, body.clone()).await.unwrap()).unwrap();
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
    let v2 = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
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

// ── The enumeration modes ───────────────────────────────────────────

#[tokio::test]
async fn attributes_lists_keys_in_the_selection_grammar() {
    let h = handler_with_search_corpus().await;
    let mut body = window_body();
    body["attributes"] = json!({});
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert_eq!(v["status"], json!({"complete": true}));
    assert_eq!(v["truncated"], false);
    let keys: Vec<&str> = v["keys"].as_array().unwrap().iter().map(|k| k.as_str().unwrap()).collect();
    assert!(keys.contains(&"resource.service.name"), "{keys:?}");
    assert!(keys.contains(&"name"), "builtin word: {keys:?}");
    // The round-trip contract (every returned key feeds straight back
    // as a selection or a values request) is pinned in the adapter
    // tests; here we spot-check it end to end through the Function.
    let mut body = window_body();
    body["attribute_values"] = json!({"key": keys[0]});
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert_eq!(v["key"], keys[0]);
}

#[tokio::test]
async fn attributes_owner_filter_and_truncation_are_exact() {
    let h = handler_with_search_corpus().await;
    let mut body = window_body();
    body["attributes"] = json!({"owner": "resource"});
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    let keys = v["keys"].as_array().unwrap();
    assert!(!keys.is_empty());
    assert!(
        keys.iter().all(|k| k.as_str().unwrap().starts_with("resource.")),
        "{keys:?}"
    );

    let mut body = window_body();
    body["attributes"] = json!({"max_keys": 1});
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert_eq!(v["keys"].as_array().unwrap().len(), 1);
    assert_eq!(v["truncated"], true);
}

#[tokio::test]
async fn attribute_values_returns_storage_labels_with_kinds() {
    let h = handler_with_search_corpus().await;
    let mut body = window_body();
    body["attribute_values"] = json!({"key": "resource.service.name"});
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert_eq!(v["key"], "resource.service.name");
    assert_eq!(v["truncated"], false);
    let values: Vec<&str> = v["values"].as_array().unwrap().iter().map(|x| x["value"].as_str().unwrap()).collect();
    assert!(values.contains(&"svc-a") && values.contains(&"svc-b"), "{values:?}");

    // Builtin `name`: the corpus's span names.
    let mut body = window_body();
    body["attribute_values"] = json!({"key": "name"});
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    let values: Vec<&str> = v["values"].as_array().unwrap().iter().map(|x| x["value"].as_str().unwrap()).collect();
    assert!(values.contains(&"span-1"), "{values:?}");

    // Truncation flag exact.
    let mut body = window_body();
    body["attribute_values"] = json!({"key": "resource.service.name", "max_values": 1});
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert_eq!(v["values"].as_array().unwrap().len(), 1);
    assert_eq!(v["truncated"], true);
}

#[tokio::test]
async fn enumeration_invalid_requests_are_clean_client_errors() {
    let h = handler_with_search_corpus().await;
    for (body, needle) in [
        (json!({"attributes": null}), "invalid attributes selector"),
        (json!({"attributes": {"owner": "bogus"}}), "unknown owner"),
        (json!({"attributes": {"max_keys": 0}}), "zero key/value limit"),
        (json!({"attribute_values": null}), "invalid attribute_values selector"),
        (json!({"attribute_values": {}}), "missing field"),
        (json!({"attribute_values": {"key": "bogus"}}), "unknown selection key"),
        // A virtual builtin has no value dictionary — the engine's own
        // message surfaces.
        (json!({"attribute_values": {"key": "duration"}}), "virtual"),
    ] {
        let err = call_on(&h, body.clone()).await.expect_err("must be a client error");
        let msg = err.to_string();
        assert!(msg.contains(needle), "for {body}: {msg}");
    }
}
