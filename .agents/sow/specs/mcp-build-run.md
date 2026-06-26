# MCP build/run tool contracts

Durable contracts for the local MCP server under
`packaging/tools/automation/mcp/` that lets an LLM configure, build, and run the
Netdata Agent from a worktree. This file records the **decisions and accepted
trade-offs** behind that subsystem — the parts a future reader cannot infer from
the code alone. Mechanics that are obvious from the code (the tool list, the
async plumbing) are intentionally not restated here.

## Build profiles

- The tool exposes exactly **two** profiles, not arbitrary CMake toggles:
  - `debug` — `Debug` + internal runtime checks.
  - `optimized` — `RelWithDebInfo`.
- Both share one **curated, user-reviewed plugin set** (heavy/rare plugins off:
  go.d, ML, eBPF, NetFlow, exporters, etc.; kept on: systemd journal/units,
  journal-file reader, local-listeners, debugfs).
- **The otel plugin is the deliberate exception: always ON** despite its build
  cost, because this tool exists for OTel-logs development (build/run/verify the
  otel subsystem). `ENABLE_PLUGIN_OTEL=On` in both profiles.
- Rationale: a deliberately small surface for LLM-driven builds. Extra knobs
  (ASan, per-plugin toggles, a runtime-overrides layer) were **deferred**, not
  rejected — re-addable later if a concrete need appears.

## Single build directory per worktree

- Each worktree builds into a single `<worktree>/build/`. There is **one install
  per worktree** and **one build type per worktree** at a time.
- Switching a worktree's profile **reconfigures in place** (driven by
  `CMAKE_BUILD_TYPE` in `CMakeCache.txt`), it does not create a second build dir.
- A (re)configure is also triggered when the cached `CMAKE_INSTALL_PREFIX`
  (path-normalized) differs from the canonical `profiles.install_prefix(worktree)`
  (or is absent). The run path always launches from `install_prefix`, but
  `ninja install` writes to the cached prefix; without this check a stale cached
  prefix (e.g. an older tool layout) would install fresh binaries to one tree
  while the agent launches a stale binary from another. `needs_configure`
  enforces both the build-type and install-prefix invariants.
- To run two profiles in parallel, use **two worktrees** — this is the supported
  model, not a limitation to work around.
- `clangd` finds `<worktree>/build/compile_commands.json` natively, so there is
  **no compile-commands symlink** to maintain. Editor/clangd errors that
  contradict a successful build are stale-database false positives.

## Dedicated-tree ownership contract

- The tool **owns** `<worktree>/build/`. A worktree handed to the tool is a
  dedicated build tree, not a user's working build dir.
- Ownership is marked by a `build/.mcp-managed` file. The tool **refuses** to
  build into a pre-existing `build/` that lacks this marker (foreign-build guard),
  so it never clobbers a user's own CMake build.
- The build dir is **claimed before configure** (assert-ownable then stamp the
  marker), so a failed or killed configure leaves a recoverable, owned dir.
- Accepted trade-off (TOCTOU): the claim runs **before** the cross-process
  build-dir lock, so the assert-ownable → stamp window is not atomic. Under the
  one-build-type-per-worktree model this is acceptable; hardening it (claim under
  the lock) is optional future work, not a defect.

## Install paths

- An agent installs to `~/opt/netdata-builds/<worktree-basename>/netdata`.
- The path keys on the worktree **basename only**. Two worktrees with the same
  leaf name would collide.
- This is **avoided by convention, not enforced**: worktrees are expected to live
  as uniquely-named children of one top-level directory. The collision is an
  accepted residual; the tool does not guard against it.

## Build-dir lock placement

- The cross-process build lock lives at `<worktree>/.netdata-mcp-build.lock` — a
  gitignored sibling of `build/`, not inside it.
- Rationale: a lock inside `build/` would be destroyed by `rm -rf build/` while a
  build is queued on it. The sibling location survives a build-dir wipe. The
  `.netdata-mcp-build.lock` pattern is in the repo-root `.gitignore`.

## Transport

- Default transport is **stdio** (zero-config for a single user/editor).
- A persistent shared server uses `--transport http` (`--host`/`--port`).

## Cloud claiming

