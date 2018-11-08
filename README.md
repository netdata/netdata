# netdata [![Build Status](https://travis-ci.com/netdata/netdata.svg?branch=master)](https://travis-ci.com/netdata/netdata) [![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/2231/badge)](https://bestpractices.coreinfrastructure.org/projects/2231) [![License: GPL v3+](https://img.shields.io/badge/License-GPL%20v3%2B-blue.svg)](https://www.gnu.org/licenses/gpl-3.0)

[![Code Climate](https://codeclimate.com/github/netdata/netdata/badges/gpa.svg)](https://codeclimate.com/github/netdata/netdata)
[![Codacy Badge](https://api.codacy.com/project/badge/Grade/a994873f30d045b9b4b83606c3eb3498)](https://www.codacy.com/app/netdata/netdata?utm_source=github.com&amp;utm_medium=referral&amp;utm_content=netdata/netdata&amp;utm_campaign=Badge_Grade)
[![LGTM C](https://img.shields.io/lgtm/grade/cpp/g/netdata/netdata.svg?logo=lgtm)](https://lgtm.com/projects/g/netdata/netdata/context:cpp)
[![LGTM JS](https://img.shields.io/lgtm/grade/javascript/g/netdata/netdata.svg?logo=lgtm)](https://lgtm.com/projects/g/netdata/netdata/context:javascript)
[![LGTM PYTHON](https://img.shields.io/lgtm/grade/python/g/netdata/netdata.svg?logo=lgtm)](https://lgtm.com/projects/g/netdata/netdata/context:python)

> *New to netdata? Here is a live demo: [http://my-netdata.io](http://my-netdata.io)*

**netdata** is a system for **distributed real-time performance and health monitoring**.

It provides **unparalleled insights**, **in real-time**, of everything happening on the systems it runs (including containers and applications such as web and database servers), using **modern interactive web dashboards**.

### netdata core values

we value                    |netdata...
:--------------------------:|:----
high resolution metrics     |collects all metrics **every single second**
unlimited metrics           |collects **thousands of metrics** per monitored node
real-time visualization     |dashboards run with **sub-second latency**, collection to visualization
powerful anomaly detection  |has a **distributed watchdog** embedded in it, running on all monitored nodes
visual anomaly detection    |dashboards are optimized for **spotting anomalies**, across all metrics
meaningful presentation     |dashboards present **all metrics in a structured, easy to understand, way**
zero configuration          |**auto-detects** all metrics and comes with dozens of alarms
resource utilization        |core is **optimized C code**, using <1% utilization of single CPU core

netdata also supports:
 
 - monitoring **ephemeral nodes** and **auto-scaled containers**,
 - **integration** with existing monitoring infrastructure (time-series databases like `prometheus`, `graphite`, `opentsdb`) and third-party event notification methods
(like `slack`, `pagerduty`, `pushover`, and dozens more),
 - building hierarchies of monitored nodes via **real-time metrics streaming** between them,
 - embedding charts and dashboards on third party web sites and applications, such as [Atlassian's Confluence](https://github.com/netdata/netdata/wiki/Custom-Dashboard-with-Confluence).

_netdata is **fast** and **efficient**, designed to permanently run on all systems
(**physical** & **virtual** servers, **containers**, **IoT** devices), without
disrupting their core function._

netdata currently runs on **Linux**, **FreeBSD**, and **MacOS**.

[![Twitter Follow](https://img.shields.io/twitter/follow/linuxnetdata.svg?style=social&label=New%20-%20stay%20in%20touch%20-%20follow%20netdata%20on%20twitter)](https://twitter.com/linuxnetdata)
[![analytics](http://www.google-analytics.com/collect?v=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Freadme&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()

---

## User base
*Docker pulls*<br/>
[![netdata/netdata (official)](https://img.shields.io/docker/pulls/netdata/netdata.svg?label=netdata/netdata+%28official%29)](https://hub.docker.com/r/netdata/netdata/)
[![firehol/netdata (deprecated)](https://img.shields.io/docker/pulls/firehol/netdata.svg?label=firehol/netdata+%28deprecated%29)](https://hub.docker.com/r/firehol/netdata/)
[![titpetric/netdata (donated)](https://img.shields.io/docker/pulls/titpetric/netdata.svg?label=titpetric/netdata+%28third+party%29)](https://hub.docker.com/r/titpetric/netdata/)

*Since May 16th 2016 (the date the [global public netdata registry](https://github.com/netdata/netdata/wiki/mynetdata-menu-item) was released):*<br/>
[![User Base](https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_entries&dimensions=persons&label=user%20base&units=null&value_color=blue&precision=0&v42)](https://registry.my-netdata.io/#menu_netdata_submenu_registry)
[![Monitored Servers](https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_entries&dimensions=machines&label=servers%20monitored&units=null&value_color=orange&precision=0&v42)](https://registry.my-netdata.io/#menu_netdata_submenu_registry)
[![Sessions Served](https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_sessions&label=sessions%20served&units=null&value_color=yellowgreen&precision=0&v42)](https://registry.my-netdata.io/#menu_netdata_submenu_registry)

*in the last 24 hours:*<br/>
[![New Users Today](http://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_entries&dimensions=persons&after=-86400&options=unaligned&group=incremental-sum&label=new%20users%20today&units=null&value_color=blue&precision=0&v42)](https://registry.my-netdata.io/#menu_netdata_submenu_registry)
[![New Machines Today](https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_entries&dimensions=machines&group=incremental-sum&after=-86400&options=unaligned&label=servers%20added%20today&units=null&value_color=orange&precision=0&v42)](https://registry.my-netdata.io/#menu_netdata_submenu_registry)
[![Sessions Today](https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_sessions&after=-86400&group=incremental-sum&options=unaligned&label=sessions%20served%20today&units=null&value_color=yellowgreen&precision=0&v42)](https://registry.my-netdata.io/#menu_netdata_submenu_registry)

---

## News

`Nov 6th, 2018` - **[netdata v1.11.0 released!](https://github.com/netdata/netdata/releases)**

 - New query engine, supporting statistical functions.
 - Fixed security issues identified by Red4Sec.com and Synacktiv.
 - New Data Collection Modules: `rethinkdbs`, `proxysql`, `litespeed`, `uwsgi`, `unbound`, `powerdns`, `dockerd`, `puppet`, `logind`, `adaptec_raid`, `megacli`, `spigotmc`, `boinc`, `w1sensor`, `monit`, `linux_power_supplies`.
 - Improved Data Collection Modules: `statsd.plugin`, `apps.plugin`, `freeipmi.plugin`, `proc.plugin`, `diskspace.plugin`, `freebsd.plugin`, `python.d.plugin`, `web_log`, `nginx_plus`, `ipfs`, `fail2ban`, `ceph`, `elasticsearch`, `nginx_plus`,  `redis`, 
 `beanstalk`, `mysql`, `varnish`, `couchdb`, `phpfpm`, `apache`, `icecast`, `mongodb`, `postgress`, `elasticsearch`, `mdstat`, `openvpn_log`, `snmp`, `nut`.

 - Added alarms for detecting abnormally high load average, `TCP` `SYN` and `TCP` accept queue overflows, network interfaces congestion and alarms for `bcache`, `mdstat`, `apcupsd`, `mysql`.
 - system alarms are now enabled on FreeBSD.
 - New notification methods: **rocket.chat**, **Microsoft Teams**, **syslog**, **fleep.io**, **Amazon SNS**.

 - and dozens more improvements, enhancements, features and compatibility fixes

---

`Sep 18, 2018` - **netdata has its own organization**

Netdata used to be a [firehol.org](https://firehol.org) project, accessible as `firehol/netdata`.

Netdata now has its own github organization `netdata`, so all github URLs are now `netdata/netdata`. The old github URLs, repo clones, forks, etc redirect automatically to the new repo.  

---

![cncf](https://www.cncf.io/wp-content/uploads/2016/09/logo_cncf.png)

`Jun 16, 2018` - **netdata in CNCF**

Netdata is now at the [Cloud Native Computing Foundation (CNCF) landscape](https://landscape.cncf.io/grouping=no&sort=stars).

Read the [netdata presentation](https://docs.google.com/presentation/d/18C8bCTbtgKDWqPa57GXIjB2PbjjpjsUNkLtZEz6YK8s/edit?usp=sharing) we gave at CNCF TOC on Sep 18, 2018.

---

## Features

<p align="center">
<img src="https://cloud.githubusercontent.com/assets/2662304/19168687/f6a567be-8c19-11e6-8561-ce8d589e8346.gif"/>
</p>

 - **Stunning interactive bootstrap dashboards**<br/>
   mouse and touch friendly, in 2 themes: dark, light

 - **Amazingly fast**<br/>
   responds to all queries in less than 0.5 ms per metric,
   even on low-end hardware

 - **Highly efficient**<br/>
   collects thousands of metrics per server per second,
   with just 1% CPU utilization of a single core, a few MB of RAM and no disk I/O at all

 - **Sophisticated alerting**<br/>
   hundreds of alarms, **out of the box**!<br/>
   supports dynamic thresholds, hysteresis, alarm templates,
   multiple role-based notification methods (such as email, slack.com, flock.com,
   pushover.net, pushbullet.com, telegram.org, twilio.com, messagebird.com, kavenegar.com)

 - **Extensible**<br/>
   you can monitor anything you can get a metric for,
   using its Plugin API (anything can be a netdata plugin,
   BASH, python, perl, node.js, java, Go, ruby, etc)

 - **Embeddable**<br/>
   it can run anywhere a Linux kernel runs (even IoT)
   and its charts can be embedded on your web pages too

 - **Customizable**<br/>
   custom dashboards can be built using simple HTML (no javascript necessary)

 - **Zero configuration**<br/>
   auto-detects everything, it can collect up to 5000 metrics
   per server out of the box

 - **Zero dependencies**<br/>
   it is even its own web server, for its static web files and its web API

 - **Zero maintenance**<br/>
   you just run it, it does the rest

 - **scales to infinity**<br/>
   requiring minimal central resources

 - **several operating modes**<br/>
   autonomous host monitoring, headless data collector, forwarding proxy, store and forward proxy, central multi-host monitoring, in all possible configurations.
   Each node may have different metrics retention policy and run with or without health monitoring.

 - **time-series back-ends supported**<br/>
   can archive its metrics on `graphite`, `opentsdb`, `prometheus`, json document DBs, in the same or lower detail
   (lower: to prevent it from congesting these servers due to the amount of data collected)

![netdata](https://cloud.githubusercontent.com/assets/2662304/14092712/93b039ea-f551-11e5-822c-beadbf2b2a2e.gif)

---

## What does it monitor?

netdata collects several thousands of metrics per device.
All these metrics are collected and visualized in real-time.

> _Almost all metrics are auto-detected, without any configuration._

This is a list of what it currently monitors:

- **CPU**<br/>
  usage, interrupts, softirqs, frequency, total and per core, CPU states

- **Memory**<br/>
  RAM, swap and kernel memory usage, KSM (Kernel Samepage Merging), NUMA

- **Disks**<br/>
  per disk: I/O, operations, backlog, utilization, space, software RAID (md)

   ![sda](https://cloud.githubusercontent.com/assets/2662304/14093195/c882bbf4-f554-11e5-8863-1788d643d2c0.gif)

- **Network interfaces**<br/>
  per interface: bandwidth, packets, errors, drops

   ![dsl0](https://cloud.githubusercontent.com/assets/2662304/14093128/4d566494-f554-11e5-8ee4-5392e0ac51f0.gif)

- **IPv4 networking**<br/>
  bandwidth, packets, errors, fragments,
  tcp: connections, packets, errors, handshake,
  udp: packets, errors,
  broadcast: bandwidth, packets,
  multicast: bandwidth, packets

- **IPv6 networking**<br/>
  bandwidth, packets, errors, fragments, ECT,
  udp: packets, errors,
  udplite: packets, errors,
  broadcast: bandwidth,
  multicast: bandwidth, packets,
  icmp: messages, errors, echos, router, neighbor, MLDv2, group membership,
  break down by type

- **Interprocess Communication - IPC**<br/>
  such as semaphores and semaphores arrays

- **netfilter / iptables Linux firewall**<br/>
  connections, connection tracker events, errors

- **Linux DDoS protection**<br/>
  SYNPROXY metrics

- **fping** latencies</br>
  for any number of hosts, showing latency, packets and packet loss

   ![image](https://cloud.githubusercontent.com/assets/2662304/20464811/9517d2b4-af57-11e6-8361-f6cc57541cd7.png)


- **Processes**<br/>
  running, blocked, forks, active

- **Entropy**<br/>
  random numbers pool, using in cryptography

- **NFS file servers and clients**<br/>
  NFS v2, v3, v4: I/O, cache, read ahead, RPC calls

- **Network QoS**<br/>
  the only tool that visualizes network `tc` classes in realtime

   ![qos-tc-classes](https://cloud.githubusercontent.com/assets/2662304/14093004/68966020-f553-11e5-98fe-ffee2086fafd.gif)

- **Linux Control Groups**<br/>
  containers: systemd, lxc, docker

- **Applications**<br/>
  by grouping the process tree and reporting CPU, memory, disk reads,
  disk writes, swap, threads, pipes, sockets - per group

   ![apps](https://cloud.githubusercontent.com/assets/2662304/14093565/67c4002c-f557-11e5-86bd-0154f5135def.gif)

- **Users and User Groups resource usage**<br/>
  by summarizing the process tree per user and group,
  reporting: CPU, memory, disk reads, disk writes, swap, threads, pipes, sockets

- **Apache and lighttpd web servers**<br/>
   `mod-status` (v2.2, v2.4) and cache log statistics, for multiple servers

- **Nginx web servers**<br/>
  `stub-status`, for multiple servers

- **Tomcat**<br/>
  accesses, threads, free memory, volume

- **web server log files**<br/>
  extracting in real-time, web server performance metrics and applying several health checks

- **mySQL databases**<br/>
  multiple servers, each showing: bandwidth, queries/s, handlers, locks, issues,
  tmp operations, connections, binlog metrics, threads, innodb metrics, and more

- **Postgres databases**<br/>
  multiple servers, each showing: per database statistics (connections, tuples
  read - written - returned, transactions, locks), backend processes, indexes,
  tables, write ahead, background writer and more

- **Redis databases**<br/>
  multiple servers, each showing: operations, hit rate, memory, keys, clients, slaves

- **couchdb**<br/>
  reads/writes, request methods, status codes, tasks, replication, per-db, etc

- **mongodb**<br/>
  operations, clients, transactions, cursors, connections, asserts, locks, etc

- **memcached databases**<br/>
  multiple servers, each showing: bandwidth, connections, items

- **elasticsearch**<br/>
  search and index performance, latency, timings, cluster statistics, threads statistics, etc

- **ISC Bind name servers**<br/>
  multiple servers, each showing: clients, requests, queries, updates, failures and several per view metrics

- **NSD name servers**<br/>
  queries, zones, protocols, query types, transfers, etc.

- **PowerDNS**</br>
  queries, answers, cache, latency, etc.

- **Postfix email servers**<br/>
  message queue (entries, size)

- **exim email servers**<br/>
  message queue (emails queued)

- **Dovecot** POP3/IMAP servers<br/>

- **ISC dhcpd**<br/>
  pools utilization, leases, etc.

- **IPFS**<br/>
  bandwidth, peers

- **Squid proxy servers**<br/>
  multiple servers, each showing: clients bandwidth and requests, servers bandwidth and requests

- **HAproxy**<br/>
  bandwidth, sessions, backends, etc

- **varnish**<br/>
  threads, sessions, hits, objects, backends, etc

- **OpenVPN**<br/>
  status per tunnel

- **Hardware sensors**<br/>
  `lm_sensors` and `IPMI`: temperature, voltage, fans, power, humidity

- **NUT and APC UPSes**<br/>
  load, charge, battery voltage, temperature, utility metrics, output metrics

- **PHP-FPM**<br/>
  multiple instances, each reporting connections, requests, performance

- **hddtemp**<br/>
  disk temperatures

- **smartd**<br/>
  disk S.M.A.R.T. values

- **SNMP devices**<br/>
  can be monitored too (although you will need to configure these)

- **chrony**</br>
  frequencies, offsets, delays, etc.

- **beanstalkd**</br>
  global and per tube monitoring

- **statsd**<br/>
  [netdata is a fully featured statsd server](https://github.com/netdata/netdata/wiki/statsd)

- **ceph**<br/>
  OSD usage, Pool usage, number of objects, etc.

And you can extend it, by writing plugins that collect data from any source, using any computer language.

---

## netdata infographic

This is a high level overview of netdata feature set and architecture.
Click it to to interact with it (it has direct links to documentation).

[![image](https://user-images.githubusercontent.com/2662304/47672043-a47eb480-dbb9-11e8-92a4-fa422d053309.png)](https://my-netdata.io/infographic.html)

---

## Installation

Use our **[automatic installer](https://github.com/netdata/netdata/wiki/Installation)** to build and install it on your system.

It should run on **any Linux** system (including IoT). It has been tested on:

- Alpine
- Arch Linux
- CentOS
- Debian
- Fedora
- Gentoo
- openSUSE
- PLD Linux
- RedHat Enterprise Linux
- SUSE
- Ubuntu

---

## Interaction with netdata

After installation, you can interact with netdata using **[CLI](https://github.com/netdata/netdata/wiki/Command-Line-Options)** and web dashboards.
The default port of dashboard is 19999. To access the web dashboard on localhost, use: http://localhost:19999

---

## Documentation

Check the **[netdata wiki](https://github.com/netdata/netdata/wiki)**.

## License

netdata is [GPLv3+](LICENSE).

Netdata re-distributes other open-source tools and libraries. Please check the [third party licenses](https://github.com/netdata/netdata/blob/master/REDISTRIBUTED.md).
