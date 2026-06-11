//! Tier commit handoff: the machinery that lets per-tier worker threads
//! commit closed rollup buckets to their journals without the receive thread
//! ever blocking on tier disk I/O.
//!
//! Ownership model (see the SOW's "Implementation specification"):
//! - the main thread exclusively owns the accumulators and responds to
//!   doorbell requests by moving closed, epoch-keyed buckets into a slot;
//! - each worker exclusively owns its tier's `Log` and commits what it is
//!   handed, one fsync per batch;
//! - containers ping-pong back through the slot for allocation-free reuse.
//!
//! `commit_batch` has two callers — the pre-worker inline path (rebuild,
//! in-process tests, benchmarks) and the workers spawned here — and the
//! shared `flush_closed_tiers` goes through it so the inline test suite
//! proves behavioral equivalence with the worker path.

use super::*;
use crate::tiering::MetricBucket;
use std::collections::BTreeSet;
use std::sync::Condvar;
use std::sync::Mutex;
use std::sync::atomic::AtomicBool;
use std::sync::atomic::AtomicU32;
use std::time::Instant;

/// Wake stagger after each tier's epoch boundary (1m/5m/1h order). Content is
/// epoch-keyed at accumulation time, so stagger only spreads disk work.
const TIER_WAKE_STAGGER_USEC: [u64; 3] = [1_000_000, 2_000_000, 3_000_000];
const WORKER_STACK_BYTES: usize = 512 * 1024;
const SHUTDOWN_JOIN_TIMEOUT: Duration = Duration::from_secs(30);

fn tier_bit(index: usize) -> u32 {
    1 << index
}

/// Doorbell + per-tier exchange slots shared between the receive thread and
/// the commit workers. `request_flags` lives alone on its cache line: the
/// receive thread loads it once per packet, and it must never false-share
/// with slot state.
#[repr(align(64))]
struct Doorbell(AtomicU32);

pub(super) struct TierHandoffShared {
    request_flags: Doorbell,
    shutdown: AtomicBool,
    slots: [TierSlotSync; 3],
}

struct TierSlotSync {
    slot: Mutex<TierSlot>,
    ready: Condvar,
}

#[derive(Default)]
struct TierSlot {
    /// main -> worker: closed, epoch-keyed buckets.
    pending: Vec<(u64, MetricBucket)>,
    /// worker -> main: cleared containers, capacity retained.
    recycled: Vec<MetricBucket>,
    /// Bumped by the main thread on every response; the workers' wait
    /// predicate (immune to spurious wakeups and lost notifies).
    generation: u64,
    /// Hours covered by `pending` — unioned into the prune's active set so
    /// flow-index entries a worker still needs are never pruned mid-commit.
    in_flight_hours: BTreeSet<u64>,
    /// Observability, mirrored into charts by the tick. `last_commit_usec`
    /// is stamped on EVERY completed claim — including empty ones — so its
    /// age measures worker liveness (commit lag), not tier traffic.
    last_commit_usec: u64,
    last_commit_duration_usec: u64,
    committed_batches: u64,
    /// Claims that handed over more than one closed bucket: the worker
    /// missed at least one anniversary (or caught up a spawn backlog) and
    /// the window stretched, exactly as the design allows.
    stretched_commits: u64,
}

/// One tier's commit telemetry, copied out of the slot by the tick and
/// mirrored into `IngestMetrics` for the charts sampler.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub(super) struct TierCommitTelemetry {
    pub(super) last_commit_usec: u64,
    pub(super) last_commit_duration_usec: u64,
    pub(super) committed_batches: u64,
    pub(super) stretched_commits: u64,
}

impl TierHandoffShared {
    pub(super) fn new() -> Self {
        let slot = || TierSlotSync {
            slot: Mutex::new(TierSlot {
                pending: Vec::with_capacity(4),
                recycled: Vec::with_capacity(4),
                ..TierSlot::default()
            }),
            ready: Condvar::new(),
        };
        Self {
            request_flags: Doorbell(AtomicU32::new(0)),
            shutdown: AtomicBool::new(false),
            slots: [slot(), slot(), slot()],
        }
    }