- Auto-claim is **opt-in via credentials, default-on when present**: when
  `NETDATA_CLAIM_TOKEN` (optional `NETDATA_CLAIM_ROOMS`, `NETDATA_CLAIM_URL`) is in
  the server's environment, every launched agent claims to Netdata Cloud; with no
  token, agents launch **unclaimed** and local/MCP access is unaffected.
- Credentials are passed via the launch **process env**, never the command line,
  never written to the generated `netdata.conf`, never logged.
- Each agent is a unique, stable, ephemeral cloud node:
  - `[global] hostname = mcp-<agent_id>` → unique display name; one agent_id maps
    to one stable machine_guid (its run dir), so a restart reuses the same node
    and distinct agents are distinct nodes.
  - `[global] is ephemeral node = yes` → Cloud auto-cleans the node once offline,
    so stopped dev agents don't accumulate.
- Claiming is **best-effort and never gates the run**. It has two phases with
  different timing, both reported (never waited on) as `claimed` / `cloud_connected`
  on `RunInfo`:
  - Phase 1, registration (gets a `claimed_id`): the daemon does this at startup,
    **before** the web server — a blocking curl loop bounded by netdata's own
    retries (~50s worst case on an unreachable Cloud). It delays readiness on a
    Cloud outage but never fails the run (the agent comes up unclaimed).
  - Phase 2, ACLK connection (the node goes live in the Cloud UI): asynchronous,
    background, retries indefinitely. We do not wait for it; `cloud_connected`
    typically flips true a few seconds after the agent is `ready`.
- Rationale for not waiting on Cloud: runs must not depend on Cloud responsiveness
  for success. The phase-1 startup tail is an accepted residual (no external knob
  to shorten it); claiming after launch to remove it was rejected as not worth the
  machinery.
- Credential provisioning: the client-wiring helper `scripts/setup_mcp.py` injects
  the claim creds into each client's per-server env (opencode `environment` map,
  Claude `mcp add --env`) so the server process has them. Each credential resolves
  CLI flag (`--claim-token`/`--claim-rooms`/`--claim-url`) → matching env var; the
  token is **mandatory at setup time** and setup fails if it resolves via neither.
  This is a stricter setup-time guard than the runtime, which still launches
  unclaimed when the server env carries no token. The token is written into the
  user-global client configs (accepted, never committed).

## OTel-logs automation surface

This subsystem exists to iterate on the **OTel-logs** path end to end —
configure → feed (synthetic or live) → query — plus one hard constraint a future
reader cannot infer from the code:

- **Run-time otel config (`netdata_agent_otel_config`).** Sets typed otel knobs
  on a declared agent; applied on the next `netdata_run_start`. Tuning is
  **per-signal**: `logs_*` and `traces_*` knob pairs (WAL
  `max_file_size`/`max_log_entries`/`max_file_duration`, CRC/compression, index
  `max_files`/`max_total_size`, catalog `rotation_count`) configure the two
  pipelines independently in one call — an omitted knob keeps that signal's stock
  default. `storage_enabled`/`storage_uri` and the OTLP endpoint are **global**
  (unprefixed), mirroring the plugin's global storage + per-signal tuning model;
  `journal_dir` is logs-only (below). The runtime writes
  them to `<run_dir>/etc/otel.yaml` and **pins `[directories] config` to
  `<run_dir>/etc`** so the otel plugin reads exactly that file. Only set fields
  are emitted; the rest take plugin defaults. The OTLP endpoint is an
  auto-assigned free loopback port surfaced as `RunInfo.otlp_endpoint`. Purpose:
  force storage edge cases (rotation/retention) with tiny thresholds + small
  data.
  - `journal_dir` is `logs.journal_dir` for the read-only `legacy-otel-logs`
    viewer: the directory of journal files written by the **former** otel plugin.
    Unlike WAL/index (always pinned under the run dir), it is a caller-supplied
    path the plugin only **reads**. It is the one knob that lets the MCP point
    the legacy viewer at a fixture for scripted verification.
  - Coupling: emitting `journal_dir` as a sibling of `wal`/`index` under `logs`
    relies on the otel plugin's override structs **not** using
    `serde(deny_unknown_fields)` (it reads `journal_dir` via a separate permissive
    probe). A future agent-side tightening that adds `deny_unknown_fields` to the
    `logs` override would make every MCP-launched agent fail to parse its user
    `otel.yaml`; keep the `logs` override permissive, or stop emitting this key.
