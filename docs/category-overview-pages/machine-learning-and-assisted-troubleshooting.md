# Machine Learning and Anomaly Detection

In order to help you troubleshoot scenarios where anomalies and unexpected behavior occurs on your infrastructure, Netdata provides the following Machine Learning features:

- K-means clustering [Machine Learning models](/src/ml/README.md) are trained to power the [Anomaly Advisor](/docs/dashboards-and-charts/anomaly-advisor-tab.md) on the dashboard, which allows you to identify Anomalies in your infrastructure.
- [Metric Correlations](/docs/metric-correlations.md) are possible through the dashboard using the [Two-sample Kolmogorov Smirnov](https://en.wikipedia.org/wiki/Kolmogorov%E2%80%93Smirnov_test#Two-sample_Kolmogorov%E2%80%93Smirnov_test) statistical test and Volume heuristic measures.
- The [Netdata Assistant](/docs/netdata-assistant.md) is able to answer your prompts when it comes to troubleshooting Alerts and Anomalies.
