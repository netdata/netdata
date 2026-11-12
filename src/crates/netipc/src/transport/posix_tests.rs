use super::*;
use crate::protocol::{
    self, ChunkHeader, FLAG_BATCH, KIND_REQUEST, KIND_RESPONSE, MAGIC_CHUNK, MAGIC_MSG, VERSION,
};
use crate::transport::framing::{HELLO_ACK_PAYLOAD_SIZE, HELLO_PAYLOAD_SIZE};
use std::sync::Arc;
use std::sync::atomic::{AtomicBool, AtomicU32, Ordering};
use std::thread;
use std::time::Duration;

const TEST_RUN_DIR: &str = "/tmp/nipc_rust_test";
const AUTH_TOKEN: u64 = 0xDEADBEEFCAFEBABE;

fn ensure_run_dir() {
    let _ = std::fs::create_dir_all(TEST_RUN_DIR);
}

fn cleanup_socket(service: &str) {
    let path = format!("{TEST_RUN_DIR}/{service}.sock");
    let _ = std::fs::remove_file(&path);
}

fn default_server_config() -> ServerConfig {
    ServerConfig {
        supported_profiles: PROFILE_BASELINE,
        preferred_profiles: 0,
        max_request_payload_bytes: 4096,
        max_request_batch_items: 16,
        max_response_payload_bytes: 4096,
        max_response_batch_items: 16,
        auth_token: AUTH_TOKEN,
        packet_size: 0,
        backlog: 4,
    }
}

fn default_client_config() -> ClientConfig {
    ClientConfig {
        supported_profiles: PROFILE_BASELINE,
        preferred_profiles: 0,
        max_request_payload_bytes: 4096,
        max_request_batch_items: 16,
        max_response_payload_bytes: 4096,
        max_response_batch_items: 16,
        auth_token: AUTH_TOKEN,
        packet_size: 0,
    }
}

/// On a stream a short or over-long packet is only visible once the peer
/// stops sending: end the peer's writes there so the receiver sees EOF where
/// a SEQPACKET receiver sees the packet end.
fn end_stream_writes(fd: RawFd) {
    #[cfg(any(target_os = "macos", target_os = "freebsd"))]
    unsafe {
        libc::shutdown(fd, libc::SHUT_WR)
    };
    #[cfg(not(any(target_os = "macos", target_os = "freebsd")))]
    let _ = fd;
}

/// Unique service name to avoid parallel test collisions.
static TEST_COUNTER: AtomicU32 = AtomicU32::new(0);
fn unique_service(prefix: &str) -> String {
    let n = TEST_COUNTER.fetch_add(1, Ordering::Relaxed);
    let service = format!("{prefix}_{n}_{}", std::process::id());
    crate::test_locks::track(TEST_RUN_DIR, &service);
    service
}

fn socketpair_seqpacket() -> (RawFd, RawFd) {
    let mut fds = [-1; 2];
    let rc = unsafe { libc::socketpair(libc::AF_UNIX, SOCK_TYPE, 0, fds.as_mut_ptr()) };
    assert_eq!(rc, 0, "socketpair failed: {}", errno());
    (fds[0], fds[1])
}

fn test_session(fd: RawFd, role: Role, packet_size: u32) -> UdsSession {
    UdsSession {
        fd,
        role,
        max_request_payload_bytes: 4096,
        max_request_batch_items: 16,
        max_response_payload_bytes: 4096,
        max_response_batch_items: 16,
        packet_size,
        selected_profile: PROFILE_BASELINE,
        session_id: 1,
        recv_buf: Vec::new(),
        pkt_buf: Vec::new(),
        inflight_ids: HashSet::new(),
        pending: Pending::default(),
        recv_failed: false,
    }
}

/// Read one HELLO from a raw server socket. A stream socket may return it
/// in parts, so read until the whole packet is in.
fn recv_hello(fd: RawFd, buf: &mut [u8]) -> usize {
    let want = HEADER_SIZE + HELLO_PAYLOAD_SIZE;
    let mut got = 0;
    while got < want {
        got += raw_recv(fd, &mut buf[got..want]).expect("recv hello");
    }
    got
}

fn raw_listener_fd(service: &str) -> RawFd {
    let path = build_socket_path(TEST_RUN_DIR, service).expect("socket path");
    let fd = unsafe { libc::socket(libc::AF_UNIX, SOCK_TYPE, 0) };
    assert!(fd >= 0, "socket failed: {}", errno());
    bind_unix(fd, &path).expect("bind raw listener");
    let rc = unsafe { libc::listen(fd, DEFAULT_BACKLOG) };
    assert_eq!(rc, 0, "listen failed: {}", errno());
    fd
}

/// A signal that interrupts a socket call before data moved must not fail
/// the handshake or the session: each client send, sendmsg and recv below
/// fails once with EINTR first.
#[test]
fn test_eintr_retried() {
    ensure_run_dir();
    let svc = unique_service("rs_eintr");
    cleanup_socket(&svc);

    let listener = UdsListener::bind(TEST_RUN_DIR, &svc, default_server_config()).expect("listen");
    let server_thread = thread::spawn(move || {
        // The server's first accept is interrupted before a client arrives.
        eintr_inject::ACCEPT.with(|c| c.set(1));
        let mut session = listener.accept().expect("accept survives EINTR");
        assert_eq!(eintr_inject::pending(), 0, "the accept EINTR was hit");
        let mut buf = [0u8; 8192];
        let (hdr, payload) = session.receive(&mut buf).expect("recv");
        let payload = payload.to_vec();
        let mut resp = hdr;
        resp.kind = protocol::KIND_RESPONSE;
        resp.transport_status = protocol::STATUS_OK;
        session.send(&mut resp, &payload).expect("send");
    });

    // HELLO goes through send (sendmsg on a stream), the request
    // through sendmsg; HELLO_ACK and the response through recv. Injection is
    // per thread: only this client's calls fail.
    #[cfg(not(any(target_os = "macos", target_os = "freebsd")))]
    {
        eintr_inject::SEND.with(|c| c.set(1));
        eintr_inject::SENDMSG.with(|c| c.set(1));
    }
    #[cfg(any(target_os = "macos", target_os = "freebsd"))]
    eintr_inject::SENDMSG.with(|c| c.set(2));
    eintr_inject::RECV.with(|c| c.set(2));

    let mut session = UdsSession::connect(TEST_RUN_DIR, &svc, &default_client_config())
        .expect("connect survives EINTR");
    let payload = [0x05u8, 0x06];
    let mut hdr = Header {
        kind: protocol::KIND_REQUEST,
        code: protocol::METHOD_INCREMENT,
        item_count: 1,
        message_id: 7,
        ..Header::default()
    };
    session
        .send(&mut hdr, &payload)
        .expect("send survives EINTR");
    let mut rbuf = [0u8; 4096];
    let (rhdr, rpayload) = session.receive(&mut rbuf).expect("receive survives EINTR");
    assert_eq!(rhdr.message_id, 7);
    assert_eq!(rpayload, payload);
    assert_eq!(eintr_inject::pending(), 0, "every injected EINTR was hit");

    drop(session);
    server_thread.join().unwrap();
    cleanup_socket(&svc);
}

// -----------------------------------------------------------------------
//  Test 1: Single client ping-pong
// -----------------------------------------------------------------------

#[test]
fn test_ping_pong() {
    ensure_run_dir();
    let svc = unique_service("rs_ping");
    cleanup_socket(&svc);

    let svc_clone = svc.clone();

    let listener =
        UdsListener::bind(TEST_RUN_DIR, &svc_clone, default_server_config()).expect("listen");
    let server_thread = thread::spawn(move || {
        let mut session = listener.accept().expect("accept");

        let mut buf = [0u8; 8192];
        let (hdr, payload) = session.receive(&mut buf).expect("recv");
        let payload = payload.to_vec();

        // Echo back as response
        let mut resp = hdr;
        resp.kind = protocol::KIND_RESPONSE;
        resp.transport_status = protocol::STATUS_OK;
        session.send(&mut resp, &payload).expect("send");
    });

    // Wait for server

    let mut session =
        UdsSession::connect(TEST_RUN_DIR, &svc, &default_client_config()).expect("connect");

    assert_eq!(session.selected_profile, PROFILE_BASELINE);

    let payload = [0x01u8, 0x02, 0x03, 0x04];
    let mut hdr = Header {
        kind: protocol::KIND_REQUEST,
        code: protocol::METHOD_INCREMENT,
        flags: 0,
        item_count: 1,
        message_id: 42,
        ..Header::default()
    };

    session.send(&mut hdr, &payload).expect("send");

    let mut rbuf = [0u8; 4096];
    let (rhdr, rpayload) = session.receive(&mut rbuf).expect("recv");
    assert_eq!(rhdr.kind, protocol::KIND_RESPONSE);
    assert_eq!(rhdr.message_id, 42);
    assert_eq!(rpayload, payload);

    drop(session);
    server_thread.join().expect("server join");
    cleanup_socket(&svc);
}

// -----------------------------------------------------------------------
//  Test 2: Multi-client (2 clients, 1 listener)
// -----------------------------------------------------------------------

#[test]
fn test_multi_client() {
    ensure_run_dir();
    let svc = unique_service("rs_multi");
    cleanup_socket(&svc);

    let svc_clone = svc.clone();

    let listener =
        UdsListener::bind(TEST_RUN_DIR, &svc_clone, default_server_config()).expect("listen");
    let server_thread = thread::spawn(move || {
        for _ in 0..2 {
            let mut session = listener.accept().expect("accept");
            let mut buf = [0u8; 8192];
            let (hdr, payload) = session.receive(&mut buf).expect("recv");
            let payload = payload.to_vec();
            let mut resp = hdr;
            resp.kind = protocol::KIND_RESPONSE;
            resp.transport_status = protocol::STATUS_OK;
            session.send(&mut resp, &payload).expect("send");
        }
    });

    let results: Vec<_> = (0..2)
        .map(|i| {
            let svc = svc.clone();
            thread::spawn(move || {
                let mut session = UdsSession::connect(TEST_RUN_DIR, &svc, &default_client_config())
                    .expect("connect");

                let payload = [0xAA + i as u8];
                let msg_id = 100 + i as u64;
                let mut hdr = Header {
                    kind: protocol::KIND_REQUEST,
                    code: 1,
                    item_count: 1,
                    message_id: msg_id,
                    ..Header::default()
                };
                session.send(&mut hdr, &payload).expect("send");

                let mut rbuf = [0u8; 4096];
                let (rhdr, rpayload) = session.receive(&mut rbuf).expect("recv");
                assert_eq!(rhdr.message_id, msg_id);
                assert_eq!(rpayload, payload);
            })
        })
        .collect();

    for t in results {
        t.join().expect("client join");
    }

    server_thread.join().expect("server join");
    cleanup_socket(&svc);
}

// -----------------------------------------------------------------------
//  Test 3: Pipelining (send N, receive N)
// -----------------------------------------------------------------------

#[test]
fn test_pipelining() {
    ensure_run_dir();
    let svc = unique_service("rs_pipe");
    cleanup_socket(&svc);

    let svc_clone = svc.clone();

    let listener =
        UdsListener::bind(TEST_RUN_DIR, &svc_clone, default_server_config()).expect("listen");
    let server_thread = thread::spawn(move || {
        let mut session = listener.accept().expect("accept");

        for _ in 0..3 {
            let mut buf = [0u8; 8192];
            let (hdr, payload) = session.receive(&mut buf).expect("recv");
            let payload = payload.to_vec();
            let mut resp = hdr;
            resp.kind = protocol::KIND_RESPONSE;
            resp.transport_status = protocol::STATUS_OK;
            session.send(&mut resp, &payload).expect("send");
        }
    });

    let mut session =
        UdsSession::connect(TEST_RUN_DIR, &svc, &default_client_config()).expect("connect");

    // Send 3 requests
    for i in 1u64..=3 {
        let payload = [i as u8];
        let mut hdr = Header {
            kind: protocol::KIND_REQUEST,
            code: 1,
            item_count: 1,
            message_id: i,
            ..Header::default()
        };
        session.send(&mut hdr, &payload).expect("send");
    }

    // Receive 3 responses
    for i in 1u64..=3 {
        let mut rbuf = [0u8; 4096];
        let (rhdr, rpayload) = session.receive(&mut rbuf).expect("recv");
        assert_eq!(rhdr.message_id, i);
        assert_eq!(rpayload, [i as u8]);
    }

    drop(session);
    server_thread.join().expect("server join");
    cleanup_socket(&svc);
}

// -----------------------------------------------------------------------
//  Test 4: Chunking (large message with small packet_size)
// -----------------------------------------------------------------------

#[test]
fn test_chunking() {
    ensure_run_dir();
    let svc = unique_service("rs_chunk");
    cleanup_socket(&svc);

    let svc_clone = svc.clone();
    let scfg = ServerConfig {
        packet_size: 128,
        max_request_payload_bytes: 65536,
        max_response_payload_bytes: 65536,
        ..default_server_config()
    };
    let listener = UdsListener::bind(TEST_RUN_DIR, &svc_clone, scfg).expect("listen");
    let server_thread = thread::spawn(move || {
        let mut session = listener.accept().expect("accept");

        let mut buf = [0u8; 256]; // small buf, forces recv_buf usage
        let (hdr, payload) = session.receive(&mut buf).expect("recv");
        let payload = payload.to_vec();

        let mut resp = hdr;
        resp.kind = protocol::KIND_RESPONSE;
        session.send(&mut resp, &payload).expect("send");
    });

    let ccfg = ClientConfig {
        packet_size: 128,
        max_request_payload_bytes: 65536,
        max_response_payload_bytes: 65536,
        ..default_client_config()
    };
    let mut session = UdsSession::connect(TEST_RUN_DIR, &svc, &ccfg).expect("connect");

    assert_eq!(session.packet_size, 128);

    // Build a payload larger than 128 - 32 = 96 bytes
    let big_len = 500;
    let big: Vec<u8> = (0..big_len).map(|i| (i & 0xFF) as u8).collect();

    let mut hdr = Header {
        kind: protocol::KIND_REQUEST,
        code: 1,
        item_count: 1,
        message_id: 7,
        ..Header::default()
    };

    session.send(&mut hdr, &big).expect("send chunked");

    let mut rbuf = [0u8; 256];
    let (rhdr, rpayload) = session.receive(&mut rbuf).expect("recv chunked");
    assert_eq!(rhdr.message_id, 7);
    assert_eq!(rpayload.len(), big_len);
    assert_eq!(rpayload, big);

    drop(session);
    server_thread.join().expect("server join");
    cleanup_socket(&svc);
}

// -----------------------------------------------------------------------
//  Test 5: Handshake failure - bad auth
// -----------------------------------------------------------------------

#[test]
fn test_bad_auth() {
    ensure_run_dir();
    let svc = unique_service("rs_badauth");
    cleanup_socket(&svc);

    let svc_clone = svc.clone();

    let listener =
        UdsListener::bind(TEST_RUN_DIR, &svc_clone, default_server_config()).expect("listen");
    let server_thread = thread::spawn(move || {
        // accept will fail due to auth mismatch
        let _ = listener.accept();
    });

    let ccfg = ClientConfig {
        auth_token: 0xBAD,
        ..default_client_config()
    };

    let result = UdsSession::connect(TEST_RUN_DIR, &svc, &ccfg);
    assert!(matches!(result, Err(UdsError::AuthFailed)));

    server_thread.join().expect("server join");
    cleanup_socket(&svc);
}

// -----------------------------------------------------------------------
//  Test 6: Handshake failure - profile mismatch
// -----------------------------------------------------------------------

#[test]
fn test_profile_mismatch() {
    ensure_run_dir();
    let svc = unique_service("rs_badprof");
    cleanup_socket(&svc);

    let svc_clone = svc.clone();
    let scfg = ServerConfig {
        supported_profiles: protocol::PROFILE_SHM_FUTEX,
        ..default_server_config()
    };
    let listener = UdsListener::bind(TEST_RUN_DIR, &svc_clone, scfg).expect("listen");
    let server_thread = thread::spawn(move || {
        let _ = listener.accept();
    });

    let ccfg = ClientConfig {
        supported_profiles: PROFILE_BASELINE,
        ..default_client_config()
    };

    let result = UdsSession::connect(TEST_RUN_DIR, &svc, &ccfg);
    assert!(matches!(result, Err(UdsError::NoProfile)));

    server_thread.join().expect("server join");
    cleanup_socket(&svc);
}

#[test]
fn test_request_payload_over_cap() {
    ensure_run_dir();
    let svc = unique_service("rs_reqcap");
    cleanup_socket(&svc);

    let svc_clone = svc.clone();

    let listener =
        UdsListener::bind(TEST_RUN_DIR, &svc_clone, default_server_config()).expect("listen");
    let server_thread = thread::spawn(move || {
        let result = listener.accept();
        assert!(matches!(result, Err(UdsError::LimitExceeded)));
    });

    let ccfg = ClientConfig {
        max_request_payload_bytes: protocol::MAX_PAYLOAD_CAP + 1,
        ..default_client_config()
    };
    let result = UdsSession::connect(TEST_RUN_DIR, &svc, &ccfg);
    assert!(matches!(result, Err(UdsError::LimitExceeded)));

    server_thread.join().expect("server join");
    cleanup_socket(&svc);
}

// -----------------------------------------------------------------------
//  Test 7: Stale socket recovery
// -----------------------------------------------------------------------

