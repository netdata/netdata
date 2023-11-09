### Understand the alert

This alert calculates the average `cgroup CPU utilization` over the past 10 minutes in a Kubernetes cluster. If you receive this alert at the warning or critical levels, it means that your cgroup is heavily utilizing the available CPU resources.

### What does cgroup CPU utilization mean?

In Kubernetes, `cgroups` are a Linux kernel feature that helps to limit and isolate the resource usage (CPU, memory, disk I/O, etc.) of a collection of processes. The `cgroup CPU utilization` measures the percentage of available CPU resources consumed by the processes within a cgroup.

### Troubleshoot the alert

- Identify the over-utilizing cgroup

Check the alert message for the specific cgroup that is causing high CPU utilization.

- Determine the processes utilizing the most CPU resources in the cgroup

To find the processes within the cgroup with high CPU usage, you can use `systemd-cgtop` on the Kubernetes nodes:

```
systemd-cgtop -m -1 -p -n10
```

- Analyze the Kubernetes resource usage

Use `kubectl top` to get an overview of the resource usage in your Kubernetes cluster:

```
kubectl top nodes
kubectl top pods
```

- Investigate the Kubernetes events and logs

Examine the events and logs of the Kubernetes cluster and the specific resources that are causing the high CPU utilization.

```
kubectl get events --sort-by='.metadata.creationTimestamp'
kubectl logs <pod-name> -n <namespace> --timestamps -f
```

- Optimize the resource usage of the cluster

You may need to scale your cluster by adding more resources, adjusting the resource limits, or optimizing the application code to minimize CPU usage.

### Useful resources

1. [Overview of a Pod](https://kubernetes.io/docs/concepts/workloads/pods/)
2. [Assign CPU Resources to Containers and Pods](https://kubernetes.io/docs/tasks/configure-pod-container/assign-cpu-resource/)
