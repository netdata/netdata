<p align="center">
<a href="https://www.netdata.cloud#gh-light-mode-only">
  <img src="https://www.netdata.cloud/img/readme-images/netdata_readme_logo_light.png" alt="Netdata" width="300"/>
</a>
<a href="https://www.netdata.cloud#gh-dark-mode-only">
  <img src="https://www.netdata.cloud/img/readme-images/netdata_readme_logo_dark.png" alt="Netdata" width="300"/>
</a>
</p>
<h3 align="center">Monitor your servers, containers, and applications<br/>in high-resolution and in real-time.</h3>

<br />
<p align="center">
  <a href="https://github.com/netdata/netdata/"><img src="https://img.shields.io/github/stars/netdata/netdata?style=social" alt="GitHub Stars"></a>
  <br />
  <a href="https://app.netdata.cloud/spaces/netdata-demo?utm_campaign=github_readme_demo_badge"><img src="https://img.shields.io/badge/Live%20Demo-green" alt="Live Demo"></a>
  <a href="https://github.com/netdata/netdata/releases/latest"><img src="https://img.shields.io/github/release/netdata/netdata.svg" alt="Latest release"></a>
  <a href="https://github.com/netdata/netdata-nightlies/releases/latest"><img src="https://img.shields.io/github/release/netdata/netdata-nightlies.svg" alt="Latest nightly build"></a>
  <br/>
  <a href="https://community.netdata.cloud"><img alt="Discourse topics" src="https://img.shields.io/discourse/topics?server=https%3A%2F%2Fcommunity.netdata.cloud%2F&logo=discourse&label=discourse%20forum"></a>
  <a href="https://github.com/netdata/netdata/discussions"><img alt="GitHub Discussions" src="https://img.shields.io/github/discussions/netdata/netdata?logo=github&label=github%20discussions"></a>
  <br/>
  <a href="https://bestpractices.coreinfrastructure.org/projects/2231"><img src="https://bestpractices.coreinfrastructure.org/projects/2231/badge" alt="CII Best Practices"></a>
  <a href="https://scan.coverity.com/projects/netdata-netdata?tab=overview"><img alt="Coverity Scan" src="https://img.shields.io/coverity/scan/netdata"></a>
</p>

<p align="center"><b>Visit the <a href="https://www.netdata.cloud">Project's Home Page</a></b></p>

<hr class="solid">

