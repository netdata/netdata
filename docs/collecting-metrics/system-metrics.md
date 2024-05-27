# Collect System Metrics with Netdata

Netdata collects thousands of metrics directly from the operating systems of physical and virtual machines, IoT/edge devices, and [containers](/docs/collecting-metrics/container-metrics.md) with zero configuration.

To gather system metrics, Netdata uses various plugins, each of which has one or more collectors for very specific metrics exposed by the host. The system metrics Netdata users interact with most for health monitoring and performance troubleshooting are collected and visualized by `proc.plugin`, `cgroups.plugin`, and `ebpf.plugin`.

[**proc.plugin**](/src/collectors/proc.plugin/README.md) gathers metrics from the `/proc` and `/sys` folders in Linux systems, along with a few other endpoints, and is responsible for the bulk of the system metrics collected and visualized by Netdata. It collects CPU, memory, disks, load, networking, mount points, and more with zero configuration. It also allows Netdata to monitor its own resource utilization.

[**cgroups.plugin**](/src/collectors/cgroups.plugin/README.md) collects rich metrics about containers and virtual machines using the virtual files under `/sys/fs/cgroup`. By reading cgroups, Netdata can instantly collect resource utilization metrics for systemd services, all containers (Docker, LXC, LXD, Libvirt, systemd-nspawn), and more. Learn more in the [collecting container metrics](/docs/collecting-metrics/container-metrics.md) doc.

[**ebpf.plugin**](/src/collectors/ebpf.plugin/README.md): Netdata's extended Berkeley Packet Filter (eBPF) collector monitors Linux kernel-level metrics for file descriptors, virtual filesystem IO, and process management. You can use our eBPF collector to analyze how and when a process accesses files, when it makes system calls, whether it leaks memory or creating zombie processes, and more.

While the above plugins and associated collectors are the most important for system metrics, there are many others. You can find all of our data collection integrations [here](/src/collectors/COLLECTORS.md#system-collectors).
