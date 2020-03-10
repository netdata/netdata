<!--
---
title: "HP Smart Storage Arrays monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/hpssa/README.md
---
-->

# HP Smart Storage Arrays monitoring with Netdata

Monitors controller, cache module, logical and physical drive state and temperature using `ssacli` tool.

## Requirements:

This module uses `ssacli`, which can only be executed by root. It uses
`sudo` and assumes that it is configured such that the `netdata` user can
execute `ssacli` as root without password.

Add to `sudoers`:

```
netdata ALL=(root)       NOPASSWD: /path/to/ssacli
```

To collect metrics, the module executes: `sudo -n ssacli ctrl all show config detail`

This module produces:

1.  Controller state and temperature
2.  Cache module state and temperature
3.  Logical drive state
4.  Physical drive state and temperature


## Configuration

**hpssa** is disabled by default. Should be explicitly enabled in `python.d.conf`.

```yaml
hpssa: yes
```

Edit the `python.d/hpssa.conf` configuration file using `edit-config` from the your agent's [config
directory](../../../docs/step-by-step/step-04.md#find-your-netdataconf-file), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/hpssa.conf
```

If `ssacli` cannot be found in the `PATH`, configure it in `hpssa.conf`.

```yaml
ssacli_path: /usr/sbin/ssacli
```