/// Create a socket file that is bound but not listening — what a crashed
/// server leaves behind.
fn create_stale_socket_file(path: &str) {
    unsafe {
        let sock = libc::socket(libc::AF_UNIX, SOCK_TYPE, 0);
        assert!(sock >= 0);

        let c_path = CString::new(path).unwrap();
        let mut addr: libc::sockaddr_un = std::mem::zeroed();
        addr.sun_family = libc::AF_UNIX as libc::sa_family_t;
        let path_bytes = c_path.as_bytes_with_nul();
        let sun_path_ptr = addr.sun_path.as_mut_ptr() as *mut u8;
        std::ptr::copy_nonoverlapping(path_bytes.as_ptr(), sun_path_ptr, path_bytes.len());

        libc::bind(
            sock,
            &addr as *const libc::sockaddr_un as *const libc::sockaddr,
            std::mem::size_of::<libc::sockaddr_un>() as libc::socklen_t,
        );
        // Close without unlink => stale
        libc::close(sock);
    }
}

#[test]
fn test_stale_recovery() {
    ensure_run_dir();
    let svc = unique_service("rs_stale");
    cleanup_socket(&svc);

    let path = format!("{TEST_RUN_DIR}/{svc}.sock");
    create_stale_socket_file(&path);
    assert!(Path::new(&path).exists(), "stale socket should exist");

    // listen should recover it
    let listener = UdsListener::bind(TEST_RUN_DIR, &svc, default_server_config())
        .expect("listen should recover stale socket");
    drop(listener);
    cleanup_socket(&svc);
}

/// A server between bind() and listen() refuses connects just like a dead
/// server's leftover socket file. A second server starting in that window
/// must not take the endpoint over: it fails with AddrInUse, and the first
/// server stays reachable.
#[test]
fn test_listen_excludes_starting_server() {
    use std::sync::mpsc;
    ensure_run_dir();
    let svc = unique_service("rs_listen_window");
    cleanup_socket(&svc);
    let path = format!("{TEST_RUN_DIR}/{svc}.sock");

    let (bound_tx, bound_rx) = mpsc::channel::<()>();
    let (release_tx, release_rx) = mpsc::channel::<()>();
    let release_rx = std::sync::Mutex::new(release_rx);
    *LISTEN_HOOK.lock().unwrap() = Some((
        path.clone(),
        Box::new(move || {
            let _ = bound_tx.send(());
            let _ = release_rx.lock().unwrap().recv();
        }),
    ));
    let svc_first = svc.clone();
    let first =
        thread::spawn(move || UdsListener::bind(TEST_RUN_DIR, &svc_first, default_server_config()));
    bound_rx
        .recv_timeout(Duration::from_secs(10))
        .expect("first server bound");

    let second = UdsListener::bind(TEST_RUN_DIR, &svc, default_server_config());
    assert!(
        matches!(second, Err(UdsError::AddrInUse)),
        "second server must get AddrInUse, got {:?}",
        second.as_ref().map(|_| ())
    );
    drop(second);

    release_tx.send(()).unwrap();
    let first = first.join().unwrap().expect("first listen ok");

    // A connect to the endpoint must reach the first server.
    let cfd = uds_socket_cloexec();
    assert!(cfd >= 0);
    connect_unix(cfd, &path).expect("connect to endpoint");
    let mut pfd = libc::pollfd {
        fd: first.fd(),
        events: libc::POLLIN,
        revents: 0,
    };
    let ready = unsafe { libc::poll(&mut pfd, 1, 10_000) };
    unsafe { libc::close(cfd) };
    assert_eq!(ready, 1, "first server receives the connection");
    drop(first);

    // After the owner closed, the endpoint can be taken again.
    let again = UdsListener::bind(TEST_RUN_DIR, &svc, default_server_config())
        .expect("endpoint free after close");
    drop(again);
    cleanup_socket(&svc);
}

// A lock file left with a mode missing owner read or write (a server that
// ran under such a umask) must not block a start, and the start restores it.
#[test]
fn test_listen_over_leftover_lock_modes() {
    use std::os::unix::fs::PermissionsExt;
    ensure_run_dir();
    for mode in [0o200, 0o400] {
        let svc = unique_service("rs_lock_mode");
        cleanup_socket(&svc);
        let lock = format!("{TEST_RUN_DIR}/{svc}.lock");
        let _ = std::fs::remove_file(&lock);
        std::fs::write(&lock, b"").expect("create lock");
        std::fs::set_permissions(&lock, std::fs::Permissions::from_mode(mode)).unwrap();

        let first = UdsListener::bind(TEST_RUN_DIR, &svc, default_server_config())
            .unwrap_or_else(|e| panic!("listen over a {mode:o} lock: {e:?}"));
        let lock_mode = std::fs::metadata(&lock).unwrap().permissions().mode() & 0o777;
        assert_eq!(lock_mode, 0o600, "lock mode restored from {mode:o}");
        let second = UdsListener::bind(TEST_RUN_DIR, &svc, default_server_config());
        assert!(
            matches!(second, Err(UdsError::AddrInUse)),
            "second server must get AddrInUse, got {:?}",
            second.as_ref().map(|_| ())
        );
        drop(first);
        cleanup_socket(&svc);
        let _ = std::fs::remove_file(&lock);
    }
}

// An empty run_dir is a caller error: it would put the socket at "/<name>".
#[test]
fn test_empty_run_dir_rejected() {
    assert!(matches!(
        UdsListener::bind("", "rs_empty_run_dir", default_server_config()),
        Err(UdsError::BadParam(_))
    ));
    assert!(matches!(
        UdsSession::connect("", "rs_empty_run_dir", &default_client_config()),
        Err(UdsError::BadParam(_))
    ));
}

const LOCK_UMASK_HELPER_ENV: &str = "NETIPC_LOCK_UMASK_HELPER";
const LOCK_UMASK_HELPER_TEST: &str = "transport::posix::tests::lock_umask_helper_process";

// The umask is process-wide, so the restrictive-umask case runs in a child
// test process instead of the multithreaded test runner.
#[test]
fn test_listen_lock_under_restrictive_umask() {
    ensure_run_dir();
    for mask in ["477", "277", "777"] {
        let status = std::process::Command::new(std::env::current_exe().expect("current_exe"))
            .args([LOCK_UMASK_HELPER_TEST, "--exact", "--test-threads=1"])
            .env(LOCK_UMASK_HELPER_ENV, mask)
            .stdout(std::process::Stdio::null())
            .status()
            .expect("spawn umask helper");
        assert!(
            status.success(),
            "endpoint lock under umask {mask} failed: {status}"
        );
    }
}

/// Restores the umask on drop, also on panic, so files the process writes
/// at exit (coverage profiles) keep their normal mode.
struct UmaskGuard(libc::mode_t);

impl Drop for UmaskGuard {
    fn drop(&mut self) {
        unsafe { libc::umask(self.0) };
    }
}

/// Child-process entry point. Inert unless spawned by the test above.
#[test]
fn lock_umask_helper_process() {
    use std::os::unix::fs::PermissionsExt;

    let Some(mask) = std::env::var_os(LOCK_UMASK_HELPER_ENV) else {
        return;
    };
    let mask = libc::mode_t::from_str_radix(mask.to_str().expect("mask"), 8).expect("octal mask");
    let svc = format!("rs_lock_umask_{mask:o}");
    let lock = format!("{TEST_RUN_DIR}/{svc}.lock");
    cleanup_socket(&svc);
    let _ = std::fs::remove_file(&lock);

    let seen = std::sync::Arc::new(std::sync::Mutex::new(Vec::new()));
    let (hook_seen, hook_lock) = (seen.clone(), lock.clone());
    *LOCK_PUBLISH_HOOK.lock().unwrap() = Some(Box::new(move |tmp: &str| {
        let mode = std::fs::metadata(tmp)
            .expect("tmp stat")
            .permissions()
            .mode()
            & 0o777;
        let visible = std::fs::symlink_metadata(&hook_lock).is_ok();
        hook_seen
            .lock()
            .unwrap()
            .push((tmp.to_string(), mode, visible));
    }));
    // connect() needs write permission on the socket file. macOS sets the
    // socket to exactly 0600; Linux and FreeBSD keep the umask narrowing and
    // refuse to listen when it removes owner write.
    let unreachable = cfg!(not(target_os = "macos")) && mask & 0o200 != 0;
    let sock = format!("{TEST_RUN_DIR}/{svc}.sock");
    if unreachable {
        let first = {
            let _umask = UmaskGuard(unsafe { libc::umask(mask) });
            UdsListener::bind(TEST_RUN_DIR, &svc, default_server_config())
        };
        *LOCK_PUBLISH_HOOK.lock().unwrap() = None;
        assert!(
            matches!(first, Err(UdsError::Socket(libc::EACCES))),
            "listen under umask {mask:o} must fail with EACCES, got {:?}",
            first.as_ref().map(|_| ())
        );
        assert!(
            std::fs::symlink_metadata(&sock).is_err(),
            "no unreachable socket is left"
        );
        drop(
            UdsListener::bind(TEST_RUN_DIR, &svc, default_server_config())
                .expect("the endpoint starts under a usable umask"),
        );
    } else {
        let _umask = UmaskGuard(unsafe { libc::umask(mask) });
        let first = UdsListener::bind(TEST_RUN_DIR, &svc, default_server_config())
            .unwrap_or_else(|e| panic!("listen under umask {mask:o}: {e:?}"));
        *LOCK_PUBLISH_HOOK.lock().unwrap() = None;
        let sock_mode = std::fs::symlink_metadata(&sock)
            .expect("socket stat")
            .permissions()
            .mode()
            & 0o777;
        assert!(
            sock_mode & 0o200 != 0 && sock_mode & 0o077 == 0,
            "socket mode {sock_mode:o} under umask {mask:o}"
        );
        if cfg!(target_os = "macos") {
            assert_eq!(sock_mode, 0o600, "macOS socket mode under umask {mask:o}");
        }
        let second = UdsListener::bind(TEST_RUN_DIR, &svc, default_server_config());
        assert!(
            matches!(second, Err(UdsError::AddrInUse)),
            "second server must get AddrInUse, got {:?}",
            second.as_ref().map(|_| ())
        );
        drop(first);
        let again = UdsListener::bind(TEST_RUN_DIR, &svc, default_server_config())
            .unwrap_or_else(|e| panic!("restart under umask {mask:o}: {e:?}"));
        drop(again);
    }
    let lock_mode = std::fs::metadata(&lock)
        .expect("lock stat")
        .permissions()
        .mode()
        & 0o777;
    assert_eq!(lock_mode, 0o600, "lock mode under umask {mask:o}");
    let seen = seen.lock().unwrap();
    assert_eq!(seen.len(), 1, "the new lock is published once");
    let (tmp, tmp_mode, lock_visible) = &seen[0];
    assert_eq!(*tmp_mode, 0o600, "mode before the lock gets its name");
    assert!(!*lock_visible, "the lock name appears only after the link");
    assert!(
        std::fs::symlink_metadata(tmp).is_err(),
        "the temporary name is gone"
    );
    cleanup_socket(&svc);
    let _ = std::fs::remove_file(&lock);
}

// A child forked by another thread shares the lock's open file description
// until it execs. Closing the listener must still free the endpoint.
#[test]
fn test_listen_close_releases_lock_held_by_child() {
    ensure_run_dir();
    let svc = unique_service("rs_listen_child_lock");
    cleanup_socket(&svc);
    let first = UdsListener::bind(TEST_RUN_DIR, &svc, default_server_config()).expect("listen");

    let mut gate = [0 as libc::c_int; 2];
    assert_eq!(unsafe { libc::pipe(gate.as_mut_ptr()) }, 0, "pipe");
    let child = unsafe { libc::fork() };
    if child == 0 {
        // Keep every inherited fd until the parent closes the pipe. Only
        // async-signal-safe calls after fork.
        unsafe {
            libc::close(gate[1]);
            let mut c = 0u8;
            while libc::read(gate[0], (&raw mut c).cast(), 1) < 0 && errno() == libc::EINTR {}
            libc::_exit(0);
        }
    }
    assert!(child > 0, "fork");
    unsafe { libc::close(gate[0]) };

    drop(first);
    let again = UdsListener::bind(TEST_RUN_DIR, &svc, default_server_config());
    unsafe {
        libc::close(gate[1]);
        libc::waitpid(child, std::ptr::null_mut(), 0);
    }
    assert!(
        again.is_ok(),
        "endpoint free while the child holds the lock fd: {:?}",
        again.as_ref().map(|_| ())
    );
    drop(again);
    cleanup_socket(&svc);
}

#[test]
fn test_listen_socket_owner_only() {
    use std::os::unix::fs::PermissionsExt;
    ensure_run_dir();
    let svc = unique_service("rs_owner_only");
    cleanup_socket(&svc);
    let listener = UdsListener::bind(TEST_RUN_DIR, &svc, default_server_config()).expect("listen");
    let mode = std::fs::metadata(format!("{TEST_RUN_DIR}/{svc}.sock"))
        .expect("stat socket")
        .permissions()
        .mode();
    drop(listener);
    cleanup_socket(&svc);
    assert_eq!(mode & 0o777, 0o600, "listener socket must be owner-only");
}

#[test]
fn test_stale_recovery_in_group_writable_run_dir() {
    use std::os::unix::fs::PermissionsExt;
    // Crash recovery must work regardless of run-dir permissions
    // (netdata's systemd unit ships RuntimeDirectoryMode=0775).
    let dir = format!("{TEST_RUN_DIR}_0775");
    let _ = std::fs::create_dir_all(&dir);
    std::fs::set_permissions(&dir, std::fs::Permissions::from_mode(0o775)).expect("chmod 0775");

    let svc = unique_service("rs_stale_gw");
    crate::test_locks::track(&dir, &svc);
    let path = format!("{dir}/{svc}.sock");
    let _ = std::fs::remove_file(&path);
    create_stale_socket_file(&path);

    let listener = UdsListener::bind(&dir, &svc, default_server_config())
        .expect("stale recovery must work in a group-writable run dir");
    drop(listener);
    let _ = std::fs::remove_file(&path);
}

#[test]
fn test_foreign_file_at_socket_path_is_reclaimed() {
    ensure_run_dir();
    let svc = unique_service("rs_foreign");
    cleanup_socket(&svc);

    let path = format!("{TEST_RUN_DIR}/{svc}.sock");
    std::fs::write(&path, b"accidentally copied file").expect("write foreign file");

    let listener = UdsListener::bind(TEST_RUN_DIR, &svc, default_server_config())
        .expect("listen should silently replace a foreign file at the socket path");
    drop(listener);
    cleanup_socket(&svc);
}

// Unlinking a directory fails with EISDIR on Linux and EPERM on FreeBSD and
// macOS; every platform must reclaim an empty one.
#[test]
fn test_empty_directory_at_socket_path_is_reclaimed() {
    ensure_run_dir();
    let svc = unique_service("rs_stale_dir");
    cleanup_socket(&svc);

    let path = format!("{TEST_RUN_DIR}/{svc}.sock");
    let _ = std::fs::remove_dir(&path);
    std::fs::create_dir(&path).expect("create directory at socket path");

    let listener = UdsListener::bind(TEST_RUN_DIR, &svc, default_server_config())
        .expect("listen should reclaim an empty directory at the socket path");
    let meta = std::fs::symlink_metadata(&path).expect("socket path");
    assert!(
        std::os::unix::fs::FileTypeExt::is_socket(&meta.file_type()),
        "the socket must replace the directory"
    );
    drop(listener);
    cleanup_socket(&svc);
}

// -----------------------------------------------------------------------
//  Test 8: Disconnect detection
// -----------------------------------------------------------------------

#[test]
fn test_disconnect_detection() {
    ensure_run_dir();
    let svc = unique_service("rs_disc");
    cleanup_socket(&svc);

    let svc_clone = svc.clone();

    let listener =
        UdsListener::bind(TEST_RUN_DIR, &svc_clone, default_server_config()).expect("listen");
    let server_thread = thread::spawn(move || {
        let mut session = listener.accept().expect("accept");
        // Read request then close without responding
        let mut buf = [0u8; 4096];
        let _ = session.receive(&mut buf);
        drop(session); // close socket
    });

    let mut session =
        UdsSession::connect(TEST_RUN_DIR, &svc, &default_client_config()).expect("connect");

    let mut hdr = Header {
        kind: protocol::KIND_REQUEST,
        code: 1,
        item_count: 1,
        message_id: 99,
        ..Header::default()
    };
    session.send(&mut hdr, &[0xFF]).expect("send");
    session.inflight_ids.insert(100);

    // Receive should fail because server disconnected
    let mut rbuf = [0u8; 4096];
    let result = session.receive(&mut rbuf);
    assert!(result.is_err());
    assert!(
        session.inflight_ids.is_empty(),
        "disconnect must fail every in-flight request on the session"
    );

    drop(session);
    server_thread.join().expect("server join");
    cleanup_socket(&svc);
}

// -----------------------------------------------------------------------
//  Test 9: Batch send/receive
// -----------------------------------------------------------------------

