# OTel plugin remote object-storage configuration & credential contract

Scope: how the OTel plugin configures a remote object-storage backend for SFST +
catalog upload, how credentials are supplied, and the startup reachability probe.
Owner surfaces: `otel-ledger` (`storage.rs`, `ledger/mod.rs`), `otel-plugin`
config (`config/logs.rs`, `config/env.rs`), `netdata-plugin/bridge` config
(`StorageConfig`), and `otel.yaml`.

## Configuration contract

- `StorageConfig` (`netdata-plugin/bridge/src/config.rs`) has four fields:
  `enabled: bool`, `uri: String`, and the read-cache pair `read_cache_dir:
  Option<PathBuf>` + `read_cache_max_size: ByteSize` (default `4 GiB`). All four are
  env-overridable: `NETDATA_OTEL_LOGS_STORAGE_ENABLED`, `_URI`,
  `_READ_CACHE_DIR`, and `_READ_CACHE_MAX_SIZE` (parsed as `ByteSize`). Env
  overrides apply on top of `otel.yaml` (env wins), via the shared
  `StorageOverride` path in `config/{logs,env}.rs`.
- `uri` is a single **OpenDAL URI**. The scheme selects the backend
  (`fs://`, `s3://`, …); all **non-secret** backend options are URI query params
  (`s3://bucket/prefix?region=…&endpoint=…`). OpenDAL owns the per-backend option
  schema; the plugin models **no** per-backend keys.
- Unknown query keys are **silently ignored** by OpenDAL (no error). The startup
  probe (below) is the safety net that surfaces a resulting misconfiguration.
- Adding a backend (GCS, Azure, …) is a **compile-time** decision: enable the
  OpenDAL `services-*` cargo feature in the workspace pin. Today only
  `services-fs` and `services-s3` are compiled in, so only those schemes resolve;
  an unbuilt scheme fails at `Operator::from_uri`. **No `otel.yaml` schema change
  is ever required to add a backend.**

## Credential contract (secret-free `otel.yaml`)

- Credentials are **delegated entirely to OpenDAL's ambient loading** in each
  service's `build()`: environment variables, instance role / EC2 IMDS / GCE
  metadata / Azure managed identity, and the providers' standard credential files
  (AWS shared-credentials / IRSA, GCP application-default JSON). The plugin injects
  nothing and constructs the operator with raw `Operator::from_uri` (`storage.rs`).
- Secrets **MUST NOT** appear in `otel.yaml` (neither in `uri` query params nor
  elsewhere). Operators supply credentials to the **netdata process environment**
  (which the plugin inherits — netdata spawns plugins with `posix_spawn(...,
  environ)`), or via instance identity / standard credential files.
- There is **no** custom secret-reference grammar, env-name/file-path indirection,
  or per-field credential config in `otel.yaml`. (Contrast go.d's
  [secret-reference-grammar.md](secret-reference-grammar.md), a Go-collector
  subsystem the Rust OTel plugin does not share — deliberately not adopted to keep
  one configuration path.)

## Startup reachability probe

- When `storage.enabled`, `Ledger::new` spawns a **non-blocking** probe
  (`storage::probe_reachable`): a `stat` on a sentinel key
  (`.netdata-otel-storage-probe`). `Ok`/`NotFound` ⇒ reachable + authorized
  (logged `info`); `Other` ⇒ misconfig (logged `error`). It never aborts startup.
- The probe is **spawned, not awaited**: the OpenDAL retry layer can stall for the
  full retry window, and awaiting it in `Ledger::new` would re-introduce the
  startup stall documented in `recovery/remote.rs`. It is deliberately **not** tied
  to the ledger `CancellationToken` — it is a read-only one-shot diagnostic, and
  cancelling it would only suppress a useful signal during shutdown.
- It shares the operator's retry layer (10 attempts), so a **temporary** failure
  (e.g. the credential chain hitting an unreachable IMDS endpoint when
  misconfigured off-EC2, or an unreachable backend) is retried for the full retry
  window — several minutes, since each attempt also waits on its own
  credential-load / connect timeout — before the error surfaces. A **permanent**
  failure (auth rejected, missing bucket) is not retried by opendal, so it surfaces
  after a single request — though that one request still costs the backend's own
  connect/read timeout, so "fast" is relative to the multi-minute temporary case,
  not instant.
- The **configured URI is intentionally not logged** (only the opendal error,
  which carries the operation/backend/endpoint but not URI query-string
  credentials), so a misplaced secret in the URI cannot leak into the journal.
- Logs land in systemd-journal under `otel-plugin/ledger` (query via the MCP
  `netdata_agent_logs` tool, component `ledger`).

## Read-through cache (query path)

- When `storage.enabled`, `Ledger::new` opens a bounded local read-through cache
  (the `file-cache` crate) at `read_cache_dir` (default: a `remote-read` directory
  beside the index dir), capped at `read_cache_max_size`. The directory MUST be
  local, owner-private storage; it is created `0700` and recovered on open.
- A log query answers over SFSTs that local retention evicted but that a catalog
  records on remote. `Registry::remote_candidates(q)` returns the catalog entries
  whose seq is absent locally (no local SFST and no durable-prefix WAL), deduped by
  seq — the same local-wins rule the query planner uses. The handler fetches them
  through the cache (`Storage::read(remote_key)` as the fetch closure), maps each to
  a sealed-SFST source (summary taken from the catalog entry), and holds the cache
  pin guards across the blocking query run. Per-object fetch failures degrade (the
  source is omitted); the local query path is unchanged.
- A query whose remote footprint exceeds the cache cap is **rejected** with an
  actionable error (`CacheError::TooLarge` → "narrow the time window or stream
  filter"); a broken cache directory surfaces as `EvictionFailed`. Reject is the
  **final** design: time-window narrowing (serving the most recent fitting window
  instead of rejecting) was considered and declined — without cross-repo in-UI
  "window narrowed" disclosure it would silently truncate results, a worse trust
  hazard than an honest error.
- Cache filenames are the SFST `FileId` encoding (content-stable), never the remote
  key, so the cache's path-traversal guard is belt-and-suspenders.

## Stream selector (query path)

- The otel-logs "Services" stream selector (`required_params.__streams`) is
  **window-scoped and remote-inclusive**: it lists exactly the streams with data in
  the query window — local files **plus** remote-only (evicted-but-cataloged)
  streams — so a stream quiet in the window is omitted, and an evicted stream whose
  data is still fetchable is offered. (This deliberately diverges from the former
  window-independent behavior.)
- `Registry::enumerate_streams_from` folds the in-window local SFST/WAL candidates
  and the in-window catalog entries (keyed on the stream's `ns_hash`), skipping a
  catalog seq already served locally. The catalog is folded **only when remote is
  enabled** — otherwise the selector would advertise streams that cannot be fetched.
- The handler parses the in-window catalog **once** and shares it (plus a single
  time-only `local_seqs` mask) between the selector and the remote fetch, avoiding a
  second parse under the read lock. The shared time-only mask is correct because one
  seq maps to exactly one file and one stream (`build_catalog_entry` copies both the
  id and the stream from the same SFST).

## Non-goals

- Modeling per-backend options in `otel.yaml`.
- Any in-config secret storage, secret-reference grammar, or secret-key-in-URI
  guardrail (the latter explicitly declined).
