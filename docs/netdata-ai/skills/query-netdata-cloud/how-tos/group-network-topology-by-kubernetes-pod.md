# Group network topology by Kubernetes pod through Cloud

## Question

How can an assistant find network topology process actors for one
Kubernetes namespace and summarize them by pod through Netdata Cloud,
without exposing Cloud tokens, node ids, pod labels, or cgroup paths?

## Inputs

- `NODE_UUID`: the target node id.
- `NAMESPACE`: the Kubernetes namespace to inspect.
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
     --function 'topology:network-connections' \
     --body '{"selections":{"group_by":["pid"]}}' \
     > .local/audits/query-netdata-cloud/network-topology-by-pod.json
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
   ' .local/audits/query-netdata-cloud/network-topology-by-pod.json
   ```

## Output

Return a sanitized summary:

- Namespace inspected.
- Pod count and process count.
- Per-pod process names and PIDs only when the user needs that detail.

Do not paste Cloud tokens, node ids, cgroup paths, raw labels, private
IP addresses, or customer-identifying pod names into durable artifacts.

## Notes / gotchas

- Canonical Kubernetes columns (`k8s_namespace`, `k8s_pod_name`,
  `k8s_workload`) do not require `labels:<pattern>`.
- `labels:<pattern>` controls only free-form actor labels. Its separator
  is `|`; commas are literal.
- The Agent payload exposes grouping metadata. If a Cloud consumer does
  not yet rebucket by `view.group_by`, summarize the actor table as shown.

## Source guides

- [Topology queries](../query-topology.md)
- [Generic Function invocation](../query-functions.md)
- [Direct-agent sibling how-to](../../query-netdata-agents/how-tos/group-network-topology-by-kubernetes-pod-direct.md)
