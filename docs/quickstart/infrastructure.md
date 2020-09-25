<!--
title: "Infrastructure monitoring with Netdata"
sidebar_label: "Infrastructure monitoring"
description: "Build a robust, infinitely scalable infrastructure monitoring solution with Netdata. Any number of nodes and every available metric."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/quickstart/infrastructure.md
-->

# Infrastructure monitoring with Netdata

Together, the Netdata Agent and Netdata Cloud create a powerful, infinitely-scalable infrastructure monitoring solution.

The Netdata Agent uses zero-configuration collectors to gather metrics from every application and container instantly.
The distributed data model lets you monitor everything without a slow and troublesome centralized data lake for your
infrastructure's metrics, reducing the resources you need to invest in metrics retention. Netdata Cloud unifies all the
metrics from these distributed nodes on customizable, interactive, and real-time visualizations.

In this quickstart guide, you'll learn how to see key metrics from all your nodes in one interface and build your first
dashboard for aggregating like metrics from many distributed nodes. You'll then take a peek into configuring individual
nodes and get helpful pointers about collecting all the metrics from every critical application in your infrastructure.

> This quickstart assumes you've installed the Netdata Agent on more than one node in your infrastructure, and claimed
> that node to your Space in Netdata Cloud. If you haven't yet, see the [_Get Netdata_ doc](/docs/get/README.md) for
> details on installation and claiming.

## See your infrastructure's metrics

To see all your nodes from a single pane of glass, first [sign in](https://app.netdata.cloud) to Netdata Cloud. As you
sign in, Netdata Cloud pings each claimed node to start on-demand streaming from your nodes to your browser. When
Netdata Cloud loads your War Room's **Nodes** view, you'll immediately see key metrics from your nodes, streamed in
real time, in a single interface.

![Animated GIF of Netdata
Cloud](https://user-images.githubusercontent.com/1153921/80828986-1ebb3b00-8b9b-11ea-957f-2c8d0d009e44.gif)

You can drill down into any node's full dashboard by clicking on that node's hostname in the Nodes view.

![Screenshot of an embedded node
dashboard](https://user-images.githubusercontent.com/1153921/87457036-9b678e00-c5bc-11ea-977d-ad561a73beef.png)

You can use single-node dashboards in Netdata Cloud to drill down on specific issues, scrub backward in time to
investigate historical data, and see like metrics presented meaningfully to help you troubleshoot performance problems.
Learn about [interacting with dashboards and charts](/docs/visualize/interact-dashboards-charts.md) to get the most from
all of Netdata's real-time metrics.

> If you want to monitor a Kubernetes cluster with Netdata, see our [k8s installation
> doc](/packaging/installer/methods/kubernetes.md) for setup details, and then read our guide, [_Monitor a Kubernetes
> cluster with Netdata_](/docs/guides/monitor/kubernetes-k8s-netdata.md).

## Build new dashboards for your infrastructure

You can use Netdata Cloud to build new dashboards that match your infrastructure's topology or help you diagnose complex
issues by aggregating correlated charts from any number of nodes.

To build your first dashboard, click on the **Nodes** dropdown, then select **+ Add**. Enter a name to assign to this
dashboard. Click on either of the **Add chart** buttons, then select the node you want to add a chart from. Select the
context, which is Netdata's way to organize charts, and then click **Add chart**.

Once you add a few charts, you can move them around, resize them, and add text. Make sure you click the **Save** button
before you navigate away.

Read more about [creating new dashboards](/docs/visualize/create-dashboards.md) for more details about the process and
additional tips on best leveraging the feature to help you troubleshoot complex performance problems.

## Configure your nodes

You can configure any node in your infrastructure if you need to, although most users will find the default settings
work extremely well for monitoring their infrastructures.

Each node has a configuration file called `netdata.conf`, which is typically at `/etc/netdata/netdata.conf`. The best
way to edit this file is using the `edit-config` script, which ensures updates to the Netdata Agent do not overwrite
your changes. For example:

```bash
cd /etc/netdata
sudo ./edit-config netdata.conf
```

Our [configuration basics doc](/docs/configure/nodes.md) contains more information about `netdata.conf`, `edit-config`,
along with simple examples to get you familiar with editing your node's configuration.

After you've learned the basics, you should [secure your infrastructure's nodes](/docs/configure/secure-nodes.md) using
one of our recommended methods. These security best practices ensure no untrusted parties gain access to the metrics
collected on any of your nodes.

## Collect metrics from your systems and applications

Netdata has [300+ pre-installed collectors](/collectors/COLLECTORS.md) that gather thousands of metrics with zero
configuration. Collectors search each of your nodes in default locations and ports to find running applications and
gather as many metrics as they can without you having to configure them individually.

In fact, Netdata is already collecting thousands of metrics per second from your webservers, databases, containers, and
much more, on each node in your infrastructure.

These metrics enrich your Netdata Cloud experience. You can see metrics from systems, containers, and applications in
the individual node dashboards, and you can create new dashboards around very specific charts, such as the real-time
volume of 503 responses from each of your webserver nodes.

Most collectors work without configuration, but you should read up on [how collectors
work](/docs/collect/how-collectors-work.md) and [how to enable/configure](/docs/collect/enable-configure.md) them.

In addition, find detailed information about which [system](/docs/collect/system-metrics.md),
[container](/docs/collect/container-metrics.md), and [application](/docs/collect/application-metrics.md) metrics you can
collect from across your infrastructure with Netdata.

## What's next?

Netdata has many features that help you monitor the health of your nodes and troubleshoot complex performance problems.
Once you have a handle on configuration and are collecting all the right metrics, try out some of Netdata's other
infrastructure-focused features:

-   [Organize your nodes](/docs/configure/spaces-war-rooms.md) into **War Rooms** based on your preferred strategy.
-   [Invite your team](/docs/configure/invite-collaborate.md) to collaborate on identifying anomalies or troubleshooting
    complex performance problems.
-   [Export metrics](/docs/export/external-databases.md) to an external time-series database to use Netdata alongside
    other monitoring and troubleshooting tools.

To change how the Netdata Agent runs on each node, dig in to configuration files:

-   [Change how long nodes in your infrastructure retain metrics](/docs/store/change-metrics-storage.md) based on how
    many metrics each node collects, your preferred retention period, and the resources you want to dedicate toward
    long-term metrics retention.
-   [Create new alarms](/docs/monitor/configure-alarms.md), or tweak some of the pre-configured alarms, to stay on top
    of anomalies.
-   [Enable notifications](/docs/monitor/enable-notifications.md) to Slack, PagerDuty, email, and 30+ other services.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fquickstart%2Finfrastructure&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
