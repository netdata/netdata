<!--
title: "Export metrics to Google Cloud Pub/Sub Service"
description: "Export Netdata metrics to the Google Cloud Pub/Sub Service for long-term archiving or analytical processing."
custom_edit_url: "https://github.com/netdata/netdata/edit/master/exporting/pubsub/README.md"
sidebar_label: "Google Cloud Pub/Sub Service"
learn_status: "Published"
learn_rel_path: "Integrations/Export"
-->

# Export metrics to Google Cloud Pub/Sub Service

## Prerequisites

To use the Pub/Sub service for metric collecting and processing, you should first
[install](https://github.com/googleapis/google-cloud-cpp/) Google Cloud Platform C++ Client Libraries.
Pub/Sub support is also dependent on the dependencies of those libraries, like `protobuf`, `protoc`, and `grpc`. Next,
Netdata should be re-installed from the source. The installer will detect that the required libraries are now available.

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


