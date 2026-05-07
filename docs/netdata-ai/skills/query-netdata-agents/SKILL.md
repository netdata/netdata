---
name: query-netdata-agents
description: Query Netdata Agents (parents and children) directly via their HTTP API on port 19999. Includes a bearer-token helper that mints, caches, and transparently refreshes a per-agent bearer from a long-lived Netdata Cloud token, and auto-detects bearer-protected agents. Use when the user asks how to call an agent's REST API or Function directly, query an agent's logs/metrics/alerts directly, mint a bearer token from a cloud token, or work around bearer protection.
---

# Query Netdata Agents directly

This skill teaches end-users (and AI assistants helping them) how to
talk to a Netdata Agent's HTTP API directly, including
bearer-protected agents that require an SSO-issued bearer token.

It is the sibling of [`query-netdata-cloud`](../query-netdata-cloud/SKILL.md).
The two skills cover different transports for the same underlying
agent API.

## Index of guides

| Domain | Guide |
|---|---|
| Generic Function invocation | [query-functions.md](./query-functions.md) |
| Logs (`systemd-journal`, `windows-events`, `otel-logs`) | [query-logs.md](./query-logs.md) |
| Topology (`topology:snmp`) | [query-topology.md](./query-topology.md) |
| Flows (`flows:netflow`) | [query-flows.md](./query-flows.md) |
| Alerts (v3 paths) | [query-alerts.md](./query-alerts.md) |
| DynCfg (`/api/v3/config`) | [query-dyncfg.md](./query-dyncfg.md) |
| Time-series metrics (`/api/v3/data`) | [query-metrics.md](./query-metrics.md) |
| Node identity, hardware, vnodes | [query-nodes.md](./query-nodes.md) |
| Streaming (parent / child / replication) -- agent-only | [query-streaming.md](./query-streaming.md) |
| **Operational how-tos (live catalog)** | [how-tos/INDEX.md](./how-tos/INDEX.md) |
| **Verification questions (consumed by SOW-0006 harness)** | [verify/questions.md](./verify/questions.md) |


| Transport | Auth | When to use |
|---|---|---|
| Cloud-proxied (sibling skill) | Cloud token | Default. Works for any team member with cloud access. No agent-side bearer needed. |
| Direct-agent (this skill) | Per-agent bearer (UUID, ~24h TTL) | Power users; lower-latency batch fetches; bypasses the Cloud round-trip; required when Cloud is unavailable. |

For **what** to query (function payloads, body schemas), see the
sibling skill -- the agent and the Cloud proxy expose the same
Function payload shape.

This skill ships shell scripts at
[`scripts/_lib.sh`](./scripts/_lib.sh) that automate the bearer mint
/ cache / refresh / call-function flow. End-users can either use the
scripts as a black box, or read the script source as a reference
implementation.

---

## Mandatory Requirements (READ FIRST)

1. **If you analyze, you author a how-to.** When asked a concrete
   question about an agent that isn't already covered by an
   existing how-to under [`how-tos/`](./how-tos/), you MUST author
   a new how-to and add it to
   [`how-tos/INDEX.md`](./how-tos/INDEX.md) BEFORE completing the
   task. The catalog is **live** -- the next assistant should not
   redo the same analysis.
2. **Use the token-safe wrappers.** `agents_query_cloud`,
   `agents_query_agent`, `agents_call_function` from
   [`scripts/_lib.sh`](./scripts/_lib.sh) handle auth internally
   and emit only the response body to stdout. Never write raw
   curl with a literal `Authorization: Bearer $TOKEN` or
   `X-Netdata-Auth: Bearer <uuid>`. Bearers / cloud tokens /
   claim_ids must NEVER reach assistant-captured stdout.
3. **Provide actionable instructions.** End every recommendation
   with a runnable wrapper invocation.
4. **Never request credentials.** Use env-key placeholders
   (`NETDATA_CLOUD_TOKEN`, `AGENT_EVENTS_HOSTNAME`,
   `AGENT_EVENTS_NODE_ID`, etc.) -- the user fills `.env` locally.
5. **Bearer values stay in `.env` and `.local/`.** The bearer
   cache file at `<repo>/.local/audits/query-netdata-agents/
   bearers/<machine_guid>.json` is mode 0600 and gitignored. The
   internal helper `_agents_resolve_bearer` returns it via bash
   nameref, never to stdout.
6. **For bearer-protected agents, default to the Cloud-token
   flow** in this skill (it auto-mints + caches the bearer).

---

## Prerequisites

