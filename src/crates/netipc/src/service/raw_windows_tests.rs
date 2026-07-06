use super::*;
use crate::protocol::{
    increment_encode, AppsLookupBuilder, AppsLookupRequestView, AppsLookupResponseView,
    BatchBuilder, CgroupsBuilder, CgroupsLookupBuilder, CgroupsLookupRequestView,
    CgroupsLookupResponseView, CgroupsRequest, Header, HelloAck, NipcError, APPS_CGROUP_HOST_ROOT,
    APPS_CGROUP_KNOWN, APPS_LOOKUP_RESP_HDR_SIZE, CGROUPS_LOOKUP_RESP_HDR_SIZE,
    CGROUP_LOOKUP_KNOWN, CGROUP_LOOKUP_OVERSIZED_ITEM, CGROUP_LOOKUP_PAYLOAD_EXCEEDED,
    CGROUP_LOOKUP_UNKNOWN_RETRY_LATER, CODE_HELLO_ACK, FLAG_BATCH, HEADER_SIZE,
    INCREMENT_PAYLOAD_SIZE, KIND_CONTROL, KIND_REQUEST, KIND_RESPONSE, MAX_PAYLOAD_CAP,
    METHOD_APPS_LOOKUP, METHOD_CGROUPS_LOOKUP, METHOD_CGROUPS_SNAPSHOT, METHOD_INCREMENT,
    METHOD_STRING_REVERSE, NIPC_UID_UNSET, ORCHESTRATOR_DOCKER, ORCHESTRATOR_K8S, PID_LOOKUP_KNOWN,
    PID_LOOKUP_OVERSIZED_ITEM, PID_LOOKUP_PAYLOAD_EXCEEDED, PID_LOOKUP_UNKNOWN, PROFILE_BASELINE,
    PROFILE_SHM_HYBRID, STATUS_INTERNAL_ERROR, STATUS_LIMIT_EXCEEDED, STATUS_OK, VERSION,
};
use crate::transport::windows::build_pipe_name;
use std::ptr;
use std::sync::atomic::{AtomicBool, AtomicU32, AtomicU64, Ordering};
use std::sync::mpsc;
use std::sync::Arc;
use std::thread;
use std::time::{Duration, Instant};

const TEST_RUN_DIR: &str = r"C:\Temp\nipc_svc_rust_test";
const AUTH_TOKEN: u64 = 0xDEADBEEFCAFEBABE;
const RESPONSE_BUF_SIZE: usize = 65536;
const LOOKUP_TOPOLOGY_SCALE_ITEMS: usize = 8192;
const LOOKUP_HPC_SCALE_ITEMS: usize = 32768;
const LOOKUP_SCALE_REQUEST_PAYLOAD_BYTES: u32 = 8192;
const LOOKUP_SCALE_CALL_TIMEOUT_MS: u32 = 120_000;
const LOOKUP_RESPONSE_SPLIT_SCALE_ITEMS: usize = 512;
const LOOKUP_RESPONSE_SPLIT_REQUEST_PAYLOAD_BYTES: u32 = 65_536;
const LOOKUP_RESPONSE_SPLIT_PAYLOAD_BYTES: u32 = 98_304;
const LOOKUP_RESPONSE_SPLIT_MIN_CALLS: u32 = 2;
const LOOKUP_RESPONSE_SPLIT_LABEL_BYTES: usize = 512;
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

fn cgroups_lookup_client(service: &str, config: ClientConfig) -> RawClient {
    RawClient::new_cgroups_lookup(TEST_RUN_DIR, service, config)
}

fn apps_lookup_client(service: &str, config: ClientConfig) -> RawClient {
    RawClient::new_apps_lookup(TEST_RUN_DIR, service, config)
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

fn cgroups_lookup_dispatch_handler() -> DispatchHandler {
    cgroups_lookup_dispatch(Arc::new(|req, builder: &mut CgroupsLookupBuilder<'_>| {
        builder.set_generation(55);
        for i in 0..req.item_count {
            let path = match req.item(i) {
                Ok(path) => path,
                Err(_) => return false,
            };
            let result = if path.as_bytes() == b"/docker/abc" {
                builder.add(
                    CGROUP_LOOKUP_KNOWN,
                    ORCHESTRATOR_DOCKER,
                    path.as_bytes(),
                    b"container-a",
                    &[(b"role".as_slice(), b"web".as_slice())],
                )
            } else {
                builder.add(
                    CGROUP_LOOKUP_UNKNOWN_RETRY_LATER,
                    0,
                    path.as_bytes(),
                    b"",
                    &[],
                )
            };
            if result.is_err() {
                return false;
            }
        }
        true
    }))
}

