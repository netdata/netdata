<!--
title: "nfacct.plugin"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/nfacct.plugin/README.md
-->

# nfacct.plugin

`nfacct.plugin` collects Netfilter statistics.

## Prerequisites

1.  install `libmnl-dev` and `libnetfilter-acct-dev` using the package manager of your system.

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


