# Nodes tab

The Nodes tab lets you see and customize key metrics from any number of Agent-monitored nodes and seamlessly navigate
to any node's dashboard for troubleshooting performance issues or anomalies using Netdata's highly-granular metrics.

![The Nodes tab in Netdata
Cloud](https://user-images.githubusercontent.com/1153921/119035218-2eebb700-b964-11eb-8b74-4ec2df0e457c.png)

Each War Room's Nodes tab is populated based on the nodes you added to that specific War Room. Each node occupies a
single row, first featuring that node's alarm status (yellow for warnings, red for critical alarms) and operating
system, some essential information about the node, followed by columns of user-defined key metrics represented in
real-time charts.

Use the [Overview](https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/overview.md) for monitoring an infrastructure in real time using
composite charts and Netdata's familiar dashboard UI.

Check the [War Room docs](https://github.com/netdata/netdata/blob/master/docs/cloud/war-rooms.md) for details on the utility bar, which contains the [node
filter](https://github.com/netdata/netdata/blob/master/docs/cloud/war-rooms.md#node-filter) and the [timeframe
selector](https://github.com/netdata/netdata/blob/master/docs/cloud/war-rooms.md#play-pause-force-play-and-timeframe-selector).

## Add and customize metrics columns

Add more metrics columns by clicking the gear icon. Choose the context you'd like to add, give it a relevant name, and
select whether you want to see all dimensions (the default), or only the specific dimensions your team is interested in.

Click the gear icon and hover over any existing charts, then click the pencil icon. This opens a panel to
edit that chart. Edit the context, its title, add or remove dimensions, or delete the chart altogether.

These customizations appear for anyone else with access to that War Room.

## See more metrics in Netdata Cloud

If you want to add more metrics to your War Rooms and they don't show up when you add new metrics to Nodes, you likely
need to configure those nodes to collect from additional data sources. See our [collectors configuration reference](https://github.com/netdata/netdata/blob/master/collectors/REFERENCE.md) 
to learn how to use dozens of pre-installed collectors that can instantly collect from your favorite services and applications.

If you want to see up to 30 days of historical metrics in Cloud (and more on individual node dashboards), read about [changing how long Netdata stores metrics](https://github.com/netdata/netdata/blob/master/docs/store/change-metrics-storage.md). Also, see our
[calculator](https://github.com/netdata/netdata/blob/master/docs/store/change-metrics-storage.md#calculate-the-system-resources-ram-disk-space-needed-to-store-metrics)
for finding the disk and RAM you need to store metrics for a certain period of time.
