use super::*;
use crate::ledger::rpc::traces::fixtures::{
    install_sfst, install_wal, make_registries, otlp_req, otlp_req_svc,
};
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
    let resp = call(json!({"info": {}})).await.unwrap();
    let v = serde_json::to_value(&resp).unwrap();
    assert_eq!(v["mode"], "info");
    assert_eq!(v["version"], 1);
    assert_eq!(v["status"], 200);
    assert_eq!(
        v["accepted_params"],
        json!([
            "info", "trace", "attributes", "attribute_values", "overview",
            "slowest", "search", "tenant"
        ])
    );
    assert_eq!(v["required_params"], json!([]));
}

#[tokio::test]
async fn an_empty_search_object_is_an_empty_complete_page() {
    // `{"search": {}}` takes every default (recent window, limit 20);
    // on an empty agent that's an empty COMPLETE page, never a panic
    // or an error. (A bodyless `{}` request is a missing-mode client
    // error now — pinned in the wire tests and the bridge-level tests.)
    let resp = call(json!({"search": {}})).await.unwrap();
    let v = serde_json::to_value(&resp).unwrap();
    assert_eq!(v["mode"], "search");
    assert_eq!(v["status"], json!({"complete": true}));
    assert_eq!(v["items"], json!({"returned": 0, "max_to_return": 20}));
    assert_eq!(v["traces"], json!([]));
    assert!(v.get("anchor").is_none(), "a short page has no next cursor");
}

#[tokio::test]
async fn every_mode_is_implemented_an_empty_agent_answers_them_all() {
    // The mode catalog is complete: no selector errors as
    // not-implemented anymore; an empty agent answers each cleanly.
    for body in [
        json!({"overview": {}}),
        json!({"slowest": {}}),
        json!({"attributes": {}}),
        json!({"attribute_values": {"key": "name"}}),
    ] {
        call(body.clone()).await.unwrap_or_else(|e| panic!("{body}: {e}"));
    }
}

