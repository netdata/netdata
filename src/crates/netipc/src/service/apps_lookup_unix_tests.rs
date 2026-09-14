use super::*;
use crate::protocol::{
    AppsLookupBuilder, AppsLookupRequestView, APPS_CGROUP_HOST_ROOT, APPS_CGROUP_KNOWN,
    NIPC_UID_UNSET, ORCHESTRATOR_DOCKER, PID_LOOKUP_KNOWN, PID_LOOKUP_UNKNOWN, PROFILE_BASELINE,
};
use std::path::PathBuf;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use std::thread;
use std::time::{Duration, SystemTime, UNIX_EPOCH};

const TEST_RUN_DIR: &str = "/tmp/nipc_apps_lookup_rust_test";
const AUTH_TOKEN: u64 = 0xDEADBEEFCAFEBABE;
const RESPONSE_BUF_SIZE: u32 = 65536;
static SERVICE_COUNTER: AtomicU64 = AtomicU64::new(0);

fn ensure_run_dir() {
    let _ = std::fs::create_dir_all(TEST_RUN_DIR);
}

fn unique_service(prefix: &str) -> String {
    let stamp = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_nanos();
    format!(
        "{}_{}_{}_{}",
        prefix,
        std::process::id(),
        SERVICE_COUNTER.fetch_add(1, Ordering::Relaxed) + 1,
        stamp
    )
}

fn socket_path(service: &str) -> PathBuf {
    PathBuf::from(format!("{TEST_RUN_DIR}/{service}.sock"))
}

fn cleanup(service: &str) {
    let _ = std::fs::remove_file(socket_path(service));
}

fn wait_for_listener_bind(service: &str) {
    let sock = socket_path(service);
    for _ in 0..2000 {
        if sock.exists() {
            return;
        }
        thread::sleep(Duration::from_micros(500));
    }
    panic!("listener did not bind for service {service}");
}

fn server_config() -> ServerConfig {
    ServerConfig {
        supported_profiles: PROFILE_BASELINE,
        preferred_profiles: PROFILE_BASELINE,
        max_request_payload_bytes: 4096,
        max_request_batch_items: 1,
        max_response_payload_bytes: RESPONSE_BUF_SIZE,
        auth_token: AUTH_TOKEN,
    }
}

fn client_config() -> ClientConfig {
    ClientConfig {
        supported_profiles: PROFILE_BASELINE,
        preferred_profiles: PROFILE_BASELINE,
        max_request_payload_bytes: 4096,
        max_request_batch_items: 1,
        max_response_payload_bytes: RESPONSE_BUF_SIZE,
        auth_token: AUTH_TOKEN,
        max_logical_lookup_items: 64,
        max_logical_lookup_subcalls: 8,
        max_logical_lookup_response_bytes: RESPONSE_BUF_SIZE,
    }
}

fn handler() -> Handler {
    Handler {
        handle: Some(Arc::new(
            |req: &AppsLookupRequestView<'_>, builder: &mut AppsLookupBuilder<'_>| {
                for i in 0..req.item_count {
                    let pid = match req.item(i) {
                        Ok(pid) => pid,
                        Err(_) => return false,
                    };
                    let added = match pid {
                        1234 => builder.add(
                            PID_LOOKUP_KNOWN,
                            APPS_CGROUP_KNOWN,
                            ORCHESTRATOR_DOCKER,
                            pid,
                            1,
                            1000,
                            42,
                            b"nginx",
                            b"/docker/abc",
                            b"container-a",
                            &[(b"image".as_slice(), b"nginx:latest".as_slice())],
                        ),
                        0 => builder.add(
                            PID_LOOKUP_KNOWN,
                            APPS_CGROUP_HOST_ROOT,
                            0,
                            pid,
                            0,
                            0,
                            0,
                            b"swapper",
                            b"",
                            b"",
                            &[],
                        ),
                        _ => builder.add(
                            PID_LOOKUP_UNKNOWN,
                            APPS_CGROUP_KNOWN,
                            0,
                            pid,
                            0,
                            NIPC_UID_UNSET,
                            0,
                            b"",
                            b"",
                            b"",
                            &[],
                        ),
                    };
                    if added.is_err() {
                        return false;
                    }
                }
                true
            },
        )),
    }
}

