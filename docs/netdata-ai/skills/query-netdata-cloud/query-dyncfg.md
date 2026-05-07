# Query Netdata Dynamic Configuration (DynCfg)

This guide is part of the [`query-netdata-cloud`](./SKILL.md) skill.
Read the [SKILL.md prerequisites](./SKILL.md#prerequisites) first.

DynCfg is Netdata's dynamic-configuration system. Every plugin /
collector / module that participates registers configuration
objects which the user can list, view, edit, enable, disable, add,
remove, test, restart, and export -- through a single REST surface
distinct from the Function-call surface. Internally DynCfg is
implemented on top of Functions, but it has its own dedicated API
endpoint. Don't confuse the two: function-call paths
(`/api/v2/nodes/{nodeID}/function?function=...`) DO NOT operate on
configuration.

**Canonical references** (authoritative; read these for full detail):

| File | What it covers |
|---|---|
| `<repo>/src/daemon/dyncfg/README.md` | Internal-plugin DynCfg API and lifecycle (high-level + low-level) |
| `<repo>/src/plugins.d/DYNCFG.md` | External-plugin (go.d.plugin, etc.) DynCfg protocol |

---

## Mandatory Requirements (READ FIRST)

1. **Provide actionable instructions.** Each answer ends in a
   runnable curl command.
2. **Never request credentials.** Use `YOUR_API_TOKEN`,
   `YOUR_NODE_UUID`, etc. placeholders.
3. **Read actions are GET; write actions are POST.** Cloud's
   permission gate is `PermissionFunctionExecPrivileged` for the
   write paths. A read-only token cannot mutate configuration.
4. **The `action=tree` listing is the entry point.** Always start
   there; the IDs returned are the inputs to all other actions.

---

## Endpoints

### Cloud-proxied (preferred)

| Method | Path | Purpose | Permission |
|---|---|---|---|
| `GET` | `/api/v2/nodes/{nodeID}/config?action=tree&path=/` | List configuration objects | `nodeAuth()` |
| `GET` | `/api/v2/nodes/{nodeID}/config?action=<read-action>&id=<id>` | Read one configuration | `nodeAuth()` |
| `POST` | `/api/v2/nodes/{nodeID}/config?action=<write-action>&id=<id>[&name=<name>]` | Mutate configuration (body = the configuration JSON) | `PermissionFunctionExecPrivileged` |

Cloud-side route registration:
`cloud-charts-service/http/http.go:150-151`.

### Direct-agent (fallback)

Same shape, served by the agent itself:
`http://<agent>:19999/host/{nodeID}/api/v3/config?...`. The agent
also accepts `/api/v1/config` for backwards compatibility -- both
paths use the same handler at
`<repo>/src/web/api/v1/api_v1_config.c`.

For direct-agent calls, `X-Netdata-Auth: Bearer <agent-bearer>` is
required when the agent is bearer-protected. See the
[`query-netdata-agents`](../query-netdata-agents/SKILL.md) skill
for the bearer mint/cache flow.

---

## Query parameters

The handler at `<repo>/src/web/api/v1/api_v1_config.c:5-80` accepts
these query parameters:

| Param | Required for | Purpose |
|---|---|---|
| `action` | every call (defaults to `tree`) | Which DynCfg command to execute |
| `path` | `tree` | Path within the configuration tree (e.g. `/`, `/health/alerts/prototypes`) |
| `id` | every action except `tree` | Configuration object id (colon-separated, e.g. `health:alert:prototype:ram_usage`) |
| `name` | `add`, `userconfig`, `test` | Job/object name when adding to a template, getting user-config, or testing |
| `timeout` | optional (default 120) | Operation timeout in seconds; minimum 10 |

The body of a `POST` carries the **payload** for write actions
(the configuration JSON to apply, or the test input for `test`).

---

## DynCfg actions

From `<repo>/src/daemon/dyncfg/README.md` and the agent's command
enum:

| `action` value | Method | Purpose |
|---|---|---|
| `tree` | GET | List configuration objects under `path` |
| `schema` | GET | JSON Schema for a configuration object |
| `get` | GET | Current value of a configuration object |
| `userconfig` | GET | Configuration in user-friendly form (used for conf files); requires `name` |
| `update` | POST | Replace a configuration object's value (body = new JSON) |
| `add` | POST | Add a new job to a template; requires `name` |
| `remove` | POST | Remove a `dyncfg`-source job (cannot remove user-file jobs) |
| `enable` | POST | Enable a configuration object |
| `disable` | POST | Disable a configuration object |
| `test` | POST | Test a configuration without applying it; requires `name`; body = the candidate JSON |
| `restart` | POST | Restart the configuration / re-apply |

### DynCfg response codes

DynCfg uses HTTP-like codes verified against
`<repo>/src/daemon/dyncfg/README.md`:

| Code | Meaning |
|---|---|
| 200 | Running -- accepted and active |
| 202 | Accepted -- queued, not yet running |
| 298 | Accepted but disabled |
| 299 | Accepted but restart required |
| 400 | Bad request / invalid configuration |
| 404 | Configuration id not found |
| 500 | Internal error |
| 501 | Action not implemented for this object |

---

## Configuration ID structure

Configuration IDs follow a colon-separated hierarchy
(`<repo>/src/daemon/dyncfg/README.md`):

```
component:category:name
component:template_name:job_name
```

Examples (verified live -- agent-events node returns these under
`tree`):

- `/collectors/go.d/Jobs` -- the go.d.plugin Jobs tree
- `/collectors/go.d/ServiceDiscovery`
- `/collectors/go.d/Vnodes`
- `/collectors/ibm.d/Jobs`
- `/collectors/ibm.d/Vnodes`
- `/health/alerts/prototypes` -- alert prototypes
- `/logs/systemd-journal` -- systemd-journal collector configs

Each tree entry contains configuration objects with their own ids
(e.g. `health:alert:prototype:ram_usage`,
`go.d:nginx:local_server`).

### Templates vs Jobs

- **Template id**: `component:template_name`. Templates DEFINE the
  schema for jobs. They cannot be `update`d but can be `add`'d to.
- **Job id**: `component:template_name:job_name`. The portion
  before the last colon must match an existing template id.
- **Single id**: `component:name`. A standalone configuration
  object that is neither template nor job.

---

## Examples

### Example 1: list every configuration object on a node

```bash
TOKEN="YOUR_API_TOKEN"
NODE="YOUR_NODE_UUID"

curl -sS \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/nodes/$NODE/config?action=tree&path=/"
```

Response top level (verified live): `{agent, attention, tree,
version}`. The `tree` is keyed by configuration path; each entry
lists per-object ids, types, statuses, supported commands, and
sources.

### Example 2: get the JSON Schema for an alert prototype

```bash
TOKEN="YOUR_API_TOKEN"
NODE="YOUR_NODE_UUID"
ID="health:alert:prototype:ram_usage"

curl -sS \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/nodes/$NODE/config?action=schema&id=$(printf %s "$ID" | jq -sRr @uri)"
```

The schema describes which fields the configuration accepts and is
the basis for any UI form. Use this before constructing an
`update` payload.

### Example 3: read the current value of a configuration

```bash
TOKEN="YOUR_API_TOKEN"
NODE="YOUR_NODE_UUID"
ID="health:alert:prototype:ram_usage"

curl -sS \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/nodes/$NODE/config?action=get&id=$(printf %s "$ID" | jq -sRr @uri)"
```

### Example 4: add a new go.d.plugin job to a template

```bash
TOKEN="YOUR_API_TOKEN"
NODE="YOUR_NODE_UUID"
TPL_ID="go.d:nginx"
JOB_NAME="local_server"

read -r -d '' PAYLOAD <<'EOF'
{
  "url":             "http://127.0.0.1:80/stub_status",
  "update_every":    5,
  "timeout":         2
}
EOF

curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/nodes/$NODE/config?action=add&id=$(printf %s "$TPL_ID" | jq -sRr @uri)&name=$JOB_NAME" \
  -d "$PAYLOAD"
```

### Example 5: test a configuration without applying it

```bash
TOKEN="YOUR_API_TOKEN"
NODE="YOUR_NODE_UUID"
TPL_ID="go.d:nginx"
JOB_NAME="local_server"

read -r -d '' PAYLOAD <<'EOF'
{
  "url":          "http://127.0.0.1:80/stub_status",
  "update_every": 5
}
EOF

curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/nodes/$NODE/config?action=test&id=$(printf %s "$TPL_ID" | jq -sRr @uri)&name=$JOB_NAME" \
  -d "$PAYLOAD"
```

A successful response (200/202) means the configuration would
work; the test does NOT make the change persistent.

### Example 6: enable / disable a configuration

```bash
TOKEN="YOUR_API_TOKEN"
NODE="YOUR_NODE_UUID"
ID="go.d:nginx:local_server"

# Enable
curl -sS -X POST \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/nodes/$NODE/config?action=enable&id=$(printf %s "$ID" | jq -sRr @uri)"

# Disable
curl -sS -X POST \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/nodes/$NODE/config?action=disable&id=$(printf %s "$ID" | jq -sRr @uri)"
```

### Example 7: remove a dyncfg-created job

```bash
TOKEN="YOUR_API_TOKEN"
NODE="YOUR_NODE_UUID"
ID="go.d:nginx:local_server"

curl -sS -X POST \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/nodes/$NODE/config?action=remove&id=$(printf %s "$ID" | jq -sRr @uri)"
```

Only objects whose `source_type == DYNCFG_SOURCE_TYPE_DYNCFG` (i.e.
created via DynCfg, not from user files or internally) can be
removed.

### Example 8: get a job's user-friendly configuration (for conf-file export)

```bash
TOKEN="YOUR_API_TOKEN"
NODE="YOUR_NODE_UUID"
ID="go.d:nginx"
JOB_NAME="local_server"

curl -sS \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/nodes/$NODE/config?action=userconfig&id=$(printf %s "$ID" | jq -sRr @uri)&name=$JOB_NAME"
```

---

## Source types

A configuration object's `source_type` (verified in
`<repo>/src/daemon/dyncfg/README.md`) determines what's allowed:

| Source type | Origin | Removable? |
|---|---|---|
| `INTERNAL` | Defined inside Netdata code | No |
| `DYNCFG` | Created or modified through DynCfg | Yes |
| `USER` | Loaded from user-provided conf files in `/etc/netdata/...` | No (edit the file instead) |

---

## Direct-agent reload note

There is **no REST endpoint to reload `/etc/netdata/health.d/*.conf`
files** -- DynCfg manages dynamic configuration objects, not raw
file reload. To reload static health/alert files, send `SIGHUP` to
the agent process or use the dyncfg `restart` action on the
relevant configuration object.

---

## Sensitive data

DynCfg responses can include credentials embedded in collector
job configurations (database URIs, API tokens used by collectors,
etc.). Treat raw responses as production-sensitive:

- Direct working output to `<repo>/.local/audits/...` (gitignored).
- Never paste raw `get` / `userconfig` payloads into committed
  files.
- See `<repo>/.agents/sow/specs/sensitive-data-discipline.md` for
  the full rule.
