<!--
title: "Machine learning (ML) powered anomaly detection"
sidebar_label: "Machine learning (ML) powered anomaly detection"
description: "Detect anomalies in any system, container, or application in your infrastructure with machine learning and the open-source Netdata Agent."
image: /img/seo/guides/monitor/anomaly-detection.png
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/guides/monitor/anomaly-detection.md
learn_status: "Published"
learn_rel_path: "Operations"
-->

# Machine learning (ML) powered anomaly detection


## Overview

As of [`v1.32.0`](https://github.com/netdata/netdata/releases/tag/v1.32.0), Netdata comes with some ML powered [anomaly detection](https://en.wikipedia.org/wiki/Anomaly_detection) capabilities built into it and available to use out of the box, with zero configuration required (ML was enabled by default in `v1.35.0-29-nightly` in [this PR](https://github.com/netdata/netdata/pull/13158), previously it required a one line config change).

This means that in addition to collecting raw value metrics, the Netdata agent will also produce an [`anomaly-bit`](https://github.com/netdata/netdata/blob/master/ml/README.md#anomaly-bit---100--anomalous-0--normal) every second which will be `100` when recent raw metric values are considered anomalous by Netdata and `0` when they look normal. Once we aggregate beyond one second intervals this aggregated `anomaly-bit` becomes an ["anomaly rate"](https://github.com/netdata/netdata/blob/master/ml/README.md#anomaly-rate---averageanomaly-bit).

To be as concrete as possible, the below api call shows how to access the raw anomaly bit of the `system.cpu` chart from the [london.my-netdata.io](https://london.my-netdata.io) Netdata demo server. Passing `options=anomaly-bit` returns the anomaly bit instead of the raw metric value.

```
https://london.my-netdata.io/api/v1/data?chart=system.cpu&options=anomaly-bit
```

If we aggregate the above to just 1 point by adding `points=1` we get an "[Anomaly Rate](https://github.com/netdata/netdata/blob/master/ml/README.md#anomaly-rate---averageanomaly-bit)":

```
https://london.my-netdata.io/api/v1/data?chart=system.cpu&options=anomaly-bit&points=1
```

The fundamentals of Netdata's anomaly detection approach and implementation are covered in lots more detail in the [agent ML documentation](https://github.com/netdata/netdata/blob/master/ml/README.md). 

This guide will explain how to get started using these ML based anomaly detection capabilities within Netdata.

## Anomaly Advisor

The [Anomaly Advisor](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/anomaly-advisor.md) is the flagship anomaly detection feature within Netdata. In the "Anomalies" tab of Netdata you will see an overall "Anomaly Rate" chart that aggregates node level anomaly rate for all nodes in a space. The aim of this chart is to make it easy to quickly spot periods of time where the overall "[node anomaly rate](https://github.com/netdata/netdata/blob/master/ml/README.md#node-anomaly-rate)" is elevated in some unusual way and for what node or nodes this relates to.

![image](https://user-images.githubusercontent.com/2178292/175928290-490dd8b9-9c55-4724-927e-e145cb1cc837.png)

Once an area on the Anomaly Rate chart is highlighted netdata will append a "heatmap" to the bottom of the screen that shows which metrics were more anomalous in the highlighted timeframe. Each row in the heatmap consists of an anomaly rate sparkline graph that can be expanded to reveal the raw underlying metric chart for that dimension.

![image](https://user-images.githubusercontent.com/2178292/175929162-02c8fe69-cc4f-4cf4-9b3a-a5e559a6feca.png)

## Embedded Anomaly Rate Charts

Charts in both the [Overview](https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/overview.md) and [single node dashboard](https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/overview.md#jump-to-single-node-dashboards) tabs also expose the underlying anomaly rates for each dimension so users can easily see if the raw metrics are considered anomalous or not by Netdata.

Pressing the anomalies icon (next to the information icon in the chart header) will expand the anomaly rate chart to make it easy to see how the anomaly rate for any individual dimension corresponds to the raw underlying data. In the example below we can see that the spike in `system.pgpgio|in` corresponded in the anomaly rate for that dimension jumping to 100% for a small period of time until the spike passed.

![image](https://user-images.githubusercontent.com/2178292/175933078-5dd951ff-7709-4bb9-b4be-34199afb3945.png)

## Anomaly Rate Based Alerts

It is possible to use the `anomaly-bit` when defining traditional Alerts within netdata. The `anomaly-bit` is just another `options` parameter that can be passed as part of an [alarm line lookup](https://github.com/netdata/netdata/blob/master/docs/configure/start-stop-restart.md#alarm-line-lookup). 

You can see some example ML based alert configurations below:

- [Anomaly rate based CPU dimensions alarm](https://github.com/netdata/netdata/blob/master/docs/configure/start-stop-restart.md#example-8---anomaly-rate-based-cpu-dimensions-alarm)
- [Anomaly rate based CPU chart alarm](https://github.com/netdata/netdata/blob/master/docs/configure/start-stop-restart.md#example-9---anomaly-rate-based-cpu-chart-alarm)
- [Anomaly rate based node level alarm](https://github.com/netdata/netdata/blob/master/docs/configure/start-stop-restart.md#example-10---anomaly-rate-based-node-level-alarm)
- More examples in the [`/health/health.d/ml.conf`](https://github.com/netdata/netdata/blob/master/health/health.d/ml.conf) file that ships with the agent.

## Learn More

Check out the resources below to learn more about how Netdata is approaching ML:

- [Agent ML documentation](https://github.com/netdata/netdata/blob/master/ml/README.md).
- [Anomaly Advisor documentation](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/anomaly-advisor.md).
- [Metric Correlations documentation](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/metric-correlations.md).
- Anomaly Advisor [launch blog post](https://www.netdata.cloud/blog/introducing-anomaly-advisor-unsupervised-anomaly-detection-in-netdata/).
- Netdata Approach to ML [blog post](https://www.netdata.cloud/blog/our-approach-to-machine-learning/).
- `areal/ml` related [GitHub Discussions](https://github.com/netdata/netdata/discussions?discussions_q=label%3Aarea%2Fml).
- Netdata Machine Learning Meetup [deck](https://docs.google.com/presentation/d/1rfSxktg2av2k-eMwMbjN0tXeo76KC33iBaxerYinovs/edit?usp=sharing) and [YouTube recording](https://www.youtube.com/watch?v=eJGWZHVQdNU).
- Netdata Anomaly Advisor [YouTube Playlist](https://youtube.com/playlist?list=PL-P-gAHfL2KPeUcCKmNHXC-LX-FfdO43j). 
