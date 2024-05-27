# Collect Application Metrics with Netdata

Netdata collects per-second metrics from a wide variety of applications running on your systems, including web servers,
databases, message brokers, email servers, search platforms, and more. These metrics collectors are pre-installed with
every Netdata Agent and typically require no configuration. Netdata also
uses [`apps.plugin`](/src/collectors/apps.plugin/README.md) to gather and visualize resource utilization per application
on Linux systems.

The `apps.plugin` inspects the Linux process tree every second, similar to `top` or `ps fax`, and collects resource
utilization data for every running process. However, Netdata goes a step further: instead of just displaying raw data,
it transforms it into easy-to-understand charts. Rather than presenting a long list of processes, Netdata categorizes
applications into meaningful groups, such as "web servers" or "databases." Each category has its own charts in the
**Applications** section of your Netdata dashboard. Additionally, there are charts for individual users and user groups
under the **Users** and **User Groups** sections.

In addition to charts, `apps.plugin` offers the **Processes** [Function](/docs/top-monitoring-netdata-functions.md),
which visualizes process entries in a table and allows for intuitive exploration of the processes. For more details on
how the visualization of Functions works, check out the documentation on
the [Top tab](/docs/dashboards-and-charts/top-tab.md).

Popular application collectors:

| Collector                                                                                  | Description                                                                                                                                         |
|--------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------|
| [Prometheus](/src/go/collectors/go.d.plugin/modules/prometheus/README.md)                  | Gathers metrics from one or more Prometheus endpoints that use the OpenMetrics exposition format. Auto-detects more than 600 endpoints.             |
| [Web server logs (Apache, NGINX)](/src/go/collectors/go.d.plugin/modules/weblog/README.md) | Tails access logs and provides very detailed web server performance statistics. This module is able to parse 200k+ rows in less than half a second. |
| [MySQL](/src/go/collectors/go.d.plugin/modules/mysql/README.md)                            | Collects database global, replication, and per-user statistics.                                                                                     |
| [Redis](/src/go/collectors/go.d.plugin/modules/redis/README.md)                            | Monitors database status by reading the server's response to the `INFO` command.                                                                    |
| [Apache](/src/go/collectors/go.d.plugin/modules/apache/README.md)                          | Collects Apache web server performance metrics via the `server-status?auto` endpoint.                                                               |
| [Nginx](/src/go/collectors/go.d.plugin/modules/nginx/README.md)                            | Monitors web server status information by gathering metrics via `ngx_http_stub_status_module`.                                                      |
| [Postgres](/src/go/collectors/go.d.plugin/modules/postgres/README.md)                      | Collects database health and performance metrics.                                                                                                   |
| [ElasticSearch](/src/go/collectors/go.d.plugin/modules/elasticsearch/README.md)            | Collects search engine performance and health statistics. Can optionally collect per-index metrics as well.                                         |

Check available [data collection integrations](/src/collectors/COLLECTORS.md#available-data-collection-integrations) for
a comprehensive view to all the integrations you can use to gather metrics with Netdata.
