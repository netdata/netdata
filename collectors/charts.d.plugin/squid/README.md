# squid

> THIS MODULE IS OBSOLETE.
> USE [THE PYTHON ONE](../../python.d.plugin/squid) - IT SUPPORTS MULTIPLE JOBS AND IT IS MORE EFFICIENT

The plugin will monitor a squid server.

It will produce 4 charts:

1. **Squid Client Bandwidth** in kbps

 * in
 * out
 * hits

2. **Squid Client Requests** in requests/sec

 * requests
 * hits
 * errors

3. **Squid Server Bandwidth** in kbps

 * in
 * out

4. **Squid Server Requests** in requests/sec

 * requests
 * errors

### autoconfig

The plugin will by itself detect squid servers running on
localhost, on ports 3128 or 8080.

It will attempt to download URLs in the form:

- `cache_object://HOST:PORT/counters`
- `/squid-internal-mgr/counters`

If any succeeds, it will use this.

### configuration

If you need to configure it by hand, create the file
`/etc/netdata/squid.conf` with the following variables:

- `squid_host=IP` the IP of the squid host
- `squid_port=PORT` the port the squid is listening
- `squid_url="URL"` the URL with the statistics to be fetched from squid
- `squid_timeout=SECONDS` how much time we should wait for squid to respond
- `squid_update_every=SECONDS` the frequency of the data collection

Example `/etc/netdata/squid.conf`:

```sh
squid_host=127.0.0.1
squid_port=3128
squid_url="cache_object://127.0.0.1:3128/counters"
squid_timeout=2
squid_update_every=5
```

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fcharts.d.plugin%2Fsquid%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
