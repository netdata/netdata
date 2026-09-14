# Filter by severity across a room

## Question

Which nodes in a room received critical or emergency SNMP traps?

## Inputs

- `SPACE_ID`: Netdata Cloud space ID.
- `ROOM_ID`: Netdata Cloud room ID.
- `SNMP_TRAPS_JOB`: trap listener job name on each node. Default examples use `local`.
- Optional severity list. The example uses `emerg` and `crit`.

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

2. List node UUIDs visible in the room:

   ```bash
   SPACE_ID="YOUR_SPACE_ID"
   ROOM_ID="YOUR_ROOM_ID"

   agents_query_cloud \
     POST "/api/v3/spaces/${SPACE_ID}/rooms/${ROOM_ID}/nodes" '{}' \
     > "$TRAP_QUERY_DIR/room-nodes.json"

   jq -r '.nodes[] | select(.state=="reachable") | .nd' \
     "$TRAP_QUERY_DIR/room-nodes.json" \
     | sort -u \
     > "$TRAP_QUERY_DIR/room-node-ids.txt"
   ```

3. Query every node for severe trap rows:

   ```bash
   SNMP_TRAPS_JOB="local"
   SNMP_TRAPS_FUNCTION="snmp:traps"

   BODY="$(jq -n --arg job "$SNMP_TRAPS_JOB" '{
     after: -3600,
     before: 0,
     last: 200,
     direction: "backward",
     selections: {
       __logs_sources: [$job],
       TRAP_REPORT_TYPE: ["trap"],
       TRAP_SEVERITY: ["emerg", "crit"]
     },
     facets: ["TRAP_SEVERITY", "TRAP_SOURCE_IP", "_HOSTNAME", "TRAP_NAME"]
   }')"

   while IFS= read -r NODE_UUID; do
     [ -n "$NODE_UUID" ] || continue
     PARTIAL="$(mktemp "$TRAP_QUERY_DIR/failed.XXXXXX")"
     OUT="$TRAP_QUERY_DIR/severity-${NODE_UUID}.json"
     if agents_call_function \
         --via cloud \
         --node "$NODE_UUID" \
         --function "$SNMP_TRAPS_FUNCTION" \
         --body "$BODY" \
         > "$PARTIAL" &&
         jq -e 'type == "object" and .status == 200 and (.data | type == "array")' "$PARTIAL" >/dev/null; then
       mv "$PARTIAL" "$OUT"
     else
       echo "WARN: node query failed; partial response retained privately, skipping" >&2
     fi
   done < "$TRAP_QUERY_DIR/room-node-ids.txt"
   ```

4. Aggregate severity facet counts:

   ```bash
   shopt -s nullglob
   files=("$TRAP_QUERY_DIR"/severity-*.json)

   if [[ ${#files[@]} -eq 0 ]]; then
     echo "No severe trap query results were collected from reachable nodes."
   else
     partial_nodes="$(jq -s '[.[] | select(.partial == true)] | length' "${files[@]}")"
     if [[ "$partial_nodes" -gt 0 ]]; then
       printf 'WARN: %s node responses are partial; aggregated counts may be incomplete.\n' "$partial_nodes" >&2
     fi
     jq -s --argjson requested "$(jq '.selections.TRAP_SEVERITY' <<<"$BODY")" '
       ($requested // []) as $selected
       | [ .[]
         | .facets[]?
         | select((.id // .name) == "TRAP_SEVERITY")
         | .options[]?
         | {severity: (.id // .name), count: (.count // 0)}
         | select(($selected | length) == 0 or (.severity as $severity | $selected | index($severity)))
       ]
       | group_by(.severity)
       | map({severity: .[0].severity, count: (map(.count) | add)})
       | sort_by(-.count)
     ' "${files[@]}"
   fi
   ```

5. List nodes that had matching rows for local investigation:

   ```bash
   shopt -s nullglob

   for f in "$TRAP_QUERY_DIR"/severity-*.json; do
     rows="$(jq -r '(.data // []) | length' "$f")"
     [[ "$rows" -gt 0 ]] || continue
     node_uuid="${f##*/severity-}"
     node_uuid="${node_uuid%.json}"
     printf '%s rows=%s\n' "$node_uuid" "$rows"
   done > "$TRAP_QUERY_DIR/matching-nodes.txt"
   ```

## Output

Return the severity-count summary. Inspect `matching-nodes.txt` privately when node identities are needed;
redact identifying details before sharing a node list or copying it into durable artifacts.

## Notes / gotchas

- Severity facets can include unselected options when several fields are selected. The aggregation explicitly
  keeps only the severities in `BODY` when that selection is nonempty; an omitted, null or empty selection keeps
  all severities. Successful responses alone enter this run’s aggregate. Failed partial
  responses remain private for diagnosis. Status-200 partial results remain usable but emit a completeness warning.
- Cloud Log Function calls are node-scoped. Room-wide trap questions
  require listing nodes, querying each node, and aggregating locally.
- The node list filters to `.state=="reachable"` to avoid failed calls
  to stale or offline nodes. Individual node calls can still fail if the
  node state changes while the loop is running; failed nodes are skipped.
- If the room is large, reduce the time window first or query only
  nodes that run the `snmp_traps` collector.
- If nodes use different trap listener job names, repeat the query with
  each job name in `selections.__logs_sources`, or omit that selection
  to query all direct-journal trap jobs on each node.
- Use `alert` as an additional severity when the question is about
  all urgent traps, not just critical/emergency traps.

## Source guides

Facet-filter behavior checked in `netdata/systemd-journal-sdk @ 9d5e3e19cf53179aaec3af67ac409d844a44c15f`:
`go/journal/netdata.go` (`netdataRequest.toExplorerQuery`) and `go/journal/explorer.go` (`facetPassGroups`).

- [query-snmp-traps](../SKILL.md)
- [Cloud nodes guide](../../query-netdata-cloud/query-nodes.md)
- [Cloud log Function guide](../../query-netdata-cloud/query-logs.md)
