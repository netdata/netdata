# Search a varbind value in TRAP_JSON

## Question

How can an operator find traps whose structured varbind payload
contains a specific value?

## Inputs

- `NODE_UUID`: node running the `snmp_traps` collector.
- `SNMP_TRAPS_JOB`: trap listener job name. Default examples use `local`.
- `NEEDLE`: the value or substring to find.
- Optional time window and trap OID/category/severity selectors.

## Steps

1. Load the token-safe wrappers:

   ```bash
   source "$(git rev-parse --show-toplevel)/docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh"
   agents_load_env
   ```

2. Query trap rows using structured selections plus a full-text
   residual search:

   ```bash
   NODE_UUID="YOUR_NODE_UUID"
   SNMP_TRAPS_JOB="local"
   SNMP_TRAPS_FUNCTION="snmp:traps"
   NEEDLE="[VARBIND_VALUE]"

   BODY="$(jq -n --arg job "$SNMP_TRAPS_JOB" --arg needle "$NEEDLE" '{
     after: -86400,
     before: 0,
     last: 200,
     direction: "backward",
     selections: {
       __logs_sources: [$job],
       TRAP_REPORT_TYPE: ["trap"]
     },
     query: $needle,
     facets: ["TRAP_NAME", "TRAP_OID", "TRAP_CATEGORY", "TRAP_SEVERITY", "TRAP_SOURCE_IP"]
   }')"

   mkdir -p .local/audits/query-snmp-traps

   agents_call_function \
     --via cloud \
     --node "$NODE_UUID" \
     --function "$SNMP_TRAPS_FUNCTION" \
     --body "$BODY" \
     > .local/audits/query-snmp-traps/varbind-search.json
   ```

3. Decode rows and keep only rows whose `TRAP_JSON` contains the
   value:

   ```bash
   jq --arg needle "$NEEDLE" '
     .columns as $c
     | [ .data[]? as $row
         | $c | to_entries | sort_by(.value.index)
         | map({(.key): $row[.value.index]}) | add
         | select((.TRAP_JSON // "") | contains($needle))
         | {
             trap: (.TRAP_NAME // .TRAP_OID // ""),
             category: (.TRAP_CATEGORY // ""),
             severity: (.TRAP_SEVERITY // ""),
             source_ip_present: ((.TRAP_SOURCE_IP // "") | length > 0),
             message: (.MESSAGE // "")
           }
       ]
   ' .local/audits/query-snmp-traps/varbind-search.json
   ```

4. If local inspection of the structured varbind object is needed,
   parse it locally:

   ```bash
   jq --arg needle "$NEEDLE" '
     .columns as $c
     | .data[]? as $row
     | $c | to_entries | sort_by(.value.index)
     | map({(.key): $row[.value.index]}) | add
     | select((.TRAP_JSON // "") | contains($needle))
     | {
         trap: (.TRAP_NAME // .TRAP_OID // ""),
         varbinds: ((.TRAP_JSON // "{}") | try fromjson catch {})
       }
   ' .local/audits/query-snmp-traps/varbind-search.json
   ```

## Output

Return sanitized trap names, categories, severities, and messages.
Do not paste full parsed varbind objects into durable artifacts if
they contain MAC addresses, usernames, packet contents, public IPs,
or customer identifiers.

## Notes / gotchas

- `TRAP_JSON` is payload data. It is searchable, but it should not
  be used as a facet because values are high-cardinality.
- Narrow with `TRAP_OID`, `TRAP_CATEGORY`, `TRAP_SEVERITY`, or source
  identity when possible before using full-text search.
- For exact structured extraction, keep the raw response under
  `.local/` and parse it locally with `jq`.

## Source guides

- [query-snmp-traps](../SKILL.md)
- [Cloud log Function guide](../../query-netdata-cloud/query-logs.md)
