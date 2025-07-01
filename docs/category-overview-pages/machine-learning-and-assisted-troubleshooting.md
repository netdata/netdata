# Netdata AI and Machine Learning

Boost your monitoring and troubleshooting capabilities with Netdata's AI-powered features.

Netdata AI helps you **detect anomalies, understand metric relationships, and resolve issues quickly** with intelligent assistance all designed to make your infrastructure management smarter, faster, and bulletproof.

## What Can Netdata AI Do For You?

Netdata AI combines powerful machine learning capabilities with intuitive interfaces to help you:

1. **Detect anomalies automatically** before they escalate into critical issues
2. **Understand relationships** between metrics during troubleshooting
3. **Get expert guidance** when resolving alerts and performance problems

## Machine Learning and Anomaly Detection

Our ML-powered anomaly detection works silently in the background, monitoring your metrics and identifying unusual patterns.

| Feature                          | What It Does For You                                                         |
|----------------------------------|------------------------------------------------------------------------------|
| **Unsupervised Learning**        | Works automatically without requiring manual training or labeling of data    |
| **Multiple Model Consensus**     | Reduces false positives by 99% by requiring agreement across multiple models |
| **Real-time Anomaly Bits**       | Flags unusual metrics instantly, with zero storage overhead                  |
| **Anomaly Rate Visualization**   | Highlights anomalous time periods in your dashboard for quick investigation  |
| **Node-Level Anomaly Detection** | Identifies when your entire system is behaving unusually                     |
| **Metric Correlations**          | Helps you find relationships between metrics to pinpoint root causes         |

Learn more in the [Machine Learning and Anomaly Detection](/src/ml/README.md) documentation.

## Netdata Assistant

When alerts trigger or anomalies emerge, Netdata Assistant serves as your AI-powered troubleshooting companion.

| Feature                    | What It Does For You                                                  |
|----------------------------|-----------------------------------------------------------------------|
| **Alert Context**          | Explains what each alert means and why you should care about it       |
| **Guided Troubleshooting** | Offers step-by-step instructions tailored to your specific situation  |
| **Persistent Window**      | Follows you throughout your dashboards as you investigate issues      |
| **Curated Resources**      | Provides links to relevant documentation to deepen your understanding |
| **Time-Saving**            | Eliminates the need for searching documentation or online forums      |

Learn more about [Netdata Assistant](/docs/netdata-assistant.md) and how it helps streamline your troubleshooting workflow.

## Getting Started

Netdata AI features are enabled by default with the standard installation. The machine learning capabilities require the `dbengine` database mode, which is the default setting.

To start exploring:

1. **Anomaly Detection**: Check the [Anomaly Advisor tab](/docs/dashboards-and-charts/anomaly-advisor-tab.md) to see detected anomalies
2. **Metric Correlations**: Use the Metric Correlations button in the dashboard to analyze relationships between metrics
3. **Netdata Assistant**: Click the Assistant button in the Alerts tab when troubleshooting alerts

These AI features work seamlessly with Netdata's other capabilities, enhancing your overall monitoring and troubleshooting experience without requiring any AI expertise.
