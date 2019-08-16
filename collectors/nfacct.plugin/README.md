# nfacct.plugin

`nfacct.plugin` collects Netfilter statistics.

## Prerequisites

1.  install `libmnl-dev` and `libnetfilter_acct-dev` using the package manager of your system.

2.  re-install Netdata from source. The installer will detect that the required libraries are now available and will also build `netdata.plugin`.

Keep in mind that NFACCT requires root access, so the plugin is setuid to root.

## Charts

The plugin provides Netfilter connection tracker statistics and nfacct packet and bandwidth accounting:

Connection tracker:

1.  Connections.
2.  Changes.
3.  Expectations.
4.  Errors.
5.  Searches.

Netfilter accounting:

1.  Packets.
2.  Bandwidth.

## Configuration

If you need to disable NFACCT for Netdata, edit /etc/netdata/netdata.conf and set:

```
[plugins]
    nfacct = no
```

## Debugging

You can run the plugin by hand:

```
sudo /usr/libexec/netdata/plugins.d/nfacct.plugin 1 debug
```

You will get verbose output on what the plugin does.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fnfacct.plugin%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
