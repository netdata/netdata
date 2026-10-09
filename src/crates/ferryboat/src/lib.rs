//! A transport-agnostic messaging library for inter-process and in-process
//! communication under the OTel plugin family.
//!
//! [`Listener`] accepts bidirectional [`Connection`]s; a connection sends
//! values of type `S` and receives values of type `R`, and the two ends of a
//! link mirror each other's type parameters. Switching from in-process to
//! cross-process (or vice versa) is a single [`Endpoint`] change —
//! application logic stays the same.
//!
//! # Transports
//!
//! - [`Endpoint::InProcess`] — a named acceptor in a process-global registry
//!   (see `registry()` below). Values move through bounded tokio mpsc
//!   channels, so nothing is serialized at runtime (the serde bounds are
//!   still required by the API). The listener registers its acceptor at
//!   `open()` and removes it again on drop; a connecting client polls the
//!   registry with the builder's retry settings until the name appears.
//! - [`Endpoint::Ipc`] — Unix domain sockets (Unix) or named pipes (Windows).
//!   Each message is bincode-serialized, optionally LZ4-compressed, and sent
//!   as one length-delimited frame, so both ends of a link must agree on the
//!   compression flag and the message-size limit, and the serde types are
//!   wire contracts (see "Size limits and framing" below).
//!
//! # Example
//!
//! ```no_run
//! use ferryboat::{Connection, Endpoint, Listener};
//! use serde::{Deserialize, Serialize};
//! use std::time::Duration;
//!
//! #[derive(Serialize, Deserialize)]
//! struct Request {
//!     query: String,
//! }
//!
//! #[derive(Serialize, Deserialize)]
//! struct Response {
//!     answer: u64,
//! }
//!
//! # #[tokio::main] async fn main() -> ferryboat::Result<()> {
//! let endpoint = Endpoint::ipc("/tmp/my.sock");
//!
//! // Server side
//! let mut listener = Listener::<Response, Request>::bind(endpoint.clone())
//!     .compress(true)
//!     .open()?;
//!
//! tokio::spawn(async move {
//!     let mut conn = listener.accept().await.unwrap();
//!     let req = conn.recv().await.unwrap();
//!     conn.send(Response { answer: 42 }).await.unwrap();
//! });
//!
//! // Client side (typically in another process)
//! let mut client = Connection::<Request, Response>::connect(endpoint)
//!     .retry_interval(Duration::from_millis(200))
//!     .compress(true)
//!     .open()
//!     .await?;
//!
//! client.send(Request { query: "meaning of life".into() }).await?;
//! let resp = client.recv().await?;
//! assert_eq!(resp.answer, 42);
//! # Ok(())
//! # }
//! ```
//!
//! # Multiplexed RPC
//!
//! [`RpcClient`] and [`RpcServer`] add a request-response layer on top of
//! [`Connection`]. Multiple requests can be in-flight concurrently, and
//! responses are matched by ID so handlers can complete out of order.
//!
//! Shared types:
//!
//! ```
//! use serde::{Deserialize, Serialize};
//!
//! #[derive(Serialize, Deserialize)]
//! struct Ping {
//!     seq: u64,
//! }
//!
//! #[derive(Serialize, Deserialize)]
//! struct Pong {
//!     seq: u64,
//! }
//! ```
//!
//! Server binary:
//!
//! ```no_run
//! # use ferryboat::{Endpoint, RpcServer};
//! # use serde::{Deserialize, Serialize};
//! # #[derive(Serialize, Deserialize)] struct Ping { seq: u64 }
//! # #[derive(Serialize, Deserialize)] struct Pong { seq: u64 }
//! struct PingService;
//!
//! impl PingService {
//!     async fn run(mut server: RpcServer<Ping, Pong>) -> ferryboat::Result<()> {
//!         loop {
//!             let session = server.accept().await?;
//!             tokio::spawn(session.serve(|ping| async move { Pong { seq: ping.seq } }));
//!         }
//!     }
//! }
//!
//! #[tokio::main]
//! async fn main() -> ferryboat::Result<()> {
//!     let server = RpcServer::<Ping, Pong>::bind(Endpoint::ipc("/tmp/rpc.sock")).open()?;
//!     PingService::run(server).await
//! }
//! ```
//!
//! Client binary:
//!
//! ```no_run
//! # use ferryboat::{Endpoint, RpcClient};
//! # use serde::{Deserialize, Serialize};
//! # #[derive(Serialize, Deserialize)] struct Ping { seq: u64 }
//! # #[derive(Serialize, Deserialize)] struct Pong { seq: u64 }
//! #[tokio::main]
//! async fn main() -> ferryboat::Result<()> {
//!     let client = RpcClient::<Ping, Pong>::connect(Endpoint::ipc("/tmp/rpc.sock"))
//!         .open()
//!         .await?;
//!
//!     // Multiple concurrent requests
//!     let (a, b) = tokio::join!(
//!         client.call(Ping { seq: 1 }),
//!         client.call(Ping { seq: 2 }),
//!     );
//!     assert_eq!(a?.seq, 1);
//!     assert_eq!(b?.seq, 2);
//!
//!     Ok(())
//! }
//! ```
//!
//! # Size limits and framing
//!
//! IPC messages cross the wire as one `tokio_util::codec::LengthDelimitedCodec`
//! frame: a 4-byte big-endian length prefix followed by the payload (the
//! tokio-util defaults). The payload is the `bincode::config::standard()`
//! encoding of the value, optionally LZ4-compressed with a prepended size —
//! so enum variant order, field shapes, and the compression flag are
//! cross-process contracts: both ends of a link must be built from the same
//! type definitions and the same builder settings. The supervisor↔worker
//! message contracts live in `netdata-plugin/bridge/src/lib.rs`; the socket
//! lifecycle (bind, accept, stale-file removal, pipe instance handling) in
//! `transport/unix.rs` and `transport/windows.rs`.
//!
//! The message-size limit is enforced at three points keyed to the same
//! `max_message_size` value: at serialize time (an oversized send fails with
//! `Error::MessageTooLarge` before any byte is written), by the codec's
//! `max_frame_length` in both directions, and after decompression on receive.
//! Both ends must set the same limit: a mismatch means one side accepts
//! frames the other refuses, and the link dies on the first oversized
//! message. Production usage: the supervisor↔worker links set
//! `bridge::IPC_MAX_MESSAGE_SIZE` (16 MiB) on both ends; the ingestor→ledger
//! WAL-event link runs at this crate's 8 MiB default on both ends. On the
//! compressed path the check lands after decompression — `lz4_flex` allocates
//! the embedded size up front — so the limit bounds the accepted message, not
//! the peak allocation.
//!
//! There is no internal send queue: `send` awaits the bounded in-process
//! channel or the socket, so a reader that stops draining applies
//! backpressure to the sender (over IPC, via the kernel socket buffer).
//!
//! # Errors
//!
//! [`Connection::send`] fails with `Error::Encode` or `Error::MessageTooLarge`
//! before touching the socket (IPC), or `Error::ConnectionClosed` when the
//! in-process peer is gone; transport failures surface as `Error::Io`.
//! [`Connection::recv`] fails with `Error::Decode`, `Error::MessageTooLarge`
//! (post-decompress) or `Error::Io` — a compression-setting mismatch between
//! the two ends and an oversized frame both arrive as `Error::Io` — and
//! returns `Error::ConnectionClosed` when the peer closed the connection
//! cleanly. Connect retry exhaustion returns the last `Error::Io` (IPC) or
//! `Error::ConnectionClosed` (in-process); `Error::TypeMismatch` is returned
//! immediately, never retried.
//!
//! # Lifecycle and thread-safety
//!
//! Builders are plain values consumed by `open()`: `ListenerBuilder::open` is
//! synchronous (bind or registry insert only), `ConnectionBuilder::open` is
//! async (it retries until the peer is reachable or the retry budget is
//! spent). `Connection` and `Listener` methods take `&mut self`, so each
//! belongs to one task; everything runs on tokio (the Windows named-pipe open
//! inside `transport::connect` is the one synchronous call), and the
//! in-process registry's `std::sync::Mutex` is never held across an await.
//! Dropping a `Connection` closes the channel or socket; the peer observes
//! that as `Error::ConnectionClosed` on the next recv (and on in-process
//! sends).
//!
//! # Consumers in this tree
//!
//! Direct dependents (workspace `src/crates/Cargo.toml`): `otel-plugin` — the
//! supervisor binds one socket per worker and spawns them
//! (`src/crates/otel-plugin/src/supervisor.rs`); the worker crates
//! `otel-ingestor`, `otel-ledger` and `otel-legacy-logs`, which connect back
//! to the supervisor and speak the `bridge` enum pairs; and `file-lifecycle`
//! (`ipc.rs`), whose ledger accepts the ingestor's WAL-event connection
//! carrying `wal::Message` (`src/crates/otel-ingestor/src/ledger_sender.rs`
//! is the connecting side). The `bridge` crate defines the message contracts
//! but does not depend on ferryboat; `file-registry`, `otel-catalog`, `wal`,
//! `sfst` and `sfsq` do not use ferryboat directly. The [`RpcClient`] /
//! [`RpcServer`] layer and the in-process transport have no production users
//! in this tree yet — they are exercised by this crate's tests and examples.

