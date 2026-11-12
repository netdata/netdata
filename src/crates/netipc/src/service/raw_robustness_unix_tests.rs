//! Robustness reproduction tests (SOW-0040).
//!
//! Each test asserts the correct behavior and runs every blocking step on a
//! helper thread under a watchdog, so a regression fails instead of hanging.
//! Bounds are deliberately lenient so they do not dictate a fix design.

use super::*;
use crate::protocol::{METHOD_STRING_REVERSE, STRING_REVERSE_HDR_SIZE, string_reverse_encode};
use std::io::{BufRead, BufReader};
use std::process::{Child, Command, Stdio};

const HELPER_ENV: &str = "NETIPC_ROBUSTNESS_HELPER";
const HELPER_TEST: &str = "service::raw::tests::robustness::robustness_helper_process";

/// Run `f` on a thread; return its result, or None if it did not finish in time.
/// On timeout the thread is left running; callers release whatever blocks it.
fn within<T: Send + 'static>(
    limit: Duration,
    f: impl FnOnce() -> T + Send + 'static,
) -> (Option<T>, thread::JoinHandle<()>) {
    let (tx, rx) = mpsc::channel();
    let handle = thread::spawn(move || {
        let _ = tx.send(f());
    });
    (rx.recv_timeout(limit).ok(), handle)
}

/// Connect and complete one increment call. The server listens before tests
/// connect and a slot is free by then, so one attempt must succeed.
fn served_once(service: &str, config: ClientConfig) -> Result<(), String> {
    let mut client = increment_client(service, config);
    client.refresh();
    let result = if client.ready() {
        match client.call_increment_with_timeout(1, 1000) {
            Ok(2) => Ok(()),
            Ok(v) => Err(format!("unexpected value {v}")),
            Err(e) => Err(format!("call error {e:?}")),
        }
    } else {
        Err(format!("state {:?}", client.state))
    };
    client.close();
    result
}

/// Increment handler that reports entry, then blocks until its process is
/// killed.
fn blocking_increment_dispatch(entered: impl Fn() + Send + Sync + 'static) -> DispatchHandler {
    increment_dispatch(Arc::new(move |_v| {
        entered();
        loop {
            thread::park();
        }
    }))
}

#[cfg(any(target_os = "linux", target_os = "freebsd"))]
fn shm_files(service: &str) -> Vec<String> {
    let prefix = format!("{service}-");
    std::fs::read_dir(TEST_RUN_DIR)
        .map(|rd| {
            rd.filter_map(|e| e.ok())
                .map(|e| e.file_name().to_string_lossy().into_owned())
                .filter(|n| n.starts_with(&prefix) && n.ends_with(".ipcshm"))
                .collect()
        })
        .unwrap_or_default()
}

/// Connect a raw SOCK_SEQPACKET socket to the service without sending HELLO.
fn silent_connect(service: &str) -> RawFd {
    let path = socket_path(service);
    let bytes = path.as_os_str().as_bytes();
    let fd = unsafe { libc::socket(libc::AF_UNIX, crate::transport::posix::SOCK_TYPE, 0) };
    assert!(fd >= 0, "socket: {}", std::io::Error::last_os_error());
    let mut addr: libc::sockaddr_un = unsafe { std::mem::zeroed() };
    addr.sun_family = libc::AF_UNIX as libc::sa_family_t;
    for (i, b) in bytes.iter().enumerate() {
        addr.sun_path[i] = *b as libc::c_char;
    }
    let rc = unsafe {
        libc::connect(
            fd,
            &addr as *const libc::sockaddr_un as *const libc::sockaddr,
            std::mem::size_of::<libc::sockaddr_un>() as libc::socklen_t,
        )
    };
    assert_eq!(rc, 0, "connect: {}", std::io::Error::last_os_error());
    fd
}

/// Start a managed server on its own thread with test timings. Dropping or
/// stopping the result stops the server.
fn start_server(
    service: &str,
    config: ServerConfig,
    method: u16,
    handler: DispatchHandler,
    workers: usize,
) -> server::RunningTestServer {
    ensure_run_dir();
    cleanup_all(service);
    ManagedServer::with_workers(
        TEST_RUN_DIR,
        service,
        config,
        method,
        Some(handler),
        workers,
    )
    .spawn_for_test()
}

