//! Robustness reproduction tests (SOW-0040), Windows.
//!
//! Each test asserts the correct behavior and runs every blocking step on a
//! helper thread under a watchdog, so a regression fails instead of hanging.
//! Bounds are deliberately lenient so they do not dictate a fix design.

use super::*;
use crate::protocol::{STRING_REVERSE_HDR_SIZE, string_reverse_encode};
use std::io::{BufRead, BufReader};
use std::process::{Child, Command, Stdio};

const HELPER_ENV: &str = "NETIPC_ROBUSTNESS_HELPER";
const HELPER_TEST: &str = "service::raw::windows_tests::robustness::robustness_helper_process";
const GENERIC_READ: Dword = 0x8000_0000;
const GENERIC_WRITE: Dword = 0x4000_0000;
const OPEN_EXISTING: Dword = 3;
const FILE_FLAG_FIRST_PIPE_INSTANCE: Dword = 0x0008_0000;

unsafe extern "system" {
    fn CreateFileW(
        name: *const u16,
        access: Dword,
        share: Dword,
        security: *const core::ffi::c_void,
        disposition: Dword,
        flags: Dword,
        template: RawHandle,
    ) -> RawHandle;
}

/// Run `f` on a thread; return its result, or None if it did not finish in time.
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

fn open_pipe_silently(service: &str) -> RawHandle {
    let name = build_pipe_name(TEST_RUN_DIR, service).expect("pipe name");
    let h = unsafe {
        CreateFileW(
            name.as_ptr(),
            GENERIC_READ | GENERIC_WRITE,
            0,
            ptr::null(),
            OPEN_EXISTING,
            0,
            0,
        )
    };
    assert_ne!(
        h,
        INVALID_HANDLE_VALUE,
        "could not open pipe for {service}: {}",
        unsafe { GetLastError() }
    );
    h
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

fn slow_increment_dispatch(delay: Duration) -> DispatchHandler {
    increment_dispatch(Arc::new(move |v| {
        thread::sleep(delay);
        Some(v + 1)
    }))
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

/// Child-process entry point. Inert unless spawned by `spawn_helper`.
#[test]
fn robustness_helper_process() {
    let Ok(spec) = std::env::var(HELPER_ENV) else {
        return;
    };
    let (mode, service) = spec.split_once(':').expect("helper spec");
    match mode {
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
        "shm_server" => {
            let _server = start_server(
                service,
                shm_server_config(),
                METHOD_INCREMENT,
                blocking_increment_dispatch(|| println!("HELPER_HANDLER_ENTERED")),
                2,
            );
            println!("HELPER_READY");
            loop {
                thread::park();
            }
        }
        other => panic!("unknown helper mode {other}"),
    }
}

// K2: SHM server must release a session once its client crashes.
#[test]
fn test_robust_win_shm_session_released_after_client_crash() {
    let svc = unique_service("rs_wrob_shm_crash");
    let server = start_server(
        &svc,
        shm_server_config(),
        METHOD_INCREMENT,
        increment_dispatch_handler(),
        2,
    );

    for _ in 0..2 {
        let mut helper = spawn_helper("shm_client", &svc);
        let _ = helper.child.kill(); // TerminateProcess
        let _ = helper.child.wait();
    }
    // Both workers must notice their crashed clients and free the slots.
    server.events.wait_session_exits(2);
    let served = served_once(&svc, shm_client_config());
    stop_within(server, Duration::from_secs(5));
    assert!(
        served.is_ok(),
        "after 2 crashed SHM clients on 2 workers, a new client must be served: {served:?}"
    );
}

// K3: a client that opens the pipe but never sends HELLO.
#[test]
fn test_robust_win_silent_client_does_not_block_other_clients() {
    let svc = unique_service("rs_wrob_silent");
    let server = start_server(
        &svc,
        server_config(),
        METHOD_INCREMENT,
        increment_dispatch_handler(),
        4,
    );

    let silent = open_pipe_silently(&svc);
    server.events.wait_handshakes_started(1);
    let svc2 = svc.clone();
    let (served, worker) = within(Duration::from_secs(5), move || {
        served_once(&svc2, client_config())
    });

    unsafe { CloseHandle(silent) };
    let _ = worker.join();
    stop_within(server, Duration::from_secs(5));
    assert!(
        matches!(served, Some(Ok(()))),
        "a legitimate client must be served within 5s while a silent client is connected: {served:?}"
    );
}

/// A silent client holds a worker only until the server handshake deadline.
#[test]
fn test_robust_win_silent_client_released_at_handshake_deadline() {
    let svc = unique_service("rs_wrob_silent_deadline");
    let server = start_server(
        &svc,
        server_config(),
        METHOD_INCREMENT,
        increment_dispatch_handler(),
        1,
    );

    let silent = open_pipe_silently(&svc);
    let start = Instant::now();
    server.events.wait_handshakes_started(1);
    server.events.wait_session_exits(1);
    let elapsed = start.elapsed();
    let served = served_once(&svc, client_config());

    unsafe { CloseHandle(silent) };
    stop_within(server, Duration::from_secs(5));
    let deadline = server::TEST_SERVER_TIMINGS.handshake_ms;
    assert!(
        elapsed >= Duration::from_millis(u64::from(deadline)),
        "silent client released after {elapsed:?}, before the {deadline} ms handshake deadline"
    );
    assert!(
        served.is_ok(),
        "the only worker must be freed once the silent client misses the handshake deadline: {served:?}"
    );
}

#[test]
fn test_robust_win_silent_client_does_not_block_shutdown() {
    let svc = unique_service("rs_wrob_silent_stop");
    let server = start_server(
        &svc,
        server_config(),
        METHOD_INCREMENT,
        increment_dispatch_handler(),
        4,
    );

    let silent = open_pipe_silently(&svc);
    server.events.wait_handshakes_started(1);
    let stopped = stop_within(server, Duration::from_secs(3));

    unsafe { CloseHandle(silent) };
    assert!(
        stopped,
        "server run() must return within 3s of shutdown while a silent client is connected"
    );
}

// K4: a client call must be bounded even when the server never answers HELLO.
#[test]
fn test_robust_win_client_bounded_when_server_never_sends_hello_ack() {
    let svc = unique_service("rs_wrob_nohelloack");
    let name = build_pipe_name(TEST_RUN_DIR, &svc).expect("pipe name");
    let pipe = unsafe {
        CreateNamedPipeW(
            name.as_ptr(),
            PIPE_ACCESS_DUPLEX | FILE_FLAG_FIRST_PIPE_INSTANCE,
            PIPE_TYPE_MESSAGE | PIPE_READMODE_MESSAGE | PIPE_WAIT,
            4,
            RAW_PIPE_PACKET_SIZE,
            RAW_PIPE_PACKET_SIZE,
            0,
            ptr::null(),
        )
    };
    assert_ne!(pipe, INVALID_HANDLE_VALUE, "fake server pipe");

    let svc2 = svc.clone();
    let (result, worker) = within(Duration::from_secs(6), move || {
        let mut client = increment_client(&svc2, client_config());
        client.set_call_timeout(1000);
        client.refresh();
        let r = client.call_increment_with_timeout(1, 1000);
        client.close();
        r
    });

    unsafe { CloseHandle(pipe) };
    let _ = worker.join();
    assert!(
        result.is_some(),
        "refresh + call with a 1s timeout must return within 6s when the server never sends HELLO_ACK"
    );
}

/// A pipe that accepts clients but never answers HELLO.
fn fake_server_pipe(service: &str) -> RawHandle {
    let name = build_pipe_name(TEST_RUN_DIR, service).expect("pipe name");
    let pipe = unsafe {
        CreateNamedPipeW(
            name.as_ptr(),
            PIPE_ACCESS_DUPLEX | FILE_FLAG_FIRST_PIPE_INSTANCE,
            PIPE_TYPE_MESSAGE | PIPE_READMODE_MESSAGE | PIPE_WAIT,
            4,
            RAW_PIPE_PACKET_SIZE,
            RAW_PIPE_PACKET_SIZE,
            0,
            ptr::null(),
        )
    };
    assert_ne!(pipe, INVALID_HANDLE_VALUE, "fake server pipe");
    pipe
}

#[test]
fn test_robust_win_call_reconnect_bounded_by_call_deadline() {
    let svc = unique_service("rs_wrob_reconnect_noack");
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
    let pipe = fake_server_pipe(&svc);
    let (result, worker) = within(Duration::from_secs(6), move || {
        let start = Instant::now();
        let r = client.call_increment_with_timeout(1, 1000);
        let elapsed = start.elapsed();
        client.close();
        (r, elapsed)
    });

    unsafe { CloseHandle(pipe) };
    let _ = worker.join();
    let (r, elapsed) = result.expect("a call with a 1s timeout must return within 6s");
    assert_eq!(
        r,
        Err(NipcError::Timeout),
        "call reports the timeout after {elapsed:?}"
    );
}

#[test]
fn test_robust_win_abort_ends_refresh_waiting_for_hello_ack() {
    let svc = unique_service("rs_wrob_abort_refresh");
    let pipe = fake_server_pipe(&svc);

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
    // A client that never connects must fail the test, not hang it.
    let (connected, _connect) = within(Duration::from_secs(5), move || {
        let ok = unsafe { ConnectNamedPipe(pipe, ptr::null_mut()) } != 0;
        ok || unsafe { GetLastError() } == ERROR_PIPE_CONNECTED
    });
    assert_eq!(connected, Some(true), "client connected to the fake server");
    abort.abort();
    let ready = worker_rx.recv_timeout(Duration::from_secs(2));

    unsafe { CloseHandle(pipe) };
    let _ = worker.join();
    assert_eq!(
        ready,
        Ok(false),
        "refresh returns within 2s of the abort, not ready"
    );
}

// N1: the configured call timeout must be honored on the named-pipe path.
#[test]
fn test_robust_win_pipe_call_honors_timeout_longer_than_100ms() {
    let svc = unique_service("rs_wrob_slow_handler");
    let server = start_server(
        &svc,
        server_config(),
        METHOD_INCREMENT,
        slow_increment_dispatch(Duration::from_millis(300)),
        2,
    );

    let mut client = increment_client(&svc, client_config());
    connect_ready(&mut client);
    let start = Instant::now();
    let result = client.call_increment_with_timeout(41, 2000);
    let elapsed = start.elapsed();
    client.close();

    stop_within(server, Duration::from_secs(5));
    assert_eq!(
        result,
        Ok(42),
        "a 300ms handler must succeed with a 2000ms timeout (failed after {elapsed:?})"
    );
}

// N2: shutdown must not wait forever on a worker blocked writing a response
// to a client that stopped reading.
#[test]
fn test_robust_win_shutdown_not_blocked_by_non_reading_client() {
    const BIG: u32 = 8 * 1024 * 1024;
    let svc = unique_service("rs_wrob_noread");
    let mut scfg = server_config();
    scfg.max_request_payload_bytes = BIG;
    scfg.max_response_payload_bytes = BIG;
    let mut ccfg = client_config();
    ccfg.max_request_payload_bytes = BIG;
    ccfg.max_response_payload_bytes = BIG;
    // After the handler returns, the worker can only be writing the 4 MiB
    // response, which blocks because the client never reads.
    let (handled_tx, handled_rx) = mpsc::channel();
    let handled_tx = std::sync::Mutex::new(handled_tx);
    let handler = string_reverse_dispatch(Arc::new(move |s: &str| {
        let out = Some(s.chars().rev().collect());
        let _ = handled_tx.lock().unwrap().send(());
        out
    }));
    let server = start_server(&svc, scfg, METHOD_STRING_REVERSE, handler, 2);

    let mut session = NpSession::connect(TEST_RUN_DIR, &svc, &ccfg).expect("connect");
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

    let stopped = stop_within(server, Duration::from_secs(3));
    drop(session);
    assert!(
        stopped,
        "server run() must return within 3s of shutdown while a worker is blocked writing"
    );
}

// N3: stop() issued before run() must not be lost.
#[test]
fn test_robust_win_stop_before_run_is_honored() {
    let svc = unique_service("rs_wrob_stop_first");
    ensure_run_dir();
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
        let _ = NpSession::connect(TEST_RUN_DIR, &svc, &client_config());
    }
    let _ = worker.join();
    assert!(
        returned.is_some(),
        "run() after stop() must return promptly instead of serving forever"
    );
}

// N5: an SHM client must notice a crashed server instead of waiting out its
// whole timeout.
#[test]
fn test_robust_win_shm_client_notices_crashed_server() {
    let svc = unique_service("rs_wrob_dead_server");
    let mut helper = spawn_helper("shm_server", &svc);

    let svc2 = svc.clone();
    let (call_tx, call_rx) = mpsc::channel();
    let caller = thread::spawn(move || {
        let mut client = increment_client(&svc2, shm_client_config());
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
// session and destroys the prepared WinSHM objects. The client must not spend
// its attach retry budget (5 s) on objects that will never appear, and must
// keep WinSHM enabled for its next connect.
#[test]
fn test_robust_win_shm_attach_stops_when_server_closes_session() {
    let svc = unique_service("rs_wrob_attach_refused");
    let server = start_server(
        &svc,
        shm_server_config(),
        METHOD_INCREMENT,
        increment_dispatch_handler(),
        1,
    );

    let mut holder = increment_client(&svc, shm_client_config());
    holder.refresh();
    let holder_ready = holder.ready();

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

    assert!(holder_ready, "first client must hold the only worker");
    assert!(
        elapsed < Duration::from_secs(1),
        "refused connect must return within 1s (attach retry budget is 5s): {elapsed:?}"
    );
    assert!(ready, "refused client must connect once the worker is free");
    assert!(uses_shm, "refused client must still use WinSHM");
}

// ---------------------------------------------------------------------------
// Bounded send: a request the server never reads ends at the call deadline,
// or on abort, instead of blocking in the write.
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
    let cfg = ServerConfig {
        max_request_payload_bytes: STALLED_READER_REQUEST_BYTES,
        ..server_config()
    };
    let mut listener = NpListener::bind(TEST_RUN_DIR, svc, cfg).expect("bind");
    thread::spawn(move || {
        let Ok(session) = listener.accept() else {
            let _ = started.send(());
            return;
        };
        let _ = session.wait_readable(10_000);
        let _ = started.send(());
        let _ = release.recv_timeout(server::TEST_WAIT_LIMIT);
        drop(session);
        drop(listener);
    })
}

#[test]
fn test_win_robust_send_bounded_by_deadline_and_abort() {
    let pids: Vec<u32> = (1..=STALLED_READER_PIDS).collect();
    for use_abort in [false, true] {
        let svc = unique_service(if use_abort {
            "rs_winrob_send_abort"
        } else {
            "rs_winrob_send_deadline"
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
