<!--
title: "Netdata Functions"
sidebar_label: "Netdata Functions"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/cloud/netdata-functions.md"
sidebar_position: "2800"
learn_status: "Published"
learn_topic_type: "Concepts"
learn_rel_path: "Concepts"
learn_docs_purpose: "Present the Netdata Functions what these are and why they should be used."
-->

# Netdata Functions

Netdata Agent collectors are able to expose functions that can be executed in run-time and on-demand. These will be
executed on the node - host where the function is made
available.

#### What is a function?

Collectors besides the metric collection, storing, and/or streaming work are capable of executing specific routines on
request. These routines will bring additional information
to help you troubleshoot or even trigger some action to happen on the node itself.

A function is a  `key`  -  `value`  pair. The  `key`  uniquely identifies the function within a node. The  `value`  is a
function (i.e. code) to be run by a data collector when
the function is invoked.

For more details please check out documentation on how we use our internal collector to get this from the first collector that exposes
functions - [plugins.d](https://github.com/netdata/netdata/blob/master/collectors/plugins.d/README.md#function).

#### What functions are currently available?

| Function           | Description                                                                                                                                                    | Alternative to CLI tools        | Require Cloud | plugin - module                                                                                            |
|:-------------------|:---------------------------------------------------------------------------------------------------------------------------------------------------------------|---------------------------------|:--------------|:-----------------------------------------------------------------------------------------------------------|
| Block-devices      | Disk I/O activity for all block devices, offering insights into both data transfer volume and operation performance.                                           | `iostat`                        | no            | [proc](https://github.com/netdata/netdata/tree/master/collectors/proc.plugin#readme)                       |
| Containers-vms     | Insights into the resource utilization of containers and QEMU virtual machines: CPU usage, memory consumption, disk I/O, and network traffic.                  | `docker stats`, `systemd-cgtop` | no            | [cgroups](https://github.com/netdata/netdata/tree/master/collectors/cgroups.plugin#readme)                 |
| Ipmi-sensors       | Readings and status of IPMI sensors.                                                                                                                           | `ipmi-sensors`                  | no            | [freeipmi](https://github.com/netdata/netdata/tree/master/collectors/freeipmi.plugin#readme)               |
| Mount-points       | Disk usage for each mount point, including used and available space, both in terms of percentage and actual bytes, as well as used and available inode counts. | `df`                            | no            | [diskspace](https://github.com/netdata/netdata/tree/master/collectors/diskspace.plugin#readme)             |
| Network-interfaces | Network traffic, packet drop rates, interface states, MTU, speed, and duplex mode for all network interfaces.                                                  | `bmon`, `bwm-ng`                | no            | [proc](https://github.com/netdata/netdata/tree/master/collectors/proc.plugin#readme)                       |
| Processes          | Real-time information about the system's resource usage, including CPU utilization, memory consumption, and disk IO for every running process.                 | `top`, `htop`                   | yes           | [apps](https://github.com/netdata/netdata/blob/master/collectors/apps.plugin/README.md)                    |
| Systemd-journal    | Viewing, exploring and analyzing systemd journal logs.                                                                                                         | `journalctl`                    | yes           | [systemd-journal](https://github.com/netdata/netdata/tree/master/collectors/systemd-journal.plugin#readme) |
| Systemd-list-units | Information about all systemd units, including their active state, description, whether or not they are enabled, and more.                                     | `systemctl list-units`          | yes           | [systemd-journal](https://github.com/netdata/netdata/tree/master/collectors/systemd-journal.plugin#readme) |
| Systemd-services   | System resource utilization for all running systemd services: CPU, memory, and disk IO.                                                                        | `systemd-cgtop`                 | no            | [cgroups](https://github.com/netdata/netdata/tree/master/collectors/cgroups.plugin#readme)                 |
| Streaming          | Comprehensive overview of all Netdata children instances, offering detailed information about their status, replication completion time, and many more.        |                                 | yes           |                                                                                                            |


If you have ideas or requests for other functions:
* Participate in the relevant [GitHub discussion](https://github.com/netdata/netdata/discussions/14412)
* Open a [feature request](https://github.com/netdata/netdata-cloud/issues/new?assignees=&labels=feature+request%2Cneeds+triage&template=FEAT_REQUEST.yml&title=%5BFeat%5D%3A+) on Netdata Cloud repo
* Join the Netdata community on [Discord](https://discord.com/invite/mPZ6WZKKG2) and let us know.

#### How do functions work with streaming?

Via streaming, the definitions of functions are transmitted to a parent node, so it knows all the functions available on
any children connected to it.

If the parent node is the one connected to Netdata Cloud it is capable of triggering the call to the respective children
node to run the function.

#### Why are some functions only available on Netdata Cloud?

Since these functions are able to execute routines on the node and due to the potential use cases that they can cover, our
concern is to ensure no sensitive information or disruptive actions are exposed through the Agent's API.

With the communication between the Netdata Agent and Netdata Cloud being
through [ACLK](https://github.com/netdata/netdata/blob/master/aclk/README.md) this
concern is addressed.

## Related Topics

### **Related Concepts**

- [ACLK](https://github.com/netdata/netdata/blob/master/aclk/README.md)
- [plugins.d](https://github.com/netdata/netdata/blob/master/collectors/plugins.d/README.md)

### Related Tasks

- [Run-time troubleshooting with Functions](https://github.com/netdata/netdata/blob/master/docs/cloud/runtime-troubleshooting-with-functions.md)
