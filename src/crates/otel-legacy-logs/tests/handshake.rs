//! Worker lifecycle handshake tests: the worker must report `Disabled` and
//! exit when there is nothing to serve (absent journal dir, handler init
//! failure), and must serve (then shut down gracefully) when the journal
//! directory exists — even an empty one.

use std::time::Duration;

use bridge::config::LegacyLogsConfig;
use bridge::{LegacyLogsRequest, LegacyLogsResponse};
use ferryboat::{Connection, Endpoint, Listener};
use tokio::task::JoinHandle;

/// Bind the supervisor side of the socket, spawn `run_worker` against it, and
/// return the accepted connection plus the worker's join handle.
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

async fn recv(
    conn: &mut Connection<LegacyLogsRequest, LegacyLogsResponse>,
) -> LegacyLogsResponse {
    tokio::time::timeout(Duration::from_secs(30), conn.recv())
        .await
        .expect("timed out waiting for worker response")
        .unwrap()
}

/// The worker future must complete cleanly within the timeout.
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
    // A cache path nested under a regular file makes the disk-cache init fail.
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
