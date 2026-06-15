# NetFlow Tier Commit Workers

Durable contract for the netflow plugin's rollup-tier commit offload
(`src/crates/netflow-plugin/src/ingest/tier_commit.rs` and its wiring in
`ingest/service/runtime.rs`). Shipped with the tier-offload branch stacked on
the systemd-journal-sdk migration.

## Ownership model

- The main ingest (receive) thread does ALL flow work: decode, raw journal
  writes, rollup aggregation into per-tier accumulators, facet observation,
  and rollup row preparation state (flow-index interning).
- Three worker threads (`nf-tier-1m`, `nf-tier-5m`, `nf-tier-1h`; 512 KiB
  stacks) ONLY commit closed, epoch-keyed rollup buckets to their tier's
  journal — one `Log::sync` per claimed batch.
- Only the main thread allocates or frees bucket containers. Workers return
  cleared containers (`HashMap::clear` retains capacity; entry types are
  `Copy`) through the slot; the accumulator recycles them from a free pool.
  Steady state is allocation-free on both sides.
- Each worker exclusively owns its tier's `Log`. `Log` is `Send` and `!Sync`;
  a compile probe (`journal_log_is_send`) guards SDK regressions.

## Doorbell protocol

- One cache-line-isolated `AtomicU32` (`#[repr(align(64))]`) carries one
  request bit per tier. The receive thread's per-packet cost is ONE relaxed
  load (`has_requests`); responding costs a short per-tier mutex hold a few
  times per minute.
- Worker cycle: sleep to the tier's next epoch anniversary (+1s/+2s/+3s
  stagger for 1m/5m/1h), raise the bit, block on the slot condvar until the
  generation changes, take `pending`, commit, fsync, return containers and
  telemetry, repeat.
- The main thread responds (per packet and on the 1s tick): move closed
  buckets into `slot.pending`, record in-flight hours, take `recycled`,
  clear the bit, bump `generation`, `notify_one` — all under the slot mutex.
- An empty respond still bumps the generation: the worker wakes, takes an
  empty batch, stamps liveness, and sleeps to its next anniversary.
- LOAD-BEARING INVARIANTS (regressions here were real bugs caught in
  review/validation):
  - Every condvar notify MUST happen under the slot mutex. The wait
    predicates read the shutdown flag under the mutex; an unguarded notify
    can land between a predicate check and the re-wait — a lost wakeup that
    leaves the worker sleeping toward its next anniversary (or forever, in
    the no-timeout doorbell wait) until the join deadline abandons it WITH
    ITS FINAL BATCH UNCOMMITTED. Symptom signature: shutdown wall times
    equal to the 30s join deadline.
  - Worker wait predicates are evaluated under the slot mutex
    (`wait_timeout` in a manual loop at the anniversary site; `wait_while`
    at the doorbell site) — lost wakeups impossible by construction only
    when BOTH sides honor the mutex.

## Stretch semantics

- If a worker misses an anniversary (slow disk, spawn backlog), closed
  buckets accumulate and the next claim hands them all in one batch — a 1m
  rollup may become a 70-80s rollup. No data loss, bounded memory (the
  accumulator's containers are the bound). Counted per tier as
  `stretched_commits` (a claim handing >1 bucket).
- Keep-up guarantee: a worker writes at most as many rows as the raw tier
  wrote in the same window, write-only, on the same disk.

## Shutdown sequence (order matters)

1. One final respond per tier — fills every slot's `pending` under its mutex
   BEFORE the flag is raised, so a draining worker always finds its final
   batch (the flag store happens after the responds on the same thread).
2. `begin_shutdown`: store the flag, then `notify_all` each slot UNDER its
   mutex (see load-bearing invariants).
3. Join workers synchronously: shared ~30s deadline; on expiry log the stuck
   tier by name and keep checking the remaining handles.
4. After the joins: final facet persist, then decoder persist. The raw
   journal sync stays on the main thread.
- Workers observing shutdown drain `pending` best-effort (write failures
  logged and counted, never abort during drain), final sync, exit.
- Open (incomplete) buckets are dropped exactly as the pre-worker design did
  and recovered by rebuild-from-raw on next start.

## Failure policy

- Worker panic: `catch_unwind` then `std::process::abort()` — a poisoned
  slot must never silently stall a tier (the agent restarts the plugin;
  rebuild recovers the tiers). Poisoned slot mutexes abort likewise.
- Three consecutive `Log::sync` failures on a tier abort the process
  (long-retention tiers fail loud). A poisoned flow-index read lock in
  `commit_batch` aborts (a silent return would drop the rest of the batch).

## Lock discipline

- `commit_batch` takes the tier-flow-index READ lock PER ROW and drops it
  before the journal write. The main thread takes per-flow WRITE locks. Do
  NOT "optimize" this into one batch-wide read guard — bounding the main
  thread's write-lock waits is the single most important property of the
  design. Measured: avg 8-12µs main-thread write-lock wait under a
  continuously hammering reader (production duty cycle is ≤once per minute);
  gates live as `#[ignore]` tests in `tier_commit.rs` (run with
  `--test-threads=1`).
- The tick's prune holds the store WRITE lock through
  `prune_unused_hours`; hours of buckets handed to (or being committed by)
  workers are unioned from the slots' `in_flight_hours` so the prune never
  drops an hour a worker still needs. Measured drop-hold for a 200k-flow
  hour: 421µs (budget 500ms).

## Telemetry contract

- `TierSlot` records `last_commit_usec` (stamped on EVERY completed claim,
  including empty ones — age measures worker liveness/commit lag, NOT tier
  traffic), `last_commit_duration_usec`, `committed_batches`,
  `stretched_commits`.
- The 1s tick mirrors slot telemetry into 12 `IngestMetrics` atomics; charts
  `netflow.tier_commit_{age,duration,batches,stretched}` (1m/5m/1h dims).
  Never-claimed slots report age 0, not distance-to-epoch.
- The live (open) tier snapshot refreshes on the 1s tick (moved off the
  per-packet path): API/Function consumers can see the in-progress rollup
  lag raw by up to one second. Documented in the network-flows
  troubleshooting page.

## Validation evidence (2026-06-11)

- Five-reviewer step gates: steps 2-7 unanimous production grade across four
  cumulative rounds; two real bugs found and fixed by the process (shutdown
  respond/signal order inversion; lost shutdown wakeup from an unguarded
  notify).
- Boundary soak across a real HH:00:00: 1m+5m+1h workers committed ~22.5k
  rows each within their staggers under sustained 31.9k flows/s — 0 kernel
  drops, sent==recv exactly, 0 stretched windows.
- SIGKILL mid-stretch-batch: rebuild survived torn tails, zero duplicate
  tier rows, second rebuild a strict no-op.
- Benchmarks vs pre-offload baseline: microbench parity within ±3-10%
  run-to-run noise; production-shaped paced envelope clearly better at high
  cardinality (29.9% vs 42.5% CPU at 20k flows/s; lower peak RSS), and
  benchmark methodology requires alternating repeat runs — single runs
  mislead.
- Crash-window semantics (characterized at step 0, unchanged): a torn tier
  row at the per-tier cutoff is lost, never duplicated; late arrivals inside
  covered buckets are not re-derived.
