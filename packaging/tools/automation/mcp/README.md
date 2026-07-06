# netdata-build-mcp

An [MCP](https://modelcontextprotocol.io) server that lets an LLM client
**configure**, **build**, and **run** the Netdata Agent from a local worktree,
using predefined build profiles.

It is a **localhost-only developer tool**. There is no authentication and no
path sandboxing: it runs `cmake`/`ninja` against any worktree path a client
sends. Do not expose it on a network.

## Model: fire-and-poll

Builds take minutes, which is longer than an MCP client will wait for a single
tool call. So nothing blocks: a `*_start` tool launches a background job and
returns a `job_id` immediately, and the client polls until the job finishes.

| Tool | Purpose |
|------|---------|
| `netdata_configure_start(worktree, profile)` | Start a cmake configure job → returns `job_id`. |
| `netdata_build_start(worktree, profile)` | Start a build job → returns `job_id`. Configures first if the build dir isn't configured for the profile (one job, phased: configure → ninja). |
| `netdata_job_status(job_id)` | Long-polls ~8s, then returns state (`running`/`succeeded`/`failed`/`cancelled`), a log-tail preview, and `log_file`. Call repeatedly until not `running`. |
| `netdata_job_logs(job_id, offset)` | Incremental output. Pass back `next_offset` to read only new lines. |
| `netdata_job_cancel(job_id)` | Terminate a running job. |

Each worktree has a **single** build tree at **`<worktree>/build`** (the standard
cmake location). The two profiles share it, so switching profile in a worktree
reconfigures + rebuilds; to run two different build types at once, use two
worktrees. The complete, unbounded build output is teed to
**`<worktree>/build/.netdata-build.log`**; `job_status` returns its path as
`log_file`. The inline `log_tail` is only a preview (≈5 lines while running, ≈10
on success, ≈80 on failure) — to keep tool responses small. On failure, grep the
`log_file` for every error rather than relying on the tail.

`configure` and `build` are peer capabilities; `status`, `logs`, and `cancel`
are generic over any `job_id`.

## Running agents

You can also run built agents, addressed by an LLM-supplied **`agent-id`**. The
build behind an agent is the worktree's single `build/` (the profile sets its
build type); each agent is its own **isolated run instance** (own dirs + port),
so multiple agents of one worktree coexist without clobbering.

| Tool | Purpose |
|------|---------|
| `netdata_agent_declare(agent_id, worktree, profile)` | Bind an `agent-id` to a `(worktree, profile)`. Idempotent. |
| `netdata_run_start(agent_id, restart=False)` | Build+install if needed, then launch netdata on an auto-assigned loopback port. Returns immediately; poll status until `ready`. First start for a profile can take minutes. Idempotent while live: a plain start does **not** rebuild a running agent. Pass `restart=true` after editing source — it stops, rebuilds (incremental), and relaunches. |
| `netdata_run_status(agent_id)` | `building`/`starting`/`ready`/`stopped`/`failed` + the port and `url` (when ready), plus `claimed`/`cloud_connected` once ready. Long-polls ~8s while coming up. |
| `netdata_run_logs(agent_id, offset)` | Combined build + netdata output, incremental. |
| `netdata_agent_logs(agent_id, component, lines, grep, priority)` | Structured logs from the systemd journal for one part of the agent — `daemon` (netdata), `supervisor` (otel-plugin), or a worker (`ledger`/`ingestor`/`legacy-logs`) — scoped to that process by `_PID`. Read-only `journalctl` wrapper. Only registered where the journal is usable (`journalctl` on PATH and a running journald); elsewhere use `netdata_run_logs`. |
| `netdata_run_stop(agent_id)` | Stop the agent (terminates its process group). |

Each agent gets an isolated runtime dir **`~/opt/netdata-mcp/run/<agent-id>/`**
(`etc`, `cache`, `lib`, `log`) and a generated `netdata.conf` (ram db, isolated
`[directories]`, bind `127.0.0.1`); netdata launches as
`<install>/usr/sbin/netdata -D -p <port> -c <conf>`. Readiness is probed via
`/api/v1/info`. The run dir is **kept after stop** for inspection; agent logs are
not in the run dir — they're in the journal (`netdata_agent_logs`) on journald
hosts, or in `netdata_run_logs` otherwise. Agents are in-memory and do not
survive a server restart.

Two agents of the **same** profile share one worktree's build/install (each gets
its own run dir + port). For two **different** build types (e.g. an optimized
parent + a debug child), use **two worktrees** — one build type each:
```
netdata_agent_declare("parent", "/path/to/worktree-opt",   "optimized")
netdata_agent_declare("child",  "/path/to/worktree-debug", "debug")
netdata_run_start("parent");  netdata_run_start("child")
# poll netdata_run_status("parent") / ("child") until "ready"
```

