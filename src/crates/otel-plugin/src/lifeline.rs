//! A worker's lifeline to the supervisor: the supervisor gives each worker a
//! pipe as stdin and never writes to it, so the worker reads end-of-file only
//! when the supervisor process is gone (its last copy of the write end closed).
//!
//! The watcher is an OS thread that exits the process on that end-of-file, so
//! it works while the worker's runtime is busy (startup recovery, the store
//! lock wait) and on every platform, unlike Linux's `PR_SET_PDEATHSIG`, which
//! `supervisor::spawn_worker` keeps as the immediate kill where it exists.

use std::io::{self, Read};
use std::thread;

/// Exit this worker when its stdin (the supervisor's lifeline) closes.
pub(crate) fn exit_when_stdin_closes() -> io::Result<()> {
    watch(io::stdin(), || {
        tracing::warn!("the supervisor is gone (lifeline closed), exiting");
        std::process::exit(1);
    })
}

/// Run `on_close` on a thread once `reader` reports end-of-file or fails.
fn watch<R, F>(mut reader: R, on_close: F) -> io::Result<()>
where
    R: Read + Send + 'static,
    F: FnOnce() + Send + 'static,
{
    thread::Builder::new()
        .name("otel-lifeline".into())
        .spawn(move || {
            let mut buf = [0u8; 64];
            loop {
                match reader.read(&mut buf) {
                    Ok(0) => break,
                    // The supervisor never writes; ignore anything that arrives.
                    Ok(_) => {}
                    Err(e) if e.kind() == io::ErrorKind::Interrupted => {}
                    Err(_) => break,
                }
            }
            on_close();
        })
        .map(|_| ())
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::mpsc;
    use std::time::Duration;

    #[test]
    fn fires_only_after_the_write_end_closes() {
        let (reader, writer) = io::pipe().unwrap();
        let (closed, on_close) = mpsc::channel();
        watch(reader, move || closed.send(()).unwrap()).unwrap();

        assert_eq!(
            on_close.recv_timeout(Duration::from_millis(100)),
            Err(mpsc::RecvTimeoutError::Timeout),
            "the lifeline is still open"
        );
        drop(writer);
        on_close
            .recv_timeout(Duration::from_secs(2))
            .expect("end-of-file runs the close action");
    }
}
