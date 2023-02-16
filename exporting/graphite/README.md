<!--
title: "Export metrics to Graphite providers"
description: "Archive your Agent's metrics to a any Graphite database provider for long-term storage, further analysis, or correlation with data from other sources."
custom_edit_url: "https://github.com/netdata/netdata/edit/master/exporting/graphite/README.md"
sidebar_label: "Graphite"
learn_status: "Published"
learn_rel_path: "Integrations/Export"
-->

# Export metrics to Graphite providers

You can use the Graphite connector for
the [exporting engine](https://github.com/netdata/netdata/blob/master/exporting/README.md) to archive your agent's
metrics to Graphite providers for long-term storage, further analysis, or correlation with data from other sources.

## Configuration

To enable data exporting to a Graphite database, run `./edit-config exporting.conf` in the Netdata configuration
directory and set the following options:

```conf
[graphite:my_graphite_instance]
    enabled = yes
    destination = localhost:2003
```

Add `:http` or `:https` modifiers to the connector type if you need to use other than a plaintext protocol. For
example: `graphite:http:my_graphite_instance`,
`graphite:https:my_graphite_instance`. You can set basic HTTP authentication credentials using

```conf
    username = my_username
    password = my_password
```

The Graphite connector is further configurable using additional settings. See
the [exporting reference doc](https://github.com/netdata/netdata/blob/master/exporting/README.md#options) for details.


