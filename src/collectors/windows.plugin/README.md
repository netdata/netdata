# Windows.plugin

This internal plugin is only available for Microsoft Windows operating systems.

## The Collector

This plugin primarily collects metrics from Microsoft Windows [Performance Counters](https://learn.microsoft.com/en-us/windows/win32/perfctrs/performance-counters-what-s-new). All detected metrics are automatically displayed without requiring additional configuration.

## Configuration File

The Windows plugin doesn't use a separate configuration file. Instead, configure it through Netdata's main configuration file located at:
`C:\Program Files\Netdata\etc\netdata\netdata.conf`.

### Enabling or Disabling Collection

By default, all collector threads are enabled except for `PerflibThermalZone` and `PerflibServices`. You can enable these or disable others by modifying options in the `[plugin:windows]` section of the configuration file.

To change a setting, remove the comment symbol (`#`) from the beginning of the line and set the value to either `yes` or `no`:

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