#[test]
fn test_batch() {
    ensure_run_dir();
    let svc = unique_service("rs_batch");
    cleanup_socket(&svc);

    let svc_clone = svc.clone();

    let listener =
        UdsListener::bind(TEST_RUN_DIR, &svc_clone, default_server_config()).expect("listen");
    let server_thread = thread::spawn(move || {
        let mut session = listener.accept().expect("accept");
        let mut buf = [0u8; 8192];
        let (hdr, payload) = session.receive(&mut buf).expect("recv");
        let payload = payload.to_vec();
        let mut resp = hdr;
        resp.kind = protocol::KIND_RESPONSE;
        resp.transport_status = protocol::STATUS_OK;
        session.send(&mut resp, &payload).expect("send");
    });

    let mut session =
        UdsSession::connect(TEST_RUN_DIR, &svc, &default_client_config()).expect("connect");

    // Build batch using protocol layer
    let mut batch_buf = [0u8; 2048];
    let mut builder = protocol::BatchBuilder::new(&mut batch_buf, 3);

    let item0 = [0x10u8, 0x20];
    let item1 = [0x30u8, 0x40, 0x50];
    let item2 = [0x60u8];

    builder.add(&item0).expect("add item0");
    builder.add(&item1).expect("add item1");
    builder.add(&item2).expect("add item2");

    let (batch_len, batch_count) = builder.finish();
    assert_eq!(batch_count, 3);

    let mut hdr = Header {
        kind: protocol::KIND_REQUEST,
        code: protocol::METHOD_INCREMENT,
        flags: protocol::FLAG_BATCH,
        item_count: batch_count,
        message_id: 55,
        ..Header::default()
    };

    session
        .send(&mut hdr, &batch_buf[..batch_len])
        .expect("send batch");

    let mut rbuf = [0u8; 4096];
    let (rhdr, rpayload) = session.receive(&mut rbuf).expect("recv batch");
    assert_eq!(rhdr.message_id, 55);
    assert!(rhdr.flags & protocol::FLAG_BATCH != 0);
    assert_eq!(rhdr.item_count, 3);

    // Verify items
    let (ip0, len0) = protocol::batch_item_get(&rpayload, 3, 0).expect("item0");
    assert_eq!(len0, 2);
    assert_eq!(ip0, &item0);

    let (ip1, len1) = protocol::batch_item_get(&rpayload, 3, 1).expect("item1");
    assert_eq!(len1, 3);
    assert_eq!(ip1, &item1);

    let (ip2, len2) = protocol::batch_item_get(&rpayload, 3, 2).expect("item2");
    assert_eq!(len2, 1);
    assert_eq!(ip2, &item2);

    drop(session);
    server_thread.join().expect("server join");
    cleanup_socket(&svc);
}

// -----------------------------------------------------------------------
//  Test 10: Large chunked pipelining
// -----------------------------------------------------------------------

#[test]
fn test_chunked_pipelining() {
    ensure_run_dir();
    let svc = unique_service("rs_chkpipe");
    cleanup_socket(&svc);

    let svc_clone = svc.clone();
    let scfg = ServerConfig {
        packet_size: 128,
        max_request_payload_bytes: 65536,
        max_response_payload_bytes: 65536,
        ..default_server_config()
    };
    let listener = UdsListener::bind(TEST_RUN_DIR, &svc_clone, scfg).expect("listen");
    let server_thread = thread::spawn(move || {
        let mut session = listener.accept().expect("accept");

        // Echo 3 messages
        for _ in 0..3 {
            let mut buf = [0u8; 256];
            let (hdr, payload) = session.receive(&mut buf).expect("recv");
            let payload = payload.to_vec();
            let mut resp = hdr;
            resp.kind = protocol::KIND_RESPONSE;
            session.send(&mut resp, &payload).expect("send");
        }
    });

    let ccfg = ClientConfig {
        packet_size: 128,
        max_request_payload_bytes: 65536,
        max_response_payload_bytes: 65536,
        ..default_client_config()
    };
    let mut session = UdsSession::connect(TEST_RUN_DIR, &svc, &ccfg).expect("connect");

    // Send 3 chunked messages in pipeline
    let sizes = [200usize, 500, 300];
    for (i, &sz) in sizes.iter().enumerate() {
        let payload: Vec<u8> = (0..sz).map(|j| ((i + j) & 0xFF) as u8).collect();
        let mut hdr = Header {
            kind: protocol::KIND_REQUEST,
            code: 1,
            item_count: 1,
            message_id: (i + 1) as u64,
            ..Header::default()
        };
        session.send(&mut hdr, &payload).expect("send");
    }

    // Receive 3 responses
    for (i, &sz) in sizes.iter().enumerate() {
        let mut rbuf = [0u8; 256];
        let (rhdr, rpayload) = session.receive(&mut rbuf).expect("recv");
        assert_eq!(rhdr.message_id, (i + 1) as u64);
        let expected: Vec<u8> = (0..sz).map(|j| ((i + j) & 0xFF) as u8).collect();
        assert_eq!(rpayload, expected);
    }

    drop(session);
    server_thread.join().expect("server join");
    cleanup_socket(&svc);
}

// -----------------------------------------------------------------------
//  Test: Pipeline 10 requests, verify all matched by message_id
// -----------------------------------------------------------------------

#[test]
fn test_pipeline_10() {
    ensure_run_dir();
    let svc = unique_service("rs_pipe10");
    cleanup_socket(&svc);

    let svc_clone = svc.clone();

    let listener =
        UdsListener::bind(TEST_RUN_DIR, &svc_clone, default_server_config()).expect("listen");
    let server_thread = thread::spawn(move || {
        let mut session = listener.accept().expect("accept");

        for _ in 0..10 {
            let mut buf = [0u8; 8192];
            let (hdr, payload) = session.receive(&mut buf).expect("recv");
            let payload = payload.to_vec();
            let mut resp = hdr;
            resp.kind = protocol::KIND_RESPONSE;
            resp.transport_status = protocol::STATUS_OK;
            session.send(&mut resp, &payload).expect("send");
        }
    });

    let mut session =
        UdsSession::connect(TEST_RUN_DIR, &svc, &default_client_config()).expect("connect");

    // Send 10 requests before reading any response
    for i in 1u64..=10 {
        let payload = i.to_ne_bytes();
        let mut hdr = Header {
            kind: protocol::KIND_REQUEST,
            code: 1,
            item_count: 1,
            message_id: i,
            ..Header::default()
        };
        session.send(&mut hdr, &payload).expect("send");
    }

    // Receive 10 responses, verify message_id and payload
    for i in 1u64..=10 {
        let mut rbuf = [0u8; 4096];
        let (rhdr, rpayload) = session.receive(&mut rbuf).expect("recv");
        assert_eq!(rhdr.message_id, i, "message_id mismatch at {i}");
        assert_eq!(rpayload.len(), 8, "payload len at {i}");
        let val = u64::from_ne_bytes(rpayload.try_into().unwrap());
        assert_eq!(val, i, "payload value at {i}");
    }

    drop(session);
    server_thread.join().expect("server join");
    cleanup_socket(&svc);
}

// -----------------------------------------------------------------------
//  Test: Pipeline 100 requests (stress pipelining)
// -----------------------------------------------------------------------

#[test]
fn test_pipeline_100() {
    ensure_run_dir();
    let svc = unique_service("rs_pipe100");
    cleanup_socket(&svc);

    let svc_clone = svc.clone();
    let scfg = ServerConfig {
        max_request_payload_bytes: 65536,
        max_response_payload_bytes: 65536,
        ..default_server_config()
    };
    let listener = UdsListener::bind(TEST_RUN_DIR, &svc_clone, scfg).expect("listen");
    let server_thread = thread::spawn(move || {
        let mut session = listener.accept().expect("accept");

        for _ in 0..100 {
            let mut buf = [0u8; 8192];
            let (hdr, payload) = session.receive(&mut buf).expect("recv");
            let payload = payload.to_vec();
            let mut resp = hdr;
            resp.kind = protocol::KIND_RESPONSE;
            resp.transport_status = protocol::STATUS_OK;
            session.send(&mut resp, &payload).expect("send");
        }
    });

    let ccfg = ClientConfig {
        max_request_payload_bytes: 65536,
        max_response_payload_bytes: 65536,
        ..default_client_config()
    };
    let mut session = UdsSession::connect(TEST_RUN_DIR, &svc, &ccfg).expect("connect");

    // Send 100 requests
    for i in 1u64..=100 {
        let payload = i.to_ne_bytes();
        let mut hdr = Header {
            kind: protocol::KIND_REQUEST,
            code: 1,
            item_count: 1,
            message_id: i,
            ..Header::default()
        };
        session.send(&mut hdr, &payload).expect("send");
    }

    // Receive 100 responses
    for i in 1u64..=100 {
        let mut rbuf = [0u8; 4096];
        let (rhdr, rpayload) = session.receive(&mut rbuf).expect("recv");
        assert_eq!(rhdr.message_id, i);
        let val = u64::from_ne_bytes(rpayload.try_into().unwrap());
        assert_eq!(val, i);
    }

    drop(session);
    server_thread.join().expect("server join");
    cleanup_socket(&svc);
}

// -----------------------------------------------------------------------
//  Test: Pipeline with mixed message sizes
// -----------------------------------------------------------------------

#[test]
fn test_pipeline_mixed_sizes() {
    ensure_run_dir();
    let svc = unique_service("rs_pipemix");
    cleanup_socket(&svc);

    let svc_clone = svc.clone();

    let sizes = [8usize, 256, 1024, 8, 256, 1024, 8, 256, 1024];
    let count = sizes.len();

    let scfg = ServerConfig {
        max_request_payload_bytes: 65536,
        max_response_payload_bytes: 65536,
        ..default_server_config()
    };
    let listener = UdsListener::bind(TEST_RUN_DIR, &svc_clone, scfg).expect("listen");
    let server_thread = thread::spawn(move || {
        let mut session = listener.accept().expect("accept");

        for _ in 0..count {
            let mut buf = [0u8; 8192];
            let (hdr, payload) = session.receive(&mut buf).expect("recv");
            let payload = payload.to_vec();
            let mut resp = hdr;
            resp.kind = protocol::KIND_RESPONSE;
            resp.transport_status = protocol::STATUS_OK;
            session.send(&mut resp, &payload).expect("send");
        }
    });

    let ccfg = ClientConfig {
        max_request_payload_bytes: 65536,
        max_response_payload_bytes: 65536,
        ..default_client_config()
    };
    let mut session = UdsSession::connect(TEST_RUN_DIR, &svc, &ccfg).expect("connect");

    // Send all messages with varying sizes
    for (i, &sz) in sizes.iter().enumerate() {
        let payload: Vec<u8> = (0..sz).map(|j| ((i * 37 + j) & 0xFF) as u8).collect();
        let mut hdr = Header {
            kind: protocol::KIND_REQUEST,
            code: 1,
            item_count: 1,
            message_id: (i + 1) as u64,
            ..Header::default()
        };
        session.send(&mut hdr, &payload).expect("send");
    }

    // Receive all responses
    for (i, &sz) in sizes.iter().enumerate() {
        let mut rbuf = [0u8; 4096];
        let (rhdr, rpayload) = session.receive(&mut rbuf).expect("recv");
        assert_eq!(rhdr.message_id, (i + 1) as u64, "message_id at {i}");
        assert_eq!(rpayload.len(), sz, "payload len at {i}");
        let expected: Vec<u8> = (0..sz).map(|j| ((i * 37 + j) & 0xFF) as u8).collect();
        assert_eq!(rpayload, expected, "payload data at {i}");
    }

    drop(session);
    server_thread.join().expect("server join");
    cleanup_socket(&svc);
}

// -----------------------------------------------------------------------
//  Test: Pipeline with chunked messages (> packet_size)
// -----------------------------------------------------------------------

#[test]
fn test_pipeline_chunked_multi() {
    ensure_run_dir();
    let svc = unique_service("rs_pipechk2");
    cleanup_socket(&svc);

    let svc_clone = svc.clone();

    let sizes = [200usize, 500, 300, 800, 150];
    let count = sizes.len();

    let scfg = ServerConfig {
        packet_size: 128,
        max_request_payload_bytes: 65536,
        max_response_payload_bytes: 65536,
        ..default_server_config()
    };
    let listener = UdsListener::bind(TEST_RUN_DIR, &svc_clone, scfg).expect("listen");
    let server_thread = thread::spawn(move || {
        let mut session = listener.accept().expect("accept");

        for _ in 0..count {
            let mut buf = [0u8; 256];
            let (hdr, payload) = session.receive(&mut buf).expect("recv");
            let payload = payload.to_vec();
            let mut resp = hdr;
            resp.kind = protocol::KIND_RESPONSE;
            session.send(&mut resp, &payload).expect("send");
        }
    });

    let ccfg = ClientConfig {
        packet_size: 128,
        max_request_payload_bytes: 65536,
        max_response_payload_bytes: 65536,
        ..default_client_config()
    };
    let mut session = UdsSession::connect(TEST_RUN_DIR, &svc, &ccfg).expect("connect");

    // Send all chunked messages
    for (i, &sz) in sizes.iter().enumerate() {
        let payload: Vec<u8> = (0..sz).map(|j| ((i + j) & 0xFF) as u8).collect();
        let mut hdr = Header {
            kind: protocol::KIND_REQUEST,
            code: 1,
            item_count: 1,
            message_id: (i + 1) as u64,
            ..Header::default()
        };
        session.send(&mut hdr, &payload).expect("send");
    }

    // Receive all responses
    for (i, &sz) in sizes.iter().enumerate() {
        let mut rbuf = [0u8; 256];
        let (rhdr, rpayload) = session.receive(&mut rbuf).expect("recv");
        assert_eq!(rhdr.message_id, (i + 1) as u64);
        let expected: Vec<u8> = (0..sz).map(|j| ((i + j) & 0xFF) as u8).collect();
        assert_eq!(rpayload, expected);
    }

    drop(session);
    server_thread.join().expect("server join");
    cleanup_socket(&svc);
}

// Pipelining beyond the socket buffers. Small buffers make the deadlock
// reachable on every platform (macOS and FreeBSD 14 default to 8 KiB, FreeBSD
// 15 to 64 KiB, Linux to about 208 KiB). The client's send timeout only bounds a failing run: a
// passing run never waits on it, and the server has none, so a slow server
// cannot fail a passing run.
const PIPE_BUF_COUNT: usize = 16;
const PIPE_BUF_SIZE: usize = 16000;
const PIPE_BUF_PKT: u32 = 1024;
// Flood mode: small responses keep the client's queue bound small.
const PIPE_BUF_FLOOD_RESP: u32 = 4096;

fn shrink_socket_buffers(fd: RawFd, bound_sends: bool) {
    let size: libc::c_int = 4096;
    let tv = libc::timeval {
        tv_sec: 10,
        tv_usec: 0,
    };
    unsafe {
        for opt in [libc::SO_SNDBUF, libc::SO_RCVBUF] {
            libc::setsockopt(
                fd,
                libc::SOL_SOCKET,
                opt,
                &size as *const _ as *const libc::c_void,
                std::mem::size_of_val(&size) as libc::socklen_t,
            );
        }
        if !bound_sends {
            return;
        }
        libc::setsockopt(
            fd,
            libc::SOL_SOCKET,
            libc::SO_SNDTIMEO,
            &tv as *const _ as *const libc::c_void,
            std::mem::size_of_val(&tv) as libc::socklen_t,
        );
    }
}

#[derive(Clone, Copy, PartialEq)]
enum SmallBufMode {
    /// Receive one request, send its response.
    Echo,
    /// Receive one request, then close.
    CloseAfterOne,
    /// Receive one request, then send responses without reading until the
    /// send fails.
    Flood,
}

/// Starts the server in `mode` and connects a client; both ends get small
/// socket buffers.
fn small_buffer_start(
    prefix: &str,
    mode: SmallBufMode,
    max_response: u32,
) -> (String, thread::JoinHandle<()>, UdsSession) {
    ensure_run_dir();
    let svc = unique_service(prefix);
    cleanup_socket(&svc);

    let scfg = ServerConfig {
        packet_size: PIPE_BUF_PKT,
        max_request_payload_bytes: 65536,
        max_response_payload_bytes: max_response,
        ..default_server_config()
    };
    let listener = UdsListener::bind(TEST_RUN_DIR, &svc, scfg).expect("listen");
    let server = thread::spawn(move || {
        let mut session = listener.accept().expect("accept");
        shrink_socket_buffers(session.fd(), false);
        let count = if mode == SmallBufMode::Echo {
            PIPE_BUF_COUNT
        } else {
            1
        };
        for _ in 0..count {
            let mut buf = [0u8; PIPE_BUF_PKT as usize];
            let Ok((hdr, payload)) = session.receive(&mut buf) else {
                return;
            };
            if mode == SmallBufMode::Echo {
                let payload = payload.to_vec();
                let mut resp = hdr;
                resp.kind = protocol::KIND_RESPONSE;
                if session.send(&mut resp, &payload).is_err() {
                    return;
                }
            }
        }
        if mode == SmallBufMode::Flood {
            let payload = vec![0u8; PIPE_BUF_FLOOD_RESP as usize];
            loop {
                let mut resp = Header {
                    kind: protocol::KIND_RESPONSE,
                    code: 1,
                    item_count: 1,
                    message_id: 1,
                    ..Header::default()
                };
                if session.send(&mut resp, &payload).is_err() {
                    break;
                }
            }
        }
    });

    let ccfg = ClientConfig {
        packet_size: PIPE_BUF_PKT,
        max_request_payload_bytes: 65536,
        max_response_payload_bytes: max_response,
        ..default_client_config()
    };
    let session = UdsSession::connect(TEST_RUN_DIR, &svc, &ccfg).expect("connect");
    shrink_socket_buffers(session.fd(), true);
    (svc, server, session)
}

fn pipelined_payload(i: usize) -> Vec<u8> {
    (0..PIPE_BUF_SIZE)
        .map(|j| ((i * 7 + j) & 0xFF) as u8)
        .collect()
}

fn send_pipelined_request(session: &mut UdsSession, i: usize) -> Result<(), UdsError> {
    let mut hdr = Header {
        kind: protocol::KIND_REQUEST,
        code: 1,
        item_count: 1,
        message_id: (i + 1) as u64,
        ..Header::default()
    };
    session.send(&mut hdr, &pipelined_payload(i))
}

