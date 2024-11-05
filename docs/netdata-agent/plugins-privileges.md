# Netdata Plugin Privileges

Netdata uses various plugins and helper binaries that require elevated privileges to collect system metrics.
This document outlines the required privileges and how they are configured in different environments.

## Privileges

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

## File Permissions and Ownership

To ensure security, all plugin and helper binary files have the following permissions and ownership:

- **Ownership**: `root:netdata`.
- **Permissions**: `0750` (for non-setuid binaries) or `4750` (for setuid binaries).

This configuration limits access to the files to the `netdata` user and the `root` user, while allowing execution by the `netdata` user.
