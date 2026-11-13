//! Worker tasks: one [`Component`] trait and one [`ComponentHandle`] for
//! every request/response worker.
//!
//! A component is a self-contained tokio task running one event loop over two
//! unbounded channels: it receives requests, emits responses, and exits when
//! its cancellation token fires or every request sender is gone. The component
//! type carries no state. Everything per-instance rides `Args`, which
//! [`ComponentHandle::spawn`] moves into the task.
//!
//! # Contract
//!
//! - Trait shape: [`Component::run`] is an associated function, not a method,
//!   because the impl type carries nothing. The `Send + 'static` bounds make
//!   its future spawnable as a tokio task.
//! - Spawn lifecycle: [`ComponentHandle::spawn`] spawns `run` as a detached
//!   tokio task. A panicking task is observed only through its channels
//!   closing. The handle does not own the cancellation token: shutdown stays
//!   with whoever created it. Cancellation is cooperative. The loop finishes
//!   the `select!` arm it is running and then exits. Each component decides
//!   what happens to its unfinished work.
//! - Handle/request pairing: `send` never blocks (unbounded channels). Any
//!   backpressure is the component's internal policy, invisible to the
//!   sender. A `send` fails only once the component task is gone.
//! - In-flight counter: `send` increments it, `recv` decrements it
//!   (saturating), so [`ComponentHandle::pending`] is a lower bound on the
//!   responses still queued when a component also emits unsolicited
//!   responses.
//! - Single owner: the handle's mutating methods take `&mut self` and the
//!   response channel is single-consumer, so a handle is driven by one task
//!   at a time.
//! - Error semantics: the machinery raises no errors of its own. A dead
//!   component surfaces only as closed channels. [`batch_recover`] and
//!   [`drain_pending`] turn that into an `anyhow` error.

use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

/// A self-contained request/response worker: one tokio task, one event loop.
///
/// The trait fixes only the loop's shape — the [`Component::run`] signature
/// and the channel + cancellation wiring around it. Behavior, backpressure
/// and shutdown-drain policy are each impl's own. See the module docs for the
/// full contract.
pub trait Component: Send + 'static {
    type Request: Send + 'static;
    type Response: Send + 'static;
    type Args: Send + 'static;

    /// Run the component's event loop.
    ///
    /// Receives requests on `rx`, sends responses on `tx`, and exits when
    /// `cancel` is triggered or `rx` is closed (every request sender
    /// dropped). Sending on `tx` is best-effort. A closed response channel
    /// means the consuming side is already gone, and is not an exit
    /// condition. `cancel` is observed between requests, on the loop's
    /// next `select!` arm; it does not preempt in-flight work. A component
    /// MAY emit responses with no matching request (see
    /// [`ComponentHandle::recv`]).
    fn run(
        args: Self::Args,
        rx: mpsc::UnboundedReceiver<Self::Request>,
        tx: mpsc::UnboundedSender<Self::Response>,
        cancel: CancellationToken,
    ) -> impl std::future::Future<Output = ()> + Send;
}

/// Handle for communicating with a spawned [`Component`]: the request
/// sender, the response receiver, and the in-flight counter.
pub struct ComponentHandle<Req, Resp> {
    tx: mpsc::UnboundedSender<Req>,
    rx: mpsc::UnboundedReceiver<Resp>,
    pending: usize,
}

impl<Req: Send + 'static, Resp: Send + 'static> ComponentHandle<Req, Resp> {
    /// Spawn `C`'s event loop as a new tokio task and return its handle.
    ///
    /// The `JoinHandle` is dropped: the task is
    /// detached, its only observable failure mode being the channels closing
    /// (see [`ComponentHandle::recv`]). Nothing here stops the component —
    /// cancellation stays with the token the caller passes in.
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

    /// Send a request to the component; never blocks (unbounded channel).
    ///
    /// Fails only once the component task is gone (receiver dropped).
    /// Increments the in-flight counter — see [`ComponentHandle::recv`].
    pub fn send(&mut self, req: Req) -> Result<(), mpsc::error::SendError<Req>> {
        self.tx.send(req)?;
        self.pending += 1;
        Ok(())
    }

    /// Receive the next response from the component.
    ///
    /// The decrement saturates at zero: a component MAY emit unsolicited
    /// responses with no matching request, so `pending` is a lower bound on
    /// the responses still queued, not an exact send/recv balance.
    ///
    /// Returns `None` once the component task is gone and the queue is
    /// drained — responses buffered before its exit are still delivered.
    pub async fn recv(&mut self) -> Option<Resp> {
        let resp = self.rx.recv().await;
        if resp.is_some() {
            self.pending = self.pending.saturating_sub(1);
        }
        resp
    }

    /// Requests sent but not yet answered — a best-effort lower bound on the
    /// responses still queued; see [`recv`](Self::recv).
    pub fn pending(&self) -> usize {
        self.pending
    }

    /// Split into the raw request sender and response receiver, dropping the
    /// in-flight counter.
    ///
    /// Responses still queued on the receiver are preserved.
    pub fn into_parts(self) -> (mpsc::UnboundedSender<Req>, mpsc::UnboundedReceiver<Resp>) {
        (self.tx, self.rx)
    }
}

/// Drain the responses a handle's in-flight counter still accounts for,
/// processing each one.
///
/// Used to flush responses enqueued as side effects of another component's
/// `batch_recover`.
/// Precondition: every earlier request/response round on this handle was
/// itself consumed 1:1 (a `batch_recover` recv-s exactly what it sent,
/// rebalancing the counter), so the counter here counts only the
/// fire-and-forget sends; a component that emits unsolicited responses could
/// otherwise end the drain early. Each drained response must still answer
/// exactly one such send. A closed response channel mid-drain means the
/// component died with requests outstanding: an `anyhow` error.
pub async fn drain_pending<Req: Send + 'static, Resp: Send + 'static>(
    handle: &mut ComponentHandle<Req, Resp>,
    mut process: impl FnMut(Resp),
) -> anyhow::Result<()> {
    while handle.pending() > 0 {
        let resp = handle
            .recv()
            .await
            .ok_or_else(|| anyhow::anyhow!("component died during drain"))?;
        process(resp);
    }
    Ok(())
}

/// Send a batch of requests to a component and process every response.
///
/// Recovery's replay primitive: pending work re-enters through the normal
/// component path instead of duplicating its processing logic.
///
/// The two loops are decoupled on purpose: all requests go out first, then
/// exactly `count` responses are consumed in arrival order and handed to
/// `process` — which must pair by response content (each carries its
/// seq/path), not by position. This requires a worker that answers every
/// request with exactly one response. Because the loop recv-s exactly what
/// it sent, the in-flight counter returns to its pre-call value — which is
/// what lets a following [`drain_pending`] count only the fire-and-forget
/// sends `process` makes. A dead component — a failed `send` or a `None`
/// mid-batch — is an `anyhow` error.
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
