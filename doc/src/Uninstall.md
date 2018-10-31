# Uninstalling netdata

## netdata was installed from source (or `kickstart.sh`)

The script `netdata-installer.sh` generates another script called `netdata-uninstaller.sh`.

To uninstall netdata, run:

```
cd /path/to/netdata.git
./netdata-uninstaller.sh --force
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
