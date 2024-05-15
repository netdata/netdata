<!--
title: "Collect application metrics with Netdata"
sidebar_label: "Application metrics"
description: "Monitor and troubleshoot every application on your infrastructure with per-second metrics, zero configuration, and meaningful charts."
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/collecting-metrics/application-metrics.md"
learn_status: "Published"
learn_topic_type: "Concepts"
learn_rel_path: "Concepts"
-->

# Collect application metrics with Netdata

Netdata instantly collects per-second metrics from many different types of applications running on your systems, such as
web servers, databases, message brokers, email servers, search platforms, and much more. Metrics collectors are
pre-installed with every Netdata Agent and usually require zero configuration. Netdata also collects and visualizes
resource utilization per application on Linux systems using `apps.plugin`.

[**apps.plugin**](https://github.com/netdata/netdata/blob/master/src/collectors/apps.plugin/README.md) looks at the Linux process tree every second, much like `top` or
`ps fax`, and collects resource utilization information on every running process. By reading the process tree, Netdata
shows CPU, disk, networking, processes, and eBPF for every application or Linux user. Unlike `top` or `ps fax`, Netdata
adds a layer of meaningful visualization on top of the process tree metrics, such as grouping applications into useful
dimensions, and then creates per-application charts under the **Applications** section of a Netdata dashboard, per-user
charts under **Users**, and per-user group charts under **User Groups**.

Our most popular application collectors:

- [Prometheus endpoints](https://github.com/netdata/netdata/blob/master/src/go/collectors/go.d.plugin/modules/prometheus/README.md): Gathers
  metrics from one or more Prometheus endpoints that use the OpenMetrics exposition format. Auto-detects more than 600
  endpoints.
- [Web server logs (Apache, NGINX)](https://github.com/netdata/netdata/blob/master/src/go/collectors/go.d.plugin/modules/weblog/README.md):
  Tail access logs and provide very detailed web server performance statistics. This module is able to parse 200k+
  rows in less than half a second.
- [MySQL](https://github.com/netdata/netdata/blob/master/src/go/collectors/go.d.plugin/modules/mysql/README.md): Collect database global,
  replication, and per-user statistics.
- [Redis](https://github.com/netdata/netdata/blob/master/src/go/collectors/go.d.plugin/modules/redis/README.md): Monitor database status by
  reading the server's response to the `INFO` command.
- [Apache](https://github.com/netdata/netdata/blob/master/src/go/collectors/go.d.plugin/modules/apache/README.md): Collect Apache web server
  performance metrics via the `server-status?auto` endpoint.
- [Nginx](https://github.com/netdata/netdata/blob/master/src/go/collectors/go.d.plugin/modules/nginx/README.md): Monitor web server status
  information by gathering metrics via `ngx_http_stub_status_module`.
- [Postgres](https://github.com/netdata/netdata/blob/master/src/go/collectors/go.d.plugin/modules/postgres/README.md): Collect database health
  and performance metrics.
- [ElasticSearch](https://github.com/netdata/netdata/blob/master/src/go/collectors/go.d.plugin/modules/elasticsearch/README.md): Collect search
  engine performance and health statistics. Optionally collects per-index metrics.
- [PHP-FPM](https://github.com/netdata/netdata/blob/master/src/go/collectors/go.d.plugin/modules/phpfpm/README.md): Collect application summary
  and processes health metrics by scraping the status page (`/status?full`).

Our [supported collectors list](https://github.com/netdata/netdata/blob/master/src/collectors/COLLECTORS.md#service-and-application-collectors) shows all Netdata's
application metrics collectors, including those for containers/k8s clusters.

## Collect metrics from applications running on Windows

Netdata is fully capable of collecting and visualizing metrics from applications running on Windows systems. The only
caveat is that you must [install Netdata](https://github.com/netdata/netdata/blob/master/packaging/installer/README.md) on a separate system or a compatible VM because there
is no native Windows version of the Netdata Agent.

Once you have Netdata running on that separate system, you can follow the [collectors configuration reference](https://github.com/netdata/netdata/blob/master/src/collectors/REFERENCE.md) documentation to tell the collector to look for exposed metrics on the Windows system's IP
address or hostname, plus the applicable port.

For example, you have a MySQL database with a root password of `my-secret-pw` running on a Windows system with the IP
address 203.0.113.0. you can configure the [MySQL
collector](https://github.com/netdata/netdata/blob/master/src/go/collectors/go.d.plugin/modules/mysql/README.md) to look at `203.0.113.0:3306`:

```yml
jobs:
  - name: local
    dsn: root:my-secret-pw@tcp(203.0.113.0:3306)/
```

This same logic applies to any application in our [supported collectors
list](https://github.com/netdata/netdata/blob/master/src/collectors/COLLECTORS.md#service-and-application-collectors) that can run on Windows.

## What's next?

If you haven't yet seen the [supported collectors list](https://github.com/netdata/netdata/blob/master/src/collectors/COLLECTORS.md) give it a once-over for any
additional applications you may want to monitor using Netdata's native collectors, or the [generic Prometheus
collector](https://github.com/netdata/netdata/blob/master/src/go/collectors/go.d.plugin/modules/prometheus/README.md).

Collecting all the available metrics on your nodes, and across your entire infrastructure, is just one piece of the
puzzle. Next, learn more about Netdata's famous real-time visualizations by [seeing an overview of your
infrastructure](https://github.com/netdata/netdata/blob/master/docs/dashboards-and-charts/home-tab.md) using Netdata Cloud.