/// A client that sends every request before reading any response must not
/// deadlock with the server when the bytes in flight exceed the buffers.
#[test]
fn test_pipeline_exceeds_socket_buffers() {
    let (svc, server, mut session) = small_buffer_start("rs_pipebuf", SmallBufMode::Echo, 65536);

    assert!(!session.has_pending(), "no pending data before sending");
    for i in 0..PIPE_BUF_COUNT {
        send_pipelined_request(&mut session, i).expect("pipelined send");
    }
    // The buffers hold a fraction of the responses, so finishing the sends
    // required reading responses into the queue.
    assert!(session.has_pending(), "responses queued during the sends");
    for i in 0..PIPE_BUF_COUNT {
        let mut rbuf = [0u8; PIPE_BUF_PKT as usize];
        let (rhdr, rpayload) = session.receive(&mut rbuf).expect("recv");
        assert_eq!(rhdr.message_id, (i + 1) as u64);
        assert!(rpayload == pipelined_payload(i).as_slice(), "payload {i}");
    }
    assert!(session.pending.is_empty(), "pending queue drained");
    assert!(!session.has_pending(), "no pending data after receiving");

    drop(session);
    server.join().expect("server join");
    cleanup_socket(&svc);
}

/// Stream baseline: a receive that fails mid-stream marks the receive side failed, and
/// every later receive must fail. Bytes left in the pending queue are part
/// of that abandoned stream: here a forged response for an in-flight id
/// follows an over-limit header, and must not be delivered.
#[cfg(any(target_os = "macos", target_os = "freebsd"))]
#[test]
fn test_stream_failed_receive_drops_pending() {
    let (svc, server, mut session) =
        small_buffer_start("rs_pipestale", SmallBufMode::CloseAfterOne, 65536);
    send_pipelined_request(&mut session, 0).expect("request 1");

    let mut over = Header {
        magic: protocol::MAGIC_MSG,
        version: protocol::VERSION,
        header_len: HEADER_SIZE as u16,
        kind: protocol::KIND_RESPONSE,
        item_count: 1,
        message_id: 1,
        payload_len: 65537,
        ..Header::default()
    };
    let mut buf = vec![0u8; 2 * HEADER_SIZE];
    over.encode(&mut buf[..HEADER_SIZE]);
    over.payload_len = 0;
    over.encode(&mut buf[HEADER_SIZE..]);
    session.pending = Pending { buf, head: 0 };

    let mut rbuf = [0u8; PIPE_BUF_PKT as usize];
    assert!(matches!(
        session.receive(&mut rbuf),
        Err(UdsError::LimitExceeded)
    ));
    // No response can arrive any more, so the request in flight has failed.
    assert!(
        session.inflight_ids.is_empty(),
        "requests still in flight after the receive side failed"
    );
    assert!(session.receive(&mut rbuf).is_err(), "later receive fails");

    drop(session);
    server.join().expect("server join");
    cleanup_socket(&svc);
}

/// The server closes while the pipelining client waits for buffer space:
/// the send fails instead of waiting forever.
#[test]
fn test_pipeline_peer_close_during_send() {
    let (svc, server, mut session) =
        small_buffer_start("rs_pipeclose", SmallBufMode::CloseAfterOne, 65536);

    let mut result = Ok(());
    for i in 0..PIPE_BUF_COUNT {
        result = send_pipelined_request(&mut session, i);
        if result.is_err() {
            break;
        }
    }
    assert!(
        matches!(result, Err(UdsError::Send(_))),
        "expected Send, got {result:?}"
    );
    assert!(session.inflight_ids.is_empty(), "in-flight set cleared");

    drop(session);
    server.join().expect("server join");
    cleanup_socket(&svc);
}

/// A receive that times out leaves the client's in-flight requests pending,
/// as in C and Go: only a lost peer fails them. The late response is then
/// still accepted.
#[test]
fn test_receive_timeout_keeps_inflight() {
    let (fd0, fd1) = socketpair_seqpacket();
    let mut client = test_session(fd0, Role::Client, 4096);
    let mut server = test_session(fd1, Role::Server, 4096);
    client.inflight_ids.insert(9);

    let mut buf = [0u8; 256];
    let err = client
        .receive_with_control(&mut buf, 1, None)
        .expect_err("nothing to receive");
    assert!(matches!(err, UdsError::Timeout), "timeout: {err:?}");
    assert!(client.inflight_ids.contains(&9), "in-flight id kept");

    let mut hdr = Header {
        kind: KIND_RESPONSE,
        code: 1,
        item_count: 1,
        message_id: 9,
        ..Header::default()
    };
    server.send(&mut hdr, &[1, 2, 3, 4]).expect("late response");
    let (h, _) = client.receive(&mut buf).expect("late response accepted");
    assert_eq!(h.message_id, 9);
}

/// A receive into a buffer too small for a header fails without reading a
/// byte, so the message stays queued and the session stays usable.
#[test]
fn test_receive_undersized_buffer_keeps_session() {
    let (fd0, fd1) = socketpair_seqpacket();
    let mut rx = test_session(fd0, Role::Server, 4096);
    let mut tx = test_session(fd1, Role::Server, 4096);

    let payload = [1u8, 2, 3, 4, 5, 6, 7, 8];
    let mut hdr = Header {
        kind: KIND_REQUEST,
        code: 1,
        item_count: 1,
        message_id: 7,
        ..Header::default()
    };
    tx.send(&mut hdr, &payload).expect("request sent");

    let mut small = [0u8; HEADER_SIZE - 1];
    let err = rx.receive(&mut small).expect_err("undersized buffer");
    assert!(matches!(err, UdsError::BadParam(_)), "undersized: {err:?}");

    let mut buf = [0u8; 256];
    let (h, p) = rx
        .receive_with_control(&mut buf, 1000, None)
        .expect("request still received");
    assert_eq!(h.message_id, 7);
    assert_eq!(p, &payload);
}

/// A datagram longer than its header declares is a protocol error. The
/// pending queue, filled while a send waits for buffer space, must keep the
/// whole datagram so its receive rejects it as the direct receive does,
/// instead of storing it cut to packet_size and accepting it.
#[cfg(not(any(target_os = "macos", target_os = "freebsd")))]
#[test]
fn test_pending_oversized_datagram_rejected() {
    let (fd0, fd1) = socketpair_seqpacket();
    let packet_size = 2 * HEADER_SIZE;
    let mut session = test_session(fd0, Role::Server, packet_size as u32);

    // The header declares exactly packet_size; 16 more bytes follow.
    let mut dgram = vec![0xABu8; packet_size + 16];
    let hdr = Header {
        magic: MAGIC_MSG,
        version: VERSION,
        header_len: protocol::HEADER_LEN,
        kind: KIND_REQUEST,
        code: 1,
        flags: 0,
        transport_status: protocol::STATUS_OK,
        payload_len: HEADER_SIZE as u32,
        item_count: 1,
        message_id: 1,
    };
    hdr.encode(&mut dgram[..HEADER_SIZE]);
    let mut buf = [0u8; 256];

    raw_send(fd1, &dgram).expect("direct datagram");
    let err = session.receive(&mut buf).expect_err("direct receive");
    assert!(matches!(err, UdsError::Protocol(_)), "direct: {err:?}");

    raw_send(fd1, &dgram).expect("queued datagram");
    let mut eof = false;
    session
        .pending
        .drain(fd0, packet_size, 4096, &mut eof)
        .expect("drain");
    assert!(!eof && !session.pending.is_empty(), "drain queues it");
    let err = session.receive(&mut buf).expect_err("pending receive");
    assert!(matches!(err, UdsError::Protocol(_)), "pending: {err:?}");
    assert!(session.pending.is_empty(), "pending queue consumed");

    unsafe { libc::close(fd1) };
}

/// A server that sends more than the in-flight responses can occupy fails
/// the client session instead of growing its queue without limit.
#[test]
fn test_pipeline_pending_bound() {
    let (svc, server, mut session) =
        small_buffer_start("rs_pipebound", SmallBufMode::Flood, PIPE_BUF_FLOOD_RESP);

    let mut result = Ok(());
    for i in 0..PIPE_BUF_COUNT {
        result = send_pipelined_request(&mut session, i);
        if result.is_err() {
            break;
        }
    }
    assert!(
        matches!(result, Err(UdsError::Protocol(_))),
        "expected Protocol, got {result:?}"
    );
    assert!(session.inflight_ids.is_empty(), "in-flight set cleared");

    drop(session);
    server.join().expect("server join");
    cleanup_socket(&svc);
}

#[test]
fn test_invalid_service_name() {
    let bad_names = &["", ".", "..", "foo/bar", "../etc", "name space", "a@b"];
    for name in bad_names {
        let result = validate_service_name(name);
        assert!(result.is_err(), "should reject {:?}", name);
    }

    let good_names = &[
        "valid-name",
        "valid_name",
        "valid.name",
        "ValidName123",
        "a",
    ];
    for name in good_names {
        validate_service_name(name).unwrap_or_else(|e| panic!("{:?} should be valid: {e}", name));
    }
}

#[test]
fn test_hello_decode_nonzero_padding() {
    let h = Hello {
        layout_version: 1,
        supported_profiles: PROFILE_BASELINE,
        max_request_payload_bytes: 1024,
        max_request_batch_items: 1,
        max_response_payload_bytes: 1024,
        max_response_batch_items: 1,
        packet_size: 65536,
        ..Default::default()
    };

    let mut buf = [0u8; 44];
    h.encode(&mut buf);
    Hello::decode(&buf).expect("valid hello should decode");

    // Corrupt padding bytes 28..32
    buf[28] = 0xFF;
    assert_eq!(Hello::decode(&buf), Err(protocol::NipcError::BadLayout));
}

// -----------------------------------------------------------------------
//  UdsError Display coverage (lines 73-91)
// -----------------------------------------------------------------------

#[test]
fn uds_error_display_all_variants() {
    let cases: Vec<(UdsError, &str)> = vec![
        (UdsError::PathTooLong, "socket path exceeds sun_path limit"),
        (UdsError::Socket(22), "socket syscall failed: errno 22"),
        (UdsError::Connect(111), "connect failed: errno 111"),
        (UdsError::Accept(24), "accept failed: errno 24"),
        (UdsError::Send(32), "send failed: errno 32"),
        (UdsError::Recv(0), "recv failed: errno 0"),
        (UdsError::Handshake("test".into()), "handshake error: test"),
        (UdsError::AuthFailed, "authentication token rejected"),
        (UdsError::NoProfile, "no common transport profile"),
        (
            UdsError::Incompatible("version mismatch".into()),
            "incompatible protocol: version mismatch",
        ),
        (UdsError::Protocol("bad".into()), "protocol violation: bad"),
        (UdsError::AddrInUse, "address already in use by live server"),
        (UdsError::Chunk("mismatch".into()), "chunk error: mismatch"),
        (UdsError::Alloc, "memory allocation failed"),
        (UdsError::LimitExceeded, "negotiated limit exceeded"),
        (UdsError::BadParam("foo".into()), "bad parameter: foo"),
        (UdsError::DuplicateMsgId(42), "duplicate message_id: 42"),
        (
            UdsError::UnknownMsgId(99),
            "unknown response message_id: 99",
        ),
    ];
    for (err, expected) in cases {
        assert_eq!(format!("{}", err), expected);
    }
    // Verify Error trait
    let e: &dyn std::error::Error = &UdsError::PathTooLong;
    let _ = format!("{e}");
}

// -----------------------------------------------------------------------
//  UdsSession::role() coverage (lines 205-206)
// -----------------------------------------------------------------------

#[test]
fn test_session_role() {
    ensure_run_dir();
    let svc = unique_service("rs_role");
    cleanup_socket(&svc);

    let svc_clone = svc.clone();

    let listener =
        UdsListener::bind(TEST_RUN_DIR, &svc_clone, default_server_config()).expect("listen");
    let server_thread = thread::spawn(move || {
        let session = listener.accept().expect("accept");
        assert_eq!(session.role(), Role::Server);
    });

    let session =
        UdsSession::connect(TEST_RUN_DIR, &svc, &default_client_config()).expect("connect");
    assert_eq!(session.role(), Role::Client);

    drop(session);
    server_thread.join().expect("server join");
    cleanup_socket(&svc);
}

// -----------------------------------------------------------------------
//  Send on closed session (line 238)
// -----------------------------------------------------------------------

#[test]
fn test_send_on_closed_session() {
    ensure_run_dir();
    let svc = unique_service("rs_sendclosed");
    cleanup_socket(&svc);

    let svc_clone = svc.clone();

    let listener =
        UdsListener::bind(TEST_RUN_DIR, &svc_clone, default_server_config()).expect("listen");
    let server_thread = thread::spawn(move || {
        let _session = listener.accept().expect("accept");
    });

    let mut session =
        UdsSession::connect(TEST_RUN_DIR, &svc, &default_client_config()).expect("connect");

    // Simulate closed session by closing the fd via Drop-like behavior
    // We can't call close() (no such method), so we close fd directly
    unsafe {
        libc::close(session.fd);
    }
    session.fd = -1;

    let mut hdr = Header {
        kind: protocol::KIND_REQUEST,
        code: 1,
        item_count: 1,
        message_id: 1,
        ..Header::default()
    };
    let result = session.send(&mut hdr, &[1, 2, 3]);
    assert!(matches!(result, Err(UdsError::BadParam(_))));

    // Receive on closed session (line 333)
    let mut rbuf = [0u8; 4096];
    let result = session.receive(&mut rbuf);
    assert!(matches!(result, Err(UdsError::BadParam(_))));

    server_thread.join().expect("server join");
    cleanup_socket(&svc);
}

#[test]
fn test_accept_on_closed_listener_returns_accept_error() {
    ensure_run_dir();
    let svc = unique_service("rs_accept_closed");
    cleanup_socket(&svc);

    let mut listener =
        UdsListener::bind(TEST_RUN_DIR, &svc, default_server_config()).expect("listen");
    let fd = listener.fd;
    assert!(fd >= 0, "listener fd should be valid");
    assert_eq!(unsafe { libc::close(fd) }, 0, "close listener fd");
    // A parallel test thread may reuse the closed number at once; accepting on
    // it would succeed and a later close would hit that thread's fd. Point the
    // listener at a number that is never open instead.
    listener.fd = 1 << 30;

    let err = match listener.accept() {
        Ok(_) => panic!("accept on closed listener should fail"),
        Err(err) => err,
    };
    assert!(matches!(err, UdsError::Accept(_)));

    listener.fd = -1;
    drop(listener);
    cleanup_socket(&svc);
}

// -----------------------------------------------------------------------
//  Duplicate message_id (line 244)
// -----------------------------------------------------------------------

#[test]
fn test_duplicate_message_id() {
    ensure_run_dir();
    let svc = unique_service("rs_dupmsg");
    cleanup_socket(&svc);

    let svc_clone = svc.clone();

    let listener =
        UdsListener::bind(TEST_RUN_DIR, &svc_clone, default_server_config()).expect("listen");
    let (done_tx, done_rx) = std::sync::mpsc::channel::<()>();
    let server_thread = thread::spawn(move || {
        let _session = listener.accept().expect("accept");
        // Hold the connection open until the client is done.
        let _ = done_rx.recv();
    });

    let mut session =
        UdsSession::connect(TEST_RUN_DIR, &svc, &default_client_config()).expect("connect");

    // First send with message_id=42 should succeed
    let mut hdr1 = Header {
        kind: protocol::KIND_REQUEST,
        code: 1,
        item_count: 1,
        message_id: 42,
        ..Header::default()
    };
    session.send(&mut hdr1, &[1]).expect("first send ok");

    // Second send with same message_id=42 should fail
    let mut hdr2 = Header {
        kind: protocol::KIND_REQUEST,
        code: 1,
        item_count: 1,
        message_id: 42,
        ..Header::default()
    };
    let result = session.send(&mut hdr2, &[2]);
    assert!(matches!(result, Err(UdsError::DuplicateMsgId(42))));

    drop(session);
    drop(done_tx);
    server_thread.join().expect("server join");
    cleanup_socket(&svc);
}

// -----------------------------------------------------------------------
//  Receive: payload exceeds limit (line 354)
// -----------------------------------------------------------------------

#[test]
fn test_receive_payload_exceeds_limit() {
    let (fd0, fd1) = socketpair_seqpacket();
    let mut session = test_session(fd0, Role::Client, 4096);
    session.max_response_payload_bytes = 16;
    session.inflight_ids.insert(99);

    let payload = [0xAB; 32];
    let hdr = Header {
        magic: MAGIC_MSG,
        version: VERSION,
        header_len: protocol::HEADER_LEN,
        kind: KIND_RESPONSE,
        code: 1,
        flags: 0,
        transport_status: protocol::STATUS_OK,
        payload_len: payload.len() as u32,
        item_count: 1,
        message_id: 99,
    };
    let mut pkt = [0u8; HEADER_SIZE + 32];
    hdr.encode(&mut pkt[..HEADER_SIZE]);
    pkt[HEADER_SIZE..].copy_from_slice(&payload);
    raw_send(fd1, &pkt).expect("send oversized payload");

    let mut buf = [0u8; 64];
    let err = session
        .receive(&mut buf)
        .expect_err("payload exceeds limit");
    assert!(matches!(err, UdsError::LimitExceeded));

    unsafe { libc::close(fd1) };
}

// -----------------------------------------------------------------------
//  Receive: batch item_count exceeds limit (line 364)
// -----------------------------------------------------------------------

