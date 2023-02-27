<!--
title: "Monitor your infrastructure"
description: "Real-time infrastructure monitoring with Netdata Cloud. View key metrics, insightful charts, and active alarms from all your nodes."
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/quickstart/infrastructure.md"
sidebar_label: "Monitor your infrastructure"
learn_status: "Published"
learn_topic_type: "Concepts"
learn_rel_path: "Getting Started"
sidebar_position: "50"
-->

import { Grid, Box, BoxList, BoxListItemRegexLink } from '@site/src/components/Grid/'
import { RiExternalLinkLine } from 'react-icons/ri'

[Netdata Cloud](https://app.netdata.cloud) provides scalable infrastructure monitoring for any number of distributed
nodes running the Netdata Agent. A node is any system in your infrastructure that you want to monitor, whether it's a
physical or virtual machine (VM), container, cloud deployment, or edge/IoT device.

The Netdata Agent uses zero-configuration collectors to gather metrics from every application and container instantly,
and uses Netdata's [distributed data architecture](https://github.com/netdata/netdata/blob/master/docs/store/distributed-data-architecture.md) to store metrics
locally. Without a slow and troublesome centralized data lake for your infrastructure's metrics, you reduce the
resources you need to invest in, and the complexity of, monitoring your infrastructure. 

Netdata Cloud unifies infrastructure monitoring by _centralizing the interface_ you use to query and visualize your
nodes' metrics, not the data. By streaming metrics values to your browser, with Netdata Cloud acting as the secure proxy
between them, you can monitor your infrastructure using customizable, interactive, and real-time visualizations from any
number of distributed nodes.

In this quickstart guide, you'll learn the basics of using Netdata Cloud to monitor an infrastructure with dashboards,
composite charts, and alarm viewing. You'll then learn about the most critical ways to configure the Agent on each of
your nodes to maximize the value you get from Netdata.

This quickstart assumes you've [installed Netdata](https://github.com/netdata/netdata/edit/master/packaging/installer/README.md)
on more than one node in your infrastructure, and connected those nodes to your Space in Netdata Cloud. 

## Set up your Netdata Cloud experience

Start your infrastructure monitoring experience by setting up your Netdata Cloud account.

### Organize Spaces and War Rooms

Spaces are high-level containers to help you organize your team members and the nodes they can view in each War Room.
You already have at least one Space in your Netdata Cloud account.

A single Space puts all your metrics in one easily-accessible place, while multiple Spaces creates logical division
between different users and different pieces of a large infrastructure. For example, a large organization might have one
SRE team for the user-facing SaaS application, and a second IT team for managing employees' hardware. Since these teams
don't monitor the same nodes, they can work in separate Spaces and then further organize their nodes into War Rooms.

Next, set up War Rooms. Netdata Cloud creates dashboards and visualizations based on the nodes added to a given War
Room. You can [organize War Rooms](https://github.com/netdata/netdata/blob/master/docs/cloud/war-rooms.md#war-room-organization) in any way
you want, such as by the application type, for end-to-end application monitoring, or as an incident response tool.

Learn more about [Spaces](https://github.com/netdata/netdata/blob/master/docs/cloud/spaces.md) and [War
Rooms](https://github.com/netdata/netdata/blob/master/docs/cloud/war-rooms.md), including how to manage each, in their respective reference
documentation.

### Invite your team

Netdata Cloud makes an infrastructure's real-time metrics available and actionable to all organization members. By
inviting others, you can better synchronize with your team or colleagues to understand your infrastructure's heartbeat.
When something goes wrong, you'll be ready to collaboratively troubleshoot complex performance problems from a single
pane of glass.

To [invite new users](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/invite-your-team.md), click on **Invite Users** in the
Space management Area. Choose which War Rooms to add this user to, then click **Send**.

If your team members have trouble signing in, direct them to the [Netdata Cloud sign
in](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/sign-in.md) doc.

### See an overview of your infrastructure

The default way to visualize the health and performance of an infrastructure with Netdata Cloud is the
[**Overview**](https://github.com/netdata/netdata/blob/master/docs/visualize/overview-infrastructure.md), which is the default interface of every War Room. The
Overview features composite charts, which display aggregated metrics from every node in a given War Room. These metrics
are streamed on-demand from individual nodes and composited onto a single, familiar dashboard.

![The War Room
Overview](https://user-images.githubusercontent.com/1153921/108732681-09791980-74eb-11eb-9ba2-98cb1b6608de.png)

Read more about the Overview in the [infrastructure overview](https://github.com/netdata/netdata/blob/master/docs/visualize/overview-infrastructure.md) doc.

Netdata Cloud also features the [**Nodes view**](https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/nodes.md), which you can
use to configure and see a few key metrics from every node in the War Room, view health status, and more.

### Drill down to specific nodes

Both the Overview and Nodes view offer easy access to **single-node dashboards** for targeted analysis. You can use
single-node dashboards in Netdata Cloud to drill down on specific issues, scrub backward in time to investigate
historical data, and see like metrics presented meaningfully to help you troubleshoot performance problems.

Read about the process in the [infrastructure
overview](https://github.com/netdata/netdata/blob/master/docs/visualize/overview-infrastructure.md#drill-down-with-single-node-dashboards) doc, then learn about [interacting with
dashboards and charts](https://github.com/netdata/netdata/blob/master/docs/visualize/interact-dashboards-charts.md) to get the most from all of Netdata's real-time
metrics.

### Create new dashboards

You can use Netdata Cloud to create new dashboards that match your infrastructure's topology or help you diagnose
complex issues by aggregating correlated charts from any number of nodes. For example, you could monitor the system CPU
from every node in your infrastructure on a single dashboard.

![An example system CPU
dashboard](https://user-images.githubusercontent.com/1153921/108732974-4b09c480-74eb-11eb-87a2-c67e569c08b6.png)

Read more about [creating new dashboards](https://github.com/netdata/netdata/blob/master/docs/visualize/create-dashboards.md) for more details about the process and
additional tips on best leveraging the feature to help you troubleshoot complex performance problems.

## Set up your nodes

You get the most value out of Netdata Cloud's infrastructure monitoring capabilities if each node collects every
possible metric. For example, if a node in your infrastructure is responsible for serving a MySQL database, you should
ensure that the Netdata Agent on that node is properly collecting and streaming all MySQL-related metrics.

In most cases, collectors autodetect their data source and require no configuration, but you may need to configure
certain behaviors based on your infrastructure. Or, you may want to enable/configure advanced functionality, such as
longer metrics retention or streaming.

### Configure the Netdata Agent on your nodes

You can configure any node in your infrastructure if you need to, although most users will find the default settings
work extremely well for monitoring their infrastructures.

Each node has a configuration file called `netdata.conf`, which is typically at `/etc/netdata/netdata.conf`. The best
way to edit this file is using the `edit-config` script, which ensures updates to the Netdata Agent do not overwrite
your changes. For example:

```bash
cd /etc/netdata
sudo ./edit-config netdata.conf
```

Our [configuration basics doc](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md) contains more information about `netdata.conf`, `edit-config`,
along with simple examples to get you familiar with editing your node's configuration.

After you've learned the basics, you should [secure your infrastructure's nodes](https://github.com/netdata/netdata/blob/master/docs/netdata-security.md) using
one of our recommended methods. These security best practices ensure no untrusted parties gain access to the metrics
collected on any of your nodes.

### Collect metrics from systems and applications

Netdata has [300+ pre-installed collectors](https://github.com/netdata/netdata/blob/master/collectors/COLLECTORS.md) that gather thousands of metrics with zero
configuration. Collectors search each of your nodes in default locations and ports to find running applications and
gather as many metrics as they can without you having to configure them individually.

Most collectors work without configuration, should you want more info, you can read more on [how Netdata's metrics collectors work](https://github.com/netdata/netdata/blob/master/collectors/README.md) and the [Collectors configuration reference](https://github.com/netdata/netdata/blob/master/collectors/REFERENCE.md) documentation.

In addition, find detailed information about which [system](https://github.com/netdata/netdata/blob/master/docs/collect/system-metrics.md),
[container](https://github.com/netdata/netdata/blob/master/docs/collect/container-metrics.md), and [application](https://github.com/netdata/netdata/blob/master/docs/collect/application-metrics.md) metrics you can
collect from across your infrastructure with Netdata.

## Netdata Cloud features

<Grid columns="2">
  <Box
    title="Spaces and War Rooms">
    <BoxList>
      <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/cloud/spaces.md)" title="Spaces" />
      <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/cloud/war-rooms.md)" title="War Rooms" />
    </BoxList>
  </Box>
  <Box
    title="Dashboards">
    <BoxList>
      <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/overview.md)" title="Overview" />
      <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/nodes.md)" title="Nodes view" />
      <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/kubernetes.md)" title="Kubernetes" />
      <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/dashboards.md)" title="Create new dashboards" />
    </BoxList>
  </Box>
  <Box
    title="Alerts and notifications">
    <BoxList>
      <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/view-active-alerts.md)" title="View active alerts" />
      <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/smartboard.md)" title="Alerts Smartboard" />
      <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/notifications.md)" title="Alert notifications" />
    </BoxList>
  </Box>
  <Box
    title="Troubleshooting with Netdata Cloud">
    <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/metric-correlations.md)" title="Metric Correlations" />
    <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/anomaly-advisor.md)" title="Anomaly Advisor" />
    <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/events-feed.md)" title="Events Feed" />
  </Box>
  <Box
    title="Management and settings">
    <BoxList>
      <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/sign-in.md)" title="Sign in with email, Google, or GitHub" />
      <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/invite-your-team.md)" title="Invite your team" />
      <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/themes.md)" title="Choose your Netdata Cloud theme" />
      <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/role-based-access.md)" title="Role-Based Access" />
      <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/plans.md)" title="Paid Plans" />
    </BoxList>
  </Box>
</Grid>

- Spaces and War Rooms
  - [Spaces](https://github.com/netdata/netdata/blob/master/docs/cloud/spaces.md)
  - [War Rooms](https://github.com/netdata/netdata/blob/master/docs/cloud/war-rooms.md)
- Dashboards
  - [Overview](https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/overview.md)
  - [Nodes view](https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/nodes.md)
  - [Kubernetes](https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/kubernetes.md)
  - [Create new dashboards](https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/dashboards.md)
- Alerts and notifications
  - [View active alerts](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/view-active-alerts.md)
  - [Alerts Smartboard](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/smartboard.md)
  - [Alert notifications](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/notifications.md)
- Troubleshooting with Netdata Cloud
  - [Metric Correlations](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/metric-correlations.md)
  - [Anomaly Advisor](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/anomaly-advisor.md)
  - [Events Feed](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/events-feed.md)
- Management and settings
  - [Sign in with email, Google, or GitHub](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/sign-in.md)
  - [Invite your team](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/invite-your-team.md)
  - [Choose your Netdata Cloud theme](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/themes.md)
  - [Role-Based Access](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/role-based-access.md)
  - [Paid Plans](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/plans.md)
