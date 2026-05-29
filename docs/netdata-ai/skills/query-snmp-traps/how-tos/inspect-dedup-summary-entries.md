# Inspect dedup summary entries during a flap storm

## Question

During a trap storm, how many duplicate traps were suppressed by the
collector deduplication window?

## Inputs

- `NODE_UUID`: node running the `snmp_traps` collector.
- Time window covering the suspected flap.
- Optional trap OID or device selector.

## Steps

1. Load the token-safe wrappers:

   ```bash
   source "$(git rev-parse --show-toplevel)/docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh"
   agents_load_env
   ```

2. Query dedup summary entries:

   ```bash
   NODE_UUID="YOUR_NODE_UUID"

   BODY="$(jq -n '{
     after: -3600,
     before: 0,
     last: 200,
     direction: "backward",
     "__logs_sources": "all",
     selections: {
       ND_LOG_SOURCE: ["snmp-trap"],
       TRAP_REPORT_TYPE: ["deduplication_summary"]
     },
     facets: ["TRAP_REPORT_PERIOD_SEC"]
   }')"

   mkdir -p .local/audits/query-snmp-traps

   agents_call_function \
     --via cloud \
     --node "$NODE_UUID" \
     --function systemd-journal \
     --body "$BODY" \
     > .local/audits/query-snmp-traps/dedup-summaries.json
   ```

3. Summarize suppression counts:

   ```bash
   jq '
     .columns as $c
     | [ .data[]? as $row
         | $c | to_entries | sort_by(.value.index)
         | map({(.key): $row[.value.index]}) | add
         | {
             suppressed: ((.TRAP_SUPPRESSED_COUNT // "0") | tonumber? // 0),
             fingerprints: ((.TRAP_SUPPRESSED_FINGERPRINTS // "0") | tonumber? // 0),
             period_sec: ((.TRAP_REPORT_PERIOD_SEC // "0") | tonumber? // 0),
             message: (.MESSAGE // "")
           }
       ]
     | {
         entries: length,
         suppressed_total: (map(.suppressed) | add // 0),
         fingerprints_total: (map(.fingerprints) | add // 0),
         max_period_sec: (map(.period_sec) | max // 0),
         rows: .
       }
   ' .local/audits/query-snmp-traps/dedup-summaries.json
   ```

4. To focus on one trap OID, add a full-text narrower because the
   per-OID breakdown lives inside `TRAP_JSON`:

   ```bash
   TRAP_OID="[TRAP_OID]"

   BODY="$(jq -n --arg oid "$TRAP_OID" '{
     after: -3600,
     before: 0,
     last: 200,
     direction: "backward",
     "__logs_sources": "all",
     selections: {
       ND_LOG_SOURCE: ["snmp-trap"],
       TRAP_REPORT_TYPE: ["deduplication_summary"]
     },
     query: $oid
   }')"
   ```

## Output

Return total suppressed count, number of summary entries, and whether
one or many fingerprints were involved. Avoid pasting full
`TRAP_JSON` if it contains identifying device details.

## Notes / gotchas

- Dedup summaries are separate journal entries distinguished by
  `TRAP_REPORT_TYPE=deduplication_summary`.
- If no summaries appear, dedup may be disabled, the flap may be
  outside the query window, or duplicates may not share the same
  configured fingerprint.

## Source guides

- [query-snmp-traps](../SKILL.md)
- [Cloud log Function guide](../../query-netdata-cloud/query-logs.md)
