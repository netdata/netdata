use super::*;
use crate::protocol::{CgroupsBuilder, NipcError, PROFILE_BASELINE, PROFILE_SHM_HYBRID};
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use std::thread;
use std::time::Duration;

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

fn connect_ready(client: &mut CgroupsClient) {
    for _ in 0..200 {
        client.refresh();
        if client.ready() {
            return;
        }
        thread::sleep(Duration::from_millis(10));
    }

    panic!("client did not reach READY state");
}

struct TestServer {
    service: String,
    wake_config: ClientConfig,
    stop_flag: Arc<std::sync::atomic::AtomicBool>,
    thread: Option<thread::JoinHandle<()>>,
}

impl TestServer {
    fn start(service: &str, config: ServerConfig) -> Self {
        ensure_run_dir();

        let svc = service.to_string();
        let wake_config = ClientConfig {
            supported_profiles: config.supported_profiles,
            preferred_profiles: config.preferred_profiles,
            max_request_batch_items: config.max_request_batch_items,
            max_response_payload_bytes: config.max_response_payload_bytes,
            auth_token: config.auth_token,
            ..ClientConfig::default()
        };
        let mut server = ManagedServer::new(TEST_RUN_DIR, service, config, snapshot_handler());
        let stop_flag = server.running_flag();
        let thread = thread::spawn(move || {
            let _ = server.run();
        });

        thread::sleep(Duration::from_millis(200));

        Self {
            service: svc,
            wake_config,
            stop_flag,
            thread: Some(thread),
        }
    }
}

impl Drop for TestServer {
    fn drop(&mut self) {
        self.stop_flag
            .store(false, std::sync::atomic::Ordering::Release);
        let mut wake = CgroupsClient::new(TEST_RUN_DIR, &self.service, self.wake_config.clone());
        let _ = wake.refresh();
        if let Some(thread) = self.thread.take() {
            let _ = thread.join();
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

    let mut cache = CgroupsCache::new(TEST_RUN_DIR, &service, client_config());
    let mut updated = false;
    for _ in 0..200 {
        if cache.refresh() {
            updated = true;
            break;
        }
        thread::sleep(Duration::from_millis(10));
    }
    assert!(updated);
    assert!(cache.ready());

    let item = cache.lookup(1001, "docker-abc123").expect("lookup");
    assert_eq!(item.path, "/sys/fs/cgroup/docker/abc123");
}

#[test]
fn test_client_not_ready_returns_error_windows() {
    let service = unique_service("not_ready");
    let mut client = CgroupsClient::new(TEST_RUN_DIR, &service, client_config());
    match client.call_snapshot() {
        Err(NipcError::BadLayout) => {}
        other => panic!("unexpected result: {other:?}"),
    }
}
