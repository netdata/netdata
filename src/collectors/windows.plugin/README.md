# Windows.plugin

This internal plugin is exclusively available for Microsoft Windows operating systems.

## Overview

The Windows plugin primarily collects metrics from Microsoft Windows [Performance Counters](https://learn.microsoft.com/en-us/windows/win32/perfctrs/performance-counters-what-s-new). All detected metrics are automatically displayed in the Netdata dashboard without requiring additional configuration.

## Default Configuration

By default, all collector threads are enabled except for `PerflibThermalZone` and `GetPrinters`. `PerflibSMB` is also enabled by default; on systems where the "SMB Server Shares" object is not available, no SMB server metrics are collected. `PerflibSMBClient` is enabled by default and charts the remote SMB shares the host is connected to. You can enable disabled collectors or disable any of the currently active ones by modifying the `[plugin:windows]` section in your configuration file.

To change a setting, remove the comment symbol (`#`) from the beginning of the line and set the value to either `yes` or `no`.

```text
[plugin:windows]
        # GetSystemUptime = yes
        # GetSystemRAM = yes
        # GetPowerSupply = yes
        # GetSensors = yes
        # GetHardwareInfo = yes
        # PerflibServices = yes
        # GetPrinters = no
        # PerflibProcesses = yes
        # PerflibProcessor = yes
        # PerflibMemory = yes
        # PerflibPagingFile = yes
        # PerflibStorage = yes
        # PerflibNetwork = yes
        # PerflibSMB = yes
        # PerflibSMBClient = yes
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

| Period (seconds) | Threads                                                                                            |
|------------------|----------------------------------------------------------------------------------------------------|
| 5                | `PerflibHyperV`, `PerflibThermalZone`                                                              |
| 10               | `GetHardwareInfo`, `GetPrinters`, `PerflibAD`, `PerflibADCS`, `PerflibADFS`, and `PerflibExchange` |
| 30               | `PerflibServices`                                                                                  |

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
to aggregate all queues. A negative-only selector such as `!*temp*` selects no queues for individual charts, so all
queues go to `Other`; an empty selector has the same result. Append `*` (`!*temp*,*`) to select every queue except
those matching `*temp*`.

Perflib may not report a newly created queue until it becomes active, such as after a message is sent. Selectors match
the raw Perflib instance name. The `queue` chart label is sanitized for display; for example,
`host\private$\name` appears as `host/private_/name`. Use the original queue path in the selector rather than copying
the displayed label.

## GetPrinters

`GetPrinters` reads local printers and printer connections from the Windows print spooler. It lists printer names
without contacting print servers (`EnumPrintersW` level 4), then opens only the selected printers to read their status
and queue (`GetPrinterW`, and `EnumJobsW` for non-empty queues). It is disabled by default. Enable it in `[plugin:windows]` with `GetPrinters = yes`
and configure it in `[plugin:windows:GetPrinters]`:

```ini
[plugin:windows:GetPrinters]
    update every = 10s
    printers to monitor = *
    max monitored printers = 100
```

The selector uses the same syntax as the MSMQ selector above; printer connections such as `\\server\printer` can be
pasted verbatim. Matching printers are selected in case-insensitive alphabetical order up to the cap; unselected
printers get no charts and their queues are not opened.

Each selected printer gets four charts: queued jobs, printer status, printer problems, and jobs by status. The problems
chart has one dimension per Windows problem flag (for example `offline`, `paper_jam`, `toner_low`), each `1` while the
flag is set, so several can be active together and each can be alerted on.

Status values match the `Win32_Printer` and `Win32_PrintJob` mappings used by Windows Exporter, with these exceptions:

- A printer reporting several status flags at once gets its most severe state (problems such as offline or paper out
  first, then unknown, printing, warmup and idle) instead of falling back to its jobs.
- A printer connection whose print server is offline reports `other`; a printer whose status the server reports as
  unknown reports `unknown`.
- A job with no status flags is waiting its turn and counts as `queued`; a job the driver cannot print (blocked) or one
  waiting for user intervention counts as `degraded`.
- A printer with no status flags whose first queued job is just waiting reports `idle`; a growing backlog shows in the
  queued jobs chart.

An offline printer reports `other`. The printer state comes from the printer's status flags or, when none of them is
mapped, from its first queued job, so the charts update only when that data can be read:

- If a printer cannot be opened or its status cannot be read (for example, its print server is unreachable), all four
  of its charts have a gap for that cycle.
- If only its job list cannot be read, the queued jobs and problems charts still update, and the status and jobs by
  status charts have a gap.

The spooler returns the job owner, submitting machine, and document title with each job. The module reads only the job
status; those fields stay only in the in-memory reply buffer, which is overwritten by the next read, and are never
logged, charted, or exported.

The module always runs on its own thread, so a slow spooler call never delays other modules; if that thread cannot be
created, the module stays disabled. Opening a selected printer connection whose print server is unreachable can block;
while it is blocked, this module collects nothing and Netdata shutdown waits for the call to return. Exclude such
connections with `printers to monitor` (for example `!\\oldserver\*,*`); excluded printers are never opened. If the
spooler cannot be enumerated (for example, the Spooler service is stopped), the charts show gaps and are kept until a
successful enumeration shows the printer is gone.
