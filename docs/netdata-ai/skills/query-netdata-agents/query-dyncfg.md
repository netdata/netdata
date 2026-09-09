# Query agent DynCfg directly

This guide is part of the [`query-netdata-agents`](./SKILL.md) skill.
Read [SKILL.md](./SKILL.md#prerequisites) first.

For the full DynCfg surface (actions, id structure,
templates/jobs, source types, response codes, schema flow), see
[../query-netdata-cloud/query-dyncfg.md](../query-netdata-cloud/query-dyncfg.md).

Direct-agent uses the same query parameters and the same payloads
as the Cloud-proxied path. The agent's handler at
`<repo>/src/web/api/v1/api_v1_config.c` is the canonical
implementation; both `/api/v1/config` and `/api/v3/config` route
to it. Prefer v3.

---

## Endpoint (agent v3)

| Method | Path | Purpose |
|---|---|---|
| `GET`  | `/api/v3/config?action=tree&path=/` | List configuration objects |
| `GET`  | `/api/v3/config?action=<read>&id=<id>[&name=<name>]` | Read a configuration |
| `POST` | `/api/v3/config?action=<write>&id=<id>[&name=<name>]` | Mutate a configuration (body = config JSON) |

## Use the wrapper

```bash
source "$(git rev-parse --show-toplevel)/.agents/skills/query-netdata-agents/scripts/_lib.sh"
agents_load_env

# List all configuration objects.
agents_query_agent \
    --node    "$NODE_UUID" \
    --host    "$AGENT_HOST:19999" \
    --machine-guid "$AGENT_MG" \
    GET '/api/v3/config?action=tree&path=/'

# Get a specific alert prototype's JSON Schema.
ID='health:alert:prototype:ram_usage'
agents_query_agent \
    --node    "$NODE_UUID" \
    --host    "$AGENT_HOST:19999" \
    --machine-guid "$AGENT_MG" \
    GET "/api/v3/config?action=schema&id=$(printf %s "$ID" | jq -sRr @uri)"

# Add a new go.d.plugin nginx job.
TPL='go.d:nginx'; JOB='local_server'
agents_query_agent \
    --node    "$NODE_UUID" \
    --host    "$AGENT_HOST:19999" \
    --machine-guid "$AGENT_MG" \
    POST "/api/v3/config?action=add&id=$(printf %s "$TPL" | jq -sRr @uri)&name=$JOB" \
    '{"url":"http://127.0.0.1/stub_status","update_every":5}'
```

The wrapper handles the bearer internally; for write actions
(`POST`), Cloud requires `PermissionFunctionExecPrivileged`. Direct-
agent requires the bearer to grant similar access at the agent
level.

## When direct-agent is best

- **Bulk schema fetches.** `tree` -> `schema` for many ids is
  faster direct (skip the Cloud round-trip).
- **`update`/`test` write workflows where you want immediate
  feedback** without going through Cloud's permission gate.

## Limits and gotchas

- **`/api/v1/config` is the legacy alias** -- use `/api/v3/config`.
- **id encoding**: ids contain colons; URL-encode them with
  `printf %s "$ID" | jq -sRr @uri` to avoid breaking on
  special characters.
- **`action=test`** without a `name` falls back to a derived name
  (the part after the last colon in the id) for backwards
  compatibility -- best practice is to supply `name` explicitly.

## See also

- [../query-netdata-cloud/query-dyncfg.md](../query-netdata-cloud/query-dyncfg.md)
  -- full reference (actions, id structure, response codes).
- `<repo>/src/daemon/dyncfg/README.md` -- internal DynCfg API.
- `<repo>/src/plugins.d/DYNCFG.md` -- external-plugin DynCfg
  protocol.