#[test]
fn test_receive_batch_count_exceeds_limit() {
    let (fd0, fd1) = socketpair_seqpacket();
    let mut session = test_session(fd0, Role::Client, 4096);
    session.max_response_batch_items = 16;
    session.inflight_ids.insert(1);

    let hdr = Header {
        magic: MAGIC_MSG,
        version: VERSION,
        header_len: protocol::HEADER_LEN,
        kind: KIND_RESPONSE,
        code: 1,
        flags: 0,
        transport_status: protocol::STATUS_OK,
        payload_len: 1,
        item_count: 17,
        message_id: 1,
    };
    let mut pkt = [0u8; HEADER_SIZE + 1];
    hdr.encode(&mut pkt[..HEADER_SIZE]);
    pkt[HEADER_SIZE] = 0xAD;
    raw_send(fd1, &pkt).expect("send oversized batch-count response");

    let mut buf = [0u8; 64];
    let err = session
        .receive(&mut buf)
        .expect_err("batch count exceeds limit");
    assert!(matches!(err, UdsError::LimitExceeded));

    unsafe { libc::close(fd1) };
}

// -----------------------------------------------------------------------
//  Receive: abort-poll slices must not end the wait (SOW-0041)
// -----------------------------------------------------------------------

/// Send a one-byte response for message 1 on `fd` after `delay`.
fn send_response_after(fd: RawFd, delay: Duration) -> thread::JoinHandle<()> {
    thread::spawn(move || {
        thread::sleep(delay);
        let hdr = Header {
            magic: MAGIC_MSG,
            version: VERSION,
            header_len: protocol::HEADER_LEN,
            kind: KIND_RESPONSE,
            code: 1,
            flags: 0,
            transport_status: protocol::STATUS_OK,
            payload_len: 1,
            item_count: 1,
            message_id: 1,
        };
        let mut pkt = [0u8; HEADER_SIZE + 1];
        hdr.encode(&mut pkt[..HEADER_SIZE]);
        pkt[HEADER_SIZE] = 0x5A;
        raw_send(fd, &pkt).expect("send delayed response");
    })
}

#[test]
fn test_receive_with_abort_waits_past_poll_slice() {
    // (timeout_ms, label): 0 means wait indefinitely.
    for (timeout_ms, label) in [(0u32, "infinite"), (2000u32, "2000 ms")] {
        let (fd0, fd1) = socketpair_seqpacket();
        let mut session = test_session(fd0, Role::Client, 4096);
        session.inflight_ids.insert(1);
        let abort = AtomicBool::new(false);
        let sender = send_response_after(fd1, Duration::from_millis(300));

        let mut buf = [0u8; 64];
        let started = std::time::Instant::now();
        let result = session
            .receive_with_control(&mut buf, timeout_ms, Some(&abort))
            .map(|(hdr, payload)| (hdr.message_id, payload.to_vec()));
        let elapsed = started.elapsed();

        assert_eq!(
            result.expect(label),
            (1, vec![0x5A]),
            "{label}: delayed response must arrive"
        );
        assert!(
            elapsed >= Duration::from_millis(250),
            "{label}: {elapsed:?}"
        );
        sender.join().expect("sender join");
        unsafe { libc::close(fd1) };
    }
}

#[test]
fn test_receive_abort_after_poll_slice_reports_aborted() {
    let (fd0, fd1) = socketpair_seqpacket();
    let mut session = test_session(fd0, Role::Client, 4096);
    session.inflight_ids.insert(1);
    let abort = Arc::new(AtomicBool::new(false));
    let abort_setter = abort.clone();
    let setter = thread::spawn(move || {
        thread::sleep(Duration::from_millis(300));
        abort_setter.store(true, Ordering::Release);
    });

    let mut buf = [0u8; 64];
    let started = std::time::Instant::now();
    let err = session
        .receive_with_control(&mut buf, 5000, Some(&abort))
        .expect_err("abort must end the wait");
    let elapsed = started.elapsed();

    assert!(matches!(err, UdsError::Aborted), "got {err:?}");
    assert!(elapsed >= Duration::from_millis(250), "{elapsed:?}");
    assert!(elapsed < Duration::from_millis(2000), "{elapsed:?}");
    setter.join().expect("setter join");
    unsafe { libc::close(fd1) };
}

#[test]
fn test_directional_limit_negotiation() {
    ensure_run_dir();
    let svc = unique_service("rs_dir_limits");
    cleanup_socket(&svc);

    let svc_clone = svc.clone();
    let scfg = ServerConfig {
        max_request_payload_bytes: 4096,
        max_request_batch_items: 8,
        max_response_payload_bytes: 8192,
        max_response_batch_items: 32,
        ..default_server_config()
    };
    let listener = UdsListener::bind(TEST_RUN_DIR, &svc_clone, scfg).expect("listen");
    let server_thread = thread::spawn(move || {
        let session = listener.accept().expect("accept");
        assert_eq!(session.max_request_payload_bytes, 4096);
        assert_eq!(session.max_request_batch_items, 16);
        assert_eq!(session.max_response_payload_bytes, 8192);
        assert_eq!(session.max_response_batch_items, 16);
        assert_ne!(session.session_id, 0);
    });

    let ccfg = ClientConfig {
        max_request_payload_bytes: 4096,
        max_request_batch_items: 16,
        max_response_payload_bytes: 4096,
        max_response_batch_items: 16,
        ..default_client_config()
    };
    let session = UdsSession::connect(TEST_RUN_DIR, &svc, &ccfg).expect("connect");

    assert_eq!(session.max_request_payload_bytes, 4096);
    assert_eq!(session.max_request_batch_items, 16);
    assert_eq!(session.max_response_payload_bytes, 8192);
    assert_eq!(session.max_response_batch_items, 16);
    assert_ne!(session.session_id, 0);

    drop(session);
    server_thread.join().expect("server join");
    cleanup_socket(&svc);
}

// -----------------------------------------------------------------------
//  Connect to nonexistent socket (line 220)
// -----------------------------------------------------------------------

#[test]
fn test_connect_nonexistent() {
    ensure_run_dir();
    let svc = unique_service("rs_noexist");
    cleanup_socket(&svc);

    let result = UdsSession::connect(TEST_RUN_DIR, &svc, &default_client_config());
    assert!(matches!(result, Err(UdsError::Connect(_))));
}

#[test]
fn test_send_packet_size_too_small_rolls_back_inflight() {
    let (fd0, fd1) = socketpair_seqpacket();
    let mut session = test_session(fd0, Role::Client, HEADER_SIZE as u32);

    let mut hdr = Header {
        kind: KIND_REQUEST,
        code: 1,
        item_count: 1,
        message_id: 77,
        ..Header::default()
    };

    let err = session
        .send(&mut hdr, &[0xAA])
        .expect_err("packet_size too small");
    assert!(matches!(err, UdsError::BadParam(_)));
    assert!(
        !session.inflight_ids.contains(&77),
        "failed send should roll back the tracked message_id"
    );

    unsafe { libc::close(fd1) };
}

#[test]
fn test_receive_packet_too_short_for_header() {
    let (fd0, fd1) = socketpair_seqpacket();
    let mut session = test_session(fd0, Role::Server, 4096);

    raw_send(fd1, &[0x01]).expect("send short packet");
    end_stream_writes(fd1);

    let mut buf = [0u8; 64];
    let err = session.receive(&mut buf).expect_err("packet too short");
    assert!(matches!(err, UdsError::Protocol(_)));

    unsafe { libc::close(fd1) };
}

#[test]
fn test_receive_packet_longer_than_declared_payload() {
    let (fd0, fd1) = socketpair_seqpacket();
    let mut session = test_session(fd0, Role::Server, 4096);

    let payload = [0xBE, 0xEF];
    let mut pkt = [0u8; HEADER_SIZE + 2];
    let hdr = Header {
        magic: MAGIC_MSG,
        version: VERSION,
        header_len: protocol::HEADER_LEN,
        kind: KIND_REQUEST,
        code: 1,
        flags: 0,
        transport_status: protocol::STATUS_OK,
        payload_len: 1,
        item_count: 1,
        message_id: 1,
    };
    hdr.encode(&mut pkt[..HEADER_SIZE]);
    pkt[HEADER_SIZE..].copy_from_slice(&payload);

    raw_send(fd1, &pkt).expect("send packet with trailing bytes");
    end_stream_writes(fd1);

    let mut buf = [0u8; 128];
    // A stream has no packet end to check: the message completes and the
    // trailing byte is rejected as the next packet, a truncated header.
    #[cfg(any(target_os = "macos", target_os = "freebsd"))]
    {
        let (_, payload) = session.receive(&mut buf).expect("message delivered");
        assert_eq!(payload, &[0xBE]);
    }
    let err = session
        .receive(&mut buf)
        .expect_err("trailing bytes should be rejected");
    #[cfg(not(any(target_os = "macos", target_os = "freebsd")))]
    assert!(matches!(err, UdsError::Protocol(ref msg)
        if msg.contains("exceeds declared payload_len")));
    #[cfg(any(target_os = "macos", target_os = "freebsd"))]
    assert!(matches!(err, UdsError::Protocol(_)));

    unsafe { libc::close(fd1) };
}

#[test]
fn test_receive_batch_directory_too_short_nonchunked() {
    let (fd0, fd1) = socketpair_seqpacket();
    let mut session = test_session(fd0, Role::Server, 4096);

    let payload = [0u8; 8];
    let mut pkt = [0u8; HEADER_SIZE + 8];
    let hdr = Header {
        magic: MAGIC_MSG,
        version: VERSION,
        header_len: protocol::HEADER_LEN,
        kind: KIND_REQUEST,
        code: 1,
        flags: FLAG_BATCH,
        transport_status: protocol::STATUS_OK,
        payload_len: payload.len() as u32,
        item_count: 2,
        message_id: 1,
    };
    hdr.encode(&mut pkt[..HEADER_SIZE]);
    pkt[HEADER_SIZE..].copy_from_slice(&payload);

    raw_send(fd1, &pkt).expect("send malformed batch packet");

    let mut buf = [0u8; 128];
    let err = session
        .receive(&mut buf)
        .expect_err("batch directory exceeds payload");
    assert!(matches!(err, UdsError::Protocol(_)));

    unsafe { libc::close(fd1) };
}

#[test]
fn test_receive_batch_directory_invalid_nonchunked() {
    let (fd0, fd1) = socketpair_seqpacket();
    let mut session = test_session(fd0, Role::Server, 4096);

    let mut payload = [0u8; 16];
    payload[0..4].copy_from_slice(&0u32.to_ne_bytes());
    payload[4..8].copy_from_slice(&32u32.to_ne_bytes());
    payload[8..12].copy_from_slice(&0u32.to_ne_bytes());
    payload[12..16].copy_from_slice(&32u32.to_ne_bytes());

    let mut pkt = [0u8; HEADER_SIZE + 16];
    let hdr = Header {
        magic: MAGIC_MSG,
        version: VERSION,
        header_len: protocol::HEADER_LEN,
        kind: KIND_REQUEST,
        code: 1,
        flags: FLAG_BATCH,
        transport_status: protocol::STATUS_OK,
        payload_len: payload.len() as u32,
        item_count: 2,
        message_id: 2,
    };
    hdr.encode(&mut pkt[..HEADER_SIZE]);
    pkt[HEADER_SIZE..].copy_from_slice(&payload);

    raw_send(fd1, &pkt).expect("send invalid batch packet");

    let mut buf = [0u8; 128];
    let err = session
        .receive(&mut buf)
        .expect_err("batch directory validation should fail");
    assert!(matches!(err, UdsError::Protocol(_)));

    unsafe { libc::close(fd1) };
}

#[test]
fn test_receive_chunk_message_id_mismatch() {
    let (fd0, fd1) = socketpair_seqpacket();
    let mut session = test_session(fd0, Role::Server, (HEADER_SIZE + 10) as u32);

    let first_payload = [1u8; 10];
    let hdr = Header {
        magic: MAGIC_MSG,
        version: VERSION,
        header_len: protocol::HEADER_LEN,
        kind: KIND_REQUEST,
        code: 1,
        flags: 0,
        transport_status: protocol::STATUS_OK,
        payload_len: 20,
        item_count: 1,
        message_id: 11,
    };
    let mut first_pkt = [0u8; HEADER_SIZE + 10];
    hdr.encode(&mut first_pkt[..HEADER_SIZE]);
    first_pkt[HEADER_SIZE..].copy_from_slice(&first_payload);
    raw_send(fd1, &first_pkt).expect("send first chunk");

    let second_payload = [2u8; 10];
    let chk = ChunkHeader {
        magic: MAGIC_CHUNK,
        version: VERSION,
        flags: 0,
        message_id: 12,
        total_message_len: (HEADER_SIZE + 20) as u32,
        chunk_index: 1,
        chunk_count: 2,
        chunk_payload_len: second_payload.len() as u32,
    };
    let mut second_pkt = [0u8; HEADER_SIZE + 10];
    chk.encode(&mut second_pkt[..HEADER_SIZE]);
    second_pkt[HEADER_SIZE..].copy_from_slice(&second_payload);
    raw_send(fd1, &second_pkt).expect("send mismatched continuation");

    let mut buf = [0u8; 64];
    let err = session.receive(&mut buf).expect_err("message_id mismatch");
    assert!(matches!(err, UdsError::Chunk(_)));

    unsafe { libc::close(fd1) };
}

#[test]
fn test_receive_chunk_index_mismatch() {
    let (fd0, fd1) = socketpair_seqpacket();
    let mut session = test_session(fd0, Role::Server, (HEADER_SIZE + 10) as u32);

    let first_payload = [1u8; 10];
    let hdr = Header {
        magic: MAGIC_MSG,
        version: VERSION,
        header_len: protocol::HEADER_LEN,
        kind: KIND_REQUEST,
        code: 1,
        flags: 0,
        transport_status: protocol::STATUS_OK,
        payload_len: 20,
        item_count: 1,
        message_id: 11,
    };
    let mut first_pkt = [0u8; HEADER_SIZE + 10];
    hdr.encode(&mut first_pkt[..HEADER_SIZE]);
    first_pkt[HEADER_SIZE..].copy_from_slice(&first_payload);
    raw_send(fd1, &first_pkt).expect("send first chunk");

    let second_payload = [2u8; 10];
    let chk = ChunkHeader {
        magic: MAGIC_CHUNK,
        version: VERSION,
        flags: 0,
        message_id: 11,
        total_message_len: (HEADER_SIZE + 20) as u32,
        chunk_index: 2,
        chunk_count: 2,
        chunk_payload_len: second_payload.len() as u32,
    };
    let mut second_pkt = [0u8; HEADER_SIZE + 10];
    chk.encode(&mut second_pkt[..HEADER_SIZE]);
    second_pkt[HEADER_SIZE..].copy_from_slice(&second_payload);
    raw_send(fd1, &second_pkt).expect("send mismatched continuation");

    let mut buf = [0u8; 64];
    let err = session.receive(&mut buf).expect_err("chunk index mismatch");
    assert!(matches!(
        err,
        UdsError::Chunk(ref msg) if msg == "chunk_index mismatch: expected 1, got 2"
    ));

    unsafe { libc::close(fd1) };
}

#[test]
fn test_receive_chunked_batch_directory_too_short() {
    let (fd0, fd1) = socketpair_seqpacket();
    let mut session = test_session(fd0, Role::Server, (HEADER_SIZE + 16) as u32);

    let first_payload = [0u8; 8];
    let hdr = Header {
        magic: MAGIC_MSG,
        version: VERSION,
        header_len: protocol::HEADER_LEN,
        kind: KIND_REQUEST,
        code: 1,
        flags: FLAG_BATCH,
        transport_status: protocol::STATUS_OK,
        payload_len: 12,
        item_count: 2,
        message_id: 21,
    };
    let mut first_pkt = [0u8; HEADER_SIZE + 8];
    hdr.encode(&mut first_pkt[..HEADER_SIZE]);
    first_pkt[HEADER_SIZE..].copy_from_slice(&first_payload);
    raw_send(fd1, &first_pkt).expect("send first chunk");

    let second_payload = [0u8; 4];
    let chk = ChunkHeader {
        magic: MAGIC_CHUNK,
        version: VERSION,
        flags: 0,
        message_id: 21,
        total_message_len: (HEADER_SIZE + 12) as u32,
        chunk_index: 1,
        chunk_count: 2,
        chunk_payload_len: second_payload.len() as u32,
    };
    let mut second_pkt = [0u8; HEADER_SIZE + 4];
    chk.encode(&mut second_pkt[..HEADER_SIZE]);
    second_pkt[HEADER_SIZE..].copy_from_slice(&second_payload);
    raw_send(fd1, &second_pkt).expect("send continuation");

    let mut buf = [0u8; 64];
    let err = session
        .receive(&mut buf)
        .expect_err("chunked batch directory too short");
    assert!(matches!(err, UdsError::Protocol(_)));

    unsafe { libc::close(fd1) };
}

#[test]
fn test_receive_chunked_batch_directory_invalid() {
    let (fd0, fd1) = socketpair_seqpacket();
    // The first chunk below carries 16 payload bytes, a full packet.
    let mut session = test_session(fd0, Role::Server, (HEADER_SIZE + 16) as u32);

    let mut full_payload = [0u8; 24];
    full_payload[0..4].copy_from_slice(&0u32.to_ne_bytes());
    full_payload[4..8].copy_from_slice(&16u32.to_ne_bytes());
    full_payload[8..12].copy_from_slice(&0u32.to_ne_bytes());
    full_payload[12..16].copy_from_slice(&4u32.to_ne_bytes());
    full_payload[16..24].copy_from_slice(b"payload!");

    let hdr = Header {
        magic: MAGIC_MSG,
        version: VERSION,
        header_len: protocol::HEADER_LEN,
        kind: KIND_REQUEST,
        code: 1,
        flags: FLAG_BATCH,
        transport_status: protocol::STATUS_OK,
        payload_len: full_payload.len() as u32,
        item_count: 2,
        message_id: 46,
    };
    let mut first_pkt = [0u8; HEADER_SIZE + 16];
    hdr.encode(&mut first_pkt[..HEADER_SIZE]);
    first_pkt[HEADER_SIZE..].copy_from_slice(&full_payload[..16]);
    raw_send(fd1, &first_pkt).expect("send first chunk");

    let second_payload = &full_payload[16..];
    let chk = ChunkHeader {
        magic: MAGIC_CHUNK,
        version: VERSION,
        flags: 0,
        message_id: 46,
        total_message_len: (HEADER_SIZE + full_payload.len()) as u32,
        chunk_index: 1,
        chunk_count: 2,
        chunk_payload_len: second_payload.len() as u32,
    };
    let mut second_pkt = [0u8; HEADER_SIZE + 8];
    chk.encode(&mut second_pkt[..HEADER_SIZE]);
    second_pkt[HEADER_SIZE..HEADER_SIZE + second_payload.len()].copy_from_slice(second_payload);
    raw_send(fd1, &second_pkt).expect("send invalid continuation");

    let mut buf = [0u8; 64];
    let err = session
        .receive(&mut buf)
        .expect_err("chunked batch directory validation should fail");
    assert!(matches!(err, UdsError::Protocol(_)));

    unsafe { libc::close(fd1) };
}

