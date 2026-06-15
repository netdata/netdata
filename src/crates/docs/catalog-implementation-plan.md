# Catalog Implementation Plan

Implementation plan for the catalog subsystem described in `~/mo/catalog-design/`.
The typst design doc is **advisory, not authoritative** — this plan captures the
agreed-upon decisions and deviations.

## Scope

Single-node functionality only. Parent/children coordination is deferred.
We implement phases 1–4 of the typst doc's seven-phase plan.

## Confirmed decisions

- **Scope:** Phases 1–4 below; skip multi-node (typst §8) and `RemoteRegistry`
  replacement (typst §9 phase 5) for now.
- **`pending_catalog` persistence:** Do **not** persist the in-memory cache. On
  crash, recovery re-derives `IndexMetadata` by reading the SFST header via a
  new `read_header_metadata()` helper — no WAL re-indexing needed.
- **Serialization:** JSON for v1. Debuggability beats compactness at current
  scales. Version header is additive; binary format is a future optimization.

## Pushbacks against the typst design we're acting on

- **`pending_catalog` loss on crash.** Typst doc doesn't specify. We re-derive
  from SFST header on recovery (see Phase 4).
- **Observability.** Typst doc hand-waves. We ship concrete counters and spans
  as part of Phase 3 (not as a follow-up).

## Split rationale

Four separate phases / PRs, not one bundle:

- Each phase has a distinct risk profile (types → plumbing → new component →
  crash-safety). Bundling hides the hardest thinking (Phase 4) under plumbing.
- Each is independently testable — the exit criterion is a green build with new
  tests, not "the whole catalog works."
- Phases 1 and 2 are independent (disjoint files), can proceed in either order.
- Small focused changes beat one big merge — `git bisect` works on a 300-line
  diff in a way it doesn't on a 2000-line one.

Dependencies: Phase 3 needs 1 + 2. Phase 4 needs 1 + 2 + 3.

Suggested order: Phase 1 → Phase 2 → Phase 3 → Phase 4. (1 and 2 swappable.)

---

## Phase 1 — Data model (`otel-catalog` crate)

**Goal:** Pure library code. `CatalogEntry`, `Catalog`, `CatalogQuery`,
`StreamEntry`, plus JSON serialization with a version header. No wiring to the
pipeline.

**New crate:** `src/crates/otel-catalog/` (parallels `wal`, `sfst`,
`file-registry`).

**Files:**

- `otel-catalog/Cargo.toml` — deps: `serde`, `serde_json`, `file-registry`,
  `uuid`, `time` (or whatever Date crate is already used in the workspace)
- `otel-catalog/src/lib.rs` — re-exports
- `otel-catalog/src/entry.rs` — `CatalogEntry`, `StreamEntry`
- `otel-catalog/src/catalog.rs` — `Catalog` backed by
  `BTreeMap<FileId, CatalogEntry>`, with `add` / `remove` / `find` /
  `entries_in_range`
- `otel-catalog/src/query.rs` — `CatalogQuery`
- `otel-catalog/src/format.rs` — JSON envelope with `version: u32` header;
  reject unknown versions with a typed error (no panic)
- `otel-catalog/tests/` — see below

**Key interfaces (sketch):**

```rust
pub struct CatalogEntry {
    pub id: FileId,
    pub remote_key: String,
    pub min_timestamp_s: i64,
    pub max_timestamp_s: i64,
    pub total_logs: u32,
    pub streams: Vec<StreamEntry>,
    pub size: u64,
    pub uploaded_at_ns: i64,
}

pub struct Catalog {
    pub tenant_id: String,
    pub date: Date,
    pub machine_id: Uuid,
    pub boot_id: Uuid,
    pub entries: BTreeMap<FileId, CatalogEntry>,
    pub created_at_ns: i64,
    pub updated_at_ns: i64,
}

impl Catalog {
    pub fn add(&mut self, entry: CatalogEntry);
    pub fn remove(&mut self, id: &FileId) -> Option<CatalogEntry>;
    pub fn find<'a>(&'a self, q: &CatalogQuery)
        -> impl Iterator<Item = &'a CatalogEntry> + 'a;
    pub fn to_json(&self) -> Result<Vec<u8>>;
    pub fn from_json(bytes: &[u8]) -> Result<Self>;
}
```