mod mux;
mod transport;

use std::any::Any;
use std::collections::HashMap;
use std::marker::PhantomData;
use std::path::PathBuf;
use std::sync::{Mutex, OnceLock};
use std::time::Duration;

use bytes::Bytes;
use futures::{SinkExt, StreamExt};
use serde::{Serialize, de::DeserializeOwned};
use tokio::sync::mpsc;
use tokio_util::codec::{Framed, LengthDelimitedCodec};

use transport::{ConnectionStream, Listener as TransportListener};

pub use mux::{RpcClient, RpcClientBuilder, RpcServer, RpcServerBuilder, RpcSession};

/// Default IPC message-size limit (8 MiB); see the module docs for the three
/// enforcement points. Supervisor↔worker links override it with
/// `bridge::IPC_MAX_MESSAGE_SIZE`.
const DEFAULT_MAX_MESSAGE_SIZE: usize = 8 * 1024 * 1024;

// Capacity of the mpsc channels backing in-process connections.
const IN_PROCESS_CHANNEL_BUFFER: usize = 128;

// Capacity of the accept queue for in-process listeners.
const DEFAULT_ACCEPT_CAPACITY: usize = 64;

/// Codec for IPC links: each message is one length-delimited frame (4-byte
/// big-endian prefix, tokio-util default), with `max_frame_length` set to the
/// configured message-size limit.
fn build_codec(max_message_size: usize) -> LengthDelimitedCodec {
    LengthDelimitedCodec::builder()
        .max_frame_length(max_message_size)
        .new_codec()
}

