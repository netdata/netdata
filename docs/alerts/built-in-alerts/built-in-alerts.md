# 11. Built-In Alerts Reference

Netdata ships with hundreds of pre-configured stock alerts that cover common monitoring scenarios across systems, containers, databases, and applications. This chapter catalogs these alerts organized by category to help you understand what Netdata monitors out of the box and identify which alerts may require adjustment or replacement for your environment.

Stock alerts are installed in `/usr/lib/netdata/conf.d/health.d/` and are designed to be safe, conservative defaults that work across diverse infrastructures. They are updated with each Netdata release to incorporate new collectors, improve thresholds, and address false positives reported by the community. Understanding what these alerts cover—and where they may not match your requirements—is essential for building an effective alerting strategy.

## 11.1 Understanding Stock Alert Design

Before diving into specific alerts, it helps to understand the philosophy behind their design.

Stock alerts prioritize reliability over sensitivity. They are calibrated to minimize false positives while still catching genuine problems early. This means some thresholds may feel conservative for high-traffic environments, while others may be too aggressive for resource-constrained deployments. Neither extreme is wrong—they represent different priorities, and both can be adjusted.

The stock alert files use a modular structure that mirrors the Netdata collector organization. System-level alerts live in `health.d/system.conf`, database alerts in `health.d/mysql.conf`, and so on. This modularity makes it easy to identify which file controls which alerts and selectively override only the sections that need modification.

When evaluating stock alerts for your environment, ask three questions. First, does this alert apply to your infrastructure at all? An alert for Redis makes no sense if you do not run Redis. Second, are the thresholds appropriate? A CPU alert at 80% may suit a database server but could be too sensitive for a log aggregator. Third, does the notification destination match your needs? Stock alerts send to the default notification methods, which may require reconfiguration for your team's workflows.

## 11.2 System Resource Alerts

System resource alerts cover the fundamental building blocks of any server: CPU, memory, disk, network, and load. These alerts apply to every Netdata node and provide the baseline monitoring that every infrastructure should have.

### CPU Alerts

The CPU alerts monitor utilization, saturation, and scheduling behavior. The primary alert, `cpu_core_cumulative_100ms_percentage`, tracks aggregate CPU usage across all cores as a percentage of available capacity. This alert uses a 100ms window to capture brief spikes that might indicate transient load bursts without triggering on normal variation.

For CPU saturation, the `system.cpu_suppression` alert measures time spent waiting in the scheduler queue. This metric catches problems that pure utilization percentage misses—a server at 70% CPU with high queue depth is more stressed than a server at 90% with empty queues. The alert thresholds of 5 seconds for warning and 10 seconds for critical are calibrated to catch genuine saturation before it impacts service response times.

CPU frequency scaling can also indicate problems, particularly on systems where dynamic scaling is used for power management. The `cpu_core_frequency` alert fires when CPU frequency drops below 90% of the nominal maximum, which may indicate thermal throttling or power-saving behavior that affects performance.

### Memory Alerts

Memory monitoring requires balancing three competing concerns: availability for new allocations, pressure on cached data, and swapping activity that indicates the working set exceeds physical memory.

The `ram_available` alert monitors actual available memory, which accounts for free memory, reclaimable caches, and reserved allocations. The thresholds at 20% warning and 10% critical provide headroom for memory allocation spikes while catching genuine exhaustion before it causes out-of-memory conditions.

The `ram_in_use` alert takes the complementary perspective, tracking utilization percentage. This alert is useful for identifying workloads that consistently run near capacity, even when available memory appears adequate. A server with 85% utilization has less headroom for traffic spikes than one at 60%, even if both have identical available memory figures.

For systems with swap configured, `swap_fill` monitors swap space usage. Significant swap activity indicates that the working set exceeds physical memory, which typically causes severe performance degradation. The alert thresholds of 80% warning and 95% critical catch situations where swap exhaustion may be imminent.

