# Machine Learning and Anomaly Detection

Netdata provides a variety of Machine Learning features to help you troubleshoot certain scenarios that might come up.

- K-means clustering [Machine Learning models](https://github.com/netdata/netdata/blob/master/src/ml/README.md) are trained to power the [Anomaly Advisor](https://github.com/netdata/netdata/blob/master/docs/dashboards-and-charts/anomaly-advisor-tab.md) on the dashboard, which allows you to identify anomalies in your infrastructure
- [Metric Correlations](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/metric-correlations.md) are possible through the dashboard using the [Two-sample Kolmogorov Smirnov](https://en.wikipedia.org/wiki/Kolmogorov%E2%80%93Smirnov_test#Two-sample_Kolmogorov%E2%80%93Smirnov_test) statistical test and Volume heuristic measures
- The [Netdata Assistant](https://github.com/netdata/netdata/blob/master/docs/cloud/netdata-assistant.md) is able to answer your prompts when it comes to troubleshooting alerts and anomalies.
