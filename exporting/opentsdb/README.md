<!--
title: "Export metrics to OpenTSDB"
description: "Archive your Agent's metrics to an OpenTSDB database for long-term storage and further analysis."
custom_edit_url: "https://github.com/netdata/netdata/edit/master/exporting/opentsdb/README.md"
sidebar_label: "OpenTSDB"
learn_status: "Published"
learn_rel_path: "Integrations/Export"
-->

# Export metrics to OpenTSDB

You can use the OpenTSDB connector for
the [exporting engine](https://github.com/netdata/netdata/blob/master/exporting/README.md) to archive your agent's
metrics to OpenTSDB databases for long-term storage, further analysis, or correlation with data from other sources.

## Configuration

To enable data exporting to an OpenTSDB database, run `./edit-config exporting.conf` in the Netdata configuration
directory and set the following options:

```conf
[opentsdb:my_opentsdb_instance]
    enabled = yes
    destination = localhost:4242
```

Add `:http` or `:https` modifiers to the connector type if you need to use other than a plaintext protocol. For
example: `opentsdb:http:my_opentsdb_instance`,
`opentsdb:https:my_opentsdb_instance`. You can set basic HTTP authentication credentials using

```conf
    username = my_username
    password = my_password
```

The OpenTSDB connector is further configurable using additional settings. See
the [exporting reference doc](https://github.com/netdata/netdata/blob/master/exporting/README.md#options) for details.


