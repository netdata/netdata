<!--
title: "Samba monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/samba/README.md
sidebar_label: "Samba"
-->

# Samba monitoring with Netdata

Monitors the performance metrics of Samba file sharing using `smbstatus` command-line tool.

Executed commands:

- `sudo -n smbstatus -P`

## Requirements

- `smbstatus` program
- `sudo` program
- `smbd` must be compiled with profiling enabled
- `smbd` must be started either with the `-P 1` option or inside `smb.conf` using `smbd profiling level`

The module uses `smbstatus`, which can only be executed by `root`. It uses
`sudo` and assumes that it is configured such that the `netdata` user can execute `smbstatus` as root without a
password.

- Add to your `/etc/sudoers` file:

`which smbstatus` shows the full path to the binary.

```bash
netdata ALL=(root)       NOPASSWD: /path/to/smbstatus
```

- Reset Netdata's systemd
  unit [CapabilityBoundingSet](https://www.freedesktop.org/software/systemd/man/systemd.exec.html#Capabilities) (Linux
  distributions with systemd)

The default CapabilityBoundingSet doesn't allow using `sudo`, and is quite strict in general. Resetting is not optimal, but a next-best solution given the inability to execute `smbstatus` using `sudo`.


As the `root` user, do the following:

```cmd
mkdir /etc/systemd/system/netdata.service.d
echo -e '[Service]\nCapabilityBoundingSet=~' | tee /etc/systemd/system/netdata.service.d/unset-capability-bounding-set.conf
systemctl daemon-reload
systemctl restart netdata.service
```

## Charts

1. **Syscall R/Ws** in kilobytes/s

    - sendfile
    - recvfile

2. **Smb2 R/Ws** in kilobytes/s

    - readout
    - writein
    - readin
    - writeout

3. **Smb2 Create/Close** in operations/s

    - create
    - close

4. **Smb2 Info** in operations/s

    - getinfo
    - setinfo

5. **Smb2 Find** in operations/s

    - find

6. **Smb2 Notify** in operations/s

    - notify

7. **Smb2 Lesser Ops** as counters

    - tcon
    - negprot
    - tdis
    - cancel
    - logoff
    - flush
    - lock
    - keepalive
    - break
    - sessetup

## Enable the collector

The `samba` collector is disabled by default. To enable it, use `edit-config` from the
Netdata [config directory](/docs/configure/nodes.md), which is typically at `/etc/netdata`, to edit the `python.d.conf`
file.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d.conf
```

Change the value of the `samba` setting to `yes`. Save the file and restart the Netdata Agent with `sudo systemctl
restart netdata`, or the [appropriate method](/docs/configure/start-stop-restart.md) for your system.

## Configuration

Edit the `python.d/samba.conf` configuration file using `edit-config` from the
Netdata [config directory](/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/samba.conf
```

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fsamba%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
