# Query agent alerts directly

This guide is part of the [`query-netdata-agents`](./SKILL.md) skill.
Read [SKILL.md](./SKILL.md#prerequisites) first.

For the alert REST surface (current alerts, transitions, single
config, the `cfg` field of an alert instance, the `options[]`
array on `/alerts`), see
[../query-netdata-cloud/query-alerts.md](../query-netdata-cloud/query-alerts.md).
The body and response of the agent-direct paths are identical to
the per-agent rows of the Cloud-proxied responses.

Three v3 paths are available on the agent (prefer v3; v2 shares
the same handler; v1 `/alarms*` only on pre-v2 agents):

| Path | Method | Purpose |
|---|---|---|
| `/api/v3/alerts` | POST | Current alerts on this host |
| `/api/v3/alert_transitions` | POST | Alert transition history |
| `/api/v3/alert_config?config=<hash>` | GET | Full configuration of one alert |

Silencing rules, `alerts:misconfigured`, alert-config
generate/suggest/explain, and `alarms`/`alarms/metas` are
**Cloud-only** -- there is no agent-direct equivalent. See
[../query-netdata-cloud/query-alerts.md](../query-netdata-cloud/query-alerts.md).

---

## Use the wrapper

```bash
source "$(git rev-parse --show-toplevel)/.agents/skills/query-netdata-agents/scripts/_lib.sh"
agents_load_env

# Currently-firing alerts on this host with summary + values + instances.
agents_query_agent \
    --node    "$NODE_UUID" \
    --host    "$AGENT_HOST:19999" \
    --machine-guid "$AGENT_MG" \
    POST /api/v3/alerts '{"options":["summary","values","instances"]}'

# Transitions over the last hour. NOTE: agent /alert_transitions
# accepts negative `after` values too (different from cloud).
AFTER=$(( $(date +%s) - 3600 ))
NOW=$(date +%s)
agents_query_agent \
    --node    "$NODE_UUID" \
    --host    "$AGENT_HOST:19999" \
    --machine-guid "$AGENT_MG" \
    POST /api/v3/alert_transitions "{\"after\":$AFTER,\"before\":$NOW}"

# Read a specific alert's config. Get the hash from .alert_instances[].cfg.
CFG="ALERT_CONFIG_HASH_UUID"   # the cfg field of an alert instance
agents_query_agent \
    --node    "$NODE_UUID" \
    --host    "$AGENT_HOST:19999" \
    --machine-guid "$AGENT_MG" \
    GET "/api/v3/alert_config?config=$CFG"
```

## Limits and gotchas

- **Single host.** Aggregation across nodes is Cloud's job.
- **Compact field names**: `cfg`, `nm`, `ctx`, `st`, `tr_i`,
  `tr_v`, `tr_t`, `cl`, `cp`, `tp`, `to`. Spelled-out
  cross-reference is in the cloud guide.
- **`config_hash_id` source**: the `cfg` field on an alert
  instance from `/api/v3/alerts` (with `options:["instances"]`)
  is what to pass as `config=` to `/alert_config`.

## See also

- [../query-netdata-cloud/query-alerts.md](../query-netdata-cloud/query-alerts.md)
  -- full alerts surface (Cloud + agent), including everything
  Cloud-only (silencing, generate/suggest/explain, etc.).
