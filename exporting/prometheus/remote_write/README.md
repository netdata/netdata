# Prometheus remote write exporting connector

The Prometheus remote write exporting connector uses the exporting engine to send Netdata metrics to your choice of more
than 20 external storage providers for long-term archiving and further analysis.

## Prerequisites

To use the Prometheus remote write API with [storage
providers](https://prometheus.io/docs/operating/integrations/#remote-endpoints-and-storage), install
[protobuf](https://developers.google.com/protocol-buffers/) and [snappy](https://github.com/google/snappy) libraries.
Next, re-install Netdata from the source, which detects that the required libraries and
utilities are now available.

## Configuration

To enable data exporting to a storage provider using the Prometheus remote write API, run `./edit-config exporting.conf`
in the Netdata configuration directory and set the following options:

```conf
[remote_write:my_instance]
    enabled = yes
    destination = example.domain:example_port
    remote write URL path = /receive
```

`remote write URL path` is used to set an endpoint path for the remote write protocol. The default value is `/receive`.
For example, if your endpoint is `http://example.domain:example_port/storage/read`:

```conf
    destination = example.domain:example_port
    remote write URL path = /storage/read
```

`buffered` and `lost` dimensions in the Netdata Exporting Connector Data Size operation monitoring chart estimate uncompressed
buffer size on failures.

## Notes

The remote write exporting connector does not support `buffer on failures`

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fexporting%2Fprometheus%2Fremote_write%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
