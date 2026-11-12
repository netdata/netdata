use super::*;
use crate::protocol::{CgroupsBuilder, NipcError, PROFILE_BASELINE, PROFILE_SHM_HYBRID};
use std::sync::Arc;
use std::sync::atomic::{AtomicU64, Ordering};

const TEST_RUN_DIR: &str = r"C:\Temp\nipc_cgroups_rust_test";
const AUTH_TOKEN: u64 = 0xDEADBEEFCAFEBABE;
const RESPONSE_BUF_SIZE: usize = 65536;
static SERVICE_COUNTER: AtomicU64 = AtomicU64::new(0);

fn ensure_run_dir() {
    let _ = std::fs::create_dir_all(TEST_RUN_DIR);
}

fn unique_service(prefix: &str) -> String {
    format!(
        "{}_{}_{}",
        prefix,
        std::process::id(),
        SERVICE_COUNTER.fetch_add(1, Ordering::Relaxed) + 1
    )
}

fn server_config() -> ServerConfig {
    ServerConfig {
        supported_profiles: PROFILE_BASELINE,
        max_request_batch_items: 1,
        max_response_payload_bytes: RESPONSE_BUF_SIZE as u32,
        auth_token: AUTH_TOKEN,
        ..ServerConfig::default()
    }
}

fn client_config() -> ClientConfig {
    ClientConfig {
        supported_profiles: PROFILE_BASELINE,
        max_request_batch_items: 1,
        max_response_payload_bytes: RESPONSE_BUF_SIZE as u32,
        auth_token: AUTH_TOKEN,
        ..ClientConfig::default()
    }
}

fn shm_server_config() -> ServerConfig {
    ServerConfig {
        supported_profiles: PROFILE_SHM_HYBRID | PROFILE_BASELINE,
        max_request_batch_items: 1,
        max_response_payload_bytes: RESPONSE_BUF_SIZE as u32,
        auth_token: AUTH_TOKEN,
        ..ServerConfig::default()
    }
}

fn shm_client_config() -> ClientConfig {
    ClientConfig {
        supported_profiles: PROFILE_SHM_HYBRID | PROFILE_BASELINE,
        max_request_batch_items: 1,
        max_response_payload_bytes: RESPONSE_BUF_SIZE as u32,
        auth_token: AUTH_TOKEN,
        ..ClientConfig::default()
    }
}

fn fill_snapshot(builder: &mut CgroupsBuilder<'_>) -> bool {
    let items = [
        (
            1001u32,
            0u32,
            1u32,
            b"docker-abc123" as &[u8],
            b"/sys/fs/cgroup/docker/abc123" as &[u8],
        ),
        (2002, 0, 1, b"k8s-pod-xyz", b"/sys/fs/cgroup/kubepods/xyz"),
        (
            3003,
            0,
            0,
            b"systemd-user",
            b"/sys/fs/cgroup/user.slice/user-1000",
        ),
    ];

    for (hash, options, enabled, name, path) in &items {
        if builder.add(*hash, *options, *enabled, name, path).is_err() {
            return false;
        }
    }

    true
}

fn snapshot_handler() -> Handler {
    Handler {
        handle: Some(Arc::new(|req, builder| {
            if req.layout_version != 1 || req.flags != 0 {
                return false;
            }
            builder.set_header(1, 42);
            fill_snapshot(builder)
        })),
        snapshot_max_items: 3,
    }
}

/// The server listens before tests connect, so one refresh must succeed.
fn connect_ready(client: &mut CgroupsClient) {
    client.refresh();
    assert!(client.ready(), "client did not reach READY state");
}

struct TestServer {
    _inner: raw::RunningTestServer,
}

impl TestServer {
    fn start(service: &str, config: ServerConfig) -> Self {
        ensure_run_dir();
        let server = ManagedServer::new(TEST_RUN_DIR, service, config, snapshot_handler());
        Self {
            _inner: server.inner.spawn_for_test(),
        }
    }
}

#[test]
fn test_snapshot_round_trip_windows() {
    let service = unique_service("snapshot");
    let _server = TestServer::start(&service, server_config());

    let mut client = CgroupsClient::new(TEST_RUN_DIR, &service, client_config());
    connect_ready(&mut client);

    let view = client.call_snapshot().expect("snapshot");
    assert_eq!(view.item_count, 3);
    assert_eq!(view.systemd_enabled, 1);
    assert_eq!(view.generation, 42);
}

