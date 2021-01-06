<!--
title: "Visualize and monitor anomalies with Netdata"
description: "TK"
image: /img/seo/guides/monitor/vizualize-monitor-anomalies.png
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/guides/monitor/vizualize-monitor-anomalies.md
-->

# Visualize and monitor anomalies with Netdata

Welcome to part 2 of our series of guides on using _unsupervised anomaly detection_ to detect issues with your systems,
containers, and applications using the open-source Netdata Agent. For an introduction into detecting anomalies and
monitoring associated metrics, see [part 1](/docs/guides/monitor/anomaly-detection.md), which covers prerequisites and
configuration basics.

With anomaly detection in the Netdata Agent set up, you will now want to visualize and monitor which charts have
anomalous data, when, and where to look next.

> 💡 In certain cases, the anomalies collector doesn't start immediately after restarting the Netdata Agent. If this
> happens, you won't see the dashboard section, or the relevant [charts](#visualize-anomalies-in-charts) right away.
> Wait a minute or two, refresh, and look again. If the anomalies charts and alarms are still not present, investigate
> the error log with `less /var/log/netdata/error.log | grep anomalies`.

## Visualize anomalies in charts

In either [Netdata Cloud](https://app.netdata.cloud) or the local Agent dashboard at `http://NODE:19999`, click on the
**Anomalies** [section](/web/gui/README.md#sections) to see the pair of anomaly detection charts, which are
preconfigured to visualize per-second anomaly metrics based on your [configuration in
`anomalies.conf`](/docs/guides/monitor/anomaly-detection.md#configure-the-anomalies-collector).

These charts have the contexts `anomalies_local.probability` and `anomalies_local.anomaly`.

![Two charts created by the anomalies
collector](https://user-images.githubusercontent.com/1153921/103576933-1e451380-4e91-11eb-9cef-f3bd769a70cf.png)

The `anomalies_local.probability` chart shows the probability that the latest observed data is anomalous, based on the
trained model. The `anomalies_local.anomaly` chart visualizes 0&rarr;1 predictions based on whether the latest observed
data is anomalous based on the trained model.

In other words, the `probability` chart shows the amplitude of anomaly, whereas the `anomaly` chart provides quick
yes/no context. Together, these charts create meaningful visualizations for immediately recognizing not only that
something is going wrong on your node, but give context as to where to look next.

## Monitor anomalies with alarms

The anomalies collector creates two "classes" of alarms for each chart captured by the `charts_regex` setting, which can
be [viewed in Netdata Cloud or the local Agent dashboard](/docs/monitor/view-active-alarms.md), or sent to a [supported
notification endpoint](/docs/monitor/enable-notifications.md). All these alarms are preconfigured based on your
[configuration in `anomalies.conf`](/docs/guides/monitor/anomaly-detection.md#configure-the-anomalies-collector).

With the `charts_regex` and `charts_to_exclude` settings from [part 1](/docs/guides/monitor/anomaly-detection.md) of
this guide series, the Netdata Agent creates 32 alarms driven by unsupervised anomaly detection.

The first class triggers warning alarms when the average anomaly probability has stayed above 50% for at least the last
two minutes.

![An example anomaly probability
alarm](https://user-images.githubusercontent.com/1153921/103571889-ec2fb380-4e88-11eb-9321-bafb01e1acee.png)

The second class triggers warning alarms when the number of anomalies in the last two minutes hits 10 or higher.

![An example anomaly count
alarm](https://user-images.githubusercontent.com/1153921/103571893-ee920d80-4e88-11eb-9ddd-512b727c74ce.png)

If you see either of these alarms in Netdata Cloud, the local Agent dashboard, or on your preferred notification
platform, it's a safe bet that the nodes current metrics have deviated from normal. That doesn't necessarily mean
there's a full-blown incident, depending on what application/service you're using anomaly detection on, but it's worth
further investigation.

As you use the anomalies collector, you may find that the default settings provide too many or two few genuine alarms.
In this case, [configure the alarm](/docs/monitor/configure-alarms.md) with `sudo ./edit-config
health.d/anomalies.conf`. Take a look at the `lookup` line syntax in the [health
reference](/health/REFERENCE.md#alarm-line-lookup) to understand how the anomalies collector automatically creates
alarms for any dimension on the `anomalies_local.probability` and `anomalies_local.anomaly` charts.

## Build an anomaly detection dashboard

[Netdata Cloud](https://app.netdata.cloud) features a drag-and-drop [dashboard
editor](/docs/visualize/create-dashboards.md) that helps you create entirely new dashboards with charts target for your
specific applications.

For example, here's an dashboard designed for visualizing anomalies present in an Nginx web server.

, including documentation about why the dashboard exists and where to look next
based on what you're seeing:

![An example anomaly detection
dashboard](https://user-images.githubusercontent.com/1153921/103581955-2c4b6200-4e9a-11eb-93bf-cd1273983d87.png)



## Test anomaly detection

Time to see the Netdata Agent's anomaly detection in action. To trigger anomalies on the Nginx web server, use `ab`,
otherwise known as [Apache Bench](https://httpd.apache.org/docs/2.4/programs/ab.html). Despite its name, it works just
as well with Nginx web servers. Install it on Ubuntu/Debian systems with `sudo apt install apache2-utils`.

The following test creates 100,000 requests for Nginx to handle, with a maximum of 20 at any given time. Run it a few
times to see a longer-running anomaly.

```bash
ab -n 100000 -c 20 http://127.0.0.1/
```

Unsurprisingly, the anomalies collector triggers a handful of warnings related to Nginx, such as `nginx_local.requests`.

![An active warning alarm from the anomalies
collector](https://user-images.githubusercontent.com/1153921/103583498-06738c80-4e9d-11eb-8d29-964af78263c2.png)

**Visit the custom dashboard**

**View the charts**

## What's next?



### Related reference documentation

- [Netdata Agent · Anomalies collector](/collectors/python.d.plugin/anomalies/README.md)

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fguides%2Fmonitor%2Fanomaly-detectionl&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
