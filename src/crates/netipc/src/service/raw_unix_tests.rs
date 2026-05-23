use super::*;
#[cfg(target_os = "linux")]
use crate::protocol::PROFILE_SHM_FUTEX;
use crate::protocol::{increment_encode, BatchBuilder, CgroupsBuilder, PROFILE_BASELINE};
use std::os::fd::RawFd;
use std::os::unix::ffi::OsStrExt;
use std::path::PathBuf;
use std::thread;
use std::time::Duration;

const TEST_RUN_DIR: &str = "/tmp/nipc_svc_rust_test";
const AUTH_TOKEN: u64 = 0xDEADBEEFCAFEBABE;
const RESPONSE_BUF_SIZE: usize = 65536;
static RAW_SERVICE_COUNTER: std::sync::atomic::AtomicU64 = std::sync::atomic::AtomicU64::new(0);

fn ensure_run_dir() {
    let _ = std::fs::create_dir_all(TEST_RUN_DIR);
}

fn cleanup_all(service: &str) {
    let _ = std::fs::remove_file(format!("{TEST_RUN_DIR}/{service}.sock"));
    #[cfg(target_os = "linux")]
    crate::transport::shm::cleanup_stale(TEST_RUN_DIR, service);
}

fn socket_path(service: &str) -> PathBuf {
    PathBuf::from(format!("{TEST_RUN_DIR}/{service}.sock"))
}