**Tests:**

- Round-trip: construct → serialize → parse → equal (proptest if cheap)
- `add` then `remove` returns to empty
- Range queries: synthetic entries at known timestamps; boundary cases
  (inclusive ends, empty range, single-point range)
- Exact stream match: empty-string stream matches only empty-string
- Unknown format version → typed error
- Truncated JSON → typed error, not panic

**Exit criteria:** Crate compiles; tests pass; `cargo clippy` clean. Nothing
imports it yet.

**Depends on:** `file-registry` only.

---

## Phase 2 — Indexer extension + ledger `pending_catalog` cache

**Goal:** Route `IndexMetadata` from indexer to ledger on `IndexFinalized` and
cache it keyed by sequence number. No catalog writes yet — pure plumbing.

**Files:**

- `sfst/src/…` — audit what `IndexMetadata` currently carries. Needs at
  minimum: `min_ts`, `max_ts`, `total_logs`, per-stream list. Extend if any are
  missing.
- `otel-ledger/src/ipc.rs` — extend `IndexerResponse::IndexFinalized` to carry
  `IndexMetadata`
- `otel-ledger/src/indexer.rs` — populate the new field from the freshly-written
  SFST
- `otel-ledger/src/ledger.rs` — add `pending_catalog: HashMap<u64, IndexMetadata>`;
  populate in `handle_indexer_resp`; evict on `IndexFileDeleted` (rare-but-possible
  retention-before-upload case — typst §5)

**Tests:**

- Unit: stubbed `IndexerResponse::IndexFinalized` → `ledger.pending_catalog`
  has one entry at that `seq`
- Unit: `IndexFileDeleted` before upload → cache entry removed
- Existing ledger tests still pass — behavior change must be invisible to the
  upload path until Phase 3

**Exit criteria:** Cache is populated and never read. Ledger behavior is
otherwise unchanged.

**Depends on:** nothing (independent of Phase 1).

---

## Phase 3 — `CatalogWriter` component + wiring

**Goal:** New component consuming successful uploads, maintaining in-memory
catalogs, writing local JSON (tmp + rename) and uploading to remote via opendal.

**Files:**

- `otel-ledger/src/catalog_writer.rs` — new component following the existing
  `Component` trait pattern. Owns
  `HashMap<(tenant_id, Date, Uuid /* boot_id */), Catalog>`.
- `otel-ledger/src/ipc.rs` — add `CatalogWriterRequest::{Record, Remove}` and
  `CatalogWriterResponse::{Recorded, RecordFailed { stage: Local | Remote }}`.
  `Remove` is a stub with no handler body (forward-compat hook for the future
  remote cleaner — typst §7.5).
- `otel-ledger/src/ledger.rs`:
  - Spawn `CatalogWriter` alongside existing components
  - In `handle_uploader_resp` on `Uploaded`: pop from `pending_catalog`,
    construct `Record`, send to `CatalogWriter`
  - Handle `CatalogWriterResponse` (log failures; do not fail the pipeline)
- `otel-ledger/src/config.rs` — catalog base dir (derive from index dir if not
  explicitly configured)

**Write path:**

```rust
// In handle_uploader_resp, on Uploaded:
if let Some(metadata) = self.pending_catalog.remove(&seq) {
    let req = CatalogWriterRequest::Record {
        file_id, remote_key, metadata, size,
        tenant_id, date, uploaded_at_ns,
    };
    self.catalog_writer.send(req).await?;
}
```

**Observability (ship with the component, not after):**

- `tracing::info_span!("catalog.record", tenant, date, file_id)` around each
  request