/// Stop `server`; returns whether `run()` returned within `limit`.
fn stop_within(mut server: server::RunningTestServer, limit: Duration) -> bool {
    let (stopped, waiter) = within(limit, move || server.stop());
    let finished = stopped.is_some();
    if finished {
        let _ = waiter.join();
    }
    finished
}

/// A helper process and the signal it sends when its server handler starts.
struct Helper {
    child: Child,
    handler_entered: mpsc::Receiver<()>,
}

fn spawn_helper(mode: &str, service: &str) -> Helper {
    let mut child = Command::new(std::env::current_exe().expect("current_exe"))
        .args([HELPER_TEST, "--exact", "--nocapture", "--test-threads=1"])
        .env(HELPER_ENV, format!("{mode}:{service}"))
        .stdout(Stdio::piped())
        .stderr(Stdio::null())
        .spawn()
        .expect("spawn helper");
    let stdout = child.stdout.take().expect("helper stdout");
    let (ready_tx, ready_rx) = mpsc::channel();
    let (entered_tx, handler_entered) = mpsc::channel();
    thread::spawn(move || {
        for line in BufReader::new(stdout).lines().map_while(Result::ok) {
            if line.contains("HELPER_READY") {
                let _ = ready_tx.send(());
            }
            if line.contains("HELPER_HANDLER_ENTERED") {
                let _ = entered_tx.send(());
            }
        }
    });
    if ready_rx.recv_timeout(server::TEST_WAIT_LIMIT).is_err() {
        let _ = child.kill();
        let _ = child.wait();
        panic!("helper {mode} did not become ready");
    }
    Helper {
        child,
        handler_entered,
    }
}

/// Child-process entry point. Inert unless spawned by `spawn_helper`.
#[test]
fn robustness_helper_process() {
    let Ok(spec) = std::env::var(HELPER_ENV) else {
        return;
    };
    let (mode, service) = spec.split_once(':').expect("helper spec");
    match mode {
        #[cfg(any(target_os = "linux", target_os = "freebsd"))]
        "shm_client" => {
            let mut client = increment_client(service, shm_client_config());
            connect_ready(&mut client);
            assert_eq!(client.call_increment_with_timeout(1, 2000), Ok(2));
            println!("HELPER_READY");
            // Until the test kills this process.
            loop {
                thread::park();
            }
        }
        "shm_server" | "uds_server" => {
            let handler = blocking_increment_dispatch(|| println!("HELPER_HANDLER_ENTERED"));
            let config = match mode {
                #[cfg(any(target_os = "linux", target_os = "freebsd"))]
                "shm_server" => shm_server_config(),
                _ => server_config(),
            };
            let _server = start_server(service, config, METHOD_INCREMENT, handler, 2);
            println!("HELPER_READY");
            loop {
                thread::park();
            }
        }
        other => panic!("unknown helper mode {other}"),
    }
}

// ---------------------------------------------------------------------------
// K1: SHM server must release a session once its client leaves.
// ---------------------------------------------------------------------------

#[test]
#[cfg(any(target_os = "linux", target_os = "freebsd"))]
fn test_robust_shm_sessions_released_after_clean_close() {
    let svc = unique_service("rs_rob_shm_clean");
    let server = start_server(
        &svc,
        shm_server_config(),
        METHOD_INCREMENT,
        increment_dispatch_handler(),
        2,
    );

    // Each worker must exit and remove its region once its client closes;
    // a leaked session fails wait_session_exits loudly.
    let mut failures = Vec::new();
    for i in 0..5 {
        if let Err(e) = served_once(&svc, shm_client_config()) {
            failures.push(format!("client {i}: {e}"));
        }
        server.events.wait_session_exits(1);
    }
    let leftover = shm_files(&svc);

    stop_within(server, Duration::from_secs(5));
    cleanup_all(&svc);
    assert!(
        failures.is_empty(),
        "with 2 workers, sequential SHM clients must all be served: {failures:?}"
    );
    assert!(leftover.is_empty(), "SHM regions leaked: {leftover:?}");
}

