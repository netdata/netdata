# Node filter

The node filter allows you to quickly filter the nodes visualized in a War Room's views. It appears on all views, except on single-node dashboards.

Inside the filter, the nodes get categorized into three groups:

| Group   | Description                                                                                                                                                                                                                                                                                                                                                                                                           |
|---------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Live    | Nodes that are currently online, collecting and streaming metrics to Cloud. Live nodes display raised [Alert](https://github.com/netdata/netdata/blob/master/docs/dashboards-and-charts/alerts-tab.md) counters, [Machine Learning](https://github.com/netdata/netdata/blob/master/src/ml/README.md) availability, and [Functions](https://github.com/netdata/netdata/blob/master/docs/cloud/netdata-functions.md) availability |
| Stale   | Nodes that are offline and not streaming metrics to Cloud. Only historical data can be presented from a parent node. For these nodes you can only see their ML status, as they are not online to provide more information                                                                                                                                                                                             |
| Offline | Nodes that are offline, not streaming metrics to Cloud and not available in any parent node. Offline nodes are automatically deleted after 30 days and can also be deleted manually.                                                                                                                                                                                                                                  |

By using the search bar, you can narrow down to specific nodes based on their name.

When you select one or more nodes, the total selected number will appear in the **Nodes** bar on the **Selected** field.

![The node filter](https://user-images.githubusercontent.com/70198089/225249850-60ce4fcc-4398-4412-a6b5-6082308f4e60.png)
