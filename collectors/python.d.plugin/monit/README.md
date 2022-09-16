<!--
title: "Monit monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/monit/README.md
sidebar_label: "Monit"
-->

# Monit monitoring with Netdata

Monit monitoring module. Data is grabbed from stats XML interface (exists for a long time, but not mentioned in official documentation). Mostly this plugin shows statuses of monit targets, i.e. [statuses of specified checks](https://mmonit.com/monit/documentation/monit.html#Service-checks).

1.  **Filesystems**

    -   Filesystems
    -   Directories
    -   Files
    -   Pipes

2.  **Applications**

    -   Processes (+threads/childs)
    -   Programs

3.  **Network**

    -   Hosts (+latency)
    -   Network interfaces

## Configuration

Edit the `python.d/monit.conf` configuration file using `edit-config` from the Netdata [config
directory](/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/monit.conf
```

Sample:

```yaml
local:
  name     : 'local'
  url     : 'http://localhost:2812'
  user:    : admin
  pass:    : monit
```

If no configuration is given, module will attempt to connect to monit as `http://localhost:2812`.

---


