# varnish

Module uses the `varnishstat` command to provide varnish cache statistics.

It produces:

1.  **Connections Statistics** in connections/s

    -   accepted
    -   dropped

2.  **Client Requests** in requests/s

    -   received

3.  **All History Hit Rate Ratio** in percent

    -   hit
    -   miss
    -   hitpass

4.  **Current Poll Hit Rate Ratio** in percent

    -   hit
    -   miss
    -   hitpass

5.  **Expired Objects** in expired/s

    -   objects

6.  **Least Recently Used Nuked Objects** in nuked/s

    -   objects

7.  **Number Of Threads In All Pools** in threads

    -   threads

8.  **Threads Statistics** in threads/s

    -   created
    -   failed
    -   limited

9.  **Current Queue Length** in requests

    -   in queue

10. **Backend Connections Statistics** in connections/s

    -   successful
    -   unhealthy
    -   reused
    -   closed
    -   resycled
    -   failed

11. **Requests To The Backend** in requests/s

    -   received

12. **ESI Statistics** in problems/s

    -   errors
    -   warnings

13. **Memory Usage** in MB

    -   free
    -   allocated

14. **Uptime** in seconds

    -   uptime

## configuration

Only one parameter is supported:

```yaml
instance_name: 'name'
```

The name of the varnishd instance to get logs from. If not specified, the host name is used.

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fvarnish%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
