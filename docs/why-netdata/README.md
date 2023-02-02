<!--
title: "Why Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/why-netdata/README.md
-->

# Why Netdata

> Any performance monitoring solution that does not go down to per second
> collection and visualization of the data, is useless.
> It will make you happy to have it, but it will not help you more than that. 

Netdata is built around 4 principles:

1.  **[Per second data collection for all metrics.](https://github.com/netdata/netdata/blob/master/docs/why-netdata/1s-granularity.md)**

    _It is impossible to monitor a 2 second SLA, with 10 second metrics._

2.  **[Collect and visualize all the metrics from all possible sources.](https://github.com/netdata/netdata/blob/master/docs/why-netdata/unlimited-metrics.md)**

    _To troubleshoot slowdowns, we need all the available metrics. The console should not provide more metrics._

3.  **[Meaningful presentation, optimized for visual anomaly detection.](https://github.com/netdata/netdata/blob/master/docs/why-netdata/meaningful-presentation.md)**

    _Metrics are a lot more than name-value pairs over time. The monitoring tool should know all the metrics. Users should not!_

4.  **[Immediate results, just install and use.](https://github.com/netdata/netdata/blob/master/docs/why-netdata/immediate-results.md)**

    _Most of our infrastructure is standardized. There is no point to configure everything metric by metric._

Unlike other monitoring solutions that focus on metrics visualization,
Netdata's helps us troubleshoot slowdowns without touching the console.

So, everything is a bit different.


