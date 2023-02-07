<!--
title: "Collecting metrics"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/README.md"
id: "collectors-ref"
sidebar_label: "Plugins Reference"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "References/Collectors"
-->

# Collecting metrics

Netdata can collect metrics from hundreds of different sources, be they internal data created by the system itself, or
external data created by services or applications. To see _all_ of the sources Netdata collects from, view our
[list of supported collectors](https://github.com/netdata/netdata/blob/master/collectors/COLLECTORS.md).

There are two essential points to understand about how collecting metrics works in Netdata:

- All collectors are **installed by default** with every installation of Netdata. You do not need to install
  collectors manually to collect metrics from new sources.
- Upon startup, Netdata will **auto-detect** any application or service that has a
  [collector](https://github.com/netdata/netdata/blob/master/collectors/COLLECTORS.md), as long as both the collector
  and the app/service are configured correctly.

Most users will want to enable a new Netdata collector for their app/service. For those details, see
our [collectors' configuration reference](https://github.com/netdata/netdata/blob/master/collectors/REFERENCE.md).

## Take your next steps with collectors

[Supported collectors list](https://github.com/netdata/netdata/blob/master/collectors/COLLECTORS.md)

[Collectors configuration reference](https://github.com/netdata/netdata/blob/master/collectors/REFERENCE.md)

## Guides

[Monitor Nginx or Apache web server log files with Netdata](https://github.com/netdata/netdata/blob/master/docs/guides/collect-apache-nginx-web-logs.md)

[Monitor CockroachDB metrics with Netdata](https://github.com/netdata/netdata/blob/master/docs/guides/monitor-cockroachdb.md)

[Monitor Unbound DNS servers with Netdata](https://github.com/netdata/netdata/blob/master/docs/guides/collect-unbound-metrics.md)

[Monitor a Hadoop cluster with Netdata](https://github.com/netdata/netdata/blob/master/docs/guides/monitor-hadoop-cluster.md)

## Related features

**[Dashboards](https://github.com/netdata/netdata/blob/master/web/README.md)**: Visualize your newly-collect metrics in
real-time using Netdata's [built-in dashboard](https://github.com/netdata/netdata/blob/master/web/gui/README.md).

**[Exporting](https://github.com/netdata/netdata/blob/master/exporting/README.md)**: Extend our
built-in [database engine](https://github.com/netdata/netdata/blob/master/database/engine/README.md), which supports
long-term metrics storage, by archiving metrics to external databases like Graphite, Prometheus, MongoDB, TimescaleDB,
and more. It can export metrics to multiple databases simultaneously.


