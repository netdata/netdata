# Top Consumers

Netdata Agent collectors provide on-demand, runtime executable functions on the host where they are deployed. Available since v1.37.1.

## What is a function?

Beyond their primary roles of collecting metrics, collectors can execute specific routines when requested. These routines provide additional diagnostic information or trigger actions directly on the host node.

## What functions are currently available?

| Function            | Description                                                                                                                                                    | Alternative to CLI tools        | Require Cloud | plugin - module                                                                                                |
|:--------------------|:---------------------------------------------------------------------------------------------------------------------------------------------------------------|---------------------------------|:--------------|:---------------------------------------------------------------------------------------------------------------|
| Block-devices       | Disk I/O activity for all block devices, offering insights into both data transfer volume and operation performance.                                           | `iostat`                        | no            | [proc](https://github.com/netdata/netdata/tree/master/src/collectors/proc.plugin#readme)                       |
| Containers-vms      | Insights into the resource utilization of containers and QEMU virtual machines: CPU usage, memory consumption, disk I/O, and network traffic.                  | `docker stats`, `systemd-cgtop` | no            | [cgroups](https://github.com/netdata/netdata/tree/master/src/collectors/cgroups.plugin#readme)                 |
| Ipmi-sensors        | Readings and status of IPMI sensors.                                                                                                                           | `ipmi-sensors`                  | no            | [freeipmi](https://github.com/netdata/netdata/tree/master/src/collectors/freeipmi.plugin#readme)               |
| Mount-points        | Disk usage for each mount point, including used and available space, both in terms of percentage and actual bytes, as well as used and available inode counts. | `df`                            | no            | [diskspace](https://github.com/netdata/netdata/tree/master/src/collectors/diskspace.plugin#readme)             |
| Network-connections | Real-time monitoring of all network connections, showing established connections, ports, protocols, and connection states across TCP/UDP services.             | `netstat`, `ss`                 | yes           | [network-viewer](https://github.com/netdata/netdata/tree/master/src/collectors/network-viewer.plugin)          |
| Network-interfaces  | Network traffic, packet drop rates, interface states, MTU, speed, and duplex mode for all network interfaces.                                                  | `bmon`, `bwm-ng`                | no            | [proc](https://github.com/netdata/netdata/tree/master/src/collectors/proc.plugin#readme)                       |
| Processes           | Real-time information about the system's resource usage, including CPU utilization, memory consumption, and disk IO for every running process.                 | `top`, `htop`                   | yes           | [apps](/src/collectors/apps.plugin/README.md)                                                                  |
| Systemd-journal     | Viewing, exploring and analyzing systemd journal logs.                                                                                                         | `journalctl`                    | yes           | [systemd-journal](https://github.com/netdata/netdata/tree/master/src/collectors/systemd-journal.plugin#readme) |
| Systemd-list-units  | Information about all systemd units, including their active state, description, whether or not they are enabled, and more.                                     | `systemctl list-units`          | yes           | [systemd-journal](https://github.com/netdata/netdata/tree/master/src/collectors/systemd-journal.plugin#readme) |
| Systemd-services    | System resource utilization for all running systemd services: CPU, memory, and disk IO.                                                                        | `systemd-cgtop`                 | no            | [cgroups](https://github.com/netdata/netdata/tree/master/src/collectors/cgroups.plugin#readme)                 |
| Netdata-api-calls   | Real-time tracing of API calls made to the Netdata Agent. It provides information on query, source, status, elapsed time, and more.                            |                                 | yes           |                                                                                                                |
| Netdata-streaming   | Comprehensive overview of all Netdata children instances, offering detailed information about their status, replication completion time, and many more.        |                                 | yes           |                                                                                                                |

## How do functions work with streaming?

When streaming is enabled, function definitions propagate from Child nodes to their Parent node. If this Parent node is connected to Netdata Cloud, it can trigger function execution on any of its connected Child nodes.

## Why are some functions only available on Netdata Cloud?

Some functions are exclusively available through Netdata Cloud for security reasons. Since functions can execute node-level routines that may access sensitive information, we restrict their exposure through the Agent's API. This security concern is addressed by our [ACLK](/src/aclk/README.md) protocol, which provides secure communication between Netdata Agent and Netdata Cloud.

## Feedback

If you have ideas or requests for other functions:

- Participate in the relevant [GitHub discussion](https://github.com/netdata/netdata/discussions/14412)
- Open a [feature request](https://github.com/netdata/netdata-cloud/issues/new?assignees=&labels=feature+request%2Cneeds+triage&template=FEAT_REQUEST.yml&title=%5BFeat%5D%3A+) on Netdata Cloud repo
- Join the Netdata community on [Discord](https://discord.com/invite/2mEmfW735j) and let us know.
