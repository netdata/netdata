use super::*;
use crate::protocol::{
    increment_encode, BatchBuilder, CgroupsBuilder, CgroupsRequest, Header, HelloAck, NipcError,
    CODE_HELLO_ACK, FLAG_BATCH, HEADER_SIZE, INCREMENT_PAYLOAD_SIZE, KIND_CONTROL, KIND_REQUEST,
    KIND_RESPONSE, METHOD_CGROUPS_SNAPSHOT, METHOD_INCREMENT, METHOD_STRING_REVERSE,
    PROFILE_BASELINE, PROFILE_SHM_HYBRID, STATUS_INTERNAL_ERROR, STATUS_OK, VERSION,
};
use crate::transport::windows::build_pipe_name;
use std::ptr;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::Arc;
use std::thread;
use std::time::Duration;

const TEST_RUN_DIR: &str = r"C:\Temp\nipc_svc_rust_test";
const AUTH_TOKEN: u64 = 0xDEADBEEFCAFEBABE;
const RESPONSE_BUF_SIZE: usize = 65536;
static WIN_SERVICE_COUNTER: AtomicU64 = AtomicU64::new(0);

fn ensure_run_dir() {
    let _ = std::fs::create_dir_all(TEST_RUN_DIR);
}

fn cleanup_all(_service: &str) {}

fn unique_service(prefix: &str) -> String {
    format!(
        "{}_{}_{}",
        prefix,
        std::process::id(),
        WIN_SERVICE_COUNTER.fetch_add(1, Ordering::Relaxed) + 1
    )
}

fn server_config() -> ServerConfig {
    ServerConfig {
        supported_profiles: PROFILE_BASELINE,
        max_request_payload_bytes: 4096,
        max_request_batch_items: 1,
        max_response_payload_bytes: RESPONSE_BUF_SIZE as u32,
        max_response_batch_items: 1,
        auth_token: AUTH_TOKEN,
        ..ServerConfig::default()
    }
}

fn client_config() -> ClientConfig {
    ClientConfig {
        supported_profiles: PROFILE_BASELINE,
        max_request_payload_bytes: 4096,
        max_request_batch_items: 1,
        max_response_payload_bytes: RESPONSE_BUF_SIZE as u32,
        max_response_batch_items: 1,
        auth_token: AUTH_TOKEN,
        ..ClientConfig::default()
    }
}

fn shm_server_config() -> ServerConfig {
    ServerConfig {
        supported_profiles: PROFILE_SHM_HYBRID | PROFILE_BASELINE,
        max_request_payload_bytes: 4096,
        max_request_batch_items: 1,
        max_response_payload_bytes: RESPONSE_BUF_SIZE as u32,
        max_response_batch_items: 1,
        auth_token: AUTH_TOKEN,
        ..ServerConfig::default()
    }
}

fn shm_client_config() -> ClientConfig {
    ClientConfig {
        supported_profiles: PROFILE_SHM_HYBRID | PROFILE_BASELINE,
        max_request_payload_bytes: 4096,
        max_request_batch_items: 1,
        max_response_payload_bytes: RESPONSE_BUF_SIZE as u32,
        max_response_batch_items: 1,
        auth_token: AUTH_TOKEN,
        ..ClientConfig::default()
    }
}

fn batch_server_config() -> ServerConfig {
    ServerConfig {
        supported_profiles: PROFILE_BASELINE,
        max_request_payload_bytes: 4096,
        max_request_batch_items: 16,
        max_response_payload_bytes: RESPONSE_BUF_SIZE as u32,
        max_response_batch_items: 16,
        auth_token: AUTH_TOKEN,
        ..ServerConfig::default()
    }
}

fn batch_client_config() -> ClientConfig {
    ClientConfig {
        supported_profiles: PROFILE_BASELINE,
        max_request_payload_bytes: 4096,
        max_request_batch_items: 16,
        max_response_payload_bytes: RESPONSE_BUF_SIZE as u32,
        max_response_batch_items: 16,
        auth_token: AUTH_TOKEN,
        ..ClientConfig::default()
    }
}

fn snapshot_client(service: &str, config: ClientConfig) -> RawClient {
    RawClient::new_snapshot(TEST_RUN_DIR, service, config)
}

fn increment_client(service: &str, config: ClientConfig) -> RawClient {
    RawClient::new_increment(TEST_RUN_DIR, service, config)
}

