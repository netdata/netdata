//! A transport-agnostic messaging library for inter-process and in-process
//! communication.
//!
//! [`Listener`] accepts bidirectional [`Connection`]s. Switching from
//! in-process to cross-process (or vice versa) is a single [`Endpoint`]
//! change — application logic stays the same.
//!
//! # Transports
//!
//! - [`Endpoint::InProcess`] — in-memory channel, no serialization overhead.
//! - [`Endpoint::Ipc`] — Unix domain sockets (Unix) or named pipes (Windows).
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

const DEFAULT_MAX_MESSAGE_SIZE: usize = 8 * 1024 * 1024;

// Capacity of the mpsc channels backing in-process connections.
const IN_PROCESS_CHANNEL_BUFFER: usize = 128;

// Capacity of the accept queue for in-process listeners.
const DEFAULT_ACCEPT_CAPACITY: usize = 64;

fn build_codec(max_message_size: usize) -> LengthDelimitedCodec {
    LengthDelimitedCodec::builder()
        .max_frame_length(max_message_size)
        .new_codec()
}

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

fn registry() -> &'static Mutex<HashMap<String, Box<dyn Any + Send + Sync>>> {
    static INSTANCE: OnceLock<Mutex<HashMap<String, Box<dyn Any + Send + Sync>>>> = OnceLock::new();
    INSTANCE.get_or_init(|| Mutex::new(HashMap::new()))
}

// --- Error type ---

/// Errors returned by [`Connection::send`] and [`Connection::recv`].
#[derive(Debug, thiserror::Error)]
pub enum Error {
    /// Transport-level I/O error.
    #[error("io: {0}")]
    Io(#[from] std::io::Error),

    /// Failed to serialize a message.
    #[error("serialization: {0}")]
    Encode(#[from] bincode::error::EncodeError),

    /// Failed to deserialize a message.
    #[error("deserialization: {0}")]
    Decode(#[from] bincode::error::DecodeError),

    /// The other side of the connection has been closed.
    #[error("connection closed")]
    ConnectionClosed,

    /// Serialized message exceeds the configured limit.
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

/// In-process accept queue: the listener receives paired channels from clients.
struct InProcessAcceptor<S, R> {
    tx: mpsc::Sender<(mpsc::Sender<S>, mpsc::Receiver<R>)>,
}

/// A bidirectional connection that can send messages of type `S` and receive
/// messages of type `R`.
///
/// For RPC, a client typically creates `Connection<Req, Resp>` (sends
/// requests, receives responses), while the server's accepted connections are
/// `Connection<Resp, Req>` (sends responses, receives requests).
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
    /// channel. For IPC connections it is serialized.
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

    /// Max allowed frame size in bytes (default: 8 MB). IPC only.
    pub fn max_message_size(mut self, size: usize) -> Self {
        self.max_message_size = size;
        self
    }

    /// Whether to LZ4-compress payloads (default: false).
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
    accept_capacity: usize,
    _phantom: PhantomData<(S, R)>,
}

impl<S, R> ListenerBuilder<S, R>
where
    S: Serialize + Send + 'static,
    R: DeserializeOwned + Send + 'static,
{
    /// Max allowed frame size in bytes (default: 8 MB). IPC only.
    pub fn max_message_size(mut self, size: usize) -> Self {
        self.max_message_size = size;
        self
    }

    /// Whether to expect LZ4-compressed payloads (default: false).
    pub fn compress(mut self, compress: bool) -> Self {
        self.compress = compress;
        self
    }

    /// Starts listening and returns the [`Listener`].
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