#[test]
#[cfg(any(target_os = "linux", target_os = "freebsd"))]
fn test_robust_shm_sessions_released_after_client_crash() {
    let svc = unique_service("rs_rob_shm_crash");
    let server = start_server(
        &svc,
        shm_server_config(),
        METHOD_INCREMENT,
        increment_dispatch_handler(),
        2,
    );

    for _ in 0..2 {
        let mut helper = spawn_helper("shm_client", &svc);
        let _ = helper.child.kill(); // SIGKILL
        let _ = helper.child.wait();
    }
    // Both workers must notice their crashed clients and free the slots.
    server.events.wait_session_exits(2);
    let served = served_once(&svc, shm_client_config());
    server.events.wait_session_exits(1);
    let leftover = shm_files(&svc);

    stop_within(server, Duration::from_secs(5));
    cleanup_all(&svc);
    assert!(
        served.is_ok(),
        "after 2 crashed SHM clients on 2 workers, a new client must be served: {served:?}"
    );
    assert!(leftover.is_empty(), "SHM regions leaked: {leftover:?}");
}

// ---------------------------------------------------------------------------
// K3: a client that connects but never sends HELLO must not block others or
// shutdown.
// ---------------------------------------------------------------------------

#[test]
fn test_robust_silent_client_does_not_block_other_clients() {
    let svc = unique_service("rs_rob_silent");
    let server = start_server(
        &svc,
        server_config(),
        METHOD_INCREMENT,
        increment_dispatch_handler(),
        4,
    );

    let silent = silent_connect(&svc);
    server.events.wait_handshakes_started(1);
    let svc2 = svc.clone();
    let (served, worker) = within(Duration::from_secs(5), move || {
        served_once(&svc2, client_config())
    });

    unsafe { libc::close(silent) };
    let _ = worker.join();
    stop_within(server, Duration::from_secs(5));
    cleanup_all(&svc);
    assert!(
        matches!(served, Some(Ok(()))),
        "a legitimate client must be served within 5s while a silent client is connected: {served:?}"
    );
}

/// A silent client holds a worker only until the server handshake deadline.
#[test]
fn test_robust_silent_client_released_at_handshake_deadline() {
    let svc = unique_service("rs_rob_silent_deadline");
    let server = start_server(
        &svc,
        server_config(),
        METHOD_INCREMENT,
        increment_dispatch_handler(),
        1,
    );

    let silent = silent_connect(&svc);
    let start = Instant::now();
    server.events.wait_handshakes_started(1);
    server.events.wait_session_exits(1);
    let elapsed = start.elapsed();
    let served = served_once(&svc, client_config());

    let mut ch = 0u8;
    let n = unsafe { libc::recv(silent, (&mut ch as *mut u8).cast(), 1, libc::MSG_DONTWAIT) };
    unsafe { libc::close(silent) };
    stop_within(server, Duration::from_secs(5));
    cleanup_all(&svc);
    let deadline = server::TEST_SERVER_TIMINGS.handshake_ms;
    assert!(
        elapsed >= Duration::from_millis(u64::from(deadline)),
        "silent client released after {elapsed:?}, before the {deadline} ms handshake deadline"
    );
    assert!(
        served.is_ok(),
        "the only worker must be freed once the silent client misses the handshake deadline: {served:?}"
    );
    assert_eq!(
        n, 0,
        "server must close the silent connection at the deadline"
    );
}

#[test]
fn test_robust_silent_client_does_not_block_shutdown() {
    let svc = unique_service("rs_rob_silent_stop");
    let server = start_server(
        &svc,
        server_config(),
        METHOD_INCREMENT,
        increment_dispatch_handler(),
        4,
    );

    let silent = silent_connect(&svc);
    server.events.wait_handshakes_started(1);
    let joined = stop_within(server, Duration::from_secs(3));

    unsafe { libc::close(silent) };
    cleanup_all(&svc);
    assert!(
        joined,
        "server run() must return within 3s of stop() while a silent client is connected"
    );
}

// ---------------------------------------------------------------------------
// K4: a client call must be bounded even when the server never answers HELLO.
// ---------------------------------------------------------------------------