/// Serializes one IPC message: `bincode::config::standard()` encoding,
/// optionally LZ4-compressed with a prepended size. The size check runs
/// against the final (post-compression) length, so an oversized message fails
/// with [`Error::MessageTooLarge`] before any byte reaches the wire.
pub(crate) fn serialize_ipc<T: Serialize>(
    msg: &T,
    compress: bool,
    max_message_size: usize,
) -> Result<Bytes> {
    let data = bincode::serde::encode_to_vec(msg, bincode::config::standard())?;
    let data = if compress {
        lz4_flex::compress_prepend_size(&data)
    } else {
        data
    };
    if data.len() > max_message_size {
        return Err(Error::MessageTooLarge {
            size: data.len(),
            max: max_message_size,
        });
    }
    Ok(Bytes::from(data))
}

/// Deserializes one IPC message: inverse of [`serialize_ipc`]. Decompression
/// failure (the two ends disagree on `compress`, or the frame is corrupt)
/// maps to `Error::Io` (`InvalidData`); the decompressed payload is
/// size-checked after decompression — the compressed frame itself was already
/// bounded by the codec.
pub(crate) fn deserialize_ipc<T: DeserializeOwned>(
    data: &[u8],
    compress: bool,
    max_message_size: usize,
) -> Result<T> {
    let payload = if compress {
        let decompressed = lz4_flex::decompress_size_prepended(data).map_err(|_| {
            Error::Io(std::io::Error::new(
                std::io::ErrorKind::InvalidData,
                "LZ4 decompression failed \
                 (possible compression setting mismatch between client and server)",
            ))
        })?;
        if decompressed.len() > max_message_size {
            return Err(Error::MessageTooLarge {
                size: decompressed.len(),
                max: max_message_size,
            });
        }
        std::borrow::Cow::Owned(decompressed)
    } else {
        std::borrow::Cow::Borrowed(data)
    };
    let (val, _len) = bincode::serde::decode_from_slice(&payload, bincode::config::standard())?;
    Ok(val)
}

