//! The seal component: turns closed WAL files into SFST index files.
//!
//! One component type serves both signals — each pipeline spawns its own
//! [`Indexer`] and injects the seal through [`IndexerArgs`]: the logs
//! pipeline passes `ng_index::build_sfst_file`, the traces pipeline
//! `ng_index::build_sfst_traces_file` (`ledger/pipeline.rs`,
//! `ledger/traces_pipeline.rs`). Both share this queueing loop and the
//! `Indexed`/`IndexFailed` response mapping; only the WAL-decoding seal
//! differs.
//!
//! Requests arrive from the ingestor on a WAL `FileEvent::Closed`
//! (`ledger/ingestor.rs`) and, at startup, from the pipeline builder for
//! every WAL a previous run left unindexed (`ledger/pipeline.rs`); responses
//! go back to the ledger's `handle_indexer_resp` (`ledger/indexer.rs` — a
//! different indexer.rs: the response handler, not this component).
//! Live-tail chunk range builds (`ng_index::build_sfst_range`) bypass this
//! component.
//!
//! Concurrency is adaptive within [`SealLimits`]: a seal is CPU-bound and
//! holds its whole WAL's index in memory, so the number of concurrent seals
//! follows the backlog slowly, by at most one every [`SCALE_INTERVAL`] (see
//! [`Scaler`]); excess requests queue. Cancelling or closing the request
//! channel exits the loop, but already-spawned blocking seals are not aborted
//! — their responses are dropped.

use std::collections::{HashMap, VecDeque};
use std::path::Path;
use std::time::Duration;

use tokio::sync::mpsc;
use tokio::time::Instant;
use tokio_util::sync::CancellationToken;

use file_lifecycle::component::Component;
use file_lifecycle::ipc::{IndexerRequest, IndexerResponse};
use otel_bridge::config::IndexersTuning;

/// How often the number of concurrent seals is re-evaluated, and the window
/// its inputs are averaged over.
pub const SCALE_INTERVAL: Duration = Duration::from_secs(10);

/// The seal an [`Indexer`] runs per closed WAL file: read the WAL at the first
/// path, write the SFST at the second, return the registry-facing summary +
/// written size. Exactly the signature of the `ng_index` builders, so spawn
/// sites pass them as plain fn items. Format checking lives inside the seal
/// (each builder pins its own WAL `payload_format`), not in this component.
pub type SealFn =
    fn(&Path, &Path, &ng_index::Metrics) -> Result<(sfst::Summary, u64), ng_index::Error>;

/// Bounds on concurrently running seals; `1 <= min <= max`.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct SealLimits {
    pub min: usize,
    pub max: usize,
}

impl SealLimits {
    /// Resolve the operator tuning: an absent `max` means one seal per tokio
    /// worker thread, the same scaling the ingestor gets. Must be called from
    /// within the runtime that will run the seals.
    pub fn resolve(tuning: &IndexersTuning) -> Self {
        let min = tuning.min.max(1);
        let auto = || {
            tokio::runtime::Handle::try_current()
                .map(|h| h.metrics().num_workers())
                .unwrap_or_else(|_| std::thread::available_parallelism().map_or(1, |n| n.get()))
        };
        let max = tuning.max.unwrap_or_else(auto).max(min);
        Self { min, max }
    }
}

/// One [`SCALE_INTERVAL`] step of the seal count `n`, given the window's
/// time-averaged queue length and `per_seal`, the files one seal completes in
/// an interval. A whole `per_seal` of extra backlog adds a seal and a whole
/// `per_seal` less removes one; the one-`per_seal` dead band between the two
/// keeps bursty arrivals from flip-flopping the count. Never moves by more
/// than one, so memory follows the backlog gradually.
fn scale_step(n: usize, avg_queued: f64, per_seal: Option<f64>, limits: SealLimits) -> usize {
    let Some(per_seal) = per_seal.filter(|y| y.is_finite() && *y > 0.0) else {
        // No seal has completed yet: nothing to size against.
        return n;
    };
    if n < limits.max && avg_queued >= (n + 1) as f64 * per_seal {
        n + 1
    } else if n > limits.min && avg_queued <= (n - 1) as f64 * per_seal {
        n - 1
    } else {
        n
    }
}

/// Tracks the inputs of [`scale_step`] over the current window and owns the
/// target seal count.
struct Scaler {
    limits: SealLimits,
    target: usize,
    /// Files one seal completes per interval, from the last window that
    /// completed any seal.
    per_seal: Option<f64>,
    window_start: Instant,
    /// Queue length × seconds accumulated this window (piecewise constant).
    queued_area: f64,
    last_len: usize,
    last_change: Instant,
    seal_secs: f64,
    seals: u32,
}

