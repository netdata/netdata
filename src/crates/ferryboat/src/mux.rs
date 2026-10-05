//! Multiplexed request/response RPC on top of [`Connection`](crate::Connection).
//!
//! The multiplexing unit is the request/response exchange, not byte streams
//! or channels: every frame is a `Frame<T>` — a client-assigned `u64`
//! correlation id plus the payload. The server echoes the id on each
//! response, so requests can be dispatched, served, and answered out of
//! order. Ids are scoped to one connection.
//!
//! Over IPC, frames are bincode-serialized (optionally LZ4-compressed),
//! size-checked against the builders' `max_message_size`, and written as
//! length-delimited frames; in-process, values move directly through
//! bounded mpsc channels. Client and server must agree on `compress`, and
//! each side's `max_message_size` must be at least as large as the peer's
//! frames, or decode fails.
//!
//! [`RpcClient`] is a cloneable handle over one connection: a single
//! dispatch task assigns ids, tracks pending calls, and fails all of them
//! with [`Error::ConnectionClosed`] when the connection ends. [`RpcServer`]
//! accepts one [`RpcSession`] per client; `serve` spawns a task per request
//! and streams responses back through a bounded channel, so a slow client
//! backpressures its own handlers only.
//!
//! Consumers: re-exported at the crate root, but used only inside ferryboat
//! (crate doc examples, tests/integration.rs). The plugin crates
//! (otel-ingestor, otel-ledger, file-lifecycle, otel-legacy-logs, and the
//! otel-plugin supervisor) drive [`Connection`] and
//! [`Listener`](crate::Listener) directly instead.

use std::collections::HashMap;
use std::future::Future;
use std::marker::PhantomData;
use std::time::Duration;

use bytes::Bytes;
use futures::stream::{SplitSink, SplitStream};
use futures::{SinkExt, StreamExt};
use serde::de::DeserializeOwned;
use serde::{Deserialize, Serialize};
use tokio::sync::{mpsc, oneshot};
use tokio_util::codec::{Framed, LengthDelimitedCodec};

use crate::transport::ConnectionStream;
use crate::{Connection, ConnectionInner, Endpoint, Error, Listener, Result};

// Capacity of the bounded queues that hold incoming client requests on
// each RpcClient and outgoing responses on each RpcSession.
const RPC_CHANNEL_BUFFER: usize = 64;

// --- Wire frame ---

/// Request envelope: a client-assigned correlation id plus the payload.
///
/// Serialized on the wire for IPC, moved directly for in-process. The id
/// identifies the request within its connection; the server copies it onto
/// the response so the client can match the pair.
#[derive(Serialize, Deserialize)]
struct Frame<T> {
    id: u64,
    payload: T,
}

// --- Split halves ---

/// Send half of a split [`Connection`].
///
/// In-process sends move the value into the peer's channel, failing with
/// `Error::ConnectionClosed` once the peer is gone. IPC sends serialize the
/// value (bincode, optional LZ4, size-checked against `max_message_size`)
/// and write it as a length-delimited frame; I/O, encode, and oversize
/// failures surface as [`Error`]. The `_phantom` field ties the raw-bytes
/// sink to the message type.
enum Writer<S> {
    InProcess(mpsc::Sender<S>),
    Ipc {
        sink: SplitSink<Framed<ConnectionStream, LengthDelimitedCodec>, Bytes>,
        max_message_size: usize,
        compress: bool,
        _phantom: PhantomData<S>,
    },
}

impl<S: Serialize> Writer<S> {
    async fn send(&mut self, msg: S) -> Result<()> {
        match self {
            Writer::InProcess(tx) => tx.send(msg).await.map_err(|_| Error::ConnectionClosed),
            Writer::Ipc {
                sink,
                max_message_size,
                compress,
                ..
            } => {
                let data = crate::serialize_ipc(&msg, *compress, *max_message_size)?;
                sink.send(data).await.map_err(Error::Io)?;
                Ok(())
            }
        }
    }
}

