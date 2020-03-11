<!--
---
title: "macos.plugin"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/macos.plugin/README.md
---
-->

# macos.plugin

Collects resource usage and performance data on MacOS systems

By default, Netdata will enable monitoring metrics for disks, memory, and network only when they are not zero. If they are constantly zero they are ignored. Metrics that will start having values, after Netdata is started, will be detected and charts will be automatically added to the dashboard (a refresh of the dashboard is needed for them to appear though). Use `yes` instead of `auto` in plugin configuration sections to enable these charts permanently. You can also set the `enable zero metrics` option to `yes` in the `[global]` section which enables charts with zero metrics for all internal Netdata plugins.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fmacos.plugin%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
