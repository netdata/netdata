# Windows.plugin

This internal plugin is exclusively available for Microsoft Windows operating systems.

## Overview

The Windows plugin primarily collects metrics from Microsoft Windows [Performance Counters](https://learn.microsoft.com/en-us/windows/win32/perfctrs/performance-counters-what-s-new). All detected metrics are automatically displayed in the Netdata dashboard without requiring additional configuration.

## Default Configuration

By default, all collector threads are enabled except for `PerflibThermalZone` and `PerflibServices`. You can enable these disabled collectors or disable any of the currently active ones by modifying the `[plugin:windows]` section in your configuration file.

To change a setting, remove the comment symbol (`#`) from the beginning of the line and set the value to either `yes` or `no`.

```text
[plugin:windows]
        # GetSystemUptime = yes
        # GetSystemRAM = yes
        # PerflibProcesses = yes
        # PerflibProcessor = yes
        # PerflibMemory = yes
        # PerflibStorage = yes
        # PerflibNetwork = yes
        # PerflibObjects = yes
        # PerflibHyperV = yes
        # PerflibThermalZone = no
        # PerflibWebService = yes
        # PerflibServices = no
        # PerflibNetFramework = yes
        # PerflibAD = yes
        # PerflibADCS = yes
        # PerflibADFS = yes
        # PerflibExchange = yes
        # PerflibNUMA = yes
```

## Update Every per Thread

When the plugin is running, most threads will collect data using Netdata's default update `every interval`. However,
to avoid overloading the host, Netdata uses different `update every` intervals for specific threads, as shown below:

| Period (seconds) | Threads                                                                          |
|------------------|----------------------------------------------------------------------------------|
| 5                | `PerflibHyperV`, `PerflibThermalZone`                                            |
| 10               | `PerflibAD`, `PerflibADCS`, `PerflibADFS`, and `PerflibExchange` |
| 30               | `PerflibServices`                                                                |

To customize the update interval for a specific thread, you can set the update every value within the corresponding
thread configuration in your `netdata.conf` file. For example, to modify the intervals for the `Object`
and `HyperV` threads:

```text
[plugin:windows:PerflibObjects]
        #update every = 10s

[plugin:windows:PerflibHyperV]
        #update every = 15s
```
