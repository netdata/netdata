<!--
---
title: "Spec to build Netdata RPM for sles 11"
custom_edit_url: https://github.com/netdata/netdata/edit/master/contrib/sles11/README.md
---
-->

# Spec to build Netdata RPM for sles 11

Based on [openSUSE rpm spec](https://build.opensuse.org/package/show/network/netdata) with some 
changes and additions for sles 11 backport, namely:

-   init.d script 
-   run-time dependency on python ordereddict backport
-   patch for Netdata python.d plugin to work with older python
-   crude hack of notification script to work with bash 3 (email and syslog only, one destination,
    see comments at the top)

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcontrib%2Fsles11%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