fn unique_service(prefix: &str) -> String {
    format!(
        "{}_{}_{}",
        prefix,
        std::process::id(),
        RAW_SERVICE_COUNTER.fetch_add(1, std::sync::atomic::Ordering::Relaxed) + 1
    )
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
        max_request_payload_bytes: 4096,
        max_request_batch_items: 1,
        max_response_payload_bytes: RESPONSE_BUF_SIZE as u32,
        max_response_batch_items: 1,
        auth_token: AUTH_TOKEN,
        backlog: 4,
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

#[cfg(target_os = "linux")]
fn shm_server_config() -> ServerConfig {
    ServerConfig {
        supported_profiles: PROFILE_BASELINE | PROFILE_SHM_FUTEX,
        preferred_profiles: PROFILE_SHM_FUTEX,
        max_request_payload_bytes: 4096,
        max_request_batch_items: 16,
        max_response_payload_bytes: RESPONSE_BUF_SIZE as u32,
        max_response_batch_items: 16,
        auth_token: AUTH_TOKEN,
        backlog: 4,
        ..ServerConfig::default()
    }
}

#[cfg(target_os = "linux")]
fn shm_client_config() -> ClientConfig {
    ClientConfig {
        supported_profiles: PROFILE_BASELINE | PROFILE_SHM_FUTEX,
        preferred_profiles: PROFILE_SHM_FUTEX,
        max_request_payload_bytes: 4096,
        max_request_batch_items: 16,
        max_response_payload_bytes: RESPONSE_BUF_SIZE as u32,
        max_response_batch_items: 16,
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
        backlog: 4,
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

fn panic_payload_to_string(payload: Box<dyn std::any::Any + Send>) -> String {
    match payload.downcast::<String>() {
        Ok(msg) => *msg,
        Err(payload) => match payload.downcast::<&'static str>() {
            Ok(msg) => (*msg).to_string(),
            Err(_) => "<non-string panic>".to_string(),
        },
    }
}

fn send_raw_packet(fd: i32, data: &[u8]) {
    let sent = unsafe { libc::send(fd, data.as_ptr() as *const libc::c_void, data.len(), 0) };
    assert_eq!(
        sent,
        data.len() as isize,
        "raw send failed: {:?}",
        std::io::Error::last_os_error()
    );
}

fn build_increment_request_message(message_id: u64, value: u64) -> Vec<u8> {
    let mut payload = [0u8; INCREMENT_PAYLOAD_SIZE];
    let payload_len = increment_encode(value, &mut payload);
    assert_eq!(
        payload_len, INCREMENT_PAYLOAD_SIZE,
        "increment_encode should fit the fixed-size request buffer"
    );

    let hdr = Header {
        magic: MAGIC_MSG,
        version: VERSION,
        header_len: protocol::HEADER_LEN,
        kind: KIND_REQUEST,
        code: METHOD_INCREMENT,
        flags: 0,
        payload_len: payload_len as u32,
        item_count: 1,
        message_id,
        transport_status: STATUS_OK,
    };

    let mut msg = vec![0u8; HEADER_SIZE + payload_len];
    hdr.encode(&mut msg[..HEADER_SIZE]);
    msg[HEADER_SIZE..].copy_from_slice(&payload[..payload_len]);
    msg
}

fn verify_increment_service_ok(service: &str, config: ClientConfig) {
    let mut verify = increment_client(service, config);
    connect_ready(&mut verify);
    assert_eq!(
        verify.call_increment(1).expect("verification call"),
        2,
        "server should remain healthy after rejecting the malformed session"
    );
    verify.close();
}

fn test_cgroups_snapshot_handler() -> SnapshotHandler {
    Arc::new(|req, builder| {
        if req.layout_version != 1 || req.flags != 0 {
            return false;
        }
        builder.set_header(1, 42);
        fill_test_cgroups_snapshot(builder)
    })
}

fn test_cgroups_dispatch() -> DispatchHandler {
    snapshot_dispatch(test_cgroups_snapshot_handler(), 3)
}

fn increment_handler() -> IncrementHandler {
    Arc::new(|value| Some(value + 1))
}

fn increment_dispatch_handler() -> DispatchHandler {
    increment_dispatch(increment_handler())
}

fn string_reverse_handler() -> StringReverseHandler {
    Arc::new(|s| Some(s.chars().rev().collect()))
}

fn string_reverse_dispatch_handler() -> DispatchHandler {
    string_reverse_dispatch(string_reverse_handler())
}

struct TestServer {
    stop_flag: Arc<AtomicBool>,
    thread: Option<thread::JoinHandle<()>>,
}

impl TestServer {
    fn start(service: &str, expected_method_code: u16, handler: Option<DispatchHandler>) -> Self {
        Self::start_with(service, server_config(), expected_method_code, handler, 8)
    }

    #[cfg(target_os = "linux")]
    fn start_shm(
        service: &str,
        expected_method_code: u16,
        handler: Option<DispatchHandler>,
    ) -> Self {
        Self::start_with(
            service,
            shm_server_config(),
            expected_method_code,
            handler,
            8,
        )
    }

    fn start_with_workers(
        service: &str,
        expected_method_code: u16,
        handler: Option<DispatchHandler>,
        worker_count: usize,
    ) -> Self {
        Self::start_with(
            service,
            server_config(),
            expected_method_code,
            handler,
            worker_count,
        )
    }

    fn start_with(
        service: &str,
        config: ServerConfig,
        expected_method_code: u16,
        handler: Option<DispatchHandler>,
        worker_count: usize,
    ) -> Self {
        ensure_run_dir();
        cleanup_all(service);

        let svc = service.to_string();
        let ready_flag = Arc::new(AtomicBool::new(false));
        let ready_clone = ready_flag.clone();

        let mut server = ManagedServer::with_workers(
            TEST_RUN_DIR,
            &svc,
            config,
            expected_method_code,
            handler,
            worker_count,
        );
        let stop_flag = server.running_flag();

        let thread = thread::spawn(move || {
            // We need to signal readiness after bind but before accept loop.
            // The run() method binds internally, so we signal immediately
            // after it starts (it blocks on accept).
            ready_clone.store(true, Ordering::Release);
            let _ = server.run();
        });

        // Wait for server to be ready
        for _ in 0..2000 {
            if ready_flag.load(Ordering::Acquire) {
                break;
            }
            thread::sleep(Duration::from_micros(500));
        }
        wait_for_listener_bind(service);

        TestServer {
            stop_flag,
            thread: Some(thread),
        }
    }

    fn start_with_resp_size(
        service: &str,
        expected_method_code: u16,
        handler: Option<DispatchHandler>,
        resp_buf_size: usize,
    ) -> Self {
        ensure_run_dir();
        cleanup_all(service);

        let svc = service.to_string();
        let ready_flag = Arc::new(AtomicBool::new(false));
        let ready_clone = ready_flag.clone();

        let mut scfg = server_config();
        scfg.max_response_payload_bytes = resp_buf_size as u32;

        let mut server =
            ManagedServer::new(TEST_RUN_DIR, &svc, scfg, expected_method_code, handler);
        let stop_flag = server.running_flag();

        let thread = thread::spawn(move || {
            ready_clone.store(true, Ordering::Release);
            let _ = server.run();
        });

        for _ in 0..2000 {
            if ready_flag.load(Ordering::Acquire) {
                break;
            }
            thread::sleep(Duration::from_micros(500));
        }
        wait_for_listener_bind(service);

        TestServer {
            stop_flag,
            thread: Some(thread),
        }
    }

    fn stop(&mut self) {
        self.stop_flag.store(false, Ordering::Release);
        if let Some(t) = self.thread.take() {
            let _ = t.join();
        }
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

struct RawHelloAckServer {
    thread: Option<thread::JoinHandle<Result<(), String>>>,
}

fn raw_listener_fd_for_service(service: &str) -> RawFd {
    let path = socket_path(service);
    let path_bytes = path.as_os_str().as_bytes();
    assert!(
        path_bytes.len() < std::mem::size_of::<libc::sockaddr_un>() - 2,
        "socket path too long"
    );

    let fd = unsafe { libc::socket(libc::AF_UNIX, libc::SOCK_SEQPACKET, 0) };
    assert!(
        fd >= 0,
        "socket failed: {}",
        std::io::Error::last_os_error()
    );

    let mut addr: libc::sockaddr_un = unsafe { std::mem::zeroed() };
    addr.sun_family = libc::AF_UNIX as libc::sa_family_t;
    for (idx, byte) in path_bytes.iter().enumerate() {
        addr.sun_path[idx] = *byte as libc::c_char;
    }

    let rc = unsafe {
        libc::bind(
            fd,
            &addr as *const libc::sockaddr_un as *const libc::sockaddr,
            std::mem::size_of::<libc::sockaddr_un>() as libc::socklen_t,
        )
    };
    assert_eq!(rc, 0, "bind failed: {}", std::io::Error::last_os_error());

    let rc = unsafe { libc::listen(fd, 4) };
    assert_eq!(rc, 0, "listen failed: {}", std::io::Error::last_os_error());
    fd
}

fn hello_ack_packet_with_version(version: u16, status: u16, layout_version: u16) -> Vec<u8> {
    let ack = crate::protocol::HelloAck {
        layout_version,
        flags: 0,
        server_supported_profiles: crate::protocol::PROFILE_BASELINE,
        intersection_profiles: crate::protocol::PROFILE_BASELINE,
        selected_profile: crate::protocol::PROFILE_BASELINE,
        agreed_max_request_payload_bytes: crate::protocol::MAX_PAYLOAD_DEFAULT,
        agreed_max_request_batch_items: 1,
        agreed_max_response_payload_bytes: RESPONSE_BUF_SIZE as u32,
        agreed_max_response_batch_items: 1,
        agreed_packet_size: 0,
        session_id: 77,
    };

    let mut payload = vec![0u8; 48];
    let payload_len = ack.encode(&mut payload);
    payload.truncate(payload_len);

    let hdr = crate::protocol::Header {
        magic: crate::protocol::MAGIC_MSG,
        version,
        header_len: crate::protocol::HEADER_LEN,
        kind: crate::protocol::KIND_CONTROL,
        flags: 0,
        code: crate::protocol::CODE_HELLO_ACK,
        transport_status: status,
        payload_len: payload.len() as u32,
        item_count: 1,
        message_id: 0,
    };

    let mut pkt = vec![0u8; crate::protocol::HEADER_SIZE + payload.len()];
    hdr.encode(&mut pkt[..crate::protocol::HEADER_SIZE]);
    pkt[crate::protocol::HEADER_SIZE..].copy_from_slice(&payload);
    pkt
}

fn start_raw_hello_ack_server(service: &str, packet: Vec<u8>) -> RawHelloAckServer {
    ensure_run_dir();
    cleanup_all(service);

    let svc = service.to_string();
    let thread = thread::spawn(move || {
        let fd = raw_listener_fd_for_service(&svc);
        let client_fd = unsafe { libc::accept(fd, std::ptr::null_mut(), std::ptr::null_mut()) };
        if client_fd < 0 {
            unsafe { libc::close(fd) };
            return Err(format!("accept: {}", std::io::Error::last_os_error()));
        }

        let mut hello_buf = [0u8; crate::protocol::HEADER_SIZE + 128];
        let n = unsafe {
            libc::recv(
                client_fd,
                hello_buf.as_mut_ptr() as *mut libc::c_void,
                hello_buf.len(),
                0,
            )
        };
        if n < 0 {
            unsafe {
                libc::close(client_fd);
                libc::close(fd);
            }
            return Err(format!("recv: {}", std::io::Error::last_os_error()));
        }

        let wrote = unsafe {
            libc::send(
                client_fd,
                packet.as_ptr() as *const libc::c_void,
                packet.len(),
                0,
            )
        };
        unsafe {
            libc::close(client_fd);
            libc::close(fd);
        }
        if wrote != packet.len() as isize {
            return Err(format!(
                "send short write: wrote {wrote}, want {}",
                packet.len()
            ));
        }

        Ok(())
    });

    thread::sleep(Duration::from_millis(50));
    RawHelloAckServer {
        thread: Some(thread),
    }
}

impl RawHelloAckServer {
    fn wait(&mut self) {
        if let Some(thread) = self.thread.take() {
            match thread.join() {
                Ok(Ok(())) => {}
                Ok(Err(err)) => panic!("raw hello-ack server failed: {err}"),
                Err(_) => panic!("raw hello-ack server panicked"),
            }
        }
    }
}

fn start_raw_session_server<F>(service: &str, cfg: ServerConfig, handler: F) -> RawSessionServer
where
    F: FnOnce(&mut UdsSession, Header, &[u8]) -> Result<(), String> + Send + 'static,
{
    ensure_run_dir();
    cleanup_all(service);

    let svc = service.to_string();
    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();

    let thread = thread::spawn(move || {
        let listener =
            UdsListener::bind(TEST_RUN_DIR, &svc, cfg).map_err(|e| format!("bind: {e}"))?;
        ready_clone.store(true, Ordering::Release);
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

    for _ in 0..2000 {
        if ready.load(Ordering::Acquire) {
            break;
        }
        thread::sleep(Duration::from_micros(500));
    }
    thread::sleep(Duration::from_millis(50));

    RawSessionServer {
        thread: Some(thread),
    }
}

impl RawSessionServer {
    fn wait(&mut self) {
        if let Some(thread) = self.thread.take() {
            match thread.join() {
                Ok(Ok(())) => {}
                Ok(Err(err)) => panic!("raw unix session server failed: {err}"),
                Err(_) => panic!("raw unix session server panicked"),
            }
        }
    }
}

#[cfg(target_os = "linux")]
struct RawShmSessionServer {
    thread: Option<thread::JoinHandle<Result<(), String>>>,
}

#[cfg(target_os = "linux")]
fn start_raw_shm_session_server<F>(
    service: &str,
    cfg: ServerConfig,
    handler: F,
) -> RawShmSessionServer
where
    F: FnOnce(&mut ShmContext, Header, &[u8]) -> Result<(), String> + Send + 'static,
{
    ensure_run_dir();
    cleanup_all(service);

    let svc = service.to_string();
    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();

    let thread = thread::spawn(move || {
        let listener =
            UdsListener::bind(TEST_RUN_DIR, &svc, cfg).map_err(|e| format!("bind: {e}"))?;
        ready_clone.store(true, Ordering::Release);
        let session = listener.accept().map_err(|e| format!("accept: {e}"))?;
        if session.selected_profile != PROFILE_SHM_FUTEX {
            return Err(format!("unexpected profile {}", session.selected_profile));
        }

        let mut shm = ShmContext::server_create(
            TEST_RUN_DIR,
            &svc,
            session.session_id,
            session.max_request_payload_bytes + HEADER_SIZE as u32,
            session.max_response_payload_bytes + HEADER_SIZE as u32,
        )
        .map_err(|e| format!("server_create: {e}"))?;

        let mut recv_buf = vec![0u8; session.max_request_payload_bytes as usize + HEADER_SIZE];
        let mlen = shm
            .receive(&mut recv_buf, 5000)
            .map_err(|e| format!("shm receive: {e}"))?;
        if mlen < HEADER_SIZE {
            return Err(format!("request too short: {mlen}"));
        }

        let hdr = Header::decode(&recv_buf[..mlen]).map_err(|e| format!("decode: {e:?}"))?;
        let payload = recv_buf[HEADER_SIZE..mlen].to_vec();
        handler(&mut shm, hdr, &payload)
    });

    for _ in 0..2000 {
        if ready.load(Ordering::Acquire) {
            break;
        }
        thread::sleep(Duration::from_micros(500));
    }
    thread::sleep(Duration::from_millis(50));

    RawShmSessionServer {
        thread: Some(thread),
    }
}

#[cfg(target_os = "linux")]
impl RawShmSessionServer {
    fn wait(&mut self) {
        if let Some(thread) = self.thread.take() {
            match thread.join() {
                Ok(Ok(())) => {}
                Ok(Err(err)) => panic!("raw shm session server failed: {err}"),
                Err(_) => panic!("raw shm session server panicked"),
            }
        }
    }
}

#[cfg(target_os = "linux")]
fn encode_raw_message(hdr: &Header, payload: &[u8]) -> Vec<u8> {
    let mut msg = vec![0u8; HEADER_SIZE + payload.len()];
    hdr.encode(&mut msg[..HEADER_SIZE]);
    if !payload.is_empty() {
        msg[HEADER_SIZE..].copy_from_slice(payload);
    }
    msg
}

#[test]
fn test_client_lifecycle() {
    let svc = "rs_svc_lifecycle";
    ensure_run_dir();
    cleanup_all(svc);

    // Init without server running
    let mut client = snapshot_client(svc, client_config());
    assert_eq!(client.state, ClientState::Disconnected);
    assert!(!client.ready());

    // Refresh without server -> NOT_FOUND
    let changed = client.refresh();
    assert!(changed);
    assert_eq!(client.state, ClientState::NotFound);

    // Start server
    let mut server = TestServer::start(svc, METHOD_CGROUPS_SNAPSHOT, Some(test_cgroups_dispatch()));

    // Refresh -> READY
    let changed = client.refresh();
    assert!(changed);
    assert_eq!(client.state, ClientState::Ready);
    assert!(client.ready());

    // Status reporting
    let status = client.status();
    assert_eq!(status.connect_count, 1);
    assert_eq!(status.reconnect_count, 0);

    // Close
    client.close();
    assert_eq!(client.state, ClientState::Disconnected);
    assert!(!client.ready());

    server.stop();
    cleanup_all(svc);
}

#[test]
fn test_fill_test_cgroups_snapshot_small_builder_returns_false() {
    let mut buf = [0u8; 64];
    let mut builder = CgroupsBuilder::new(&mut buf, 3, 0, 0);
    assert!(
        !fill_test_cgroups_snapshot(&mut builder),
        "small builder should reject the synthetic snapshot"
    );
}

#[test]
fn test_snapshot_handler_rejects_bad_request_metadata() {
    let on_snapshot = test_cgroups_snapshot_handler();
    let req = CgroupsRequest {
        layout_version: 2,
        flags: 0,
    };
    let mut response_payload = [0u8; RESPONSE_BUF_SIZE];
    let mut builder = CgroupsBuilder::new(&mut response_payload, 4, 1, 99);
    assert!(
        !on_snapshot(&req, &mut builder),
        "snapshot handler should reject bad request metadata"
    );
}

#[test]
fn test_cgroups_call() {
    let svc = "rs_svc_cgroups";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server = TestServer::start(svc, METHOD_CGROUPS_SNAPSHOT, Some(test_cgroups_dispatch()));

    let mut client = snapshot_client(svc, client_config());
    client.refresh();
    assert!(client.ready());

    let view = client.call_snapshot().expect("call should succeed");

    assert_eq!(view.item_count, 3);
    assert_eq!(view.systemd_enabled, 1);
    assert_eq!(view.generation, 42);

    // Verify first item
    let item0 = view.item(0).expect("item 0");
    assert_eq!(item0.hash, 1001);
    assert_eq!(item0.enabled, 1);
    assert_eq!(item0.name.as_bytes(), b"docker-abc123");
    assert_eq!(item0.path.as_bytes(), b"/sys/fs/cgroup/docker/abc123");

    // Verify third item
    let item2 = view.item(2).expect("item 2");
    assert_eq!(item2.hash, 3003);
    assert_eq!(item2.enabled, 0);
    assert_eq!(item2.name.as_bytes(), b"systemd-user");

    // Verify stats
    let status = client.status();
    assert_eq!(status.call_count, 1);
    assert_eq!(status.error_count, 0);

    client.close();
    server.stop();
    cleanup_all(svc);
}

#[cfg(target_os = "linux")]
#[test]
fn test_cgroups_call_shm() {
    let svc = "rs_svc_cgroups_shm";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server =
        TestServer::start_shm(svc, METHOD_CGROUPS_SNAPSHOT, Some(test_cgroups_dispatch()));

    let mut client = snapshot_client(svc, shm_client_config());
    connect_ready(&mut client);
    assert!(client.shm.is_some(), "expected SHM to be negotiated");
    assert_eq!(
        client.session.as_ref().map(|s| s.selected_profile),
        Some(PROFILE_SHM_FUTEX)
    );

    let view = client.call_snapshot().expect("snapshot over SHM");
    assert_eq!(view.item_count, 3);
    assert_eq!(view.generation, 42);
    assert_eq!(view.item(0).expect("item 0").hash, 1001);

    client.close();
    server.stop();
    cleanup_all(svc);
}

#[cfg(target_os = "linux")]
#[test]
fn test_client_call_string_reverse_shm_success() {
    let svc = "rs_svc_strrev_shm";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server = TestServer::start_shm(
        svc,
        METHOD_STRING_REVERSE,
        Some(string_reverse_dispatch_handler()),
    );

    let mut client = string_reverse_client(svc, shm_client_config());
    connect_ready(&mut client);
    assert!(client.shm.is_some(), "expected SHM to be negotiated");

    let result = client
        .call_string_reverse("hello")
        .expect("string reverse over SHM");
    assert_eq!(result.as_str(), "olleh");

    client.close();
    server.stop();
    cleanup_all(svc);
}

#[cfg(target_os = "linux")]
#[test]
fn test_increment_batch_shm() {
    let svc = "rs_pp_batch_shm";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server =
        TestServer::start_shm(svc, METHOD_INCREMENT, Some(increment_dispatch_handler()));

    let mut client = increment_client(svc, shm_client_config());
    connect_ready(&mut client);
    assert!(client.shm.is_some(), "expected SHM to be negotiated");

    let values = vec![10u64, 20, 30, 40];
    let results = client
        .call_increment_batch(&values)
        .expect("batch over SHM");
    assert_eq!(results, vec![11, 21, 31, 41]);

    client.close();
    server.stop();
    cleanup_all(svc);
}

#[cfg(target_os = "linux")]
#[test]
fn test_refresh_shm_attach_failure_falls_back_to_baseline() {
    let svc = "rs_svc_shm_attach_fail";
    ensure_run_dir();
    cleanup_all(svc);

    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();
    let svc_clone = svc.to_string();

    let server_thread = thread::spawn(move || {
        let listener = UdsListener::bind(TEST_RUN_DIR, &svc_clone, shm_server_config())
            .expect("bind shm listener");
        ready_clone.store(true, Ordering::Release);
        let mut first = listener.accept().expect("accept negotiated shm session");
        assert_eq!(first.selected_profile, PROFILE_SHM_FUTEX);

        let mut recv_buf = vec![0u8; RESPONSE_BUF_SIZE];
        let first_result = first.receive(&mut recv_buf);
        assert!(
            first_result.is_err(),
            "first SHM-selected session should disconnect after attach failure"
        );

        let mut second = listener.accept().expect("accept baseline fallback session");
        assert_eq!(second.selected_profile, PROFILE_BASELINE);

        let second_result = second.receive(&mut recv_buf);
        assert!(
            second_result.is_err(),
            "second baseline session should close cleanly when client closes"
        );
    });

    while !ready.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(1));
    }

    let mut client = snapshot_client(svc, shm_client_config());
    assert!(
        client.refresh(),
        "refresh should transition to READY via baseline fallback"
    );
    assert_eq!(client.state, ClientState::Ready);
    assert!(client.ready());
    assert!(
        client.session.is_some(),
        "attach fallback should end with a live baseline session"
    );
    assert!(
        client.shm.is_none(),
        "fallback session must not retain SHM state"
    );
    assert_eq!(
        client.session.as_ref().map(|s| s.selected_profile),
        Some(PROFILE_BASELINE)
    );
    assert_eq!(client.transport_config.supported_profiles, PROFILE_BASELINE);
    assert_eq!(client.transport_config.preferred_profiles, 0);

    client.close();
    server_thread.join().expect("server join");
    cleanup_all(svc);
}

#[cfg(target_os = "linux")]
#[test]
fn test_server_falls_back_to_baseline_when_linux_shm_prepare_fails() {
    let svc = "rs_svc_shm_upgrade_fail";
    ensure_run_dir();
    cleanup_all(svc);

    let shm_path = format!("{TEST_RUN_DIR}/{svc}-{:016x}.ipcshm", 1u64);
    let _ = std::fs::remove_dir_all(&shm_path);
    std::fs::create_dir(&shm_path).expect("create SHM obstruction directory");

    let mut server = TestServer::start_with(
        svc,
        shm_server_config(),
        METHOD_INCREMENT,
        Some(increment_dispatch_handler()),
        8,
    );
    let mut client = increment_client(svc, shm_client_config());

    assert!(client.refresh(), "client should transition to READY");
    assert!(client.ready(), "client should remain usable over baseline");
    assert_eq!(client.state, ClientState::Ready);
    assert_eq!(
        client.session.as_ref().map(|s| s.selected_profile),
        Some(PROFILE_BASELINE)
    );
    assert!(client.shm.is_none(), "fallback session must not attach SHM");

    client.close();
    server.stop();
    let _ = std::fs::remove_dir_all(&shm_path);
    cleanup_all(svc);
}

#[cfg(target_os = "linux")]
#[test]
fn test_call_increment_shm_short_response_truncated() {
    let svc = "rs_svc_shm_short_resp";
    ensure_run_dir();
    cleanup_all(svc);

    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();
    let svc_clone = svc.to_string();

    let server_thread = thread::spawn(move || -> Result<(), String> {
        let listener = UdsListener::bind(TEST_RUN_DIR, &svc_clone, shm_server_config())
            .map_err(|e| format!("bind: {e}"))?;
        ready_clone.store(true, Ordering::Release);
        let session = listener.accept().map_err(|e| format!("accept: {e}"))?;
        if session.selected_profile != PROFILE_SHM_FUTEX {
            return Err(format!("unexpected profile {}", session.selected_profile));
        }

        let mut shm = ShmContext::server_create(
            TEST_RUN_DIR,
            &svc_clone,
            session.session_id,
            session.max_request_payload_bytes + HEADER_SIZE as u32,
            session.max_response_payload_bytes + HEADER_SIZE as u32,
        )
        .map_err(|e| format!("server_create: {e}"))?;

        let mut req_buf = vec![0u8; session.max_request_payload_bytes as usize + HEADER_SIZE];
        let mlen = shm
            .receive(&mut req_buf, 5000)
            .map_err(|e| format!("shm receive: {e}"))?;
        if mlen < HEADER_SIZE {
            return Err(format!("request too short: {mlen}"));
        }

        shm.send(&[0xAB])
            .map_err(|e| format!("shm send short response: {e}"))?;
        Ok(())
    });

    while !ready.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(1));
    }

    let mut client = increment_client(svc, shm_client_config());
    connect_ready(&mut client);
    assert!(client.shm.is_some(), "expected SHM to be negotiated");

    let err = client
        .call_increment(41)
        .expect_err("short SHM response should be truncated");
    assert_eq!(err, NipcError::Truncated);

    client.close();
    match server_thread.join() {
        Ok(Ok(())) => {}
        Ok(Err(err)) => panic!("raw shm server failed: {err}"),
        Err(_) => panic!("raw shm server panicked"),
    }
    cleanup_all(svc);
}

#[cfg(target_os = "linux")]
#[test]
fn test_raw_shm_session_server_wait_panics_on_baseline_profile() {
    let svc = "rs_svc_shm_helper_bad_profile";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server = start_raw_shm_session_server(svc, server_config(), move |_, _, _| Ok(()));

    let mut client = increment_client(svc, client_config());
    connect_ready(&mut client);
    client.close();

    let panic = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| server.wait()))
        .expect_err("baseline profile should panic in raw SHM helper");
    let msg = panic_payload_to_string(panic);
    assert!(
        msg.contains("unexpected profile"),
        "unexpected panic message: {msg}"
    );

    cleanup_all(svc);
}

