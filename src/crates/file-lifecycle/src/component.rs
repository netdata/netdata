//! The worker-actor substrate of the file-lifecycle stack: one [`Component`]
//! trait and one [`ComponentHandle`] for every request/response worker.
//!
//! A component is a self-contained tokio task running one event loop over two
//! unbounded channels: it receives requests, emits responses, and exits when
//! its cancellation token fires or every request sender is gone. The component
//! type carries no state — it is a marker (stateless unit structs; the
//! uploader names its storage backend through a phantom type) — and
//! everything per-instance rides `Args`, moved into the task by
//! [`ComponentHandle::spawn`]. This module owns only the plumbing: the message
//! vocabulary is `crate::ipc`'s, and each worker's behavior contract lives in
//! its own module ([`crate::cleaner`], [`crate::uploader`],
//! [`crate::catalog_builder`]; otel-ledger's indexer is the fourth `Component`
//! impl).
//!
//! # Contract
//!
//! - Trait shape: [`Component::run`] is an associated async fn, not a method,
//!   because the impl type carries nothing — it takes the args, the request
//!   receiver, the response sender and the cancellation token by value and
//!   returns `()`. The component type, `Request`, `Response` and `Args` are
//!   `Send + 'static`, which is exactly what makes `C::run(...)` a
//!   `Send + 'static` future spawnable as a tokio task.
//! - Spawn lifecycle: [`ComponentHandle::spawn`] pairs the channels, spawns
//!   `run` as a detached tokio task (the `JoinHandle` is dropped, so a
//!   panicking task is observed only through its channels closing) and
//!   returns the handle with the in-flight counter at 0. The handle does not
//!   own the cancellation token: shutdown stays with whoever created it (the
//!   ledger's root token; each component gets a `child_token()` at its spawn
//!   site, cancelled after the run loop exits — `otel-ledger/src/lib.rs`).
//!   Cancellation is cooperative: the loop exits on its next `select!` arm
//!   and never preempts in-flight work; what happens to unfinished work at
//!   cancel is the component's own policy (the uploader drains in-flight
//!   uploads briefly — see its module docs).
//! - Handle/request pairing: `send` never blocks (unbounded channels). Any
//!   backpressure — the indexer queueing requests at its concurrency limit,
//!   the uploader's semaphore bounding uploads in flight — is the
//!   component's internal policy, invisible to the sender. A `send` fails
//!   only once the component task is gone.
//! - In-flight counter: `send` increments it, `recv` decrements it
//!   (saturating), so [`ComponentHandle::pending`] is a lower bound on the
//!   responses still queued when a component also emits unsolicited
//!   responses (the catalog builder's time-trigger and `Flush` rotations
//!   each emit a `Rotated` with no preceding `AddEntry`). Production code
//!   consults it only in the synchronous recovery drains — [`batch_recover`]
//!   leaves it balanced, [`drain_pending`] drains it to zero — while
//!   steady-state routing goes through [`ComponentHandle::into_parts`],
//!   which drops it; recovery tests assert it to pin fire-and-forget request
//!   counts. Nothing else observes backlog.
//! - Single owner: every handle method takes `&mut self` and the response
//!   channel is single-consumer, so a handle is driven by one task at a
//!   time. The shared cleaner/uploader handles live on the ledger and are
//!   polled by its run loop's `select!` (`Ledger::run`); per-pipeline indexer
//!   and catalog-builder handles are born inside the ledger's pipeline
//!   builders, driven by recovery through `&mut`, then split by
//!   [`ComponentHandle::into_parts`] — the sender becomes the `Pipeline`'s
//!   request sender and the receiver moves into a pid-tagging forwarder task
//!   feeding the run loop's merged response channel.
//! - Error semantics: the machinery raises no errors of its own. A dead
//!   component surfaces only as closed channels; [`batch_recover`] and
//!   [`drain_pending`] translate that into the fatal `anyhow` errors that
//!   fail startup, and the ledger's run loop exits its event loop when a
//!   worker channel closes (cleaner, uploader, per-pipeline forwarder
//!   alike).
//!
//! # Who spawns which component (grep-verified)
//!
//! - `otel-ledger/src/ledger/mod.rs` (`Ledger::new`), once per process:
//!   [`crate::cleaner::Cleaner`] (shared, `Args = ()`) and, when remote
//!   storage is enabled, [`crate::uploader::Uploader`] (shared, generic over
//!   the storage backend).
//! - `otel-ledger/src/ledger/pipeline.rs` and `traces_pipeline.rs`, one pair
//!   per signal: otel-ledger's indexer (spawned by the per-signal build
//!   functions, which own its seal-fn arg) and
//!   [`crate::catalog_builder::CatalogBuilder`] (spawned inside the shared
//!   `build_pipeline`).
//!
//! [`crate::recovery`] replays startup work through these same handles via
//! [`batch_recover`] / [`drain_pending`] so recovery and steady state share
//! one code path; `recovery/remote.rs` deliberately sends fire-and-forget
//! instead, never awaiting uploads during startup. Tests pin the machinery's
//! behavior through custom `Component` impls (`recovery/tests.rs`) and by
//! spawning the real workers (`catalog_builder/tests.rs`).

