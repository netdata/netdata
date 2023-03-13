<!--
title: "Monit monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/monit/README.md"
sidebar_label: "Monit"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Storage"
-->

# Monit collector

Monit monitoring module. Data is grabbed from stats XML interface (exists for a long time, but not mentioned in official
documentation). Mostly this plugin shows statuses of monit targets, i.e.
[statuses of specified checks](https://mmonit.com/monit/documentation/monit.html#Service-checks).

1. **Filesystems**

    - Filesystems
    - Directories
    - Files
    - Pipes

2. **Applications**

    - Processes (+threads/childs)
    - Programs

3. **Network**

    - Hosts (+latency)
    - Network interfaces

## Configuration

Edit the `python.d/monit.conf` configuration file using `edit-config` from the
Netdata [config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically
at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/monit.conf
```

Sample:

```yaml
local:
  name: 'local'
  url: 'http://localhost:2812'
  user: : admin
  pass: : monit
```

If no configuration is given, module will attempt to connect to monit as `http://localhost:2812`.




### Troubleshooting

To troubleshoot issues with the `monit` module, run the `python.d.plugin` with the debug option enabled. The 
output will give you the output of the data collection job or error messages on why the collector isn't working.

First, navigate to your plugins directory, usually they are located under `/usr/libexec/netdata/plugins.d/`. If that's 
not the case on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the 
plugin's directory, switch to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

Now you can manually run the `monit` module in debug mode:

```bash
./python.d.plugin monit debug trace
```

