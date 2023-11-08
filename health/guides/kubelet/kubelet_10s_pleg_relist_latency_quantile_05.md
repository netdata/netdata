### Troubleshoot the alert

1. Check Kubelet logs
   To diagnose issues with the PLEG relist process, look at the Kubelet logs. The following command can be used to fetch the logs from the affected node:

   ```
   kubectl logs -n kube-system <node_name>
   ```

   Look for any error messages related to PLEG or container runtime.

2. Check container runtime status
   Monitor the health status and performance of the container runtime (e.g. Docker, containerd) by running the appropriate commands like `docker ps`, `docker info` or `ctr version` and `ctr info`. Check container runtime logs for any issues as well.

3. Inspect node resources
   Verify if the node is overloaded or under excessive pressure by checking the CPU, memory, disk, and network resources. Use tools like `top`, `vmstat`, `df`, and `iostat`. You can also use the Kubernetes `kubectl top node` command to view resource utilization on your nodes.

4. Limit maximum Pods per node
   To avoid overloading nodes in your cluster, consider limiting the maximum number of Pods that can run on a single node. You can follow these steps to update the max Pods value:

   - Edit the Kubelet configuration file (usually located at `/etc/kubernetes/kubelet.conf` or `/var/lib/kubelet/config.yaml`) on the affected node.
   - Change the value of the `maxPods` parameter to a more appropriate number. The default value is 110.
   - Restart the Kubelet service with `systemctl restart kubelet` or `service kubelet restart`.
   - Check the Kubelet logs to ensure the new value is effective.

5. Check Pod eviction thresholds
   Review the Pod eviction thresholds defined in the Kubelet configuration, which might cause Pods to be evicted due to resource pressure. Adjust the threshold values if needed.

6. Investigate Pods causing high relisting latency
   Analyze the Pods running on the affected node and identify any Pods that might be causing high PLEG relist latency. These could be Pods with a large number of containers or high resource usage. Consider optimizing or removing these Pods if they are not essential to your workload.

### Useful resources

1. [Kubelet CLI in Kubernetes official docs](https://kubernetes.io/docs/reference/command-line-tools-reference/kubelet/)
2. [PLEG mechanism explained in Redhat's blogspot](https://developers.redhat.com/blog/2019/11/13/pod-lifecycle-event-generator-understanding-the-pleg-is-not-healthy-issue-in-kubernetes/)
