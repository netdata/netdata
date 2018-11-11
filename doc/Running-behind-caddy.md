# netdata via Caddy

To run netdata via [Caddy's proxying,](https://caddyserver.com/docs/proxy) set your Caddyfile up like this:

```
netdata.domain.tld {
    proxy / localhost:19999
}
```

Other directives can be added between the curly brackets as needed.

To run netdata in a subfolder:

```
netdata.domain.tld {
    proxy /netdata/ localhost:19999 {
        without /netdata
    }
}
```

## limit direct access to netdata

You would also need to instruct netdata to listen only to `127.0.0.1` or `::1`.

To limit access to netdata only from localhost, set `bind socket to IP = 127.0.0.1` or `bind socket to IP = ::1` in `/etc/netdata/netdata.conf`.
