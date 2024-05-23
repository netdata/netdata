# Collect Application Metrics with Netdata

Netdata instantly collects per-second metrics from many different types of applications running on your systems, such as web servers, databases, message brokers, email servers, search platforms, and much more. Metrics collectors are pre-installed with every Netdata Agent and usually require zero configuration. Netdata also collects and visualizes resource utilization per application on Linux systems using `apps.plugin`.

[`apps.plugin`](/src/collectors/apps.plugin/README.md) looks at the Linux process tree every second, much like `top` or `ps fax`, and collects resource utilization information on every running process.

By reading the process tree, Netdata shows CPU, disk, networking, processes, and eBPF for every application or Linux user. Unlike `top` or `ps fax`, Netdata adds a layer of meaningful visualization on top of the process tree metrics, such as grouping applications into **useful dimensions**, and then creates per-application charts under the **Applications** section of a Netdata dashboard, per-user charts under **Users**, and per-user group charts under **User Groups**.

In parallel with charts, `apps.plugin` provides the **Processes** [Function](/docs/top-monitoring-netdata-functions.md) which visualizes the process entries in a table, and allows you to explore the processes with an intuitive UI. Check our documentation on the [Top tab](/docs/dashboards-and-charts/top-tab.md) to read more about how our visualization of Functions works.

Our most popular application collectors:

- [Prometheus endpoints](/src/go/collectors/go.d.plugin/modules/prometheus/README.md): Gathers metrics from one or more Prometheus endpoints that use the OpenMetrics exposition format. Auto-detects more than 600 endpoints.
- [Web server logs (Apache, NGINX)](/src/go/collectors/go.d.plugin/modules/weblog/README.md): Tails access logs and provides very detailed web server performance statistics. This module is able to parse 200k+ rows in less than half a second.
- [MySQL](/src/go/collectors/go.d.plugin/modules/mysql/README.md): Collects database global, replication, and per-user statistics.
- [Redis](/src/go/collectors/go.d.plugin/modules/redis/README.md): Monitors database status by reading the server's response to the `INFO` command.
- [Apache](/src/go/collectors/go.d.plugin/modules/apache/README.md): Collects Apache web server performance metrics via the `server-status?auto` endpoint.
- [Nginx](/src/go/collectors/go.d.plugin/modules/nginx/README.md): Monitors web server status information by gathering metrics via `ngx_http_stub_status_module`.
- [Postgres](/src/go/collectors/go.d.plugin/modules/postgres/README.md): Collects database health and performance metrics.
- [ElasticSearch](/src/go/collectors/go.d.plugin/modules/elasticsearch/README.md): Collects search engine performance and health statistics. Can optionally collect per-index metrics as well.
- [PHP-FPM](/src/go/collectors/go.d.plugin/modules/phpfpm/README.md): Collects application summary and processes health metrics by scraping the status page (`/status?full`).

Our [supported collectors list](/src/collectors/COLLECTORS.md#service-and-application-collectors) shows a comprehensive list of all of Netdata's application metrics collectors.
