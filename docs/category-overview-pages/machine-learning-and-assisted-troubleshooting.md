# Machine Learning and Anomaly Detection

Netdata includes advanced Machine Learning capabilities to help you detect and resolve anomalies in your infrastructure before they escalate into critical issues. These features provide real-time insights and proactive monitoring to improve system reliability.

## Key Features

### Anomaly Detection with K-Means Clustering
Netdata trains K-means clustering models to detect anomalies in your infrastructure. These models power the [Anomaly Advisor](/docs/dashboards-and-charts/anomaly-advisor-tab.md), which visually highlights anomalies on the dashboard, allowing you to quickly identify and investigate unexpected behavior.

### Metric Correlations
Netdata enables metric correlation analysis through the dashboard. This feature uses the [Two-sample Kolmogorov-Smirnov test](https://en.wikipedia.org/wiki/Kolmogorov%E2%80%93Smirnov_test#Two-sample_Kolmogorov%E2%80%93Smirnov_test) and volume heuristic measures to help you understand relationships between different metrics and identify potential causes of anomalies.

### Netdata Assistant for Troubleshooting
The [Netdata Assistant](/docs/netdata-assistant.md) provides AI-driven assistance for troubleshooting alerts and anomalies. You can interact with it directly to get explanations, recommendations, and next steps based on detected anomalies and system behavior.

These Machine Learning features enhance observability and streamline incident response, helping you maintain system health with greater efficiency.