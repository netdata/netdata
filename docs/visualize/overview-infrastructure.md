<!--
title: "See an overview of your infrastructure"
description: "With Netdata Cloud's War Rooms, you can see real-time metrics, from any number of nodes in your infrastructure, in composite charts."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/visualize/overview-infrastructure.md
-->

# See an overview of your infrastructure

In Netdata Cloud, your nodes are organized into War Rooms. The default view for any War Room is called the **Overview**,
which uses composite charts to display real-time aggregated metrics from all the nodes (or a filtered selection) in a
given War Room. 

With Overview's composite charts, you can see your infrastructure from a single pane of glass, discover trends or
anomalies, then drill down with filtering or single-node dashboards to see more.

For example, in the screenshot below, each chart visualizes average or sum metrics values from across 5 distributed
nodes.

![The War Room
Overview](https://user-images.githubusercontent.com/1153921/95912630-e75ed600-0d57-11eb-8a3b-49e883d16833.png)

## Using the Overview

TK

Let's talk about a few of the features available in the Overview, followed by a few examples of how you might want to
use it to monitor and troubleshoot your infrastructure.

### Filter nodes

TK

 [node
filtering](https://learn.netdata.cloud/docs/cloud/war-rooms#node-filter)

### Pick a timeframe

TK

[time
picker](https://learn.netdata.cloud/docs/cloud/war-rooms#time-picker)

### Configure individual composite charts

TK

[definition
bar](https://learn.netdata.cloud/docs/cloud/visualize/overview#definition-bar)

### A few example workflows

TK

## Nodes view

You can also use an alternative War Room interface, the **Nodes view**, to monitor the health status and key metrics
from multiple nodes. Read the [Nodes view doc](https://learn.netdata.cloud/docs/cloud/visualize/nodes) for details.

![The Nodes view](https://user-images.githubusercontent.com/1153921/95909704-cb593580-0d53-11eb-88fa-a3416ab09849.png)

## Single-node dashboards

Both the Overview and Nodes view offer easy access to **single-node dashboards** for targeted analysis. You can use
single-node dashboards in Netdata Cloud to drill down on specific issues, scrub backward in time to investigate
historical data, and see like metrics presented meaningfully to help you troubleshoot performance problems.

![An example single-node
dashboard](https://user-images.githubusercontent.com/1153921/95912410-acf53900-0d57-11eb-8864-d3ec7114fee6.png)

When using the Overview, click on **X Charts** to display a dropdown of contexts and nodes contributing to that
composite chart. Click on the link icon <img class="img__inline img__inline--link"
src="https://user-images.githubusercontent.com/1153921/95762109-1d219300-0c62-11eb-8daa-9ba509a8e71c.png" /> to quickly
_jump to the same chart in that node's single-node dashboard_ in Netdata Cloud.

![The charts dropdown in a composite
chart](https://user-images.githubusercontent.com/1153921/95911970-06a93380-0d57-11eb-8538-5291d17498a4.png))

When using the Nodes View, click on the node's hostname to jump to its dashboard. For example, in the screenshot below,
clicking **ip-172-26-14-97** redirects you a single-node dashboard with that node's metrics streamed to your browser in
real-time.

![Click on any hostname to view single-node
dashboards](https://user-images.githubusercontent.com/1153921/95912091-33f5e180-0d57-11eb-9f25-87bddedcbb94.png)

Once you're in a single-node dashboard in Cloud, you can interact with charts the same way you would with local
dashboards served by the Netdata Agent.

## What's next?

To troubleshoot complex performance issues using Netdata, you need to understand how to interact with its meaningful
visualizations. Learn more about [interaction](/docs/visualize/interact-dashboards-charts.md) to see historical metrics,
highlight timeframes for targeted analysis, and more.

### Related reference documentation

-   [Netdata Cloud · War Rooms](https://learn.netdata.cloud/docs/cloud/war-rooms)
-   [Netdata Cloud · Overview](https://learn.netdata.cloud/docs/cloud/visualize/overview)
-   [Netdata Cloud · Nodes view](https://learn.netdata.cloud/docs/cloud/visualize/nodes-view)

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fvisualize%2Foverview-infrastructure&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
