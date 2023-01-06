<!--
title: "Collect system metrics with Netdata"
sidebar_label: "System metrics"
description: "Netdata collects thousands of metrics from physical and virtual systems, IoT/edge devices, and containers with zero configuration."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/collect/system-metrics.md
-->

# Collect system metrics with Netdata

Netdata collects thousands of metrics directly from the operating systems of physical and virtual systems, IoT/edge
devices, and [containers](/docs/collect/container-metrics.md) with zero configuration.

To gather system metrics, Netdata uses roughly a dozen plugins, each of which has one or more collectors for very
specific metrics exposed by the host. The system metrics Netdata users interact with most for health monitoring and
performance troubleshooting are collected and visualized by `proc.plugin`, `cgroups.plugin`, and `ebpf.plugin`.

[**proc.plugin**](/collectors/proc.plugin/README.md) gathers metrics from the `/proc` and `/sys` folders in Linux
systems, along with a few other endpoints, and is responsible for the bulk of the system metrics collected and
visualized by Netdata. It collects CPU, memory, disks, load, networking, mount points, and more with zero configuration.
It even allows Netdata to monitor its own resource utilization!

[**cgroups.plugin**](/collectors/cgroups.plugin/README.md) collects rich metrics about containers and virtual machines
using the virtual files under `/sys/fs/cgroup`. By reading cgroups, Netdata can instantly collect resource utilization
metrics for systemd services, all containers (Docker, LXC, LXD, Libvirt, systemd-nspawn), and more. Learn more in the
[collecting container metrics](/docs/collect/container-metrics.md) doc.

[**ebpf.plugin**](/collectors/ebpf.plugin/README.md): Netdata's extended Berkeley Packet Filter (eBPF) collector
monitors Linux kernel-level metrics for file descriptors, virtual filesystem IO, and process management. You can use our
eBPF collector to analyze how and when a process accesses files, when it makes system calls, whether it leaks memory or
creating zombie processes, and more.

While the above plugins and associated collectors are the most important for system metrics, there are many others. You
can find all system collectors in our [supported collectors list](/collectors/COLLECTORS.md#system-collectors).

## Collect Windows system metrics

Netdata is also capable of monitoring Windows systems. The [WMI
collector](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/wmi) integrates with
[windows_exporter](https://github.com/prometheus-community/windows_exporter), a small Go-based binary that you can run
on Windows systems. The WMI collector then gathers metrics from an endpoint created by windows_exporter, for more
details see [the requirements](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/wmi#requirements).

Next, [configure the WMI
collector](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/wmi#configuration) to point to the URL
and port of your exposed endpoint. Restart Netdata with `sudo systemctl restart netdata`, or the [appropriate
method](/docs/configure/start-stop-restart.md) for your system. You'll start seeing Windows system metrics, such as CPU
utilization, memory, bandwidth per NIC, number of processes, and much more.

For information about collecting metrics from applications _running on Windows systems_, see the [application metrics
doc](/docs/collect/application-metrics.md#collect-metrics-from-applications-running-on-windows).

## What's next?

Because there's some overlap between system metrics and [container metrics](/docs/collect/container-metrics.md), you
should investigate Netdata's container compatibility if you use them heavily in your infrastructure.

If you don't use containers, skip ahead to collecting [application metrics](/docs/collect/application-metrics.md) with
Netdata.


