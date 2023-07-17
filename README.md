<p align="center">
<a href="https://www.netdata.cloud#gh-light-mode-only">
  <img src="https://raw.githubusercontent.com/netdata/website/master/themes/tailwind/static/img/netdata-logo-coloured.svg?token=GHSAT0AAAAAAB5ID66RRURF4GA2GUTO2NPYZFVLFMA#gh-light-mode-only" alt="Netdata" width="300"/>
</a>
<a href="https://www.netdata.cloud#gh-dark-mode-only">
  <img src="https://raw.githubusercontent.com/netdata/website/master/themes/tailwind/static/img/netdata-logo-white.svg?token=GHSAT0AAAAAAB5ID66QFQFDXG66SH76V26CZFVLHCA#gh-dark-mode-only" alt="Netdata" width="300"/>
</a>
</p>
<h3 align="center">Monitor your servers, containers, and applications,<br/>in high-resolution and in real-time.</h3>

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

Netdata collects metrics per second and presents them in beautiful low-latency dashboards. It is designed to run on all of your physical and virtual servers, cloud deployments, Kubernetes clusters, and edge/IoT devices, to monitor everything you run.

- :boom: **Collects metrics from 800+ integrations**<br/>
  Operating system metrics, container metrics, virtual machines, hardware sensors, applications metrics, OpenMetrics exporters, StatsD, and logs.
  
- :muscle: **Real-Time, Low-Latency, High-Resolution**<br/>
  All metrics are collected per second and are on the dashboard immediately after data collection. Netdata is designed to be fast.

- :face_in_clouds: **Unsupervised Anomaly Detection**<br/>
  Trains multiple Machine-Learning (ML) models for each metric collected and detects anomalies based on the past behavior of each metric individually.

- :fire: **Powerful Visualization**<br/>
  Clear and precise visualization that allows you to quickly understand any dataset, but also to filter, slice and dice the data directly on the dashboard, without the need to learn any query language.

- :bell: **Out of box Alerts**<br/>
  Comes with hundreds of alerts out of the box to detect common issues and pitfalls, revealing issues that can easily go unnoticed.

- :sunglasses: **Low Maintenance**<br/>
  Fully automated in every aspect: automated dashboards, out-of-the-box alerts, auto-detection, auto-discovery of metrics, and easily configurable.

- :star: **Open and Extensible**<br/>
  Netdata is a modular platform that can be extended in all possible ways and it also integrates nicely with other monitoring solutions.

<p align="center">
  <img src="https://raw.githubusercontent.com/cncf/artwork/master/other/cncf/horizontal/white/cncf-white.svg#gh-dark-mode-only" alt="CNCF" width="300">
  <img src="https://raw.githubusercontent.com/cncf/artwork/master/other/cncf/horizontal/black/cncf-black.svg#gh-light-mode-only" alt="CNCF" width="300">
  <br />
  Netdata actively supports and is a member of the Cloud Native Computing Foundation (CNCF)<br />
  &nbsp;<br/>
  ...and due to your love :heart:, it is the 3rd most :star:'d project in the <a href="https://landscape.cncf.io/card-mode?grouping=no&sort=stars">CNCF landscape</a>!
</p>

<hr class="solid">

