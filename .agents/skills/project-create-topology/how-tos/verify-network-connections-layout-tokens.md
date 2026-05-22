# Verify network-connections layout tokens

## Question

How can a developer verify that a local Cloud-connected Netdata Agent serves
`topology:network-connections` with the expected v1 link layout tokens and
correlation rule wiring, without exposing Cloud tokens, agent bearers, node ids,
machine GUIDs, claim ids, cookies, or raw endpoint rows?

This is a producer-contract validation recipe. It belongs to the developer
topology skill, not to the public/operator query skills.

## Inputs

- Local Agent URL, usually `http://127.0.0.1:19999`; set `AGENT_URL` when
  using a non-default address.
- `NETDATA_CLOUD_TOKEN` and `NETDATA_CLOUD_HOSTNAME` in `<repo>/.env`.
- A local Agent that exposes `topology:network-connections`.

## Steps

1. Capture the local identity tuple in memory and print only presence
   checks:

   ```bash
   AGENT_URL="${AGENT_URL:-http://127.0.0.1:19999}"
   AGENT_URL="${AGENT_URL%/}"
   AGENT_HOST="${AGENT_URL#http://}"
   AGENT_HOST="${AGENT_HOST#https://}"
   AGENT_HOST="${AGENT_HOST%%/*}"
   AGENT_SCHEME="http"
   case "$AGENT_URL" in
     https://*) AGENT_SCHEME="https" ;;
   esac
   AGENT_ORIGIN="${AGENT_SCHEME}://${AGENT_HOST}"

   INFO_JSON="$(curl --fail -sS --max-time 10 "${AGENT_ORIGIN}/api/v3/info")"

   jq '{
     agent_count: (.agents | length),
     node_id_present: ((.agents[0].nd // "") | length > 0),
     machine_guid_present: ((.agents[0].mg // "") | length > 0),
     claim_id_present: ((.agents[0].cloud.claim_id // "") | length > 0),
     cloud_status: .agents[0].cloud.status
   }' <<<"$INFO_JSON"
   ```

2. Load the token-safe direct-agent wrappers:

   ```bash
   source docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh
   agents_load_env
   ```

3. Query the topology Function through the direct-agent path. Store the
   raw response only under `.local/`:

   ```bash
   NODE_UUID="$(jq -r '.agents[0].nd' <<<"$INFO_JSON")"
   MACHINE_GUID="$(jq -r '.agents[0].mg' <<<"$INFO_JSON")"
   FUNCTION_NAME="topology:network-connections processes:by_name mode:aggregated sockets:inbound,outbound,listening,local protocols:ipv4_tcp,ipv6_tcp,ipv4_udp,ipv6_udp endpoints:by_ip"
   FUNCTION_ENCODED="$(printf '%s' "$FUNCTION_NAME" | jq -sRr @uri)"
   OUT="$(agents_audit_dir)/network-connections-aggregated-live.json"

   agents_query_agent \
     --node "$NODE_UUID" \
     --host "$AGENT_HOST" \
     --machine-guid "$MACHINE_GUID" \
     GET "/api/v3/function?function=${FUNCTION_ENCODED}&timeout=120000&last=200" \
     > "$OUT"
   ```

4. Print a sanitized response summary:

   ```bash
   jq '{
     status,
     type,
     schema_version: .data.schema_version,
     actor_rows: .data.actors.rows,
     link_rows: .data.links.rows,
     correlation_point_rows: .data.correlation.points.rows,
     correlation_claim_rows: .data.correlation.claims.rows
   }' "$OUT"
   ```

5. Verify the producer's link layout tokens:

   ```bash
   jq '.data.types.link_types
     | with_entries({
         key: .key,
         value: {
           label: .value.presentation.label,
           color_slot: .value.presentation.color_slot,
           line_style: .value.presentation.line_style,
           width: .value.presentation.width,
           variable: .value.presentation.variable,
           layout: .value.presentation.layout
         }
       })' "$OUT"
   ```

6. Verify correlation rule wiring without printing endpoint rows:

   ```bash
   jq '.data.correlation.rules
     | with_entries({
         key: .key,
         value: {
           action: .value.action,
           priority: .value.priority,
           key_space: .value.key_space,
           point_actor_types: .value.point_actor_types,
           claim_actor_types: .value.claim_actor_types,
           correlation_link_types: .value.correlation_link_types,
           output_link_type: .value.output_link_type
         }
       })' "$OUT"
   ```

7. Count graph links by type from the compact table:

   ```bash
   jq -r '
   .data as $d
   | def col($table; $name):
       ($table.columns | map(.id) | index($name)) as $i
       | $table.values[$i];
     def v($c; $i):
       if $c.codec == "const" then $c.value
       elif $c.codec == "values" then $c.values[$i]
       elif $c.codec == "dict" then $c.values[$c.indexes[$i]]
       else null end;
     (col($d.links; "type")) as $typeCol
   | (col($d.links; "socket_count")) as $socketCol
   | [range(0; $d.links.rows)
      | {type: v($typeCol; .), socket_count: (v($socketCol; .) // 0)}]
   | group_by(.type)
   | map({type: .[0].type, links: length, sockets: (map(.socket_count) | add)})
   | sort_by(.type)
   | .[]
   | [.type, .links, .sockets]
   | @tsv
   ' "$OUT"
   ```

## Output

For the current network-connections v1 contract, expect these link
types:

- `endpoint_socket`: visible unresolved endpoint links, weakest
  strength, normal distance.
- `correlated_socket`: Cloud aggregator output after exact absorption,
  weakest strength, farthest distance.
- `socket`: local/resolved process links, stronger strength, farther
  distance, variable width by `socket_count`.
- `ownership`: graph-coherence node-to-process links, dotted/faded,
  normal strength, normal distance.

The `socket_exact` correlation rule should consume `endpoint_socket`
through `correlation_link_types` and emit `correlated_socket` through
`output_link_type`.

## Notes / gotchas

- The wrapper logs masked curl commands on stderr. Cloud tokens,
  per-agent bearers, node ids, machine GUIDs, and claim ids must not
  appear in stdout or committed files.
- Keep the raw Function response under `.local/`; it may contain
  hostnames, private addresses, and process names.
- A rendered graph can still look stretched even when the payload uses
  `endpoint_socket` with normal distance. That is a frontend force-layout
  issue, not proof that the backend emitted `farthest`.
- Compact topology tables require decoding the column codec before
  counting rows by type. Do not assume the high-cardinality rows are
  emitted as arrays of objects.

## Source guides

- [Topology producer skill](../SKILL.md)
- [Direct-agent operator skill](../../../../docs/netdata-ai/skills/query-netdata-agents/SKILL.md)
- [Direct Function calls](../../../../docs/netdata-ai/skills/query-netdata-agents/query-functions.md)
- [Direct topology calls](../../../../docs/netdata-ai/skills/query-netdata-agents/query-topology.md)
