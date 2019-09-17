# 1s granularity

High resolution metrics are required to effectively monitor and troubleshoot systems and applications.

## Why?

-   The world is going real-time. Today, customer experience is significantly affected by response time, so SLAs are tighter than ever before. It is just not practical to monitor a 2-second SLA with 10-second metrics.

-   IT goes virtual. Unlike real hardware, virtual environments are not linear, nor predictable. You cannot expect resources to be available when your applications need them. They will eventually be, but not exactly at the time they are needed. The latency of virtual environments is affected by many factors, most of which are outside our control, like: the maintenance policy of the hosting provider, the work load of third party virtual machines running on the same physical servers combined with the resource allocation and throttling policy among virtual machines, the provisioning system of the hosting provider, etc.

## What do others do?

So, why don't most monitoring platforms and monitoring SaaS providers offer high resolution metrics?

They want to, but they can't, at least not massively.

The reasons lie in their design decisions:

1.  Time-series databases (prometheus, graphite, opentsdb, influxdb, etc) centralize all the metrics. At scale, these databases can easily become the bottleneck of the whole infrastructure.

2.  SaaS providers base their business models on centralizing all the metrics. On top of the time-series database bottleneck they also have increased bandwidth costs. So, massively supporting high resolution metrics, destroys their business model.

Of course, since a couple of decades the world has fixed this kind of scaling problems: instead of scaling up, scale out, horizontally. That is, instead of investing on bigger and bigger central components, decentralize the application so that it can scale by adding more smaller nodes to it.

There have been many attempts to fix this problem for monitoring. But so far, all solutions required centralization of metrics, which can only scale up. So, although the problem is somehow managed, it is still the key problem of all monitoring platforms and one of the key reasons for increased monitoring costs.

Another important factor is how resource efficient data collection can be when running per second. Most solutions fail to do it properly. The data collection agent is consuming significant system resources when running "per second", influencing the monitored systems and applications to a great degree.

Finally, per second data collection is a lot harder. Busy virtual environments have [a constant latency of about 100ms, spread randomly to all data sources](https://docs.google.com/presentation/d/18C8bCTbtgKDWqPa57GXIjB2PbjjpjsUNkLtZEz6YK8s/edit#slide=id.g422e696d87_0_57). If data collection is not implemented properly, this latency introduces a random error of +/- 10%, which is quite significant for a monitoring system.

So, the monitoring industry fails to massively provide high resolution metrics, mainly for 3 reasons:

1.  Centralization of metrics makes monitoring cost inefficient at that rate.
2.  Data collection needs optimization, otherwise it will significantly affect the monitored systems.
3.  Data collection is a lot harder, especially on busy virtual environments.

## What does Netdata do differently?

Netdata decentralizes monitoring completely. Each Netdata node is autonomous. It collects metrics locally, it stores them locally, it runs checks against them to trigger alarms locally, and provides an API for the dashboards to visualize them. This allows Netdata to scale to infinity.

Of course, Netdata can centralize metrics when needed. For example, it is not practical to keep metrics locally on ephemeral nodes. For these cases, Netdata streams the metrics in real-time, from the ephemeral nodes to one or more non-ephemeral nodes nearby. This centralization is again distributed. On a large infrastructure, there may be many centralization points.

To eliminate the error introduced by data collection latencies on busy virtual environments, Netdata interpolates collected metrics. It does this using microsecond timings, per data source, offering measurements with an error rate of 0.0001%. When running [in debug mode, Netdata calculates this error rate](https://github.com/netdata/netdata/blob/36199f449852f8077ea915a3a14a33fa2aff6d85/database/rrdset.c#L1070-L1099) for every point collected, ensuring that the database works with acceptable accuracy.

Finally, Netdata is really fast. Optimization is a core product feature. On modern hardware, Netdata can collect metrics with a rate of above 1M metrics per second per core (this includes everything, parsing data sources, interpolating data, storing data in the time series database, etc). So, for a few thousands metrics per second per node, Netdata needs negligible CPU resources (just 1-2% of a single core). 

Netdata has been designed to:

-   Solve the centralization problem of monitoring
-   Replace the console for performance troubleshooting.

So, for Netdata 1s granularity is easy, the natural outcome...

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fwhy-netdata%2F1s-granularity&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
