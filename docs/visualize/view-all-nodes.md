<!--
title: "View all nodes at a glance"
description: "With Netdata Cloud's War Rooms, you can see the health status and real-time key metrics from any number of nodes in your infrastructure."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/visualize/view-all-nodes.md
-->

# View all nodes at a glance

In Netdata Cloud, your nodes are organized into War Rooms. The default view for any War Room is called the **Nodes
view**, which lets you see the health, performance, and alarm status of a particular cross-section of your
infrastructure. 

Each node occupies a single row, first featuring that node's alarm status (yellow for warnings, red for critical alarms)
and operating system, some essential information about the node, followed by any number of user-defined columns for key
metrics.

Click on the hostname of any node to seamlessly navigate to that node's Cloud dashboard. From here, you will see all the
same charts and real-time metrics as you would if you viewed the local dashboard at `http://NODE:19999`.

**ADD GIF**

By combining Nodes view with Cloud dashboards, you and your team can view all nodes at a glance, immediately identify
anomalies with auto-updating health statuses and key metrics, then dive into individual dashboards for discovering the
root cause.

## Add and customize metrics columns

Add more metrics columns by clicking the gear icon in the Nodes view. Choose the context you'd like to add, give it a
relevant name, and select whether you want to see all dimensions (the default), or only the specific dimensions your
team is interested in.

![GIF showing how to add new metrics to the Nodes
view](https://user-images.githubusercontent.com/1153921/87456847-593e4c80-c5bc-11ea-8063-80c768d4cf6e.gif)

You can also click the gear icon and hover over any existing charts, then click the pencil icon. This opens a panel to
edit that chart. You can the context, its title, add or remove dimensions, or delete the chart altogether.

These customizations appear for anyone else with access to that War Room.

## Change the timeframe

By default, the Nodes view shows the last 5 minutes of metrics data on every chart. The value displayed above the chart
is the 5-minute average of those metrics.

You can change the timeframe, and also change both the charts and the average value, by clicking on any of the buttons
next to the **Last** label. **15m** will display the last 15 minutes of metrics for each chart, **30m** for 30 minutes,
and so on.

![GIF showing how to change the timeframe in
Nodes](https://user-images.githubusercontent.com/1153921/87457127-bf2ad400-c5bc-11ea-9f3b-9afa4e4f1855.gif)

## Filter and group your infrastructure

Use the filter input next to the **Nodes** heading to filter your nodes based on your queries. You can enter a text
query that filters by hostname, or use the dropdown that appears as you begin typing to filter by operating system or
the service(s) that node provides.

Use the **Group by** dropdown to choose between no grouping, grouping by the node's alarm status (`critical`, `warning`,
and `clear`), and grouping by the service each node provides.

See what services Netdata Cloud can filter by with [supported collectors list](/docs/agent/collectors/collectors).

## What's next?

To troubleshoot complex performance issues using Netdata, you need to understand how to interact with its meaningful
visualizations. Learn more about [interaction](/docs/visualize/interact-dashboards-charts.md) to see historical metrics,
highlight timeframes for targeted analysis, and more.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fvisualize%2Fview-all-nodes&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
