# Recent security traps from one device

## Question

Which security-category SNMP traps did one device send recently?

## Inputs

- `NODE_UUID`: node running the `snmp_traps` collector.
- `SNMP_TRAPS_JOB`: trap listener job name. Default examples use `local`.
- One of:
  - `DEVICE_IP`: the expected `TRAP_SOURCE_IP`.
  - `DEVICE_HOSTNAME`: the expected `_HOSTNAME`.
- Time window, defaulting to the last 24 hours.

## Steps

1. Load the token-safe wrappers:

   ```bash
   source "$(git rev-parse --show-toplevel)/docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh"
   agents_load_env
   ```

2. Query by source IP:

   ```bash
   NODE_UUID="YOUR_NODE_UUID"
   SNMP_TRAPS_JOB="local"
   SNMP_TRAPS_FUNCTION="snmp:traps"
   DEVICE_IP="[DEVICE_IP]"

   BODY="$(jq -n --arg job "$SNMP_TRAPS_JOB" --arg device_ip "$DEVICE_IP" '{
     after: -86400,
     before: 0,
     last: 200,
     direction: "backward",
     selections: {
       __logs_sources: [$job],
       TRAP_REPORT_TYPE: ["trap"],
       TRAP_CATEGORY: ["security"],
       TRAP_SOURCE_IP: [$device_ip]
     },
     facets: ["TRAP_NAME", "TRAP_SEVERITY", "TRAP_SOURCE_IP", "_HOSTNAME"]
   }')"

   mkdir -p .local/audits/query-snmp-traps

   agents_call_function \
     --via cloud \
     --node "$NODE_UUID" \
     --function "$SNMP_TRAPS_FUNCTION" \
     --body "$BODY" \
     > .local/audits/query-snmp-traps/security-traps-device.json
   ```

3. If the trap source is known by hostname instead of IP, replace the
   `TRAP_SOURCE_IP` selection with `_HOSTNAME`:

   ```bash
   NODE_UUID="YOUR_NODE_UUID"
   SNMP_TRAPS_JOB="local"
   SNMP_TRAPS_FUNCTION="snmp:traps"
   DEVICE_HOSTNAME="[DEVICE_HOSTNAME]"

   BODY="$(jq -n --arg job "$SNMP_TRAPS_JOB" --arg hostname "$DEVICE_HOSTNAME" '{
     after: -86400,
     before: 0,
     last: 200,
     direction: "backward",
     selections: {
       __logs_sources: [$job],
       TRAP_REPORT_TYPE: ["trap"],
       TRAP_CATEGORY: ["security"],
       _HOSTNAME: [$hostname]
     },
     facets: ["TRAP_NAME", "TRAP_SEVERITY", "TRAP_SOURCE_IP", "_HOSTNAME"]
   }')"

   agents_call_function \
     --via cloud \
     --node "$NODE_UUID" \
     --function "$SNMP_TRAPS_FUNCTION" \
     --body "$BODY" \
     > .local/audits/query-snmp-traps/security-traps-device.json
   ```

4. Print a sanitized summary:

   ```bash
   jq '.columns as $c
       | .data[]? as $row
       | $c | to_entries | sort_by(.value.index)
       | map({(.key): $row[.value.index]}) | add
       | {
           host: (._HOSTNAME // ""),
           source_ip_present: ((.TRAP_SOURCE_IP // "") | length > 0),
           severity: (.TRAP_SEVERITY // ""),
           trap: (.TRAP_NAME // .TRAP_OID // ""),
           message: (.MESSAGE // "")
         }' \
     .local/audits/query-snmp-traps/security-traps-device.json
   ```

## Output

Return counts and sanitized rows: trap name/OID, severity, hostname
or source-present flag, and message. Do not paste public IPs, MACs,
usernames, or full varbind payloads into durable artifacts.

## Notes / gotchas

- Prefer `TRAP_SOURCE_IP` when devices do not have stable hostname
  identity.
- Prefer `_HOSTNAME` when the SNMP collector/topology identity is
  already resolving the device name.
- Keep the time window short first; widen it only after confirming
  the query shape works.

## Source guides

- [query-snmp-traps](../SKILL.md)
- [Cloud log Function guide](../../query-netdata-cloud/query-logs.md)
- [Direct-agent log Function guide](../../query-netdata-agents/query-logs.md)
