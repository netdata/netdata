<!--
title: "Storage NVMe devices monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/charts.d.plugin/nvme/README.md
-->

# Storage NVMe devices monitoring with Netdata

Collect information from NVMe devices.

Executed commands:

- `sudo -n nvme list`


## Requirements

- Install: nvme-cli

The module uses `nvme-cli`, which can only be executed by `root`. It uses
`sudo` and assumes that it is configured such that the `netdata` user can execute `nvme` as root without a password.

- Add to your `/etc/sudoers` file:

`which nvme` shows the full path to the binary.

```bash
netdata ALL=(root)       NOPASSWD: /path/to/nvme
```

- Reset Netdata's systemd
  unit [CapabilityBoundingSet](https://www.freedesktop.org/software/systemd/man/systemd.exec.html#Capabilities) (Linux
  distributions with systemd)

The default CapabilityBoundingSet doesn't allow using `sudo`, and is quite strict in general. Resetting is not optimal, but a next-best solution given the inability to execute `nvme` using `sudo`.


As the `root` user, do the following:

```cmd
mkdir /etc/systemd/system/netdata.service.d
echo -e '[Service]\nCapabilityBoundingSet=~' | tee /etc/systemd/system/netdata.service.d/unset-capability-bounding-set.conf
systemctl daemon-reload
systemctl restart netdata.service
```


## Charts
- Critical Warning
- Percentage Used
- Temperature
- Power cycles
- Power on hours


## Configuration

Edit the `charts.d/nvme.conf` configuration file using `edit-config` from the Netdata [config
directory](/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config charts.d/nvme.conf
```

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fcharts.d.plugin%2Fexample%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
