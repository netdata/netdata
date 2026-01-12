# Chapter 11: Built-in Alerts

Netdata ships with a comprehensive library of pre-configured alerts covering system resources, applications, containers, and hardware monitoring. These alerts are enabled by default and require no configuration to provide immediate visibility into common problems.

:::note

Built-in alerts follow the principle of conservative defaults. They are tuned to detect genuine issues while avoiding noise from normal operational variation. However, every environment is unique—use these as a starting point and adjust thresholds based on your specific requirements.

:::

## 11.0.1 What Built-in Alerts Cover

Built-in alerts are organized into four categories based on what they monitor.

| Category | Focus | Typical Issues |
|----------|-------|----------------|
| **System Resources** | CPU, memory, disk, network, load | Resource exhaustion, throttling, saturation |
| **Applications** | Databases, web servers, queues | Query performance, connection limits, queue depths |
| **Containers** | Docker, Kubernetes | OOM kills, restart loops, pod scheduling |
| **Hardware** | RAID, UPS, SMART | Disk failures, battery status, predictive failure |

## 11.0.2 Alert Priority and Scope

Built-in alerts are scoped to the components present on each node. A database server receives database-specific alerts but not web server alerts. This prevents noise from irrelevant alerts while ensuring critical components receive appropriate coverage.

Alert priorities are set based on impact severity. Resource exhaustion alerts (CPU, memory, disk) are highest priority because they can affect all services on a node. Application-specific alerts are medium priority, scoped to individual services.

## 11.0.3 Modifying Built-in Alerts

To modify a built-in alert, copy the specific alert definition to your custom health configuration directory (`/etc/netdata/health.d/`) and adjust as needed. The original alert remains in stock configuration and is overridden by your custom version.

Do not modify stock alerts directly—your changes will be lost during upgrades. Always copy and customize in `/etc/netdata/health.d/`.

## 11.0.4 What's Included

| Section | Description |
|---------|-------------|
| **11.1 System Resource Alerts** | CPU, memory, disk space, network, load averages |
| **11.2 Application Alerts** | MySQL, PostgreSQL, Redis, nginx, Apache, and more |
| **11.3 Container Alerts** | Docker containers, Kubernetes pods, cgroup metrics |
| **11.4 Network Alerts** | Interface errors, packet drops, bandwidth utilization |
| **11.5 Hardware Alerts** | RAID controllers, UPS battery, SMART disk status |

## 11.0.5 Related Sections

- **Chapter 2**: Creating and editing alerts via configuration files
- **Chapter 12**: Best practices for designing effective alerts
- **Chapter 3**: Alert configuration syntax reference