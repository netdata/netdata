# Uninstalling netdata

## netdata was installed from source (or `kickstart.sh`)

The script `netdata-installer.sh` generates another script called `netdata-uninstaller.sh`.

To uninstall netdata, run:

```
cd /path/to/netdata.git
./netdata-uninstaller.sh --yes
```

The uninstaller will ask you to confirm all deletions.

## netdata was installed with `kickstart-static64.sh` package

Stop netdata with one of the following:

- `service netdata stop` (non-systemd systems)
- `systemctl stop netdata` (systemd systems)

Disable running netdata at startup, with one of the following (based on your distro):

- `rc-update del netdata`
- `update-rc.d netdata disable`
- `chkconfig netdata off`
- `systemctl disable netdata`

Delete the netdata files:

1. `rm -rf /opt/netdata`
2. `groupdel netdata`
3. `userdel netdata`
4. `rm /etc/logrotate.d/netdata`
5. `rm /etc/systemd/system/netdata.service` or `rm /etc/init.d/netdata`, depending on the distro.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Finstaller%2FUNINSTALL&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
