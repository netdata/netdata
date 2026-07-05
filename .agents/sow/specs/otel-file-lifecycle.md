# OTel storage substrate — file lifecycle (as-built)

Scope: the end-to-end lifecycle of the OTel plugin's on-disk and remote
artifacts for **logs** — creation, hand-off, rotation, upload, eviction, and
restore — plus the per-process identity model that keeps it correct across
wipes, migrations, and shared buckets. As-built and verified against code.

Companion specs (do not duplicate):

- [otel-storage-substrate.md](otel-storage-substrate.md) — the content-agnostic
  `file-lifecycle` crate seam and substrate-vs-logs-binding split.
- [otel-remote-storage-config.md](otel-remote-storage-config.md) — storage
  backend/credential/cache config and the knob defaults.
- [otel-stream-identity.md](otel-stream-identity.md) — the `part_key`/stream
  content-plane identity that rides inside `FileId`.

Traces are a pre-GA scaffold; this spec describes the logs pipeline. Pre-GA:
formats may change with an archive reset; there is no cross-version migration of
archived data.

## Artifact lifecycle map

Data flows **WAL → SFST → catalog → remote object → read cache**. Each stage has
one creator, one deleter, and one authoritative copy.

- **WAL** (`{base}/{sig}/wal/{tenant}/{fileid}.wal`)
  - Created by the ingestor, lazily on the first accepted record for a stream.
  - Deleted by the ledger/cleaner **only after** the WAL is successfully indexed
    into an SFST — never by retention.
  - Authoritative for not-yet-indexed data; superseded by its SFST once indexed.
- **SFST** (`{base}/{sig}/index/{tenant}/{fileid}.sfst`) — the sealed, immutable
  columnar file.
  - Created by the ledger indexer from a rotated WAL.
  - Deleted by retention eviction — **gated on remote-cataloged** when storage is
    enabled (a local SFST is not evicted until its catalog entry is confirmed on
    the remote, so a failed catalog upload cannot orphan it).
  - Authoritative locally for its `[min_ts, max_ts]` and record count (its SUMR).
- **Local catalog** (`{catalog_base}/{date}/{tenant}/{name}.catalog`) — the small
  index that maps uploaded SFSTs to their remote keys.
  - Created by the catalog builder (atomic tmp+rename).
  - Deleted by date-partition retention driven by the archive `horizon`.
  - Authoritative for remote-fetch keys; its **filename fold**
    `(max_seq, min_ts, max_ts)` is the authoritative, CRC-free pre-filter used at
    query and restore time.
- **Remote SFST / remote catalog**
  (`v2/{sig}/tenants/{tenant}/sfst/{date}/…` and `v2/{sig}/catalog/{date}/{tenant}/…`)
  - Created by the ledger uploader.
  - **Never deleted by the plugin.** The `Storage` trait exposes only
    `write`/`list`/`read`/`stat` — there is no delete path in the codebase.
    Remote removal is the operator's object-store lifecycle rules alone.
  - The durable archive copy; what startup restore reads from.
- **Read cache** (logs, `{base}/{sig}/remote-read`)
  - Populated on query when an evicted SFST body is fetched back; bounded LRU
    under `read_cache_max_size`.
  - Non-authoritative — a local mirror of remote SFST bodies.
- **seq highwater** (`{base}/shared/seq_highwater`)
  - Written by the ingestor's seq allocator (one durable write per 256-seq
    reserve batch); overwritten atomically, never deleted; also raised by startup
    restore (below). Authoritative for the next seq to issue.

## Identity model (I5)

Every artifact is stamped with a two-part identity so state stays correct across
a local wipe, a migration to new hardware, and a bucket shared by many nodes.

- **`MachineId`** — the Netdata machine GUID, read from env
  `NETDATA_REGISTRY_UNIQUE_ID`. **Hard-required**: the supervisor fatally aborts
  before spawning any worker if it is missing, empty, non-UUID, or nil
  (`otel-plugin/src/supervisor.rs`). `NETDATA_INVOCATION_ID` is informational only
  (warn, not fatal).
- **`InstanceId`** — a fresh v4 UUID generated **once per process** at startup.
  A post-wipe or restarted process gets a new `InstanceId` under the same
  `MachineId`.
- Both are non-nil-by-construction newtypes over `Uuid` (nil rejected), serde
  representation is the bare UUID.
- **`FileId`** = `{machine_id, instance_id, pipeline_id, seq, part_key}` — the
  wire/​filename identity. **`SeqKey`** = `(machine_id, instance_id, seq)` — the
  in-process handle for identity-scoped state; deliberately **not**
  serializable (identity travels the wire as `FileId`/`Identity`).
