# Windows.plugin

This is an internal plugin available only for Microsoft Windows operating systems.

## The Collector

Most of the metrics collected by this plugin originate from Microsoft Windows
[Performance Counters](https://learn.microsoft.com/en-us/windows/win32/perfctrs/performance-counters-what-s-new).
These metrics are always displayed when detected, requiring no additional configuration.

## Configuration File

The Windows plugin does not have a separate configuration file. To configure it, use Netdata’s default configuration file:
`C:\Program Files\Netdata\etc\netdata\netdata.conf`.

### Enabling or Disabling Collection

With the exception of the `PerflibThermalZone` and `PerflibServices` threads, all other threads are enabled by default.
You can enable it or disable others by modifying a specific option for a thread inside the `[plugin:windows]` section.
To do this, remove the comment symbol (`#`) at the beginning of the line and then change the boolean value after the equals sign.

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
```

