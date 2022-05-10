<!--
title: "Collecting metrics"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/README.md
id: "collectors-ref"
-->

# Collecting metrics

Netdata can collect metrics from hundreds of different sources, be they internal data created by the system itself, or
external data created by services or applications. To see _all_ of the sources Netdata collects from, view our [list of
supported collectors](/collectors/COLLECTORS.md).

There are two essential points to understand about how collecting metrics works in Netdata:

-   All collectors are **installed by default** with every installation of Netdata. You do not need to install
    collectors manually to collect metrics from new sources.
-   Upon startup, Netdata will **auto-detect** any application or service that has a
    [collector](/collectors/COLLECTORS.md), as long as both the collector and the app/service are configured correctly.

Most users will want to enable a new Netdata collector for their app/service. For those details, see
our [collectors' configuration reference](/collectors/REFERENCE.md).

## Take your next steps with collectors

[Supported collectors list](/collectors/COLLECTORS.md)

[Collectors configuration reference](/collectors/REFERENCE.md)

## Guides

[Monitor Nginx or Apache web server log files with Netdata](/docs/guides/collect-apache-nginx-web-logs.md)

[Monitor CockroachDB metrics with Netdata](/docs/guides/monitor-cockroachdb.md)

[Monitor Unbound DNS servers with Netdata](/docs/guides/collect-unbound-metrics.md)

[Monitor a Hadoop cluster with Netdata](/docs/guides/monitor-hadoop-cluster.md)

## Related features

**[Dashboards](/web/README.md)**: Visualize your newly-collect metrics in real-time using Netdata's [built-in
dashboard](/web/gui/README.md). 

**[Exporting](/exporting/README.md)**: Extend our built-in [database engine](/database/engine/README.md), which supports
long-term metrics storage, by archiving metrics to external databases like Graphite, Prometheus, MongoDB, TimescaleDB, and more.
It can export metrics to multiple databases simultaneously.


