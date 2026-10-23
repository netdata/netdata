use std::io::{self, Write};
use std::sync::mpsc::{self, RecvTimeoutError};
use std::thread::{self, JoinHandle};
use std::time::Duration;
use tokio::sync::oneshot;

/// Startup can block every executor worker. Keep heartbeats independent until
/// publication hands stdout ownership to PluginRuntime.
pub(crate) struct StartupKeepalive {
    stop: mpsc::Sender<()>,
    worker: Option<JoinHandle<io::Result<()>>>,
    failed: oneshot::Receiver<()>,
}

impl StartupKeepalive {
    pub(crate) fn start() -> io::Result<Self> {
        Self::spawn(Duration::from_secs(60), || {
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
            .name("netflow-startup-keepalive".into())
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

    pub(crate) async fn failure(&mut self) -> io::Error {
        let _ = (&mut self.failed).await;
        self.stop()
            .err()
            .unwrap_or_else(|| io::Error::other("startup keepalive worker stopped unexpectedly"))
    }

    /// Join before publishing so a heartbeat write cannot overlap the handoff
    /// or be cancelled partway through a protocol line.
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
    async fn keepalives_continue_while_the_executor_is_blocked_and_stop_at_handoff() {
        let (sent, received) = mpsc::channel();
        let mut keepalive = StartupKeepalive::spawn(Duration::from_millis(10), move || {
            sent.send(()).unwrap();
            Ok(())
        })
        .unwrap();
        // Deliberately block the only executor thread, just like synchronous
        // cache loading or a tier commit during initialization.
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
    fn early_return_drops_and_joins_keepalive_worker() {
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
    async fn broken_writer_preserves_the_error_and_joins_worker() {
        let mut keepalive = StartupKeepalive::spawn(Duration::from_secs(60), || {
            Err(io::Error::new(
                io::ErrorKind::BrokenPipe,
                "test disconnected Agent",
            ))
        })
        .unwrap();
        let err = tokio::time::timeout(Duration::from_secs(2), keepalive.failure())
            .await
            .unwrap();
        assert_eq!(err.kind(), io::ErrorKind::BrokenPipe);
        assert!(err.to_string().contains("test disconnected Agent"));
        assert!(keepalive.worker.is_none());
        keepalive.stop().unwrap();
    }

    #[test]
    fn handoff_reports_write_failure_even_if_failure_future_was_not_polled() {
        let (sent, received) = mpsc::channel();
        let mut keepalive = StartupKeepalive::spawn(Duration::from_secs(60), move || {
            sent.send(()).unwrap();
            Err(io::Error::new(
                io::ErrorKind::BrokenPipe,
                "test write failure",
            ))
        })
        .unwrap();
        received.recv_timeout(Duration::from_secs(2)).unwrap();
        assert_eq!(
            keepalive.stop().unwrap_err().kind(),
            io::ErrorKind::BrokenPipe
        );
    }
}
