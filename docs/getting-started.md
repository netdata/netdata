# Getting started guide

Thanks for trying Netdata! In this guide, we'll quickly walk you through the first steps you should take after getting
Netdata installed.

Netdata can collect thousands of metrics in real-time without any configuration, but there are some valuable things to
know to get the most of out Netdata based on your needs.

> If you haven't installed Netdata yet, visit the [installation instructions](../packaging/installer) for details,
> including our one-liner script, which automatically installs Netdata on almost all Linux distributions.

## Access the dashboard

Open up your web browser of choice and navigate to `http://YOUR-HOST:19999`. Welcome to Netdata!

![Animated GIF of navigating to the
dashboard](https://user-images.githubusercontent.com/1153921/63463901-fcb9c800-c412-11e9-8f67-8fe182e8b0d2.gif)

**What's next?**: 

-   Read more about the [standard Netdata dashboard](../web/gui/).
-   Learn all the specifics of [using charts](../web/README.md#using-charts) or the differences between [charts,
    context, and families](../web/README.md#charts-contexts-families).

## Configuration basics

Netdata primarily uses the `netdata.conf` file for custom configurations.

On most systems, you can find that file at `/etc/netdata/netdata.conf`.

> Some operating systems will place your `netdata.conf` at `/opt/netdata/etc/netdata/netdata.conf`, so check there if
> you find nothing at `/etc/netdata/netdata.conf`.

The `netdata.conf` file is broken up into various sections, such as `[global]`, `[web]`, `[registry]`, and more. By
default, most options are commented, so you'll have to uncomment them (remove the `#`) for Netdata to recognize your
change.

Once you save your changes, [restart Netdata](#start-stop-and-restart-netdata) to load your new configuration.

**What's next?**:

-   [Change how long Netdata stores metrics](#change-how-long-netdata-stores-metrics) by either increasing the `history`
    option or switching to the database engine.
-   Move Netdata's dashboard to a [different port](https://docs.netdata.cloud/web/server/) or enable TLS/HTTPS
    encryption.
-   See all the `netdata.conf` options in our [daemon configuration documentation](../daemon/config/).
-   Run your own [registry](../registry/README.md#run-your-own-registry).

## Collect data from more sources

When Netdata _starts_, it auto-detects dozens of **data sources**, such as database servers, web servers, and more. To
auto-detect and collect metrics from a service or application you just installed, you need to [restart
Netdata](#start-stop-and-restart-netdata).

> There is one exception: When Netdata is running on the host (as in not in a container itself), it will always
> auto-detect containers and VMs.

However, auto-detection only works if you installed the source using its standard installation procedure. If Netdata
isn't collecting metrics after a restart, your source probably isn't configured correctly. Look at the [external plugin
documentation](../collectors/plugins.d/) to find the appropriate module for your source. Those pages will contain more
information about how to configure your source for auto-detection.

Some modules, like `chrony`, are disabled by default and must be enabled manually for auto-detection to work.

Once Netdata detects a valid source of data, it will continue trying to collect data from it. For example, if
Netdata is collecting data from an Nginx web server, and you shut Nginx down, Netdata will collect new data as soon as
you start the web server back up—no restart necessary.

### Configuring plugins

Even if Netdata auto-detects your service/application, you might want to configure what, or how often, Netdata is
collecting data.

Netdata uses **internal** and **external** plugins to collect data. Internal plugins run within the Netdata dæmon, while
external plugins are independent processes that send metrics to Netdata over pipes. There are also plugin
**orchestrators**, which are external plugins with one or more data collection **modules**.

You can configure both internal and external plugins, along with the individual modules. There are many ways to do so:

-   In `netdata.conf`, `[plugins]` section: Enable or disable internal or external plugins with `yes` or `no`.
-   In `netdata.conf`, `[plugin:XXX]` sections: Each plugin has a section for changing collection frequency or passing
    options to the plugin.
-   In `.conf` files for each external plugin: For example, at `/etc/netdata/python.d.conf`.
-   In `.conf` files for each module : For example, at `/etc/netdata/python.d/nginx.conf`.

It's complex, so let's walk through an example of the various `.conf` files responsible for collecting data from an
Nginx web server using the `nginx` module and the `python.d` plugin orchestrator.

First, you can enable or disable the `python.d` plugin entirely in `netdata.conf`.

```conf
[plugins]
    # Enabled
    python.d = yes
    # Disabled
    python.d = no
```

You can also configure the entire `python.d` external plugin via the `[plugin:python.d]` section in `netdata.conf`.
Here, you can change how often Netdata uses `python.d` to collect metrics or pass other command options:

```conf
[plugin:python.d]
    update every = 1
    command options = 
```

The `python.d` plugin has a separate configuration file at `/etc/netdata/python.d.conf` for enabling and disabling
modules. You can use the `edit-config` script to edit the file, or open it with your text editor of choice:

```bash
sudo /etc/netdata/edit-config python.d.conf
```

Finally, the `nginx` module has a configuration file called `nginx.conf` in the `python.d` folder. Again, use
`edit-config` or your editor of choice:

```bash
sudo /etc/netdata/edit-config python.d/nginx.conf
```

In the `nginx.conf` file, you'll find additional options. The default works in most situations, but you may need to make
changes based on your particular Nginx setup.

**What's next?**:

-   Look at the [full list of data collection modules](Add-more-charts-to-netdata.md#available-data-collection-modules)
    to configure your sources for auto-detection and monitoring.
-   Improve the [performance](Performance.md) of Netdata on low-memory systems.
-   Configure `systemd` to expose [systemd services
    utilization](../collectors/cgroups.plugin/README.md#monitoring-systemd-services) metrics automatically.
-   [Reconfigure individual charts](../daemon/config/README.md#per-chart-configuration) in `netdata.conf`.

## Health monitoring and alarms

Netdata comes with hundreds of health monitoring alarms for detecting anomalies on production servers. If you're running
Netdata on a workstation, you might want to disable Netdata's alarms.

Edit your `/etc/netdata/netdata.conf` file and set the following:

```conf
[health]
    enabled = no
```

If you want to keep health monitoring enabled, but turn email notifications off, edit your `health_alarm_notify.conf`
file with `edit-config`, or with your the text editor of your choice:

```bash
sudo /etc/netdata/edit-config health_alarm_notify.conf
```

Find the `SEND_EMAIL="YES"` line and change it to `SEND_EMAIL="NO"`.

**What's next?**:

-   Write your own health alarm using the [examples](../health/README.md#examples).
-   Add a new notification method, like [Slack](../health/notifications/slack/).

## Change how long Netdata stores metrics

By default, Netdata uses a custom database which uses both RAM and the disk to store metrics. Recent metrics are stored
in the system's RAM to keep access fast, while historical metrics are "spilled" to disk to keep RAM usage low.

This custom database, which we call the _database engine_, allows you to store a much larger dataset than your system's
available RAM.

If you're not sure whether you're using the database engine, or want to tweak the default settings to store even more
historical metrics, check out our tutorial: [**Changing how long Netdata stores
metrics**](../docs/tutorials/longer-metrics-storage.md).

**What's next?**:

-   Learn more about the [memory requirements for the database engine](../database/engine/README.md#memory-requirements)
    to understand how much RAM/disk space you should commit to storing historical metrics.
-   Read up on the memory requirements of the [round-robin database](../database/), or figure out whether your system
    has KSM enabled, which can [reduce the default database's memory usage](../database/README.md#ksm) by about 60%.

## Monitoring multiple systems with Netdata

If you have Netdata installed on multiple systems, you can have them all appear in the **My nodes** menu at the top-left
corner of the dashboard.

To show all your servers in that menu, you need to [register for or sign in](../docs/netdata-cloud/signing-in.md) to
[Netdata Cloud](../docs/netdata-cloud/) from each system. Each system will then appear in the **My nodes** menu, which
you can use to navigate between your systems quickly.

![Animated GIF of the My Nodes menu in
action](https://user-images.githubusercontent.com/1153921/64389938-9aa7b800-cff9-11e9-9653-a77e791811ad.gif)

Whenever you pan, zoom, highlight, select, or pause a chart, Netdata will synchronize those settings with any other
agent you visit via the My nodes menu. Even your scroll position is synchronized, so you'll see the same charts and
respective data for easy comparisons or root cause analysis.

You can now seamlessly track performance anomalies across your entire infrastructure!

**What's next?**:

-   Read up on how the [Netdata Cloud registry works](../registry/), and what kind of data it stores and sends to your
    web browser.
-   Familiarize yourself with the [Nodes View](../docs/netdata-cloud/nodes-view.md)

## Start, stop, and restart Netdata

When you install Netdata, it's configured to start at boot, and stop and restart/shutdown. You shouldn't need to start
or stop Netdata manually, but you will probably need to restart Netdata at some point.

-   To **start** Netdata, open a terminal and run `service netdata start`.
-   To **stop** Netdata, run `service netdata stop`.
-   To **restart** Netdata, run `service netdata restart`.

The `service` command is a wrapper script that tries to use your system's preferred method of starting or stopping
Netdata based on your system. But, if either of those commands fails, try using the equivalent commands for `systemd`
and `init.d`:

-   **systemd**: `systemctl start netdata`, `systemctl stop netdata`, `systemctl restart netdata`
-   **init.d**: `/etc/init.d/netdata start`, `/etc/init.d/netdata stop`, `/etc/init.d/netdata restart`

## What's next?

Even after you've configured `netdata.conf`, tweaked alarms, learned the basics of performance troubleshooting, and
added all your systems to the **My nodes** menu, you've just gotten started with Netdata.

Take a look at some more advanced features and configurations:

-   Centralize Netdata metrics from many systems with [streaming](../streaming)
-   Enable long-term archiving of Netdata metrics via [backends](../backends) to time-series databases.
-   Improve security by putting Netdata behind an [Nginx proxy with SSL](Running-behind-nginx.md).

Or, learn more about how you can contribute to [Netdata core](../CONTRIBUTING.md) or our
[documentation](../docs/contributing/contributing-documentation.md)!

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fgetting-started&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
