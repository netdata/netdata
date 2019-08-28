# Getting started guide

Thanks for installing Netdata! In this guide, we'll walk you through the first
steps you should take after getting Netdata installed.

Netdata can collect thousands of metrics in real-time without any configuration,
but there are a few things you can do, like extending the history, to make
Netdata work best for your particular needs.

!!! note If you haven't installed Netdata yet, visit the [installation
    instructions](../packaging/installer) for details, including our one-liner
    script that works on almost all Linux distributions.

## Access the dashboard

Open up your browser of choice. If you installed Netdata on the same system
you're using to open your browser, navigate to `http://localhost:19999/`. If you
installed Netdata on a remote system, navigate to `http://SYSTEM-IP:19999/`
after replacing `SYSTEM-IP` with the IP address of that system.

Hit `Enter`. Welcome to Netdata!

![Animated GIF of navigating to the
dashboard](https://user-images.githubusercontent.com/1153921/63463901-fcb9c800-c412-11e9-8f67-8fe182e8b0d2.gif)

**Next**: 

-   Read more about the [standard Netdata dashboard](../web/gui/).
-   Learn all the specifics of [using charts](../web/README.md#using-charts) or the
differences between [charts, context, and
families](../web/README.md#charts-contexts-families).

## Change how long Netdata stores metrics

By default, Netdata stores 1 hour of historical metrics and uses about
25MB of RAM.

If that's not enough for you, you're in luck—Netdata is quite flexible when it
comes to long-term storage based on your system and your needs.

There's two ways to quickly increase the depth of historical metrics: by
increasing the `history` value for the default database or switching to the DB
engine.

We have a tutorial for just that: [Changing how long Netdata stores metrics](tutorial/longer-metrics-storage.md).

**Next**:

-   Learn how to [configure Netdata's daemon](../daemon/config/) via the
    `netdata.conf` file.
-   Read up on the memory requirements of the [default database](../database/),
    or figure out whether your system has KSM enabled, which can [reduce the
    default database's memory usage](../database/README.md#ksm) by about 60%.
-   

## Service discovery and auto-detection

Netdata supports auto-detection of data collection sources. It auto-detects almost everything: database servers, web servers, dns server, etc.

This auto-detection process happens **only once**, when Netdata starts. To have Netdata re-discover data sources, you need to restart it. There are a few exceptions to this:

-   containers and VMs are auto-detected forever (when Netdata is running at the host).
-   many data sources are collected but are silenced by default, until there is useful information to collect (for example network interface dropped packet, will appear after a packet has been dropped).
-   services that are not optimal to collect on all systems, are disabled by default.
-   services we received feedback from users that caused issues when monitored, are also disabled by default (for example, `chrony` is disabled by default, because CentOS ships a version of it that uses 100% CPU when queried for statistics).

Once a data collection source is detected, Netdata will never quit trying to collect data from it, until Netdata is restarted. So, if you stop your web server, Netdata will pick it up automatically when it is started again.

Since Netdata is installed on all your systems (even inside containers), auto-detection is limited to `localhost`. This simplifies significantly the security model of a Netdata monitored infrastructure, since most applications allow `localhost` access  by default.

A few well known data collection sources that commonly need to be configured are:

-   [systemd services utilization](../collectors/cgroups.plugin/#monitoring-systemd-services) are not exposed by default on most systems, so `systemd` has to be configured to expose those metrics.

## Configuration quick start

In Netdata we have:

-   **internal** data collection plugins (running inside the Netdata daemon)
-   **external** data collection plugins (independent processes, sending data to Netdata over pipes)
-   modular plugin **orchestrators** (external plugins that have multiple data collection modules)

You can enable and disable plugins (internal and external) via `netdata.conf` at the section `[plugins]`.

All plugins have dedicated sections in `netdata.conf`, like `[plugin:XXX]` for overwriting their default data collection frequency and providing additional command line options to them.

All external plugins have their own `.conf` file.

All modular plugin orchestrators have a directory in `/etc/netdata` with a `.conf` file for each of their modules.

It is complex. So, let's see the whole configuration tree for the `nginx` module of `python.d.plugin`:

In `netdata.conf` at the `[plugins]` section, `python.d.plugin` can be enabled or disabled:

```
[plugins]
    python.d = yes
```

In `netdata.conf` at the `[plugin:python.d]` section, we can provide additional command line options for `python.d.plugin` and overwite its data collection frequency:

```
[plugin:python.d]
	update every = 1
	command options = 
```

`python.d.plugin` has its own configuration file for enabling and disabling its modules (here you can disable `nginx` for example):

```bash
sudo /etc/netdata/edit-config python.d.conf
```

Then, `nginx` has its own configuration file for configuring its data collection jobs (most modules can collect data from multiple sources, so the `nginx` module can collect metrics from multiple, local or remote, `nginx` servers):

```bash
sudo /etc/netdata/edit-config python.d/nginx.conf
```

## Health monitoring and alarms

Netdata ships hundreds of health monitoring alarms for detecting anomalies. These are optimized for production servers.

Many users install Netdata on workstations and are frustrated by the default alarms shipped with Netdata. On these cases, we suggest to disable health monitoring.

To disable it, edit `/etc/netdata/netdata.conf` (or `/opt/netdata/etc/netdata/netdata.conf` if you installed the static 64bit package) and set:

```
[health]
    enabled = no
```

The above will disable health monitoring entirely.

If you want to keep health monitoring enabled for the dashboard, but you want to disable email notifications, run this:

```bash
sudo /etc/netdata/edit-config health_alarm_notify.conf
```

and set `SEND_EMAIL="NO"`.

(For static 64bit installations use `sudo /opt/netdata/etc/netdata/edit-config health_alarm_notify.conf`).

## Starting and stopping Netdata

Netdata installer integrates Netdata to your init / systemd environment.

To start/stop Netdata, depending on your environment, you should use:

- `systemctl start netdata` and `systemctl stop netdata`
- `service netdata start` and `service netdata stop`
- `/etc/init.d/netdata start` and `/etc/init.d/netdata stop`

Once Netdata is installed, the installer configures it to start at boot and stop at shutdown.

For more information about using these commands, consult your system documentation.


## Add more Netdata agents to the My nodes menu

When you install multiple Netdata servers, all your servers will appear at the node menu at the top left of the dashboard. For this to work, you have to manually access just once, the dashboard of each of your netdata servers.

The node menu is more than just browser bookmarks. When switching Netdata servers from that menu, any settings of the current view are propagated to the other netdata server:

- the current charts panning (drag the charts left or right),
- the current charts zooming (`SHIFT` + mouse wheel over a chart),
- the highlighted time-frame (`ALT` + select an area on a chart),
- the scrolling position of the dashboard,
- the theme you use,
- etc.

are all sent over to other Netdata server, to allow you troubleshoot cross-server performance issues easily.


## What's next?

-   Check [Data Collection](../collectors) for configuring data collection plugins.
-   Check [Health Monitoring](../health) for configuring your own alarms, or setting up alarm notifications.
-   Check [Streaming](../streaming) for centralizing Netdata metrics.
-   Check [Backends](../backends) for long term archiving of Netdata metrics to time-series databases.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2FGettingStarted&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