#[test]
fn test_robust_client_bounded_when_server_never_accepts() {
    let svc = unique_service("rs_rob_noaccept");
    ensure_run_dir();
    cleanup_all(&svc);
    let listener = raw_listener_fd_for_service(&svc);

    let svc2 = svc.clone();
    let start = Instant::now();
    let (result, worker) = within(Duration::from_secs(6), move || {
        let mut client = increment_client(&svc2, client_config());
        client.set_call_timeout(1000);
        client.refresh();
        let r = client.call_increment_with_timeout(1, 1000);
        client.close();
        r
    });
    let elapsed = start.elapsed();

    unsafe { libc::close(listener) };
    let _ = worker.join();
    cleanup_all(&svc);
    assert!(
        result.is_some(),
        "refresh + call with a 1s timeout must return within 6s against a server that never \
         accepts (still blocked after {elapsed:?})"
    );
}

#[test]
fn test_robust_client_bounded_when_server_never_sends_hello_ack() {
    let svc = unique_service("rs_rob_nohelloack");
    ensure_run_dir();
    cleanup_all(&svc);
    let listener = raw_listener_fd_for_service(&svc);
    let (release_tx, release_rx) = mpsc::channel::<()>();
    let acceptor = thread::spawn(move || {
        let fd = unsafe { libc::accept(listener, std::ptr::null_mut(), std::ptr::null_mut()) };
        // Hold the session open without replying until the test releases it.
        let _ = release_rx.recv();
        if fd >= 0 {
            unsafe { libc::close(fd) };
        }
        unsafe { libc::close(listener) };
    });

    let svc2 = svc.clone();
    let (result, worker) = within(Duration::from_secs(6), move || {
        let mut client = increment_client(&svc2, client_config());
        client.set_call_timeout(1000);
        client.refresh();
        let r = client.call_increment_with_timeout(1, 1000);
        client.close();
        r
    });

    drop(release_tx);
    let _ = acceptor.join();
    let _ = worker.join();
    cleanup_all(&svc);
    assert!(
        result.is_some(),
        "refresh + call with a 1s timeout must return within 6s when the server accepts but \
         never sends HELLO_ACK"
    );
}

/// Accept one connection on `listener` and hold it without replying until
/// the returned sender is dropped. `accepted` fires once a client connected.
fn accept_and_hold(
    listener: i32,
) -> (mpsc::Sender<()>, mpsc::Receiver<()>, thread::JoinHandle<()>) {
    let (release_tx, release_rx) = mpsc::channel::<()>();
    let (accepted_tx, accepted_rx) = mpsc::channel::<()>();
    let holder = thread::spawn(move || {
        let fd = unsafe { libc::accept(listener, std::ptr::null_mut(), std::ptr::null_mut()) };
        let _ = accepted_tx.send(());
        let _ = release_rx.recv();
        if fd >= 0 {
            unsafe { libc::close(fd) };
        }
        unsafe { libc::close(listener) };
    });
    (release_tx, accepted_rx, holder)
}

#[test]
fn test_robust_call_reconnect_bounded_by_call_deadline() {
    let svc = unique_service("rs_rob_reconnect_noack");
    let server = start_server(
        &svc,
        server_config(),
        METHOD_INCREMENT,
        increment_dispatch_handler(),
        1,
    );
    let mut client = increment_client(&svc, client_config());
    connect_ready(&mut client);
    assert!(stop_within(server, Duration::from_secs(5)), "server stops");

    // The call fails on the dead session and reconnects to a server that
    // accepts but never sends HELLO_ACK.
    let (release_tx, _accepted, holder) = accept_and_hold(raw_listener_fd_for_service(&svc));
    let (result, worker) = within(Duration::from_secs(6), move || {
        let start = Instant::now();
        let r = client.call_increment_with_timeout(1, 1000);
        let elapsed = start.elapsed();
        client.close();
        (r, elapsed)
    });

    drop(release_tx);
    let _ = holder.join();
    let _ = worker.join();
    cleanup_all(&svc);
    let (r, elapsed) = result.expect("a call with a 1s timeout must return within 6s");
    assert_eq!(
        r,
        Err(NipcError::Timeout),
        "call reports the timeout after {elapsed:?}"
    );
}

