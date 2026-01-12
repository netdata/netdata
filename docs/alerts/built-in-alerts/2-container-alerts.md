# 11.2 Container and Orchestration Alerts

Container and orchestration alerts address the unique monitoring requirements of dynamic infrastructure. These alerts rely on collectors specific to container runtimes and orchestrators.

:::note

Container alerts require the appropriate collector to be enabled and collecting data. Check the **Collectors** tab in the dashboard to verify your container collectors are running.

:::

## 11.2.1 Docker Container Alerts

Docker container alerts monitor both the runtime state of containers and their resource consumption.

### docker_container_status

Tracks the running state of each container. A critical alert fires when a previously running container stops, which may indicate crashes, health check failures, or manual stops.

**Context:** `docker.container_state`
**Thresholds:** CRIT not running

### docker_container_cpu_usage

Tracks CPU utilization within the container's configured limits. Helps identify containers approaching resource ceilings.

**Context:** `cgroup.cpu_limit`
**Thresholds:** WARN > 80%, CRIT > 95%

### docker_container_mem_usage

Tracks memory consumption against container limits. Prevents noisy neighbor problems from affecting other containers.

**Context:** `cgroup.mem_usage`
**Thresholds:** WARN > 80%, CRIT > 95%

### docker_container_ooms

Specifically tracks out-of-memory kills. When the Linux OOM killer terminates a container process, this alert fires immediately.

**Context:** `docker.container_health_status`
**Thresholds:** CRIT > 0

### docker_container_restarts

Monitors restart counts which indicate application stability.

**Context:** `docker.container_state`
**Thresholds:** WARN > 3/hour, CRIT > 10/hour

## 11.2.2 Kubernetes Pod Alerts

Kubernetes pod alerts require the Netdata Kubernetes collector and provide visibility into pod health from the cluster perspective.

### k8s_pod_ready

Monitors the Kubernetes readiness probe status. Pods that fail readiness checks are not routable from Services.

**Context:** `k8s_state.pod_condition`
**Thresholds:** CRIT not ready

### k8s_pod_restarting

Container restart counts indicate application stability. Uses thresholds of three and ten restarts per hour.

**Context:** `k8s_state.pod_container_restarts`
**Thresholds:** WARN > 3, CRIT > 10

### k8s_pod_not_scheduled

Catches pods stuck in Pending state because no node has capacity to satisfy resource requests.

**Context:** `k8s_state.pod_condition`
**Thresholds:** CRIT > 5 minutes

### k8s_container_cpu_limits

Fires when containers reach 90% of their configured CPU limits, indicating the limit may be constraining performance.

**Context:** `k8s.cgroup.cpu_limit`
**Thresholds:** WARN > 90% of limit

### k8s_container_mem_limits

Fires when containers reach 90% of their configured memory limits.

**Context:** `k8s.cgroup.mem_usage`
**Thresholds:** WARN > 90% of limit

## 11.2.3 Kubernetes Node Alerts

### node_not_ready

Kubernetes node ready status. A not-ready node cannot schedule new workloads and may indicate hardware or system problems.

**Context:** `k8s_state.node_condition`
**Thresholds:** CRIT not ready

### node_disk_pressure

Node disk pressure indicates that kubelet is managing pressure on local storage.

**Context:** `k8s_state.node_condition`
**Thresholds:** WARN > 0

### node_memory_pressure

Node memory pressure indicates the node is approaching memory exhaustion.

**Context:** `k8s_state.node_condition`
**Thresholds:** WARN > 0

## Related Sections

- [11.1 Application Alerts](3-application-alerts.md) - Database, web server, cache, and message queue alerts
- [11.3 Hardware Alerts](5-hardware-alerts.md) - Physical server and storage device alerts
- [11.4 Network Alerts](4-network-alerts.md) - Network interface and protocol monitoring
- [11.5 System Resource Alerts](1-system-resource-alerts.md) - CPU, memory, disk, and load alerts