# 11.1 System Resource Alerts

System resource alerts cover the fundamental building blocks of any server: CPU, memory, disk, network, and load. These alerts apply to every Netdata node and provide the baseline monitoring that every infrastructure should have.

:::note
System resource alerts are enabled by default on all Netdata installations. These alerts are foundational and apply to all nodes regardless of their role.
:::

## 11.1.1 CPU Alerts

The CPU alerts monitor utilization, saturation, and scheduling behavior.

### cpu_core_cumulative_100ms_percentage

The primary CPU alert tracks aggregate CPU usage across all cores as a percentage of available capacity. This alert uses a 100ms window to capture brief spikes that might indicate transient load bursts without triggering on normal variation.

**Context:** `system.cpu`
**Thresholds:** WARN > 90%, CRIT > 95%

### system.cpu_suppression

This alert measures time spent waiting in the scheduler queue. This metric catches problems that pure utilization percentage misses—a server at 70% CPU with high queue depth is more stressed than a server at 90% with empty queues.

**Thresholds:** WARN > 5s, CRIT > 10s

### cpu_core_frequency

Detects when CPU frequency drops below 90% of the nominal maximum, which may indicate thermal throttling or power-saving behavior that affects performance.

**Context:** `system.cpu`
**Thresholds:** WARN < 90% of max

## 11.1.2 Memory Alerts

Memory monitoring balances three competing concerns: availability for new allocations, pressure on cached data, and swapping activity that indicates the working set exceeds physical memory.

### ram_available

Monitors actual available memory, accounting for free memory, reclaimable caches, and reserved allocations. The thresholds provide headroom for memory allocation spikes while catching genuine exhaustion before OOM conditions.

**Context:** `system.ram`
**Thresholds:** WARN < 20%, CRIT < 10%

### ram_in_use

Tracks utilization percentage from the complementary perspective. Useful for identifying workloads that consistently run near capacity, even when available memory appears adequate.

**Context:** `system.ram`
**Thresholds:** WARN > 80%, CRIT > 90%

### swap_fill

Monitors swap space usage for systems with swap configured. Significant swap activity indicates working set exceeds physical memory.

**Context:** `system.swap`
**Thresholds:** WARN > 80%, CRIT > 95%

### low_memory_endanger

Provides the most sensitive early warning of memory exhaustion. Fires when available memory drops below 100MB, giving operators opportunity to investigate before OOM killer activates.

**Context:** `system.ram`
**Thresholds:** CRIT < 100MB

## 11.1.3 Disk Space Alerts

Disk space monitoring addresses space availability and inode exhaustion.

### disk_space_usage

Tracks percentage of allocated space across all mounted filesystems. The 80%/90% thresholds provide time to plan capacity expansions while preventing complete failure.

**Context:** `disk.space`
**Thresholds:** WARN > 80%, CRIT > 90%

### disk_inode_usage

For filesystems with many small files, tracks inode exhaustion which can occur before space exhaustion.

**Context:** `disk.inodes`
**Thresholds:** WARN > 80%, CRIT > 90%

## 11.1.4 Disk I/O Alerts

Disk I/O monitoring catches performance problems that capacity metrics miss.

### disk_busy

Measures the percentage of time the disk spends processing I/O requests. At 80% busy, most disks begin queuing delays; at 95%, degradation is nearly certain.

**Context:** `disk利用率`
**Thresholds:** WARN > 80%, CRIT > 95%

### disk_wait

Average wait time for I/O operations. High latency indicates the disk cannot keep up with workload demands.

**Context:** `disk`
**Thresholds:** WARN > 50ms, CRIT > 100ms

## 11.1.5 Network Interface Alerts

Network alerts focus on error conditions rather than throughput.

### network_interface_errors

Fires when any interface reports errors, checksum failures, or other problems. Even a single error may indicate cable problems or hardware degradation.

**Context:** `net.errors`
**Thresholds:** WARN > 0, CRIT > 10

### network_interface_drops

Tracks packet drops indicating the interface buffer filled faster than processing could handle.

**Context:** `net.drops`
**Thresholds:** WARN > 0, CRIT > 100

### network_interface_speed

Detects when link negotiation resulted in speeds below expected maximum, indicating configuration problems.

**Context:** `net.speed`
**Thresholds:** WARN < nominal speed

## Related Sections

- [11.2 Container Alerts](container-alerts.md) - Docker and Kubernetes monitoring
- [11.3 Network Alerts](network-alerts.md) - Network interface and protocol monitoring
- [11.4 Hardware Alerts](hardware-alerts.md) - Physical server and storage device alerts
- [11.5 Application Alerts](application-alerts.md) - Database, web server, cache, and message queue alerts