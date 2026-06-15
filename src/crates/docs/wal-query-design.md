# Querying WAL files at query time

Working design document. Status: design settled, implementation not started.

## What we are trying to achieve

Logs accepted by the ingestor are durable immediately — every export batch
is appended to a per-stream WAL file and fsynced before the gRPC call
returns — but queryable only after the WAL rotates and the indexer builds
an SFST from it. With the default 25,000-entry rotation and a moderate
stream, that is roughly half an hour of data that exists on disk, survives
crashes, and is invisible to every dashboard query.

The goal: queries whose time window overlaps a WAL file (active or sealed
but not yet indexed) must see that data, with results indistinguishable
from what the same data would produce after indexing.

## The approach

Three ideas compose. Together they keep per-query work bounded by
constants we control, regardless of ingest rate or rotation configuration.

**1. Lazy chunked indexing of the WAL interior.** When a query needs an
active WAL — and only then — the ledger indexes the WAL's durable prefix
in chunks of ≥ 16,384 entries (configurable). A chunk ends at the first
frame boundary at or past the threshold (frames are never split), and its
end offset / entry count are recorded when it is built. Each chunk is
built exactly once per WAL lifetime, by the production indexer, into an
in-memory SFST byte image. Chunks are never rebuilt and never extended.

**2. A direct row-loop evaluator for the tail.** The durable suffix past
the last chunk boundary — less than one chunk at evaluation time, by
construction — is evaluated directly: decode its frames into rows and
compute filter matches, facet counts, histogram contributions, and page
rows in a plain loop. No index is built for the tail; the work is redone
per query and is bounded by the chunk-size constant.

**3. Everything is invisible outside the ledger's query path.** The
planner sees one WAL candidate per file (machinery that already exists,
currently unwired). The chunk byte images live in a ledger-private map
keyed `(wal_seq, chunk_index)` — no file ids, no registry entries, no
on-disk presence. Lost on restart, rebuilt on demand, dropped at rotation
when the normal full-WAL indexing supersedes them.

A WAL that is never queried while active takes the existing pipeline
byte-identically: rotate, index whole, delete. Chunks are pure query-side
acceleration state.

A compact way to state the cost model: this is on-demand indexing with
memoization at 16K granularity. The first query against a long-running
WAL pays one full indexing pass over the prefix (split into chunk
builds, singleflighted); after that, no chunk is ever re-indexed, and the
steady-state marginal cost per query is the O(16K) tail loop.

## Decisions made (and their rationale)

- **Chunk boundary is an entry count, not a byte budget.** Across
  deployments, ingest *rate* varies by orders of magnitude;
  bytes-per-entry varies by roughly one. The knob covers outliers
  (deployments with huge entries tune it down). Pathological scale
  (hundreds of streams, enormous entries) is explicitly the user's
  responsibility to handle via configuration and infrastructure layout —
  we design for the typical deployment out of the box, not for all
  conceivable ones.
- **Row-loop for the tail, not a mini-index.** Same asymptotics (both
  decode the tail per query); the row-loop is simpler on a bounded input
  and doubles as a reference oracle for property-testing the SFST engine.
- **Chunking is query-triggered.** No background work; the no-query path
  stays identical to today.
- **First-touch cost is accepted.** The first query against a cold WAL
  does O(prefix) work, once, singleflighted. We do not bound it in v1;
  window-limited chunk building is a possible later refinement.
- **The row-loop shares the frame→row decoding with the indexer.** The
  decode (WAL frame → Arrow → rows of `key=value` pairs + timestamps)
  is factored out of the indexer's frame processing and used by both.
  Only the *evaluation* is implemented twice, which is exactly what the
  equivalence tests then cover.

## Design rules (invariants the implementation must keep)

1. **Never read past `valid_up_to`.** The writer's buffered output can
   flush mid-frame, so the file tail past the last fsync may be torn.
   `valid_up_to` (carried by every `Synced` event; frame-aligned by
   construction) is the only safe boundary. Cheap validations: file
   length ≥ `valid_up_to`; room for a complete frame header before each
   read; decoded entry count cross-checked against the event's count.
2. **One snapshot per query.** The engine makes two passes (statistics,
   then pagination); both must see identical data. The handler captures
   — once, into one value — the WAL's `valid_up_to`, the list of built
   chunks with end offsets, and the resulting tail byte range; both
   passes consume only that frozen value. Without this, a chunk built or
   a sync arriving mid-query makes counts disagree with rows.
3. **Chunks are built once, immutable, singleflighted.** Concurrent
   queries needing the same chunk wait for the in-flight build.
4. **Per-source failures degrade gracefully.** One bad chunk/tail/file
   never sinks the response. If a WAL file vanishes mid-query (indexing
   completed and deleted it), re-consult the registry: the SFST is
   registered *before* WAL deletion is dispatched, so the fallback always
   finds it.
5. **The row-loop must produce a complete `LogsShard`** — including a
   full field table with per-field distinct-value counts over *all*
   fields in the tail (not just filtered ones), classified with the same
   cardinality threshold the indexer uses. The shard merge is an
   associative monoid; the row-loop's shard composes with chunk shards
   and on-disk SFST shards for free, but only if its field table is
   complete and its tier assignment matches.

## Known, accepted imperfections

