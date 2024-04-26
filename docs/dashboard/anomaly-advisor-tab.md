# Anomaly Advisor tab

The Anomaly Advisor tab lets you focus on potentially anomalous metrics and charts related to a particular highlighted window of interest. In addition to this tab, each chart in the [Metrics tab](https://github.com/netdata/netdata/blob/master/docs/dashboard/metrics-tab-and-single-node-tabs.md) also has an [Anomaly Rate ribbon](https://github.com/netdata/netdata/blob/master/docs/dashboard/netdata-charts.md#anomaly-rate-ribbon). 


More details about configuration can be found in the [ML documentation](https://github.com/netdata/netdata/blob/master/src/ml/README.md#configuration).

This tab uses our [Anomaly Rate ML feature](https://github.com/netdata/netdata/blob/master/src/ml/README.md#anomaly-rate---averageanomaly-bit) to score metrics in terms of anomalous behavior.

- The "Anomaly Rate" chart shows the percentage of anomalous metrics over time per node.

- The "Count of Anomalous Metrics" chart shows raw counts of anomalous metrics per node so may often be similar to the Anomaly Rate chart, apart from where nodes may have different numbers of metrics.

- The "Anomaly Events Detected" chart shows whether the anomaly rate per node has increased enough to cause a node-level anomaly. Anomaly events will appear slightly after the anomaly rate starts to increase in the timeline, this is because a significant number of metrics in the node need to be anomalous before an anomaly event is triggered.

Once you have highlighted a window of interest, you should see an ordered list of charts, with the Anomaly Rate being displayed as a purple ribbon in the chart.

> **Tip**
>
> You can also use the [node filter](https://github.com/netdata/netdata/blob/master/docs/dashboard/node-filter.md) to select which nodes you want to include or exclude.

The right side of the page displays an anomaly index for the highlighted timeline of interest. The index is sorted from most anomalous (highest level of anomaly) to least (lowest level of anomaly). Clicking on an entry in the index will get you to the corresponding chart for the anomalous metric.

## Usage Tips

- If you are interested in a subset of specific nodes then filtering to just those nodes before highlighting is recommended to get better results. When you highlight a timeframe, Netdata will ask the Agents for a ranking across all metrics, so if there is a subset of nodes there will be less "averaging" going on and you'll get a less noisy ranking.
- Ideally try and highlight close to a spike or window of interest so that the resulting ranking can narrow-in more easily on the timeline you are interested in.