- **Synthetic push (per signal).** Two one-shot tools, one per signal:
  - `netdata_agent_otel_push_logs` → `otel-streams` `synth`. Generates a
    deterministic batch of OTLP `LogRecord`s (monotonic timestamps; cycled
    severity as low-card `level`; `host`/`code` over `--field-cardinality` as
    mid-card; unique `seq` as high-card).
  - `netdata_agent_otel_push_traces` → `otel-streams` `synth-traces`. Generates a
    deterministic batch of OTLP spans; carries `duration_nanos` (per-span
    duration) instead of `field_cardinality`. Pair with small `traces_*` config
    thresholds to rotate + seal + upload the traces pipeline **without a restart**.
  Both send through the **same production `Sender` / `build_export_request`** the
  live sources use, are reproducible by params (no RNG, no clock), and are
  **one-shot, synchronous**: resolve the agent's `otlp_endpoint`, run the
  generator to completion, return records-sent / success / log tail.
  - **Service identity (`service_name` / `service_namespace`).** The push sets
    the OTLP resource attributes `service.name` (default `otel-streams-synth`)
    and `service.namespace`. otel-ledger keys a storage **stream** on
    `(service.namespace, service.name)` — one identity per indexed file — so each
    batch carries a single identity; push multiple batches with distinct
    identities to create multiple streams. Both are **queryable by their literal
    field names** (`service.name`, `service.namespace`) via `otel_logs`
    `selections`/`facets`/`query`: resource attributes are flattened into the
    same key namespace as log/scope attributes at ingest, with no remapping.
  - **Absent vs empty (verified live).** `service.name` is **always emitted**
    (even `""`), so it is always a queryable field. `service.namespace` is
    emitted **only when set**: omitting it (the default) emits no token, so those
    records are *not* a queryable `service.namespace` value (the stream's
    namespace defaults to `""` for storage partitioning, but there is no facet to
    select on) — they are reachable only via `service.name`. Passing
    `service_namespace=""` *explicitly* emits an empty-valued token that **is**
    queryable as `service.namespace=[""]`. This is standard OTel semantics:
    an absent attribute differs from a present-but-empty one.
- **Live streams (`netdata_agent_otel_stream_{start,status,stop}`).** The
  real-world sources (certstream/jetstream/github) are daemons, so they get a
  start/status/stop lifecycle (one **source-enum** trio, not a tool per source)
  backed by a `StreamRegistry` reusing `runner.py`'s process-group spawn +
  SIGTERM→SIGKILL teardown (mirrors `RunRegistry`; torn down on server
  shutdown). Source-specific params (`url`/`collections`/`start`/`rate`) are
  rejected when they don't match the chosen `source`.
- **Invocation (push + streams).** Both shell out to `cargo run -p otel-streams
  --bin <name>` from `<worktree>/src/crates` — otel-streams is **not** built by
  the agent's cmake build, so cargo builds it on first call and caches it. No
  Cloud token/bearer here: the push targets the agent's local OTLP receiver, not
  the access-gated function, so no secret surface (`tenant_id` is an identifier).
- **Dedicated query tool (`netdata_agent_otel_logs`).** POSTs a typed
  `OtelLogsRequest` straight to `/api/v3/function?function=otel-logs` —
  deliberately **not** via the `/mcp` `execute_function` wrapper, so every wire
  param is exposed and the parsed response is returned for assertion;
  `info=true` probes the function descriptor.

- **Storage-file inventory (`netdata_agent_otel_files`).** A second mode of the
  same `otel-logs` function: the request carries `files: true` (sibling of
  `info`), and the handler returns a read-only snapshot of the ledger's
  in-memory registries — per tenant, the tracked WAL / SFST / catalog files with
  size, time range, record counts, and crucially the `rotated` / `uploaded` /
  `remote_cataloged` / `pending_deletion` per-seq flags. Those flags are why this reports from the
  **plugin**, not a filesystem scan: a locally-evicted SFST can still be
  cataloged on the remote, and on-disk presence ≠ "the ledger is tracking it".
  The MCP tool `netdata_agent_otel_files(agent_id)` forwards `{"files": true}`
  and returns all tenants (no filter). Built in `otel-ledger`
  (`rpc/wire.rs` `FilesResponse` + `rpc/handler.rs` `files` branch under a brief
  read lock); the mode is additive — ordinary queries (`files` absent) are
  unaffected. It is an **inventory**, not log content (that stays
  `netdata_agent_otel_logs`).

