# 11.1 System Resource Alerts

System resource alerts cover the fundamental building blocks of any server: CPU, memory, disk, network, and load. These alerts apply to every Netdata node and provide the baseline monitoring that every infrastructure should have.

:::note

System resource alerts are enabled by default on all Netdata installations. These alerts are foundational and apply to all nodes regardless of their role.

:::

## 11.1.1 CPU Alerts

The CPU alerts monitor utilization, saturation, and scheduling behavior.

### 10min_cpu_usage

The primary CPU alert tracks aggregate CPU usage over a 10-minute window. This longer window smooths out brief spikes while still catching sustained high usage.

**Context:** `system.cpu`
**Thresholds:** WARN > 75%, CRIT > 85%

### 10min_cpu_iowait

Monitors time spent waiting for I/O operations. High iowait indicates disk or storage bottlenecks affecting system performance.

**Context:** `system.cpu`
**Thresholds:** WARN > 20%, CRIT > 40%

### 20min_steal_cpu

Tracks time stolen by other virtual machines (in cloud/VPS environments). High steal indicates neighboring VMs consuming excessive resources.

**Context:** `system.cpu`
**Thresholds:** WARN > 10%, CRIT > 20%

## 11.1.2 Memory Alerts

Memory monitoring balances three competing concerns: availability for new allocations, pressure on cached data, and swapping activity that indicates the working set exceeds physical memory.

### ram_in_use

Tracks utilization percentage from the complementary perspective. Useful for identifying workloads that consistently run near capacity, even when available memory appears adequate.

**Context:** `system.ram`
**Thresholds:** WARN > 80%, CRIT > 90%

### ram_available

Monitors actual available memory, accounting for free memory, reclaimable caches, and reserved allocations. The thresholds provide headroom for memory allocation spikes while catching genuine exhaustion before OOM conditions.

**Context:** `mem.available`
**Thresholds:** WARN < 20%, CRIT < 10%

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

## Related Sections

- [11.2 Container Alerts](2-container-alerts.md) - Docker and Kubernetes monitoring
- [11.4 Network Alerts](4-network-alerts.md) - Network interface and protocol monitoring
- [11.4 Hardware Alerts](5-hardware-alerts.md) - Physical server and storage device alerts
- [11.5 Application Alerts](3-application-alerts.md) - Database, web server, cache, and message queue alerts