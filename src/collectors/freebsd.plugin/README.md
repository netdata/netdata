# FreeBSD system metrics (freebsd.plugin)

Collects resource usage and performance data on FreeBSD systems

By default, Netdata will enable monitoring metrics for disks, memory, and network only when they are not zero. If they are constantly zero they are ignored. Metrics that will start having values, after Netdata is started, will be detected and charts will be automatically added to the dashboard (a refresh of the dashboard is needed for them to appear though). Use `yes` instead of `auto` in plugin configuration sections to enable these charts permanently. You can also set the `enable zero metrics` option to `yes` in the `[global]` section which enables charts with zero metrics for all internal Netdata plugins.