- **Cursor drift at two seams.** Pagination cursors embed a row's
  time-sorted position. Positions reshuffle when tail rows are promoted
  into a chunk (arrival order → time-sorted) and when rotation replaces
  chunks with the final SFST. A user paging a live stream across a seam
  can see duplicated or skipped rows at a page boundary — potentially a
  run of them when many rows share a timestamp. Decision deferred to the
  pagination work: either document active-WAL pagination as best-effort
  (plus a mismatch metric, broken down by seam), or use the stable WAL
  arrival ordinal as the tiebreaker for WAL-backed rows, which eliminates
  the first seam entirely.
- **Double indexing for queried WALs.** A WAL queried while active gets
  indexed twice (chunks, then the final SFST). Only queried WALs pay.
- **Tail-size invariant is strict only after warm-up.** During the
  first-touch build sequence, ingestion keeps advancing `valid_up_to`,
  so the tail can transiently exceed one chunk. Self-corrects on the
  next query.
- **Chunk-cache eviction policy** (byte budget, min-age anti-thrash,
  pinning) is deliberately deferred until that functionality is built.

## High-level implementation steps

### Milestone 1 — WAL scan evaluator, proven equivalent to SFST

The cornerstone, and deliberately first: everything else is bookkeeping
around it. Self-contained at the crate level — no ledger, planner, or IPC
involvement.

- Factor the frame→rows decode out of the indexer's frame processing so
  the indexer and the row-loop consume the same row stream.
- Implement the row-loop evaluator: given a row stream and a `LogsQuery`,
  produce a `LogsShard` (matched, facets, timeline with `unset`, complete
  field table). **Done** (`sfsq::logs::WalScan`).
- Build the equivalence harness: for generated WAL fixtures, index the
  same data into an SFST, run the engine on it, run the row-loop on the
  row stream, and assert identical output. **Done**
  (`sfsq/tests/wal_equivalence.rs`); covers multi-valued fields (repeated
  keys), missing fields (counts toward `unset`), cardinality-threshold
  boundaries, bucket-edge / equal-run / out-of-order / fallback-tier
  timestamps, degenerate inputs, byte-oriented regex over multi-byte
  UTF-8, and the cross-field `=` / absent-field regressions.

Exit criterion (**statistics: met**): the row-loop's `LogsShard` is
indistinguishable from index-then-query on the same data, under a
property test, for every query shape we serve.

**Deferred to milestone 4 — row equivalence.** `WalScan` produces only
the statistics shard today; it has no pagination. Materialized page
rows, their global ordering, and the pagination cursor are produced by
the engine's `paginate`, which must interleave chunk-SFST rows and
decoded tail rows under one cursor order — that interleave *is*
milestone 4. Row-level equivalence (and its harness cases) lands there,
not here. "Milestone 1 complete" therefore means statistics
equivalence; the row half is explicitly milestone-4 scope.

### Milestone 2 — bounded and offset WAL reading

- Ledger's WAL registry retains `valid_up_to` and `entry_count` from
  `Synced` events (carried today, currently discarded).
- Reader gains: open-at-offset (offsets come from ledger bookkeeping —
  no WAL format change), stop-at-bound, and the rule-1 validations.

### Milestone 3 — chunk building and the chunk map

- Index a byte range of a WAL into in-memory SFST bytes (the existing
  two-phase build, Phase 2 targeting a `Vec<u8>`).
- Boundary scan: walk frame headers (entry counts, no payload decode) to
  find the first frame boundary ≥ the chunk threshold.
- Ledger-private chunk map keyed `(wal_seq, chunk_index)`, recording each
  chunk's end offset and entry count. Designed to tolerate sparse
  population from day one (cheap now; enables window-limited building
  later). Singleflight on builds. Entries dropped on rotation/eviction.

### Milestone 4 — query-path wiring

- Handler enumerates WAL candidates alongside SFSTs (wire the existing
  planner machinery).
- Per query, capture the rule-2 snapshot; evaluate chunks through the
  unmodified engine (in-memory byte source) and the tail through the
  row-loop; fold all shards via the existing monoid merge.
- Pagination interleave across chunks + tail under the cursor total
  order; resolve the deferred tail-tiebreaker decision here. This is
  where `WalScan` grows row materialization, so the **row half of the
  milestone-1 equivalence** (page rows, ordering, cursors — deferred
  from M1) is proven here, extending the existing harness.

  **Done.** The cursor gained a third sort key —
  `(timestamp_ns, file_seq, part, position)` — distinguishing the
  sub-sources of one active WAL that share a `file_seq`. `part` is the
  typed `Part` enum: `Indexed(n)` for an on-disk SFST (`n = 0`) or an
  in-memory chunk (`n =` chunk index), and `Tail` for the row-scanned
  tail (which sorts after every chunk). It is purely the
  equal-`(timestamp, file_seq)` tiebreaker, and `materialize` routes by a
  named `SourceKey { file_seq, part }` (see M-4 in
  `sfsq-readability-refactors.md`). `WalScan`
  grew `page_shard` + `materialize_rows` over the tail (position = the
  row's insertion index — **tail-tiebreaker decision (a)**: stable while
  the row is in the tail; on promotion into a chunk it flips to the
  chunk-local time-sorted position, the documented seam, to be tracked
  by the M5 mismatch metric). `wal_data_rows_match_whole_file_index`
  proves the live page (chunks + tail) equals the whole-file index's
  page on monotonic-timestamp corpora.

### Milestone 5 — operations

- Chunk-cache governance (budget, min-age, pinning) — design deferred
  until here.
- Metrics: cursor-mismatch counter (by seam), tail evaluation time,
  chunk build counts/durations, first-touch build latency.
- Config: chunk entry-count knob.