The `low_memory_endanger` alert provides the most sensitive early warning of memory exhaustion. By raising a critical alert when available memory drops below 100MB, this alert gives operators a final opportunity to investigate before the OOM killer begins terminating processes.

### Disk Space Alerts

Disk space monitoring addresses two distinct concerns: the actual availability of storage and the exhaustion of inodes on filesystems that use inode-based allocation.

The `disk_space_usage` alert tracks percentage of allocated space across all mounted filesystems. The thresholds of 80% warning and 90% critical provide time to plan capacity expansions while preventing critical issues from occurring during normal operations. This alert should be tuned for each filesystem based on growth rate and time required for procurement and installation.

For filesystems with many small files, `disk_inode_usage` becomes the more relevant constraint. Filesystems can run out of inodes before they run out of space, particularly on systems that create temporary files aggressively. The same 80%/90% thresholds apply, and monitoring both space and inode usage provides complete visibility into storage constraints.

The `disk_full` alert serves as a final backstop at 95% space usage. This alert is intentionally more aggressive than the warning threshold because the consequences of complete disk failure are severe for both application data and log files that may be needed for troubleshooting.

### Disk I/O Alerts

Disk I/O monitoring catches performance problems that pure capacity metrics miss. A disk may have plenty of space yet be unable to handle the throughput demands placed upon it.

The `disk_busy` alert measures the percentage of time the disk spends processing I/O requests. This metric, sometimes called utilization, indicates whether the disk can keep up with the workload. At 80% busy, most disks begin to show queuing delays; at 95%, performance degradation is nearly certain.

Disk utilization alone does not distinguish between read and write heavy workloads, nor does it account for the difference between sequential and random access patterns. The companion alerts `disk_io` for operation rate and `disk_wait` for average latency provide additional diagnostic context. A disk with high utilization but low latency may be handling sequential streaming workloads effectively; the same utilization with high latency indicates queuing problems that warrant investigation.

The relationship between these metrics and their thresholds varies by storage type. SSDs can sustain higher utilization without queuing delays than HDDs, and NVMe devices outpace SATA SSDs by an order of magnitude. Consider adjusting these thresholds based on your storage technology.

### Network Interface Alerts

Network interface alerts focus on error conditions rather than throughput, because throughput limits are typically negotiated at the link level while errors indicate genuine problems.

The `network_interface_errors` alert fires when any interface reports errors, counters increment, or checksum failures occur. Even a single error may indicate cable problems, duplex mismatches, or hardware degradation. The thresholds at one warning and ten critical balance sensitivity with the reality that some environments see occasional errors without impact.

Packet drops, monitored by the `network_interface_drops` alert, indicate that the interface buffer filled faster than packets could be processed. Dropped receive packets may indicate incoming traffic exceeding processing capacity; dropped transmit packets indicate local buffer exhaustion. Either condition can cause application-visible problems even when underlying network connectivity remains intact.

The `network_interface_speed` alert detects when link negotiation results in speeds below the expected maximum. This catches situations where cables, switches, or NICs negotiated to half-duplex or lower speeds, which always indicates a configuration problem somewhere in the path.

## 11.3 Container and Orchestration Alerts

Container and orchestration alerts address the unique monitoring requirements of dynamic infrastructure. These alerts rely on collectors specific to container runtimes and orchestrators.

### Docker Container Alerts

Docker container alerts monitor both the runtime state of containers and their resource consumption.

The `docker_container_status` alert tracks the running state of each container. A critical alert fires when a previously running container stops, which may indicate crashes, health check failures, or manual stops. This alert distinguishes between intentional stops and unexpected failures based on whether the stop was preceded by restart counts.

Resource consumption alerts for containers mirror system-level alerts but scoped to individual containers. The `docker_container_cpu_usage` alert at 80% warning and 95% critical tracks CPU utilization within the container's configured limits. Similarly, `docker_container_mem_usage` tracks memory consumption against container limits. These alerts help identify containers that are approaching their resource ceilings and may be throttled or killed.

