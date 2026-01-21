# 6: Stock Alerts

Netdata ships with a comprehensive library of pre-configured alerts covering system resources, applications, containers, and hardware monitoring. These alerts are enabled by default and require no configuration to provide immediate visibility into common problems.

:::note

Stock alerts follow the principle of conservative defaults. They are tuned to detect genuine issues while avoiding noise from normal operational variation. However, every environment is unique—use these as a starting point and adjust thresholds based on your specific requirements.

:::

## Here

| Section | Focus Area |
|---------|------------|
| **[6.1 System Resource Alerts](#61-system-resource-alerts)** | CPU, memory, disk space, network, load averages |
| **[6.2 Container Alerts](#62-container-and-orchestration-alerts)** | Docker containers, Kubernetes pods, cgroup metrics |
| **[6.3 Application Alerts](#63-application-alerts)** | MySQL, PostgreSQL, Redis, nginx, Apache, and more |
| **[6.4 Network Alerts](#64-network-and-connectivity-alerts)** | Interface errors, packet drops, bandwidth utilization |
| **[6.5 Hardware Alerts](#65-hardware-and-sensor-alerts)** | RAID controllers, UPS battery, SMART disk status |

## 6.1 System Resource Alerts

System resource alerts cover the fundamental building blocks of any server: CPU, memory, disk, network, and load. These alerts apply to every Netdata node and provide the baseline monitoring that every infrastructure should have.

:::note

System resource alerts are enabled by default on all Netdata installations. These alerts are foundational and apply to all nodes regardless of their role.

:::

### CPU Alerts

The CPU alerts monitor utilization, saturation, and scheduling behavior.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **10min_cpu_usage** | Primary CPU alert tracking aggregate usage over 10-minute window | `system.cpu` | WARN > 75%, CRIT > 85% |
| **10min_cpu_iowait** | Monitors time spent waiting for I/O operations | `system.cpu` | WARN > 20%, CRIT > 40% |
| **20min_steal_cpu** | Tracks time stolen by other VMs in cloud/VPS environments | `system.cpu` | WARN > 10%, CRIT > 20% |

### Memory Alerts

Memory monitoring balances three competing concerns: availability for new allocations, pressure on cached data, and swapping activity.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **ram_in_use** | Tracks utilization percentage | `system.ram` | WARN > 80%, CRIT > 90% |
| **ram_available** | Monitors actual available memory | `mem.available` | WARN < 15%, CRIT < 10% |
| **oom_kill** | Number of OOM kills in the last 30 minutes | `mem.oom_kill` | WARN > 0 |
| **30min_ram_swapped_out** | Percentage of RAM swapped in last 30 minutes | `mem.swapio` | WARN > 20%, CRIT > 30% |
| **used_swap** | Swap memory utilization | `mem.swap` | WARN > 80%, CRIT > 90% |

### Disk Space Alerts

Disk space monitoring addresses space availability and inode exhaustion.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **disk_space_usage** | Tracks percentage of allocated space across filesystems | `disk.space` | WARN > 80%, CRIT > 90% |
| **disk_inode_usage** | Tracks inode exhaustion for filesystems with many small files | `disk.inodes` | WARN > 80%, CRIT > 90% |
| **10min_disk_utilization** | Average disk utilization over 10 minutes | `disk.util` | WARN > 68.6%, CRIT > 98% |
| **10min_disk_backlog** | Average disk backlog over 10 minutes | `disk.backlog` | WARN > 3500ms, CRIT > 5000ms |
| **out_of_disk_space_time** | Estimated hours until disk runs out of space | `disk.space` | WARN < 48h, CRIT < 24h |
| **out_of_disk_inodes_time** | Estimated hours until disk runs out of inodes | `disk.inodes` | WARN < 48h, CRIT < 24h |

### Load Average Alerts

System load alerts for capacity planning.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **load_cpu_number** | Number of active CPU cores in the system | `system.load` | Calculation |
| **load_average_15** | System load average for past 15 minutes | `system.load` | WARN > 175% of CPUs |
| **load_average_5** | System load average for past 5 minutes | `system.load` | WARN > 350% of CPUs |
| **load_average_1** | System load average for past 1 minute | `system.load` | WARN > 700% of CPUs |

### Process Alerts

Process and file descriptor monitoring.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **active_processes** | System PID space utilization | `system.active_processes` | WARN > 85%, CRIT > 90% |
| **system_file_descriptors_utilization** | System file descriptors utilization | `system.file_descriptors` | WARN > 80%, CRIT > 90% |

### Kernel and Synchronization Alerts

Low-level kernel and time synchronization alerts.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **lowest_entropy** | Available entropy for cryptographic operations | `kernel.entropy` | WARN < 100 |
| **system_clock_sync_state** | Clock synchronization status | `timex.ntp_offset` | Various |
| **sync_freq** | System clock drift frequency adjustment | `timex.tick_mode` | Various |
| **semaphores_used** | System V semaphores in use | `ipc.semaphore` | Various |
| **semaphore_arrays_used** | System V semaphore arrays in use | `ipc.semaphore_array` | Various |

## 6.2 Container and Orchestration Alerts

Container and orchestration alerts address the unique monitoring requirements of dynamic infrastructure.

:::note

Container alerts require the appropriate collector to be enabled. Check the **Collectors** tab in the dashboard to verify your container collectors are running.

:::

### Docker Container Alerts

Docker container alerts monitor both the runtime state and resource consumption.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **docker_container_unhealthy** | Tracks container health status from Docker daemon | `docker.container_health_status` | CRIT != 0 |
| **docker_container_down** | Tracks running state of each container | `docker.container_state` | CRIT not running |

### Kubernetes Pod Alerts

Kubernetes pod alerts require the Netdata Kubernetes collector.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **cgroup_10min_cpu_usage** | Container CPU utilization over 10 minutes | `cgroup.cpu` | WARN/CRIT based on limits |
| **cgroup_ram_in_use** | Container memory utilization | `cgroup.mem_usage` | WARN/CRIT based on limits |
| **k8s_cgroup_10min_cpu_usage** | K8s container CPU usage approaching limit | `cgroup.cpu_limit` | WARN > 80%, CRIT > 90% |
| **k8s_cgroup_ram_in_use** | K8s container memory usage approaching limit | `cgroup.mem_usage` | WARN > 80%, CRIT > 90% |

### Kubernetes State Alerts

Alerts for Kubernetes workload health and deployment status.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **k8s_state_cronjob_last_execution_failed** | CronJob last execution status | `k8s_state.cronjob` | CRIT failed |
| **k8s_state_deployment_condition_available** | Deployment availability condition | `k8s_state.deployment` | CRIT not available |

### Kubernetes Node (Kubelet) Alerts

Kubernetes node alerts available through kubelet metrics.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **kubelet_node_config_error** | Kubelet node configuration error status | `k8s_kubelet.kubelet_node_config_error` | CRIT > 0 (config error) |
| **kubelet_10s_pleg_relist_latency_quantile_05** | PLEG relist latency 50th percentile | `k8s_kubelet.pleg` | WARN/C |
| **kubelet_10s_pleg_relist_latency_quantile_09** | PLEG relist latency 90th percentile | `k8s_kubelet.pleg` | WARN/C |
| **kubelet_10s_pleg_relist_latency_quantile_099** | PLEG relist latency 99th percentile | `k8s_kubelet.pleg` | WARN/C |
| **kubelet_1m_pleg_relist_latency_quantile_05** | PLEG relist latency 50th percentile (1m avg) | `k8s_kubelet.pleg` | WARN/C |
| **kubelet_1m_pleg_relist_latency_quantile_09** | PLEG relist latency 90th percentile (1m avg) | `k8s_kubelet.pleg` | WARN/C |
| **kubelet_1m_pleg_relist_latency_quantile_099** | PLEG relist latency 99th percentile (1m avg) | `k8s_kubelet.pleg` | WARN/C |
| **kubelet_operations_error** | Kubelet operations error count | `k8s_kubelet.operations` | WARN > 0 |
| **kubelet_token_requests** | Kubelet token request metrics | `k8s_kubelet.token` | Various |

## 6.3 Application Alerts

Application alerts cover common database, web server, cache, and message queue technologies.

:::note

To see more application alerts, visit the [Collectors list](/docs/collecting-metrics/collectors-configuration) on learn.netdata.cloud.

:::

### Database Alerts

| Alert | Description |
|-------|-------------|
| [MySQL & MariaDB](/docs/src/go/plugin/go.d/collector/mysql/README.md) | Replication lag, slow queries, connection utilization |
| [PostgreSQL](/docs/src/go/plugin/go.d/collector/postgres/README.md) | Deadlocks, connection utilization, replication lag |
| [MongoDB](/docs/src/go/plugin/go.d/collector/mongodb/README.md) | Query performance, connection pooling |
| [Redis](/docs/src/go/plugin/go.d/collector/redis/README.md) | Memory pressure, eviction rates |

### Web Server Alerts

| Alert | Description |
|-------|-------------|
| [nginx](/docs/src/go/plugin/go.d/collector/nginx/README.md) | Request handling, connection states |
| [Apache](/docs/src/go/plugin/go.d/collector/apache/README.md) | Request processing, worker utilization |

### Message Queue Alerts

| Alert | Description |
|-------|-------------|
| [RabbitMQ](/docs/src/go/plugin/go.d/collector/rabbitmq/README.md) | Queue depth, message rates |
| [Kafka](/docs/src/go/plugin/go.d/collector/prometheus/integrations/kafka.md) | Consumer lag, partition health |

### Proxy & Load Balancer Alerts

| Alert | Description |
|-------|-------------|
| [HAProxy](/docs/src/go/plugin/go.d/collector/haproxy/README.md) | Request rates, backend health |
| [Traefik](/docs/src/go/plugin/go.d/collector/traefik/README.md) | Request handling, middleware health |

## 6.4 Network and Connectivity Alerts

Network alerts focus on endpoints and services rather than interface statistics.

:::note

Network connectivity alerts require specific endpoints to be configured. Add the hosts, ports, or URLs you want to monitor using the appropriate collector configuration.

:::

### Ping and Latency Monitoring

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **ping_host_latency** | Tracks round-trip time | `ping.host_rtt` | WARN > 500ms, CRIT > 1000ms |
| **ping_packet_loss** | Measures percentage of packets without responses | `ping.host_packet_loss` | WARN > 5%, CRIT > 10% |
| **ping_host_reachable** | Tracks host reachability status | `ping.host_packet_loss` | CRIT == 0 (not reachable) |

### Port and Service Monitoring

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **portcheck_connection_fails** | Attempts connection to specified port | `portcheck.status` | CRIT > 40% failed |
| **portcheck_connection_timeouts** | Tracks connection establishment time | `portcheck.status` | WARN > 10% timeout, CRIT > 40% timeout |
| **portcheck_service_reachable** | Tracks port/service reachability status | `portcheck.status` | CRIT < 75% success |

### SSL Certificate Monitoring

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **x509check_days_until_expiration** | Monitors certificate validity period | `x509check.time_until_expiration` | WARN < 14 days, CRIT < 7 days |
| **x509check_revocation_status** | Tracks SSL/TLS certificate revocation status | `x509check.revocation_status` | CRIT revoked |

### DNS Monitoring

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **dns_query_query_status** | Fires when DNS resolution fails entirely | `dns_query.query_status` | WARN != 1 (failed) |

### HTTP Endpoint Monitoring

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **httpcheck_web_service_bad_status** | Tracks non-2xx responses | `httpcheck.status` | WARN >= 10% bad status, CRIT >= 40% bad status |
| **httpcheck_web_service_timeouts** | Monitors HTTP request timeouts | `httpcheck.status` | WARN >= 10% timeouts, CRIT >= 40% timeouts |
| **httpcheck_web_service_up** | Tracks overall HTTP service availability | `httpcheck.status` | CRIT not responding |
| **httpcheck_web_service_bad_content** | Tracks unexpected HTTP response content | `httpcheck.status` | CRIT bad content |

## 6.5 Hardware and Sensor Alerts

Hardware monitoring provides visibility into infrastructure that is often neglected until failure occurs.

:::note

Hardware monitoring requires collector support for your specific hardware. Check that IPMI, SMART, and sensor collectors are enabled for your platform.

:::

### RAID Monitoring

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **adaptec_raid_logical_device_status** | Fires when array has lost redundancy | `adaptecraid.logical_device_status` | CRIT > 0 (degraded) |
| **adaptec_raid_physical_device_state** | Tracks individual disk failures within RAID arrays | `adaptecraid.physical_device_state` | CRIT > 0 (failed) |

### Power Monitoring

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **apcupsd_ups_battery_charge** | Monitors remaining battery capacity | `apcupsd.ups_battery_charge` | WARN < 25%, CRIT < 10% |
| **apcupsd_ups_status_onbatt** | Fires when mains power fails | `apcupsd.ups_status` | CRIT > 0 (on battery) |
| **apcupsd_ups_load_capacity** | Tracks UPS load percentage | `apcupsd.ups_load_capacity_utilization` | WARN > 80%, CRIT > 90% |
| **upsd_ups_battery_charge** | NUT UPS battery charge monitoring | `upsd.ups_battery_charge` | WARN < 25%, CRIT < 10% |
| **upsd_10min_ups_load** | NUT UPS load percentage | `upsd.ups_load` | WARN > 80%, CRIT > 90% |

### BMC/IPMI Monitoring

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **ipmi_sensor_state** | Monitors BMC sensor states | `ipmi.sensor_state` | WARN/CRIT based on sensor type |
| **ipmi_events** | Tracks IPMI event log entries | `ipmi.events` | Any events present |

## Related Sections

- **[Chapter 2: Creating and Managing Alerts](../creating-alerts-pages/index.md)** - Creating and editing alerts via configuration files
- **[Chapter 12: Best Practices for Designing Effective Alerts](../best-practices/index.md)** - Best practices for designing effective alerts
- **[Chapter 3: Alert Configuration Syntax](../alert-configuration-syntax/index.md)** - Alert configuration syntax reference