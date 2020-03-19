<!--
---
title: "Why Netdata"
description: "Netdata has a different approach to monitoring because SysAdmins, DevOps, and Developers designed it for troubleshooting performance problems, not just visualizing metrics. Unlike other monitoring solutions that focus on metrics visualization, Netdata helps troubleshoot slowdowns without touching the console. So, everything is a bit different."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/why-netdata/README.md
keywords:
  - 1s granularity
  - per second data collection
  - troubleshooting performance problems
  - immediate results
  - meaningful presentation
  - interactive dashboard
  - high resolution metrics
  - zero dedicated resources
  - metrics visualization
  - troubleshoot slowdowns
  - autonomous
  - integrations
  - integrate with other monitoring tools
  - anomaly detection
  - time-series database
  - unlimited metrics
date: 2020-03-19
---
-->

# Why Netdata?

etdata has a different approach to monitoring because SysAdmins, DevOps, and Developers designed it for troubleshooting performance problems, not just visualizing metrics. Unlike other monitoring solutions that focus on metrics visualization, Netdata helps troubleshoot slowdowns without touching the console. So, everything is a bit different.

t's fast and efficient and designed to permanently run on all systems (physical & virtual servers, containers, IoT devices), without disrupting their core function. It's distributed and provides unparalleled insights, in real-time, of everything happening on your systems. That means it monitors thousands of high-resolution metrics per node; 1s granularity instead of 10s at best. That's super fast!

ith Netdata, you get results immediately in a meaningful presentation that helps you understand the metrics. Netdata's dashboard is interactive, instead of just an abstract view and optimized for anomaly detection. Netdata stores and retrieves data records that are part of a "time series" and doesn't touch the disks while it runs.
       
See how Netdata compares to other monitoring solutions:
       
| Netdata | others (open-source and commercial) |
| :-----: | :---------------------------------: |
| **High resolution metrics** (1s granularity) | Low resolution metrics (10s granularity at best) |
| Monitors everything, **thousands of metrics per node** | Monitor just a few metrics |
| UI is super fast, optimized for **anomaly detection** | UI is good for just an abstract view |
| **Meaningful presentation**, to help you understand the metrics | You have to know the metrics before you start |
| Install and get results **immediately** | Long preparation is required to get any useful results |
| Use it for **troubleshooting** performance problems | Use them to get *statistics of past performance* |
| **Kills the console** for tracing performance issues | The console is always required for troubleshooting |
| Requires **zero dedicated resources** | Require large dedicated resources|

## Netdata's features
Netdata is free, open-source software that currently runs on Linux, FreeBSD, and macOS.  It runs autonomously, without any third-party components, or it can integrate with existing monitoring toolchains, like Prometheus, Graphite, OpenTSDB, Kafka, Grafana, and more.
 