- **State-keying boundary** (the I5 rule):
  - State that crosses an identity boundary — upload / rotation / remote-cataloged
    / eviction state — MUST be keyed by **`SeqKey`** (full identity), because it
    may describe a prior instance's or (via a shared bucket) another machine's
    file.
  - Local-presence lookups within one live process — WAL/SFST registry `get`,
    seq routing — use the **bare `seq`**.
  - Rationale: a bare `seq` can repeat across a post-wipe reseed (new
    `InstanceId`) or a shared bucket (different `MachineId`), so it is unique
    only within one live process instance. A same-`seq` local file whose full
    identity differs from a listed remote object is refused by the reconcile
    splice guard.
- **Role of bare `seq`**: a process-global monotonic counter (across signals,
  tenants, and streams within one process instance); it remains the primary key
  for all local single-instance registry state and for seq routing.

## Ingestion time bounds (logs)

- Records are accepted only inside the **inclusive** window
  **`[now − max_age, now + future_skew]`**, evaluated per record against its
  **resolved** timestamp. The clock is read **once per request** and the same
  bounds apply to every record in that request.
- **Resolved timestamp** = `time_unix_nano` if nonzero, else
  `observed_time_unix_nano`, else a synthesized `base + k` for the kth
  timestamp-less record. The resolved value is written back and becomes the
  record's stored/queryable time and the WAL's time range.
- **Synthesized-timestamp exemption**: a record with **both** `time_unix_nano == 0`
  **and** `observed_time_unix_nano == 0` is flagged synthesized and is **never**
  bounds-checked (the bounds police client clocks; a server-synthesized "now"
  always lands inside).
- **`partial_success`**: out-of-window records are rejected **per record**,
  counted, and reported to the sender via the OTLP `ExportLogsPartialSuccess`
  field; in-window records in the same request are still stored. A fully-rejected
  stream is dropped but still counted.
- Defaults: `logs.ingest.max_age = 24h`, `logs.ingest.future_skew = 10min`.
  `future_skew = 0` rejects any record even a nanosecond ahead of this agent's
  clock (including ordinary sender skew); the plugin **warns at startup** on a
  zero skew.

## Rotation cadences and the loss window

- **WAL rotation** (whichever fires first): `max_file_size = 25 MB`,
  `max_log_entries = 50 000`, or `max_file_duration = 15 min` (the shipped
  runtime default, resolved by the bridge policy — the `wal` crate's own 1h
  default is never used at runtime). A background **idle-rotation sweep** runs
  every **30 s** (fixed, no knob) so a quiet stream still seals on its duration
  bound. (Logs only; traces has no sweep yet.)
- **Catalog rotation** (whichever fires first): `rotation_count = 10` entries,
  `rotation_period = 15 min`, or a **Flush on clean shutdown**. The period is
  checked on a 30 s tick and floored at 1 s.
- **Loss window** (derived timeline; the component bounds are code constants, the
  sum is a design consequence, not a stored constant): recently-ingested data has
  an unprotected tail of **≤ 15 min** unsealed WAL plus **≤ 15 min**
  sealed-but-uncataloged, i.e. **~30 min worst case on a crash** and **~15 min on
  a clean shutdown** (the clean path seals and flushes). Decommission cleanly:
  clean shutdown plus a few idle minutes for uploads to drain.

## Remote layout and LIST discipline

- Key shapes (`file-lifecycle/src/remote_keys.rs`, schema `v2`):
  - SFST: `v2/{signal}/tenants/{tenant}/sfst/{YYYY-MM-DD}/{file_id}.sfst`
  - Catalog: `v2/{signal}/catalog/{YYYY-MM-DD}/{tenant}/{machine}-{instance}-{max_seq}-{min_ts}-{max_ts}.catalog`
  - Note the deliberate asymmetry: SFST keys nest tenant above date (per-tenant
    IAM scope); catalog keys nest date above tenant (per-date lifecycle rules).
- `{date}` is **data time** (from the SFST's `min_timestamp_s`, else `Utc::now()`
  fallback), not upload time.
- **Own-machine LIST filter (MANDATORY, D6):** all machines sharing a bucket write
  under the same `v2/{signal}/…` prefixes, so **every** LIST consumer MUST drop
  keys not owned by this machine. There are exactly two `storage.list`
  call-sites — startup diff-sync (catalog prefix) and the per-date SFST reconcile
  — and **both filter by own machine**. A new LIST consumer MUST add the same
  filter.
- **One-way door (D6 = B):** the flat, shared-prefix layout is correctness-safe
  **only** because each consumer filters own-machine inside fail-closed startup.
  A machine-scoped prefix layout was considered and deferred; adopting it later is
  a breaking archive-layout change (revisit before GA).

## Startup restore contract (P7)

When storage is enabled, the ledger runs a **required, fail-closed**
`startup_catalog_sync` per signal **before** per-tenant recovery, so the local
catalog set is **complete** before the first query. Steps:

1. **Recursive LIST** the whole `v2/{signal}/catalog/` prefix.
2. **Sanitize/parse** — drop `/`-terminated directory placeholders and any
   unparseable key.
