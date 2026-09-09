# Top trap senders in the last hour

## Question

Which source devices sent the most SNMP traps in the last hour?

## Inputs

- `NODE_UUID`: node running the `snmp_traps` collector.
- `SNMP_TRAPS_JOB`: trap listener job name. Default examples use `local`.
- Optional `LAST_SECONDS`, defaulting to `3600`.

## Steps

1. Load the token-safe wrappers:

   ```bash
   source "$(git rev-parse --show-toplevel)/docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh"
   agents_load_env
   ```

2. Request source facets for recent trap entries:

   ```bash
   NODE_UUID="YOUR_NODE_UUID"
   SNMP_TRAPS_JOB="local"
   SNMP_TRAPS_FUNCTION="snmp:traps"
   LAST_SECONDS=3600

   BODY="$(jq -n --arg job "$SNMP_TRAPS_JOB" --argjson last_seconds "$LAST_SECONDS" '{
     after: (0 - $last_seconds),
     before: 0,
     last: 50,
     direction: "backward",
     selections: {
       __logs_sources: [$job],
       TRAP_REPORT_TYPE: ["trap"]
     },
     facets: ["TRAP_SOURCE_IP", "_HOSTNAME", "TRAP_DEVICE_VENDOR", "TRAP_SEVERITY"]
   }')"

   mkdir -p .local/audits/query-snmp-traps

   agents_call_function \
     --via cloud \
     --node "$NODE_UUID" \
     --function "$SNMP_TRAPS_FUNCTION" \
     --body "$BODY" \
     > .local/audits/query-snmp-traps/top-senders.json
   ```

3. Print the top source-IP facet values without exposing them in a
   durable report:

   ```bash
   jq '
     .facets[]?
     | select((.id // .name) == "TRAP_SOURCE_IP")
     | .options
     | sort_by(-(.count // 0))
     | .[:20]
     | map({source_ip: (.id // .name), count})
   ' .local/audits/query-snmp-traps/top-senders.json
   ```

4. If hostnames are available, inspect the `_HOSTNAME` facet too:

   ```bash
   jq '
     .facets[]?
     | select((.id // .name) == "_HOSTNAME")
     | .options
     | sort_by(-(.count // 0))
     | .[:20]
     | map({hostname: (.id // .name), count})
   ' .local/audits/query-snmp-traps/top-senders.json
   ```

## Output

Return the top sender counts. In durable artifacts, redact or
summarize source identities unless they are local/private examples.

## Notes / gotchas

- `TRAP_SOURCE_IP` is usually the most reliable sender key because
  traps arrive over UDP.
- `_HOSTNAME` is better when identity correlation from SNMP/topology
  is available.
- If deduplication is enabled, this query counts journaled trap
  entries, not suppressed duplicates. Query
  `TRAP_REPORT_TYPE=deduplication_summary` to inspect suppression.

## Source guides

- [query-snmp-traps](../SKILL.md)
- [Cloud log Function guide](../../query-netdata-cloud/query-logs.md)
