![image7](https://cloud.githubusercontent.com/assets/2662304/14253734/536bd4c2-fa95-11e5-8872-81eed5178e4b.gif)

# Welcome to netdata!

## Demo Sites

Live demo installations of netdata are available at:

Location |  netdata demo URL | VM Donated by
:-------:|:-----:|:-----------
EU (London)|**[netdata.firehol.org](http://netdata.firehol.org)**|Donated by [DigitalOcean.com](https://www.digitalocean.com/) and located at their excellent Cloud Computing services
US (Atlanta)|**[netdata2.firehol.org](http://netdata2.firehol.org)**|Donated by [CDN77.com](https://www.cdn77.com/) and located at their awesome CDN network

Netdata dashboards are mobile and touch friendly.

## Installation

Want to set it up on your systems now? Jump to **[[Installation]]**.

---

**Netdata** is a daemon that collects data in realtime (per second) and presents a web site to view and analyze them.
The presentation is also real-time and full of interactive charts that precisely render all collected values.

It has been designed to be installed **on every system**, without disrupting the applications running on it:

1. It will just use some spare CPU cycles (check **[[Performance]]**).
2. It will use the memory you want it have (check **[[Memory Requirements]]**).
3. Once started and while running, it does not use any disk I/O, apart its logging (check **[[Log Files]]**). Of course it saves its DB to disk when it exits and loads it back when it starts.

You can use it to monitor all your systems and applications. It will run on Linux PCs, servers or embedded devices.

Out of the box, it comes with plugins that collect key system metrics and metrics of popular applications.

---

## Why another monitoring tool?

The key goal of **netdata** is to help you achieve **operational excellence**.

To achieve that, it focuses on **real-time visualization** of what is happening on your systems or applications *now* and in the *recent past*.

**netdata** tries to visualize the truth of **now**, in its **greatest detail**, with detail comparable to the console tools!

So, netdata is: **non disruptive, real-time performance monitoring and visualization, in the greatest possible detail**.

---

## How it works

You run a daemon on your linux: `netdata`. This daemon is written in C and is extremely lightweight.

**netdata**:

  - Spawns threads to collect all the data from all sources - it uses **[[Internal Plugins]]** and **[[External Plugins]]** for this.
  - Keeps track of the collected values in memory (no disk I/O at all, check **[[Memory Requirements]]**).
  - Is a standalone web server that serves its static files, for rendering its dashboards.
  - It provides a **[[REST API v1]]** for your browser to access the data.

If you install it on all your systems, each **netdata** will be standalone. There is no *central* netdata. Your web browser is the only entity that can *connect* all the netdata installations together. netdata dashboards can have charts from multiple netdata installations and these charts will still behave, on your browser, as if they were coming from the same netdata server!

In this image you can see netdata displaying charts from 2 servers. On the left is the demo site and on the right is a local installation of it (you have the same page on your netdata too, at `/tv.html`):

![tv](https://cloud.githubusercontent.com/assets/2662304/14262483/6d8500f8-fabe-11e5-84e1-c510b3bd6ebd.gif)

---

# Documentation

This wiki is the whole of it. Other than the wiki, currently there is the... source code.

You should at least walk through the pages of the wiki. They have a good overview of netdata, what it can do and how to use it.

---

# Support

If you need help, please use the github issues section.

---


# FAQ

## Is it ready?

Software is never ready. There is always something to improve.

Netdata is stable. We use it on production systems without any issues.

## Is it released?

Yeap! Check the [releases page](https://github.com/firehol/netdata/releases).

## Can I code too?

Of course! Fork the repo, adapt as you see fit and create pull requests.
If you want to discuss your plans, open a github issue to start the discussion.

## Why you wrote data collection?

Well, there are plenty of data collectors already. But we have one or more of the following problems with them:

- They are not able for per second data collection
- They can do per second data collection, but they are not optimized enough for always running on all systems
- They need to be configured, when we need auto-detection

Of course, we could use them just to get data at a slower rate, and this can be done, but it was not our priority. netdata proves that **real-time data collection and visualization can be done efficiently**.

## Is it practical to have so short historical data?

For a few purposes yes, for others no.

Our focus is **real-time data collection and visualization**. Our (let's say) "competitors" are the console tools, neither grafana nor collectd, statsd, nagios, zabbix, etc. All these are perfect tools for what they do (and they do a lot). But we think they provide "statistics about past performance" (of course with alarms, health monitoring, etc). netdata provides "real-time performance monitoring", much like the console tools do. Different things.

Of course, historical data is our next priority.

## Why there is no "central" netdata?

We strongly believe monitoring should be scaled out, not up. A "central" monitoring server is just another problem and should be avoided. Of course it is needed for health monitoring, but for real-time performance monitoring it will just add delays and eventually destroy the whole idea.

We all have a wonderful tool on our desktops, that connects us to the entire world: the **web browser**! This is the "central" netdata that connects all the netdata installations. We have done a lot of work towards this and we believe we are very close to show you what we mean. Patience...


## Is there a roadmap?

These are what we currently work on (in that order):

1. Finish packaging for the various distros.

2. More plugins - a lot more plugins!

 - monitor cgroups (containers) performance and utilization from the host
 - monitor more of the system
 - monitor more applications (hadoop and friends, postgres, etc)
 - re-write BASH data collectors (squid, mysql, etc) to node.js or python
 - rewrite the netfilter plugin to use libnlm.
 - allow internal plugins to be forked to external processes (this will protect the netdata daemon from plugin crashes, allow different security schemas for each plugin, etc).

3. Improve the memory database (possibly using an internal deduper, compression, disk archiving, mirroring it to third party databases, etc).

4. Invent a flexible UI to connect multiple netdata server together

5. Document everything (this is a work in progress already).

There are a lot more enhancements requested from our users (just navigate through the issues to get an idea).
Enhancements like authentication on UI, alarms and alerts, etc will fit somehow into this list.
Patience...