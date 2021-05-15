<!--
title: "Watchtower monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/watchtower/README.md
-->

# Watchtower monitoring with Netdata

A module for monitoring [Watchtower](https://github.com/containrrr/watchtower), a container-based solution for
automating Docker container base image updates.

## Requirements

- a running watchtower instance
    * with a configured [API token](https://containrrr.dev/watchtower/arguments/#http-api-token)
    * with [http-api-metrics](https://containrrr.dev/watchtower/arguments/#http-api-metrics) enabled.

It produces following charts:

1. **Containers**

    - scanned
        * Number of containers scanned for changes by watchtower during the last scan
    - updated
        * Number of containers updated by watchtower during the last scan
    - failed
        * Number of containers where update failed during the last scan

2. **Scans**

    - total
        * Number of scans since the watchtower started
    - skipped
        * Number of skipped scans since watchtower started

## Configuration

Edit the `python.d/watchtower.conf` configuration file using `edit-config` from the
Netdata [config directory](/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/watchtower.conf
```

Needs `url` and `api_token` to watchtower instance.

Here is an example for local server:

```yaml
update_every: 3600

local:
  update_every: 1             # the JOB's data collection frequency
  url: http://127.0.0.1:8080  # the url to your watchtower api endpoint
  api_token: abcdefg          # watchtower's HTTP API Token
```

Without configuration, module attempts to connect to `http://127.0.0.1:8080`

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fnginx%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
