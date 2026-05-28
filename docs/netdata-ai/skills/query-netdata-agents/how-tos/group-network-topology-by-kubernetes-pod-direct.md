# Group network topology by Kubernetes pod through a direct Agent call

## Question

How can an assistant find network topology process actors for one
Kubernetes namespace and summarize them by pod through the direct Agent
API, without exposing Cloud tokens, agent bearers, node ids, machine
GUIDs, pod labels, or cgroup paths?

## Inputs

- `NODE_UUID`: the target node id.
- `MACHINE_GUID`: the target agent machine GUID.
- `AGENT_HOST`: the direct agent host and port, for example
  `127.0.0.1:19999`.
- `NAMESPACE`: the Kubernetes namespace to inspect.
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
     > .local/audits/query-netdata-agents/network-topology-by-pod-agent.json
   ```

3. Decode the compact actor table and summarize process actors by pod:

   ```bash
   jq --arg namespace "$NAMESPACE" '
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
     | col($actors; "type") as $type
     | col($actors; "pid") as $pid
     | col($actors; "display_name") as $display
     | col($actors; "k8s_namespace") as $ns
     | col($actors; "k8s_pod_name") as $pod
     | col($actors; "k8s_workload") as $workload
     | col($actors; "orchestrator") as $orchestrator
     | [range(0; $actors.rows)
        | select($type[.] == "process")
        | select($ns[.] == $namespace)
        | {
            pod: $pod[.],
            workload: $workload[.],
            orchestrator: $orchestrator[.],
            pid: $pid[.],
            process: $display[.]
          }]
     | group_by(.pod)
     | map({
         pod: .[0].pod,
         workload: .[0].workload,
         process_count: length,
         processes: map({pid, process}) | sort_by(.process, .pid)
       })
   ' .local/audits/query-netdata-agents/network-topology-by-pod-agent.json
   ```

## Output

Return a sanitized summary:

- Namespace inspected.
- Pod count and process count.
- Per-pod process names and PIDs only when the user needs that detail.

Do not paste Cloud tokens, per-agent bearers, node ids, machine GUIDs,
cgroup paths, raw labels, private IP addresses, or customer-identifying
pod names into durable artifacts.

## Notes / gotchas

- The wrapper resolves and caches the per-agent bearer under
  `.local/audits/query-netdata-agents/bearers/`. That directory is
  gitignored and must remain local.
- - Canonical Kubernetes columns do not require `labels:<pattern>`.
- Use the same `NODE_UUID`, `MACHINE_GUID`, and `AGENT_HOST` tuple from
  the same Agent identity response.

## Source guides

- [Direct topology queries](../query-topology.md)
- [Direct Function calls](../query-functions.md)
- [Cloud sibling how-to](../../query-netdata-cloud/how-tos/group-network-topology-by-kubernetes-pod.md)