#[test]
fn test_robust_abort_ends_refresh_waiting_for_hello_ack() {
    let svc = unique_service("rs_rob_abort_refresh");
    ensure_run_dir();
    cleanup_all(&svc);
    let (release_tx, accepted, holder) = accept_and_hold(raw_listener_fd_for_service(&svc));

    let mut client = increment_client(&svc, client_config());
    // Only an abort can end the wait early.
    client.set_call_timeout(10_000);
    let abort = client.abort_handle();
    let (worker_tx, worker_rx) = mpsc::channel();
    let worker = thread::spawn(move || {
        client.refresh();
        let _ = worker_tx.send(client.ready());
        client.close();
    });
    accepted
        .recv_timeout(server::TEST_WAIT_LIMIT)
        .expect("server accepted the connection");
    abort.abort();
    let ready = worker_rx.recv_timeout(Duration::from_secs(2));

    drop(release_tx);
    let _ = holder.join();
    let _ = worker.join();
    cleanup_all(&svc);
    assert_eq!(
        ready,
        Ok(false),
        "refresh returns within 2s of the abort, not ready"
    );
}

// ---------------------------------------------------------------------------
// N1: the configured call timeout must be honored on the UDS path.
// ---------------------------------------------------------------------------

#[test]
fn test_robust_uds_call_honors_timeout_longer_than_100ms() {
    let svc = unique_service("rs_rob_slow_handler");
    let handler = increment_dispatch(Arc::new(|v| {
        thread::sleep(Duration::from_millis(300));
        Some(v + 1)
    }));
    let server = start_server(&svc, server_config(), METHOD_INCREMENT, handler, 2);

    let mut client = increment_client(&svc, client_config());
    connect_ready(&mut client);
    let start = Instant::now();
    let result = client.call_increment_with_timeout(41, 2000);
    let elapsed = start.elapsed();
    client.close();

    stop_within(server, Duration::from_secs(5));
    cleanup_all(&svc);
    assert_eq!(
        result,
        Ok(42),
        "a 300ms handler must succeed with a 2000ms timeout (failed after {elapsed:?})"
    );
}

// ---------------------------------------------------------------------------
// N2: shutdown must not wait forever on a worker blocked sending a response
// to a client that stopped reading.
// ---------------------------------------------------------------------------

#[test]
fn test_robust_shutdown_not_blocked_by_non_reading_client() {
    const BIG: u32 = 8 * 1024 * 1024;
    let svc = unique_service("rs_rob_noread");
    let mut scfg = server_config();
    scfg.max_request_payload_bytes = BIG;
    scfg.max_response_payload_bytes = BIG;
    // Explicit packet size isolates this test from the default-size issue
    // covered by test_robust_default_packet_size_carries_large_message.
    scfg.packet_size = 65536;
    // After the handler returns, the worker can only be sending the 4 MiB
    // response, which blocks because the client never reads.
    let (handled_tx, handled_rx) = mpsc::channel();
    let handled_tx = std::sync::Mutex::new(handled_tx);
    let reverse = string_reverse_handler();
    let handler = string_reverse_dispatch(Arc::new(move |s: &str| {
        let out = reverse(s);
        let _ = handled_tx.lock().unwrap().send(());
        out
    }));
    let server = start_server(&svc, scfg, METHOD_STRING_REVERSE, handler, 2);

    let mut ccfg = client_config();
    ccfg.max_request_payload_bytes = BIG;
    ccfg.max_response_payload_bytes = BIG;
    ccfg.packet_size = 65536;
    let mut session = UdsSession::connect(TEST_RUN_DIR, &svc, &ccfg).expect("connect");
    let text = "x".repeat(4 * 1024 * 1024);
    let mut payload = vec![0u8; STRING_REVERSE_HDR_SIZE + text.len() + 1];
    let n = string_reverse_encode(text.as_bytes(), &mut payload);
    assert!(n > 0, "encode request");
    let mut hdr = Header {
        kind: KIND_REQUEST,
        code: METHOD_STRING_REVERSE,
        item_count: 1,
        message_id: 1,
        ..Header::default()
    };
    session.send(&mut hdr, &payload[..n]).expect("send request");
    // Never read the response.
    handled_rx
        .recv_timeout(server::TEST_WAIT_LIMIT)
        .expect("string reverse handler ran");

    let joined = stop_within(server, Duration::from_secs(3));

    drop(session);
    cleanup_all(&svc);
    assert!(
        joined,
        "server run() must return within 3s of stop() while a worker is blocked sending"
    );
}