    /// The receive thread's per-packet cost: one relaxed load.
    pub(super) fn has_requests(&self) -> bool {
        self.request_flags.0.load(Ordering::Relaxed) != 0
    }

    pub(super) fn requested(&self, index: usize) -> bool {
        self.request_flags.0.load(Ordering::Relaxed) & tier_bit(index) != 0
    }

    /// Main-thread response: move `taken` buckets in, recycled containers
    /// out, clear the doorbell bit, bump the generation, wake the worker.
    /// Returns the containers the worker has finished with.
    pub(super) fn respond(
        &self,
        index: usize,
        taken: Vec<(u64, MetricBucket)>,
        in_flight_hours: BTreeSet<u64>,
    ) -> Vec<MetricBucket> {
        let sync = &self.slots[index];
        let mut slot = match sync.slot.lock() {
            Ok(slot) => slot,
            Err(poisoned) => {
                // A worker panic aborts the process (see worker_loop); a
                // poisoned slot here means that abort is already in flight.
                tracing::error!("tier slot {index} poisoned; aborting");
                drop(poisoned);
                std::process::abort();
            }
        };
        slot.pending.extend(taken);
        slot.in_flight_hours.extend(in_flight_hours);
        let recycled = std::mem::take(&mut slot.recycled);
        self.request_flags
            .0
            .fetch_and(!tier_bit(index), Ordering::Relaxed);
        slot.generation = slot.generation.wrapping_add(1);
        sync.ready.notify_one();
        recycled
    }

    /// Hours still being committed, unioned by the tick's prune.
    pub(super) fn in_flight_hours(&self, into: &mut BTreeSet<u64>) {
        for sync in &self.slots {
            if let Ok(slot) = sync.slot.lock() {
                into.extend(slot.in_flight_hours.iter().copied());
            }
        }
    }

    pub(super) fn commit_telemetry(&self, index: usize) -> TierCommitTelemetry {
        match self.slots[index].slot.lock() {
            Ok(slot) => TierCommitTelemetry {
                last_commit_usec: slot.last_commit_usec,
                last_commit_duration_usec: slot.last_commit_duration_usec,
                committed_batches: slot.committed_batches,
                stretched_commits: slot.stretched_commits,
            },
            Err(_) => TierCommitTelemetry::default(),
        }
    }

    pub(super) fn begin_shutdown(&self) {
        self.shutdown.store(true, Ordering::Relaxed);
        for sync in &self.slots {
            // Notify under the slot mutex. The workers evaluate their wait
            // predicates (which read `shutdown`) while holding it, so an
            // unguarded notify could land in the gap between a predicate
            // check and the re-wait — a lost wakeup that leaves the worker
            // sleeping toward its next anniversary (or indefinitely in the
            // doorbell wait) until the join deadline abandons it with its
            // final batch uncommitted. Holding the mutex makes that gap
            // unreachable: either the worker is already waiting and receives
            // the notify, or its next predicate check observes the flag.
            let _slot = sync.slot.lock().unwrap_or_else(|_| std::process::abort());
            sync.ready.notify_all();
        }
    }
}

/// Everything a tier commit worker owns. Constructed on the main thread
/// (the SDK `Log` is `Send`) and moved into the worker.
pub(super) struct TierWorker {
    pub(super) tier: TierKind,
    pub(super) index: usize,
    pub(super) writer: Log,
    pub(super) tier_flow_indexes: Arc<RwLock<TierFlowIndexStore>>,
    pub(super) facet_runtime: Arc<crate::facet_runtime::FacetRuntime>,
    pub(super) metrics: Arc<IngestMetrics>,
    pub(super) consecutive_sync_failures: u32,
}

