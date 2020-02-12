# Apache monitoring with Netdata

Monitors one or more Apache servers depending on configuration.

## Requirements

-   apache with enabled `mod_status`

It produces the following charts:

1.  **Requests** in requests/s

    -   requests

2.  **Connections**

    -   connections

3.  **Async Connections**

    -   keepalive
    -   closing
    -   writing

4.  **Bandwidth** in kilobytes/s

    -   sent

5.  **Workers**

    -   idle
    -   busy

6.  **Lifetime Avg. Requests/s** in requests/s

    -   requests_sec

7.  **Lifetime Avg. Bandwidth/s** in kilobytes/s

    -   size_sec

8.  **Lifetime Avg. Response Size** in bytes/request

    -   size_req

## Configuration

Edit the `python.d/apache.conf` configuration file using `edit-config` from the your agent's [config
directory](../../../docs/step-by-step/step-04.md#find-your-netdataconf-file), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/apache.conf
```

Needs only `url` to server's `server-status?auto`

Example for two servers:

```yaml
update_every : 10
priority     : 90100

local:
  url      : 'http://localhost/server-status?auto'

remote:
  url          : 'http://www.apache.org/server-status?auto'
  update_every : 5
```

Without configuration, module attempts to connect to `http://localhost/server-status?auto`

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fapache%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
