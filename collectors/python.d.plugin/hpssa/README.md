<!--
title: "HP Smart Storage Arrays monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/hpssa/README.md
sidebar_label: "HP Smart Storage Arrays"
-->

# HP Smart Storage Arrays monitoring with Netdata

Monitors controller, cache module, logical and physical drive state and temperature using `ssacli` tool.

Executed commands:

- `sudo -n ssacli ctrl all show config detail`

## Requirements:

This module uses `ssacli`, which can only be executed by root. It uses
`sudo` and assumes that it is configured such that the `netdata` user can execute `ssacli` as root without a password.

- Add to your `/etc/sudoers` file:

`which ssacli` shows the full path to the binary.

```bash
netdata ALL=(root)       NOPASSWD: /path/to/ssacli
```

- Reset Netdata's systemd
  unit [CapabilityBoundingSet](https://www.freedesktop.org/software/systemd/man/systemd.exec.html#Capabilities) (Linux
  distributions with systemd)

The default CapabilityBoundingSet doesn't allow using `sudo`, and is quite strict in general. Resetting is not optimal, but a next-best solution given the inability to execute `ssacli` using `sudo`.

As the `root` user, do the following:

```cmd
mkdir /etc/systemd/system/netdata.service.d
echo -e '[Service]\nCapabilityBoundingSet=~' | tee /etc/systemd/system/netdata.service.d/unset-capability-bounding-set.conf
systemctl daemon-reload
systemctl restart netdata.service
```

## Charts

- Controller status
- Controller temperature
- Logical drive status
- Physical drive status
- Physical drive temperature

## Enable the collector

The `hpssa` collector is disabled by default. To enable it, use `edit-config` from the
Netdata [config directory](/docs/configure/nodes.md), which is typically at `/etc/netdata`, to edit the `python.d.conf`
file.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d.conf
```

Change the value of the `hpssa` setting to `yes`. Save the file and restart the Netdata Agent with `sudo systemctl
restart netdata`, or the [appropriate method](/docs/configure/start-stop-restart.md) for your system.

## Configuration

Edit the `python.d/hpssa.conf` configuration file using `edit-config` from the
Netdata [config directory](/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/hpssa.conf
```

If `ssacli` cannot be found in the `PATH`, configure it in `hpssa.conf`.

```yaml
ssacli_path: /usr/sbin/ssacli
```

Save the file and restart the Netdata Agent with `sudo systemctl restart netdata`, or the [appropriate
method](/docs/configure/start-stop-restart.md) for your system.