#[cfg(target_os = "linux")]
#[test]
fn test_raw_shm_session_server_wait_panics_on_short_request() {
    let svc = "rs_svc_shm_helper_short_req";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server = start_raw_shm_session_server(svc, shm_server_config(), move |_, _, _| Ok(()));

    let mut client = increment_client(svc, shm_client_config());
    connect_ready(&mut client);
    assert!(
        client.shm.is_some(),
        "expected SHM transport to be negotiated"
    );

    client
        .shm
        .as_mut()
        .expect("shm")
        .send(&[0xAB])
        .expect("send short SHM request");
    client.close();

    let panic = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| server.wait()))
        .expect_err("short SHM request should panic in raw SHM helper");
    let msg = panic_payload_to_string(panic);
    assert!(
        msg.contains("request too short"),
        "unexpected panic message: {msg}"
    );

    cleanup_all(svc);
}

#[cfg(target_os = "linux")]
#[test]
fn test_call_increment_shm_server_thread_rejects_baseline_profile() {
    let svc = "rs_svc_shm_short_resp_bad_profile";
    ensure_run_dir();
    cleanup_all(svc);

    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();
    let svc_clone = svc.to_string();

    let server_thread = thread::spawn(move || -> Result<(), String> {
        let listener = UdsListener::bind(TEST_RUN_DIR, &svc_clone, shm_server_config())
            .map_err(|e| format!("bind: {e}"))?;
        ready_clone.store(true, Ordering::Release);
        let session = listener.accept().map_err(|e| format!("accept: {e}"))?;
        if session.selected_profile != PROFILE_SHM_FUTEX {
            return Err(format!("unexpected profile {}", session.selected_profile));
        }
        Ok(())
    });

    while !ready.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(1));
    }

    let mut client = increment_client(svc, client_config());
    connect_ready(&mut client);
    client.close();

    match server_thread.join() {
        Ok(Err(err)) => assert!(
            err.contains("unexpected profile"),
            "unexpected error: {err}"
        ),
        Ok(Ok(())) => panic!("baseline profile should not satisfy the SHM-only helper"),
        Err(_) => panic!("raw shm server panicked"),
    }

    cleanup_all(svc);
}

#[cfg(target_os = "linux")]
#[test]
fn test_call_increment_shm_server_thread_rejects_short_request() {
    let svc = "rs_svc_shm_short_resp_short_req";
    ensure_run_dir();
    cleanup_all(svc);

    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();
    let svc_clone = svc.to_string();

    let server_thread = thread::spawn(move || -> Result<(), String> {
        let listener = UdsListener::bind(TEST_RUN_DIR, &svc_clone, shm_server_config())
            .map_err(|e| format!("bind: {e}"))?;
        ready_clone.store(true, Ordering::Release);
        let session = listener.accept().map_err(|e| format!("accept: {e}"))?;
        if session.selected_profile != PROFILE_SHM_FUTEX {
            return Err(format!("unexpected profile {}", session.selected_profile));
        }

        let mut shm = ShmContext::server_create(
            TEST_RUN_DIR,
            &svc_clone,
            session.session_id,
            session.max_request_payload_bytes + HEADER_SIZE as u32,
            session.max_response_payload_bytes + HEADER_SIZE as u32,
        )
        .map_err(|e| format!("server_create: {e}"))?;

        let mut req_buf = vec![0u8; session.max_request_payload_bytes as usize + HEADER_SIZE];
        let mlen = shm
            .receive(&mut req_buf, 5000)
            .map_err(|e| format!("shm receive: {e}"))?;
        if mlen < HEADER_SIZE {
            return Err(format!("request too short: {mlen}"));
        }

        Ok(())
    });

    while !ready.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(1));
    }

    let mut client = increment_client(svc, shm_client_config());
    connect_ready(&mut client);
    assert!(client.shm.is_some(), "expected SHM to be negotiated");
    client
        .shm
        .as_mut()
        .expect("shm")
        .send(&[0xAB])
        .expect("send short SHM request");
    client.close();

    match server_thread.join() {
        Ok(Err(err)) => assert!(err.contains("request too short"), "unexpected error: {err}"),
        Ok(Ok(())) => panic!("short SHM request should not be accepted"),
        Err(_) => panic!("raw shm server panicked"),
    }

    cleanup_all(svc);
}

#[cfg(target_os = "linux")]
#[test]
fn test_call_increment_shm_rejects_bad_message_id() {
    let svc = "rs_svc_shm_inc_bad_mid";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server =
        start_raw_shm_session_server(svc, shm_server_config(), move |shm, req_hdr, _| {
            let mut payload = [0u8; INCREMENT_PAYLOAD_SIZE];
            let n = increment_encode(43, &mut payload);
            if n != INCREMENT_PAYLOAD_SIZE {
                return Err(format!("increment_encode returned {n}"));
            }

            let resp_hdr = Header {
                magic: MAGIC_MSG,
                version: VERSION,
                header_len: protocol::HEADER_LEN,
                kind: KIND_RESPONSE,
                code: METHOD_INCREMENT,
                flags: 0,
                payload_len: n as u32,
                item_count: 1,
                message_id: req_hdr.message_id + 1,
                transport_status: STATUS_OK,
            };
            let msg = encode_raw_message(&resp_hdr, &payload[..n]);
            shm.send(&msg).map_err(|e| format!("shm send: {e}"))
        });

    let mut client = increment_client(svc, shm_client_config());
    connect_ready(&mut client);
    assert!(client.shm.is_some(), "expected SHM to be negotiated");

    let err = client
        .call_increment(42)
        .expect_err("bad SHM response message_id");
    assert_eq!(err, NipcError::BadLayout);

    client.close();
    server.wait();
    cleanup_all(svc);
}

#[cfg(target_os = "linux")]
#[test]
fn test_call_string_reverse_shm_rejects_bad_message_id() {
    let svc = "rs_svc_shm_str_bad_mid";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server =
        start_raw_shm_session_server(svc, shm_server_config(), move |shm, req_hdr, _| {
            let mut payload = [0u8; 128];
            let n = string_reverse_encode(b"olleh", &mut payload);
            if n == 0 {
                return Err("string_reverse_encode returned 0".into());
            }

            let resp_hdr = Header {
                magic: MAGIC_MSG,
                version: VERSION,
                header_len: protocol::HEADER_LEN,
                kind: KIND_RESPONSE,
                code: METHOD_STRING_REVERSE,
                flags: 0,
                payload_len: n as u32,
                item_count: 1,
                message_id: req_hdr.message_id + 1,
                transport_status: STATUS_OK,
            };
            let msg = encode_raw_message(&resp_hdr, &payload[..n]);
            shm.send(&msg).map_err(|e| format!("shm send: {e}"))
        });

    let mut client = string_reverse_client(svc, shm_client_config());
    connect_ready(&mut client);
    assert!(client.shm.is_some(), "expected SHM to be negotiated");

    let err = client
        .call_string_reverse("hello")
        .expect_err("bad SHM response message_id");
    assert_eq!(err, NipcError::BadLayout);

    client.close();
    server.wait();
    cleanup_all(svc);
}

#[cfg(target_os = "linux")]
#[test]
fn test_call_increment_batch_shm_rejects_bad_message_id() {
    let svc = "rs_svc_shm_batch_bad_mid";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server =
        start_raw_shm_session_server(svc, shm_server_config(), move |shm, req_hdr, _| {
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

            let resp_hdr = Header {
                magic: MAGIC_MSG,
                version: VERSION,
                header_len: protocol::HEADER_LEN,
                kind: KIND_RESPONSE,
                code: METHOD_INCREMENT,
                flags: FLAG_BATCH,
                payload_len: resp_len as u32,
                item_count: 2,
                message_id: req_hdr.message_id + 1,
                transport_status: STATUS_OK,
            };
            let msg = encode_raw_message(&resp_hdr, &response_buf[..resp_len]);
            shm.send(&msg).map_err(|e| format!("shm send: {e}"))
        });

    let mut client = increment_client(svc, shm_client_config());
    connect_ready(&mut client);
    assert!(client.shm.is_some(), "expected SHM to be negotiated");

    let err = client
        .call_increment_batch(&[10, 20])
        .expect_err("bad SHM batch response message_id");
    assert_eq!(err, NipcError::BadLayout);

    client.close();
    server.wait();
    cleanup_all(svc);
}

#[test]
fn test_retry_on_failure() {
    let svc = "rs_svc_retry";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server1 =
        TestServer::start(svc, METHOD_CGROUPS_SNAPSHOT, Some(test_cgroups_dispatch()));

    let mut client = snapshot_client(svc, client_config());
    client.refresh();
    assert!(client.ready());

    // First call succeeds
    let view = client.call_snapshot().expect("first call");
    assert_eq!(view.item_count, 3);

    // Kill server
    server1.stop();
    cleanup_all(svc);
    thread::sleep(Duration::from_millis(50));

    // Restart server
    let mut server2 =
        TestServer::start(svc, METHOD_CGROUPS_SNAPSHOT, Some(test_cgroups_dispatch()));

    // Next call triggers reconnect + retry
    let view2 = client.call_snapshot().expect("retry call");
    assert_eq!(view2.item_count, 3);

    // Verify reconnect happened
    let status = client.status();
    assert!(status.reconnect_count >= 1);

    client.close();
    server2.stop();
    cleanup_all(svc);
}

#[test]
fn test_string_reverse_retry_on_failure() {
    let svc = "rs_svc_retry_str";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server1 = TestServer::start(
        svc,
        METHOD_STRING_REVERSE,
        Some(string_reverse_dispatch_handler()),
    );

    let mut client = string_reverse_client(svc, client_config());
    client.refresh();
    assert!(client.ready());
    assert_eq!(
        client
            .call_string_reverse("hello")
            .expect("first reverse")
            .as_str(),
        "olleh"
    );

    server1.stop();
    cleanup_all(svc);
    thread::sleep(Duration::from_millis(50));

    let mut server2 = TestServer::start(
        svc,
        METHOD_STRING_REVERSE,
        Some(string_reverse_dispatch_handler()),
    );

    let result = client.call_string_reverse("hello").expect("retry reverse");
    assert_eq!(result.as_str(), "olleh");

    let status = client.status();
    assert!(status.reconnect_count >= 1);

    client.close();
    server2.stop();
    cleanup_all(svc);
}

#[test]
fn test_increment_batch_retry_on_failure() {
    let svc = "rs_svc_retry_batch";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server1 = TestServer::start_with(
        svc,
        batch_server_config(),
        METHOD_INCREMENT,
        Some(increment_dispatch_handler()),
        8,
    );

    let mut client = increment_client(svc, batch_client_config());
    client.refresh();
    assert!(client.ready());
    assert_eq!(
        client.call_increment_batch(&[10, 20]).expect("first batch"),
        vec![11, 21]
    );

    server1.stop();
    cleanup_all(svc);
    thread::sleep(Duration::from_millis(50));

    let mut server2 = TestServer::start_with(
        svc,
        batch_server_config(),
        METHOD_INCREMENT,
        Some(increment_dispatch_handler()),
        8,
    );

    let result = client.call_increment_batch(&[10, 20]).expect("retry batch");
    assert_eq!(result, vec![11, 21]);

    let status = client.status();
    assert!(status.reconnect_count >= 1);

    client.close();
    server2.stop();
    cleanup_all(svc);
}

#[test]
fn test_string_reverse_retry_second_failure() {
    let svc = "rs_svc_retry_str_second_fail";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server1 = TestServer::start_with(
        svc,
        batch_server_config(),
        METHOD_STRING_REVERSE,
        Some(string_reverse_dispatch_handler()),
        8,
    );

    let mut client = string_reverse_client(svc, client_config());
    client.refresh();
    assert!(client.ready());
    assert_eq!(
        client
            .call_string_reverse("hello")
            .expect("initial reverse")
            .as_str(),
        "olleh"
    );

    server1.stop();
    cleanup_all(svc);
    thread::sleep(Duration::from_millis(50));

    let mut server2 =
        start_raw_session_server(svc, server_config(), move |session, req_hdr, payload| {
            let decoded =
                string_reverse_decode(payload).map_err(|e| format!("decode request: {e:?}"))?;
            if decoded.as_str() != "hello" {
                return Err("unexpected request payload".into());
            }

            let mut resp_hdr = Header {
                kind: KIND_REQUEST,
                code: METHOD_STRING_REVERSE,
                flags: 0,
                item_count: 1,
                message_id: req_hdr.message_id,
                transport_status: STATUS_OK,
                ..Header::default()
            };
            session
                .send(&mut resp_hdr, payload)
                .map_err(|e| format!("send: {e}"))
        });

    let err = client
        .call_string_reverse("hello")
        .expect_err("retry should fail on malformed second response");
    assert_eq!(err, NipcError::BadKind);

    let status = client.status();
    assert_eq!(status.state, ClientState::Broken);
    assert!(status.reconnect_count >= 1);
    assert!(status.error_count >= 1);

    client.close();
    server2.wait();
    cleanup_all(svc);
}

