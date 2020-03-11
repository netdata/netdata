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
pkg install pkgconf
pkg install bash
pkg install e2fsprogs-libuuid
pkg install libuv
pkg add http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/Judy-1.0.5_2.txz
pkg add http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/python36-3.6.9.txz
ln -s /usr/local/lib/libjson-c.so /usr/local/lib/libjson-c.so.4
pkg add http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/netdata-1.17.1.txz
```

**Note:** If you receive a `Not Found` error during the last two commands above, you will either need to manually look
in the [repo folder](http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/) for the latest available package and use its
URL instead, or you can try manually changing the netdata version in the URL to the latest version.  

You must edit `/usr/local/etc/netdata/netdata.conf` and change `bind to = 127.0.0.1` to `bind to = 0.0.0.0`.

To start Netdata manually, run `service netdata onestart`  

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
