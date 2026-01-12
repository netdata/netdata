# 11.3 Application Alerts

Application alerts cover common database, web server, cache, and message queue technologies. Each application has unique metrics that indicate health and performance.

:::note

Application alerts require the appropriate database or service collector to be enabled. Check the **Collectors** tab in the dashboard to verify your application collectors are running.

:::

## 11.3.1 Database Alerts

Databases are typically the most critical components in an infrastructure, and their alerts reflect this importance.

### MySQL and MariaDB

#### mysql_gtid_binlog_gtid_0

Tracks whether the GTID position is advancing, catching replication stalls immediately.

**Context:** `mysql.slave_status`
**Thresholds:** WARN > 0 lag

#### mysql_slow_queries

Identifies workloads generating excessive slow query traffic, which often precedes performance degradation.

**Context:** `mysql.queries`
**Thresholds:** WARN > 5/s

#### mysql_innodb_buffer_pool_bytes

Monitors InnoDB buffer pool usage to prevent memory pressure on buffer pool-intensive workloads.

**Context:** `mysql.innodb_buffer_pool_bytes`
**Thresholds:** WARN > 90%

### PostgreSQL

#### pg_stat_database_deadlocks

Detects deadlocks that indicate concurrent transaction conflicts.

**Context:** `postgres.stat_database`
**Thresholds:** WARN > 0

#### pg_stat_database_connections

Tracks connection pool saturation to prevent connection exhaustion.

**Context:** `postgres.stat_database`
**Thresholds:** WARN > 80% of max

#### pg_replication_lag

Monitors streaming replication lag to prevent data inconsistency.

**Context:** `postgres.replication`
**Thresholds:** WARN > 10s, CRIT > 60s

### Redis

#### redis_memory_fragmentation

Detects when memory fragmentation exceeds 1.5, indicating the allocator is struggling with the workload pattern.

**Context:** `redis.mem`
**Thresholds:** WARN > 1.5

#### redis_evictions

Catches eviction-based memory pressure, which indicates the working set exceeds capacity.

**Context:** `redis.keys`
**Thresholds:** WARN > 0

## 11.3.2 Web Server Alerts

### Nginx

#### nginx_requests

Tracks request throughput as a baseline health indicator. A sudden change indicates availability problems.

**Context:** `nginx.requests`
**Thresholds:** WARN > 10000/s

#### nginx_connections_active

Monitors active connections against worker limits to prevent connection exhaustion.

**Context:** `nginx.connections`
**Thresholds:** WARN > 80% of limit

#### nginx_4xx_requests

Tracks client error rates which may indicate client problems or configuration errors.

**Context:** `nginx.requests`
**Thresholds:** WARN > 1%, CRIT > 5%

#### nginx_5xx_requests

Tracks server error rates which indicate server problems requiring investigation.

**Context:** `nginx.requests`
**Thresholds:** WARN > 0.1%, CRIT > 1%

### Apache

#### apache_requests

Similar to nginx, tracks request throughput for baseline health.

**Context:** `apache.requests`
**Thresholds:** WARN > 5000/s

#### apache_idle_workers

Monitors available worker threads to prevent connection queuing.

**Context:** `apache.workers`
**Thresholds:** WARN < 10% available

## 11.3.3 Cache Alerts

### Memcached

#### memcached_hit_rate

Tracks the ratio of cache hits to total requests. A rate below 80% suggests the cache is not effective.

**Context:** `memcached.hits`
**Thresholds:** WARN < 80%

#### memcached_evictions

Catches when items are being removed due to size limits.

**Context:** `memcached.evictions`
**Thresholds:** WARN > 0

## 11.3.4 Message Queue Alerts

### RabbitMQ

#### rabbitmq_queue_messages_ready

Monitors messages ready for delivery to detect consumer backlog.

**Context:** `rabbitmq.queue`
**Thresholds:** WARN > 1000

#### rabbitmq_queue_messages_unacknowledged

Tracks unacknowledged messages indicating consumer problems.

**Context:** `rabbitmq.queue`
**Thresholds:** WARN > 100

### Kafka

#### kafka_under_replicated_partitions

Monitors replication health to detect partition availability issues.

**Context:** `kafka.replication`
**Thresholds:** WARN > 0, CRIT > 0

#### kafka_offline_partitions

Critical alert for partitions without leadership.

**Context:** `kafka.partition_under_replicated`
**Thresholds:** CRIT > 0

## Related Sections

- [11.1 System Resource Alerts](1-system-resource-alerts.md) - CPU, memory, disk, and load alerts
- [11.2 Container Alerts](2-container-alerts.md) - Docker and Kubernetes monitoring
- [11.4 Network Alerts](4-network-alerts.md) - Network interface and protocol monitoring
- [11.5 Hardware Alerts](5-hardware-alerts.md) - Physical server and storage device alerts