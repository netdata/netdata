//! Compile-time platform selection for ferryboat's IPC transport
//! ([`Endpoint::Ipc`](crate::Endpoint::Ipc)): the `unix` submodule (Unix
//! domain sockets) or the `windows` submodule (named pipes). Each exposes the
//! same surface — a `Listener` with `bind`/`accept` and an async `connect`
//! that makes a single connection attempt — plus its stream type. Connection
//! retry policy lives above, in `ConnectionBuilder`.
//!
//! This layer only provides raw byte streams. Bincode serialization, LZ4
//! compression, length-prefixed framing and message-size limits are applied
//! in `lib.rs`/`mux.rs`; everything here returns `std::io::Result`, which the
//! connection layer surfaces as [`Error::Io`](crate::Error::Io).
//!
//! Socket lifecycle: on Unix, `Listener::bind` removes a stale socket file at
//! the target path before binding and `Drop` removes it again; on Windows,
//! `first_pipe_instance(true)` rejects a second binder and the OS reclaims
//! the pipe when its last handle closes. Per-run socket hygiene beyond that
//! (PID-suffixed names, drop-time removal) is layered by callers — in this
//! codebase the otel-plugin supervisor does it with `SocketGuard`
//! (otel-plugin/src/supervisor.rs).
//!
//! Platform asymmetry: `unix` exports one `ConnectionStream` (UnixStream)
//! shared by both connection ends, and its `accept` takes `&self`; `windows`
//! exports separate `ServerStream`/`ClientStream` named-pipe types and its
//! `accept` takes `&mut self` because it swaps in a fresh pipe instance per
//! client. `lib.rs` and `mux.rs` reference `ConnectionStream`
//! unconditionally, so as written only the Unix selection provides the name
//! the crate's use sites need.
//!
//! Crate-private (`mod transport` in `lib.rs`): used only by `lib.rs` and
//! `mux.rs`. External ferryboat users (the otel-plugin supervisor and its
//! worker crates otel-ingestor, otel-ledger and otel-legacy-logs, plus
//! file-lifecycle's WAL→ledger link) go through the public
//! `Connection`/`Listener`/`Endpoint` API.
#[cfg(unix)]
mod unix;
#[cfg(unix)]
pub use self::unix::*;

#[cfg(windows)]
mod windows;
#[cfg(windows)]
pub use self::windows::*;
