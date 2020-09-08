<!--
title: "How Netdata's metrics collectors work"
description: ""
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/collect/how-collectors-work.md
-->

# How Netdata's metrics collectors work

When Netdata starts, and with zero configuration, it auto-detects thousands of data sources and immediately begins
collecting per-second metrics.

Netdata can immediately collect metrics from these endpoints thanks to 300+ **collectors**, which all come pre-installed
when you [install the Netdata Agent](/docs/get/README.md#).

A collector's primary job is simple: Look at a pre- or user-defined endpoint to find exposed metrics. If the collector
finds compatible metrics exposed on that endpoint, it begins a per-second collection job. The Netdata Agent gathers
these metrics, sends to them to the [database engine for storage](/docs/store/change-metrics-retention.md), and
immediately [visualizes them meaningfully](/docs/visualize/interact-dashboards-charts.md) on dashboards.

Each collector comes with a pre-defined configuration that matches the default setup for that application. This endpoint
can be a URL and port, a socket, a file, a web page, and more.

For example, the [Nginx collector](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/nginx) searches
at `http://127.0.0.1/stub_status`, which is the default endpoint for exposing Nginx metrics. The [web log collector for
Nginx or Apache](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/weblog) searches at
`/var/log/nginx/access.log` and `/var/log/apache2/access.log`, respectively, both of which are standard locations for
access log files on Linux systems.

The endpoint is user-configurable, as are many other specifics of what a given collector does.

## What's next?

[Enable or configure a collector](/docs/collect/enable-configure.md) 

See our [collectors reference](/collectors/REFERENCE.md) for detailed information on Netdata's collector architecture,
how to troubleshoot a collector, develop a custom collector, and more.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fcollect%2Fhow-collectors-work&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