We built Netdata around four principles:
1.  [**1s granularity**](#1s-granularity). It is impossible to monitor a 2 second SLA, with 10-second metrics.
2.  [**Unlimited metrics**](#unlimited-metrics). To troubleshoot slowdowns, we need all the available metrics.
3.  [**Meaningful presentation**](#meaningful-presentation). The monitoring tool should know all the metrics. Users should not!
4.  [**Immediate results**](#immediate-results). Just install and use it.
 
### 1s granularity
Any performance monitoring solution that does not go down to per second collection, and visualization of the data is useless.  It makes you happy to have it, but it doesn't help you more than that, unfortunately.
 
The world is going real-time, and today, response time affects the customer experience significantly, so SLAs are tighter than ever before. That's why high-resolution metrics are required to monitor and troubleshoot systems and applications effectively.
 
Additionally, IT has gone virtual, and unlike real hardware, virtual environments are not linear nor predictable. You also can't expect resources to be available when your applications need them. Plus, the latency of virtual environments is affected by many factors, most of which are outside our control, such as the: 


*    Maintenance policy of the hosting provider 
*    Workload of third-party virtual machines running on the same physical servers combined with the resource allocation and throttling policy among virtual machines
*    Provisioning system of the hosting provider

#### What do others do?
Although most monitoring platforms and monitoring SaaS providers want to offer high-resolution metrics, they can't, at least not massively. 

**Here's why they can't:**

*    They centralize the metrics, which can only scale-up, because:
     - Time-series databases like Prometheus, Graphite, OpenTSDB, and InfluxDB quickly become the bottleneck of the entire infrastructure
     - Increased bandwidth and monitoring costs because of the investment of bigger and bigger central components 
     - Massively supporting high-resolution metrics, destroys their business model.
*    The data collection agent consumes significant system resources when running per second, influencing the monitored systems and application to a high degree. 
*    Per second data collection is a lot harder because busy virtual environments have [a constant latency of about 100ms, spread randomly to all data sources](https://docs.google.com/presentation/d/18C8bCTbtgKDWqPa57GXIjB2PbjjpjsUNkLtZEz6YK8s/edit#slide=id.g422e696d87_0_57).  If data collection does not get implemented correctly, this latency introduces a random error of +/- 10%, which is quite significant for a monitoring system.

#### What does Netdata do differently?
As we've mentioned earlier, Netdata does things slightly different, and for a good reason.  Unlike other monitoring solutions that focus on metrics visualization, Netdata solves the centralization problem of monitoring and helps troubleshoot slowdowns without touching the console.  

**Here's why Netdata is different:**

*    Netdata completely decentralizes monitoring so that it can scale out horizontally by adding more smaller nodes to it. Each Netdata node is autonomous, which allows Netdata to scale to infinity because it: 
     - Collects and stores metrics locally
     - Runs checks against the metrics to trigger alarms locally
     - Provides an API for the dashboards to visualize metrics

*    Of course, Netdata can centralize metrics when needed. For example, it isn't practical to keep metrics locally on ephemeral nodes. For these cases, Netdata streams the metrics in real-time, from the ephemeral nodes to one or more non-ephemeral nodes nearby. Although centralized, it is again distributed, so on a large infrastructure, there may be many centralization points.

*    Netdata interpolates collected metrics to eliminate the error introduced by data collection latencies on busy virtual environments. It does this using microsecond timings, per data source, offering measurements with an error rate of 0.0001%. When running [in debug mode, Netdata calculates this error rate](https://github.com/netdata/netdata/blob/36199f449852f8077ea915a3a14a33fa2aff6d85/database/rrdset.c#L1070-L1099) for every point collected, ensuring that the database works with acceptable accuracy.

*    Netdata is just FAST because optimization is a core product feature. On modern hardware, Netdata can collect metrics with a rate of above 1M metrics per second per core (this includes everything, parsing data sources, interpolating data, storing data in the time-series database, etc.). So, for a few thousand metrics per second per node, Netdata needs negligible CPU resources (just 1-2% of a single core). 

So, for Netdata 1s granularity is easy, the natural outcome!

### Unlimited metrics
Collecting all the metrics breaks the first rule of every monitoring textbook: "collect only the metrics you need," "collect only the metrics you understand." Unfortunately, this does **not** work! Filtering out most metrics is like reading a book by skipping most of its pages.

For many people, monitoring is about:

-   Detecting outages
-   Capacity planning

However, **slowdowns are 10 times more common** compared to outages (check slide 14 of [Online Performance is Business Performance ](https://www.slideshare.net/KenGodskind/alertsitetrac) reported by Trac Research/AlertSite). Designing a monitoring system targeting only outages and capacity planning solves just a tiny part of the operational problems we face. Check also [Downtime vs. Slowtime: Which Hurts More?](https://dzone.com/articles/downtime-vs-slowtime-which-hurts-more).

All metrics are important, and all metrics should be available when you need them.  For example, to troubleshoot a slowdown, all the metrics are needed, since the real cause of a slowdown is probably quite complex. If we knew the possible reasons, chances are we would have fixed them before they become a problem.

#### What do others do?

Most monitoring solutions, when they can detect something, provide just a hint (e.g., _"Hey, there is a 20% drop in requests per second over the last minute"_), and they expect us to use the console to determine the root cause.  But this introduces a problem with the console because how do you troubleshoot a slowdown if the slowdown lifetime is just a few seconds, randomly spread throughout the day?

You can't! You end up spending your entire day on the console, waiting for the problem to happen again while logged in. A blame war starts: developers blame the systems, sysadmins blame the hosting provider, someone says it is a DNS problem, another one believes it is network related, and so on. We have all experienced this multiple times!
So, why do monitoring solutions and SaaS providers filter out metrics?

**They can't do otherwise!**

1.  Centralization of metrics depends on metrics filtering, to control monitoring costs. Time-series databases limit the number of metrics collected because the number of metrics influences their performance significantly. They get congested at scale.
2.  It is a lot easier to provide an illusion of monitoring by using a few basic metrics.
3.  Troubleshooting slowdowns is the hardest IT problem to solve, so most solutions avoid it.

#### What does Netdata do?

Netdata collects, stores, and visualizes everything, every single metric exposed by systems and applications. The console should not provide more metrics.  Because of Netdata's distributed nature, the number of metrics collected does not have any noticeable effect on the performance or the cost of the monitoring infrastructure.

Of course, since Netdata is also about [meaningful presentation](meaningful-presentation.md), the number of metrics makes Netdata development slower.  So, we need to have a good understanding of the metrics before adding them to Netdata. We organize the metrics, add information related to them, configure alarms for them, so that you, the Netdata users, have the best out-of-the-box experience and all the information required to kill the console for troubleshooting slowdowns.

### Meaningful presentation

Our world has become information-oriented and data-driven, and we're crazy about metrics. So, it becomes essential to know how to interpret meaningful information from the data. 

Metrics have a context, a meaning, and a way to interpret them, the presentation of data requires skills and understanding of data. It is necessary to make use of collected data, which must be processed to put to any use or application. 

The presentation should move from the data and to the insight and action that results from it. 

In other words, 
_What happened there?_
and
_What do we do about it?_

Data is meaningful if we have some way to act upon it, which helps you make an assessment. But even if all the metrics are collected, an even more significant challenge is revealed. What do you do with them, and how do you use them?


#### What do others do?

Although monitoring services have the same goal or health and performance monitoring, not everyone is alike when it comes to presenting the data.

**Here's why they're different:**

*    They are a time-series database that tracks name-value pairs over time, and data collections that blindly collect metrics and stores them in the database.  A dashboard editor queries the database to visualize the metrics.  

*    They collect limited metrics, but for any issue that you need to troubleshoot, the monitoring system doesn't help, so you have no choice but to use the console to access the rest of the metrics and find the root cause.

*    They instruct engineers to collect ONLY the metrics they understand, plus have:

     - A deep understanding of IT architectures and is skillful as a SysAdmin.
     - Experience as a database administrator.
     - Some software engineering and capable of understanding the internal workings of applications. 
     - Strong knowledge of Data Science, statistical algorithms, and can write advanced mathematical queries.

*    They assume the engineers will also design dashboards, configure alarms, and use a query language to investigate issues. But all of these have to be configured metric by metric. 

#### What does Netdata do?

Netdata can be browsed by humans, even if the database has 100,000 unique metrics. It is pretty natural for everyone to browse them and understand their meaning and scope. So, you can focus on solving your infrastructure problem, not on the technicalities of data collection and visualization.

**Here's why Netdata is different:**

*    The dashboard is enriched with information that is useful for most people. Netdata includes the community knowledge and expertise about the metrics right in the dashboard to improve clarity.

*    The metrics are incorporated into the database where they get converted, stored, and organized into a human-friendly format.  For example, CPU utilization in Netdata gets stored as percentages instead of kernel ticks.

*    All metrics are organized into human-friendly charts, sharing the same context and units (like what other monitoring solutions call 'cardinality'). So, when Netdata developer collects metrics, they configure the correlation of the metrics right in data collection, which is stored in the database too.

*    All charts get organized in families and then organized in applications, which are responsible for the dashboard menu, allowing you to explore the whole database. 

### Immediate results

Open-source solutions rely almost entirely on configuration, leaving you to go through endless metric-by-metric configuration. That's a lot of time and effort to configure your infrastructure dashboards and alarms.   So, why can't we have a monitoring system that can be installed and instantly provide feature-rich dashboards and hundreds of pre-configured alarms about everything we use? 

#### What do others do?

*    They focus on providing a platform "for building your monitoring," providing tools to collect metrics, store them, visualize them, check them, and query them. 
*    They offer a basic set of pre-configured metrics, dashboards, and alarms, assuming you configure the rest you may need. What a waste of time trying to understand what the metrics are, how to visualize them, how to configure alarms for them, and how to query them when issues arise.

#### What does Netdata do?

*    Immediately after installation, Netdata instantly provides feature-rich dashboards and hundreds of pre-configured alarms about everything we use. Of course, there are thousands of options to tweak, but the defaults are pretty good for most systems.
*    Engineers understand and learn what the metrics are and how to use them, thanks to the community expertise and experience for monitoring systems and applications.
*    No query languages or any other technology gets introduced. Of course, having some familiarity is required, but nothing too complicated. 







[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2FWhy-Netdata&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
