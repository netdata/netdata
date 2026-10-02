//! The legacy-logs worker's startup handshake with the otel-plugin
//! supervisor: the worker connects over IPC, receives one `Configure`,
//! then answers `Disabled` and exits, or `Ready` and serves until
//! `Shutdown`. Each test plays the supervisor side - a ferryboat
//! `Listener` on an IPC endpoint accepting the real in-process
//! `run_worker` future, no subprocess - and drives one handshake branch
//! to its end state. Production counterpart: `configure_legacy`
//! (otel-plugin/src/supervisor.rs:226-248).
//!
//! Pinned contracts (worker side: src/lib.rs:30-109):
//!
//! - Journal dir absent -> `Disabled`, then the worker future completes
//!   cleanly: no idle process lingers for the plugin's lifetime
//!   (src/lib.rs:56-66).
//! - Handler init failure -> `Disabled` and a clean exit too, exactly
//!   like the absent-dir path: the legacy viewer is best-effort and
//!   must not take the plugin down (src/lib.rs:72-85; init builds the
//!   foyer disk cache, src/handler.rs:143-152).
//! - An existing-but-empty journal dir still serves: `Ready` carrying
//!   exactly one declaration, the `legacy-otel-logs` function
//!   (src/lib.rs:27,86), because the handler watches the directory and
//!   journal files restored into it later are picked up
//!   (src/lib.rs:50-55).
//! - `Shutdown` ends the event loop and the worker exits cleanly
//!   (src/lib.rs:139-141,221-224).
//!
//! Not pinned here: Call/Cancel/Progress traffic, the oversized-response
//! degradation (src/lib.rs:163-199), late `Configure` (src/lib.rs:225-228),
//! and the supervisor's Disabled bookkeeping - the `legacy_alive` flip
//! and child reap (supervisor.rs:237-244).

use std::time::Duration;

use bridge::config::LegacyLogsConfig;
use bridge::{LegacyLogsRequest, LegacyLogsResponse};
use ferryboat::{Connection, Endpoint, Listener};
use tokio::task::JoinHandle;

/// Bind the supervisor end of the socket, spawn `run_worker` as the
/// client, and return the accepted connection plus the worker's join
/// handle. The 10 s accept timeout pins that the worker connects
/// promptly; the message-size limit mirrors the worker's own
/// (src/lib.rs:33-37).
async fn start(
    sock: &str,
) -> (
    Connection<LegacyLogsRequest, LegacyLogsResponse>,
    JoinHandle<anyhow::Result<()>>,
) {
    let mut listener = Listener::<LegacyLogsRequest, LegacyLogsResponse>::bind(Endpoint::ipc(sock))
        .max_message_size(bridge::IPC_MAX_MESSAGE_SIZE)
        .open()
        .unwrap();
    let sock = sock.to_owned();
    let worker = tokio::spawn(async move { otel_legacy_logs::run_worker(&sock).await });
    let conn = tokio::time::timeout(Duration::from_secs(10), listener.accept())
        .await
        .expect("worker did not connect")
        .unwrap();
    (conn, worker)
}

/// Await one worker response; the 30 s bound is a test hang-guard, not
/// a contract.
async fn recv(
    conn: &mut Connection<LegacyLogsRequest, LegacyLogsResponse>,
) -> LegacyLogsResponse {
    tokio::time::timeout(Duration::from_secs(30), conn.recv())
        .await
        .expect("timed out waiting for worker response")
        .unwrap()
}

/// The worker future must complete cleanly within the timeout: no hang,
/// no panic, no `Err` return.
async fn assert_exits(worker: JoinHandle<anyhow::Result<()>>) {
    tokio::time::timeout(Duration::from_secs(5), worker)
        .await
        .expect("worker did not exit")
        .expect("worker task panicked")
        .expect("worker returned an error");
}

#[tokio::test]
async fn reports_disabled_and_exits_when_journal_dir_absent() {
    let dir = tempfile::tempdir().unwrap();
    let sock = dir.path().join("legacy.sock");
    let (mut conn, worker) = start(sock.to_str().unwrap()).await;

    let config = LegacyLogsConfig::new(
        dir.path().join("does-not-exist"),
        dir.path().join("cache"),
    );
    conn.send(LegacyLogsRequest::Configure(config)).await.unwrap();

    match recv(&mut conn).await {
        LegacyLogsResponse::Disabled => {}
        other => panic!("expected Disabled, got {other:?}"),
    }
    assert_exits(worker).await;
}

#[tokio::test]
async fn reports_disabled_and_exits_when_handler_init_fails() {
    let dir = tempfile::tempdir().unwrap();
    let sock = dir.path().join("legacy.sock");
    let journal_dir = dir.path().join("journal");
    std::fs::create_dir_all(&journal_dir).unwrap();
    // A cache path nested under a regular file makes handler init fail:
    // the cache build runs create_dir_all on it
    // (journal-engine/src/indexing.rs:108-113, via src/handler.rs:146-152).
    let blocker = dir.path().join("blocker");
    std::fs::write(&blocker, b"not a directory").unwrap();
    let (mut conn, worker) = start(sock.to_str().unwrap()).await;

    let config = LegacyLogsConfig::new(journal_dir, blocker.join("cache"));
    conn.send(LegacyLogsRequest::Configure(config)).await.unwrap();

    match recv(&mut conn).await {
        LegacyLogsResponse::Disabled => {}
        other => panic!("expected Disabled, got {other:?}"),
    }
    assert_exits(worker).await;
}

#[tokio::test]
async fn serves_when_journal_dir_exists_even_empty() {
    let dir = tempfile::tempdir().unwrap();
    let sock = dir.path().join("legacy.sock");
    let journal_dir = dir.path().join("journal");
    std::fs::create_dir_all(&journal_dir).unwrap();
    let (mut conn, worker) = start(sock.to_str().unwrap()).await;

    let config = LegacyLogsConfig::new(journal_dir, dir.path().join("cache"));
    conn.send(LegacyLogsRequest::Configure(config)).await.unwrap();

    match recv(&mut conn).await {
        LegacyLogsResponse::Ready { declarations } => assert_eq!(declarations.len(), 1),
        other => panic!("expected Ready, got {other:?}"),
    }

    conn.send(LegacyLogsRequest::Shutdown).await.unwrap();
    assert_exits(worker).await;
}
