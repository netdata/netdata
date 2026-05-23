use super::*;
use crate::protocol;
use std::sync::atomic::{AtomicBool, AtomicU32, Ordering};
use std::sync::Arc;
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

/// Unique service name to avoid parallel test collisions.
static TEST_COUNTER: AtomicU32 = AtomicU32::new(0);
fn unique_service(prefix: &str) -> String {
    let n = TEST_COUNTER.fetch_add(1, Ordering::Relaxed);
    format!("{prefix}_{n}_{}", std::process::id())
}

fn socketpair_seqpacket() -> (RawFd, RawFd) {
    let mut fds = [-1; 2];
    let rc = unsafe { libc::socketpair(libc::AF_UNIX, libc::SOCK_SEQPACKET, 0, fds.as_mut_ptr()) };
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
    }
}

fn raw_listener_fd(service: &str) -> RawFd {
    let path = build_socket_path(TEST_RUN_DIR, service).expect("socket path");
    let fd = unsafe { libc::socket(libc::AF_UNIX, libc::SOCK_SEQPACKET, 0) };
    assert!(fd >= 0, "socket failed: {}", errno());
    bind_unix(fd, &path).expect("bind raw listener");
    let rc = unsafe { libc::listen(fd, DEFAULT_BACKLOG) };
    assert_eq!(rc, 0, "listen failed: {}", errno());
    fd
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
    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();

    let server_thread = thread::spawn(move || {
        let listener =
            UdsListener::bind(TEST_RUN_DIR, &svc_clone, default_server_config()).expect("listen");
        ready_clone.store(true, Ordering::Release);

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
    while !ready.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(1));
    }

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
    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();

    let server_thread = thread::spawn(move || {
        let listener =
            UdsListener::bind(TEST_RUN_DIR, &svc_clone, default_server_config()).expect("listen");
        ready_clone.store(true, Ordering::Release);

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

    while !ready.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(1));
    }

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
    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();

    let server_thread = thread::spawn(move || {
        let listener =
            UdsListener::bind(TEST_RUN_DIR, &svc_clone, default_server_config()).expect("listen");
        ready_clone.store(true, Ordering::Release);

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

    while !ready.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(1));
    }

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
    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();

    let server_thread = thread::spawn(move || {
        let scfg = ServerConfig {
            packet_size: 128,
            max_request_payload_bytes: 65536,
            max_response_payload_bytes: 65536,
            ..default_server_config()
        };
        let listener = UdsListener::bind(TEST_RUN_DIR, &svc_clone, scfg).expect("listen");
        ready_clone.store(true, Ordering::Release);

        let mut session = listener.accept().expect("accept");

        let mut buf = [0u8; 256]; // small buf, forces recv_buf usage
        let (hdr, payload) = session.receive(&mut buf).expect("recv");
        let payload = payload.to_vec();

        let mut resp = hdr;
        resp.kind = protocol::KIND_RESPONSE;
        session.send(&mut resp, &payload).expect("send");
    });

    while !ready.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(1));
    }

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
    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();

    let server_thread = thread::spawn(move || {
        let listener =
            UdsListener::bind(TEST_RUN_DIR, &svc_clone, default_server_config()).expect("listen");
        ready_clone.store(true, Ordering::Release);
        // accept will fail due to auth mismatch
        let _ = listener.accept();
    });

    while !ready.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(1));
    }

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
    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();

    let server_thread = thread::spawn(move || {
        let scfg = ServerConfig {
            supported_profiles: protocol::PROFILE_SHM_FUTEX,
            ..default_server_config()
        };
        let listener = UdsListener::bind(TEST_RUN_DIR, &svc_clone, scfg).expect("listen");
        ready_clone.store(true, Ordering::Release);
        let _ = listener.accept();
    });

    while !ready.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(1));
    }

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
    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();

    let server_thread = thread::spawn(move || {
        let listener =
            UdsListener::bind(TEST_RUN_DIR, &svc_clone, default_server_config()).expect("listen");
        ready_clone.store(true, Ordering::Release);
        let result = listener.accept();
        assert!(matches!(result, Err(UdsError::LimitExceeded)));
    });

    while !ready.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(1));
    }

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