Edit → rebuild → rerun loop: after changing source, a plain
`netdata_run_start("agent")` on a **running** agent is idempotent and serves the
**old** binary. Pass `restart=true` to apply the change:
```
# ... edit a .c/.go/... file ...
netdata_run_start("agent", restart=true)   # stop + incremental rebuild + relaunch
# poll netdata_run_status("agent") until "ready"
```

Concurrent writes to a worktree's build dir are serialized by a **cross-process
file lock** at `<worktree>/.netdata-mcp-build.lock` (gitignored; a sibling of
`build/` so a `build/` clean can't drop a held lock). Any builder waits for the
holder — another coroutine, `netdata_build_start` vs a run's internal
build+install, or a *separate server process* (the normal case under stdio
transport, where each client session spawns its own server). The kernel releases
the lock if a server crashes, so no stale locks.

Jobs are in-memory: they do **not** survive a server restart, and calls for an
unknown `job_id` return a clean "no such job" result. One job at a time runs per
worktree build dir (a second identical request is deduplicated; a different
request for a busy build dir is reported as busy).

## Querying a running agent (agent MCP)

Once an agent is `ready`, you can drive **its own** Netdata MCP (`/mcp`) through
this server — to verify the change you just built (metrics, logs, functions,
alerts). The agent's 13 `/mcp` tools are re-exposed as native, typed
**`netdata_agent_<name>`** tools (e.g. `netdata_agent_query_metrics`,
`netdata_agent_execute_function`, `netdata_agent_list_nodes`), each taking an extra
**`agent_id`**:

```
netdata_run_start("agent");  # poll netdata_run_status until "ready"
netdata_agent_list_nodes(agent_id="agent")
netdata_agent_query_metrics(agent_id="agent", metric="system.cpu", after=-60, before=0)
```

- The call resolves `agent_id` → the ready agent's loopback port and forwards to
  its `/mcp`; an unknown or not-ready agent returns a clear message (no crash).
- Errors from the agent are forwarded **verbatim** (e.g. a missing required
  argument, or `find_anomalous_metrics` which needs ML — off in these profiles).
- The tool schemas are **vendored** (`agent_tools.py`, snapshotted via
  `scripts/snapshot_agent_tools.py`); a pinned surface, refreshed on demand.
- No auth/claim needed — localhost `/mcp` is open; forwarding is per-call.

## OTel logs: configure, feed, query

A dedicated surface for iterating on the OTel-logs path against a ready agent:

| Tool | What |
|------|------|
| `netdata_agent_otel_config(agent_id, …)` | Set otel-plugin knobs applied on the next start. REPLACES prior config. Tuning is per-signal: `logs_*` (WAL rotation, index retention, catalog) and `traces_*` knobs are independent; storage is global (unprefixed). Knobs without a first-class param (auth, ingest windows, retention `max_age`/`horizon`, per-tenant overrides) are reachable via the `extra_yaml` raw-YAML passthrough. |
| `netdata_agent_otel_push_logs(agent_id, count, …)` | One-shot: send a deterministic synthetic OTLP LOG corpus (`otel-streams synth`) to the agent. `service_name`/`service_namespace` set the resource identity (one stream per batch; query by literal `service.name`/`service.namespace`). `service_name` is always emitted (queryable even when `""`); an omitted `service_namespace` emits no token (not queryable — reachable via `service.name`), while `service_namespace=""` emits a queryable empty value. |
| `netdata_agent_otel_push_traces(agent_id, count, …)` | One-shot: send a deterministic synthetic OTLP TRACE corpus (`otel-streams synth-traces`) to the agent. Like the logs push but with `duration_nanos` (per-span duration), no `field_cardinality`, and a distinct default `service.name` (`otel-streams-synth-traces`). Pair with small `traces_*` config thresholds to seal the traces pipeline without an additional restart (the thresholds applied at the prior run_start make rotation automatic). |
| `netdata_agent_otel_stream_{start,status,stop,list}(…)` | Run a live source (`source=certstream\|jetstream\|github`) as a daemon; `list` enumerates all streams. |
| `netdata_agent_otel_logs(agent_id, …)` | Query the `otel-logs` function (typed params; mints a Cloud bearer when `NETDATA_CLOUD_TOKEN` is set). |
| `netdata_agent_otel_files(agent_id, …)` | List the storage files the otel-ledger is tracking (WAL / SFST / catalog) per tenant, with sizes, time ranges, record counts, and the `rotated`/`uploaded`/`remote_cataloged`/`pending_deletion` flags not visible on disk. Inventory, not content (use `netdata_agent_otel_logs` for rows). Same SIGNED_ID gate (mints a bearer). |
| `netdata_agent_mint_bearer(agent_id, …)` | Mint and **return** a per-agent Cloud bearer for the **Playwright (browser) MCP**, so an LLM can view SIGNED_ID-gated functions (otel-logs, otel-files, systemd-journal) in the dashboard UI. The description embeds the `browser_run_code_unsafe` + `setExtraHTTPHeaders` injection recipe. Localhost-dev only (returns the bearer by design). |

- **push/stream** shell out to `cargo run -p otel-streams --bin <name>` (built on
  demand from `<worktree>/src/crates`); they target the agent's local OTLP
  receiver, so no Cloud token is needed.
- **otel_logs** is access-gated (`SIGNED_ID`): on a claimed agent with a Cloud
  token it auto-mints a bearer; otherwise it returns 412 with a hint.
- Typical loop: `otel_config` (tiny per-signal thresholds) → `otel_push_logs` /
  `otel_push_traces` → `otel_logs` / `otel_files` to assert rotation/retention
  over a known corpus.

## Cloud claiming

When `NETDATA_CLAIM_TOKEN` is set in the **server's environment** (see `.env` /
`.agents/ENV.md` — `NETDATA_CLAIM_TOKEN`, optional `NETDATA_CLAIM_ROOMS` and
`NETDATA_CLAIM_URL`), every launched agent claims itself to Netdata Cloud as an
**ephemeral** node named **`mcp-<agent-id>`**. The name is unique and stable per
agent-id (distinct agents → distinct cloud nodes; a restart reuses the same one),
and the ephemeral marker lets Cloud auto-clean a node once it goes offline. With
no token set, agents launch **unclaimed** — local and MCP access are unaffected.