MENU: **[GETTING STARTED](#getting-started)** | **[HOW IT WORKS](#how-it-works)** | **[FAQ](#faq)** | **[DOCS](#book-documentation)** | **[COMMUNITY](#tada-community)** | **[CONTRIBUTE](#pray-contribute)** | **[LICENSE](#license)**

> **Important** :bulb:<br/>
> People get addicted to Netdata. Once you use it on your systems, **there's no going back!**<br/>

**Netdata: Real-time Observability, Simplified.**

[![Platforms](https://img.shields.io/badge/Platforms-Linux%20%7C%20macOS%20%7C%20FreeBSD%20%7C%20Windows-blue)]()

Netdata is a high-performance observability platform designed to simplify modern infrastructure monitoring. With its innovative distributed architecture, Netdata delivers real-time insights into your systems, containers, and applications at a granular level.

**:sparkles: Key Features**:

- **Real-Time**: Per-second data collection provides immediate visibility into your infrastructure's behavior.
- **Zero-Configuration**: Start monitoring in minutes with automatic detection and instant insights.
- **ML-Powered Insights**: Automatic anomaly detection and pattern recognition, helping you identify issues before they become critical.
- **Enterprise-Ready**: Scale from a single node to thousands while maintaining performance and ease of use.
- **Complete Visibility**: From infrastructure to applications, logs to metrics, all in one solution.
- **Edge-Based**: Process and store metrics at the edge for superior performance and cost efficiency.
- **Advanced Visualization**: Rich, interactive dashboard for deep system insights and rapid troubleshooting.

---

### The Netdata Ecosystem

> **Note**: This repository contains the Netdata Agent, the open-source core of the Netdata ecosystem. For information about other components, see below.

This three-part architecture enables Netdata to scale seamlessly from single-node deployments to complex multi-cloud environments with thousands of nodes, supporting long-term data retention without compromising performance.

| Component     | Description                                                                                                                                                                                                                                                                                                                            | License                                         |
|---------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-------------------------------------------------|
| Netdata Agent | • The heart of Netdata's monitoring capabilities<br/>• Handles data collection, storage, querying, ML analysis, exports, and alerts<br/>• Runs on physical/virtual servers, cloud, Kubernetes, and IoT devices<br/>• Optimized for zero production impact<br/>• Core of all observability features                                     | [GPL v3+](https://www.gnu.org/licenses/gpl-3.0) |
| Netdata Cloud | • Adds enterprise-grade features:<br/>  &emsp; - User management and RBAC<br/>  &emsp; - Horizontal scalability<br/>  &emsp; - Centralized alert management<br/> &emsp;  - Access your infrastructure from anywhere<br/>• Available as SaaS or on-premises<br/>• Includes free community tier<br/>• Does not centralize metric storage |                                                 |
| Netdata UI    | • Powers all dashboards and visualizations<br/>• Free to use with both Agent and Cloud<br/>• Included in standard Netdata packages<br/>• Latest version available via CDN                                                                                                                                                              | [NCUL1](https://app.netdata.cloud/LICENSE.txt)  |

### Key capabilities of the Netdata Agent

With these capabilities, Netdata Agent provides a powerful, automated monitoring solution that works right out-of-the-box while remaining highly customizable for specific needs.

| Capability                    | Description                                                                                                                                                                                                                     |
|-------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Comprehensive Data Collection | • 800+ integrations out of the box<br/>• Collects metrics from systems, containers, VMs, hardware sensors<br/>• Supports OpenMetrics exporters, StatsD, and logs<br/>• OpenTelemetry support coming soon                        |
| Performance & Precision       | • Per-second data collection<br/>• Real-time visualization with 1-second latency<br/>• High-resolution metrics for precise monitoring                                                                                           |
| Edge-Based ML                 | • Trains ML models directly at the edge<br/>• Automatic anomaly detection per metric<br/>• Pattern recognition based on historical behavior                                                                                     |
| Advanced Log Management       | • Direct systemd-journald and Windows Event Log integrations<br/>• Tools for log conversion (log2journal, systemd-cat-native)<br/>• Process logs at the edge - no centralization needed<br/>• Rich log visualization dashboards |
| Observability Pipeline        | • Build Parent-Child relationships between Agents<br/>• Create flexible centralization points<br/>• Control data replication and retention at multiple levels                                                                   |
| Automated Visualization       | • NIDL (Nodes, Instances, Dimensions & Labels) data model<br/>• Auto-generated, correlated dashboards<br/>• Filter and analyze data without query language<br/>• Free to use, powered by Netdata UI                             |
| Smart Alerting                | • Hundreds of pre-configured alerts<br/>• Detect common issues automatically<br/>• Multiple notification methods<br/>• Proactive problem detection                                                                              |
| Low Maintenance               | • Auto-detection of metrics<br/>• Zero-touch machine learning<br/>• Easy scalability<br/>• CI/CD friendly deployment                                                                                                            |
| Open & Extensible             | • Modular architecture<br/>• Easy to extend and customize<br/>• Integrates with existing monitoring tools<br/>• Active community ecosystem                                                                                      |

### What can be monitored with the Netdata Agent

Netdata monitors all the following:

|                                                                                                   Component |              Linux               | FreeBSD | macOS |                                     Windows                                     |
|------------------------------------------------------------------------------------------------------------:|:--------------------------------:|:-------:|:-----:|:-------------------------------------------------------------------------------:|
|                             **System Resources**<small><br/>CPU, Memory and system shared resources</small> |               Full               |   Yes   |  Yes  |                                       Yes                                       |
|                                **Storage**<small><br/>Disks, Mount points, Filesystems, RAID arrays</small> |               Full               |   Yes   |  Yes  |                                       Yes                                       |
|                                 **Network**<small><br/>Network Interfaces, Protocols, Firewall, etc</small> |               Full               |   Yes   |  Yes  |                                       Yes                                       |
|                        **Hardware & Sensors**<small><br/>Fans, Temperatures, Controllers, GPUs, etc</small> |               Full               |  Some   | Some  |                                      Some                                       |
|                                       **O/S Services**<small><br/>Resources, Performance and Status</small> | Yes<small><br/>`systemd`</small> |    -    |   -   |                                        -                                        |
|                                      **Processes**<small><br/>Resources, Performance, OOM, and more</small> |               Yes                |   Yes   |  Yes  |                                       Yes                                       |
|                                                                             System and Application **Logs** | Yes<small><br/>`systemd`-journal |    -    |   -   | Yes<small><br/>`Windows Event Log`, and<br/>`Event Tracing for Windows`</small> |
|                                 **Network Connections**<small><br/>Live TCP and UDP sockets per PID</small> |               Yes                |    -    |   -   |                                        -                                        |
|                               **Containers**<small><br/>Docker/containerd, LXC/LXD, Kubernetes, etc</small> |               Yes                |    -    |   -   |                                        -                                        |
|                                 **VMs** (from the host)<small><br/>KVM, qemu, libvirt, Proxmox, etc</small> | Yes<small><br/>`cgroups`</small> |    -    |   -   |                        Yes<small><br/>`Hyper-V`</small>                         |
|                       **Synthetic Checks**<small><br/>Test APIs, TCP ports, Ping, Certificates, etc</small> |               Yes                |   Yes   |  Yes  |                                       Yes                                       |
| **Packaged Applications**<small><br/>nginx, apache, postgres, redis, mongodb,<br/>and hundreds more</small> |               Yes                |   Yes   |  Yes  |                                       Yes                                       |
|                              **Cloud Provider Infrastructure**<small><br/>AWS, GCP, Azure, and more</small> |               Yes                |   Yes   |  Yes  |                                       Yes                                       |
|                       **Custom Applications**<small><br/>OpenMetrics, StatsD and soon OpenTelemetry</small> |               Yes                |   Yes   |  Yes  |                                       Yes                                       |

When the Netdata Agent runs on Linux, it monitors every kernel feature available, providing full coverage of all kernel technologies and offers full **enterprise hardware** coverage, monitoring all components that provide hardware error reporting, like PCI AER, RAM EDAC, IPMI, S.M.A.R.T., NVMe, Fans, Power, Voltages, and more.

---

### :star: Netdata is the most energy-efficient monitoring tool :star:

<p align="center">
<a href="https://www.ivanomalavolta.com/files/papers/ICSOC_2023.pdf#gh-dark-mode-only">
  <img src="https://github.com/netdata/netdata/assets/139226121/7118757a-38fb-48d7-b12a-53e709a8e8c0" alt="Energy Efficiency" width="1000"/>
</a>
<a href="https://www.ivanomalavolta.com/files/papers/ICSOC_2023.pdf#gh-light-mode-only">
  <img src="https://github.com/netdata/netdata/assets/139226121/4f64cbb6-05e4-48e3-b7c0-d1b79e37e219" alt="Energy efficiency" width="1000"/>
</a>
</p>

Dec 11, 2023: [University of Amsterdam published a study](https://twitter.com/IMalavolta/status/1734208439096676680) related to the impact of monitoring tools for Docker based systems, aiming to answer 2 questions:

1. **The impact of monitoring on the energy efficiency of Docker-based systems**
2. **The impact of monitoring on Docker-based systems?**

- 🚀 Netdata excels in energy efficiency: **"... Netdata is the most energy-efficient tool ..."**, as the study says.
- 🚀 Netdata excels in CPU Usage, RAM Usage and Execution Time, and has a similar impact on Network Traffic as Prometheus.

The study didn’t normalize the results based on the number of metrics collected. Given that Netdata usually collects significantly more metrics than the other tools, Netdata managed to outperform the other tools, while ingesting a much higher number of metrics. [Read the full study here](https://www.ivanomalavolta.com/files/papers/ICSOC_2023.pdf).

---

### Netdata vs Prometheus 2025 Review

<p align="center">
<a href="https://blog.netdata.cloud/netdata-vs-prometheus-performance-analysis#gh-light-mode-only">
  <img src="https://github.com/netdata/netdata/assets/139226121/6c21ae39-8656-45c3-bc85-4b012679d2bb" alt="Netdata" width="1000"/>
</a>
<a href="https://blog.netdata.cloud/netdata-vs-prometheus-performance-analysis#gh-dark-mode-only">
  <img src="https://github.com/netdata/netdata/assets/139226121/f2dbde46-d3dd-4807-bd34-966da4d0ec22" alt="Netdata" width="1000"/>
</a>
</p>

NEW! On the same workload, Netdata uses **1/3rd less CPU**, consumes **1/8th of the RAM**, performes **31 times less disk I/O** and stores **40 times more data** while being up to **22 times faster** in query responses! [Read the full 2025 review in our blog](https://www.netdata.cloud/blog/netdata-vs-prometheus-2025/).

---

&nbsp;<br/>
<p align="center">
  <img src="https://raw.githubusercontent.com/cncf/artwork/master/other/cncf/horizontal/white/cncf-white.svg#gh-dark-mode-only" alt="CNCF" width="300">
  <img src="https://raw.githubusercontent.com/cncf/artwork/master/other/cncf/horizontal/black/cncf-black.svg#gh-light-mode-only" alt="CNCF" width="300">
  <br />
  Netdata actively supports and is a member of the Cloud Native Computing Foundation (CNCF)<br />
  &nbsp;<br/>
  ...and due to your love :heart:, it is one of the most :star:'d projects in the <a href="https://landscape.cncf.io/?item=observability-and-analysis--observability--netdata">CNCF landscape</a>!
</p>
&nbsp;<br/>

<hr class="solid">

<p align="center">
  <b>You can see Netdata live!</b><br/>
	<a href="https://frankfurt.netdata.rocks"><b>FRANKFURT</b></a> |
	<a href="https://newyork.netdata.rocks"><b>NEWYORK</b></a> |
	<a href="https://atlanta.netdata.rocks"><b>ATLANTA</b></a> |
	<a href="https://sanfrancisco.netdata.rocks"><b>SANFRANCISCO</b></a> |
	<a href="https://toronto.netdata.rocks"><b>TORONTO</b></a> |
	<a href="https://singapore.netdata.rocks"><b>SINGAPORE</b></a> |
	<a href="https://bangalore.netdata.rocks"><b>BANGALORE</b></a>
  <br/>
  	<i>We've set up multiple demo clusters around the world, each running with the default configuration and showing real monitoring data.</i>
  <br/>
    <i>Choose the instance closest to you for the best experience.</i>
</p>

<hr class="solid">

## Getting Started

<p align="center">
  <a href="https://registry.my-netdata.io/#menu_netdata_submenu_registry"><img src="https://registry.my-netdata.io/api/v3/badge.svg?chart=netdata.registry_entries&dimensions=persons&label=user%20base&units=M&value_color=blue&precision=2&divide=1000000&options=unaligned&tier=1&v44" alt="User base"></a>
  <a href="https://registry.my-netdata.io/#menu_netdata_submenu_registry"><img src="https://registry.my-netdata.io/api/v3/badge.svg?chart=netdata.registry_entries&dimensions=machines&label=servers%20monitored&units=M&divide=1000000&value_color=orange&precision=2&options=unaligned&tier=1&v44" alt="Servers monitored"></a>
  <a href="https://registry.my-netdata.io/#menu_netdata_submenu_registry"><img src="https://registry.my-netdata.io/api/v3/badge.svg?chart=netdata.registry_sessions&label=sessions%20served&units=M&value_color=yellowgreen&precision=2&divide=1000000&options=unaligned&tier=1&v44" alt="Sessions served"></a>
  <a href="https://hub.docker.com/r/netdata/netdata"><img src="https://registry.my-netdata.io/api/v3/badge.svg?chart=dockerhub.pulls_sum&divide=1000000&precision=1&units=M&label=docker+hub+pulls&options=unaligned&tier=1&v44" alt="Docker Hub pulls"></a>
  <br />
  <a href="https://registry.my-netdata.io/#menu_netdata_submenu_registry"><img src="https://registry.my-netdata.io/api/v3/badge.svg?chart=netdata.registry_entries&dimensions=persons&after=-86400&options=unaligned&group=incremental-sum&label=new%20users%20today&units=null&value_color=blue&precision=0&options=unaligned&tier=1&v44" alt="New users today"></a>
  <a href="https://registry.my-netdata.io/#menu_netdata_submenu_registry"><img src="https://registry.my-netdata.io/api/v3/badge.svg?chart=netdata.registry_entries&dimensions=machines&group=incremental-sum&after=-86400&options=unaligned&label=servers%20added%20today&units=null&value_color=orange&precision=0&tier=1&v44" alt="New machines today"></a>
  <a href="https://registry.my-netdata.io/#menu_netdata_submenu_registry"><img src="https://registry.my-netdata.io/api/v3/badge.svg?chart=netdata.registry_sessions&after=-86400&group=incremental-sum&options=unaligned&label=sessions%20served%20today&units=null&value_color=yellowgreen&precision=0&tier=1&v44" alt="Sessions today"></a>
  <a href="https://hub.docker.com/r/netdata/netdata"><img src="https://registry.my-netdata.io/api/v3/badge.svg?chart=dockerhub.pulls_sum&divide=1000&precision=1&units=k&label=docker+hub+pulls&after=-86400&group=incremental-sum&label=docker%20hub%20pulls%20today&options=unaligned&tier=1&v44" alt="Docker Hub pulls today"></a>
</p>

### 1. **Install Netdata everywhere** :v:

Netdata can be installed on all Linux, macOS, FreeBSD (and soon on Windows) systems. We provide binary packages for the most popular operating systems and package managers.

- Install on [Ubuntu, Debian CentOS, Fedora, Suse, Red Hat, Arch, Alpine, Gentoo, even BusyBox](https://learn.netdata.cloud/docs/installing/one-line-installer-for-all-linux-systems).
- Install with [Docker](/packaging/docker/README.md).<br/>
  Netdata is a [Verified Publisher on DockerHub](https://hub.docker.com/r/netdata/netdata) and our users enjoy free unlimited DockerHub pulls :heart_eyes:.
- Install on [macOS](https://learn.netdata.cloud/docs/installing/macos) :metal:.
- Install on [FreeBSD](https://learn.netdata.cloud/docs/installing/freebsd) and [pfSense](https://learn.netdata.cloud/docs/installing/pfsense).
- Install [from source](https://learn.netdata.cloud/docs/installing/build-the-netdata-agent-yourself/compile-from-source-code) ![github downloads](https://img.shields.io/github/downloads/netdata/netdata/total?color=success&logo=github)
- For Kubernetes deployments [check here](https://learn.netdata.cloud/docs/installation/install-on-specific-environments/kubernetes/).

Check also the [Netdata Deployment Guides](https://learn.netdata.cloud/docs/deployment-guides/) to decide how to deploy it in your infrastructure.

By default, you will have immediately available a local dashboard. Netdata starts a web server for its dashboard at port `19999`. Open up your web browser of choice and
navigate to `http://NODE:19999`, replacing `NODE` with the IP address or hostname of your Agent. If installed on localhost, you can access it through `http://localhost:19999`.

_Note: the binary packages we provide, install Netdata UI automatically. Netdata UI is closed-source, but free to use with Netdata Agents and Netdata Cloud._

### 2. **Configure Collectors** :boom:

Netdata auto-detects and auto-discovers most operating system data sources and applications. However, many data sources require some manual configuration, usually to allow Netdata to get access to the metrics.

- For a detailed list of the 800+ collectors available, check [this guide](https://learn.netdata.cloud/docs/data-collection/).
- To monitor Windows servers and applications, use [this guide](https://learn.netdata.cloud/docs/data-collection/monitor-anything/system-metrics/windows-machines).<br/><small>Note that Netdata on Windows is at its final release stage, so at the next Netdata release Netdata will natively support Windows.</small>
- To monitor SNMP devices, check [this guide](https://learn.netdata.cloud/docs/data-collection/monitor-anything/networking/snmp).

### 3. **Configure Alert Notifications** :bell:

Netdata comes with hundreds of pre-configured alerts that automatically check your metrics immediately after they start getting collected.

Netdata can dispatch alert notifications to multiple third party systems, including: `email`, `Alerta`, `AWS SNS`, `Discord`, `Dynatrace`, `flock`, `gotify`, `IRC`, `Matrix`, `MessageBird`, `Microsoft Teams`, `ntfy`, `OPSgenie`, `PagerDuty`, `Prowl`, `PushBullet`, `PushOver`, `RocketChat`, `Slack`, `SMS tools`, `Syslog`, `Telegram`, `Twilio`.

By default, Netdata will send e-mail notifications if there is a configured MTA on the system.

### 4. **Configure Netdata Parents** :family:

Optionally, configure one or more Netdata Parents. A Netdata Parent is a Netdata Agent that has been configured to accept [streaming connections](https://learn.netdata.cloud/docs/streaming/streaming-configuration-reference) from other Netdata Agents.

Netdata Parents provide:

- **Infrastructure level dashboards, at `http://parent.server.ip:19999/`.**<br/>

  Each Netdata Agent has an API listening at the TCP port 19999 of each server.
  When you hit that port with a web browser (e.g. `http://server.ip:19999/`), the Netdata Agent UI is presented.
  When the Netdata Agent is also a Parent, the UI of the Parent includes data for all nodes that stream metrics to that Parent.

- **Increased retention for all metrics of all your nodes.**<br/>

  Each Netdata Agent maintains each own database of metrics. But Parents can be given additional resources to maintain a much longer database than
  individual Netdata Agents.

- **Central configuration of alerts and dispatch of notifications.**<br/>

  Using Netdata Parents, all the alert notifications integrations can be configured only once at the Parent and they can be disabled at the Netdata Agents.

You can also use Netdata Parents to:

- Offload your production systems (the parents run ML, alerts, queries, etc. for all their children)
- Secure your production systems (the parents accept user connections for all their children)

### 5. **Connect to Netdata Cloud** :cloud:

[Sign-in](https://app.netdata.cloud/sign-in) to [Netdata Cloud](https://www.netdata.cloud/) and connect your Netdata Agents and Parents.
If you connect your Netdata Parents, there is no need to connect your Netdata Agents. They will be connected via the Parents.

When your Netdata nodes are connected to Netdata Cloud, you can (on top of the above):

- Access your Netdata Agents from anywhere
- Access sensitive Netdata Agent features (like "Netdata Functions": processes, systemd-journal)
- Organize your infra in spaces and Rooms
- Create, manage, and share **custom dashboards**
- Invite your team and assign roles to them (Role-Based Access Control)
- Get infinite horizontal scalability (multiple independent Netdata Agents are viewed as one infra)
- Configure alerts from the UI
- Configure data collection from the UI
- Netdata Mobile App notifications

:love_you_gesture: Netdata Cloud doesn’t prevent you from using your Netdata Agents and Parents directly, and vice versa.<br/>

:ok_hand: Your metrics are still stored in your network when you connect your Netdata Agents and Parents to Netdata Cloud.

<hr class="solid">

## How it works

Netdata is built around a **modular metrics processing pipeline**.

<details><summary>Click to see more details about this pipeline...</summary>
&nbsp;<br/>

Each Netdata Agent can perform the following functions:

1. **`COLLECT` metrics from their sources**<br/>
   Uses [internal](https://github.com/netdata/netdata/tree/master/src/collectors) and [external](https://github.com/netdata/go.d.plugin/tree/master/modules) plugins to collect data from their sources.

   Netdata auto-detects and collects almost everything from the operating system: including CPU, Interrupts, Memory, Disks, Mount Points, Filesystems, Network Stack, Network Interfaces, Containers, VMs, Processes, `systemd` units, Linux Performance Metrics, Linux eBPF, Hardware Sensors, IPMI, and more.

   It collects application metrics from applications: PostgreSQL, MySQL/MariaDB, Redis, MongoDB, Nginx, Apache, and hundreds more.

   Netdata also collects your custom application metrics by scraping OpenMetrics exporters, or via StatsD.

   It can convert web server log files to metrics and apply ML and alerts to them in real-time.

   And it also supports synthetic tests / white box tests, so you can ping servers, check API responses, or even check filesystem files and directories to generate metrics, train ML and run alerts and notifications on their status.

2. **`STORE` metrics to a database**<br/>
   Uses database engine plugins to store the collected data, either in memory and/or on disk. We have developed our own [`dbengine`](https://github.com/netdata/netdata/tree/master/src/database/engine#readme) for storing the data in a very efficient manner, allowing Netdata to have less than one byte per sample on disk and amazingly fast queries.

3. **`LEARN` the behavior of metrics** (ML)<br/>
   Trains multiple Machine-Learning (ML) models per metric to learn the behavior of each metric individually. Netdata uses the `kmeans` algorithm and creates by default a model per metric per hour, based on the values collected for that metric over the last 6 hours. The trained models are persisted to disk.

4. **`DETECT` anomalies in metrics** (ML)<br/>
   Uses the trained machine learning (ML) models to detect outliers and mark collected samples as **anomalies**. Netdata stores anomaly information together with each sample and also streams it to Netdata Parents so that the anomaly is also available at query time for the whole retention of each metric.

5. **`CHECK` metrics and trigger alert notifications**<br/>
   Uses its configured alerts (you can configure your own) to check the metrics for common issues and uses notification plugins to send alert notifications.

6. **`STREAM` metrics to other Netdata Agents**<br/>
   Push metrics in real-time to Netdata Parents.

7. **`ARCHIVE` metrics to third party databases**<br/>
   Export metrics to industry standard time-series databases, like `Prometheus`, `InfluxDB`, `OpenTSDB`, `Graphite`, etc.

8. **`QUERY` metrics and present dashboards**<br/>
   Provide an API to query the data and present interactive dashboards to users.

9. **`SCORE` metrics to reveal similarities and patterns**<br/>
   Score the metrics according to the given criteria, to find the needle in the haystack.

When using Netdata Parents, all the functions of a Netdata Agent (except data collection) can be delegated to Parents to offload production systems.

The core of Netdata is developed in C. We have our own `libnetdata`, that provides:

- **`DICTIONARY`**<br/>
  A high-performance algorithm to maintain both indexed and ordered pools of structures Netdata needs. It uses JudyHS arrays for indexing, although it is modular: any hashtable or tree can be integrated into it. Despite being in C, dictionaries follow object-oriented programming principles, so there are constructors, destructors, automatic memory management, garbage collection, and more. For more, see [here](https://github.com/netdata/netdata/tree/master/src/libnetdata/dictionary).

- **`ARAL`**<br/>
  ARray ALlocator (ARAL) is used to minimize the system allocations made by Netdata. ARAL is optimized for maximum multithreaded performance. It also allows all structures that use it to be allocated in memory-mapped files (shared memory) instead of RAM. For more, see [here](https://github.com/netdata/netdata/tree/master/src/libnetdata/aral).

- **`PROCFILE`**<br/>
  A high-performance `/proc` (but also any) file parser and text tokenizer. It achieves its performance by keeping files open and adjusting its buffers to read the entire file in one call (which is also required by the Linux kernel). For more, see [here](https://github.com/netdata/netdata/tree/master/src/libnetdata/procfile).

- **`STRING`**<br/>
  A string internet mechanism, for string deduplication and indexing (using JudyHS arrays), optimized for multithreaded usage. For more, see [here](https://github.com/netdata/netdata/tree/master/src/libnetdata/string).

- **`ARL`**<br/>
  Adaptive Resortable List (ARL) is a very fast list iterator, that keeps the expected items on the list in the same order they are found in an input list. So, the first iteration is somewhat slower, but all the following iterations are perfectly aligned for the best performance. For more, see [here](https://github.com/netdata/netdata/tree/master/src/libnetdata/adaptive_resortable_list).

- **`BUFFER`**<br/>
  A flexible text buffer management system that allows Netdata to automatically handle dynamically sized text buffer allocations. The same mechanism is used for generating consistent JSON output by the Netdata APIs. For more, see [here](https://github.com/netdata/netdata/tree/master/src/libnetdata/buffer).

- **`SPINLOCK`**<br/>
  Like POSIX `MUTEX` and `RWLOCK` but a lot faster, based on atomic operations, with significantly smaller memory impact, while being portable.

- **`PGC`**<br/>
  A caching layer that can be used to cache any kind of time-related data, with automatic indexing (based on a tree of JudyL arrays), memory management, evictions, flushing, pressure management. This is extensively used in `dbengine`. For more, see [here](/src/database/engine/README.md).

The above, and many more, allow Netdata developers to work on the application fast and with confidence. Most of the business logic in Netdata is a work of mixing the above.

Netdata data collection plugins can be developed in any language. Most of our application collectors though are developed in [Go](https://github.com/netdata/go.d.plugin).

</details>

## FAQ

### :shield: Is Netdata secure?

Of course, it is! We do our best to ensure it is!

<details><summary>Click to see detailed answer ...</summary>
&nbsp;<br/>&nbsp;<br/>

We understand that Netdata is a software piece installed on millions of production systems across the world. So, it is important for us, Netdata to be as secure as possible:

- We follow the [Open Source Security Foundation](https://bestpractices.coreinfrastructure.org/en/projects/2231) best practices.
- We have given great attention to detail when it comes to security design. Check out our [security design](/docs/security-and-privacy-design/README.md).
- Netdata is a popular open-source project and is frequently tested by many security analysts.
- Check also our [security policies and advisories published so far](https://github.com/netdata/netdata/security).

&nbsp;<br/>&nbsp;<br/>
</details>

### :cyclone: Will Netdata consume significant resources on my servers?

No, it will not! We promise this will be fast!

<details><summary>Click to see detailed answer ...</summary>
&nbsp;<br/>&nbsp;<br/>

Although each Netdata Agent is a complete monitoring solution packed into a single application, and despite the fact that Netdata collects **every metric every single second** and trains **multiple ML models** per metric, you will find that Netdata has amazing performance! In many cases, it outperforms other monitoring solutions that have significantly fewer features or far smaller data collection rates.

This is what you should expect:

- For production systems, each Netdata Agent with default settings (everything enabled, ML, Health, DB) should consume about 5% CPU utilization of one core and about 150 MiB or RAM.

  By using a Netdata parent and streaming all metrics to that parent, you can disable ML & health and use an ephemeral DB (like `alloc`) on the children, leading to utilization of about 1% CPU of a single core and 100 MiB of RAM. Of course, these depend on how many metrics are collected.

- For Netdata Parents, for about 1 to 2 million metrics, all collected every second, we suggest a server with 16 cores and 32GB RAM. Less than half of it will be used for data collection and ML. The rest will be available for queries.

Netdata has extensive internal instrumentation to help us reveal how the resources consumed are used. All these are available in the "Netdata Monitoring" section of the dashboard. Depending on your use case, there are many options to optimize resource consumption.

Even if you need to run Netdata on extremely weak embedded or IoT systems, you will find that Netdata can be tuned to be very performant.

&nbsp;<br/>&nbsp;<br/>
</details>

### :scroll: How much retention can I have?

As much as you need!

<details><summary>Click to see detailed answer ...</summary>
&nbsp;<br/>&nbsp;<br/>

Netdata supports **tiering**, to downsample past data and save disk space. With default settings, it has three tiers:

1. `tier 0`, with high resolution, per-second, data.
2. `tier 1`, mid-resolution, per minute, data.
3. `tier 2`, low-resolution, per hour, data.

All tiers are updated in parallel during data collection. Increase the disk space you give to Netdata to get a longer history for your metrics. Tiers are automatically chosen at query time depending on the time frame and the resolution requested.

&nbsp;<br/>&nbsp;<br/>
</details>

### :rocket: Does it scale? I really have a lot of servers!

Netdata is designed to scale and can handle large volumes of data.

<details><summary>Click to see detailed answer ...</summary>
&nbsp;<br/>&nbsp;<br/>
Netdata is a distributed monitoring solution. You can scale it to infinity by spreading Netdata Agents across your infrastructure.

With the streaming feature of the Agent, we can support monitoring ephemeral servers but also allow the creation of "monitoring islands" where metrics are aggregated to a few servers (Netdata Parents) for increased retention, or for offloading production systems.

- :airplane: Netdata Parents provide great vertical scalability, so you can have as big parents as the CPU, RAM and Disk resources you can dedicate to them. In our lab, we constantly stress test Netdata Parents with several million metrics collected per second, to ensure it is reliable, stable, and robust at scale.

- :rocket: In addition, Netdata Cloud provides virtually unlimited horizontal scalability. It "merges" all the Netdata parents you have into one unified infrastructure at query time. Netdata Cloud itself is probably the biggest single installation monitoring platform ever created, currently monitoring about 100k online servers with about 10k servers changing state (added/removed) per day!

Example: the following chart comes from a single Netdata Parent. As you can see on it, 244 nodes stream to it metrics of about 20k running containers. On this specific chart, there are three dimensions per container, so a total of about 60k time-series queries are executed to present it.

![image](https://github.com/netdata/netdata/assets/2662304/33db4aed-86af-4018-a547-e70643308f25)

&nbsp;<br/>&nbsp;<br/>
</details>

### :floppy_disk: My production servers are very sensitive in disk I/O. Can I use Netdata?

Yes, you can!

<details><summary>Click to see detailed answer ...</summary>
&nbsp;<br/>&nbsp;<br/>

The Netdata Agent has been designed to spread disk writes across time. Each metric is flushed to disk every 17 minutes (1000 seconds), but metrics are flushed evenly across time, at an almost constant rate. Also, metrics are packed into bigger blocks we call `extents` and are compressed with ZSTD before saving them, to minimize the number of I/O operations made.

The Netdata Agent also employs direct I/O for all its database operations. By managing its own caches, Netdata avoids overburdening system caches, facilitating a harmonious coexistence with other applications.

Single node Agents (not Parents), should have a constant write rate of about 50 KiB/s or less, with some spikes above that every minute (flushing of tier 1) and higher spikes every hour (flushing of tier 2).

Health Alerts and Machine-Learning run queries to evaluate their expressions and learn from the metrics' patterns. These are also spread over time, so there should be an almost constant read rate too.

To make Netdata not use the disks at all, we suggest the following:

1. Use database mode `alloc` or `ram` to disable writing metric data to disk.
2. Configure streaming to push in real-time all metrics to a Netdata Parent. The Netdata Parent will maintain metrics on disk for this node.
3. Disable ML and health on this node. The Netdata Parent will do them for this node.
4. Use the Netdata Parent to access the dashboard.

Using the above, the Netdata Agent on your production system will not use a disk.

&nbsp;<br/>&nbsp;<br/>
</details>

### :raised_eyebrow: How is Netdata different from a Prometheus and Grafana setup?

Netdata is a "ready to use" monitoring solution. Prometheus and Grafana are tools to build your own monitoring solution.

Netdata is also a lot faster, requires significantly fewer resources and puts almost no stress on the server it runs. For a performance comparison check [this blog](https://blog.netdata.cloud/netdata-vs-prometheus-performance-analysis/).

<details><summary>Click to see detailed answer ...</summary>
&nbsp;<br/>&nbsp;<br/>

First, we have to say that Prometheus as a time-series database and Grafana as a visualizer are excellent tools for what they do.

However, we believe that such a setup is missing a key element: A Prometheus and Grafana setup assumes that you know everything about the metrics you collect, and you understand deeply how they’re structured, they should be queried and visualized.

In reality, this setup has a lot of problems. The vast number of technologies, operating systems, and applications we use in our modern stacks makes it impossible for any single person to know and understand everything about anything. We get testimonials regularly from Netdata users across the biggest enterprises, that Netdata manages to reveal issues, anomalies and problems they weren’t aware of, and they didn't even have the means to find or troubleshoot.

So, the biggest difference of Netdata to Prometheus, and Grafana, is that we decided that the tool needs to have a much better understanding of the components, the applications, and the metrics it monitors.

- When compared to Prometheus, Netdata needs for each metric much more than just a name, some labels, and a value over time. A metric in Netdata is a structured entity that correlates with other metrics in a certain way and has specific attributes that depict how it should be organized, treated, queried, and visualized. We call this the NIDL (Nodes, Instances, Dimensions, Labels) framework.

  Maintaining such an index is a challenge: first, because the raw metrics collected do not provide this information, so we have to add it, and second because we need to maintain this index for the lifetime of each metric, which with our current database retention, it is usually more than a year.

  At the same time, Netdata provides better retention than Prometheus due to database tiering, scales easier than Prometheus due to streaming, supports anomaly detection, and it has a metrics scoring engine to find the needle in the haystack when needed.

- When compared to Grafana, Netdata is fully automated. Grafana has more customization capabilities than Netdata, but Netdata presents fully functional dashboards by itself, and most importantly, it gives you the means to understand, analyze, filter, slice and dice the data without the need for you to edit queries or be aware of any peculiarities the underlying metrics may have.

  Furthermore, to help you when you need to find the needle in the haystack, Netdata has advanced troubleshooting tools provided by the Netdata metrics scoring engine, that allows it to score metrics based on their anomaly rate, their differences or similarities for any given time frame.

Still, if you’re already familiar with Prometheus and Grafana, Netdata integrates nicely with them, and we have reports from users who use Netdata with Prometheus and Grafana in production.

&nbsp;<br/>&nbsp;<br/>
</details>

### :raised_eyebrow: How is Netdata different from DataDog, New Relic, Dynatrace, X SaaS Provider?

With Netdata your data are always on-prem and your metrics are always high-resolution.

<details><summary>Click to see detailed answer ...</summary>
&nbsp;<br/>&nbsp;<br/>

Most commercial monitoring providers face a significant challenge: they centralize all metrics to their infrastructure, and this is, inevitably, expensive. It leads them to one or more of the following:

1. be unrealistically expensive
2. limit the number of metrics they collect
3. limit the resolution of the metrics they collect

As a result, they try to find a balance: collect the least possible data, but collect enough to have something useful out of it.

We, at Netdata, see monitoring in a completely different way: **monitoring systems should be built bottom-up and be rich in insights**, so we focus on each component individually to collect, store, check and visualize everything related to each of them, and we make sure that all components are monitored. Each metric is important.

This is why Netdata trains multiple machine-learning models per metric, based exclusively on their own past (no sampling of data, no sharing of trained models) to detect anomalies based on the specific use case and workload each component is used.

This is also why Netdata alerts are attached to components (instances) and are configured with dynamic thresholds and rolling windows, instead of static values.

The distributed nature of Netdata helps scale this approach: your data is spread inside your infrastructure, as close to the edge as possible. Netdata is not one data lane. Each Netdata Agent is a data lane, and all of them together build a massive distributed metrics processing pipeline that ensures all your infrastructure components and applications are monitored and operating as they should.

&nbsp;<br/>&nbsp;<br/>
</details>

### :raised_eyebrow: How is Netdata different from Nagios, Icinga, Zabbix, etc.?

Netdata offers real-time, comprehensive monitoring and the ability to monitor everything without any custom configuration required.

<details><summary>Click to see detailed answer ...</summary>
&nbsp;<br/>&nbsp;<br/>

While Nagios, Icinga, Zabbix, and other similar tools are powerful and highly customizable, they can be complex to set up and manage. Their flexibility often comes at the cost of ease-of-use, especially for users who aren’t systems administrators or don’t have extensive experience with these tools. Additionally, these tools generally require you to know what you want to monitor in advance and configure it explicitly.

Netdata, on the other hand, takes a different approach. It provides a "ready to use" monitoring solution with a focus on simplicity and comprehensiveness. It automatically detects and starts monitoring many different system metrics and applications out-of-the-box, without any need for custom configuration.

In comparison to these traditional monitoring tools, Netdata:

- Provides real-time, high-resolution metrics, as opposed to the often minute-level granularity that tools like Nagios, Icinga, and Zabbix provide.

- Automatically generates meaningful, organized, and interactive visualizations of the collected data. Unlike other tools, where you have to manually create and organize graphs and dashboards, Netdata takes care of this for you.

- Applies machine learning to each individual metric to detect anomalies, providing more insightful and relevant alerts than static thresholds.

- Designed to be distributed, so your data is spread inside your infrastructure, as close to the edge as possible. This approach is more scalable and avoids the potential bottleneck of a single centralized server.

- Has a more modern and user-friendly interface, allowing anyone, not just experienced administrators, to easily assess the health and performance of their systems.

Even if you're already using Nagios, Icinga, Zabbix, or similar tools, you can use Netdata alongside them to augment your existing monitoring capabilities with real-time insights and user-friendly dashboards.

&nbsp;<br/>&nbsp;<br/>
</details>

### :flushed: I feel overwhelmed by the amount of information in Netdata. What should I do?

Netdata is designed to provide comprehensive insights, but we understand that the richness of information might sometimes feel overwhelming. Here are some tips on how to navigate and use Netdata effectively...

<details><summary>Click to see detailed answer ...</summary>
&nbsp;<br/>&nbsp;<br/>

Netdata is indeed a very comprehensive monitoring tool. It's designed to provide you with as much information as possible about your system and applications, so that you can understand and address any issues that arise. However, we understand that the sheer amount of data can sometimes be overwhelming.

Here are some suggestions on how to manage and navigate this wealth of information:

1. **Start with the Metrics Dashboard**<br/>
   Netdata's Metrics Dashboard provides a high-level summary of your system's status. We have added summary tiles on almost every section, you reveal the information that is more important. This is a great place to start, as it can help you identify any major issues or trends at a glance.

2. **Use the Search Feature**<br/>
   If you're looking for specific information, you can use the search feature to find the relevant metrics or charts. This can help you avoid scrolling through all the data.

3. **Customize your Dashboards**<br/>
   Netdata allows you to create custom dashboards, which can help you focus on the metrics that are most important to you. Sign-in to Netdata and there you can have your custom dashboards. (coming soon to the Agent dashboard too)

4. **Leverage Netdata's Anomaly Detection**<br/>
   Netdata uses machine learning to detect anomalies in your metrics. This can help you identify potential issues before they become major problems. We have added an `AR` button above the dashboard table of contents to reveal the anomaly rate per section so that you can spot what could need your attention.

5. **Take Advantage of Netdata's Documentation and Blogs**<br/>
   Netdata has extensive documentation that can help you understand the different metrics and how to interpret them. You can also find tutorials, guides, and best practices there.

Remember, it's not necessary to understand every single metric or chart right away. Netdata is a powerful tool, and it can take some time to fully explore and understand all of its features. Start with the basics and gradually delve into more complex metrics as you become more comfortable with the tool.

&nbsp;<br/>&nbsp;<br/>
</details>

### :cloud: Do I have to subscribe to Netdata Cloud?

Netdata Cloud delivers the full suite of features and functionality that Netdata offers, including a free community tier.

While our default onboarding process encourages users to take advantage of Netdata Cloud, including a complimentary one-month trial of our full business product, it is not mandatory. Users can bypass this process entirely and still use the Netdata Agents along with the Netdata UI, without the need to sign up for Netdata Cloud.

<details><summary>Click to see detailed answer ...</summary>
&nbsp;<br/>&nbsp;<br/>

The Netdata Agent dashboard and the Netdata Cloud dashboard are the same. Still, Netdata Cloud provides additional features that the Netdata Agent is not capable of. These include:

1. Access your infrastructure from anywhere.
2. Have SSO to protect sensitive features.
3. Customizable (custom dashboards and other settings are persisted when you’re signed in to Netdata Cloud)
4. Configuration of Alerts and Data Collection from the UI
5. Security (Role-Based Access Control).
6. Horizontal Scalability ("blend" multiple independent parents in one uniform infrastructure)
7. Central Dispatch of Alert Notifications (even when multiple independent parents are involved)
8. Mobile App for Alert Notifications

We encourage you to support Netdata by buying a Netdata Cloud subscription. A successful Netdata is a Netdata that evolves and gets improved to provide simpler, faster and easier monitoring for all of us.

For organizations that need a fully on-prem solution, we provide Netdata Cloud for on-prem installation. [Contact us for more information](mailto:info@netdata.cloud).

&nbsp;<br/>&nbsp;<br/>
</details>

### :mag_right: What does the anonymous telemetry collected by Netdata entail?

Your privacy is our utmost priority. As part of our commitment to improving Netdata, we rely on anonymous telemetry data from our users who choose to leave it enabled. This data greatly informs our decision-making processes and contributes to the future evolution of Netdata.

Should you wish to disable telemetry, instructions for doing so are provided in our installation guides.

<details><summary>Click to see detailed answer ...</summary>
&nbsp;<br/>&nbsp;<br/>

Netdata is in a constant state of growth and evolution. The decisions that guide this development are ideally rooted in data. By analyzing anonymous telemetry data, we can answer questions such as "What features are being used frequently?", "How do we prioritize between potential new features?" and "What elements of Netdata are most important to our users?"

By leaving anonymous telemetry enabled, users indirectly contribute to shaping Netdata's roadmap, providing invaluable information that helps us prioritize our efforts for the project and the community.

We are aware that for privacy or regulatory reasons, not all environments can allow telemetry. To cater to this, we’ve simplified the process of disabling telemetry:

- During installation, you can append `--disable-telemetry` to our `kickstart.sh` script, or
- Create the file `/etc/netdata/.opt-out-from-anonymous-statistics` and then restart Netdata.

These steps will disable the anonymous telemetry for your Netdata installation.

Please note, even with telemetry disabled, Netdata still requires a [Netdata Registry](https://learn.netdata.cloud/docs/configuring/securing-netdata-agents/registry) for alert notifications' Call To Action (CTA) functionality. When you click an alert notification, it redirects you to the Netdata Registry, which then directs your web browser to the specific Netdata Agent that issued the alert for further troubleshooting. The Netdata Registry learns the URLs of your Agents when you visit their dashboards.

Any Netdata Agent can act as a Netdata Registry. Designate one Netdata Agent as your Registry, read more [here](https://learn.netdata.cloud/docs/netdata-agent/configuration/registry).

&nbsp;<br/>&nbsp;<br/>
</details>

### :smirk: Who uses Netdata?

Netdata is a widely adopted project...

<details><summary>Click to see detailed answer ...</summary>
&nbsp;<br/>&nbsp;<br/>

Browse the [Netdata stargazers on GitHub](https://github.com/netdata/netdata/stargazers) to discover users from renowned companies and enterprises, such as ABN AMRO Bank, AMD, Amazon, Baidu, Booking.com, Cisco, Delta, Facebook, Google, IBM, Intel, Logitech, Netflix, Nokia, Qualcomm, Realtek Semiconductor Corp, Redhat, Riot Games, SAP, Samsung, Unity, Valve, and many others.

Netdata also enjoys significant usage in academia, with notable institutions including New York University, Columbia University, New Jersey University, Seoul National University, University College London, among several others.

And, Netdata is also used by many governmental organizations worldwide.

In a nutshell, Netdata proves invaluable for:

- **Infrastructure intensive organizations**<br/>
  Such as hosting/cloud providers and companies with hundreds or thousands of nodes, who require a high-resolution, real-time monitoring solution for a comprehensive view of all their components and applications.

- **Technology operators**<br/>
  Those in need of a standardized, comprehensive solution for round-the-clock operations. Netdata not only facilitates operational automation and provides controlled access for their operations engineers, but also enhances skill development over time.

- **Technology startups**<br/>
  Who seek a feature-rich monitoring solution from the get-go.

- **Freelancers**<br/>
  Who seek a simple, efficient and straightforward solution without sacrificing performance and outcomes.

- **Professional SysAdmins and DevOps**<br/>
  Who appreciate the fine details and understand the value of holistic monitoring from the ground up.

- **Everyone else**<br/>
  All of us, who are tired of the inefficiency in the monitoring industry and would love a refreshing change and a breath of fresh air. :slightly_smiling_face:

&nbsp;<br/>&nbsp;<br/>
</details>

### :globe_with_meridians: Is Netdata open-source?

The **Netdata Agent** is open-source, but the **overall Netdata ecosystem** is a hybrid solution, combining open-source and closed-source components.

<details><summary>Click to see detailed answer ...</summary>
&nbsp;<br/>&nbsp;<br/>

Open-source is about sharing intellectual property with the world, and at Netdata, we embrace this philosophy wholeheartedly.

The **Netdata Agent**, the core of our ecosystem and the engine behind all our observability features, is fully open-source. Licensed under GPLv3+, the Netdata Agent represents our commitment to open-sourcing innovation in a wide array of observability technologies, including data collection, database design, query engines, observability data modeling, machine learning and unsupervised anomaly detection, high-performance edge computing, real-time monitoring, and more.

**The Netdata Agent is our gift to the world**, ensuring that the cutting-edge advancements we've developed are freely accessible to everyone.

However, as a privately funded company, we also need to monetize our open-source software to demonstrate product-market fit and sustain our growth.

Traditionally, open-source projects have often used the open-core model, where a basic version of the software is open-source, and additional features are reserved for a commercial, closed-source version. This approach can limit access to advanced innovations, as most of these remain closed-source.

At Netdata, we take a slightly different path. We don't create a separate enterprise version of our product. Instead, all users - both commercial and non-commercial - use the same Netdata Agent, ensuring that all of our observability innovations are always open source.

To experience the full capabilities of the Netdata ecosystem, users need to combine the open-source components with our closed-source offerings. The complete product still remains free to use.

The closed-source components include:

- **Netdata UI**: This is closed-source but free to use with the Netdata Agents and Netdata Cloud. It’s also publicly available via a CDN.
- **Netdata Cloud**: A commercial product available both as an on-premises installation and as a SaaS solution, with a free community tier.

By balancing open-source and closed-source components, we ensure that all users have access to our innovations while sustaining our ability to grow and innovate as a company.

&nbsp;<br/>&nbsp;<br/>
</details>

### :moneybag: What is your monetization strategy?

Netdata generates revenue through subscriptions to advanced features of Netdata Cloud and sales of on-premise and private versions of Netdata Cloud.

<details><summary>Click to see detailed answer ...</summary>
&nbsp;<br/>&nbsp;<br/>

Netdata generates revenue from these activities:

1. **Netdata Cloud Subscriptions**<br/>
   Direct funding for our project's vision comes from users subscribing to Netdata Cloud's advanced features.

2. **Netdata Cloud On-Prem or Private**<br/>
   Purchasing the on-premises or private versions of Netdata Cloud supports our financial growth.

Our Open-Source Community and the free access to Netdata Cloud, contribute to Netdata in the following ways:

- **Netdata Cloud Community Use**<br/>
  The free usage of Netdata Cloud demonstrates its market relevance. While this doesn't generate revenue, it reinforces trust among new users and aids in securing appropriate project funding.

- **User Feedback**<br/>
  Feedback, especially issues and bug reports, is invaluable. It steers us towards a more resilient and efficient product. This, too, isn't a revenue source but is pivotal for our project's evolution.

- **Anonymous Telemetry Insights**<br/>
  Users who keep anonymous telemetry enabled, help us make data informed decisions on refining and enhancing Netdata. This isn't a revenue stream, but knowing which features are used and how, contributes in building a better product for everyone.

We don't monetize, directly or indirectly, users' or "device heuristics" data. Any data collected from community members is exclusively used for the purposes stated above.

Netdata grows financially when technology intensive organizations and operators need - due to regulatory or business requirements - the entire Netdata suite on-prem or private, bundled with top-tier support. It is a win-win case for all parties involved: these companies get a battle tested, robust and reliable solution, while the broader community that helps us build this product enjoys it at no cost.

&nbsp;<br/>&nbsp;<br/>
</details>

## :book: Documentation

Netdata's documentation is available at [**Netdata Learn**](https://learn.netdata.cloud).

This site also hosts a number of [guides](https://learn.netdata.cloud/guides) to help newer users better understand how
to collect metrics, troubleshoot via charts, export to external databases, and more.

## :tada: Community

<p align="center">
  <a href="https://discord.com/invite/2mEmfW735j"><img alt="Discord" src="https://img.shields.io/discord/847502280503590932?logo=discord&logoColor=white&label=chat%20on%20discord"></a>
  <a href="https://community.netdata.cloud"><img alt="Discourse topics" src="https://img.shields.io/discourse/topics?server=https%3A%2F%2Fcommunity.netdata.cloud%2F&logo=discourse&label=discourse%20forum"></a>
  <a href="https://github.com/netdata/netdata/discussions"><img alt="GitHub Discussions" src="https://img.shields.io/github/discussions/netdata/netdata?logo=github&label=github%20discussions"></a>
</p>

Netdata is an inclusive open-source project and community. Please read our [Code of Conduct](https://github.com/netdata/.github/blob/main/CODE_OF_CONDUCT.md).

Join the Netdata community:

- Chat with us and other community members on [Discord](https://discord.com/invite/2mEmfW735j).
- Start a discussion on [GitHub discussions](https://github.com/netdata/netdata/discussions).
- Open a topic to our [community forums](https://community.netdata.cloud).

> **Meet Up** :people_holding_hands::people_holding_hands::people_holding_hands:<br/>
> The Netdata team and community members have regular online meetups.<br/>
> **You are welcome to join us!**
> [Click here for the schedule](https://www.meetup.com/netdata/events/).

You can also find Netdata on:<br/>
[Twitter](https://twitter.com/netdatahq) | [YouTube](https://www.youtube.com/c/Netdata) | [Reddit](https://www.reddit.com/r/netdata/) | [LinkedIn](https://www.linkedin.com/company/netdata-cloud/) | [StackShare](https://stackshare.io/netdata) | [Product Hunt](https://www.producthunt.com/posts/netdata-monitoring-agent/) | [Repology](https://repology.org/metapackage/netdata/versions) | [Facebook](https://www.facebook.com/linuxnetdata/)

## :pray: Contribute

<p align="center">
  <a href="https://github.com/netdata/netdata/graphs/contributors"><img alt="Open Source Contributors" src="https://img.shields.io/github/contributors/netdata/netdata?label=open-source%20contributors"></a>
</p>

Contributions are essential to the success of open-source projects. In other words, we need your help to keep Netdata great!

What is a contribution? All the following are highly valuable to Netdata:

1. **Let us know of the best practices you believe should be standardized**<br/>
   Netdata should out-of-the-box detect as many infrastructure issues as possible. By sharing your knowledge and experiences, you help us build a monitoring solution that has baked into it all the best-practices about infrastructure monitoring.

2. **Let us know if Netdata is not perfect for your use case**<br/>
   We aim to support as many use cases as possible and your feedback can be invaluable. Open a GitHub issue, or start a GitHub discussion about it, to discuss how you want to use Netdata and what you need.

   Although we can't implement everything imaginable, we try to prioritize development on use-cases that are common to our community, are in the same direction we want Netdata to evolve and are aligned with our roadmap.

3. **Support other community members**<br/>
   Join our community on GitHub, Discord, and Reddit. Generally, Netdata is relatively easy to set up and configure, but still people may need a little push in the right direction to use it effectively. Supporting other members is a great contribution by itself!

4. **Add or improve integrations you need**<br/>
   Integrations tend to be easier and simpler to develop. If you would like to contribute your code to Netdata, we suggest that you start with the integrations you need, which Netdata doesn’t currently support.

General information about contributions:

- Check our [Security Policy](https://github.com/netdata/netdata/security/policy).
- Found a bug? Open a [GitHub issue](https://github.com/netdata/netdata/issues/new?assignees=&labels=bug%2Cneeds+triage&template=BUG_REPORT.yml&title=%5BBug%5D%3A+).
- Read our [Contributing Guide](https://github.com/netdata/.github/blob/main/CONTRIBUTING.md), which contains all the information you need to contribute to Netdata, such as improving our documentation, engaging in the community, and developing new features. We've made it as frictionless as possible, but if you need help, just ping us on our community forums!

Package maintainers should read the guide on [building Netdata from source](/packaging/installer/methods/source.md) for
instructions on building each Netdata component from the source and preparing a package.

## License

The Netdata ecosystem consists of three key parts:

- **Netdata Agent**: The heart of the Netdata ecosystem, the Netdata Agent is an open-source tool that must be installed on all systems monitored by Netdata. It offers a wide range of essential features, including data collection via various plugins, an embedded high-performance time-series database (dbengine), unsupervised anomaly detection powered by edge-trained machine learning, alerting and notifications, as well as query and scoring engines with associated APIs. Additionally, it supports exporting data to third-party monitoring systems, among other capabilities.

  The Netdata Agent is released under the [GPLv3+ license](https://github.com/netdata/netdata/blob/master/LICENSE) and redistributes several other open-source tools and libraries, which are listed in the [Netdata Agent third-party licenses](https://github.com/netdata/netdata/blob/master/REDISTRIBUTED.md).

- **Netdata Cloud**: A commercial, closed-source component, Netdata Cloud enhances the capabilities of the open-source Netdata Agent by providing horizontal scalability, centralized alert notification dispatch (including a mobile app), user management, role-based access control, and other enterprise-grade features. It is available both as a SaaS solution and for on-premises deployment, with a free-to-use community tier also offered.

- **Netdata UI**: The Netdata UI is closed-source, and handles all visualization and dashboard functionalities related to metrics, logs and other collected data, as well as the central configuration and management of the Netdata ecosystem. It serves both the Netdata Agent and Netdata Cloud. The Netdata UI is distributed in binary form with the Netdata Agent and is publicly accessible via a CDN, licensed under the [Netdata Cloud UI License 1 (NCUL1)](https://app.netdata.cloud/LICENSE.txt). It integrates third-party open-source components, detailed in the [Netdata UI third-party licenses](https://app.netdata.cloud/3D_PARTY_LICENSES.txt).

The binary installation packages provided by Netdata include the Netdata Agent and the Netdata UI. Since the Netdata Agent is open-source, it is frequently packaged by third parties (e.g., Linux Distributions) excluding the closed-source components (Netdata UI is not included). While their packages can still be useful in providing the necessary back-ends and the APIs of a fully functional monitoring solution, we recommend using the installation packages we provide to experience the full feature set of Netdata.