#[test]
fn test_increment_batch_retry_second_failure() {
    let svc = "rs_svc_retry_batch_second_fail";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server1 = TestServer::start_with(
        svc,
        batch_server_config(),
        METHOD_INCREMENT,
        Some(increment_dispatch_handler()),
        8,
    );

    let mut client = increment_client(svc, batch_client_config());
    client.refresh();
    assert!(client.ready());
    assert_eq!(
        client
            .call_increment_batch(&[10, 20])
            .expect("initial batch"),
        vec![11, 21]
    );

    server1.stop();
    cleanup_all(svc);
    thread::sleep(Duration::from_millis(50));

    let mut server2 = start_raw_session_server(
        svc,
        batch_server_config(),
        move |session, req_hdr, payload| {
            let (item0, _) =
                batch_item_get(payload, 2, 0).map_err(|e| format!("decode batch item 0: {e:?}"))?;
            let (item1, _) =
                batch_item_get(payload, 2, 1).map_err(|e| format!("decode batch item 1: {e:?}"))?;
            let v0 = increment_decode(item0).map_err(|e| format!("decode inc0: {e:?}"))?;
            let v1 = increment_decode(item1).map_err(|e| format!("decode inc1: {e:?}"))?;
            if v0 != 10 || v1 != 20 {
                return Err("unexpected batch payload".into());
            }

            let mut resp_hdr = Header {
                kind: KIND_REQUEST,
                code: METHOD_INCREMENT,
                flags: 0,
                item_count: 1,
                message_id: req_hdr.message_id,
                transport_status: STATUS_OK,
                ..Header::default()
            };
            session
                .send(&mut resp_hdr, &[])
                .map_err(|e| format!("send: {e}"))
        },
    );

    let err = client
        .call_increment_batch(&[10, 20])
        .expect_err("retry should fail on malformed second batch response");
    assert_eq!(err, NipcError::BadKind);

    let status = client.status();
    assert_eq!(status.state, ClientState::Broken);
    assert!(status.reconnect_count >= 1);
    assert!(status.error_count >= 1);

    client.close();
    server2.wait();
    cleanup_all(svc);
}

#[test]
fn test_multiple_clients() {
    let svc = "rs_svc_multi";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server = TestServer::start(svc, METHOD_CGROUPS_SNAPSHOT, Some(test_cgroups_dispatch()));

    // Create and connect client 1
    let mut client1 = snapshot_client(svc, client_config());
    client1.refresh();
    assert!(client1.ready());

    let view1 = client1.call_snapshot().expect("client 1 call");
    assert_eq!(view1.item_count, 3);

    // Now multi-client: keep client 1 open, connect client 2
    let mut client2 = snapshot_client(svc, client_config());
    client2.refresh();
    assert!(client2.ready());

    let view2 = client2.call_snapshot().expect("client 2 call");
    assert_eq!(view2.item_count, 3);

    client1.close();
    client2.close();
    server.stop();
    cleanup_all(svc);
}

#[test]
fn test_server_rejects_session_at_worker_capacity() {
    let svc = "rs_svc_worker_capacity";
    ensure_run_dir();
    cleanup_all(svc);

    let handler_calls = Arc::new(std::sync::atomic::AtomicU32::new(0));
    let release = Arc::new(AtomicBool::new(false));

    let handler = {
        let handler_calls = handler_calls.clone();
        let release = release.clone();
        increment_dispatch(Arc::new(move |value| {
            handler_calls.fetch_add(1, Ordering::AcqRel);
            while !release.load(Ordering::Acquire) {
                thread::sleep(Duration::from_millis(1));
            }
            Some(value + 1)
        }))
    };

    let mut server = TestServer::start_with_workers(svc, METHOD_INCREMENT, Some(handler), 1);

    let (call_tx, call_rx) = std::sync::mpsc::channel();
    let svc_name = svc.to_string();
    let caller = thread::spawn(move || {
        let mut client = increment_client(&svc_name, client_config());
        connect_ready(&mut client);
        let result = client.call_increment(41);
        client.close();
        call_tx.send(result).expect("send first call result");
    });

    for _ in 0..500 {
        if handler_calls.load(Ordering::Acquire) >= 1 {
            break;
        }
        thread::sleep(Duration::from_millis(1));
    }
    assert_eq!(
        handler_calls.load(Ordering::Acquire),
        1,
        "first client should occupy the only worker slot"
    );

    let mut session2 =
        UdsSession::connect(TEST_RUN_DIR, svc, &client_config()).expect("second connect");
    let mut req_hdr = Header {
        kind: KIND_REQUEST,
        code: METHOD_INCREMENT,
        item_count: 1,
        message_id: 1,
        ..Header::default()
    };
    let mut req_payload = [0u8; INCREMENT_PAYLOAD_SIZE];
    assert_eq!(
        increment_encode(9, &mut req_payload),
        INCREMENT_PAYLOAD_SIZE,
        "IncrementEncode should fill the request buffer"
    );

    let send_err = session2.send(&mut req_hdr, &req_payload).err();
    let recv_ok = if send_err.is_none() {
        let mut recv_buf = [0u8; HEADER_SIZE + 64];
        session2.receive(&mut recv_buf).is_ok()
    } else {
        false
    };
    assert!(
        !(send_err.is_none() && recv_ok),
        "second session should be rejected while the server is at worker capacity"
    );
    assert_eq!(
        handler_calls.load(Ordering::Acquire),
        1,
        "second session should not enter the handler while the first session is active"
    );
    drop(session2);

    release.store(true, Ordering::Release);
    let first_result = call_rx.recv().expect("first call result");
    assert_eq!(first_result.expect("first call should succeed"), 42);
    caller.join().expect("caller join");

    thread::sleep(Duration::from_millis(100));

    let mut verify = increment_client(svc, client_config());
    connect_ready(&mut verify);
    assert_eq!(
        verify.call_increment(1).expect("verification call"),
        2,
        "server should remain healthy after rejecting a session at capacity"
    );
    verify.close();

    server.stop();
    cleanup_all(svc);
}

#[test]
fn test_poll_fd_invalid_fd_returns_error() {
    let mut fds = [0; 2];
    let rc = unsafe { libc::pipe(fds.as_mut_ptr()) };
    assert_eq!(rc, 0, "pipe failed");

    let read_fd = fds[0];
    let write_fd = fds[1];
    unsafe {
        libc::close(read_fd);
    }

    let rc = poll_fd(read_fd, 0);
    unsafe {
        libc::close(write_fd);
    }

    assert_eq!(rc, -1);
}

#[test]
fn test_poll_fd_readable_returns_ready() {
    let mut fds = [0; 2];
    let rc = unsafe { libc::pipe(fds.as_mut_ptr()) };
    assert_eq!(rc, 0, "pipe failed");

    let read_fd = fds[0];
    let write_fd = fds[1];
    let wrote = unsafe { libc::write(write_fd, b"x".as_ptr() as *const libc::c_void, 1) };
    assert_eq!(wrote, 1, "write failed");

    let rc = poll_fd(read_fd, 0);

    unsafe {
        libc::close(read_fd);
        libc::close(write_fd);
    }

    assert_eq!(rc, 1);
}

#[cfg(target_os = "linux")]
#[test]
fn test_poll_fd_eintr_returns_timeout() {
    unsafe extern "C" fn noop_signal_handler(_: libc::c_int) {}

    let mut fds = [0; 2];
    let rc = unsafe { libc::pipe(fds.as_mut_ptr()) };
    assert_eq!(rc, 0, "pipe failed");

    let read_fd = fds[0];
    let write_fd = fds[1];
    let mut action: libc::sigaction = unsafe { std::mem::zeroed() };
    let mut old_action: libc::sigaction = unsafe { std::mem::zeroed() };
    action.sa_flags = 0;
    action.sa_sigaction = noop_signal_handler as usize;
    unsafe { libc::sigemptyset(&mut action.sa_mask) };
    assert_eq!(
        unsafe { libc::sigaction(libc::SIGUSR1, &action, &mut old_action) },
        0
    );

    let tid = unsafe { libc::pthread_self() };
    let signaler = thread::spawn(move || {
        thread::sleep(Duration::from_millis(50));
        assert_eq!(unsafe { libc::pthread_kill(tid, libc::SIGUSR1) }, 0);
    });

    let rc = poll_fd(read_fd, 5000);
    signaler.join().expect("signaler join");
    unsafe {
        libc::sigaction(libc::SIGUSR1, &old_action, std::ptr::null_mut());
        libc::close(read_fd);
        libc::close(write_fd);
    }

    assert_eq!(rc, 0);
}

#[test]
fn test_managed_server_recovers_after_short_uds_request() {
    let svc = "rs_svc_short_uds_req";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server = TestServer::start(svc, METHOD_INCREMENT, Some(increment_dispatch_handler()));
    let session = UdsSession::connect(TEST_RUN_DIR, svc, &client_config()).expect("connect");
    send_raw_packet(session.fd(), &[0xAB]);
    drop(session);
    thread::sleep(Duration::from_millis(50));

    verify_increment_service_ok(svc, client_config());

    server.stop();
    cleanup_all(svc);
}

#[test]
fn test_managed_server_recovers_after_bad_uds_header() {
    let svc = "rs_svc_bad_uds_hdr";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server = TestServer::start(svc, METHOD_INCREMENT, Some(increment_dispatch_handler()));
    let session = UdsSession::connect(TEST_RUN_DIR, svc, &client_config()).expect("connect");
    let bad_header = [0u8; HEADER_SIZE];
    send_raw_packet(session.fd(), &bad_header);
    drop(session);
    thread::sleep(Duration::from_millis(50));

    verify_increment_service_ok(svc, client_config());

    server.stop();
    cleanup_all(svc);
}

#[test]
fn test_managed_server_recovers_after_uds_peer_closes_before_response() {
    let svc = "rs_svc_uds_send_break";
    ensure_run_dir();
    cleanup_all(svc);

    let handler = increment_dispatch(Arc::new(|value| {
        thread::sleep(Duration::from_millis(50));
        Some(value + 1)
    }));
    let mut server = TestServer::start(svc, METHOD_INCREMENT, Some(handler));
    let session = UdsSession::connect(TEST_RUN_DIR, svc, &client_config()).expect("connect");
    let request = build_increment_request_message(77, 41);
    send_raw_packet(session.fd(), &request);
    assert_eq!(unsafe { libc::shutdown(session.fd(), libc::SHUT_RDWR) }, 0);
    drop(session);
    thread::sleep(Duration::from_millis(100));

    verify_increment_service_ok(svc, client_config());

    server.stop();
    cleanup_all(svc);
}

#[cfg(target_os = "linux")]
#[test]
fn test_managed_server_recovers_after_short_shm_request() {
    let svc = "rs_svc_short_shm_req";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server =
        TestServer::start_shm(svc, METHOD_INCREMENT, Some(increment_dispatch_handler()));
    let mut client = increment_client(svc, shm_client_config());
    connect_ready(&mut client);
    assert!(
        client.shm.is_some(),
        "expected SHM transport to be negotiated"
    );

    client
        .shm
        .as_mut()
        .expect("shm")
        .send(&[0xAB])
        .expect("send short SHM request");
    client.close();
    thread::sleep(Duration::from_millis(50));

    verify_increment_service_ok(svc, shm_client_config());

    server.stop();
    cleanup_all(svc);
}

#[cfg(target_os = "linux")]
#[test]
fn test_managed_server_recovers_after_bad_shm_header() {
    let svc = "rs_svc_bad_shm_hdr";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server =
        TestServer::start_shm(svc, METHOD_INCREMENT, Some(increment_dispatch_handler()));
    let mut client = increment_client(svc, shm_client_config());
    connect_ready(&mut client);
    assert!(
        client.shm.is_some(),
        "expected SHM transport to be negotiated"
    );

    let bad_header = [0u8; HEADER_SIZE];
    client
        .shm
        .as_mut()
        .expect("shm")
        .send(&bad_header)
        .expect("send bad SHM header");
    client.close();
    thread::sleep(Duration::from_millis(50));

    verify_increment_service_ok(svc, shm_client_config());

    server.stop();
    cleanup_all(svc);
}

#[test]
fn test_concurrent_clients() {
    let svc = "rs_svc_concurrent";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server = TestServer::start(svc, METHOD_CGROUPS_SNAPSHOT, Some(test_cgroups_dispatch()));

    const NUM_CLIENTS: usize = 5;
    const REQUESTS_PER: usize = 10;

    let mut handles = Vec::new();

    for _ in 0..NUM_CLIENTS {
        let svc_name = svc.to_string();
        let handle = thread::spawn(move || {
            let mut client = snapshot_client(&svc_name, client_config());

            // Connect with retry
            for _ in 0..100 {
                client.refresh();
                if client.ready() {
                    break;
                }
                thread::sleep(Duration::from_millis(10));
            }

            assert!(client.ready(), "client must be ready");

            let mut successes = 0usize;
            for _ in 0..REQUESTS_PER {
                match client.call_snapshot() {
                    Ok(view) => {
                        assert_eq!(view.item_count, 3);
                        assert_eq!(view.generation, 42);

                        // Verify first item content
                        let item0 = view.item(0).expect("item 0");
                        assert_eq!(item0.hash, 1001);
                        assert_eq!(
                            std::str::from_utf8(item0.name.as_bytes()).unwrap(),
                            "docker-abc123"
                        );

                        successes += 1;
                    }
                    Err(e) => panic!("call failed: {:?}", e),
                }
            }
            client.close();
            successes
        });
        handles.push(handle);
    }

    let mut total = 0usize;
    for h in handles {
        total += h.join().expect("client thread panicked");
    }

    assert_eq!(total, NUM_CLIENTS * REQUESTS_PER);

    server.stop();
    cleanup_all(svc);
}

