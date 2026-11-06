//! Heartbeats to the agent while the supervisor waits on worker startup.
//!
//! The agent stops a plugin that writes nothing to stdout for its idle
//! timeout (at least 120 s, `pluginsd_parser.c`), and the supervisor writes
//! nothing between `TRUST_DURATIONS` and its function declarations: that
//! window lasts as long as the ledger's startup recovery, which is unbounded
//! on a large on-disk backlog. An OS thread, not a runtime task, so a busy or
//! blocked executor cannot starve it. Same shape as the netflow plugin's
//! `startup_keepalive.rs`.

use std::io::{self, Write};
use std::sync::mpsc::{self, RecvTimeoutError};
use std::thread::{self, JoinHandle};
use std::time::Duration;

use tokio::sync::oneshot;

/// Below the agent's minimum idle timeout with margin, like the main loop's
/// keepalive.
const INTERVAL: Duration = Duration::from_secs(60);

pub(crate) struct StartupKeepalive {
    stop: mpsc::Sender<()>,
    worker: Option<JoinHandle<io::Result<()>>>,
    failed: oneshot::Receiver<()>,
}

impl StartupKeepalive {
    pub(crate) fn start() -> io::Result<Self> {
        Self::spawn(INTERVAL, || {
            let mut stdout = io::stdout().lock();
            stdout.write_all(b"PLUGIN_KEEPALIVE\n")?;
            stdout.flush()
        })
    }

    fn spawn<F>(interval: Duration, mut write: F) -> io::Result<Self>
    where
        F: FnMut() -> io::Result<()> + Send + 'static,
    {
        let (stop, stopped) = mpsc::channel();
        let (failed, failure) = oneshot::channel();
        let worker = thread::Builder::new()
            .name("otel-startup-keepalive".into())
            .spawn(move || {
                loop {
                    match stopped.try_recv() {
                        Ok(()) | Err(mpsc::TryRecvError::Disconnected) => return Ok(()),
                        Err(mpsc::TryRecvError::Empty) => {}
                    }
                    if let Err(err) = write() {
                        let _ = failed.send(());
                        return Err(err);
                    }
                    match stopped.recv_timeout(interval) {
                        Ok(()) | Err(RecvTimeoutError::Disconnected) => return Ok(()),
                        Err(RecvTimeoutError::Timeout) => {}
                    }
                }
            })?;
        Ok(Self {
            stop,
            worker: Some(worker),
            failed: failure,
        })
    }

    /// Resolves when a heartbeat write fails (the agent closed our stdout),
    /// with that write's error. Pending forever while writes succeed.
    pub(crate) async fn failure(&mut self) -> io::Error {
        if (&mut self.failed).await.is_err() {
            // The worker ended without reporting a failure: it was stopped.
            return std::future::pending().await;
        }
        self.stop()
            .err()
            .unwrap_or_else(|| io::Error::other("startup keepalive worker stopped unexpectedly"))
    }

    /// Stop and join the worker, returning a write failure it hit. Joining
    /// before the supervisor writes to stdout itself keeps a heartbeat from
    /// interleaving with a protocol line.
    pub(crate) fn stop(&mut self) -> io::Result<()> {
        let Some(worker) = self.worker.take() else {
            return Ok(());
        };
        let _ = self.stop.send(());
        worker
            .join()
            .unwrap_or_else(|_| Err(io::Error::other("startup keepalive worker panicked")))
    }
}

impl Drop for StartupKeepalive {
    fn drop(&mut self) {
        let _ = self.stop();
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test(flavor = "current_thread")]
    async fn keepalives_continue_while_the_executor_is_blocked_and_stop_on_request() {
        let (sent, received) = mpsc::channel();
        let mut keepalive = StartupKeepalive::spawn(Duration::from_millis(10), move || {
            sent.send(()).unwrap();
            Ok(())
        })
        .unwrap();
        // Block the only executor thread, as a long synchronous startup step would.
        for _ in 0..3 {
            received.recv_timeout(Duration::from_secs(2)).unwrap();
        }
        keepalive.stop().unwrap();
        while received.try_recv().is_ok() {}
        assert_eq!(
            received.recv_timeout(Duration::from_millis(50)),
            Err(mpsc::RecvTimeoutError::Disconnected),
        );
        keepalive.stop().unwrap();
    }

    #[test]
    fn drop_joins_the_worker() {
        let (sent, received) = mpsc::channel();
        let keepalive = StartupKeepalive::spawn(Duration::from_secs(60), move || {
            sent.send(()).unwrap();
            Ok(())
        })
        .unwrap();
        received.recv_timeout(Duration::from_secs(2)).unwrap();
        drop(keepalive);
        assert_eq!(received.recv(), Err(mpsc::RecvError));
    }

    #[tokio::test]
    async fn a_broken_pipe_is_reported_with_its_error() {
        let mut keepalive = StartupKeepalive::spawn(Duration::from_secs(60), || {
            Err(io::Error::new(io::ErrorKind::BrokenPipe, "agent gone"))
        })
        .unwrap();
        let err = tokio::time::timeout(Duration::from_secs(2), keepalive.failure())
            .await
            .unwrap();
        assert_eq!(err.kind(), io::ErrorKind::BrokenPipe);
        assert!(keepalive.worker.is_none());
        keepalive.stop().unwrap();
    }

    #[tokio::test]
    async fn failure_stays_pending_after_a_clean_stop() {
        let mut keepalive = StartupKeepalive::spawn(Duration::from_secs(60), || Ok(())).unwrap();
        keepalive.stop().unwrap();
        assert!(
            tokio::time::timeout(Duration::from_millis(100), keepalive.failure())
                .await
                .is_err()
        );
    }
}
