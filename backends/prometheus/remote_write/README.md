<!--
---
title: "Prometheus remote write backend"
custom_edit_url: https://github.com/netdata/netdata/edit/master/backends/prometheus/remote_write/README.md
---
-->

# Prometheus remote write backend

## Prerequisites

To use the prometheus remote write API with [storage
providers](https://prometheus.io/docs/operating/integrations/#remote-endpoints-and-storage)
[protobuf](https://developers.google.com/protocol-buffers/) and [snappy](https://github.com/google/snappy) libraries
should be installed first. Next, Netdata should be re-installed from the source. The installer will detect that the
required libraries and utilities are now available.

## Configuration

An additional option in the backend configuration section is available for the remote write backend:

```conf
[backend]
    remote write URL path = /receive
```

The default value is `/receive`. `remote write URL path` is used to set an endpoint path for the remote write protocol.
For example, if your endpoint is `http://example.domain:example_port/storage/read` you should set

```conf
[backend]
    destination = example.domain:example_port
    remote write URL path = /storage/read
```

`buffered` and `lost` dimensions in the Netdata Backend Data Size operation monitoring chart estimate uncompressed
buffer size on failures.

## Notes

The remote write backend does not support `buffer on failures`

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fbackends%2Fprometheus%2Fremote_write%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