#[test]
fn test_receive_chunk_total_message_len_mismatch() {
    let (fd0, fd1) = socketpair_seqpacket();
    let mut session = test_session(fd0, Role::Server, (HEADER_SIZE + 10) as u32);

    let first_payload = [1u8; 10];
    let hdr = Header {
        magic: MAGIC_MSG,
        version: VERSION,
        header_len: protocol::HEADER_LEN,
        kind: KIND_REQUEST,
        code: 1,
        flags: 0,
        transport_status: protocol::STATUS_OK,
        payload_len: 20,
        item_count: 1,
        message_id: 41,
    };
    let mut first_pkt = [0u8; HEADER_SIZE + 10];
    hdr.encode(&mut first_pkt[..HEADER_SIZE]);
    first_pkt[HEADER_SIZE..].copy_from_slice(&first_payload);
    raw_send(fd1, &first_pkt).expect("send first chunk");

    let second_payload = [2u8; 10];
    let chk = ChunkHeader {
        magic: MAGIC_CHUNK,
        version: VERSION,
        flags: 0,
        message_id: 41,
        total_message_len: (HEADER_SIZE + 21) as u32,
        chunk_index: 1,
        chunk_count: 2,
        chunk_payload_len: second_payload.len() as u32,
    };
    let mut second_pkt = [0u8; HEADER_SIZE + 10];
    chk.encode(&mut second_pkt[..HEADER_SIZE]);
    second_pkt[HEADER_SIZE..].copy_from_slice(&second_payload);
    raw_send(fd1, &second_pkt).expect("send bad total_message_len");

    let mut buf = [0u8; 64];
    let err = session
        .receive(&mut buf)
        .expect_err("total_message_len mismatch");
    assert!(matches!(err, UdsError::Chunk(_)));

    unsafe { libc::close(fd1) };
}

#[test]
fn test_receive_chunk_payload_len_mismatch() {
    let (fd0, fd1) = socketpair_seqpacket();
    let mut session = test_session(fd0, Role::Server, (HEADER_SIZE + 10) as u32);

    let first_payload = [1u8; 10];
    let hdr = Header {
        magic: MAGIC_MSG,
        version: VERSION,
        header_len: protocol::HEADER_LEN,
        kind: KIND_REQUEST,
        code: 1,
        flags: 0,
        transport_status: protocol::STATUS_OK,
        payload_len: 20,
        item_count: 1,
        message_id: 42,
    };
    let mut first_pkt = [0u8; HEADER_SIZE + 10];
    hdr.encode(&mut first_pkt[..HEADER_SIZE]);
    first_pkt[HEADER_SIZE..].copy_from_slice(&first_payload);
    raw_send(fd1, &first_pkt).expect("send first chunk");

    let second_payload = [2u8; 10];
    let chk = ChunkHeader {
        magic: MAGIC_CHUNK,
        version: VERSION,
        flags: 0,
        message_id: 42,
        total_message_len: (HEADER_SIZE + 20) as u32,
        chunk_index: 1,
        chunk_count: 2,
        chunk_payload_len: 9,
    };
    let mut second_pkt = [0u8; HEADER_SIZE + 10];
    chk.encode(&mut second_pkt[..HEADER_SIZE]);
    second_pkt[HEADER_SIZE..].copy_from_slice(&second_payload);
    raw_send(fd1, &second_pkt).expect("send bad chunk length");
    end_stream_writes(fd1);

    let mut buf = [0u8; 64];
    let err = session
        .receive(&mut buf)
        .expect_err("chunk_payload_len mismatch");
    assert!(matches!(err, UdsError::Chunk(_)));

    unsafe { libc::close(fd1) };
}

#[test]
fn test_receive_continuation_too_short() {
    let (fd0, fd1) = socketpair_seqpacket();
    let mut session = test_session(fd0, Role::Server, (HEADER_SIZE + 10) as u32);

    let first_payload = [1u8; 10];
    let hdr = Header {
        magic: MAGIC_MSG,
        version: VERSION,
        header_len: protocol::HEADER_LEN,
        kind: KIND_REQUEST,
        code: 1,
        flags: 0,
        transport_status: protocol::STATUS_OK,
        payload_len: 20,
        item_count: 1,
        message_id: 43,
    };
    let mut first_pkt = [0u8; HEADER_SIZE + 10];
    hdr.encode(&mut first_pkt[..HEADER_SIZE]);
    first_pkt[HEADER_SIZE..].copy_from_slice(&first_payload);
    raw_send(fd1, &first_pkt).expect("send first chunk");
    raw_send(fd1, &[0x01]).expect("send truncated continuation");
    end_stream_writes(fd1);

    let mut buf = [0u8; 64];
    let err = session
        .receive(&mut buf)
        .expect_err("continuation too short");
    assert!(matches!(err, UdsError::Chunk(_)));

    unsafe { libc::close(fd1) };
}

#[test]
fn test_receive_chunk_count_mismatch() {
    let (fd0, fd1) = socketpair_seqpacket();
    let mut session = test_session(fd0, Role::Server, (HEADER_SIZE + 10) as u32);

    let first_payload = [1u8; 10];
    let hdr = Header {
        magic: MAGIC_MSG,
        version: VERSION,
        header_len: protocol::HEADER_LEN,
        kind: KIND_REQUEST,
        code: 1,
        flags: 0,
        transport_status: protocol::STATUS_OK,
        payload_len: 20,
        item_count: 1,
        message_id: 44,
    };
    let mut first_pkt = [0u8; HEADER_SIZE + 10];
    hdr.encode(&mut first_pkt[..HEADER_SIZE]);
    first_pkt[HEADER_SIZE..].copy_from_slice(&first_payload);
    raw_send(fd1, &first_pkt).expect("send first chunk");

    let second_payload = [2u8; 10];
    let chk = ChunkHeader {
        magic: MAGIC_CHUNK,
        version: VERSION,
        flags: 0,
        message_id: 44,
        total_message_len: (HEADER_SIZE + 20) as u32,
        chunk_index: 1,
        chunk_count: 3,
        chunk_payload_len: second_payload.len() as u32,
    };
    let mut second_pkt = [0u8; HEADER_SIZE + 10];
    chk.encode(&mut second_pkt[..HEADER_SIZE]);
    second_pkt[HEADER_SIZE..].copy_from_slice(&second_payload);
    raw_send(fd1, &second_pkt).expect("send bad chunk count");

    let mut buf = [0u8; 64];
    let err = session.receive(&mut buf).expect_err("chunk_count mismatch");
    assert!(matches!(err, UdsError::Chunk(_)));

    unsafe { libc::close(fd1) };
}

#[test]
fn test_receive_more_chunks_than_declared() {
    let (fd0, fd1) = socketpair_seqpacket();
    let mut session = test_session(fd0, Role::Server, (HEADER_SIZE + 10) as u32);

    let hdr = Header {
        magic: MAGIC_MSG,
        version: VERSION,
        header_len: protocol::HEADER_LEN,
        kind: KIND_REQUEST,
        code: 1,
        flags: 0,
        transport_status: protocol::STATUS_OK,
        payload_len: 20,
        item_count: 1,
        message_id: 45,
    };
    let mut first_pkt = [0u8; HEADER_SIZE + 10];
    hdr.encode(&mut first_pkt[..HEADER_SIZE]);
    first_pkt[HEADER_SIZE..].copy_from_slice(&[1u8; 10]);
    raw_send(fd1, &first_pkt).expect("send first chunk");

    // Half-budget continuations need three chunks where two are declared.
    for ci in 1..=2u32 {
        let chk = ChunkHeader {
            magic: MAGIC_CHUNK,
            version: VERSION,
            flags: 0,
            message_id: 45,
            total_message_len: (HEADER_SIZE + 20) as u32,
            chunk_index: ci,
            chunk_count: 2,
            chunk_payload_len: 5,
        };
        let mut pkt = [0u8; HEADER_SIZE + 5];
        chk.encode(&mut pkt[..HEADER_SIZE]);
        pkt[HEADER_SIZE..].copy_from_slice(&[2u8; 5]);
        raw_send(fd1, &pkt).expect("send continuation");
    }

    let mut buf = [0u8; 64];
    let err = session
        .receive(&mut buf)
        .expect_err("chunk beyond chunk_count");
    assert!(matches!(err, UdsError::Chunk(_)), "{err:?}");

    unsafe { libc::close(fd1) };
}

#[test]
fn test_receive_chunk_exceeds_payload_len() {
    let (fd0, fd1) = socketpair_seqpacket();
    let mut session = test_session(fd0, Role::Server, (HEADER_SIZE + 10) as u32);

    let first_payload = [1u8; 10];
    let hdr = Header {
        magic: MAGIC_MSG,
        version: VERSION,
        header_len: protocol::HEADER_LEN,
        kind: KIND_REQUEST,
        code: 1,
        flags: 0,
        transport_status: protocol::STATUS_OK,
        payload_len: 15,
        item_count: 1,
        message_id: 45,
    };
    let mut first_pkt = [0u8; HEADER_SIZE + 10];
    hdr.encode(&mut first_pkt[..HEADER_SIZE]);
    first_pkt[HEADER_SIZE..].copy_from_slice(&first_payload);
    raw_send(fd1, &first_pkt).expect("send first chunk");

    let second_payload = [2u8; 10];
    let chk = ChunkHeader {
        magic: MAGIC_CHUNK,
        version: VERSION,
        flags: 0,
        message_id: 45,
        total_message_len: (HEADER_SIZE + 15) as u32,
        chunk_index: 1,
        chunk_count: 2,
        chunk_payload_len: second_payload.len() as u32,
    };
    let mut second_pkt = [0u8; HEADER_SIZE + 10];
    chk.encode(&mut second_pkt[..HEADER_SIZE]);
    second_pkt[HEADER_SIZE..].copy_from_slice(&second_payload);
    raw_send(fd1, &second_pkt).expect("send oversized continuation");

    let mut buf = [0u8; 64];
    let err = session
        .receive(&mut buf)
        .expect_err("chunk exceeds payload_len");
    assert!(matches!(err, UdsError::Chunk(_)));

    unsafe { libc::close(fd1) };
}

#[test]
fn test_bind_rejects_live_server_addr_in_use() {
    ensure_run_dir();
    let svc = unique_service("rs_live_bind");
    cleanup_socket(&svc);

    let listener =
        UdsListener::bind(TEST_RUN_DIR, &svc, default_server_config()).expect("first bind");
    let result = UdsListener::bind(TEST_RUN_DIR, &svc, default_server_config());
    assert!(matches!(result, Err(UdsError::AddrInUse)));

    drop(listener);
    cleanup_socket(&svc);
}

#[test]
fn test_bind_missing_run_dir_fails() {
    let svc = unique_service("rs_missing_bind");
    let bad_run_dir = "/tmp/nipc_rust_missing_parent/does/not/exist";
    let err = match UdsListener::bind(bad_run_dir, &svc, default_server_config()) {
        Ok(_) => panic!("missing parent bind should fail"),
        Err(err) => err,
    };
    assert!(matches!(err, UdsError::Socket(_)));
}

#[test]
fn test_profile_preference_selects_shm_futex() {
    ensure_run_dir();
    let svc = unique_service("rs_pref_prof");
    cleanup_socket(&svc);
    // Without POSIX SHM the platform limit leaves baseline on both sides.
    let want = if cfg!(any(target_os = "linux", target_os = "freebsd")) {
        protocol::PROFILE_SHM_FUTEX
    } else {
        PROFILE_BASELINE
    };

    let svc_clone = svc.clone();
    let scfg = ServerConfig {
        supported_profiles: PROFILE_BASELINE | protocol::PROFILE_SHM_FUTEX,
        preferred_profiles: protocol::PROFILE_SHM_FUTEX,
        ..default_server_config()
    };
    let listener = UdsListener::bind(TEST_RUN_DIR, &svc_clone, scfg).expect("listen");
    let server_thread = thread::spawn(move || {
        let session = listener.accept().expect("accept");
        assert_eq!(session.selected_profile, want);
    });

    let ccfg = ClientConfig {
        supported_profiles: PROFILE_BASELINE | protocol::PROFILE_SHM_FUTEX,
        preferred_profiles: protocol::PROFILE_SHM_FUTEX,
        ..default_client_config()
    };
    let session = UdsSession::connect(TEST_RUN_DIR, &svc, &ccfg).expect("connect");
    assert_eq!(session.selected_profile, want);

    drop(session);
    server_thread.join().expect("server join");
    cleanup_socket(&svc);
}

// An unassigned profile bit offered and preferred by both peers is never
// selected: they would otherwise run baseline under that label.
#[test]
fn test_unassigned_profile_bit_not_selected() {
    ensure_run_dir();
    let svc = unique_service("rs_unassigned_prof");
    cleanup_socket(&svc);
    const UNASSIGNED: u32 = 0x10;

    let svc_clone = svc.clone();
    let scfg = ServerConfig {
        supported_profiles: PROFILE_BASELINE | UNASSIGNED,
        preferred_profiles: UNASSIGNED,
        ..default_server_config()
    };
    let listener = UdsListener::bind(TEST_RUN_DIR, &svc_clone, scfg).expect("listen");
    let server_thread = thread::spawn(move || {
        let session = listener.accept().expect("accept");
        assert_eq!(session.selected_profile, PROFILE_BASELINE);
    });

    let ccfg = ClientConfig {
        supported_profiles: PROFILE_BASELINE | UNASSIGNED,
        preferred_profiles: UNASSIGNED,
        ..default_client_config()
    };
    let session = UdsSession::connect(TEST_RUN_DIR, &svc, &ccfg).expect("connect");
    assert_eq!(session.selected_profile, PROFILE_BASELINE);

    drop(session);
    server_thread.join().expect("server join");
    cleanup_socket(&svc);
}

// A deadline-bounded send of an untracked message with nothing in flight
// still has room for one response in its pending queue.
#[test]
fn test_bounded_control_send_idle() {
    ensure_run_dir();
    let svc = unique_service("rs_bounded_control");
    cleanup_socket(&svc);
    let listener = UdsListener::bind(TEST_RUN_DIR, &svc, default_server_config()).expect("listen");
    let (release_tx, release_rx) = std::sync::mpsc::channel::<()>();
    let server_thread = thread::spawn(move || {
        let session = listener.accept().expect("accept");
        let _ = release_rx.recv();
        drop(session);
    });

    let mut session =
        UdsSession::connect(TEST_RUN_DIR, &svc, &default_client_config()).expect("connect");
    let mut hdr = Header {
        kind: protocol::KIND_CONTROL,
        message_id: 1,
        ..Header::default()
    };
    session
        .send_with_control(&mut hdr, &[], 5000, None)
        .expect("bounded control send");

    drop(release_tx);
    drop(session);
    server_thread.join().expect("server join");
    cleanup_socket(&svc);
}

#[test]
fn test_zero_supported_profiles_defaults_to_baseline() {
    ensure_run_dir();
    let svc = unique_service("rs_default_profiles");
    cleanup_socket(&svc);

    let svc_clone = svc.clone();
    let scfg = ServerConfig {
        supported_profiles: 0,
        preferred_profiles: 0,
        ..default_server_config()
    };
    let listener = UdsListener::bind(TEST_RUN_DIR, &svc_clone, scfg).expect("listen");
    let server_thread = thread::spawn(move || {
        let session = listener.accept().expect("accept");
        assert_eq!(session.selected_profile, PROFILE_BASELINE);
    });

    let ccfg = ClientConfig {
        supported_profiles: 0,
        preferred_profiles: 0,
        ..default_client_config()
    };
    let session = UdsSession::connect(TEST_RUN_DIR, &svc, &ccfg).expect("connect");
    assert_eq!(session.selected_profile, PROFILE_BASELINE);

    drop(session);
    server_thread.join().expect("server join");
    cleanup_socket(&svc);
}

#[test]
fn test_bind_zero_backlog_uses_default() {
    ensure_run_dir();
    let svc = unique_service("rs_default_backlog");
    cleanup_socket(&svc);

    let listener = UdsListener::bind(
        TEST_RUN_DIR,
        &svc,
        ServerConfig {
            backlog: 0,
            ..default_server_config()
        },
    )
    .expect("bind with default backlog");

    drop(listener);
    cleanup_socket(&svc);
}

