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

The process for enabling the eBPF collector is a little more complex than we'd like to cover in this tutorial. Read the
[eBPF collector documentation](../../collectors/ebpf_process.plugin/README.md) for the full instructions.

If you don't want to enable the eBPF collector now, you can still learn about the value of monitoring eBPF metrics in
general, and what we currently support, below.

## Why eBPF monitoring matters

t/k

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
