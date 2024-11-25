# Terminology Dictionary

This guide standardizes terminology for Netdata's documentation and communications. When referring to Netdata mechanisms or concepts, use these terms and definitions to ensure clarity and consistency.

When the context is clear, we can omit the "Netdata" prefix for brevity.

## Core Components

| Term                   | Definition                                                               |
|------------------------|--------------------------------------------------------------------------|
| **Agent** (**Agents**) | The core monitoring software that collects, processes and stores metrics |
| **Cloud**              | The centralized platform for managing and visualizing Netdata metrics    |

## Database

| Term                 | Definition                                         |
|----------------------|----------------------------------------------------|
| **Tier** (**Tiers**) | Database storage layers with different granularity |

## Streaming

| Term                     | Definition                                                  |
|--------------------------|-------------------------------------------------------------|
| **Parent** (**Parents**) | An Agent that receives metrics from other Agents (Children) |
| **Child** (**Children**) | An Agent that streams metrics to another Agent (Parent)     |

## Machine Learning

| Term                    | Abbreviation | Definition                                                                                      |
|-------------------------|:------------:|-------------------------------------------------------------------------------------------------|
| **Machine Learning**    |      ML      | An umbrella term for Netdata's ML-powered features                                              |
| **Model(s)**            |              | Uppercase when referring to the ML Models Netdata uses                                          |
| **Anomaly Detection**   |              | The capability to identify unusual patterns in metrics                                          |
| **Metric Correlations** |              | Filters dashboard to show metrics with the most significant changes in the selected time window |
| **Anomaly Advisor**     |              | The interface and tooling for analyzing detected anomalies                                      |