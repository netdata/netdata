//! Unix-domain-socket transport.
//!
//! # Socket-path lifecycle
//!
//! - `bind` pre-removes any existing file at the path, ignoring removal
//!   errors: absence is the common case, and a file that survives removal
//!   (permissions, wrong type) fails `bind` with its own diagnosable error.
//!   The removal is required because `UnixListener::bind` refuses an existing
//!   path — a socket file outlives the process that created it.
//! - That pre-removal also means `bind` never reports an in-use path; it
//!   hijacks the path from any other live listener, so paths must be unique
//!   per live listener.
//! - Dropping a `Listener` unlinks the path, best-effort. Connections
//!   accepted earlier keep working — the kernel socket is independent of the
//!   file — but the path stops accepting: later `connect`s fail with
//!   `NotFound`.
use std::io;
use std::path::{Path, PathBuf};

use tokio::net::{UnixListener, UnixStream};

/// The stream type behind every IPC `Connection`: a connected Unix-domain
/// socket. A single alias suffices because `UnixStream` is symmetric: both
/// ends of a link have the same type.
pub type ConnectionStream = UnixStream;

/// Server half of a Unix-socket IPC link: a bound listening socket plus the
/// path it must unlink on drop.
pub struct Listener {
    inner: UnixListener,
    // Kept only so `Drop` can unlink the socket file.
    path: PathBuf,
}

impl Listener {
    /// Binds a listening Unix socket at `path`, first unlinking whatever file
    /// is there (lifecycle and path-hijack caveat in the module docs).
    ///
    /// The parent directory must already exist; nothing here creates it.
    pub fn bind(path: impl AsRef<Path>) -> io::Result<Self> {
        let path = path.as_ref().to_path_buf();
        let _ = std::fs::remove_file(&path);
        let inner = UnixListener::bind(&path)?;
        Ok(Self { inner, path })
    }

    /// Waits for one incoming connection and returns its socket. The peer
    /// address is discarded: peers connecting to a path-bound socket are
    /// unnamed, so the bind path is the link's only identity.
    pub async fn accept(&self) -> io::Result<ConnectionStream> {
        let (stream, _) = self.inner.accept().await?;
        Ok(stream)
    }
}

// Unlinks the socket file. Errors are ignored: the file may already be gone
// (pre-removed by a successor `bind`), and this cleanup must not mask a
// shutdown in progress. Only the path goes away; connections accepted on
// `inner` stay open.
impl Drop for Listener {
    fn drop(&mut self) {
        let _ = std::fs::remove_file(&self.path);
    }
}

/// Connects to a listener bound at `path`, one attempt, no retry — the retry
/// loop lives in `ConnectionBuilder::connect_ipc_with_retry`.
///
/// Fails with `io::ErrorKind::NotFound` when no listener owns the path,
/// including once a listener's drop has unlinked it (module docs).
pub async fn connect(path: impl AsRef<Path>) -> io::Result<ConnectionStream> {
    UnixStream::connect(path.as_ref()).await
}