fn string_reverse_client(service: &str, config: ClientConfig) -> RawClient {
    RawClient::new_string_reverse(TEST_RUN_DIR, service, config)
}

fn fill_test_cgroups_snapshot(builder: &mut CgroupsBuilder<'_>) -> bool {
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

fn test_cgroups_dispatch() -> DispatchHandler {
    snapshot_dispatch(
        Arc::new(|req, builder| {
            if req.layout_version != 1 || req.flags != 0 {
                return false;
            }
            builder.set_header(1, 42);
            fill_test_cgroups_snapshot(builder)
        }),
        3,
    )
}

fn increment_dispatch_handler() -> DispatchHandler {
    increment_dispatch(Arc::new(|value| Some(value + 1)))
}

fn connect_ready(client: &mut RawClient) {
    for _ in 0..200 {
        client.refresh();
        if client.ready() {
            return;
        }
        thread::sleep(Duration::from_millis(10));
    }

    panic!("client did not reach READY state");
}

fn wait_for_state(client: &mut RawClient, want: ClientState) {
    for _ in 0..200 {
        client.refresh();
        if client.state == want {
            return;
        }
        thread::sleep(Duration::from_millis(10));
    }

    panic!("client did not reach state {:?}", want);
}

struct TestServer {
    service: String,
    wake_config: ClientConfig,
    stop_flag: Arc<AtomicBool>,
    thread: Option<thread::JoinHandle<()>>,
}

impl TestServer {
    fn start(service: &str, expected_method_code: u16, handler: DispatchHandler) -> Self {
        Self::start_with(
            service,
            server_config(),
            client_config(),
            expected_method_code,
            handler,
            8,
        )
    }

    fn start_shm(service: &str, expected_method_code: u16, handler: DispatchHandler) -> Self {
        Self::start_with(
            service,
            shm_server_config(),
            shm_client_config(),
            expected_method_code,
            handler,
            8,
        )
    }

    fn start_batch(service: &str, expected_method_code: u16, handler: DispatchHandler) -> Self {
        Self::start_with(
            service,
            batch_server_config(),
            batch_client_config(),
            expected_method_code,
            handler,
            8,
        )
    }

    fn start_with(
        service: &str,
        config: ServerConfig,
        wake_config: ClientConfig,
        expected_method_code: u16,
        handler: DispatchHandler,
        worker_count: usize,
    ) -> Self {
        ensure_run_dir();
        cleanup_all(service);

        let svc = service.to_string();
        let stop_cfg = wake_config.clone();
        let mut server = ManagedServer::with_workers(
            TEST_RUN_DIR,
            &svc,
            config,
            expected_method_code,
            Some(handler),
            worker_count,
        );
        let stop_flag = server.running_flag();

        let thread = thread::spawn(move || {
            let _ = server.run();
        });
        thread::sleep(Duration::from_millis(50));

        TestServer {
            service: svc,
            wake_config: stop_cfg,
            stop_flag,
            thread: Some(thread),
        }
    }

    fn stop(&mut self) {
        self.stop_flag.store(false, Ordering::Release);

        // Wake a blocking ConnectNamedPipe() so the accept loop can observe
        // the stop flag and exit.
        let _ = NpSession::connect(TEST_RUN_DIR, &self.service, &self.wake_config);

        if let Some(handle) = self.thread.take() {
            let _ = handle.join();
        }

        cleanup_all(&self.service);
    }
}

impl Drop for TestServer {
    fn drop(&mut self) {
        self.stop();
    }
}

struct RawSessionServer {
    thread: Option<thread::JoinHandle<Result<(), String>>>,
}

type RawHandle = isize;
type Dword = u32;
type Bool = i32;

const INVALID_HANDLE_VALUE: RawHandle = -1;
const PIPE_ACCESS_DUPLEX: Dword = 0x00000003;
const PIPE_TYPE_MESSAGE: Dword = 0x00000004;
const PIPE_READMODE_MESSAGE: Dword = 0x00000002;
const PIPE_WAIT: Dword = 0x00000000;
const ERROR_PIPE_CONNECTED: Dword = 535;
const RAW_PIPE_PACKET_SIZE: Dword = 65536;

unsafe extern "system" {
    fn CreateNamedPipeW(
        lp_name: *const u16,
        dw_open_mode: Dword,
        dw_pipe_mode: Dword,
        n_max_instances: Dword,
        n_out_buffer_size: Dword,
        n_in_buffer_size: Dword,
        n_default_time_out: Dword,
        lp_security_attributes: *const core::ffi::c_void,
    ) -> RawHandle;

    fn ConnectNamedPipe(handle: RawHandle, overlapped: *mut core::ffi::c_void) -> Bool;
    fn ReadFile(
        handle: RawHandle,
        buffer: *mut core::ffi::c_void,
        bytes_to_read: Dword,
        bytes_read: *mut Dword,
        overlapped: *mut core::ffi::c_void,
    ) -> Bool;
    fn WriteFile(
        handle: RawHandle,
        buffer: *const core::ffi::c_void,
        bytes_to_write: Dword,
        bytes_written: *mut Dword,
        overlapped: *mut core::ffi::c_void,
    ) -> Bool;
    fn CloseHandle(handle: RawHandle) -> Bool;
    fn GetLastError() -> Dword;
}

struct RawHelloAckServer {
    accepted: Arc<AtomicBool>,
    thread: Option<thread::JoinHandle<Result<(), String>>>,
}

fn encode_hello_ack_packet_with_version(version: u16, status: u16, layout_version: u16) -> Vec<u8> {
    let ack = HelloAck {
        layout_version,
        flags: 0,
        server_supported_profiles: PROFILE_BASELINE,
        intersection_profiles: PROFILE_BASELINE,
        selected_profile: PROFILE_BASELINE,
        agreed_max_request_payload_bytes: crate::protocol::MAX_PAYLOAD_DEFAULT,
        agreed_max_request_batch_items: 1,
        agreed_max_response_payload_bytes: RESPONSE_BUF_SIZE as u32,
        agreed_max_response_batch_items: 1,
        agreed_packet_size: 0,
        session_id: 77,
    };

    let mut payload = [0u8; 48];
    let payload_len = ack.encode(&mut payload);

    let hdr = Header {
        magic: crate::protocol::MAGIC_MSG,
        version,
        header_len: HEADER_SIZE as u16,
        kind: KIND_CONTROL,
        code: CODE_HELLO_ACK,
        flags: 0,
        item_count: 1,
        message_id: 0,
        payload_len: payload_len as u32,
        transport_status: status,
    };

    let mut packet = vec![0u8; HEADER_SIZE + payload_len];
    hdr.encode(&mut packet[..HEADER_SIZE]);
    packet[HEADER_SIZE..].copy_from_slice(&payload[..payload_len]);
    packet
}

fn start_raw_hello_ack_version_server(service: &str, version: u16) -> RawHelloAckServer {
    ensure_run_dir();
    cleanup_all(service);

    let accepted = Arc::new(AtomicBool::new(false));
    let accepted_flag = Arc::clone(&accepted);
    let svc = service.to_string();
    let packet = encode_hello_ack_packet_with_version(version, STATUS_OK, 1);

    let thread = thread::spawn(move || {
        let pipe_name =
            build_pipe_name(TEST_RUN_DIR, &svc).map_err(|e| format!("pipe name: {e}"))?;
        let pipe = unsafe {
            CreateNamedPipeW(
                pipe_name.as_ptr(),
                PIPE_ACCESS_DUPLEX,
                PIPE_TYPE_MESSAGE | PIPE_READMODE_MESSAGE | PIPE_WAIT,
                1,
                RAW_PIPE_PACKET_SIZE,
                RAW_PIPE_PACKET_SIZE,
                0,
                ptr::null(),
            )
        };
        if pipe == INVALID_HANDLE_VALUE {
            return Err(format!("CreateNamedPipeW failed: {}", unsafe {
                GetLastError()
            }));
        }

        let connect_ok = unsafe { ConnectNamedPipe(pipe, ptr::null_mut()) };
        if connect_ok == 0 {
            let err = unsafe { GetLastError() };
            if err != ERROR_PIPE_CONNECTED {
                unsafe {
                    CloseHandle(pipe);
                }
                return Err(format!("ConnectNamedPipe failed: {err}"));
            }
        }

        accepted_flag.store(true, Ordering::Release);

        let mut hello_buf = [0u8; 256];
        let mut hello_n = 0u32;
        let read_ok = unsafe {
            ReadFile(
                pipe,
                hello_buf.as_mut_ptr().cast(),
                hello_buf.len() as u32,
                &mut hello_n,
                ptr::null_mut(),
            )
        };
        if read_ok == 0 || hello_n == 0 {
            unsafe {
                CloseHandle(pipe);
            }
            return Err(format!("ReadFile failed: {}", unsafe { GetLastError() }));
        }

        let mut written = 0u32;
        let write_ok = unsafe {
            WriteFile(
                pipe,
                packet.as_ptr().cast(),
                packet.len() as u32,
                &mut written,
                ptr::null_mut(),
            )
        };
        let write_err = unsafe { GetLastError() };
        unsafe {
            CloseHandle(pipe);
        }
        if write_ok == 0 || written as usize != packet.len() {
            return Err(format!("WriteFile failed: {write_err}"));
        }

        Ok(())
    });

    thread::sleep(Duration::from_millis(200));
    RawHelloAckServer {
        accepted,
        thread: Some(thread),
    }
}

impl RawHelloAckServer {
    fn wait(&mut self) {
        if let Some(thread) = self.thread.take() {
            match thread.join() {
                Ok(Ok(())) => {}
                Ok(Err(err)) => panic!("raw windows hello_ack server failed: {err}"),
                Err(_) => panic!("raw windows hello_ack server panicked"),
            }
        }

        assert!(
            self.accepted.load(Ordering::Acquire),
            "raw windows hello_ack server accepted no clients"
        );
    }
}

fn start_raw_session_server<F>(service: &str, cfg: ServerConfig, handler: F) -> RawSessionServer
where
    F: FnOnce(&mut NpSession, Header, &[u8]) -> Result<(), String> + Send + 'static,
{
    ensure_run_dir();
    cleanup_all(service);

    let svc = service.to_string();
    let thread = thread::spawn(move || {
        let mut listener =
            NpListener::bind(TEST_RUN_DIR, &svc, cfg).map_err(|e| format!("bind: {e}"))?;
        let mut session = listener.accept().map_err(|e| format!("accept: {e}"))?;

        let (hdr, payload) = {
            let mut recv_buf = vec![0u8; RESPONSE_BUF_SIZE];
            let (hdr, payload) = session
                .receive(&mut recv_buf)
                .map_err(|e| format!("receive: {e}"))?;
            (hdr, payload.to_vec())
        };

        handler(&mut session, hdr, &payload)
    });

    thread::sleep(Duration::from_millis(200));
    RawSessionServer {
        thread: Some(thread),
    }
}

impl RawSessionServer {
    fn wait(&mut self) {
        if let Some(thread) = self.thread.take() {
            match thread.join() {
                Ok(Ok(())) => {}
                Ok(Err(err)) => panic!("raw windows session server failed: {err}"),
                Err(_) => panic!("raw windows session server panicked"),
            }
        }
    }
}

#[test]
fn test_client_lifecycle_windows() {
    let svc = "rs_win_svc_lifecycle";
    ensure_run_dir();
    cleanup_all(svc);

    let mut client = snapshot_client(svc, client_config());
    assert_eq!(client.state, ClientState::Disconnected);
    assert!(!client.ready());

    let changed = client.refresh();
    assert!(changed);
    assert_eq!(client.state, ClientState::NotFound);

    let mut server = TestServer::start(svc, METHOD_CGROUPS_SNAPSHOT, test_cgroups_dispatch());

    connect_ready(&mut client);
    assert_eq!(client.state, ClientState::Ready);
    assert!(client.ready());
    assert_eq!(client.status().connect_count, 1);

    client.close();
    assert_eq!(client.state, ClientState::Disconnected);

    server.stop();
}

#[test]
fn test_cgroups_call_windows_baseline() {
    let svc = "rs_win_svc_cgroups";
    let mut server = TestServer::start(svc, METHOD_CGROUPS_SNAPSHOT, test_cgroups_dispatch());

    let mut client = snapshot_client(svc, client_config());
    connect_ready(&mut client);

    let view = client.call_snapshot().expect("snapshot");
    assert_eq!(view.item_count, 3);
    assert_eq!(view.systemd_enabled, 1);
    assert_eq!(view.generation, 42);

    let item0 = view.item(0).expect("item 0");
    assert_eq!(item0.hash, 1001);
    assert_eq!(item0.name.as_bytes(), b"docker-abc123");

    client.close();
    server.stop();
}

#[test]
fn test_cgroups_call_windows_shm() {
    let svc = "rs_win_svc_cgroups_shm";
    let mut server = TestServer::start_shm(svc, METHOD_CGROUPS_SNAPSHOT, test_cgroups_dispatch());

    let mut client = snapshot_client(svc, shm_client_config());
    connect_ready(&mut client);

    assert!(client.shm.is_some(), "expected Win SHM to be negotiated");
    assert_eq!(
        client.session.as_ref().map(|s| s.selected_profile),
        Some(PROFILE_SHM_HYBRID)
    );

    let view = client.call_snapshot().expect("snapshot");
    assert_eq!(view.item_count, 3);
    assert_eq!(view.generation, 42);

    client.close();
    server.stop();
}

#[test]
fn test_retry_on_failure_windows() {
    let svc = unique_service("rs_win_svc_retry");
    let mut server1 = TestServer::start(&svc, METHOD_CGROUPS_SNAPSHOT, test_cgroups_dispatch());

    let mut client = snapshot_client(&svc, client_config());
    connect_ready(&mut client);

    let view = client.call_snapshot().expect("first call");
    assert_eq!(view.item_count, 3);

    let (stop_tx, stop_rx) = std::sync::mpsc::channel();
    thread::spawn(move || {
        server1.stop();
        let _ = stop_tx.send(());
    });
    assert!(
        stop_rx.recv_timeout(Duration::from_secs(2)).is_ok(),
        "server stop should not hang with an active client session"
    );
    thread::sleep(Duration::from_millis(100));

    let mut server2 = TestServer::start(&svc, METHOD_CGROUPS_SNAPSHOT, test_cgroups_dispatch());

    let view2 = client.call_snapshot().expect("retry call");
    assert_eq!(view2.item_count, 3);
    assert!(client.status().reconnect_count >= 1);

    client.close();
    server2.stop();
}

#[test]
fn test_non_request_terminates_session_windows() {
    let svc = "rs_win_svc_nonreq";
    let mut server = TestServer::start(svc, METHOD_CGROUPS_SNAPSHOT, test_cgroups_dispatch());

    let mut session = NpSession::connect(TEST_RUN_DIR, svc, &client_config()).expect("connect");

    let mut hdr = Header {
        kind: KIND_RESPONSE,
        code: METHOD_CGROUPS_SNAPSHOT,
        flags: 0,
        item_count: 0,
        message_id: 1,
        transport_status: STATUS_OK,
        ..Header::default()
    };
    let send_result = session.send(&mut hdr, &[]);
    if send_result.is_ok() {
        thread::sleep(Duration::from_millis(100));
        let mut recv_buf = vec![0u8; 4096];
        let mut hdr2 = Header {
            kind: KIND_REQUEST,
            code: METHOD_CGROUPS_SNAPSHOT,
            flags: 0,
            item_count: 1,
            message_id: 2,
            transport_status: STATUS_OK,
            ..Header::default()
        };
        let req = CgroupsRequest {
            layout_version: 1,
            flags: 0,
        };
        let mut req_buf = [0u8; 4];
        req.encode(&mut req_buf);
        let _ = session.send(&mut hdr2, &req_buf);
        let recv = session.receive(&mut recv_buf);
        assert!(
            recv.is_err(),
            "server should terminate the offending session"
        );
    }

    drop(session);

    let mut verify = snapshot_client(svc, client_config());
    connect_ready(&mut verify);
    let view = verify.call_snapshot().expect("normal call");
    assert_eq!(view.item_count, 3);

    verify.close();
    server.stop();
}

#[test]
fn test_cache_full_round_trip_windows() {
    let svc = "rs_win_cache_roundtrip";
    let mut server = TestServer::start(svc, METHOD_CGROUPS_SNAPSHOT, test_cgroups_dispatch());

    let mut cache = CgroupsCache::new(TEST_RUN_DIR, svc, client_config());
    assert!(!cache.ready());

    thread::sleep(Duration::from_millis(2));

    let updated = cache.refresh();
    assert!(updated);
    assert!(cache.ready());

    let item = cache.lookup(1001, "docker-abc123").expect("lookup");
    assert_eq!(item.hash, 1001);
    assert_eq!(item.path, "/sys/fs/cgroup/docker/abc123");

    let status = cache.status();
    assert!(status.populated);
    assert_eq!(status.item_count, 3);
    assert_eq!(status.generation, 42);
    assert_eq!(status.connection_state, ClientState::Ready);

    cache.close();
    server.stop();
}

#[test]
fn test_increment_ping_pong_windows() {
    let svc = "rs_win_pp_increment";
    let mut server = TestServer::start(svc, METHOD_INCREMENT, increment_dispatch_handler());

    let mut client = increment_client(svc, client_config());
    connect_ready(&mut client);

    let mut value = 0u64;
    for _ in 0..10 {
        value = client.call_increment(value).expect("increment");
    }
    assert_eq!(value, 10);

    client.close();
    server.stop();
}

#[test]
fn test_increment_batch_windows() {
    let svc = "rs_win_pp_batch";
    let mut server = TestServer::start_batch(svc, METHOD_INCREMENT, increment_dispatch_handler());

    let mut client = increment_client(svc, batch_client_config());
    connect_ready(&mut client);

    let values = vec![10u64, 20, 30, 40];
    let results = client.call_increment_batch(&values).expect("batch call");
    assert_eq!(results, vec![11, 21, 31, 41]);

    let single = client
        .call_increment_batch(&[99])
        .expect("single-item batch");
    assert_eq!(single, vec![100]);

    client.close();
    server.stop();
}

#[test]
fn test_client_auth_failure_windows() {
    let svc = "rs_win_svc_authfail";
    let mut server = TestServer::start(svc, METHOD_CGROUPS_SNAPSHOT, test_cgroups_dispatch());

    let mut bad_cfg = client_config();
    bad_cfg.auth_token = 0xBAD_BAD_BAD;

    let mut client = snapshot_client(svc, bad_cfg);
    wait_for_state(&mut client, ClientState::AuthFailed);
    assert_eq!(client.state, ClientState::AuthFailed);
    assert!(!client.ready());

    client.refresh();
    assert_eq!(client.state, ClientState::AuthFailed);

    client.close();
    server.stop();
}

#[test]
fn test_refresh_winshm_attach_failure_falls_back_to_baseline() {
    let svc = unique_service("rs_win_svc_shm_attach_fail");
    ensure_run_dir();
    cleanup_all(&svc);

    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();
    let server_svc = svc.clone();
    let server_thread = thread::spawn(move || -> Result<(), String> {
        let mut listener = NpListener::bind(TEST_RUN_DIR, &server_svc, shm_server_config())
            .map_err(|e| format!("bind: {e}"))?;
        ready_clone.store(true, Ordering::Release);

        let mut first = listener
            .accept()
            .map_err(|e| format!("accept first: {e}"))?;
        if first.selected_profile != PROFILE_SHM_HYBRID {
            return Err(format!(
                "first selected profile = {}, want {}",
                first.selected_profile, PROFILE_SHM_HYBRID
            ));
        }

        let mut recv_buf = vec![0u8; RESPONSE_BUF_SIZE];
        if first.receive(&mut recv_buf).is_ok() {
            return Err("first SHM-selected session should disconnect after attach failure".into());
        }

        let mut second = listener
            .accept()
            .map_err(|e| format!("accept second: {e}"))?;
        if second.selected_profile != PROFILE_BASELINE {
            return Err(format!(
                "second selected profile = {}, want {}",
                second.selected_profile, PROFILE_BASELINE
            ));
        }

        if second.receive(&mut recv_buf).is_ok() {
            return Err("second baseline session should close cleanly when client closes".into());
        }

        Ok(())
    });

    while !ready.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(1));
    }

    let mut client = increment_client(&svc, shm_client_config());
    assert!(
        client.refresh(),
        "refresh should transition to READY via baseline fallback"
    );
    assert_eq!(client.state, ClientState::Ready);
    assert!(client.ready());
    assert!(
        client.shm.is_none(),
        "fallback session must not attach WinSHM"
    );
    assert_eq!(
        client.session.as_ref().map(|s| s.selected_profile),
        Some(PROFILE_BASELINE)
    );
    assert_eq!(client.transport_config.supported_profiles, PROFILE_BASELINE);
    assert_eq!(client.transport_config.preferred_profiles, 0);

    client.close();
    match server_thread.join() {
        Ok(Ok(())) => {}
        Ok(Err(err)) => panic!("raw win attach-failure server failed: {err}"),
        Err(_) => panic!("raw win attach-failure server panicked"),
    }
}

