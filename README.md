<p align="center"><a href="https://netdata.cloud"><img src="https://user-images.githubusercontent.com/1153921/95268672-a3665100-07ec-11eb-8078-db619486d6ad.png" alt="Netdata" width="300" /></a></p>

<h3 align="center">Monitor your servers, containers, and applications, in high-resolution and in real-time.</h3>

<br />
<p align="center">
  <a href="https://github.com/netdata/netdata/"><img src="https://img.shields.io/github/stars/netdata/netdata?style=social" alt="GitHub Stars"></a>
  <br />
  <a href="https://app.netdata.cloud/spaces/netdata-demo?utm_campaign=github_readme_demo_badge"><img src="https://img.shields.io/badge/Live Demo-green" alt="Live Demo"></a>
  <a href="https://github.com/netdata/netdata/releases/latest"><img src="https://img.shields.io/github/release/netdata/netdata.svg" alt="Latest release"></a>
  <a href="https://github.com/netdata/netdata-nightlies/releases/latest"><img src="https://img.shields.io/github/release/netdata/netdata-nightlies.svg" alt="Latest nightly build"></a>
  <br />
  <a href="https://bestpractices.coreinfrastructure.org/projects/2231"><img src="https://bestpractices.coreinfrastructure.org/projects/2231/badge" alt="CII Best Practices"></a>
  <a href="https://codeclimate.com/github/netdata/netdata"><img src="https://codeclimate.com/github/netdata/netdata/badges/gpa.svg" alt="Code Climate"></a>
  <a href="https://www.gnu.org/licenses/gpl-3.0"><img src="https://img.shields.io/badge/License-GPL%20v3%2B-blue.svg" alt="License: GPL v3+"></a>
</p>
<hr class="solid">

Netdata collects metrics per-second and presents them in beatiful low-latency dashboards. It is designed to run on all your physical and virtual servers, cloud deployments, kubernetes clusters and edge/IoT devices, to monitor everything you run.

- :boom: **Collects metrics from 800+ integrations**<br/>
  Operating system metrics, container metrics, virtual machines, hardware sensors, applications metrics, OpenMetrics exporters, StatsD and logs.
  
- :muscle: **Real-Time, Low-Latency, High-Resolution**<br/>
  All metrics are collected per-second, and are on the dashboard immediately after data collection. Netdata is designed to be fast.

- :face_in_clouds: **Unsupervised Anomaly Detection**<br/>
  Trains multiple ML models for each metric collected and detects anomalies based on the past behavior of each metric individually.

- :fire: **Powerful Visualization**<br/>
  Clear and precise visualuzation that allows you to quickly understand any dataset, but also to filter, slice and dice the data directly on the dashboard, without the need to learn any query language.

- :bell: **Out of box Alerts**<br/>
  Comes with hundreds of alerts out of the box to detect common issues and pitfalls, revealing issues that can easily go unnoticed.

- :sunglasses: **Low Maintenance**<br/>
  Fully automated in every aspect: automated dashboards, out of the box alerts, auto-detection and auto-discovery of metrics, easy configuration.

- :star: **Open and Extensible**<br/>
  Netdata is a modular platform that can be extented in all possible ways and it also integrates nicely with other monitoring solutions.

<p align="center">
  <img src="https://raw.githubusercontent.com/cncf/artwork/master/other/cncf/horizontal/white/cncf-white.svg" alt="CNCF" width="300">
  <br />
  Netdata actively supports and is a member of the Cloud Native Computing Foundation (CNCF)<br />
  &nbsp;<br/>
  ...and due to your love :heart:, it is the 3rd most :star:'d project in the <a href="https://landscape.cncf.io/card-mode?grouping=no&sort=stars">CNCF landscape</a>!
</p>

<hr class="solid">

> :bulb: **Important Note**<br/>
> People get addicted to Netdata. **Once you use it on your systems, there's no going back!**<br/>
> _You have been warned..._<br/>

<hr class="solid">

## Quick Start

