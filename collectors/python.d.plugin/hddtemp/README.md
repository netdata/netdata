<!--
title: "Hard drive temperature monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/hddtemp/README.md"
sidebar_label: "Hard drive temperature"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Hardware"
-->

# Hard drive temperature collector

Monitors disk temperatures from one or more `hddtemp` daemons.

**Requirement:**
Running `hddtemp` in daemonized mode with access on tcp port

It produces one chart **Temperature** with dynamic number of dimensions (one per disk)

## Configuration

Edit the `python.d/hddtemp.conf` configuration file using `edit-config` from the Netdata [config
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/hddtemp.conf
```

Sample:

```yaml
update_every: 3
host: "127.0.0.1"
port: 7634
```

If no configuration is given, module will attempt to connect to hddtemp daemon on `127.0.0.1:7634` address




### Troubleshooting

To troubleshoot issues with the `hddtemp` module, run the `python.d.plugin` with the debug option enabled. The 
output will give you the output of the data collection job or error messages on why the collector isn't working.

First, navigate to your plugins directory, usually they are located under `/usr/libexec/netdata/plugins.d/`. If that's 
not the case on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the 
plugin's directory, switch to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

Now you can manually run the `hddtemp` module in debug mode:

```bash
./python.d.plugin hddtemp debug trace
```