#[test]
fn test_client_incompatible_windows() {
    let svc = "rs_win_svc_incompat";
    let mut server = TestServer::start(svc, METHOD_CGROUPS_SNAPSHOT, test_cgroups_dispatch());

    let mut bad_cfg = client_config();
    bad_cfg.supported_profiles = 0x80000000;

    let mut client = snapshot_client(svc, bad_cfg);
    wait_for_state(&mut client, ClientState::Incompatible);
    assert_eq!(client.state, ClientState::Incompatible);
    assert!(!client.ready());

    client.refresh();
    assert_eq!(client.state, ClientState::Incompatible);

    client.close();
    server.stop();
}

#[test]
fn test_client_protocol_version_incompatible_windows() {
    let svc = unique_service("rs_win_svc_proto_incompat");
    let mut server = start_raw_hello_ack_version_server(&svc, VERSION + 1);

    let mut client = snapshot_client(&svc, client_config());
    let changed = client.refresh();
    assert!(changed, "refresh should move client into INCOMPATIBLE");
    assert_eq!(client.state, ClientState::Incompatible);
    assert!(!client.ready());

    server.wait();

    let changed = client.refresh();
    assert!(!changed, "refresh from incompatible should be a no-op");
    assert_eq!(client.state, ClientState::Incompatible);

    client.close();
    cleanup_all(&svc);
}

