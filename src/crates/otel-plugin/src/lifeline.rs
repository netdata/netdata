//! A worker's lifeline to the supervisor.
//!
//! The supervisor gives each worker a pipe as stdin and never writes to it
//! (`supervisor::spawn_worker`). The worker reads end-of-file from the pipe
//! only when every copy of its write end is closed, and the supervisor holds
//! the only one. So end-of-file means the supervisor is gone, however it
//! ended, and the worker exits.
//!
//! Without it, a worker outlives a supervisor that is killed. Nothing else
//! tells it: workers ignore SIGINT and SIGTERM (`run_worker` in `main.rs`),
//! and the ledger does not read its supervisor connection while it starts.
//! An orphaned ledger keeps working on its directory while the agent starts
//! the next plugin instance on the same one.
//!
//! The watcher is an OS thread, not a task, so that it also works while the
//! worker's runtime is busy starting.

use std::io::{self, Read};

/// Exit this worker process when its stdin closes.
pub(crate) fn exit_when_stdin_closes() -> io::Result<()> {
    watch(io::stdin(), || {
        tracing::warn!("the supervisor is gone (lifeline closed), exiting");
        std::process::exit(1);
    })
}

/// Run `on_close` on a thread of its own once `reader` reports end-of-file
/// or fails.
fn watch<R, F>(mut reader: R, on_close: F) -> io::Result<()>
where
    R: Read + Send + 'static,
    F: FnOnce() + Send + 'static,
{
    let watcher = move || {
        let mut buf = [0u8; 64];

        loop {
            match reader.read(&mut buf) {
                // The supervisor never writes; whatever arrives is ignored.
                Ok(n) if n > 0 => {}
                Err(e) if e.kind() == io::ErrorKind::Interrupted => {}
                // End-of-file, or an error that will not go away.
                Ok(_) | Err(_) => break,
            }
        }

        on_close();
    };

    std::thread::Builder::new()
        .name("otel-lifeline".into())
        .spawn(watcher)?;

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::mpsc;
    use std::time::Duration;

    #[test]
    fn fires_only_after_the_write_end_closes() {
        let (reader, mut writer) = io::pipe().unwrap();
        let (closed, on_close) = mpsc::channel();
        watch(reader, move || closed.send(()).unwrap()).unwrap();

        // Open, and even written to: the watcher waits.
        io::Write::write_all(&mut writer, b"x").unwrap();
        assert_eq!(
            on_close.recv_timeout(Duration::from_millis(100)),
            Err(mpsc::RecvTimeoutError::Timeout),
            "the lifeline is still open"
        );

        drop(writer);
        on_close
            .recv_timeout(Duration::from_secs(5))
            .expect("end-of-file runs the close action");
    }
}
