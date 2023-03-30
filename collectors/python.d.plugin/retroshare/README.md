<!--
title: "RetroShare monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/retroshare/README.md"
sidebar_label: "RetroShare"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Apm"
-->

# RetroShare collector

Monitors application bandwidth, peers and DHT metrics. 

This module will monitor one or more `RetroShare` applications, depending on your configuration.

## Charts

This module produces the following charts:

-   Bandwidth in `kilobits/s`
-   Peers in `peers`
-   DHT in `peers`


## Configuration

Edit the `python.d/retroshare.conf` configuration file using `edit-config` from the Netdata [config
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/retroshare.conf
```

Here is an example for 2 servers:

```yaml
localhost:
  url      : 'http://localhost:9090'
  user     : "user"
  password : "pass"

remote:
  url      : 'http://203.0.113.1:9090'
  user     : "user"
  password : "pass"
```



### Troubleshooting

To troubleshoot issues with the `retroshare` module, run the `python.d.plugin` with the debug option enabled. The 
output will give you the output of the data collection job or error messages on why the collector isn't working.

First, navigate to your plugins directory, usually they are located under `/usr/libexec/netdata/plugins.d/`. If that's 
not the case on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the 
plugin's directory, switch to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

Now you can manually run the `retroshare` module in debug mode:

```bash
./python.d.plugin retroshare debug trace
```

