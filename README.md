<p align="center"><a href="https://netdata.cloud"><img src="https://user-images.githubusercontent.com/1153921/95268672-a3665100-07ec-11eb-8078-db619486d6ad.png" alt="Netdata" width="300" /></a></p>

<h3 align="center">Netdata is high-fidelity infrastructure monitoring and troubleshooting.<br />Open-source, free, preconfigured, opinionated, and always real-time.</h3>
<br />
<p align="center">
  <a href="https://github.com/netdata/netdata/"><img src="https://img.shields.io/github/stars/netdata/netdata?style=social" alt="GitHub Stars"></a>
  <br />
  <a href="https://github.com/netdata/netdata/releases/latest"><img src="https://img.shields.io/github/release/netdata/netdata.svg" alt="Latest release"></a>
  <a href="https://github.com/netdata/netdata-nightlies/releases/latest"><img src="https://img.shields.io/github/release/netdata/netdata-nightlies.svg" alt="Latest nightly build"></a>
  <br />
  <a href="https://bestpractices.coreinfrastructure.org/projects/2231"><img src="https://bestpractices.coreinfrastructure.org/projects/2231/badge" alt="CII Best Practices"></a>
  <a href="https://codeclimate.com/github/netdata/netdata"><img src="https://codeclimate.com/github/netdata/netdata/badges/gpa.svg" alt="Code Climate"></a>
  <a href="https://www.gnu.org/licenses/gpl-3.0"><img src="https://img.shields.io/badge/License-GPL%20v3%2B-blue.svg" alt="License: GPL v3+"></a>
</p>

<img src="https://user-images.githubusercontent.com/1153921/95269366-1b814680-07ee-11eb-8ff4-c1b0b8758499.png" alt="---" style="max-width: 100%;" />

Netdata is a distributed, real-time, performance and health monitoring platform for systems, hardware, containers and applications, collecting thousands of useful metrics with zero configuration needed. It runs permanently on all your physical/virtual servers, containers, cloud deployments, and edge/IoT devices, and is perfectly safe to install on your systems mid-incident without any preparation.

The Netdata [Agent](https://github.com/netdata/netdata) is an enormously powerful, **Open-Sourced**, **Single Node** health monitoring and performance troubleshooting tool.
It gives you the ability to automatically identify processes, collect and store metrics locally and even more - visualize all metrics without any configuration (of course you can tweak it later on if you need).

[Netdata Cloud](https://www.netdata.cloud) is a hosted web interface that gives you **Free**, real-time visibility into your **Entire Infrastructure** with secure access to your Netdata Agents. It provides an ability to automatically route your requests to the most relevant agents to display your metrics, based on the stored metadata (Agents topology, what metrics are collected on specific Agents as well as the retention information for each metric).

It gives you some extra features, like [Metric Correlations](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/metric-correlations.md), [Anomaly Advisor](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/anomaly-advisor.md), [anomaly rates on every chart](https://blog.netdata.cloud/anomaly-rate-in-every-chart/) and much more. 

Try it for yourself right now by checking out the Netdata Cloud [demo space](https://app.netdata.cloud/spaces/netdata-demo/rooms/all-nodes/overview) (No sign up or login needed).

Netdata's mission is to help more people troubleshoot ever more complex IT infrastructures, this is why our **free** [community plan](https://www.netdata.cloud/pricing) gives you ability to monitor unlimited number of Nodes, Containers and Metrics (custom or built-in).

Due to the distributed nature of Netdata, and to ensure high-availability of your monitoring system, please check our [Data Replication](https://www.netdata.cloud/blog/why-is-data-replication-important) recommendations to increase the data availability.

You can install Netdata on most Linux distributions (Ubuntu, Debian, CentOS, and more), container platforms (Kubernetes
clusters, Docker), and many other operating systems (FreeBSD, macOS). No `sudo` required.

Netdata is designed by system administrators, DevOps engineers, and developers to collect everything, help you visualize
metrics, troubleshoot complex performance problems, and make data interoperable with the rest of your monitoring stack.

People get addicted to Netdata. Once you use it on your systems, there's no going back! _You've been warned..._

![Users who are addicted to
Netdata](https://user-images.githubusercontent.com/1153921/96495792-2e881380-11fd-11eb-85a3-53d3a84dcb29.png)

## Menu

- [Features](#features)
- [Get Netdata](#get-netdata)
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
