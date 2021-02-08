<!--
title: "Adaptec RAID controller monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/adaptec_raid/README.md
sidebar_label: "Adaptec RAID"
-->

# Adaptec RAID controller monitoring with Netdata

Collects logical and physical devices metrics using `arcconf` command-line utility.

Executed commands:

- `sudo -n arcconf GETCONFIG 1 LD`
- `sudo -n arcconf GETCONFIG 1 PD`

## Requirements

The module uses `arcconf`, which can only be executed by `root`. It uses
`sudo` and assumes that it is configured such that the `netdata` user can execute `arcconf` as root without a password.

-  Add to your `/etc/sudoers` file:

`which arcconf` shows the full path to the binary.

```bash
netdata ALL=(root)       NOPASSWD: /path/to/arcconf
```

- Reset Netdata's systemd
  unit [CapabilityBoundingSet](https://www.freedesktop.org/software/systemd/man/systemd.exec.html#Capabilities) (Linux
  distributions with systemd)

The default CapabilityBoundingSet doesn't allow using `sudo`, and is quite strict in general. Resetting is not optimal, but a next-best solution given the inability to execute `arcconf` using `sudo`.


As the `root` user, do the following:

```cmd
mkdir /etc/systemd/system/netdata.service.d
echo -e '[Service]\nCapabilityBoundingSet=~' | tee /etc/systemd/system/netdata.service.d/unset-capability-bounding-set.conf
systemctl daemon-reload
systemctl restart netdata.service
```

## Charts

- Logical Device Status
- Physical Device State
- Physical Device S.M.A.R.T warnings
- Physical Device Temperature

## Enable the collector

The `adaptec_raid` collector is disabled by default. To enable it, use `edit-config` from the
Netdata [config directory](/docs/configure/nodes.md), which is typically at `/etc/netdata`, to edit the `python.d.conf`
file.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d.conf
```

Change the value of the `adaptec_raid` setting to `yes`. Save the file and restart the Netdata Agent
with `sudo systemctl restart netdata`, or the appropriate method for your system.

## Configuration

Edit the `python.d/adaptec_raid.conf` configuration file using `edit-config` from the
Netdata [config directory](/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/adaptec_raid.conf
```

![image](https://user-images.githubusercontent.com/22274335/47278133-6d306680-d601-11e8-87c2-cc9c0f42d686.png)

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fadaptec_raid%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
