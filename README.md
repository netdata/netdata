netdata
=======

linux real time system monitoring over the web

Netdata is a daemon that collects system information from a linux system and presents a web site to view the data.
The presentation is full of charts that precisely render all system values, in realtime, for a short time (1 hour by default).

# Features

- **highly optimized C code**

  It only needs a few milliseconds per second to collect all the data.
  It will nicelly run even on a raspberry pi with just one cpu core, or any other embedded system.

- **extremely lightweight**

  It only needs a few megabytes of memory to store all its round robin database.
  
  Internally, it uses a **custom-made 32-bit number** to store all the values, along with a limited number of metadata for each collected value. This floating point number can store in 32 bits values from -167772150000000.0 to  167772150000000.0 with a precision of 0.00001 (yes, it is a floating point number, meaning that higher integer values have less decimal precision). Of course, this provides an extremely optimized memory footprint. 

- **per second data collection**

  Every chart, every value, is updated every second. Of course, you can control collection period per module.

- **visualizes QoS classes automatically**

  If you also use FireQOS for QoS, it collects class names automatically.

- **appealing web site**

  The web site uses bootstrap and google charts for a very appealing result.
  It works even on mobile devices and adapts to screen size changes and rotation.

- **web charts do respect your browser resources**

  The charts adapt to show only as many points are required to have a clear view.
  Also, the javascript code respects your browser resources (stops refreshing when the window looses focus, when scrolling, etc).

- **highly configurable**

  All charts and all features can be enabled or disabled.
  The program generates its configuration file based on the resources available on the system it runs, for you to edit.

- It reads and renders charts for all these:
 - `/proc/net/dev` (all netwrok interfaces for all their values)
 - `/proc/diskstats` (all disks for all their values)
 - `/proc/net/snmp` (total IPv4, TCP and UDP usage)
 - `/proc/net/netstat` (more IPv4 usage)
 - `/proc/net/stat/nf_conntrack` (connection tracking performance)
 - `/proc/net/ip_vs/stats` (IPVS connection statistics)
 - `/proc/stat` (CPU utilization)
 - `/proc/meminfo` (memory information)
 - `/proc/vmstat` (system performance)
 - `/proc/net/rpc/nfsd` (NFS server statistics for both v3 and v4 NFS)
 - `tc` classes (QoS classes)

- It supports **plugins** for collecting information from other sources!

  Plugins can be written in any computer language (pipe / stdout communication for data collection).

  It ships with 2 plugins: `apps.plugin` and `charts.d.plugin`:

 - `apps.plugin` is a plugin that attempts to collect statistics per process.

 - `charts.d.plugin` provides a simple way to script data collection in BASH. It includes example plugins that collect values from:

  - `nut` (UPS load, frequency, voltage, etc)
  - `pi` (raspberry pi CPU clock and temperature)
  - `postfix` (e-mail queue size)
  - `squid` (web proxy statistics)

- netdata is a web server, supporting gzip compression

  It serves its own static files and dynamic files for rendering the site.
  (it does not support authentication or SSL - limit its access using your firewall)


Check it live at:

 - [My Home Gentoo Box](http://195.97.5.206:19999/)
 - [My Home Raspberry Pi B+](http://195.97.5.202:19999/) with data collection every 5s (raspbian playing movies 24x7)
 - [My Home Raspberry Pi 2](http://195.97.5.203:19999/) (osmc as an access point)

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

  - 3600 is the number of entries to generate.
  - 1 is grouping count, 1 = every single entry, 2 = half the entries, 3 = one every 3 entries, etc
  - `average` is the grouping method. It can also be `max`.
  - 0/0 they are `before` and `after` timestamps, allowing panning on the data


2. On your web page, you add a few javascript lines and a DIV for every graph you need.
 Your browser will hit the web server to fetch the JSON data and refresh the graphs.

3. Graphs are generated using Google Charts API (so, your client needs to have internet access).


# Installation

## Automatic installation

Before you start, make sure you have `zlib` development files installed.
To install it in Ubuntu, you need to run:

```sh
apt-get install zlib1g-dev
```

Then do this to install and run netdata:

```sh
git clone https://github.com/ktsaou/netdata.git netdata.git
cd netdata.git
./netdata.start
```

Once you run it, the file conf.d/netdata.conf will be created. You can edit this file to set options for each graph.
To apply the changes you made, you have to run netdata.start again.

To access the web site for all graphs, go to:

 ```
 http://127.0.0.1:19999/
 ```

