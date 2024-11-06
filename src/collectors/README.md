# Collectors

When Netdata starts, and with zero configuration, it auto-detects thousands of data sources and immediately collects
per-second metrics.

Netdata can immediately collect metrics from these endpoints thanks to 300+ **collectors**, which all come pre-installed
when you [install Netdata](/packaging/installer/README.md).

All collectors are **installed by default** with every installation of Netdata. You do not need to install
collectors manually to collect metrics from new sources.
See how you can [monitor anything with Netdata](/src/collectors/COLLECTORS.md).

Upon startup, Netdata will **auto-detect** any application or service that has a collector, as long as both the collector
and the app/service are configured correctly. If you don't see charts for your application, see
our [collectors' configuration reference](/src/collectors/REFERENCE.md).

## How Netdata's metrics collectors work

Every collector has two primary jobs:

- Look for exposed metrics at a pre- or user-defined endpoint.
- Gather exposed metrics and use additional logic to build meaningful, interactive visualizations.

If the collector finds compatible metrics exposed on the configured endpoint, it begins a per-second collection job. The
Netdata Agent gathers these metrics, sends them to the
[database engine for storage](/docs/netdata-agent/configuration/optimizing-metrics-database/change-metrics-storage.md)
, and immediately
[visualizes them meaningfully](/docs/dashboards-and-charts/netdata-charts.md)
on dashboards.

Each collector comes with a pre-defined configuration that matches the default setup for that application. This endpoint
can be a URL and port, a socket, a file, a web page, and more. The endpoint is user-configurable, as are many other
specifics of what a given collector does.

## Collector architecture and terminology

- **Collectors** are the processes/programs that actually gather metrics from various sources.

- **Plugins** help manage all the independent data collection processes in a variety of programming languages, based on
  their purpose and performance requirements. There are three types of plugins:

    - **Internal** plugins organize collectors that gather metrics from `/proc`, `/sys` and other Linux kernel sources.
      They are written in `C`, and run as threads within the Netdata daemon.

    - **External** plugins organize collectors that gather metrics from external processes, such as a MySQL database or
      Nginx web server. They can be written in any language, and the `netdata` daemon spawns them as long-running
      independent processes. They communicate with the daemon via pipes. All external plugins are managed by
      [plugins.d](/src/plugins.d/README.md), which provides additional management options.

- **Orchestrators** are external plugins that run and manage one or more modules. They run as independent processes.
  The Go orchestrator is in active development.

    - [go.d.plugin](/src/go/plugin/go.d/README.md): An orchestrator for data
      collection modules written in `go`.

    - [python.d.plugin](/src/collectors/python.d.plugin/README.md):
      An orchestrator for data collection modules written in `python` v2/v3.

    - [charts.d.plugin](/src/collectors/charts.d.plugin/README.md):
      An orchestrator for data collection modules written in`bash` v4+.

- **Modules** are the individual programs controlled by an orchestrator to collect data from a specific application, or type of endpoint.

## Netdata Plugin Privileges

Netdata uses various plugins and helper binaries that require elevated privileges to collect system metrics.
This section outlines the required privileges and how they are configured in different environments.

### Privileges

| Plugin/Binary          | Privileges (Linux)                              | Privileges (Non-Linux or Containerized Environment) |   
|------------------------|-------------------------------------------------|-----------------------------------------------------|
| apps.plugin            | CAP_DAC_READ_SEARCH, CAP_SYS_PTRACE             | setuid root                                         |
| debugfs.plugin         | CAP_DAC_READ_SEARCH                             | setuid root                                         |
| systemd-journal.plugin | CAP_DAC_READ_SEARCH                             | setuid root                                         |
| perf.plugin            | CAP_PERFMON                                     | setuid root                                         |
| slabinfo.plugin        | CAP_DAC_READ_SEARCH                             | setuid root                                         |
| go.d.plugin            | CAP_DAC_READ_SEARCH, CAP_NET_ADMIN, CAP_NET_RAW | setuid root                                         |
| freeipmi.plugin        | setuid root                                     | setuid root                                         |
| nfacct.plugin          | setuid root                                     | setuid root                                         |
| xenstat.plugin         | setuid root                                     | setuid root                                         |
| ioping                 | setuid root                                     | setuid root                                         |
| ebpf.plugin            | setuid root                                     | setuid root                                         |
| cgroup-network         | setuid root                                     | setuid root                                         |
| local-listeners        | setuid root                                     | setuid root                                         |
| network-viewer.plugin  | setuid root                                     | setuid root                                         |
| ndsudo                 | setuid root                                     | setuid root                                         |

**About ndsudo**:

`ndsudo` is a purpose-built privilege escalation utility for Netdata that executes a predefined set of commands with root privileges. Unlike traditional `sudo`, it operates with a [hard-coded list of allowed commands](https://github.com/netdata/netdata/blob/master/src/collectors/utils/ndsudo.c), providing better security through reduced scope and eliminating the need for `sudo` configuration.

It’s used by the `go.d.plugin` to collect data by executing certain binaries that require root access.

### File Permissions and Ownership

To ensure security, all plugin and helper binary files have the following permissions and ownership:

- **Ownership**: `root:netdata`.
- **Permissions**: `0750` (for non-setuid binaries) or `4750` (for setuid binaries).

This configuration limits access to the files to the `netdata` user and the `root` user, while allowing execution by the `netdata` user.