- **Access-gating constraint (durable).** The `otel-logs`
  function declares `access = SIGNED_ID | SAME_SPACE | SENSITIVE_DATA`
  (`otel-ledger/src/ledger/rpc/handler.rs`). A **local, unclaimed, anonymous**
  caller gets at most `HTTP_ACCESS_ANONYMOUS_DATA`, so the function returns
  **HTTP 412** ("authenticated via Netdata Cloud SSO") on **every** transport —
  raw HTTP *and* `/mcp` (which passes the caller's access straight through; no
  localhost bypass exists). Bearer tokens are signature-guarded local files and
  a minted token inherits only the caller's own access, so a `SIGNED_ID` bearer
  requires an already-SSO'd identity. The gate is enforced before the function
  handler runs, so it applies to **every** `/api/v3/function` call including
  `info=true` and `files=true` — there is no info/files exemption. Consequence: only the **push side**
  (OTLP/gRPC to the receiver, a separate endpoint) works on a plain local agent;
  **both `info` probing and live queries require a claimed agent + a Cloud-minted
  bearer**. The query tool's code is transport-correct regardless; the gate is
  the agent's, not the request's. (Every wrapped `netdata_agent_*` tool —
  `execute_function` and the other vendored tools — now satisfies this gate by
  minting+attaching a per-agent bearer on every forwarded `/mcp` call; see "Agent
  MCP wrapper". The dedicated `otel_logs` tool stays separate for its typed
  params; it is the one path that still permits an anonymous call (then 412s with
  a hint) rather than hard-erroring — a deliberate divergence from the wrapper's
  bearer-required invariant, not an oversight.)

- **Bearer auth (`bearer.py`).** To satisfy that gate, when `NETDATA_CLOUD_TOKEN`
  is in the server env the `otel_logs` tool mints a per-agent bearer and sends it
  as `Authorization: Bearer` (lazily, on the first query — not at run_start). The
  mint mirrors the `query-netdata-agents` flow: read the agent's identity from
  `/api/v3/info` (`agents[0].mg`/`.nd`/`.cloud.claim_id`), then
  `GET https://<NETDATA_CLOUD_HOSTNAME>/api/v2/bearer_get_token?node_id&machine_guid&claim_id`
  with the Cloud token in the header. Bearers are cached **in-process** keyed by
  `(machine_guid, claim_id)` with a refresh buffer (no secrets on disk; a re-claim
  re-mints, and a restart re-mints).
  With no Cloud token the call stays anonymous (→ 412 with a hint). A mint
  failure is a hard error (an anonymous retry would just 412); it usually means
  the agent is not yet claimed/`cloud_connected`.
  - **Token-safety (HARD):** `NETDATA_CLOUD_TOKEN` (the Cloud account token) is a
    secret — never logged, never returned in tool output, never on a command
    line (header only); error strings are scrubbed. A no-leak unit test drives a
    mint failure with a sentinel token and asserts it never surfaces. Minted
    per-agent bearers are normally also kept out of tool output; the **one
    deliberate exception** is `netdata_agent_mint_bearer` (below), which returns
    the per-agent bearer for the localhost Playwright workflow. The Cloud token
    is never exposed by that tool either.
  - **Provisioning:** `setup_mcp.py` requires a Cloud token (like the claim
    token) and injects `NETDATA_CLOUD_TOKEN` (+ optional `NETDATA_CLOUD_HOSTNAME`,
    default `app.netdata.cloud`) into the client per-server env, alongside the
    claim creds. Same accepted cost as the claim token (briefly on the `claude`
    argv; written to the user-global client config).
  - **Cloudflare gotcha:** `app.netdata.cloud` is behind Cloudflare, whose WAF
    rejects the default `Python-urllib/*` User-Agent with `HTTP 403 "error code:
    1010"` (banned signature) before auth runs. The mint request therefore sends
    a plain `User-Agent: netdata-build-mcp/1.0`, which passes the WAF and reaches
    the auth layer.

