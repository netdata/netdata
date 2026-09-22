//! Reading evicted files back from remote storage, end to end through the
//! Function: the answers equal the same files served locally, and every way
//! remote data can be missing is reported, never silent.

use super::*;
use crate::ledger::rpc::traces::fixtures::{
    Evicted, TestRemote, install_sealed, make_registries, otlp_req_svc,
};
use bridge::function::ProgressState;
use file_registry::test_identity;
use serde_json::json;
use tokio_util::sync::CancellationToken;

/// Corpus base, unix seconds.
const T_S: u32 = 1_700_000_000;
const MIB: u64 = 1024 * 1024;

fn base_ns(offset_s: u64) -> u64 {
    (u64::from(T_S) + offset_s) * 1_000_000_000
}

/// File 1: traces A (1 span) and B (2 spans) in the window's first half.
fn file_one() -> Vec<opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest> {
    vec![
        otlp_req_svc(0x0A, 1, base_ns(10), "svc-a"),
        otlp_req_svc(0x0B, 2, base_ns(20), "svc-b"),
    ]
}

/// File 2: traces C (3 spans) and E (1 span) in its second half.
fn file_two() -> Vec<opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest> {
    vec![
        otlp_req_svc(0x0C, 3, base_ns(30), "svc-a"),
        otlp_req_svc(0x0E, 1, base_ns(40), "svc-a"),
    ]
}

fn trace_id(byte: u8) -> String {
    format!("{byte:02x}").repeat(16)
}

fn handler(registries: Arc<RwLock<TenantRegistries>>, remote: &TestRemote) -> OtelTracesHandler {
    OtelTracesHandler::new(
        registries,
        Arc::new(ChunkCache::new(64 * 1024 * 1024)),
        4,
        Some(remote.read()),
    )
}

async fn call(h: &OtelTracesHandler, body: serde_json::Value) -> netdata_plugin_error::Result<serde_json::Value> {
    call_with(h, body, &ProgressState::new()).await
}

async fn call_with(
    h: &OtelTracesHandler,
    body: serde_json::Value,
    progress: &ProgressState,
) -> netdata_plugin_error::Result<serde_json::Value> {
    let ctx = FunctionCallContext::new(
        "tx-remote".to_string(),
        progress.clone(),
        CancellationToken::new(),
    );
    let req: OtelTracesRequest = serde_json::from_value(body).unwrap();
    h.on_call(ctx, req)
        .await
        .map(|resp| serde_json::to_value(resp).unwrap())
}

fn search_body(limit: usize) -> serde_json::Value {
    json!({"search": {"after": T_S, "before": T_S + 100, "limit": limit}})
}

/// The Functions view over the corpus window (no mode selector).
fn functions_body(last: usize) -> serde_json::Value {
    json!({"after": T_S, "before": T_S + 100, "last": last})
}

fn trace_body(byte: u8) -> serde_json::Value {
    json!({"trace": {"id": trace_id(byte)}})
}

/// Both files local; remote storage on but nothing cataloged.
async fn local_setup() -> (OtelTracesHandler, TestRemote) {
    let registries = make_registries();
    install_sealed(&registries, "default", 1, file_one()).await;
    install_sealed(&registries, "default", 2, file_two()).await;
    let remote = TestRemote::new(64 * MIB);
    (handler(registries, &remote), remote)
}

/// Both files evicted: remote objects and catalog entries only.
async fn evicted_setup(capacity: u64) -> (OtelTracesHandler, TestRemote, [Evicted; 2]) {
    let registries = make_registries();
    let remote = TestRemote::new(capacity);
    let one = remote
        .evicted(&registries, "default", test_identity(), 1, file_one())
        .await;
    let two = remote
        .evicted(&registries, "default", test_identity(), 2, file_two())
        .await;
    (handler(registries, &remote), remote, [one, two])
}

fn traces_of(v: &serde_json::Value) -> Vec<String> {
    v["traces"]
        .as_array()
        .unwrap()
        .iter()
        .map(|t| t["trace_id"].as_str().unwrap()[..2].to_string())
        .collect()
}

#[tokio::test]
async fn evicted_files_answer_exactly_as_local_files() {
    let (local, _) = local_setup().await;
    let (evicted, remote, _) = evicted_setup(64 * MIB).await;

    for body in [
        trace_body(0x0B),
        trace_body(0x0C),
        search_body(20),
        functions_body(2),
    ] {
        let from_local = call(&local, body.clone()).await.unwrap();
        let from_remote = call(&evicted, body.clone()).await.unwrap();
        assert_eq!(from_remote, from_local, "{body}");
    }
    // The local side is Complete with remote storage on: nothing remote
    // was needed, nothing reported missing.
    let local_search = call(&local, search_body(20)).await.unwrap();
    assert_eq!(local_search["status"], json!({"complete": true}));
    assert_eq!(traces_of(&local_search), ["0e", "0c", "0b", "0a"]);
    // The answers came from downloaded files.
    assert_eq!(remote.cache.file_count(), 2);
}

