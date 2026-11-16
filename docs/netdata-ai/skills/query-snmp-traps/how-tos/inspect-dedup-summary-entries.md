# Inspect dedup summary entries during a flap storm

## Question

During a trap storm, how many duplicate traps were suppressed by the
collector deduplication window?

## Inputs

- `NODE_UUID`: node running the `snmp_traps` collector.
- `SNMP_TRAPS_JOB`: trap listener job name. Default examples use `local`.
- Time window covering the suspected flap.
- Optional trap OID or device selector.

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

2. Query dedup summary entries:

   ```bash
   NODE_UUID="YOUR_NODE_UUID"
   SNMP_TRAPS_JOB="local"
   SNMP_TRAPS_FUNCTION="snmp:traps"

   BODY="$(jq -n --arg job "$SNMP_TRAPS_JOB" '{
     after: -3600,
     before: 0,
     last: 200,
     direction: "backward",
     selections: {
       __logs_sources: [$job],
       TRAP_REPORT_TYPE: ["deduplication_summary"]
     },
     facets: ["TRAP_REPORT_PERIOD_SEC"]
   }')"

   RESPONSE="$TRAP_QUERY_DIR/dedup-summaries.json"

   agents_call_function \
     --via cloud \
     --node "$NODE_UUID" \
     --function "$SNMP_TRAPS_FUNCTION" \
     --body "$BODY" \
     > "$RESPONSE"
   ```

3. Summarize suppression counts from the returned rows:

   ```bash
   summarize_dedup() {
   jq -e '
     if type == "object" and .status == 200
        and (.columns | type == "object") and (.data | type == "array")
     then . else error("Expected a successful trap query response") end
     | .columns as $c
     | [ .data[]? as $row
         | $c | to_entries | sort_by(.value.index)
         | map({(.key): $row[.value.index]}) | add
         | {
             suppressed: ((.TRAP_SUPPRESSED_COUNT // "0") | tonumber? // 0),
             fingerprints: ((.TRAP_SUPPRESSED_FINGERPRINTS // "0") | tonumber? // 0),
             period_sec: ((.TRAP_REPORT_PERIOD_SEC // "0") | tonumber? // 0)
           }
       ]
     | {
         entries: length,
         suppressed_total: (map(.suppressed) | add // 0),
         fingerprint_interval_total: (map(.fingerprints) | add // 0),
         max_period_sec: (map(.period_sec) | max // 0),
         rows: .
       }
   ' "$RESPONSE"
   }
   summarize_dedup
   ```

4. To focus on one trap OID, add a full-text narrower because the
   per-OID breakdown lives inside `TRAP_JSON`:

   ```bash
   TRAP_OID="[TRAP_OID]"

   BODY="$(jq -n --arg job "$SNMP_TRAPS_JOB" --arg oid "$TRAP_OID" '{
     after: -3600,
     before: 0,
     last: 200,
     direction: "backward",
     selections: {
       __logs_sources: [$job],
       TRAP_REPORT_TYPE: ["deduplication_summary"]
     },
     query: $oid
   }')"

   RESPONSE="$(mktemp "$TRAP_QUERY_DIR/dedup-oid.XXXXXX")"
   agents_call_function \
     --via cloud \
     --node "$NODE_UUID" \
     --function "$SNMP_TRAPS_FUNCTION" \
     --body "$BODY" \
     > "$RESPONSE"
   summarize_dedup
   ```

## Output

Return suppressed totals, summary-entry count and summed interval fingerprint counts for the returned rows only
(at most 200). Fingerprints may recur across summary intervals; their sum is not a globally distinct count. Use
[log pagination](../../query-netdata-cloud/query-logs.md#example-3-paginate-forward-from-a-known-anchor) for
a full-window total.

The OID full-text query selects matching summary entries. Each entry’s total includes every OID in that entry,
not just the searched OID. You MAY inspect `TRAP_JSON` and its `by_trap` breakdown in the private response for per-OID
counts. Messages and full payloads remain available for local investigation; redact identifying details before sharing.

## Notes / gotchas

- Dedup summaries are separate journal entries distinguished by
  `TRAP_REPORT_TYPE=deduplication_summary`.
- If no summaries appear, dedup may be disabled, the flap may be
  outside the query window, or duplicates may not share the same
  configured fingerprint.

## Source guides

- [query-snmp-traps](../SKILL.md)
- [Cloud log Function guide](../../query-netdata-cloud/query-logs.md)