#[test]
fn test_connect_rejects_bad_hello_ack_kind() {
    ensure_run_dir();
    let svc = unique_service("rs_bad_ack_kind");
    cleanup_socket(&svc);

    let svc_clone = svc.clone();
    let fd = raw_listener_fd(&svc_clone);
    let server = thread::spawn(move || {
        let client_fd = unsafe { libc::accept(fd, std::ptr::null_mut(), std::ptr::null_mut()) };
        assert!(client_fd >= 0, "accept failed: {}", errno());

        let mut hello = [0u8; 128];
        let n = recv_hello(client_fd, &mut hello);
        assert!(n >= HEADER_SIZE + HELLO_PAYLOAD_SIZE);

        let ack = HelloAck {
            layout_version: 1,
            ..HelloAck::default()
        };
        let mut ack_buf = [0u8; HELLO_ACK_PAYLOAD_SIZE];
        ack.encode(&mut ack_buf);
        let hdr = Header {
            magic: MAGIC_MSG,
            version: VERSION,
            header_len: protocol::HEADER_LEN,
            kind: KIND_RESPONSE,
            flags: 0,
            code: protocol::CODE_HELLO_ACK,
            transport_status: protocol::STATUS_OK,
            payload_len: HELLO_ACK_PAYLOAD_SIZE as u32,
            item_count: 1,
            message_id: 0,
        };
        let mut pkt = [0u8; HEADER_SIZE + HELLO_ACK_PAYLOAD_SIZE];
        hdr.encode(&mut pkt[..HEADER_SIZE]);
        pkt[HEADER_SIZE..].copy_from_slice(&ack_buf);
        raw_send(client_fd, &pkt).expect("send malformed ack");

        unsafe {
            libc::close(client_fd);
            libc::close(fd);
        }
    });

    let err = match UdsSession::connect(TEST_RUN_DIR, &svc, &default_client_config()) {
        Ok(_) => panic!("bad hello ack kind should fail"),
        Err(err) => err,
    };
    assert!(matches!(err, UdsError::Protocol(_)));

    server.join().expect("server join");
    cleanup_socket(&svc);
}

#[test]
fn test_connect_rejects_selected_profile_not_offered() {
    ensure_run_dir();
    // The client offers baseline only.
    for (i, selected) in [
        protocol::PROFILE_SHM_HYBRID,
        0,
        PROFILE_BASELINE | protocol::PROFILE_SHM_HYBRID,
    ]
    .into_iter()
    .enumerate()
    {
        let svc = unique_service(&format!("rs_ack_selected_{i}"));
        cleanup_socket(&svc);

        let fd = raw_listener_fd(&svc);
        let server = thread::spawn(move || {
            let client_fd = unsafe { libc::accept(fd, std::ptr::null_mut(), std::ptr::null_mut()) };
            assert!(client_fd >= 0, "accept failed: {}", errno());

            let mut buf = [0u8; 128];
            let n = recv_hello(client_fd, &mut buf);
            let hello = Hello::decode(&buf[HEADER_SIZE..n]).expect("decode hello");

            let ack = HelloAck {
                layout_version: 1,
                server_supported_profiles: PROFILE_BASELINE | protocol::PROFILE_SHM_HYBRID,
                intersection_profiles: PROFILE_BASELINE,
                selected_profile: selected,
                agreed_max_request_payload_bytes: hello.max_request_payload_bytes,
                agreed_max_request_batch_items: hello.max_request_batch_items,
                agreed_max_response_payload_bytes: hello.max_response_payload_bytes,
                agreed_max_response_batch_items: hello.max_response_batch_items,
                agreed_packet_size: hello.packet_size,
                session_id: 1,
                ..HelloAck::default()
            };
            let mut ack_buf = [0u8; HELLO_ACK_PAYLOAD_SIZE];
            ack.encode(&mut ack_buf);
            let hdr = Header {
                magic: MAGIC_MSG,
                version: VERSION,
                header_len: protocol::HEADER_LEN,
                kind: protocol::KIND_CONTROL,
                flags: 0,
                code: protocol::CODE_HELLO_ACK,
                transport_status: protocol::STATUS_OK,
                payload_len: HELLO_ACK_PAYLOAD_SIZE as u32,
                item_count: 1,
                message_id: 0,
            };
            let mut pkt = [0u8; HEADER_SIZE + HELLO_ACK_PAYLOAD_SIZE];
            hdr.encode(&mut pkt[..HEADER_SIZE]);
            pkt[HEADER_SIZE..].copy_from_slice(&ack_buf);
            raw_send(client_fd, &pkt).expect("send ack");

            unsafe {
                libc::close(client_fd);
                libc::close(fd);
            }
        });

        let err = match UdsSession::connect(TEST_RUN_DIR, &svc, &default_client_config()) {
            Ok(_) => panic!("HELLO_ACK selecting 0x{selected:x} should fail"),
            Err(err) => err,
        };
        assert!(
            matches!(&err, UdsError::Protocol(msg) if msg.contains("not offered")),
            "selected 0x{selected:x}: {err:?}"
        );

        server.join().expect("server join");
        cleanup_socket(&svc);
    }
}

#[test]
fn test_connect_rejects_bad_hello_ack_status() {
    ensure_run_dir();
    let svc = unique_service("rs_bad_ack_status");
    cleanup_socket(&svc);

    let svc_clone = svc.clone();
    let fd = raw_listener_fd(&svc_clone);
    let server = thread::spawn(move || {
        let client_fd = unsafe { libc::accept(fd, std::ptr::null_mut(), std::ptr::null_mut()) };
        assert!(client_fd >= 0, "accept failed: {}", errno());

        let mut hello = [0u8; 128];
        let _ = recv_hello(client_fd, &mut hello);

        let ack = HelloAck {
            layout_version: 1,
            ..HelloAck::default()
        };
        let mut ack_buf = [0u8; HELLO_ACK_PAYLOAD_SIZE];
        ack.encode(&mut ack_buf);
        let hdr = Header {
            magic: MAGIC_MSG,
            version: VERSION,
            header_len: protocol::HEADER_LEN,
            kind: protocol::KIND_CONTROL,
            flags: 0,
            code: protocol::CODE_HELLO_ACK,
            transport_status: 0x7777,
            payload_len: HELLO_ACK_PAYLOAD_SIZE as u32,
            item_count: 1,
            message_id: 0,
        };
        let mut pkt = [0u8; HEADER_SIZE + HELLO_ACK_PAYLOAD_SIZE];
        hdr.encode(&mut pkt[..HEADER_SIZE]);
        pkt[HEADER_SIZE..].copy_from_slice(&ack_buf);
        raw_send(client_fd, &pkt).expect("send malformed ack");

        unsafe {
            libc::close(client_fd);
            libc::close(fd);
        }
    });

    let err = match UdsSession::connect(TEST_RUN_DIR, &svc, &default_client_config()) {
        Ok(_) => panic!("bad hello ack status should fail"),
        Err(err) => err,
    };
    assert!(matches!(err, UdsError::Handshake(_)));

    server.join().expect("server join");
    cleanup_socket(&svc);
}

#[test]
fn test_connect_rejects_bad_hello_ack_version_as_incompatible() {
    ensure_run_dir();
    let svc = unique_service("rs_bad_ack_version");
    cleanup_socket(&svc);

    let svc_clone = svc.clone();
    let fd = raw_listener_fd(&svc_clone);
    let server = thread::spawn(move || {
        let client_fd = unsafe { libc::accept(fd, std::ptr::null_mut(), std::ptr::null_mut()) };
        assert!(client_fd >= 0, "accept failed: {}", errno());

        let mut hello = [0u8; 128];
        let _ = recv_hello(client_fd, &mut hello);

        let ack = HelloAck {
            layout_version: 1,
            ..HelloAck::default()
        };
        let mut ack_buf = [0u8; HELLO_ACK_PAYLOAD_SIZE];
        ack.encode(&mut ack_buf);
        let hdr = Header {
            magic: MAGIC_MSG,
            version: VERSION + 1,
            header_len: protocol::HEADER_LEN,
            kind: protocol::KIND_CONTROL,
            flags: 0,
            code: protocol::CODE_HELLO_ACK,
            transport_status: protocol::STATUS_OK,
            payload_len: HELLO_ACK_PAYLOAD_SIZE as u32,
            item_count: 1,
            message_id: 0,
        };
        let mut pkt = [0u8; HEADER_SIZE + HELLO_ACK_PAYLOAD_SIZE];
        hdr.encode(&mut pkt[..HEADER_SIZE]);
        pkt[HEADER_SIZE..].copy_from_slice(&ack_buf);
        raw_send(client_fd, &pkt).expect("send bad-version ack");

        unsafe {
            libc::close(client_fd);
            libc::close(fd);
        }
    });

    let err = match UdsSession::connect(TEST_RUN_DIR, &svc, &default_client_config()) {
        Ok(_) => panic!("bad hello ack version should fail"),
        Err(err) => err,
    };
    assert!(matches!(err, UdsError::Incompatible(_)));

    server.join().expect("server join");
    cleanup_socket(&svc);
}

#[test]
fn test_connect_rejects_incompatible_hello_ack_status() {
    ensure_run_dir();
    let svc = unique_service("rs_ack_incompat_status");
    cleanup_socket(&svc);

    let svc_clone = svc.clone();
    let fd = raw_listener_fd(&svc_clone);
    let server = thread::spawn(move || {
        let client_fd = unsafe { libc::accept(fd, std::ptr::null_mut(), std::ptr::null_mut()) };
        assert!(client_fd >= 0, "accept failed: {}", errno());

        let mut hello = [0u8; 128];
        let _ = recv_hello(client_fd, &mut hello);

        let ack = HelloAck {
            layout_version: 1,
            ..HelloAck::default()
        };
        let mut ack_buf = [0u8; HELLO_ACK_PAYLOAD_SIZE];
        ack.encode(&mut ack_buf);
        let hdr = Header {
            magic: MAGIC_MSG,
            version: VERSION,
            header_len: protocol::HEADER_LEN,
            kind: protocol::KIND_CONTROL,
            flags: 0,
            code: protocol::CODE_HELLO_ACK,
            transport_status: protocol::STATUS_INCOMPATIBLE,
            payload_len: HELLO_ACK_PAYLOAD_SIZE as u32,
            item_count: 1,
            message_id: 0,
        };
        let mut pkt = [0u8; HEADER_SIZE + HELLO_ACK_PAYLOAD_SIZE];
        hdr.encode(&mut pkt[..HEADER_SIZE]);
        pkt[HEADER_SIZE..].copy_from_slice(&ack_buf);
        raw_send(client_fd, &pkt).expect("send incompatible-status ack");

        unsafe {
            libc::close(client_fd);
            libc::close(fd);
        }
    });

    let err = match UdsSession::connect(TEST_RUN_DIR, &svc, &default_client_config()) {
        Ok(_) => panic!("incompatible hello ack status should fail"),
        Err(err) => err,
    };
    assert!(matches!(err, UdsError::Incompatible(_)));

    server.join().expect("server join");
    cleanup_socket(&svc);
}

#[test]
fn test_connect_rejects_bad_hello_ack_layout_as_incompatible() {
    ensure_run_dir();
    let svc = unique_service("rs_bad_ack_layout");
    cleanup_socket(&svc);

    let svc_clone = svc.clone();
    let fd = raw_listener_fd(&svc_clone);
    let server = thread::spawn(move || {
        let client_fd = unsafe { libc::accept(fd, std::ptr::null_mut(), std::ptr::null_mut()) };
        assert!(client_fd >= 0, "accept failed: {}", errno());

        let mut hello = [0u8; 128];
        let _ = recv_hello(client_fd, &mut hello);

        let ack = HelloAck {
            layout_version: 2,
            ..HelloAck::default()
        };
        let mut ack_buf = [0u8; HELLO_ACK_PAYLOAD_SIZE];
        ack.encode(&mut ack_buf);
        let hdr = Header {
            magic: MAGIC_MSG,
            version: VERSION,
            header_len: protocol::HEADER_LEN,
            kind: protocol::KIND_CONTROL,
            flags: 0,
            code: protocol::CODE_HELLO_ACK,
            transport_status: protocol::STATUS_OK,
            payload_len: HELLO_ACK_PAYLOAD_SIZE as u32,
            item_count: 1,
            message_id: 0,
        };
        let mut pkt = [0u8; HEADER_SIZE + HELLO_ACK_PAYLOAD_SIZE];
        hdr.encode(&mut pkt[..HEADER_SIZE]);
        pkt[HEADER_SIZE..].copy_from_slice(&ack_buf);
        raw_send(client_fd, &pkt).expect("send bad-layout ack");

        unsafe {
            libc::close(client_fd);
            libc::close(fd);
        }
    });

    let err = match UdsSession::connect(TEST_RUN_DIR, &svc, &default_client_config()) {
        Ok(_) => panic!("bad hello ack layout should fail"),
        Err(err) => err,
    };
    assert!(matches!(err, UdsError::Incompatible(_)));

    server.join().expect("server join");
    cleanup_socket(&svc);
}

#[test]
fn test_connect_rejects_truncated_hello_ack() {
    ensure_run_dir();
    let svc = unique_service("rs_bad_ack_trunc");
    cleanup_socket(&svc);

    let svc_clone = svc.clone();
    let fd = raw_listener_fd(&svc_clone);
    let server = thread::spawn(move || {
        let client_fd = unsafe { libc::accept(fd, std::ptr::null_mut(), std::ptr::null_mut()) };
        assert!(client_fd >= 0, "accept failed: {}", errno());

        let mut hello = [0u8; 128];
        let _ = recv_hello(client_fd, &mut hello);

        let hdr = Header {
            magic: MAGIC_MSG,
            version: VERSION,
            header_len: protocol::HEADER_LEN,
            kind: protocol::KIND_CONTROL,
            flags: 0,
            code: protocol::CODE_HELLO_ACK,
            transport_status: protocol::STATUS_OK,
            payload_len: HELLO_ACK_PAYLOAD_SIZE as u32,
            item_count: 1,
            message_id: 0,
        };
        let mut pkt = [0u8; HEADER_SIZE];
        hdr.encode(&mut pkt);
        raw_send(client_fd, &pkt).expect("send truncated ack");

        unsafe {
            libc::close(client_fd);
            libc::close(fd);
        }
    });

    let err = match UdsSession::connect(TEST_RUN_DIR, &svc, &default_client_config()) {
        Ok(_) => panic!("truncated hello ack should fail"),
        Err(err) => err,
    };
    assert!(matches!(err, UdsError::Protocol(_)));

    server.join().expect("server join");
    cleanup_socket(&svc);
}

#[test]
fn test_accept_rejects_bad_hello_kind() {
    ensure_run_dir();
    let svc = unique_service("rs_bad_hello_kind");
    cleanup_socket(&svc);

    let listener =
        UdsListener::bind(TEST_RUN_DIR, &svc, default_server_config()).expect("bind listener");

    let svc_clone = svc.clone();
    let client = thread::spawn(move || {
        let path = build_socket_path(TEST_RUN_DIR, &svc_clone).expect("socket path");
        let fd = unsafe { libc::socket(libc::AF_UNIX, SOCK_TYPE, 0) };
        assert!(fd >= 0, "socket failed: {}", errno());
        connect_unix(fd, &path).expect("connect");

        let hdr = Header {
            magic: MAGIC_MSG,
            version: VERSION,
            header_len: protocol::HEADER_LEN,
            kind: KIND_REQUEST,
            flags: 0,
            code: protocol::CODE_HELLO,
            transport_status: protocol::STATUS_OK,
            payload_len: HELLO_PAYLOAD_SIZE as u32,
            item_count: 1,
            message_id: 0,
        };
        let mut payload = [0u8; HELLO_PAYLOAD_SIZE];
        let hello = Hello {
            layout_version: 1,
            supported_profiles: PROFILE_BASELINE,
            max_request_payload_bytes: 4096,
            max_request_batch_items: 16,
            max_response_payload_bytes: 4096,
            max_response_batch_items: 16,
            auth_token: AUTH_TOKEN,
            packet_size: DEFAULT_PACKET_SIZE_FALLBACK,
            ..Hello::default()
        };
        hello.encode(&mut payload);
        let mut pkt = [0u8; HEADER_SIZE + HELLO_PAYLOAD_SIZE];
        hdr.encode(&mut pkt[..HEADER_SIZE]);
        pkt[HEADER_SIZE..].copy_from_slice(&payload);
        raw_send(fd, &pkt).expect("send malformed hello");
        unsafe { libc::close(fd) };
    });

    let err = match listener.accept() {
        Ok(_) => panic!("bad hello kind should fail"),
        Err(err) => err,
    };
    assert!(matches!(err, UdsError::Protocol(_)));

    client.join().expect("client join");
    drop(listener);
    cleanup_socket(&svc);
}

#[test]
fn test_accept_rejects_bad_hello_version_as_incompatible() {
    ensure_run_dir();
    let svc = unique_service("rs_bad_hello_version");
    cleanup_socket(&svc);

    let listener =
        UdsListener::bind(TEST_RUN_DIR, &svc, default_server_config()).expect("bind listener");

    let svc_clone = svc.clone();
    let client = thread::spawn(move || {
        let path = build_socket_path(TEST_RUN_DIR, &svc_clone).expect("socket path");
        let fd = unsafe { libc::socket(libc::AF_UNIX, SOCK_TYPE, 0) };
        assert!(fd >= 0, "socket failed: {}", errno());
        connect_unix(fd, &path).expect("connect");

        let hdr = Header {
            magic: MAGIC_MSG,
            version: VERSION + 1,
            header_len: protocol::HEADER_LEN,
            kind: protocol::KIND_CONTROL,
            flags: 0,
            code: protocol::CODE_HELLO,
            transport_status: protocol::STATUS_OK,
            payload_len: HELLO_PAYLOAD_SIZE as u32,
            item_count: 1,
            message_id: 0,
        };
        let mut payload = [0u8; HELLO_PAYLOAD_SIZE];
        let hello = Hello {
            layout_version: 1,
            supported_profiles: PROFILE_BASELINE,
            max_request_payload_bytes: 4096,
            max_request_batch_items: 16,
            max_response_payload_bytes: 4096,
            max_response_batch_items: 16,
            auth_token: AUTH_TOKEN,
            packet_size: DEFAULT_PACKET_SIZE_FALLBACK,
            ..Hello::default()
        };
        hello.encode(&mut payload);
        let mut pkt = [0u8; HEADER_SIZE + HELLO_PAYLOAD_SIZE];
        hdr.encode(&mut pkt[..HEADER_SIZE]);
        pkt[HEADER_SIZE..].copy_from_slice(&payload);
        raw_send(fd, &pkt).expect("send incompatible hello");
        unsafe { libc::close(fd) };
    });

    let err = match listener.accept() {
        Ok(_) => panic!("bad hello version should fail"),
        Err(err) => err,
    };
    assert!(matches!(err, UdsError::Incompatible(_)));

    client.join().expect("client join");
    drop(listener);
    cleanup_socket(&svc);
}

