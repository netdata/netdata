<p align="center"><a href="https://netdata.cloud"><img src="https://user-images.githubusercontent.com/1153921/95268672-a3665100-07ec-11eb-8078-db619486d6ad.png" alt="Netdata" width="300" /></a></p>

<h3 align="center">The best monitoring and troubleshooting for systems, containers, applications, and infrastructure. <br />Preconfigured, opinionated, and always real-time.</h3>
<br />
<p align="center">
  <a href="https://github.com/netdata/netdata/releases/latest"><img src="https://img.shields.io/github/release/netdata/netdata.svg" alt="Latest release"></a>
  <a href="https://travis-ci.com/netdata/netdata"><img src="https://travis-ci.com/netdata/netdata.svg?branch=master" alt="Build status"></a>
  <a href="https://bestpractices.coreinfrastructure.org/projects/2231"><img src="https://bestpractices.coreinfrastructure.org/projects/2231/badge" alt="CII Best Practices"></a>
  <a href="https://www.gnu.org/licenses/gpl-3.0"><img src="https://img.shields.io/badge/License-GPL%20v3%2B-blue.svg" alt="License: GPL v3+"></a>
  <a href="<>"><img src="https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Freadme&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3" alt="analytics"></a>
  <br />
  <a href="https://codeclimate.com/github/netdata/netdata"><img src="https://codeclimate.com/github/netdata/netdata/badges/gpa.svg" alt="Code Climate"></a>
  <a href="https://www.codacy.com/app/netdata/netdata?utm_source=github.com&utm_medium=referral&utm_content=netdata/netdata&utm_campaign=Badge_Grade"><img src="https://api.codacy.com/project/badge/Grade/a994873f30d045b9b4b83606c3eb3498" alt="Codacy"></a>
  <a href="https://lgtm.com/projects/g/netdata/netdata/context:cpp"><img src="https://img.shields.io/lgtm/grade/cpp/g/netdata/netdata.svg?logo=lgtm" alt="LGTM C"></a>
  <a href="https://lgtm.com/projects/g/netdata/netdata/context:javascript"><img src="https://img.shields.io/lgtm/grade/javascript/g/netdata/netdata.svg?logo=lgtm" alt=""LGTM JS></a>
  <a href="https://lgtm.com/projects/g/netdata/netdata/context:python"><img src="https://img.shields.io/lgtm/grade/python/g/netdata/netdata.svg?logo=lgtm" alt="LGTM PYTHON"></a>
</p>

<img src="https://user-images.githubusercontent.com/1153921/95269366-1b814680-07ee-11eb-8ff4-c1b0b8758499.png" alt="---" style="max-width: 100%;" />

Netdata's **distributed, real-time monitoring Agent** collects thousands of metrics from systems, hardware, containers,
and applications with zero configuration. It runs permanently on all your physical/virtual servers, containers, cloud
deployments, and edge/IoT devices.

You can install Netdata on most Linux distributions (Ubuntu, Debian, CentOS, and more), container platforms (Kubernetes
clusters, Docker), and many other operating systems (FreeBSD, macOS). No `sudo` required.

Netdata is designed by system administrators, DevOps engineers, and developers to collect everything, help you visualize
metrics, troubleshoot complex performance problems, and make data interoperable with the rest of your monitoring stack.

People get addicted to Netdata. Once you use it on your systems, there's no going back! _You've been warned..._