/// Receive half of a split [`Connection`].
///
/// In-process receives pop from the channel; IPC receives read the next
/// length-delimited frame and deserialize it. An exhausted transport maps
/// to `Error::ConnectionClosed`; frame-limit violations and decode
/// failures surface as [`Error`]. The `_phantom` field ties the raw-bytes
/// stream to the message type.
enum Reader<R> {
    InProcess(mpsc::Receiver<R>),
    Ipc {
        stream: SplitStream<Framed<ConnectionStream, LengthDelimitedCodec>>,
        max_message_size: usize,
        compress: bool,
        _phantom: PhantomData<R>,
    },
}

impl<R: DeserializeOwned> Reader<R> {
    async fn recv(&mut self) -> Result<R> {
        match self {
            Reader::InProcess(rx) => rx.recv().await.ok_or(Error::ConnectionClosed),
            Reader::Ipc {
                stream,
                max_message_size,
                compress,
                ..
            } => {
                let data = stream
                    .next()
                    .await
                    .ok_or(Error::ConnectionClosed)?
                    .map_err(Error::Io)?;
                crate::deserialize_ipc(&data, *compress, *max_message_size)
            }
        }
    }
}

/// Splits a connection into independent send and receive halves so one
/// `select!` loop can write and read concurrently. The connection is
/// consumed.
fn split_connection<S, R>(conn: Connection<S, R>) -> (Writer<S>, Reader<R>) {
    match conn.inner {
        ConnectionInner::InProcess { tx, rx } => (Writer::InProcess(tx), Reader::InProcess(rx)),
        ConnectionInner::Ipc {
            framed,
            max_message_size,
            compress,
            ..
        } => {
            let (sink, stream) = framed.split();
            (
                Writer::Ipc {
                    sink,
                    max_message_size,
                    compress,
                    _phantom: PhantomData,
                },
                Reader::Ipc {
                    stream,
                    max_message_size,
                    compress,
                    _phantom: PhantomData,
                },
            )
        }
    }
}

// --- RpcClient ---

/// A multiplexed RPC client.
///
/// Supports multiple in-flight requests on a single connection.
/// Cloneable — all clones share the same underlying connection and its
/// single dispatch task. [`call`](RpcClient::call) takes `&self`, so any
/// number of tasks can issue requests concurrently; they queue on a
/// bounded channel, so sustained overload backpressures callers instead of
/// growing without bound. Dropping the last clone ends the connection and
/// fails every pending call with [`Error::ConnectionClosed`].
pub struct RpcClient<Req, Resp> {
    tx: mpsc::Sender<(Req, oneshot::Sender<Result<Resp>>)>,
}

impl<Req, Resp> Clone for RpcClient<Req, Resp> {
    fn clone(&self) -> Self {
        RpcClient {
            tx: self.tx.clone(),
        }
    }
}

