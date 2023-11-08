### Understand the alert

This alert indicates a potential `received packets storm` in your Kubernetes (k8s) cluster's network on a cgroup (control group) network interface. A received packets storm occurs when the average number of received packets in the last 10 seconds significantly exceeds the rate over the last minute.

### What is a cgroup?

A `cgroup` (control group) is a Linux kernel feature to limit, account, and isolate resource usage (CPU, memory, disk I/O, etc.) for a process or a group of processes. In Kubernetes, cgroups are used to manage resources for each container within a pod.

### What is a received packets storm?

A received packets storm occurs when the average number of received packets on a network interface becomes significantly higher than the recent background rate. This can cause network congestion, increased latency, or even denial of service, affecting the performance of services running on the Kubernetes cluster.

### Troubleshoot the alert

1. Inspect overall network activity on the affected node(s):

   Use the `iftop` command to monitor network activity on the host in real-time:

   ```
   sudo iftop
   ```

   If you don't have `iftop` installed, install it before running the command.

2. Identify the container(s) responsible for the high packet rate:

   To list the running container(s) and their associated cgroups, run the following command:

   ```
   sudo kubectl get pods --all-namespaces -o jsonpath='{range.items[*]}{.metadata.namespace}:{.metadata.name}{"\t"}{.status.containerStatuses[].containerID}{"\t"}{"cgroup id: "}{"\n"}{end}'
   ```

   Now, check the network interface statistics for each container:

   ```
   cat /sys/fs/cgroup/net_cls,net_prio/net_cls.classid
   ```

3. Investigate the cause:

   - Inspect the logs of the affected container(s) for any errors or unusual activity:

     ```
     sudo kubectl logs -f <pod-name> -c <container-name> -n <namespace>
     ```

   - Check if there are any misconfigurations or if network rate limits are not set correctly in the Kubernetes Deployment, StatefulSet, or DaemonSet manifest.

4. Mitigate the issue:

   - If unnecessary traffic is causing the packet storm, consider implementing network throttling or limiting the rate at which packets are generated or received by the container(s).

   - If the issue is caused by a bug or misconfiguration, fix the problem and redeploy the affected component(s).

### Useful resources

1. [Kubernetes Cgroups documentation](https://kubernetes.io/docs/tasks/debug-application-cluster/debug-application/#debugging-the-kernel-cgroups-and-kubernetes-primitives)
2. [Monitoring and Visualizing Network Bandwidth on Linux](https://www.tecmint.com/linux-network-bandwidth-monitoring-tools/)
3. [Networking in Kubernetes](https://kubernetes.io/docs/concepts/cluster-administration/networking/)
