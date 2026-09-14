# Export CPU, disk I/O, and memory for three days through a direct Agent call

## Question

How can an assistant export CPU, disk I/O, and memory metrics for the
72 hours ending at 14:00 today through the direct Agent API, without
exposing Cloud tokens, agent bearers, node ids, or machine GUIDs?

## Inputs

- `NODE_UUID`: the target node id.
- `MACHINE_GUID`: the target Agent machine GUID.
- `AGENT_HOST`: the Agent host and port without a URL scheme, for
  example `127.0.0.1:19999`.
- `NETDATA_CLOUD_TOKEN` and `NETDATA_CLOUD_HOSTNAME` in `<repo>/.env`.
- `jq` and either GNU `date` or BSD/macOS `date`.

The command uses the workstation's local timezone when interpreting
"today at 14:00". Run it after 14:00 to avoid requesting an end time
that the Agent has not collected yet.

## Steps

1. Load the token-safe direct-Agent wrappers:

   ```bash
   REPO_ROOT="$(git rev-parse --show-toplevel)"
   source "$REPO_ROOT/docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh"
   agents_load_env
   ```

2. Calculate the absolute query window. The GNU branch covers Linux;
   the fallback covers BSD and macOS:

   ```bash
   if END_EPOCH="$(date -d 'today 14:00' +%s 2>/dev/null)"; then
     :
   else
     END_EPOCH="$(date -j -f '%Y-%m-%d %H:%M:%S' \
       "$(date +%Y-%m-%d) 14:00:00" +%s)"
   fi

   AFTER_EPOCH=$((END_EPOCH - 3 * 24 * 60 * 60))
   POINTS=4320
   ```

   `POINTS=4320` requests one-minute buckets over 72 hours. This keeps
   the export manageable while preserving useful operational detail.

3. Build an encoded Agent query string and save the full JSON response
   under the repo-local, gitignored audit directory:

   ```bash
   QUERY="$(jq -rn \
     --arg scope_nodes "$NODE_UUID" \
     --arg scope_contexts 'system.cpu,disk.io,system.ram' \
     --arg nodes '*' \
     --arg contexts '*' \
     --arg instances '*' \
     --arg dimensions '*' \
     --arg labels '*' \
     --arg alerts '*' \
     --arg after "$AFTER_EPOCH" \
     --arg before "$END_EPOCH" \
     --arg points "$POINTS" \
     --arg group_by 'context,instance,dimension' \
     --arg aggregation 'sum' \
     --arg time_group 'average' \
     --arg format 'json2' \
     --arg options 'jsonwrap,minify,unaligned' \
     --arg timeout '60000' \
     '$ARGS.named | to_entries
      | map("\(.key)=\(.value | @uri)")
      | join("&")')"

   OUTPUT_DIR="$REPO_ROOT/.local/audits/query-netdata-agents"
   OUTPUT_FILE="$OUTPUT_DIR/cpu-io-memory-three-days.json"
   mkdir -p "$OUTPUT_DIR"
   chmod 0700 "$OUTPUT_DIR"
   : > "$OUTPUT_FILE"
   chmod 0600 "$OUTPUT_FILE"

   agents_query_agent \
     --node "$NODE_UUID" \
     --host "$AGENT_HOST" \
     --machine-guid "$MACHINE_GUID" \
     GET "/api/v3/data?$QUERY" \
     > "$OUTPUT_FILE"
   ```

4. Print a sanitized summary without exposing node identifiers or raw
   metric values:

   ```bash
   jq '{
     labels: .view.dimensions.names,
     rows: (.result.data | length),
     after: .view.after,
     before: .view.before
   }' "$OUTPUT_FILE"
   ```

## Output

The saved JSON contains the matching `system.cpu`, `disk.io`, and
`system.ram` time series in one-minute buckets for the requested
window. Keep the raw export under `.local/`; it can contain host,
chart, disk, and dimension identifiers.

## Notes / gotchas

- `disk.io` normally has one instance per disk. Grouping by context,
  instance, and dimension keeps those devices separate in the export.
- Keep `scope_nodes` in the query even though the wrapper URL contains
  `/host/{node}`. That path does not scope the v3 data query itself.
- `POINTS` controls bucket count, not collection frequency. A request
  cannot create finer samples than the selected storage tier holds.
- Setting `points: 0` leaves the target to the Agent query planner's
  default virtual-point behavior; it does not guarantee all available
  samples or a particular resolution.
- Requesting `tier: 0` is not a reliable availability check. Partial
  overlap returns only the tier-0 overlap, zero overlap returns no
  data for that metric, and a structurally invalid tier request can
  trigger automatic tier selection. Check `db.per_tier` before
  relying on the response resolution.
- The wrapper masks and caches the per-Agent bearer. Do not print raw
  tokens, node ids, machine GUIDs, or the complete response into
  durable artifacts.

## Source guides

- [Direct metrics queries](../query-metrics.md)
- [Direct-Agent skill](../SKILL.md)
- [Cloud metrics sibling guide](../../query-netdata-cloud/query-metrics.md)
