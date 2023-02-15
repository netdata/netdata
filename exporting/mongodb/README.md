<!--
title: "Export metrics to MongoDB"
description: "Archive your Agent's metrics to a MongoDB database for long-term storage, further analysis, or correlation with data from other sources."
custom_edit_url: "https://github.com/netdata/netdata/edit/master/exporting/mongodb/README.md"
sidebar_label: "MongoDB"
learn_status: "Published"
learn_rel_path: "Integrations/Export"
-->

# Export metrics to MongoDB

You can use the MongoDB connector for
the [exporting engine](https://github.com/netdata/netdata/blob/master/exporting/README.md) to archive your agent's
metrics to a MongoDB database for long-term storage, further analysis, or correlation with data from other sources.

## Prerequisites

To use MongoDB as an external storage for long-term archiving, you should first
[install](http://mongoc.org/libmongoc/current/installing.html) `libmongoc` 1.7.0 or higher. Next, re-install Netdata
from the source, which detects that the required library is now available.

## Configuration

To enable data exporting to a MongoDB database, run `./edit-config exporting.conf` in the Netdata configuration
directory and set the following options:

```conf
[mongodb:my_instance]
    enabled = yes
    destination = mongodb://<hostname>
    database = your_database_name
    collection = your_collection_name
```

You can find more information about the `destination` string URI format in the MongoDB
[documentation](https://docs.mongodb.com/manual/reference/connection-string/)

The default socket timeout depends on the exporting connector update interval. The timeout is 500 ms shorter than the
interval (but not less than 1000 ms). You can alter the timeout using the `sockettimeoutms` MongoDB URI option.


