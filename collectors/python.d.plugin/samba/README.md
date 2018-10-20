# samba

Performance metrics of Samba file sharing.

It produces the following charts:

1. **Syscall R/Ws** in kilobytes/s
 * sendfile
 * recvfle

2. **Smb2 R/Ws** in kilobytes/s
 * readout
 * writein
 * readin
 * writeout

3. **Smb2 Create/Close** in operations/s
 * create
 * close

4. **Smb2 Info** in operations/s
 * getinfo
 * setinfo

5. **Smb2 Find** in operations/s
 * find

6. **Smb2 Notify** in operations/s
 * notify

7. **Smb2 Lesser Ops** as counters
 * tcon
 * negprot
 * tdis
 * cancel
 * logoff
 * flush
 * lock
 * keepalive
 * break
 * sessetup

### configuration

Requires that smbd has been compiled with profiling enabled.  Also required
that `smbd` was started either with the `-P 1` option or inside `smb.conf`
using `smbd profiling level`.

This plugin uses `smbstatus -P` which can only be executed by root.  It uses
sudo and assumes that it is configured such that the `netdata` user can
execute smbstatus as root without password.

For example:

    netdata ALL=(ALL)       NOPASSWD: /usr/bin/smbstatus -P

```yaml
update_every : 5 # update frequency
```

---
