<!--
title: "Export metrics to MongoDB"
description: "Archive your Agent's metrics to a MongoDB database for long-term storage, further analysis, or correlation with data from other sources."
custom_edit_url: https://github.com/netdata/netdata/edit/master/exporting/mongodb/README.md
sidebar_label: MongoDB
-->

# Export metrics to MongoDB

You can use the MongoDB connector for the [exporting engine](/exporting/README.md) to archive your agent's metrics to a
MongoDB database for long-term storage, further analysis, or correlation with data from other sources.

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

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fexporting%2Fmongodb%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
