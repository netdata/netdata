<!--
title: "Gearman monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/gearman/README.md
sidebar_label: "Gearman"
-->

# Gearman monitoring with Netdata

Monitors Gearman worker statistics. A chart is shown for each job as well as one showing a summary of all workers.

Note: Charts may show as a line graph rather than an area 
graph if you load Netdata with no jobs running. To change 
this go to "Settings" > "Which dimensions to show?" and 
select "All".

Plugin can obtain data from tcp socket **OR** unix socket.

**Requirement:**
Socket MUST be readable by netdata user.

It produces:

 * Workers queued
 * Workers idle
 * Workers running

## Configuration

Edit the `python.d/gearman.conf` configuration file using `edit-config` from the Netdata [config
directory](/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/gearman.conf
```

```yaml
localhost:
  name     : 'local'
  host     : 'localhost'
  port     : 4730
  
  # TLS information can be provided as well
  tls      : no
  cert     : /path/to/cert
  key      : /path/to/key
```

When no configuration file is found, module tries to connect to TCP/IP socket: `localhost:4730`.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fgearman%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