/// Process-global registry of in-process channels: channel name → acceptor.
/// `ListenerBuilder::open` inserts an entry — replacing any existing one
/// under the same name — and `Listener`'s `Drop` removes it;
/// `ConnectionBuilder::connect_in_process` polls this map with the builder's
/// retry settings until the name appears.
fn registry() -> &'static Mutex<HashMap<String, Box<dyn Any + Send + Sync>>> {
    static INSTANCE: OnceLock<Mutex<HashMap<String, Box<dyn Any + Send + Sync>>>> = OnceLock::new();
    INSTANCE.get_or_init(|| Mutex::new(HashMap::new()))
}

// --- Error type ---

/// Errors returned by [`Connection::send`] and [`Connection::recv`].
#[derive(Debug, thiserror::Error)]
pub enum Error {
    /// Transport-level I/O error: socket failures, codec frame-limit
    /// rejections, and the LZ4 decompression mismatch from
    /// `deserialize_ipc`. Transparent (as are the bincode error variants
    /// below): embedding the source in the message while also chaining it
    /// would print it twice in anyhow chains.
    #[error(transparent)]
    Io(#[from] std::io::Error),

    /// Failed to serialize a message.
    #[error(transparent)]
    Encode(#[from] bincode::error::EncodeError),

    /// Failed to deserialize a message.
    #[error(transparent)]
    Decode(#[from] bincode::error::DecodeError),

    /// The other side of the connection has been closed. Also returned when
    /// an in-process connect exhausts its retry budget (IPC returns the last
    /// `Error::Io` instead).
    #[error("connection closed")]
    ConnectionClosed,

    /// Serialized message exceeds the configured limit — checked after
    /// compression on send and after decompression on recv.
    #[error("message too large: {size} bytes exceeds {max} byte limit")]
    MessageTooLarge {
        /// Actual serialized size in bytes.
        size: usize,
        /// Configured maximum in bytes.
        max: usize,
    },

    /// An in-process channel with the same name was bound with a different type.
    #[error("type mismatch: channel '{0}' was bound with a different message type")]
    TypeMismatch(String),
}

/// Convenience alias for `std::result::Result<T, ferryboat::Error>`.
pub type Result<T> = std::result::Result<T, Error>;

// --- Endpoint ---

/// Selects the transport for a [`Connection`] or [`Listener`].
#[derive(Clone)]
pub enum Endpoint {
    /// In-memory channel identified by name. No serialization overhead.
    InProcess(String),
    /// Unix domain socket (Unix) or named pipe (Windows) at the given path.
    Ipc(PathBuf),
}

impl Endpoint {
    /// Creates an in-process endpoint with the given channel name.
    pub fn in_process(name: impl Into<String>) -> Self {
        Endpoint::InProcess(name.into())
    }

    /// Creates an IPC endpoint at the given socket/pipe path.
    pub fn ipc(path: impl Into<PathBuf>) -> Self {
        Endpoint::Ipc(path.into())
    }
}

// --- Connection ---

/// Send half of an in-process listener's accept queue: each connecting client
/// deposits a `(Sender<S>, Receiver<R>)` pair — the listener's send and
/// receive halves — which `Listener::accept` takes as one accepted
/// connection.
struct InProcessAcceptor<S, R> {
    tx: mpsc::Sender<(mpsc::Sender<S>, mpsc::Receiver<R>)>,
}

/// A bidirectional connection that can send messages of type `S` and receive
/// messages of type `R`.
///
/// For RPC, a client typically creates `Connection<Req, Resp>` (sends
/// requests, receives responses), while the server's accepted connections are
/// `Connection<Resp, Req>` (sends responses, receives requests).
///
/// Methods take `&mut self`, so a connection belongs to one task. Dropping it
/// closes the channel or socket; the peer observes that as
/// [`Error::ConnectionClosed`] on the next recv (and on in-process sends).
pub struct Connection<S, R> {
    pub(crate) inner: ConnectionInner<S, R>,
}

pub(crate) enum ConnectionInner<S, R> {
    InProcess {
        tx: mpsc::Sender<S>,
        rx: mpsc::Receiver<R>,
    },
    Ipc {
        framed: Framed<ConnectionStream, LengthDelimitedCodec>,
        max_message_size: usize,
        compress: bool,
        _phantom: PhantomData<(S, R)>,
    },
}

impl<S, R> Connection<S, R>
where
    S: Serialize + Send + 'static,
    R: DeserializeOwned + Send + 'static,
{
    /// Returns a [`ConnectionBuilder`] that will connect to a [`Listener`]
    /// bound at the given endpoint.
    pub fn connect(endpoint: Endpoint) -> ConnectionBuilder<S, R> {
        ConnectionBuilder {
            endpoint,
            retry_interval: Duration::from_millis(100),
            max_retries: Some(50),
            max_message_size: DEFAULT_MAX_MESSAGE_SIZE,
            compress: false,
            _phantom: PhantomData,
        }
    }

    /// Sends a message to the remote side.
    ///
    /// For in-process connections the value is moved directly into the
    /// channel. For IPC connections it is bincode-serialized (and optionally
    /// LZ4-compressed) and written as one frame; an oversized message fails
    /// with `Error::MessageTooLarge` before any byte is written.
    pub async fn send(&mut self, msg: S) -> Result<()> {
        match &mut self.inner {
            ConnectionInner::InProcess { tx, .. } => {
                tx.send(msg).await.map_err(|_| Error::ConnectionClosed)
            }
            ConnectionInner::Ipc {
                framed,
                max_message_size,
                compress,
                ..
            } => {
                let data = serialize_ipc(&msg, *compress, *max_message_size)?;
                framed.send(data).await?;
                Ok(())
            }
        }
    }

    /// Waits for the next message from the remote side.
    ///
    /// Returns `Error::ConnectionClosed` once the peer has closed the
    /// connection (channel closed in-process, socket EOF over IPC).
    pub async fn recv(&mut self) -> Result<R> {
        match &mut self.inner {
            ConnectionInner::InProcess { rx, .. } => rx.recv().await.ok_or(Error::ConnectionClosed),
            ConnectionInner::Ipc {
                framed,
                compress,
                max_message_size,
                ..
            } => {
                let data = framed
                    .next()
                    .await
                    .ok_or(Error::ConnectionClosed)?
                    .map_err(Error::Io)?;
                deserialize_ipc(&data, *compress, *max_message_size)
            }
        }
    }

    /// Sends a message and waits for the response.
    ///
    /// Strict request/response: the next `recv` must be the reply to this
    /// send. For several requests in flight over one connection, use
    /// [`RpcClient`] instead.
    pub async fn call(&mut self, msg: S) -> Result<R> {
        self.send(msg).await?;
        self.recv().await
    }
}

/// Builder for [`Connection`]. Created by [`Connection::connect`].
pub struct ConnectionBuilder<S, R> {
    endpoint: Endpoint,
    retry_interval: Duration,
    max_retries: Option<usize>,
    max_message_size: usize,
    compress: bool,
    _phantom: PhantomData<(S, R)>,
}

impl<S, R> ConnectionBuilder<S, R>
where
    S: Serialize + Send + 'static,
    R: DeserializeOwned + Send + 'static,
{
    /// Delay between connection attempts (default: 100ms).
    pub fn retry_interval(mut self, interval: Duration) -> Self {
        self.retry_interval = interval;
        self
    }

    /// Maximum connection attempts before giving up (default: 50).
    /// Use `None` to retry indefinitely.
    pub fn max_retries(mut self, max: Option<usize>) -> Self {
        self.max_retries = max;
        self
    }

    /// Max allowed message size in bytes (default 8 MiB). IPC only; both
    /// ends of a link must set the same value.
    pub fn max_message_size(mut self, size: usize) -> Self {
        self.max_message_size = size;
        self
    }

    /// Whether to LZ4-compress payloads (default: false). IPC only; both
    /// ends must agree, or recv fails with `Error::Io`.
    pub fn compress(mut self, compress: bool) -> Self {
        self.compress = compress;
        self
    }

    /// Connects and returns the [`Connection`].
    pub async fn open(self) -> Result<Connection<S, R>> {
        let inner = match self.endpoint {
            Endpoint::InProcess(ref name) => {
                let (tx, rx) = self.connect_in_process(name).await?;
                ConnectionInner::InProcess { tx, rx }
            }
            Endpoint::Ipc(ref path) => {
                let stream = self.connect_ipc_with_retry(path).await?;
                ConnectionInner::Ipc {
                    framed: Framed::new(stream, build_codec(self.max_message_size)),
                    max_message_size: self.max_message_size,
                    compress: self.compress,
                    _phantom: PhantomData,
                }
            }
        };
        Ok(Connection { inner })
    }

    async fn connect_in_process(&self, name: &str) -> Result<(mpsc::Sender<S>, mpsc::Receiver<R>)> {
        let mut attempt = 0usize;
        loop {
            let result = {
                let map = registry().lock().expect("channel registry lock poisoned");
                match map.get(name) {
                    Some(any) => {
                        // The acceptor is InProcessAcceptor<R, S> because:
                        // - R = what listener sends (= what we receive)
                        // - S = what listener receives (= what we send)
                        match any.downcast_ref::<InProcessAcceptor<R, S>>() {
                            Some(acceptor) => Some(Ok(acceptor.tx.clone())),
                            None => Some(Err(Error::TypeMismatch(name.to_string()))),
                        }
                    }
                    None => None,
                }
            };

            if let Some(result) = result {
                let accept_tx = result?;
                let (c2s_tx, c2s_rx) = mpsc::channel::<S>(IN_PROCESS_CHANNEL_BUFFER);
                let (s2c_tx, s2c_rx) = mpsc::channel::<R>(IN_PROCESS_CHANNEL_BUFFER);

                accept_tx
                    .send((s2c_tx, c2s_rx))
                    .await
                    .map_err(|_| Error::ConnectionClosed)?;

                return Ok((c2s_tx, s2c_rx));
            }

            attempt += 1;
            if let Some(max) = self.max_retries {
                if attempt >= max {
                    return Err(Error::ConnectionClosed);
                }
            }
            tokio::time::sleep(self.retry_interval).await;
        }
    }

    /// Connects over IPC, retrying until success or the retry budget is
    /// exhausted (then returns the last `Error::Io`).
    async fn connect_ipc_with_retry(&self, path: &PathBuf) -> Result<ConnectionStream> {
        let mut attempt = 0usize;
        loop {
            match transport::connect(path).await {
                Ok(stream) => return Ok(stream),
                Err(e) => {
                    attempt += 1;
                    if let Some(max) = self.max_retries {
                        if attempt >= max {
                            return Err(Error::Io(e));
                        }
                    }
                    tokio::time::sleep(self.retry_interval).await;
                }
            }
        }
    }
}

// --- Listener ---

/// Server-side listener that accepts bidirectional [`Connection`]s.
///
/// `S` is the type this side sends, `R` is the type it receives. Each
/// accepted connection is a `Connection<S, R>`.
pub struct Listener<S, R> {
    inner: ListenerInner<S, R>,
}

enum ListenerInner<S, R> {
    InProcess {
        rx: mpsc::Receiver<(mpsc::Sender<S>, mpsc::Receiver<R>)>,
        name: String,
    },
    Ipc {
        listener: TransportListener,
        max_message_size: usize,
        compress: bool,
        _phantom: PhantomData<(S, R)>,
    },
}

impl<S, R> Listener<S, R>
where
    S: Serialize + Send + 'static,
    R: DeserializeOwned + Send + 'static,
{
    /// Returns a [`ListenerBuilder`] bound to the given endpoint.
    pub fn bind(endpoint: Endpoint) -> ListenerBuilder<S, R> {
        ListenerBuilder {
            endpoint,
            max_message_size: DEFAULT_MAX_MESSAGE_SIZE,
            compress: false,
            accept_capacity: DEFAULT_ACCEPT_CAPACITY,
            _phantom: PhantomData,
        }
    }

    /// Accepts a new client connection.
    ///
    /// In-process: takes the next queued pair from the accept queue. IPC:
    /// accepts the next socket client; the returned connection inherits this
    /// listener's `max_message_size` and `compress` settings.
    pub async fn accept(&mut self) -> Result<Connection<S, R>> {
        match &mut self.inner {
            ListenerInner::InProcess { rx, .. } => {
                let (tx, rx) = rx.recv().await.ok_or(Error::ConnectionClosed)?;
                Ok(Connection {
                    inner: ConnectionInner::InProcess { tx, rx },
                })
            }
            ListenerInner::Ipc {
                listener,
                max_message_size,
                compress,
                ..
            } => {
                let stream = listener.accept().await?;
                Ok(Connection {
                    inner: ConnectionInner::Ipc {
                        framed: Framed::new(stream, build_codec(*max_message_size)),
                        max_message_size: *max_message_size,
                        compress: *compress,
                        _phantom: PhantomData,
                    },
                })
            }
        }
    }
}

/// Unregisters the in-process channel name from the registry (IPC listeners
/// have no registry state). Removal is by name, not by entry: a listener
/// whose name was re-bound by a newer listener removes the newer registration
/// on drop.
impl<S, R> Drop for Listener<S, R> {
    fn drop(&mut self) {
        if let ListenerInner::InProcess { name, .. } = &self.inner {
            registry()
                .lock()
                .expect("channel registry lock poisoned")
                .remove(name);
        }
    }
}

/// Builder for [`Listener`]. Created by [`Listener::bind`].
pub struct ListenerBuilder<S, R> {
    endpoint: Endpoint,
    max_message_size: usize,
    compress: bool,
    // No builder setter: every in-process accept queue is created at
    // DEFAULT_ACCEPT_CAPACITY.
    accept_capacity: usize,
    _phantom: PhantomData<(S, R)>,
}

impl<S, R> ListenerBuilder<S, R>
where
    S: Serialize + Send + 'static,
    R: DeserializeOwned + Send + 'static,
{
    /// Max allowed message size in bytes (default 8 MiB). IPC only; applied
    /// to every accepted connection, and both ends of a link must set the
    /// same value.
    pub fn max_message_size(mut self, size: usize) -> Self {
        self.max_message_size = size;
        self
    }

    /// Whether to expect LZ4-compressed payloads (default: false). IPC only;
    /// both ends must agree, or recv fails with `Error::Io`.
    pub fn compress(mut self, compress: bool) -> Self {
        self.compress = compress;
        self
    }

    /// Starts listening and returns the [`Listener`]. Synchronous: binds the
    /// socket (IPC) or registers the channel name in the registry
    /// (in-process).
    pub fn open(self) -> Result<Listener<S, R>> {
        let inner = match self.endpoint {
            Endpoint::InProcess(name) => {
                let (tx, rx) = mpsc::channel(self.accept_capacity);
                registry()
                    .lock()
                    .expect("channel registry lock poisoned")
                    .insert(name.clone(), Box::new(InProcessAcceptor::<S, R> { tx }));
                ListenerInner::InProcess { rx, name }
            }
            Endpoint::Ipc(path) => {
                let listener = TransportListener::bind(&path)?;
                ListenerInner::Ipc {
                    listener,
                    max_message_size: self.max_message_size,
                    compress: self.compress,
                    _phantom: PhantomData,
                }
            }
        };
        Ok(Listener { inner })
    }
}