// Conflicting/malformed/missing-mode bodies no longer reach on_call —
// they fail request DESERIALIZATION (pinned in the wire tests) and the
// bridge maps them to transport 400s (pinned in the bridge-level tests
// at the end of this file).

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
async fn trace_coverage_declares_the_full_range_for_absent_bounds() {
    let h = handler_with_fixture_wal().await;
    let resp = call_on(&h, json!({"trace": {"id": FIXTURE_TRACE_ID}}))
        .await
        .unwrap();
    let v = serde_json::to_value(&resp).unwrap();
    assert_eq!(v["coverage"], json!({"after": 0, "before": 4_294_967_295_u32}));
}

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
async fn trace_bounds_width_at_the_cap_is_accepted() {
    let h = handler_with_fixture_wal().await;
    let v = serde_json::to_value(
        call_on(
            &h,
            json!({"trace": {"id": FIXTURE_TRACE_ID, "after": 0, "before": 172_800}}),
        )
        .await
        .unwrap(),
    )
    .unwrap();
    assert_eq!(v["coverage"], json!({"after": 0, "before": 172_800}));
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
    assert_eq!(v["coverage"], json!({"after": 0, "before": 4_294_967_295_u32}));
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
    // Pin WHICH spans: the globally earliest two (fixture starts ascend
    // with the span id), so a latest-two regression fails here, not only
    // in the combiner's own unit test.
    assert_eq!(v["spans"][0]["span_id"], "01".repeat(8));
    assert_eq!(v["spans"][1]["span_id"], "02".repeat(8));
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
async fn semantically_invalid_trace_requests_are_clean_client_errors() {
    // SHAPE errors (null/array selectors, missing/unknown fields) fail
    // request deserialization and are pinned in the wire tests; what
    // stays here is the SEMANTIC validation the handler owns.
    for (body, needle) in [
        (json!({"trace": {"id": "xyz"}}), "32 hex"),
        // The engine's own request validation surfaces verbatim.
        (
            json!({"trace": {"id": "00000000000000000000000000000000"}}),
            "all-zero",
        ),
        (
            json!({"trace": {"id": FIXTURE_TRACE_ID, "span_cap": 0}}),
            "zero span cap",
        ),
        // The wire may only tighten the runaway-merge bound.
        (
            json!({"trace": {"id": FIXTURE_TRACE_ID, "span_cap": 65_537}}),
            "exceeds the maximum",
        ),
        // Assembly bounds: both-or-neither, ordered, width-capped.
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
        (
            json!({"trace": {"id": FIXTURE_TRACE_ID, "after": 0, "before": 172_901}}),
            "exceeds the maximum 172800",
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

/// Wrap flat params as the wire's `{mode: params}` shape — tests build
/// params flat and name the mode at the call site.
fn as_mode(mode: &str, params: serde_json::Value) -> serde_json::Value {
    json!({ mode: params })
}

fn merge(base: &mut serde_json::Value, extra: serde_json::Value) {
    base.as_object_mut()
        .unwrap()
        .extend(extra.as_object().unwrap().clone());
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
async fn search_declares_the_widened_completion_coverage() {
    let h = handler_with_search_corpus().await;
    let v = serde_json::to_value(call_on(&h, as_mode("search", window_body())).await.unwrap()).unwrap();
    // Window width 100s clamps to the 1h minimum slack per side.
    assert_eq!(
        v["completion_coverage"],
        json!({"after": T_S - 3_600, "before": T_S + 100 + 3_600})
    );
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
    let v = serde_json::to_value(call_on(&h, as_mode("search", window_body())).await.unwrap()).unwrap();
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
    let v = serde_json::to_value(call_on(&h, as_mode("search", window_body())).await.unwrap()).unwrap();
    assert_eq!(v["status"], json!({"complete": true}));
}

#[tokio::test]
async fn search_returns_most_recent_first_with_deterministic_ties() {
    let h = handler_with_search_corpus().await;
    let mut body = window_body();
    body["spans_per_trace"] = json!(1);
    let v = serde_json::to_value(call_on(&h, as_mode("search", body)).await.unwrap()).unwrap();
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
    let v = serde_json::to_value(call_on(&h, as_mode("search", body)).await.unwrap()).unwrap();
    assert_eq!(ids(&v), vec!["0e", "0c", "0a"]);

    // Multi-value OR within a key.
    let mut body = window_body();
    body["selections"] = json!({"resource.service.name": ["svc-a", "svc-b"]});
    let v = serde_json::to_value(call_on(&h, as_mode("search", body)).await.unwrap()).unwrap();
    assert_eq!(ids(&v).len(), 5);

    // Builtin word: only B and E have a second span.
    let mut body = window_body();
    body["selections"] = json!({"name": ["span-2"]});
    let v = serde_json::to_value(call_on(&h, as_mode("search", body)).await.unwrap()).unwrap();
    assert_eq!(ids(&v), vec!["0e", "0b"]);

    // Keys AND across selections.
    let mut body = window_body();
    body["selections"] = json!({"name": ["span-2"], "resource.service.name": ["svc-b"]});
    let v = serde_json::to_value(call_on(&h, as_mode("search", body)).await.unwrap()).unwrap();
    assert_eq!(ids(&v), vec!["0b"]);
}

#[tokio::test]
async fn duration_bounds_filter_spans() {
    let h = handler_with_search_corpus().await;
    // Fixture spans last 500 ns each.
    let mut body = window_body();
    body["min_duration_ns"] = json!(501);
    let v = serde_json::to_value(call_on(&h, as_mode("search", body)).await.unwrap()).unwrap();
    assert_eq!(ids(&v).len(), 0);

    let mut body = window_body();
    body["min_duration_ns"] = json!(100);
    body["max_duration_ns"] = json!(500);
    let v = serde_json::to_value(call_on(&h, as_mode("search", body)).await.unwrap()).unwrap();
    assert_eq!(ids(&v).len(), 5);
}

#[tokio::test]
async fn pagination_walks_the_corpus_without_dups_or_gaps_across_the_tie() {
    let h = handler_with_search_corpus().await;

    // Page 1 (limit=2): E, C — the tail C ties with D.
    let mut body = window_body();
    body["limit"] = json!(2);
    let v1 = serde_json::to_value(call_on(&h, as_mode("search", body.clone())).await.unwrap()).unwrap();
    assert_eq!(ids(&v1), vec!["0e", "0c"]);
    let next1 = v1["anchor"]["next"].as_str().unwrap().to_string();

    // Page 2: the tie partner D, then B.
    body["anchor"] = json!(next1);
    let v2 = serde_json::to_value(call_on(&h, as_mode("search", body.clone())).await.unwrap()).unwrap();
    assert_eq!(ids(&v2), vec!["0d", "0b"]);
    let next2 = v2["anchor"]["next"].as_str().unwrap().to_string();

    // Page 3: A alone; short page, walk over.
    body["anchor"] = json!(next2);
    let v3 = serde_json::to_value(call_on(&h, as_mode("search", body.clone())).await.unwrap()).unwrap();
    assert_eq!(ids(&v3), vec!["0a"]);
    assert!(v3.get("anchor").is_none());
}

#[tokio::test]
async fn spans_per_trace_zero_attaches_no_spans() {
    let h = handler_with_search_corpus().await;
    let mut body = window_body();
    body["spans_per_trace"] = json!(0);
    let v = serde_json::to_value(call_on(&h, as_mode("search", body)).await.unwrap()).unwrap();
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
        (json!({"limit": 0}), "zero 'limit'"),
        (json!({"limit": 1001}), "exceeds the maximum"),
        (json!({"spans_per_trace": 200}), "exceeds the library maximum"),
        (json!({"selections": {"bogus": ["x"]}}), "unknown selection key"),
        (json!({"anchor": "junk"}), "malformed anchor"),
        (json!({"after": 500, "before": 400}), "invalid window"),
    ] {
        let err = call_on(&h, as_mode("search", body.clone()))
            .await
            .expect_err("must be a client error");
        let msg = err.to_string();
        assert!(msg.contains(needle), "for {body}: {msg}");
    }
    // The boundary itself is legal (locks the cap against an accidental
    // off-by-one `<`).
    call_on(&h, as_mode("search", json!({"limit": 1000})))
        .await
        .expect("limit == max succeeds");
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
    body["spans_per_trace"] = json!(0);
    let v1 = serde_json::to_value(call_on(&h, as_mode("search", body.clone())).await.unwrap()).unwrap();
    assert_eq!(ids(&v1), vec!["1b", "0f"], "V (50s) then F (rank 45s)");
    body["anchor"] = v1["anchor"]["next"].clone();
    let v2 = serde_json::to_value(call_on(&h, as_mode("search", body)).await.unwrap()).unwrap();
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
    body["limit"] = json!(2);
    body["spans_per_trace"] = json!(0);
    let v1 = serde_json::to_value(call_on(&h, as_mode("search", body.clone())).await.unwrap()).unwrap();
    assert_eq!(ids(&v1), vec!["0f", "1a"], "F (rank 70s) then U (50s)");
    body["anchor"] = v1["anchor"]["next"].clone();
    let v2 = serde_json::to_value(call_on(&h, as_mode("search", body.clone())).await.unwrap()).unwrap();
    assert_eq!(ids(&v2), vec!["1b"], "only W is fresh — F must not duplicate");
    assert!(v2.get("anchor").is_none());
}

#[tokio::test]
async fn anchor_pages_ignore_the_requests_own_window() {
    // The cursor freezes page 1's window; a page-2 request carrying a
    // different (or defaulted) window must not shift the walk.
    let h = handler_with_search_corpus().await;
    let mut body = window_body();
    body["limit"] = json!(2);
    body["spans_per_trace"] = json!(0);
    let v1 = serde_json::to_value(call_on(&h, as_mode("search", body)).await.unwrap()).unwrap();
    let next = v1["anchor"]["next"].clone();

    // Page 2 with a WRONG window — the frozen one must win.
    let body2 = as_mode(
        "search",
        json!({"after": 1, "before": 2, "limit": 2, "spans_per_trace": 0, "anchor": next}),
    );
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
    body["limit"] = json!(2);
    body["spans_per_trace"] = json!(0);
    let v1 = serde_json::to_value(call_on(&h, as_mode("search", body.clone())).await.unwrap()).unwrap();
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
    let v2 = serde_json::to_value(call_on(&h, as_mode("search", body)).await.unwrap()).unwrap();
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
    merge(&mut body, json!({}));
    let body = as_mode("attributes", body);
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
    merge(&mut body, json!({"key": keys[0]}));
    let body = as_mode("attribute_values", body);
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert_eq!(v["key"], keys[0]);
}

#[tokio::test]
async fn attributes_owner_filter_and_truncation_are_exact() {
    let h = handler_with_search_corpus().await;
    let mut body = window_body();
    merge(&mut body, json!({"owner": "resource"}));
    let body = as_mode("attributes", body);
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    let keys = v["keys"].as_array().unwrap();
    assert!(!keys.is_empty());
    assert!(
        keys.iter().all(|k| k.as_str().unwrap().starts_with("resource.")),
        "{keys:?}"
    );

    let mut body = window_body();
    merge(&mut body, json!({"max_keys": 1}));
    let body = as_mode("attributes", body);
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert_eq!(v["keys"].as_array().unwrap().len(), 1);
    assert_eq!(v["truncated"], true);
}

#[tokio::test]
async fn attribute_values_returns_storage_labels_with_kinds() {
    let h = handler_with_search_corpus().await;
    let mut body = window_body();
    merge(&mut body, json!({"key": "resource.service.name"}));
    let body = as_mode("attribute_values", body);
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert_eq!(v["key"], "resource.service.name");
    assert_eq!(v["truncated"], false);
    let values: Vec<&str> = v["values"].as_array().unwrap().iter().map(|x| x["value"].as_str().unwrap()).collect();
    assert!(values.contains(&"svc-a") && values.contains(&"svc-b"), "{values:?}");
    assert!(
        v["values"].as_array().unwrap().iter().all(|x| x["kind"] == "str"),
        "service names carry their schema kind: {}",
        v["values"]
    );

    // Builtin `name`: the corpus's span names.
    let mut body = window_body();
    merge(&mut body, json!({"key": "name"}));
    let body = as_mode("attribute_values", body);
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    let values: Vec<&str> = v["values"].as_array().unwrap().iter().map(|x| x["value"].as_str().unwrap()).collect();
    assert!(values.contains(&"span-1"), "{values:?}");

    // Truncation flag exact.
    let mut body = window_body();
    merge(&mut body, json!({"key": "resource.service.name", "max_values": 1}));
    let body = as_mode("attribute_values", body);
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert_eq!(v["values"].as_array().unwrap().len(), 1);
    assert_eq!(v["truncated"], true);
}

#[tokio::test]
async fn enumeration_invalid_requests_are_clean_client_errors() {
    let h = handler_with_search_corpus().await;
    // Shape errors (null selectors, missing `key`) are wire-test
    // territory now; these are the handler's semantic rejections.
    for (body, needle) in [
        (json!({"attributes": {"owner": "bogus"}}), "unknown owner"),
        (json!({"attributes": {"max_keys": 0}}), "zero key/value limit"),
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

// ── The overview mode ───────────────────────────────────────────────

#[tokio::test]
async fn overview_facets_are_opt_in_and_partition_the_population() {
    // Corpus roots are each trace's span-1: services svc-a×3 (A,C,E)
    // and svc-b×2 (B,D); every root is named "span-1".
    let h = handler_with_search_corpus().await;

    let mut body = window_body();
    merge(&mut body, json!({}));
    let body = as_mode("overview", body);
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert!(v.get("top_root_services").is_none(), "absent unless requested");
    assert!(v.get("top_root_operations").is_none());

    let mut body = window_body();
    merge(&mut body, json!({"facets": true}));
    let body = as_mode("overview", body);
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert_eq!(v["totals"]["traces"], 5);
    assert_eq!(
        v["top_root_services"],
        json!({
            "top": [
                {"value": "svc-a", "traces": 3},
                {"value": "svc-b", "traces": 2}
            ],
            "other": 0,
            "unattributed": 0
        })
    );
    assert_eq!(
        v["top_root_operations"],
        json!({
            "top": [{"value": "span-1", "traces": 5}],
            "other": 0,
            "unattributed": 0
        })
    );
    // The wire-side partition identity, asserted structurally too.
    for name in ["top_root_services", "top_root_operations"] {
        let f = &v[name];
        let sum: u64 = f["top"]
            .as_array()
            .unwrap()
            .iter()
            .map(|e| e["traces"].as_u64().unwrap())
            .sum();
        assert_eq!(
            sum + f["other"].as_u64().unwrap() + f["unattributed"].as_u64().unwrap(),
            v["totals"]["traces"].as_u64().unwrap(),
            "{name}"
        );
    }
}

#[tokio::test]
async fn slowest_ranks_the_corpus_by_merged_envelope() {
    // Envelope durations: E 2500ns > B 1500ns > A/C/D 500ns (the tie
    // breaks by ascending trace id). Roots are each trace's span-1.
    let h = handler_with_search_corpus().await;
    let mut body = window_body();
    merge(&mut body, json!({}));
    let body = as_mode("slowest", body);
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert_eq!(v["status"], json!({"complete": true}));
    assert_eq!(ids(&v), ["0e", "0b", "0a", "0c", "0d"]);
    assert_eq!(v["items"], json!({"returned": 5, "max_to_return": 20}));
    let top = &v["traces"][0];
    assert_eq!(top["duration_ns"], 2500);
    assert_eq!(top["root_service"], "svc-a");
    assert_eq!(top["root_name"], "span-1");
    assert_eq!(top["span_count"], 3);
    assert_eq!(top["error_count"], 0);
}

#[tokio::test]
async fn slowest_limit_truncates_and_zero_is_a_client_error() {
    let h = handler_with_search_corpus().await;
    let mut body = window_body();
    merge(&mut body, json!({"limit": 2}));
    let body = as_mode("slowest", body);
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert_eq!(ids(&v), ["0e", "0b"]);
    assert_eq!(v["items"], json!({"returned": 2, "max_to_return": 2}));

    let mut body = window_body();
    merge(&mut body, json!({"limit": 0}));
    let body = as_mode("slowest", body);
    let err = call_on(&h, body).await.expect_err("zero limit");
    assert!(err.to_string().contains("zero limit"), "{err}");

    let mut body = window_body();
    merge(&mut body, json!({"limit": 1001}));
    let body = as_mode("slowest", body);
    let err = call_on(&h, body).await.expect_err("limit beyond max");
    assert!(err.to_string().contains("exceeds the library maximum"), "{err}");
}

#[tokio::test]
async fn overview_grid_matches_the_corpus_distribution() {
    // 100s window → 1s buckets aligned to [T_S, T_S+100). The corpus's
    // 5 TRACES (A=1 span, B=2, C=1, D=1, E=3 — 8 stored spans), every
    // envelope sub-1ms (bin 0), binned at each trace's start second.
    let h = handler_with_search_corpus().await;
    let mut body = window_body();
    merge(&mut body, json!({}));
    let body = as_mode("overview", body);
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert_eq!(v["unit"], "traces");
    assert_eq!(v["status"], json!({"complete": true}));
    assert_eq!(v["totals"], json!({"traces": 5, "spans": 8, "errors": 0}));
    assert_eq!(v["grid"]["bucket_start_s"], T_S);
    assert_eq!(v["grid"]["bucket_width_s"], 1);
    assert_eq!(
        v["grid"]["duration_bins"],
        json!(["<1ms", "1-10ms", "10-100ms", "100ms-1s", "1-10s", ">10s"])
    );
    let cells = v["grid"]["cells"].as_array().unwrap();
    assert_eq!(cells.len(), 100);
    let sum: u64 = cells.iter().flat_map(|r| r.as_array().unwrap()).map(|c| c.as_u64().unwrap()).sum();
    assert_eq!(sum, 5, "cell sums equal totals.traces");
    // A starts at second 10, B at 20; the C/D pair share second 30
    // (two traces, one cell); E is ONE trace at second 40.
    assert_eq!(cells[10][0], 1);
    assert_eq!(cells[20][0], 1);
    assert_eq!(cells[30][0], 2);
    assert_eq!(cells[40][0], 1);
}

#[tokio::test]
async fn overview_counts_error_spans_in_totals() {
    use crate::ledger::rpc::traces::fixtures::otlp_req_err;
    let registries = make_registries();
    install_wal(
        &registries,
        "default",
        1,
        vec![
            otlp_req_svc(0x0A, 2, base_ns(10), "svc"),
            otlp_req_err(0x0B, 2, base_ns(20), "svc"), // span 1 = ERROR
        ],
    )
    .await;
    let h = make_handler_over(registries);
    let mut body = window_body();
    merge(&mut body, json!({}));
    let body = as_mode("overview", body);
    let v = serde_json::to_value(call_on(&h, body).await.unwrap()).unwrap();
    assert_eq!(v["totals"], json!({"traces": 2, "spans": 4, "errors": 1}));
}

#[tokio::test]
async fn overview_invalid_selectors_are_clean_client_errors() {
    let h = handler_with_search_corpus().await;
    // Shape errors are wire-test territory; the inverted window is the
    // semantic rejection the handler owns (the window now rides INSIDE
    // the mode object).
    for (body, needle) in [
        (json!({"overview": {"after": 500, "before": 400}}), "invalid window"),
    ] {
        let err = call_on(&h, body.clone()).await.expect_err("must be a client error");
        let msg = err.to_string();
        assert!(msg.contains(needle), "for {body}: {msg}");
    }
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

    let mut body = window_body();
    body["spans_per_trace"] = json!(0);
    // Default tenant: tenant-a's data is invisible.
    let v = serde_json::to_value(call_on(&h, as_mode("search", body.clone())).await.unwrap()).unwrap();
    assert_eq!(v["items"]["returned"], 0);
    // The owning tenant sees it — `tenant` rides at the TOP level.
    let mut wrapped = as_mode("search", body.clone());
    wrapped["tenant"] = json!("tenant-a");
    let v = serde_json::to_value(call_on(&h, wrapped).await.unwrap()).unwrap();
    assert_eq!(v["items"]["returned"], 1);
    // An unknown tenant is empty, not an error and not a union.
    let mut wrapped = as_mode("search", body);
    wrapped["tenant"] = json!("nope");
    let v = serde_json::to_value(call_on(&h, wrapped).await.unwrap()).unwrap();
    assert_eq!(v["items"]["returned"], 0);

    // Tenant routes identically through every data mode (the four
    // TenantId::resolve_query sites): trace, overview, and one
    // enumeration path each see tenant-a's data only when scoped.
    let mut values_body = window_body();
    merge(&mut values_body, json!({"key": "resource.service.name"}));
    for mut wrapped in [
        json!({"trace": {"id": "0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a"}}),
        as_mode("overview", window_body()),
        as_mode("attribute_values", values_body),
    ] {
        let unscoped = serde_json::to_value(call_on(&h, wrapped.clone()).await.unwrap()).unwrap();
        wrapped["tenant"] = json!("tenant-a");
        let scoped = serde_json::to_value(call_on(&h, wrapped.clone()).await.unwrap()).unwrap();
        let count = |v: &serde_json::Value| -> usize {
            if let Some(items) = v.get("items").and_then(|i| i.get("returned")) {
                items.as_u64().unwrap() as usize
            } else if let Some(values) = v.get("values") {
                values.as_array().unwrap().len()
            } else {
                v["totals"]["traces"].as_u64().unwrap() as usize
            }
        };
        assert_eq!(count(&unscoped), 0, "unscoped sees nothing: {wrapped}");
        assert!(count(&scoped) > 0, "tenant-a sees its data: {wrapped}");
    }
}

// ── The transport status boundary ───────────────────────────────────
//
// Shape errors fail request DESERIALIZATION and must surface as
// transport 400s through the bridge; the handler's semantic
// validation keeps its (pre-existing) 500 mapping. These cross
// `HandlerAdapter::handle_raw` — the same path the live bridge runs.

async fn raw_call(payload: Option<&[u8]>) -> (u32, String) {
    use bridge::function::{FunctionContext, HandlerAdapter, RawFunctionHandler};
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
    (res.status, String::from_utf8_lossy(&res.payload).into_owned())
}

#[tokio::test]
async fn shape_errors_are_transport_400s() {
    for (payload, needle) in [
        (&br#"{}"#[..], "missing mode selector"),
        (br#"{"bogus": 1}"#, "unknown field"),
        (br#"{"search": {}, "after": 1}"#, "unknown field"),
        (br#"{"trace": {}, "overview": {}}"#, "conflicting mode selectors"),
        (br#"{"info": true}"#, "invalid info selector"),
        (br#"{"trace": null}"#, "invalid trace selector"),
        (br#"{"overview": []}"#, "expected an object"),
        (br#"[]"#, "otel-traces request object"),
        (br#"{"search": {}, "tenant": "a", "tenant": "b"}"#, "duplicate field"),
    ] {
        let (status, body) = raw_call(Some(payload)).await;
        assert_eq!(status, 400, "for {}: {body}", String::from_utf8_lossy(payload));
        assert!(body.contains(needle), "for {}: {body}", String::from_utf8_lossy(payload));
    }
}

#[tokio::test]
async fn semantic_errors_keep_the_handler_status() {
    // The handler's own validation (zero limit, one-sided trace
    // bounds, bad trace id) rides the pre-existing handler-error
    // mapping — frozen behavior, deliberately not reclassified here.
    for payload in [
        &br#"{"search": {"limit": 0}}"#[..],
        br#"{"trace": {"id": "11111111111111111111111111111111", "after": 100}}"#,
        br#"{"trace": {"id": "xyz"}}"#,
    ] {
        let (status, body) = raw_call(Some(payload)).await;
        assert_eq!(status, 500, "for {}: {body}", String::from_utf8_lossy(payload));
        assert!(
            body.contains("invalid otel-traces request"),
            "for {}: {body}",
            String::from_utf8_lossy(payload)
        );
    }
}

#[tokio::test]
async fn the_three_400_payload_bodies_are_distinct() {
    // Absent payload: the bridge's special empty-payload body (a
    // misnomer now — the cause is the missing mode — but the bridge is
    // frozen and this pin keeps anyone from "fixing" it here).
    let (status, body) = raw_call(None).await;
    assert_eq!(status, 400);
    assert!(body.contains("Request payload is empty"), "{body}");
    // Present `{}`: the ordinary missing-mode deserialization error.
    let (status, body) = raw_call(Some(b"{}")).await;
    assert_eq!(status, 400);
    assert!(body.contains("missing mode selector"), "{body}");
    // Present zero bytes: the ordinary EOF deserialization error.
    let (status, body) = raw_call(Some(b"")).await;
    assert_eq!(status, 400);
    assert!(body.contains("EOF"), "{body}");
}

#[tokio::test]
async fn every_response_shape_declares_its_mode() {
    let h = handler_with_search_corpus().await;
    for (body, mode) in [
        (json!({"info": {}}), "info"),
        (as_mode("search", window_body()), "search"),
        (as_mode("overview", window_body()), "overview"),
        (as_mode("slowest", window_body()), "slowest"),
        (as_mode("attributes", window_body()), "attributes"),
        (
            {
                let mut inner = window_body();
                merge(&mut inner, json!({"key": "kind"}));
                as_mode("attribute_values", inner)
            },
            "attribute_values",
        ),
        (json!({"trace": {"id": "0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a"}}), "trace"),
    ] {
        let v = serde_json::to_value(call_on(&h, body.clone()).await.unwrap()).unwrap();
        assert_eq!(v["mode"], mode, "for {body}");
    }
}