impl Scaler {
    fn new(limits: SealLimits, now: Instant) -> Self {
        Self {
            limits,
            target: limits.min,
            per_seal: None,
            window_start: now,
            queued_area: 0.0,
            last_len: 0,
            last_change: now,
            seal_secs: 0.0,
            seals: 0,
        }
    }

    /// Record the queue length from `now` on.
    fn observe_queue(&mut self, now: Instant, len: usize) {
        let held = now
            .saturating_duration_since(self.last_change)
            .as_secs_f64();
        self.queued_area += self.last_len as f64 * held;
        self.last_len = len;
        self.last_change = now;
    }

    fn seal_done(&mut self, elapsed: Duration) {
        self.seal_secs += elapsed.as_secs_f64();
        self.seals += 1;
    }

    /// Close the window ending at `now`, apply one [`scale_step`] and start
    /// the next window. Returns `(old, new, avg_queued)` when the target
    /// changed.
    fn tick(&mut self, now: Instant) -> Option<(usize, usize, f64)> {
        self.observe_queue(now, self.last_len);
        let window = now
            .saturating_duration_since(self.window_start)
            .as_secs_f64();
        let avg_queued = if window > 0.0 {
            self.queued_area / window
        } else {
            self.last_len as f64
        };
        if self.seals > 0 && self.seal_secs > 0.0 {
            let mean = self.seal_secs / f64::from(self.seals);
            self.per_seal = Some(SCALE_INTERVAL.as_secs_f64() / mean);
        }
        self.window_start = now;
        self.queued_area = 0.0;
        self.seal_secs = 0.0;
        self.seals = 0;

        let old = self.target;
        self.target = scale_step(old, avg_queued, self.per_seal, self.limits);
        (self.target != old).then_some((old, self.target, avg_queued))
    }
}

/// What a pipeline hands its [`Indexer`] at spawn.
pub struct IndexerArgs {
    pub seal: SealFn,
    pub limits: SealLimits,
}

/// One in-flight indexing operation: the WAL's sequence number and the start
/// time, for the completion log and the seal-time average.
struct IndexerTask {
    seq: u64,
    started_at: Instant,
}

/// The seal-running component: a unit struct — all state lives in
/// `Component::run`'s locals; one instance per signal pipeline.
pub struct Indexer;

impl Component for Indexer {
    type Request = IndexerRequest;
    type Response = IndexerResponse;
    type Args = IndexerArgs;

    async fn run(
        args: IndexerArgs,
        mut rx: mpsc::UnboundedReceiver<IndexerRequest>,
        tx: mpsc::UnboundedSender<IndexerResponse>,
        cancel: CancellationToken,
    ) {
        let IndexerArgs { seal, limits } = args;
        tracing::info!(
            "indexer started min_concurrent={} max_concurrent={}",
            limits.min,
            limits.max,
        );

        // In-flight seals keyed by a per-component task id: a WAL seq is not
        // unique (an unparseable file name maps to 0).
        let (done_tx, mut done_rx) = mpsc::unbounded_channel::<(u64, IndexerResponse)>();
        let mut in_flight: HashMap<u64, IndexerTask> = HashMap::new();
        let mut queue: VecDeque<IndexerRequest> = VecDeque::new();
        let mut next_task_id: u64 = 0;
        let mut scaler = Scaler::new(limits, Instant::now());
        let mut scale_tick =
            tokio::time::interval_at(Instant::now() + SCALE_INTERVAL, SCALE_INTERVAL);
        scale_tick.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Delay);

        loop {
            tokio::select! {
                _ = cancel.cancelled() => break,
                req = rx.recv() => match req {
                    Some(req) => queue.push_back(req),
                    None => break,
                },
                Some((task_id, resp)) = done_rx.recv() => {
                    if let Some(task) = in_flight.remove(&task_id) {
                        let elapsed = task.started_at.elapsed();
                        scaler.seal_done(elapsed);
                        tracing::info!(
                            "indexing done seq={} elapsed_ms={} running={} queued={}",
                            task.seq,
                            elapsed.as_millis(),
                            in_flight.len(),
                            queue.len(),
                        );
                    }
                    let _ = tx.send(resp);
                }
                _ = scale_tick.tick() => {
                    if let Some((old, new, avg_queued)) = scaler.tick(Instant::now()) {
                        tracing::info!(
                            "indexer concurrency {old} -> {new} \
                             (avg queued {avg_queued:.1}, files per seal per {}s {:.1})",
                            SCALE_INTERVAL.as_secs(),
                            scaler.per_seal.unwrap_or(0.0),
                        );
                    }
                }
            }

            while in_flight.len() < scaler.target {
                let Some(req) = queue.pop_front() else { break };
                start_indexing(seal, req, next_task_id, &mut in_flight, done_tx.clone());
                next_task_id += 1;
            }
            scaler.observe_queue(Instant::now(), queue.len());
        }
    }
}

