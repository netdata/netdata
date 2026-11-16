use super::*;
use crate::protocol::{
    CGROUP_LOOKUP_KNOWN, CGROUP_LOOKUP_UNKNOWN_RETRY_LATER, CgroupsLookupBuilder,
    CgroupsLookupRequestView, ORCHESTRATOR_K8S, PROFILE_BASELINE,
};
use std::path::PathBuf;
use std::sync::Arc;
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::{SystemTime, UNIX_EPOCH};

const TEST_RUN_DIR: &str = "/tmp/nipc_cgroups_lookup_rust_test";
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

fn socket_path(service: &str) -> PathBuf {
    PathBuf::from(format!("{TEST_RUN_DIR}/{service}.sock"))
}

fn cleanup(service: &str) {
    let _ = std::fs::remove_file(socket_path(service));
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
            |req: &CgroupsLookupRequestView<'_>, builder: &mut CgroupsLookupBuilder<'_>| {
                for i in 0..req.item_count {
                    let item = match req.item(i) {
                        Ok(item) => item,
                        Err(_) => return false,
                    };
                    let added = if item.as_bytes() == b"/known" {
                        builder.add(
                            CGROUP_LOOKUP_KNOWN,
                            ORCHESTRATOR_K8S,
                            item.as_bytes(),
                            b"pod-a",
                            &[(b"namespace".as_slice(), b"default".as_slice())],
                        )
                    } else {
                        builder.add(
                            CGROUP_LOOKUP_UNKNOWN_RETRY_LATER,
                            0,
                            item.as_bytes(),
                            b"",
                            &[],
                        )
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
    _inner: raw::RunningTestServer,
}

impl TestServer {
    fn start(service: &str) -> Self {
        ensure_run_dir();
        cleanup(service);
        let server = ManagedServer::new(TEST_RUN_DIR, service, server_config(), handler());
        Self {
            _inner: server.inner.spawn_for_test(),
        }
    }
}

/// The server listens before tests connect, so one refresh must succeed.
fn connect_ready(client: &mut CgroupsLookupClient) {
    client.refresh();
    assert!(client.ready(), "client did not reach READY state");
}

#[test]
fn test_cgroups_lookup_facade_round_trip_unix() {
    let service = unique_service("cgroups_lookup_facade");
    let _server = TestServer::start(&service);

    let mut client = CgroupsLookupClient::new(TEST_RUN_DIR, &service, client_config());
    connect_ready(&mut client);

    let paths: [&[u8]; 2] = [b"/known".as_slice(), b"/missing".as_slice()];
    let view = client.call(&paths).expect("cgroups lookup");
    assert_eq!(view.item_count, 2);

    let item0 = view.item(0).expect("item 0");
    assert_eq!(item0.status, CGROUP_LOOKUP_KNOWN);
    assert_eq!(item0.orchestrator, ORCHESTRATOR_K8S);
    assert_eq!(item0.path.as_bytes(), b"/known");
    assert_eq!(item0.name.as_bytes(), b"pod-a");
    assert_eq!(item0.label_count, 1);

    let item1 = view.item(1).expect("item 1");
    assert_eq!(item1.status, CGROUP_LOOKUP_UNKNOWN_RETRY_LATER);
    assert_eq!(item1.path.as_bytes(), b"/missing");
    assert_eq!(item1.name.as_bytes(), b"");
}

#[test]
fn test_cgroups_lookup_facade_not_ready_and_abort_unix() {
    let service = unique_service("cgroups_lookup_not_ready");
    ensure_run_dir();
    cleanup(&service);

    let mut client = CgroupsLookupClient::new(TEST_RUN_DIR, &service, client_config());
    assert!(!client.ready());
    assert!(client.refresh());
    assert_eq!(client.status().state, ClientState::NotFound);

    client.set_call_timeout(1);
    let abort = client.abort_handle();
    abort.abort();
    let paths: [&[u8]; 1] = [b"/x".as_slice()];
    assert!(matches!(
        client.call_with_timeout(&paths, 1),
        Err(NipcError::NotReady)
    ));
    client.clear_abort();
    client.close();
    assert_eq!(client.status().state, ClientState::Disconnected);
}

#[test]
fn test_cgroups_lookup_facade_applies_logical_item_limit_unix() {
    let mut cfg = client_config();
    cfg.max_logical_lookup_items = 1;

    let paths: [&[u8]; 2] = [b"/a".as_slice(), b"/b".as_slice()];
    let mut client = CgroupsLookupClient::new(TEST_RUN_DIR, "cgroups-lookup-logical-limit", cfg);
    assert!(matches!(client.call(&paths), Err(NipcError::Overflow)));
}

#[test]
fn test_cgroups_lookup_managed_server_new_stopped_unix() {
    let server = ManagedServer::new(
        TEST_RUN_DIR,
        "cgroups-lookup-facade-new-unix",
        ServerConfig::default(),
        Handler::default(),
    );
    let running = server.running_flag();
    assert!(!running.load(std::sync::atomic::Ordering::SeqCst));
    server.stop();
    assert!(!running.load(std::sync::atomic::Ordering::SeqCst));
}
