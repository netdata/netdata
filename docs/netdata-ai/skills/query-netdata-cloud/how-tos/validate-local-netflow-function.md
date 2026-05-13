# Validate a local Cloud-connected flow Function

## Question

How can an assistant validate `flows:netflow` on a local Netdata Agent
that is connected to Netdata Cloud, without exposing Cloud tokens,
agent bearers, node ids, or raw flow rows?

## Inputs

- Local agent URL, usually `http://127.0.0.1:19999`.
- `NETDATA_CLOUD_TOKEN` and `NETDATA_CLOUD_HOSTNAME` in `<repo>/.env`.
- The agent must have `flows:netflow` registered.

## Steps

1. Capture local agent identity in memory without printing identifiers:

   ```bash
   INFO_JSON="$(curl -sS --max-time 10 http://127.0.0.1:19999/api/v3/info)"

   jq -r '.agents[0] | {
     cloud_status: .cloud.status,
     node_id_present: ((.nd // "") | length > 0),
     machine_guid_present: ((.mg // "") | length > 0),
     claim_id_present: ((.cloud.claim_id // "") | length > 0)
   }' <<<"$INFO_JSON"
   ```

2. Load the token-safe wrappers:

   ```bash
   source docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh
   agents_load_env
   ```

3. Verify the Function info envelope via Cloud:

   ```bash
   NODE_UUID="$(jq -r '.agents[0].nd' \
     <<<"$INFO_JSON")"

   mkdir -p .local/audits/query-netdata-agents

   agents_call_function \
     --via cloud \
     --node "$NODE_UUID" \
     --function flows:netflow \
     --body '{"info":true}' \
     > .local/audits/query-netdata-agents/flows-netflow-info-cloud.json

   jq '{status, type, has_history,
        accepted_params_count: (.accepted_params | length),
        required_params_count: (.required_params | length)}' \
     .local/audits/query-netdata-agents/flows-netflow-info-cloud.json
   ```

4. Run a real flow query using the documented request shape:

   ```bash
   read -r -d '' BODY <<'JSON'
   {
     "mode": "flows",
     "view": "table-sankey",
     "after": -3600,
     "before": 0,
     "group_by": ["SRC_AS_NAME", "PROTOCOL", "DST_AS_NAME"],
     "sort_by": "bytes",
     "top_n": 100
   }
   JSON

   agents_call_function \
     --via cloud \
     --node "$NODE_UUID" \
     --function flows:netflow \
     --body "$BODY" \
     > .local/audits/query-netdata-agents/flows-netflow-last-hour-cloud.json

   jq '{status, type, view: .data.view,
        flows_count: (.data.flows | length),
        group_by: .data.group_by,
        stats: .data.stats}' \
     .local/audits/query-netdata-agents/flows-netflow-last-hour-cloud.json
   ```

## Output

Return only a sanitized summary:

- Function info `status` and `type`.
- Flow query row count.
- Group-by fields.
- Selected aggregate counters from `.data.stats`, such as
  `decoded_netflow_v5`, `decoded_netflow_v9`, `decoded_ipfix`,
  `decoded_sflow`, `journal_entries_written`, and
  `journal_write_errors`.

Do not paste node ids, machine GUIDs, claim ids, Cloud tokens, agent
bearers, raw IP addresses, or raw flow rows into durable artifacts.

## Notes / gotchas

- Prefer the Cloud transport for validation. It needs only the Cloud
  token and does not require a direct agent bearer.
- Direct-agent validation is also possible. Use the sibling
  direct-agent how-to when the test must prove the bearer mint/cache
  path and the `X-Netdata-Auth` call path.
- Negative `after` values are relative to `before`; `before: 0` means
  now. `top_n` accepts the documented values `25`, `50`, `100`,
  `200`, or `500`.

## Source guides

- [Network-flow Functions](../query-flows.md)
- [Generic Function invocation](../query-functions.md)
- [Direct-agent sibling skill](../../query-netdata-agents/SKILL.md)
- [Direct local flow Function validation](../../query-netdata-agents/how-tos/validate-direct-local-flow-function.md)
