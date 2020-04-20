<!--
---
title: "Export metrics to Google Cloud Pub/Sub Service"
custom_edit_url: https://github.com/netdata/netdata/edit/master/exporting/pubsub/README.md
---
-->

# Export metrics to Google Cloud Pub/Sub Service

## Prerequisites

To use Pub/Sub for metric collecting and processing, you should first
[install](https://github.com/googleapis/cpp-cmakefiles) Google Cloud Platform C++ Proto Libraries.
`protobuf`, and `grpc` are also required to compile Netdata with Pub/Sub support enabled. Next, Netdata
should be re-installed from the source. The installer will detect that the required libraries are now available.

## Configuration

To enable data sending to the Pub/Sub service, run `./edit-config exporting.conf` in the Netdata configuration directory
and set the following options:

```conf
[pubsub:my_instance]
    enabled = yes
```

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fexporting%2Fpubsub%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
