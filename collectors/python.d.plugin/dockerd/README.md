<!--
---
title: "Docker engine monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/dockerd/README.md
---
-->

# Docker engine monitoring with Netdata

Collects docker container health metrics.

**Requirement:**

-   `docker` package, required version 3.2.0+

Following charts are drawn:

1.  **running containers**

    -   count

2.  **healthy containers**

    -   count

3.  **unhealthy containers**

    -   count

## Configuration

Edit the `python.d/dockerd.conf` configuration file using `edit-config` from the your agent's [config
directory](../../../docs/step-by-step/step-04.md#find-your-netdataconf-file), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different, if different
sudo ./edit-config python.d/dockerd.conf
```

```yaml
 update_every : 1
 priority     : 60000
```

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fdockerd%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
