<!--
title: "Tor monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/tor/README.md"
sidebar_label: "Tor"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Apps"
-->

# Tor collector

Connects to the Tor control port to collect traffic statistics.

## Requirements

-   `tor` program
-   `stem` python package

It produces only one chart:

1.  **Traffic**

    -   read
    -   write

## Configuration

Edit the `python.d/tor.conf` configuration file using `edit-config` from the Netdata [config
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/tor.conf
```

Needs only `control_port`.

Here is an example for local server:

```yaml
update_every : 1
priority     : 60000

local_tcp:
 name: 'local'
 control_port: 9051
 password: <password> # if required

local_socket:
 name: 'local'
 control_port: '/var/run/tor/control'
 password: <password> # if required
```

### prerequisite

Add to `/etc/tor/torrc`:

```
ControlPort 9051
```

For more options please read the manual.

Without configuration, module attempts to connect to `127.0.0.1:9051`.




### Troubleshooting

To troubleshoot issues with the `tor` module, run the `python.d.plugin` with the debug option enabled. The 
output will give you the output of the data collection job or error messages on why the collector isn't working.

First, navigate to your plugins directory, usually they are located under `/usr/libexec/netdata/plugins.d/`. If that's 
not the case on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the 
plugin's directory, switch to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

Now you can manually run the `tor` module in debug mode:

```bash
./python.d.plugin tor debug trace
```

