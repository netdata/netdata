# Collectors

Netdata automatically collects per-second metrics from thousands of data sources without any configuration:

- **Zero-touch setup**: All collectors are pre-installed, allowing you to start collecting detailed metrics right after Netdata starts.
- **Universal Monitoring**: Monitor virtually anything with Netdata's extensive collector library.

If you don't see charts for your application, check our collectors' [configuration reference](/src/collectors/REFERENCE.md) to ensure both the collector and your application are properly configured.

## Collector Types

Netdata's collectors are specialized data collection plugins that gather metrics from various sources. They are divided into two main categories:

| Type     | Description                                                           | Key Features                                                                                                                                                                                                               |
|----------|-----------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Internal | Native collectors that gather system-level metrics                    | • Written in `C` for optimal performance<br/>• Run as threads within Netdata daemon<br/>• Zero external dependencies<br/>• Minimal system overhead                                                                         |
| External | Modular collectors that gather metrics from applications and services | • Support multiple programming languages<br/>• Run as independent processes<br/>• Communicate via pipes with Netdata<br/>• Managed by [plugins.d](/src/plugins.d/README.md)<br/>• Examples: MySQL, Nginx, Redis collectors |

## Collector Privileges

Netdata uses various plugins and helper binaries that require elevated privileges to collect system metrics.
This section outlines the required privileges and how they are configured in different environments.

### Privileges

| Plugin/Binary          | Privileges (Linux)                              | Privileges (Non-Linux or Containerized Environment) |   
|------------------------|-------------------------------------------------|-----------------------------------------------------|
| apps.plugin            | CAP_DAC_READ_SEARCH, CAP_SYS_PTRACE             | setuid root                                         |
| debugfs.plugin         | CAP_DAC_READ_SEARCH                             | setuid root                                         |
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
