<!--
---
title: "Install Netdata on FreeBSD"
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/methods/freebsd.md
---
-->

# Install Netdata on FreeBSD

## Install from ports/packages
You can install Netdata from either the `ports` or `packages` collections. To install from packages:
```sh
# pkg install netdata
```
You also need to enable the netdata service in `/etc/rc.conf` (add `netdata_enable="YES"`) and start the service:
```sh
# service netdata start
```

## Install latest version
This is how to install the latest Netdata version from source on FreeBSD:

Install required packages (need root permission)
```sh
# pkg install bash e2fsprogs-libuuid git curl autoconf automake pkgconf pidof Judy liblz4 libuv json-c cmake
```

Download Netdata
```sh
# git clone https://github.com/netdata/netdata.git --depth=100 && cd netdata
```

Install Netdata in /opt/netdata (need root permission)
```sh
# ./netdata-installer.sh --install /opt
```

Now we will include the flag that will make Netdata boot with FreeBSD, whenever you turn on or restart your computer (need root permission):
```sh
# sysrc netdata_enable="YES"
```

Finally, and very importantly, update Netdata using the script provided by the Netdata team (need root permission):
```sh
# cd /opt/netdata/usr/libexec/netdata/ && ./netdata-updater.sh
```

# Important
Whenever you have an update available from Netdata, repeat the last step to update Netdata; the netdata-updater.sh script will update. For now, this is the way that the Netdata team offers to make Netdata updates, whenever there is a new version.
