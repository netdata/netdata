# Top trap senders in the last hour

## Question

Which source devices sent the most SNMP traps in the last hour?

## Inputs

- `NODE_UUID`: node running the `snmp_traps` collector.
- `SNMP_TRAPS_JOB`: trap listener job name. Default examples use `local`.
- Optional `LAST_SECONDS`, defaulting to `3600`.

## Steps

Run from the repository root in one Bash session. The private run directory retains raw responses for local
inspection; token-safe request logging does not sanitize their contents. Start a new run for another execution.

1. Load the token-safe wrappers:

   ```bash
   source "$(git rev-parse --show-toplevel)/docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh"
   agents_load_env
   mkdir -p .local/audits/query-snmp-traps
   TRAP_QUERY_DIR="$(mktemp -d .local/audits/query-snmp-traps/query.XXXXXX)"
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

   agents_call_function \
     --via cloud \
     --node "$NODE_UUID" \
     --function "$SNMP_TRAPS_FUNCTION" \
     --body "$BODY" \
     > "$TRAP_QUERY_DIR/top-senders.json"

   if jq -e '.partial == true' "$TRAP_QUERY_DIR/top-senders.json" >/dev/null; then
     printf 'WARN: partial query response; sender counts and rankings may be incomplete.\n' >&2
   fi
   ```

3. Save the top source-IP facet values privately and print their ranked counts:

   ```bash
   jq -e '
     if type == "object" and .status == 200
        and (.data | type == "array") and (.facets | type == "array")
     then . else error("Expected a successful trap query response") end
     | .facets[]?
     | select((.id // .name) == "TRAP_SOURCE_IP")
     | .options
     | sort_by(-(.count // 0))
     | .[:20]
     | map({source_ip: (.id // .name), count})
   ' "$TRAP_QUERY_DIR/top-senders.json" > "$TRAP_QUERY_DIR/top-source-ips.json"

   jq 'to_entries | map({rank: (.key + 1), count: .value.count})' \
     "$TRAP_QUERY_DIR/top-source-ips.json"
   ```

4. Optionally inspect the `_HOSTNAME` facet. Because `BODY` requests this facet, successful responses include
   it with `options: []` when no hostname values are available; the ranking is then `[]`:

   ```bash
   jq -e '
     if type == "object" and .status == 200
        and (.data | type == "array") and (.facets | type == "array")
     then . else error("Expected a successful trap query response") end
     | .facets[]?
     | select((.id // .name) == "_HOSTNAME")
     | .options
     | sort_by(-(.count // 0))
     | .[:20]
     | map({hostname: (.id // .name), count})
   ' "$TRAP_QUERY_DIR/top-senders.json" > "$TRAP_QUERY_DIR/top-hostnames.json"
   ```

## Output

Return the ranked sender counts. You MAY inspect the private source-IP and hostname files locally to identify
the senders. These files contain raw identities; review and redact them before copying details into durable artifacts.

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
