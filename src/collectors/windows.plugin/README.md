# Windows.plugin

This internal plugin is exclusively available for Microsoft Windows operating systems.

## Overview

The Windows plugin primarily collects metrics from Microsoft Windows [Performance Counters](https://learn.microsoft.com/en-us/windows/win32/perfctrs/performance-counters-what-s-new). All detected metrics are automatically displayed in the Netdata dashboard without requiring additional configuration.

## Default Configuration

By default, all collector threads are enabled except for `PerflibThermalZone`. `PerflibSMB` is also enabled by default; on systems where the "SMB Server Shares" object is not available, no SMB metrics are collected. You can enable disabled collectors or disable any of the currently active ones by modifying the `[plugin:windows]` section in your configuration file.

To change a setting, remove the comment symbol (`#`) from the beginning of the line and set the value to either `yes` or `no`.

```text
[plugin:windows]
        # GetSystemUptime = yes
        # GetSystemRAM = yes
        # GetPowerSupply = yes
        # GetSensors = yes
        # GetHardwareInfo = yes
        # PerflibServices = yes
        # PerflibProcesses = yes
        # PerflibProcessor = yes
        # PerflibMemory = yes
        # PerflibStorage = yes
        # PerflibNetwork = yes
        # PerflibSMB = yes
        # PerflibTerminalServices = yes
        # PerflibDNS = yes
        # PerflibObjects = yes
        # PerflibHyperV = yes
        # PerflibThermalZone = no
        # PerflibWebService = yes
        # PerflibHttpService = yes
        # PerflibNetFramework = yes
        # PerflibAD = yes
        # PerflibADCS = yes
        # PerflibADFS = yes
        # PerflibExchange = yes
        # PerflibMSMQ = yes
        # PerflibNUMA = yes
        # PerflibASP = yes
```

## Update Every per Thread

When the plugin is running, most threads will collect data using Netdata's default update `every interval`. However,
to avoid overloading the host, Netdata uses different `update every` intervals for specific threads, as shown below:

| Period (seconds) | Threads                                                                             |
|------------------|-------------------------------------------------------------------------------------|
| 5                | `PerflibHyperV`, `PerflibThermalZone`                                               |
| 10               | `GetHardwareInfo`, `PerflibAD`, `PerflibADCS`, `PerflibADFS`, and `PerflibExchange` |
| 30               | `PerflibServices`                                                                   |

To customize the update interval for a specific thread, you can set the update every value within the corresponding
thread configuration in your `netdata.conf` file. For example, to modify the intervals for the `Object`
and `HyperV` threads:

```text
[plugin:windows:PerflibObjects]
        #update every = 10s

[plugin:windows:PerflibHyperV]
        #update every = 15s
```

## PerflibMSMQ

`PerflibMSMQ` collects queue and journal message/byte counts. By default, it charts up to 100 queues individually and
sums the remaining queues into an `Other (aggregated)` series. Configure it in `[plugin:windows:PerflibMSMQ]`:

```ini
[plugin:windows:PerflibMSMQ]
    queues to chart individually = *
    max individually charted queues = 100
```

The selector accepts comma or pipe separated case-insensitive simple patterns. Windows queue paths can be pasted
verbatim; prefix a comma or pipe with `\` to match that character literally. A leading `\!` matches a literal `!`
instead of negating the pattern. The selector treats `*` as a wildcard; backslashes elsewhere are treated as Windows
path separators. Matching queue names are selected in case-insensitive alphabetical order; the cap may be set to `0`
to aggregate all queues.