#[test]
fn test_stale_recovery() {
    ensure_run_dir();
    let svc = unique_service("rs_stale");
    cleanup_socket(&svc);

    let path = format!("{TEST_RUN_DIR}/{svc}.sock");

    // Create a stale socket file (bound but not listening)
    unsafe {
        let sock = libc::socket(libc::AF_UNIX, libc::SOCK_SEQPACKET, 0);
        assert!(sock >= 0);

        let c_path = CString::new(path.as_str()).unwrap();
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

    assert!(Path::new(&path).exists(), "stale socket should exist");

    // listen should recover it
    let listener = UdsListener::bind(TEST_RUN_DIR, &svc, default_server_config())
        .expect("listen should recover stale socket");
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
    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();

    let server_thread = thread::spawn(move || {
        let listener =
            UdsListener::bind(TEST_RUN_DIR, &svc_clone, default_server_config()).expect("listen");
        ready_clone.store(true, Ordering::Release);

        let mut session = listener.accept().expect("accept");
        // Read request then close without responding
        let mut buf = [0u8; 4096];
        let _ = session.receive(&mut buf);
        drop(session); // close socket
    });

    while !ready.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(1));
    }

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
    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();

    let server_thread = thread::spawn(move || {
        let listener =
            UdsListener::bind(TEST_RUN_DIR, &svc_clone, default_server_config()).expect("listen");
        ready_clone.store(true, Ordering::Release);

        let mut session = listener.accept().expect("accept");
        let mut buf = [0u8; 8192];
        let (hdr, payload) = session.receive(&mut buf).expect("recv");
        let payload = payload.to_vec();
        let mut resp = hdr;
        resp.kind = protocol::KIND_RESPONSE;
        resp.transport_status = protocol::STATUS_OK;
        session.send(&mut resp, &payload).expect("send");
    });

    while !ready.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(1));
    }

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
    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();

    let server_thread = thread::spawn(move || {
        let scfg = ServerConfig {
            packet_size: 128,
            max_request_payload_bytes: 65536,
            max_response_payload_bytes: 65536,
            ..default_server_config()
        };
        let listener = UdsListener::bind(TEST_RUN_DIR, &svc_clone, scfg).expect("listen");
        ready_clone.store(true, Ordering::Release);

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

    while !ready.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(1));
    }

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
    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();

    let server_thread = thread::spawn(move || {
        let listener =
            UdsListener::bind(TEST_RUN_DIR, &svc_clone, default_server_config()).expect("listen");
        ready_clone.store(true, Ordering::Release);

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

    while !ready.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(1));
    }

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
    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();

    let server_thread = thread::spawn(move || {
        let scfg = ServerConfig {
            max_request_payload_bytes: 65536,
            max_response_payload_bytes: 65536,
            ..default_server_config()
        };
        let listener = UdsListener::bind(TEST_RUN_DIR, &svc_clone, scfg).expect("listen");
        ready_clone.store(true, Ordering::Release);

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

    while !ready.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(1));
    }

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
    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();

    let sizes = [8usize, 256, 1024, 8, 256, 1024, 8, 256, 1024];
    let count = sizes.len();

    let server_thread = thread::spawn(move || {
        let scfg = ServerConfig {
            max_request_payload_bytes: 65536,
            max_response_payload_bytes: 65536,
            ..default_server_config()
        };
        let listener = UdsListener::bind(TEST_RUN_DIR, &svc_clone, scfg).expect("listen");
        ready_clone.store(true, Ordering::Release);

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

    while !ready.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(1));
    }

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
    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();

    let sizes = [200usize, 500, 300, 800, 150];
    let count = sizes.len();

    let server_thread = thread::spawn(move || {
        let scfg = ServerConfig {
            packet_size: 128,
            max_request_payload_bytes: 65536,
            max_response_payload_bytes: 65536,
            ..default_server_config()
        };
        let listener = UdsListener::bind(TEST_RUN_DIR, &svc_clone, scfg).expect("listen");
        ready_clone.store(true, Ordering::Release);

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

    while !ready.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(1));
    }

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
    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();

    let server_thread = thread::spawn(move || {
        let listener =
            UdsListener::bind(TEST_RUN_DIR, &svc_clone, default_server_config()).expect("listen");
        ready_clone.store(true, Ordering::Release);
        let session = listener.accept().expect("accept");
        assert_eq!(session.role(), Role::Server);
    });

    while !ready.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(1));
    }

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
    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();

    let server_thread = thread::spawn(move || {
        let listener =
            UdsListener::bind(TEST_RUN_DIR, &svc_clone, default_server_config()).expect("listen");
        ready_clone.store(true, Ordering::Release);
        let _session = listener.accept().expect("accept");
    });

    while !ready.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(1));
    }

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
    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();

    let server_thread = thread::spawn(move || {
        let listener =
            UdsListener::bind(TEST_RUN_DIR, &svc_clone, default_server_config()).expect("listen");
        ready_clone.store(true, Ordering::Release);
        let _session = listener.accept().expect("accept");
        // Hold connection open while client tests
        thread::sleep(Duration::from_millis(500));
    });

    while !ready.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(1));
    }

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

