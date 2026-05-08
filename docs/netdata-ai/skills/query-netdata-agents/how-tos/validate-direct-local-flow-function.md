# Validate a local flow Function through direct-agent bearer auth

## Question

How can an assistant prove that a local Cloud-connected Netdata Agent
accepts a Cloud-minted per-agent bearer and serves `flows:netflow`
directly, without exposing Cloud tokens, agent bearers, node ids,
machine GUIDs, claim ids, or raw flow rows?

## Inputs

- Local agent URL, usually `http://127.0.0.1:19999`.
- `NETDATA_CLOUD_TOKEN` and `NETDATA_CLOUD_HOSTNAME` in `<repo>/.env`.
- The local agent must be connected to Cloud and expose `flows:netflow`.

## Steps

1. Capture the local identity tuple in memory and print only presence
   checks:

   ```bash
   INFO_JSON="$(curl -sS --max-time 10 http://127.0.0.1:19999/api/v3/info)"

   jq '{
     agent_count: (.agents | length),
     node_id_present: ((.agents[0].nd // "") | length > 0),
     machine_guid_present: ((.agents[0].mg // "") | length > 0),
     claim_id_present: ((.agents[0].cloud.claim_id // "") | length > 0),
     cloud_status: .agents[0].cloud.status
   }' <<<"$INFO_JSON"
   ```

2. Load the token-safe direct-agent wrappers:

   ```bash
   source docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh
   agents_load_env
   ```

3. Call the flow Function through the direct-agent path:

   ```bash
   NODE_UUID="$(jq -r '.agents[0].nd' \
     <<<"$INFO_JSON")"
   MACHINE_GUID="$(jq -r '.agents[0].mg' \
     <<<"$INFO_JSON")"

   mkdir -p .local/audits/query-netdata-agents

   agents_call_function \
     --via agent \
     --node "$NODE_UUID" \
     --host 127.0.0.1:19999 \
     --machine-guid "$MACHINE_GUID" \
     --function flows:netflow \
     --body '{"info":true}' \
     > .local/audits/query-netdata-agents/flows-netflow-info-agent.json
   ```

4. Print a sanitized result:

   ```bash
   jq '{
     status,
     type,
     has_history,
     response_keys: keys
   }' .local/audits/query-netdata-agents/flows-netflow-info-agent.json
   ```

## Output

Expected success shape:

```json
{
  "status": 200,
  "type": "flows",
  "has_history": true
}
```

The wrapper logs masked curl commands on stderr. The Cloud token,
per-agent bearer, node id, machine GUID, and claim id must not appear
in stdout or durable artifacts.

## Notes / gotchas

- Use the exact `nd`, `mg`, and `cloud.claim_id` tuple from the same
  local `/api/v3/info` response. A mixed tuple from a different node,
  parent, child, room, or stale cache can produce Cloud or agent
  rejection even when the Cloud-proxied Function path works.
- The direct agent uses `X-Netdata-Auth: Bearer <agent-bearer>`, not
  `Authorization: Bearer <cloud-token>`.
- The helper caches the raw bearer under
  `.local/audits/query-netdata-agents/bearers/`; that directory is
  gitignored and should stay mode `0700`, with bearer files mode
  `0600`.
- For content validation of flow rows, prefer a grouped query and print
  only counts/statistics. Do not paste raw flow rows into durable files.

## Source guides

- [Direct-agent skill](../SKILL.md)
- [Direct Function calls](../query-functions.md)
- [Network-flow Functions](../query-flows.md)
- [Cloud flow validation sibling how-to](../../query-netdata-cloud/how-tos/validate-local-netflow-function.md)
