# netdata

**Real-time performance monitoring, done right!**

![image](https://cloud.githubusercontent.com/assets/2662304/14090945/e9aea428-f545-11e5-8942-9f9cf03fc592.png)

## Features

**netdata** is a highly optimized Linux daemon providing **real-time performance monitoring for Linux systems, Applications, SNMP devices, over the web**!

It tries to visualize the **truth of now**, in its **greatest detail**, so that you can get insights of what is happening now and what just happened, on your systems and applications.

This is what you get:

1. **Beautiful out of the box** with bootstrap dashboards
2. You can build your **custom dashboards**, with simple HTML (no javascript necessary)
3. **Blazingly fast** and **super efficient**, just 2% of a single core and a few MB of RAM
3. **Zero configuration** - you just install it and it autodetects everything
4. **Zero dependencies**, it is its own web server for its static web files and its web API
4. **Extensible**, you can monitor anything you can get a metric for, using its Plugin API (anything can be a netdata plugin - from BASH to node.js)
7. **Embeddable**, it can run anywhere a Linux kernel runs

This is what it currently monitors (most with zero configuration):

1. **CPU usage, interrupts, softirqs and frequency** (total and per core)
2. **RAM, swap and kernel memory usage** (including KSM and kernel memory deduper)
3. **Disk I/O** (per disk: bandwidth, operations, backlog, utilization, etc)
4. **Network interfaces** (per interface: bandwidth, packets, errors, drops, etc)
5. **IPv4 networking** (packets, errors, fragments, tcp: connections, packets, errors, handshake, udp: packets, errors, broadcast: bandwidth, packets, multicast: bandwidth, packets)
6. **netfilter / iptables** Linux firewall (connections, connection tracker events, errors, etc)
7. **Processes** (running, blocked, forks, active, etc)
8. **Entropy**
9. **NFS file servers**, v2, v3, v4 (I/O, cache, read ahead, RPC calls)
10. **Network QoS** (yes, the only tool that visualizes network `tc` classes in realtime)
11. **Applications**, by grouping the process tree (CPU, memory, disk reads, disk writes, swap, threads, pipes, sockets, etc)
12. **Apache web server** mod-status (v2.2, v2.4)
13. **Nginx web server** stub-status
14. **mySQL databases** (more than one DBs, each showing: bandwidth, queries/s, handlers, locks, issues, tmp operations, connections, binlog metrics, threads, innodb metrics, etc)
15. **ISC Bind / Named** (clients, requests, queries, updates, failures and several per view metrics)
16. **Postfix** message queue (entries, size)
17. **Squid proxy** (clients bandwidth and requests, servers bandwidth and requests) 
18. **Hardware sensors** (temperature, voltage, fans, power, humidity, etc)
19. **NUT UPSes** (load, charge, battery voltage, temperature, utility metrics, output metrics)

You can also monitor any number of **SNMP devices**, although you will need to configure these.

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

