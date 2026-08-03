# Collect Metrics with OpenTelemetry Collector

Use these recipes when an OpenTelemetry Collector is already part of your observability pipeline or must fan metrics out to multiple backends. If Netdata is the only consumer, prefer the linked native Netdata collector: it requires fewer moving parts and provides purpose-built charts and alerts.

Before you begin, complete [Ingest OpenTelemetry Metrics and Logs](/docs/opentelemetry/otlp-ingestion.md). The examples on this page use OpenTelemetry Collector Contrib `0.157.0` and the local, plaintext loopback endpoint from that guide. Use TLS when the Collector and Netdata Agent are on different hosts.

## Shared exporter

Add this exporter once, then combine it with a receiver and pipeline block from a recipe below:

```yaml
exporters:
  otlp_grpc/netdata:
    endpoint: "127.0.0.1:4317"
    tls:
      insecure: true
```

After starting or reloading the Collector, verify an actual chart in Netdata. A successful connection to port `4317` does not prove that the receiver is producing metrics or that Netdata accepted them.

The `service.pipelines.metrics` blocks are alternatives. To run several receivers in one Collector, define one metrics pipeline and list every enabled receiver in its `receivers` array.

## Host metrics

The `host_metrics` receiver collects CPU, memory, disk, filesystem, network, load, paging, and process metrics from the Collector host. Netdata ships mappings that group common host metrics into multi-dimension charts.

