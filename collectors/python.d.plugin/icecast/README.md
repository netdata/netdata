<!--
title: "Icecast monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/icecast/README.md"
sidebar_label: "Icecast"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Networking"
-->

# Icecast collector

Monitors the number of listeners for active sources.

## Requirements

-   icecast version >= 2.4.0

It produces the following charts:

1.  **Listeners** in listeners

-   source number

## Configuration

Edit the `python.d/icecast.conf` configuration file using `edit-config` from the Netdata [config
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/icecast.conf
```

Needs only `url` to server's `/status-json.xsl`

Here is an example for remote server:

```yaml
remote:
  url      : 'http://1.2.3.4:8443/status-json.xsl'
```

Without configuration, module attempts to connect to `http://localhost:8443/status-json.xsl`




### Troubleshooting

To troubleshoot issues with the `icecast` module, run the `python.d.plugin` with the debug option enabled. The 
output will give you the output of the data collection job or error messages on why the collector isn't working.

First, navigate to your plugins directory, usually they are located under `/usr/libexec/netdata/plugins.d/`. If that's 
not the case on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the 
plugin's directory, switch to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

Now you can manually run the `icecast` module in debug mode:

```bash
./python.d.plugin icecast debug trace
```

