# Transports

Three ways to query agent-events. The first two are scripted
in this skill; the third is operator-only.

## Priority order

1. **Cloud API** -- proxied through Netdata Cloud at the
   space hosting agent-events. **Default for the team.**
2. **Direct agent API** -- against the agent-events node's
   own HTTP. Used when bypassing the Cloud is acceptable
   (latency, debugging the proxy).
3. **ssh to the host** -- operator-only path. Costa-only.
   Mentioned here for completeness; this skill does NOT
   ship a scripted ssh transport.

## What each transport calls

All three speak the same `systemd-journal` Function. The
payload shape (`after`, `before`, `last`, `query`, `facets`,
`histogram`, `__logs_sources`, `selections`, ...) is identical
across transports. The Function payload is documented once at:

- `<repo>/docs/netdata-ai/skills/query-netdata-cloud/query-logs.md`

That doc is the canonical reference for:
- payload keys + types,
- the **`selections` multi-value field-filter** (AND across
  fields, OR across values) -- this skill leans on it heavily,
- the response envelope (top-level `data` rows, `columns` map,
  `facets`, `histogram`, etc.).

This skill EXTENDS that doc with agent-events specifics: which
AE_* fields are best as facets, what default `selections`
predicate to use, what `__logs_sources` value to set.

## Cloud API (transport 1)

### Endpoint

`POST https://${NETDATA_CLOUD_HOSTNAME}/api/v2/nodes/${AGENT_EVENTS_NODE_ID}/function?function=systemd-journal`

Auth: `Authorization: Bearer ${NETDATA_CLOUD_TOKEN}`.

### Helper (from query-netdata-cloud)

```bash
source "$(git rev-parse --show-toplevel)/.agents/skills/query-netdata-cloud/scripts/_lib.sh"
cloud_load_env
cloud_query \
  "/api/v2/nodes/${AGENT_EVENTS_NODE_ID}/function?function=systemd-journal" \
  "$PAYLOAD"
```

Or the `query-netdata-agents` skill's wrapper, which works
identically and routes through Cloud when configured:

```bash
source "$(git rev-parse --show-toplevel)/.agents/skills/query-netdata-agents/scripts/_lib.sh"
agents_load_env
agents_call_function "$AGENT_EVENTS_NODE_ID" systemd-journal "$PAYLOAD"
```

### Pros / cons

- **Pro:** team-accessible (no per-host SSH); central auth via
  `NETDATA_CLOUD_TOKEN`; works from any network.
- **Con:** slight latency vs direct agent; rate-limited at the
  Cloud edge; subject to Cloud-side query timeout.

### When to use

- The default for the team.
- Anything you want to share later (Cloud requests are
  loggable / repeatable).

## Direct agent API (transport 2)

### Endpoint

`POST http://${AGENT_EVENTS_HOSTNAME}:19999/api/v3/function?function=systemd-journal`

Auth: bearer token minted from the Cloud token. The
`agents_query_agent` helper handles minting + caching
transparently.

### Helper

```bash
source "$(git rev-parse --show-toplevel)/.agents/skills/query-netdata-agents/scripts/_lib.sh"
agents_load_env

agents_query_agent \
    --node "$AGENT_EVENTS_NODE_ID" \
    --host "$AGENT_EVENTS_HOSTNAME:19999" \
    --machine-guid "$AGENT_EVENTS_MACHINE_GUID" \
    POST '/api/v3/function?function=systemd-journal' "$PAYLOAD"
```

Output is the response body only. The bearer never reaches
the assistant's captured stdout.

### Pros / cons

- **Pro:** no Cloud-edge round-trip; lower latency; agent's
  own timeout (you set it in the body).
- **Con:** only reachable from inside the network; requires
  the agent to be reachable on port 19999.

### When to use

- Tight loops during local debugging (sub-second iteration).
- When the Cloud edge is the bottleneck.

## ssh to the host (transport 3 -- operator-only)

This skill does NOT ship a scripted ssh transport. The
operator (Costa) sometimes runs `journalctl` directly on the
host:

```bash
ssh "$AGENT_EVENTS_HOSTNAME" \
  sudo /usr/bin/journalctl --namespace=agent-events \
    --since '24 hours ago' -o json
```

Notes:
- The ssh host is `${AGENT_EVENTS_HOSTNAME}` (env-keyed; can be
  an IP or DNS name). The journal namespace is `agent-events`
  (hardcoded constant, set on the ingestion server's log2journal
  invocation, NOT a function of the hostname).
- Raw `journalctl` does NOT support multi-value field filters
  (they are a Netdata-engine feature, not journald). If you
  need AND-of-OR filtering, use transport 1 or 2.
- This path requires sudo + a member of the `systemd-journal`
  group on the ingestion host. Most team members do not have
  this. Use transports 1 or 2 instead.

## Default `__logs_sources` value

Always set `__logs_sources` to the agent-events namespace name
(`"agent-events"` -- a hardcoded constant set on the ingestion
server's log2journal invocation; NOT derived from
`${AGENT_EVENTS_HOSTNAME}`):

```json
{ "__logs_sources": "agent-events" }
```

Without this, the Function defaults to all-local-logs on the
ingestion-server agent -- which is huge and unrelated.

## What goes in `selections` for agent-events

For the agent-events namespace, the most-useful index-friendly
predicates (always present on every record):

- `AE_VERSION` -- schema version anchor (always 28+).
- `AE_AGENT_HEALTH` -- crash class (filter to `crash-*` for crashes).
- `AE_EXIT_CAUSE` -- exit reason (filter to specific causes).
- `AE_AGENT_VERSION` -- producing agent version (regression slicing).
- `AE_FATAL_SIGNAL_CODE` -- non-empty for signal crashes.
- `AE_FATAL_FUNCTION` / `AE_FATAL_FILENAME` -- localize to a
  function or file.
- `AE_HOST_ARCHITECTURE` / `AE_OS_FAMILY` / `AE_AGENT_INSTALL_TYPE`
  -- arch / distro / packaging slicers.
- `AE_AGENT_PROFILE_0` -- standalone / parent / child / iot.
- `AE_AGENT_KUBERNETES` -- k8s-specific.
- `AE_AGENT_ACLK` -- cloud-claimed vs not.

See `AE_FIELDS.md` for the full field map and enum meanings.

## See also

- `<repo>/docs/netdata-ai/skills/query-netdata-cloud/query-logs.md`
  -- canonical Function payload shape and the `selections`
  multi-value filter section.
- `<repo>/docs/netdata-ai/skills/query-netdata-agents/query-logs.md`
  -- direct-agent transport details.
- `query-discipline.md` (this skill) -- how to compose
  index-friendly queries against agent-events.
- `update-cadence.md` (this skill) -- when events arrive and why.
