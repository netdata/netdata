# Netdata Agent — OpenTelemetry logs remote storage (operator guide)

Operator-facing guide to the OTel plugin's **remote object-storage archive** for
logs: what it does, the startup behavior that will surprise you first, what
happens during a remote outage, how to migrate a node, and how to read the
corruption and loss-window semantics.

> Scope: this is about **logs** storage and restore. Metrics are unaffected.
> Traces are a pre-GA scaffold. Formats may change with an archive reset
> (pre-GA); there is no cross-version migration of archived data.

This file is hand-authored and operator-facing. The generated integration page
(`README.md` → `integrations/opentelemetry.md`) is produced from `metadata.yaml`
and is not the home for this guide.

## The model in one paragraph

The remote bucket is an **archive the plugin can restore from**. Local disk is
the **working set** plus a small **table of contents** — the *catalog* (index)
files that record which sealed log files (SFSTs) were uploaded and where. Local
data files age out under your retention limits; the catalogs are kept much longer
(the `horizon`, 10 years by default), so the queryable history in the bucket
outlives the data on local disk. A query for evicted-but-archived data fetches the sealed files back
into a bounded local read cache on demand.

## Fail-closed startup (the one behavior to know first)

**With `storage.enabled: true`, the plugin refuses to start until the bucket is
reachable.** On an unreachable or misconfigured backend it exits with a configure
error and the Netdata Agent restarts it (a configure → retry-exhaust (~3 min) →
exit → respawn loop). It does **not** start blind and it does **not** silently
skip the archive.

Why: startup makes the local catalog set **complete** by listing and downloading
any catalogs missing locally, before it serves the first query. That guarantee
requires the bucket, so **remote availability gates log ingestion at startup.**
This is the behavior operators are most often surprised by — plan credentials and
network reachability accordingly before enabling storage.

## Remote outage during operation

If the bucket becomes unreachable **while running**:

- Ingestion **continues** — incoming logs are still accepted and written locally.
- Uploads **queue and retry** in the background.
- **Local disk grows unbounded** until the remote returns: nothing is evicted
  before it has been archived, so retention cannot reclaim space while uploads
  are backed up. **Watch local disk on long outages.**

When the remote returns, the backlog drains and retention resumes.

## The plugin never deletes from the bucket

The plugin only ever **writes** to the bucket. It never deletes archived objects.
Remote retention is **your** lever: configure the object store's own lifecycle
rules. Those rules **MUST NOT** expire objects younger than the `horizon`
setting (10 years unless overridden), or a query within the horizon will find a
catalog entry pointing at an object you deleted.

## Migrating a node to new hardware

Archived data is keyed by the node's **Netdata machine GUID**. To move a node:

1. **Stop the old node first** (clean shutdown, so its last uploads drain).
2. Bring up the new machine with the **same Netdata machine GUID**.
3. First boot **fail-closes** until the bucket answers, then downloads its
   catalogs (each ~1 KB; minutes even for years of history) and full history is
   queryable again. Log **data** downloads lazily, on query — only what you
   actually look at is fetched.

Do not run the old and new nodes against the same bucket with the same GUID at
the same time.

## Shared buckets

Multiple nodes may share one bucket. Each node touches only **its own** data
(everything is keyed by machine GUID; every node filters the listing to its own
objects). The cost: startup listing time grows with **fleet size × history** —
a shared bucket lists every node's objects, and each node filters. Very large
fleets may need a higher `storage.startup_op_timeout` (default 5 minutes; an
advanced option not listed in the stock file — set it in your user `otel.yaml`).

## Loss windows (crash vs clean shutdown)

Recently-ingested data has a short unprotected tail:

- **~15 min** unsealed working file that has not yet rotated to an SFST, plus a
  similar window where a sealed SFST is uploaded but not yet cataloged.
- **~30 min worst case on a crash**; **~15 min on a clean shutdown** (the clean
  path seals and flushes).
- **Decommission cleanly**: clean shutdown **plus a few idle minutes** to let
  uploads drain before you tear the node down.

## Time bounds on incoming records

Log records are accepted only within an inclusive window around the agent's
clock: **not older than `logs.ingest.max_age` (default 24 h)** and **not more
than `logs.ingest.future_skew` (default 10 min) in the future**, measured per
record from its resolved timestamp. Out-of-window records are rejected
**per record** and reported back to the sender in the OTLP `partial_success`
response; in-window records in the same request are still stored.

- This guards retention and local disk against clock-skewed or bulk-backfilled
  data. Bulk backfill of old data is a **future feature**, not a knob to loosen.
- `future_skew: "0s"` rejects any record even a nanosecond ahead of this agent's
  clock (including ordinary sender skew); the plugin **warns at startup** if you
  set it to zero.

## Corruption runbook

A **corrupt catalog** is detected at **startup**, not at query time:

- It is logged at **ERROR** (treat the ERROR as a **disk-health signal** — these
  files are immutable and written atomically, so corruption points at the disk).
- It is **quarantined** by renaming it to `<name>.corrupt.<timestamp>` (safe to
  delete after investigation) and then **re-fetched from the bucket
  automatically**.
- If the re-fetch fails (or storage is disabled), the quarantine stands and boot
  continues — a single bad object never bricks startup.
- A catalog file that is a **newer, unsupported format version** (e.g. after a
  downgrade) is **left in place** untouched — re-upgrade the plugin to read it.

A catalog that is somehow **missing** locally is restored by the next restart's
startup sync. **Catalog completeness is a startup property: if in doubt, restart
the plugin.** The query path never fetches catalogs — it only reads what startup
made complete.

## Backend caveats

- **`fs://` backend readability (important):** an `fs://` bucket root that
  **exists but is unreadable** (permissions) lists as **empty** rather than
  failing — the plugin will boot with an **empty archive view** instead of
  fail-closing. Ensure the `fs://` path is readable by the netdata user. A
  permission error on an `fs://` root is indistinguishable from an empty bucket.
  S3-class backends surface auth/permission errors and fail closed correctly.
- **Credentials** are never configured in `otel.yaml`. Supply them through the
  netdata process environment or standard cloud locations (for S3:
  `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`, `~/.aws/credentials`, or an
  attached instance role). See the storage block in `otel.yaml`.

## Related configuration

All knobs live in `otel.yaml`. The stock file (`configs/otel.yaml.in`)
documents the public surface with defaults: `storage:` (enable, URI, read
cache), `logs.rotation:` (file size/entry limits), `logs.retention:` (local
disk limits).

A few advanced knobs are deliberately **not listed in the stock file** but are
accepted in the user `otel.yaml` (and as environment variables), resolving to
hard-coded defaults otherwise: `storage.startup_op_timeout` (5 minutes),
`logs.rotation.<tenant>.max_file_duration` (15 minutes),
`logs.retention.<tenant>.horizon` (10 years), `logs.catalog:` (rotation count
10 / period 15 minutes), and `logs.ingest:` (max age 24 hours / future skew
10 minutes). The developer-facing contracts are in
`.agents/sow/specs/otel-file-lifecycle.md` and
`.agents/sow/specs/otel-remote-storage-config.md`.
