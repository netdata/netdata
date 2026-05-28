# Find containers exposing a topology TCP port through a direct Agent call

## Question

How can an assistant find the containers or pods that expose a specific
TCP port in `topology:network-connections` through the direct Agent API,
without exposing Cloud tokens, agent bearers, node ids, machine GUIDs,
raw labels, cgroup paths, or private IPs?

## Inputs

- `NODE_UUID`: the target node id.
- `MACHINE_GUID`: the target agent machine GUID.
- `AGENT_HOST`: the direct agent host and port, for example
  `127.0.0.1:19999`.
- `PORT`: the TCP port to inspect.
- `NETDATA_CLOUD_TOKEN` and `NETDATA_CLOUD_HOSTNAME` in `<repo>/.env`.
- The node must expose `topology:network-connections`.

## Steps

1. Load the token-safe direct-agent wrappers:

   ```bash
   source docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh
   agents_load_env
   ```

2. Query the topology Function through the direct-agent path:

   ```bash
   mkdir -p .local/audits/query-netdata-agents

   agents_call_function \
     --via agent \
     --node "$NODE_UUID" \
     --host "$AGENT_HOST" \
     --machine-guid "$MACHINE_GUID" \
     --function 'topology:network-connections' \
     --body '{"selections":{"group_by":["pid"]}}' \
     > .local/audits/query-netdata-agents/network-topology-port-agent.json
   ```

3. Decode actors and socket ports, then join port rows to process actors:

   ```bash
   jq --argjson port "$PORT" '
     def col($table; $id):
       ($table.columns | map(.id) | index($id)) as $idx
       | if $idx == null then error("missing column: " + $id)
         else $table.values[$idx] as $enc
         | if $enc.codec == "const" then [range(0; $table.rows) | $enc.value]
           elif $enc.codec == "values" then $enc.values
           elif $enc.codec == "dict" then [$enc.indexes[] as $i | $enc.values[$i]]
           else error("unsupported codec: " + ($enc.codec // "null"))
           end
         end;

     .data.actors as $actors
     | .data.tables.actor.socket_ports.table as $ports
     | col($actors; "type") as $type
     | col($actors; "display_name") as $display
     | col($actors; "pid") as $pid
     | col($actors; "cgroup_name") as $container
     | col($actors; "docker_container_name") as $docker_name
     | col($actors; "k8s_namespace") as $namespace
     | col($actors; "k8s_pod_name") as $pod
     | col($actors; "k8s_workload") as $workload
     | col($actors; "orchestrator") as $orchestrator
     | col($ports; "actor") as $port_actor
     | col($ports; "port") as $port_value
     | col($ports; "protocol") as $protocol
     | col($ports; "socket_count") as $socket_count
     | [range(0; $ports.rows)
        | select($port_value[.] == $port)
        | $port_actor[.] as $actor
        | select($type[$actor] == "process")
        | {
            port: $port,
            protocol: $protocol[.],
            sockets: $socket_count[.],
            process: $display[$actor],
            pid: $pid[$actor],
            orchestrator: $orchestrator[$actor],
            container: ($container[$actor] // $docker_name[$actor]),
            namespace: $namespace[$actor],
            pod: $pod[$actor],
            workload: $workload[$actor]
          }]
     | sort_by(.orchestrator, .namespace, .pod, .container, .process, .pid)
   ' .local/audits/query-netdata-agents/network-topology-port-agent.json
   ```

## Output

Return only a sanitized table:

- Port and protocol.
- Process name and PID.
- Orchestrator, container name, namespace, pod, and workload when present.
- Socket count.

Do not paste Cloud tokens, per-agent bearers, node ids, machine GUIDs,
cgroup paths, raw labels, private IP addresses, or customer-identifying
workload names into durable artifacts unless explicitly approved.

## Notes / gotchas

- This relies on the `socket_ports` actor table emitted by
  `topology:network-connections`.
- `group_by:pid` gives the best container attribution.
- Raw cgroup paths are present in `group_by:pid`; do not copy them into durable artifacts.
- Use the Cloud sibling how-to when direct Agent access is not available.

## Source guides

- [Direct topology queries](../query-topology.md)
- [Direct Function calls](../query-functions.md)
- [Cloud sibling how-to](../../query-netdata-cloud/how-tos/find-containers-for-topology-port.md)
