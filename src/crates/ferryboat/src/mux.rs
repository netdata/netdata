//! Multiplexed RPC layer on top of [`Connection`](crate::Connection).
//!
//! [`RpcClient`] supports multiple in-flight requests on a single connection.
//! [`RpcServer`] accepts connections and processes requests concurrently via
//! [`RpcSession::serve`].

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

// Capacity of mpsc channels used for RPC request routing and response collection.
const RPC_CHANNEL_BUFFER: usize = 64;

// --- Wire frame ---

/// Request ID + payload, serialized on the wire for IPC,
/// moved directly for in-process.
#[derive(Serialize, Deserialize)]
struct Frame<T> {
    id: u64,
    payload: T,
}

// --- Split halves ---

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
/// Cloneable — all clones share the same underlying connection.
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
    pub async fn call(&self, req: Req) -> Result<Resp> {
        let (resp_tx, resp_rx) = oneshot::channel();
        self.tx
            .send((req, resp_tx))
            .await
            .map_err(|_| Error::ConnectionClosed)?;
        resp_rx.await.map_err(|_| Error::ConnectionClosed)?
    }

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
    pub fn max_message_size(mut self, size: usize) -> Self {
        self.inner = self.inner.max_message_size(size);
        self
    }

    /// Whether to LZ4-compress payloads (default: false).
    pub fn compress(mut self, compress: bool) -> Self {
        self.inner = self.inner.compress(compress);
        self
    }

    /// Connects and returns the [`RpcClient`].
    pub async fn open(self) -> Result<RpcClient<Req, Resp>> {
        let conn = self.inner.open().await?;
        Ok(RpcClient::spawn(conn))
    }
}

// --- RpcServer ---

/// A multiplexed RPC server that accepts connections from [`RpcClient`]s.
///
/// Each accepted [`RpcSession`] can handle concurrent requests.
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
    pub fn max_message_size(mut self, size: usize) -> Self {
        self.inner = self.inner.max_message_size(size);
        self
    }

    /// Whether to expect LZ4-compressed payloads (default: false).
    pub fn compress(mut self, compress: bool) -> Self {
        self.inner = self.inner.compress(compress);
        self
    }

    /// Starts listening and returns the [`RpcServer`].
    pub fn open(self) -> Result<RpcServer<Req, Resp>> {
        let inner = self.inner.open()?;
        Ok(RpcServer { inner })
    }
}

/// A single client session on the server side.
///
/// Call [`serve`](RpcSession::serve) to process requests concurrently.
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
    /// For each incoming request, `handler` is spawned as a new task.
    /// Responses are tagged with the correct request ID so the client
    /// matches them even if handlers complete out of order.
    ///
    /// Returns `Ok(())` when the client disconnects.
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
                Some(frame) = resp_rx.recv() => {
                    writer.send(frame).await?;
                }
            }
        }

        Ok(())
    }
}
