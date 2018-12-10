# Why Netdata

![image8](https://cloud.githubusercontent.com/assets/2662304/14253735/536f4580-fa95-11e5-9f7b-99112b31a5d7.gif)

## Netdata is unique!

The following is an animated GIF showing **netdata**'s ability to monitor QoS. The timings of this animation have not been altered, this is the real thing:

![animation5](https://cloud.githubusercontent.com/assets/2662304/12373715/0da509d8-bc8b-11e5-85cf-39d5234bf976.gif)

Check the details on this animation:

1. At the beginning the charts auto-refresh, in real-time
2. Charts can be dragged and zoomed (either mouse or touch)
3. You pan or zoom one, the others follow
4. Mouse over on one, selects the same timestamp on all
5. Dimensions can be enabled or disabled
6. All refreshes are instant (an 8 year old core-2 duo computer was used to record this)

There are a lot of excellent open source tools for collecting and visualizing performance metrics. Check for example [collectd](https://collectd.org/), [OpenTSDB](http://opentsdb.net/), [influxdb](https://influxdata.com/), [Grafana](http://grafana.org/), etc.

So, why **netdata**?

Well, **netdata** has a quite different approach.

## Simplicity

> Most monitoring solutions require endless configuration of whatever imaginable. Well, this is a linux box. Why do we need to configure every single metric we need to monitor. Of course it has a CPU and RAM and a few disks, and ethernet ports, it might run a firewall, a web server, or a database server and so on. Why do we need to configure all these metrics?

**Netdata** has been designed to auto-detect everything. Of course you can enable, tweak or disable things. But by default, if **netdata** can retrieve `/server-status` from an web server you run on your linux box, it will automatically collect all performance metrics. This happens for apache, squid, nginx, mysql, opensips, etc. It will also automatically collect all available system values for CPU, memory, disks, network interfaces, QoS (with labels if you also use [FireQOS](http://firehol.org)), etc. Even for applications that do not offer performance metrics, it will automatically group the whole process tree and provide metrics like CPU usage, memory allocated, opened files, sockets, disk activity, swap activity, etc per application group.

Netdata supports plenty of [configuration](../daemon/config/). However, we have done everything we can to allow netdata to auto-detect as much as possible.

Even netdata plugins are designed to support configuration-less operation. So, you just install and run netdata. You will need to configure something only if it cannot be auto-detected.

> Take any performance monitoring solution and try to troubleshoot a performance problem. At the end of the day you will have to ssh to the server to understand what exactly is happening. You will have to use `iostat`, `iotop`, `vmstat`, `top`, `iperf`, `ethtool` and probably a few dozen more console tools to figure it out.

With **netdata**, this need is eliminated significantly. Of course you will ssh. Just not for monitoring performance.

If you install **netdata** you will prefer it over the console tools. **Netdata** visualizes the data, while the console tools just show their values. The detail is the same - I have spent pretty much time reading the source code of the console tools, to figure out what needs to do done in netdata, so that the data, the values, will be the same. Actually, **netdata** is more precise than most console tools, it will interpolate all collected values to second boundary, so that even if something took a few microseconds more to be collected, netdata will correctly estimate the per second value.

**Netdata** visualizes data in ways you cannot even imagine on a console. It allows you to see the present in real-time, much like the console tools, but also the recent past, compare different metrics with each other, zoom in to see the recent past in detail, or zoom out to have a helicopter view of what is happening in longer durations, build custom dashboards with just the charts you need for a specific purpose.

Most engineers that install netdata, ssh to the server to tweak system or application settings and at the same time they monitor the result of the new settings in **netdata** on their browser.

## Per second data collection and visualization

**Per second data collection and visualization** is usually only available in dedicated console tools, like `top`, `vmstat`, `iostat`, etc. Netdata brings per second data collection and visualization to all applications, accessible through the web.

*You are not convinced per second data collection is important?*
**Click** this image for a demo:

[![image](https://cloud.githubusercontent.com/assets/2662304/12373555/abd56f04-bc85-11e5-9fa1-10aa3a4b648b.png)](http://netdata.firehol.org/demo2.html)

## Realtime monitoring

> Any performance monitoring solution that does not go down to per second collection and visualization of the data, is useless. It will make you happy to have it, but it will not help you more than that. 

Visualizing the present in **real-time and in great detail**, is the most important value a performance monitoring solution should provide. The next most important is the last hour, again per second. The next is the last 8 hours and so on, up to a week, or at most a month. In my 20+ years in IT, I needed just once or twice to look a year back. And this was mainly out of curiosity.

Of course real-time monitoring requires resources. **netdata** is designed to be very efficient:

1. collecting performance data is a repeating process - you do the same thing again and again. **Netdata** has been designed to learn from each iteration, so that the next one will be faster. It learns the sizes of files (it even keeps them open when it can), the number of lines and words per line they contain, the sizes of the buffers it needs to process them, etc. It adapts, so that everything will be as ready as possible for the next iteration.
2. internally, it uses hashes and indexes (b-trees), to speed up lookups of metrics, charts, dimensions, settings.
3. it has an in-memory round robin database based on a custom floating point number that allows it to pack values and flags together, in 32 bits, to lower its memory footprint.
4. its internal web server is capable of generating JSON responses from live performance data with speeds comparable to static content delivery (it does not use `printf`, it is actually 11 times faster than in generating JSON compared to `printf`).

**Netdata** will use some CPU and memory, but it **will not produce any disk I/O at all**, apart its logs (which you can disable if you like).

Most servers should have plenty of CPU resources (I consider a hardware upgrade or application split when a server averages around 40% CPU utilization at the peak hour). Even if a server has limited CPU resources available, you can just lower the data collection frequency of **netdata**. Going from per second to every 2 seconds data collection, will cut the **netdata** CPU requirements in half and you will still get charts that are just 2 seconds behind.

The same goes for memory. If you just keep an hour of data (which is perfect for performance troubleshooting), you will most probably need 15-20MB. You can also enable the kernel de-duper (Kernel Same-Page Merging) and **netdata** will offer to it all its round robin database. KSM can free 20-60% of the memory used by **netdata** (guess why: there are a lot of metrics that are always zero or just constant).

When netdata runs on modern computers (even on CELERON processors), most chart queries are replied in less than 3 milliseconds! **Not seconds, MILLISECONDS!** Less than 3 milliseconds for calculating the chart, generating JSON text, compressing it and sending it to your web browser. Timings are logged in netdata's `access.log` for you to examine.

Netdata is written in plain `C` and the key system plugins are written in `C` too. Its speed can only be compared to the native console system administration tools.

You can also stress test your netdata installation by running the script `tests/stress.sh` found in the distribution. Most modern server hardware can serve more than 300 chart refreshes per second per core. A raspberry pi 2, can serve 300+ chart refreshes per second utilizing all of its 4 cores.


## No disk I/O at all

Netdata does not use any disk I/O, apart from its logs and even these can be disabled.

Netdata will use some memory (you size it, check [[Memory Requirements]] and CPU (below 2% of a single core for the daemon, plugins may require more, check [[Performance]]), but normally your systems should have plenty of these resources available and spare.

The design goal of **NO DISK I/O AT ALL** effectively means netdata will not disrupt your applications.

## No root access

You don't need to run netdata as root. If started as root, netdata will switch to the `netdata` user (or any other user given in its configuration or command line argument).

There are a few plugins that in order to collect values need root access. These (and only these) are setuid to root.

## Visualizes QoS

Netdata visualizes `tc` QoS classes automatically. If you also use FireQOS, it will also collect interface and class names.

Check this animated GIF (generated with [ScreenToGif](https://github.com/NickeManarin/ScreenToGif)):

![animation5](https://cloud.githubusercontent.com/assets/2662304/12373715/0da509d8-bc8b-11e5-85cf-39d5234bf976.gif)

## Embedded web server

> Most solutions require dedicated servers to actually use the monitoring console. To my perspective, this is totally unneeded for performance monitoring. All of us have a spectacular tool on our desktops, that allows us to connect in real time to any server in the world: **the web browser**. It shouldn't be so hard to use the same tool to connect in real-time to all our servers.

With **netdata**, there is no need to centralize anything for performance monitoring. You view everything directly from their source. No need to run something else to access netdata. Of course you can use a firewall, or a reverse proxy, to limit access to it. But for most systems, inside your DMZ, just running it will be enough.

Still, with **netdata** you can build dashboards with charts from any number of servers. And these charts will be connected to each other much like the ones that come from the same server. You will hover on one and all of them will show the relative value for the selected timestamp. You will zoom or pan one and all of them will follow. **Netdata** achieves that because the logic that connects the charts together is at the browser, not the server, so that all charts presented on the same page are connected, no matter where they come from.

## Performance monitoring, scaled properly

"Properly"? What is "properly"?

We know software solutions can **scale up** (i.e. you replace its resources with bigger ones), or **scale out** (i.e. you add more smaller resources to it). In both cases, to get more of it, you need to supply **more resources**.

So, what is "scaled properly"?

Traditionally, monitoring solutions centralize all metric data to provide unified dashboards across all servers. So, you install agents on all your servers to collect system and application metrics which are then sent to a central place for storage and processing. Depending on the solution you use, the central place can either **scale up** or **scale out** (or a mix of the two).

"Scaled properly" is something completely different. "Scaled properly" minimizes the need for a "central place", so that **there is nothing to be scaled**!

Wait a moment! You cannot take out the "central place" of a monitoring solution!

Yes, we can! well... most of it, but before explaining how, let's see what happens today:

Monitoring solutions are a key component for any online service. These solutions usually consume considerable amount of resources. This is true for both "scale-up" and "scale-out" solutions. These resources require maintenance and administration too. To balance the resources required, these monitoring solutions follow a few simple rules:

1. The number of metrics collected per server is limited. They collect CPU, RAM, DISK, NETWORK metrics and a few application metrics.

2. The data collection frequency of each metric is also very low, at best it is once every 10 or 15 seconds, at worst every 5 or 10 mins.

Due to all the above, most centralized monitoring solutions are usually good for alarms and **statistics of past performance**. The alarms usually trigger every 1 to 5 minutes and you get a few low-resolution charts about the past performance of your servers.

Well... there is something wrong in this approach! Can you see it?

Let's see the netdata approach:

1. Data collection happens **per second**. This allows true real-time performance monitoring.

2. **Thousands of metrics** per server and application are collected, **every single second**. The number of metrics collected is not a problem.
 
3. Data do not leave the server they are collected. Data are not centralized, so the need for a huge central place that will process and store gazillions of data is not needed.

   > Ok, I hear a few of you complaining already - you will find out... patience...

4. netdata does not use any DISK I/O while running (apart its log files - and even these can be disabled) and netdata runs with the lowest possible process priority, so that **your applications will never be affected by it**.

5. Each netdata is standalone. Your web browser connects directly to each server to present real-time dashboards. The charts are so snappy, so real-time, so fast that we can call netdata, **a console killer for performance monitoring**.

The charting libraries **netdata** uses, are the fastest possible ([Dygraphs](http://dygraphs.com/) do make the difference!) and **netdata** respects browser resources. Data are just rendered on a canvas. No processing in javascript at all.

6. netdata is very efficient: just 2% of a single core is required and some RAM, and you can actually control how much of both you want to allocate to it.


Server side, chart data generation scales pretty well. You can expect 400+ chart refreshes per second per core on modern hardware. For a page with 10 charts visible (the page may have hundreds, but only the visible are refreshed), just a tiny fraction of a single CPU core will be used for servicing them. Even these refreshes stop when you switch tabs on your browser, you focus on another window, scroll to a part of the page without charts, zoom or pan a chart. And of course the **netdata** server runs with the lowest possible process priority, so that your production environment, your applications, will not be slowed down by the netdata server.

7. netdata dashboards can be multi-server (check: [http://my-netdata.io](http://my-netdata.io)) - your browser connects to each netdata server directly.

So, using netdata, your monitoring infrastructure is embedded on each server, limiting significantly the need of additional resources. netdata is very resource efficient and utilizes server resources that already exist and are spare (on each server).

Of course, there are a few issues that need to be addressed with this approach:

1. We need an index of all netdata installations we have
2. We need a place to handle notifications and alarms
3. We need a place to save statistics of past performance

Our approach uses the netdata [registry](../registry/). The registry solves the problem of maintaining a list of all the netdata installations we have. It does this transparently, without any configuration. It tracks the netdata servers your web browser has visited and bookmarks them at the `my-netdata` menu.

Every netdata can be a registry. You can use the global one we provided for free, or pick one of your netdata servers and turn it to a registry for your network.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2FWhy-Netdata&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
