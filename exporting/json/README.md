<!--
title: "Export metrics to JSON document databases"
description: "Archive your Agent's metrics to a JSON document database for long-term storage, further analysis, or correlation with data from other sources."
custom_edit_url: https://github.com/netdata/netdata/edit/master/exporting/json/README.md
sidebar_label: JSON Document Databases
-->

# Export metrics to JSON document databases

You can use the JSON connector for the [exporting engine](/exporting/README.md) to archive your agent's metrics to JSON
document databases for long-term storage, further analysis, or correlation with data from other sources.

## Configuration

To enable data exporting to a JSON document database, run `./edit-config exporting.conf` in the Netdata configuration
directory and set the following options:

```conf
[json:my_json_instance]
    enabled = yes
    destination = localhost:5448
```

Add `:http` or `:https` modifiers to the connector type if you need to use other than a plaintext protocol. For example: `json:http:my_json_instance`,
`json:https:my_json_instance`. You can set basic HTTP authentication credentials using

```conf
    username = my_username
    password = my_password
```

The JSON connector is further configurable using additional settings. See the [exporting reference
doc](/exporting/README.md#options) for details.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fexporting%2Fjson%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