#[test]
fn test_call_when_not_ready_windows() {
    let svc = "rs_win_svc_noready";
    let mut snapshot = snapshot_client(svc, client_config());
    assert_eq!(snapshot.state, ClientState::Disconnected);
    assert!(snapshot.call_snapshot().is_err());
    assert_eq!(snapshot.status().error_count, 1);
    snapshot.close();

    let mut increment = increment_client(svc, client_config());
    assert_eq!(increment.state, ClientState::Disconnected);
    assert!(increment.call_increment(42).is_err());
    assert_eq!(increment.status().error_count, 1);
    increment.close();

    let mut string_reverse = string_reverse_client(svc, client_config());
    assert_eq!(string_reverse.state, ClientState::Disconnected);
    assert!(string_reverse.call_string_reverse("test").is_err());
    assert_eq!(string_reverse.status().error_count, 1);
    string_reverse.close();
}

#[test]
fn test_server_worker_count_clamped_windows() {
    let svc = "rs_win_svc_w0";
    ensure_run_dir();
    cleanup_all(svc);

    let server = ManagedServer::with_workers(
        TEST_RUN_DIR,
        svc,
        server_config(),
        METHOD_INCREMENT,
        None,
        0,
    );
    assert_eq!(server.worker_count, 1);
}

#[test]
fn test_server_stop_flag_windows() {
    let svc = "rs_win_svc_stopflag";
    ensure_run_dir();
    cleanup_all(svc);

    let server = ManagedServer::new(TEST_RUN_DIR, svc, server_config(), METHOD_INCREMENT, None);
    let flag = server.running_flag();
    assert!(!flag.load(Ordering::Acquire));

    server.stop();
    assert!(!flag.load(Ordering::Acquire));
}