impl<Req, Resp> RpcClient<Req, Resp>
where
    Req: Serialize + Send + 'static,
    Resp: DeserializeOwned + Send + 'static,
{
    /// Returns a builder that will connect to an [`RpcServer`] at the given
    /// endpoint.
    pub fn connect(endpoint: Endpoint) -> RpcClientBuilder<Req, Resp> {
        RpcClientBuilder {
            inner: Connection::<Frame<Req>, Frame<Resp>>::connect(endpoint),
        }
    }

    /// Sends a request and waits for the response.
    ///
    /// Takes `&self` — safe to call concurrently from multiple tasks.
    /// Returns [`Error::ConnectionClosed`] when the connection has ended,
    /// whether the request never left the queue or the response never
    /// arrived. A `call` future dropped mid-flight leaves its pending slot
    /// occupied until the response arrives; the response is then
    /// discarded.
    pub async fn call(&self, req: Req) -> Result<Resp> {
        let (resp_tx, resp_rx) = oneshot::channel();
        self.tx
            .send((req, resp_tx))
            .await
            .map_err(|_| Error::ConnectionClosed)?;
        resp_rx.await.map_err(|_| Error::ConnectionClosed)?
    }

    /// Spawns the dispatch task that multiplexes `conn`.
    ///
    /// Requests are pulled from the shared queue, assigned a sequential
    /// (wrapping) id, and tracked in `pending` until their response frame
    /// arrives. Response ids with no matching entry are dropped silently.
    /// Any write or read failure ends the task and reports
    /// [`Error::ConnectionClosed`] to every pending call — the underlying
    /// I/O error is not propagated. The task owns both connection halves;
    /// when it exits, the connection closes.
    fn spawn(conn: Connection<Frame<Req>, Frame<Resp>>) -> Self {
        let (request_tx, mut request_rx) =
            mpsc::channel::<(Req, oneshot::Sender<Result<Resp>>)>(RPC_CHANNEL_BUFFER);
        let (mut writer, mut reader) = split_connection(conn);

        tokio::spawn(async move {
            let mut next_id: u64 = 0;
            let mut pending: HashMap<u64, oneshot::Sender<Result<Resp>>> = HashMap::new();

            loop {
                tokio::select! {
                    req = request_rx.recv() => {
                        let Some((req, resp_tx)) = req else { break };
                        let id = next_id;
                        next_id = next_id.wrapping_add(1);
                        pending.insert(id, resp_tx);
                        if writer.send(Frame { id, payload: req }).await.is_err() {
                            break;
                        }
                    }
                    resp = reader.recv() => {
                        match resp {
                            Ok(frame) => {
                                if let Some(tx) = pending.remove(&frame.id) {
                                    let _ = tx.send(Ok(frame.payload));
                                }
                            }
                            Err(_) => break,
                        }
                    }
                }
            }

            for (_, tx) in pending {
                let _ = tx.send(Err(Error::ConnectionClosed));
            }
        });

        RpcClient { tx: request_tx }
    }
}

/// Builder for [`RpcClient`]. Created by [`RpcClient::connect`].
pub struct RpcClientBuilder<Req, Resp> {
    inner: crate::ConnectionBuilder<Frame<Req>, Frame<Resp>>,
}

impl<Req, Resp> RpcClientBuilder<Req, Resp>
where
    Req: Serialize + Send + 'static,
    Resp: DeserializeOwned + Send + 'static,
{
    /// Delay between connection attempts (default: 100ms).
    pub fn retry_interval(mut self, interval: Duration) -> Self {
        self.inner = self.inner.retry_interval(interval);
        self
    }

    /// Maximum connection attempts before giving up (default: 50).
    /// Use `None` to retry indefinitely.
    pub fn max_retries(mut self, max: Option<usize>) -> Self {
        self.inner = self.inner.max_retries(max);
        self
    }

    /// Max allowed frame size in bytes (default: 8 MB). IPC only.
    ///
    /// Applied to the serialized frame — id included, after compression —
    /// on send, and to frames received from the peer.
    pub fn max_message_size(mut self, size: usize) -> Self {
        self.inner = self.inner.max_message_size(size);
        self
    }

    /// Whether to LZ4-compress IPC frames (default: false). Both ends
    /// must agree, or decompression fails on the receiving side.
    pub fn compress(mut self, compress: bool) -> Self {
        self.inner = self.inner.compress(compress);
        self
    }

    /// Connects and returns the [`RpcClient`], spawning its dispatch
    /// task.
    pub async fn open(self) -> Result<RpcClient<Req, Resp>> {
        let conn = self.inner.open().await?;
        Ok(RpcClient::spawn(conn))
    }
}

// --- RpcServer ---

/// A multiplexed RPC server that accepts connections from [`RpcClient`]s.
///
/// Each accepted [`RpcSession`] can handle concurrent requests. Internally
/// a `Listener<Frame<Resp>, Frame<Req>>`: the parameters are flipped
/// because the server sends responses and receives requests.
pub struct RpcServer<Req, Resp> {
    inner: Listener<Frame<Resp>, Frame<Req>>,
}

