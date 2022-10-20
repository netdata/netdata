<!--
title: "Guided troubleshooting"
sidebar_label: "Guided troubleshooting"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-architecture/guided-troubleshooting.md"
sidebar_position: 16
learn_status: "Published"
learn_topic_type: "Concepts"
learn_rel_path: "netdata-architecture"
learn_docs_purpose: "Present what tools Netdata utilize to help you troubleshoot your infrastructure"
-->

Netdata provides you with a high fidelity monitoring solution to observe your systems. With thousands of metrics, even
though neatly arranged into charts and dashboards, sometimes even masters of their craft can become overwhelmed. In this
section we will explain the methods and components Netdata provides, to guide you troubleshoot your infrastructure's
issues. These components are:

1. The [Anomaly Advisor](#anomaly-advisor)
2. The [Metric Correlation](#metric-corellation) component

The ML driving our Anomaly Adviser and Metric Correlations feature works at the edge to learn what anomalies look like, so it can identify and alert you to them immediately. And the algorithm behind our Cloud-based Metric Correlations tool can take you straight to the potential root cause of your problem in seconds.

## Anomaly Advisor

## Metric correlation

The Metric Correlation (MC) component search and identifies correlations between metrics/charts on a particular window
of interest (timeline) that you specify. By displaying the standard Netdata dashboard, filtered to show only charts that
are relevant to the window of interest, you can get to the root cause sooner.

Because Metric Correlations uses every available metric from that node, with as high as 1-second granularity, you get
extraordinary insights for the status of your system.

### Metric Correlation under the hood

When you specify a timelapse and run a Metric Correlation Job, Netdata Agent will:

1. Focus on the all the data **or** the Anomaly bits stored for this particular timeline
2. Perform a pre-processing; aggregating the raw data, such that, arbitrary window lengths can be selected for MC (by
   default it averages them).
3. Run a Metric correlation strategy/algorithm on those data.
4. Present you the top N correlated metrics and their charts.

#### Notable aspect of the metric correlation component

##### Input data

Netdata is different than typical observability agents since, in addition to just collecting raw metric values, it will
by default also assign an "Anomaly Bit" related to each collected metric each second. This bit will be 0 for "normal"
and 1 for "anomalous". This means that each metric also natively has an "Anomaly Rate" associated with it and, as such,
MC can be run against the raw metric values or their corresponding anomaly rates.

Metrics - Run MC on the raw metric values. 
Anomaly Rate - Run MC on the corresponding anomaly rate for each metric.

##### Data Aggregation methods

By default, Netdata will just `Average` raw data when needed as part of pre-processing. You can pick and choose more aggregation methods like `Median`, `Min`, `Max` and `Stddev`.

##### Algorithms

There are two algorithms available that aim to score metrics based on how much they have changed between the baseline
and highlight windows.

1. `KS2` - A statistical test (Two-sample Kolmogorov Smirnov) comparing the distribution of the highlighted window to
   the baseline to try and quantify which metrics have most evidence of a significant change. You can explore our
   implementation [in the Agent's source code](https://github.com/netdata/netdata/blob/d917f9831c0a1638ef4a56580f321eb6c9a88037/database/metric_correlations.c#L212)
   .
2. `Volume` - An heuristic measure based on the percentage change in averages between highlighted window and baseline,
   with various edge cases sensibly controlled for. You can explore our
   implementation [in the Agent's source code](https://github.com/netdata/netdata/blob/d917f9831c0a1638ef4a56580f321eb6c9a88037/database/metric_correlations.c#L516)
   .

## Summary

You can think of these two methods as an extra pair of eyes, eyes that understand your system and help you find the root
cause of an issue

## Related topics
