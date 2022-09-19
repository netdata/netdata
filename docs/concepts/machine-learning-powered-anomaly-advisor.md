<!--
title: "Anomaly Advisor"
sidebar_label: "Anomaly Advisor"
custom_edit_url: "https://github.com/netdata/learn/blob/master/docs/concepts/machine-learning/anomaly-advisor.md"
learn_status: "Published"
learn_topic_type: "Concepts"
learn_rel_path: "netdata-hub"
learn_docs_purpose: "Present the concept of Netdata's Anomaly Advisor feature, it's purpose and use cases"
learn_repo_doc: "True"
-->

**********************************************************************

Netdata's Anomaly Advisor feature lets you quickly surface potentially anomalous metrics and charts related to a particular highlight window of interest. 
If you are running a Netdata version higher than v1.35.0-29-nightly, you will be able to use the Anomaly Advisor out of the box with zero configuration. If you 
are on an earlier Netdata version, you will need to first enable ML on your nodes by following the steps below.

To enable the Anomaly Advisor you must first enable ML on your nodes via a small config change in netdata.conf. Once the anomaly detection models have trained 
on the Agent (with default settings this takes a couple of hours until enough data has been seen to train the models) you will then be able to enable the Anomaly 
Advisor feature in Netdata Cloud.
 
## How Does Anomaly Advisor Work

You can find the Anomaly Advisor in the the **Anomalies** tab of the Netdata Cloud dashboard. Within this tab, you can highlight a particular timeframe 
of interest and a selection of the most anomalous dimensions will appear below. 

This will surface the most anomalous metrics in the space or room for the highlighted window to try and cut down on the amount of manual searching required 
to get to the root cause of your issues.

The **Anomaly Rate** chart shows the percentage of anomalous metrics over time per node. 

For example, in the following image, 3.21% of the metrics on the "ml-demo-ml-disabled" node were considered anomalous. This elevated anomaly rate could be a 
sign of something worth investigating.

**Note**: in this example the anomaly rates for this node are actually being calculated on the parent it streams to, you can run ml on the Agent itselt or 
on a parent the Agent stream to. Read more about the various configuration options in the [Agent docs](https://github.com/netdata/netdata/blob/master/ml/README.md).

![image](https://user-images.githubusercontent.com/2178292/164428307-6a86989a-611d-47f8-a673-911d509cd954.png)

The **Count of Anomalous Metrics** chart (collapsed by default) shows raw counts of anomalous metrics per node so may often be similar to the anomaly rate chart, 
apart from where nodes may have different numbers of metrics.

The "Anomaly Events Detected" chart (collapsed by default) shows if the anomaly rate per node was sufficiently elevated to trigger a node level anomaly. Anomaly events will appear slightly after the anomaly rate starts to increase in the timeline, this is because a significant number of metrics in the node need to be anomalous before an anomaly event is triggered.

Once you have highlighted a window of interest, you should see an ordered list of anomaly rate sparklines in the **Anomalous metrics**.

You can expand any sparkline chart to see the underlying raw data to see how it relates to the corresponding anomaly rate.

On the upper right hand side of the page, you can select which nodes to filter on if you wish to do so. The ML training status of each node is also displayed. 

On the lower right hand side of the page, an index of anomaly rates is displayed for the highlighted timeline of interest. The index is sorted from most anomalous 
metric (highest anomaly rate) to least (lowest anomaly rate). Clicking on an entry in the index will scroll the rest of the page to the corresponding anomaly rate 
sparkline for that metric.

### Usage Tips

- If you are interested in a subset of specific nodes then filtering to just those nodes before highlighting tends to give better results. This is because when 
you highlight a region, Netdata Cloud will ask the Agents for a ranking over all metrics so if you can filter this early to just the subset of nodes you are 
interested in, less 'averaging' will occur and so you might be a less noisy ranking.
- Ideally try and highlight close to a spike or window of interest so that the resulting ranking can narrow in more easily on the timeline you are interested in.

You can read more detail on how anomaly detection in the Netdata Agent works in our [Agent docs](https://github.com/netdata/netdata/blob/master/ml/README.md).

🚧 **Note**: This functionality is still **under active development** and considered experimental. We dogfood it internally and among early adopters within the 
Netdata community to build the feature. If you would like to get involved and help us with feedback, you can reach us through any of the following channels:
- Email us at analytics-ml-team@netdata.cloud
- Comment on the [beta launch post](https://community.netdata.cloud/t/anomaly-advisor-beta-launch/2717) in the Netdata community
- Join us in the [🤖-ml-powered-monitoring](https://discord.gg/4eRSEUpJnc) channel of the Netdata discord.
- Or open a discussion in GitHub if that's more your thing

## Related Topics
