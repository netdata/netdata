<!--
title: "Export metrics to OpenTSDB with HTTP"
description: "Archive your Agent's metrics to a OpenTSDB database for long-term storage and further analysis."
custom_edit_url: https://github.com/netdata/netdata/edit/master/exporting/opentsdb/README.md
sidebar_label: OpenTSDB with HTTP
-->

# Export metrics to OpenTSDB with HTTP

Netdata can easily communicate with OpenTSDB using HTTP API. To enable this channel, run `./edit-config exporting.conf`
in the Netdata configuration directory and set the following options:

```conf
[opentsdb:http:my_instance]
    enabled = yes
    destination = localhost:4242
```

In this example, OpenTSDB is running with its default port, which is `4242`. If you run OpenTSDB on a different port,
change the `destination = localhost:4242` line accordingly.

## HTTPS

As of [v1.16.0](https://github.com/netdata/netdata/releases/tag/v1.16.0), Netdata can send metrics to OpenTSDB using
TLS/SSL. Unfortunately, OpenTDSB does not support encrypted connections, so you will have to configure a reverse proxy
to enable HTTPS communication between Netdata and OpenTSBD. You can set up a reverse proxy with
[Nginx](/docs/Running-behind-nginx.md).

After your proxy is configured, make the following changes to `exporting.conf`:

```conf
[opentsdb:https:my_instance]
    enabled = yes
    destination = localhost:8082
```

In this example, we used the port `8082` for our reverse proxy. If your reverse proxy listens on a different port,
change the `destination = localhost:8082` line accordingly.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fexporting%2Fopentsdb%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
