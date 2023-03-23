<!--
title: "SpigotMC monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/spigotmc/README.md"
sidebar_label: "SpigotMC"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Webapps"
-->

# SpigotMC collector

Performs basic monitoring for Spigot Minecraft servers.

It provides two charts, one tracking server-side ticks-per-second in
1, 5 and 15 minute averages, and one tracking the number of currently
active users.

This is not compatible with Spigot plugins which change the format of
the data returned by the `tps` or `list` console commands.

## Configuration

Edit the `python.d/spigotmc.conf` configuration file using `edit-config` from the Netdata [config
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/spigotmc.conf
```

```yaml
host: localhost
port: 25575
password: pass
```

By default, a connection to port 25575 on the local system is attempted with an empty password.




### Troubleshooting

To troubleshoot issues with the `spigotmc` module, run the `python.d.plugin` with the debug option enabled. The 
output will give you the output of the data collection job or error messages on why the collector isn't working.

First, navigate to your plugins directory, usually they are located under `/usr/libexec/netdata/plugins.d/`. If that's 
not the case on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the 
plugin's directory, switch to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

Now you can manually run the `spigotmc` module in debug mode:

```bash
./python.d.plugin spigotmc debug trace
```

