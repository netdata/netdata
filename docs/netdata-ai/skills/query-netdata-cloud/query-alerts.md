# Query Netdata alerts via Netdata Cloud

This guide is part of the [`query-netdata-cloud`](./SKILL.md) skill.
Read the [SKILL.md prerequisites](./SKILL.md#prerequisites) first.

Alerts are exposed as **REST endpoints** -- not as Functions. Both
Netdata Cloud and the Netdata Agent expose dedicated alert paths.
Use the Cloud-proxied paths by default (no per-agent bearer needed).
Use the agent-direct paths when you need single-host detail or when
Cloud is unavailable (see the sibling
[`query-netdata-agents`](../query-netdata-agents/SKILL.md) skill for
direct-agent auth).

---

## Mandatory Requirements (READ FIRST)

1. **Provide actionable instructions.** Every recommendation ends in
   a runnable curl command.
2. **Never request credentials.** Use `YOUR_API_TOKEN`,
   `YOUR_SPACE_ID`, `YOUR_ROOM_ID` placeholders.
3. **Always include a heredoc body.** Avoids quote-escaping pain.
4. **Cloud and agent endpoints have different shapes.** Cloud
   endpoints aggregate across nodes in a room/space. Agent
   endpoints serve a single host. Pick the one that matches the
   question.

---

## Cloud-side endpoints

Base URL: `https://app.netdata.cloud`. All require
`Authorization: Bearer YOUR_API_TOKEN` and the
`PermissionAlertReadAll` role on the target space (notification
silencing endpoints require write permission).

### Current alerts in a room

`POST /api/v2/spaces/{spaceID}/rooms/{roomID}/alerts`

```bash
TOKEN="YOUR_API_TOKEN"
SPACE="YOUR_SPACE_ID"
ROOM="YOUR_ROOM_ID"

read -r -d '' PAYLOAD <<'EOF'
{
  "options": ["instances", "values", "summary", "config"]
}
EOF

curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/spaces/$SPACE/rooms/$ROOM/alerts" \
  -d "$PAYLOAD"
```

Body accepts optional filters: `status[]` (`CRITICAL`, `WARNING`,
`CLEAR`, etc.), `name` pattern, `alarm_id_filter`, pagination
(`offset`, `limit`), and a time window. Without
`options.instances` the per-instance array is empty -- only the
aggregated `alerts[]` summary is returned.

Response top-level: `api`, `alerts[]` (one entry per template),
`alert_instances[]` (one entry per running instance, when
requested), `nodes[]`, `timings`. Per-instance compact fields
(verified live):

| Field | Meaning |
|---|---|
| `nm` | Alert name (e.g. `10min_cpu_iowait`) |
| `ctx` | Context (e.g. `system.cpu`) |
| `ch` / `ch_n` | Chart id / name |
| `st` | Current status (`CRITICAL`, `WARNING`, `CLEAR`, ...) |
| `v` | Current value |
| `t` | Last evaluation timestamp (Unix seconds) |
| `tr_i` | Last transition id (UUID) |
| `tr_v` | Value at last transition |
| `tr_t` | Timestamp of last transition |
| `units` | Unit string |
| `cfg` | **Config hash UUID** -- pass to `/alert_config` as `config` |
| `exec` | Notification executable |
| `tp` / `cl` / `cp` | Type / classification / component |
| `to` | Notification role(s) |

### Space-wide alarm stats

`GET /api/v2/spaces/{spaceID}/alarms`

```bash
TOKEN="YOUR_API_TOKEN"
SPACE="YOUR_SPACE_ID"

curl -sS \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/spaces/$SPACE/alarms"
```

Returns total counts (`critical`, `warning`, `clear`, `silenced`)
across all rooms in the space. Use to drive a dashboard summary.

### Available alert templates / metas

`GET /api/v2/spaces/{spaceID}/alarms/metas`

```bash
TOKEN="YOUR_API_TOKEN"
SPACE="YOUR_SPACE_ID"

curl -sS \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/spaces/$SPACE/alarms/metas"
```

Lists every alert template/prototype configured across the space:
names, contexts, severities, available config hashes. Use this to
discover what alerts exist before drilling into a specific one.

### Per-room alert summary stats

`GET /api/v2/spaces/{spaceID}/rooms/{roomID}/alerts_stats`

```bash
TOKEN="YOUR_API_TOKEN"
SPACE="YOUR_SPACE_ID"
ROOM="YOUR_ROOM_ID"

curl -sS \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/spaces/$SPACE/rooms/$ROOM/alerts_stats"
```

Same shape as `/alarms` but scoped to one room. Optional
node-filter query params.

### Misconfigured alerts

`POST /api/v2/spaces/{spaceID}/rooms/{roomID}/alerts:misconfigured`

```bash
TOKEN="YOUR_API_TOKEN"
SPACE="YOUR_SPACE_ID"
ROOM="YOUR_ROOM_ID"

read -r -d '' PAYLOAD <<'EOF'
{
  "categories": ["firing_often", "stuck_raised", "silenced_long", "dispatch_none"],
  "thresholds": {
    "firing_often_min_count": 10,
    "stuck_raised_min_hours": 24
  }
}
EOF

curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/spaces/$SPACE/rooms/$ROOM/alerts:misconfigured" \
  -d "$PAYLOAD"
```

Categories: `firing_often`, `stuck_raised`, `silenced_long`,
`dispatch_none`. Returns alerts grouped by category with metrics so
you can clean up noisy or broken alert configurations.

### Alert state transitions (history)

`POST /api/v2/spaces/{spaceID}/rooms/{roomID}/alert_transitions`

```bash
TOKEN="YOUR_API_TOKEN"
SPACE="YOUR_SPACE_ID"
ROOM="YOUR_ROOM_ID"
# absolute Unix seconds; the endpoint rejects negative or 0 values.
AFTER=$(( $(date +%s) - 86400 ))

read -r -d '' PAYLOAD <<EOF
{
  "after":  ${AFTER},
  "before": $(date +%s),
  "status": ["CRITICAL", "WARNING"]
}
EOF

curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/spaces/$SPACE/rooms/$ROOM/alert_transitions" \
  -d "$PAYLOAD"
```

`after` must be **absolute Unix seconds > 0** (verified live; the
endpoint returns
`{"errorMsgKey":"ErrBadRequest","errorMessage":"after parameter must be greater than 0",...}`
otherwise). `before` is also Unix seconds (`0` is rejected; pass
`now` or omit). With an empty body `{}` the endpoint applies its
own default lookback.

Optional filters: `status[]` (`CRITICAL`, `WARNING`, `CLEAR`, ...),
`alert_names[]`, `node_ids[]`, `context[]`, plus pagination
(`limit`, `last`).

Response top-level: `api`, `transitions[]`. Each transition record:
`transition_id`, `node_id`, `name`/`alert`, `instance`, `context`,
`when` (unix-seconds), `new` / `old` (`{status, value}`), `summary`,
`info`, `src`, `config_hash_id`, `component`, `classification`,
`to`, `units`, `exec`.

### Single alert configuration

`POST /api/v2/spaces/{spaceID}/rooms/{roomID}/alert_config`

```bash
TOKEN="YOUR_API_TOKEN"
SPACE="YOUR_SPACE_ID"
ROOM="YOUR_ROOM_ID"

read -r -d '' PAYLOAD <<'EOF'
{
  "config":  "ALERT_CONFIG_HASH_UUID",
  "node_id": "YOUR_NODE_UUID"
}
EOF

curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/spaces/$SPACE/rooms/$ROOM/alert_config" \
  -d "$PAYLOAD"
```

`config` is the hash UUID from the `cfg` field of an alert
instance in the `/alerts` response (request
`options:["instances","config"]` there to get it populated).
Returns the full alert definition: top-level keys `name`, `info`,
`class`, `component`, `selectors`, `status`, `notification`,
`config_hash_id` (echo of input).

### Evaluate an alert config against historical data

`POST /api/v2/spaces/{spaceID}/rooms/{roomID}/alert_config/evaluate`

```bash
TOKEN="YOUR_API_TOKEN"
SPACE="YOUR_SPACE_ID"
ROOM="YOUR_ROOM_ID"

read -r -d '' PAYLOAD <<'EOF'
{
  "node_id": "YOUR_NODE_UUID",
  "config":  "alarm: example_high_cpu\n on: system.cpu\n lookup: average -1m of user\n warn: $this > 70\n crit: $this > 90\n",
  "after":   -3600,
  "before":  0
}
EOF

curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/spaces/$SPACE/rooms/$ROOM/alert_config/evaluate" \
  -d "$PAYLOAD"
```

Replays the alert definition against real metric data over the
window. Useful for tuning before deployment. Returns evaluation
results showing what the alert would have done.

### AI-assisted alert config generation

Three companion endpoints that take a context/metric and either
generate, suggest, or explain an alert configuration. All three are
`POST` under `/api/v2/spaces/{spaceID}/alert-config/...`:

| Endpoint | Purpose |
|---|---|
| `/alert-config/generate` | Produce a full config from a context+metric description |
| `/alert-config/suggest` | Suggest several config variants |
| `/alert-config/explain` | Explain in prose what an existing config does |

```bash
TOKEN="YOUR_API_TOKEN"
SPACE="YOUR_SPACE_ID"

read -r -d '' PAYLOAD <<'EOF'
{
  "context":  "system.cpu",
  "instance": "system",
  "metric":   "user"
}
EOF

curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/spaces/$SPACE/alert-config/generate" \
  -d "$PAYLOAD"
```

### Notification silencing rules

Silencing rules are Cloud-only (the agent has no silencing REST
API). Five endpoints, all under
`/api/v2/spaces/{spaceID}/notifications/silencing/`:

| Path | Method | Purpose |
|---|---|---|
| `rules` | GET | List all silencing rules in the space (state: `INACTIVE`, `ACTIVE`, `SCHEDULED`) |
| `rule` | POST | Create a rule |
| `rule/{ruleID}` | PUT | Update a rule |
| `rules/delete` | POST | Bulk-delete rules by ID list |
| `rrule/evaluate` | POST | Evaluate an iCal-style RRULE recurrence expression |

```bash
TOKEN="YOUR_API_TOKEN"
SPACE="YOUR_SPACE_ID"

# List all silencing rules.
curl -sS \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/spaces/$SPACE/notifications/silencing/rules"
```

Create-rule body:

```bash
TOKEN="YOUR_API_TOKEN"
SPACE="YOUR_SPACE_ID"

read -r -d '' PAYLOAD <<'EOF'
{
  "name":            "Maintenance window for db cluster",
  "room_ids":        ["YOUR_ROOM_ID"],
  "node_ids":        [],
  "host_labels":     { "role": "database" },
  "alert_names":     [],
  "alert_contexts":  ["disk.space"],
  "severities":      ["WARNING", "CRITICAL"],
  "starts_at":       1700000000,
  "lasts_until":     1700003600,
  "rrule":           ""
}
EOF

curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/spaces/$SPACE/notifications/silencing/rule" \
  -d "$PAYLOAD"
```

`rrule` is an iCalendar RFC 5545 recurrence string (e.g.
`FREQ=WEEKLY;BYDAY=SA,SU`). Use `rrule/evaluate` first to confirm
the schedule before creating.

---

## Direct-agent fallback (single-host alerts)

When you need detail for a specific host or Cloud is unavailable,
talk to the agent directly. All paths below are reachable at
`http://<agent>:19999/host/<node-uuid>` and require a per-agent
bearer if the agent is bearer-protected (see
[query-netdata-agents](../query-netdata-agents/SKILL.md) for the
mint flow).

### Multi-status alerts (preferred -- agent v3)

`POST /api/v3/alerts`

```bash
HOST="agent.example:19999"
NODE="YOUR_NODE_UUID"
BEARER="MINTED_AGENT_BEARER"

read -r -d '' PAYLOAD <<'EOF'
{
  "options": ["summary", "values", "instances"]
}
EOF

curl -sS -X POST \
  -H "X-Netdata-Auth: Bearer $BEARER" \
  -H 'Content-Type: application/json' \
  "http://$HOST/host/$NODE/api/v3/alerts" \
  -d "$PAYLOAD"
```

Same body fields as the Cloud-proxied `/alerts` endpoint
(`status[]`, `name`, time range, options). Response is a
single-node alert table. The handler at
`<repo>/src/web/api/v2/api_v2_alerts.c` is shared with `/api/v2/alerts`
(use v2 only on older agents that lack v3).

### Alert transitions on a single agent (agent v3)

`POST /api/v3/alert_transitions`

Same body shape as the Cloud transitions endpoint; result is
single-host. Shared handler with `/api/v2/alert_transitions`; use
v3 by default.

### Single alert config on a single agent (agent v3)

`GET /api/v3/alert_config?config=CONFIG_HASH_UUID`

```bash
HOST="agent.example:19999"
NODE="YOUR_NODE_UUID"
BEARER="MINTED_AGENT_BEARER"
CFG="ALERT_CONFIG_HASH_UUID"   # the cfg field of an alert instance

curl -sS \
  -H "X-Netdata-Auth: Bearer $BEARER" \
  "http://$HOST/host/$NODE/api/v3/alert_config?config=$CFG"
```

`config` is the hash UUID (the `cfg` field of an alert instance).
The Cloud endpoint above points to the same data; use this only
for direct-agent workflows. Response top-level keys verified live:
`name`, `info`, `class`, `component`, `selectors`, `status`,
`notification`, `config_hash_id`. Shared handler with v2; v3 is
the default.

### Legacy v1 alarm endpoints (use only on pre-v2 agents)

These remain only for agents older than v1.40 that have no v2/v3
alert endpoints. On any modern agent, use the v3 endpoints above.

| Path | Method | Purpose |
|---|---|---|
| `/api/v1/alarms` | GET | Active alarms; query `?all=true` for inactive too |
| `/api/v1/alarms_values` | GET | Numeric state per alarm |
| `/api/v1/alarm_log` | GET | History; `?after=<unix-seconds>&chart=<name>` |
| `/api/v1/alarm_count` | GET | Count by status; `?status=CRITICAL&context=<name>` |
| `/api/v1/alarm_variables` | GET | Per-chart alert variables; `?chart=<name>` (required) |
| `/api/v1/variable` | GET | Single variable lookup; `?chart=<name>&variable=<name>` |

```bash
HOST="agent.example:19999"
NODE="YOUR_NODE_UUID"
BEARER="MINTED_AGENT_BEARER"

# Active alarms only
curl -sS \
  -H "X-Netdata-Auth: Bearer $BEARER" \
  "http://$HOST/host/$NODE/api/v1/alarms"

# Alarm transition history since a given timestamp
curl -sS \
  -H "X-Netdata-Auth: Bearer $BEARER" \
  "http://$HOST/host/$NODE/api/v1/alarm_log?after=1700000000"
```

Migration: `/api/v1/alarms` -> `/api/v2/alerts`,
`/api/v1/alarm_log` -> `/api/v2/alert_transitions`.

---

## Question-to-endpoint cheatsheet

| Question | Cloud | Agent direct |
|---|---|---|
| What alerts are firing across the room? | `POST /api/v2/spaces/{sp}/rooms/{rm}/alerts` | `POST /host/{node}/api/v3/alerts` |
| What alerts are firing across the entire space? | `GET /api/v2/spaces/{sp}/alarms` | (run per-room) |
| Which alert templates are configured? | `GET /api/v2/spaces/{sp}/alarms/metas` | (per-host config inspection) |
| Show alert state transitions over the last 24h | `POST /api/v2/spaces/{sp}/rooms/{rm}/alert_transitions` body `{after:<unix-s>,before:<unix-s>,...}` | `POST /host/{node}/api/v3/alert_transitions` |
| Get the full configuration of a specific alert | `POST /api/v2/spaces/{sp}/rooms/{rm}/alert_config` body `{config,node_id}` | `GET /host/{node}/api/v3/alert_config?config=...` |
| Evaluate a candidate alert config against history | `POST /api/v2/spaces/{sp}/rooms/{rm}/alert_config/evaluate` | not available (Cloud-only) |
| Generate / suggest / explain an alert config | `POST /api/v2/spaces/{sp}/alert-config/{generate,suggest,explain}` | not available (Cloud-only) |
| Which alerts are misconfigured (firing-often, stuck-raised, silenced-long, dispatch-none)? | `POST /api/v2/spaces/{sp}/rooms/{rm}/alerts:misconfigured` | not available (Cloud-only) |
| What silencing rules are active or scheduled? | `GET /api/v2/spaces/{sp}/notifications/silencing/rules` | not available (Cloud-only) |
| Create / update / delete a silencing rule | `POST/PUT/DELETE /api/v2/spaces/{sp}/notifications/silencing/rule[s]/...` | not available (Cloud-only) |
| Reload alert definitions on the agent | not exposed via REST | not exposed via REST -- use SIGHUP or dyncfg |

---

## Limits and gotchas

- **`PermissionAlertReadAll` is required** for all alert reads --
  `scope:all` tokens have it; `scope:grafana-plugin` tokens do
  NOT. If you get HTTP 403, mint a wider-scoped token.
- **Silencing rules are Cloud-only.** The agent's internal
  `SILENCER` structures are not REST-addressable. There is no
  `/api/v[123]/silencers` on the agent.
- **No REST endpoint for "reload alert configs"** on either side.
  The agent reloads on `SIGHUP` or via the dyncfg callback at
  `src/health/health_dyncfg.c`. For programmatic config changes,
  push files to `etc/netdata/health.d/` and signal the agent.
- **`config_hash_id` is required for `/alert_config`** on both
  sides. Get it from the alert metadata (`/alerts` response,
  `config_hash_id` field, or `/alarms/metas` for templates).
- **Agent-direct paths return single-host data.** For aggregated
  cross-room/cross-space queries, you must use Cloud or aggregate
  agent responses client-side.
- **`alert_transitions` time bounds are seconds, NOT
  milliseconds.** Negative values are relative offsets from "now".
  This differs from `systemd-journal` time bounds (microseconds).