### 1. **Install Netdata Agents everywhere** :v:
   
   Netdata can be installed on all Linux, MacOS and FreeBSD systems. We provide binary packages for the most popular operating systems and package managers.

   - Install on [Ubuntu, Debiam CentOS, Arch, Alpine, Gentoo](https://learn.netdata.cloud/docs/installing/one-line-installer-for-all-linux-systems).
   - Install with [Docker](https://learn.netdata.cloud/docs/installing/docker). Netdata is a [Verified Publisher on DockerHub](https://hub.docker.com/r/netdata/netdata) and our users enjoy free unlimited DockerHub pulls :heart_eyes:.
   - Install on [MacOS](https://learn.netdata.cloud/docs/installing/macos) :metal:.
   - Install on [FreeBSD](https://learn.netdata.cloud/docs/installing/freebsd) and [pfSense](https://learn.netdata.cloud/docs/installing/pfsense).
   - Install [from source](https://learn.netdata.cloud/docs/installing/build-the-netdata-agent-yourself/compile-from-source-code)
   - For Kubernetes deployments [check here](https://learn.netdata.cloud/docs/installation/install-on-specific-environments/kubernetes/).

### 2. **Configure Collectors** :boom:

   Netdata auto-detects and auto-discovers most operating system data sources and applications. However, many data sources require some manual configuration, usually to allow Netdata get access to the metrics.
   
   - For a detailed list of all data collectors, check [this guide](https://learn.netdata.cloud/docs/data-collection/).
   - To monitor Windows servers and applications use [this guide](https://learn.netdata.cloud/docs/data-collection/monitor-anything/system-metrics/windows-machines).
   - To monitor SNMP devices check [this guide](https://learn.netdata.cloud/docs/data-collection/monitor-anything/networking/snmp).

### 3. **Configure Netdata Parents** :family:

   A Netdata Parent is a Netdata Agent that has been configured to accept [streaming connections](https://learn.netdata.cloud/docs/streaming/streaming-configuration-reference) from other Netdata agents.
   
   Netdata Parents provide:

   - Infrastructure level dashboards, at `http://parent.server.ip:19999/`
   - Increased retention for all metrics of all your nodes
   - Central configuration of alerts and dispatch of notifications

   You can also use Netdata Parents to:

   - Offload your production systems (the parents runs ML, alerts, queries, etc for all its children)
   - Secure your production systems (the parents accept user connections, for all its children)

### 4. **Connect your Parents to Netdata Cloud** :cloud:

   Optionally, sign-up to Netdata Cloud and claim your Netdata Parents.
   
   When your parents are connected to Netdata Cloud, you can (on top of the above):

   - Organize your infra in spaces and rooms
   - Create, manage and share **custom dashboards**
   - Invite your team and assign roles to them (Role Based Access Control - RBAC)
   - Access Netdata Functions (processes top from the UI and more)
   - Get infinite horizontal scalability (multiple independent parents are viewed as one infra)
   - Centrally configure alerts (coming soon)
   - Centrally configure data collection (coming soon)
   - Netdata Mobile App notifications (coming soon)

   :love_you_gesture: Netdata Cloud does not prevent you from using your Netdata Agents and Parents directly, and vice versa.<br/>
   :ok_hand: Your metrics are still stored in your network when you connect your Netdata Agents and Parent to Netdata Cloud.

## FAQ

### :shield: Is this secure?

Of course it is! We give our best for it to be!

We understand that Netdata is a software piece that is installed on millions of production systems across the world. So, it is important for us, Netdata to be as secure as possible:

  - We follow the [Open Source Security Foundation](https://bestpractices.coreinfrastructure.org/en/projects/2231) best practices.
  - We have given great attention to detail when it comes to security design. Check out our [security design](https://learn.netdata.cloud/docs/architecture/security-and-privacy-design).
  - Netdata is a popular open source project and is frequently tested by many security analysts.
  - Check also our [security polices and advisories published so far](https://github.com/netdata/netdata/security).

### :cyclone: Will this consume a lot of resources on my servers?

No. It will not! We promise this will be fast!

Although each Netdata Agent is a complete monitoring solution packed into a single application, and despite the fact that Netdata collects **every metric every single second** and trains **multiple ML models** per metric, you will find that Netdata has amazing performance! In many cases it outperforms other monitoring solutions that have singificantly less features or far smaller data collection rate.

This is what you should expect:

  - For production systems, each Netdata Agent with default settings (everything enabled, ML, Health, DB) should consume about 5% CPU utilization of single core and about 150 MiB or RAM. By using a Netdata parent and streaming all metrics to that parent, you can disable ML, health and use an ephemeral DB mode (like `alloc`) on the children, leading to a utilization of about 1% CPU of a single core and 100 MiB of RAM. Of course, these depend on how many metrics are collected.
  - For Netdata Parents, for about 1 to 2 million metrics, all collected every second, we suggest a server with 16 cores and 32GB RAM. Less than half of it will be used for data collection and ML. The rest will be available for queries.

Netdata has extensive internal instrumentation to help us reveal how the resources consumed are used. All these are available at the "Netdata Monitoring" section of the dashboard. Depending on your use case, there are many options to optimize resource consumption.

Even if you need to run Netdata on extremely weak embedded or IoT systems, you will find that Netdata can be tuned to be very performant.

### :scroll: How much retention will I get?

As much as you need!

Netdata supports **tiering**, to downsample past data and save disk space. With default settings it has 3 tiers:

  1. `tier 0`, with high resolution, per-second, data.
  2. `tier 1`, mid-resolution, per minute, data.
  3. `tier 2`, low-resolution, per hour, data.

All tiers are updated in parallel during data collection. Just increase the disk space you give to Netdata to get a longer history for your metrics.

### :rocket: Does it scale?



### :cloud: Do I need Netdata Cloud?


## Menu

- [Features](#features)
- [Get Netdata](#get-netdata)
  - [Infrastructure view](#infrastructure-view)
  - [Single node view](#single-node-view)
  - [Docker](#docker)
  - [Other operating systems](#other-operating-systems)
  - [Post-installation](#post-installation)
  - [Netdata Cloud](#netdata-cloud)
- [How it works](#how-it-works)
- [Infographic](#infographic)
- [Documentation](#documentation)
- [Community](#community)
- [Contribute](#contribute)
- [License](#license)
- [Is it any good?](#is-it-any-good)

## Features

![Netdata in
action](https://user-images.githubusercontent.com/1153921/113440964-449c2180-93a2-11eb-9664-663afa1257a8.gif)

Here's what you can expect from Netdata:

-   **1s granularity**: The highest possible resolution for all metrics.
-   **Unlimited metrics**: Netdata collects all the available metrics—the more, the better.
-   **1% CPU utilization of a single core**: It's unbelievably optimized.
-   **A few MB of RAM**: The highly-efficient database engine stores per-second metrics in RAM and then "spills"
    historical metrics to disk long-term storage.
-   **Minimal disk I/O**: While running, Netdata only writes historical metrics and reads `error` and `access` logs.
-   **Zero configuration**: Netdata auto-detects everything, and can collect up to 10,000 metrics per server out of the
    box.
-   **Zero maintenance**: You just run it. Netdata does the rest.
-   **Stunningly fast, interactive visualizations**: The dashboard responds to queries in less than 1ms per metric to
    synchronize charts as you pan through time, zoom in on anomalies, and more.
-   **Visual anomaly detection**: Our UI/UX emphasizes the relationships between charts to help you detect the root
    cause of anomalies.
-   **Machine learning (ML) features out of the box**: Unsupervised ML-based [anomaly detection](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/anomaly-advisor.md), every second, every metric, zero-config! [Metric correlations](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/metric-correlations.md) to help with short-term change detection. And other [additional](https://github.com/netdata/netdata/blob/master/docs/guides/monitor/anomaly-detection.md) ML-based features to help make your life easier.
-   **Scales to infinity**: You can install it on all your servers, containers, VMs, and IoT devices. Metrics are not
    centralized by default, so there is no limit.
-   **Several operating modes**: Autonomous host monitoring (the default), headless data collector, forwarding proxy,
    store and forward proxy, central multi-host monitoring, in all possible configurations. Use different metrics
    retention policies per node and run with or without health monitoring.

Netdata works with tons of applications, notifications platforms, and other time-series databases:

-   **300+ system, container, and application endpoints**: Collectors autodetect metrics from default endpoints and
    immediately visualize them into meaningful charts designed for troubleshooting. See [everything we
    support](https://github.com/netdata/netdata/blob/master/collectors/COLLECTORS.md).
-   **20+ notification platforms**: Netdata's health watchdog sends warning and critical alarms to your [favorite
    platform](https://github.com/netdata/netdata/blob/master/docs/monitor/enable-notifications.md) to inform you of anomalies just seconds
    after they affect your node.
-   **30+ external time-series databases**: Export resampled metrics as they're collected to other [local- and
    Cloud-based databases](https://github.com/netdata/netdata/blob/master/docs/export/external-databases.md) for best-in-class
    interoperability.

> 💡 **Want to leverage the monitoring power of Netdata across entire infrastructure**? View metrics from
> any number of distributed nodes in a single interface and unlock even more
> [features](https://github.com/netdata/netdata/blob/master/docs/overview/why-netdata.md) with [Netdata
> Cloud](https://learn.netdata.cloud/docs/overview/what-is-netdata#netdata-cloud).

## Get Netdata

<p align="center">
  <a href="https://registry.my-netdata.io/#menu_netdata_submenu_registry"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_entries&dimensions=persons&label=user%20base&units=M&value_color=blue&precision=2&divide=1000000&options=unaligned&v44" alt="User base"></a>
  <a href="https://registry.my-netdata.io/#menu_netdata_submenu_registry"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_entries&dimensions=machines&label=servers%20monitored&units=k&divide=1000&value_color=orange&precision=2&options=unaligned&v44" alt="Servers monitored"></a>
  <a href="https://registry.my-netdata.io/#menu_netdata_submenu_registry"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_sessions&label=sessions%20served&units=M&value_color=yellowgreen&precision=2&divide=1000000&options=unaligned&v44" alt="Sessions served"></a>
  <a href="https://hub.docker.com/r/netdata/netdata"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=dockerhub.pulls_sum&divide=1000000&precision=1&units=M&label=docker+hub+pulls&options=unaligned&v44" alt="Docker Hub pulls"></a>
  <br />
  <a href="https://registry.my-netdata.io/#menu_netdata_submenu_registry"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_entries&dimensions=persons&after=-86400&options=unaligned&group=incremental-sum&label=new%20users%20today&units=null&value_color=blue&precision=0&options=unaligned&v44" alt="New users today"></a>
  <a href="https://registry.my-netdata.io/#menu_netdata_submenu_registry"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_entries&dimensions=machines&group=incremental-sum&after=-86400&options=unaligned&label=servers%20added%20today&units=null&value_color=orange&precision=0&v44" alt="New machines today"></a>
  <a href="https://registry.my-netdata.io/#menu_netdata_submenu_registry"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_sessions&after=-86400&group=incremental-sum&options=unaligned&label=sessions%20served%20today&units=null&value_color=yellowgreen&precision=0&v44" alt="Sessions today"></a>
  <a href="https://hub.docker.com/r/netdata/netdata"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=dockerhub.pulls_sum&divide=1000&precision=1&units=k&label=docker+hub+pulls&after=-86400&group=incremental-sum&label=docker%20hub%20pulls%20today&options=unaligned&v44" alt="Docker Hub pulls today"></a>
</p>

### Infrastructure view

Due to the distributed nature of the Netdata ecosystem, it is recommended to setup not only one Netdata Agent on your production system, but also an additional Netdata Agent acting as a [Parent](https://github.com/netdata/netdata/blob/master/streaming/README.md). A local Netdata Agent (child), without any database or alarms, collects metrics and sends them to another Netdata Agent (parent). The same parent can collect data for any number of child nodes and serves as a centralized health check engine for each child by triggering alerts on their behalf.

![Netdata Cloud](https://user-images.githubusercontent.com/423236/205926887-43024984-6d38-46ad-96cb-d0c388117c6d.png)

Get started by [signing in](https://app.netdata.cloud/?utm_source=website&utm_content=top_navigation_sign_up) to Netdata.cloud and follow the setup guide.

Community version is free to use forever. No restriction on number of nodes, clusters or metrics. Unlimited alerts.

#### Claiming existing Agents

You can easily [connect (claim)](https://github.com/netdata/netdata/blob/master/claim/README.md) your existing Agents to the Cloud to unlock features for free and to find weaknesses before they turn into outages. 

### Single Node view

In case you do not need the infrastructure view of you system you can install standalone Agent and enjoy the local dashboard.

To install Netdata from source on most Linux systems (physical, virtual, container, IoT, edge), run our [one-line
installation script](https://learn.netdata.cloud/docs/agent/packaging/installer/methods/packages). This script downloads
and builds all dependencies, including those required to connect to [Netdata Cloud](https://netdata.cloud/cloud) if you
choose, and enables [automatic nightly
updates](https://learn.netdata.cloud/docs/agent/packaging/installer#nightly-vs-stable-releases) and [anonymous
statistics](https://github.com/netdata/netdata/blob/master/docs/anonymous-statistics.md).
<!-- candidate for reuse -->
```bash
wget -O /tmp/netdata-kickstart.sh https://my-netdata.io/kickstart.sh && sh /tmp/netdata-kickstart.sh
```

To view the Netdata dashboard, navigate to `http://localhost:19999`, or `http://NODE:19999`.

### Docker

You can also try out Netdata's capabilities in a [Docker
container](https://github.com/netdata/netdata/blob/master/packaging/docker/README.md):

```bash
docker run -d --name=netdata \
  -p 19999:19999 \
  -v netdataconfig:/etc/netdata \
  -v netdatalib:/var/lib/netdata \
  -v netdatacache:/var/cache/netdata \
  -v /etc/passwd:/host/etc/passwd:ro \
  -v /etc/group:/host/etc/group:ro \
  -v /proc:/host/proc:ro \
  -v /sys:/host/sys:ro \
  -v /etc/os-release:/host/etc/os-release:ro \
  --restart unless-stopped \
  --cap-add SYS_PTRACE \
  --security-opt apparmor=unconfined \
  netdata/netdata
```

To view the Netdata dashboard, navigate to `http://localhost:19999`, or `http://NODE:19999`.

### Other operating systems

See our documentation for [additional operating
systems](https://github.com/netdata/netdata/blob/master/packaging/installer/README.md#have-a-different-operating-system-or-want-to-try-another-method), including
[Kubernetes](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/kubernetes.md), [`.deb`/`.rpm`
packages](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/kickstart.md#native-packages), and more.

### Post-installation

When you're finished with installation, check out our [single-node](https://github.com/netdata/netdata/blob/master/docs/quickstart/single-node.md) or
[infrastructure](https://github.com/netdata/netdata/blob/master/docs/quickstart/infrastructure.md) monitoring quickstart guides based on your use case.

Or, skip straight to [configuring the Netdata Agent](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md).

Read through Netdata's [documentation](https://learn.netdata.cloud/docs), which is structured based on actions and
solutions, to enable features like health monitoring, alarm notifications, long-term metrics storage, exporting to
external databases, and more.

## How it works

Netdata is a highly efficient, highly modular, metrics management engine. Its lockless design makes it ideal for
concurrent operations on the metrics.

![Diagram of Netdata's core
functionality](https://user-images.githubusercontent.com/1153921/95367248-5f755980-0889-11eb-827f-9b7aa02a556e.png)

The result is a highly efficient, low-latency system, supporting multiple readers and one writer on each metric.

## Infographic

This is a high-level overview of Netdata features and architecture. Click on it to view an interactive version, which
has links to our documentation.

[![An infographic of how Netdata
works](https://user-images.githubusercontent.com/43294513/212722097-fdd85dee-2fc8-47f5-90dc-d3149428cdfa.png)](https://my-netdata.io/infographic.html)

## Documentation

Netdata's documentation is available at [**Netdata Learn**](https://learn.netdata.cloud).

This site also hosts a number of [guides](https://learn.netdata.cloud/guides) to help newer users better understand how
to collect metrics, troubleshoot via charts, export to external databases, and more.

## Community

Netdata is an inclusive open-source project and community. Please read our [Code of Conduct](https://github.com/netdata/.github/blob/main/CODE_OF_CONDUCT.md).

Find most of the Netdata team in our [community forums](https://community.netdata.cloud). It's the best place to
ask questions, find resources, and engage with passionate professionals. The team is also available and active in our [Discord](https://discord.com/invite/mPZ6WZKKG2) too.

You can also find Netdata on:

-   [Twitter](https://twitter.com/linuxnetdata)
-   [YouTube](https://www.youtube.com/c/Netdata)
-   [Reddit](https://www.reddit.com/r/netdata/)
-   [LinkedIn](https://www.linkedin.com/company/netdata-cloud/)
-   [StackShare](https://stackshare.io/netdata)
-   [Product Hunt](https://www.producthunt.com/posts/netdata-monitoring-agent/)
-   [Repology](https://repology.org/metapackage/netdata/versions)
-   [Facebook](https://www.facebook.com/linuxnetdata/)

## Contribute

Contributions are the lifeblood of open-source projects. While we continue to invest in and improve Netdata, we need help to democratize monitoring!

- Read our [Contributing Guide](https://github.com/netdata/.github/blob/main/CONTRIBUTING.md), which contains all the information you need to contribute to Netdata, such as improving our documentation, engaging in the community, and developing new features. We've made it as frictionless as possible, but if you need help, just ping us on our community forums!
- We have a whole category dedicated to contributing and extending Netdata on our [community forums](https://community.netdata.cloud/c/agent-development/9)
- Found a bug? Open a [GitHub issue](https://github.com/netdata/netdata/issues/new?assignees=&labels=bug%2Cneeds+triage&template=BUG_REPORT.yml&title=%5BBug%5D%3A+).
- View our [Security Policy](https://github.com/netdata/netdata/security/policy).

Package maintainers should read the guide on [building Netdata from source](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/source.md) for
instructions on building each Netdata component from source and preparing a package.

## License

The Netdata Agent is [GPLv3+](https://github.com/netdata/netdata/blob/master/LICENSE). Netdata re-distributes other open-source tools and libraries. Please check the
[third party licenses](https://github.com/netdata/netdata/blob/master/REDISTRIBUTED.md).

## Is it any good?

Yes.

_When people first hear about a new product, they frequently ask if it is any good. A Hacker News user
[remarked](https://news.ycombinator.com/item?id=3067434):_

> Note to self: Starting immediately, all raganwald projects will have a “Is it any good?” section in the readme, and
> the answer shall be “yes.".
