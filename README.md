# netdata

#### 230.000+ views, 62.000+ visitors, 18.500+ downloads, 9.500+ github stars, 500+ forks, 14 days!

And it still runs with 700+ git downloads... per day!

**[Check what our users say about netdata](https://github.com/firehol/netdata/issues/148)**.

Thank you!

---

**Real-time performance monitoring, done right!**

This is the default dashboard of **netdata**:

 - real-time, per second updates, snappy refreshes!
 - 300+ charts out of the box, 2000+ metrics monitored!
 - zero configuration, zero maintenance, zero dependencies!

Live demo: [http://netdata.firehol.org](http://netdata.firehol.org)

![netdata](https://cloud.githubusercontent.com/assets/2662304/14092712/93b039ea-f551-11e5-822c-beadbf2b2a2e.gif)

---

## Features

**netdata** is a highly optimized Linux daemon providing **real-time performance monitoring for Linux systems, Applications, SNMP devices, over the web**!

It tries to visualize the **truth of now**, in its **greatest detail**, so that you can get insights of what is happening now and what just happened, on your systems and applications.

This is what you get:

- **Stunning bootstrap dashboards**, out of the box (themable: dark, light)
- **Blazingly fast** and **super efficient**, mostly written in C (for default installations, expect just 2% of a single core CPU usage and a few MB of RAM)
- **Zero configuration** - you just install it and it autodetects everything
- **Zero dependencies**, it is its own web server for its static web files and its web API
- **Zero maintenance**, you just run it, it does the rest
- **Custom dashboards** that can be built using simple HTML (no javascript necessary)
- **Extensible**, you can monitor anything you can get a metric for, using its Plugin API (anything can be a netdata plugin - from BASH to node.js, so you can easily monitor any application, any API)
- **Embeddable**, it can run anywhere a Linux kernel runs and its charts can be embedded on your web pages too

---

## What does it monitor?

This is what it currently monitors (most with zero configuration):

- **CPU usage, interrupts, softirqs and frequency** (total and per core)

- **RAM, swap and kernel memory usage** (including KSM and kernel memory deduper)

- **Disks** (per disk: I/O, operations, backlog, utilization, etc)

   ![sda](https://cloud.githubusercontent.com/assets/2662304/14093195/c882bbf4-f554-11e5-8863-1788d643d2c0.gif)

- **Network interfaces** (per interface: bandwidth, packets, errors, drops, etc)

   ![dsl0](https://cloud.githubusercontent.com/assets/2662304/14093128/4d566494-f554-11e5-8ee4-5392e0ac51f0.gif)

- **IPv4 networking** (bandwidth, packets, errors, fragments, tcp: connections, packets, errors, handshake, udp: packets, errors, broadcast: bandwidth, packets, multicast: bandwidth, packets)

- **IPv6 networking** (bandwidth, packets, errors, fragments, ECT, udp: packets, errors, udplite: packets, errors, broadcast: bandwidth, multicast: bandwidth, packets, icmp: messages, errors, echos, router, neighbor, MLDv2, group membership, break down by type)

- **netfilter / iptables Linux firewall** (connections, connection tracker events, errors, etc)

- **Linux anti-DDoS protection** (SYNPROXY metrics)

- **Processes** (running, blocked, forks, active, etc)

- **Entropy** (random numbers pool, using in cryptography)

- **NFS file servers**, v2, v3, v4 (I/O, cache, read ahead, RPC calls)

- **Network QoS** (yes, the only tool that visualizes network `tc` classes in realtime)

   ![qos-tc-classes](https://cloud.githubusercontent.com/assets/2662304/14093004/68966020-f553-11e5-98fe-ffee2086fafd.gif)

- **Applications**, by grouping the process tree (CPU, memory, disk reads, disk writes, swap, threads, pipes, sockets, etc)

   ![apps](https://cloud.githubusercontent.com/assets/2662304/14093565/67c4002c-f557-11e5-86bd-0154f5135def.gif)

- **Users and User Groups resource usage**, by summarizing the process tree per user and group (CPU, memory, disk reads, disk writes, swap, threads, pipes, sockets, etc)

- **Apache web server** mod-status (v2.2, v2.4)

- **Nginx web server** stub-status

- **mySQL databases** (multiple servers, each showing: bandwidth, queries/s, handlers, locks, issues, tmp operations, connections, binlog metrics, threads, innodb metrics, etc)

- **ISC Bind name server** (multiple servers, each showing: clients, requests, queries, updates, failures and several per view metrics)

- **Postfix email server** message queue (entries, size)

- **Squid proxy server** (clients bandwidth and requests, servers bandwidth and requests) 

- **Hardware sensors** (temperature, voltage, fans, power, humidity, etc)

- **NUT UPSes** (load, charge, battery voltage, temperature, utility metrics, output metrics)

- **SNMP devices** can be monitored too (although you will need to configure these)

And you can extend it, by writing plugins that collect data from any source, using any computer language.

---

## Still not convinced?

Read **[Why netdata?](https://github.com/firehol/netdata/wiki/Why-netdata%3F)**

---

## Installation

Use our **[automatic installer](https://github.com/firehol/netdata/wiki/Installation)** to build and install it on your system

It should run on **any Linux** system. It has been tested on:

- Gentoo
- Arch Linux
- Ubuntu / Debian
- CentOS
- Fedora
- RedHat Enterprise Linux
- SUSE
- Alpine Linux
- PLD Linux

---

## Documentation

Check the **[netdata wiki](https://github.com/firehol/netdata/wiki)**.
