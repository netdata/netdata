# Query agent network flows directly

This guide is part of the [`query-netdata-agents`](./SKILL.md) skill.
Read [SKILL.md](./SKILL.md#prerequisites) first.

For the body parameters (`mode`, `view`, `after`, `before`,
`query`, `selections`, `facets`, `group_by`, `sort_by`, `top_n`,
`field`, `term`), the response envelope (`data.flows[]`,
`data.columns`, `data.facets`, `data.stats`), and the three modes
(`flows` / `autocomplete`) plus five views (`table-sankey`,
`timeseries`, `country-map`, `state-map`, `city-map`), see
[../query-netdata-cloud/query-flows.md](../query-netdata-cloud/query-flows.md).
The body and response are identical between Cloud-proxied and
direct-agent calls.

Today only `flows:netflow` is registered (covers NetFlow v5/v9,
IPFIX, sFlow). The agent must run the netflow-plugin Rust crate
for the Function to be available.

---

## Endpoint (agent v3)

`POST /api/v3/function?function=flows:netflow`

## Use the wrapper

```bash
source "$(git rev-parse --show-toplevel)/.agents/skills/query-netdata-agents/scripts/_lib.sh"
agents_load_env

read -r -d '' BODY <<'JSON'
{
  "mode":     "flows",
  "view":     "table-sankey",
  "after":    -3600,
  "before":   0,
  "group_by": ["SRC_ADDR", "DST_ADDR", "PROTOCOL"],
  "sort_by":  "bytes",
  "top_n":    100
}
JSON

agents_query_agent \
    --node    "$NODE_UUID" \
    --host    "$AGENT_HOST:19999" \
    --machine-guid "$AGENT_MG" \
    POST '/api/v3/function?function=flows:netflow' "$BODY" \
  | jq '.data | {view, group_by, flows_count: (.flows|length), stats}'
```

## Discover supported parameters

```bash
agents_query_agent --node "$NODE_UUID" --host "$AGENT_HOST:19999" --machine-guid "$AGENT_MG" \
    POST '/api/v3/function?function=flows:netflow' '{"info":true}' \
  | jq '.required_params'
```

## Limits and gotchas

- **L3 only.** No L2 visibility, no application-layer dissection.
- **Sampling matters.** NetFlow v5/v9 and sFlow are sampled at
  the source device; reported byte/packet counts are scaled by
  the sample rate. Verify source-device sampling configuration
  before treating absolute volumes as ground truth.
- **GeoIP / AS DB dependency.** `SRC_AS_NAME`, `DST_AS_NAME`,
  `*_COUNTRY`, `*_CITY` only populate when the collector has the
  corresponding databases configured.
- **`top_n` is enumerated**: 25, 50, 100, 200, 500. Other values
  rejected.

## See also

- [../query-netdata-cloud/query-flows.md](../query-netdata-cloud/query-flows.md)
  -- full reference.
- [query-functions.md](./query-functions.md) -- generic Function
  transport.
