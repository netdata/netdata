<!--
title: "Spec to build Netdata RPM for sles 11"
custom_edit_url: https://github.com/netdata/netdata/edit/master/contrib/sles11/README.md
-->

# Spec to build Netdata RPM for sles 11

Based on [openSUSE rpm spec](https://build.opensuse.org/package/show/network/netdata) with some 
changes and additions for sles 11 backport, namely:

-   init.d script 
-   run-time dependency on python ordereddict backport
-   patch for Netdata python.d plugin to work with older python
-   crude hack of notification script to work with bash 3 (email and syslog only, one destination,
    see comments at the top)


