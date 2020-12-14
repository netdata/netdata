<!--
title: "Install Netdata on FreeNAS"
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/methods/freenas.md
-->

# Install Netdata on FreeNAS

On FreeNAS-Corral-RELEASE (>=10.0.3 and <11.3), Netdata is pre-installed.

To use Netdata, the service will need to be enabled and started from the FreeNAS [CLI](https://github.com/freenas/cli).

To enable the Netdata service:

```sh
service netdata config set enable=true
```

To start the Netdata service:

```sh
service netdata start
```

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fpackaging%2Finstaller%2Fmethods%2Ffreenas&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