#[test]
fn test_handler_failure() {
    let svc = "rs_svc_hfail";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server = TestServer::start(svc, METHOD_CGROUPS_SNAPSHOT, None);

    let mut client = snapshot_client(svc, client_config());
    client.refresh();
    assert!(client.ready());

    // Call should fail (handler returns None -> INTERNAL_ERROR)
    let err = client.call_snapshot();
    assert!(err.is_err());

    let status = client.status();
    assert!(status.error_count >= 1);

    client.close();
    server.stop();
    cleanup_all(svc);
}

#[test]
fn test_status_reporting() {
    let svc = "rs_svc_status";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server = TestServer::start(svc, METHOD_CGROUPS_SNAPSHOT, Some(test_cgroups_dispatch()));

    let mut client = snapshot_client(svc, client_config());
    client.refresh();
    assert!(client.ready());

    // Initial counters
    let s0 = client.status();
    assert_eq!(s0.connect_count, 1);
    assert_eq!(s0.call_count, 0);
    assert_eq!(s0.error_count, 0);

    // Make 3 successful calls
    for _ in 0..3 {
        client.call_snapshot().expect("call ok");
    }

    let s1 = client.status();
    assert_eq!(s1.call_count, 3);
    assert_eq!(s1.error_count, 0);

    // Call on disconnected client
    client.close();
    let err = client.call_snapshot();
    assert!(err.is_err());

    let s2 = client.status();
    assert_eq!(s2.error_count, 1);

    server.stop();
    cleanup_all(svc);
}

#[test]
fn test_non_request_terminates_session() {
    // Send a RESPONSE message to a server; the server must terminate
    // the session (protocol violation), so subsequent requests fail.
    let svc = "rs_svc_nonreq";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server = TestServer::start(svc, METHOD_CGROUPS_SNAPSHOT, Some(test_cgroups_dispatch()));

    // Connect via raw UDS session
    let mut session = UdsSession::connect(TEST_RUN_DIR, svc, &client_config()).expect("connect");

    // Send a RESPONSE (not REQUEST) - protocol violation
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
    // Send may succeed (the bytes go out)
    if send_result.is_ok() {
        // But subsequent communication should fail because the
        // server terminated the session
        thread::sleep(Duration::from_millis(100));
        let mut recv_buf = vec![0u8; 4096];
        // Try to send a valid request and receive - should fail
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
            "server should have terminated session after non-request message"
        );
    }

    drop(session);

    // Verify server is still alive: connect a new client and do a normal call
    let mut verify_client = snapshot_client(svc, client_config());
    verify_client.refresh();
    assert!(
        verify_client.ready(),
        "server should still be alive after bad client"
    );

    let view = verify_client
        .call_snapshot()
        .expect("normal call should succeed after bad client");
    assert_eq!(
        view.item_count, 3,
        "response should be correct after bad client"
    );

    verify_client.close();
    server.stop();
    cleanup_all(svc);
}

// ---------------------------------------------------------------
//  L3 Cache tests
// ---------------------------------------------------------------

#[test]
fn test_cache_full_round_trip() {
    let svc = "rs_cache_rt";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server = TestServer::start(svc, METHOD_CGROUPS_SNAPSHOT, Some(test_cgroups_dispatch()));

    let mut cache = CgroupsCache::new(TEST_RUN_DIR, svc, client_config());
    assert!(!cache.ready());

    // Ensure monotonic epoch advances past 0ms before refresh
    thread::sleep(Duration::from_millis(2));

    // Refresh populates the cache
    let updated = cache.refresh();
    assert!(updated);
    assert!(cache.ready());

    // Lookup by hash + name
    let item = cache.lookup(1001, "docker-abc123");
    assert!(item.is_some());
    let item = item.unwrap();
    assert_eq!(item.hash, 1001);
    assert_eq!(item.options, 0);
    assert_eq!(item.enabled, 1);
    assert_eq!(item.name, "docker-abc123");
    assert_eq!(item.path, "/sys/fs/cgroup/docker/abc123");

    let item2 = cache.lookup(3003, "systemd-user");
    assert!(item2.is_some());
    assert_eq!(item2.unwrap().enabled, 0);

    // Status
    let status = cache.status();
    assert!(status.populated);
    assert_eq!(status.item_count, 3);
    assert_eq!(status.systemd_enabled, 1);
    assert_eq!(status.generation, 42);
    assert_eq!(status.refresh_success_count, 1);
    assert_eq!(status.refresh_failure_count, 0);
    assert_eq!(status.connection_state, ClientState::Ready);
    assert!(status.last_refresh_ts > 0);

    cache.close();
    server.stop();
    cleanup_all(svc);
}

#[test]
fn test_cache_refresh_failure_preserves() {
    let svc = "rs_cache_preserve";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server = TestServer::start(svc, METHOD_CGROUPS_SNAPSHOT, Some(test_cgroups_dispatch()));

    let mut cache = CgroupsCache::new(TEST_RUN_DIR, svc, client_config());

    // First refresh populates cache
    assert!(cache.refresh());
    assert!(cache.ready());
    assert!(cache.lookup(1001, "docker-abc123").is_some());

    // Kill server
    server.stop();
    cleanup_all(svc);
    thread::sleep(Duration::from_millis(50));

    // Refresh fails, but old cache is preserved
    let updated = cache.refresh();
    assert!(!updated);
    assert!(cache.ready()); // still has cached data
    assert!(cache.lookup(1001, "docker-abc123").is_some());

    let status = cache.status();
    assert_eq!(status.refresh_success_count, 1);
    assert!(status.refresh_failure_count >= 1);

    cache.close();
    cleanup_all(svc);
}

#[test]
fn test_cache_reconnect_rebuilds() {
    let svc = "rs_cache_reconn";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server1 =
        TestServer::start(svc, METHOD_CGROUPS_SNAPSHOT, Some(test_cgroups_dispatch()));

    let mut cache = CgroupsCache::new(TEST_RUN_DIR, svc, client_config());
    assert!(cache.refresh());
    assert_eq!(cache.status().item_count, 3);

    // Kill and restart server
    server1.stop();
    cleanup_all(svc);
    thread::sleep(Duration::from_millis(50));

    let mut server2 =
        TestServer::start(svc, METHOD_CGROUPS_SNAPSHOT, Some(test_cgroups_dispatch()));

    // Refresh should reconnect and rebuild cache
    let updated = cache.refresh();
    assert!(updated);
    assert!(cache.ready());
    assert_eq!(cache.status().item_count, 3);
    assert_eq!(cache.status().refresh_success_count, 2);

    cache.close();
    server2.stop();
    cleanup_all(svc);
}

#[test]
fn test_cache_lookup_not_found() {
    let svc = "rs_cache_notfound";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server = TestServer::start(svc, METHOD_CGROUPS_SNAPSHOT, Some(test_cgroups_dispatch()));

    let mut cache = CgroupsCache::new(TEST_RUN_DIR, svc, client_config());
    assert!(cache.refresh());

    // Non-existent hash
    assert!(cache.lookup(9999, "nonexistent").is_none());

    // Correct hash, wrong name
    assert!(cache.lookup(1001, "wrong-name").is_none());

    // Correct name, wrong hash
    assert!(cache.lookup(9999, "docker-abc123").is_none());

    cache.close();
    server.stop();
    cleanup_all(svc);
}

#[test]
fn test_cache_empty() {
    let svc = "rs_cache_empty";
    ensure_run_dir();
    cleanup_all(svc);

    let cache = CgroupsCache::new(TEST_RUN_DIR, svc, client_config());

    // Not ready before any refresh
    assert!(!cache.ready());

    // Lookup on empty cache returns None
    assert!(cache.lookup(1001, "docker-abc123").is_none());

    let status = cache.status();
    assert!(!status.populated);
    assert_eq!(status.item_count, 0);
    assert_eq!(status.refresh_success_count, 0);
    assert_eq!(status.refresh_failure_count, 0);

    cleanup_all(svc);
}

#[test]
fn test_cache_large_dataset() {
    let svc = "rs_cache_large";
    ensure_run_dir();
    cleanup_all(svc);

    const N: u32 = 1000;

    // Handler that builds N items
    fn large_snapshot_dispatch() -> DispatchHandler {
        snapshot_dispatch(
            Arc::new(|req, builder| {
                if req.layout_version != 1 || req.flags != 0 {
                    return false;
                }
                builder.set_header(1, 100);

                for i in 0..N {
                    let name = format!("cgroup-{i}");
                    let path = format!("/sys/fs/cgroup/test/{i}");
                    if builder
                        .add(
                            i + 1000,
                            0,
                            if i % 3 == 0 { 0 } else { 1 },
                            name.as_bytes(),
                            path.as_bytes(),
                        )
                        .is_err()
                    {
                        return false;
                    }
                }

                true
            }),
            N,
        )
    }

    // Use a larger response buf size
    let mut cfg = client_config();
    cfg.max_response_payload_bytes = 256 * N;

    let mut server = TestServer::start_with_resp_size(
        svc,
        METHOD_CGROUPS_SNAPSHOT,
        Some(large_snapshot_dispatch()),
        256 * N as usize,
    );

    let mut cache = CgroupsCache::new(TEST_RUN_DIR, svc, cfg);

    assert!(cache.refresh());
    assert_eq!(cache.status().item_count, N);

    // Verify all lookups
    for i in 0..N {
        let name = format!("cgroup-{i}");
        let item = cache.lookup(i + 1000, &name);
        assert!(item.is_some(), "item {i} not found");
        let item = item.unwrap();
        assert_eq!(item.hash, i + 1000);
        let expected_path = format!("/sys/fs/cgroup/test/{i}");
        assert_eq!(item.path, expected_path);
    }

    cache.close();
    server.stop();
    cleanup_all(svc);
}

#[test]
fn test_cache_refresh_lossy_utf8() {
    let svc = "rs_cache_lossy";
    ensure_run_dir();
    cleanup_all(svc);

    let handler = snapshot_dispatch(
        Arc::new(|_, builder| {
            builder.set_header(1, 7);
            builder
                .add(1001, 0, 1, b"bad-\xFF-name", b"/bad/\xFF/path")
                .is_ok()
        }),
        1,
    );

    let mut server = TestServer::start(svc, METHOD_CGROUPS_SNAPSHOT, Some(handler));
    let mut cache = CgroupsCache::new(TEST_RUN_DIR, svc, client_config());

    assert!(cache.refresh(), "cache refresh should succeed");
    let item = cache
        .lookup(1001, "bad-\u{FFFD}-name")
        .expect("lossy lookup");
    assert_eq!(item.name, "bad-\u{FFFD}-name");
    assert_eq!(item.path, "/bad/\u{FFFD}/path");

    cache.close();
    server.stop();
    cleanup_all(svc);
}

#[test]
fn test_cache_refresh_preserves_old_cache_on_malformed_snapshot_item() {
    let svc = "rs_cache_preserve_bad_item";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server = TestServer::start(svc, METHOD_CGROUPS_SNAPSHOT, Some(test_cgroups_dispatch()));
    let mut cache = CgroupsCache::new(TEST_RUN_DIR, svc, client_config());

    assert!(cache.refresh(), "initial refresh should succeed");
    let old_status = cache.status();
    let old_item = cache
        .lookup(1001, "docker-abc123")
        .expect("existing cache item")
        .clone();

    server.stop();
    cleanup_all(svc);

    let mut raw_server =
        start_raw_session_server(svc, server_config(), move |session, req_hdr, payload| {
            let req = crate::protocol::CgroupsRequest::decode(payload)
                .map_err(|e| format!("decode request: {e:?}"))?;
            if req.layout_version != 1 || req.flags != 0 {
                return Err("unexpected snapshot request".into());
            }

            let mut response_payload = [0u8; 512];
            let response_len = {
                let mut builder = CgroupsBuilder::new(&mut response_payload, 1, 1, 99);
                builder
                    .add(9999, 0, 1, b"new-item", b"/new/path")
                    .map_err(|e| format!("builder add: {e:?}"))?;
                builder.finish()
            };

            let dir_end = 24 + 8;
            let item_off =
                u32::from_ne_bytes(response_payload[24..28].try_into().unwrap()) as usize;
            let item_start = dir_end + item_off;
            response_payload[item_start..item_start + 2].copy_from_slice(&99u16.to_ne_bytes());

            let mut resp_hdr = Header {
                kind: KIND_RESPONSE,
                code: METHOD_CGROUPS_SNAPSHOT,
                flags: 0,
                item_count: 1,
                message_id: req_hdr.message_id,
                transport_status: STATUS_OK,
                ..Header::default()
            };
            session
                .send(&mut resp_hdr, &response_payload[..response_len])
                .map_err(|e| format!("send: {e}"))
        });

    assert!(
        !cache.refresh(),
        "malformed snapshot item should preserve the old cache"
    );

    let status = cache.status();
    assert!(cache.ready(), "cache should stay populated");
    assert_eq!(status.item_count, old_status.item_count);
    assert_eq!(status.generation, old_status.generation);
    assert_eq!(
        status.refresh_success_count,
        old_status.refresh_success_count
    );
    assert_eq!(
        status.refresh_failure_count,
        old_status.refresh_failure_count + 1
    );
    let preserved = cache
        .lookup(1001, "docker-abc123")
        .expect("old cache item should remain");
    assert_eq!(preserved.hash, old_item.hash);
    assert_eq!(preserved.path, old_item.path);
    assert!(
        cache.lookup(9999, "new-item").is_none(),
        "bad refresh must not replace the old cache"
    );

    cache.close();
    raw_server.wait();
    cleanup_all(svc);
}

// ---------------------------------------------------------------
//  Stress tests (Phase H4)
// ---------------------------------------------------------------

/// djb2 hash matching the C implementation
fn simple_hash(s: &str) -> u32 {
    let mut hash: u32 = 5381;
    for c in s.bytes() {
        hash = hash
            .wrapping_shl(5)
            .wrapping_add(hash)
            .wrapping_add(c as u32);
    }
    hash
}

struct StressTestServer {
    stop_flag: Arc<AtomicBool>,
    thread: Option<thread::JoinHandle<()>>,
}