#[tokio::test]
async fn a_failed_download_is_reported_and_leaves_search_inexact() {
    let (h, remote, [_, two]) = evicted_setup(64 * MIB).await;
    remote.lose(&two);
    let partial = json!({"partial": ["remote_unavailable"]});

    // B lives in the file that downloads: served, but the lookup cannot
    // know whether the lost file held more of it.
    let v = call(&h, trace_body(0x0B)).await.unwrap();
    assert_eq!(v["status"], partial);
    assert_eq!(v["items"]["returned"], 2);

    // A full page over a partial result: every summary inexact, no cursor.
    let v = call(&h, search_body(2)).await.unwrap();
    assert_eq!(v["status"], partial);
    assert_eq!(traces_of(&v), ["0b", "0a"]);
    assert!(v["traces"].as_array().unwrap().iter().all(|t| t["exact"] == false));
    assert!(v.get("anchor").is_none(), "a partial page has no next cursor");

    // The Functions view: the page and its aggregate both say so.
    let v = call(&h, functions_body(2)).await.unwrap();
    assert_eq!(v["data"]["status"], partial);
    assert_eq!(v["data"]["overview"]["status"], partial);
}

#[tokio::test]
async fn an_unreadable_catalog_is_reported() {
    let (h, _remote, [one, _]) = evicted_setup(64 * MIB).await;
    std::fs::write(&one.catalog, b"not a catalog").unwrap();
    let partial = json!({"partial": ["remote_unavailable"]});

    let v = call(&h, trace_body(0x0A)).await.unwrap();
    assert_eq!(v["status"], partial);
    assert_eq!(v["items"]["returned"], 0);

    let v = call(&h, search_body(20)).await.unwrap();
    assert_eq!(v["status"], partial);
    assert_eq!(traces_of(&v), ["0e", "0c"]);
}

#[tokio::test]
async fn a_query_too_large_for_the_download_cache_is_a_hard_error() {
    let (h, remote, _) = evicted_setup(100).await;

    for body in [trace_body(0x0B), search_body(20), functions_body(2)] {
        let err = call(&h, body.clone()).await.unwrap_err().to_string();
        assert!(
            err.contains("more than 100 B of remote trace data")
                && err.contains("narrow the time range or raise `remote_storage.read_cache_max_size`"),
            "{body}: {err}"
        );
    }
    assert_eq!(remote.cache.file_count(), 0, "nothing was downloaded");
}

#[tokio::test]
async fn an_unwritable_cache_directory_degrades_each_download() {
    let (h, remote, _) = evicted_setup(64 * MIB).await;
    // The cache path stops being a directory: every write into it fails,
    // for root too (unlike a permission change).
    std::fs::remove_dir(&remote.cache_dir).unwrap();
    std::fs::write(&remote.cache_dir, b"not a directory").unwrap();

    let v = call(&h, trace_body(0x0B)).await.unwrap();

    assert_eq!(v["status"], json!({"partial": ["remote_unavailable"]}));
    assert_eq!(v["items"]["returned"], 0);
}

#[tokio::test]
async fn a_cache_that_cannot_evict_is_a_hard_error() {
    let (_, mut remote, [one, two]) = evicted_setup(64 * MIB).await;
    // Room for either file, not both: the second lookup must evict the first.
    let (a, b) = (one.entry.size.as_u64(), two.entry.size.as_u64());
    let capacity = a.max(b);
    assert!(a + b > capacity);
    remote.cache = file_cache::FileCache::open(&remote.cache_dir, capacity).unwrap();
    let registries = make_registries();
    {
        let mut guard = registries.write().await;
        crate::test_helpers::track_catalog_entry(&mut guard, "default", one.entry.clone());
        crate::test_helpers::track_catalog_entry(&mut guard, "default", two.entry.clone());
    }
    let h = handler(registries, &remote);
    // Bounded lookups: each plans only its own file.
    let first = json!({"trace": {"id": trace_id(0x0A), "after": T_S + 5, "before": T_S + 25}});
    let second = json!({"trace": {"id": trace_id(0x0C), "after": T_S + 28, "before": T_S + 45}});
    assert_eq!(call(&h, first).await.unwrap()["status"], json!({"complete": true}));

    // The cached first file becomes a directory the cache cannot unlink,
    // for root too (unlike a permission change).
    let cached = remote.cache_dir.join(one.entry.id.to_filename("sfst"));
    std::fs::remove_file(&cached).unwrap();
    std::fs::create_dir(&cached).unwrap();
    let err = call(&h, second).await.unwrap_err().to_string();
    assert!(err.contains("download cache directory is unwritable"), "{err}");
}

#[tokio::test]
async fn a_local_copy_is_served_and_not_downloaded() {
    // Uploaded, cataloged and still local: the catalog entry is masked, so
    // the lost object is never needed.
    let registries = make_registries();
    let remote = TestRemote::new(64 * MIB);
    let one = remote
        .evicted(&registries, "default", test_identity(), 1, file_one())
        .await;
    remote.lose(&one);
    install_sealed(&registries, "default", 1, file_one()).await;
    let h = handler(registries, &remote);

    let v = call(&h, trace_body(0x0B)).await.unwrap();
    assert_eq!(v["status"], json!({"complete": true}));
    assert_eq!(v["items"]["returned"], 2);
    assert_eq!(remote.cache.file_count(), 0);
}

