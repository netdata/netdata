<!--
title: "Varnish Cache monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/varnish/README.md"
sidebar_label: "Varnish Cache"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Webapps"
-->

# Varnish Cache collector

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
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/varnish.conf
```

Only one parameter is supported:

```yaml
instance_name: 'name'
```

The name of the `varnishd` instance to get logs from. If not specified, the host name is used.




### Troubleshooting

To troubleshoot issues with the `varnish` module, run the `python.d.plugin` with the debug option enabled. The 
output will give you the output of the data collection job or error messages on why the collector isn't working.

First, navigate to your plugins directory, usually they are located under `/usr/libexec/netdata/plugins.d/`. If that's 
not the case on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the 
plugin's directory, switch to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

Now you can manually run the `varnish` module in debug mode:

```bash
./python.d.plugin varnish debug trace
```