// ---------------------------------------------------------------------------
// N3: stop() issued before run() must not be lost.
// ---------------------------------------------------------------------------

#[test]
fn test_robust_stop_before_run_is_honored() {
    let svc = unique_service("rs_rob_stop_first");
    ensure_run_dir();
    cleanup_all(&svc);
    let mut server = ManagedServer::with_workers(
        TEST_RUN_DIR,
        &svc,
        server_config(),
        METHOD_INCREMENT,
        Some(increment_dispatch_handler()),
        2,
    );
    server.stop();
    let flag = server.running_flag();
    let (returned, worker) = within(Duration::from_secs(2), move || {
        let _ = server.run();
    });

    if returned.is_none() {
        flag.store(false, Ordering::Release);
    }
    let _ = worker.join();
    cleanup_all(&svc);
    assert!(
        returned.is_some(),
        "run() after stop() must return promptly instead of serving forever"
    );
}

// ---------------------------------------------------------------------------
// N11: with the auto-detected packet size, a message larger than SO_SNDBUF
// must still be delivered (chunked).
// ---------------------------------------------------------------------------

#[test]
fn test_robust_default_packet_size_carries_large_message() {
    let result = large_round_trip(0);
    assert!(
        result.is_ok(),
        "a 512 KiB request must round-trip with default packet size: {result:?}"
    );
}

/// Control: the same message with an explicit 64 KiB packet size passes,
/// isolating the failure above to the auto-detected packet size.
#[test]
fn test_robust_explicit_packet_size_carries_large_message() {
    let result = large_round_trip(65536);
    assert!(
        result.is_ok(),
        "a 512 KiB request must round-trip with a 64 KiB packet size: {result:?}"
    );
}

fn large_round_trip(packet_size: u32) -> Result<(), NipcError> {
    const BIG: u32 = 1024 * 1024;
    let svc = unique_service("rs_rob_bigmsg");
    let mut scfg = server_config();
    scfg.max_request_payload_bytes = BIG;
    scfg.max_response_payload_bytes = BIG;
    scfg.packet_size = packet_size;
    let server = start_server(
        &svc,
        scfg,
        METHOD_STRING_REVERSE,
        string_reverse_dispatch_handler(),
        2,
    );

    let mut ccfg = client_config();
    ccfg.max_request_payload_bytes = BIG;
    ccfg.max_response_payload_bytes = BIG;
    ccfg.packet_size = packet_size;
    let mut client = string_reverse_client(&svc, ccfg);
    connect_ready(&mut client);
    let text = "y".repeat(512 * 1024);
    let result = client
        .call_string_reverse_with_timeout(&text, 5000)
        .map(|v| v.as_str().len());
    client.close();

    stop_within(server, Duration::from_secs(5));
    cleanup_all(&svc);
    match result {
        Ok(n) if n == text.len() => Ok(()),
        Ok(_) => Err(NipcError::Truncated),
        Err(e) => Err(e),
    }
}

// ---------------------------------------------------------------------------
// N5: an SHM client must notice a crashed server instead of waiting out its
// whole timeout.
// ---------------------------------------------------------------------------

#[test]
#[cfg(any(target_os = "linux", target_os = "freebsd"))]
fn test_robust_shm_client_notices_crashed_server() {
    client_notices_crashed_server(true);
}

#[test]
fn test_robust_uds_client_reports_crashed_server_as_disconnected() {
    client_notices_crashed_server(false);
}

