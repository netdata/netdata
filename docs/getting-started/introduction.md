# Getting started with Netdata

Learn how Netdata can get you monitoring your infrastructure in minutes.

## What is Netdata ?

Netdata is designed by system administrators, DevOps engineers, and developers to collect everything, help you visualize
metrics, troubleshoot complex performance problems, and make data interoperable with the rest of your monitoring stack.

You can install Netdata on most Linux distributions (Ubuntu, Debian, CentOS, and more), container platforms (Kubernetes
clusters, Docker), and many other operating systems (FreeBSD).

Netdata is:

### Simple to deploy

- **One-line deployment** for Linux distributions, plus support for Kubernetes/Docker infrastructures.
- **Zero configuration and maintenance** required to collect thousands of metrics, every second, from the underlying
    OS and running applications.
- **Prebuilt charts and alarms** alert you to common anomalies and performance issues without manual configuration.
- **Distributed storage** to simplify the cost and complexity of storing metrics data from any number of nodes.

### Powerful and scalable

- **1% CPU utilization, a few MB of RAM, and minimal disk I/O** to run the monitoring Agent on bare metal, virtual
    machines, containers, and even IoT devices.
- **Per-second granularity** for an unlimited number of metrics based on the hardware and applications you're running
    on your nodes.
- **Interoperable exporters** let you connect Netdata's per-second metrics with an existing monitoring stack and other
    time-series databases.

### Optimized for troubleshooting

- **Visual anomaly detection** with a UI/UX that emphasizes the relationships between charts.
- **Customizable dashboards** to pinpoint correlated metrics, respond to incidents, and help you streamline your
    workflows.
- **Distributed metrics in a centralized interface** to assist users or teams trace complex issues between distributed
    nodes.

### Secure by design

- **Distributed data architecture**  so fast and efficient, there’s no limit to the number of metrics you can follow.
- Because your data is **stored at the edge**, security is ensured.

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

