# Find containers exposing a topology TCP port through Cloud

## Question

How can an assistant find the containers or pods that expose a specific
TCP port in `topology:network-connections` through Netdata Cloud,
without exposing Cloud tokens, raw labels, cgroup paths, or private IPs?

## Inputs

- `NODE_UUID`: the target node id.
- `PORT`: the TCP port to inspect.
- `NETDATA_CLOUD_TOKEN` and `NETDATA_CLOUD_HOSTNAME` in `<repo>/.env`.
- The node must expose `topology:network-connections`.

## Steps

1. Load the token-safe wrappers:

   ```bash
   source docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh
   agents_load_env
   ```

2. Query the topology Function through Cloud:

   ```bash
   mkdir -p .local/audits/query-netdata-cloud

   agents_call_function \
     --via cloud \
     --node "$NODE_UUID" \
     --function 'topology:network-connections%20processes:by_pid%20cgroup-paths:hide' \
     --body '{}' \
     > .local/audits/query-netdata-cloud/network-topology-port.json
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
   ' .local/audits/query-netdata-cloud/network-topology-port.json
   ```

## Output

Return only a sanitized table:

- Port and protocol.
- Process name and PID.
- Orchestrator, container name, namespace, pod, and workload when present.
- Socket count.

Do not paste Cloud tokens, node ids, cgroup paths, raw labels, private
IP addresses, or customer-identifying workload names into durable
artifacts unless explicitly approved.

## Notes / gotchas

- This relies on the `socket_ports` actor table emitted by
  `topology:network-connections`.
- `processes:by_pid` gives the best container attribution. The
  process-name view can merge same-name processes from different
  containers.
- `cgroup-paths:hide` keeps raw cgroup paths out of saved audit output.

## Source guides

- [Topology queries](../query-topology.md)
- [Generic Function invocation](../query-functions.md)
- [Direct-agent sibling how-to](../../query-netdata-agents/how-tos/find-containers-for-topology-port-direct.md)
