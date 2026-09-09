# Filter an indexed varbind field and inspect TRAP_JSON

## Question

How can an operator find traps by a decoded varbind value and inspect
the full structured payload when needed?

## Inputs

- `NODE_UUID`: node running the `snmp_traps` collector.
- `SNMP_TRAPS_JOB`: trap listener job name. Default examples use `local`.
- `TRAP_VAR_FIELD`: indexed varbind field, such as `TRAP_VAR_IFINDEX`.
- `TRAP_VAR_VALUE`: exact value to filter on.
- Optional time window and trap OID/category/severity selectors.

## Steps

1. Load the token-safe wrappers:

   ```bash
   source "$(git rev-parse --show-toplevel)/docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh"
   agents_load_env
   ```

2. Query trap rows using structured selections. Prefer `TRAP_VAR_*`
   fields for filtering; use `TRAP_JSON` only as the audit copy:

   ```bash
   NODE_UUID="YOUR_NODE_UUID"
   SNMP_TRAPS_JOB="local"
   SNMP_TRAPS_FUNCTION="snmp:traps"
   TRAP_VAR_FIELD="TRAP_VAR_IFINDEX"
   TRAP_VAR_VALUE="29"

   BODY="$(jq -n \
     --arg job "$SNMP_TRAPS_JOB" \
     --arg field "$TRAP_VAR_FIELD" \
     --arg value "$TRAP_VAR_VALUE" '{
     after: -86400,
     before: 0,
     last: 200,
     direction: "backward",
     selections: {
       __logs_sources: [$job],
       TRAP_REPORT_TYPE: ["trap"],
       ($field): [$value]
     },
     facets: ["TRAP_NAME", "TRAP_OID", "TRAP_CATEGORY", "TRAP_SEVERITY", "TRAP_SOURCE_IP", $field]
   }')"

   mkdir -p .local/audits/query-snmp-traps

   agents_call_function \
     --via cloud \
     --node "$NODE_UUID" \
     --function "$SNMP_TRAPS_FUNCTION" \
     --body "$BODY" \
     > .local/audits/query-snmp-traps/varbind-filter.json
   ```

3. Decode matching rows:

   ```bash
   jq '
     .columns as $c
     | [ .data[]? as $row
         | $c | to_entries | sort_by(.value.index)
         | map({(.key): $row[.value.index]}) | add
         | {
             trap: (.TRAP_NAME // .TRAP_OID // ""),
             category: (.TRAP_CATEGORY // ""),
             severity: (.TRAP_SEVERITY // ""),
             source_ip_present: ((.TRAP_SOURCE_IP // "") | length > 0),
             ifindex: (.TRAP_VAR_IFINDEX // ""),
             message: (.MESSAGE // "")
           }
       ]
   ' .local/audits/query-snmp-traps/varbind-filter.json
   ```

4. If local inspection of the structured varbind object is needed,
   parse it locally:

   ```bash
   jq '
     .columns as $c
     | .data[]? as $row
     | $c | to_entries | sort_by(.value.index)
     | map({(.key): $row[.value.index]}) | add
     | {
         trap: (.TRAP_NAME // .TRAP_OID // ""),
         varbinds: ((.TRAP_JSON // "{}") | try fromjson catch {})
       }
   ' .local/audits/query-snmp-traps/varbind-filter.json
   ```

## Output

Return sanitized trap names, categories, severities, and messages.
Do not paste full parsed varbind objects into durable artifacts if
they contain MAC addresses, usernames, packet contents, public IPs,
or customer identifiers.

## Notes / gotchas

- `TRAP_VAR_*` fields are indexed journal fields and are the primary
  way to filter by decoded varbind values.
- `TRAP_JSON` is the audit/debug copy. It is searchable, but full-text
  JSON search should be the fallback when no indexed `TRAP_VAR_*`
  field exists for the value being investigated.
- Narrow with `TRAP_OID`, `TRAP_CATEGORY`, `TRAP_SEVERITY`, or source
  identity when possible.
- For exact structured extraction, keep the raw response under
  `.local/` and parse it locally with `jq`.

## Source guides

- [query-snmp-traps](../SKILL.md)
- [Cloud log Function guide](../../query-netdata-cloud/query-logs.md)
