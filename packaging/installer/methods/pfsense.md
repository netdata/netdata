<!--
title: "Install Netdata on pfSense"
description: "Install Netdata on pfSense to monitor the health and performance of firewalls with thousands of real-time, per-second metrics."
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/methods/pfsense.md
sidebar_label: "pfSense"
learn_status: "Published"
learn_rel_path: "Installation/Install on specific environments"
-->

# Install Netdata on pfSense

> 💡 This document is maintained by Netdata's community, and may not be completely up-to-date. Please double-check the
> details of the installation process, such as version numbers for downloadable packages, before proceeding.
>
> You can help improve this document by [submitting a
> PR](https://github.com/netdata/netdata/edit/master/packaging/installer/methods/pfsense.md) with your recommended
> improvements or changes. Thank you!

## Install prerequisites/dependencies

To install Netdata on pfSense, first run the following command (within a shell or under the **Diagnostics/Command**
prompt within the pfSense web interface).

```bash
pkg install -y pkgconf bash e2fsprogs-libuuid libuv nano
```

Then run the following commands to download various dependencies from the FreeBSD repository.

```sh
pkg add http://pkg.freebsd.org/FreeBSD:12:amd64/latest/All/json-c-0.15_1.txz
pkg add http://pkg.freebsd.org/FreeBSD:12:amd64/latest/All/py38-certifi-2021.10.8.txz
pkg add http://pkg.freebsd.org/FreeBSD:12:amd64/latest/All/py38-asn1crypto-1.4.0.txz
pkg add http://pkg.freebsd.org/FreeBSD:12:amd64/latest/All/py38-pycparser-2.20.txz
pkg add http://pkg.freebsd.org/FreeBSD:12:amd64/latest/All/py38-cffi-1.14.6.txz
pkg add http://pkg.freebsd.org/FreeBSD:12:amd64/latest/All/py38-six-1.16.0.txz
pkg add http://pkg.freebsd.org/FreeBSD:12:amd64/latest/All/py38-cryptography-3.3.2.txz
pkg add http://pkg.freebsd.org/FreeBSD:12:amd64/latest/All/py38-idna-2.10.txz
pkg add http://pkg.freebsd.org/FreeBSD:12:amd64/latest/All/py38-openssl-20.0.1.txz
pkg add http://pkg.freebsd.org/FreeBSD:12:amd64/latest/All/py38-pysocks-1.7.1.txz
pkg add http://pkg.freebsd.org/FreeBSD:12:amd64/latest/All/py38-urllib3-1.26.7,1.txz
pkg add http://pkg.freebsd.org/FreeBSD:12:amd64/latest/All/py38-yaml-5.4.1.txz
```

> ⚠️ If any of the above commands return a `Not Found` error, you need to manually search for the latest package in the
> [FreeBSD repository](https://www.freebsd.org/ports/). Search for the package's name, such as `py37-cffi`, find the
> latest version number, and update the command accordingly.

> ⚠️ On pfSense 2.4.5, Python version 3.7 may be installed by the system, in which case you should should not install
> Python from the FreeBSD repository as instructed above.

> ⚠️ If you are using the `apcupsd` collector, you need to make sure that apcupsd is up before starting Netdata.
> Otherwise a infinitely running `cat` process triggered by the default activated apcupsd charts plugin will eat up CPU
> and RAM (`/tmp/.netdata-charts.d-*/run-*`). This also applies to `OPNsense`.

## Install Netdata

You can now install Netdata from the FreeBSD repository.

```bash
pkg add http://pkg.freebsd.org/FreeBSD:12:amd64/latest/All/netdata-1.31.0_1.txz
```

> ⚠️ If the above command returns a `Not Found` error, you need to manually search for the latest version of Netdata in
> the [FreeBSD repository](https://www.freebsd.org/ports/). Search for `netdata`, find the latest version number, and
> update the command accordingly.

You must edit `/usr/local/etc/netdata/netdata.conf` and change `bind to = 127.0.0.1` to `bind to = 0.0.0.0`.

To start Netdata manually, run `service netdata onestart`.

Visit the Netdata dashboard to confirm it's working: `http://<pfsenseIP>:19999`

To start Netdata automatically every boot, add `service netdata onestart` as a Shellcmd entry within the pfSense web
interface under **Services/Shellcmd**. You'll need to install the Shellcmd package beforehand under **System/Package
Manager/Available Packages**. The Shellcmd Type should be set to `Shellcmd`.  
![](https://i.imgur.com/wcKiPe1.png) Alternatively more information can be found in
<https://doc.pfsense.org/index.php/Installing_FreeBSD_Packages>, for achieving the same via the command line and
scripts.

If you experience an issue with `/usr/bin/install` being absent in pfSense 2.3 or earlier, update pfSense or use a
workaround from <https://redmine.pfsense.org/issues/6643>  

**Note:** In pfSense, the Netdata configuration files are located under `/usr/local/etc/netdata`.