pub(super) fn spawn_tier_workers(
    shared: &Arc<TierHandoffShared>,
    workers: Vec<TierWorker>,
) -> Vec<std::thread::JoinHandle<()>> {
    workers
        .into_iter()
        .map(|worker| {
            let shared = Arc::clone(shared);
            let name = format!("nf-tier-{}", tier_thread_suffix(worker.tier));
            std::thread::Builder::new()
                .name(name)
                .stack_size(WORKER_STACK_BYTES)
                .spawn(move || {
                    // Loud-failure policy: a panicking worker must not leave a
                    // poisoned slot silently stalling its tier (unbounded
                    // accumulator growth). Abort; the agent restarts the
                    // plugin and rebuild-from-raw recovers the tiers.
                    let result = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
                        worker_loop(&shared, worker);
                    }));
                    if let Err(panic) = result {
                        tracing::error!("tier commit worker panicked: {:?}", panic);
                        std::process::abort();
                    }
                })
                .expect("spawn tier commit worker")
        })
        .collect()
}

fn tier_thread_suffix(tier: TierKind) -> &'static str {
    match tier {
        TierKind::Minute1 => "1m",
        TierKind::Minute5 => "5m",
        TierKind::Hour1 => "1h",
        TierKind::Raw => "raw",
    }
}

fn worker_loop(shared: &TierHandoffShared, mut worker: TierWorker) {
    let bucket_usec = worker
        .tier
        .bucket_duration()
        .expect("materialized tier has a bucket duration")
        .as_micros() as u64;
    let stagger = TIER_WAKE_STAGGER_USEC[worker.index];
    let mut encode_buf = JournalEncodeBuffer::new();
    let mut seen_generation = 0_u64;
    // Claim once immediately: buckets that closed while a long rebuild ran
    // commit now instead of waiting for the first anniversary.
    let mut next_wake = Instant::now();

    loop {
        // Sleep to the next anniversary (or immediately on the first pass),
        // waking early on shutdown or an early response. All blocking goes
        // through wait_timeout_while with the predicate evaluated under the
        // slot mutex — lost wakeups are impossible by construction.
        let sync = &shared.slots[worker.index];
        {
            let mut slot = sync.slot.lock().unwrap_or_else(|_| std::process::abort());
            while !shared.shutdown.load(Ordering::Relaxed) {
                let now = Instant::now();
                if now >= next_wake {
                    break;
                }
                let (next, _timeout) = sync
                    .ready
                    .wait_timeout(slot, next_wake - now)
                    .unwrap_or_else(|_| std::process::abort());
                slot = next;
            }
        }
        if shared.shutdown.load(Ordering::Relaxed) {
            drain_and_exit(shared, &mut worker, bucket_usec, &mut encode_buf);
            return;
        }

        // Raise the doorbell and wait for the main thread's response.
        shared
            .request_flags
            .0
            .fetch_or(tier_bit(worker.index), Ordering::Relaxed);
        let batch = {
            let slot = sync.slot.lock().unwrap_or_else(|_| std::process::abort());
            let mut slot = sync
                .ready
                .wait_while(slot, |slot| {
                    slot.generation == seen_generation
                        && !shared.shutdown.load(Ordering::Relaxed)
                })
                .unwrap_or_else(|_| std::process::abort());
            seen_generation = slot.generation;
            std::mem::take(&mut slot.pending)
        };

        commit_and_return(shared, &mut worker, bucket_usec, &mut encode_buf, batch);

        if shared.shutdown.load(Ordering::Relaxed) {
            drain_and_exit(shared, &mut worker, bucket_usec, &mut encode_buf);
            return;
        }

        next_wake = next_anniversary(bucket_usec, stagger);
    }
}

