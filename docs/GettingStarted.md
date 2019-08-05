# Getting started guide

Thanks for installing Netdata! In this guide, we'll walk you through the first steps you should take after getting Netdata installed.

Netdata can collect thousands of metrics in real-time without any configuration, but there are a few things you can do, like extending the history, to make Netdata work best for your particular needs.

!!! note
    If you haven't installed Netdata yet, visit the [installation instructions](../packaging/installer) for details, including our one-liner script that works on almost all Linux distributions.


## Access the dashboard

Open up your browser of choice and navigate to `http://SERVER-IP:19999/`. Replace `SERVER-IP` with the IP address of the system you have Netdata installed on. You'll then be able to see Netdata's dashboard directly in your browser window.

<details markdown="1"><summary>I don't know my system's IP address!</summary>

If you installed Netdata on your **local system** (as in the same system you're using to browse this very guide), replace `SERVER-IP` with `localhost`: `http://localhost:19999`.

If you installed Netdata on **another system in your network**, you need to know its *internal IP address*. Run `ip route get 8.8.8.8 | grep -oP " src [0-9\.]+ "` and try to navigate to the IP address(es) that the command returns.

If you installed Netdata on a **remote system**, such as VPS or other cloud hosting environment, you need to know its *external IP address*. Check out your provider's dashboard for that information. If you happen to be connected to the remote system with SSH, run either `dig +short myip.opendns.com @resolver1.opendns.com` or `curl ifconfig.me` to find the external IP address, and use that to visit the Netdata dashboard.

</details>

<details markdown="1"><summary>Can't see the Netdata dashboard? Try to verify that Netdata is installed, running, and operational:</summary>

**Verify Netdata is running**

If the system you're trying to monitor is a remote server, open an SSH session to the server and execute `sudo ps -e | grep netdata`. You should see a response with the PID of the Netdata daemon.

If it prints nothing, Netdata is not running. Check out the [installation page](../packaging/installer) to reinstall Netdata or fix your installation.

When you install multiple Netdata servers, all your servers will appear at the node menu at the top left of the dashboard. For this to work, you have to manually access just once, the dashboard of each of your Netdata servers.

The node menu is more than just browser bookmarks. When switching Netdata servers from that menu, any settings of the current view are propagated to the other Netdata server:

-   the current charts panning (drag the charts left or right),
-   the current charts zooming (`SHIFT` + mouse wheel over a chart),
-   the highlighted time-frame (`ALT` + select an area on a chart),
-   the scrolling position of the dashboard,
-   the theme you use,
-   etc.

On the same SSH session, execute `tail -f /var/log/netdata/access.log`. Or, if you used the static 64-bit package, try `tail -f /opt/netdata/var/log/netdata/access.log`. This command will print all the HTTP requests Netdata receives.

With this command still running, try accessing your dashboard using your browser. If nothing new is printed into your terminal, it means Netdata isn't receiving your HTTP request. That probably means something, like a firewall, is blocking the requests from reaching Netdata.

</details>

To start/stop Netdata, depending on your environment, you should use:

-   `systemctl start netdata` and `systemctl stop netdata`
-   `service netdata start` and `service netdata stop`
-   `/etc/init.d/netdata start` and `/etc/init.d/netdata stop`

## Change how long Netdata stores metrics



The default installation of Netdata is configured for a small round-robin database: just 1 hour of data. Depending on the memory your system has and the amount you can dedicate to Netdata, you should adapt this. On production systems with limited RAM, we suggest to set this to 3-4 hours. For best results you should set this to 24 or 48 hours.

For every hour of data, Netdata needs about 25MB of RAM. If you can dedicate about 100MB of RAM to Netdata, you should set its database size to 4 hours.

To do this, edit `/etc/netdata/netdata.conf` (or `/opt/netdata/etc/netdata/netdata.conf`) and set:

```
[global]
    history = SECONDS
```

Make sure the `history` line is not commented (comment lines start with `#`).

1 hour is 3600 seconds, so the number you need to set is the result of `HOURS * 3600`.

!!! danger
    Be careful when you set this on production systems. If you set it too high, your system may run out of memory. By default, Netdata is configured to be killed first when the system starves for memory, but better be careful to avoid issues.

For more information about Netdata memory requirements, [check this page](../database).

If your kernel supports KSM (most do), you can [enable KSM to half Netdata memory requirement](../database#ksm).

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
