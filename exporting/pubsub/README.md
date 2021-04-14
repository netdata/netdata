<!--
title: "Export metrics to Google Cloud Pub/Sub Service"
description: "Export Netdata metrics to the Google Cloud Pub/Sub Service for long-term archiving or analytical processing."
custom_edit_url: https://github.com/netdata/netdata/edit/master/exporting/pubsub/README.md
sidebar_label: Google Cloud Pub/Sub Service
-->

# Export metrics to Google Cloud Pub/Sub Service

## Prerequisites

To use the Pub/Sub service for metric collecting and processing, you should first
[install](https://github.com/googleapis/cpp-cmakefiles) Google Cloud Platform C++ Proto Libraries.
Pub/Sub support is also dependent on the dependencies of those libraries, like `protobuf`, `protoc`, and `grpc`. Next,
Netdata should be re-installed from the source. The installer will detect that the required libraries are now available.

> You [cannot compile Netdata](https://github.com/netdata/netdata/issues/10193) with Pub/Sub support enabled using
> `grpc` 1.32 or higher.
>
> Some distributions don't have `.cmake` files in packages. To build the C++ Proto Libraries on such distributions we
> advise you to delete `protobuf`, `protoc`, and `grpc` related packages and
> [install](https://github.com/grpc/grpc/blob/master/BUILDING.md) `grpc` with its dependencies from source.

## Configuration

To enable data sending to the Pub/Sub service, run `./edit-config exporting.conf` in the Netdata configuration directory
and set the following options:

```conf
[pubsub:my_instance]
    enabled = yes
    destination = pubsub.googleapis.com
    credentials file = /etc/netdata/google_cloud_credentials.json
    project id = my_project
    topic id = my_topic
```

Set the `destination` option to a Pub/Sub service endpoint. `pubsub.googleapis.com` is the default one.

Next, create the credentials JSON file by following Google Cloud's [authentication guide](https://cloud.google.com/docs/authentication/getting-started#creating_a_service_account). The user running the Agent
(typically `netdata`) needs read access to `google_cloud_credentials.json`, which you can set with
`chmod 400 google_cloud_credentials.json; chown netdata google_cloud_credentials.json`. Set the `credentials file`
option to the full path of the file.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fexporting%2Fpubsub%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