fn client_notices_crashed_server(shm: bool) {
    let svc = unique_service(if shm {
        "rs_rob_dead_server"
    } else {
        "rs_rob_dead_uds_server"
    });
    ensure_run_dir();
    cleanup_all(&svc);
    let mut helper = spawn_helper(if shm { "shm_server" } else { "uds_server" }, &svc);

    let svc2 = svc.clone();
    let (call_tx, call_rx) = mpsc::channel();
    let caller = thread::spawn(move || {
        let config = match shm {
            #[cfg(any(target_os = "linux", target_os = "freebsd"))]
            true => shm_client_config(),
            _ => client_config(),
        };
        let mut client = increment_client(&svc2, config);
        connect_ready(&mut client);
        let _ = call_tx.send(());
        let start = Instant::now();
        let r = client.call_increment_with_timeout(1, 10_000);
        let elapsed = start.elapsed();
        client.close();
        (r, elapsed)
    });
    call_rx
        .recv_timeout(Duration::from_secs(10))
        .expect("client connected");
    // Request parked in the stalled handler.
    helper
        .handler_entered
        .recv_timeout(server::TEST_WAIT_LIMIT)
        .expect("helper server handler entered");
    let _ = helper.child.kill();
    let _ = helper.child.wait();

    let (result, elapsed) = caller.join().expect("caller");
    cleanup_all(&svc);
    assert!(
        result.is_err() && elapsed < Duration::from_millis(3500),
        "call must fail within 3s of the server crash (timeout is 10s): {result:?} after {elapsed:?}"
    );
    assert_eq!(
        result,
        Err(NipcError::Disconnected),
        "call must report the server as disconnected"
    );
}

// A server at worker capacity completes the handshake, then closes the
// session and removes the prepared SHM region. The client must not spend its
// attach retry budget (5 s) on a region that will never appear, and must keep
// SHM enabled for its next connect.
#[test]
#[cfg(any(target_os = "linux", target_os = "freebsd"))]
fn test_robust_shm_attach_stops_when_server_closes_session() {
    let svc = unique_service("rs_rob_attach_refused");
    let handler = increment_dispatch(Arc::new(|v| Some(v + 1)));
    let server = start_server(&svc, shm_server_config(), METHOD_INCREMENT, handler, 1);

    let mut holder = increment_client(&svc, shm_client_config());
    connect_ready(&mut holder);

    // Production attach budget: a short test budget would hide the bug.
    let mut refused = RawClient::new_increment(TEST_RUN_DIR, &svc, shm_client_config());
    let start = Instant::now();
    refused.refresh();
    let elapsed = start.elapsed();

    holder.close();
    server.events.wait_session_exits(1);
    refused.refresh();
    let ready = refused.ready();
    let uses_shm = refused.shm.is_some();
    refused.close();
    stop_within(server, Duration::from_secs(5));
    cleanup_all(&svc);

    assert!(
        elapsed < Duration::from_secs(1),
        "refused connect must return within 1s (attach retry budget is 5s): {elapsed:?}"
    );
    assert!(ready, "refused client must connect once the worker is free");
    assert!(uses_shm, "refused client must still use SHM");
}

// ---------------------------------------------------------------------------
// Shutdown order: run closes the listening socket when its loop exits; the
// endpoint lock stays held until every worker has exited.
// ---------------------------------------------------------------------------

#[test]
fn test_robust_shutdown_closes_socket_before_endpoint() {
    let svc = unique_service("rs_rob_shut_order");
    let (entered_tx, entered_rx) = mpsc::channel();
    let entered_tx = std::sync::Mutex::new(entered_tx);
    let (release_tx, release_rx) = mpsc::channel::<()>();
    let release_rx = std::sync::Mutex::new(release_rx);
    let handler = increment_dispatch(Arc::new(move |v| {
        let _ = entered_tx.lock().unwrap().send(());
        let _ = release_rx.lock().unwrap().recv();
        Some(v + 1)
    }));
    let mut server = start_server(&svc, server_config(), METHOD_INCREMENT, handler, 1);

    let call_svc = svc.clone();
    let call = thread::spawn(move || {
        let mut client = increment_client(&call_svc, client_config());
        client.refresh();
        let result = client.call_increment_with_timeout(1, 10_000);
        client.close();
        result
    });
    entered_rx
        .recv_timeout(server::TEST_WAIT_LIMIT)
        .expect("handler entered");

    // stop() returns only after the workers exit, so it runs aside.
    let stopper = thread::spawn(move || server.stop());
    // After its grace the server shuts the blocked worker's socket, which
    // fails the call; the worker itself stays in the handler.
    assert!(
        call.join().expect("call join").is_err(),
        "blocked call fails"
    );

    let socket = std::path::Path::new(TEST_RUN_DIR).join(format!("{svc}.sock"));
    let socket_gone = !socket.exists();
    let held = UdsListener::bind(TEST_RUN_DIR, &svc, server_config()).is_err();

    release_tx.send(()).expect("release handler");
    stopper.join().expect("stopper join");
    let free = UdsListener::bind(TEST_RUN_DIR, &svc, server_config()).is_ok();
    cleanup_all(&svc);

    assert!(
        socket_gone,
        "socket file removed once the accept loop exits"
    );
    assert!(held, "endpoint held while a worker runs");
    assert!(free, "endpoint free once the server stopped");
}

