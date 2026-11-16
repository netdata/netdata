use super::*;
#[cfg(any(target_os = "linux", target_os = "freebsd"))]
use crate::protocol::PROFILE_SHM_FUTEX;
use crate::protocol::{CgroupsBuilder, NipcError, PROFILE_BASELINE};
use std::sync::Arc;
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::{SystemTime, UNIX_EPOCH};

const TEST_RUN_DIR: &str = "/tmp/nipc_cgroups_rust_test";
const AUTH_TOKEN: u64 = 0xDEADBEEFCAFEBABE;
const RESPONSE_BUF_SIZE: usize = 65536;
static SERVICE_COUNTER: AtomicU64 = AtomicU64::new(0);

fn ensure_run_dir() {
    let _ = std::fs::create_dir_all(TEST_RUN_DIR);
}

fn unique_service(prefix: &str) -> String {
    let stamp = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_nanos();
    let service = format!(
        "{}_{}_{}_{}",
        prefix,
        std::process::id(),
        SERVICE_COUNTER.fetch_add(1, Ordering::Relaxed) + 1,
        stamp
    );
    crate::test_locks::track(TEST_RUN_DIR, &service);
    service
}

fn cleanup_all(service: &str) {
    let _ = std::fs::remove_file(format!("{TEST_RUN_DIR}/{service}.sock"));
}

fn server_config() -> ServerConfig {
    ServerConfig {
        supported_profiles: PROFILE_BASELINE,
        preferred_profiles: PROFILE_BASELINE,
        max_request_batch_items: 1,
        max_response_payload_bytes: RESPONSE_BUF_SIZE as u32,
        auth_token: AUTH_TOKEN,
        ..ServerConfig::default()
    }
}

fn client_config() -> ClientConfig {
    ClientConfig {
        supported_profiles: PROFILE_BASELINE,
        preferred_profiles: PROFILE_BASELINE,
        max_request_batch_items: 1,
        max_response_payload_bytes: RESPONSE_BUF_SIZE as u32,
        auth_token: AUTH_TOKEN,
        ..ClientConfig::default()
    }
}

#[cfg(any(target_os = "linux", target_os = "freebsd"))]
fn shm_server_config() -> ServerConfig {
    ServerConfig {
        supported_profiles: PROFILE_BASELINE | PROFILE_SHM_FUTEX,
        preferred_profiles: PROFILE_SHM_FUTEX,
        max_request_batch_items: 1,
        max_response_payload_bytes: RESPONSE_BUF_SIZE as u32,
        auth_token: AUTH_TOKEN,
        ..ServerConfig::default()
    }
}

#[cfg(any(target_os = "linux", target_os = "freebsd"))]
fn shm_client_config() -> ClientConfig {
    ClientConfig {
        supported_profiles: PROFILE_BASELINE | PROFILE_SHM_FUTEX,
        preferred_profiles: PROFILE_SHM_FUTEX,
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
        cleanup_all(service);
        let server = ManagedServer::new(TEST_RUN_DIR, service, config, snapshot_handler());
        Self {
            _inner: server.inner.spawn_for_test(),
        }
    }
}

#[test]
fn test_snapshot_round_trip_unix() {
    let service = unique_service("snapshot");
    let _server = TestServer::start(&service, server_config());

    let mut client = CgroupsClient::new(TEST_RUN_DIR, &service, client_config());
    connect_ready(&mut client);

    let view = client.call_snapshot().expect("snapshot");
    assert_eq!(view.item_count, 3);
    assert_eq!(view.systemd_enabled, 1);
    assert_eq!(view.generation, 42);

    let item0 = view.item(0).expect("item 0");
    assert_eq!(item0.hash, 1001);
    assert_eq!(item0.name.as_bytes(), b"docker-abc123");
    assert_eq!(item0.path.as_bytes(), b"/sys/fs/cgroup/docker/abc123");
}

#[cfg(any(target_os = "linux", target_os = "freebsd"))]
#[test]
fn test_snapshot_round_trip_shm_unix() {
    let service = unique_service("snapshot_shm");
    let _server = TestServer::start(&service, shm_server_config());

    let mut client = CgroupsClient::new(TEST_RUN_DIR, &service, shm_client_config());
    connect_ready(&mut client);

    let view = client.call_snapshot().expect("snapshot");
    assert_eq!(view.item_count, 3);
    assert_eq!(view.generation, 42);
}

#[test]
fn test_cache_round_trip_unix() {
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
    assert_eq!(status.generation, 42);
}

#[test]
fn test_client_not_ready_returns_error_unix() {
    let service = unique_service("not_ready");
    cleanup_all(&service);

    let mut client = CgroupsClient::new(TEST_RUN_DIR, &service, client_config());
    match client.call_snapshot() {
        Err(NipcError::NotReady) => {}
        other => panic!("unexpected result: {other:?}"),
    }
}
