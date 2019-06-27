# OpenTSDB with HTTP

Since version 1.16 the Netdata has the feature to communicate with OpenTSDB using HTTP API. To enable this channel
it is necessary to set the following options in your netdata.conf

```
[backend]
    type = opentsdb:http
    destination = localhost:4242
```

, in this example we are considering that OpenTSDB is running with its default port (4242).

## HTTPS

Netdata also supports sending the metrics using SSL/TLS, but OpenTDSB does not have support to safety connections,
so it will be necessary to configure a reverse-proxy to enable the HTTPS communication. After to configure your proxy the
following changes must be done in the netdata.conf:

```
[backend]
    type = opentsdb:https
    destination = localhost:8082
```

In this example we used the port 8082 for our reverse proxy.
