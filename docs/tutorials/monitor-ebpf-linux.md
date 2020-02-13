# Monitor eBPF metrics with Netdata

eBPF (Extended Berkeley Packet Filter) is a virtual bytecode machine built into the Linux kernel that can be used for
advanced monitoring and tracing. With eBPF, you can get detailed metrics about I/O and filesystem latency, CPU usage by
process, and network performance. You can even use it for tracing, as you might `tcpdump` or `strace`.

Brendan Gregg of Netflix has called eBPF a
["superpower"](http://www.brendangregg.com/blog/2016-03-05/linux-bpf-superpowers.html) and claims ["eBPF does to Linux
what JavaScript does to HTML"](http://www.brendangregg.com/blog/2019-01-01/learn-ebpf-tracing.html).

All in all, eBPF is a powerful tool for anyone who wants to use Netdata to get even deeper insights into their systems
and applications. Thanks to a new plugin and associated collectors released in
[v1.20](https://blog.netdata.cloud/posts/release-1.20/) of Netdata, you can now monitor eBPF metrics with Netdata's
per-second metrics collection and troubleshoot your applications with interactive visualizations.

## Why eBPF monitoring matters

t/k

## Set up your system

eBPF monitoring requires a Linux system running kernel version `4.11.0` or later, the `Kprobes` module enabled, and the
`tracefs` and `debugfs` filesystems mounted. See our [eBPF plugin documentation](#enable-the-plugin-on-linux) for
details about how you can ensure your system meets these prerequisites, and steps to follow if it doesn't already.

As with all of our plugins and their associated collectors, restart Netdata with `service netdata restart`, or use the
[appropriate method](../getting-started.md#start-stop-and-restart-netdata) for your system, and refresh your browser.
Your Netdata dashboard should now feature eBPF charts brimming with new metrics every second!

**IMAGE GOES HERE**

## Configure the eBPF plugin

The only other optional configuration is to use a different eBPF program than the default, `entry`. Read about each in
our [documentation](#load). The default produces the fewest charts, but has the least amount of processing overload, and
is the best option when getting started with monitoring eBPF.

## Example #1

t/k

## Example #2

t/k

## Example #3

t/k

## Alarms?

t/k

## What's next?

-   [A thorough introduction to eBPF](https://lwn.net/Articles/740157/)
-   [Berkely Packet Filter](https://en.wikipedia.org/wiki/Berkeley_Packet_Filter)
-   [eBPF Superpowers](https://www.youtube.com/watch?v=4SiWL5tULnQ)
