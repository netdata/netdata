<!--
title: "Infrastructure monitoring with Netdata"
sidebar_label: "Infrastructure monitoring"
description: "."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/quickstart/infrastructure.md
-->

While the free, open-source Netdata Agent is a surpurb single-node monitoring tool, it also works in parallel with
Netdata Cloud to create a unified infrastructure monitoring tool.

The Netdata Agent turns each node in your infrastructure into a distributed metrics collection and storage system, and then Netdat Cloud puts real-time metrics from every node into a single interface.

In this quickstart guide, you'll learn how to see key metrics from all your nodes in one interface, and build your first
dashboard for aggregating like metrics from many distributed nodes. You'll then take a peek into configuring individual
nodes and tweaking collectors to gather metrics on every critical application.

> This quickstart assumes you've installed the Netdata Agent on more than one node in your infrastructure, and claimed
> that node to your Space in Netdata Cloud. If you haven't yet, see the [_Get Netdata_ doc](/docs/get/README.md) for
> details on installation and claiming.

## See your infrastructure's metrics

To see all your nodes from a single pane of glass, first [sign in](https://app.netdata.cloud) to Netdata Cloud. You
should immediately see all your nodes on a single interface inside of your Space.

![Animated GIF of Netdata
Cloud](https://user-images.githubusercontent.com/1153921/80828986-1ebb3b00-8b9b-11ea-957f-2c8d0d009e44.gif)

You can [organize your nodes](/docs/configure/spaces-war-rooms.md) into **War Rooms** based on your preferred strategy.
Most Netdata Cloud users set up War Rooms based on the node's purpose or the primary application it's responsible for
running.

Once you've properly set up your Space, you can [invite your team](/docs/configure/invite-collaborate.md) and
collaborate on identifying anomalies or troubleshooting complex performance problems.

> If you want to monitor a Kubernetes cluster with Netdata, see our [k8s installation
> doc](/packaging/installer/methods/kubernetes.md) and read our guide, [_Monitor a Kubernetes cluster with
> Netdata_](/docs/guides/kubernetes-k8s-netdata.md).

## Build new dashboards for your infrastructure



## Configure your nodes

You can configure any node in your infrastructure based on its unique needs, whether that's to store more metrics
locally, reduce the frequency of data collection, or reduce resource usage.

Each node has a configuration file called `netdata.conf`, which is typically at `/etc/netdata/netdata.conf`. The best
way to edit this file is using the `edit-config` script, which ensures the configuration changes you make are not
overwritten by updates to the Netdata Agent. For example:

```bash
cd /etc/netdata
sudo ./edit-config netdata.conf
```

Our [configuration basics doc](/docs/configure/nodes.md) contains more information about `netdata.conf`, `edit-config`,
along with simple examples to get you familiar with editing your node's configuration.

After you've learned the basics, you should [secure your infrastructure's nodes](/docs/configure/secure-nodes.md) using
one of our recommended methods. These security best practices ensure no untrusted parties gain access to the metrics
collected on any of your nodes.

## Collect metrics from your system and applications

Netdata has [300+ pre-installed collectors](/docs/collectors/COLLECTORS.md) that gather thousands of metrics with zero
configuration. In fact, Netdata is already collecting thousands of metrics per second on your single node, all without
you setting it up first.

These collectors search your system in default locations and ports to find the applications running on your node and
gather as many metrics. The dashboard presents them meaningfully in charts to help you understand the baseline and
identify anomalies.

You may need to set up specific collectors for them to work, or you might want to configure a specific collector's
behavior. Read more about [collector configuration](/docs/collect/configuration.md), or find detailed information about
which [system](/docs/collect/system-metrics.md), [container](/docs/collect/container-metrics.md), and
[application](/docs/collect/application-metrics.md) metrics you can collect with Netdata.

## What's next?

Netdata has many features that help you monitor the health of your node and troubleshoot complex performance problems.
Once you have a handle on configuration and are collecting all the right metrics, try out some of Netdata's other
features:

-   [Build new dashboards](/docs/visualize/create-dashboards.md) to put disparate-but-relevant metrics onto a single
    interface.
-   [Create new alarms](/docs/monitor/configure-alarms.md), or tweak some of the pre-configured alarms, to stay on top
    of anomalies.
-   [Enable notifications](/docs/monitor/enable-notifications.md) to Slack, PagerDuty, email, and 30+ other services.
-   [Change how long your node stores metrics](/docs/store/change-metrics-retention.md) based on how many metrics it
    collects, your preferred retention period, and the resources you want to dedicate toward long-term metrics
    retention.
-   [Export metrics](/docs/export/enable-exporting.md) to an external time-series database to use Netdata alongside
    other monitoring and troubleshooting tools.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fquickstart%2Finfrastructure&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