Claiming never fails a run: credentials are passed via the launch environment
(never the command line), and `netdata_run_status` reports `claimed` (has a
claimed_id) and `cloud_connected` (ACLK online / live in the Cloud UI) **as
observed, never waited on** — `cloud_connected` typically flips true a few seconds
after `ready`. One caveat inherited from netdata: the startup claim registration
is a blocking call, so an *unreachable* Cloud can delay readiness by up to ~50s —
the run still succeeds, just unclaimed.

## Dedicated worktree + clangd

Point this tool at a **worktree dedicated to LLM runs** — one where you do *not*
run a regular `cmake`/`ninja` build yourself. The tool **owns `<worktree>/build/`**:
the first configure stamps a `build/.mcp-managed` marker, and the tool **refuses**
to build if a `build/` exists *without* that marker (so it never clobbers a build
you created). Builds into `build/` would otherwise overwrite a manual build's
cache and objects — hence the dedicated-tree contract.

clangd needs no help here: the top-level CMake sets `CMAKE_EXPORT_COMPILE_COMMANDS`,
so cmake writes `<worktree>/build/compile_commands.json` — exactly where clangd's
default search looks (a file's ancestor dirs and their `build/` subdir). The
build path is also returned as `compile_commands` on build/run responses.
**clangd/editor errors that contradict a successful build are stale-database
false positives — trust the build.**

## Profiles

| Profile | Build type | Notes |
|---------|-----------|-------|
| `debug` | Debug | internal runtime checks; curated plugin set |
| `optimized` | RelWithDebInfo | no LTO; same curated plugin set |

Both profiles share one curated plugin set: common system-monitoring features on
(apps, cgroups, network-viewer, systemd-journal/units, local-listeners, debugfs,
dbengine, dashboard); heavy-to-build (go.d, ML, NetFlow, eBPF) and rarely used
plugins off. The OTEL plugin is the deliberate exception — always on, despite its
build cost, because this tool exists for OTel-logs development. Definitions live in
[`netdata_mcp/profiles.py`](netdata_mcp/profiles.py).

## Transports: stdio (default) vs http

Requires [`uv`](https://docs.astral.sh/uv/), plus `cmake` and `ninja` on `PATH`.
clang is also required (Netdata's Rust plugins drive the linker with a clang-only
flag). First: `cd packaging/tools/automation/mcp && uv sync`.

- **`stdio` (default)** — the client **spawns** the server; no port, no URL, no
  separate "start the server" step. Agents and builds are **scoped to the client
  session**: closing the client stops every agent it launched (clean — no
  leftover `netdata` processes). Best for iterate-and-done work.
- **`http`** — streamable-HTTP; you run the server yourself and point clients at
  a URL. The server runs **independently**, so agents/builds **survive client
  restarts** and can be inspected from other tools (curl, a browser). Best when
  you want to start agents (e.g. a parent/child topology) and poke at them over
  time.

```sh
uv run netdata-build-mcp                          # stdio (default)
uv run netdata-build-mcp --transport http         # streamable-HTTP on 127.0.0.1:8000/mcp
uv run netdata-build-mcp --transport http --port 8011
```

`--host` defaults to `127.0.0.1` and should stay on loopback — this server is
unauthenticated and unsandboxed; do not bind it to a public interface.

## Quick setup (recommended)

Wire the server into your agent client in one step (configures **opencode** and
**Claude Code** in your **global** config, pointing at this checkout, over stdio):

```sh
# claim creds are required (launched agents auto-claim to Cloud):
export NETDATA_CLAIM_TOKEN=…   # and optionally NETDATA_CLAIM_ROOMS
ninja setup-mcp                 # from your build dir, or:
python3 packaging/tools/automation/mcp/scripts/setup_mcp.py --tool all
# …or pass them explicitly (CLI beats env):
#   setup_mcp.py --claim-token … [--claim-rooms …] [--claim-url …]
```

- It mutates **your** global config (`~/.config/opencode/opencode.json` via a
  safe merge; Claude via `claude mcp add --scope user`) — never the repo. It
  adds only the `netdata-build` server and is idempotent.
- **Claim creds are wired into the client's per-server env** (opencode
  `environment`, Claude `--env`), so launched agents auto-claim. The **token is
  required** — `--claim-token` or `NETDATA_CLAIM_TOKEN`; setup **fails** if
  neither is set. This writes the token into your global client config (the
  intended cost of pinning it per-server).
- `--tool opencode|claude|all` selects which to configure; a missing client is
  skipped, not an error.
- Global config points the server at **this checkout's code**, but the server
  builds whatever worktree you pass to `netdata_agent_declare` — so **one setup
  serves all your worktrees**. Re-run only if you want the server itself to run
  from a different checkout (e.g. you're developing the MCP server). Requires
  `uv` on `PATH`.

Restart your client afterwards to pick up the server. To wire it up by hand
instead, see below.

## Client configuration (example: opencode)

**stdio** — the client launches the server (use `--directory` so `uv` finds the
project regardless of the client's cwd):

```json
{
  "mcp": {
    "netdata-build": {
      "type": "local",
      "command": ["uv", "run", "--directory",
        "/abs/path/to/packaging/tools/automation/mcp",
        "netdata-build-mcp", "--transport", "stdio"]
    }
  }
}
```

**http** — run `uv run netdata-build-mcp --transport http` yourself, then:

```json
{
  "mcp": {
    "netdata-build": {
      "type": "remote",
      "url": "http://127.0.0.1:8000/mcp"
    }
  }
}
```

> Tool names changed from the earlier proof-of-concept (`netdata_configure` /
> `netdata_build`) to the fire-and-poll surface above. Update any existing
> client config to the new `*_start` + `netdata_job_*` tools.

## Development

```sh
uv run pytest          # unit tests (profiles, runner, job registry)
```

Layout — a transport-free core with no `mcp` imports, plus a thin MCP layer:

```
netdata_mcp/
  profiles.py      # profile -> cmake -D flags (pure data)         ── core
  buildcfg.py      # single per-worktree build dir + ownership + commands ── core
  locks.py         # cross-process build-dir file lock                ── core
  runner.py        # async subprocess + bounded log buffer         ── core
  jobs.py          # Job + JobRegistry (build/configure lifecycle) ── core
  runtime.py       # agent run dir + netdata.conf gen, port, readiness probe ── core
  agents.py        # AgentRegistry (agent-id -> worktree/profile)  ── core
  run.py           # Run + RunRegistry (launch/readiness/stop)     ── core
  bearer.py        # Cloud per-agent bearer minting/cache (otel-logs auth) ── core
  agentfn.py       # call a netdata function over HTTP (otel-logs) ── core
  streams.py       # Stream + StreamRegistry, synth/stream cargo runners ── core
  journal.py       # /proc PID resolution + read-only journalctl wrapper ── core
  server.py        # FastMCP instance, lifespan-held registries
  tools/
    job_control.py # netdata_job_status / _logs / _cancel
    configure.py   # netdata_configure_start
    build.py       # netdata_build_start
    agents.py      # netdata_agent_declare
    run.py         # netdata_run_start / _status / _logs / _stop
    logs.py        # netdata_agent_logs (journalctl wrapper; journald hosts only)
    mint_bearer.py # netdata_agent_mint_bearer (per-agent bearer for the Playwright MCP)
    otel_config.py # netdata_agent_otel_config
    otel_logs.py   # netdata_agent_otel_logs
    otel_files.py  # netdata_agent_otel_files (storage-file inventory)
    otel_push.py   # netdata_agent_otel_push_{logs,traces} (one-shot synth)
    otel_stream.py # netdata_agent_otel_stream_{start,status,stop,list}
```

Adding a capability domain = a new `tools/<domain>.py` exposing `register(mcp)`
plus one call in `server.py`.
