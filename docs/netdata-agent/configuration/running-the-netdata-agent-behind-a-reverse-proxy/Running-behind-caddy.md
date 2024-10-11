# Running Netdata behind Caddy

To run Netdata via [Caddy v2 reverse proxy,](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy) set your Caddyfile up like this:

```caddyfile
netdata.domain.tld {
    reverse_proxy localhost:19999
}
```

Other directives can be added between the curly brackets as needed.

To run Netdata in a subfolder:

```caddyfile
netdata.domain.tld {
    handle_path /netdata/* {
        reverse_proxy localhost:19999
    }
}
```

## limit direct access to Netdata

You would also need to instruct Netdata to listen only to `127.0.0.1` or `::1`.

To limit access to Netdata only from localhost, set `bind socket to IP = 127.0.0.1` or `bind socket to IP = ::1` in `/etc/netdata/netdata.conf`.
