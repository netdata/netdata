# 6.3 Application Alerts

Application alerts cover common database, web server, cache, and message queue technologies. Each application has unique metrics that indicate health and performance.

:::note

To see more application alerts, visit the [Collectors list](https://learn.netdata.io/docs/observability/collectors/collectors/) on learn.netdata.io.

:::

## 6.3.1 Database Alerts

- [MySQL & MariaDB](https://learn.netdata.io/docs/observability/collectors/collectors.complex.list/mysql/) - replication lag, slow queries, connection utilization
- [PostgreSQL](https://learn.netdata.io/docs/observability/collectors/collectors.complex.list/postgresql/) - deadlocks, connection utilization, replication lag
- [MongoDB](https://learn.netdata.io/docs/observability/collectors/collectors.complex.list/mongodb/) - query performance, connection pooling
- [Redis](https://learn.netdata.io/docs/observability/collectors/collectors.complex.list/redis/) - memory pressure, eviction rates

## 6.3.2 Web Server Alerts

- [nginx](https://learn.netdata.io/docs/observability/collectors/collectors.complex.list/nginx/) - request handling, connection states
- [Apache](https://learn.netdata.io/docs/observability/collectors/collectors.complex.list/apache/) - request processing, worker utilization

## 6.3.3 Message Queue Alerts

- [RabbitMQ](https://learn.netdata.io/docs/observability/collectors/collectors.complex.list/rabbitmq/) - queue depth, message rates
- [Kafka](https://learn.netdata.io/docs/observability/collectors/collectors.complex.list/apache_kafka/) - consumer lag, partition health

## 6.3.4 Proxy & Load Balancer Alerts

- [HAProxy](https://learn.netdata.io/docs/observability/collectors/collectors.complex.list/haproxy/) - request rates, backend health
- [Traefik](https://learn.netdata.io/docs/observability/collectors/collectors.complex.list/traefik/) - request handling, middleware health

## Related Sections

- [6.1 System Resource Alerts](./1-system-resource-alerts.md) - CPU, memory, disk, and load alerts
- [6.2 Container Alerts](./2-container-alerts.md) - Docker and Kubernetes monitoring
- [6.4 Network Alerts](./4-network-alerts.md) - Network interface and protocol monitoring
- [6.5 Hardware Alerts](./5-hardware-alerts.md) - Physical server and storage device alerts