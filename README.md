<p align="center"><a href="https://netdata.cloud"><img src="https://user-images.githubusercontent.com/1153921/95268672-a3665100-07ec-11eb-8078-db619486d6ad.png" alt="Netdata" width="300" /></a></p>

<h3 align="center">Monitor your servers, containers and applications, in high-resolution and in real-time.</h3>
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

- :star: **Collects metrics from 800+ integrations**<br/>
  Operating system metrics, container metrics, virtual machines, hardware sensors, applications metrics, OpenMetrics exporters, StatsD and logs.
  
- :muscle: **Real-Time, Low-Latency, High-Resolution**<br/>
  All metrics are collected per-second, and are on the dashboard immediately after data collection. Netdata is designed to be fast.

- :metal: **Unsupervised Anomaly Detection**<br/>
  Trains multiple ML models for each metric collected and detects anomalies based on the past behavior of each metric individually.

- :fire: **Powerful Visualization**<br/>
  Clear and precise visualuzation that allows you to slice and dice/filter the data directly on the dashboard, without the need to learn any query language.

- :bell: **Out of box Alerts**<br/>
  Comes with hundreds of alerts out of the box to detect common issues and pitfalls, revealing issues that can easily go unnoticed.

- :sunglasses: **Low Maintenance**<br/>
  Fully automated in every aspect: automated dashboards, out of the box alerts, auto-detection and auto-discovery of metrics, easy configuration.

<hr class="solid">

<p align="center">
  <img src="https://raw.githubusercontent.com/cncf/artwork/master/other/cncf/horizontal/white/cncf-white.svg" alt="CNCF" width="300">
  <br />
  Netdata is a member of the Cloud Native Computing Foundation (CNCF)<br />(and it is the 3rd most starred project in the <a href="https://landscape.cncf.io/card-mode?grouping=no&sort=stars">CNCF landscape</a>)
</p>

<hr class="solid">

> **Important Note**:<br/>
> People get addicted to Netdata. Once you use it on your systems, there's no going back!<br/>
> _You have been warned..._

<img src="https://user-images.githubusercontent.com/1153921/95269366-1b814680-07ee-11eb-8ff4-c1b0b8758499.png" alt="---" style="max-width: 100%;" />


Netdata is designed to be super easy to setup and use, high performant and real-time, low-maintenance and cost efficient. It makes ML-assisted, high resolution monitoring easy and affordable, bringing to everyone the value that enterprises pay millions for.

- The [Netdata Agent](https://github.com/netdata/netdata) is the heart of the Netdata ecosystem. It is powering everything Netdata can do, it is **open-source** and it can be used standalone.

- [Netdata Cloud](https://www.netdata.cloud) is an optional service on top of Netdata Agents, providing infinite horizontal scalability, fully automated infrastructure level dashboards, auditing events, role based access control (RBAC), central dispatch of alert notifications, easy customization (including custom dashboards, point-and-click, without the need of a query language) and team collaboration. All these, without copying your data. Your data are always stored inside your servers.<br/>

:star: Netdata actively supports and is a [Silver Member of CNCF](https://www.cncf.io/about/members/), and although not a CNCF incubating or graduated project, [Netdata is the 3rd most starred project in the CNCF landscape](https://landscape.cncf.io/card-mode?grouping=no&sort=stars).

:star: Netdata is an open platform. It can exchange data using all popular protocols. It can scrape OpenMetrics exporters, it is a StatsD server, it can export metrics to Prometheus, OpenTSDB, Graphite, and more.

![image](https://github.com/netdata/netdata/assets/2662304/5fb726a4-65b9-4f58-85ab-b58bd01af12a)

## Quick Start

1. [Install Netdata Agents everywhere](https://learn.netdata.cloud/docs/getting-started/install-netdata#install-on-linux-with-one-line-installer).
   
   Netdata runs on almost all Linux distributions (Ubuntu, Debian, CentOS, Arch, their derivatives, etc), container platforms (Kubernetes clusters, Docker, etc), FreeBSD, pfSense and macOS. To monitor Windows servers and applications use [this guide](https://learn.netdata.cloud/docs/data-collection/monitor-anything/system-metrics/windows-machines). To monitor SNMP devices check [this guide](https://learn.netdata.cloud/docs/data-collection/monitor-anything/networking/snmp). For a detailed list of all data collectors, check [this guide](https://learn.netdata.cloud/docs/data-collection/). For Kubernetes deployments [check here](https://learn.netdata.cloud/docs/installation/install-on-specific-environments/kubernetes/).

2. Install a few Netdata Agents as centralization points (Parents) inside your network and configure streaming on all your agents to push their metrics to these central (parent) Netdata agents.

   Due to the distributed nature of Netdata, and to ensure high-availability of your monitoring system, please check our [Data Replication](https://www.netdata.cloud/blog/why-is-data-replication-important) recommendations to increase the data availability.

3. **If you plan to use Netdata Cloud**

      - Configure as many centralization points as you wish.
        The more the better. Netdata Cloud supports virtually unlimited horizontal scalability and the more parents you add, the faster it gets.

      - Visit Netdata Cloud to have your centralized views and central dispatch of alert notifications.
   
        Your data are still inside your network (your Netdata Agents). They pass through Netdata Cloud to reach your browser for the dashboards you view (only when you view them).
        In the next few releases we will introduce WebRTC peer-to-peer communication between browsers and agents, so that your data will never be exposed to Netdata Cloud.

   **If you don't want to use Netdata Cloud**

      - Configure only 1 parent cluster (2 servers in active-active setup), to centralize everything from your infra.
   
        Netdata scales verically very well: for >1M metrics/s you are going to need a 16 core VM with 32GB RAM, utilized at about 50% for ingestion and ML, leaving the rest for queries. Give it enough storage for retention. Netdata supports tiering, so you can have really long retention with a relatively small disk footprint.

      - Currently, at the parents you will find single-node monitoring dashboards for all your nodes. At the next agent release, the agents will get the same dashboard as Netdata Cloud (ML-first approach, etc).

💡 Netdata Cloud does not prevent you from using your Netdata Agents directly, and vice versa.

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