impl StressTestServer {
    fn start(service: &str, n: u32, resp_buf_size: usize) -> Self {
        ensure_run_dir();
        cleanup_all(service);

        let svc = service.to_string();
        let ready_flag = Arc::new(AtomicBool::new(false));
        let ready_clone = ready_flag.clone();

        let mut scfg = server_config();
        scfg.max_response_payload_bytes = resp_buf_size as u32;
        scfg.packet_size = 65536; // force smaller packets for chunked transport

        let handler = snapshot_dispatch(
            Arc::new(move |req, builder| {
                if req.layout_version != 1 || req.flags != 0 {
                    return false;
                }
                builder.set_header(1, 42);

                for i in 0..n {
                    let name = format!("container-{i:04}");
                    let path = format!("/sys/fs/cgroup/docker/{i:04}");
                    let hash = simple_hash(&name);
                    let enabled = if i % 5 == 0 { 0 } else { 1 };
                    if builder
                        .add(hash, 0x10, enabled, name.as_bytes(), path.as_bytes())
                        .is_err()
                    {
                        return false;
                    }
                }

                true
            }),
            n,
        );

        let mut server = ManagedServer::new(
            TEST_RUN_DIR,
            &svc,
            scfg,
            METHOD_CGROUPS_SNAPSHOT,
            Some(handler),
        );
        let stop_flag = server.running_flag();

        let thread = thread::spawn(move || {
            ready_clone.store(true, Ordering::Release);
            let _ = server.run();
        });

        for _ in 0..2000 {
            if ready_flag.load(Ordering::Acquire) {
                break;
            }
            thread::sleep(Duration::from_micros(500));
        }
        thread::sleep(Duration::from_millis(50));

        StressTestServer {
            stop_flag,
            thread: Some(thread),
        }
    }

    fn stop(&mut self) {
        self.stop_flag.store(false, Ordering::Release);
        if let Some(t) = self.thread.take() {
            let _ = t.join();
        }
    }
}

impl Drop for StressTestServer {
    fn drop(&mut self) {
        self.stop();
    }
}

#[test]
fn test_stress_1000_items() {
    let svc = "rs_stress_1k";

    const N: u32 = 1000;
    const BUF_SIZE: usize = 300 * N as usize;

    let mut server = StressTestServer::start(svc, N, BUF_SIZE);

    let mut cfg = client_config();
    cfg.max_response_payload_bytes = BUF_SIZE as u32;
    cfg.packet_size = 65536;

    let mut client = snapshot_client(svc, cfg);
    client.refresh();
    assert!(client.ready(), "client not ready");

    let start = std::time::Instant::now();
    let view = client.call_snapshot().expect("call should succeed");
    let elapsed = start.elapsed();

    eprintln!("  1000 items: {:?}", elapsed);

    assert_eq!(view.item_count, N);
    assert_eq!(view.systemd_enabled, 1);
    assert_eq!(view.generation, 42);

    // Verify ALL items
    for i in 0..N {
        let item = view
            .item(i)
            .unwrap_or_else(|_| panic!("item {i} decode failed"));
        let expected_name = format!("container-{i:04}");
        let expected_path = format!("/sys/fs/cgroup/docker/{i:04}");
        let expected_hash = simple_hash(&expected_name);
        let expected_enabled = if i % 5 == 0 { 0 } else { 1 };

        assert_eq!(item.hash, expected_hash, "item {i} hash mismatch");
        assert_eq!(
            std::str::from_utf8(item.name.as_bytes()).unwrap(),
            expected_name,
            "item {i} name mismatch"
        );
        assert_eq!(
            std::str::from_utf8(item.path.as_bytes()).unwrap(),
            expected_path,
            "item {i} path mismatch"
        );
        assert_eq!(item.enabled, expected_enabled, "item {i} enabled mismatch");
        assert_eq!(item.options, 0x10, "item {i} options mismatch");
    }

    client.close();
    server.stop();
    cleanup_all(svc);
}

#[test]
fn test_stress_5000_items() {
    let svc = "rs_stress_5k";

    const N: u32 = 5000;
    const BUF_SIZE: usize = 300 * N as usize;

    let mut server = StressTestServer::start(svc, N, BUF_SIZE);

    let mut cfg = client_config();
    cfg.max_response_payload_bytes = BUF_SIZE as u32;
    cfg.packet_size = 65536;

    let mut client = snapshot_client(svc, cfg);
    client.refresh();
    assert!(client.ready(), "client not ready");

    let start = std::time::Instant::now();
    let view = client.call_snapshot().expect("call should succeed");
    let elapsed = start.elapsed();

    eprintln!("  5000 items: {:?}", elapsed);

    assert_eq!(view.item_count, N);

    // Spot-check first, middle, last
    for idx in [0, N / 2, N - 1] {
        let item = view.item(idx).unwrap();
        let expected_name = format!("container-{idx:04}");
        let expected_hash = simple_hash(&expected_name);
        assert_eq!(item.hash, expected_hash);
        assert_eq!(
            std::str::from_utf8(item.name.as_bytes()).unwrap(),
            expected_name
        );
    }

    client.close();
    server.stop();
    cleanup_all(svc);
}

#[test]
fn test_stress_concurrent_clients() {
    let svc = "rs_stress_concurrent";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server = TestServer::start_with_workers(
        svc,
        METHOD_CGROUPS_SNAPSHOT,
        Some(test_cgroups_dispatch()),
        64,
    );

    const NUM_CLIENTS: usize = 50;
    const REQUESTS_PER: usize = 10;

    let start = std::time::Instant::now();

    let mut handles = Vec::new();
    for client_id in 0..NUM_CLIENTS {
        let svc_name = svc.to_string();
        let handle = thread::spawn(move || {
            let mut client = snapshot_client(&svc_name, client_config());

            for _ in 0..200 {
                client.refresh();
                if client.ready() {
                    break;
                }
                thread::sleep(Duration::from_millis(5));
            }

            assert!(client.ready(), "client {client_id} not ready");

            let mut successes = 0usize;
            for _ in 0..REQUESTS_PER {
                match client.call_snapshot() {
                    Ok(view) => {
                        assert_eq!(view.item_count, 3);
                        assert_eq!(view.generation, 42);
                        let item0 = view.item(0).expect("item 0");
                        assert_eq!(item0.hash, 1001);
                        assert_eq!(
                            std::str::from_utf8(item0.name.as_bytes()).unwrap(),
                            "docker-abc123"
                        );
                        let item2 = view.item(2).expect("item 2");
                        assert_eq!(item2.hash, 3003);
                        successes += 1;
                    }
                    Err(e) => panic!("client {client_id} call failed: {:?}", e),
                }
            }
            client.close();
            successes
        });
        handles.push(handle);
    }

    let mut total = 0usize;
    for h in handles {
        total += h.join().expect("client thread panicked");
    }

    let elapsed = start.elapsed();
    eprintln!(
        "  {NUM_CLIENTS} clients x {REQUESTS_PER} req: {total}/{} in {:?}",
        NUM_CLIENTS * REQUESTS_PER,
        elapsed
    );

    assert_eq!(total, NUM_CLIENTS * REQUESTS_PER);

    server.stop();
    cleanup_all(svc);
}

#[test]
fn test_stress_rapid_connect_disconnect() {
    let svc = "rs_stress_rapid";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server = TestServer::start(svc, METHOD_CGROUPS_SNAPSHOT, Some(test_cgroups_dispatch()));

    const CYCLES: usize = 1000;
    let mut successes = 0usize;
    let mut failures = 0usize;

    let start = std::time::Instant::now();

    for _ in 0..CYCLES {
        let mut client = snapshot_client(svc, client_config());

        let mut connected = false;
        /* Under full ctest -j load, a freshly spawned server may need a
         * slightly longer connect window than the hot-path unit tests. */
        for _ in 0..200 {
            client.refresh();
            if client.ready() {
                connected = true;
                break;
            }
            thread::sleep(Duration::from_millis(2));
        }

        if !connected {
            failures += 1;
            client.close();
            continue;
        }

        match client.call_snapshot() {
            Ok(view) => {
                if view.item_count == 3 && view.generation == 42 {
                    successes += 1;
                } else {
                    failures += 1;
                }
            }
            Err(_) => failures += 1,
        }

        client.close();
    }

    let elapsed = start.elapsed();
    eprintln!(
        "  {CYCLES} rapid cycles: {successes} ok, {failures} fail, {:?}",
        elapsed
    );

    assert_eq!(successes, CYCLES, "all cycles should succeed");
    assert_eq!(failures, 0, "no failures expected");

    server.stop();
    cleanup_all(svc);
}

#[test]
fn test_stress_cache_concurrent() {
    let svc = "rs_stress_cache";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server = TestServer::start_with_workers(
        svc,
        METHOD_CGROUPS_SNAPSHOT,
        Some(test_cgroups_dispatch()),
        16,
    );

    const NUM_CLIENTS: usize = 10;
    const REQUESTS_PER: usize = 100;

    let start = std::time::Instant::now();

    let mut handles = Vec::new();
    for _ in 0..NUM_CLIENTS {
        let svc_name = svc.to_string();
        let handle = thread::spawn(move || {
            let mut cache = CgroupsCache::new(TEST_RUN_DIR, &svc_name, client_config());
            let mut successes = 0usize;

            for _ in 0..REQUESTS_PER {
                let updated = cache.refresh();
                if updated || cache.ready() {
                    let status = cache.status();
                    if status.item_count != 3 {
                        continue;
                    }
                    let item = cache.lookup(1001, "docker-abc123");
                    if item.is_some() && item.unwrap().hash == 1001 {
                        successes += 1;
                    }
                }
            }
            cache.close();
            successes
        });
        handles.push(handle);
    }

    let mut total = 0usize;
    for h in handles {
        total += h.join().expect("cache thread panicked");
    }

    let elapsed = start.elapsed();
    eprintln!(
        "  {NUM_CLIENTS} cache clients x {REQUESTS_PER} req: {total}/{} in {:?}",
        NUM_CLIENTS * REQUESTS_PER,
        elapsed
    );

    assert_eq!(total, NUM_CLIENTS * REQUESTS_PER);

    server.stop();
    cleanup_all(svc);
}

#[test]
fn test_stress_long_running() {
    let svc = "rs_stress_long";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server = TestServer::start(svc, METHOD_CGROUPS_SNAPSHOT, Some(test_cgroups_dispatch()));

    const NUM_CLIENTS: usize = 5;
    let run_duration = Duration::from_secs(30);

    let running = Arc::new(AtomicBool::new(true));
    let mut handles = Vec::new();

    for _ in 0..NUM_CLIENTS {
        let svc_name = svc.to_string();
        let r = running.clone();
        let handle = thread::spawn(move || {
            let mut cache = CgroupsCache::new(TEST_RUN_DIR, &svc_name, client_config());
            let mut refreshes = 0u64;
            let mut errors = 0u64;

            while r.load(Ordering::Acquire) {
                let updated = cache.refresh();
                if updated || cache.ready() {
                    let status = cache.status();
                    if status.item_count == 3 {
                        refreshes += 1;
                    } else {
                        errors += 1;
                    }
                } else {
                    errors += 1;
                }
                thread::sleep(Duration::from_millis(1));
            }

            cache.close();
            (refreshes, errors)
        });
        handles.push(handle);
    }

    thread::sleep(run_duration);
    running.store(false, Ordering::Release);

    let mut total_refreshes = 0u64;
    let mut total_errors = 0u64;
    for h in handles {
        let (r, e) = h.join().expect("client thread panicked");
        total_refreshes += r;
        total_errors += e;
    }

    eprintln!("  30s run: {total_refreshes} refreshes, {total_errors} errors");

    assert!(total_refreshes > 0, "expected some refreshes");
    assert_eq!(total_errors, 0, "expected zero errors in 60s run");

    server.stop();
    cleanup_all(svc);
}

// ---------------------------------------------------------------
//  Ping-pong tests per service kind
// ---------------------------------------------------------------

#[test]
fn test_increment_ping_pong() {
    let svc = "rs_pp_incr";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server = TestServer::start(svc, METHOD_INCREMENT, Some(increment_dispatch_handler()));

    let mut client = increment_client(svc, client_config());
    client.refresh();
    assert!(client.ready(), "client not ready");

    // Ping-pong: send 0 -> get 1 -> send 1 -> get 2 -> ... -> 10
    let mut value = 0u64;
    let mut responses_received = 0u64;
    for round in 0..10 {
        let sent = value;
        let result = client
            .call_increment(sent)
            .unwrap_or_else(|e| panic!("round {round}: call_increment({sent}) failed: {e:?}"));
        assert_eq!(
            result,
            sent + 1,
            "round {round}: expected {} got {result}",
            sent + 1
        );
        responses_received += 1;
        value = result;
    }
    assert_eq!(
        responses_received, 10,
        "expected 10 responses, got {responses_received}"
    );
    assert_eq!(value, 10, "final value after 10 rounds");

    client.close();
    server.stop();
    cleanup_all(svc);
}

#[test]
fn test_string_reverse_ping_pong() {
    let svc = "rs_pp_strrev";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server = TestServer::start(
        svc,
        METHOD_STRING_REVERSE,
        Some(string_reverse_dispatch_handler()),
    );

    let mut client = string_reverse_client(svc, client_config());
    client.refresh();
    assert!(client.ready(), "client not ready");

    let original = "abcdefghijklmnopqrstuvwxyz";
    let mut current = original.to_string();
    let mut responses_received = 0u64;

    // 6 rounds: feed each response back as next request
    for round in 0..6 {
        let sent = current.clone();
        let expected: String = sent.chars().rev().collect();
        let result = client.call_string_reverse(&sent).unwrap_or_else(|e| {
            panic!("round {round}: call_string_reverse({sent:?}) failed: {e:?}")
        });
        assert_eq!(
            result.as_str(),
            expected,
            "round {round}: reverse of {sent:?} should be {expected:?}, got {result:?}"
        );
        responses_received += 1;
        current = result.as_str().to_string();
    }
    assert_eq!(
        responses_received, 6,
        "expected 6 responses, got {responses_received}"
    );
    // even number of reversals = identity
    assert_eq!(
        current, original,
        "6 reversals should restore original string"
    );

    client.close();
    server.stop();
    cleanup_all(svc);
}

