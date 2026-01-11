# 11.3 Application Alerts

Application alerts cover common database, web server, cache, and message queue technologies. Each application has unique metrics that indicate health and performance.

:::note
Application alerts require the corresponding collector to be enabled (MySQL, PostgreSQL, Nginx, Redis, etc.).
:::

## 11.3.1 Database Alerts

Databases are typically the most critical components in an infrastructure, and their alerts reflect this importance.

### MySQL and MariaDB

| Alert | Context | Thresholds | Purpose |
|-------|---------|------------|---------|
| mysql_gtid_binlog_gtid_0 | `mysql.gtid` | WARN > 0 lag | Track replication stalls |
| mysql_slow_queries | `mysql.global_status` | WARN > 5/s | Identify slow workloads |
| mysql_innodb_buffer_pool_bytes | `mysql.innodb` | WARN > 90% | Monitor buffer pool usage |

### PostgreSQL

| Alert | Context | Thresholds | Purpose |
|-------|---------|------------|---------|
| pg_connections | `pg.stat.activity` | WARN > 80% max | Track connection exhaustion |
| pg_replication_lag | `pg.replication` | WARN > 10s | Monitor replication delay |
| pg_xlog_delay | `pg.xlog` | WARN > 100MB | Track WAL backlog |

## 11.3.2 Web Server Alerts

Web server alerts focus on request processing and error rates.

### Nginx

| Alert | Context | Thresholds | Purpose |
|-------|---------|------------|---------|
| nginx_connections_accepted | `nginx.connections` | WARN > capacity | Detect connection limits |
| nginx_requests | `nginx.requests` | WARN > 10000/s | Monitor request volume |
| nginx_upstream_5xx | `nginx.upstream` | WARN > 1% | Track backend errors |

### Apache

| Alert | Context | Thresholds | Purpose |
|-------|---------|------------|---------|
| apache_connections | `apache.connections` | WARN > max_workers | Detect worker exhaustion |
| apache_requests_per_child | `apache.perchild` | WARN > 1000 | Monitor per-child load |

## 11.3.3 Cache and Message Queue Alerts

### Redis

| Alert | Context | Thresholds | Purpose |
|-------|---------|------------|---------|
| redis_memory_usage | `redis.memory` | WARN > 80% max | Track memory exhaustion |
| redis_connected_clients | `redis.clients` | WARN > max_clients | Monitor client connections |
| redis_keyspace_evictions | `redis.evictions` | WARN > 0 | Detect memory pressure |

### RabbitMQ

| Alert | Context | Thresholds | Purpose |
|-------|---------|------------|---------|
| rabbitmq_queue_messages_ready | `rabbitmq.queues` | WARN > 10000 | Track queue backlog |
| rabbitmq_consumers | `rabbitmq.consumers` | WARN < 1 | Monitor consumer health |
| rabbitmq_connection_channels | `rabbitmq.connections` | WARN > 1000/ch | Detect channel limits |

## Related Sections

- [11.1 System Resource Alerts](system-resource-alerts.md) - CPU, memory, disk
- [11.2 Hardware Alerts](hardware-alerts.md) - Disk SMART, sensors
- [11.4 Network Alerts](network-alerts.md) - Interface monitoring
- [11.5 Container Alerts](container-alerts.md) - Container metrics