#[test]
fn test_snapshot_round_trip_win_shm() {
    let service = unique_service("snapshot_shm");
    let _server = TestServer::start(&service, shm_server_config());

    let mut client = CgroupsClient::new(TEST_RUN_DIR, &service, shm_client_config());
    connect_ready(&mut client);

    let view = client.call_snapshot().expect("snapshot");
    assert_eq!(view.item_count, 3);
    assert_eq!(view.generation, 42);
}

#[test]
fn test_cache_round_trip_windows() {
    let service = unique_service("cache");
    let _server = TestServer::start(&service, server_config());

    let cache = CgroupsCache::new(TEST_RUN_DIR, &service, client_config());
    assert!(cache.refresh());
    assert!(cache.ready());

    let item = {
        let guard = cache.read_lock();
        guard.get(1001, "docker-abc123").expect("lookup").dup()
    };
    assert_eq!(item.path, "/sys/fs/cgroup/docker/abc123");

    let status = cache.status();
    assert!(status.populated);
    assert_eq!(status.item_count, 3);
    assert_eq!(status.systemd_enabled, 1);
    assert_eq!(status.generation, 42);
    assert_eq!(status.refresh_success_count, 1);
    assert_eq!(status.refresh_failure_count, 0);
    assert_eq!(status.connection_state, ClientState::Ready);

    cache.set_call_timeout(500);
    let abort = cache.abort_handle();
    abort.abort();
    cache.clear_abort();
    abort.clear();
    cache.close();
    assert!(!cache.ready());
    {
        let guard = cache.read_lock();
        assert!(guard.get(1001, "docker-abc123").is_none());
    }
}

#[test]
fn test_client_not_ready_returns_error_windows() {
    let service = unique_service("not_ready");
    let mut client = CgroupsClient::new(TEST_RUN_DIR, &service, client_config());
    match client.call_snapshot() {
        Err(NipcError::NotReady) => {}
        other => panic!("unexpected result: {other:?}"),
    }
}

#[test]
fn test_client_facade_controls_windows() {
    ensure_run_dir();
    let service = unique_service("client_controls");
    let mut client = CgroupsClient::new(TEST_RUN_DIR, &service, client_config());

    assert!(!client.ready());
    let initial = client.status();
    assert_eq!(initial.state, ClientState::Disconnected);
    assert_eq!(initial.connect_count, 0);
    assert_eq!(initial.reconnect_count, 0);
    assert_eq!(initial.call_count, 0);
    assert_eq!(initial.error_count, 0);

    client.set_call_timeout(1);
    let abort = client.abort_handle();
    abort.abort();
    match client.call_snapshot_with_timeout(1) {
        Err(NipcError::NotReady) => {}
        other => panic!("unexpected result: {other:?}"),
    }

    client.abort();
    client.clear_abort();
    abort.clear();
    client.close();

    let closed = client.status();
    assert_eq!(closed.state, ClientState::Disconnected);
    assert!(!client.ready());
}

#[test]
fn test_cache_facade_controls_windows() {
    ensure_run_dir();
    let service = unique_service("cache_controls");
    let cache = CgroupsCache::new(TEST_RUN_DIR, &service, client_config());

    assert!(!cache.ready());
    {
        let guard = cache.read_lock();
        assert!(guard.get(1001, "docker-abc123").is_none());
    }
    let initial = cache.status();
    assert!(!initial.populated);
    assert_eq!(initial.item_count, 0);
    assert_eq!(initial.refresh_success_count, 0);
    assert_eq!(initial.refresh_failure_count, 0);
    assert_eq!(initial.connection_state, ClientState::Disconnected);

    cache.set_call_timeout(1);
    let abort = cache.abort_handle();
    abort.abort();
    assert!(!cache.refresh());

    let failed = cache.status();
    assert!(!failed.populated);
    assert_eq!(failed.refresh_success_count, 0);
    assert_eq!(failed.refresh_failure_count, 1);

    cache.abort();
    cache.clear_abort();
    abort.clear();
    cache.close();
    assert!(!cache.ready());
}

#[test]
fn test_managed_server_facade_controls_windows() {
    ensure_run_dir();
    let service = unique_service("server_controls");
    let server = ManagedServer::with_workers(
        TEST_RUN_DIR,
        &service,
        server_config(),
        snapshot_handler(),
        1,
    );

    let running = server.running_flag();
    assert!(!running.load(Ordering::Acquire));
    server.stop();
    assert!(!running.load(Ordering::Acquire));
}