impl<Req, Resp> RpcServer<Req, Resp>
where
    Req: DeserializeOwned + Send + 'static,
    Resp: Serialize + Send + 'static,
{
    /// Returns a builder bound to the given endpoint.
    pub fn bind(endpoint: Endpoint) -> RpcServerBuilder<Req, Resp> {
        RpcServerBuilder {
            inner: Listener::<Frame<Resp>, Frame<Req>>::bind(endpoint),
        }
    }

    /// Accepts a new client connection.
    ///
    /// Callers typically move the returned session into a spawned task
    /// running [`serve`](RpcSession::serve).
    pub async fn accept(&mut self) -> Result<RpcSession<Req, Resp>> {
        let conn = self.inner.accept().await?;
        Ok(RpcSession { conn })
    }
}

/// Builder for [`RpcServer`]. Created by [`RpcServer::bind`].
pub struct RpcServerBuilder<Req, Resp> {
    inner: crate::ListenerBuilder<Frame<Resp>, Frame<Req>>,
}

impl<Req, Resp> RpcServerBuilder<Req, Resp>
where
    Req: DeserializeOwned + Send + 'static,
    Resp: Serialize + Send + 'static,
{
    /// Max allowed frame size in bytes (default: 8 MB). IPC only.
    ///
    /// Applied to the serialized frame — id included, after compression —
    /// on send, and to frames received from the peer.
    pub fn max_message_size(mut self, size: usize) -> Self {
        self.inner = self.inner.max_message_size(size);
        self
    }

    /// Whether to expect LZ4-compressed IPC frames (default: false). Both
    /// ends must agree, or decompression fails on the receiving side.
    pub fn compress(mut self, compress: bool) -> Self {
        self.inner = self.inner.compress(compress);
        self
    }

    /// Starts listening and returns the [`RpcServer`].
    ///
    /// Synchronous: the socket is bound (or the in-process channel
    /// registered) before returning; accepts start at
    /// [`RpcServer::accept`].
    pub fn open(self) -> Result<RpcServer<Req, Resp>> {
        let inner = self.inner.open()?;
        Ok(RpcServer { inner })
    }
}

/// A single client session on the server side.
///
/// Created by [`RpcServer::accept`] and consumed by
/// [`serve`](RpcSession::serve), which processes that client's requests
/// concurrently.
pub struct RpcSession<Req, Resp> {
    conn: Connection<Frame<Resp>, Frame<Req>>,
}

impl<Req, Resp> RpcSession<Req, Resp>
where
    Req: DeserializeOwned + Send + 'static,
    Resp: Serialize + Send + 'static,
{
    /// Runs the request handler loop.
    ///
    /// For each incoming request, `handler` (cloned per request) is
    /// spawned as a new task. Responses are tagged with the correct
    /// request ID so the client matches them even if handlers complete
    /// out of order. Responses queue on a bounded channel, so a slow
    /// client backpressures its own handlers; handler tasks are detached
    /// and may outlive this call, with late responses dropped.
    ///
    /// Returns `Ok(())` when the read side ends (client disconnect or an
    /// undecodable stream) and `Err` only when writing a response fails.
    pub async fn serve<F, Fut>(self, handler: F) -> Result<()>
    where
        F: Fn(Req) -> Fut + Send + 'static + Clone,
        Fut: Future<Output = Resp> + Send + 'static,
    {
        let (mut writer, mut reader) = split_connection(self.conn);
        let (resp_tx, mut resp_rx) = mpsc::channel::<Frame<Resp>>(RPC_CHANNEL_BUFFER);

        loop {
            tokio::select! {
                req = reader.recv() => {
                    match req {
                        Ok(frame) => {
                            let handler = handler.clone();
                            let tx = resp_tx.clone();
                            tokio::spawn(async move {
                                let resp = handler(frame.payload).await;
                                let _ = tx.send(Frame {
                                    id: frame.id,
                                    payload: resp,
                                }).await;
                            });
                        }
                        Err(_) => break,
                    }
                }
                // The channel's original sender outlives this loop (only
                // clones reach handler tasks), so recv() never yields
                // None; the Some pattern is defensive.
                Some(frame) = resp_rx.recv() => {
                    writer.send(frame).await?;
                }
            }
        }

        Ok(())
    }
}