#[test]
fn test_transport_without_session_windows() {
    let svc = unique_service("rs_win_transport");
    let mut client = increment_client(&svc, client_config());

    let mut hdr = Header {
        kind: KIND_REQUEST,
        code: METHOD_INCREMENT,
        flags: 0,
        item_count: 1,
        message_id: 1,
        transport_status: STATUS_OK,
        ..Header::default()
    };

    assert_eq!(
        client.transport_send(&mut hdr, &[]),
        Err(NipcError::Truncated)
    );
    assert!(matches!(
        client.transport_receive(),
        Err(NipcError::Truncated)
    ));
    client.close();
}

#[test]
fn test_call_increment_rejects_malformed_response_envelope_windows() {
    struct Case {
        name: &'static str,
        kind: u16,
        code: u16,
        status: u16,
        want: NipcError,
    }

    let cases = [
        Case {
            name: "bad kind",
            kind: KIND_REQUEST,
            code: METHOD_INCREMENT,
            status: STATUS_OK,
            want: NipcError::BadKind,
        },
        Case {
            name: "bad code",
            kind: KIND_RESPONSE,
            code: METHOD_STRING_REVERSE,
            status: STATUS_OK,
            want: NipcError::BadLayout,
        },
        Case {
            name: "bad status",
            kind: KIND_RESPONSE,
            code: METHOD_INCREMENT,
            status: STATUS_INTERNAL_ERROR,
            want: NipcError::BadLayout,
        },
    ];

    for tc in cases {
        let svc = unique_service("rs_win_inc_env");
        let mut server =
            start_raw_session_server(&svc, server_config(), move |session, req_hdr, _| {
                let mut payload = [0u8; INCREMENT_PAYLOAD_SIZE];
                let n = increment_encode(43, &mut payload);
                if n != INCREMENT_PAYLOAD_SIZE {
                    return Err(format!("increment_encode returned {n}"));
                }

                let mut resp_hdr = Header {
                    kind: tc.kind,
                    code: tc.code,
                    flags: 0,
                    item_count: 1,
                    message_id: req_hdr.message_id,
                    transport_status: tc.status,
                    ..Header::default()
                };
                session
                    .send(&mut resp_hdr, &payload)
                    .map_err(|e| format!("send: {e}"))
            });

        let mut client = increment_client(&svc, client_config());
        connect_ready(&mut client);

        let err = client.call_increment(42).expect_err(tc.name);
        assert_eq!(err, tc.want, "{}", tc.name);

        client.close();
        server.wait();
    }
}