- **Browser UI E2E (`netdata_agent_mint_bearer` + the Playwright MCP).** The
  Agent dashboard renders SIGNED_ID-gated functions (otel-logs, otel-files,
  systemd-journal) only when the browser's requests carry an
  `Authorization: Bearer` with a signed identity; a headless browser hitting the
  agent anonymously gets the same **412** as any anonymous caller. The
  netdata-build MCP and the Playwright (browser) MCP cannot talk directly, so
  `netdata_agent_mint_bearer(agent_id)` mints (via `bearer.py`) and **returns**
  the per-agent bearer to the LLM, which injects it into the page. The canonical
  workflow (also embedded verbatim in the tool's description so no session
  re-derives it):
  1. `netdata_agent_mint_bearer(agent_id)` → `bearer`.
  2. Via the Playwright MCP's `browser_run_code_unsafe`, set the header on the
     page: `async (page) => { await page.setExtraHTTPHeaders({ Authorization:
     'Bearer ' + <bearer> }); }`. The header persists across the page's
     navigations/reloads until the page closes.
  3. Re-trigger the request (navigate within the dashboard or reload) so the
     gated call re-fires with the header → 200 → renders.
  This is the **one** tool that returns the per-agent bearer (the deliberate,
  localhost-dev-only exception to the token-safety bullet above); the Cloud
  account token is still never exposed. Verified live: anonymous `otel-logs` →
  412; after injection → 200 and the Logs Explorer rendered the pushed corpus.

## Agent MCP wrapper

- A `ready` agent's own Netdata MCP (`http://127.0.0.1:<port>/mcp`) is re-exposed
  through the build-MCP so an LLM can query the agent it just built. The agent's
  13 `/mcp` tools are **vendored** (baked typed schemas, `agent_tools.py`) and
  registered as native `netdata_agent_<name>` tools, each with an injected required
  `agent_id` (chosen over a generic dispatcher for LLM tool-use accuracy).
- Registration uses the FastMCP `parameters`/`fn_metadata` split: a `Tool` carries
  the vendored `parameters` (what the LLM sees) backed by a permissive arg model +
  one generic forwarder (`tools/vendored.register_forwarding_tool`). Leans on SDK
  internals (`mcp>=1.27.2`); guarded by tests (register+list+call) rather than a
  hard pin, so a breaking SDK upgrade fails CI rather than ships silently.
- A call resolves `agent_id` → the ready run's port via `RunRegistry` (unknown /
  not-ready → a clear message, never a crash) and forwards opaquely to the agent's
  `/mcp` per-call (stateless transport, no cached session).
- **Bearer invariant (auth required, no anonymous path).** Every forwarded call
  attaches a per-agent Cloud bearer (`bearer.py`, minted from the agent's
  `/api/v3/info` identity) as `Authorization: Bearer` on the `/mcp` connection.
  Agent functions are access-gated (e.g. `SIGNED_ID`) even on localhost once the
  agent is claimed, so an anonymous `/mcp` call would 412. Therefore
  `NETDATA_CLOUD_TOKEN` is **required** and the mint **must** succeed — both are
  hard errors with a clear message (no best-effort anonymous fallback), even for
  functions that don't need auth, so downstream code can assume a bearer is
  present. Consequence: the wrapped `netdata_agent_*` tools require a configured
  Cloud token and a claimed/cloud_connected agent; they do not work on a
  no-cloud/unclaimed localhost run. (The bearer is attached via the SDK's
  `create_mcp_http_client` factory — an httpx client preserving the MCP 30s/300s
  timeout defaults — since the streamable-HTTP client's own `headers`/`auth`
  kwargs are deprecated.)
