<!--
title: "timex.plugin"
description: "Monitor the system clock synchronization state."
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/timex.plugin/README.md
-->

# timex.plugin

This plugin monitors the system clock synchronization state on Linux nodes.

This plugin creates two charts:

-   System Clock Synchronization State
-   Computed Time Offset Between Local System and Reference Clock

## configuration

```
[plugin:timex]
    # update every = 1
    # clock synchronization state = yes
    # time offset = yes
```

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Ftimex.plugin%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
