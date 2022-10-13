<!--
title: "Configure retention"
sidebar_label: "Configure retention"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/tasks/manage-retained-metrics/configure-retention.md"
learn_status: "Published"
sidebar_position: 4
learn_topic_type: "Tasks"
learn_rel_path: "manage-retained-metrics"
learn_docs_purpose: "Instructions on how to use the calculator to find the values that meet the user’s needs along with instructions on configuring Agent Retention"
-->

The Agent uses a custom-made time-series database (TSDB), named
the [`dbengine`](https://github.com/netdata/netdata/blob/master/database/engine/README.md), to store metrics.

The default settings retain approximately two day's worth of metrics on a system collecting 2,000 metrics every second,
but the Agent is highly configurable if you want your nodes to store days, weeks, or months worth of per-second
data.

## Prerequisites

- A node with the Agent installed, and terminal access to that node

## Steps

### Calculate the system resources (RAM, disk space) needed to store metrics

You can store more or less metrics using the database engine by changing the allocated disk space. Use the calculator
below to find the appropriate value for the `dbengine` based on how many metrics your node(s) collect, whether you are
streaming metrics to a parent node, and more.

You do not need to edit the `dbengine page cache size` setting to store more metrics using the database engine. However,
if you want to store more metrics _specifically in memory_, you can increase the cache size.

:::note

This calculator provides an estimation of disk and RAM usage for **metrics usage**. Real-life usage may vary based on
the accuracy of the values you enter below, changes in the compression ratio, and the types of metrics stored.

:::

Download
the [calculator](https://docs.google.com/spreadsheets/d/e/2PACX-1vTYMhUU90aOnIQ7qF6iIk6tXps57wmY9lxS6qDXznNJrzCKMDzxU3zkgh8Uv0xj_XqwFl3U6aHDZ6ag/pub?output=xlsx)
to optimize the data retention to your preferences. Utilize the "Front" spreadsheet. Experiment with the variables which
are padded with yellow to come up with the best settings for your use case.

### Edit `netdata.conf` with recommended database engine settings

Now that you have a recommended setting for your Agent's `dbengine`, edit
the [Netdata configuration file](https://github.com/netdata/netdata/blob/master/docs/tasks/general-configuration/configure-the-agent.md)
and look for the `[db]` subsection. Change it to the recommended values you calculated from the calculator. For example:

```conf
[db]
   mode = dbengine
   storage tiers = 3
   update every = 1
   dbengine multihost disk space MB = 1024
   dbengine page cache size MB = 32
   dbengine tier 1 update every iterations = 60
   dbengine tier 1 multihost disk space MB = 384
   dbengine tier 1 page cache size MB = 32
   dbengine tier 2 update every iterations = 60
   dbengine tier 2 multihost disk space MB = 16
   dbengine tier 2 page cache size MB = 32
```

Save the file
and [restart the Agent](https://github.com/netdata/netdata/blob/master/docs/tasks/general-configuration/start-stop-and-restart-agent.md)
, to change the database engine's size.

## Related topics

1. [dbengine Reference](https://github.com/netdata/netdata/blob/master/database/engine/README.md)