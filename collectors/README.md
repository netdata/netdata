# Collecting metrics

Netdata can collect metrics from hundreds of different sources, be they internal data created by the system itself, or
external data created by services or applications. To see _all_ of the sources Netdata collects from, view our [list of
supported collectors](COLLECTORS.md), and then view our [quickstart guide](QUICKSTART.md) to get up-and-running.

There are two essential points to understand about how collecting metrics works in Netdata:

-   All collectors are **installed by default** with every installation of Netdata. You do not need to install
    collectors manually to collect metrics from new sources.
-   Upon startup, Netdata will **auto-detect** any application or service that has a [collector](COLLECTORS.md), as long
    as both the collector and the app/service are configured correctly.

Most users will want to enable a new Netdata collector for their app/service. For those details, see our [quickstart
guide](QUICKSTART.md).

## Take your next steps with collectors

[Collectors quickstart](QUICKSTART.md)

[Supported collectors list](COLLECTORS.md)

[Collectors configuration reference](REFERENCE.md)

## Tutorials

[Monitor Nginx or Apache web server log files with Netdata](../docs/tutorials/collect-apache-nginx-web-logs.md)

[Monitor Unbound DNS servers with Netdata](../docs/tutorials/collect-unbound-metrics.md)

[Monitor a Hadoop cluster with Netdata](../docs/tutorials/monitor-hadoop-cluster.md)

## Related features

**[Dashboards](../web/README.md)**: Vizualize your newly-collect metrics in real-time using Netdata's [built-in
dashboard](../web/gui/README.md). 

**[Backends](../backends/README.md)**: Extend our built-in [database engine](), which supports long-term metrics
storage, by archiving metrics to like Graphite, Prometheus, MongoDB, TimescaleDB, and more.

**[Exporting](../exporting/README.md)**: An experimental refactoring of our backends system with a modular system and
support for exporting metrics to multiple systems simultaneously.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
