<!--
title: "Single-node monitoring with Netdata"
sidebar_label: "Single-node monitoring"
description: "Learn dashboard basics, configuring your nodes, and collecting metrics from applications to create a powerful single-node monitoring tool."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/quickstart/single-node.md
-->

# Single-node monitoring with Netdata

Because it's free, open-source, and requires only 1% CPU utilization to collect thousands of metrics every second,
Netdata is a superb single-node monitoring tool.

In this quickstart guide, you'll learn how to access your single node's metrics through dashboards, configure your node
to your liking, and make sure the Netdata Agent is collecting metrics from the applications or containers you're running
on your node.

> This quickstart assumes you have installed the Netdata Agent on your node. If you haven't yet, see the [_Get Netdata_
> doc](/docs/get/README.md) for details on installation. In addition, this quickstart mentions features available only
> through Netdata Cloud, which requires you to [claim your node](/docs/get/README.md#claim-your-node-on-netdata-cloud).

## See your node's metrics

To see your node's real-time metrics, you need to access its dashboard. You can either view the local dashboard, which
runs on the node itself, or see the dashboard through Netdata Cloud. Both methods feature real-time, interactive, and
synchronized charts, with the same metrics, and use the same UI.

The primary difference is that Netdata Cloud also has a few extra features, like creating new dashboards using a
drag-and-drop editor, that enhance your monitoring and troubleshooting experience.

To see your node's local dashboard, open up your web browser of choice and navigate to `http://NODE:19999`, replacing
`NODE` with the IP address or hostname of your Agent. Hit `Enter`. 

![Animated GIF of navigating to the
dashboard](https://user-images.githubusercontent.com/1153921/80825153-abaec600-8b94-11ea-8b17-1b770a2abaa9.gif)

To see a node's dashboard in Netdata Cloud, [sign in](https://app.netdata.cloud). From the **Nodes** view in your
**General** War Room, click on the hostname of your node to access its dashboard through Netdata Cloud.

![Screenshot of an embedded node
dashboard](https://user-images.githubusercontent.com/1153921/87457036-9b678e00-c5bc-11ea-977d-ad561a73beef.png)

Once you've decided which dashboard you prefer, learn about [interacting with dashboards and
charts](/docs/visualize/interact-dashboards-charts.md) to get the most from Netdata's real-time metrics.

## Configure your node

The Netdata Agent is highly configurable so that you can match its behavior to your node. You will find most
configuration options in the `netdata.conf` file, which is typically at `/etc/netdata/netdata.conf`. The best way to
edit this file is using the `edit-config` script, which ensures updates to the Netdata Agent do not overwrite your
changes. For example:

```bash
cd /etc/netdata
sudo ./edit-config netdata.conf
```

Our [configuration basics doc](/docs/configure/nodes.md) contains more information about `netdata.conf`, `edit-config`,
along with simple examples to get you familiar with editing your node's configuration.

After you've learned the basics, you should [secure your node](/docs/configure/secure-nodes.md) using one of our
recommended methods. These security best practices ensure no untrusted parties gain access to your dashboard or its
metrics.

## Collect metrics from your system and applications

Netdata has [300+ pre-installed collectors](/collectors/COLLECTORS.md) that gather thousands of metrics with zero
configuration. Collectors search your node in default locations and ports to find running applications and gather as
many metrics as possible without you having to configure them individually.

These metrics enrich both the local and Netdata Cloud dashboards.

Most collectors work without configuration, but you should read up on [how collectors
work](/docs/collect/how-collectors-work.md) and [how to enable/configure](/docs/collect/enable-configure.md) them.

In addition, find detailed information about which [system](/docs/collect/system-metrics.md),
[container](/docs/collect/container-metrics.md), and [application](/docs/collect/application-metrics.md) metrics you can
collect from across your infrastructure with Netdata.

## What's next?

Netdata has many features that help you monitor the health of your node and troubleshoot complex performance problems.
Once you understand configuration, and are certain Netdata is collecting all the important metrics from your node, try
out some of Netdata's other visualization and health monitoring features:

-   [Build new dashboards](/docs/visualize/create-dashboards.md) to put disparate but relevant metrics onto a single
    interface.
-   [Create new alarms](/docs/monitor/configure-alarms.md), or tweak some of the pre-configured alarms, to stay on top
    of anomalies.
-   [Enable notifications](/docs/monitor/enable-notifications.md) to Slack, PagerDuty, email, and 30+ other services.
-   [Change how long your node stores metrics](/docs/store/change-metrics-storage.md) based on how many metrics it
    collects, your preferred retention period, and the resources you want to dedicate toward long-term metrics
    retention.
-   [Export metrics](/docs/export/external-databases.md) to an external time-series database to use Netdata alongside
    other monitoring and troubleshooting tools.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fquickstart%2Fsingle-node&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