#[test]
fn test_accept_rejects_bad_hello_layout_as_incompatible() {
    ensure_run_dir();
    let svc = unique_service("rs_bad_hello_layout");
    cleanup_socket(&svc);

    let listener =
        UdsListener::bind(TEST_RUN_DIR, &svc, default_server_config()).expect("bind listener");

    let svc_clone = svc.clone();
    let client = thread::spawn(move || {
        let path = build_socket_path(TEST_RUN_DIR, &svc_clone).expect("socket path");
        let fd = unsafe { libc::socket(libc::AF_UNIX, SOCK_TYPE, 0) };
        assert!(fd >= 0, "socket failed: {}", errno());
        connect_unix(fd, &path).expect("connect");

        let hdr = Header {
            magic: MAGIC_MSG,
            version: VERSION,
            header_len: protocol::HEADER_LEN,
            kind: protocol::KIND_CONTROL,
            flags: 0,
            code: protocol::CODE_HELLO,
            transport_status: protocol::STATUS_OK,
            payload_len: HELLO_PAYLOAD_SIZE as u32,
            item_count: 1,
            message_id: 0,
        };
        let mut payload = [0u8; HELLO_PAYLOAD_SIZE];
        let hello = Hello {
            layout_version: 2,
            supported_profiles: PROFILE_BASELINE,
            max_request_payload_bytes: 4096,
            max_request_batch_items: 16,
            max_response_payload_bytes: 4096,
            max_response_batch_items: 16,
            auth_token: AUTH_TOKEN,
            packet_size: DEFAULT_PACKET_SIZE_FALLBACK,
            ..Hello::default()
        };
        hello.encode(&mut payload);
        let mut pkt = [0u8; HEADER_SIZE + HELLO_PAYLOAD_SIZE];
        hdr.encode(&mut pkt[..HEADER_SIZE]);
        pkt[HEADER_SIZE..].copy_from_slice(&payload);
        raw_send(fd, &pkt).expect("send incompatible-layout hello");
        unsafe { libc::close(fd) };
    });

    let err = match listener.accept() {
        Ok(_) => panic!("bad hello layout should fail"),
        Err(err) => err,
    };
    assert!(matches!(err, UdsError::Incompatible(_)));

    client.join().expect("client join");
    drop(listener);
    cleanup_socket(&svc);
}

// -----------------------------------------------------------------------
//  Receive: unknown response message_id (line 370)
// -----------------------------------------------------------------------

#[test]
fn test_unknown_response_msg_id() {
    ensure_run_dir();
    let svc = unique_service("rs_unkmsg");
    cleanup_socket(&svc);

    let svc_clone = svc.clone();

    let listener =
        UdsListener::bind(TEST_RUN_DIR, &svc_clone, default_server_config()).expect("listen");
    let server_thread = thread::spawn(move || {
        let mut session = listener.accept().expect("accept");
        let mut buf = [0u8; 8192];
        let (hdr, payload) = session.receive(&mut buf).expect("recv");
        let payload = payload.to_vec();

        // Respond with a different message_id
        let mut resp = Header {
            kind: protocol::KIND_RESPONSE,
            code: hdr.code,
            message_id: hdr.message_id + 999, // wrong message_id
            item_count: 1,
            transport_status: protocol::STATUS_OK,
            ..Header::default()
        };
        session.send(&mut resp, &payload).expect("send");
    });

    let mut session =
        UdsSession::connect(TEST_RUN_DIR, &svc, &default_client_config()).expect("connect");

    let mut hdr = Header {
        kind: protocol::KIND_REQUEST,
        code: 1,
        item_count: 1,
        message_id: 50,
        ..Header::default()
    };
    session.send(&mut hdr, &[1]).expect("send");

    let mut rbuf = [0u8; 8192];
    let result = session.receive(&mut rbuf);
    assert!(matches!(result, Err(UdsError::UnknownMsgId(_))));

    drop(session);
    server_thread.join().expect("server join");
    cleanup_socket(&svc);
}

// -----------------------------------------------------------------------
//  Path length validation
// -----------------------------------------------------------------------

#[test]
fn test_path_too_long() {
    let long_name = "a".repeat(200);
    let result = build_socket_path("/tmp", &long_name);
    assert!(matches!(result, Err(UdsError::PathTooLong)));
}

#[cfg(not(any(target_os = "macos", target_os = "freebsd")))]
#[test]
fn test_packet_size_limit_invalid_fd_uses_fallback() {
    assert_eq!(packet_size_limit(-1, 0), DEFAULT_PACKET_SIZE_FALLBACK);
    assert_eq!(packet_size_limit(-1, 1000), 1000);
    assert_eq!(
        packet_size_limit(-1, DEFAULT_PACKET_SIZE_FALLBACK + 1),
        DEFAULT_PACKET_SIZE_FALLBACK
    );
}

#[cfg(not(any(target_os = "macos", target_os = "freebsd")))]
fn set_sndbuf(fd: RawFd, bytes: libc::c_int) {
    let rc = unsafe {
        libc::setsockopt(
            fd,
            libc::SOL_SOCKET,
            libc::SO_SNDBUF,
            &bytes as *const _ as *const libc::c_void,
            std::mem::size_of::<libc::c_int>() as libc::socklen_t,
        )
    };
    assert_eq!(rc, 0, "setsockopt(SO_SNDBUF) failed: {}", errno());
}

#[cfg(any(target_os = "macos", target_os = "freebsd"))]
#[test]
fn test_packet_size_limit_is_fixed_on_a_stream() {
    assert_eq!(packet_size_limit(-1, 0), PACKET_SIZE_MAX);
    assert_eq!(packet_size_limit(-1, 1000), 1000);
    assert_eq!(packet_size_limit(-1, u32::MAX), PACKET_SIZE_MAX);

    // A full packet crosses even though it is far above SO_SNDBUF.
    let (fd0, fd1) = socketpair_seqpacket();
    let mut pkt = vec![0xA5u8; PACKET_SIZE_MAX as usize];
    let hdr = Header {
        magic: MAGIC_MSG,
        version: VERSION,
        header_len: protocol::HEADER_LEN,
        kind: KIND_REQUEST,
        item_count: 1,
        payload_len: PACKET_SIZE_MAX - HEADER_SIZE as u32,
        ..Header::default()
    };
    hdr.encode(&mut pkt[..HEADER_SIZE]);
    let reader = thread::spawn(move || {
        let mut buf = vec![0u8; PACKET_SIZE_MAX as usize];
        let n = stream::recv_packet(fd1, &mut buf, PACKET_SIZE_MAX, None).expect("recv");
        unsafe { libc::close(fd1) };
        (n, buf[n - 1])
    });
    send_packet(fd0, &pkt[..HEADER_SIZE], &pkt[HEADER_SIZE..]).expect("send");
    assert_eq!(reader.join().unwrap(), (PACKET_SIZE_MAX as usize, 0xA5));
    unsafe { libc::close(fd0) };
}

#[cfg(not(any(target_os = "macos", target_os = "freebsd")))]
#[test]
fn test_packet_size_limit_matches_kernel_datagram_limit() {
    // None keeps the system default SO_SNDBUF.
    for sndbuf in [None, Some(4096), Some(65536)] {
        let (fd0, fd1) = socketpair_seqpacket();
        if let Some(bytes) = sndbuf {
            set_sndbuf(fd0, bytes);
        }
        let limit = packet_size_limit(fd0, 0);
        assert!(limit > HEADER_SIZE as u32 && limit <= PACKET_SIZE_MAX);
        assert_eq!(packet_size_limit(fd0, u32::MAX), limit, "{sndbuf:?}");

        let pkt = vec![0u8; limit as usize];
        raw_send(fd0, &pkt).unwrap_or_else(|e| panic!("{sndbuf:?}: {limit} bytes: {e:?}"));
        let mut buf = vec![0u8; limit as usize];
        assert_eq!(raw_recv(fd1, &mut buf).expect("recv"), limit as usize);

        if limit < PACKET_SIZE_MAX {
            // Guards the SO_SNDBUF - 32 rule: one more byte must not fit.
            let over = vec![0u8; limit as usize + 1];
            assert!(raw_send(fd0, &over).is_err(), "{sndbuf:?}: {limit} + 1");
        }
        unsafe {
            libc::close(fd0);
            libc::close(fd1);
        }
    }
}

#[test]
fn test_connect_rejects_agreed_packet_size_above_proposal() {
    ensure_run_dir();
    let svc = unique_service("rs_ack_pkt_above");
    cleanup_socket(&svc);

    let svc_clone = svc.clone();
    let fd = raw_listener_fd(&svc_clone);
    let server = thread::spawn(move || {
        let client_fd = unsafe { libc::accept(fd, std::ptr::null_mut(), std::ptr::null_mut()) };
        assert!(client_fd >= 0, "accept failed: {}", errno());

        let mut hello_buf = [0u8; 128];
        let n = recv_hello(client_fd, &mut hello_buf);
        let hello = Hello::decode(&hello_buf[HEADER_SIZE..n]).expect("decode hello");

        let ack = HelloAck {
            layout_version: 1,
            server_supported_profiles: PROFILE_BASELINE,
            intersection_profiles: PROFILE_BASELINE,
            selected_profile: PROFILE_BASELINE,
            agreed_max_request_payload_bytes: 4096,
            agreed_max_request_batch_items: 16,
            agreed_max_response_payload_bytes: 4096,
            agreed_max_response_batch_items: 16,
            agreed_packet_size: hello.packet_size + 1,
            session_id: 1,
            ..HelloAck::default()
        };
        let mut ack_buf = [0u8; HELLO_ACK_PAYLOAD_SIZE];
        ack.encode(&mut ack_buf);
        let hdr = Header {
            magic: MAGIC_MSG,
            version: VERSION,
            header_len: protocol::HEADER_LEN,
            kind: protocol::KIND_CONTROL,
            flags: 0,
            code: protocol::CODE_HELLO_ACK,
            transport_status: protocol::STATUS_OK,
            payload_len: HELLO_ACK_PAYLOAD_SIZE as u32,
            item_count: 1,
            message_id: 0,
        };
        let mut pkt = [0u8; HEADER_SIZE + HELLO_ACK_PAYLOAD_SIZE];
        hdr.encode(&mut pkt[..HEADER_SIZE]);
        pkt[HEADER_SIZE..].copy_from_slice(&ack_buf);
        raw_send(client_fd, &pkt).expect("send ack");

        unsafe {
            libc::close(client_fd);
            libc::close(fd);
        }
    });

    let err = match UdsSession::connect(TEST_RUN_DIR, &svc, &default_client_config()) {
        Ok(_) => panic!("agreed packet_size above the proposal must fail"),
        Err(err) => err,
    };
    assert!(matches!(err, UdsError::Protocol(_)), "got {err:?}");

    server.join().expect("server join");
    cleanup_socket(&svc);
}

#[test]
fn test_raw_send_closed_peer_returns_send_error() {
    let (fd0, fd1) = socketpair_seqpacket();
    unsafe {
        libc::close(fd1);
    }

    let err = raw_send(fd0, &[1, 2, 3]).expect_err("closed peer should fail send");
    assert!(matches!(err, UdsError::Send(_)));

    unsafe {
        libc::close(fd0);
    }
}

// Sockets must not leak into children a consumer forks and execs: an
// inherited session socket keeps the peer from ever seeing EOF, which is the
// dead-peer signal.
#[test]
fn test_listener_and_session_sockets_are_cloexec() {
    ensure_run_dir();
    let svc = unique_service("rs_cloexec");
    cleanup_socket(&svc);

    let cloexec = |fd: RawFd| unsafe { libc::fcntl(fd, libc::F_GETFD) } & libc::FD_CLOEXEC != 0;
    let listener = UdsListener::bind(TEST_RUN_DIR, &svc, default_server_config()).expect("listen");
    let listener_fd = listener.fd();
    let server_thread = thread::spawn(move || {
        let session = listener.accept().expect("accept");
        (cloexec(listener.fd()), cloexec(session.fd()))
    });

    let session =
        UdsSession::connect(TEST_RUN_DIR, &svc, &default_client_config()).expect("connect");
    let (listener_ok, accepted_ok) = server_thread.join().expect("server thread");
    assert!(listener_fd >= 0);
    assert!(listener_ok, "listener socket must be close-on-exec");
    assert!(accepted_ok, "accepted session socket must be close-on-exec");
    assert!(
        cloexec(session.fd()),
        "client session socket must be close-on-exec"
    );
    cleanup_socket(&svc);
}

// -----------------------------------------------------------------------
//  Stream baseline (macOS, FreeBSD)
// -----------------------------------------------------------------------

/// A receive that consumed part of a message cannot leave the session
/// usable: the rest is still queued and would be read as a header.
#[cfg(any(target_os = "macos", target_os = "freebsd"))]
#[test]
fn test_stream_partial_receive_ends_reads() {
    let (fd0, fd1) = socketpair_seqpacket();
    let mut session = test_session(fd0, Role::Server, 1024);
    let mut buf = [0u8; 2048];

    assert_eq!(
        session.receive_with_control(&mut buf, 1, None).err(),
        Some(UdsError::Timeout),
        "timeout before the first byte"
    );

    let hdr = Header {
        magic: MAGIC_MSG,
        version: VERSION,
        header_len: protocol::HEADER_LEN,
        kind: KIND_REQUEST,
        code: 1,
        item_count: 1,
        message_id: 9,
        payload_len: 5,
        ..Header::default()
    };
    let mut pkt = [0u8; HEADER_SIZE + 5];
    hdr.encode(&mut pkt[..HEADER_SIZE]);
    pkt[HEADER_SIZE..].copy_from_slice(b"hello");
    send_packet(fd1, &pkt, &[]).unwrap();
    let (_, payload) = session
        .receive(&mut buf)
        .expect("intact after a clean timeout");
    assert_eq!(payload, b"hello");

    send_packet(fd1, &pkt[..10], &[]).unwrap();
    assert_eq!(
        session.receive_with_control(&mut buf, 1, None).err(),
        Some(UdsError::Timeout),
        "timeout inside a message"
    );
    send_packet(fd1, &pkt[10..], &[]).unwrap();
    assert_eq!(
        session.receive(&mut buf).err(),
        Some(UdsError::Recv(0)),
        "later receives fail"
    );

    // Sends always write whole messages, so they stay valid.
    let mut resp = Header {
        kind: protocol::KIND_RESPONSE,
        code: 1,
        item_count: 1,
        message_id: 9,
        ..Header::default()
    };
    session.send(&mut resp, b"ok").expect("sends still work");
    let n = stream::recv_packet(fd1, &mut buf, 1024, None).expect("peer gets the response");
    assert_eq!(n, HEADER_SIZE + 2);
    unsafe { libc::close(fd1) };
}

/// macOS binds in `{service}.d/` and renames the socket into place.
#[cfg(target_os = "macos")]
#[test]
fn test_listen_bind_dir() {
    use std::os::unix::fs::PermissionsExt;

    ensure_run_dir();
    let svc = unique_service("rs_bind_dir");
    cleanup_socket(&svc);
    let dir = format!("{TEST_RUN_DIR}/{svc}.d");
    let path = build_socket_path(TEST_RUN_DIR, &svc).unwrap();
    let listen = || UdsListener::bind(TEST_RUN_DIR, &svc, default_server_config());
    let gone = |p: &str| std::fs::symlink_metadata(p).is_err();

    let listener = listen().expect("listen");
    let mode = std::fs::metadata(&path).unwrap().permissions().mode() & 0o777;
    assert_eq!(mode, 0o600, "socket is owner-only");
    assert!(gone(&dir), "bind directory removed");
    drop(listener);

    // A crashed server's leftover directory is reclaimed.
    std::fs::create_dir(&dir).unwrap();
    std::fs::write(format!("{dir}/s"), b"").unwrap();
    drop(listen().expect("listen over a leftover directory"));
    assert!(gone(&dir));

    // A non-directory at the name is junk, as at the socket name.
    std::fs::write(&dir, b"").unwrap();
    drop(listen().expect("listen over a leftover file"));
    assert!(gone(&dir));

    // Foreign content is never deleted: listen fails instead.
    std::fs::create_dir(&dir).unwrap();
    let other = format!("{dir}/other");
    std::fs::write(&other, b"").unwrap();
    assert!(matches!(listen(), Err(UdsError::Socket(_))));
    assert!(std::fs::metadata(&other).is_ok(), "foreign content kept");
    std::fs::remove_file(&other).unwrap();

    // A directory where the socket goes is foreign content too.
    let sub = format!("{dir}/s");
    std::fs::create_dir(&sub).unwrap();
    assert!(matches!(listen(), Err(UdsError::Socket(_))));
    assert!(
        std::fs::metadata(&sub).unwrap().is_dir(),
        "foreign subdirectory kept"
    );
    std::fs::remove_dir(&sub).unwrap();
    std::fs::remove_dir(&dir).unwrap();
    cleanup_socket(&svc);
}
