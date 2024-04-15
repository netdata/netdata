<!--
title: "Network Viewer monitoring (network-viewer.plugin)"
sidebar_label: "Network Viewer monitoring "
custom_edit_url: "https://github.com/netdata/netdata/edit/master/src/collectors/network-viewer.plugin/README.md"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/System metrics"
-->

# Network Viewer monitoring (network-viewer.plugin)

The `network-viewer.plugin` provides users with information about TCP and UDP
resources.
It is enabled by default on every Netdata installation.

To accomplish this, it scans through the entire process tree, gathering socket
information for each process identified during its parsing of `/proc` or using
`libmnl`. Alternatively, it monitors calls to kernel internal functions when
utilizing `eBPF`.

## Functions

To visualize your network traffic, you'll need to log in to the Netdata cloud.
Navigate to the `Top` tab, select a specific host, and then click on
`Network connections`.

## Performance

The `network-viewer.plugin` is a sophisticated piece of software that requires
significant processing.

On Linux systems, the plugin reads multiple `network-viewer.plugin` reads
several `/proc` files for each running process to gather necessary data.
Performing this task every second, particularly on hosts with numerous
processes, can result in increased CPU utilization by the plugin.

To minimize the load caused by parsing `/proc` the plugin can utilize `eBPF`,
provided it's available on your host. However, this approach will introduce
additional overhead for each TCP and UDP function call on the host. To switch
from using `/proc` files to eBPF, adjustments to the configuration file are also
required:

```
[plugin:network-viewer]
	command options = ebpf
```

You can further customize the collection level by specifying the desired option
`apps-level`:

```
[plugin:network-viewer]
	command options = ebpf apps-level 1
```

The accepted values are as follows:
    - 0: Group data under the real parent
    - 1: Group data under the parent.

Lastly, you have the option to personalize whether the plugin will display data per parent PID,
which is the default behavior, or utilize per-thread information by enabling the `use-pid` option:

```
[plugin:network-viewer]
	command options = ebpf apps-level 1 use-pid
```
