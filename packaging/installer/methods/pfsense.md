<!--
---
title: "Install Netdata on pfSense"
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/methods/pfsense.md
---
-->

# Install Netdata on pfSense

To install Netdata on pfSense, run the following commands (within a shell or under the **Diagnostics/Command** prompt
within the pfSense web interface).

Note that the first four packages are downloaded from the pfSense repository for maintaining compatibility with pfSense,
Netdata, Judy and Python are downloaded from the FreeBSD repository.

```sh
pkg install -y pkgconf bash e2fsprogs-libuuid libuv nano
pkg add http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/Judy-1.0.5_2.txz
pkg add http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/json-c-0.15_1.txz
pkg add http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/py37-certifi-2020.6.20.txz
pkg add http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/py37-asn1crypto-1.3.0.txz
pkg add http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/py37-pycparser-2.20.txz
pkg add http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/py37-cffi-1.14.3.txz
pkg add http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/py37-six-1.14.0.txz
pkg add http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/py37-cryptography-2.6.1.txz
pkg add http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/py37-idna-2.8.txz
pkg add http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/py37-openssl-19.0.0.txz
pkg add http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/py37-pysocks-1.7.1.txz
pkg add http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/py37-urllib3-1.25.7,1.txz
pkg add http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/py37-yaml-5.3.1.txz
pkg add http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/netdata-1.26.0.txz
```

**Note:** If you receive a `Not Found` error at any of the commands above, you will either need to manually look
in the [repo folder](http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/) for the latest available package and use its
URL instead, or you can try manually changing the Netdata version in the URL to the latest version
(in case Netdata is the only package erroring out).

**Note:** On pfSense 2.4.5, Python version 3.7 may be installed by the system, in which case you should should not install Python from the FreeBSD repository as instructed above.

**WARNING:** Packages from the `latest` tag in the FreeBSD 11 repository will always be at the latest stable train. As of early July 2020, that means `11.3-RELEASE`. If you are running earlier versions of pfSense, such as 2.4.4 at this time, then you'll be running `11.2-RELEASE`. Be mindful of your update train, and pay close attention to warnings from your package manager, as they should not be dealt with lightly.

if you are running an older version, you might need to manually browse for the correct package versions for your release train: [11.3-RELEASE](http://pkg.freebsd.org/FreeBSD:11:amd64/release_3/), [11.2-RELEASE](http://pkg.freebsd.org/FreeBSD:11:amd64/release_2/), [11.1-RELEASE](http://pkg.freebsd.org/FreeBSD:11:amd64/release_1/), and [11.0-RELEASE](http://pkg.freebsd.org/FreeBSD:11:amd64/release_0/).

You must edit `/usr/local/etc/netdata/netdata.conf` and change `bind to = 127.0.0.1` to `bind to = 0.0.0.0`.

To start Netdata manually, run `service netdata onestart`.

**Warning:** If you are using the `apcupsd` collector, you need to make sure that apcupsd is up before starting Netdata. Otherwise a infinitely running `cat` process triggered by the default activated apcuspd charts plugin will eat up CPU and RAM (`/tmp/.netdata-charts.d-*/run-*`). This also applies to `OPNsense`.

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
