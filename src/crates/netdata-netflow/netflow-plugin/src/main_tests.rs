use super::{
    FLOWS_FUNCTION_VERSION, FLOWS_UPDATE_EVERY_SECONDS, FlowsFunctionResponse, NetflowFlowsHandler,
    flows_required_params, ingest, plugin_config, query, tiering,
};
use chrono::Utc;
use etherparse::{SlicedPacket, TransportSlice};
use journal_core::file::Mmap;
use journal_core::repository::File as RepoFile;
use journal_core::{Direction, JournalFile, JournalReader, Location};
use pcap_file::pcap::PcapReader;
use rt::ProgressState;
use std::collections::HashMap;
use std::fs;
use std::net::UdpSocket as StdUdpSocket;
use std::num::NonZeroU64;
use std::path::{Path, PathBuf};
use std::sync::atomic::Ordering;
use std::sync::{Arc, RwLock};
use std::time::{Duration, Instant};
use tempfile::TempDir;
use tokio::net::UdpSocket;
use tokio_util::sync::CancellationToken;

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_ingest_writes_journals_and_query_reads_flows() {
    let (cfg, metrics, _open_tiers, _tier_flow_indexes, _tmp) = ingest_fixture("nfv5.pcap").await;

    assert_tier_has_files(&cfg.journal.raw_tier_dir(), "raw");
    assert_tier_dir_exists(&cfg.journal.minute_1_tier_dir(), "1m");
    assert_tier_dir_exists(&cfg.journal.minute_5_tier_dir(), "5m");
    assert_tier_dir_exists(&cfg.journal.hour_1_tier_dir(), "1h");

    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service");
    let before = (Utc::now().timestamp().max(1) as u32).saturating_add(3600);
    let request = query::FlowsRequest {
        view: query::ViewMode::TableSankey,
        after: Some(1),
        before: Some(before),
        group_by: vec![
            "SRC_ADDR".to_string(),
            "DST_ADDR".to_string(),
            "PROTOCOL".to_string(),
        ],
        top_n: query::TopN::N100,
        ..Default::default()
    };
    let output = query_service
        .query_flows(&request)
        .await
        .expect("query tuple flows");

    assert!(
        !output.flows.is_empty(),
        "expected at least one flow from ingested fixture"
    );
    assert!(
        output.metrics.get("bytes").copied().unwrap_or(0) > 0,
        "expected bytes metric to be positive"
    );
    assert!(
        output.facets.is_some(),
        "expected facets in query output for UI filtering"
    );
    assert!(
        !output.stats.is_empty(),
        "expected stats to remain available for backend debugging"
    );
    assert!(
        metrics.journal_entries_written.load(Ordering::Relaxed) > 0,
        "expected raw journal entries written by ingest service"
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_query_service_timeseries_path_returns_chart_data() {
    let (cfg, _metrics, _open_tiers, _tier_flow_indexes, _tmp) = ingest_fixture("nfv5.pcap").await;
    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service");
    let before = (Utc::now().timestamp().max(1) as u32).saturating_add(3600);
    let after = before.saturating_sub(3600);

    let output = query_service
        .query_flow_metrics(&query::FlowsRequest {
            view: query::ViewMode::TimeSeries,
            after: Some(after),
            before: Some(before),
            group_by: vec!["PROTOCOL".to_string()],
            sort_by: query::SortBy::Bytes,
            top_n: query::TopN::N25,
            ..Default::default()
        })
        .await
        .expect("query timeseries metrics");

    assert_eq!(output.metric, "bytes");
    assert_eq!(output.group_by, vec!["PROTOCOL".to_string()]);
    assert!(
        output.chart["result"]["data"]
            .as_array()
            .map(|rows| !rows.is_empty())
            .unwrap_or(false),
        "expected timeseries chart rows from query service"
    );
}

#[test]
fn default_group_by_required_param_preserves_selected_field_order() {
    let request = query::FlowsRequest::default();
    let params = flows_required_params(
        request.normalized_view(),
        &request.normalized_group_by(),
        request.normalized_sort_by(),
        request.normalized_top_n(),
    );

    let group_by_param = params
        .iter()
        .find(|param| param.id == "group_by")
        .expect("group_by required param");

    let selected_fields: Vec<&str> = group_by_param
        .options
        .iter()
        .filter(|option| option.default_selected)
        .map(|option| option.id.as_str())
        .collect();

    assert_eq!(
        selected_fields,
        vec!["SRC_AS_NAME", "PROTOCOL", "DST_AS_NAME"]
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_flows_function_returns_expected_response_sections() {
    let (cfg, metrics, _open_tiers, _tier_flow_indexes, _tmp) = ingest_fixture("nfv5.pcap").await;
    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service");
    let handler = NetflowFlowsHandler::new(Arc::clone(&metrics), Arc::new(query_service));
    let before = (Utc::now().timestamp().max(1) as u32).saturating_add(3600);

    let response = handler
        .handle_request(query::FlowsRequest {
            view: query::ViewMode::TableSankey,
            after: Some(1),
            before: Some(before),
            group_by: vec![
                "SRC_ADDR".to_string(),
                "DST_ADDR".to_string(),
                "PROTOCOL".to_string(),
            ],
            top_n: query::TopN::N100,
            ..Default::default()
        })
        .await
        .expect("flows function call");
    let response = match response {
        FlowsFunctionResponse::Table(response) => response,
        FlowsFunctionResponse::Metrics(_) => panic!("expected table response"),
        FlowsFunctionResponse::Autocomplete(_) => panic!("expected table response"),
    };

    assert_eq!(response.status, 200);
    assert_eq!(response.version, FLOWS_FUNCTION_VERSION);
    assert_eq!(response.update_every, FLOWS_UPDATE_EVERY_SECONDS);
    assert_eq!(response.response_type, "flows");
    assert_eq!(response.data.view, "table-sankey");
    assert_eq!(
        response
            .data
            .columns
            .as_object()
            .map(|columns| columns.len())
            .unwrap_or_default(),
        5,
        "expected grouped table columns to include only group_by fields plus bytes and packets"
    );
    assert!(response.data.columns.get("timestamp").is_none());
    assert!(response.data.columns.get("durationSec").is_none());
    assert!(response.data.columns.get("exporterIp").is_none());
    assert!(response.data.columns.get("exporterName").is_none());
    assert!(response.data.columns.get("flowVersion").is_none());
    assert!(response.data.columns.get("samplingRate").is_none());
    assert!(
        !response.data.flows.is_empty(),
        "expected non-empty flows data section"
    );
    let first = response.data.flows.first().expect("first grouped flow");
    assert!(first.get("timestamp").is_none());
    assert!(first.get("duration_sec").is_none());
    assert!(first.get("exporter").is_none());
    assert!(
        response.data.facets.is_some(),
        "expected facets section in flows response"
    );
    assert!(
        !response.data.metrics.is_empty(),
        "expected top-level metrics to remain in table-family response"
    );
    assert!(
        response
            .required_params
            .iter()
            .any(|param| param.id == "view"),
        "expected required 'view' parameter declaration"
    );
    assert!(
        response
            .required_params
            .iter()
            .any(|param| param.id == "group_by"),
        "expected required 'group_by' parameter declaration"
    );
    let group_by_param = response
        .required_params
        .iter()
        .find(|param| param.id == "group_by")
        .expect("group_by required param");
    assert_eq!(group_by_param.kind, "multiselect");
    assert!(
        group_by_param
            .options
            .iter()
            .any(|option| option.id == "SRC_AS"),
        "expected SRC_AS group_by option to be available"
    );
    assert!(
        group_by_param
            .options
            .iter()
            .any(|option| option.id == "SRC_AS_NAME"),
        "expected SRC_AS_NAME group_by option to be available"
    );
    assert!(
        group_by_param
            .options
            .iter()
            .any(|option| option.id == "SRC_ADDR" && option.default_selected),
        "expected current request group_by selection to be reflected"
    );
    assert!(
        response
            .required_params
            .iter()
            .any(|param| param.id == "sort_by"),
        "expected required 'sort_by' parameter declaration"
    );
    assert!(
        response
            .required_params
            .iter()
            .any(|param| param.id == "top_n"),
        "expected required 'top_n' parameter declaration"
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_flows_function_marks_progress_complete_with_execution_context() {
    let (cfg, metrics, _open_tiers, _tier_flow_indexes, _tmp) = ingest_fixture("nfv5.pcap").await;
    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service");
    let handler = NetflowFlowsHandler::new(Arc::clone(&metrics), Arc::new(query_service));
    let before = (Utc::now().timestamp().max(1) as u32).saturating_add(3600);
    let progress = ProgressState::default();
    let execution = query::QueryExecutionContext::new(progress.clone(), CancellationToken::new());

    let response = handler
        .handle_request_with_execution(
            Some(execution),
            query::FlowsRequest {
                view: query::ViewMode::TableSankey,
                after: Some(1),
                before: Some(before),
                group_by: vec![
                    "SRC_ADDR".to_string(),
                    "DST_ADDR".to_string(),
                    "PROTOCOL".to_string(),
                ],
                top_n: query::TopN::N100,
                ..Default::default()
            },
        )
        .await
        .expect("flows function call with execution");

    match response {
        FlowsFunctionResponse::Table(_) => {}
        FlowsFunctionResponse::Metrics(_) => panic!("expected table response"),
        FlowsFunctionResponse::Autocomplete(_) => panic!("expected table response"),
    }

    let (done, total) = progress.snapshot();
    assert!(total > 0, "expected progress total to be initialized");
    assert_eq!(done, total, "expected completed progress after response");
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_flows_function_marks_progress_complete_for_empty_projected_query() {
    let tmp = tempfile::tempdir().expect("create temp dir");
    let journal_root = tmp.path().join("flows");
    fs::create_dir_all(journal_root.join("raw")).expect("create raw dir");
    fs::create_dir_all(journal_root.join("1m")).expect("create 1m dir");
    fs::create_dir_all(journal_root.join("5m")).expect("create 5m dir");
    fs::create_dir_all(journal_root.join("1h")).expect("create 1h dir");

    let mut cfg = plugin_config::PluginConfig::default();
    cfg.journal.journal_dir = journal_root.to_string_lossy().to_string();

    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service");
    let handler = NetflowFlowsHandler::new(
        Arc::new(ingest::IngestMetrics::default()),
        Arc::new(query_service),
    );
    let before = (Utc::now().timestamp().max(1) as u32).saturating_add(3600);
    let progress = ProgressState::default();
    let execution = query::QueryExecutionContext::new(progress.clone(), CancellationToken::new());

    let response = handler
        .handle_request_with_execution(
            Some(execution),
            query::FlowsRequest {
                view: query::ViewMode::TableSankey,
                after: Some(1),
                before: Some(before),
                group_by: vec![
                    "SRC_ADDR".to_string(),
                    "DST_ADDR".to_string(),
                    "PROTOCOL".to_string(),
                ],
                top_n: query::TopN::N100,
                ..Default::default()
            },
        )
        .await
        .expect("empty projected flows function call with execution");

    match response {
        FlowsFunctionResponse::Table(response) => {
            assert!(
                response.data.flows.is_empty(),
                "expected empty flows data for empty journals"
            );
        }
        FlowsFunctionResponse::Metrics(_) => panic!("expected table response"),
        FlowsFunctionResponse::Autocomplete(_) => panic!("expected table response"),
    }

    let (done, total) = progress.snapshot();
    assert!(total > 0, "expected progress total to be initialized");
    assert_eq!(
        done, total,
        "expected completed progress after empty response"
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_flows_function_honors_cancelled_execution_context() {
    let (cfg, metrics, _open_tiers, _tier_flow_indexes, _tmp) = ingest_fixture("nfv5.pcap").await;
    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service");
    let handler = NetflowFlowsHandler::new(Arc::clone(&metrics), Arc::new(query_service));
    let before = (Utc::now().timestamp().max(1) as u32).saturating_add(3600);
    let cancellation = CancellationToken::new();
    cancellation.cancel();
    let execution = query::QueryExecutionContext::new(ProgressState::default(), cancellation);

    let err = handler
        .handle_request_with_execution(
            Some(execution),
            query::FlowsRequest {
                view: query::ViewMode::TableSankey,
                after: Some(1),
                before: Some(before),
                group_by: vec![
                    "SRC_ADDR".to_string(),
                    "DST_ADDR".to_string(),
                    "PROTOCOL".to_string(),
                ],
                top_n: query::TopN::N100,
                ..Default::default()
            },
        )
        .await
        .expect_err("cancelled execution should fail");

    let message = err.to_string();
    assert!(
        message.contains("cancelled"),
        "expected cancellation error, got: {message}"
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_flows_function_supports_autocomplete_mode() {
    let (cfg, metrics, _open_tiers, _tier_flow_indexes, _tmp) = ingest_fixture("nfv5.pcap").await;
    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service");
    let handler = NetflowFlowsHandler::new(Arc::clone(&metrics), Arc::new(query_service));

    let response = handler
        .handle_request(query::FlowsRequest {
            mode: query::RequestMode::Autocomplete,
            field: Some("PROTOCOL".to_string()),
            term: "6".to_string(),
            ..Default::default()
        })
        .await
        .expect("autocomplete function call");

    let response = match response {
        FlowsFunctionResponse::Autocomplete(response) => response,
        FlowsFunctionResponse::Table(_) => panic!("expected autocomplete response"),
        FlowsFunctionResponse::Metrics(_) => panic!("expected autocomplete response"),
    };

    assert_eq!(response.status, 200);
    assert_eq!(response.version, FLOWS_FUNCTION_VERSION);
    assert_eq!(response.response_type, "flows");
    assert_eq!(response.data.mode, "autocomplete");
    assert_eq!(response.data.field, "PROTOCOL");
    assert_eq!(response.data.term, "6");
    assert!(
        response
            .data
            .values
            .iter()
            .any(|entry| entry["value"] == "6"),
        "expected autocomplete values to contain protocol 6"
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_flows_metrics_function_returns_top_n_chart_with_on_disk_tier_fallback() {
    let (cfg, metrics, _open_tiers, _tier_flow_indexes, _tmp) = ingest_fixture("nfv5.pcap").await;
    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service");
    let handler = NetflowFlowsHandler::new(Arc::clone(&metrics), Arc::new(query_service));
    let before = (Utc::now().timestamp().max(1) as u32).saturating_add(3600);
    let after = before.saturating_sub(3600);
    let materialized_tier_files = tier_file_count(&cfg.journal.hour_1_tier_dir())
        + tier_file_count(&cfg.journal.minute_5_tier_dir())
        + tier_file_count(&cfg.journal.minute_1_tier_dir());

    let response = handler
        .handle_request(query::FlowsRequest {
            view: query::ViewMode::TimeSeries,
            after: Some(after),
            before: Some(before),
            group_by: vec!["PROTOCOL".to_string()],
            sort_by: query::SortBy::Bytes,
            top_n: query::TopN::N50,
            ..Default::default()
        })
        .await
        .expect("flow metrics function call");
    let response = match response {
        FlowsFunctionResponse::Metrics(response) => response,
        FlowsFunctionResponse::Table(_) => panic!("expected metrics response"),
        FlowsFunctionResponse::Autocomplete(_) => panic!("expected metrics response"),
    };

    assert_eq!(response.status, 200);
    assert_eq!(response.version, FLOWS_FUNCTION_VERSION);
    assert_eq!(response.update_every, FLOWS_UPDATE_EVERY_SECONDS);
    assert_eq!(response.response_type, "flows");
    assert_eq!(response.data.view, "timeseries");
    assert_eq!(response.data.metric, "bytes");
    assert_eq!(response.data.group_by, vec!["PROTOCOL".to_string()]);
    assert_eq!(response.data.columns["PROTOCOL"]["name"], "Protocol");
    assert_eq!(response.data.chart["view"]["units"], "bytes/s");
    assert_eq!(
        response.data.stats.get("query_tier").copied().unwrap_or(0) > 0,
        materialized_tier_files > 0,
        "expected timeseries query tier to reflect on-disk materialized-tier availability"
    );
    assert_eq!(
        response.data.stats.get("query_bucket_seconds").copied(),
        Some(60)
    );
    assert!(
        response.data.chart["view"]["dimensions"]["ids"]
            .as_array()
            .map(|dims| !dims.is_empty() && dims.len() <= 50)
            .unwrap_or(false),
        "expected Top-N chart dimensions limited by request"
    );
    assert!(
        response.data.chart["result"]["data"]
            .as_array()
            .map(|rows| !rows.is_empty())
            .unwrap_or(false),
        "expected chart datapoints in metrics response"
    );
    assert!(
        response
            .required_params
            .iter()
            .any(|param| param.id == "sort_by"),
        "expected required 'sort_by' parameter declaration"
    );
    assert!(
        response
            .required_params
            .iter()
            .any(|param| param.id == "top_n"),
        "expected required 'top_n' parameter declaration"
    );
    assert_eq!(
        response
            .required_params
            .iter()
            .find(|param| param.id == "group_by")
            .and_then(|param| param.options.iter().find(|option| option.id == "PROTOCOL"))
            .map(|option| option.name.as_str()),
        Some("Protocol")
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_aggregated_safe_group_by_falls_back_to_on_disk_lower_tiers() {
    let (cfg, metrics, _open_tiers, _tier_flow_indexes, _tmp) =
        ingest_fixture_with_timestamp_source("nfv5.pcap", plugin_config::TimestampSource::Input)
            .await;
    let materialized_tier_files = tier_file_count(&cfg.journal.hour_1_tier_dir())
        + tier_file_count(&cfg.journal.minute_5_tier_dir())
        + tier_file_count(&cfg.journal.minute_1_tier_dir());

    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service");
    let before = (Utc::now().timestamp().max(1) as u32).saturating_add(3600);
    let request = query::FlowsRequest {
        view: query::ViewMode::TableSankey,
        after: Some(1),
        before: Some(before),
        group_by: vec![
            "SRC_AS_NAME".to_string(),
            "DST_AS_NAME".to_string(),
            "PROTOCOL".to_string(),
        ],
        top_n: query::TopN::N100,
        ..Default::default()
    };
    let output = query_service
        .query_flows(&request)
        .await
        .expect("query aggregated-safe flows");
    assert!(
        !output.flows.is_empty(),
        "expected non-empty aggregated-safe flows from on-disk tiers"
    );
    assert_eq!(
        output.stats.get("query_tier").copied().unwrap_or(0) > 0,
        materialized_tier_files > 0,
        "expected grouped query tier to reflect on-disk materialized-tier availability"
    );

    let handler = NetflowFlowsHandler::new(Arc::clone(&metrics), Arc::new(query_service));
    let response = handler
        .handle_request(request)
        .await
        .expect("flows function call for aggregated-safe view");
    let response = match response {
        FlowsFunctionResponse::Table(response) => response,
        FlowsFunctionResponse::Metrics(_) => panic!("expected table response"),
        FlowsFunctionResponse::Autocomplete(_) => panic!("expected table response"),
    };
    assert!(
        !response.data.flows.is_empty(),
        "expected non-empty function flows for aggregated-safe view"
    );
    assert_eq!(
        response.data.group_by,
        vec![
            "SRC_AS_NAME".to_string(),
            "DST_AS_NAME".to_string(),
            "PROTOCOL".to_string()
        ]
    );
    assert_eq!(
        response.data.columns["SRC_AS_NAME"]["name"],
        "Source AS Name"
    );
    assert_eq!(response.data.columns["PROTOCOL"]["name"], "Protocol");
    assert_eq!(
        response.data.stats.get("query_tier").copied().unwrap_or(0) > 0,
        materialized_tier_files > 0,
        "expected function response query tier to reflect on-disk materialized-tier availability"
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_country_map_reuses_tuple_table_shape_with_country_keys() {
    let (cfg, metrics, _open_tiers, _tier_flow_indexes, _tmp) = ingest_fixture("nfv5.pcap").await;
    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service");
    let handler = NetflowFlowsHandler::new(Arc::clone(&metrics), Arc::new(query_service));
    let before = (Utc::now().timestamp().max(1) as u32).saturating_add(3600);

    let response = handler
        .handle_request(query::FlowsRequest {
            view: query::ViewMode::CountryMap,
            after: Some(1),
            before: Some(before),
            group_by: vec!["SRC_ADDR".to_string(), "DST_ADDR".to_string()],
            top_n: query::TopN::N25,
            ..Default::default()
        })
        .await
        .expect("country-map function call");
    let response = match response {
        FlowsFunctionResponse::Table(response) => response,
        FlowsFunctionResponse::Metrics(_) => panic!("expected table response"),
        FlowsFunctionResponse::Autocomplete(_) => panic!("expected table response"),
    };

    assert_eq!(response.status, 200);
    assert_eq!(response.response_type, "flows");
    assert_eq!(response.data.view, "country-map");
    assert_eq!(
        response.data.group_by,
        vec!["SRC_COUNTRY".to_string(), "DST_COUNTRY".to_string()]
    );
    assert!(
        !response.data.flows.is_empty(),
        "expected non-empty country-map tuple rows"
    );
    assert_eq!(
        response
            .data
            .columns
            .as_object()
            .map(|columns| columns.len())
            .unwrap_or_default(),
        4,
        "expected country-map columns to include only country keys plus bytes and packets"
    );
    assert!(response.data.columns.get("timestamp").is_none());
    assert!(response.data.columns.get("durationSec").is_none());
    assert!(response.data.columns.get("exporterIp").is_none());
    assert!(response.data.columns.get("exporterName").is_none());
    assert!(response.data.columns.get("flowVersion").is_none());
    assert!(response.data.columns.get("samplingRate").is_none());

    let first = response.data.flows.first().expect("first flow row");
    assert!(
        first["key"].get("SRC_COUNTRY").is_some(),
        "expected country-map rows to expose SRC_COUNTRY"
    );
    assert!(
        first["key"].get("DST_COUNTRY").is_some(),
        "expected country-map rows to expose DST_COUNTRY"
    );
    assert!(first.get("timestamp").is_none());
    assert!(first.get("duration_sec").is_none());
    assert!(first.get("exporter").is_none());
    assert!(
        !response
            .required_params
            .iter()
            .any(|param| param.id == "group_by"),
        "expected country-map response to hide group_by controls"
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_state_map_reuses_tuple_table_shape_with_state_keys() {
    let (cfg, metrics, _open_tiers, _tier_flow_indexes, _tmp) = ingest_fixture("nfv5.pcap").await;
    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service");
    let handler = NetflowFlowsHandler::new(Arc::clone(&metrics), Arc::new(query_service));
    let before = (Utc::now().timestamp().max(1) as u32).saturating_add(3600);

    let response = handler
        .handle_request(query::FlowsRequest {
            view: query::ViewMode::StateMap,
            after: Some(1),
            before: Some(before),
            group_by: vec!["SRC_ADDR".to_string(), "DST_ADDR".to_string()],
            top_n: query::TopN::N25,
            ..Default::default()
        })
        .await
        .expect("state-map function call");
    let response = match response {
        FlowsFunctionResponse::Table(response) => response,
        FlowsFunctionResponse::Metrics(_) => panic!("expected table response"),
        FlowsFunctionResponse::Autocomplete(_) => panic!("expected table response"),
    };

    assert_eq!(response.status, 200);
    assert_eq!(response.response_type, "flows");
    assert_eq!(response.data.view, "state-map");
    assert_eq!(
        response.data.group_by,
        vec![
            "SRC_COUNTRY".to_string(),
            "SRC_GEO_STATE".to_string(),
            "DST_COUNTRY".to_string(),
            "DST_GEO_STATE".to_string()
        ]
    );
    assert!(
        !response.data.flows.is_empty(),
        "expected non-empty state-map rows"
    );
    assert!(
        !response
            .required_params
            .iter()
            .any(|param| param.id == "group_by"),
        "expected state-map response to hide group_by controls"
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_city_map_reuses_tuple_table_shape_with_city_and_coordinate_keys() {
    let (cfg, metrics, _open_tiers, _tier_flow_indexes, _tmp) = ingest_fixture("nfv5.pcap").await;
    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service");
    let handler = NetflowFlowsHandler::new(Arc::clone(&metrics), Arc::new(query_service));
    let before = (Utc::now().timestamp().max(1) as u32).saturating_add(3600);

    let response = handler
        .handle_request(query::FlowsRequest {
            view: query::ViewMode::CityMap,
            after: Some(1),
            before: Some(before),
            group_by: vec!["SRC_ADDR".to_string(), "DST_ADDR".to_string()],
            top_n: query::TopN::N25,
            ..Default::default()
        })
        .await
        .expect("city-map function call");
    let response = match response {
        FlowsFunctionResponse::Table(response) => response,
        FlowsFunctionResponse::Metrics(_) => panic!("expected table response"),
        FlowsFunctionResponse::Autocomplete(_) => panic!("expected table response"),
    };

    assert_eq!(response.status, 200);
    assert_eq!(response.response_type, "flows");
    assert_eq!(response.data.view, "city-map");
    assert_eq!(
        response.data.group_by,
        vec![
            "SRC_COUNTRY".to_string(),
            "SRC_GEO_STATE".to_string(),
            "SRC_GEO_CITY".to_string(),
            "SRC_GEO_LATITUDE".to_string(),
            "SRC_GEO_LONGITUDE".to_string(),
            "DST_COUNTRY".to_string(),
            "DST_GEO_STATE".to_string(),
            "DST_GEO_CITY".to_string(),
            "DST_GEO_LATITUDE".to_string(),
            "DST_GEO_LONGITUDE".to_string(),
        ]
    );
    assert_eq!(
        response.data.columns["SRC_GEO_LATITUDE"]["visible"],
        serde_json::json!(false)
    );
    assert_eq!(
        response.data.columns["SRC_GEO_LONGITUDE"]["visible"],
        serde_json::json!(false)
    );
    assert_eq!(
        response.data.columns["DST_GEO_LATITUDE"]["visible"],
        serde_json::json!(false)
    );
    assert_eq!(
        response.data.columns["DST_GEO_LONGITUDE"]["visible"],
        serde_json::json!(false)
    );
    assert!(
        response.data.columns["SRC_GEO_CITY"]
            .get("visible")
            .is_none()
    );
    assert!(
        response.data.columns["DST_GEO_CITY"]
            .get("visible")
            .is_none()
    );
    assert!(
        !response.data.flows.is_empty(),
        "expected non-empty city-map rows"
    );
    assert!(
        response
            .data
            .stats
            .get("query_forced_raw_tier")
            .copied()
            .unwrap_or_default()
            > 0,
        "expected city-map query to force raw tier"
    );
    assert!(
        !response
            .required_params
            .iter()
            .any(|param| param.id == "group_by"),
        "expected city-map response to hide group_by controls"
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_selection_filter_uses_streaming_reader_path() {
    let (cfg, _metrics, _open_tiers, _tier_flow_indexes, _tmp) = ingest_fixture("nfv5.pcap").await;
    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service");
    let before = (Utc::now().timestamp().max(1) as u32).saturating_add(3600);

    let request_base = query::FlowsRequest {
        view: query::ViewMode::TableSankey,
        after: Some(1),
        before: Some(before),
        group_by: vec![
            "SRC_ADDR".to_string(),
            "DST_ADDR".to_string(),
            "PROTOCOL".to_string(),
        ],
        top_n: query::TopN::N100,
        ..Default::default()
    };
    let request_match = query::FlowsRequest {
        selections: HashMap::from([("FLOW_VERSION".to_string(), vec!["v5".to_string()])]),
        ..request_base
    };
    let matched = query_service
        .query_flows(&request_match)
        .await
        .expect("query with matching FLOW_VERSION selection");
    assert_eq!(
        matched.stats.get("query_reader_path").copied().unwrap_or(0),
        1,
        "expected query to use streaming reader path"
    );
    assert!(
        matched
            .stats
            .get("query_matched_entries")
            .copied()
            .unwrap_or(0)
            > 0,
        "expected at least one matched entry for FLOW_VERSION=v5"
    );

    let request_multi = query::FlowsRequest {
        selections: HashMap::from([(
            "PROTOCOL".to_string(),
            vec!["6".to_string(), "17".to_string()],
        )]),
        ..Default::default()
    };
    let request_multi = query::FlowsRequest {
        view: query::ViewMode::TableSankey,
        after: Some(1),
        before: Some(before),
        group_by: vec![
            "SRC_ADDR".to_string(),
            "DST_ADDR".to_string(),
            "PROTOCOL".to_string(),
        ],
        top_n: query::TopN::N100,
        ..request_multi
    };
    let multi = query_service
        .query_flows(&request_multi)
        .await
        .expect("query with multi-value PROTOCOL selection");
    assert_eq!(
        multi.stats.get("query_reader_path").copied().unwrap_or(0),
        1,
        "expected multi-value selection query to use streaming reader path"
    );
    assert!(
        multi
            .stats
            .get("query_matched_entries")
            .copied()
            .unwrap_or(0)
            > 0,
        "expected at least one matched entry for PROTOCOL in [6,17]"
    );
    assert!(
        multi.flows.iter().all(|row| {
            row["key"]
                .get("PROTOCOL")
                .and_then(|value| value.as_str())
                .map(|value| value == "6" || value == "17")
                .unwrap_or(false)
        }),
        "expected every returned row to respect the multi-value protocol filter"
    );

    let request_miss = query::FlowsRequest {
        selections: HashMap::from([("FLOW_VERSION".to_string(), vec!["999".to_string()])]),
        view: query::ViewMode::TableSankey,
        after: Some(1),
        before: Some(before),
        group_by: vec![
            "SRC_ADDR".to_string(),
            "DST_ADDR".to_string(),
            "PROTOCOL".to_string(),
        ],
        top_n: query::TopN::N100,
        ..Default::default()
    };
    let missed = query_service
        .query_flows(&request_miss)
        .await
        .expect("query with non-matching FLOW_VERSION selection");
    assert_eq!(
        missed
            .stats
            .get("query_matched_entries")
            .copied()
            .unwrap_or(0),
        0,
        "expected no matched entries for FLOW_VERSION=999"
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_post_style_nested_required_controls_still_filter_correctly() {
    let (cfg, _metrics, _open_tiers, _tier_flow_indexes, _tmp) = ingest_fixture("nfv5.pcap").await;
    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service");
    let before = (Utc::now().timestamp().max(1) as u32).saturating_add(3600);
    let payload = format!(
        r#"{{
                "after":1,
                "before":{before},
                "query":"",
                "selections":{{
                    "view":"table-sankey",
                    "group_by":["SRC_ADDR","DST_ADDR","PROTOCOL"],
                    "sort_by":"bytes",
                    "top_n":"100",
                    "FLOW_VERSION":["v5"]
                }},
                "timeout":120000,
                "last":200
            }}"#
    );
    let request =
        serde_json::from_str::<query::FlowsRequest>(&payload).expect("request should deserialize");

    let output = query_service
        .query_flows(&request)
        .await
        .expect("query should honor nested required controls");

    assert_eq!(
        output.stats.get("query_reader_path").copied().unwrap_or(0),
        1,
        "expected query to use streaming reader path"
    );
    assert!(
        output
            .stats
            .get("query_matched_entries")
            .copied()
            .unwrap_or(0)
            > 0,
        "expected nested required controls not to suppress real filtering"
    );
    assert!(
        !output.flows.is_empty(),
        "expected rows after hoisting nested required controls out of selections"
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
#[ignore]
async fn profile_live_day_query_against_local_journals() {
    let journal_dir = PathBuf::from("/var/cache/netdata/flows");
    assert!(
        journal_dir.exists(),
        "expected live netflow journal directory at {}",
        journal_dir.display()
    );

    let mut cfg = plugin_config::PluginConfig::default();
    cfg.journal.journal_dir = journal_dir.to_string_lossy().to_string();

    let _open_tiers = Arc::new(RwLock::new(tiering::OpenTierState::default()));
    let _tier_flow_indexes = Arc::new(RwLock::new(tiering::TierFlowIndexStore::default()));
    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service for live journals");

    let before = Utc::now().timestamp().max(1) as u32;
    let after = before.saturating_sub(24 * 60 * 60);
    let request = query::FlowsRequest {
        view: query::ViewMode::TableSankey,
        after: Some(after),
        before: Some(before),
        group_by: vec![
            "PROTOCOL".to_string(),
            "SRC_AS_NAME".to_string(),
            "DST_AS_NAME".to_string(),
        ],
        top_n: query::TopN::N25,
        ..Default::default()
    };

    let start = Instant::now();
    let output = query_service
        .query_flows(&request)
        .await
        .expect("query live journals");
    let elapsed = start.elapsed();

    eprintln!();
    eprintln!("=== Live Day Query Profile Harness ===");
    eprintln!("journal_dir:             {}", journal_dir.display());
    eprintln!("after:                   {}", after);
    eprintln!("before:                  {}", before);
    eprintln!("group_by:                {:?}", request.group_by);
    eprintln!(
        "elapsed_ms:              {:.2}",
        elapsed.as_secs_f64() * 1_000.0
    );
    eprintln!("flow_rows:               {}", output.flows.len());
    eprintln!(
        "metric_bytes:            {}",
        output.metrics.get("bytes").copied().unwrap_or(0)
    );
    eprintln!(
        "metric_packets:          {}",
        output.metrics.get("packets").copied().unwrap_or(0)
    );
    eprintln!("stats:                   {:?}", output.stats);
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
#[ignore]
async fn profile_fixed_raw_direct_journal_core_against_local_journals() {
    const FIXED_FILES: [&str; 4] = [
        "/tmp/netdata-raw-snapshot-1774481723/system@92ecfa81f20440b9a0762a3a4656e37a-00000000045ab310-00064da65a07dfc3.journal",
        "/tmp/netdata-raw-snapshot-1774481723/system@92ecfa81f20440b9a0762a3a4656e37a-00000000045d8ec3-00064da8006c73a9.journal",
        "/tmp/netdata-raw-snapshot-1774481723/system@92ecfa81f20440b9a0762a3a4656e37a-0000000004606cfe-00064da9f3edc98c.journal",
        "/tmp/netdata-raw-snapshot-1774481723/system@92ecfa81f20440b9a0762a3a4656e37a-0000000004634242-00064dabd29b631e.journal",
    ];

    let started = Instant::now();
    let mut rows_read = 0usize;
    let mut fields_read = 0usize;
    let mut files_opened = 0usize;
    let mut data_offsets = Vec::<NonZeroU64>::new();

    for src in FIXED_FILES {
        let src_path = Path::new(src);
        let repo_file = RepoFile::from_path(src_path).expect("parse journal repository metadata");

        let journal = JournalFile::<Mmap>::open(&repo_file, 8 * 1024 * 1024).expect("open journal");
        files_opened += 1;

        let mut reader = JournalReader::default();
        reader.set_location(Location::Head);

        while reader
            .step(&journal, Direction::Forward)
            .expect("step journal reader")
        {
            rows_read += 1;
            data_offsets.clear();
            reader
                .entry_data_offsets(&journal, &mut data_offsets)
                .expect("enumerate entry data offsets");
            for data_offset in data_offsets.iter().copied() {
                let _data_guard = journal.data_ref(data_offset).expect("read payload object");
                fields_read += 1;
            }
        }
    }

    let elapsed_usec = started.elapsed().as_micros();
    eprintln!();
    eprintln!("=== Fixed Raw Direct Journal-Core Harness ===");
    eprintln!("files_opened:            {}", files_opened);
    eprintln!("rows_read:               {}", rows_read);
    eprintln!("fields_read:             {}", fields_read);
    eprintln!(
        "fields_per_row:          {:.4}",
        fields_read as f64 / rows_read as f64
    );
    eprintln!("time_usec:               {}", elapsed_usec);
    eprintln!(
        "usec_per_row:            {:.6}",
        elapsed_usec as f64 / rows_read as f64
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
#[ignore]
async fn profile_fixed_raw_plugin_scan_only_against_local_journals() {
    const FIXED_FILES: [&str; 4] = [
        "/tmp/netdata-raw-snapshot-1774481723/system@92ecfa81f20440b9a0762a3a4656e37a-00000000045ab310-00064da65a07dfc3.journal",
        "/tmp/netdata-raw-snapshot-1774481723/system@92ecfa81f20440b9a0762a3a4656e37a-00000000045d8ec3-00064da8006c73a9.journal",
        "/tmp/netdata-raw-snapshot-1774481723/system@92ecfa81f20440b9a0762a3a4656e37a-0000000004606cfe-00064da9f3edc98c.journal",
        "/tmp/netdata-raw-snapshot-1774481723/system@92ecfa81f20440b9a0762a3a4656e37a-0000000004634242-00064dabd29b631e.journal",
    ];

    let tmp = tempfile::tempdir().expect("create temp dir");
    let journal_root = tmp.path().join("flows");
    let raw_dir = journal_root.join("raw");
    let minute_1_dir = journal_root.join("1m");
    let minute_5_dir = journal_root.join("5m");
    let hour_1_dir = journal_root.join("1h");
    fs::create_dir_all(&raw_dir).expect("create raw dir");
    fs::create_dir_all(&minute_1_dir).expect("create 1m dir");
    fs::create_dir_all(&minute_5_dir).expect("create 5m dir");
    fs::create_dir_all(&hour_1_dir).expect("create 1h dir");

    for src in FIXED_FILES {
        let src_path = Path::new(src);
        assert!(
            src_path.is_file(),
            "expected journal file {}",
            src_path.display()
        );
        let dst_path = raw_dir.join(src_path.file_name().expect("journal filename"));
        if let Err(err) = fs::hard_link(src_path, &dst_path) {
            if err.raw_os_error() == Some(18) {
                fs::copy(src_path, &dst_path)
                    .unwrap_or_else(|copy_err| panic!("copy {}: {}", src_path.display(), copy_err));
            } else {
                panic!("hard link {}: {}", src_path.display(), err);
            }
        }
    }

    let mut cfg = plugin_config::PluginConfig::default();
    cfg.journal.journal_dir = journal_root.to_string_lossy().to_string();

    let _open_tiers = Arc::new(RwLock::new(tiering::OpenTierState::default()));
    let _tier_flow_indexes = Arc::new(RwLock::new(tiering::TierFlowIndexStore::default()));
    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service for fixed raw journals");

    let before = (Utc::now().timestamp().max(1) as u32).saturating_add(3600);
    let request = query::FlowsRequest {
        view: query::ViewMode::TableSankey,
        after: Some(1),
        before: Some(before),
        group_by: vec![
            "SRC_ADDR".to_string(),
            "DST_ADDR".to_string(),
            "PROTOCOL".to_string(),
        ],
        top_n: query::TopN::N25,
        ..Default::default()
    };

    let result = query_service
        .benchmark_projected_raw_scan_only(&request)
        .expect("scan-only benchmark should succeed");

    eprintln!();
    eprintln!("=== Fixed Raw Plugin Scan-Only Harness ===");
    eprintln!("journal_dir:             {}", journal_root.display());
    eprintln!("files_opened:            {}", result.files_opened);
    eprintln!("rows_read:               {}", result.rows_read);
    eprintln!("fields_read:             {}", result.fields_read);
    eprintln!(
        "fields_per_row:          {:.4}",
        result.fields_read as f64 / result.rows_read as f64
    );
    eprintln!("time_usec:               {}", result.elapsed_usec);
    eprintln!(
        "usec_per_row:            {:.6}",
        result.elapsed_usec as f64 / result.rows_read as f64
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
#[ignore]
async fn profile_fixed_raw_plugin_stage_breakdown_against_local_journals() {
    const FIXED_FILES: [&str; 4] = [
        "/tmp/netdata-raw-snapshot-1774481723/system@92ecfa81f20440b9a0762a3a4656e37a-00000000045ab310-00064da65a07dfc3.journal",
        "/tmp/netdata-raw-snapshot-1774481723/system@92ecfa81f20440b9a0762a3a4656e37a-00000000045d8ec3-00064da8006c73a9.journal",
        "/tmp/netdata-raw-snapshot-1774481723/system@92ecfa81f20440b9a0762a3a4656e37a-0000000004606cfe-00064da9f3edc98c.journal",
        "/tmp/netdata-raw-snapshot-1774481723/system@92ecfa81f20440b9a0762a3a4656e37a-0000000004634242-00064dabd29b631e.journal",
    ];

    let tmp = tempfile::tempdir().expect("create temp dir");
    let journal_root = tmp.path().join("flows");
    let raw_dir = journal_root.join("raw");
    let minute_1_dir = journal_root.join("1m");
    let minute_5_dir = journal_root.join("5m");
    let hour_1_dir = journal_root.join("1h");
    fs::create_dir_all(&raw_dir).expect("create raw dir");
    fs::create_dir_all(&minute_1_dir).expect("create 1m dir");
    fs::create_dir_all(&minute_5_dir).expect("create 5m dir");
    fs::create_dir_all(&hour_1_dir).expect("create 1h dir");

    for src in FIXED_FILES {
        let src_path = Path::new(src);
        assert!(
            src_path.is_file(),
            "expected journal file {}",
            src_path.display()
        );
        let dst_path = raw_dir.join(src_path.file_name().expect("journal filename"));
        if let Err(err) = fs::hard_link(src_path, &dst_path) {
            if err.raw_os_error() == Some(18) {
                fs::copy(src_path, &dst_path)
                    .unwrap_or_else(|copy_err| panic!("copy {}: {}", src_path.display(), copy_err));
            } else {
                panic!("hard link {}: {}", src_path.display(), err);
            }
        }
    }

    let mut cfg = plugin_config::PluginConfig::default();
    cfg.journal.journal_dir = journal_root.to_string_lossy().to_string();

    let _open_tiers = Arc::new(RwLock::new(tiering::OpenTierState::default()));
    let _tier_flow_indexes = Arc::new(RwLock::new(tiering::TierFlowIndexStore::default()));
    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service for fixed raw journals");

    let before = (Utc::now().timestamp().max(1) as u32).saturating_add(3600);
    let request = query::FlowsRequest {
        view: query::ViewMode::TableSankey,
        after: Some(1),
        before: Some(before),
        group_by: vec![
            "SRC_ADDR".to_string(),
            "DST_ADDR".to_string(),
            "PROTOCOL".to_string(),
        ],
        top_n: query::TopN::N25,
        ..Default::default()
    };

    let stage4_match_only = query_service
        .benchmark_projected_raw_stage(&request, query::RawProjectedBenchStage::MatchOnly)
        .expect("stage4 match-only benchmark should succeed");
    let stage4 = query_service
        .benchmark_projected_raw_stage(&request, query::RawProjectedBenchStage::MatchAndExtract)
        .expect("stage4 benchmark should succeed");
    let stage5 = query_service
        .benchmark_projected_raw_stage(
            &request,
            query::RawProjectedBenchStage::MatchExtractAndParseMetrics,
        )
        .expect("stage5 benchmark should succeed");
    let stage6 = query_service
        .benchmark_projected_raw_stage(&request, query::RawProjectedBenchStage::GroupAndAccumulate)
        .expect("stage6 benchmark should succeed");

    eprintln!();
    eprintln!("=== Fixed Raw Plugin Stage Breakdown Harness ===");
    eprintln!("journal_dir:             {}", journal_root.display());
    eprintln!("group_by:                {:?}", request.group_by);
    eprintln!("match_rows_read:         {}", stage4_match_only.rows_read);
    eprintln!("match_fields_read:       {}", stage4_match_only.fields_read);
    eprintln!(
        "match_processed_fields:  {}",
        stage4_match_only.processed_fields
    );
    eprintln!(
        "match_compressed_fields: {}",
        stage4_match_only.compressed_processed_fields
    );
    eprintln!(
        "match_matched_entries:   {}",
        stage4_match_only.matched_entries
    );
    eprintln!(
        "match_checksum:          {}",
        stage4_match_only.work_checksum
    );
    eprintln!(
        "match_time_usec:         {}",
        stage4_match_only.elapsed_usec
    );
    eprintln!(
        "match_usec_per_row:      {:.6}",
        stage4_match_only.elapsed_usec as f64 / stage4_match_only.rows_read as f64
    );
    eprintln!("stage4_rows_read:        {}", stage4.rows_read);
    eprintln!("stage4_fields_read:      {}", stage4.fields_read);
    eprintln!("stage4_processed_fields: {}", stage4.processed_fields);
    eprintln!(
        "stage4_compressed_fields:{}",
        stage4.compressed_processed_fields
    );
    eprintln!("stage4_matched_entries:  {}", stage4.matched_entries);
    eprintln!("stage4_checksum:         {}", stage4.work_checksum);
    eprintln!("stage4_time_usec:        {}", stage4.elapsed_usec);
    eprintln!(
        "stage4_usec_per_row:     {:.6}",
        stage4.elapsed_usec as f64 / stage4.rows_read as f64
    );
    eprintln!("stage5_rows_read:        {}", stage5.rows_read);
    eprintln!("stage5_fields_read:      {}", stage5.fields_read);
    eprintln!("stage5_processed_fields: {}", stage5.processed_fields);
    eprintln!(
        "stage5_compressed_fields:{}",
        stage5.compressed_processed_fields
    );
    eprintln!("stage5_matched_entries:  {}", stage5.matched_entries);
    eprintln!("stage5_checksum:         {}", stage5.work_checksum);
    eprintln!("stage5_time_usec:        {}", stage5.elapsed_usec);
    eprintln!(
        "stage5_usec_per_row:     {:.6}",
        stage5.elapsed_usec as f64 / stage5.rows_read as f64
    );
    eprintln!("stage6_rows_read:        {}", stage6.rows_read);
    eprintln!("stage6_fields_read:      {}", stage6.fields_read);
    eprintln!("stage6_processed_fields: {}", stage6.processed_fields);
    eprintln!(
        "stage6_compressed_fields:{}",
        stage6.compressed_processed_fields
    );
    eprintln!("stage6_matched_entries:  {}", stage6.matched_entries);
    eprintln!("stage6_grouped_rows:     {}", stage6.grouped_rows);
    eprintln!("stage6_checksum:         {}", stage6.work_checksum);
    eprintln!("stage6_time_usec:        {}", stage6.elapsed_usec);
    eprintln!(
        "stage6_usec_per_row:     {:.6}",
        stage6.elapsed_usec as f64 / stage6.rows_read as f64
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
#[ignore]
async fn profile_fixed_raw_query_processing_against_local_journals() {
    const FIXED_FILES: [&str; 4] = [
        "/tmp/netdata-raw-snapshot-1774481723/system@92ecfa81f20440b9a0762a3a4656e37a-00000000045ab310-00064da65a07dfc3.journal",
        "/tmp/netdata-raw-snapshot-1774481723/system@92ecfa81f20440b9a0762a3a4656e37a-00000000045d8ec3-00064da8006c73a9.journal",
        "/tmp/netdata-raw-snapshot-1774481723/system@92ecfa81f20440b9a0762a3a4656e37a-0000000004606cfe-00064da9f3edc98c.journal",
        "/tmp/netdata-raw-snapshot-1774481723/system@92ecfa81f20440b9a0762a3a4656e37a-0000000004634242-00064dabd29b631e.journal",
    ];

    let tmp = tempfile::tempdir().expect("create temp dir");
    let journal_root = tmp.path().join("flows");
    let raw_dir = journal_root.join("raw");
    let minute_1_dir = journal_root.join("1m");
    let minute_5_dir = journal_root.join("5m");
    let hour_1_dir = journal_root.join("1h");
    fs::create_dir_all(&raw_dir).expect("create raw dir");
    fs::create_dir_all(&minute_1_dir).expect("create 1m dir");
    fs::create_dir_all(&minute_5_dir).expect("create 5m dir");
    fs::create_dir_all(&hour_1_dir).expect("create 1h dir");

    for src in FIXED_FILES {
        let src_path = Path::new(src);
        assert!(
            src_path.is_file(),
            "expected journal file {}",
            src_path.display()
        );
        let dst_path = raw_dir.join(src_path.file_name().expect("journal filename"));
        if let Err(err) = fs::hard_link(src_path, &dst_path) {
            if err.raw_os_error() == Some(18) {
                fs::copy(src_path, &dst_path)
                    .unwrap_or_else(|copy_err| panic!("copy {}: {}", src_path.display(), copy_err));
            } else {
                panic!("hard link {}: {}", src_path.display(), err);
            }
        }
    }

    let mut cfg = plugin_config::PluginConfig::default();
    cfg.journal.journal_dir = journal_root.to_string_lossy().to_string();

    let _open_tiers = Arc::new(RwLock::new(tiering::OpenTierState::default()));
    let _tier_flow_indexes = Arc::new(RwLock::new(tiering::TierFlowIndexStore::default()));
    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service for fixed raw journals");

    let before = (Utc::now().timestamp().max(1) as u32).saturating_add(3600);
    let request = query::FlowsRequest {
        view: query::ViewMode::TableSankey,
        after: Some(1),
        before: Some(before),
        group_by: vec![
            "SRC_ADDR".to_string(),
            "DST_ADDR".to_string(),
            "PROTOCOL".to_string(),
        ],
        top_n: query::TopN::N25,
        ..Default::default()
    };

    let cold_start = Instant::now();
    let cold_output = query_service
        .query_flows(&request)
        .await
        .expect("query fixed raw journals");
    let cold_elapsed = cold_start.elapsed();

    let warm_start = Instant::now();
    let warm_output = query_service
        .query_flows(&request)
        .await
        .expect("query fixed raw journals warm");
    let warm_elapsed = warm_start.elapsed();

    eprintln!();
    eprintln!("=== Fixed Raw Query Processing Harness ===");
    eprintln!("journal_dir:             {}", journal_root.display());
    eprintln!("files:                   {}", FIXED_FILES.len());
    eprintln!("after:                   1");
    eprintln!("before:                  {}", before);
    eprintln!("group_by:                {:?}", request.group_by);
    eprintln!(
        "cold_elapsed_ms:         {:.2}",
        cold_elapsed.as_secs_f64() * 1_000.0
    );
    eprintln!(
        "warm_elapsed_ms:         {:.2}",
        warm_elapsed.as_secs_f64() * 1_000.0
    );
    eprintln!("cold_flow_rows:          {}", cold_output.flows.len());
    eprintln!("warm_flow_rows:          {}", warm_output.flows.len());
    eprintln!(
        "cold_metric_bytes:       {}",
        cold_output.metrics.get("bytes").copied().unwrap_or(0)
    );
    eprintln!(
        "cold_metric_packets:     {}",
        cold_output.metrics.get("packets").copied().unwrap_or(0)
    );
    eprintln!(
        "warm_metric_bytes:       {}",
        warm_output.metrics.get("bytes").copied().unwrap_or(0)
    );
    eprintln!(
        "warm_metric_packets:     {}",
        warm_output.metrics.get("packets").copied().unwrap_or(0)
    );
    eprintln!("cold_stats:              {:?}", cold_output.stats);
    eprintln!("warm_stats:              {:?}", warm_output.stats);
}

async fn ingest_fixture(
    fixture_name: &str,
) -> (
    plugin_config::PluginConfig,
    Arc<ingest::IngestMetrics>,
    Arc<RwLock<tiering::OpenTierState>>,
    Arc<RwLock<tiering::TierFlowIndexStore>>,
    TempDir,
) {
    ingest_fixture_with_timestamp_source(fixture_name, plugin_config::TimestampSource::Input).await
}

async fn ingest_fixture_with_timestamp_source(
    fixture_name: &str,
    timestamp_source: plugin_config::TimestampSource,
) -> (
    plugin_config::PluginConfig,
    Arc<ingest::IngestMetrics>,
    Arc<RwLock<tiering::OpenTierState>>,
    Arc<RwLock<tiering::TierFlowIndexStore>>,
    TempDir,
) {
    let tmp = tempfile::tempdir().expect("create temp dir");
    let listen = reserve_udp_listen_addr();
    let mut cfg = plugin_config::PluginConfig::default();
    cfg.journal.journal_dir = tmp.path().join("flows").to_string_lossy().to_string();
    cfg.listener.listen = listen.clone();
    cfg.listener.sync_interval = Duration::from_millis(50);
    cfg.listener.sync_every_entries = 1;
    cfg.protocols.timestamp_source = timestamp_source;

    let metrics = Arc::new(ingest::IngestMetrics::default());
    let open_tiers = Arc::new(RwLock::new(tiering::OpenTierState::default()));
    let tier_flow_indexes = Arc::new(RwLock::new(tiering::TierFlowIndexStore::default()));
    let service = ingest::IngestService::new(
        cfg.clone(),
        Arc::clone(&metrics),
        Arc::clone(&open_tiers),
        Arc::clone(&tier_flow_indexes),
    )
    .expect("create ingest service");

    let shutdown = CancellationToken::new();
    let run_shutdown = shutdown.clone();
    let ingest_task = tokio::spawn(async move { service.run(run_shutdown).await });

    tokio::time::sleep(Duration::from_millis(100)).await;
    replay_fixture_udp(&listen, fixture_name).await;

    wait_for_ingest_progress(&metrics).await;
    shutdown.cancel();

    ingest_task
        .await
        .expect("join ingestion task")
        .expect("ingestion run");

    (cfg, metrics, open_tiers, tier_flow_indexes, tmp)
}

async fn wait_for_ingest_progress(metrics: &Arc<ingest::IngestMetrics>) {
    tokio::time::timeout(Duration::from_secs(10), async {
        loop {
            if metrics.journal_entries_written.load(Ordering::Relaxed) > 0 {
                break;
            }
            tokio::time::sleep(Duration::from_millis(25)).await;
        }
    })
    .await
    .expect("ingest did not write raw entries in time");
}

async fn replay_fixture_udp(listen: &str, fixture_name: &str) {
    let sender = UdpSocket::bind("127.0.0.1:0")
        .await
        .expect("bind udp sender");
    let payloads = fixture_udp_payloads(fixture_name);
    assert!(
        !payloads.is_empty(),
        "fixture {fixture_name} should contain udp payloads"
    );

    for payload in payloads {
        sender
            .send_to(&payload, listen)
            .await
            .expect("send fixture datagram");
    }
}

fn fixture_udp_payloads(fixture_name: &str) -> Vec<Vec<u8>> {
    let path = fixture_dir().join(fixture_name);
    let file = fs::File::open(&path)
        .unwrap_or_else(|err| panic!("open fixture {}: {}", path.display(), err));
    let mut reader =
        PcapReader::new(file).unwrap_or_else(|err| panic!("open pcap {}: {}", path.display(), err));

    let mut payloads = Vec::new();
    while let Some(packet) = reader.next_packet() {
        let packet = packet.unwrap_or_else(|err| panic!("read packet {}: {}", path.display(), err));
        if let Some(payload) = extract_udp_payload(packet.data.as_ref()) {
            payloads.push(payload.to_vec());
        }
    }
    payloads
}

fn extract_udp_payload(packet: &[u8]) -> Option<&[u8]> {
    let sliced = SlicedPacket::from_ethernet(packet).ok()?;
    match sliced.transport {
        Some(TransportSlice::Udp(udp)) => Some(udp.payload()),
        _ => None,
    }
}

fn fixture_dir() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("testdata/flows")
}

fn reserve_udp_listen_addr() -> String {
    let sock = StdUdpSocket::bind("127.0.0.1:0").expect("reserve udp listen socket");
    let addr = sock.local_addr().expect("read local addr");
    addr.to_string()
}

fn assert_tier_has_files(path: &Path, tier_name: &str) {
    let count = tier_file_count(path);
    assert!(
        count > 0,
        "expected journal files in {tier_name} tier directory {}, found {}",
        path.display(),
        count
    );
}

fn assert_tier_dir_exists(path: &Path, tier_name: &str) {
    assert!(
        path.is_dir(),
        "expected {tier_name} tier directory to exist at {}",
        path.display()
    );
}

fn tier_file_count(path: &Path) -> usize {
    fn count_journal_files(path: &Path) -> usize {
        fs::read_dir(path)
            .unwrap_or_else(|err| panic!("read tier dir {}: {}", path.display(), err))
            .filter_map(Result::ok)
            .map(|entry| entry.path())
            .map(|entry_path| {
                if entry_path.is_dir() {
                    return count_journal_files(&entry_path);
                }

                entry_path
                    .extension()
                    .and_then(|ext| ext.to_str())
                    .map(|ext| usize::from(ext == "journal"))
                    .unwrap_or(0)
            })
            .sum()
    }

    count_journal_files(path)
}
