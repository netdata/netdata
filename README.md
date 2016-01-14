netdata
=======

[Live Demo Site](http://netdata.firehol.org)


### Realtime time data collection and charts!

**Netdata** is a daemon that collects data in realtime (per second) and presents a web site to view and analyze them.
The presentation is full of charts that precisely render all system values, in realtime.

It has been designed to be installed **on every system**, without desrupting it:

1. It will just use some spare CPU cycles.

    Although it is very light weight, on slow processors you can futher control its CPU consumption by lowering its data collection frequency.
    By default it is running with the lowest possible linux priority.

2. It will use the memory you want it have.

    Although we have made the most to make its memory footprint the smallest possible,
    you can further control the memory it will use, by sizing its round robin memory database.

3. It does not use disk I/O.

    All its round robin database is in memory.
    It is only saved on disk and loaded back when netdata restarts.
    You can also disable the access log of its embedded web server, so that netdata will not use any I/O at all while running.


You can use it to monitor all your applications, servers, linux PCs or linux embedded devices.

Out of the box, it comes with plugins for data collection about system information and popular applications.


# Features

- **highly optimized C code**

  It only needs a few milliseconds per second to collect all the data.
  It will nicelly run even on a raspberry pi with just one cpu core, or any other embedded system.

- **extremely lightweight**

  It only needs a few megabytes of memory to store its round robin database.

  Although `netdata` does all its calculation using `long double` (128 bit) arithmetics, it stores all values using a **custom-made 32-bit number**. This custom-made number can store in 29 bits values from -167772150000000.0 to  167772150000000.0 with a precision of 0.00001 (yes, it is a floating point number, meaning that higher integer values have less decimal precision) and 3 bits for flags. This provides an extremely optimized memory footprint with just 0.0001% max accuracy loss (run: `./netdata --unittest` to see it in action).

  If your linux box has KSM enabled, netdata will give it all its round robbin database, to lower its memory requirements even further.

- **per second data collection**

  Every chart, every value, is updated every second. Of course, you can control collection period per plugin.

  **netdata** can perform several calculations on each value (dimension) collected:

  - **absolute**, stores the collected value, as collected (this is used, for example for the number of processes running, the number of connections open, the amount of RAM used, etc)

  - **incremental**, stores the difference of the collected value to the last collected value (this is used, for example, for the bandwidth of interfaces, disk I/O, i.e. for counters that always get incremented) - **netdata** automatically interpolates these values to second boundary, using nanosecond calculations so that small delays at the data collection layer will not affect the quality of the result - also, **netdata** detects arithmetic overflows and presents them properly at the charts.

  - **percentage of absolute row**, stores the percentage of the collected value, over the sum of all dimensions of the chart.

  - **percentage of incremental row**, stores the percentage of this collected value, over the sum of the the **incremental** differences of all dimensions of the chart (this is used, for example, for system CPU utilization).

- **visualizes QoS classes automatically**

  If you also use FireQOS for QoS, it collects class names automatically.

- **appealing web site**

  The web site uses bootstrap and the excellent [dygraphs](http://dygraphs.com), for a very appealing and responsive result.
  It works even on mobile devices and adapts to screen size changes and rotation (responsive design).

- **web charts do respect your browser resources**

  The charts adapt to show only as many points are required to have a clear view.
  Also, the javascript code respects your browser resources (stops refreshing when the window looses focus, when something is selected, when charts are not in the visible viewport, etc).

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
 - `/proc/interrupts` (total and per core hardware interrupts)
 - `/proc/softirqs` (total and per core software interrupts)
 - `tc` classes (QoS classes - [with FireQOS class names](http://firehol.org/tutorial/fireqos-new-user/))

- It supports **plugins** for collecting information from other sources!

  Plugins can be written in any computer language (pipe / stdout communication for data collection).

  It ships with 2 plugins: `apps.plugin` and `charts.d.plugin`:

 - `apps.plugin` is a plugin that attempts to collect statistics per process. It groups the entire process tree based on your settings (for example, mplayer, kodi, vlc are all considered `media`) and for each group it attempts to find CPU usage, memory usages, physical and logical disk read and writes, number of processes, number of threads, number of open files, number of open sockets, number of open pipes, minor and major page faults (major = swapping), etc. 15 stackable (per group) charts in total.

 - `charts.d.plugin` provides a simple way to script data collection in BASH. It includes example plugins that collect values from:

    - `nut` (UPS load, frequency, voltage, etc, for multiple UPSes)
    - `sensors` (temperature, voltage, current, power, humidity, fans rotation sensors)
    - `cpufreq` (current CPU clock frequency, for all CPUs)
    - `postfix` (e-mail queue size)
    - `squid` (web proxy statistics)
    - `mysql` (mysql global statistics)
    - `opensips` (opensips statistics)

    Of course, you can write your own using BASH scripting.

- netdata is a web server, supporting gzip compression

  It serves its own static files and dynamic files for rendering the site.
  It does not support authentication or SSL - limit its access using your firewall, or put it behind an authentication proxy.
  It does not allow ` .. ` in the files requested (so it can only serve files stored in the web directory `/usr/share/netdata/web`).


# How it works

1. You run a daemon on your linux: netdata.
 This deamon is written in C and is extremely lightweight.

 netdata:

  - Spawns threads to collect all the data for all sources
  - Keeps track of the collected values in memory (no disk I/O at all)
  - Generates JSON and JSONP HTTP responses containing all the data needed for the web graphs
  - Is a standalone web server.

 For example, you can access JSON data by using:

 ```
 
 http://netdata.firehol.org/api/v1/data?chart=net.eth0&after=-300&before=0&points=120&group=average&format=json

 ```

 The above will give you the last 300 seconds of traffic for eth0, aggregated in 120 points, grouped as averages, in json format.

 Check [Netdata Swagger UI](http://netdata.firehol.org/swagger/) for more information about the API.

 
2. If you need to embed a **netdata** chart on your web page, you can add a few javascript lines and a `div` for every graph you need. Check [this example](http://netdata.firehol.org/dashboard.html) (open it in a new tab and view its source to get the idea).

3. No internet access is required. Netdata is standalone.


# Installation

Check the **[[Installation]]** page.
