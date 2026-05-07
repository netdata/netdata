# Query agent logs directly

This guide is part of the [`query-netdata-agents`](./SKILL.md) skill.
Read [SKILL.md](./SKILL.md#prerequisites) first.

For the body shape (`after`, `before`, `last`, `query`, `facets`,
`histogram`, `__logs_sources`, `selections`, etc.) and the
response envelope (top-level `data` is an array of row arrays;
`columns` defines positions; `facets` and `histogram` accompany),
see
[../query-netdata-cloud/query-logs.md](../query-netdata-cloud/query-logs.md).
The body and response are identical between Cloud-proxied and
direct-agent calls -- including the multi-value `selections`
field-filter mechanism (AND across fields, OR across values),
which makes index-friendly queries possible on large namespaces.
See the "Multi-value field selections" section in the Cloud doc
for the exact shape and the structured-filters-first rule.

The agent ships the same three log Functions:

- `systemd-journal` (Linux nodes)
- `windows-events` (Windows nodes)
- `otel-logs` (when the OTEL log receiver is enabled)

---

## Endpoint (agent v3)

`POST /api/v3/function?function=<log-fn>` on the agent.

## Use the wrapper

```bash
source "$(git rev-parse --show-toplevel)/.agents/skills/query-netdata-agents/scripts/_lib.sh"
agents_load_env

# Last-hour skim of a specific journal namespace, 50 rows.
agents_query_agent \
    --node "$AGENT_EVENTS_NODE_ID" \
    --host "$AGENT_EVENTS_HOSTNAME:19999" \
    --machine-guid "$AGENT_EVENTS_MACHINE_GUID" \
    POST '/api/v3/function?function=systemd-journal' \
    '{"after":-3600,"before":0,"last":50,"direction":"backward","__logs_sources":"agent-events"}'
```

The wrapper minted/cached the bearer internally; stdout is the
response body only. The bearer never reaches the assistant's
captured output.

## Discover the available log sources

```bash
agents_query_agent \
    --node "$AGENT_EVENTS_NODE_ID" \
    --host "$AGENT_EVENTS_HOSTNAME:19999" \
    --machine-guid "$AGENT_EVENTS_MACHINE_GUID" \
    POST '/api/v3/function?function=systemd-journal' '{"info":true}' \
  | jq '.required_params[] | select(.id=="__logs_sources") | .options'
```

Reads the `info=true` response and lists the `__logs_sources`
widget options the agent currently exposes. The `name`+`id` of
each option is what you pass back as the `__logs_sources` value.

## Limits and gotchas (single-agent-specific)

- **Single-host only.** The agent answers for itself; for fleet
  queries, use the Cloud-side path or aggregate per-agent
  responses client-side.
- **Time bounds**: negative values are seconds-relative-to-now.
  Positive values are unix-microseconds (NOT seconds, NOT
  milliseconds). Mixing units is the most common bug.
- **Slow queries**: large windows + wide facets can take seconds.
  Bump `timeout` in the body to 60000 or higher when the default
  10-second cloud-proxy default isn't relevant (the agent itself
  honors the body timeout up to its own ceiling).

## See also

- [../query-netdata-cloud/query-logs.md](../query-netdata-cloud/query-logs.md)
  -- full body/response shape, examples, response field
  reference.
- [query-functions.md](./query-functions.md) -- the generic
  Function transport.
- `<repo>/src/plugins.d/FUNCTION_UI_REFERENCE.md` -- canonical
  Log Explorer Format spec.
