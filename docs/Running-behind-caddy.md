# Netdata via Caddy

To run Netdata via [Caddy's proxying,](https://caddyserver.com/docs/proxy) set your Caddyfile up like this:

```caddyfile
netdata.domain.tld {
    proxy / localhost:19999
}
```

Other directives can be added between the curly brackets as needed.

To run Netdata in a subfolder:

```caddyfile
netdata.domain.tld {
    proxy /netdata/ localhost:19999 {
        without /netdata
    }
}
```

## limit direct access to Netdata

You would also need to instruct Netdata to listen only to `127.0.0.1` or `::1`.

To limit access to Netdata only from localhost, set `bind socket to IP = 127.0.0.1` or `bind socket to IP = ::1` in `/etc/netdata/netdata.conf`.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2FRunning-behind-caddy&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