![Users who are addicted to
Netdata](https://user-images.githubusercontent.com/1153921/96495792-2e881380-11fd-11eb-85a3-53d3a84dcb29.png)

> **[Latest release](https://github.com/netdata/netdata/releases/latest): v1.26.0, October 14, 2020**
>
> The v1.26.0 release of the Netdata Agent brings exciting new collectors written in Go, a new integration with the
> DevOps startup StackPulse, and massive improvements to the way users navigate Netdata's documentation. We've also
> added compatibility with an exciting new feature that's coming soon to Netdata Cloud—stay tuned! This release contains
> 3 new collectors, 1 new notifcation method, 21 improvements, 13 documentation updates, and 12 bug fixes.

## Menu

- [Features](#features)
- [Get Netdata](#get-netdata)
- [How it works](#how-it-works)
- [Documentation](#documentation)
- [Community](#community)
- [Contribute](#contribute)
- [License](#license)
- [Is it any good?](#is-it-any-good)

## Features

![Netdata in
action](https://user-images.githubusercontent.com/1153921/95625372-40afc800-0a2d-11eb-8425-3355c8c4a3f3.gif)

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
-   **Scales to infinity**: You can install it on all your servers, containers, VMs, and IoT devices. Metrics are not
    centralized by default, so there is no limit.
-   **Several operating modes**: Autonomous host monitoring (the default), headless data collector, forwarding proxy,
    store and forward proxy, central multi-host monitoring, in all possible configurations. Use different metrics
    retention policies per node and run with or without health monitoring.

Netdata works with tons of applications, notifications platforms, and other time-series databases:

-   **300+ system, container, and application endpoints**: Collectors autodetect metrics from default endpoints and
    immediately visualize them into meaningful charts designed for troubleshooting. See [everything we
    support](https://learn.netdata.cloud/docs/agent/collectors/collectors).
-   **20+ notification platforms**: Netdata's health watchdog sends warning and critical alarms to your [favorite
    platform](https://learn.netdata.cloud/docs/monitor/enable-notifications) to inform you of anomalies just seconds
    after they affect your node.
-   **30+ external time-series databases**: Export resampled metrics as they're collected to other [local- and
    Cloud-based databases](https://learn.netdata.cloud/docs/export/external-databases) for best-in-class
    interoperability.

> 💡 **Want to leverage the monitoring power of Netdata across entire infrastructure**? View metrics from
> any number of distributed nodes in a single interface and unlock even more
> [features](https://learn.netdata.cloud/docs/overview/why-netdata) with [Netdata
> Cloud](https://learn.netdata.cloud/docs/overview/what-is-netdata#netdata-cloud).

## Get Netdata

<p align="center">
  <a href="https://registry.my-netdata.io/#menu_netdata_submenu_registry"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_entries&dimensions=persons&label=user%20base&units=M&value_color=blue&precision=2&divide=1000000&v43" alt="User base"></a>
  <a href="https://registry.my-netdata.io/#menu_netdata_submenu_registry"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_entries&dimensions=machines&label=servers%20monitored&units=k&divide=1000&value_color=orange&precision=2&v43" alt="Servers monitored"></a>
  <a href="https://registry.my-netdata.io/#menu_netdata_submenu_registry"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_sessions&label=sessions%20served&units=M&value_color=yellowgreen&precision=2&divide=1000000&v43" alt="Sessions served"></a>
  <a href="https://hub.docker.com/r/netdata/netdata"><img src="https://frankfurt.my-netdata.io/api/v1/badge.svg?chart=dockerhub.pulls_sum&divide=1000000&precision=1&units=M&label=docker+hub+pulls" alt="Docker Hub pulls"></a>
  <br />
  <a href="https://registry.my-netdata.io/#menu_netdata_submenu_registry"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_entries&dimensions=persons&after=-86400&options=unaligned&group=incremental-sum&label=new%20users%20today&units=null&value_color=blue&precision=0&v42" alt="New users today"></a>
  <a href="https://registry.my-netdata.io/#menu_netdata_submenu_registry"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_entries&dimensions=machines&group=incremental-sum&after=-86400&options=unaligned&label=servers%20added%20today&units=null&value_color=orange&precision=0&v42" alt="New machines today"></a>
  <a href="https://registry.my-netdata.io/#menu_netdata_submenu_registry"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_sessions&after=-86400&group=incremental-sum&options=unaligned&label=sessions%20served%20today&units=null&value_color=yellowgreen&precision=0&v42" alt="Sessions today"></a>
  <a href="https://hub.docker.com/r/netdata/netdata"><img src="https://frankfurt.my-netdata.io/api/v1/badge.svg?chart=dockerhub.pulls_sum&divide=1000&precision=1&units=k&label=docker+hub+pulls&after=-86400&group=incremental-sum&label=docker%20hub%20pulls%20today" alt="Docker Hub pulls today"></a>
</p>

To install Netdata from source on most Linux systems (physical, virtual, container, IoT, edge), run our [one-line
installation script](https://learn.netdata.cloud/docs/agent/packaging/installer/methods/packages). This script downloads
and builds all dependencies, including those required to connect to [Netdata Cloud](https://netdata.cloud/cloud) if you
choose, and enables [automatic nightly
updates](https://learn.netdata.cloud/docs/agent/packaging/installer#nightly-vs-stable-releases) and [anonymous
statistics](https://learn.netdata.cloud/docs/agent/anonymous-statistics).

```bash
bash <(curl -Ss https://my-netdata.io/kickstart.sh)
```

To view the Netdata dashboard, navigate to `http://localhost:19999`, or `http://NODE:19999`.

### Docker

You can also try out Netdata's capabilities in a [Docker
container](https://learn.netdata.cloud/docs/agent/packaging/docker/):

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
systems](/packaging/installer/README.md#have-a-different-operating-system-or-want-to-try-another-method), including
[Kubernetes](/packaging/installer/methods/kubernetes.md), [`.deb`/`.rpm`
packages](/packaging/installer/methods/packages.md), and more.

### Post-installation

When you're finished with installation, check out our
[single-node](https://learn.netdata.cloud/docs/quickstart/single-node) or
[infrastructure](https://learn.netdata.cloud/docs/quickstart/infrastructure) monitoring quickstart guides based on your
use case. Then, see our [documentation](https://learn.netdata.cloud/docs) for the full picture.

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
works](https://user-images.githubusercontent.com/43294513/60951037-8ba5d180-a2f8-11e9-906e-e27356f168bc.png)](https://my-netdata.io/infographic.html)

## Documentation

Netdata's documentation is available at [**Netdata Learn**](https://learn.netdata.cloud).

This site also hosts a number of [guides](https://learn.netdata.cloud/guides) to help newer users better understand how
to collect metrics, troubleshoot via charts, export to external databases, and more.

## Community

You can find most of the Netdata team in our [community forum](https://community.netdata.cloud). It's the best place to
ask questions, find resources, and get to know the Netdata team.

You can also find Netdata on:

-   [Facebook](https://www.facebook.com/linuxnetdata/)
-   [Twitter](https://twitter.com/linuxnetdata)
-   [StackShare](https://stackshare.io/netdata)
-   [Product Hunt](https://www.producthunt.com/posts/netdata-monitoring-agent/)
-   [Repology](https://repology.org/metapackage/netdata/versions)

## Contribute

We welcome [contributions](/CONTRIBUTING.md) to our code and to our
[documentation](/docs/contributing/contributing-documentation.md). Feel free to join the team!

To report bugs or get help, use [GitHub's issues](https://github.com/netdata/netdata/issues).

Package maintainers should read the guide on [building Netdata from source](/packaging/installer/methods/source.md) for
instructions on building each Netdata component from source and preparing a package.

## License

The Netdata Agent is [GPLv3+](/LICENSE). Netdata re-distributes other open-source tools and libraries. Please check the
[third party licenses](/REDISTRIBUTED.md).

## Is it any good?

Yes.

_When people first hear about a new product, they frequently ask if it is any good. A Hacker News user
[remarked](https://news.ycombinator.com/item?id=3067434):_

> Note to self: Starting immediately, all raganwald projects will have a “Is it any good?” section in the readme, and
> the answer shall be “yes.".

So, we follow the tradition...