3. **Own-machine filter** (D6) — drop keys owned by another machine.
4. **Tenant union** — return the discovered remote tenants; the caller unions
   them with locally-discovered tenants and instantiates remote-only tenants.
5. **Highwater seed, write-only-when-raising** — raise the shared seq highwater to
   the max listed seq **before** any download (the ceiling must hold regardless
   of download outcome), and only when it would rise.
6. **Diff-download** the bodies missing locally, bounded to `DOWNLOAD_CONCURRENCY = 8`
   with a short-circuiting collect.
7. **Validate** each body against its key before install — container magic/CRC +
   framing version, then the JSON envelope's format version; envelope
   tenant/date/identity vs the key; entries fold vs the filename; and every
   entry's `remote_key` a well-formed SFST key on this signal/machine/tenant whose
   embedded `FileId` equals the entry's id and whose date equals the catalog date.
8. **Atomic install** off the runtime (`spawn_blocking` + tmp/fsync/rename).

Failure policy:

- **Hard error → startup fails** (plugin exits, agent restarts it): LIST error or
  timeout; download transport error or timeout.
- **Skip loudly → boot continues**: an unparseable key, a GET 404 (object raced
  away), or a validation reject. A single bad object never bricks startup.
- **`storage.startup_op_timeout`** (default 5 min) bounds **each** remote op (the
  LIST and each GET); there is **no** phase-total cap, so a large restore stays
  work-proportional. Startup **local** I/O (highwater read/write, atomic install)
  is deliberately un-timed — a hung local filesystem is bounded by the agent's
  plugin-restart loop.
- Two **optional** per-tenant reconciles follow, each wrapped in a separate
  `STARTUP_REMOTE_BUDGET = 10 s` bound (distinct from `startup_op_timeout`); on
  timeout/error they are skipped and eviction simply stays deferred.

## Corrupt-catalog startup-heal (D-P8.1)

Corrupt-present catalogs are healed at **startup**, never at query time, inside
the local-catalog seeding pass:

- **Body-parse failure** (true corruption): logged at **ERROR** (loud even when
  the heal succeeds — corruption of an immutable, atomically-written file signals
  disk damage), then **quarantined** by renaming to `{name}.corrupt.{unix-ns}`
  and **re-fetched** as a single object (re-fetch → validate → atomic install →
  re-parse → seed). Any failure returns cleanly and boot continues.
- **`UnsupportedVersion`** (a newer format, e.g. after a downgrade): **left in
  place**, not healed — a re-fetch would return the same future-version bytes and
  destroy the local copy a re-upgrade could read.
- **Storage disabled**: ERROR + leave in place as operator evidence (no rename,
  nothing can restore it).
- Quarantine files (`.corrupt.<ns>`) carry a non-`.catalog` suffix, so recovery
  and scan sweeps ignore them by construction.

## Restore / migration contract

- A wipe or migration boot gets a **fresh `InstanceId`** under the **unchanged
  `MachineId`**; the highwater is seeded from `max(local scans, remote max)` so a
  reseeded ingestor never reissues a seq the remote already holds.
- **Catalog completeness is a STARTUP property** — made complete by the required
  fail-closed `startup_catalog_sync` before Ready; per-tenant recovery then runs
  against a locally-complete catalog set.
- **The query path NEVER fetches catalogs remotely.** Query-time remote reads are
  limited to **SFST bodies** of evicted files, pulled through the read cache. If a
  catalog is missing locally, the fix is the next restart's sync — "if in doubt,
  restart the plugin."

## Known boundaries

From the live end-to-end validation (tracked follow-ups):

- **`fs://` fails open on an unreadable root (F-e2e-1).** On the `fs` backend, an
  existing-but-**unreadable** storage root lists as **empty** (the simulated
  recursive walk skips permission-denied dirs), so fail-closed does **not**
  trigger and the plugin boots with an empty archive view. The operator MUST keep
  the `fs://` root readable. S3-class backends surface auth/permission errors and
  fail closed correctly. ("Empty" cannot itself be an error — a first-enable boot
  legitimately sees an empty bucket.) Candidate hardening: probe root readability
  at startup.
- **Hostile-but-legal tenant segments can instantiate an empty registry (F-e2e-2).**
  A charset-legal but hostile tenant segment (e.g. `..evil`) in an own-machine
  key instantiates an empty tenant registry, because the tenant union runs on
  kept keys **before** download validation. The catalog itself is still correctly
  rejected by the body↔path cross-check (nothing installed, no dirs created), so
  the effect is bounded noise, own-machine keys only. Candidate polish: defer
  tenant instantiation until ≥ 1 catalog validates.
- **Two-tenant restore is integration-test-proven, not live.** With auth off, all
  data is a single `default` tenant; the live e2e exercises exactly the auth-off
  `default`-tenant restore path. The multi-tenant restore dimension is covered at
  the integration-test level.
