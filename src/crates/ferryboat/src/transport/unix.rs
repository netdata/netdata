//! Unix-domain-socket transport: the Unix half of ferryboat's private
//! `transport` module, selected by `Endpoint::Ipc` (`windows.rs` is the
//! named-pipe counterpart). The layer is deliberately primitive — bind,
//! accept, connect, nothing else. Framing, size limits, serialization, and
//! connect retries live above it in `lib.rs`: streams are wrapped in
//! `Framed<ConnectionStream, LengthDelimitedCodec>` with `max_frame_length`
//! set to the configured message size (8 MiB default; the supervisor↔worker
//! links pin `bridge::IPC_MAX_MESSAGE_SIZE`, 16 MiB, on both ends, while the
//! writer→ledger link runs at the default), payloads are bincode with
//! optional LZ4, and `connect` makes a single attempt — the retry loop is
//! `ConnectionBuilder::connect_ipc_with_retry` (100 ms interval, 50 attempts
//! by default).
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
//!   per live listener (the supervisor names sockets `<worker>-<pid>.sock`).
//! - Dropping a `Listener` unlinks the path, best-effort. Connections
//!   accepted earlier keep working — the kernel socket is independent of the
//!   file — but the path stops accepting: later `connect`s fail with
//!   `NotFound`. Production callers drop the listener right after the first
//!   accept (`spawn_worker`, `accept_writer`).
//! - The supervisor's `SocketGuard` pre-removes and drop-removes the same
//!   paths around this — redundant but harmless, and it covers the case where
//!   ferryboat's `Listener` is never constructed (bind failure).
//!
//! Consumers reach this only through ferryboat's typed public API (the
//! `transport` module is private): the otel-plugin supervisor binds one
//! socket per worker and accepts, the workers (otel-ingestor, otel-ledger,
//! otel-legacy-logs) connect; the writer→ledger WAL-event link is bound by
//! `file_lifecycle::ipc::accept_writer` (called by otel-ledger) and connected
//! by the ingestor's `LedgerSender`, which retries forever. Message contracts
//! live in the `bridge` crate and `file_lifecycle::ipc`; socket naming and
//! child-process lifecycle in otel-plugin/src/supervisor.rs. Tests across the
//! family bind and connect over unique temp paths (PID-suffixed `/tmp` names
//! or `tempfile` dirs).
use std::io;
use std::path::{Path, PathBuf};

use tokio::net::{UnixListener, UnixStream};

/// The stream type behind every IPC `Connection`: a connected Unix-domain
/// socket. A single alias suffices because `UnixStream` is symmetric — both
/// ends of a link have the same type — unlike the Windows transport, whose
/// named-pipe server and client are distinct types (`ServerStream` /
/// `ClientStream` in `windows.rs`). `lib.rs` and `mux.rs` name this alias
/// unconditionally in their `Framed<ConnectionStream, _>` plumbing, which
/// therefore resolves on Unix only today.
pub type ConnectionStream = UnixStream;

/// Server half of a Unix-socket IPC link: a bound listening socket plus the
/// path it must unlink on drop. Wrapped by the typed `Listener<S, R>` in
/// `lib.rs` (`ListenerInner::Ipc`), which frames each accepted stream.
pub struct Listener {
    inner: UnixListener,
    // Kept only so `Drop` can unlink the socket file.
    path: PathBuf,
}

impl Listener {
    /// Binds a listening Unix socket at `path`, first unlinking whatever file
    /// is there (lifecycle and path-hijack caveat in the module docs).
    ///
    /// The parent directory must already exist; nothing here creates it (the
    /// supervisor's `socket_dir()` does; tests use unique temp paths).
    /// Errors are `UnixListener::bind`'s `io::Error`s verbatim, surfaced as
    /// `ferryboat::Error::Io` by `ListenerBuilder::open`.
    pub fn bind(path: impl AsRef<Path>) -> io::Result<Self> {
        let path = path.as_ref().to_path_buf();
        let _ = std::fs::remove_file(&path);
        let inner = UnixListener::bind(&path)?;
        Ok(Self { inner, path })
    }

    /// Waits for one incoming connection and returns its socket. The peer
    /// address is discarded: peers connecting to a path-bound socket are
    /// unnamed, so the bind path is the link's only identity.
    ///
    /// Takes `&self` — a Unix listening socket is OS state that outlives each
    /// accept — where the Windows transport needs `&mut self` because each
    /// accept consumes its named-pipe server instance.
    pub async fn accept(&self) -> io::Result<ConnectionStream> {
        let (stream, _) = self.inner.accept().await?;
        Ok(stream)
    }
}

// Unlinks the socket file. Errors are ignored: the file is usually already
// gone (pre-removed by a successor `bind` or the supervisor's `SocketGuard`),
// and this cleanup must not mask a shutdown in progress. Only the path goes
// away; connections accepted on `inner` stay open.
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
