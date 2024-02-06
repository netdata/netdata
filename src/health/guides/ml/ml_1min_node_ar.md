### Understand the alert

This alert is triggered when the [node anomaly rate](https://learn.netdata.cloud/docs/ml-and-troubleshooting/machine-learning-ml-powered-anomaly-detection#node-anomaly-rate) exceeds the threshold defined in the [alert configuration](https://github.com/netdata/netdata/blob/master/src/health/health.d/ml.conf) over the most recent 1 minute window evaluated. 

For example, with the default of `warn: $this > 1`, this means that 1% or more of the metrics collected on the node have across the most recent 1 minute window been flagged as [anomalous](https://learn.netdata.cloud/docs/ml-and-troubleshooting/machine-learning-ml-powered-anomaly-detection) by Netdata.

### Troubleshoot the alert

This alert is a signal that some significant percentage of metrics within your infrastructure have been flagged as anomalous accoring to the ML based anomaly detection models the Netdata agent continually trains and re-trains for each metric. This tells us something somewhere might look strange in some way. THe next step is to try drill in and see what metrics are actually driving this.

1. **Filter for the node or nodes relevant**: First we need to reduce as much noise as possible by filtering for just those nodes that have the elevated node anomaly rate. Look at the `anomaly_detection.anomaly_rate` chart and group by `node` to see which nodes have an elevated anomaly rate. Filter for just those nodes since this will reduce any noise as much as possible.

2. **Highlight the area of interest**: Highlight the timeframne of interest where you see an elevated anomaly rate.

3. **Check the anomalies tab**: Check the [Anomaly Advisor](https://learn.netdata.cloud/docs/ml-and-troubleshooting/anomaly-advisor) ("Anomalies" tab) to see an ordered list of what metrics were most anomalous in the highlighted window.

4. **Press the AR% button on Overview**: You can also press the "[AR%](https://blog.netdata.cloud/anomaly-rates-in-the-menu/)" button on the Overview or single node dashboard to see what parts of the menu have the highest chart anomaly rates. Pressing the AR% button should add some "pills" to each menu item and if you hover over it you will see that chart within each menu section that was most anomalous during the highlighted timeframe. 

5. **Use Metric Correlations**: Use [metric correlations](https://learn.netdata.cloud/docs/ml-and-troubleshooting/metric-correlations) to see what metrics may have changed most significantly comparing before to the highlighted timeframe.

### Useful resources

1. [Machine learning (ML) powered anomaly detection](https://learn.netdata.cloud/docs/ml-and-troubleshooting/machine-learning-ml-powered-anomaly-detection)
2. [Anomaly Advisor](https://learn.netdata.cloud/docs/ml-and-troubleshooting/anomaly-advisor)
3. [Metric Correlations](https://learn.netdata.cloud/docs/ml-and-troubleshooting/metric-correlations)
4. [Anomaly Rates in the Menu!](https://blog.netdata.cloud/anomaly-rates-in-the-menu/)