- All [SKILL.md prereqs from `query-netdata-cloud`](../query-netdata-cloud/SKILL.md#prerequisites):
  cloud token, space ID, room ID, node UUID.
- Network access to the agent on port 19999 (or whatever it binds).
  Test with: `curl -sS http://AGENT_HOST:19999/api/v3/info` -- a 200
  with JSON confirms reachability.
- The agent's `claim_id` if you intend to mint a bearer. It's at
  `/api/v3/info` -> `.agents[0].cloud.claim_id`, or with shell
  access at `<netdata-prefix>/var/lib/netdata/cloud.d/claimed_id`.
  For the install-prefix detection rule, see
  [`scripts/_lib.sh`](./scripts/_lib.sh).

`.env` keys consumed (none are added by this skill -- the four
existing `AGENT_EVENTS_*` keys cover the maintainer-facing
agent-events workflow):

| Key | Role |
|---|---|
| `NETDATA_CLOUD_TOKEN` | Cloud REST token used to mint per-agent bearers |
| `NETDATA_CLOUD_HOSTNAME` | Cloud REST host |
| `AGENT_EVENTS_HOSTNAME` | When working with the agent-events node specifically -- ssh + direct-HTTP host (IP or DNS name). NOT the journal namespace (hardcoded `agent-events`). |
| `AGENT_EVENTS_NODE_ID` | Target node UUID for direct calls |
| `AGENT_EVENTS_MACHINE_GUID` | Bearer cache key (one bearer per machine_guid) |

---

## Detect bearer protection

The signal is HTTP `412 Precondition Failed` from the agent for any
authenticated path (e.g. `/host/<uuid>/api/v3/function?...`). The
response body is `You need to be authorized to access this resource`.

```bash
# Probe -- 412 means bearer required, 200 means open access
HOST="agent.example.invalid:19999"
NODE="YOUR_NODE_UUID"

curl -s -o /dev/null -w '%{http_code}\n' -X POST \
  -H 'Content-Type: application/json' \
  "http://$HOST/host/$NODE/api/v3/function?function=systemd-journal" \
  -d '{"info":true}'
```

The unauthenticated `/api/v3/info` endpoint is always reachable
(returns 200 with the agent's identity). Use it to confirm the host
is up before checking auth.

---

## Mint a per-agent bearer

**Endpoint:** `GET /api/v2/bearer_get_token` on Netdata Cloud.

Required query parameters: `node_id`, `machine_guid`, `claim_id`.
Auth: Cloud token in `Authorization: Bearer ...`.

```bash
TOKEN="YOUR_API_TOKEN"
NODE_ID="YOUR_NODE_UUID"
MACHINE_GUID="YOUR_MACHINE_GUID"
CLAIM_ID="YOUR_CLAIM_ID"

curl -sS \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/bearer_get_token?node_id=$NODE_ID&machine_guid=$MACHINE_GUID&claim_id=$CLAIM_ID"
```

Response body:

| Field | Description |
|---|---|
| `token` | The 36-char UUID bearer; pass to the agent in `X-Netdata-Auth: Bearer <token>` |
| `expiration` | Numeric. Format may be Unix ms or seconds; treat values > 10^12 as ms |
| `bearer_protection` | `true` if the agent IS bearer-protected; the token still works either way |
| `mg` | Echoed `machine_guid` |
| `status` | Status code |

Permission gate (Cloud-side): `PermissionSpaceRead` on the target
space; node must be `reachable`. If the agent is `stale`, the call
returns 400.

---

## Use the bearer to call an agent

```bash
HOST="agent.example.invalid:19999"   # the agent's bind address
NODE="YOUR_NODE_UUID"                # the node UUID (== nd field)
BEARER="MINTED_BEARER_UUID"

curl -sS -X POST \
  -H "X-Netdata-Auth: Bearer $BEARER" \
  -H 'Content-Type: application/json' \
  "http://$HOST/host/$NODE/api/v3/function?function=systemd-journal" \
  -d '{"info":true,"timeout":30000}'
```

Notes:

- The header is **`X-Netdata-Auth: Bearer ...`**, NOT
  `Authorization: Bearer ...`. The agent rejects the latter for
  per-agent bearer auth.
- The agent's HTTP API path mirrors the Cloud-proxied path. For the
  Function payload shape (e.g. `systemd-journal` query body), see
  the matching guide in
  [`query-netdata-cloud`](../query-netdata-cloud/SKILL.md).

---

## Bearer cache and refresh

The shipped scripts cache bearers per `machine_guid` under
`<repo>/.local/audits/query-netdata-agents/bearers/<machine_guid>.json`
(gitignored, mode 0600). Each cache entry stores the raw mint
response.

Refresh policy: the cache is considered expired when
`expiration - now < 3600` (one-hour buffer before actual TTL).
Mirror of the Cloud frontend's policy
(`cloud-frontend/src/domains/nodes/useAgentBearer.js`).

A failed mint clears the cache entry so the next call re-mints from
scratch.

---

## Scripts

The reference implementation lives in
[`scripts/_lib.sh`](./scripts/_lib.sh). It exposes **token-safe
public wrappers** (the assistant never sees the cloud token,
agent bearer, or claim_id on stdout) and a **self-test** that
asserts no token bytes leak.

```bash
# In your script:
source "$(git rev-parse --show-toplevel)/.agents/skills/query-netdata-agents/scripts/_lib.sh"
agents_load_env

# Cloud-side call. NETDATA_CLOUD_TOKEN is read from .env
# internally; stdout is the response body only.
agents_query_cloud GET /api/v2/spaces

# Direct-agent call. The bearer is minted/cached/refreshed
# internally. stdout is the response body only; stderr shows the
# curl invocation with `<CLOUD_TOKEN>` and `<AGENT_BEARER>`
# masked.
agents_query_agent \
    --node "$AGENT_EVENTS_NODE_ID" \
    --host "$AGENT_EVENTS_HOSTNAME:19999" \
    --machine-guid "$AGENT_EVENTS_MACHINE_GUID" \
    POST '/api/v3/function?function=systemd-journal' '{"info":true}'

# Convenience: pick transport with --via cloud|agent.
agents_call_function \
    --via cloud \
    --node "$AGENT_EVENTS_NODE_ID" \
    --function systemd-journal
```

### Public API (assistant-facing)

| Function | Purpose |
|---|---|
| `agents_load_env` | Source `<repo>/.env`; validate required keys |
| `agents_repo_root` | Locate this repo's checkout root |
| `agents_audit_dir` | Create + return `<repo>/.local/audits/query-netdata-agents/` |
| `agents_netdata_prefix` | Autodetect Netdata install prefix (system / `/opt/netdata` / `/usr/local/netdata`) |
| `agents_query_cloud METHOD PATH [BODY]` | Call any Cloud REST endpoint. Auth is added internally. **Stdout = response body only.** |
| `agents_query_agent --node N --host H --machine-guid M METHOD PATH [BODY]` | Call any direct-agent path. Bearer resolved internally. **Stdout = response body only.** |
| `agents_call_function --via cloud\|agent --node N --function F [--body J]` | Convenience wrapper around the two above |
| `agents_run` / `agents_run_read` | Run curl with masked-token argv echo on stderr (used by the wrappers; rarely needed directly) |
| `agents_selftest_no_token_leak` | Self-test: drives the wrappers with a sentinel token and asserts the sentinel never reaches captured stdout |

### Internal helpers (do NOT call directly)

These start with `_` and operate on token bytes inside their own
scope. They return token data via bash namerefs (so the assistant
never sees them on stdout). Don't shell-out to them.

| Internal | Purpose |
|---|---|
| `_agents_resolve_bearer OUTVAR <node> <mg> <host>` | Cache-aware bearer resolution; writes the bearer into `$OUTVAR` |
| `_agents_get_claim_id OUTVAR <host>` | Resolve `claim_id` from `/api/v3/info`; writes to `$OUTVAR` |
| `_agents_mint_bearer_json <node> <mg> <claim>` | One-shot Cloud bearer mint; the caller MUST capture into a local |
| `_agents_log_masked` | Token / bearer redaction for stderr argv echoes |
| `_agents_exp_to_seconds` | Normalize Cloud `expiration` (sec or ms) to seconds |

---

## Direct-agent vs Cloud-proxied: how `agents_call_function` chooses

Default is `--via cloud` -- the safe choice for any team member.

`--via agent` requires:
1. The agent host is reachable from the workstation on port 19999.
2. A bearer (auto-minted internally via `_agents_resolve_bearer`).

Falls back to `--via cloud` if the direct call fails.

---

## Sensitive data

- Bearer values appear in script stderr only when masked.
- The cache file at `.local/audits/.../bearers/<machine_guid>.json`
  contains the raw bearer; mode 0600.
- Never paste bearer values, claim ids, machine GUIDs, or node UUIDs
  into committed files. See
  `<repo>/.agents/sow/specs/sensitive-data-discipline.md` for the
  full rule.
