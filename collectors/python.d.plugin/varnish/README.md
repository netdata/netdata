<!--
title: "Varnish Cache monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/varnish/README.md
sidebar_label: "Varnish Cache"
-->

# Varnish Cache monitoring with Netdata

Provides HTTP accelerator global, Backends (VBE) and Storages (SMF, SMA, MSE) statistics using `varnishstat` tool.

Note that both, Varnish-Cache (free and open source) and Varnish-Plus (Commercial/Enterprise version), are supported.

## Requirements

-   `netdata` user must be a member of the `varnish` group 

## Charts

This module produces the following charts:

-   Connections Statistics in `connections/s`
-   Client Requests in `requests/s`
-   All History Hit Rate Ratio in `percent`
-   Current Poll Hit Rate Ratio in `percent`
-   Expired Objects in `expired/s`
-   Least Recently Used Nuked Objects in `nuked/s`
-   Number Of Threads In All Pools in `pools`
-   Threads Statistics in `threads/s`
-   Current Queue Length in `requests`
-   Backend Connections Statistics in `connections/s`
-   Requests To The Backend in `requests/s`
-   ESI Statistics in `problems/s`
-   Memory Usage in `MiB`
-   Uptime in `seconds`

For every backend (VBE):

-   Backend Response Statistics in `kilobits/s`

For every storage (SMF, SMA, or MSE):

-   Storage Usage in `KiB` 
-   Storage Allocated Objects

## Configuration

Edit the `python.d/varnish.conf` configuration file using `edit-config` from the Netdata [config
directory](/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/varnish.conf
```

Only one parameter is supported:

```yaml
instance_name: 'name'
```

The name of the `varnishd` instance to get logs from. If not specified, the host name is used.

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fvarnish%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