![Netdata Agent](https://github.com/netdata/netdata/assets/2662304/af4caa23-19be-46ef-9779-8fdad8d99d2a)

<hr class="solid">

> :bulb: **Important Note**<br/>
> People get addicted to Netdata. **Once you use it on your systems, there's no going back!**<br/>
> _You have been warned..._<br/>

<hr class="solid">

## Quick Start

<p align="center">
  <a href="https://registry.my-netdata.io/#menu_netdata_submenu_registry"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_entries&dimensions=persons&label=user%20base&units=M&value_color=blue&precision=2&divide=1000000&options=unaligned&v44" alt="User base"></a>
  <a href="https://registry.my-netdata.io/#menu_netdata_submenu_registry"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_entries&dimensions=machines&label=servers%20monitored&units=M&divide=1000000&value_color=orange&precision=2&options=unaligned&v44" alt="Servers monitored"></a>
  <a href="https://registry.my-netdata.io/#menu_netdata_submenu_registry"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_sessions&label=sessions%20served&units=M&value_color=yellowgreen&precision=2&divide=1000000&options=unaligned&v44" alt="Sessions served"></a>
  <a href="https://hub.docker.com/r/netdata/netdata"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=dockerhub.pulls_sum&divide=1000000&precision=1&units=M&label=docker+hub+pulls&options=unaligned&v44" alt="Docker Hub pulls"></a>
  <br />
  <a href="https://registry.my-netdata.io/#menu_netdata_submenu_registry"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_entries&dimensions=persons&after=-86400&options=unaligned&group=incremental-sum&label=new%20users%20today&units=null&value_color=blue&precision=0&options=unaligned&v44" alt="New users today"></a>
  <a href="https://registry.my-netdata.io/#menu_netdata_submenu_registry"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_entries&dimensions=machines&group=incremental-sum&after=-86400&options=unaligned&label=servers%20added%20today&units=null&value_color=orange&precision=0&v44" alt="New machines today"></a>
  <a href="https://registry.my-netdata.io/#menu_netdata_submenu_registry"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_sessions&after=-86400&group=incremental-sum&options=unaligned&label=sessions%20served%20today&units=null&value_color=yellowgreen&precision=0&v44" alt="Sessions today"></a>
  <a href="https://hub.docker.com/r/netdata/netdata"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=dockerhub.pulls_sum&divide=1000&precision=1&units=k&label=docker+hub+pulls&after=-86400&group=incremental-sum&label=docker%20hub%20pulls%20today&options=unaligned&v44" alt="Docker Hub pulls today"></a>
</p>

### 1. **Install Netdata Agents everywhere** :v:
   
   Netdata can be installed on all Linux, MacOS, and FreeBSD systems. We provide binary packages for the most popular operating systems and package managers.

   - Install on [Ubuntu, Debian CentOS, Fedora, Suse, Red Hat, Arch, Alpine, Gentoo, even BusyBox](https://learn.netdata.cloud/docs/installing/one-line-installer-for-all-linux-systems).
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
   - Create, manage, and share **custom dashboards**
   - Invite your team and assign roles to them (Role Based Access Control - RBAC)
   - Access Netdata Functions (processes top from the UI and more)
   - Get infinite horizontal scalability (multiple independent parents are viewed as one infra)
   - Configure alerts from the UI (coming soon)
   - Configure data collection from the UI (coming soon)
   - Netdata Mobile App notifications (coming soon)

   :love_you_gesture: Netdata Cloud does not prevent you from using your Netdata Agents and Parents directly, and vice versa.<br/>
   :ok_hand: Your metrics are still stored in your network when you connect your Netdata Agents and Parents to Netdata Cloud.

## How it works

Netdata is built around a **modular metrics processing pipeline**.

Each Netdata Agent can perform the following functions:

1. **`COLLECT` metrics from their sources**<br/>
   Uses [internal](https://github.com/netdata/netdata/tree/master/collectors) and [external](https://github.com/netdata/go.d.plugin/tree/master/modules) plugins to collect data from their sources.

   Netdata auto-detects and collects almost everything from the operating system: including CPU, Interrupts, Memory, Disks, Mount Points, Filesystems, Network Stack, Network Interfaces, Containers, VMs, Processes, SystemD Units, Linux Performance Metrics, Linux eBPF, Hardware Sensors, IPMI, and more.

   It collects application metrics from applications: PostgreSQL, MySQL/MariaDB, Redis, MongoDB, Nginx, Apache, and hundreds more.

   Netdata also collects your custom application metrics by scraping OpenMetrics exporters, or via StatsD.

   It can convert web server log files to metrics and apply ML and alerts to them, in real-time.

   And it also supports synthetic tests / white box tests, so you can ping servers, check API responses, or even check filesystem files and directories to generate metrics, train ML and run alerts and notifications on their status.
   
2. **`STORE` metrics to a database**<br/>
   Uses database engine plugins to store the collected data, either in memory and/or on disk. We have developed our own [`dbengine`](https://github.com/netdata/netdata/tree/master/database/engine#readme) for storing the data in a very efficient manner, allowing Netdata to have less than 1 byte per sample on disk and amazingly fast queries.
   
3. **`LEARN` the behavior of metrics** (ML)<br/>
   Trains multiple Machine-Learning (ML) models per metric to learn the behavior of each metric individually.
   
4. **`DETECT` anomalies in metrics** (ML)<br/>
   Uses the trained machine learning (ML) models to detect outliers and mark collected samples as **anomalies**. Netdata stores anomaly information together with each sample and also streams it to Netdata Parents so that the anomaly is also available at query time for the whole retention of each metric.

5. **`CHECK` metrics and trigger alert notifications**<br/>
   Uses its configured alerts to check the metrics for common issues and send alert notifications.

6. **`STREAM` metrics to other Netdata Agents**<br/>
   Push metrics in real-time to Netdata Parents.

7. **`ARCHIVE` metrics to 3rd party databases**<br/>
   Export metrics to industry standard time-series databases, like `Prometheus`, `InfluxDB`, `OpenTSDB`, `Graphite`, etc.

8. **`QUERY` metrics and present dashboards**<br/>
   Provide an API to query the data and present interactive dashboards to users.

9. **`SCORE` metrics to reveal similarities and patterns**<br/>
   Score the metrics according to given criteria, to find the needle in the haystack.

When using Netdata Parents, all the functions of a Netdata Agent (except data collection) can be delegated to Parents to offload production systems.


## FAQ

### :shield: Is this secure?

Of course it is! We do our best to ensure it is!

<details><summary>Click to see detailed answer ...</summary>
&nbsp;<br/>&nbsp;<br/>

We understand that Netdata is a software piece that is installed on millions of production systems across the world. So, it is important for us, Netdata to be as secure as possible:

  - We follow the [Open Source Security Foundation](https://bestpractices.coreinfrastructure.org/en/projects/2231) best practices.
  - We have given great attention to detail when it comes to security design. Check out our [security design](https://learn.netdata.cloud/docs/architecture/security-and-privacy-design).
  - Netdata is a popular open-source project and is frequently tested by many security analysts.
  - Check also our [security policies and advisories published so far](https://github.com/netdata/netdata/security).

&nbsp;<br/>&nbsp;<br/>
</details>

### :cyclone: Will this consume a lot of resources on my servers?

No. It will not! We promise this will be fast!

<details><summary>Click to see detailed answer ...</summary>
&nbsp;<br/>&nbsp;<br/>

Although each Netdata Agent is a complete monitoring solution packed into a single application, and despite the fact that Netdata collects **every metric every single second** and trains **multiple ML models** per metric, you will find that Netdata has amazing performance! In many cases, it outperforms other monitoring solutions that have significantly fewer features or far smaller data collection rates.

This is what you should expect:

  - For production systems, each Netdata Agent with default settings (everything enabled, ML, Health, DB) should consume about 5% CPU utilization of one core and about 150 MiB or RAM.

    By using a Netdata parent and streaming all metrics to that parent, you can disable ML & health and use an ephemeral DB mode (like `alloc`) on the children, leading to utilization of about 1% CPU of a single core and 100 MiB of RAM. Of course, these depend on how many metrics are collected.
    
  - For Netdata Parents, for about 1 to 2 million metrics, all collected every second, we suggest a server with 16 cores and 32GB RAM. Less than half of it will be used for data collection and ML. The rest will be available for queries.

Netdata has extensive internal instrumentation to help us reveal how the resources consumed are used. All these are available in the "Netdata Monitoring" section of the dashboard. Depending on your use case, there are many options to optimize resource consumption.

Even if you need to run Netdata on extremely weak embedded or IoT systems, you will find that Netdata can be tuned to be very performant.

&nbsp;<br/>&nbsp;<br/>
</details>

### :scroll: How much retention can I have?

As much as you need!

<details><summary>Click to see detailed answer ...</summary>
&nbsp;<br/>&nbsp;<br/>

Netdata supports **tiering**, to downsample past data and save disk space. With default settings, it has 3 tiers:

  1. `tier 0`, with high resolution, per-second, data.
  2. `tier 1`, mid-resolution, per minute, data.
  3. `tier 2`, low-resolution, per hour, data.

All tiers are updated in parallel during data collection. Just increase the disk space you give to Netdata to get a longer history for your metrics. Tiers are automatically chosen at query time depending on the time frame and the resolution requested.

&nbsp;<br/>&nbsp;<br/>
</details>

### :rocket: Does it scale? I have really a lot of servers!

Yes, of course it does!

<details><summary>Click to see detailed answer ...</summary>
&nbsp;<br/>&nbsp;<br/>
Netdata is a distributed monitoring solution. You can scale it to infinity by spreading Netdata servers across your infrastructure.

With the streaming feature of Netdata Agents, we can support monitoring ephemeral servers, but also allow the creation of "monitoring islands" where metrics are aggregated to a few servers (Netdata Parents) for increased retention, or for offloading production systems.

  - :airplane: Netdata Parents provide great vertical scalability, so you can have as big parents as the CPU, RAM and Disk resources you can dedicate to them. In our lab we constantly stress test Netdata Parents with about 2 million metrics collected per second.
    
  - :rocket: In addition, Netdata Cloud provides virtually unlimited horizontal scalability. It "merges" all the Netdata parents you have into one unified infrastructure at query time. Netdata Cloud itself is probably the biggest single installation monitoring platform ever created, currently monitoring about 100k online servers with about 10k servers changing state (added/removed) per day!

&nbsp;<br/>&nbsp;<br/>
</details>

### :floppy_disk: My production servers are very sensitive in disk I/O. Can I use Netdata?

Yes, you can!

<details><summary>Click to see detailed answer ...</summary>
&nbsp;<br/>&nbsp;<br/>

We suggest the following:

  1. Use database mode `alloc` or `ram` to disable writing metric data to disk.
  2. Configure streaming to push in real-time all metrics to a Netdata Parent. The Netdata Parent will maintain metrics on disk for this node.
  3. Disable ML and health on this node. The Netdata Parent will do them for this node.
  4. Use the Netdata Parent to access the dashboard.

Using the above, the Netdata Agent on your production system will not need a disk.

&nbsp;<br/>&nbsp;<br/>
</details>

### :raised_eyebrow: How is Netdata different from a Prometheus and Grafana setup?

Netdata is a "ready to use" monitoring solution. Prometheus and Grafana are tools to build your own monitoring solution.

<details><summary>Click to see detailed answer ...</summary>
&nbsp;<br/>&nbsp;<br/>

First we have to say that Prometheus as a time-series database and Grafana as a visualizer are excellent tools for what they do.

However, we believe that such a setup is missing a key element: A Prometheus and Grafana setup assumes that you know everything about the metrics you collect and you understand deeply how they are structured, they should be queried and visualized.

In reality this setup has a lot of problems. The vast number of technologies, operating systems, and applications we use in our modern stacks, makes it impossible for any single person to know and understand everything about anything. We get testimonials regularly from Netdata users across the biggest enterprises, that Netdata manages to reveal issues, anomalies and problems they were not aware of and they didn't even have the means to find or troubleshoot.

So, the biggest difference of Netdata to Prometheus and Grafana, is that we decided that the tool needs to have a much better understanding of the components, the applications and the metrics it monitors.

  - When compared to Prometheus, Netdata needs for each metric much more than just a name, some labels and a value over time. A metric in Netdata is a structured entity that correlates with other metrics in a certain way, has specific attributes that depict how it should be organized, treated, queried and visualized. We call this the NIDL (Nodes, Instances, Dimensions, Labels) framework.

    To maintain such an index is a challenge: first because the raw metrics collected do not provide this information, so we have to add it, and second because we need to maintain this index for the lifetime of each metric, which with our current database retention, it is usually more than a year.

  - When compared to Grafana, Netdata is fully automated. Grafana has more customization capabilities than Netdata, but Netdata presents fully functional dashboards by itself and most importantly it gives you the means to understand, analyze, filter, slice and dice the data without the need for you to edit queries or be aware of any peculiarities the underlying metrics may have.

    Furthermore, to help you when you need to find the needle in the haystack, Netdata has advanced troubleshooting tools provided by the Netdata metrics scoring engine, that allows it to score metrics based on their anomaly rate, their differences or similarities for any given time-frame.

Still, if you are already familiar with Prometheus and Grafana, Netdata integrates nicely with them, and we have reports from users who use Netdata with Prometheus and Grafana in production.

&nbsp;<br/>&nbsp;<br/>
</details>

### :cloud: Do I have to subscribe to Netdata Cloud?

No. But we hope you will find it useful.

<details><summary>Click to see detailed answer ...</summary>
&nbsp;<br/>&nbsp;<br/>

The Netdata Agent dashboard and the Netdata Cloud dashboard are the same. Still, Netdata Cloud provides additional features, that the Netdata Agent is not capable of. These include:

  1. Customizability (custom dashboards and other settings are persisted when you are signed in to Netdata Cloud)
  2. Configuration of Alerts and Data Collection from the UI (coming soon)
  3. Security (role-based access control - RBAC).
  4. Horizontal Scalability ("blend" multiple independent parents in one uniform infrastructure)
  5. Central Dispatch of Alert Notifications (even when multiple independent parents are involved)
  6. Mobile App for Alert Notifications (coming soon)

So, although it is not required, you can get the most out of your Netdata installation by using Netdata Cloud.

&nbsp;<br/>&nbsp;<br/>
</details>

### :office: Who uses Netdata?

Netdata is a popular project. Almost everyone uses it.

<details><summary>Click to see detailed answer ...</summary>
&nbsp;<br/>&nbsp;<br/>

Check its [stargazers on github](https://github.com/netdata/netdata/stargazers). You will find people from quite popular companies and enterprises, including: SAP, Qualcomm, IBM, Amazon, Intel, AMD, Unity, Baidu, Cisco, Samsung, Netflix, Facebook and hundreds more.

Netdata is also popular in universities, including New York University, Columbia University, New Jersey University, and dozens more.

&nbsp;<br/>&nbsp;<br/>
</details>

## :book: Documentation

Netdata's documentation is available at [**Netdata Learn**](https://learn.netdata.cloud).

This site also hosts a number of [guides](https://learn.netdata.cloud/guides) to help newer users better understand how
to collect metrics, troubleshoot via charts, export to external databases, and more.

## :tada: Community

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

## :pray: Contribute

Contributions are essential to the success of open-source projects. In other words, we need your help to keep Netdata great!

What is a contribution? All the following are highly valuable to Netdata:

1. **Let us know of the best-practices you believe should be standardized**<br/>
   Netdata should out-of-the-box detect as many infrastructure issues as possible. By sharing your knowledge and experiences, you help us build a monitoring solution that has baked into it all the best-practices about infrastructure monitoring.

2. **Let us know if Netdata is not perfect for your use case**<br/>
   We aim to support as many use cases, operating systems and integrations, as possible. We rely on you to bring to our attention that Netdata is sub-optimal for a certain use-case. Open a github issue, or start a github discussion about it, to discuss what you need and what you believe Netdata should be doing for your use case.

   Keep in mind though that we can't promise we will implement it. We prioritize development on use-cases that are common to our community, are in the same direction we want Netdata to evolve and are aligned with our roadmap.

4. **Support other community members**<br/>
   Join our community on Github, Discord and Reddit and help them out. Generally, Netdata is relatively easy to setup and configure, but still people may need a little push in the right direction to use it effectively. Supporting other members is a great contribution by itself!

5. **Add or improve integrations you need**<br/>
   Integrations are generally easier and simpler to develop. If you want to contribute code to Netdata, we suggest to start with integrations you need and Netdata may not currently support.

General information about contributions:

- Check our [Security Policy](https://github.com/netdata/netdata/security/policy).
- Found a bug? Open a [GitHub issue](https://github.com/netdata/netdata/issues/new?assignees=&labels=bug%2Cneeds+triage&template=BUG_REPORT.yml&title=%5BBug%5D%3A+).
- Read our [Contributing Guide](https://github.com/netdata/.github/blob/main/CONTRIBUTING.md), which contains all the information you need to contribute to Netdata, such as improving our documentation, engaging in the community, and developing new features. We've made it as frictionless as possible, but if you need help, just ping us on our community forums!
- We have a whole category dedicated to contributing and extending Netdata on our [community forums](https://community.netdata.cloud/c/agent-development/9)

Package maintainers should read the guide on [building Netdata from source](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/source.md) for
instructions on building each Netdata component from the source and preparing a package.

## License

The Netdata Agent is [GPLv3+](https://github.com/netdata/netdata/blob/master/LICENSE). Netdata re-distributes other open-source tools and libraries. Please check the
[third party licenses](https://github.com/netdata/netdata/blob/master/REDISTRIBUTED.md).
