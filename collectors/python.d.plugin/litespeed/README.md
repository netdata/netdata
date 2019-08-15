# litespeed

Module monitor litespeed web server performance metrics.

It produces:

1.  **Network Throughput HTTP** in kilobits/s

    -   in
    -   out

2.  **Network Throughput HTTPS** in kilobits/s

    -   in
    -   out

3.  **Connections HTTP** in connections

    -   free
    -   used

4.  **Connections HTTPS** in connections

    -   free
    -   used

5.  **Requests** in requests/s

    -   requests

6.  **Requests In Processing** in requests

    -   processing

7.  **Public Cache Hits** in hits/s

    -   hits

8.  **Private Cache Hits** in hits/s

    -   hits

9.  **Static Hits** in hits/s

    -   hits

## configuration

```yaml
local:
  path  : 'PATH'
```

If no configuration is given, module will use "/tmp/lshttpd/".

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Flitespeed%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