#[test]
fn test_increment_batch() {
    let svc = "rs_pp_batch";
    ensure_run_dir();
    cleanup_all(svc);

    // Need batch items > 1 for both client and server configs
    fn batch_server_config() -> ServerConfig {
        ServerConfig {
            supported_profiles: PROFILE_BASELINE,
            max_request_payload_bytes: 4096,
            max_request_batch_items: 16,
            max_response_payload_bytes: 4096,
            max_response_batch_items: 16,
            auth_token: AUTH_TOKEN,
            backlog: 4,
            ..ServerConfig::default()
        }
    }

    fn batch_client_config() -> ClientConfig {
        ClientConfig {
            supported_profiles: PROFILE_BASELINE,
            max_request_payload_bytes: 4096,
            max_request_batch_items: 16,
            max_response_payload_bytes: 4096,
            max_response_batch_items: 16,
            auth_token: AUTH_TOKEN,
            ..ClientConfig::default()
        }
    }

    // Start server with batch-capable config
    let svc_name = svc.to_string();
    let ready_flag = Arc::new(AtomicBool::new(false));
    let ready_clone = ready_flag.clone();

    let mut server_obj = ManagedServer::with_workers(
        TEST_RUN_DIR,
        &svc_name,
        batch_server_config(),
        METHOD_INCREMENT,
        Some(increment_dispatch_handler()),
        8,
    );
    let stop_flag = server_obj.running_flag();

    let thread_handle = thread::spawn(move || {
        ready_clone.store(true, Ordering::Release);
        let _ = server_obj.run();
    });

    for _ in 0..2000 {
        if ready_flag.load(Ordering::Acquire) {
            break;
        }
        thread::sleep(Duration::from_micros(500));
    }
    thread::sleep(Duration::from_millis(50));

    let mut client = increment_client(svc, batch_client_config());
    client.refresh();
    assert!(client.ready(), "client not ready");

    // Send batch of [10, 20, 30, 40, 50]
    let values = vec![10u64, 20, 30, 40, 50];
    let results = client.call_increment_batch(&values).expect("batch call");

    assert_eq!(results.len(), 5);
    for (i, (&input, &output)) in values.iter().zip(results.iter()).enumerate() {
        assert_eq!(
            output,
            input + 1,
            "batch item {i}: expected {}, got {output}",
            input + 1
        );
    }

    // Single item batch
    let single = client
        .call_increment_batch(&[99])
        .expect("single-item batch");
    assert_eq!(single, vec![100]);

    // Empty batch
    let empty = client.call_increment_batch(&[]).expect("empty batch");
    assert!(empty.is_empty());

    client.close();
    stop_flag.store(false, Ordering::Release);
    let _ = thread_handle.join();
    cleanup_all(svc);
}

// ---------------------------------------------------------------
//  Client state machine: auth failure (lines 438-440)
// ---------------------------------------------------------------

#[test]
fn test_client_auth_failure() {
    let svc = "rs_svc_authfail";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server = TestServer::start(svc, METHOD_CGROUPS_SNAPSHOT, Some(test_cgroups_dispatch()));

    // Client with wrong auth token
    let mut bad_cfg = client_config();
    bad_cfg.auth_token = 0xBAD_BAD_BAD;

    let mut client = snapshot_client(svc, bad_cfg);
    client.refresh();
    assert_eq!(client.state, ClientState::AuthFailed);
    assert!(!client.ready());

    // Subsequent refresh stays stuck in AuthFailed
    client.refresh();
    assert_eq!(client.state, ClientState::AuthFailed);

    client.close();
    server.stop();
    cleanup_all(svc);
}

// ---------------------------------------------------------------
//  Client state machine: incompatible (lines 439-440)
// ---------------------------------------------------------------

#[test]
fn test_client_incompatible() {
    let svc = "rs_svc_incompat";
    ensure_run_dir();
    cleanup_all(svc);

    // Server supports only PROFILE_BASELINE, but start it first
    let mut server = TestServer::start(svc, METHOD_CGROUPS_SNAPSHOT, Some(test_cgroups_dispatch()));

    // Client requires SHM_FUTEX only (no baseline)
    let mut bad_cfg = client_config();
    #[cfg(target_os = "linux")]
    {
        bad_cfg.supported_profiles = crate::protocol::PROFILE_SHM_FUTEX;
    }
    #[cfg(not(target_os = "linux"))]
    {
        // On non-Linux, use a profile bit that won't match
        bad_cfg.supported_profiles = 0x80000000;
    }

    let mut client = snapshot_client(svc, bad_cfg);
    client.refresh();
    assert_eq!(client.state, ClientState::Incompatible);
    assert!(!client.ready());

    // Stays stuck
    client.refresh();
    assert_eq!(client.state, ClientState::Incompatible);

    client.close();
    server.stop();
    cleanup_all(svc);
}

#[test]
fn test_client_protocol_version_incompatible() {
    let svc = unique_service("rs_svc_proto_incompat");
    ensure_run_dir();
    cleanup_all(&svc);

    let packet =
        hello_ack_packet_with_version(crate::protocol::VERSION + 1, crate::protocol::STATUS_OK, 1);
    let mut server = start_raw_hello_ack_server(&svc, packet);

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

// ---------------------------------------------------------------
//  Client: call_snapshot when not ready (line 324-326)
// ---------------------------------------------------------------

#[test]
fn test_call_when_not_ready() {
    let svc = "rs_svc_noready";
    ensure_run_dir();
    cleanup_all(svc);

    let mut snapshot = snapshot_client(svc, client_config());
    assert_eq!(snapshot.state, ClientState::Disconnected);
    assert!(snapshot.call_snapshot().is_err());
    assert_eq!(snapshot.status().error_count, 1);
    snapshot.close();

    let mut increment = increment_client(svc, client_config());
    assert_eq!(increment.state, ClientState::Disconnected);
    assert!(increment.call_increment(42).is_err());
    assert!(increment.call_increment_batch(&[1, 2]).is_err());
    assert_eq!(increment.status().error_count, 2);
    increment.close();

    let mut string_reverse = string_reverse_client(svc, client_config());
    assert_eq!(string_reverse.state, ClientState::Disconnected);
    assert!(string_reverse.call_string_reverse("test").is_err());
    assert_eq!(string_reverse.status().error_count, 1);
    string_reverse.close();

    cleanup_all(svc);
}

#[test]
fn test_client_invalid_service_name_maps_to_disconnected() {
    let bad_service = "x".repeat(400);
    let mut client = snapshot_client(&bad_service, client_config());

    client.refresh();

    assert_eq!(client.state, ClientState::Disconnected);
    assert!(!client.ready());
}

// ---------------------------------------------------------------
//  Client: broken -> reconnect cycle (lines 136-141)
// ---------------------------------------------------------------

#[test]
fn test_broken_reconnect() {
    let svc = "rs_svc_broken";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server = TestServer::start(svc, METHOD_CGROUPS_SNAPSHOT, Some(test_cgroups_dispatch()));

    let mut client = snapshot_client(svc, client_config());
    client.refresh();
    assert_eq!(client.state, ClientState::Ready);

    // Force broken state
    client.state = ClientState::Broken;

    // refresh from Broken should disconnect, reconnect
    let changed = client.refresh();
    assert!(changed);
    assert_eq!(client.state, ClientState::Ready);
    assert!(client.status().reconnect_count >= 1);

    client.close();
    server.stop();
    cleanup_all(svc);
}

// ---------------------------------------------------------------
//  Cache: close resets everything (line 1561-1566)
// ---------------------------------------------------------------

#[test]
fn test_cache_close_resets() {
    let svc = "rs_cache_close";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server = TestServer::start(svc, METHOD_CGROUPS_SNAPSHOT, Some(test_cgroups_dispatch()));

    let mut cache = CgroupsCache::new(TEST_RUN_DIR, svc, client_config());
    assert!(cache.refresh());
    assert!(cache.ready());
    assert_eq!(cache.status().item_count, 3);

    cache.close();
    assert!(!cache.ready());
    assert!(cache.lookup(1001, "docker-abc123").is_none());
    assert_eq!(cache.status().item_count, 0);

    server.stop();
    cleanup_all(svc);
}

// ---------------------------------------------------------------
//  Cache: with max_response_payload_bytes = 0 (line 1437-1440)
// ---------------------------------------------------------------

#[test]
fn test_cache_default_buf_size() {
    let svc = "rs_cache_defbuf";
    ensure_run_dir();
    cleanup_all(svc);

    // Config with max_response_payload_bytes = 0 triggers default buf
    let mut cfg = client_config();
    cfg.max_response_payload_bytes = 0;

    let cache = CgroupsCache::new(TEST_RUN_DIR, svc, cfg);
    assert_eq!(
        cache.client.max_receive_message_bytes(),
        HEADER_SIZE + CACHE_RESPONSE_BUF_SIZE
    );

    cleanup_all(svc);
}

// ---------------------------------------------------------------
//  ManagedServer: worker_count = 0 -> clamped to 1 (line 778)
// ---------------------------------------------------------------

#[test]
fn test_server_worker_count_clamped() {
    let svc = "rs_svc_w0";
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

    cleanup_all(svc);
}

// ---------------------------------------------------------------
//  ManagedServer: stop flag (line 946)
// ---------------------------------------------------------------

#[test]
fn test_server_stop_flag() {
    let svc = "rs_svc_stopflag";
    ensure_run_dir();
    cleanup_all(svc);

    let server = ManagedServer::new(TEST_RUN_DIR, svc, server_config(), METHOD_INCREMENT, None);
    let flag = server.running_flag();
    assert!(!flag.load(Ordering::Acquire));

    // stop sets running to false
    server.stop();
    assert!(!flag.load(Ordering::Acquire));

    cleanup_all(svc);
}

// ---------------------------------------------------------------
//  ClientStatus / CgroupsCacheStatus fields
// ---------------------------------------------------------------

#[test]
fn test_client_status_fields() {
    let svc = "rs_svc_csf";
    ensure_run_dir();
    cleanup_all(svc);

    let client = snapshot_client(svc, client_config());
    let status = client.status();
    assert_eq!(status.state, ClientState::Disconnected);
    assert_eq!(status.connect_count, 0);
    assert_eq!(status.reconnect_count, 0);
    assert_eq!(status.call_count, 0);
    assert_eq!(status.error_count, 0);
    cleanup_all(svc);
}

// ---------------------------------------------------------------
//  call_increment and call_string_reverse success paths
// ---------------------------------------------------------------

#[test]
fn test_client_call_increment_success() {
    let svc = "rs_svc_incr_ok";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server = TestServer::start(svc, METHOD_INCREMENT, Some(increment_dispatch_handler()));

    let mut client = increment_client(svc, client_config());
    client.refresh();
    assert!(client.ready());

    let result = client.call_increment(99).expect("increment");
    assert_eq!(result, 100);

    client.close();
    server.stop();
    cleanup_all(svc);
}

#[test]
fn test_client_call_string_reverse_success() {
    let svc = "rs_svc_strrev_ok";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server = TestServer::start(
        svc,
        METHOD_STRING_REVERSE,
        Some(string_reverse_dispatch_handler()),
    );

    let mut client = string_reverse_client(svc, client_config());
    client.refresh();
    assert!(client.ready());

    let result = client.call_string_reverse("hello").expect("reverse");
    assert_eq!(result.as_str(), "olleh");

    client.close();
    server.stop();
    cleanup_all(svc);
}

#[test]
fn test_dispatch_single_helper_paths() {
    let mut response_buf = [0u8; 128];

    assert!(matches!(
        dispatch_single(
            METHOD_INCREMENT,
            None,
            METHOD_INCREMENT,
            &[0; 8],
            &mut response_buf,
        ),
        Err(_)
    ));

    assert!(matches!(
        dispatch_single(
            METHOD_STRING_REVERSE,
            None,
            METHOD_STRING_REVERSE,
            &[0; 8],
            &mut response_buf,
        ),
        Err(_)
    ));

    assert!(matches!(
        dispatch_single(
            METHOD_CGROUPS_SNAPSHOT,
            None,
            METHOD_CGROUPS_SNAPSHOT,
            &[1, 0, 0, 0],
            &mut response_buf,
        ),
        Err(_)
    ));

    let snapshot_handler = snapshot_dispatch(Arc::new(|_, _| true), 0);
    assert!(matches!(
        dispatch_single(
            METHOD_CGROUPS_SNAPSHOT,
            Some(&snapshot_handler),
            METHOD_CGROUPS_SNAPSHOT,
            &[1, 0, 0, 0],
            &mut [],
        ),
        Err(_)
    ));

    let reverse_handler = string_reverse_dispatch_handler();
    let mut invalid_utf8_req = [0u8; 16];
    let invalid_len = string_reverse_encode(&[0xff], &mut invalid_utf8_req);
    let n = dispatch_single(
        METHOD_STRING_REVERSE,
        Some(&reverse_handler),
        METHOD_STRING_REVERSE,
        &invalid_utf8_req[..invalid_len],
        &mut response_buf,
    )
    .expect("non-UTF8 string_reverse input should decode as an empty string");
    let view =
        string_reverse_decode(&response_buf[..n]).expect("decode empty-string reverse response");
    assert_eq!(view.as_str(), "");

    assert!(matches!(
        dispatch_single(
            METHOD_STRING_REVERSE,
            Some(&reverse_handler),
            METHOD_INCREMENT,
            &invalid_utf8_req[..invalid_len],
            &mut response_buf,
        ),
        Err(_)
    ));

    let snapshot_fail_handler = snapshot_dispatch(Arc::new(|_, _| false), 1);
    assert!(matches!(
        dispatch_single(
            METHOD_CGROUPS_SNAPSHOT,
            Some(&snapshot_fail_handler),
            METHOD_CGROUPS_SNAPSHOT,
            &[1, 0, 0, 0],
            &mut response_buf,
        ),
        Err(_)
    ));

    assert!(matches!(
        dispatch_single(0xFFFF, None, 0xFFFF, &[], &mut response_buf),
        Err(_)
    ));

    assert!(
        snapshot_max_items(4096, 0) > 0,
        "default snapshot item estimate should be positive for a non-empty buffer"
    );
}

#[test]
fn test_response_payload_transport_buf_bounds() {
    let mut client = snapshot_client("rs_payload_bounds", client_config());
    client.transport_buf.resize(HEADER_SIZE + 8, 0);

    let response = ClientResponseRef {
        source: ClientResponseSource::TransportBuf,
        len: 16,
    };

    assert_eq!(client.response_payload(response), Err(NipcError::Truncated));
}

#[test]
fn test_call_increment_rejects_malformed_response_envelope_unix() {
    struct Case {
        name: &'static str,
        kind: u16,
        code: u16,
        status: u16,
        message_id_delta: u64,
        want: NipcError,
    }

    let cases = [
        Case {
            name: "bad kind",
            kind: KIND_REQUEST,
            code: METHOD_INCREMENT,
            status: STATUS_OK,
            message_id_delta: 0,
            want: NipcError::BadKind,
        },
        Case {
            name: "bad code",
            kind: KIND_RESPONSE,
            code: METHOD_STRING_REVERSE,
            status: STATUS_OK,
            message_id_delta: 0,
            want: NipcError::BadLayout,
        },
        Case {
            name: "bad status",
            kind: KIND_RESPONSE,
            code: METHOD_INCREMENT,
            status: STATUS_INTERNAL_ERROR,
            message_id_delta: 0,
            want: NipcError::BadLayout,
        },
        Case {
            name: "bad message id",
            kind: KIND_RESPONSE,
            code: METHOD_INCREMENT,
            status: STATUS_OK,
            message_id_delta: 1,
            want: NipcError::Truncated,
        },
    ];

    for tc in cases {
        let svc = format!("rs_unix_inc_env_{}", tc.name.replace(' ', "_"));
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
                    message_id: req_hdr.message_id + tc.message_id_delta,
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
        cleanup_all(&svc);
    }
}

#[test]
fn test_call_string_reverse_rejects_malformed_response_envelope_unix() {
    struct Case {
        name: &'static str,
        kind: u16,
        code: u16,
        status: u16,
        message_id_delta: u64,
        want: NipcError,
    }

    let cases = [
        Case {
            name: "bad kind",
            kind: KIND_REQUEST,
            code: METHOD_STRING_REVERSE,
            status: STATUS_OK,
            message_id_delta: 0,
            want: NipcError::BadKind,
        },
        Case {
            name: "bad code",
            kind: KIND_RESPONSE,
            code: METHOD_INCREMENT,
            status: STATUS_OK,
            message_id_delta: 0,
            want: NipcError::BadLayout,
        },
        Case {
            name: "bad status",
            kind: KIND_RESPONSE,
            code: METHOD_STRING_REVERSE,
            status: STATUS_INTERNAL_ERROR,
            message_id_delta: 0,
            want: NipcError::BadLayout,
        },
        Case {
            name: "bad message id",
            kind: KIND_RESPONSE,
            code: METHOD_STRING_REVERSE,
            status: STATUS_OK,
            message_id_delta: 1,
            want: NipcError::Truncated,
        },
    ];

    for tc in cases {
        let svc = format!("rs_unix_str_env_{}", tc.name.replace(' ', "_"));
        let mut server =
            start_raw_session_server(&svc, server_config(), move |session, req_hdr, _| {
                let mut payload = [0u8; 128];
                let n = string_reverse_encode(b"olleh", &mut payload);
                if n == 0 {
                    return Err("string_reverse_encode returned 0".into());
                }

                let mut resp_hdr = Header {
                    kind: tc.kind,
                    code: tc.code,
                    flags: 0,
                    item_count: 1,
                    message_id: req_hdr.message_id + tc.message_id_delta,
                    transport_status: tc.status,
                    ..Header::default()
                };
                session
                    .send(&mut resp_hdr, &payload[..n])
                    .map_err(|e| format!("send: {e}"))
            });

        let mut client = string_reverse_client(&svc, client_config());
        connect_ready(&mut client);

        let err = client.call_string_reverse("hello").expect_err(tc.name);
        assert_eq!(err, tc.want, "{}", tc.name);

        client.close();
        server.wait();
        cleanup_all(&svc);
    }
}

#[test]
fn test_call_increment_batch_rejects_wrong_item_count_unix() {
    let svc = "rs_unix_batch_count";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server =
        start_raw_session_server(svc, batch_server_config(), move |session, req_hdr, _| {
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

    let mut client = increment_client(svc, batch_client_config());
    connect_ready(&mut client);

    let err = client
        .call_increment_batch(&[10, 20])
        .expect_err("wrong batch item_count");
    assert_eq!(err, NipcError::BadItemCount);

    client.close();
    server.wait();
    cleanup_all(svc);
}

#[test]
fn test_call_increment_batch_rejects_malformed_response_envelope_unix() {
    struct Case {
        name: &'static str,
        kind: u16,
        code: u16,
        status: u16,
        message_id_delta: u64,
        want: NipcError,
    }

    let cases = [
        Case {
            name: "bad kind",
            kind: KIND_REQUEST,
            code: METHOD_INCREMENT,
            status: STATUS_OK,
            message_id_delta: 0,
            want: NipcError::BadKind,
        },
        Case {
            name: "bad code",
            kind: KIND_RESPONSE,
            code: METHOD_STRING_REVERSE,
            status: STATUS_OK,
            message_id_delta: 0,
            want: NipcError::BadLayout,
        },
        Case {
            name: "bad status",
            kind: KIND_RESPONSE,
            code: METHOD_INCREMENT,
            status: STATUS_INTERNAL_ERROR,
            message_id_delta: 0,
            want: NipcError::BadLayout,
        },
        Case {
            name: "bad message id",
            kind: KIND_RESPONSE,
            code: METHOD_INCREMENT,
            status: STATUS_OK,
            message_id_delta: 1,
            want: NipcError::Truncated,
        },
    ];

    for tc in cases {
        let svc = format!("rs_unix_batch_env_{}", tc.name.replace(' ', "_"));
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
                    kind: tc.kind,
                    code: tc.code,
                    flags: FLAG_BATCH,
                    item_count: 2,
                    message_id: req_hdr.message_id + tc.message_id_delta,
                    transport_status: tc.status,
                    ..Header::default()
                };
                session
                    .send(&mut resp_hdr, &response_buf[..resp_len])
                    .map_err(|e| format!("send: {e}"))
            });

        let mut client = increment_client(&svc, batch_client_config());
        connect_ready(&mut client);

        let err = client.call_increment_batch(&[10, 20]).expect_err(tc.name);
        assert_eq!(err, tc.want, "{}", tc.name);

        client.close();
        server.wait();
        cleanup_all(&svc);
    }
}