- Errors are forwarded verbatim: tool-execution failures arrive as content;
  protocol-level rejections (e.g. a missing required argument) are raised by the
  client as `McpError` and turned into text; an unreachable agent yields a clean
  string. (So e.g. `find_anomalous_metrics`, which needs ML — off in these
  profiles — just returns the agent's error.)
- The vendored schemas are a **pinned** surface (drift from the live agent is
  accepted); refresh with `scripts/snapshot_agent_tools.py` against a live agent.

## Job and run lifecycle

- State is **in-memory only**: jobs and runs do not survive a server restart, and
  there is **no eviction** — `_jobs`/`_runs`/`_start_locks` grow with distinct
  ids for the server's lifetime. Acceptable for a localhost dev tool with few
  agents; not a persistence layer.
- There are **two registries by design**, sharing primitives but not merged:
  - `JobRegistry` — finite build/configure jobs (run an ordered phase list to a
    terminal state; dedup/refuse-if-busy per build dir).
  - `RunRegistry` — long-lived `netdata` launches whose readiness is probed and
    whose terminal state is derived from process exit, with a per-agent restart
    that tears down and relaunches to pick up source edits.
- Shared lifecycle primitives live in `runner.py`
  (`escalate_cancel`/`drain_all`/`await_task` over a `Cancellable` protocol;
  `run_phases` over a `PhaseHost` protocol). Cancellation is always
  SIGTERM → grace → SIGKILL on the **process group** (children spawn in a new
  session) so build descendants cannot survive holding the output pipe.
- A run records `netdata`'s exit code as `returncode` once the launch ends;
  **negative means killed by a signal** (e.g. `-9` for SIGKILL).

## Agent logging and diagnostics

- Generated `netdata.conf` always sets `[logs] collector` and `daemon` so a
  launched agent's logs are readable back by the harness, never left only in
  on-disk `collector.log`/`daemon.log`. The method depends on the host:
  - **journald present** → `collector = journal`, `daemon = journal`: logs go to
    the **systemd journal**, queryable per-agent via `netdata_agent_logs`.
  - **journald absent** → `collector = stderr`, `daemon = stderr`: logs go to
    netdata's stderr, which the runner captures into the run buffer, so they
    surface through `netdata_run_logs` (`netdata_agent_logs` is not registered on
    such hosts — see the gate bullet below).
- `collector` is load-bearing for plugins: netdata exports `NETDATA_LOG_METHOD`
  (plus `NETDATA_LOG_FORMAT`/`LEVEL`) to the plugins it spawns from it. With
  `journal` the otel-plugin (Rust, `tracing`) emits via its journald layer; with
  `stderr` it writes to fd 2. netdata redirects a plugin's fd 2 to a file **only**
  when the collector method is itself a file (`nd_log_collectors_fd`,
  `nd_log-internals.c`), so `stderr` leaves fd 2 inherited up to netdata's own
  stderr — captured by the runner. Single mechanism, no launch-env wiring.
  `journal` is gated on a present journald socket because the otel-plugin's
  `tracing-journald` layer *panics* if it can't connect; `stderr` is always safe.
- SYSLOG_IDENTIFIERs: the netdata daemon → `netdata`; the otel-plugin supervisor
  → `otel-plugin`; each worker → `otel-plugin/<worker>` (`ledger`, `ingestor`,
  `legacy-logs`). The worker name equals both the identifier suffix and the
  `worker <name>` spawn arg (otel-plugin `supervisor.rs`/`main.rs`).
- `netdata_agent_logs(agent_id, component, lines, grep, priority)` is a
  **read-only** `journalctl` wrapper returning structured output. It scopes a
  query to one agent by `_PID`: `daemon` uses the tracked netdata PID;
  `supervisor`/worker PIDs are resolved from `/proc` as descendants of that
  netdata PID (workers are never restarted, so the PID is stable for the run).
  `_PID` scoping is required for `daemon` because `SYSLOG_IDENTIFIER=netdata` is
  shared host-wide. If the process can't be resolved (agent stopped, or its
  process not yet/no-longer in `/proc`), the query falls back to identifier-only
  and says so in `message`.
- The wrapper never raises: a missing `journalctl`, a timeout
  (`journal.READ_TIMEOUT`), or a non-zero exit are surfaced as text, matching the
  structured-result contract of the other agent tools.
- `netdata_agent_logs` is registered only when the journal is usable on the host
  — `journalctl` on PATH **and** a running journald (`journal.usable()`). Off
  systemd it isn't exposed at all (agents log to stderr/files there, so it would
  have nothing to read), steering callers to `netdata_run_logs`.
- `netdata_run_logs` remains the catch-all combined build+netdata stream
  (unfiltered); `netdata_agent_logs` is the scoped, per-component journal view.
