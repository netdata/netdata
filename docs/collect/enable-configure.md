<!--
title: "Enable or configure a collector"
description: "Every collector is highly configurable, allowing them to collect metrics from any node and any infrastructure."
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/collect/enable-configure.md"
sidebar_label: "Enable or configure a collector"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Setup"
-->

# Enable or configure a collector

When Netdata starts up, each collector searches for exposed metrics on the default endpoint established by that service
or application's standard installation procedure. For example, the [Nginx
collector](https://github.com/netdata/go.d.plugin/blob/master/modules/nginx/README.md) searches at
`http://127.0.0.1/stub_status` for exposed metrics in the correct format. If an Nginx web server is running and exposes
metrics on that endpoint, the collector begins gathering them.

However, not every node or infrastructure uses standard ports, paths, files, or naming conventions. You may need to
enable or configure a collector to gather all available metrics from your systems, containers, or applications.

## Enable a collector or its orchestrator

You can enable/disable collectors individually, or enable/disable entire orchestrators, using their configuration files.
For example, you can change the behavior of the Go orchestrator, or any of its collectors, by editing `go.d.conf`.

Use `edit-config` from your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory) to open
the orchestrator primary configuration file:

```bash
cd /etc/netdata
sudo ./edit-config go.d.conf
```

Within this file, you can either disable the orchestrator entirely (`enabled: yes`), or find a specific collector and
enable/disable it with `yes` and `no` settings. Uncomment any line you change to ensure the Netdata daemon reads it on
start.

After you make your changes, restart the Agent with `sudo systemctl restart netdata`, or the [appropriate
method](https://github.com/netdata/netdata/blob/master/docs/configure/start-stop-restart.md) for your system.

## Configure a collector

First, [find the collector](https://github.com/netdata/netdata/blob/master/collectors/COLLECTORS.md) you want to edit and open its documentation. Some software has
collectors written in multiple languages. In these cases, you should always pick the collector written in Go.

Use `edit-config` from your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory) to open a
collector's configuration file. For example, edit the Nginx collector with the following:

```bash
./edit-config go.d/nginx.conf
```

Each configuration file describes every available option and offers examples to help you tweak Netdata's settings
according to your needs. In addition, every collector's documentation shows the exact command you need to run to
configure that collector. Uncomment any line you change to ensure the collector's orchestrator or the Netdata daemon
read it on start.

After you make your changes, restart the Agent with `sudo systemctl restart netdata`, or the [appropriate
method](https://github.com/netdata/netdata/blob/master/docs/configure/start-stop-restart.md) for your system.

## What's next?

Read high-level overviews on how Netdata collects [system metrics](https://github.com/netdata/netdata/blob/master/docs/collect/system-metrics.md), [container
metrics](https://github.com/netdata/netdata/blob/master/docs/collect/container-metrics.md), and [application metrics](https://github.com/netdata/netdata/blob/master/docs/collect/application-metrics.md).

If you're already collecting all metrics from your systems, containers, and applications, it's time to move into
Netdata's visualization features. [See an overview of your infrastructure](https://github.com/netdata/netdata/blob/master/docs/visualize/overview-infrastructure.md)
using Netdata Cloud, or learn how to [interact with dashboards and
charts](https://github.com/netdata/netdata/blob/master/docs/visualize/interact-dashboards-charts.md).


