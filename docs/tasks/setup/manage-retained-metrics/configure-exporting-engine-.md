<!--
title: "Configure exporting engine "
sidebar_label: "Configure exporting engine "
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/tasks/manage-retained-metrics/configure-exporting-engine-.md"
learn_status: "Published"
sidebar_position: 10
learn_topic_type: "Tasks"
learn_rel_path: "manage-retained-metrics"
learn_docs_purpose: "Instructions on how to configure the exporting engine to export metrics to an external target"
-->


In this task, you will learn how to enable the exporting engine, and the exporting connector, followed by two examples
using the OpenTSDB and Graphite connectors.

:::note
When you enable the exporting engine and a connector, the Netdata Agent exports metrics _beginning from the time you
restart its process_, not the entire database of long-term metrics.
:::

Once you understand the process of enabling a connector, you can translate that knowledge to any other connector.

## Prerequisites

You need to find the right connector for your [external time-series
database](MISSING LINK FOR METRIC EXPORTING REFERENCES), and then you can proceed on with the task.

## Enable the exporting engine

1. Edit the `exporting.conf` configuration file,
   by [editing the Netdata configuration](https://github.com/netdata/netdata/blob/master/docs/tasks/general-configuration/configure-the-agent.md)
   .
2. Enable the exporting engine itself by setting `enabled` to `yes`:

    ```conf
    [exporting:global]
        enabled = yes
    ```

3. Save the file but keep it open, as you will edit it again to enable specific connectors.

### Example: Enable the OpenTSDB connector

Use the following configuration as a starting point. Copy and paste it into `exporting.conf`.

```conf
[opentsdb:http:my_opentsdb_http_instance]
    enabled = yes
    destination = localhost:4242
```

1. Replace `my_opentsdb_http_instance` with an instance name of your choice, and change the `destination` setting to the
   IP
   address or hostname of your OpenTSDB database.

2. [Restart your Agent](https://github.com/netdata/netdata/blob/master/docs/tasks/general-configuration/start-stop-and-restart-agent.md#restarting-the-agent)
   to begin exporting to your OpenTSDB database. The
   Netdata Agent exports metrics _beginning from the time the process starts_, and because it exports as metrics are
   collected, you should start seeing data in your external database after only a few seconds.

<!--Any further configuration is optional, based on your needs and the configuration of your OpenTSDB database. See the
[OpenTSDB connector doc](/exporting/opentsdb/README.md)-->

### Example: Enable the Graphite connector

Use the following configuration as a starting point. Copy and paste it into `exporting.conf`.

```conf
[graphite:my_graphite_instance]
    enabled = yes
    destination = 203.0.113.0:2003
```

1. Replace `my_graphite_instance` with an instance name of your choice, and change the `destination` setting to the IP
   address or hostname of your Graphite-supported database.

2. [Restart your Agent](https://github.com/netdata/netdata/blob/master/docs/tasks/general-configuration/start-stop-and-restart-agent.md#restarting-the-agent)
   to begin exporting to your Graphite-supported database.
   Because the Agent exports metrics as they're collected, you should start seeing data in your external database after
   only a few seconds.

<!--Any further configuration is optional, based on your needs and the configuration of your Graphite-supported database.
See [exporting engine reference](/exporting/README.md#configuration) for details.-->