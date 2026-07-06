# OTel plugin remote object-storage configuration & credential contract

Scope: how the OTel plugin configures a remote object-storage backend for SFST +
catalog upload, how credentials are supplied, and the startup reachability probe.
Owner surfaces: `otel-ledger` (`storage.rs`, `ledger/mod.rs`), `otel-plugin`
config (`config/signal.rs`, `config/env.rs`), `netdata-plugin/bridge` config
(`StorageConfig`, `PluginConfig::lifecycle_for`), and `otel.yaml`.

## Configuration contract

- Remote storage is **global** (one switch + one backend for the whole plugin),
  not per signal. `StorageConfig` (`netdata-plugin/bridge/src/config.rs`) is a
  top-level `otel.yaml` section with four fields: `enabled: bool`, `uri: String`,
  `read_cache_max_size: ByteSize` (default `1 GB`, decimal like every other
  configured size), and `startup_op_timeout: humantime` (default **5 min**;
  hidden from the stock file — see "Hidden knobs" below; see also the
  startup-sync note). All four are
  env-overridable: `NETDATA_OTEL_STORAGE_ENABLED`, `_URI`,
  `_READ_CACHE_MAX_SIZE` (parsed as `ByteSize`), and `_STARTUP_OP_TIMEOUT`
  (parsed as a humantime duration). Env overrides apply on top of
  `otel.yaml` (env wins), via the shared `StorageOverride` path in
  `config/{signal,env}.rs`.
- **`startup_op_timeout`** bounds **each** remote operation of the fail-closed
  startup catalog diff-sync — the single recursive LIST and every catalog GET —
  not the whole restore, so recovering a large archive stays work-proportional.
  It also bounds that one recursive LIST, whose cost scales with the **whole
  bucket's** catalog cardinality (shared buckets list every machine's objects),
  so very large fleets may need a higher value. It SHOULD exceed the backend's own
  retry window (~3 min) so transient blips still retry internally; a lower value
  cuts those retries short and can crash-loop the plugin under a flaky remote. The
  startup diff-sync behavior it governs is specified in
  [otel-file-lifecycle.md](otel-file-lifecycle.md).
- There is **no** `read_cache_dir` config knob. The read-cache directory is
  **derived per signal** as `{base_dir}/{signal}/remote-read` and surfaced on the
  runtime `LifecycleConfig::read_cache_dir` by `PluginConfig::lifecycle_for`. See
  [otel-storage-substrate.md](otel-storage-substrate.md) for the base-dir /
  derived-layout contract.
- Each signal uploads under its own remote-key segment `v2/{signal}/...` (e.g.
  `v2/logs/...`, `v2/traces/...`), so one backend cleanly holds every signal.

### Operator migration (storage moved global)

The storage/auth config moved from per-signal (`logs.storage.*`) to global, with
renamed env vars. otel-logs is experimental (never GA), so this is a hard break
with no shim:

- `NETDATA_OTEL_LOGS_STORAGE_{ENABLED,URI,READ_CACHE_MAX_SIZE}` →
  `NETDATA_OTEL_STORAGE_{ENABLED,URI,READ_CACHE_MAX_SIZE}`.
- `NETDATA_OTEL_LOGS_AUTH_ENABLED` → `NETDATA_OTEL_AUTH_ENABLED`.
- `NETDATA_OTEL_LOGS_STORAGE_READ_CACHE_DIR` is **removed** with no replacement —
  the read-cache dir is now derived per signal (`{base_dir}/{signal}/remote-read`).
- The old env names are now **fatal at startup**: env resolution rejects any
  `NETDATA_OTEL_*` variable no consumer recognizes (read-tracking in
  `config/env.rs` — consumers query their full vocabulary unconditionally, so
  an unread name is a typo or a removed variable; there is no separate
  accepted-name list to maintain). The per-signal tuning env vars keep their
  `NETDATA_OTEL_{LOGS,TRACES}_*` namespaces.
- **Removed/unknown YAML keys are fatal too** (2026-07 strictness pass): every
  config struct on both parse paths (stock `PluginConfig`, user
  `ConfigOverride`) carries `serde(deny_unknown_fields)`, so a user `otel.yaml`
  with old-schema keys, misplaced sections, or typos refuses startup with an
  error naming the key and file. Per-tenant `rotation:`/`retention:` maps stay
  open (tenant names are data); typos inside a tenant entry are caught via the
  strict `RotationEntry`/`RetentionEntry` structs. When the rejected user file
  is recognizably the FORMER plugin's schema (`logs.size_of_journal_file`,
  `logs.number_of_journal_files`, ...), the error carries a migration guide
  (`config/legacy.rs`): the old→new key mapping, a defaults-changed warning
  (values are never migrated automatically — journal-file counts/sizes do not
  translate to SFST retention), and the note that old logs remain queryable.
  `logs.journal_dir` is the one former key still valid: a declared,
  strict-parse-accepted field (logs-only; rejected under `traces:`) whose value
  is consumed by the separate tolerant probe `resolve_legacy_journal_dir`.
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
- The plugin **redacts the URI in its own log messages**: the startup config
  dump (`otel-plugin` `config::log_config` via `redact_uri`) logs `storage.uri`
  as its scheme only (`s3://[redacted]`). The redaction is logging-only — the
  verbatim URI still crosses the supervisor→worker IPC intact.
