# Terminology Dictionary

This guide standardizes terminology for Netdata's documentation and communications. When referring to Netdata mechanisms or concepts, use these terms and definitions to ensure clarity and consistency.

When the context is clear, we can omit the "Netdata" prefix for brevity.

## Core Components

| Term                   | Definition                                                               |
|------------------------|--------------------------------------------------------------------------|
| **Agent** (**Agents**) | The core monitoring software that collects, processes and stores metrics |
| **Daemon**             | The main Netdata process                                                 |
| **Collector(s)**       | The various collectors of Netdata                                        |
| **Registry**           | The default Netdata Registry, or any Agent acting as one                 |

## Cloud

| Term                  | Definition                                                                            |
|-----------------------|---------------------------------------------------------------------------------------|
| **Cloud**             | The centralized platform for managing and visualizing Netdata metrics                 |
| **Claim(ing) Token**  | The token used to Connect the Agent to the Cloud                                      |
| **Connect(ing)(ion)** | The process of connecting the Agent to the Cloud. Do not use the word "claim" instead |
| **Cloud On Prem**     | The version of Cloud we ship for Businesses that want to run it on premises           |

## Database

| Term                 | Definition                                         |
|----------------------|----------------------------------------------------|
| **Tier** (**Tiers**) | Database storage layers with different granularity |
| **Mode(s)**          | The different Modes of the Database                |

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
