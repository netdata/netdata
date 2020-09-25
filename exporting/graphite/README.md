<!--
title: "Export metrics to Graphite providers"
sidebar_label: Graphite
description: "Archive your Agent's metrics to a any Graphite database provider for long-term storage, further analysis, or correlation with data from other sources."
custom_edit_url: https://github.com/netdata/netdata/edit/master/exporting/graphite/README.md
-->

# Export metrics to Graphite providers

You can use the Graphite connector for the [exporting engine](/exporting/README.md) to archive your agent's metrics to
Graphite providers for long-term storage, further analysis, or correlation with data from other sources.

## Configuration

To enable data exporting to a Graphite database, run `./edit-config exporting.conf` in the Netdata configuration
directory and set the following options:

```conf
[graphite:my_graphite_instance]
    enabled = yes
    destination = localhost:2003
```

The Graphite connector is further configurable using additional settings. See the [exporting reference
doc](/exporting/README.md#options) for details.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fexporting%2Fjson%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
