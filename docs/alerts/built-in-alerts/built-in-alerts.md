# 11. Built-In Alerts Reference

This chapter catalogs Netdata's stock alerts.

## What You'll Find in This Chapter

| Section | Alerts Covered |
|---------|----------------|
| **11.1 System Resource** | CPU, memory, disk, network |
| **11.2 Container Alerts** | Docker, Kubernetes |
| **11.3 Application Alerts** | Databases, web servers |
| **11.4 Network Alerts** | Ping, HTTP, port checks |
| **11.5 Hardware Alerts** | RAID, SMART, sensors |

## 11.1 System Resource Alerts

| Alert | Context | Threshold |
|-------|----------|-----------|
| `10min_cpu_usage` | `system.cpu` | WARN > 80%, CRIT > 95% |
| `ram_available` | `system.ram` | WARN < 20%, CRIT < 10% |
| `disk_space_usage` | `disk.space` | WARN < 20%, CRIT < 10% |
| `interface_errors` | `net.net` | WARN > 0, CRIT > 10 |

## 11.2 Container Alerts

| Alert | Context | Threshold |
|-------|----------|-----------|
| `docker_container_status` | `docker.status` | CRIT not running |
| `k8s_pod_ready` | `k8s.pod` | CRIT not ready |

## 11.3 Application Alerts

| Alert | Context | Threshold |
|-------|----------|-----------|
| `mysql_slow_queries` | `mysql.global_status` | WARN > 10/s |
| `pg_stat_database_deadlocks` | `pg.stat_database` | WARN > 0 |

## 11.4 Network Alerts

| Alert | Context | Threshold |
|-------|----------|-----------|
| `ping_latency` | `ping.latency` | WARN > 100ms |
| `http_response_time` | `httpcheck.response` | WARN > 1s |

## 11.5 Hardware Alerts

| Alert | Context | Threshold |
|-------|----------|-----------|
| `raid_degraded` | `raid.status` | CRIT degraded |
| `smart_self_test` | `smart.test` | WARN failed |

## What's Next

- **2.4 Managing Stock vs Custom Alerts** for overriding stock alerts
- **12. Best Practices for Alerting** for design guidance