#[test]
fn test_call_increment_rejects_malformed_payload_windows() {
    let svc = unique_service("rs_win_inc_payload");
    let mut server = start_raw_session_server(&svc, server_config(), move |session, req_hdr, _| {
        let payload = [1u8, 2, 3, 4];
        let mut resp_hdr = Header {
            kind: KIND_RESPONSE,
            code: METHOD_INCREMENT,
            flags: 0,
            item_count: 1,
            message_id: req_hdr.message_id,
            transport_status: STATUS_OK,
            ..Header::default()
        };
        session
            .send(&mut resp_hdr, &payload)
            .map_err(|e| format!("send: {e}"))
    });

    let mut client = increment_client(&svc, client_config());
    connect_ready(&mut client);

    let err = client
        .call_increment(42)
        .expect_err("malformed increment response");
    assert_eq!(err, NipcError::Truncated);

    client.close();
    server.wait();
}

#[test]
fn test_call_string_reverse_rejects_missing_nul_windows() {
    let svc = unique_service("rs_win_str_payload");
    let mut server = start_raw_session_server(&svc, server_config(), move |session, req_hdr, _| {
        let payload = [
            8u8, 0, 0, 0, // str_offset = 8
            2, 0, 0, 0, // str_length = 2
            b'o', b'k', b'!', // missing trailing NUL
        ];
        let mut resp_hdr = Header {
            kind: KIND_RESPONSE,
            code: METHOD_STRING_REVERSE,
            flags: 0,
            item_count: 1,
            message_id: req_hdr.message_id,
            transport_status: STATUS_OK,
            ..Header::default()
        };
        session
            .send(&mut resp_hdr, &payload)
            .map_err(|e| format!("send: {e}"))
    });

    let mut client = string_reverse_client(&svc, client_config());
    connect_ready(&mut client);

    let err = client
        .call_string_reverse("ok")
        .expect_err("malformed string response");
    assert_eq!(err, NipcError::MissingNul);

    client.close();
    server.wait();
}

