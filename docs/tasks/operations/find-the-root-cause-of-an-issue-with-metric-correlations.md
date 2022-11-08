<!--
title: "Find the root cause of an issue with Metric correlations"
sidebar_label: "Find the root cause of an issue with Metric correlations"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/tasks/operations/find-the-root-cause-of-an-issue-with-metric-correlations.md"
learn_status: "Published"
learn_topic_type: "Tasks"
sidebar_position: "7"
learn_rel_path: "Operations"
learn_docs_purpose: "Instructions on how to use metric correlations to find correlated charts "
learn_repo_doc: "True"
-->

The Metric Correlations (MC) feature lets you quickly find metrics and charts related to a particular window of interest
that you want to explore further.

Because Metric Correlations uses every available metric from a node, with as high as 1-second granularity, you get
the most accurate insights using every possible metric.

In this Task you will learn how to run a Metric Correlation from the Netdata Cloud interface.

## Prerequisites

- A Netdata Cloud account with at least one node claimed to one of its Spaces

## Steps

From within a War Room:

1. Click on the **Nodes** view
2. Click on any live node
3. Click on the **Metric Correlation** button in the top right corner
4. Go to the "suspicious" chart you want to analyze
5. From its toolbox, select the **Highlight** button
6. Click and drag, selecting at least a 15-second timeframe
7. Click the **Find Correlations** button in the top right corner

You then will be presented with the charts that are most correlated with the behavior that you highlighted.

The top bar will then change and present you with a slider to **show less** or **show more** metrics, while also being
able to change the **correlation method**, the **aggregation method** and the **data to correlate**.

## Related topics

1. [Guided Troubleshooting Concept](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-architecture/guided-troubleshooting.md)
2. [Metric Correlation Concept](https://github.com/netdata/netdata/blob/master/docs/concepts/guided-troubleshooting/metric-correlations.md)
