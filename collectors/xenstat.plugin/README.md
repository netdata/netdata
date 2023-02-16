<!--
title: "xenstat.plugin"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/xenstat.plugin/README.md"
sidebar_label: "xenstat.plugin"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Virtualized environments/Virtualize hosts"
-->

# xenstat.plugin

`xenstat.plugin` collects XenServer and XCP-ng statistics.

## Prerequisites

1.  install `xen-dom0-libs-devel` and `yajl-devel` using the package manager of your system.
    Note: On Cent-OS systems you will need `centos-release-xen` repository and the required package for xen is `xen-devel`

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