/// Commit a claimed batch, fsync once, return the cleared containers and
/// telemetry through the slot.
fn commit_and_return(
    shared: &TierHandoffShared,
    worker: &mut TierWorker,
    bucket_usec: u64,
    encode_buf: &mut JournalEncodeBuffer,
    batch: Vec<(u64, MetricBucket)>,
) {
    if batch.is_empty() {
        // Nothing to write, but the claim cycle completed: stamp liveness so
        // the commit-age metric tracks a stuck worker, not an idle tier.
        let sync = &shared.slots[worker.index];
        let mut slot = sync.slot.lock().unwrap_or_else(|_| std::process::abort());
        // Already empty here (an empty respond extends nothing and the prior
        // commit cleared it); the clear is defense in depth, not a fix.
        slot.in_flight_hours.clear();
        slot.last_commit_usec = now_usec();
        return;
    }
    let stretched = batch.len() > 1;
    let started = Instant::now();
    commit_batch(
        worker.tier,
        bucket_usec,
        &batch,
        &worker.tier_flow_indexes,
        &mut worker.writer,
        encode_buf,
        &worker.facet_runtime,
        &worker.metrics,
    );
    sync_with_failure_policy(worker);
    let duration_usec = started.elapsed().as_micros() as u64;

    let mut containers: Vec<MetricBucket> = batch
        .into_iter()
        .map(|(_, mut bucket)| {
            bucket.clear();
            bucket
        })
        .collect();
    let sync = &shared.slots[worker.index];
    let mut slot = sync.slot.lock().unwrap_or_else(|_| std::process::abort());
    slot.recycled.append(&mut containers);
    slot.in_flight_hours.clear();
    slot.last_commit_usec = now_usec();
    slot.last_commit_duration_usec = duration_usec;
    slot.committed_batches = slot.committed_batches.wrapping_add(1);
    if stretched {
        slot.stretched_commits = slot.stretched_commits.wrapping_add(1);
    }
}

/// Per-commit durability for the long-retention tiers, with the loud-failure
/// policy: three consecutive sync failures abort the process.
fn sync_with_failure_policy(worker: &mut TierWorker) {
    if let Err(err) = worker.writer.sync() {
        worker
            .metrics
            .tier_journal_sync_errors
            .fetch_add(1, Ordering::Relaxed);
        worker.consecutive_sync_failures += 1;
        tracing::warn!(
            "tier {:?} journal sync failed ({} consecutive): {}",
            worker.tier,
            worker.consecutive_sync_failures,
            err
        );
        if worker.consecutive_sync_failures >= 3 {
            tracing::error!(
                "tier {:?}: 3 consecutive journal sync failures — aborting (durability tiers must fail loud)",
                worker.tier
            );
            std::process::abort();
        }
    } else {
        worker.consecutive_sync_failures = 0;
    }
}

/// Final drain: claim whatever the main thread posted before the shutdown
/// signal (`finish_shutdown` responds to every tier BEFORE raising the flag,
/// so the batch is always in the slot by the time a worker gets here), commit
/// it best-effort, sync, and exit. Write failures during the drain are logged
/// and counted, never abort.
fn drain_and_exit(
    shared: &TierHandoffShared,
    worker: &mut TierWorker,
    bucket_usec: u64,
    encode_buf: &mut JournalEncodeBuffer,
) {
    let batch = {
        let sync = &shared.slots[worker.index];
        let mut slot = sync.slot.lock().unwrap_or_else(|_| std::process::abort());
        // No prune runs once shutdown begins; cleared for slot hygiene.
        slot.in_flight_hours.clear();
        std::mem::take(&mut slot.pending)
    };
    if !batch.is_empty() {
        commit_batch(
            worker.tier,
            bucket_usec,
            &batch,
            &worker.tier_flow_indexes,
            &mut worker.writer,
            encode_buf,
            &worker.facet_runtime,
            &worker.metrics,
        );
    }
    if let Err(err) = worker.writer.sync() {
        worker
            .metrics
            .tier_journal_sync_errors
            .fetch_add(1, Ordering::Relaxed);
        tracing::warn!("tier {:?} final sync failed: {}", worker.tier, err);
    }
}

/// Wall-clock instant of the next epoch boundary + stagger for this tier.
fn next_anniversary(bucket_usec: u64, stagger_usec: u64) -> Instant {
    let now = now_usec();
    let next_boundary = (now / bucket_usec + 1) * bucket_usec + stagger_usec;
    Instant::now() + Duration::from_micros(next_boundary.saturating_sub(now))
}

