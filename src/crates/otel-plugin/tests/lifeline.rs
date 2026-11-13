//! A worker of the real binary exits when the supervisor's lifeline closes,
//! and not before.

use std::os::unix::net::UnixListener;
use std::process::{Command, Stdio};
use std::time::{Duration, Instant};

#[test]
fn a_worker_exits_when_its_lifeline_closes() {
    let tmp = tempfile::tempdir().unwrap();
    let socket = tmp.path().join("legacy-logs.sock");
    // The supervisor's side of the worker's connection. It is accepted and
    // kept open and silent: the worker waits for its configuration, as a
    // ledger that is still starting does not read its connection.
    let listener = UnixListener::bind(&socket).unwrap();
    listener.set_nonblocking(true).unwrap();

    let mut worker = Command::new(env!("CARGO_BIN_EXE_otel-plugin"))
        .args(["worker", "legacy-logs", "--socket"])
        .arg(&socket)
        .arg("--lifeline")
        .stdin(Stdio::piped())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .unwrap();

    let started = Instant::now();
    let _connection = loop {
        match listener.accept() {
            Ok((connection, _)) => break connection,
            Err(e) if e.kind() == std::io::ErrorKind::WouldBlock => {
                assert!(
                    started.elapsed() < Duration::from_secs(20),
                    "the worker did not connect"
                );
                std::thread::sleep(Duration::from_millis(20));
            }
            Err(e) => panic!("accept failed: {e}"),
        }
    };

    // Connected, lifeline open: the worker stays.
    std::thread::sleep(Duration::from_millis(300));
    assert_eq!(worker.try_wait().unwrap(), None);

    // The supervisor goes away: its end of the pipe closes.
    drop(worker.stdin.take());

    let closed = Instant::now();
    let status = loop {
        if let Some(status) = worker.try_wait().unwrap() {
            break status;
        }
        if closed.elapsed() > Duration::from_secs(10) {
            worker.kill().unwrap();
            panic!("the worker outlived its lifeline");
        }
        std::thread::sleep(Duration::from_millis(20));
    };
    assert_eq!(status.code(), Some(1));
}
