# 6.3 Application Alerts

Application alerts cover common database, web server, cache, and message queue technologies. Each application has unique metrics that indicate health and performance.

:::note

To see more application alerts, visit the [Collectors list](https://learn.netdata.cloud/docs/collecting-metrics/collectors-configuration) on learn.netdata.cloud.

:::

## 6.3.1 Database Alerts

- [MySQL & MariaDB](https://learn.netdata.cloud/docs/data-collection/monitor-anything/Databases/MySQL) - replication lag, slow queries, connection utilization
- [PostgreSQL](https://learn.netdata.cloud/docs/data-collection/databases/postgresql) - deadlocks, connection utilization, replication lag
- [MongoDB](https://learn.netdata.cloud/docs/data-collection/databases/mongodb) - query performance, connection pooling
- [Redis](https://learn.netdata.cloud/docs/data-collection/monitor-anything/Databases/Redis) - memory pressure, eviction rates

## 6.3.2 Web Server Alerts

- [nginx](https://learn.netdata.cloud/docs/data-collection/monitor-anything/Web%20Servers/NGINX) - request handling, connection states
- [Apache](https://learn.netdata.cloud/docs/data-collection/monitor-anything/Web%20Servers/Apache) - request processing, worker utilization

## 6.3.3 Message Queue Alerts

- [RabbitMQ](https://learn.netdata.cloud/docs/data-collection/monitor-anything/Message%20Brokers/RabbitMQ) - queue depth, message rates
- [Kafka](https://learn.netdata.cloud/docs/data-collection/message-brokers/kafka) - consumer lag, partition health

## 6.3.4 Proxy & Load Balancer Alerts

- [HAProxy](https://learn.netdata.cloud/docs/data-collection/monitor-anything/Web%20Servers/haproxy-go.d.plugin-Recommended) - request rates, backend health
- [Traefik](https://learn.netdata.cloud/docs/data-collection/web-servers-and-web-proxies/traefik) - request handling, middleware health

## Related Sections

- [6.1 System Resource Alerts](./1-system-resource-alerts.md) - CPU, memory, disk, and load alerts
- [6.2 Container Alerts](./2-container-alerts.md) - Docker and Kubernetes monitoring
- [6.4 Network Alerts](./4-network-alerts.md) - Network interface and protocol monitoring
- [6.5 Hardware Alerts](./5-hardware-alerts.md) - Physical server and storage device alerts