The `docker_container_ooms` alert specifically tracks out-of-memory kills. When the Linux OOM killer terminates a container process, this alert fires immediately. OOM kills are serious events that typically cause service interruption and may indicate either application memory leaks or incorrectly sized resource limits.

### Kubernetes Pod Alerts

Kubernetes pod alerts require the Netdata Kubernetes collector and provide visibility into pod health from the cluster perspective.

The `k8s_pod_ready` alert monitors the Kubernetes readiness probe status. Pods that fail readiness checks are not routable from Services, which means traffic is not reaching them even though they appear in pod listings. This alert catches situations where applications start but fail health checks.

Container restart counts indicate application stability. The `k8s_pod_restarting` alert uses thresholds of three and ten restarts per hour to distinguish between transient failures that self-recover from persistent problems that require intervention. Frequent restarts also trigger the `docker_container_restart` alert at the Docker level.

The `k8s_pod_not_scheduled` alert catches pods stuck in Pending state because no node has capacity to satisfy their resource requests. After five minutes in Pending state, a critical alert fires indicating that cluster capacity is insufficient for the current workload.

Resource limit alerts help prevent noisy neighbor problems. The `k8s_container_cpu_limits` and `k8s_container_mem_limits` alerts fire when containers reach 90% of their configured limits, indicating that the limit may be constraining application performance.

## 11.4 Application Alerts

Application alerts cover common database, web server, cache, and message queue technologies. Each application has unique metrics that indicate health and performance.

### Database Alerts

Databases are typically the most critical components in an infrastructure, and their alerts reflect this importance.

MySQL and MariaDB alerts focus on connection management, query performance, and replication health. The `mysql_gtid_binlog_gtid_0` alert tracks whether the GTID position is advancing, catching replication stalls immediately. The `mysql_slow_queries` alert at five queries per second identifies workloads generating excessive slow query traffic, which often precedes performance degradation.

PostgreSQL alerts emphasize connection pool saturation and replication lag. The `pg_stat_database_connections` alert at 80% of maximum connections prevents saturation-induced failures while providing warning time. Replication lag alerts use both absolute lag in bytes and time-based thresholds to catch problems across different network conditions.

Redis alerts address memory management and eviction behavior. The `redis_memory_fragmentation` alert detects when memory fragmentation exceeds 1.5, indicating that the allocator is struggling with the workload pattern. High fragmentation can cause apparent memory pressure even when usage appears normal. The `redis_evictions` alert catches eviction-based memory pressure, which indicates the working set exceeds configured limits.

### Web Server Alerts

Web server alerts monitor request rates, connection management, and error frequencies.

The `nginx_requests` and `apache_requests` alerts track request throughput as a baseline health indicator. A sudden change in request rate—whether a spike indicating traffic attacks or a drop indicating availability problems—triggers investigation. The 10,000 requests per second and 5,000 requests per second thresholds respectively are calibrated for typical server capacities.

Connection management alerts ensure that web servers can accept incoming connections. The `nginx_connections_active` alert uses 80% of worker limits as a warning threshold, while `apache_idle_workers` tracks the percentage of workers available to handle new connections. Either alert can indicate capacity exhaustion before requests begin queuing.

Error rate alerts are the most important for service health. The `nginx_4xx_requests` and `nginx_5xx_requests` alerts track the percentage of requests that result in client or server errors. A rising 4xx rate indicates client problems; a rising 5xx rate indicates server problems. Both warrant investigation, but 5xx errors are more urgent because they indicate server failures.

### Cache Alerts

Cache systems exist to improve performance, and their alerts focus on cache effectiveness.

The `memcached_hit_rate` alert tracks the ratio of cache hits to total requests. A hit rate below 80% suggests the cache is too small, the TTL is too short, or the access pattern defeats caching. The `memcached_evictions` alert catches when items are being removed from cache due to size limits, which indicates the working set exceeds capacity.

Redis alerts for evictions and memory usage serve similar purposes but with more granularity. Redis supports multiple eviction policies, and the alerts adapt to whatever policy is configured. The `redis_rdb_last_bgsave` alert tracks the duration of background saves, which can indicate either large datasets or I/O bottlenecks.