- Storage **operation/probe errors are credential-redacted before logging**:
  `StorageError`'s `Display` (file-lifecycle `storage.rs`) renders the full
  error source chain through `redact` (file-lifecycle `redact.rs`), which
  strips every URL query string (`?…` → `?[REDACTED]`) — that is where AWS
  requests carry credentials (the raw web-identity JWT on the STS call,
  `X-Amz-Signature`/`X-Amz-Security-Token` on query-signed requests; reqsign
  sends the STS call as GET-with-token-in-query). The retry-layer notify log
  goes through the same redaction, and the query-path remote-read errors
  handed to the file-cache (which logs `{e:#}` verbatim) are flattened
  through the same `Display` (otel-ledger `rpc/handler.rs`
  `read_error_to_anyhow` — never extract the raw inner error there).
  `Debug` on `StorageError` is UNREDACTED
  (test-only) and MUST NOT be used to log real backend errors.
- One remaining unredacted path: a `from_uri` **parse** failure
  (`OpendalStorage::new`) may quote the URI as-is — parse errors carry no
  request, so only a secret misplaced in the URI itself could surface there.
  Credentials therefore still MUST NOT appear in the URI (the credential
  contract above).
- Logs land in systemd-journal under `otel-plugin/ledger` (query via the MCP
  `netdata_agent_logs` tool, component `ledger`).

## Read-through cache (query path)

- When `storage.enabled`, `Ledger::new` opens a bounded local read-through cache
  (the `file-cache` crate) at the per-signal derived `read_cache_dir`
  (`{base_dir}/{signal}/remote-read`), capped at `read_cache_max_size`. The
  directory MUST be local, owner-private storage; it is created `0700` and
  recovered on open.
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

## Lifecycle & ingest tuning knobs (logs)

Per-signal `logs.*` tuning in `otel.yaml` (defaults from
`netdata-plugin/bridge/src/config.rs`). These govern the on-disk/upload lifecycle
whose behavior is specified in [otel-file-lifecycle.md](otel-file-lifecycle.md);
this table is the **config surface** (names, defaults, override scope).

| Knob | Default | Override scope | Note |
|---|---|---|---|
| `logs.rotation.default.max_file_size` | `25MB` | per-tenant | WAL seal trigger (size). |
| `logs.rotation.default.max_log_entries` | `50000` | per-tenant | WAL seal trigger (count). |
| `logs.rotation.default.max_file_duration` | `15 min` | per-tenant | **Hidden.** WAL seal trigger (age); also sealed by a fixed 30 s idle sweep. Optional in the `default` entry (falls back to the code default). |
| `logs.retention.default.max_files` | `100000` | per-tenant | Local SFST retention (count). |
| `logs.retention.default.max_total_size` | `1GB` | per-tenant | Local SFST retention (bytes). |
| `logs.retention.default.max_age` | `7 days` | per-tenant | Local SFST retention (age) — **distinct from `ingest.max_age`**. |
| `logs.retention.default.horizon` | `10 years` | per-tenant | **Hidden.** Catalog (archive-index) retention = queryable remote depth. **MUST exceed `retention.max_age` by more than a day (in day units) or the plugin refuses to start** — a catalog must outlive the data it indexes. The default is deliberately far above any realistic `max_age` because the knob is hidden. |
| `logs.catalog.rotation_count` | `10` | global (logs) | **Hidden.** Catalog seal trigger (entries). |
| `logs.catalog.rotation_period` | `15 min` | global (logs) | **Hidden.** Catalog seal trigger (age); bounds how long uploaded data stays uncataloged. Plus a Flush on clean shutdown. |
| `logs.ingest.max_age` | `24 hours` | global (logs) | **Hidden.** Reject records older than this (inclusive), per record; reported via OTLP `partial_success`. |
| `logs.ingest.future_skew` | `10 minutes` | global (logs) | **Hidden.** Reject records more than this far ahead (inclusive). `0` warns at startup. |

- **Hidden knobs** (first-release contract minimization): knobs marked
  **Hidden** — plus `storage.startup_op_timeout` above — are deliberately NOT
  listed in the shipped stock `otel.yaml.in`. They remain in the schema and are
  accepted from the user `otel.yaml` and their env vars; absent, they resolve to
  the code defaults in this table. The stock-file drift test
  (`otel-plugin/src/config/mod.rs`, `shipped_stock_file_resolves_with_shipped_values`)
  asserts both the absence of hidden keys from the shipped file and the values
  their code defaults resolve to. Re-exposing (possibly renamed/moved) is an
  expected follow-up; renames land on the strict-config migration-guide path.
- Per-tenant `rotation:`/`retention:` maps stay open (tenant names are data);
  typos inside a tenant entry are still caught by the strict
  `RotationEntry`/`RetentionEntry` structs.
- The `horizon > max_age` invariant is validated in **day units** at config load;
  the violation is a hard startup error, not a warning.

## Non-goals

- Modeling per-backend options in `otel.yaml`.
- Any in-config secret storage, secret-reference grammar, or secret-key-in-URI
  guardrail (the latter explicitly declined).
