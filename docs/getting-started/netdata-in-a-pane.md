<!--
title: "Netdata in a pane"
sidebar_label: "Netdata in a pane"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/getting-started/netdata-in-a-pane.md"
learn_status: "Published"
learn_topic_type: "Getting started"
learn_rel_path: ""
learn_docs_purpose: "Present netdata in a nutshell"
-->

<h2 align="center">Netdata is an ecosystem that provides monitoring, visualization, and troubleshooting solution for systems, containers, services, and applications.</h3>
<br />
<p align="center">
  <a href="https://github.com/netdata/netdata/"><img src="https://img.shields.io/github/stars/netdata/netdata?style=social" alt="GitHub Stars"></a>
  <br />
  <a href="https://github.com/netdata/netdata/releases/latest"><img src="https://img.shields.io/github/release/netdata/netdata.svg" alt="Latest release"></a>
  <a href="https://storage.googleapis.com/netdata-nightlies/latest-version.txt"><img src="https://img.shields.io/badge/dynamic/xml?url=https://storage.googleapis.com/netdata-nightlies/latest-version.txt&label=nightly%20release&query=/text()" alt="Nightly release"></a>
  <br />
  <a href="https://travis-ci.com/netdata/netdata"><img src="https://travis-ci.com/netdata/netdata.svg?branch=master" alt="Build status"></a>
  <a href="https://bestpractices.coreinfrastructure.org/projects/2231"><img src="https://bestpractices.coreinfrastructure.org/projects/2231/badge" alt="CII Best Practices"></a>
  <a href="https://www.gnu.org/licenses/gpl-3.0"><img src="https://img.shields.io/badge/License-GPL%20v3%2B-blue.svg" alt="License: GPL v3+"></a>
  <br />
  <a href="https://codeclimate.com/github/netdata/netdata"><img src="https://codeclimate.com/github/netdata/netdata/badges/gpa.svg" alt="Code Climate"></a>
  <a href="https://lgtm.com/projects/g/netdata/netdata/context:cpp"><img src="https://img.shields.io/lgtm/grade/cpp/g/netdata/netdata.svg?logo=lgtm" alt="LGTM C"></a>
  <a href="https://lgtm.com/projects/g/netdata/netdata/context:python"><img src="https://img.shields.io/lgtm/grade/python/g/netdata/netdata.svg?logo=lgtm" alt="LGTM PYTHON"></a>
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


## <image> a random Netdata deployment


## Why use Netdata?

Netdata is designed by system administrators, DevOps engineers, and developers to collect everything, help you visualize
metrics, troubleshoot complex performance problems, and make data interoperable with the rest of your monitoring stack.

You can install Netdata on most Linux distributions (Ubuntu, Debian, CentOS, and more), container platforms (Kubernetes
clusters, Docker), and many other operating systems (FreeBSD).

Netdata is:

### Simple to deploy

-   **One-line deployment** for Linux distributions, plus support for Kubernetes/Docker infrastructures.
-   **Zero configuration and maintenance** required to collect thousands of metrics, every second, from the underlying
    OS and running applications.
-   **Prebuilt charts and alarms** alert you to common anomalies and performance issues without manual configuration.
-   **Distributed storage** to simplify the cost and complexity of storing metrics data from any number of nodes.

### Powerful and scalable

-   **1% CPU utilization, a few MB of RAM, and minimal disk I/O** to run the monitoring Agent on bare metal, virtual
    machines, containers, and even IoT devices.
-   **Per-second granularity** for an unlimited number of metrics based on the hardware and applications you're running
    on your nodes.
-   **Interoperable exporters** let you connect Netdata's per-second metrics with an existing monitoring stack and other
    time-series databases.

### Optimized for troubleshooting

-   **Visual anomaly detection** with a UI/UX that emphasizes the relationships between charts.
-   **Customizable dashboards** to pinpoint correlated metrics, respond to incidents, and help you streamline your
    workflows.
-   **Distributed metrics in a centralized interface** to assist users or teams trace complex issues between distributed
    nodes.

### Secure by design


### Comparison with other monitoring solutions

Netdata offers many benefits over the existing monitoring landscape, whether they're expensive SaaS products or other
open-source tools.

| Netdata                                                         | Others (open-source and commercial)                              |
| :-------------------------------------------------------------- | :--------------------------------------------------------------- |
| **High resolution metrics** (1s granularity)                    | Low resolution metrics (10s granularity at best)                 |
| Collects **thousands of metrics per node**                      | Collects just a few metrics                                      |
| Fast UI optimized for **anomaly detection**                     | UI is good for just an abstract view                             |
| **Long-term, autonomous storage** at one-second granularity     | Centralized metrics in an expensive data lake at 10s granularity |
| **Meaningful presentation**, to help you understand the metrics | You have to know the metrics before you start                    |
| Install and get results **immediately**                         | Long sales process and complex installation process              |
| Use it for **troubleshooting** performance problems             | Only gathers _statistics of past performance_                    |
| **Kills the console** for tracing performance issues            | The console is always required for troubleshooting               |
| Requires **zero dedicated resources**                           | Require large dedicated resources                                |


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

## Community

Netdata is an inclusive open-source project and community. Please read our [Code of Conduct](https://learn.netdata.cloud/contribute/code-of-conduct).

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

- Read our [Contributing Guide](https://learn.netdata.cloud/contribute/handbook), which contains all the information you need to contribute to Netdata, such as improving our documentation, engaging in the community, and developing new features. We've made it as frictionless as possible, but if you need help, just ping us on our community forums!
- We have a whole category dedicated to contributing and extending Netdata on our [community forums](https://community.netdata.cloud/c/agent-development/9)
- Found a bug? Open a [GitHub issue](https://github.com/netdata/netdata/issues/new?assignees=&labels=bug%2Cneeds+triage&template=BUG_REPORT.yml&title=%5BBug%5D%3A+).
- View our [Security Policy](https://github.com/netdata/netdata/security/policy).

Package maintainers should read the guide on [building Netdata from source](/packaging/installer/methods/source.md) for
instructions on building each Netdata component from source and preparing a package.

## License

The Netdata Agent is an open source project distributed under [GPLv3+](/LICENSE). Netdata re-distributes other open-source tools and libraries. Please check the
[third party licenses](/REDISTRIBUTED.md).

## Is it any good?

Yes.

_When people first hear about a new product, they frequently ask if it is any good. A Hacker News user
[remarked](https://news.ycombinator.com/item?id=3067434):_

> Note to self: Starting immediately, all raganwald projects will have a “Is it any good?” section in the readme, and
> the answer shall be “yes.".
*******************************************************************************