#[test]
fn test_directional_limit_negotiation() {
    ensure_run_dir();
    let svc = unique_service("rs_dir_limits");
    cleanup_socket(&svc);

    let svc_clone = svc.clone();
    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();

    let server_thread = thread::spawn(move || {
        let scfg = ServerConfig {
            max_request_payload_bytes: 2048,
            max_request_batch_items: 8,
            max_response_payload_bytes: 8192,
            max_response_batch_items: 32,
            ..default_server_config()
        };
        let listener = UdsListener::bind(TEST_RUN_DIR, &svc_clone, scfg).expect("listen");
        ready_clone.store(true, Ordering::Release);

        let session = listener.accept().expect("accept");
        assert_eq!(session.max_request_payload_bytes, 4096);
        assert_eq!(session.max_request_batch_items, 16);
        assert_eq!(session.max_response_payload_bytes, 8192);
        assert_eq!(session.max_response_batch_items, 16);
        assert_ne!(session.session_id, 0);
    });

    while !ready.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(1));
    }

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

    let mut buf = [0u8; 64];
    let err = session.receive(&mut buf).expect_err("packet too short");
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
    let mut session = test_session(fd0, Role::Server, (HEADER_SIZE + 8) as u32);

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

    let svc_clone = svc.clone();
    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();

    let server_thread = thread::spawn(move || {
        let scfg = ServerConfig {
            supported_profiles: PROFILE_BASELINE | protocol::PROFILE_SHM_FUTEX,
            preferred_profiles: protocol::PROFILE_SHM_FUTEX,
            ..default_server_config()
        };
        let listener = UdsListener::bind(TEST_RUN_DIR, &svc_clone, scfg).expect("listen");
        ready_clone.store(true, Ordering::Release);
        let session = listener.accept().expect("accept");
        assert_eq!(session.selected_profile, protocol::PROFILE_SHM_FUTEX);
    });

    while !ready.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(1));
    }

    let ccfg = ClientConfig {
        supported_profiles: PROFILE_BASELINE | protocol::PROFILE_SHM_FUTEX,
        preferred_profiles: protocol::PROFILE_SHM_FUTEX,
        ..default_client_config()
    };
    let session = UdsSession::connect(TEST_RUN_DIR, &svc, &ccfg).expect("connect");
    assert_eq!(session.selected_profile, protocol::PROFILE_SHM_FUTEX);

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
    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();

    let server_thread = thread::spawn(move || {
        let scfg = ServerConfig {
            supported_profiles: 0,
            preferred_profiles: 0,
            ..default_server_config()
        };
        let listener = UdsListener::bind(TEST_RUN_DIR, &svc_clone, scfg).expect("listen");
        ready_clone.store(true, Ordering::Release);
        let session = listener.accept().expect("accept");
        assert_eq!(session.selected_profile, PROFILE_BASELINE);
    });

    while !ready.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(1));
    }

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

    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();
    let svc_clone = svc.clone();
    let server = thread::spawn(move || {
        let fd = raw_listener_fd(&svc_clone);
        ready_clone.store(true, Ordering::Release);
        let client_fd = unsafe { libc::accept(fd, std::ptr::null_mut(), std::ptr::null_mut()) };
        assert!(client_fd >= 0, "accept failed: {}", errno());

        let mut hello = [0u8; 128];
        let n = raw_recv(client_fd, &mut hello).expect("recv hello");
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

    while !ready.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(1));
    }

    let err = match UdsSession::connect(TEST_RUN_DIR, &svc, &default_client_config()) {
        Ok(_) => panic!("bad hello ack kind should fail"),
        Err(err) => err,
    };
    assert!(matches!(err, UdsError::Protocol(_)));

    server.join().expect("server join");
    cleanup_socket(&svc);
}

