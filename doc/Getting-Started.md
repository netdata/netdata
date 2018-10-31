# Getting started

## Working with netdata

- You can start netdata by executing it with `/usr/sbin/netdata` (the installer will also start it).

- You can stop netdata by killing it with `killall netdata`.
    You can stop and start netdata at any point. Netdata saves on exit its round robbin
    database to `/var/cache/netdata` so that it will continue from where it stopped the last time.

Access to the web site, for all graphs, is by default on port `19999`, so go to:

 ```
 http://127.0.0.1:19999/
 ```

You can get the running config file at any time, by accessing `http://127.0.0.1:19999/netdata.conf`.

## starting netdata at boot

In the `system` directory you can find scripts and configurations for the various distros.

### systemd

The installer already installs `netdata.service` if it detects a systemd system.

To install `netdata.service` by hand, run:

```sh
# stop netdata
killall netdata

# copy netdata.service to systemd
cp system/netdata.service /etc/systemd/system/

# let systemd know there is a new service
systemctl daemon-reload

# enable netdata at boot
systemctl enable netdata

# start netdata
systemctl start netdata
```

### init.d

In the system directory you can find `netdata-lsb`. Copy it to the proper place according to your distribution documentation. For Ubuntu, this can be done via running the following commands as root.

```sh
# copy the netdata startup file to /etc/init.d
cp system/netdata-lsb /etc/init.d/netdata

# make sure it is executable
chmod +x /etc/init.d/netdata

# enable it
update-rc.d netdata defaults
```

### openrc (gentoo)

In the `system` directory you can find `netdata-openrc`. Copy it to the proper place according to your distribution documentation.

### CentOS / Red Hat Enterprise Linux

For older versions of RHEL/CentOS that don't have systemd, an init script is included in the system directory. This can be installed by running the following commands as root.

```sh
# copy the netdata startup file to /etc/init.d
cp system/netdata-init-d /etc/init.d/netdata

# make sure it is executable
chmod +x /etc/init.d/netdata

# enable it
chkconfig --add netdata
```

_There have been some recent work on the init script, see PR https://github.com/netdata/netdata/pull/403_

### other systems

You can start netdata by running it from `/etc/rc.local` or equivalent.

