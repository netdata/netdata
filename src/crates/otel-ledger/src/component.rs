//! Generic component abstraction for in-process task coordination.
//!
//! Components implement the [`Component`] trait to define their own event
//! loop. The ledger communicates with them through a [`ComponentHandle`],
//! which tracks in-flight request count for backlog visibility.

use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

/// A self-contained component that processes requests and produces responses.
///
/// Each component owns its event loop via [`Component::run`]. Simple
/// components implement a basic recv-process-send loop. Complex components
/// (like the indexer) can manage internal concurrency, queue requests,
/// and track in-flight tasks — all invisible to the ledger.
pub trait Component: Send + 'static {
    type Request: Send + 'static;
    type Response: Send + 'static;
    type Args: Send + 'static;

    /// Run the component's event loop.
    ///
    /// Receives requests on `rx`, sends responses on `tx`, and exits
    /// when `cancel` is triggered or `rx` is closed.
    fn run(
        args: Self::Args,
        rx: mpsc::UnboundedReceiver<Self::Request>,
        tx: mpsc::UnboundedSender<Self::Response>,
        cancel: CancellationToken,
    ) -> impl std::future::Future<Output = ()> + Send;
}

/// Handle for communicating with a spawned [`Component`].
///
/// Tracks the number of in-flight requests (sent but not yet responded to)
/// so the ledger can observe backlog depth.
pub struct ComponentHandle<Req, Resp> {
    tx: mpsc::UnboundedSender<Req>,
    rx: mpsc::UnboundedReceiver<Resp>,
    pending: usize,
}

impl<Req: Send + 'static, Resp: Send + 'static> ComponentHandle<Req, Resp> {
    /// Spawn a component in a new tokio task and return a handle to it.
    pub fn spawn<C>(args: C::Args, cancel: CancellationToken) -> Self
    where
        C: Component<Request = Req, Response = Resp>,
    {
        let (req_tx, req_rx) = mpsc::unbounded_channel::<Req>();
        let (resp_tx, resp_rx) = mpsc::unbounded_channel::<Resp>();

        tokio::spawn(C::run(args, req_rx, resp_tx, cancel));

        Self {
            tx: req_tx,
            rx: resp_rx,
            pending: 0,
        }
    }

    /// Send a request to the component. Never blocks (unbounded channel).
    pub fn send(&mut self, req: Req) -> Result<(), mpsc::error::SendError<Req>> {
        self.tx.send(req)?;
        self.pending += 1;
        Ok(())
    }

    /// Receive the next response from the component.
    pub async fn recv(&mut self) -> Option<Resp> {
        let resp = self.rx.recv().await;
        if resp.is_some() {
            self.pending -= 1;
        }
        resp
    }

    /// Number of requests sent but not yet responded to.
    pub fn pending(&self) -> usize {
        self.pending
    }
}

/// Send a batch of requests to a component and process all responses.
///
/// Used during recovery to replay pending work through the normal
/// component path instead of duplicating the processing logic.
pub async fn batch_recover<Req: Send + 'static, Resp: Send + 'static>(
    requests: Vec<Req>,
    handle: &mut ComponentHandle<Req, Resp>,
    mut process: impl FnMut(Resp),
) -> anyhow::Result<()> {
    if requests.is_empty() {
        return Ok(());
    }

    let count = requests.len();
    for req in requests {
        handle
            .send(req)
            .map_err(|_| anyhow::anyhow!("component died during recovery"))?;
    }

    for _ in 0..count {
        let resp = handle
            .recv()
            .await
            .ok_or_else(|| anyhow::anyhow!("component died during recovery"))?;
        process(resp);
    }

    Ok(())
}
