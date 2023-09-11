<!--
title: "Install Netdata on pfSense"
description: "Install Netdata on pfSense to monitor the health and performance of firewalls with thousands of real-time, per-second metrics."
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/methods/pfsense.md
sidebar_label: "pfSense"
learn_status: "Published"
learn_rel_path: "Installation/Install on specific environments"
-->

# Install Netdata on pfSense CE

> 💡 This document is maintained by Netdata's community, and may not be completely up-to-date. Please double-check the
> details of the installation process, such as version numbers for downloadable packages, before proceeding.
>
> You can help improve this document by [submitting a
> PR](https://github.com/netdata/netdata/edit/master/packaging/installer/methods/pfsense.md) with your recommended
> improvements or changes. Thank you!

## Install prerequisites/dependencies

To install Netdata on pfSense, first enable the [FreeBSD package repo](https://docs.netgate.com/pfsense/en/latest/recipes/freebsd-pkg-repo.html)
Then run the following command (within a shell or under the **Diagnostics/Command**
prompt within the pfSense web interface).

```bash
pkg install -y pkgconf bash e2fsprogs-libuuid libuv nano
```

Then run the following commands to download various dependencies from the FreeBSD repository.

```sh
pkg install json-c-0.15_1
pkg install py39-certifi-2023.5.7
pkg install py39-asn1crypto
pkg install py39-pycparser
pkg install py39-cffi
pkg install py39-six
pkg install py39-cryptography
pkg install py39-idna
pkg install py39-openssl
pkg install py39-pysocks
pkg install py39-urllib3
pkg install py39-yaml
```

> ⚠️ If any of the above commands return a `Not Found` error, you need to manually search for the latest package in the
> [FreeBSD repository](https://www.freebsd.org/ports/) or by running `pkg search`. Search for the package's name, such as `py37-cffi`, find the
> latest version number, and update the command accordingly.

> ⚠️ On pfSense 2.4.5, Python version 3.7 may be installed by the system, in which case you should should not install
> Python from the FreeBSD repository as instructed above.

> ⚠️ If you are using the `apcupsd` collector, you need to make sure that apcupsd is up before starting Netdata.
> Otherwise a infinitely running `cat` process triggered by the default activated apcupsd charts plugin will eat up CPU
> and RAM (`/tmp/.netdata-charts.d-*/run-*`). This also applies to `OPNsense`.

## Install Netdata

You can now install Netdata from the FreeBSD repository.

```bash
pkg install netdata
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