#[test]
fn test_call_increment_batch_rejects_wrong_item_count_windows() {
    let svc = unique_service("rs_win_batch_count");
    let mut server =
        start_raw_session_server(&svc, batch_server_config(), move |session, req_hdr, _| {
            let mut encoded = [0u8; INCREMENT_PAYLOAD_SIZE];
            let n = increment_encode(11, &mut encoded);
            if n != INCREMENT_PAYLOAD_SIZE {
                return Err(format!("increment_encode returned {n}"));
            }

            let mut response_buf = vec![0u8; 128];
            let resp_len = {
                let mut batch = BatchBuilder::new(&mut response_buf, 2);
                batch
                    .add(&encoded)
                    .map_err(|e| format!("batch add 1: {e:?}"))?;
                batch
                    .add(&encoded)
                    .map_err(|e| format!("batch add 2: {e:?}"))?;
                let (len, _count) = batch.finish();
                len
            };

            let mut resp_hdr = Header {
                kind: KIND_RESPONSE,
                code: METHOD_INCREMENT,
                flags: FLAG_BATCH,
                item_count: 1,
                message_id: req_hdr.message_id,
                transport_status: STATUS_OK,
                ..Header::default()
            };
            session
                .send(&mut resp_hdr, &response_buf[..resp_len])
                .map_err(|e| format!("send: {e}"))
        });

    let mut client = increment_client(&svc, batch_client_config());
    connect_ready(&mut client);

    let err = client
        .call_increment_batch(&[10, 20])
        .expect_err("wrong batch item_count");
    assert_eq!(err, NipcError::BadItemCount);

    client.close();
    server.wait();
}
