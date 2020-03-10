<!--
---
title: "Samba monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/samba/README.md
---
-->

# Samba monitoring with Netdata

Monitors the performance metrics of Samba file sharing.

## Requirements

-   `smbstatus` program
-   `sudo` program
-   `smbd` must be compiled with profiling enabled
-   `smbd` must be started either with the `-P 1` option or inside `smb.conf` using `smbd profiling level`
-   `netdata` user needs to be able to sudo the `smbstatus` program without password

It produces the following charts:

1.  **Syscall R/Ws** in kilobytes/s

    -   sendfile
    -   recvfle

2.  **Smb2 R/Ws** in kilobytes/s

    -   readout
    -   writein
    -   readin
    -   writeout

3.  **Smb2 Create/Close** in operations/s

    -   create
    -   close

4.  **Smb2 Info** in operations/s

    -   getinfo
    -   setinfo

5.  **Smb2 Find** in operations/s

    -   find

6.  **Smb2 Notify** in operations/s

    -   notify

7.  **Smb2 Lesser Ops** as counters

    -   tcon
    -   negprot
    -   tdis
    -   cancel
    -   logoff
    -   flush
    -   lock
    -   keepalive
    -   break
    -   sessetup

## prerequisite

This module uses `smbstatus` which can only be executed by root.  It uses
`sudo` and assumes that it is configured such that the `netdata` user can
execute `smbstatus` as root without password.

Add to `sudoers`:

```
netdata ALL=(root)       NOPASSWD: /path/to/smbstatus
```

## Configuration

**samba** is disabled by default. Should be explicitly enabled in `python.d.conf`.

```yaml
samba: yes
```

Edit the `python.d/samba.conf` configuration file using `edit-config` from the your agent's [config
directory](../../../docs/step-by-step/step-04.md#find-your-netdataconf-file), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/samba.conf
```

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fsamba%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
