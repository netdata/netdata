<!--
title: "Install Netdata on OpenWrt"
description: "Install Netdata on OpenWrt to monitor the health and performance of routers with thousands of real-time, per-second metrics."
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/methods/openwrt.md
sidebar_label: "OpenWrt"
learn_status: "Published"
learn_rel_path: "Installation/Install on specific environments"
-->

# Install Netdata on OpenWrt

> 💡 This document may not be completely up-to-date. Please double-check the
> details of the installation process before proceeding, particularly in 
> places where specific software or package versions are mentioned.
>
> You can help improve this document by [submitting a
> PR](https://github.com/netdata/netdata/edit/master/packaging/installer/methods/openwrt.md) with your recommended
> improvements or changes. Thank you!

Since PRs [#14822](https://github.com/netdata/netdata/pull/14822) and [#14994](https://github.com/netdata/netdata/pull/14994), it is possible to install Netdata static builds on OpenWrt using [kickstart.sh](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/kickstart.md). However, some extra steps are required to ensure a successful Netdata install, with features such as log rotation and automatic updates working correctly.

A new installation of OpenWrt [may not offer enough space](https://forum.openwrt.org/t/howto-resizing-root-partition-on-x86/140631#the-root-cause-1) for the Netdata static builds to be decompressed and for the installation to be completed successfully. In that case, the root partition and the root filesystem may need to be expanded. A comprehensive guide on how to accomplish that on x86 can be found [here](https://forum.openwrt.org/t/howto-resizing-root-partition-on-x86/140631#the-root-cause-1).

As of Netdata version `v1.39.0`, at least 300MB of space are required for the root filesystem, if `dbengine` is disabled. If `dbengine` is used to store metrics locally, a larger root partition is required.

Once there is enough space for the installation, proceed with installing `bash`: 
```sh
opkg update && opkg install bash
```

For proper user and group management during the installation process, the following packages are required: 
```sh
opkg install shadow-groupadd shadow-groupdel shadow-useradd shadow-userdel  shadow-usermod
```
or to install all packages of the `shadow` group:
```sh
opkg install shadow
```

`logrotate` package is also missing from the default OpenWrt image, so to install it:
```sh
opkg install logrotate
```

Finally, proceed with the Netdata installation using the [kickstart.sh one line installer](https://learn.netdata.cloud/docs/installation/installation-methods/one-line-installer-kickstart.sh) method. The installer should complete successfully and it should start Netdata.

OpenWrt keeps cron jobs configuration in `/etc/crontabs/root`, so the Netdata installer cannot enable automatic updates during the installation process. To enable them manually, run the following commands:
```sh
# Enable the cron service 
/etc/init.d/cron enable

# Edit cron jobs configuration using vi
crontab -e 

# Add your desired configuration for netdata-updater.sh 
# It is recommended to have it scheduled to run daily:
# 0 5 * * * /opt/netdata/usr/libexec/netdata/netdata-updater.sh

# Restart the cron service
/etc/init.d/cron restart
