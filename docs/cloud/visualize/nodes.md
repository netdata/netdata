---
title: "Nodes view"
description: "See charts from all your nodes in one pane of glass, then dive in to embedded dashboards for granular troubleshooting of ongoing issues."
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/nodes.md"
sidebar_label: "Nodes view"
learn_status: "Published"
learn_topic_type: "Concepts"
learn_rel_path: "Operations/Visualizations"
---

The Nodes view lets you see and customize key metrics from any number of Agent-monitored nodes and seamlessly navigate
to any node's dashboard for troubleshooting performance issues or anomalies using Netdata's highly-granular metrics.

![The Nodes view in Netdata
Cloud](https://user-images.githubusercontent.com/1153921/119035218-2eebb700-b964-11eb-8b74-4ec2df0e457c.png)

Each War Room's Nodes view is populated based on the nodes you added to that specific War Room. Each node occupies a
single row, first featuring that node's alarm status (yellow for warnings, red for critical alarms) and operating
system, some essential information about the node, followed by columns of user-defined key metrics represented in
real-time charts.

Use the [Overview](/docs/cloud/visualize/overview) for monitoring an infrastructure in real time using
composite charts and Netdata's familiar dashboard UI.

Check the [War Room docs](/docs/cloud/war-rooms) for details on the utility bar, which contains the [node
filter](/docs/cloud/war-rooms#node-filter) and the [timeframe
selector](/docs/cloud/war-rooms#play-pause-force-play-and-timeframe-selector).

## Add and customize metrics columns

Add more metrics columns by clicking the gear icon. Choose the context you'd like to add, give it a relevant name, and
select whether you want to see all dimensions (the default), or only the specific dimensions your team is interested in.

Click the gear icon and hover over any existing charts, then click the pencil icon. This opens a panel to
edit that chart. Edit the context, its title, add or remove dimensions, or delete the chart altogether.

These customizations appear for anyone else with access to that War Room.

## See more metrics in Netdata Cloud

If you want to add more metrics to your War Rooms and they don't show up when you add new metrics to Nodes, you likely
need to configure those nodes to collect from additional data sources. See our [collectors doc](/docs/collect/enable-configure) 
to learn how to use dozens of pre-installed collectors that can instantly collect from your favorite services and applications.

If you want to see up to 30 days of historical metrics in Cloud (and more on individual node dashboards), read our guide
on [long-term storage of historical metrics](/guides/longer-metrics-storage). Also, see our
[calculator](/docs/store/change-metrics-storage#calculate-the-system-resources-RAM-disk-space-needed-to-store-metrics)
for finding the disk and RAM you need to store metrics for a certain period of time.

## What's next?

Now that you know how to view your nodes at a glance, learn how to [track active
alarms](/docs/cloud/alerts-notifications/view-active-alerts) with the Alerts Smartboard.
