//! Windows named-pipe transport: the `#[cfg(windows)]` half of ferryboat's
//! private IPC transport ([`Endpoint::Ipc`](crate::Endpoint::Ipc)). The `unix`
//! submodule is the other half and shares this module's `Listener` +
//! `connect` surface shape. Only raw byte streams live here; serialization,
//! compression, framing and size limits are layered in `lib.rs`/`mux.rs`, and
//! every failure is a `std::io::Result`, surfaced as
//! [`Error::Io`](crate::Error::Io).
//!
//! Naming and lifecycle: paths pass through to the OS verbatim, so callers
//! supply the full pipe name (`\\.\pipe\...`). Named pipes have no
//! filesystem entry — nothing to unlink, so `Listener` needs no `Drop` — and
//! the OS reclaims the name when its last handle closes. `Listener::bind`
//! creates the first instance with `first_pipe_instance(true)`, which fails
//! with `ErrorKind::PermissionDenied` while any instance of that name exists,
//! so concurrent or leftover binders fail fast instead of sharing the pipe.
//! Replacement instances created by `accept` omit that flag on purpose: the
//! name has live instances by then.
//!
//! `accept` runs the per-client instance lifecycle, which is why it takes
//! `&mut self`: the listener holds exactly one waiting instance between
//! accepts; `accept` waits for a client (succeeding immediately if one is
//! already connected), hands the connected instance out, and creates the next
//! waiting instance in the same call. During that hand-over there is no
//! waiting instance, so a client opening inside the window fails instead of
//! queuing. If creating the replacement fails, `accept` returns `Err` without
//! handing anything out and the connected instance stays in the listener; the
//! next `accept` returns it immediately.
//!
//! The two ends are distinct tokio types (`NamedPipeServer`, `NamedPipeClient`
//! — both `Send` + `Sync`, byte-mode, remote clients rejected by default), so
//! each side of a connection gets its own stream type instead of the single
//! `ConnectionStream` the Unix half exports — and the crate's plumbing is
//! written for the Unix shape: this module exports `ServerStream`,
//! `ClientStream`, `Listener` and `connect` but not `ConnectionStream`, while
//! `lib.rs` and `mux.rs` use `ConnectionStream` unconditionally (`lib.rs`:
//! the `use transport::{ConnectionStream, ..}` import plus the
//! `Framed<ConnectionStream, ..>` field and the `Result<ConnectionStream>`
//! return of the IPC connect helper; `mux.rs`: the
//! `use crate::transport::ConnectionStream;` import behind `Writer::Ipc` and
//! `Reader::Ipc`). No `#[cfg]` gate covers those uses, so on Windows the crate
//! does not compile as written and only the Unix selection builds; the
//! breakage is invisible to non-Windows builds, which never type-check this
//! module.
//!
//! Crate-private (`mod transport` in `lib.rs`): consumed only by `lib.rs` and
//! `mux.rs`; ferryboat's plugin users go through the public
//! [`Connection`](crate::Connection)/[`Listener`](crate::Listener) API.
use std::io;
use std::path::{Path, PathBuf};

use tokio::net::windows::named_pipe::{
    ClientOptions, NamedPipeClient, NamedPipeServer, ServerOptions,
};

/// Server-side end of one accepted client connection, as returned by
/// [`Listener::accept`].
pub type ServerStream = NamedPipeServer;
/// Client-side end of a named-pipe connection, as returned by [`connect`].
pub type ClientStream = NamedPipeClient;

/// Server side of the named-pipe transport. Holds the current waiting pipe
/// instance and the path it is created at, so
/// [`accept`](Listener::accept) can install the next instance.
pub struct Listener {
    server: NamedPipeServer,
    path: PathBuf,
}

impl Listener {
    /// Creates the first pipe instance at `path` and takes ownership of it.
    /// Fails with `ErrorKind::PermissionDenied` while any instance of the name
    /// exists — another binder, or a still-running previous process — since
    /// named pipes leave no file behind to clean up.
    pub fn bind(path: impl AsRef<Path>) -> io::Result<Self> {
        let path = path.as_ref().to_path_buf();
        let server = ServerOptions::new()
            .first_pipe_instance(true)
            .create(&path)?;
        Ok(Self { server, path })
    }

    /// Waits for a client to connect to the waiting instance, returns the
    /// connected server end, and installs a fresh waiting instance at the same
    /// path for the next client. The replacement is created with default
    /// options; it must not set `first_pipe_instance`, which would fail while
    /// live instances exist. If creating it fails, the connected instance
    /// stays in the listener and the next call returns it immediately.
    pub async fn accept(&mut self) -> io::Result<ServerStream> {
        // Succeeds immediately if a client connected before this call.
        self.server.connect().await?;

        // Hand the connected instance out and create the next waiting
        // instance so the following client has somewhere to connect.
        let connected =
            std::mem::replace(&mut self.server, ServerOptions::new().create(&self.path)?);
        Ok(connected)
    }
}

/// Opens the client end of the pipe in one synchronous attempt. Fails when no
/// server instance is available to connect to at `path`; retries live above,
/// in [`ConnectionBuilder`](crate::ConnectionBuilder).
pub async fn connect(path: impl AsRef<Path>) -> io::Result<ClientStream> {
    // The open is a synchronous OS call, not an await; the async
    // signature only matches the Unix transport.
    ClientOptions::new().open(path.as_ref())
}
