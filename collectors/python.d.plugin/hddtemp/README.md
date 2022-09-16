<!--
title: "Hard drive temperature monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/hddtemp/README.md
sidebar_label: "Hard drive temperature"
-->

# Hard drive temperature monitoring with Netdata

Monitors disk temperatures from one or more `hddtemp` daemons.

**Requirement:**
Running `hddtemp` in daemonized mode with access on tcp port

It produces one chart **Temperature** with dynamic number of dimensions (one per disk)

## Configuration

Edit the `python.d/hddtemp.conf` configuration file using `edit-config` from the Netdata [config
directory](/docs/configure/nodes.md), which is typically at `/etc/netdata`.

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

---


