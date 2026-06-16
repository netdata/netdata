# Windows.plugin

This internal plugin is exclusively available for Microsoft Windows operating systems.

## Overview

The Windows plugin primarily collects metrics from Microsoft Windows [Performance Counters](https://learn.microsoft.com/en-us/windows/win32/perfctrs/performance-counters-what-s-new). All detected metrics are automatically displayed in the Netdata dashboard without requiring additional configuration.

## Default Configuration

By default, all collector threads are enabled except for `PerflibSMB` and `PerflibThermalZone`. You can enable these disabled collectors or disable any of the currently active ones by modifying the `[plugin:windows]` section in your configuration file.

To change a setting, remove the comment symbol (`#`) from the beginning of the line and set the value to either `yes` or `no`.

> **Note**
>
> The `[plugin:windows]` section is generated dynamically by the Agent and appears in `netdata.conf` **only on Windows hosts**, because `windows.plugin` runs exclusively on Microsoft Windows. On non-Windows systems (for example Linux) this section is not present in `netdata.conf` at all — its absence there is expected and is not an error. On Windows, the complete live configuration (including every `[plugin:windows]` key with its current default) is available at `http://localhost:19999/netdata.conf`; copy it from there or add the `[plugin:windows]` section manually to your `netdata.conf`. Every option listed below is a valid key.

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
        # PerflibSMB = no
        # PerflibObjects = yes
        # PerflibHyperV = yes
        # PerflibThermalZone = no
        # PerflibWebService = yes
        # PerflibNetFramework = yes
        # PerflibAD = yes
        # PerflibADCS = yes
        # PerflibADFS = yes
        # PerflibExchange = yes
        # PerflibNUMA = yes
        # PerflibASP = yes
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
