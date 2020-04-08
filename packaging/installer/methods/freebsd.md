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

```sh
# install required packages
pkg install bash e2fsprogs-libuuid git curl autoconf automake pkgconf pidof Judy liblz4 libuv json-c

# download Netdata
git clone https://github.com/netdata/netdata.git --depth=100

# install Netdata in /opt/netdata
cd netdata
./netdata-installer.sh --install /opt
```
