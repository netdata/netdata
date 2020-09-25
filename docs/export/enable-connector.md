<!--
title: "Enable an exporting connector"
description: "Learn how to enable and configure any connector using examples to start exporting metrics to external time-series databases in minutes."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/export/enable-connector.md
-->

# Enable an exporting connector

Now that you found the right connector for your [external time-series
database](/docs/export/external-databases.md#supported-databases), you can now enable the exporting engine and the
connector itself. We'll walk through the process of enabling the exporting engine itself, followed by two examples using
the OpenTSDB and Graphite connectors.

Once you understand the process of enabling a connector, you can translate that knowledge to any other connector.

## Enable the exporting engine

Use `edit-config` from your [Netdata config directory](/docs/configure/nodes.md#the-netdata-config-directory) to open
`exporting.conf`:

```bash
sudo ./edit-config exporting.conf
```

Enable the exporting engine itself by setting `enabled` to `yes`:

```conf
[exporting:global]
    enabled = yes
```

Save the file but keep it open, as you will edit it again to enable specific connectors.

## Example: Enable the OpenTSDB connector

Use the following configuration as a starting point. Copy and paste it into `exporting.conf`.

```conf
[opentsdb:http:my_opentsdb_http_instance]
    enabled = yes
    destination = localhost:4242
```

Replace `my_opentsdb_http_instance` with an instance name of your choice, and change the `destination` setting to the IP
address or hostname of your OpenTSDB database.

Restart your Agent with `service netdata restart` to begin exporting to your OpenTSDB database. Because the
Agent exports metrics as they're collected, you should start seeing data in your external database after only a few
seconds.

Any further configuration is optional, based on your needs and the configuration of your OpenTSDB database. See the
[OpenTSDB connector doc](/exporting/opentsdb/README.md) and [exporting engine
reference](/exporting/README.md#configuration) for details.

## Example: Enable the Graphite connector

Use the following configuration as a starting point. Copy and paste it into `exporting.conf`.

```conf
[graphite:my_graphite_instance]
    enabled = yes
    destination = 203.0.113.0:2003
```

Replace `my_graphite_instance` with an instance name of your choice, and change the `destination` setting to the IP
address or hostname of your Graphite-supported database.

Restart your Agent with `service netdata restart` to begin exporting to your Graphite-supported database. Because the
Agent exports metrics as they're collected, you should start seeing data in your external database after only a few
seconds.

Any further configuration is optional, based on your needs and the configuration of your Graphite-supported database.
See [exporting engine reference](/exporting/README.md#configuration) for details.

## What's next?

If you want to further configure your exporting connectors, see the [exporting engine
reference](/exporting/README.md#configuration).

For a comprehensive example of using the Graphite connector, read our guide: [_Export and visualize Netdata metrics in
Graphite_](/docs/guides/export/export-netdata-metrics-graphite.md). Or, start [using host
labels](/docs/guides/using-host-labels.md) on exported metrics.

### Related reference documentation

-   [Exporting engine reference](/exporting/README.md)
-   [OpenTSDB connector](/exporting/opentsdb/README.md)
-   [Graphite connector](/exporting/graphite/README.md)

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fexporting%2Fenable-connector&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
