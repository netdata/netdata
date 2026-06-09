# Filter by severity across a room

## Question

Which nodes in a room received critical or emergency SNMP traps?

## Inputs

- `SPACE_ID`: Netdata Cloud space ID.
- `ROOM_ID`: Netdata Cloud room ID.
- `SNMP_TRAPS_JOB`: trap listener job name on each node. Default examples use `local`.
- Optional severity list. The example uses `emerg` and `crit`.

## Steps

1. Load the token-safe wrappers:

   ```bash
   source "$(git rev-parse --show-toplevel)/docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh"
   agents_load_env
   ```

2. List node UUIDs visible in the room:

   ```bash
   SPACE_ID="YOUR_SPACE_ID"
   ROOM_ID="YOUR_ROOM_ID"

   mkdir -p .local/audits/query-snmp-traps

   agents_query_cloud \
     POST "/api/v3/spaces/${SPACE_ID}/rooms/${ROOM_ID}/nodes" '{}' \
     > .local/audits/query-snmp-traps/room-nodes.json

   jq -r '.[] | select(.state=="reachable") | .nd' \
     .local/audits/query-snmp-traps/room-nodes.json \
     | sort -u \
     > .local/audits/query-snmp-traps/room-node-ids.txt
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

   rm -f .local/audits/query-snmp-traps/severity-*.json

   while IFS= read -r NODE_UUID; do
     [ -n "$NODE_UUID" ] || continue
     OUT=".local/audits/query-snmp-traps/severity-${NODE_UUID}.json"
     if ! agents_call_function \
         --via cloud \
         --node "$NODE_UUID" \
         --function "$SNMP_TRAPS_FUNCTION" \
         --body "$BODY" \
         > "$OUT"; then
       rm -f "$OUT"
       echo "WARN: query failed for node ${NODE_UUID}, skipping" >&2
     fi
   done < .local/audits/query-snmp-traps/room-node-ids.txt
   ```

4. Aggregate severity facet counts:

   ```bash
   shopt -s nullglob
   files=(.local/audits/query-snmp-traps/severity-*.json)

   if [[ ${#files[@]} -eq 0 ]]; then
     echo "No severe trap query results were collected from reachable nodes."
   else
     jq -s '
       [ .[]
         | .facets[]?
         | select((.id // .name) == "TRAP_SEVERITY")
         | .options[]?
         | {severity: (.id // .name), count: (.count // 0)}
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

   for f in .local/audits/query-snmp-traps/severity-*.json; do
     rows="$(jq -r '(.data // []) | length' "$f")"
     [[ "$rows" -gt 0 ]] || continue
     node_uuid="${f##*/severity-}"
     node_uuid="${node_uuid%.json}"
     printf '%s rows=%s\n' "$node_uuid" "$rows"
   done
   ```

## Output

Return a severity-count summary and, when needed, a local-only node list
or a sanitized node list that had matching rows. Do not paste raw node
UUIDs or public device IPs unless the user explicitly asks for local-only
output.

## Notes / gotchas

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

- [query-snmp-traps](../SKILL.md)
- [Cloud nodes guide](../../query-netdata-cloud/query-nodes.md)
- [Cloud log Function guide](../../query-netdata-cloud/query-logs.md)
