# Top Monitoring (Netdata Functions)

Netdata Agent collectors are able to expose functions that can be executed in run-time and on-demand. These will be
executed on the node/host where the function is made available.

## What is a function?

Collectors besides the metric collection, storing, and/or streaming work are capable of executing specific routines on request. These routines will bring additional information to help you troubleshoot or even trigger some action to happen on the node itself.

For more details please check out documentation on how we use our internal collector to get this from the first collector that exposes functions - [plugins.d](https://github.com/netdata/netdata/blob/master/src/collectors/plugins.d/README.md#function).

## Prerequisites

The following is required to be able to run Functions from Netdata Cloud.

- At least one of the nodes claimed to your Space should be on a Netdata agent version higher than `v1.37.1`
- Ensure that the node has the collector that exposes the function you want enabled

## What functions are currently available?

| Function           | Description                                                                                                                                                    | Alternative to CLI tools        | Require Cloud | plugin - module                                                                                                |
|:-------------------|:---------------------------------------------------------------------------------------------------------------------------------------------------------------|---------------------------------|:--------------|:---------------------------------------------------------------------------------------------------------------|
| Block-devices      | Disk I/O activity for all block devices, offering insights into both data transfer volume and operation performance.                                           | `iostat`                        | no            | [proc](https://github.com/netdata/netdata/tree/master/src/collectors/proc.plugin#readme)                       |
| Containers-vms     | Insights into the resource utilization of containers and QEMU virtual machines: CPU usage, memory consumption, disk I/O, and network traffic.                  | `docker stats`, `systemd-cgtop` | no            | [cgroups](https://github.com/netdata/netdata/tree/master/src/collectors/cgroups.plugin#readme)                 |
| Ipmi-sensors       | Readings and status of IPMI sensors.                                                                                                                           | `ipmi-sensors`                  | no            | [freeipmi](https://github.com/netdata/netdata/tree/master/src/collectors/freeipmi.plugin#readme)               |
| Mount-points       | Disk usage for each mount point, including used and available space, both in terms of percentage and actual bytes, as well as used and available inode counts. | `df`                            | no            | [diskspace](https://github.com/netdata/netdata/tree/master/src/collectors/diskspace.plugin#readme)             |
| Network-interfaces | Network traffic, packet drop rates, interface states, MTU, speed, and duplex mode for all network interfaces.                                                  | `bmon`, `bwm-ng`                | no            | [proc](https://github.com/netdata/netdata/tree/master/src/collectors/proc.plugin#readme)                       |
| Processes          | Real-time information about the system's resource usage, including CPU utilization, memory consumption, and disk IO for every running process.                 | `top`, `htop`                   | yes           | [apps](https://github.com/netdata/netdata/blob/master/src/collectors/apps.plugin/README.md)                    |
| Systemd-journal    | Viewing, exploring and analyzing systemd journal logs.                                                                                                         | `journalctl`                    | yes           | [systemd-journal](https://github.com/netdata/netdata/tree/master/src/collectors/systemd-journal.plugin#readme) |
| Systemd-list-units | Information about all systemd units, including their active state, description, whether or not they are enabled, and more.                                     | `systemctl list-units`          | yes           | [systemd-journal](https://github.com/netdata/netdata/tree/master/src/collectors/systemd-journal.plugin#readme) |
| Systemd-services   | System resource utilization for all running systemd services: CPU, memory, and disk IO.                                                                        | `systemd-cgtop`                 | no            | [cgroups](https://github.com/netdata/netdata/tree/master/src/collectors/cgroups.plugin#readme)                 |
| Streaming          | Comprehensive overview of all Netdata children instances, offering detailed information about their status, replication completion time, and many more.        |                                 | yes           |                                                                                                                |
| Netdata-api-calls  | Real-time tracing of API calls made to the Netdata Agent. It provides information on query, source, status, elapsed time, and more.                            |                                 | yes           |                                                                                                                |

## How do functions work with streaming?

Via streaming, the definitions of functions are transmitted to a parent node, so it knows all the functions available on any children connected to it. If the parent node is the one connected to Netdata Cloud it is capable of triggering the call to the respective children node to run the function.

## Why are some functions only available on Netdata Cloud?

Since these functions are able to execute routines on the node and due to the potential use cases that they can cover, our concern is to ensure no sensitive information or disruptive actions are exposed through the Agent's API.

With the communication between the Netdata Agent and Netdata Cloud being through [ACLK](https://github.com/netdata/netdata/blob/master/src/aclk/README.md) this concern is addressed.

## Feedback

If you have ideas or requests for other functions:

- Participate in the relevant [GitHub discussion](https://github.com/netdata/netdata/discussions/14412)
- Open a [feature request](https://github.com/netdata/netdata-cloud/issues/new?assignees=&labels=feature+request%2Cneeds+triage&template=FEAT_REQUEST.yml&title=%5BFeat%5D%3A+) on Netdata Cloud repo
- Join the Netdata community on [Discord](https://discord.com/invite/2mEmfW735j) and let us know.
