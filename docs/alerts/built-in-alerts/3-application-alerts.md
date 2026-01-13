# 11.3 Application Alerts

Application alerts cover common database, web server, cache, and message queue technologies. Each application has unique metrics that indicate health and performance.

:::note

Application alerts require the appropriate database or service collector to be enabled. Check the **Collectors** tab in the dashboard to verify your application collectors are running.

:::

## 11.3.1 Database Alerts

Databases are typically the most critical components in an infrastructure, and their alerts reflect this importance.

### MySQL and MariaDB

#### mysql_replication_lag

Tracks whether the replication position is advancing, catching replication stalls immediately.

**Context:** `mysql.slave_behind`
**Thresholds:** WARN > 5s, CRIT > 10s

#### mysql_10s_slow_queries

Identifies workloads generating excessive slow query traffic, which often precedes performance degradation.

**Context:** `mysql.queries`
**Thresholds:** WARN > 5/s, CRIT > 10/s

#### mysql_connections

Tracks connection pool saturation to prevent connection exhaustion.

**Context:** `mysql.connections_active`
**Thresholds:** WARN > 60% of limit, CRIT > 80% of limit

### PostgreSQL

#### postgres_db_deadlocks_rate

Detects deadlocks that indicate concurrent transaction conflicts.

**Context:** `postgres.db_deadlocks_rate`
**Thresholds:** WARN > 0

#### postgres_total_connection_utilization

Tracks connection pool saturation to prevent connection exhaustion.

**Context:** `postgres.connections_utilization`
**Thresholds:** WARN > 70%, CRIT > 80%

## Related Sections

- [11.1 System Resource Alerts](./1-system-resource-alerts.md) - CPU, memory, disk, and load alerts
- [11.2 Container Alerts](./2-container-alerts.md) - Docker and Kubernetes monitoring
- [11.4 Network Alerts](./4-network-alerts.md) - Network interface and protocol monitoring
- [11.5 Hardware Alerts](./5-hardware-alerts.md) - Physical server and storage device alerts