#[tokio::test]
async fn another_identitys_file_at_the_same_seq_is_downloaded() {
    // A prior process instance used the same seq: its remote file is a
    // different file, and the local one does not mask it.
    let registries = make_registries();
    let remote = TestRemote::new(64 * MIB);
    install_sealed(&registries, "default", 1, file_one()).await;
    let foreign = file_registry::Identity::new(
        file_registry::MachineId::new(uuid::Uuid::from_u128(0x77)).unwrap(),
        file_registry::InstanceId::new(uuid::Uuid::from_u128(0x78)).unwrap(),
    );
    remote
        .evicted(&registries, "default", foreign, 1, file_two())
        .await;
    let h = handler(registries, &remote);

    let v = call(&h, search_body(20)).await.unwrap();
    assert_eq!(v["status"], json!({"complete": true}));
    assert_eq!(traces_of(&v), ["0e", "0c", "0b", "0a"]);
    assert_eq!(remote.cache.file_count(), 1);
}

/// File 1 local, file 2 evicted.
async fn half_evicted_setup() -> (OtelTracesHandler, TestRemote) {
    let registries = make_registries();
    let remote = TestRemote::new(64 * MIB);
    install_sealed(&registries, "default", 1, file_one()).await;
    remote
        .evicted(&registries, "default", test_identity(), 2, file_two())
        .await;
    (handler(registries, &remote), remote)
}

#[tokio::test]
async fn progress_counts_the_downloads_beside_the_sources() {
    // Every walked source ticks, and so does each planned download: two
    // sources plus one download; the Functions view walks both of its
    // ranges' two sources.
    for (body, expected) in [
        (trace_body(0x0C), (3, 3)),
        (search_body(20), (3, 3)),
        (functions_body(2), (5, 5)),
    ] {
        let (h, _remote) = half_evicted_setup().await;
        let progress = ProgressState::new();
        call_with(&h, body.clone(), &progress).await.unwrap();
        assert_eq!(progress.load(), expected, "{body}");
    }

    // A repeat finds the file cached: nothing downloads, so the total is an
    // upper bound the answer arrives under.
    let (h, _remote) = half_evicted_setup().await;
    call(&h, trace_body(0x0C)).await.unwrap();
    let progress = ProgressState::new();
    call_with(&h, trace_body(0x0C), &progress).await.unwrap();
    assert_eq!(progress.load(), (2, 3));
}

/// The windowed aggregate modes over the corpus window.
fn aggregate_bodies() -> [serde_json::Value; 4] {
    let window = |mode: &str, extra: serde_json::Value| {
        let mut params = json!({"after": T_S, "before": T_S + 100});
        params
            .as_object_mut()
            .unwrap()
            .extend(extra.as_object().unwrap().clone());
        json!({ mode: params })
    };
    [
        window("overview", json!({"facets": true})),
        window("slowest", json!({})),
        window("attributes", json!({})),
        window("attribute_values", json!({"key": "resource.service.name"})),
    ]
}

#[tokio::test]
async fn evicted_files_answer_the_aggregate_modes_as_local_files() {
    let (local, _) = local_setup().await;
    let (evicted, _remote, _) = evicted_setup(64 * MIB).await;

    for body in aggregate_bodies() {
        let from_local = call(&local, body.clone()).await.unwrap();
        let from_remote = call(&evicted, body.clone()).await.unwrap();
        assert_eq!(from_local["status"], json!({"complete": true}), "{body}");
        assert_eq!(from_remote, from_local, "{body}");
    }
}

#[tokio::test]
async fn every_aggregate_mode_reports_a_failed_download() {
    let (h, remote, [_, two]) = evicted_setup(64 * MIB).await;
    remote.lose(&two);

    for body in aggregate_bodies() {
        let v = call(&h, body.clone()).await.unwrap();
        assert_eq!(v["status"], json!({"partial": ["remote_unavailable"]}), "{body}");
    }
    // What the downloaded file holds is still served.
    let v = call(&h, aggregate_bodies()[1].clone()).await.unwrap();
    assert_eq!(traces_of(&v), ["0b", "0a"]);
}

#[tokio::test]
async fn a_lost_file_outside_the_window_does_not_make_an_answer_partial() {
    // File 2 (seconds 30-40) is lost; a window over file 1 alone does not
    // need it.
    let (h, remote, [_, two]) = evicted_setup(64 * MIB).await;
    remote.lose(&two);

    for mode in ["overview", "slowest", "attributes"] {
        let body = json!({ mode: {"after": T_S + 5, "before": T_S + 25} });
        let v = call(&h, body.clone()).await.unwrap();
        assert_eq!(v["status"], json!({"complete": true}), "{body}");
    }
    let body = json!({"attribute_values": {"after": T_S + 5, "before": T_S + 25, "key": "resource.service.name"}});
    let v = call(&h, body).await.unwrap();
    assert_eq!(v["status"], json!({"complete": true}));
}
