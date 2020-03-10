<!--
---
title: "Netdata [![Build Status](https://travis-ci.com/netdata/netdata.svg?branch=master)](https://travis-ci.com/netdata/netdata) [![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/2231/badge)](https://bestpractices.coreinfrastructure.org/projects/2231) [![License: GPL v3+](https://img.shields.io/badge/License-GPL%20v3%2B-blue.svg)](https://www.gnu.org/licenses/gpl-3.0) [![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Freadme&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)"
custom_edit_url: https://github.com/netdata/netdata/edit/master/README.md
---
-->

# Netdata [![Build Status](https://travis-ci.com/netdata/netdata.svg?branch=master)](https://travis-ci.com/netdata/netdata) [![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/2231/badge)](https://bestpractices.coreinfrastructure.org/projects/2231) [![License: GPL v3+](https://img.shields.io/badge/License-GPL%20v3%2B-blue.svg)](https://www.gnu.org/licenses/gpl-3.0) [![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Freadme&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)

[![Code
Climate](https://codeclimate.com/github/netdata/netdata/badges/gpa.svg)](https://codeclimate.com/github/netdata/netdata)
[![Codacy
Badge](https://api.codacy.com/project/badge/Grade/a994873f30d045b9b4b83606c3eb3498)](https://www.codacy.com/app/netdata/netdata?utm_source=github.com&utm_medium=referral&utm_content=netdata/netdata&utm_campaign=Badge_Grade)
[![LGTM
C](https://img.shields.io/lgtm/grade/cpp/g/netdata/netdata.svg?logo=lgtm)](https://lgtm.com/projects/g/netdata/netdata/context:cpp)
[![LGTM
JS](https://img.shields.io/lgtm/grade/javascript/g/netdata/netdata.svg?logo=lgtm)](https://lgtm.com/projects/g/netdata/netdata/context:javascript)
[![LGTM
PYTHON](https://img.shields.io/lgtm/grade/python/g/netdata/netdata.svg?logo=lgtm)](https://lgtm.com/projects/g/netdata/netdata/context:python)

---

Netdata is **distributed, real-time performance and health monitoring** for systems and applications. It is a
highly-optimized monitoring agent you install on all your systems and containers.

Netdata provides **unparalleled insights**, in **real-time**, of everything happening on the systems it's running on
(including web servers, databases, applications), using **highly interactive web dashboards**. 

A highly-efficient database **stores long-term historical metrics for days, weeks, or months**, all at 1-second
granularity. Run this long-term storage autonomously, or integrate Netdata with your existing monitoring toolchains
(Prometheus, Graphite, OpenTSDB, Kafka, Grafana, and more).

Netdata is **fast** and **efficient**, designed to permanently run on all systems (**physical** and **virtual** servers,
**containers**, **IoT** devices), without disrupting their core function.

Netdata is **free, open-source software** and it currently runs on **Linux**, **FreeBSD**, and **MacOS**, along with
other systems derived from them, such as **Kubernetes** and **Docker**.

Netdata is not hosted by the CNCF but is the 3rd most starred open-source project in the [Cloud Native Computing
Foundation (CNCF) landscape](https://landscape.cncf.io/format=card-mode&grouping=no&sort=stars).

---

People get **addicted to Netdata**. Once you use it on your systems, **there is no going back**! _You've been warned..._

![image](https://user-images.githubusercontent.com/2662304/48305662-9de82980-e537-11e8-9f5b-aa1a60fbb82f.png)

[![Tweet about
Netdata!](https://img.shields.io/twitter/url/http/shields.io.svg?style=social&label=Tweet%20about%20netdata)](https://twitter.com/intent/tweet?text=Netdata,%20real-time%20performance%20and%20health%20monitoring,%20done%20right!&url=https://my-netdata.io/&via=linuxnetdata&hashtags=netdata,monitoring)

## Contents

1.  [What does it look like?](#what-does-it-look-like) - Take a quick tour through the dashboard
2.  [Our userbase](#user-base) - Enterprises we help monitor and our userbase
3.  [Quickstart](#quickstart) - How to try it now on your systems
4.  [Why Netdata](#why-netdata) - Why people love Netdata and how it compares with other solutions
5.  [News](#news) - The latest news about Netdata
6.  [How Netdata works](#how-it-works) - A high-level diagram of how Netdata works
7.  [Infographic](#infographic) - Everything about Netdata in a single graphic
8.  [Features](#features) - How you'll use Netdata on your systems
9.  [Visualization](#visualization) - Learn about visual anomaly detection
10. [What Netdata monitors](#what-netdata-monitors) - See which apps/services Netdata auto-detects
11. [Documentation](#documentation) - Read the documentation
12. [Community](#community) - Discuss Netdata with others and get support
13. [License](#license) - Check Netdata's licencing
14. [Is it any good?](#is-it-any-good) - Yes.
15. [Is it awesome?](#is-it-awesome) - Yes.

## What does it look like?

The following animated GIF shows the top part of a typical Netdata dashboard.

![The Netdata dashboard in
action](https://user-images.githubusercontent.com/1153921/70638670-85dd5080-1bf6-11ea-893e-94400f445574.gif)

> A typical Netdata dashboard, in 1:1 timing. Charts can be panned by dragging them, zoomed in/out with `SHIFT` + `mouse
> wheel`, an area can be selected for zoom-in with `SHIFT` + `mouse selection`. Netdata is highly interactive,
> **real-time**, and optimized to get the work done!

Want to see Netdata live? Check out any of our [live demos](https://www.netdata.cloud/#live-demo).

## User base

Netdata is used by hundreds of thousands of users all over the world. Check our [GitHub watchers
list](https://github.com/netdata/netdata/watchers). You will find people working for **Amazon**, **Atos**, **Baidu**,
**Cisco Systems**, **Citrix**, **Deutsche Telekom**, **DigitalOcean**, **Elastic**, **EPAM Systems**, **Ericsson**,
**Google**, **Groupon**, **Hortonworks**, **HP**, **Huawei**, **IBM**, **Microsoft**, **NewRelic**, **Nvidia**, **Red
Hat**, **SAP**, **Selectel**, **TicketMaster**, **Vimeo**, and many more!

### Docker pulls

We provide Docker images for the most common architectures. These are statistics reported by Docker Hub:

[![netdata/netdata
(official)](https://img.shields.io/docker/pulls/netdata/netdata.svg?label=netdata/netdata+%28official%29)](https://hub.docker.com/r/netdata/netdata/)
[![firehol/netdata
(deprecated)](https://img.shields.io/docker/pulls/firehol/netdata.svg?label=firehol/netdata+%28deprecated%29)](https://hub.docker.com/r/firehol/netdata/)
[![titpetric/netdata
(donated)](https://img.shields.io/docker/pulls/titpetric/netdata.svg?label=titpetric/netdata+%28third+party%29)](https://hub.docker.com/r/titpetric/netdata/)

### Registry

When you install multiple Netdata, they are integrated into **one distributed application**, via a [Netdata
registry](registry/README.md). This is a web browser feature and it allows us to count the number of unique users and
unique Netdata servers installed. The following information comes from the global public Netdata registry we run:

[![User
Base](https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_entries&dimensions=persons&label=user%20base&units=M&value_color=blue&precision=2&divide=1000000&v43)](https://registry.my-netdata.io/#menu_netdata_submenu_registry)
[![Monitored
Servers](https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_entries&dimensions=machines&label=servers%20monitored&units=k&divide=1000&value_color=orange&precision=2&v43)](https://registry.my-netdata.io/#menu_netdata_submenu_registry)
[![Sessions
Served](https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_sessions&label=sessions%20served&units=M&value_color=yellowgreen&precision=2&divide=1000000&v43)](https://registry.my-netdata.io/#menu_netdata_submenu_registry)

_In the last 24 hours:_<br/> [![New Users
Today](https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_entries&dimensions=persons&after=-86400&options=unaligned&group=incremental-sum&label=new%20users%20today&units=null&value_color=blue&precision=0&v42)](https://registry.my-netdata.io/#menu_netdata_submenu_registry)
[![New Machines
Today](https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_entries&dimensions=machines&group=incremental-sum&after=-86400&options=unaligned&label=servers%20added%20today&units=null&value_color=orange&precision=0&v42)](https://registry.my-netdata.io/#menu_netdata_submenu_registry)
[![Sessions
Today](https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_sessions&after=-86400&group=incremental-sum&options=unaligned&label=sessions%20served%20today&units=null&value_color=yellowgreen&precision=0&v42)](https://registry.my-netdata.io/#menu_netdata_submenu_registry)

## Quickstart

![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart&group=sum&after=-3600&label=last+hour&units=installations&value_color=orange&precision=0)
![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart&group=sum&after=-86400&label=today&units=installations&precision=0)

To install Netdata from source on any Linux system (physical, virtual, container, IoT, edge) and keep it up to date with
our **nightly releases** automatically, run the following:

```bash
# make sure you run `bash` for your shell
bash

# install Netdata directly from GitHub source
bash <(curl -Ss https://my-netdata.io/kickstart.sh)
```

Starting with v1.12, Netdata collects anonymous usage information by default and sends it to Google Analytics. Read
about the information collected, and learn how to-opt, on our [anonymous statistics](docs/anonymous-statistics.md) page.

The usage statistics are _vital_ for us, as we use them to discover bugs and prioritize new features. We thank you for
_actively_ contributing to Netdata's future.

To learn more about the pros and cons of using _nightly_ vs. _stable_ releases, see our [notice about the two options](packaging/installer/README.md#nightly-vs-stable-releases).

The above command will:

-   Install any required packages on your system (it will ask you to confirm before doing so)
-   Compile it, install it, and start it.

More installation methods and additional options can be found at the [installation page](packaging/installer/).

To try Netdata in a Docker container, run this:

```sh
docker run -d --name=netdata \
  -p 19999:19999 \
  -v /etc/passwd:/host/etc/passwd:ro \
  -v /etc/group:/host/etc/group:ro \
  -v /proc:/host/proc:ro \
  -v /sys:/host/sys:ro \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  --cap-add SYS_PTRACE \
  --security-opt apparmor=unconfined \
  netdata/netdata
```

For more information about running Netdata in Docker, check the [docker installation page](packaging/docker/).

![image](https://user-images.githubusercontent.com/2662304/48304090-fd384080-e51b-11e8-80ae-eecb03118dda.png)

From Netdata v1.12 and above, anonymous usage information is collected by default and sent to Google Analytics. To read
more about the information collected and how to opt-out, check the [anonymous statistics
page](docs/anonymous-statistics.md).

## Why Netdata

Netdata has a quite different approach to monitoring.

Netdata is a monitoring agent you install on all your systems. It is:

-   A **metrics collector** for system and application metrics (including web servers, databases, containers, and much
    more),
-   A **long-term metrics database** that stores recent metrics in memory and "spills" historical metrics to disk for
    efficient long-term storage,
-   A super fast, interactive, and modern **metrics visualizer** optimized for anomaly detection,
-   And an **alarms notification engine** for detecting performance and availability issues.

All the above, are packaged together in a very flexible, extremely modular, distributed application.

This is how Netdata compares to other monitoring solutions:

| Netdata                                                         | others (open-source and commercial)                              |
| :-------------------------------------------------------------- | :--------------------------------------------------------------- |
| **High resolution metrics** (1s granularity)                    | Low resolution metrics (10s granularity at best)                 |
| Monitors everything, **thousands of metrics per node**          | Monitor just a few metrics                                       |
| UI is super fast, optimized for **anomaly detection**           | UI is good for just an abstract view                             |
| **Long-term, autonomous storage** at one-second granularity     | Centralized metrics in an expensive data lake at 10s granularity |
| **Meaningful presentation**, to help you understand the metrics | You have to know the metrics before you start                    |
| Install and get results **immediately**                         | Long preparation is required to get any useful results           |
| Use it for **troubleshooting** performance problems             | Use them to get _statistics of past performance_                 |
| **Kills the console** for tracing performance issues            | The console is always required for troubleshooting               |
| Requires **zero dedicated resources**                           | Require large dedicated resources                                |

Netdata is **open-source**, **free**, super **fast**, very **easy**, completely **open**, extremely **efficient**,
**flexible** and integrate-able.

It has been designed by **system administrators**, **DevOps engineers**, and **developers** for to not just visualize
metrics, but also troubleshoot complex performance problems.

## News

`Nov 27th, 2019` - **[Netdata v1.19.0 released!](https://github.com/netdata/netdata/releases)**

Release v1.19.0 contains 2 new collectors, 19 bug fixes, 17 improvements, and 19 documentation updates.

We completed a major rewrite of our **web log collector** to dramatically improve its flexibility and performance. The
[new collector](https://github.com/netdata/go.d.plugin/pull/141), written entirely in Go, can parse and chart logs from
Nginx and Apache servers, and combines numerous improvements. Netdata now supports the LTSV log format, creates charts
for TLS and cipher usage, and is amazingly fast. In a test using SSD storage, the collector parsed the logs for 200,000
requests in about 200ms, using 30% of a single core.

This Go-based collector also has powerful custom log parsing capabilities, which means we're one step closer to a
generic application log parser for Netdata. We're continuing to work on this parser to support more application log
formatting in the future.

We have a new tutorial on [enabling the Go web log
collector](https://docs.netdata.cloud/docs/tutorials/collect-apache-nginx-web-logs/) and using it with Nginx and/or
Apache access logs with minimal configuration. Thanks to [Wing924](https://github.com/Wing924) for starting the Go
rewrite!

We introduced more **cmocka unit testing** to Netdata. In this release, we're testing how Netdata's internal web server
processes HTTP requests—the first step to improve the quality of code throughout, reduce bugs, and make refactoring
easier. We wanted to validate the web server's behavior but needed to build a layer of parametric testing on top of the
CMocka test runner. Read all about our process of testing and selecting cmocka on our blog post: [Building an agile
team's 'safety harness' with cmocka and FOSS](https://blog.netdata.cloud/posts/agile-team-cmocka-foss/).

Netdata's **Unbound collector** was also [completely rewritten in Go](https://github.com/netdata/go.d.plugin/pull/287)
to improve how it collects and displays metrics. This new version can get dozens of metrics, including details on
queries, cache, uptime, and even show per-thread metrics. See our
[tutorial](https://docs.netdata.cloud/docs/tutorials/collect-unbound-metrics/) on enabling the new collector via
Netdata's amazing auto-detection feature.

We [fixed an error](https://github.com/netdata/netdata/pull/7220) where **invalid spikes** appeared on certain charts by
improving the incremental counter reset/wraparound detection algorithm.

Netdata can now send [**health alarm notifications to IRC
channels**](https://docs.netdata.cloud/health/notifications/irc/) thanks to [Strykar](https://github.com/Strykar)!

And, Netdata can now monitor [**AM2320 sensors**](https://docs.netdata.cloud/collectors/python.d.plugin/am2320/), thanks
to hard work from [Tom Buck](https://github.com/tommybuck).

---

See more news and previous releases at our [blog](https://blog.netdata.cloud) or our [releases
page](https://github.com/netdata/netdata/releases).

## How it works

Netdata is a highly efficient, highly modular, metrics management engine. Its lockless design makes it ideal for
concurrent operations on the metrics.

![image](https://user-images.githubusercontent.com/2662304/48323827-b4c17580-e636-11e8-842c-0ee72fcb4115.png)

This is how it works:

| Function    | Description                                                                                                                                                                                                                                                    | Documentation                                       |
| :---------- | :------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | :-------------------------------------------------- |
| **Collect** | Multiple independent data collection workers are collecting metrics from their sources using the optimal protocol for each application and push the metrics to the database. Each data collection worker has lockless write access to the metrics it collects. | [`collectors`](collectors/README.md)                |
| **Store**   | Metrics are first stored in RAM in a custom database engine that then "spills" historical metrics to disk for efficient long-term metrics storage.                                                                                                             | [`database`](database/README.md)                    |
| **Check**   | A lockless independent watchdog is evaluating **health checks** on the collected metrics, triggers alarms, maintains a health transaction log and dispatches alarm notifications.                                                                              | [`health`](health/README.md)                        |
| **Stream**  | A lockless independent worker is streaming metrics, in full detail and in real-time, to remote Netdata servers, as soon as they are collected.                                                                                                                 | [`streaming`](streaming/README.md)                  |
| **Archive** | A lockless independent worker is down-sampling the metrics and pushes them to **backend** time-series databases.                                                                                                                                               | [`backends`](backends/README.md)                    |
| **Query**   | Multiple independent workers are attached to the [internal web server](web/server/README.md), servicing API requests, including [data queries](web/api/queries/README.md).                                                                                     | [`web/api`](web/api/README.md)                      |

The result is a highly efficient, low-latency system, supporting multiple readers and one writer on each metric.

## Infographic

This is a high level overview of Netdata feature set and architecture. Click it to to interact with it (it has direct
links to our documentation).

[![image](https://user-images.githubusercontent.com/43294513/60951037-8ba5d180-a2f8-11e9-906e-e27356f168bc.png)](https://my-netdata.io/infographic.html)

## Features

![finger-video](https://user-images.githubusercontent.com/2662304/48346998-96cf3180-e685-11e8-9f4e-059d23aa3aa5.gif)

This is what you should expect from Netdata:

### General

-   **1s granularity** - The highest possible resolution for all metrics.
-   **Unlimited metrics** - Netdata collects all the available metrics—the more, the better.
-   **1% CPU utilization of a single core** - It's unbelievably optimized.
-   **A few MB of RAM** - The highly-efficient database engine stores per-second metrics in RAM and then "spills"
    historical metrics to disk long-term storage.   
-   **Minimal disk I/O** - While running, Netdata only writes historical metrics and reads `error` and `access` logs.
-   **Zero configuration** - Netdata auto-detects everything, and can collect up to 10,000 metrics per server out of the
    box.
-   **Zero maintenance** - You just run it. Netdata does the rest.
-   **Zero dependencies** - Netdata runs a custom web server for its static web files and its web API (though its
    plugins may require additional libraries, depending on the applications monitored).
-   **Scales to infinity** - You can install it on all your servers, containers, VMs, and IoT devices. Metrics are not
    centralized by default, so there is no limit.
-   **Several operating modes** - Autonomous host monitoring (the default), headless data collector, forwarding proxy,
    store and forward proxy, central multi-host monitoring, in all possible configurations. Each node may have different
    metrics retention policies and run with or without health monitoring.

### Health Monitoring & Alarms

-   **Sophisticated alerting** - Netdata comes with hundreds of alarms **out of the box**! It supports dynamic
    thresholds, hysteresis, alarm templates, multiple role-based notification methods, and more.
-   **Notifications**: [alerta.io](health/notifications/alerta/), [amazon sns](health/notifications/awssns/),
    [discordapp.com](health/notifications/discord/), [email](health/notifications/email/),
    [flock.com](health/notifications/flock/), [hangouts](health/notifications/hangouts/),
    [irc](health/notifications/irc/), [kavenegar.com](health/notifications/kavenegar/),
    [messagebird.com](health/notifications/messagebird/), [pagerduty.com](health/notifications/pagerduty/),
    [prowl](health/notifications/prowl/), [pushbullet.com](health/notifications/pushbullet/),
    [pushover.net](health/notifications/pushover/), [rocket.chat](health/notifications/rocketchat/),
    [slack.com](health/notifications/slack/), [smstools3](health/notifications/smstools3/),
    [syslog](health/notifications/syslog/), [telegram.org](health/notifications/telegram/),
    [twilio.com](health/notifications/twilio/), [web](health/notifications/web/) and [custom
    notifications](health/notifications/custom/).

### Integrations

-   **Time-series databases** - Netdata can archive its metrics to **Graphite**, **OpenTSDB**, **Prometheus**, **AWS
    Kinesis**, **MongoDB**, **JSON document DBs**, in the same or lower resolution (lower: to prevent it from congesting
    these servers due to the amount of data collected). Netdata also supports **Prometheus remote write API**, which
    allows storing metrics to **Elasticsearch**, **Gnocchi**, **InfluxDB**, **Kafka**, **PostgreSQL/TimescaleDB**,
    **Splunk**, **VictoriaMetrics** and a lot of other [storage
    providers](https://prometheus.io/docs/operating/integrations/#remote-endpoints-and-storage).

## Visualization

-   **Stunning interactive dashboards** - Our dashboard is mouse-, touchpad-, and touch-screen friendly in 2 themes:
    `slate` (dark) and `white`.
-   **Amazingly fast visualization** - Even on low-end hardware, the dashboard responds to all queries in less than 1 ms
    per metric.
-   **Visual anomaly detection** - Our UI/UX emphasizes the relationships between charts so you can better detect
    anomalies visually.
-   **Embeddable** - Charts can be embedded on your web pages, wikis and blogs. You can even use [Atlassian's Confluence
    as a monitoring dashboard](web/gui/confluence/).
-   **Customizable** - You can build custom dashboards using simple HTML. No JavaScript needed!

### Positive and negative values

To improve clarity on charts, Netdata dashboards present **positive** values for metrics representing `read`, `input`,
`inbound`, `received` and **negative** values for metrics representing `write`, `output`, `outbound`, `sent`.

![positive-and-negative-values](https://user-images.githubusercontent.com/2662304/48309090-7c5c6180-e57a-11e8-8e03-3a7538c14223.gif)

_Netdata charts showing the bandwidth and packets of a network interface. `received` is positive and `sent` is
negative._

### Autoscaled y-axis

Netdata charts automatically zoom vertically, to visualize the variation of each metric within the visible time-frame.

![non-zero-based](https://user-images.githubusercontent.com/2662304/48309139-3d2f1000-e57c-11e8-9a44-b91758134b00.gif)

_A zero-based `stacked` chart, automatically switches to an auto-scaled `area` chart when a single dimension is
selected._

### Charts are synchronized

Charts on Netdata dashboards are synchronized to each other. There is no master chart. Any chart can be panned or zoomed
at any time, and all other charts will follow.

![charts-are-synchronized](https://user-images.githubusercontent.com/2662304/48309003-b4fb3b80-e578-11e8-86f6-f505c7059c15.gif)

_Charts are panned by dragging them with the mouse. Charts can be zoomed in/out with`SHIFT` + `mouse wheel` while the
mouse pointer is over a chart._

> The visible time-frame (pan and zoom) is propagated from Netdata server to Netdata server when navigating via the
> [My nodes menu](registry/README.md).

### Highlighted time-frame

To improve visual anomaly detection across charts, the user can highlight a time-frame (by pressing `Alt` + `mouse
selection`) on all charts.

![highlighted-timeframe](https://user-images.githubusercontent.com/2662304/48311876-f9093300-e5ae-11e8-9c74-e3e291741990.gif)

_A highlighted time-frame can be given by pressing `Alt` + `mouse selection` on any chart. Netdata will highlight the
same range on all charts._

> Highlighted ranges are propagated from Netdata server to Netdata server, when navigating via the [My nodes
> menu](registry/README.md).

## What Netdata monitors

Netdata can collect metrics from 200+ popular services and applications, on top of dozens of system-related metrics
jocs, such as CPU, memory, disks, filesystems, networking, and more. We call these **collectors**, and they're managed
by [**plugins**](collectors/plugins.d/), which support a variety of programming languages, including Go and Python.

Popular collectors include **Nginx**, **Apache**, **MySQL**, **statsd**, **cgroups** (containers, Docker, Kubernetes,
LXC, and more), **Traefik**, **web server `access.log` files**, and much more. 

See the **full list of [supported collectors](collectors/COLLECTORS.md)**.

Netdata's data collection is **extensible**, which means you can monitor anything you can get a metric for. You can even
write a collector for your custom application using our [plugin API](collectors/plugins.d/README.md).

## Documentation

The Netdata documentation is at <https://docs.netdata.cloud>, but you can also find each page inside of Netdata's
repository itself in Markdown (`.md`) files. You can find all our documentation by navigating the repository.

Here is a quick list of notable documents:

| Directory                                   | Description                                                                                                           |
| :------------------------------------------ | :-------------------------------------------------------------------------------------------------------------------- |
| [`installer`](packaging/installer/)         | Instructions to install Netdata on your systems.                                                                      |
| [`docker`](packaging/docker/)               | Instructions to install Netdata using docker.                                                                         |
| [`daemon`](daemon/)                         | Information about the Netdata daemon and its configuration.                                                           |
| [`collectors`](collectors/)                 | Information about data collection plugins.                                                                            |
| [`health`](health/)                         | How Netdata's health monitoring works, how to create your own alarms and how to configure alarm notification methods. |
| [`streaming`](streaming/)                   | How to build hierarchies of Netdata servers, by streaming metrics between them.                                       |
| [`backends`](backends/)                     | Long term archiving of metrics to industry-standard time-series databases, like `prometheus`, `graphite`, `opentsdb`. |
| [`web/api`](web/api/)                       | Learn how to query the Netdata API and the queries it supports.                                                       |
| [`web/api/badges`](web/api/badges/)         | Learn how to generate badges (SVG images) from live data.                                                             |
| [`web/gui/custom`](web/gui/custom/)         | Learn how to create custom Netdata dashboards.                                                                        |
| [`web/gui/confluence`](web/gui/confluence/) | Learn how to create Netdata dashboards on Atlassian's Confluence.                                                     |

You can also check all the other directories. Most of them have plenty of documentation.

## Community

We welcome [contributions](CONTRIBUTING.md). Feel free to join the team!

To report bugs or get help, use [GitHub's issues](https://github.com/netdata/netdata/issues).

You can also find Netdata on:

-   [Facebook](https://www.facebook.com/linuxnetdata/)
-   [Twitter](https://twitter.com/linuxnetdata)
-   [StackShare](https://stackshare.io/netdata)
-   [LinkedIn](https://www.linkedin.com/company/my-netdata.io/)
-   [Product Hunt](https://www.producthunt.com/posts/netdata-monitoring-agent/)
-   [Repology](https://repology.org/metapackage/netdata/versions)

## License

Netdata is [GPLv3+](LICENSE).

Netdata re-distributes other open-source tools and libraries. Please check the [third party licenses](REDISTRIBUTED.md).

## Is it any good?

Yes.

_When people first hear about a new product, they frequently ask if it is any good. A Hacker News user
[remarked](https://news.ycombinator.com/item?id=3067434):_

> Note to self: Starting immediately, all raganwald projects will have a “Is it any good?” section in the readme, and
> the answer shall be “yes.".

So, we follow the tradition...

## Is it awesome?

[These people](https://github.com/netdata/netdata/stargazers) seem to like it.