/// Join the workers with a deadline; on expiry, log and abandon (the process
/// is exiting anyway).
pub(super) fn join_workers(handles: Vec<std::thread::JoinHandle<()>>) {
    let deadline = Instant::now() + SHUTDOWN_JOIN_TIMEOUT;
    'workers: for handle in handles {
        let name = handle.thread().name().unwrap_or("nf-tier-?").to_string();
        while !handle.is_finished() {
            if Instant::now() >= deadline {
                tracing::error!("{name} did not finish within the shutdown deadline; abandoning");
                // Keep checking the remaining workers: a finished one still
                // joins instantly, and each abandonment is logged by name.
                continue 'workers;
            }
            std::thread::sleep(Duration::from_millis(10));
        }
        if handle.join().is_err() {
            tracing::error!("{name} terminated with a panic during shutdown");
        }
    }
}

/// Commit a batch of closed, epoch-keyed buckets to one tier's journal.
///
/// Row expansion happens here (off the receive thread once workers own it):
/// each bucket row gets `timestamp = bucket_end - 1`, exactly like the legacy
/// `flush_closed_rows`. The flow-index read lock is taken PER ROW and released
/// before the journal write — the main thread takes a write lock per flow, so
/// a batch-wide read lock would stall it for the whole commit (the single most
/// important property of this design; do not "optimize" it into one guard).
///
/// Row-level failures (emit misses, write errors) are logged and counted,
/// never fatal: a batch commits everything that can be committed. The caller
/// owns the once-per-batch `Log::sync` durability decision.
#[allow(clippy::too_many_arguments)]
pub(super) fn commit_batch(
    tier: TierKind,
    bucket_usec: u64,
    batch: &[(u64, MetricBucket)],
    tier_flow_indexes: &RwLock<TierFlowIndexStore>,
    writer: &mut Log,
    encode_buf: &mut JournalEncodeBuffer,
    facet_runtime: &crate::facet_runtime::FacetRuntime,
    metrics: &IngestMetrics,
) {
    for (bucket_start, bucket) in batch {
        let row_timestamp_usec = bucket_start
            .saturating_add(bucket_usec)
            .saturating_sub(1);
        for (&flow_ref, &row_metrics) in bucket {
            let contribution = {
                // Poison means a thread panicked holding the store; the
                // module's loud-failure policy applies — a silent return here
                // would drop the rest of the batch on the floor.
                let indexes = tier_flow_indexes.read().unwrap_or_else(|_| {
                    tracing::error!("tier flow index store poisoned; aborting");
                    std::process::abort()
                });
                indexes.emit_row(flow_ref, row_metrics, encode_buf)
            };
            let Some(contribution) = contribution else {
                tracing::warn!("failed to emit tier flow {:?} for {:?}", flow_ref, tier);
                continue;
            };
            let logical_bytes = encode_buf.encoded_len();
            let timestamps = EntryTimestamps::default()
                .with_source_realtime_usec(row_timestamp_usec)
                .with_entry_realtime_usec(row_timestamp_usec);
            if let Err(err) = encode_buf.write_encoded(writer, timestamps) {
                metrics.tier_write_errors.fetch_add(1, Ordering::Relaxed);
                tracing::warn!("tier writer {:?} write failed: {}", tier, err);
                continue;
            }
            if let Some(active_file) = writer.active_file() {
                if let Err(err) = facet_runtime
                    .observe_active_contribution(Path::new(active_file.path()), &contribution)
                {
                    tracing::warn!("facet runtime tier {:?} write update failed: {}", tier, err);
                }
            }
            metrics.tier_entries_written.fetch_add(1, Ordering::Relaxed);
            metrics.increment_materialized_tier(tier, logical_bytes);
        }
    }
    if !batch.is_empty() {
        metrics.tier_flushes.fetch_add(1, Ordering::Relaxed);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::flow::FlowRecord;
    use crate::tiering::FlowMetrics;
    use std::sync::Arc;
    use std::sync::atomic::AtomicBool;
    use std::time::{Duration, Instant};

    /// Compile-time gate for the whole worker-ownership design: the SDK's
    /// `Log` must be movable into a thread. (`Log` is `!Sync`, which exclusive
    /// per-worker ownership satisfies.) If an SDK upgrade ever regresses this,
    /// the failure surfaces here, not in a worker spawn at runtime.
    #[test]
    fn journal_log_is_send() {
        fn assert_send<T: Send>() {}
        assert_send::<journal_log_writer::Log>();
    }

    /// Age semantics of the tick mirror: a never-claimed slot reports 0 (not
    /// the distance to the epoch), a claimed slot reports seconds since the
    /// claim, and the raw tier is a no-op.
    #[test]
    fn tier_commit_telemetry_age_semantics() {
        let metrics = IngestMetrics::default();
        let now = 10_000_000_000_u64;

        metrics.store_tier_commit_telemetry(
            TierKind::Minute1,
            now,
            &TierCommitTelemetry::default(),
        );
        assert_eq!(
            metrics.minute_1_commit_age_seconds.load(Ordering::Relaxed),
            0,
            "no claim yet must read as age 0"
        );

        let telemetry = TierCommitTelemetry {
            last_commit_usec: now - 5_000_000,
            last_commit_duration_usec: 1234,
            committed_batches: 7,
            stretched_commits: 2,
        };
        metrics.store_tier_commit_telemetry(TierKind::Minute5, now, &telemetry);
        assert_eq!(
            metrics.minute_5_commit_age_seconds.load(Ordering::Relaxed),
            5
        );
        assert_eq!(
            metrics.minute_5_commit_duration_usec.load(Ordering::Relaxed),
            1234
        );
        assert_eq!(metrics.minute_5_commit_batches.load(Ordering::Relaxed), 7);
        assert_eq!(metrics.minute_5_commit_stretched.load(Ordering::Relaxed), 2);

        metrics.store_tier_commit_telemetry(TierKind::Raw, now, &telemetry);
        assert_eq!(metrics.minute_1_commit_batches.load(Ordering::Relaxed), 0);
        assert_eq!(metrics.hour_1_commit_batches.load(Ordering::Relaxed), 0);
    }

    /// Manual measurement gate (SOW step 5, gate b): the tick's prune holds
    /// the flow-index store WRITE lock while `BTreeMap::retain` drops
    /// aged-out hours — at high cardinality that drop frees an entire
    /// hour's FlowIndex under the lock, stalling the receive thread (and
    /// any worker read). Two properties gated: the per-second steady-state
    /// prune (nothing to drop) must be trivially cheap, and the
    /// once-per-hour drop of a 200k-flow hour must stay well inside the
    /// socket buffer's ~1s absorption. Fallback knob if this regresses:
    /// prune every 10s instead of every tick.
    #[test]
    #[ignore = "manual prune-hold measurement gate (timing-sensitive)"]
    fn prune_write_lock_hold_is_bounded_at_high_cardinality() {
        const FLOWS_PER_HOUR: u32 = 200_000;
        const HOUR_USEC: u64 = 3_600_000_000;

        let hour_a = (1_700_000_000_000_000_u64 / HOUR_USEC) * HOUR_USEC;
        let hour_b = hour_a + HOUR_USEC;
        let store = std::sync::RwLock::new(TierFlowIndexStore::default());
        {
            let mut guard = store.write().expect("populate store");
            for seed in 0..FLOWS_PER_HOUR {
                let spread = (seed as u64 % 3_600) * 1_000_000;
                let _ = guard.get_or_insert_record_flow(hour_a + spread, &flow_record(seed));
                let _ = guard.get_or_insert_record_flow(hour_b + spread, &flow_record(seed));
            }
        }

        // Steady state: the every-second prune with nothing to drop.
        let keep_both: BTreeSet<u64> = [hour_a, hour_b].into_iter().collect();
        let mut noop_waits = Vec::with_capacity(100);
        for _ in 0..100 {
            let begin = Instant::now();
            let mut guard = store.write().expect("noop prune");
            guard.prune_unused_hours(&keep_both);
            drop(guard);
            noop_waits.push(begin.elapsed().as_micros() as u64);
        }
        let noop_p99 = p99_usec(noop_waits);
        assert!(
            noop_p99 <= 1_000,
            "steady-state prune held the write lock {noop_p99}µs at p99 — \
             the per-second tick cannot afford this"
        );

        // Hour transition: drop the 200k-flow hour A under the lock.
        let keep_b: BTreeSet<u64> = [hour_b].into_iter().collect();
        let begin = Instant::now();
        let mut guard = store.write().expect("drop prune");
        guard.prune_unused_hours(&keep_b);
        drop(guard);
        let drop_hold_usec = begin.elapsed().as_micros() as u64;
        eprintln!("prune drop-hold for a {FLOWS_PER_HOUR}-flow hour: {drop_hold_usec}µs");
        assert!(
            drop_hold_usec <= 500_000,
            "dropping a {FLOWS_PER_HOUR}-flow hour held the write lock \
             {drop_hold_usec}µs — exceeds the drop-safety budget; consider \
             the 10s prune cadence fallback"
        );
    }

    fn flow_record(seed: u32) -> FlowRecord {
        let mut rec = FlowRecord::default();
        rec.protocol = 6;
        rec.src_as = seed;
        rec.bytes = 1;
        rec.packets = 1;
        rec
    }

    fn p99_usec(samples: Vec<u64>) -> u64 {
        let (stats, p99) = lock_wait_stats(samples);
        eprintln!("{stats}");
        p99
    }

    fn lock_wait_stats(mut samples: Vec<u64>) -> (String, u64) {
        samples.sort_unstable();
        let len = samples.len();
        let pick = |q: usize| samples[(len.saturating_sub(1)).min(len * q / 100)];
        let avg = samples.iter().sum::<u64>() / len.max(1) as u64;
        let p99 = pick(99);
        (
            format!(
                "samples={len} avg={avg}µs p50={}µs p90={}µs p99={p99}µs max={}µs",
                pick(50),
                pick(90),
                samples[len - 1]
            ),
            p99,
        )
    }

    /// SOW step-2 gate: a worker taking PER-ROW read locks on the flow-index
    /// store must not stall the main thread's per-flow write locks beyond
    /// ~one row's emit. Manual (timing-sensitive on busy machines):
    /// `cargo test --release -p netflow-plugin tier_commit -- --ignored --nocapture`
    #[test]
    #[ignore = "manual lock-contention gate (timing-sensitive)"]
    fn per_row_read_locks_keep_main_thread_write_waits_bounded() {
        let store = Arc::new(RwLock::new(TierFlowIndexStore::default()));
        let ts = 120_000_000_u64;
        let refs: Vec<_> = (0..20_000_u32)
            .map(|i| {
                store
                    .write()
                    .expect("seed store")
                    .get_or_insert_record_flow(ts, &flow_record(i))
                    .expect("intern flow")
            })
            .collect();

        let stop = Arc::new(AtomicBool::new(false));
        let reader_store = Arc::clone(&store);
        let reader_stop = Arc::clone(&stop);
        let reader = std::thread::spawn(move || {
            let mut encode_buf = JournalEncodeBuffer::new();
            let mut emitted = 0_u64;
            'outer: loop {
                for &flow_ref in &refs {
                    if reader_stop.load(Ordering::Relaxed) {
                        break 'outer;
                    }
                    let guard = reader_store.read().expect("read store");
                    let _ = guard.emit_row(
                        flow_ref,
                        FlowMetrics {
                            bytes: 1,
                            packets: 1,
                        },
                        &mut encode_buf,
                    );
                    drop(guard);
                }
            }
            emitted += 1;
            emitted
        });

        let mut waits = Vec::with_capacity(100_000);
        let started = Instant::now();
        let mut seed = 1_000_000_u32;
        while started.elapsed() < Duration::from_secs(2) {
            seed += 1;
            let begin = Instant::now();
            let mut guard = store.write().expect("write store");
            let acquired = begin.elapsed();
            let _ = guard.get_or_insert_record_flow(ts, &flow_record(seed));
            drop(guard);
            waits.push(acquired.as_micros() as u64);
        }
        stop.store(true, Ordering::Relaxed);
        reader.join().expect("join reader");

        // Gate derivation (measured on this design 2026-06-10, serial run,
        // busy desktop: avg=8µs p50=0µs p90=23µs p99=93µs max=13.9ms): the
        // p50/avg show the lock itself is near-free; the tail is futex-wake +
        // scheduler preemption, which any blocking lock shares. The property
        // that matters is DROP-SAFETY: the reader (worker) is active only
        // during a commit burst (≤ once per minute per tier), and the
        // receive thread's accumulated extra latency during such a burst —
        // avg-wait × rows-in-burst — must stay within the UDP socket
        // buffer's absorption (~1s at the 64MiB request and ~100k flows/s).
        // At the 25µs budget a 30k-row burst accumulates ~750ms (measured
        // 8-12µs ⇒ ~250-350ms, a 2-3x margin), spread over the burst's
        // multi-second commit window rather than a single stall; max ≤ 50ms
        // bounds any single preemption spike to a trivial queue depth. A
        // p99-under-continuous-saturation bound mis-measures the production
        // duty cycle and was retired after measurement.
        let (stats, _) = lock_wait_stats(waits.clone());
        eprintln!("main-thread write-lock acquire under continuous reader: {stats}");
        let avg = waits.iter().sum::<u64>() / waits.len().max(1) as u64;
        let max = waits.iter().copied().max().unwrap_or(0);
        assert!(
            avg <= 25,
            "write-lock avg {avg}µs exceeds the drop-safety budget — per-row \
             read locking is not bounding the main thread"
        );
        assert!(
            max <= 50_000,
            "write-lock max {max}µs — a single stall this long indicates a \
             held-across-I/O lock, not scheduler noise"
        );
    }

    /// SOW step-2 gate: worker-side facet contributions contending with
    /// main-thread per-flow facet observes. Decides per-row vs per-commit
    /// batched facet updates in the worker. Manual.
    #[test]
    #[ignore = "manual facet-convoy gate (timing-sensitive)"]
    fn facet_lock_convoy_from_worker_contributions_is_bounded() {
        let tmp = tempfile::tempdir().expect("tempdir");
        let runtime = Arc::new(crate::facet_runtime::FacetRuntime::new(tmp.path()));
        let fields = flow_record(7).to_fields();
        let contribution = crate::facet_runtime::facet_contribution_from_flow_fields(&fields);
        let worker_path = tmp.path().join("tier-file.journal");
        let main_path = tmp.path().join("raw-file.journal");

        let stop = Arc::new(AtomicBool::new(false));
        let worker_runtime = Arc::clone(&runtime);
        let worker_stop = Arc::clone(&stop);
        let worker = std::thread::spawn(move || {
            while !worker_stop.load(Ordering::Relaxed) {
                worker_runtime
                    .observe_active_contribution(&worker_path, &contribution)
                    .expect("observe contribution");
            }
        });

        let record = flow_record(9);
        let mut waits = Vec::with_capacity(100_000);
        let started = Instant::now();
        while started.elapsed() < Duration::from_secs(2) {
            let begin = Instant::now();
            runtime
                .observe_active_record(&main_path, &record)
                .expect("observe record");
            waits.push(begin.elapsed().as_micros() as u64);
        }
        stop.store(true, Ordering::Relaxed);
        worker.join().expect("join worker");

        let p99 = p99_usec(waits);
        eprintln!("main-thread facet observe p99 under worker contention: {p99} µs");
        assert!(
            p99 < 100,
            "facet observe p99 {p99}µs under contention — batch worker \
             contributions per commit instead of per row"
        );
    }
}
