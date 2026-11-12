//! Test-only cleanup of endpoint lock files.
//!
//! Lock files stay after a server closes, by design
//! (`docs/level1-posix-uds.md`, "Endpoint lock"). Test service names are
//! unique per run, so without this every run would leave new locks behind:
//! the test process removes the locks it named when it exits.

use std::sync::Mutex;
use std::sync::Once;
use std::sync::atomic::{AtomicU32, Ordering};

static PATHS: Mutex<Vec<String>> = Mutex::new(Vec::new());
static REGISTER: Once = Once::new();
static OWNER_PID: AtomicU32 = AtomicU32::new(0);

extern "C" fn remove_tracked_locks() {
    // A forked child inherits the handler; only the process that named the
    // services removes their locks.
    if std::process::id() != OWNER_PID.load(Ordering::Relaxed) {
        return;
    }
    let paths = PATHS.lock().unwrap_or_else(|e| e.into_inner());
    for path in paths.iter() {
        let _ = std::fs::remove_file(path);
    }
}

/// Records `{dir}/{service}.lock` for removal at process exit.
pub(crate) fn track(dir: &str, service: &str) {
    REGISTER.call_once(|| {
        OWNER_PID.store(std::process::id(), Ordering::Relaxed);
        // SAFETY: registers a plain extern "C" function with no captured state.
        unsafe {
            libc::atexit(remove_tracked_locks);
        }
    });
    PATHS
        .lock()
        .unwrap_or_else(|e| e.into_inner())
        .push(format!("{dir}/{service}.lock"));
}
