# 6.3 Application Alerts

Application alerts cover common database, web server, cache, and message queue technologies. Each application has unique metrics that indicate health and performance.

:::note

To see more application alerts, visit the [Collectors list](/docs/collecting-metrics/collectors-configuration) on learn.netdata.cloud.

:::

## 6.3.1 Database Alerts

- [MySQL & MariaDB](/docs/src/go/plugin/go.d/collector/mysql/README.md) - replication lag, slow queries, connection utilization
- [PostgreSQL](/docs/src/go/plugin/go.d/collector/postgres/README.md) - deadlocks, connection utilization, replication lag
- [MongoDB](/docs/src/go/plugin/go.d/collector/mongodb/README.md) - query performance, connection pooling
- [Redis](/docs/src/go/plugin/go.d/collector/redis/README.md) - memory pressure, eviction rates

## 6.3.2 Web Server Alerts

- [nginx](/docs/src/go/plugin/go.d/collector/nginx/README.md) - request handling, connection states
- [Apache](/docs/src/go/plugin/go.d/collector/apache/README.md) - request processing, worker utilization

## 6.3.3 Message Queue Alerts

- [RabbitMQ](/docs/src/go/plugin/go.d/collector/rabbitmq/README.md) - queue depth, message rates
- [Kafka](/docs/src/go/plugin/go.d/collector/prometheus/integrations/kafka.md) - consumer lag, partition health

## 6.3.4 Proxy & Load Balancer Alerts

- [HAProxy](/docs/src/go/plugin/go.d/collector/haproxy/README.md) - request rates, backend health
- [Traefik](/docs/src/go/plugin/go.d/collector/traefik/README.md) - request handling, middleware health

## Related Sections

- [6.1 System Resource Alerts](./1-system-resource-alerts.md) - CPU, memory, disk, and load alerts
- [6.2 Container Alerts](./2-container-alerts.md) - Docker and Kubernetes monitoring
- [6.4 Network Alerts](./4-network-alerts.md) - Network interface and protocol monitoring
- [6.5 Hardware Alerts](./5-hardware-alerts.md) - Physical server and storage device alerts