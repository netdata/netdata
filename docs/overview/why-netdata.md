<!--
title: "Why use Netdata?"
description: "Netdata is simple to deploy, scalable, and optimized for troubleshooting. Cut the complexity and expense out of your monitoring stack."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/overview/why-netdata.md
-->

# Why use Netdata?

Netdata takes a different approach to helping people build extraordinary infrastructure. It was built out of frustration
with existing monitoring tools that are too complex, too expensive, and don't help their users actually troubleshoot
complex performance and health issues.

Netdata is:

## Simple to deploy

-   **One-line deployment** for Linux distributions, plus support for Kubernetes/Docker infrastructures
-   **Zero configuration and maintenance** required to collect thousands of metrics, every second, from the underlying
    OS and running applications.
-   **Prebuilt charts and alarms** alert you to common anomalies and performance issues without manual configuration.
-   **Distributed storage** to simplify the cost and complexity of storing metrics data from any number of nodes.

## Powerful and scalable

-   **1% CPU utilization, a few MB of RAM, and minimal disk I/O** to run the monitoring Agent on bare metal, virtual
    machines, containers, and even IoT devices.
-   **Per-second granularity** for an unlimited number of metrics based on the hardware and applications you're running
    on your nodes.
-   **Interoperable exporters** let you connect Netdata's per-second metrics with an existing monitoring stack and other
    time-series databases.

## Optimized for troubleshooting

-   **Visual anomaly detection** with a UI/UX that emphasizes the relationships between charts.
-   **Customizable dashboards** to pinpoint correlated metrics, respond to incidents, and help you streamline your
    workflows.
-   **Distributed metrics in a centralized interface** to assist users or teams trace complex issues between distributed
    nodes.

## Comparison with other monitoring solutions

Netdata offers many benefits over the existing monitoring landscape, whether they're expensive SaaS products or other
open-source tools.

| Netdata                                                         | Others (open-source and commercial)                              |
| :-------------------------------------------------------------- | :--------------------------------------------------------------- |
| **High resolution metrics** (1s granularity)                    | Low resolution metrics (10s granularity at best)                 |
| Collects **thousands of metrics per node**                      | Collects just a few metrics                                      |
| Fast UI optimized for **anomaly detection**                     | UI is good for just an abstract view                             |
| **Long-term, autonomous storage** at one-second granularity     | Centralized metrics in an expensive data lake at 10s granularity |
| **Meaningful presentation**, to help you understand the metrics | You have to know the metrics before you start                    |
| Install and get results **immediately**                         | Long sales process and complex installation process              |
| Use it for **troubleshooting** performance problems             | Only gathers _statistics of past performance_                    |
| **Kills the console** for tracing performance issues            | The console is always required for troubleshooting               |
| Requires **zero dedicated resources**                           | Require large dedicated resources                                |

## What's next?

Whether you already have a monitoring stack you want to integrate Netdata into, or are building something from the
ground-up, you should read more on how Netdata can work either [standalone or as an interoperable part of a monitoring
stack](/docs/overview/netdata-monitoring-stack.md).

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Foverview%2Fwhy-netdata&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
