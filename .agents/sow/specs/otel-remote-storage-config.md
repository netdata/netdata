# OTel plugin remote object-storage configuration & credential contract

Scope: how the OTel plugin configures a remote object-storage backend for SFST +
catalog upload, how credentials are supplied, and the startup reachability probe.
Owner surfaces: `otel-ledger` (`storage.rs`, `ledger/mod.rs`), `otel-plugin`
config (`config/logs.rs`, `config/env.rs`), `netdata-plugin/bridge` config
(`StorageConfig`), and `otel.yaml`.

## Configuration contract

- Storage config is exactly two fields: `StorageConfig { enabled: bool, uri: String }`
  (`netdata-plugin/bridge/src/config.rs`). Overridable via env
  `NETDATA_OTEL_LOGS_STORAGE_ENABLED` / `NETDATA_OTEL_LOGS_STORAGE_URI`.
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

## Non-goals

- Modeling per-backend options in `otel.yaml`.
- Any in-config secret storage, secret-reference grammar, or secret-key-in-URI
  guardrail (the latter explicitly declined).
