<!--
title: "Export metrics to OpenTSDB"
description: "Archive your Agent's metrics to an OpenTSDB database for long-term storage and further analysis."
custom_edit_url: https://github.com/netdata/netdata/edit/master/exporting/opentsdb/README.md
sidebar_label: OpenTSDB
-->

# Export metrics to OpenTSDB

You can use the OpenTSDB connector for the [exporting engine](/exporting/README.md) to archive your agent's metrics to OpenTSDB
databases for long-term storage, further analysis, or correlation with data from other sources.

## Configuration

To enable data exporting to an OpenTSDB database, run `./edit-config exporting.conf` in the Netdata configuration
directory and set the following options:

```conf
[opentsdb:my_opentsdb_instance]
    enabled = yes
    destination = localhost:4242
```

Add `:http` or `:https` modifiers to the connector type if you need to use other than a plaintext protocol. For example: `opentsdb:http:my_opentsdb_instance`,
`opentsdb:https:my_opentsdb_instance`.

The OpenTSDB connector is further configurable using additional settings. See the [exporting reference
doc](/exporting/README.md#options) for details.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fexporting%2Fopentsdb%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
