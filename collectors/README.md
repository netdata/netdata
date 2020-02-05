# Collecting metrics

Netdata can collect metrics from hundreds of different sources, be they internal data created by the system itself, or
external data created by services or applications. To see _all_ of the sources Netdata collects from, view our [list of
supported collectors](COLLECTORS.md).

There are two essential points to understand about how collecting metrics works in Netdata:

-   All collectors are **installed by default** with every installation of Netdata. You do not need to install
    collectors manually to collect metrics from new sources.
-   Upon startup, Netdata will **auto-detect** any service and application that has a [collector](COLLECTORS.md), as
    long as both the collector and the service/application are configured correctly. If Netdata fails to show charts for
    a service that's running on your system, it's due to a misconfiguration.

Netdata uses **plugins** to organize its collectors.

-   **Internal** plugins gather metrics from `/proc`, `/sys` and other Linux kernel sources. They are written in `C`,
    and run as threads within the Netdata daemon.
-   **External** plugins gather metrics from external processes, such as a MySQL database or Nginx web server. They
    can be written in any language, and the `netdata` daemon spawns them as long-running independent processes. They
    communicate with the daemon via pipes.
-   **Plugin orchestrators**, which are external plugins that instead support a number of **modules**. Modules are a
    type of collector. We have a few plugin orchestrators available for those who want to develop their own collectors,
    but focus most of our efforts on the [Go plugin](go.d.plugin/).

## Take your next steps with collectors

[Collectors quickstart](QUICKSTART.md)
[Collectors configuration reference](REFERENCE.md)

## Related features

**[Dashboards](../web/README.md)**: Vizualize your newly-collect metrics in real-time using Netdata's [built-in
dashboard](../web/gui/README.md).

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