use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

/// A self-contained request/response worker: one tokio task, one event loop.
///
/// The trait fixes only the loop's shape — the [`Component::run`] signature
/// and the channel + cancellation wiring around it. Behavior, backpressure
/// and shutdown-drain policy are each impl's own: compare the stateless
/// recv-process-send [`crate::cleaner::Cleaner`] with the indexer's internal
/// request queue or the uploader's semaphore. See the module docs for the
/// full contract and the spawn map.
pub trait Component: Send + 'static {
    type Request: Send + 'static;
    type Response: Send + 'static;
    type Args: Send + 'static;

    /// Run the component's event loop.
    ///
    /// Receives requests on `rx`, sends responses on `tx`, and exits when
    /// `cancel` is triggered or `rx` is closed (every request sender
    /// dropped). Sending on `tx` is best-effort: all four impls ignore a
    /// failed send (`let _ = tx.send(...)`) and keep serving — a closed
    /// response channel means the consuming side is already gone, not an
    /// exit condition. `cancel` is observed between requests, on the loop's
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
///
/// Exists only as [`ComponentHandle::spawn`]'s return value — the fields are
/// private, so the channel pair cannot be rebuilt except through
/// [`ComponentHandle::into_parts`]. All methods take `&mut self`; a handle
/// belongs to one driving task at a time.
pub struct ComponentHandle<Req, Resp> {
    tx: mpsc::UnboundedSender<Req>,
    rx: mpsc::UnboundedReceiver<Resp>,
    pending: usize,
}

impl<Req: Send + 'static, Resp: Send + 'static> ComponentHandle<Req, Resp> {
    /// Spawn `C`'s event loop as a new tokio task and return its handle.
    ///
    /// Pairs the two unbounded channels around `C::run` and moves `args` and
    /// `cancel` into the task. The `JoinHandle` is dropped: the task is
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
    /// responses with no matching request (the catalog builder's time-trigger
    /// and `Flush` rotations each emit a `Rotated` with no preceding
    /// `AddEntry`), so `pending` is a lower bound on the responses still
    /// queued, not an exact send/recv balance. Production code consults the
    /// counter only in the synchronous recovery drains, which run against
    /// 1:1 request/response workers; steady-state routing goes through
    /// [`ComponentHandle::into_parts`] and ignores it.
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
    /// Used once per per-pipeline worker at the end of recovery
    /// (`otel-ledger/src/ledger/pipeline.rs`): the owning `Pipeline` keeps
    /// the sender for steady-state requests, while the receiver moves into a
    /// forwarder task that tags each response with the pipeline id and feeds
    /// the run loop's single merged channel. Responses still queued on the
    /// receiver — e.g. catalog-builder `AddEntry`s enqueued fire-and-forget
    /// by `reconcile_remote_uploads` — are preserved: the live receiver
    /// carries them into the forwarder and on to the run loop. The dropped
    /// counter was only ever consulted by the synchronous recovery drains
    /// (`batch_recover` / `drain_pending`); steady-state routing ignores it.
    pub fn into_parts(self) -> (mpsc::UnboundedSender<Req>, mpsc::UnboundedReceiver<Resp>) {
        (self.tx, self.rx)
    }
}

/// Drain the responses a handle's in-flight counter still accounts for,
/// processing each one.
///
/// Used to flush responses enqueued as side effects of another component's
/// `batch_recover` — e.g. the WAL-delete requests `recover_unindexed` fires
/// at the cleaner while its indexer responses are still streaming back.
/// Precondition: every earlier request/response round on this handle was
/// itself consumed 1:1 (a `batch_recover` recv-s exactly what it sent,
/// rebalancing the counter), so the counter here counts only the
/// fire-and-forget sends; a component that emits unsolicited responses could
/// otherwise end the drain early. Each drained response must still answer
/// exactly one such send (true of the cleaner, the sole production target —
/// `recovery::local::drain_wal_deletes`). A closed response channel
/// mid-drain means the component died with requests outstanding: fatal
/// `anyhow` error, failing startup.
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
/// component path instead of duplicating its processing logic
/// (`recovery::local` calls it on the indexer and the cleaner; an empty
/// batch returns `Ok(())` without touching the component).
///
/// The two loops are decoupled on purpose: all requests go out first, then
/// exactly `count` responses are consumed in arrival order and handed to
/// `process` — which must pair by response content (each carries its
/// seq/path), not by position. This requires a worker that answers every
/// request with exactly one response; workers that also emit unsolicited
/// responses (the catalog builder) are driven fire-and-forget instead
/// (`recovery/remote.rs`). Because the loop recv-s exactly what it sent, the
/// in-flight counter returns to its pre-call value — which is what lets a
/// following [`drain_pending`] count only the fire-and-forget sends `process`
/// makes. A dead component — a failed `send` or a `None` mid-batch — is a
/// fatal `anyhow` error that fails startup.
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