#[test]
fn test_connect_rejects_bad_hello_ack_status() {
    ensure_run_dir();
    let svc = unique_service("rs_bad_ack_status");
    cleanup_socket(&svc);

    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();
    let svc_clone = svc.clone();
    let server = thread::spawn(move || {
        let fd = raw_listener_fd(&svc_clone);
        ready_clone.store(true, Ordering::Release);
        let client_fd = unsafe { libc::accept(fd, std::ptr::null_mut(), std::ptr::null_mut()) };
        assert!(client_fd >= 0, "accept failed: {}", errno());

        let mut hello = [0u8; 128];
        let _ = raw_recv(client_fd, &mut hello).expect("recv hello");

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

    while !ready.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(1));
    }

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

    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();
    let svc_clone = svc.clone();
    let server = thread::spawn(move || {
        let fd = raw_listener_fd(&svc_clone);
        ready_clone.store(true, Ordering::Release);
        let client_fd = unsafe { libc::accept(fd, std::ptr::null_mut(), std::ptr::null_mut()) };
        assert!(client_fd >= 0, "accept failed: {}", errno());

        let mut hello = [0u8; 128];
        let _ = raw_recv(client_fd, &mut hello).expect("recv hello");

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

    while !ready.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(1));
    }

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

    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();
    let svc_clone = svc.clone();
    let server = thread::spawn(move || {
        let fd = raw_listener_fd(&svc_clone);
        ready_clone.store(true, Ordering::Release);
        let client_fd = unsafe { libc::accept(fd, std::ptr::null_mut(), std::ptr::null_mut()) };
        assert!(client_fd >= 0, "accept failed: {}", errno());

        let mut hello = [0u8; 128];
        let _ = raw_recv(client_fd, &mut hello).expect("recv hello");

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

    while !ready.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(1));
    }

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

    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();
    let svc_clone = svc.clone();
    let server = thread::spawn(move || {
        let fd = raw_listener_fd(&svc_clone);
        ready_clone.store(true, Ordering::Release);
        let client_fd = unsafe { libc::accept(fd, std::ptr::null_mut(), std::ptr::null_mut()) };
        assert!(client_fd >= 0, "accept failed: {}", errno());

        let mut hello = [0u8; 128];
        let _ = raw_recv(client_fd, &mut hello).expect("recv hello");

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

    while !ready.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(1));
    }

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

    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();
    let svc_clone = svc.clone();
    let server = thread::spawn(move || {
        let fd = raw_listener_fd(&svc_clone);
        ready_clone.store(true, Ordering::Release);
        let client_fd = unsafe { libc::accept(fd, std::ptr::null_mut(), std::ptr::null_mut()) };
        assert!(client_fd >= 0, "accept failed: {}", errno());

        let mut hello = [0u8; 128];
        let _ = raw_recv(client_fd, &mut hello).expect("recv hello");

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

    while !ready.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(1));
    }

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
        let fd = unsafe { libc::socket(libc::AF_UNIX, libc::SOCK_SEQPACKET, 0) };
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
        let fd = unsafe { libc::socket(libc::AF_UNIX, libc::SOCK_SEQPACKET, 0) };
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
        let fd = unsafe { libc::socket(libc::AF_UNIX, libc::SOCK_SEQPACKET, 0) };
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
    let ready = Arc::new(AtomicBool::new(false));
    let ready_clone = ready.clone();

    let server_thread = thread::spawn(move || {
        let listener =
            UdsListener::bind(TEST_RUN_DIR, &svc_clone, default_server_config()).expect("listen");
        ready_clone.store(true, Ordering::Release);

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

    while !ready.load(Ordering::Acquire) {
        thread::sleep(Duration::from_millis(1));
    }

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

// -----------------------------------------------------------------------
//  Helpers: highest_bit, apply_default
// -----------------------------------------------------------------------

#[test]
fn test_highest_bit() {
    assert_eq!(highest_bit(0), 0);
    assert_eq!(highest_bit(1), 1);
    assert_eq!(highest_bit(0b0101), 4);
    assert_eq!(highest_bit(0b1000), 8);
    assert_eq!(highest_bit(0xFF), 128);
}

#[test]
fn test_apply_default() {
    assert_eq!(apply_default(0, 42), 42);
    assert_eq!(apply_default(10, 42), 10);
}

#[test]
fn test_detect_packet_size_invalid_fd_returns_fallback() {
    assert_eq!(detect_packet_size(-1), DEFAULT_PACKET_SIZE_FALLBACK);
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