#[test]
fn test_call_string_reverse_chunked_response_unix() {
    let svc = "rs_unix_chunked_reverse";
    ensure_run_dir();
    cleanup_all(svc);

    let long_input = "abcdefghi".repeat(16);
    let expected: String = long_input.chars().rev().collect();
    let scfg = ServerConfig {
        packet_size: 64,
        max_response_payload_bytes: 4096,
        ..server_config()
    };
    let ccfg = ClientConfig {
        packet_size: 64,
        max_response_payload_bytes: 4096,
        ..client_config()
    };

    let mut server = TestServer::start_with(
        svc,
        scfg,
        METHOD_STRING_REVERSE,
        Some(string_reverse_dispatch_handler()),
        8,
    );
    let mut client = string_reverse_client(svc, ccfg);
    connect_ready(&mut client);

    let result = client
        .call_string_reverse(&long_input)
        .expect("chunked reverse");
    assert_eq!(result.as_str(), expected);

    client.close();
    server.stop();
    cleanup_all(svc);
}

// ---------------------------------------------------------------
//  Batch dispatch: handler failure returns INTERNAL_ERROR (lines 1244-1248)
// ---------------------------------------------------------------

#[test]
fn test_batch_dispatch_handler_failure() {
    let svc = "rs_svc_batchfail";
    ensure_run_dir();
    cleanup_all(svc);

    // Handler that fails on the 2nd item
    fn fail_second_increment_handler() -> IncrementHandler {
        static CALL_COUNT: std::sync::atomic::AtomicU32 = std::sync::atomic::AtomicU32::new(0);
        Arc::new(move |value| {
            let n = CALL_COUNT.fetch_add(1, std::sync::atomic::Ordering::Relaxed);
            if n % 3 == 1 {
                return None;
            }
            Some(value + 1)
        })
    }

    fn batch_server_config() -> ServerConfig {
        ServerConfig {
            supported_profiles: PROFILE_BASELINE,
            max_request_payload_bytes: 4096,
            max_request_batch_items: 16,
            max_response_payload_bytes: 4096,
            max_response_batch_items: 16,
            auth_token: AUTH_TOKEN,
            backlog: 4,
            ..ServerConfig::default()
        }
    }

    fn batch_client_config() -> ClientConfig {
        ClientConfig {
            supported_profiles: PROFILE_BASELINE,
            max_request_payload_bytes: 4096,
            max_request_batch_items: 16,
            max_response_payload_bytes: 4096,
            max_response_batch_items: 16,
            auth_token: AUTH_TOKEN,
            ..ClientConfig::default()
        }
    }

    let svc_name = svc.to_string();
    let ready_flag = Arc::new(AtomicBool::new(false));
    let ready_clone = ready_flag.clone();

    let mut server_obj = ManagedServer::with_workers(
        TEST_RUN_DIR,
        &svc_name,
        batch_server_config(),
        METHOD_INCREMENT,
        Some(increment_dispatch(fail_second_increment_handler())),
        8,
    );
    let stop_flag = server_obj.running_flag();

    let thread_handle = thread::spawn(move || {
        ready_clone.store(true, Ordering::Release);
        let _ = server_obj.run();
    });

    for _ in 0..2000 {
        if ready_flag.load(Ordering::Acquire) {
            break;
        }
        thread::sleep(Duration::from_micros(500));
    }
    thread::sleep(Duration::from_millis(50));

    let mut client = increment_client(svc, batch_client_config());
    client.refresh();
    assert!(client.ready());

    // Batch of 3: handler fails on the 2nd -> server returns INTERNAL_ERROR
    let values = vec![10u64, 20, 30];
    let result = client.call_increment_batch(&values);
    // The batch should fail because the handler returned None for item 2
    assert!(result.is_err());

    client.close();
    stop_flag.store(false, Ordering::Release);
    let _ = thread_handle.join();
    cleanup_all(svc);
}

#[test]
fn test_batch_dispatch_builder_overflow_retries_and_recovers() {
    let svc = "rs_svc_batch_overflow";
    ensure_run_dir();
    cleanup_all(svc);

    let mut scfg = batch_server_config();
    scfg.max_response_payload_bytes = 8;

    let mut server = TestServer::start_with(
        svc,
        scfg,
        METHOD_INCREMENT,
        Some(increment_dispatch_handler()),
        8,
    );
    let mut client = increment_client(svc, batch_client_config());
    connect_ready(&mut client);

    let values = client
        .call_increment_batch(&[10, 20])
        .expect("batch builder overflow should transparently reconnect and retry");
    assert_eq!(values, vec![11, 21]);
    assert!(
        client.ready(),
        "client should stay READY after overflow recovery"
    );
    assert!(
        client.status().reconnect_count >= 1,
        "overflow recovery should reconnect at least once"
    );

    client.close();
    server.stop();
    cleanup_all(svc);
}

#[cfg(target_os = "linux")]
#[test]
fn test_shm_batch_request_item_decode_failure_returns_bad_envelope() {
    let svc = "rs_svc_shm_batch_bad_item";
    ensure_run_dir();
    cleanup_all(svc);

    let mut server = TestServer::start_with(
        svc,
        shm_server_config(),
        METHOD_INCREMENT,
        Some(increment_dispatch_handler()),
        8,
    );
    let mut client = increment_client(svc, shm_client_config());
    connect_ready(&mut client);
    assert!(
        client.shm.is_some(),
        "expected SHM transport to be negotiated"
    );

    let mut bad_payload = [0u8; 16];
    bad_payload[0..4].copy_from_slice(&0u32.to_ne_bytes());
    bad_payload[4..8].copy_from_slice(&32u32.to_ne_bytes());
    bad_payload[8..12].copy_from_slice(&0u32.to_ne_bytes());
    bad_payload[12..16].copy_from_slice(&4u32.to_ne_bytes());

    let req_hdr = Header {
        magic: MAGIC_MSG,
        version: VERSION,
        header_len: protocol::HEADER_LEN,
        kind: KIND_REQUEST,
        code: METHOD_INCREMENT,
        flags: FLAG_BATCH,
        payload_len: bad_payload.len() as u32,
        item_count: 2,
        message_id: 7,
        transport_status: STATUS_OK,
    };
    let mut msg = [0u8; HEADER_SIZE + 16];
    req_hdr.encode(&mut msg[..HEADER_SIZE]);
    msg[HEADER_SIZE..].copy_from_slice(&bad_payload);
    client
        .shm
        .as_mut()
        .expect("shm")
        .send(&msg)
        .expect("send malformed batch request");

    let (resp_hdr, response) = client.transport_receive().expect("receive response");
    assert_eq!(resp_hdr.kind, KIND_RESPONSE);
    assert_eq!(resp_hdr.code, METHOD_INCREMENT);
    assert_eq!(resp_hdr.transport_status, STATUS_BAD_ENVELOPE);
    assert_eq!(resp_hdr.flags, 0);
    assert_eq!(resp_hdr.item_count, 1);
    assert!(
        client
            .response_payload(response)
            .expect("response payload view")
            .is_empty(),
        "bad-envelope response should have no payload"
    );

    client.close();
    server.stop();
    cleanup_all(svc);
}