struct TestServer {
    stop_flag: Arc<std::sync::atomic::AtomicBool>,
    thread: Option<thread::JoinHandle<()>>,
}

impl TestServer {
    fn start(service: &str) -> Self {
        ensure_run_dir();
        cleanup(service);

        let mut server = ManagedServer::new(TEST_RUN_DIR, service, server_config(), handler());
        let stop_flag = server.running_flag();
        let thread = thread::spawn(move || {
            let _ = server.run();
        });
        wait_for_listener_bind(service);
        Self {
            stop_flag,
            thread: Some(thread),
        }
    }
}

impl Drop for TestServer {
    fn drop(&mut self) {
        self.stop_flag
            .store(false, std::sync::atomic::Ordering::Release);
        if let Some(thread) = self.thread.take() {
            let _ = thread.join();
        }
    }
}

fn connect_ready(client: &mut AppsLookupClient) {
    for _ in 0..200 {
        client.refresh();
        if client.ready() {
            return;
        }
        thread::sleep(Duration::from_millis(10));
    }
    panic!("client did not reach READY state");
}

#[test]
fn test_apps_lookup_facade_round_trip_unix() {
    let service = unique_service("apps_lookup_facade");
    let _server = TestServer::start(&service);

    let mut client = AppsLookupClient::new(TEST_RUN_DIR, &service, client_config());
    connect_ready(&mut client);

    let view = client.call(&[1234, 0, 9999]).expect("apps lookup");
    assert_eq!(view.item_count, 3);

    let item0 = view.item(0).expect("item 0");
    assert_eq!(item0.pid, 1234);
    assert_eq!(item0.status, PID_LOOKUP_KNOWN);
    assert_eq!(item0.cgroup_status, APPS_CGROUP_KNOWN);
    assert_eq!(item0.comm.as_bytes(), b"nginx");
    assert_eq!(item0.cgroup_path.as_bytes(), b"/docker/abc");
    assert_eq!(item0.label_count, 1);

    let item1 = view.item(1).expect("item 1");
    assert_eq!(item1.pid, 0);
    assert_eq!(item1.cgroup_status, APPS_CGROUP_HOST_ROOT);

    let item2 = view.item(2).expect("item 2");
    assert_eq!(item2.pid, 9999);
    assert_eq!(item2.status, PID_LOOKUP_UNKNOWN);
}

#[test]
fn test_apps_lookup_facade_not_ready_and_abort_unix() {
    let service = unique_service("apps_lookup_not_ready");
    ensure_run_dir();
    cleanup(&service);

    let mut client = AppsLookupClient::new(TEST_RUN_DIR, &service, client_config());
    assert!(!client.ready());
    assert!(client.refresh());
    assert_eq!(client.status().state, ClientState::NotFound);

    client.set_call_timeout(1);
    let abort = client.abort_handle();
    abort.abort();
    assert!(matches!(
        client.call_with_timeout(&[1], 1),
        Err(NipcError::BadLayout)
    ));
    client.clear_abort();
    client.close();
    assert_eq!(client.status().state, ClientState::Disconnected);
}

#[test]
fn test_apps_lookup_facade_applies_logical_item_limit_unix() {
    let mut cfg = client_config();
    cfg.max_logical_lookup_items = 2;

    let mut client = AppsLookupClient::new(TEST_RUN_DIR, "apps-lookup-logical-limit", cfg);
    assert!(matches!(client.call(&[1, 2, 3]), Err(NipcError::Overflow)));
}

#[test]
fn test_apps_lookup_managed_server_new_stopped_unix() {
    let server = ManagedServer::new(
        TEST_RUN_DIR,
        "apps-lookup-facade-new-unix",
        ServerConfig::default(),
        Handler::default(),
    );
    let running = server.running_flag();
    assert!(!running.load(std::sync::atomic::Ordering::SeqCst));
    server.stop();
    assert!(!running.load(std::sync::atomic::Ordering::SeqCst));
}