/// Register the request as in-flight and spawn the seal on a blocking
/// thread; the `Indexed`/`IndexFailed` response is routed through `done_tx`
/// so the run loop logs it and re-evaluates the queue.
fn start_indexing(
    seal: SealFn,
    req: IndexerRequest,
    task_id: u64,
    in_flight: &mut HashMap<u64, IndexerTask>,
    done_tx: mpsc::UnboundedSender<(u64, IndexerResponse)>,
) {
    let IndexerRequest::Index {
        wal_path,
        sfst_path,
    } = req;

    // Parsed from the WAL file name; 0 when it doesn't parse. The ledger
    // routes the response to a tenant by it.
    let seq = file_registry::FileId::parse(&wal_path)
        .map(|id| id.seq)
        .unwrap_or(0);

    in_flight.insert(
        task_id,
        IndexerTask {
            seq,
            started_at: Instant::now(),
        },
    );

    tracing::info!(
        "indexing started wal={} index={} running={}",
        wal_path.display(),
        sfst_path.display(),
        in_flight.len(),
    );

    tokio::task::spawn_blocking(move || {
        let resp = match seal(&wal_path, &sfst_path, &ng_index::Metrics::new()) {
            Ok((summary, size)) => IndexerResponse::Indexed {
                seq,
                path: sfst_path,
                summary,
                size: file_registry::ByteSize(size),
            },
            Err(e) => {
                tracing::error!("indexing failed wal={}: {e}", wal_path.display());
                IndexerResponse::IndexFailed {
                    path: wal_path,
                    error: e.to_string(),
                }
            }
        };

        let _ = done_tx.send((task_id, resp));
    });
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::PathBuf;
    use std::sync::atomic::{AtomicUsize, Ordering};

    use file_lifecycle::component::ComponentHandle;

    const L: SealLimits = SealLimits { min: 1, max: 8 };

    #[test]
    fn step_needs_a_whole_seal_of_backlog_to_move() {
        // Y = 15 files per seal per interval, two seals running.
        let y = Some(15.0);
        assert_eq!(scale_step(2, 35.0, y, L), 2, "inside the dead band");
        assert_eq!(scale_step(2, 50.0, y, L), 3, ">= 3 * Y adds one");
        assert_eq!(scale_step(3, 35.0, y, L), 3, "inside the dead band");
        assert_eq!(scale_step(3, 29.0, y, L), 2, "<= 2 * Y removes one");
        assert_eq!(scale_step(2, 45.0, y, L), 3, "exactly (n+1) * Y adds");
        assert_eq!(scale_step(3, 30.0, y, L), 2, "exactly (n-1) * Y removes");
    }

    #[test]
    fn step_moves_by_at_most_one_and_respects_limits() {
        let y = Some(15.0);
        assert_eq!(
            scale_step(1, 10_000.0, y, L),
            2,
            "a huge backlog still adds one"
        );
        assert_eq!(scale_step(8, 10_000.0, y, L), 8, "capped at max");
        assert_eq!(scale_step(5, 0.0, y, L), 4, "an empty queue removes one");
        assert_eq!(scale_step(1, 0.0, y, L), 1, "never below min");
        let pinned = SealLimits { min: 3, max: 3 };
        assert_eq!(scale_step(3, 0.0, y, pinned), 3);
        assert_eq!(scale_step(3, 1_000.0, y, pinned), 3);
    }

    #[test]
    fn step_holds_until_a_seal_time_is_known() {
        assert_eq!(scale_step(1, 10_000.0, None, L), 1);
        assert_eq!(scale_step(1, 10_000.0, Some(0.0), L), 1);
    }

    #[test]
    fn resolve_clamps_to_valid_limits() {
        let t = |min, max| IndexersTuning { min, max };
        assert_eq!(
            SealLimits::resolve(&t(2, Some(5))),
            SealLimits { min: 2, max: 5 }
        );
        assert_eq!(
            SealLimits::resolve(&t(4, Some(2))),
            SealLimits { min: 4, max: 4 }
        );
        assert_eq!(
            SealLimits::resolve(&t(0, Some(1))),
            SealLimits { min: 1, max: 1 }
        );
        assert!(SealLimits::resolve(&t(1, None)).max >= 1);
    }

    /// Bursts of 36 files every 12 s, as lockstep rotation produces: the
    /// time-averaged queue, not where a sample lands, drives the step.
    #[test]
    fn scaler_averages_the_queue_over_the_window() {
        let t0 = Instant::now();
        let at = |s: f64| t0 + Duration::from_secs_f64(s);
        let mut sc = Scaler::new(SealLimits { min: 2, max: 8 }, t0);
        // 36 queued for 5 s, empty for 5 s: average 18.
        sc.observe_queue(at(0.0), 36);
        sc.observe_queue(at(5.0), 0);
        // 0.666 s seals: Y ~= 15.
        for _ in 0..3 {
            sc.seal_done(Duration::from_millis(666));
        }
        assert_eq!(sc.tick(at(10.0)), None, "avg 18 with Y 15 keeps two seals");
        assert_eq!(sc.target, 2);
        let y = sc.per_seal.unwrap();
        assert!((y - 15.0).abs() < 0.1, "Y = {y}");
    }

    #[test]
    fn scaler_ramps_a_startup_backlog_one_seal_per_interval() {
        let t0 = Instant::now();
        let mut sc = Scaler::new(L, t0);
        sc.observe_queue(t0, 391);
        let mut targets = Vec::new();
        for i in 1..=9 {
            sc.seal_done(Duration::from_millis(666));
            sc.tick(t0 + SCALE_INTERVAL * i);
            targets.push(sc.target);
        }
        assert_eq!(targets, vec![2, 3, 4, 5, 6, 7, 8, 8, 8]);
        // Backlog gone: one seal less per interval, back to min.
        sc.observe_queue(t0 + SCALE_INTERVAL * 9, 0);
        let mut down = Vec::new();
        for i in 10..=17 {
            sc.tick(t0 + SCALE_INTERVAL * i);
            down.push(sc.target);
        }
        assert_eq!(down, vec![7, 6, 5, 4, 3, 2, 1, 1]);
    }

    fn summary() -> (sfst::Summary, u64) {
        (
            sfst::Summary {
                min_timestamp_s: 0,
                max_timestamp_s: 0,
                record_count: 0,
                content_meta: Vec::new(),
            },
            0,
        )
    }

    /// Hold the slot long enough for every request to queue, recording the
    /// peak number of concurrent seals in `peak`.
    fn slow(running: &AtomicUsize, peak: &AtomicUsize) {
        let now = running.fetch_add(1, Ordering::SeqCst) + 1;
        peak.fetch_max(now, Ordering::SeqCst);
        std::thread::sleep(Duration::from_millis(300));
        running.fetch_sub(1, Ordering::SeqCst);
    }

    static PINNED_RUNNING: AtomicUsize = AtomicUsize::new(0);
    static PINNED_PEAK: AtomicUsize = AtomicUsize::new(0);
    fn pinned_seal(
        _: &Path,
        _: &Path,
        _: &ng_index::Metrics,
    ) -> Result<(sfst::Summary, u64), ng_index::Error> {
        slow(&PINNED_RUNNING, &PINNED_PEAK);
        Ok(summary())
    }

    static ADAPTIVE_RUNNING: AtomicUsize = AtomicUsize::new(0);
    static ADAPTIVE_PEAK: AtomicUsize = AtomicUsize::new(0);
    fn adaptive_seal(
        _: &Path,
        _: &Path,
        _: &ng_index::Metrics,
    ) -> Result<(sfst::Summary, u64), ng_index::Error> {
        slow(&ADAPTIVE_RUNNING, &ADAPTIVE_PEAK);
        Ok(summary())
    }

    /// Queue 8 requests and wait for all 8 responses.
    async fn run_eight(seal: SealFn, limits: SealLimits) {
        let cancel = CancellationToken::new();
        let handle =
            ComponentHandle::spawn::<Indexer>(IndexerArgs { seal, limits }, cancel.clone());
        let (tx, mut rx) = handle.into_parts();
        for i in 0..8 {
            tx.send(IndexerRequest::Index {
                wal_path: PathBuf::from(format!("w{i}")),
                sfst_path: PathBuf::from(format!("s{i}")),
            })
            .unwrap();
        }
        for _ in 0..8 {
            let resp = tokio::time::timeout(Duration::from_secs(10), rx.recv())
                .await
                .expect("seal response")
                .expect("indexer alive");
            assert!(matches!(resp, IndexerResponse::Indexed { .. }));
        }
        cancel.cancel();
    }

    #[tokio::test]
    async fn min_seals_run_concurrently_from_the_start() {
        run_eight(pinned_seal, SealLimits { min: 4, max: 4 }).await;
        assert_eq!(PINNED_PEAK.load(Ordering::SeqCst), 4);
    }

    #[tokio::test]
    async fn a_burst_does_not_add_seals_before_an_interval_passes() {
        // 8 x 300 ms on one seal finishes well inside the first interval.
        run_eight(adaptive_seal, SealLimits { min: 1, max: 8 }).await;
        assert_eq!(ADAPTIVE_PEAK.load(Ordering::SeqCst), 1);
    }
}