## 11.5 Network and Connectivity Alerts

Network alerts focus on endpoints and services rather than interface statistics.

### Ping and Latency Monitoring

The `ping_latency` alert tracks round-trip time with thresholds at 100ms for warning and 500ms for critical. These thresholds suit most operational requirements but may be too tight for geographically distributed systems where latency inherently runs higher.

The `ping_packet_loss` alert measures the percentage of packets that do not receive responses. Network problems often manifest as partial packet loss before complete connectivity failure, making this an effective early warning. The thresholds of 1% warning and 5% critical balance sensitivity with the reality that lossy networks still provide useful connectivity.

### Port and Service Monitoring

The `port_check_failed` alert attempts to connect to a specified port and fires when the connection fails. This alert can monitor any TCP service, from databases to custom applications. The `port_response_time` alert extends this by tracking how long the connection takes, catching slow services before they fail completely.

Certificate monitoring alerts before expiration. The `ssl_certificate_expiry` alert fires at 30 days warning and 7 days critical, providing time to renew certificates before they expire. Certificate expiration causes immediate service disruption and is easily preventable with proper monitoring.

### DNS Monitoring

DNS monitoring ensures that name resolution continues to function. The `dns_query_time` alert tracks resolution latency with thresholds of 50ms for warning and 200ms for critical. The `dns_query_failures` alert fires when DNS resolution fails entirely.

DNS problems cascade into application problems because most applications depend on name resolution. Monitoring DNS separately from application health ensures that DNS failures are caught even when applications appear responsive.

## 11.6 Hardware and Sensor Alerts

Hardware monitoring provides visibility into infrastructure that is often neglected until failure occurs.

### RAID Monitoring

RAID alerts track both the logical array status and individual disk health. The `raid_degraded` alert fires when an array has lost redundancy, which could mean a second disk failure would cause data loss. This alert is critical for storage systems.

The `smart_self_test` alert monitors the results of regular SMART self-tests. Self-test failures indicate imminent disk failure and warrant immediate attention. The `smart_reallocated_sectors` alert tracks bad sector remapping, which indicates the disk is beginning to fail.

### Temperature and Environmental Monitoring

Temperature alerts protect hardware from thermal damage. The `sensor_temperature` alert uses both warning and critical thresholds that vary by hardware specification but typically fire around 80C warning and 90C critical.

Fan speed alerts catch cooling failures before they cause temperature problems. The `fan_speed_low` alert at 90% of expected speed provides early warning, while `fan_speed_zero` at zero RPM indicates immediate failure.

### Power Monitoring

Power monitoring applies primarily to UPS-equipped systems. The `ups_battery_charge` alert tracks remaining battery capacity with thresholds at 25% warning and 10% critical. The `ups_on_battery` alert fires immediately when mains power fails, providing the first indication of power events.

## 11.7 Adjusting Stock Alerts

Stock alerts provide conservative defaults, but your environment may require different thresholds or additional alerts entirely. The recommended approach is to copy stock alerts into `/etc/netdata/health.d/` and modify them there.

When adjusting thresholds, document the reasoning. A threshold adjustment without documented rationale is difficult to review and impossible to audit later. Include the original threshold, the new threshold, and the business reason for the change.

Avoid disabling alerts entirely unless you have replaced them with equivalent monitoring. A disabled stock alert without replacement creates a monitoring gap that may prevent problem detection. If a stock alert does not apply to your environment, disable it explicitly rather than ignoring it.

## Related Chapters

- **2.4 Managing Stock vs Custom Alerts**: Safe procedures for overriding stock alerts
- **6. Alert Examples and Common Patterns**: Templates based on stock alerts
- **12. Best Practices for Alerting**: Guidance on designing new alerts

## What's Next

- **12. Best Practices for Alerting** for guidance on designing maintainable alert configurations
- **13. Alerts and Notifications Architecture** for understanding internal behavior