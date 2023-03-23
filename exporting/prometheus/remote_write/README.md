<!--
title: "Export metrics to Prometheus remote write providers"
description: "Send Netdata metrics to your choice of more than 20 external storage providers for long-term archiving and further analysis."
custom_edit_url: "https://github.com/netdata/netdata/edit/master/exporting/prometheus/remote_write/README.md"
sidebar_label: "Prometheus remote write"
learn_status: "Published"
learn_rel_path: "Integrations/Export"
-->

# Export metrics to Prometheus remote write providers

The Prometheus remote write exporting connector uses the exporting engine to send Netdata metrics to your choice of more
than 20 external storage providers for long-term archiving and further analysis.

## Prerequisites

To use the Prometheus remote write API with [storage
providers](https://prometheus.io/docs/operating/integrations/#remote-endpoints-and-storage), install
[protobuf](https://developers.google.com/protocol-buffers/) and [snappy](https://github.com/google/snappy) libraries.
Next, [reinstall Netdata](https://github.com/netdata/netdata/blob/master/packaging/installer/REINSTALL.md), which detects that the required libraries and utilities
are now available.

## Configuration

To enable data exporting to a storage provider using the Prometheus remote write API, run `./edit-config exporting.conf`
in the Netdata configuration directory and set the following options:

```conf
[prometheus_remote_write:my_instance]
    enabled = yes
    destination = example.domain:example_port
    remote write URL path = /receive
```

You can also add `:https` modifier to the connector type if you need to use the TLS/SSL protocol. For example:
`remote_write:https:my_instance`.

`remote write URL path` is used to set an endpoint path for the remote write protocol. The default value is `/receive`.
For example, if your endpoint is `http://example.domain:example_port/storage/read`:

```conf
    destination = example.domain:example_port
    remote write URL path = /storage/read
```

You can set basic HTTP authentication credentials using

```conf
    username = my_username
    password = my_password
```

`buffered` and `lost` dimensions in the Netdata Exporting Connector Data Size operation monitoring chart estimate uncompressed
buffer size on failures.

## Notes

The remote write exporting connector does not support `buffer on failures`