fn apps_lookup_dispatch_handler() -> DispatchHandler {
    apps_lookup_dispatch(Arc::new(|req, builder: &mut AppsLookupBuilder<'_>| {
        builder.set_generation(77);
        for i in 0..req.item_count {
            let pid = match req.item(i) {
                Ok(pid) => pid,
                Err(_) => return false,
            };
            let result = match pid {
                123 => builder.add(
                    PID_LOOKUP_KNOWN,
                    APPS_CGROUP_KNOWN,
                    ORCHESTRATOR_K8S,
                    pid,
                    1,
                    1000,
                    42,
                    b"nginx",
                    b"/kubepods/pod-a",
                    b"pod-a",
                    &[(b"namespace".as_slice(), b"default".as_slice())],
                ),
                124 => builder.add(
                    PID_LOOKUP_KNOWN,
                    APPS_CGROUP_HOST_ROOT,
                    0,
                    pid,
                    1,
                    0,
                    43,
                    b"sshd",
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
            if result.is_err() {
                return false;
            }
        }
        true
    }))
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

#[test]
fn test_client_call_timeout_on_wedged_peer() {
    let svc = unique_service("rs_win_call_timeout");
    let mut server =
        start_raw_session_server(&svc, server_config(), move |_session, _hdr, _payload| {
            thread::sleep(Duration::from_millis(150));
            Ok(())
        });

    let mut client = increment_client(&svc, client_config());
    connect_ready(&mut client);

    let start = Instant::now();
    let err = client
        .call_increment_with_timeout(41, 30)
        .expect_err("wedged peer should time out");
    assert_eq!(err, NipcError::Timeout);
    assert!(
        start.elapsed() < Duration::from_secs(1),
        "timeout took too long: {:?}",
        start.elapsed()
    );

    client.close();
    server.wait();
    cleanup_all(&svc);
}

#[test]
fn test_client_abort_unblocks_call() {
    let svc = unique_service("rs_win_call_abort");
    let (entered_tx, entered_rx) = mpsc::channel();
    let (release_tx, release_rx) = mpsc::channel();
    let mut server =
        start_raw_session_server(&svc, server_config(), move |_session, _hdr, _payload| {
            let _ = entered_tx.send(());
            let _ = release_rx.recv_timeout(Duration::from_secs(5));
            Ok(())
        });

    let mut client = increment_client(&svc, client_config());
    connect_ready(&mut client);
    let abort = client.abort_handle();

    let call_thread = thread::spawn(move || {
        client
            .call_increment_with_timeout(41, 5_000)
            .expect_err("aborted call should fail")
    });

    entered_rx
        .recv_timeout(Duration::from_secs(2))
        .expect("server handler should receive request");

    let start = Instant::now();
    abort.abort();
    let err = call_thread.join().expect("call thread should not panic");
    assert_eq!(err, NipcError::Aborted);
    assert!(
        start.elapsed() < Duration::from_secs(1),
        "abort took too long: {:?}",
        start.elapsed()
    );

    release_tx.send(()).expect("release handler");
    server.wait();
    cleanup_all(&svc);
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

fn start_raw_overflow_no_growth_server(service: &str, cfg: ServerConfig) -> RawSessionServer {
    ensure_run_dir();
    cleanup_all(service);

    let svc = service.to_string();
    let thread = thread::spawn(move || {
        let mut listener =
            NpListener::bind(TEST_RUN_DIR, &svc, cfg).map_err(|e| format!("bind: {e}"))?;

        let mut first = listener
            .accept()
            .map_err(|e| format!("first accept: {e}"))?;
        let req_hdr = {
            let mut recv_buf = vec![0u8; RESPONSE_BUF_SIZE];
            let (hdr, _) = first
                .receive(&mut recv_buf)
                .map_err(|e| format!("first receive: {e}"))?;
            hdr
        };

        let mut resp_hdr = Header {
            kind: KIND_RESPONSE,
            code: req_hdr.code,
            flags: 0,
            item_count: 1,
            message_id: req_hdr.message_id,
            transport_status: STATUS_LIMIT_EXCEEDED,
            ..Header::default()
        };
        first
            .send(&mut resp_hdr, &[])
            .map_err(|e| format!("first send: {e}"))?;
        drop(first);

        let second = listener
            .accept()
            .map_err(|e| format!("second accept: {e}"))?;
        drop(second);
        Ok(())
    });

    thread::sleep(Duration::from_millis(200));
    RawSessionServer {
        thread: Some(thread),
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
fn test_cgroups_lookup_call_windows_baseline() {
    let svc = unique_service("rs_win_cgroups_lookup");
    let mut server = TestServer::start(
        &svc,
        METHOD_CGROUPS_LOOKUP,
        cgroups_lookup_dispatch_handler(),
    );

    let mut client = cgroups_lookup_client(&svc, client_config());
    connect_ready(&mut client);

    let paths: [&[u8]; 2] = [b"/docker/abc".as_slice(), b"/missing".as_slice()];
    let view = client.call_cgroups_lookup(&paths).expect("cgroups lookup");
    assert_eq!(view.item_count, 2);
    assert_eq!(view.generation, 55);

    let known = view.item(0).expect("known item");
    assert_eq!(known.status, CGROUP_LOOKUP_KNOWN);
    assert_eq!(known.orchestrator, ORCHESTRATOR_DOCKER);
    assert_eq!(known.path.as_bytes(), b"/docker/abc");
    assert_eq!(known.name.as_bytes(), b"container-a");
    let label = known.label(0).expect("known label");
    assert_eq!(label.key.as_bytes(), b"role");
    assert_eq!(label.value.as_bytes(), b"web");

    let unknown = view.item(1).expect("unknown item");
    assert_eq!(unknown.status, CGROUP_LOOKUP_UNKNOWN_RETRY_LATER);
    assert_eq!(unknown.path.as_bytes(), b"/missing");
    assert_eq!(unknown.name.as_bytes(), b"");
    assert_eq!(client.status().call_count, 1);

    client.close();
    server.stop();
}

#[test]
fn test_apps_lookup_call_windows_baseline() {
    let svc = unique_service("rs_win_apps_lookup");
    let mut server = TestServer::start(&svc, METHOD_APPS_LOOKUP, apps_lookup_dispatch_handler());

    let mut client = apps_lookup_client(&svc, client_config());
    connect_ready(&mut client);

    let view = client
        .call_apps_lookup(&[123, 124, 999])
        .expect("apps lookup");
    assert_eq!(view.item_count, 3);
    assert_eq!(view.generation, 77);

    let known = view.item(0).expect("known pid");
    assert_eq!(known.status, PID_LOOKUP_KNOWN);
    assert_eq!(known.cgroup_status, APPS_CGROUP_KNOWN);
    assert_eq!(known.orchestrator, ORCHESTRATOR_K8S);
    assert_eq!(known.pid, 123);
    assert_eq!(known.comm.as_bytes(), b"nginx");
    assert_eq!(known.cgroup_path.as_bytes(), b"/kubepods/pod-a");
    assert_eq!(known.cgroup_name.as_bytes(), b"pod-a");
    let label = known.label(0).expect("known pid label");
    assert_eq!(label.key.as_bytes(), b"namespace");
    assert_eq!(label.value.as_bytes(), b"default");

    let host = view.item(1).expect("host pid");
    assert_eq!(host.status, PID_LOOKUP_KNOWN);
    assert_eq!(host.cgroup_status, APPS_CGROUP_HOST_ROOT);
    assert_eq!(host.comm.as_bytes(), b"sshd");
    assert_eq!(host.cgroup_path.as_bytes(), b"");

    let unknown = view.item(2).expect("unknown pid");
    assert_eq!(unknown.status, PID_LOOKUP_UNKNOWN);
    assert_eq!(unknown.pid, 999);
    assert_eq!(unknown.uid, NIPC_UID_UNSET);
    assert_eq!(unknown.comm.as_bytes(), b"");
    assert_eq!(client.status().call_count, 1);

    client.close();
    server.stop();
}

#[test]
fn test_lookup_zero_item_calls_windows() {
    let apps_svc = unique_service("rs_win_apps_lookup_zero");
    let mut apps_server = TestServer::start(
        &apps_svc,
        METHOD_APPS_LOOKUP,
        apps_lookup_dispatch_handler(),
    );
    let mut apps_client = apps_lookup_client(&apps_svc, client_config());
    connect_ready(&mut apps_client);

    let apps_view = apps_client
        .call_apps_lookup(&[])
        .expect("zero-item apps lookup");
    assert_eq!(apps_view.item_count, 0);
    assert_eq!(apps_view.generation, 77);
    assert_eq!(apps_client.status().call_count, 1);
    apps_client.close();
    apps_server.stop();

    let cgroups_svc = unique_service("rs_win_cgroups_lookup_zero");
    let mut cgroups_server = TestServer::start(
        &cgroups_svc,
        METHOD_CGROUPS_LOOKUP,
        cgroups_lookup_dispatch_handler(),
    );
    let mut cgroups_client = cgroups_lookup_client(&cgroups_svc, client_config());
    connect_ready(&mut cgroups_client);

    let cgroups_view = cgroups_client
        .call_cgroups_lookup(&[])
        .expect("zero-item cgroups lookup");
    assert_eq!(cgroups_view.item_count, 0);
    assert_eq!(cgroups_view.generation, 55);
    assert_eq!(cgroups_client.status().call_count, 1);
    cgroups_client.close();
    cgroups_server.stop();
}

#[test]
fn test_cgroups_lookup_transparent_payload_exceeded_retry_windows() {
    let svc = unique_service("rs_win_cgroups_lookup_scale");
    let mut cfg = server_config();
    cfg.max_response_payload_bytes = 256;
    let calls = Arc::new(AtomicU32::new(0));
    let handler_calls = calls.clone();
    let handler = cgroups_lookup_dispatch(Arc::new(move |req, builder| {
        handler_calls.fetch_add(1, Ordering::SeqCst);
        builder.set_generation(7);
        for i in 0..req.item_count {
            let item = match req.item(i) {
                Ok(item) => item,
                Err(_) => return false,
            };
            let name;
            let name_ref: &[u8] = if item.as_bytes() == b"/huge" {
                name = vec![b'x'; 512];
                &name
            } else {
                b"ok"
            };
            let label_value;
            let labels;
            let labels_ref: &[(&[u8], &[u8])] = if item.as_bytes() == b"/huge-label" {
                label_value = vec![b'l'; 512];
                labels = [(b"huge".as_slice(), label_value.as_slice())];
                &labels
            } else {
                &[]
            };
            if builder
                .add(
                    CGROUP_LOOKUP_KNOWN,
                    ORCHESTRATOR_K8S,
                    item.as_bytes(),
                    name_ref,
                    labels_ref,
                )
                .is_err()
            {
                return false;
            }
        }
        true
    }));

    let mut server = TestServer::start_with(
        &svc,
        cfg,
        client_config(),
        METHOD_CGROUPS_LOOKUP,
        handler,
        8,
    );
    let mut client = cgroups_lookup_client(&svc, client_config());
    connect_ready(&mut client);

    let view = client
        .call_cgroups_lookup(&[
            b"/a".as_slice(),
            b"/huge".as_slice(),
            b"/huge-label".as_slice(),
            b"/b".as_slice(),
        ])
        .expect("cgroups lookup scale call");
    assert!(
        calls.load(Ordering::SeqCst) >= 2,
        "handler should be called for at least two subrequests"
    );
    assert_eq!(view.item_count, 4);
    assert_eq!(view.generation, 7);
    let item0 = view.item(0).expect("item 0");
    assert_eq!(item0.status, CGROUP_LOOKUP_KNOWN);
    assert_eq!(item0.path.as_bytes(), b"/a");
    let item1 = view.item(1).expect("item 1");
    assert_eq!(item1.status, CGROUP_LOOKUP_OVERSIZED_ITEM);
    assert_eq!(item1.path.as_bytes(), b"/huge");
    let item2 = view.item(2).expect("item 2");
    assert_eq!(item2.status, CGROUP_LOOKUP_OVERSIZED_ITEM);
    assert_eq!(item2.path.as_bytes(), b"/huge-label");
    let item3 = view.item(3).expect("item 3");
    assert_eq!(item3.status, CGROUP_LOOKUP_KNOWN);
    assert_eq!(item3.path.as_bytes(), b"/b");
    assert_eq!(item3.name.as_bytes(), b"ok");

    client.close();
    server.stop();
}

#[test]
fn test_apps_lookup_transparent_payload_exceeded_retry_windows() {
    let svc = unique_service("rs_win_apps_lookup_scale");
    let mut cfg = server_config();
    cfg.max_response_payload_bytes = 320;
    let calls = Arc::new(AtomicU32::new(0));
    let handler_calls = calls.clone();
    let handler = apps_lookup_dispatch(Arc::new(move |req, builder| {
        handler_calls.fetch_add(1, Ordering::SeqCst);
        builder.set_generation(9);
        for i in 0..req.item_count {
            let pid = match req.item(i) {
                Ok(pid) => pid,
                Err(_) => return false,
            };
            let cgroup_path;
            let cgroup_path_ref: &[u8] = if pid == 22 {
                cgroup_path = [b"/".as_slice(), &vec![b'x'; 1024]].concat();
                &cgroup_path
            } else {
                b"/ok"
            };
            let label_value;
            let labels;
            let labels_ref: &[(&[u8], &[u8])] = if pid == 44 {
                label_value = vec![b'l'; 512];
                labels = [(b"huge".as_slice(), label_value.as_slice())];
                &labels
            } else {
                &[]
            };
            if builder
                .add(
                    PID_LOOKUP_KNOWN,
                    APPS_CGROUP_KNOWN,
                    0,
                    pid,
                    0,
                    0,
                    1,
                    b"ok",
                    cgroup_path_ref,
                    b"name",
                    labels_ref,
                )
                .is_err()
            {
                return false;
            }
        }
        true
    }));

    let mut server =
        TestServer::start_with(&svc, cfg, client_config(), METHOD_APPS_LOOKUP, handler, 8);
    let mut client = apps_lookup_client(&svc, client_config());
    connect_ready(&mut client);

    let view = client
        .call_apps_lookup(&[11, 22, 44, 33])
        .expect("apps lookup scale call");
    assert!(
        calls.load(Ordering::SeqCst) >= 2,
        "handler should be called for at least two subrequests"
    );
    assert_eq!(view.item_count, 4);
    assert_eq!(view.generation, 9);
    let item0 = view.item(0).expect("item 0");
    assert_eq!(item0.status, PID_LOOKUP_KNOWN);
    assert_eq!(item0.pid, 11);
    assert_eq!(item0.comm.as_bytes(), b"ok");
    let item1 = view.item(1).expect("item 1");
    assert_eq!(item1.status, PID_LOOKUP_OVERSIZED_ITEM);
    assert_eq!(item1.pid, 22);
    let item2 = view.item(2).expect("item 2");
    assert_eq!(item2.status, PID_LOOKUP_OVERSIZED_ITEM);
    assert_eq!(item2.pid, 44);
    let item3 = view.item(3).expect("item 3");
    assert_eq!(item3.status, PID_LOOKUP_KNOWN);
    assert_eq!(item3.pid, 33);
    assert_eq!(item3.comm.as_bytes(), b"ok");

    client.close();
    server.stop();
}

fn run_apps_lookup_request_boundary_case_windows(
    request_cap: u32,
    expected_max_items: u32,
    min_calls: u32,
    label: &str,
) {
    let svc = unique_service(&format!("rs_win_apps_lookup_request_split_{label}"));
    let mut cfg = server_config();
    cfg.max_request_payload_bytes = request_cap;
    cfg.max_response_payload_bytes = 4096;
    let calls = Arc::new(AtomicU32::new(0));
    let max_seen = Arc::new(AtomicU32::new(0));
    let handler_calls = calls.clone();
    let handler_max = max_seen.clone();
    let handler = apps_lookup_dispatch(Arc::new(move |req, builder| {
        handler_calls.fetch_add(1, Ordering::SeqCst);
        handler_max.fetch_max(req.item_count, Ordering::SeqCst);
        builder.set_generation(11);
        for i in 0..req.item_count {
            let pid = match req.item(i) {
                Ok(pid) => pid,
                Err(_) => return false,
            };
            if builder
                .add(
                    PID_LOOKUP_UNKNOWN,
                    0,
                    0,
                    pid,
                    0,
                    NIPC_UID_UNSET,
                    0,
                    b"",
                    b"",
                    b"",
                    &[],
                )
                .is_err()
            {
                return false;
            }
        }
        true
    }));

    let mut ccfg = client_config();
    ccfg.max_request_payload_bytes = request_cap;
    let mut server =
        TestServer::start_with(&svc, cfg, ccfg.clone(), METHOD_APPS_LOOKUP, handler, 8);
    let mut client = apps_lookup_client(&svc, ccfg);
    connect_ready(&mut client);

    let pids = [4, 1, 4, 7, 1, 9, 7];
    let view = client
        .call_apps_lookup(&pids)
        .expect("apps request split call");
    assert_eq!(view.item_count, 7);
    assert_eq!(view.generation, 11);
    for (i, want) in pids.iter().enumerate() {
        let item = view.item(i as u32).expect("apps split item");
        assert_eq!(item.status, PID_LOOKUP_UNKNOWN);
        assert_eq!(item.pid, *want);
    }
    assert!(
        calls.load(Ordering::SeqCst) >= min_calls,
        "apps {label} handler should see split requests"
    );
    assert!(
        max_seen.load(Ordering::SeqCst) == expected_max_items,
        "apps {label} request fragment should hit expected boundary"
    );

    client.close();
    server.stop();
}

fn run_cgroups_lookup_request_boundary_case_windows(
    request_cap: u32,
    expected_max_items: u32,
    min_calls: u32,
    label: &str,
) {
    let svc = unique_service(&format!("rs_win_cgroups_lookup_request_split_{label}"));
    let mut cfg = server_config();
    cfg.max_request_payload_bytes = request_cap;
    cfg.max_response_payload_bytes = 4096;
    let calls = Arc::new(AtomicU32::new(0));
    let max_seen = Arc::new(AtomicU32::new(0));
    let handler_calls = calls.clone();
    let handler_max = max_seen.clone();
    let handler = cgroups_lookup_dispatch(Arc::new(move |req, builder| {
        handler_calls.fetch_add(1, Ordering::SeqCst);
        handler_max.fetch_max(req.item_count, Ordering::SeqCst);
        builder.set_generation(12);
        for i in 0..req.item_count {
            let item = match req.item(i) {
                Ok(item) => item,
                Err(_) => return false,
            };
            if builder
                .add(
                    CGROUP_LOOKUP_KNOWN,
                    ORCHESTRATOR_K8S,
                    item.as_bytes(),
                    b"ok",
                    &[],
                )
                .is_err()
            {
                return false;
            }
        }
        true
    }));

    let mut ccfg = client_config();
    ccfg.max_request_payload_bytes = request_cap;
    let mut server =
        TestServer::start_with(&svc, cfg, ccfg.clone(), METHOD_CGROUPS_LOOKUP, handler, 8);
    let mut client = cgroups_lookup_client(&svc, ccfg);
    connect_ready(&mut client);

    let paths: [&[u8]; 5] = [b"/bbbbbb", b"/aaaaaa", b"/bbbbbb", b"/cccccc", b"/aaaaaa"];
    let view = client
        .call_cgroups_lookup(&paths)
        .expect("cgroups request split call");
    assert_eq!(view.item_count, 5);
    assert_eq!(view.generation, 12);
    for (i, want) in paths.iter().enumerate() {
        let item = view.item(i as u32).expect("cgroups split item");
        assert_eq!(item.status, CGROUP_LOOKUP_KNOWN);
        assert_eq!(item.path.as_bytes(), *want);
    }
    assert!(
        calls.load(Ordering::SeqCst) >= min_calls,
        "cgroups {label} handler should see split requests"
    );
    assert!(
        max_seen.load(Ordering::SeqCst) == expected_max_items,
        "cgroups {label} request fragment should hit expected boundary"
    );

    client.close();
    server.stop();
}

#[test]
fn test_lookup_proactive_request_split_windows() {
    run_apps_lookup_request_boundary_case_windows(63, 2, 4, "cap_minus_1");
    run_apps_lookup_request_boundary_case_windows(64, 3, 3, "cap_exact");
    run_apps_lookup_request_boundary_case_windows(65, 3, 3, "cap_plus_1");

    run_cgroups_lookup_request_boundary_case_windows(47, 1, 5, "cap_minus_1");
    run_cgroups_lookup_request_boundary_case_windows(48, 2, 3, "cap_exact");
    run_cgroups_lookup_request_boundary_case_windows(49, 2, 3, "cap_plus_1");
}

#[test]
fn test_cgroups_lookup_oversized_request_key_windows() {
    let svc = unique_service("rs_win_cgroups_lookup_oversized_request_key");
    let mut cfg = server_config();
    cfg.max_request_payload_bytes = 48;
    cfg.max_response_payload_bytes = 4096;
    let calls = Arc::new(AtomicU32::new(0));
    let max_seen = Arc::new(AtomicU32::new(0));
    let handler_calls = calls.clone();
    let handler_max = max_seen.clone();
    let handler = cgroups_lookup_dispatch(Arc::new(move |req, builder| {
        handler_calls.fetch_add(1, Ordering::SeqCst);
        handler_max.fetch_max(req.item_count, Ordering::SeqCst);
        builder.set_generation(12);
        for i in 0..req.item_count {
            let item = match req.item(i) {
                Ok(item) => item,
                Err(_) => return false,
            };
            if builder
                .add(
                    CGROUP_LOOKUP_KNOWN,
                    ORCHESTRATOR_K8S,
                    item.as_bytes(),
                    b"ok",
                    &[],
                )
                .is_err()
            {
                return false;
            }
        }
        true
    }));

    let mut ccfg = client_config();
    ccfg.max_request_payload_bytes = 48;
    let mut server =
        TestServer::start_with(&svc, cfg, ccfg.clone(), METHOD_CGROUPS_LOOKUP, handler, 8);
    let mut client = cgroups_lookup_client(&svc, ccfg);
    connect_ready(&mut client);

    let paths: [&[u8]; 2] = [b"/request-key-too-large-for-configured-cap", b"/ok"];
    let view = client
        .call_cgroups_lookup(&paths)
        .expect("cgroups oversized request-key call");
    assert_eq!(view.item_count, 2);
    assert_eq!(view.generation, 12);
    let item0 = view.item(0).expect("item 0");
    assert_eq!(item0.status, CGROUP_LOOKUP_OVERSIZED_ITEM);
    assert_eq!(item0.path.as_bytes(), paths[0]);
    let item1 = view.item(1).expect("item 1");
    assert_eq!(item1.status, CGROUP_LOOKUP_KNOWN);
    assert_eq!(item1.path.as_bytes(), b"/ok");
    assert_eq!(item1.name.as_bytes(), b"ok");
    assert_eq!(calls.load(Ordering::SeqCst), 1);
    assert_eq!(max_seen.load(Ordering::SeqCst), 1);

    client.close();
    server.stop();
}

#[test]
fn test_lookup_request_capacity_reconnects_windows() {
    let svc = unique_service("rs_win_apps_lookup_request_capacity");
    let mut cfg = server_config();
    cfg.max_request_payload_bytes = 256;
    let mut wake_cfg = client_config();
    wake_cfg.max_request_payload_bytes = 256;
    let mut server = TestServer::start_with(
        &svc,
        cfg,
        wake_cfg,
        METHOD_APPS_LOOKUP,
        apps_lookup_dispatch_handler(),
        8,
    );
    let mut ccfg = client_config();
    ccfg.max_request_payload_bytes = 256;
    let mut client = apps_lookup_client(&svc, ccfg);
    connect_ready(&mut client);
    client
        .session
        .as_mut()
        .expect("apps session")
        .max_request_payload_bytes = 8;

    let view = client
        .call_apps_lookup(&[1234])
        .expect("apps lookup capacity reconnect");
    assert_eq!(view.item_count, 1);
    assert!(client.status().reconnect_count >= 1);

    client.close();
    server.stop();

    let svc = unique_service("rs_win_cgroups_lookup_request_capacity");
    let mut cfg = server_config();
    cfg.max_request_payload_bytes = 256;
    let mut wake_cfg = client_config();
    wake_cfg.max_request_payload_bytes = 256;
    let mut server = TestServer::start_with(
        &svc,
        cfg,
        wake_cfg,
        METHOD_CGROUPS_LOOKUP,
        cgroups_lookup_dispatch_handler(),
        8,
    );
    let mut ccfg = client_config();
    ccfg.max_request_payload_bytes = 256;
    let mut client = cgroups_lookup_client(&svc, ccfg);
    connect_ready(&mut client);
    client
        .session
        .as_mut()
        .expect("cgroups session")
        .max_request_payload_bytes = 8;

    let view = client
        .call_cgroups_lookup(&[b"/known".as_slice()])
        .expect("cgroups lookup capacity reconnect");
    assert_eq!(view.item_count, 1);
    assert!(client.status().reconnect_count >= 1);

    client.close();
    server.stop();
}

#[test]
fn test_raw_call_overflow_no_growth_stops_bounded_windows() {
    let svc = unique_service("rs_win_overflow_no_growth");
    let mut scfg = server_config();
    scfg.max_response_payload_bytes = MAX_PAYLOAD_CAP;
    let mut server = start_raw_overflow_no_growth_server(&svc, scfg);

    let mut ccfg = client_config();
    ccfg.max_response_payload_bytes = MAX_PAYLOAD_CAP;
    let mut client = increment_client(&svc, ccfg);
    connect_ready(&mut client);

    let err = client
        .call_increment(41)
        .expect_err("overflow with no capacity growth should stop");
    assert_eq!(err, NipcError::Overflow);

    let status = client.status();
    assert_eq!(status.state, ClientState::Broken);
    assert_eq!(status.reconnect_count, 1);
    assert_eq!(status.error_count, 1);

    client.close();
    server.wait();
}

#[test]
fn test_lookup_timeout_during_followup_subcall_windows() {
    let svc = unique_service("rs_win_apps_lookup_followup_timeout");
    let mut cfg = server_config();
    cfg.max_response_payload_bytes = 320;
    let calls = Arc::new(AtomicU32::new(0));
    let handler_calls = calls.clone();
    let handler = apps_lookup_dispatch(Arc::new(move |req, builder| {
        if handler_calls.fetch_add(1, Ordering::SeqCst) + 1 == 2 {
            thread::sleep(Duration::from_millis(150));
        }
        builder.set_generation(9);
        for i in 0..req.item_count {
            let pid = match req.item(i) {
                Ok(pid) => pid,
                Err(_) => return false,
            };
            let cgroup_path;
            let cgroup_path_ref: &[u8] = if pid == 22 {
                cgroup_path = [b"/".as_slice(), &vec![b'x'; 1024]].concat();
                &cgroup_path
            } else {
                b"/ok"
            };
            if builder
                .add(
                    PID_LOOKUP_KNOWN,
                    APPS_CGROUP_KNOWN,
                    0,
                    pid,
                    0,
                    0,
                    1,
                    b"ok",
                    cgroup_path_ref,
                    b"name",
                    &[],
                )
                .is_err()
            {
                return false;
            }
        }
        true
    }));
    let mut server =
        TestServer::start_with(&svc, cfg, client_config(), METHOD_APPS_LOOKUP, handler, 8);
    let mut client = apps_lookup_client(&svc, client_config());
    connect_ready(&mut client);
    let err = match client.call_apps_lookup_with_timeout(&[11, 22, 33], 30) {
        Ok(_) => panic!("apps follow-up timeout should fail"),
        Err(err) => err,
    };
    assert_eq!(err, NipcError::Timeout);
    assert!(calls.load(Ordering::SeqCst) >= 2);
    client.close();
    server.stop();

    let svc = unique_service("rs_win_cgroups_lookup_followup_timeout");
    let mut cfg = server_config();
    cfg.max_response_payload_bytes = 160;
    let calls = Arc::new(AtomicU32::new(0));
    let handler_calls = calls.clone();
    let handler = cgroups_lookup_dispatch(Arc::new(move |req, builder| {
        if handler_calls.fetch_add(1, Ordering::SeqCst) + 1 == 2 {
            thread::sleep(Duration::from_millis(150));
        }
        builder.set_generation(7);
        for i in 0..req.item_count {
            let item = match req.item(i) {
                Ok(item) => item,
                Err(_) => return false,
            };
            let name;
            let name_ref: &[u8] = if item.as_bytes() == b"/huge" {
                name = vec![b'x'; 512];
                &name
            } else {
                b"ok"
            };
            if builder
                .add(
                    CGROUP_LOOKUP_KNOWN,
                    ORCHESTRATOR_K8S,
                    item.as_bytes(),
                    name_ref,
                    &[],
                )
                .is_err()
            {
                return false;
            }
        }
        true
    }));
    let mut server = TestServer::start_with(
        &svc,
        cfg,
        client_config(),
        METHOD_CGROUPS_LOOKUP,
        handler,
        8,
    );
    let mut client = cgroups_lookup_client(&svc, client_config());
    connect_ready(&mut client);
    let paths: [&[u8]; 3] = [b"/a".as_slice(), b"/huge".as_slice(), b"/b".as_slice()];
    let err = match client.call_cgroups_lookup_with_timeout(&paths, 30) {
        Ok(_) => panic!("cgroups follow-up timeout should fail"),
        Err(err) => err,
    };
    assert_eq!(err, NipcError::Timeout);
    assert!(calls.load(Ordering::SeqCst) >= 2);
    client.close();
    server.stop();
}

#[test]
fn test_lookup_abort_during_followup_subcall_windows() {
    let svc = unique_service("rs_win_apps_lookup_followup_abort");
    let mut cfg = server_config();
    cfg.max_response_payload_bytes = 320;
    let calls = Arc::new(AtomicU32::new(0));
    let handler_calls = calls.clone();
    let (second_tx, second_rx) = mpsc::channel();
    let handler = apps_lookup_dispatch(Arc::new(move |req, builder| {
        if handler_calls.fetch_add(1, Ordering::SeqCst) + 1 == 2 {
            let _ = second_tx.send(());
            thread::sleep(Duration::from_millis(250));
        }
        builder.set_generation(9);
        for i in 0..req.item_count {
            let pid = match req.item(i) {
                Ok(pid) => pid,
                Err(_) => return false,
            };
            let cgroup_path;
            let cgroup_path_ref: &[u8] = if pid == 22 {
                cgroup_path = [b"/".as_slice(), &vec![b'x'; 1024]].concat();
                &cgroup_path
            } else {
                b"/ok"
            };
            if builder
                .add(
                    PID_LOOKUP_KNOWN,
                    APPS_CGROUP_KNOWN,
                    0,
                    pid,
                    0,
                    0,
                    1,
                    b"ok",
                    cgroup_path_ref,
                    b"name",
                    &[],
                )
                .is_err()
            {
                return false;
            }
        }
        true
    }));
    let mut server =
        TestServer::start_with(&svc, cfg, client_config(), METHOD_APPS_LOOKUP, handler, 8);
    let mut client = apps_lookup_client(&svc, client_config());
    connect_ready(&mut client);
    let abort = client.abort_handle();
    let (result_tx, result_rx) = mpsc::channel();
    thread::spawn(move || {
        let result = client.call_apps_lookup_with_timeout(&[11, 22, 33], 5_000);
        let _ = result_tx.send(result.map(|view| view.item_count));
        client.close();
    });
    second_rx
        .recv_timeout(Duration::from_secs(2))
        .expect("apps follow-up subcall should start");
    abort.abort();
    assert_eq!(
        result_rx
            .recv_timeout(Duration::from_secs(2))
            .expect("apps abort result"),
        Err(NipcError::Aborted)
    );
    server.stop();

    let svc = unique_service("rs_win_cgroups_lookup_followup_abort");
    let mut cfg = server_config();
    cfg.max_response_payload_bytes = 160;
    let calls = Arc::new(AtomicU32::new(0));
    let handler_calls = calls.clone();
    let (second_tx, second_rx) = mpsc::channel();
    let handler = cgroups_lookup_dispatch(Arc::new(move |req, builder| {
        if handler_calls.fetch_add(1, Ordering::SeqCst) + 1 == 2 {
            let _ = second_tx.send(());
            thread::sleep(Duration::from_millis(250));
        }
        builder.set_generation(7);
        for i in 0..req.item_count {
            let item = match req.item(i) {
                Ok(item) => item,
                Err(_) => return false,
            };
            let name;
            let name_ref: &[u8] = if item.as_bytes() == b"/huge" {
                name = vec![b'x'; 512];
                &name
            } else {
                b"ok"
            };
            if builder
                .add(
                    CGROUP_LOOKUP_KNOWN,
                    ORCHESTRATOR_K8S,
                    item.as_bytes(),
                    name_ref,
                    &[],
                )
                .is_err()
            {
                return false;
            }
        }
        true
    }));
    let mut server = TestServer::start_with(
        &svc,
        cfg,
        client_config(),
        METHOD_CGROUPS_LOOKUP,
        handler,
        8,
    );
    let mut client = cgroups_lookup_client(&svc, client_config());
    connect_ready(&mut client);
    let abort = client.abort_handle();
    let (result_tx, result_rx) = mpsc::channel();
    thread::spawn(move || {
        let paths: [&[u8]; 3] = [b"/a".as_slice(), b"/huge".as_slice(), b"/b".as_slice()];
        let result = client.call_cgroups_lookup_with_timeout(&paths, 5_000);
        let _ = result_tx.send(result.map(|view| view.item_count));
        client.close();
    });
    second_rx
        .recv_timeout(Duration::from_secs(2))
        .expect("cgroups follow-up subcall should start");
    abort.abort();
    assert_eq!(
        result_rx
            .recv_timeout(Duration::from_secs(2))
            .expect("cgroups abort result"),
        Err(NipcError::Aborted)
    );
    server.stop();
}

fn large_lookup_pids(item_count: usize) -> Vec<u32> {
    (0..item_count).map(|i| 100000u32 + i as u32).collect()
}

fn large_lookup_paths(item_count: usize) -> Vec<Vec<u8>> {
    (0..item_count)
        .map(|i| format!("/cg/{i:05}").into_bytes())
        .collect()
}

fn verify_large_apps_lookup(view: &AppsLookupResponseView<'_>, pids: &[u32]) {
    assert_eq!(view.item_count, pids.len() as u32);
    assert_eq!(view.generation, 9);
    for (i, expected) in pids.iter().enumerate() {
        let item = view.item(i as u32).expect("apps large item");
        assert_eq!(item.status, PID_LOOKUP_KNOWN);
        assert_eq!(item.pid, *expected);
        assert_eq!(item.comm.as_bytes(), b"ok");
        assert_eq!(item.cgroup_path.as_bytes(), b"/ok");
    }
}

fn verify_large_cgroups_lookup(view: &CgroupsLookupResponseView<'_>, paths: &[Vec<u8>]) {
    assert_eq!(view.item_count, paths.len() as u32);
    assert_eq!(view.generation, 7);
    for (i, expected) in paths.iter().enumerate() {
        let item = view.item(i as u32).expect("cgroups large item");
        assert_eq!(item.status, CGROUP_LOOKUP_KNOWN);
        assert_eq!(item.path.as_bytes(), expected.as_slice());
        assert_eq!(item.name.as_bytes(), b"ok");
    }
}

fn verify_response_split_apps_lookup(view: &AppsLookupResponseView<'_>, pids: &[u32]) {
    let label_value = vec![b'l'; LOOKUP_RESPONSE_SPLIT_LABEL_BYTES];
    assert_eq!(view.item_count, pids.len() as u32);
    assert_eq!(view.generation, 9);
    for (i, expected) in pids.iter().enumerate() {
        let item = view.item(i as u32).expect("apps response-split item");
        assert_eq!(item.status, PID_LOOKUP_KNOWN);
        assert_eq!(item.pid, *expected);
        assert_eq!(item.comm.as_bytes(), b"ok");
        assert_eq!(item.cgroup_path.as_bytes(), b"/ok");
        assert_eq!(item.label_count, 1);
        let label = item.label(0).expect("apps response-split label");
        assert_eq!(label.key.as_bytes(), b"scale");
        assert_eq!(label.value.as_bytes(), label_value.as_slice());
    }
}

fn verify_response_split_cgroups_lookup(view: &CgroupsLookupResponseView<'_>, paths: &[Vec<u8>]) {
    let label_value = vec![b'l'; LOOKUP_RESPONSE_SPLIT_LABEL_BYTES];
    assert_eq!(view.item_count, paths.len() as u32);
    assert_eq!(view.generation, 7);
    for (i, expected) in paths.iter().enumerate() {
        let item = view.item(i as u32).expect("cgroups response-split item");
        assert_eq!(item.status, CGROUP_LOOKUP_KNOWN);
        assert_eq!(item.path.as_bytes(), expected.as_slice());
        assert_eq!(item.name.as_bytes(), b"ok");
        assert_eq!(item.label_count, 1);
        let label = item.label(0).expect("cgroups response-split label");
        assert_eq!(label.key.as_bytes(), b"scale");
        assert_eq!(label.value.as_bytes(), label_value.as_slice());
    }
}

fn patch_lookup_item_u16(
    payload: &mut [u8],
    hdr_size: usize,
    item_count: usize,
    index: usize,
    item_offset: usize,
    value: u16,
) {
    let packed_start = hdr_size + item_count * 8;
    let dir = hdr_size + index * 8;
    let off = u32::from_ne_bytes(payload[dir..dir + 4].try_into().unwrap()) as usize;
    let item_field = packed_start + off + item_offset;
    payload[item_field..item_field + 2].copy_from_slice(&value.to_ne_bytes());
}

fn patch_lookup_item_status(
    payload: &mut [u8],
    hdr_size: usize,
    item_count: usize,
    index: usize,
    status: u16,
) {
    patch_lookup_item_u16(payload, hdr_size, item_count, index, 2, status);
}

fn apps_lookup_response_payload_unknown(pid: u32) -> Vec<u8> {
    apps_lookup_response_payload_unknowns(&[pid])
}

fn apps_lookup_response_payload_unknowns(pids: &[u32]) -> Vec<u8> {
    let mut buf = vec![0u8; 256];
    let mut builder = AppsLookupBuilder::new(&mut buf, pids.len() as u32, 1);
    for pid in pids {
        builder
            .add(
                PID_LOOKUP_UNKNOWN,
                0,
                0,
                *pid,
                0,
                NIPC_UID_UNSET,
                0,
                b"",
                b"",
                b"",
                &[],
            )
            .unwrap();
    }
    let n = builder.finish().unwrap();
    buf.truncate(n);
    buf
}

fn apps_lookup_response_payload_known_with_label(pid: u32) -> Vec<u8> {
    let mut buf = vec![0u8; 512];
    let mut builder = AppsLookupBuilder::new(&mut buf, 1, 1);
    builder
        .add(
            PID_LOOKUP_KNOWN,
            APPS_CGROUP_KNOWN,
            ORCHESTRATOR_DOCKER,
            pid,
            1,
            1000,
            42,
            b"comm",
            b"/cg",
            b"name",
            &[(b"role".as_slice(), b"api".as_slice())],
        )
        .unwrap();
    let n = builder.finish().unwrap();
    buf.truncate(n);
    buf
}

fn apps_lookup_response_payload_count_zero() -> Vec<u8> {
    let mut buf = vec![0u8; 64];
    let builder = AppsLookupBuilder::new(&mut buf, 0, 1);
    let n = builder.finish().unwrap();
    buf.truncate(n);
    buf
}

fn apps_lookup_response_payload_exceeded_suffix(pids: &[u32], malformed: bool) -> Vec<u8> {
    let mut buf = vec![0u8; 512];
    let mut builder = AppsLookupBuilder::new(&mut buf, pids.len() as u32, 1);
    for pid in pids {
        builder
            .add(
                PID_LOOKUP_PAYLOAD_EXCEEDED,
                0,
                0,
                *pid,
                0,
                NIPC_UID_UNSET,
                0,
                b"",
                b"",
                b"",
                &[],
            )
            .unwrap();
    }
    let n = builder.finish().unwrap();
    buf.truncate(n);
    if malformed {
        patch_lookup_item_status(
            &mut buf,
            APPS_LOOKUP_RESP_HDR_SIZE,
            pids.len(),
            1,
            PID_LOOKUP_UNKNOWN,
        );
    }
    buf
}

fn cgroups_lookup_response_payload_unknown(path: &[u8]) -> Vec<u8> {
    cgroups_lookup_response_payload_unknowns(&[path])
}

fn cgroups_lookup_response_payload_unknowns(paths: &[&[u8]]) -> Vec<u8> {
    let mut buf = vec![0u8; 256];
    let mut builder = CgroupsLookupBuilder::new(&mut buf, paths.len() as u32, 1);
    for path in paths {
        builder
            .add(CGROUP_LOOKUP_UNKNOWN_RETRY_LATER, 0, path, b"", &[])
            .unwrap();
    }
    let n = builder.finish().unwrap();
    buf.truncate(n);
    buf
}

fn cgroups_lookup_response_payload_known_with_label(path: &[u8]) -> Vec<u8> {
    let mut buf = vec![0u8; 512];
    let mut builder = CgroupsLookupBuilder::new(&mut buf, 1, 1);
    builder
        .add(
            CGROUP_LOOKUP_KNOWN,
            ORCHESTRATOR_K8S,
            path,
            b"name",
            &[(b"role".as_slice(), b"db".as_slice())],
        )
        .unwrap();
    let n = builder.finish().unwrap();
    buf.truncate(n);
    buf
}

fn cgroups_lookup_response_payload_count_zero() -> Vec<u8> {
    let mut buf = vec![0u8; 64];
    let builder = CgroupsLookupBuilder::new(&mut buf, 0, 1);
    let n = builder.finish().unwrap();
    buf.truncate(n);
    buf
}

fn cgroups_lookup_response_payload_exceeded_suffix(paths: &[&[u8]], malformed: bool) -> Vec<u8> {
    let mut buf = vec![0u8; 512];
    let mut builder = CgroupsLookupBuilder::new(&mut buf, paths.len() as u32, 1);
    for path in paths {
        builder
            .add(CGROUP_LOOKUP_PAYLOAD_EXCEEDED, 0, path, b"", &[])
            .unwrap();
    }
    let n = builder.finish().unwrap();
    buf.truncate(n);
    if malformed {
        patch_lookup_item_status(
            &mut buf,
            CGROUPS_LOOKUP_RESP_HDR_SIZE,
            paths.len(),
            1,
            CGROUP_LOOKUP_UNKNOWN_RETRY_LATER,
        );
    }
    buf
}

fn assert_apps_lookup_bad_response_windows(
    name: &str,
    pids: &[u32],
    payload: Vec<u8>,
    want: NipcError,
) {
    let svc = unique_service(&format!("rs_win_apps_lookup_bad_{name}"));
    let mut server = start_raw_session_server(&svc, server_config(), move |session, req_hdr, _| {
        let mut resp_hdr = Header {
            kind: KIND_RESPONSE,
            code: METHOD_APPS_LOOKUP,
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

    let mut client = apps_lookup_client(&svc, client_config());
    connect_ready(&mut client);
    let err = client.call_apps_lookup(pids).expect_err(name);
    assert_eq!(err, want, "{name}");

    client.close();
    server.wait();
}

fn assert_cgroups_lookup_bad_response_windows(
    name: &str,
    paths: &[&[u8]],
    payload: Vec<u8>,
    want: NipcError,
) {
    let svc = unique_service(&format!("rs_win_cgroups_lookup_bad_{name}"));
    let mut server = start_raw_session_server(&svc, server_config(), move |session, req_hdr, _| {
        let mut resp_hdr = Header {
            kind: KIND_RESPONSE,
            code: METHOD_CGROUPS_LOOKUP,
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

    let mut client = cgroups_lookup_client(&svc, client_config());
    connect_ready(&mut client);
    let err = client.call_cgroups_lookup(paths).expect_err(name);
    assert_eq!(err, want, "{name}");

    client.close();
    server.wait();
}

fn run_large_apps_lookup_windows(prefix: &str, item_count: usize) {
    let svc = unique_service(prefix);
    let pids = large_lookup_pids(item_count);
    let mut cfg = server_config();
    cfg.max_request_payload_bytes = LOOKUP_SCALE_REQUEST_PAYLOAD_BYTES;
    cfg.max_response_payload_bytes = RESPONSE_BUF_SIZE as u32;
    let mut wake_cfg = client_config();
    wake_cfg.max_request_payload_bytes = LOOKUP_SCALE_REQUEST_PAYLOAD_BYTES;
    let calls = Arc::new(AtomicU32::new(0));
    let max_seen = Arc::new(AtomicU32::new(0));
    let handler_calls = calls.clone();
    let handler_max = max_seen.clone();
    let handler = apps_lookup_dispatch(Arc::new(move |req, builder| {
        handler_calls.fetch_add(1, Ordering::SeqCst);
        handler_max.fetch_max(req.item_count, Ordering::SeqCst);
        builder.set_generation(9);
        for i in 0..req.item_count {
            let pid = match req.item(i) {
                Ok(pid) => pid,
                Err(_) => return false,
            };
            if builder
                .add(
                    PID_LOOKUP_KNOWN,
                    APPS_CGROUP_KNOWN,
                    ORCHESTRATOR_DOCKER,
                    pid,
                    1,
                    1000,
                    42,
                    b"ok",
                    b"/ok",
                    b"name",
                    &[],
                )
                .is_err()
            {
                return false;
            }
        }
        true
    }));

    let mut server = TestServer::start_with(&svc, cfg, wake_cfg, METHOD_APPS_LOOKUP, handler, 8);
    let mut ccfg = client_config();
    ccfg.max_request_payload_bytes = LOOKUP_SCALE_REQUEST_PAYLOAD_BYTES;
    let mut client = apps_lookup_client(&svc, ccfg);
    client.set_call_timeout(LOOKUP_SCALE_CALL_TIMEOUT_MS);
    connect_ready(&mut client);

    let view = client.call_apps_lookup(&pids).expect("apps large lookup");
    verify_large_apps_lookup(&view, &pids);
    assert!(calls.load(Ordering::SeqCst) > 1);
    assert!(max_seen.load(Ordering::SeqCst) < item_count as u32);

    client.close();
    server.stop();
}

fn run_large_cgroups_lookup_windows(prefix: &str, item_count: usize) {
    let svc = unique_service(prefix);
    let paths = large_lookup_paths(item_count);
    let path_refs: Vec<&[u8]> = paths.iter().map(Vec::as_slice).collect();
    let mut cfg = server_config();
    cfg.max_request_payload_bytes = LOOKUP_SCALE_REQUEST_PAYLOAD_BYTES;
    cfg.max_response_payload_bytes = RESPONSE_BUF_SIZE as u32;
    let mut wake_cfg = client_config();
    wake_cfg.max_request_payload_bytes = LOOKUP_SCALE_REQUEST_PAYLOAD_BYTES;
    let calls = Arc::new(AtomicU32::new(0));
    let max_seen = Arc::new(AtomicU32::new(0));
    let handler_calls = calls.clone();
    let handler_max = max_seen.clone();
    let handler = cgroups_lookup_dispatch(Arc::new(move |req, builder| {
        handler_calls.fetch_add(1, Ordering::SeqCst);
        handler_max.fetch_max(req.item_count, Ordering::SeqCst);
        builder.set_generation(7);
        for i in 0..req.item_count {
            let path = match req.item(i) {
                Ok(path) => path,
                Err(_) => return false,
            };
            if builder
                .add(
                    CGROUP_LOOKUP_KNOWN,
                    ORCHESTRATOR_K8S,
                    path.as_bytes(),
                    b"ok",
                    &[],
                )
                .is_err()
            {
                return false;
            }
        }
        true
    }));

    let mut server = TestServer::start_with(&svc, cfg, wake_cfg, METHOD_CGROUPS_LOOKUP, handler, 8);
    let mut ccfg = client_config();
    ccfg.max_request_payload_bytes = LOOKUP_SCALE_REQUEST_PAYLOAD_BYTES;
    let mut client = cgroups_lookup_client(&svc, ccfg);
    client.set_call_timeout(LOOKUP_SCALE_CALL_TIMEOUT_MS);
    connect_ready(&mut client);

    let view = client
        .call_cgroups_lookup(&path_refs)
        .expect("cgroups large lookup");
    verify_large_cgroups_lookup(&view, &paths);
    assert!(calls.load(Ordering::SeqCst) > 1);
    assert!(max_seen.load(Ordering::SeqCst) < item_count as u32);

    client.close();
    server.stop();
}

#[test]
fn test_lookup_large_logical_calls_windows() {
    run_large_apps_lookup_windows("rs_win_apps_lookup_large_8192", LOOKUP_TOPOLOGY_SCALE_ITEMS);
    run_large_apps_lookup_windows("rs_win_apps_lookup_large_32768", LOOKUP_HPC_SCALE_ITEMS);
    run_large_cgroups_lookup_windows(
        "rs_win_cgroups_lookup_large_8192",
        LOOKUP_TOPOLOGY_SCALE_ITEMS,
    );
    run_large_cgroups_lookup_windows("rs_win_cgroups_lookup_large_32768", LOOKUP_HPC_SCALE_ITEMS);
}

fn run_large_apps_response_split_windows(prefix: &str) {
    let svc = unique_service(prefix);
    let pids = large_lookup_pids(LOOKUP_RESPONSE_SPLIT_SCALE_ITEMS);
    let mut cfg = server_config();
    cfg.max_request_payload_bytes = LOOKUP_RESPONSE_SPLIT_REQUEST_PAYLOAD_BYTES;
    cfg.max_response_payload_bytes = LOOKUP_RESPONSE_SPLIT_PAYLOAD_BYTES;
    let mut wake_cfg = client_config();
    wake_cfg.max_request_payload_bytes = LOOKUP_RESPONSE_SPLIT_REQUEST_PAYLOAD_BYTES;
    wake_cfg.max_response_payload_bytes = LOOKUP_RESPONSE_SPLIT_PAYLOAD_BYTES;
    let calls = Arc::new(AtomicU32::new(0));
    let max_seen = Arc::new(AtomicU32::new(0));
    let handler_calls = calls.clone();
    let handler_max = max_seen.clone();
    let label_value = vec![b'l'; LOOKUP_RESPONSE_SPLIT_LABEL_BYTES];
    let handler = apps_lookup_dispatch(Arc::new(move |req, builder| {
        handler_calls.fetch_add(1, Ordering::SeqCst);
        handler_max.fetch_max(req.item_count, Ordering::SeqCst);
        builder.set_generation(9);
        for i in 0..req.item_count {
            let pid = match req.item(i) {
                Ok(pid) => pid,
                Err(_) => return false,
            };
            let labels = [(b"scale".as_slice(), label_value.as_slice())];
            if builder
                .add(
                    PID_LOOKUP_KNOWN,
                    APPS_CGROUP_KNOWN,
                    ORCHESTRATOR_DOCKER,
                    pid,
                    1,
                    1000,
                    42,
                    b"ok",
                    b"/ok",
                    b"name",
                    &labels,
                )
                .is_err()
            {
                return false;
            }
        }
        true
    }));

    let mut server = TestServer::start_with(&svc, cfg, wake_cfg, METHOD_APPS_LOOKUP, handler, 8);
    let mut ccfg = client_config();
    ccfg.max_request_payload_bytes = LOOKUP_RESPONSE_SPLIT_REQUEST_PAYLOAD_BYTES;
    ccfg.max_response_payload_bytes = LOOKUP_RESPONSE_SPLIT_PAYLOAD_BYTES;
    let mut client = apps_lookup_client(&svc, ccfg);
    client.set_call_timeout(LOOKUP_SCALE_CALL_TIMEOUT_MS);
    connect_ready(&mut client);

    let view = client
        .call_apps_lookup(&pids)
        .expect("apps response-split lookup");
    verify_response_split_apps_lookup(&view, &pids);
    assert!(calls.load(Ordering::SeqCst) > LOOKUP_RESPONSE_SPLIT_MIN_CALLS);
    assert_eq!(
        max_seen.load(Ordering::SeqCst),
        LOOKUP_RESPONSE_SPLIT_SCALE_ITEMS as u32
    );

    client.close();
    server.stop();
}

fn run_large_cgroups_response_split_windows(prefix: &str) {
    let svc = unique_service(prefix);
    let paths = large_lookup_paths(LOOKUP_RESPONSE_SPLIT_SCALE_ITEMS);
    let path_refs: Vec<&[u8]> = paths.iter().map(Vec::as_slice).collect();
    let mut cfg = server_config();
    cfg.max_request_payload_bytes = LOOKUP_RESPONSE_SPLIT_REQUEST_PAYLOAD_BYTES;
    cfg.max_response_payload_bytes = LOOKUP_RESPONSE_SPLIT_PAYLOAD_BYTES;
    let mut wake_cfg = client_config();
    wake_cfg.max_request_payload_bytes = LOOKUP_RESPONSE_SPLIT_REQUEST_PAYLOAD_BYTES;
    wake_cfg.max_response_payload_bytes = LOOKUP_RESPONSE_SPLIT_PAYLOAD_BYTES;
    let calls = Arc::new(AtomicU32::new(0));
    let max_seen = Arc::new(AtomicU32::new(0));
    let handler_calls = calls.clone();
    let handler_max = max_seen.clone();
    let label_value = vec![b'l'; LOOKUP_RESPONSE_SPLIT_LABEL_BYTES];
    let handler = cgroups_lookup_dispatch(Arc::new(move |req, builder| {
        handler_calls.fetch_add(1, Ordering::SeqCst);
        handler_max.fetch_max(req.item_count, Ordering::SeqCst);
        builder.set_generation(7);
        for i in 0..req.item_count {
            let path = match req.item(i) {
                Ok(path) => path,
                Err(_) => return false,
            };
            let labels = [(b"scale".as_slice(), label_value.as_slice())];
            if builder
                .add(
                    CGROUP_LOOKUP_KNOWN,
                    ORCHESTRATOR_K8S,
                    path.as_bytes(),
                    b"ok",
                    &labels,
                )
                .is_err()
            {
                return false;
            }
        }
        true
    }));

    let mut server = TestServer::start_with(&svc, cfg, wake_cfg, METHOD_CGROUPS_LOOKUP, handler, 8);
    let mut ccfg = client_config();
    ccfg.max_request_payload_bytes = LOOKUP_RESPONSE_SPLIT_REQUEST_PAYLOAD_BYTES;
    ccfg.max_response_payload_bytes = LOOKUP_RESPONSE_SPLIT_PAYLOAD_BYTES;
    let mut client = cgroups_lookup_client(&svc, ccfg);
    client.set_call_timeout(LOOKUP_SCALE_CALL_TIMEOUT_MS);
    connect_ready(&mut client);

    let view = client
        .call_cgroups_lookup(&path_refs)
        .expect("cgroups response-split lookup");
    verify_response_split_cgroups_lookup(&view, &paths);
    assert!(calls.load(Ordering::SeqCst) > LOOKUP_RESPONSE_SPLIT_MIN_CALLS);
    assert_eq!(
        max_seen.load(Ordering::SeqCst),
        LOOKUP_RESPONSE_SPLIT_SCALE_ITEMS as u32
    );

    client.close();
    server.stop();
}

#[test]
fn test_lookup_large_response_split_calls_windows() {
    run_large_apps_response_split_windows("rs_win_apps_lookup_large_response_split");
    run_large_cgroups_response_split_windows("rs_win_cgroups_lookup_large_response_split");
}

#[test]
fn test_lookup_logical_limits_windows() {
    let mut client = apps_lookup_client("unused", client_config());
    client.set_lookup_logical_config(LookupLogicalConfig {
        max_items: 2,
        max_subcalls: 0,
        max_response_bytes: 0,
    });
    assert!(client.call_apps_lookup(&[1, 2, 3]).is_err());

    let mut client = cgroups_lookup_client("unused", client_config());
    client.set_lookup_logical_config(LookupLogicalConfig {
        max_items: 1,
        max_subcalls: 0,
        max_response_bytes: 0,
    });
    assert!(client
        .call_cgroups_lookup(&[b"/a".as_slice(), b"/b".as_slice()])
        .is_err());

    let svc = unique_service("rs_win_apps_lookup_response_limit");
    let mut server = TestServer::start(&svc, METHOD_APPS_LOOKUP, apps_lookup_dispatch_handler());
    let mut client = apps_lookup_client(&svc, client_config());
    connect_ready(&mut client);
    client.set_lookup_logical_config(LookupLogicalConfig {
        max_items: 0,
        max_subcalls: 0,
        max_response_bytes: 1,
    });
    assert_eq!(
        client.call_apps_lookup(&[123]).unwrap_err(),
        NipcError::Overflow
    );
    client.close();
    server.stop();

    let svc = unique_service("rs_win_cgroups_lookup_response_limit");
    let mut server = TestServer::start(
        &svc,
        METHOD_CGROUPS_LOOKUP,
        cgroups_lookup_dispatch_handler(),
    );
    let mut client = cgroups_lookup_client(&svc, client_config());
    connect_ready(&mut client);
    client.set_lookup_logical_config(LookupLogicalConfig {
        max_items: 0,
        max_subcalls: 0,
        max_response_bytes: 1,
    });
    assert_eq!(
        client
            .call_cgroups_lookup(&[b"/docker/abc".as_slice()])
            .unwrap_err(),
        NipcError::Overflow
    );
    client.close();
    server.stop();

    let svc = unique_service("rs_win_apps_lookup_logical_subcall_limit");
    let mut cfg = server_config();
    cfg.max_request_payload_bytes = 48;
    let mut wake_cfg = client_config();
    wake_cfg.max_request_payload_bytes = 48;
    let mut server = TestServer::start_with(
        &svc,
        cfg,
        wake_cfg,
        METHOD_APPS_LOOKUP,
        apps_lookup_dispatch_handler(),
        8,
    );
    let mut ccfg = client_config();
    ccfg.max_request_payload_bytes = 48;
    let mut client = apps_lookup_client(&svc, ccfg);
    connect_ready(&mut client);
    client.set_lookup_logical_config(LookupLogicalConfig {
        max_items: 0,
        max_subcalls: 1,
        max_response_bytes: 0,
    });
    assert_eq!(
        client.call_apps_lookup(&[1, 2, 3]).unwrap_err(),
        NipcError::Overflow
    );
    assert_eq!(client.status().reconnect_count, 0);
    client.close();
    server.stop();

    let mut ccfg = client_config();
    ccfg.max_request_payload_bytes = 48;
    let mut client = cgroups_lookup_client("unused", ccfg);
    assert_eq!(
        client
            .call_cgroups_lookup(&[b"/request-key-too-large-for-configured-cap".as_slice()])
            .unwrap_err(),
        NipcError::BadLayout
    );
}

#[test]
fn test_cgroups_lookup_rejects_mixed_generation_retry_windows() {
    let svc = unique_service("rs_win_cgroups_lookup_generation");
    let mut cfg = server_config();
    cfg.max_response_payload_bytes = 160;
    let calls = Arc::new(AtomicU32::new(0));
    let handler_calls = calls.clone();
    let handler = cgroups_lookup_dispatch(Arc::new(move |req, builder| {
        let generation = handler_calls.fetch_add(1, Ordering::SeqCst) as u64 + 1;
        builder.set_generation(generation);
        for i in 0..req.item_count {
            let item = match req.item(i) {
                Ok(item) => item,
                Err(_) => return false,
            };
            let name;
            let name_ref: &[u8] = if item.as_bytes() == b"/huge" {
                name = vec![b'x'; 512];
                &name
            } else {
                b"ok"
            };
            if builder
                .add(
                    CGROUP_LOOKUP_KNOWN,
                    ORCHESTRATOR_K8S,
                    item.as_bytes(),
                    name_ref,
                    &[],
                )
                .is_err()
            {
                return false;
            }
        }
        true
    }));

    let mut server = TestServer::start_with(
        &svc,
        cfg,
        client_config(),
        METHOD_CGROUPS_LOOKUP,
        handler,
        8,
    );
    let mut client = cgroups_lookup_client(&svc, client_config());
    connect_ready(&mut client);

    assert!(client
        .call_cgroups_lookup(&[b"/a".as_slice(), b"/huge".as_slice(), b"/b".as_slice()])
        .is_err());
    assert!(
        calls.load(Ordering::SeqCst) >= 2,
        "handler should be called for at least two subrequests"
    );

    client.close();
    server.stop();
}

#[test]
fn test_apps_lookup_rejects_mixed_generation_retry_windows() {
    let svc = unique_service("rs_win_apps_lookup_generation");
    let mut cfg = server_config();
    cfg.max_response_payload_bytes = 320;
    let calls = Arc::new(AtomicU32::new(0));
    let handler_calls = calls.clone();
    let handler = apps_lookup_dispatch(Arc::new(move |req, builder| {
        let generation = handler_calls.fetch_add(1, Ordering::SeqCst) as u64 + 1;
        builder.set_generation(generation);
        for i in 0..req.item_count {
            let pid = match req.item(i) {
                Ok(pid) => pid,
                Err(_) => return false,
            };
            let cgroup_path;
            let cgroup_path_ref: &[u8] = if pid == 22 {
                cgroup_path = [b"/".as_slice(), &vec![b'x'; 1024]].concat();
                &cgroup_path
            } else {
                b"/ok"
            };
            if builder
                .add(
                    PID_LOOKUP_KNOWN,
                    APPS_CGROUP_KNOWN,
                    0,
                    pid,
                    0,
                    0,
                    1,
                    b"ok",
                    cgroup_path_ref,
                    b"name",
                    &[],
                )
                .is_err()
            {
                return false;
            }
        }
        true
    }));

    let mut server =
        TestServer::start_with(&svc, cfg, client_config(), METHOD_APPS_LOOKUP, handler, 8);
    let mut client = apps_lookup_client(&svc, client_config());
    connect_ready(&mut client);

    assert!(client.call_apps_lookup(&[11, 22, 33]).is_err());
    assert!(
        calls.load(Ordering::SeqCst) >= 2,
        "handler should be called for at least two subrequests"
    );

    client.close();
    server.stop();
}

#[test]
fn test_lookup_rejects_malformed_followup_response_windows() {
    let svc = unique_service("rs_win_apps_lookup_bad_followup");
    let calls = Arc::new(AtomicU32::new(0));
    let handler_calls = calls.clone();
    let handler = apps_lookup_dispatch(Arc::new(move |req, builder| {
        let call = handler_calls.fetch_add(1, Ordering::SeqCst) + 1;
        builder.set_generation(88);
        for i in 0..req.item_count {
            let pid = match req.item(i) {
                Ok(pid) => pid,
                Err(_) => return false,
            };
            if call == 1 && i > 0 {
                if builder
                    .add(
                        PID_LOOKUP_PAYLOAD_EXCEEDED,
                        0,
                        0,
                        pid,
                        0,
                        NIPC_UID_UNSET,
                        0,
                        b"",
                        b"",
                        b"",
                        &[],
                    )
                    .is_err()
                {
                    return false;
                }
                continue;
            }

            let mut echo_pid = pid;
            if call > 1 && i == 0 {
                echo_pid = if pid == 0 { 1 } else { 0 };
            }
            if builder
                .add(
                    PID_LOOKUP_UNKNOWN,
                    0,
                    0,
                    echo_pid,
                    0,
                    NIPC_UID_UNSET,
                    0,
                    b"",
                    b"",
                    b"",
                    &[],
                )
                .is_err()
            {
                return false;
            }
        }
        true
    }));
    let mut server = TestServer::start_with(
        &svc,
        server_config(),
        client_config(),
        METHOD_APPS_LOOKUP,
        handler,
        8,
    );
    let mut client = apps_lookup_client(&svc, client_config());
    connect_ready(&mut client);

    let err = client
        .call_apps_lookup(&[11, 22, 33])
        .expect_err("apps malformed follow-up");
    assert_eq!(err, NipcError::BadLayout);
    assert!(
        calls.load(Ordering::SeqCst) >= 2,
        "handler should be called for at least two subrequests"
    );
    client.close();
    server.stop();

    let svc = unique_service("rs_win_cgroups_lookup_bad_followup");
    let calls = Arc::new(AtomicU32::new(0));
    let handler_calls = calls.clone();
    let handler = cgroups_lookup_dispatch(Arc::new(move |req, builder| {
        let call = handler_calls.fetch_add(1, Ordering::SeqCst) + 1;
        builder.set_generation(77);
        for i in 0..req.item_count {
            let item = match req.item(i) {
                Ok(item) => item,
                Err(_) => return false,
            };
            if call == 1 && i > 0 {
                if builder
                    .add(CGROUP_LOOKUP_PAYLOAD_EXCEEDED, 0, item.as_bytes(), b"", &[])
                    .is_err()
                {
                    return false;
                }
                continue;
            }

            let echo_path = if call > 1 && i == 0 {
                b"/wrong".as_slice()
            } else {
                item.as_bytes()
            };
            if builder
                .add(CGROUP_LOOKUP_UNKNOWN_RETRY_LATER, 0, echo_path, b"", &[])
                .is_err()
            {
                return false;
            }
        }
        true
    }));
    let mut server = TestServer::start_with(
        &svc,
        server_config(),
        client_config(),
        METHOD_CGROUPS_LOOKUP,
        handler,
        8,
    );
    let mut client = cgroups_lookup_client(&svc, client_config());
    connect_ready(&mut client);

    let err = client
        .call_cgroups_lookup(&[b"/a".as_slice(), b"/b".as_slice(), b"/c".as_slice()])
        .expect_err("cgroups malformed follow-up");
    assert_eq!(err, NipcError::BadLayout);
    assert!(
        calls.load(Ordering::SeqCst) >= 2,
        "handler should be called for at least two subrequests"
    );
    client.close();
    server.stop();
}

fn apps_lookup_partial_response(payload: &[u8]) -> Result<(Vec<u8>, u32), String> {
    let request =
        AppsLookupRequestView::decode(payload).map_err(|e| format!("apps request decode: {e}"))?;
    let mut response = vec![0u8; 1024];
    let response_len = {
        let mut builder = AppsLookupBuilder::new(&mut response, request.item_count, 88);
        for i in 0..request.item_count {
            let pid = request.item(i).map_err(|e| format!("apps item {i}: {e}"))?;
            let status = if i == 0 {
                PID_LOOKUP_UNKNOWN
            } else {
                PID_LOOKUP_PAYLOAD_EXCEEDED
            };
            builder
                .add(status, 0, 0, pid, 0, NIPC_UID_UNSET, 0, b"", b"", b"", &[])
                .map_err(|e| format!("apps response item {i}: {e}"))?;
        }
        builder
            .finish()
            .map_err(|e| format!("apps response finish: {e}"))?
    };
    response.truncate(response_len);
    Ok((response, request.item_count))
}

fn cgroups_lookup_partial_response(payload: &[u8]) -> Result<(Vec<u8>, u32), String> {
    let request = CgroupsLookupRequestView::decode(payload)
        .map_err(|e| format!("cgroups request decode: {e}"))?;
    let mut response = vec![0u8; 1024];
    let response_len = {
        let mut builder = CgroupsLookupBuilder::new(&mut response, request.item_count, 77);
        for i in 0..request.item_count {
            let path = request
                .item(i)
                .map_err(|e| format!("cgroups item {i}: {e}"))?;
            let (status, orchestrator, name): (u16, u16, &[u8]) = if i == 0 {
                (CGROUP_LOOKUP_KNOWN, ORCHESTRATOR_K8S, b"ok")
            } else {
                (CGROUP_LOOKUP_PAYLOAD_EXCEEDED, 0, b"")
            };
            builder
                .add(status, orchestrator, path.as_bytes(), name, &[])
                .map_err(|e| format!("cgroups response item {i}: {e}"))?;
        }
        builder
            .finish()
            .map_err(|e| format!("cgroups response finish: {e}"))?
    };
    response.truncate(response_len);
    Ok((response, request.item_count))
}

#[test]
fn test_lookup_endpoint_gone_after_partial_progress_windows() {
    let svc = unique_service("rs_win_apps_lookup_partial_disconnect");
    let mut server = start_raw_session_server(&svc, server_config(), |session, hdr, payload| {
        if hdr.code != METHOD_APPS_LOOKUP {
            return Err(format!("unexpected apps request header: {hdr:?}"));
        }
        let (response_payload, _item_count) = apps_lookup_partial_response(payload)?;
        let mut resp_hdr = Header {
            kind: KIND_RESPONSE,
            code: METHOD_APPS_LOOKUP,
            flags: 0,
            item_count: 1,
            message_id: hdr.message_id,
            transport_status: STATUS_OK,
            ..Header::default()
        };
        session
            .send(&mut resp_hdr, &response_payload)
            .map_err(|e| format!("apps response send: {e}"))
    });
    let mut client = apps_lookup_client(&svc, client_config());
    connect_ready(&mut client);
    assert!(client
        .call_apps_lookup_with_timeout(&[11, 22, 33], 1_000)
        .is_err());
    client.close();
    server.wait();

    let svc = unique_service("rs_win_cgroups_lookup_partial_disconnect");
    let mut server = start_raw_session_server(&svc, server_config(), |session, hdr, payload| {
        if hdr.code != METHOD_CGROUPS_LOOKUP {
            return Err(format!("unexpected cgroups request header: {hdr:?}"));
        }
        let (response_payload, _item_count) = cgroups_lookup_partial_response(payload)?;
        let mut resp_hdr = Header {
            kind: KIND_RESPONSE,
            code: METHOD_CGROUPS_LOOKUP,
            flags: 0,
            item_count: 1,
            message_id: hdr.message_id,
            transport_status: STATUS_OK,
            ..Header::default()
        };
        session
            .send(&mut resp_hdr, &response_payload)
            .map_err(|e| format!("cgroups response send: {e}"))
    });
    let mut client = cgroups_lookup_client(&svc, client_config());
    connect_ready(&mut client);
    assert!(client
        .call_cgroups_lookup_with_timeout(
            &[b"/a".as_slice(), b"/b".as_slice(), b"/c".as_slice()],
            1_000,
        )
        .is_err());
    client.close();
    server.wait();
}

#[test]
fn test_lookup_endpoint_gone_before_first_subcall_windows() {
    let svc = unique_service("rs_win_apps_lookup_gone_before_call");
    let mut server = TestServer::start(&svc, METHOD_APPS_LOOKUP, apps_lookup_dispatch_handler());
    let mut client = apps_lookup_client(&svc, client_config());
    connect_ready(&mut client);
    server.stop();
    assert!(client
        .call_apps_lookup_with_timeout(&[11, 22], 1_000)
        .is_err());
    client.close();

    let svc = unique_service("rs_win_cgroups_lookup_gone_before_call");
    let mut server = TestServer::start(
        &svc,
        METHOD_CGROUPS_LOOKUP,
        cgroups_lookup_dispatch_handler(),
    );
    let mut client = cgroups_lookup_client(&svc, client_config());
    connect_ready(&mut client);
    server.stop();
    assert!(client
        .call_cgroups_lookup_with_timeout(&[b"/a".as_slice(), b"/b".as_slice()], 1_000)
        .is_err());
    client.close();
}

#[test]
fn test_lookup_endpoint_absent_before_call_windows() {
    let svc = unique_service("rs_win_apps_lookup_absent");
    let mut client = apps_lookup_client(&svc, client_config());
    assert!(client.refresh(), "apps absent refresh should change state");
    assert_eq!(client.state, ClientState::NotFound);
    let err = client
        .call_apps_lookup_with_timeout(&[11, 22], 1_000)
        .expect_err("apps absent lookup should fail");
    assert_eq!(err, NipcError::BadLayout);
    client.close();

    let svc = unique_service("rs_win_cgroups_lookup_absent");
    let mut client = cgroups_lookup_client(&svc, client_config());
    assert!(
        client.refresh(),
        "cgroups absent refresh should change state"
    );
    assert_eq!(client.state, ClientState::NotFound);
    let err = client
        .call_cgroups_lookup_with_timeout(&[b"/a".as_slice(), b"/b".as_slice()], 1_000)
        .expect_err("cgroups absent lookup should fail");
    assert_eq!(err, NipcError::BadLayout);
    client.close();
}

#[test]
fn test_lookup_rejects_malformed_typed_responses_windows() {
    assert_apps_lookup_bad_response_windows(
        "truncated",
        &[1234],
        vec![1, 2, 3],
        NipcError::Truncated,
    );
    assert_apps_lookup_bad_response_windows(
        "count",
        &[1234],
        apps_lookup_response_payload_count_zero(),
        NipcError::BadItemCount,
    );
    assert_apps_lookup_bad_response_windows(
        "pid",
        &[1234],
        apps_lookup_response_payload_unknown(9999),
        NipcError::BadLayout,
    );
    assert_apps_lookup_bad_response_windows(
        "reordered",
        &[1, 2],
        apps_lookup_response_payload_unknowns(&[2, 1]),
        NipcError::BadLayout,
    );
    assert_apps_lookup_bad_response_windows(
        "duplicate",
        &[1, 2],
        apps_lookup_response_payload_unknowns(&[1, 1]),
        NipcError::BadLayout,
    );
    let mut payload = apps_lookup_response_payload_known_with_label(1234);
    patch_lookup_item_status(&mut payload, APPS_LOOKUP_RESP_HDR_SIZE, 1, 0, 0xffff);
    assert_apps_lookup_bad_response_windows(
        "invalid_status",
        &[1234],
        payload,
        NipcError::BadLayout,
    );
    let mut payload = apps_lookup_response_payload_known_with_label(1234);
    patch_lookup_item_status(
        &mut payload,
        APPS_LOOKUP_RESP_HDR_SIZE,
        1,
        0,
        PID_LOOKUP_UNKNOWN,
    );
    assert_apps_lookup_bad_response_windows(
        "invalid_status_fields",
        &[1234],
        payload,
        NipcError::BadLayout,
    );
    let mut payload = apps_lookup_response_payload_known_with_label(1234);
    patch_lookup_item_u16(&mut payload, APPS_LOOKUP_RESP_HDR_SIZE, 1, 0, 56, 2);
    assert_apps_lookup_bad_response_windows(
        "invalid_label_table",
        &[1234],
        payload,
        NipcError::OutOfBounds,
    );
    assert_apps_lookup_bad_response_windows(
        "first_payload_exceeded",
        &[1, 2],
        apps_lookup_response_payload_exceeded_suffix(&[1, 2], false),
        NipcError::Overflow,
    );
    assert_apps_lookup_bad_response_windows(
        "bad_payload_exceeded_suffix",
        &[1, 2],
        apps_lookup_response_payload_exceeded_suffix(&[1, 2], true),
        NipcError::BadLayout,
    );

    assert_cgroups_lookup_bad_response_windows(
        "truncated",
        &[b"/a"],
        vec![1, 2, 3],
        NipcError::Truncated,
    );
    assert_cgroups_lookup_bad_response_windows(
        "count",
        &[b"/a"],
        cgroups_lookup_response_payload_count_zero(),
        NipcError::BadItemCount,
    );
    assert_cgroups_lookup_bad_response_windows(
        "path",
        &[b"/a"],
        cgroups_lookup_response_payload_unknown(b"/other"),
        NipcError::BadLayout,
    );
    assert_cgroups_lookup_bad_response_windows(
        "reordered",
        &[b"/a", b"/b"],
        cgroups_lookup_response_payload_unknowns(&[b"/b", b"/a"]),
        NipcError::BadLayout,
    );
    assert_cgroups_lookup_bad_response_windows(
        "duplicate",
        &[b"/a", b"/b"],
        cgroups_lookup_response_payload_unknowns(&[b"/a", b"/a"]),
        NipcError::BadLayout,
    );
    let mut payload = cgroups_lookup_response_payload_known_with_label(b"/a");
    patch_lookup_item_status(&mut payload, CGROUPS_LOOKUP_RESP_HDR_SIZE, 1, 0, 0xffff);
    assert_cgroups_lookup_bad_response_windows(
        "invalid_status",
        &[b"/a"],
        payload,
        NipcError::BadLayout,
    );
    let mut payload = cgroups_lookup_response_payload_known_with_label(b"/a");
    patch_lookup_item_status(
        &mut payload,
        CGROUPS_LOOKUP_RESP_HDR_SIZE,
        1,
        0,
        CGROUP_LOOKUP_UNKNOWN_RETRY_LATER,
    );
    assert_cgroups_lookup_bad_response_windows(
        "invalid_status_fields",
        &[b"/a"],
        payload,
        NipcError::BadLayout,
    );
    let mut payload = cgroups_lookup_response_payload_known_with_label(b"/a");
    patch_lookup_item_u16(&mut payload, CGROUPS_LOOKUP_RESP_HDR_SIZE, 1, 0, 24, 2);
    assert_cgroups_lookup_bad_response_windows(
        "invalid_label_table",
        &[b"/a"],
        payload,
        NipcError::OutOfBounds,
    );
    assert_cgroups_lookup_bad_response_windows(
        "first_payload_exceeded",
        &[b"/a", b"/b"],
        cgroups_lookup_response_payload_exceeded_suffix(&[b"/a", b"/b"], false),
        NipcError::Overflow,
    );
    assert_cgroups_lookup_bad_response_windows(
        "bad_payload_exceeded_suffix",
        &[b"/a", b"/b"],
        cgroups_lookup_response_payload_exceeded_suffix(&[b"/a", b"/b"], true),
        NipcError::BadLayout,
    );
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

    let cache = CgroupsCache::new(TEST_RUN_DIR, svc, client_config());
    assert!(!cache.ready());

    thread::sleep(Duration::from_millis(2));

    let updated = cache.refresh();
    assert!(updated);
    assert!(cache.ready());

    let item = {
        let guard = cache.read_lock();
        guard.get(1001, "docker-abc123").expect("lookup").dup()
    };
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
        client.transport_receive(0),
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
