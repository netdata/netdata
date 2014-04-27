netdata
=======

linux real time system monitoring

This program is a daemon that collects system information from a linux system and presents a web site to view the data.
This presentation is full of charts that precisely render all system values, in realtime, for a short time (1 hour by default).

# Features

- highly optimized C code
  it only needs a few milliseconds per second to collect all the data.
 
- extremely lightweight
  it only needs a few megabytes of memory to store all its round robin database.

- per second data collection
  every chart, every value, is updated every second.

- visualizes QoS classes automatically
  if you also use fireqos for QoS, it even collects class names automatically.

- the generated web site uses bootstrap and google charts for a very appealing result
  it works even on mobile devices, adapts to screen size changes and rotation.

- web charts do respect your browser resources
  the charts adapt to show only as many points are required to have a clear view.

- highly configurable
  all charts and all features can be enabled or disabled.

- it reads and renders charts for all these:
 - /proc/net/dev (all netwrok interfaces for all their values)
 - /proc/diskstats (all disks for all their values)
 - /proc/net/snmp (total IPv4, TCP and UDP usage)
 - /proc/net/netstat (more IPv4 usage)
 - /proc/net/stat/nf_conntrack (connection tracking performance)
 - /proc/net/ip_vs/stats (IPVS connection statistics)
 - /proc/stat (CPU utilization)
 - /proc/meminfo (memory information)
 - /proc/vmstat (system performance)
 - tc classes (QoS classes)


Check it live at:

http://www.tsaousis.gr:19999/

Here is a screenshot:

![image](https://cloud.githubusercontent.com/assets/2662304/2593406/3c797e88-ba80-11e3-8ec7-c10174d59ad6.png)


# How it works

1. You run a daemon on your linux: netdata.
 This deamon is written in C and is extremely lightweight.
 
 netdata:

  - reads several /proc files
  - keeps track of the values in memroy (a short history)
  - generates JSON and JSONP HTTP responses containing all the data needed for the web graphs
  - is a web server. You can access JSON data by using:
 
 ```
 http://127.0.0.1:19999/data/net.eth0
 ```
 
 This will give you the JSON file for traffic on eth0.
 The above is equivalent to:
 
 ```
 http://127.0.0.1:19999/data/net.eth0/3600/1/average/0/0
 ```
 
 where:

  - 3600 is the number of entries to generate (3600 is a default which can be overwritten by -l).
  - 1 is grouping count, 1 = every single entry, 2 = half the entries, 3 = one every 3 entries, etc
  - `average` is the grouping method. It can also be `max`.
  - 0/0 they are `before` and `after` timestamps, allowing panning on the data


2. On your web page, you add a few javascript lines and a DIV for every graph you need.
 Your browser will hit the web server to fetch the JSON data and refresh the graphs.

3. Graphs are generated using Google Charts API.



# Installation

## Automatic installation

Before you start, make sure you have `zlib` development files installed.
To install it in Ubuntu, you need to run:

```sh
apt-get install zlib1g-dev
```

Then do this to install and run netdata:

```sh
git clone https://github.com/ktsaou/netdata.git netdata
cd netdata
./netdata.start
```

Once you run it, the file netdata.conf will be created. You can edit this file to set options for each graph.
To apply the changes you made, you have to run netdata.start again.

To access the web site for all graphs, go to:

 ```
 http://127.0.0.1:19999/
 ```

