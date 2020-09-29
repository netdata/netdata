<!--
title: "Enable or configure a collector"
description: "Every collector is highly configurable, allowing them to collect metrics from any node and any infrastructure."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/collect/enable-configure.md
-->

# Enable or configure a collector

When Netdata starts up, each collector searches for exposed metrics on the default endpoint established by that service
or application's standard installation procedure. For example, the [Nginx
collector](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/nginx) searches at
`http://127.0.0.1/stub_status` for exposed metrics in the correct format. If an Nginx web server is running and exposes
metrics on that endpoint, the collector begins gathering them.

However, not every node or infrastructure uses standard ports, paths, files, or naming conventions. You may need to
enable or configure a collector to gather all available metrics from your systems, containers, or applications.

## Enable a collector or its orchestrator

You can enable/disable collectors individually, or enable/disable entire orchestrators, using their configuration files.
For example, you can change the behavior of the Go orchestator, or any of its collectors, by editing `go.d.conf`.

Use `edit-config` from your [Netdata config directory](/docs/configure/nodes.md#the-netdata-config-directory) to open
the orchestrator's primary configuration file:

```bash
cd /etc/netdata
sudo ./edit-config go.d.conf
```

Within this file, you can either disable the orchestrator entirely (`enabled: yes`), or find a specific collector and
enable/disable it with `yes` and `no` settings. Uncomment any line you change to ensure the Netdata deamon reads it on
start.

After you make your changes, restart the Agent with `service netdata restart`.

## Configure a collector

First, [find the collector](/collectors/COLLECTORS.md) you want to edit and open its documentation. Some software has
collectors written in multiple languages. In these cases, you should always pick the collector written in Go.

Use `edit-config` from your [Netdata config directory](/docs/configure/nodes.md#the-netdata-config-directory) to open a
collector's configuration file. For example, edit the Nginx collector with the following:

```bash
./edit-config go.d/nginx.conf
```

Each configuration file describes every available option and offers examples to help you tweak Netdata's settings
according to your needs. In addition, every collector's documentation shows the exact command you need to run to
configure that collector. Uncomment any line you change to ensure the collector's orchestrator or the Netdata daemon
read it on start.

After you make your changes, restart the Agent with `service netdata restart`.

## What's next?

Read high-level overviews on how Netdata collects [system metrics](/docs/collect/system-metrics.md), [container
metrics](/docs/collect/container-metrics.md), and [application metrics](/docs/collect/application-metrics.md).

If you're already collecting all metrics from your systems, containers, and applications, it's time to move into
Netdata's visualization features. [View all your nodes at a glance](/docs/visualize/view-all-nodes.md) or learn how to
[interact with dashboards and charts](/docs/visualize/interact-dashboards-charts.md).

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fcollect%2Fenable-configure&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
