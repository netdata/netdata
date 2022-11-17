<!--
title: "Netdata Views"
sidebar_label: "Netdata Views"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-cloud/netdata-views.md"
sidebar_position: "1800"
learn_status: "Published"
learn_topic_type: "Concepts"
learn_rel_path: "Concepts/Netdata cloud"
learn_docs_purpose: "Present the Netdata cloud's views/tabs, not focusing on dashboards which we explain them in depth in visualizations"
-->


## Home

The Home tab provides a predefined dashboard of relevant information about entities in the War Room.

This tab will 
automatically present summarized information in an easily digestible display. You can see information about your 
nodes, data collection and retention stats, alerts, users and dashboards.

## Overview

The Overview tab is another great way to monitor infrastructure using Netdata Cloud. While the interface might look similar to local
dashboards served by an Agent, or even the single-node dashboards in Netdata Cloud, Overview uses **composite charts**.
These charts display real-time aggregated metrics from all the nodes (or a filtered selection) in a given War Room.

With Overview's composite charts, you can see your infrastructure from a single pane of glass, discover trends or
anomalies, then drill down by grouping metrics by node and jumping to single-node dashboards for root cause analysis.

## Before you get started

Only nodes with v1.25.0-127 or later of the the [open-source Netdata](https://github.com/netdata/netdata) monitoring
agent can contribute to composite charts. If your node(s) use an earlier version of Netdata, you will see them marked as
**needs upgrade** in various dropdowns.

See our [update docs](https://github.com/netdata/netdata/blob/master/docs/tasks/operations/update-the-agent.md) for the preferred update method based on how you installed
Netdata.

## Composite charts

The Overview uses composite charts, which aggregate metrics from all the nodes (or a filtered selection) in a given War
Room.

## Definition bar

Each composite chart has a definition bar to provide information about the following:
* Grouping option
* Aggregate function to be applied in case multiple data sources exist
* Instances
* Nodes
* Dimensions, and 
* Aggregate function over time to be applied if one point in the chart consists of multiple data points aggregated

### Group by dimension, node, or chart

Click on the **dimension** dropdown to change how a composite chart groups metrics.

The default option is by _dimension_, so that each line/area in the visualization is the aggregation of a single dimension.
This provides a per dimension view of the data from all the nodes in the War Room, taking into account filtering criteria if defined. 

A composite chart grouped by _node_ visualizes a single metric across contributing nodes. If the composite chart has five
contributing nodes, there will be five lines/areas. This is typically an absolute value of the sum of the dimensions over each node but there
are some opinionated-but-valuable exceptions where a specific dimension is selected.
Grouping by nodes allows you to quickly understand which nodes in your infrastructure are experiencing anomalous behavior.

A composite chart grouped by _instance_ visualizes each instance of one software or hardware on a node and displays these as a separate dimension. By grouping the
`disk.io` chart by chart, you can visualize the activity of each disk on each node that contributes to the composite
chart.

### Aggregate functions over data sources

Each chart uses an opinionated-but-valuable default aggregate function over the data sources. For example, the `system.cpu` chart shows the
average for each dimension from every contributing chart, while the `net.net` chart shows the sum for each dimension
from every contributing chart, which can also come from multiple networking interfaces.

The following aggregate functions are available for each selected dimension:

-   **Average**: Displays the average value from contributing nodes. If a composite chart has 5 nodes with the following
    values for the `out` dimension&mdash;`-2.1`, `-5.5`, `-10.2`, `-15`, `-0.1`&mdash;the composite chart displays a
    value of `−6.58`.
-   **Sum**: Displays the sum of contributed values. Using the same nodes, dimension, and values as above, the composite
    chart displays a metric value of `-32.9`.
-   **Min**: Displays a minimum value. For dimensions with positive values, the min is the value closest to zero. For
    charts with negative values, the min is the value with the largest magnitude.
-   **Max**: Displays a maximum value. For dimensions with positive values, the max is the value with the largest
    magnitude. For charts with negative values, the max is the value closet to zero.

### Dimensions

Select which dimensions to display on the composite chart. You can choose **All dimensions**, a single dimension, or any
number of dimensions available on that context.

### Instances

Click on **X Instances** to display a dropdown of instances and nodes contributing to that composite chart. Each line in the
dropdown displays an instance name and the associated node's hostname.

### Nodes

Click on **X Nodes** to display a dropdown of nodes contributing to that composite chart. Each line displays a hostname
to help you identify which nodes contribute to a chart.

If one or more nodes can't contribute to a given chart, the definition bar shows a warning symbol plus the number of
affected nodes, then lists them in the dropdown along with the associated error. Nodes might return errors because of
networking issues, a stopped `netdata` service, or because that node does not have any metrics for that context.

### Aggregate functions over time

When the granularity of the data collected is higher than the plotted points on the chart an aggregation function over time
is applied. By default the aggregation applied is _average_ but the user can choose different options from the following:
* Min
* Max
* Average
* Sum
* Incremental sum (Delta)
* Standard deviation
* Median
* Single exponential smoothing
* Double exponential smoothing
* Coefficient variation

For more details on each, you can refer to our Agent's HTTP API details on [Data Queries - Data Grouping](https://github.com/netdata/netdata/blob/master/web/api/queries/README.md).

### Reset to defaults

Click on the 3-dot icon (**⋮**) on any chart, then **Reset to Defaults**, to reset the definition bar to its initial
state.

## Jump to single-node dashboards

Click on **X Charts**/**X Nodes** to display one of the two dropdowns that list the charts and nodes contributing to a
given composite chart. For example, the nodes dropdown.

![The nodes dropdown in a composite
chart](https://user-images.githubusercontent.com/1153921/99305049-7c019b80-2810-11eb-942a-8ebfcf236b7f.png)

To jump to a single-node dashboard, click on the link icon <img class="img__inline img__inline--link"
src="https://user-images.githubusercontent.com/1153921/95762109-1d219300-0c62-11eb-8daa-9ba509a8e71c.png" /> next to the
node you're interested in.

The single-node dashboard opens in a new tab. From there, you can continue to troubleshoot or run [Metric
Correlations](https://github.com/netdata/netdata/blob/master/docs/tasks/operations/find-the-root-cause-of-an-issue-with-metric-correlations.md) for faster root cause analysis.

## Add composite charts to a dashboard

Click on the 3-dot icon (**⋮**) on any chart, then click on **Add to Dashboard**. Click the **+** button for any
dashboard you'd like to add this composite chart to, or create a new dashboard an initiate it with your chosen chart by
entering the name and clicking **New Dashboard**.

## Interacting with composite charts: pan, zoom, and resize

You can interact with composite charts as you would with other Netdata charts. You can use the controls beneath each
chart to pan, zoom, or resize the chart, or use various combinations of the keyboard and mouse. See the [interact with the charts Task](https://github.com/netdata/netdata/blob/master/docs/tasks/operations/interact-with-the-charts.md) for details.

## Menu

The Overview uses a similar menu to local Agent dashboards and single-node dashboards in Netdata Cloud, with sections
and sub-menus aggregated from every contributing node. For example, even if only two nodes actively collect from and
monitor an Apache web server, the **Apache** section still appears and displays composite charts from those two nodes.

![A menu in the Overview
screen](https://user-images.githubusercontent.com/1153921/95785094-fa0ad980-0c89-11eb-8328-2ff11ac630b4.png)

One difference between the Overview's menu and those found in single-node dashboards or local Agent dashboards is that
the Overview condenses multiple services, families, or instances into single sections, sub-menus, and associated charts.

For services, let's say you have two concurrent jobs with the [web_log
collector](https://github.com/netdata/go.d.plugin/blob/master/modules/weblog/README.md), one for Apache and another for Nginx. A single-node or
local dashboard shows two section, **web_log apache** and **web_log nginx**, whereas the Overview condenses these into a
single **web_log** section containing composite charts from both jobs.

The Overview also consdenses multiple families or multiple instances into a single **all** sub-menu and associated
charts. For example, if Node A has 5 disks, and Node B has 3, each disk contributes to a single `disk.io` composite
chart. The utility bar should show that there are 8 charts from 2 nodes contributing to that chart.

This action applies to disks, network devices, and other metric types that involve multiple instances of a piece of
hardware or software. The Overview currently does not display metrics from filesystems.

## Persistence of composite chart settings

When you change a composite chart via its definition bar, Netdata Cloud persists these settings in a query string
attached to the URL in your browser. You can "save" these settings by bookmarking this particular URL, or share it with
colleagues by having them copy-paste it into their browser.


The Nodes view lets you see and customize key metrics from any number of Agent-monitored nodes and seamlessly navigate
to any node's dashboard for troubleshooting performance issues or anomalies using Netdata's highly-granular metrics.

![The Nodes view in Netdata
Cloud](https://user-images.githubusercontent.com/1153921/119035218-2eebb700-b964-11eb-8b74-4ec2df0e457c.png)

Each War Room's Nodes view is populated based on the nodes you added to that specific War Room. Each node occupies a
single row, first featuring that node's alarm status (yellow for warnings, red for critical alarms) and operating
system, some essential information about the node, followed by columns of user-defined key metrics represented in
real-time charts.

Use the Overview for monitoring an infrastructure in real time using
composite charts and Netdata's familiar dashboard UI.

Check the [War Room docs](https://github.com/netdata/netdata/blob/master/docs/tasks/setup/space-administration/rooms.md) for details on the utility bar, which contains the node
filter and the timeframe
selector.

## Add and customize metrics columns

Add more metrics columns by clicking the gear icon. Choose the context you'd like to add, give it a relevant name, and
select whether you want to see all dimensions (the default), or only the specific dimensions your team is interested in.

Click the gear icon and hover over any existing charts, then click the pencil icon. This opens a panel to
edit that chart. Edit the context, its title, add or remove dimensions, or delete the chart altogether.

These customizations appear for anyone else with access to that War Room.

## Jump to single-node dashboards

Click on the name of any node to seamlessly navigate to that node's dashboard. This is the same dashboard that comes
pre-configured with every installation of the Netdata Agent, so it features thousands of metrics and hundreds of
interactive charts without needing to waste time setting it up.

![Screenshot of an single-node
dashboard](https://user-images.githubusercontent.com/1153921/119035389-5e9abf00-b964-11eb-8e51-29af0c55b65d.png)

With all of the an individual node's real-time data at your fingertips, you can first identify health or performance
anomalies with Netdata Cloud, and then engage your team to perform root-cause analysis using the Agent's granular
metrics.

## See more metrics in Netdata Cloud

If you want to add more metrics to your War Rooms and they don't show up when you add new metrics to Nodes, you likely
need to configure those nodes to collect from additional data sources. See our [Agent configuration Task](https://github.com/netdata/netdata/blob/master/docs/tasks/setup/agent-configuration.md#-configure-a-collector-module) 
to learn how to use dozens of pre-installed collectors that can instantly collect from your favorite services and applications.

If you want to see up to 30 days of historical metrics in Cloud (and more on individual node dashboards), read our [Agent configuration Task](https://github.com/netdata/netdata/blob/master/docs/tasks/setup/agent-configuration.md#change-the-agents-metric-retention). Also, see our
[calculator](/docs/store/change-metrics-storage#calculate-the-system-resources-RAM-disk-space-needed-to-store-metrics)
for finding the disk and RAM you need to store metrics for a certain period of time.


## Related topics

### **Related Concepts**
- [Spaces](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-cloud/spaces.md)
- [Rooms](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-cloud/rooms.md)
- [Dashboards](https://github.com/netdata/netdata/blob/master/docs/concepts/visualizations/dashboards.md)
- [From raw metrics to visualizations](https://github.com/netdata/learn/blob/master/docs/concepts/visualizations/from-raw-metrics-to-visualization.md)

### Related Tasks
- [Claiming an existing agent to Cloud](https://github.com/netdata/netdata/blob/master/docs/tasks/setup/claim-existing-agent-to-cloud.md)
- [Interact with charts](https://github.com/netdata/netdata/blob/master/docs/tasks/operations/interact-with-the-charts.md)

