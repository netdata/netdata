# xenstat.plugin

`xenstat.plugin` collects XenServer and XCP-ng statistics.

## Prerequisites

1.  install `xen-dom0-libs-devel` and `yajl-devel` using the package manager of your system.

2.  re-install Netdata from source. The installer will detect that the required libraries are now available and will also build xenstat.plugin.

Keep in mind that `libxenstat` requires root access, so the plugin is setuid to root.

## Charts

The plugin provides XenServer and XCP-ng host and domains statistics:

Host:

1.  Number of domains.

Domain:

1.  CPU.
2.  Memory.
3.  Networks.
4.  VBDs.

## Configuration

If you need to disable xenstat for Netdata, edit /etc/netdata/netdata.conf and set:

```
[plugins]
    xenstat = no
```

## Debugging

You can run the plugin by hand:

```
sudo /usr/libexec/netdata/plugins.d/xenstat.plugin 1 debug
```

You will get verbose output on what the plugin does.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fnfacct.plugin%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
