<!--
---
title: "freebsd.plugin"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/freebsd.plugin/README.md
---
-->

# freebsd.plugin

Collects resource usage and performance data on Free Berkeley Software Distribution (FreeBSD) systems.

By default, Netdata enables monitoring metrics for disks, memory, and network only when they are not zero. If they are always zero, they get ignored. Metrics that start having values, after Netdata starts, get detected, and charts get added to the dashboard automatically. A refresh of the dashboard is needed for them to appear though. In plugin configuration sections, use `yes` instead of `auto` to enable these charts permanently. You can also set the `enable zero metrics` option to `yes` in the `[global]` section, which enables charts with zero metrics for all internal Netdata plugins.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Ffreebsd.plugin%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