- **300+ system, container, and application endpoints**: Collectors autodetect metrics from default endpoints and
    immediately visualize them into meaningful charts designed for troubleshooting. See [everything we
    support](https://github.com/netdata/netdata/blob/master/collectors/COLLECTORS.md).
- **20+ notification platforms**: Netdata's health watchdog sends warning and critical alarms to your [favorite
    platform](https://github.com/netdata/netdata/blob/master/docs/monitor/enable-notifications.md) to inform you of anomalies just seconds
    after they affect your node.
- **30+ external time-series databases**: Export resampled metrics as they're collected to other [local- and
    Cloud-based databases](https://github.com/netdata/netdata/blob/master/docs/export/external-databases.md) for best-in-class
    interoperability.

## How it works

Netdata is a highly efficient, highly modular, metrics management engine. Its lockless design makes it ideal for concurrent operations on the metrics.

You can see a high level representation in the following diagram.

![Diagram of Netdata's core functionality](https://user-images.githubusercontent.com/2662304/199225735-01a41cc5-c074-4fe2-b780-5f08e92c6769.png)

And a higher level diagram in this one.

![Diagram 2 of Netdata's core
functionality](https://user-images.githubusercontent.com/1153921/95367248-5f755980-0889-11eb-827f-9b7aa02a556e.png)

You can even visit this slightly dated [interactive infographic](https://my-netdata.io/infographic.html) and get lost in a rabbit hole.

But the best way to get under the hood or in the steering wheel of our highly efficient, low-latency system (supporting multiple readers and one writer on each metric) is to read the rest of our docs, or just to jump in and [get started](https://github.com/netdata/netdata/blob/master/packaging/installer/README.md). But here's a good breakdown:

### Netdata Agent

Netdata's distributed monitoring Agent collects thousands of metrics from systems, hardware, and applications with zero configuration. It runs permanently on all your physical/virtual servers, containers, cloud deployments, and edge/IoT devices.

You can install Netdata on most Linux distributions (Ubuntu, Debian, CentOS, and more), container/microservice platforms (Kubernetes clusters, Docker), and many other operating systems (FreeBSD, macOS), with no sudo required.

### Netdata Cloud

Netdata Cloud is a web application that gives you real-time visibility for your entire infrastructure. With Netdata Cloud, you can view key metrics, insightful charts, and active alarms from all your nodes in a single web interface. When an anomaly strikes, seamlessly navigate to any node to troubleshoot and discover the root cause with the familiar Netdata dashboard.

Netdata Cloud is free! You can add an entire infrastructure of nodes, invite all your colleagues, and visualize any number of metrics, charts, and alarms entirely for free.

While Netdata Cloud offers a centralized method of monitoring your Agents, your metrics data is not stored or centralized in any way. Metrics data remains with your nodes and is only streamed to your browser, through Cloud, when you're viewing the Netdata Cloud interface.

## Use Netdata standalone or as part of your monitoring stack

Netdata is an extremely powerful monitoring, visualization, and troubleshooting platform. While you can use it as an
effective standalone tool, we also designed it to be open and interoperable with other tools you might already be using.

Netdata helps you collect everything and scales to infrastructure of any size, but it doesn't lock-in data or force you
to use specific tools or methodologies. Each feature is extensible and interoperable so they can work in parallel with
other tools. For example, you can use Netdata to collect metrics, visualize metrics with a second open-source program,
and centralize your metrics in a cloud-based time-series database solution for long-term storage or further analysis.

You can build a new monitoring stack, including Netdata, or integrate Netdata's metrics with your existing monitoring
stack. No matter which route you take, Netdata helps you monitor infrastructure of any size.

Here are a few ways to enrich your existing monitoring and troubleshooting stack with Netdata:

### Collect metrics from Prometheus endpoints

Netdata automatically detects 600 popular endpoints and collects per-second metrics from them via the [generic
Prometheus collector](https://github.com/netdata/go.d.plugin/blob/master/modules/prometheus/README.md). This even
includes support for Windows 10 via [`windows_exporter`](https://github.com/prometheus-community/windows_exporter).

This collector is installed and enabled on all Agent installations by default, so you don't need to waste time
configuring Netdata. Netdata will detect these Prometheus metrics endpoints and collect even more granular metrics than
your existing solutions. You can now use all of Netdata's meaningfully-visualized charts to diagnose issues and
troubleshoot anomalies.

### Export metrics to external time-series databases

Netdata can send its per-second metrics to external time-series databases, such as InfluxDB, Prometheus, Graphite,
TimescaleDB, ElasticSearch, AWS Kinesis Data Streams, Google Cloud Pub/Sub Service, and many others.

Once you have Netdata's metrics in a secondary time-series database, you can use them however you'd like, such as
additional visualization/dashboarding tools or aggregation of data from multiple sources.

### Visualize metrics with Grafana

One popular monitoring stack is Netdata, Prometheus, and Grafana. Netdata acts as the stack's metrics collection
powerhouse, Prometheus as the time-series database, and Grafana as the visualization platform. You can also use Grafite instead of Prometheus,
or  directly use the [Netdata source plugin for Grafana](https://blog.netdata.cloud/introducing-netdata-source-plugin-for-grafana/)

Of course, just because you export or visualize metrics elsewhere, it doesn't mean Netdata's equivalent features
disappear. You can always build new dashboards in Netdata Cloud, drill down into per-second metrics using Netdata's
charts, or use Netdata's health watchdog to send notifications whenever an anomaly strikes.

## Community

Netdata is an inclusive open-source project and community. Please read our [Code of Conduct](https://github.com/netdata/.github/blob/main/CODE_OF_CONDUCT.md).

Find most of the Netdata team in our [community forums](https://community.netdata.cloud). It's the best place to
ask questions, find resources, and engage with passionate professionals. The team is also available and active in our [Discord](https://discord.com/invite/mPZ6WZKKG2) too.

You can also find Netdata on:

- [Twitter](https://twitter.com/linuxnetdata)
- [YouTube](https://www.youtube.com/c/Netdata)
- [Reddit](https://www.reddit.com/r/netdata/)
- [LinkedIn](https://www.linkedin.com/company/netdata-cloud/)
- [StackShare](https://stackshare.io/netdata)
- [Product Hunt](https://www.producthunt.com/posts/netdata-monitoring-agent/)
- [Repology](https://repology.org/metapackage/netdata/versions)
- [Facebook](https://www.facebook.com/linuxnetdata/)

## Contribute

Contributions are the lifeblood of open-source projects. While we continue to invest in and improve Netdata, we need help to democratize monitoring!

- Read our [Contributing Guide](https://github.com/netdata/.github/blob/main/CONTRIBUTING.md), which contains all the information you need to contribute to Netdata, such as improving our documentation, engaging in the community, and developing new features. We've made it as frictionless as possible, but if you need help, just ping us on our community forums!
- We have a whole category dedicated to contributing and extending Netdata on our [community forums](https://community.netdata.cloud/c/agent-development/9)
- Found a bug? Open a [GitHub issue](https://github.com/netdata/netdata/issues/new?assignees=&labels=bug%2Cneeds+triage&template=BUG_REPORT.yml&title=%5BBug%5D%3A+).
- View our [Security Policy](https://github.com/netdata/netdata/security/policy).

Package maintainers should read the guide on [building Netdata from source](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/source.md) for
instructions on building each Netdata component from source and preparing a package.

## License

The Netdata Agent is an open source project distributed under [GPLv3+](https://github.com/netdata/netdata/blob/master/LICENSE). Netdata re-distributes other open-source tools and libraries. Please check the
[third party licenses](https://github.com/netdata/netdata/blob/master/REDISTRIBUTED.md).

## Is it any good?

Yes.

_When people first hear about a new product, they frequently ask if it is any good. A Hacker News user
[remarked](https://news.ycombinator.com/item?id=3067434):_

> Note to self: Starting immediately, all raganwald projects will have a “Is it any good?” section in the readme, and
> the answer shall be “yes.".
*******************************************************************************