- Counters: `catalog_record_ok_total`, `catalog_record_fail_total`, split by
  `stage = "local" | "remote"`
- Histogram: `catalog_record_latency_ns`, labeled by stage

**Tests:**

- Component test with in-memory opendal backend
- Happy path: one `Record` → local file present, remote object present, bytes
  equal
- Idempotency: same `Record` twice → single entry (`BTreeMap` overwrite)
- Remote failure: local succeeds, remote fails → `RecordFailed { stage: Remote }`,
  local correct, next successful record overwrites remote
- Local write failure (read-only FS): `RecordFailed { stage: Local }`, remote
  not attempted
- Sequential processing guaranteed by construction (single mpsc receiver);
  document this, no test needed

**Exit criteria:** After each successful upload, a `.catalog` file appears
locally and remotely containing the `CatalogEntry`. Metrics visible.

**Depends on:** Phase 1 (types), Phase 2 (`pending_catalog`).

---

## Phase 4 — Recovery

**Goal:** On ledger startup, reconcile catalog state against `RemoteRegistry`.
Insert as the final step of the existing recovery sequence.

**Files:**

- `sfst/src/reader.rs` (or wherever the reader lives) — add
  `read_header_metadata(&Path) -> Result<IndexMetadata>` that reads just the
  header chunks without loading the FST. Implements the "re-derive on recovery"
  decision.
- `otel-ledger/src/recovery.rs` — new `recover_catalog(&mut Ledger) -> Result<()>`
  called after `recover_unuploaded`
- `otel-ledger/src/ledger.rs` — invoke it during startup

**Algorithm (per `tenant × date × boot`):**

1. If local catalog file exists and parses → load into `CatalogWriter`'s map.
   Done.
2. Else rebuild:
   - For each SFST in `RemoteRegistry`: call `read_header_metadata` on the local
     SFST (or fetch the header via opendal range read if local is gone);
     construct `CatalogEntry`; add to a fresh `Catalog`.
   - Write locally; queue an upload request.
3. If local catalog exists but is corrupt (parse fails) → log error, rename to
   `.catalog.bad.<ts>` (don't delete — operator can recover), rebuild as in (2).

**Crash matrix (integration tests, kill + restart):**

| Crash point | `RemoteRegistry` | Catalog | Recovery action |
|---|---|---|---|
| After SFST write, before upload | no | no | no-op |
| After upload, before `Record` | yes | no | re-derive from SFST header |
| After local catalog write, before remote upload | yes | local only | re-upload |
| After remote catalog upload, before response | yes | both | idempotent overwrite (verify) |
| Corrupt local catalog | yes | corrupt | rename to `.bad`, rebuild |
| Missing local, present remote | yes | remote only | (v1: rebuild from `RemoteRegistry`; future: fetch remote as baseline) |

The "missing local, present remote" case can be deferred to a follow-up if it
complicates the initial PR — note as a known limitation and rebuild from
`RemoteRegistry` in v1.

**Exit criteria:** After any supported crash point, post-recovery state
satisfies the three typst invariants:

1. **No data loss** — uploaded SFSTs remain discoverable.
2. **No phantom entries** — no catalog entry references a file that doesn't
   exist remotely.
3. **Convergence** — catalog and `RemoteRegistry` agree after recovery.

**Depends on:** Phases 1, 2, 3.

---

## Deferred (not in this plan)

- Multi-node / parent-child (typst §8). `CatalogView` and the parent read loop.
- Using the catalog as the authoritative source (typst §9 phase 5). Keep the
  list-based `RemoteRegistry::recover()` running alongside catalog for a period
  of operational confidence before switching.
- Query planner using the catalog (typst §9 phase 6).
- Binary catalog format (bincode + zstd). Deferred per confirmed decision.
- Catalog compaction. Deferred per typst §1.
- Remote-side GC and `CatalogWriterRequest::Remove` handler body. Message
  variant exists from Phase 3 for forward-compat; handler arrives with the
  remote cleaner.
