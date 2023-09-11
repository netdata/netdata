<!--
title: "Gearman monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/gearman/README.md"
sidebar_label: "Gearman"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Distributed computing"
-->

# Gearman collector

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
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`.

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

### Troubleshooting

To troubleshoot issues with the `gearman` module, run the `python.d.plugin` with the debug option enabled. The 
output will give you the output of the data collection job or error messages on why the collector isn't working.

First, navigate to your plugins directory, usually they are located under `/usr/libexec/netdata/plugins.d/`. If that's 
not the case on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the 
plugin's directory, switch to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

Now you can manually run the `gearman` module in debug mode:

```bash
./python.d.plugin gearman debug trace
```