// ---------------------------------------------------------------------------
// Bounded send: a request the server never reads ends at the call deadline,
// or on abort, instead of blocking in the send.
// ---------------------------------------------------------------------------

const STALLED_READER_REQUEST_BYTES: u32 = 8 * 1024 * 1024;
const STALLED_READER_PIDS: u32 = 1024 * 1024;

/// Completes the handshake and never reads. Sends on `started` once the
/// request's first bytes arrived, then holds the session until `release`.
fn start_stalled_reader(
    svc: &str,
    started: mpsc::Sender<()>,
    release: mpsc::Receiver<()>,
) -> thread::JoinHandle<()> {
    ensure_run_dir();
    cleanup_all(svc);
    let cfg = ServerConfig {
        max_request_payload_bytes: STALLED_READER_REQUEST_BYTES,
        ..server_config()
    };
    let listener = UdsListener::bind(TEST_RUN_DIR, svc, cfg).expect("bind");
    thread::spawn(move || {
        let Ok(session) = listener.accept() else {
            let _ = started.send(());
            return;
        };
        let mut pfd = libc::pollfd {
            fd: session.fd(),
            events: libc::POLLIN,
            revents: 0,
        };
        unsafe { libc::poll(&mut pfd, 1, 10_000) };
        let _ = started.send(());
        let _ = release.recv_timeout(server::TEST_WAIT_LIMIT);
        drop(session);
        drop(listener);
    })
}

#[test]
fn test_robust_send_bounded_by_deadline_and_abort() {
    let pids: Vec<u32> = (1..=STALLED_READER_PIDS).collect();
    for use_abort in [false, true] {
        let svc = unique_service(if use_abort {
            "rs_rob_send_abort"
        } else {
            "rs_rob_send_deadline"
        });
        let (started_tx, started_rx) = mpsc::channel();
        let (release_tx, release_rx) = mpsc::channel();
        let server = start_stalled_reader(&svc, started_tx, release_rx);

        let mut client = apps_lookup_client(
            &svc,
            ClientConfig {
                max_request_payload_bytes: STALLED_READER_REQUEST_BYTES,
                ..client_config()
            },
        );
        client.set_lookup_logical_config(LookupLogicalConfig {
            max_items: STALLED_READER_PIDS,
            ..LookupLogicalConfig::default()
        });
        client.refresh();
        assert!(client.ready(), "client ready against the stalled reader");

        let abort = client.abort_handle();
        let timeout_ms = if use_abort { 30_000 } else { 200 };
        let start = Instant::now();
        let call_pids = pids.clone();
        let call = thread::spawn(move || {
            let result = client
                .call_apps_lookup_with_timeout(&call_pids, timeout_ms)
                .map(|_| ());
            client.close();
            result
        });
        if use_abort {
            started_rx
                .recv_timeout(server::TEST_WAIT_LIMIT)
                .expect("request started");
            abort.abort();
        }
        let result = call.join().expect("call join");
        let elapsed = start.elapsed();
        let _ = release_tx.send(());
        server.join().expect("server join");
        cleanup_all(&svc);

        let expected = if use_abort {
            NipcError::Aborted
        } else {
            NipcError::Timeout
        };
        assert_eq!(result, Err(expected), "blocked send result");
        assert!(
            elapsed < Duration::from_secs(5),
            "blocked send ends promptly: {elapsed:?}"
        );
    }
}
