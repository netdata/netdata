<!--
title: "Export metrics to JSON document databases"
description: "Archive your Agent's metrics to a JSON document database for long-term storage, further analysis, or correlation with data from other sources."
custom_edit_url: "https://github.com/netdata/netdata/edit/master/exporting/json/README.md"
sidebar_label: "JSON Document Databases"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Integrations/Export"
-->

# Export metrics to JSON document databases

You can use the JSON connector for the [exporting engine](https://github.com/netdata/netdata/blob/master/exporting/README.md) to archive your agent's metrics to JSON
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

The JSON connector is further configurable using additional settings. See 
the [exporting reference doc](https://github.com/netdata/netdata/blob/master/exporting/README.md#options) for details.


