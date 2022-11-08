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

In the following task sections, you will learn how to run a Metric Correlation from the Netdata Cloud interface.

## Prerequisites

To find the root cause of an issue with Metric Correlations, you will need the following:

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
8. You will be presented with the charts who metric behavior is most correlated with the behavior that you highlighted.
9. Use the top bar to investigate the correlations with the following button functions:
- **show less**: Use this button to show less metric data.   
- **show more**: Use this button to show more metric data.
- **correlation method**: Use this to select a correlation method.
- **aggregation method**: Use this to select an aggregation method.
- **data to correlate**: Use this to select which data to correlate.

## Related topics

### Related Concepts

- [Guided Troubleshooting Concept](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-architecture/guided-troubleshooting.md)
- [Metric Correlation Concept](https://github.com/netdata/netdata/blob/master/docs/concepts/guided-troubleshooting/metric-correlations.md)
