# netdata

**Real-time performance monitoring, done right!**

![netdata](https://cloud.githubusercontent.com/assets/2662304/14092712/93b039ea-f551-11e5-822c-beadbf2b2a2e.gif)

---

## Features

**netdata** is a highly optimized Linux daemon providing **real-time performance monitoring for Linux systems, Applications, SNMP devices, over the web**!

It tries to visualize the **truth of now**, in its **greatest detail**, so that you can get insights of what is happening now and what just happened, on your systems and applications.

This is what you get:

1. **Beautiful out of the box** bootstrap dashboards
2. **Custom dashboards** that can be built using simple HTML (no javascript necessary)
3. **Blazingly fast** and **super efficient**, written in C (for default installations, expect just 2% of a single core CPU usage and a few MB of RAM)
3. **Zero configuration** - you just install it and it autodetects everything
4. **Zero dependencies**, it is its own web server for its static web files and its web API
4. **Extensible**, you can monitor anything you can get a metric for, using its Plugin API (anything can be a netdata plugin - from BASH to node.js)
7. **Embeddable**, it can run anywhere a Linux kernel runs

---

## What does it monitor?

This is what it currently monitors (most with zero configuration):

1. **CPU usage, interrupts, softirqs and frequency** (total and per core)
2. **RAM, swap and kernel memory usage** (including KSM and kernel memory deduper)
3. **Disk I/O** (per disk: bandwidth, operations, backlog, utilization, etc)

   ![sda](https://cloud.githubusercontent.com/assets/2662304/14093195/c882bbf4-f554-11e5-8863-1788d643d2c0.gif)

4. **Network interfaces** (per interface: bandwidth, packets, errors, drops, etc)

   ![dsl0](https://cloud.githubusercontent.com/assets/2662304/14093128/4d566494-f554-11e5-8ee4-5392e0ac51f0.gif)

5. **IPv4 networking** (packets, errors, fragments, tcp: connections, packets, errors, handshake, udp: packets, errors, broadcast: bandwidth, packets, multicast: bandwidth, packets)
6. **netfilter / iptables Linux firewall** (connections, connection tracker events, errors, etc)
7. **Processes** (running, blocked, forks, active, etc)
8. **Entropy**
9. **NFS file servers**, v2, v3, v4 (I/O, cache, read ahead, RPC calls)
10. **Network QoS** (yes, the only tool that visualizes network `tc` classes in realtime)

   ![qos-tc-classes](https://cloud.githubusercontent.com/assets/2662304/14093004/68966020-f553-11e5-98fe-ffee2086fafd.gif)


11. **Applications**, by grouping the process tree (CPU, memory, disk reads, disk writes, swap, threads, pipes, sockets, etc)

   ![apps](https://cloud.githubusercontent.com/assets/2662304/14093565/67c4002c-f557-11e5-86bd-0154f5135def.gif)

12. **Apache web server** mod-status (v2.2, v2.4)
13. **Nginx web server** stub-status
14. **mySQL databases** (multiple servers, each showing: bandwidth, queries/s, handlers, locks, issues, tmp operations, connections, binlog metrics, threads, innodb metrics, etc)
15. **ISC Bind name server** (multiple servers, each showing: clients, requests, queries, updates, failures and several per view metrics)
16. **Postfix email server** message queue (entries, size)
17. **Squid proxy server** (clients bandwidth and requests, servers bandwidth and requests) 
18. **Hardware sensors** (temperature, voltage, fans, power, humidity, etc)
19. **NUT UPSes** (load, charge, battery voltage, temperature, utility metrics, output metrics)

Any number of **SNMP devices** can be monitored, although you will need to configure these.

And you can extend it, by writing plugins that collect data from any source, using any computer language.

---

## Still not convinced?

Read **[Why netdata?](https://github.com/firehol/netdata/wiki/Why-netdata%3F)**

---

## Installation

Use our **[automatic installer](https://github.com/firehol/netdata/wiki/Installation)** to build and install it on your system

It should run on any Linux system. We have tested it on:

- Gentoo
- ArchLinux
- Ubuntu / Debian
- CentOS
- Fedora

---

## Documentation

Check the **[netdata wiki](https://github.com/firehol/netdata/wiki)**.