Use the smaller [host metrics smoke test](/docs/opentelemetry/otlp-ingestion.md#smoke-test-host-metrics) to verify the OTLP path. For a broader existing OpenTelemetry host pipeline, add the shared exporter and this receiver and pipeline:

```yaml
receivers:
  host_metrics:
    collection_interval: 10s
    scrapers:
      cpu: {}
      memory: {}
      disk: {}
      filesystem: {}
      network: {}
      load: {}
      paging: {}
      processes: {}

service:
  pipelines:
    metrics:
      receivers: [host_metrics]
      exporters: [otlp_grpc/netdata]
```

Remove scrapers you do not need. Representative output includes CPU time by core and state, memory by state, disk and network I/O by device, filesystem usage by mount point, load averages, paging usage, and process counts by status. For ongoing host monitoring, Netdata's native system collectors are usually the better choice because they are enabled automatically and do not duplicate the same host signals through OTLP.

## Prometheus endpoints

The `prometheus` receiver scrapes Prometheus-format HTTP endpoints without requiring a Prometheus server. Netdata also has a [native Prometheus endpoint collector](/src/go/plugin/go.d/collector/prometheus/README.md), which is simpler when Netdata is the only destination.

Add the shared exporter and this receiver and pipeline:

```yaml
receivers:
  prometheus:
    config:
      scrape_configs:
        - job_name: "my-application"
          scrape_interval: 10s
          static_configs:
            - targets: ["127.0.0.1:9090"]

service:
  pipelines:
    metrics:
      receivers: [prometheus]
      exporters: [otlp_grpc/netdata]
```

The `config` block embeds Prometheus scrape configuration. It supports multiple jobs, service discovery, relabeling, and `metric_relabel_configs`. At least one of `config.scrape_configs`, `config.scrape_config_files`, or `target_allocator` is required. If an embedded Prometheus expression contains a literal `$`, escape it as `$$` because Collector configuration performs environment-variable substitution.

For example, add another job and filter its scraped series before export:

```yaml
receivers:
  prometheus:
    config:
      scrape_configs:
        - job_name: "node-exporter"
          scrape_interval: 10s
          static_configs:
            - targets: ["127.0.0.1:9100"]
        - job_name: "my-application"
          scrape_interval: 10s
          metrics_path: /metrics
          static_configs:
            - targets: ["127.0.0.1:8080"]
          metric_relabel_configs:
            - source_labels: [__name__]
              regex: "my_app_.*"
              action: keep
```

See the pinned upstream [Prometheus receiver documentation](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/v0.157.0/receiver/prometheusreceiver) for unsupported Prometheus server features and the complete scrape configuration surface.

## Redis

The `redis` receiver connects to one Redis instance and uses the `INFO` command to collect client, memory, CPU, keyspace, command, network, and replication statistics. The Collector must be able to reach the Redis endpoint. Netdata also has a [native Redis collector](/src/go/plugin/go.d/collector/redis/README.md).

Add the shared exporter and this receiver and pipeline:

```yaml
receivers:
  redis:
    endpoint: "127.0.0.1:6379"
    collection_interval: 10s
    password: ${env:REDIS_PASSWORD}

service:
  pipelines:
    metrics:
      receivers: [redis]
      exporters: [otlp_grpc/netdata]
```

Omit `password` when Redis does not require authentication. For Redis 6 or later with ACLs, add `username` and set the selected user's password through the Collector environment. Do not put credentials directly in the configuration file.

Representative metrics include `redis.memory.used`, `redis.memory.rss`, `redis.clients.connected`, `redis.commands.processed`, `redis.keyspace.hits`, `redis.keyspace.misses`, `redis.keys.expired`, `redis.keys.evicted`, `redis.net.input`, `redis.net.output`, and per-database `redis.db.keys`.

See the pinned upstream [Redis receiver documentation](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/v0.157.0/receiver/redisreceiver) for transport, TLS, ACL, and metric details.

## NGINX

The `nginx` receiver reads NGINX's built-in `stub_status` endpoint. Netdata's [native NGINX collector](/src/go/plugin/go.d/collector/nginx/README.md) uses the same endpoint and is simpler when Netdata is the only destination.

Ensure NGINX includes `ngx_http_stub_status_module`, then expose a status location only to the Collector host:

```nginx
server {
    listen 127.0.0.1:80;

    location /status {
        stub_status;
        allow 127.0.0.1;
        deny all;
    }
}
```

Reload NGINX, then add the shared exporter and this receiver and pipeline:

```yaml
receivers:
  nginx:
    endpoint: "http://127.0.0.1:80/status"
    collection_interval: 10s

service:
  pipelines:
    metrics:
      receivers: [nginx]
      exporters: [otlp_grpc/netdata]
```

The receiver emits `nginx.connections_accepted`, `nginx.connections_handled`, `nginx.connections_current` by state, and `nginx.requests`. If no data appears, request the status URL from the Collector host and confirm that access controls allow that source address.

See the pinned upstream [NGINX receiver documentation](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/v0.157.0/receiver/nginxreceiver) for the complete receiver surface.

## PostgreSQL

The `postgresql` receiver queries PostgreSQL statistics views for database, connection, transaction, table, index, row, block, and background-writer metrics. The monitoring account needs access to the statistics views. Netdata also has a [native PostgreSQL collector](/src/go/plugin/go.d/collector/postgres/README.md).

Create a dedicated login and grant the predefined monitoring role. Replace the example password through your normal secret-management process:

```sql
CREATE USER otel WITH PASSWORD 'replace-with-a-generated-password';
GRANT pg_monitor TO otel;
```

Expose the same secret to the Collector as `POSTGRESQL_PASSWORD`, then add the shared exporter and this receiver and pipeline:

```yaml
receivers:
  postgresql:
    endpoint: "127.0.0.1:5432"
    username: otel
    password: ${env:POSTGRESQL_PASSWORD}
    databases: [mydb]
    collection_interval: 10s
    tls:
      insecure: true

service:
  pipelines:
    metrics:
      receivers: [postgresql]
      exporters: [otlp_grpc/netdata]
```

The example disables database transport security for a loopback connection only. For a remote database, enable TLS verification and configure the appropriate CA. Representative metrics include `postgresql.commits`, `postgresql.rollbacks`, `postgresql.db_size`, `postgresql.rows`, `postgresql.operations`, `postgresql.blocks_read`, `postgresql.connection.max`, `postgresql.table.count`, and `postgresql.index.scans`.

See the pinned upstream [PostgreSQL receiver documentation](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/v0.157.0/receiver/postgresqlreceiver) for TLS, feature gates, query sampling, top-query collection, and the complete metric set.

## Troubleshoot receiver recipes

- **The Collector rejects the configuration:** confirm that you installed the Contrib distribution and that its release includes the receiver. Check the documentation for your exact Collector version when it differs from `0.157.0`.
- **The Collector runs but emits no metrics:** inspect its logs for receiver connection, authentication, permission, scrape, and parse errors. Test the source endpoint from the Collector host.
- **Netdata shows one dimension per chart:** add or adjust an OTel metric mapping. See the mapping section of the [OpenTelemetry plugin reference](/src/crates/otel-plugin/README.md).
- **A metric is missing:** Netdata does not currently ingest OTLP exponential histograms. For other types, inspect both the Collector export result and the Agent journal.
- **The source is already monitored natively:** disable one path if the duplicated charts and collection load are not intentional.
