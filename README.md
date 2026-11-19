<p align="center">
<a href="https://www.netdata.cloud#gh-light-mode-only">
  <img src="https://www.netdata.cloud/img/readme-images/netdata_readme_logo_light.png" alt="Netdata" width="300"/>
</a>
<a href="https://www.netdata.cloud#gh-dark-mode-only">
  <img src="https://www.netdata.cloud/img/readme-images/netdata_readme_logo_dark.png" alt="Netdata" width="300"/>
</a>
</p>
<h3 align="center">Full-stack observability: the AI SRE that learns your stack!</h3>
<p align="center">
  <b>Metrics · Logs · Traces · Flows · Topology</b><br/>
  Infrastructure · APM · NPM · DBM · RUM · Synthetics · Cloud · AI<br/>
  OpenTelemetry · SNMP · NetFlow/IPFIX/sFlow · IPMI · Redfish · 2,000+ integrations
</p>

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

<p align="center">
  <a href="https://registry.my-netdata.io/#menu_netdata_submenu_registry"><img src="https://registry.my-netdata.io/api/v3/badge.svg?chart=netdata.registry_entries&dimensions=persons&label=user%20base&units=M&value_color=blue&precision=2&divide=1000000&options=unaligned&tier=1&v44" alt="User base"></a>
  <a href="https://registry.my-netdata.io/#menu_netdata_submenu_registry"><img src="https://registry.my-netdata.io/api/v3/badge.svg?chart=netdata.registry_entries&dimensions=machines&label=servers%20monitored&units=M&divide=1000000&value_color=orange&precision=2&options=unaligned&tier=1&v44" alt="Servers monitored"></a>
  <a href="https://registry.my-netdata.io/#menu_netdata_submenu_registry"><img src="https://registry.my-netdata.io/api/v3/badge.svg?chart=netdata.registry_sessions&label=sessions%20served&units=M&value_color=yellowgreen&precision=2&divide=1000000&options=unaligned&tier=1&v44" alt="Sessions served"></a>
  <a href="https://hub.docker.com/r/netdata/netdata"><img src="https://registry.my-netdata.io/api/v3/badge.svg?chart=dockerhub.pulls_sum&divide=1000000&precision=1&units=M&label=docker+hub+pulls&options=unaligned&tier=1&v44" alt="Docker Hub pulls"></a>
</p>
<p align="center"><b>Visit our <a href="https://www.netdata.cloud">Home Page</a></b></p>

<hr class="solid">

MENU: **[WHO WE ARE](#who-we-are)** | **[KEY FEATURES](#key-features)** | **[GETTING STARTED](#getting-started)** | **[HOW IT WORKS](#how-it-works)** | **[FAQ](#faq)** | **[DOCS](#book-documentation)** | **[COMMUNITY](#tada-community)** | **[CONTRIBUTE](#pray-contribute)** | **[LICENSE](#scroll-license)**


> [!WARNING]
> People **get addicted to Netdata.**
> Once you use it on your systems, *there's no going back.*

[![Platforms](https://img.shields.io/badge/Platforms-Linux%20%7C%20macOS%20%7C%20FreeBSD%20%7C%20Windows-blue)]()

---

## WHO WE ARE

Netdata is a full-stack observability platform: infrastructure, application performance monitoring (OpenTelemetry traces, service maps, RED), digital experience (real user monitoring, synthetic monitoring), network performance monitoring (SNMP, topology, NetFlow/IPFIX/sFlow, traps, BGP), databases, logs (monitoring and management) and cloud, with AI troubleshooting over all of it.

Netdata is distributed and real-time. Netdata Agents collect, store and analyze data where it is generated, per second, with machine learning anomaly detection on every metric. Netdata Parents and Netdata Cloud combine them into one view, without centralizing the data. Netdata scales to 100,000+ nodes, and its Agent is open source.

**Netdata Design:**

* **Understanding over traffic lights** – Monitoring tells you that something is wrong; observability tells you why. Netdata gives you both, from the same data.
* **Everything, every second** – Netdata collects everything, every second, and engineers the cost down (about 0.6 bytes per sample) until choosing what to watch is no longer necessary.
* **Independent hubs, not a central database** – Data stays close to where it is created, in independent hubs you place where they make sense. Netdata Cloud unifies and correlates them, and Netdata scales to 100,000+ nodes.
* **Meaning at collection time** – Every piece of data gets its meaning when it is collected, so dashboards build themselves and every chart is correct.
* **Machine learning that models your entire stack** – Every metric is scored for anomalies every second. Clusters of anomalies reveal hidden dependencies, show how problems cascade across systems, networks and applications, can identify issues before they become outages, steer root cause analysis with data, and surface the actual impact of failures.
* **AI at the core** – Machine learning, a scoring engine and AI work together to deliver automated root-cause and impact analysis. MCP servers give any AI assistant the same evidence.
* **Expertise built in** – No query language and nothing to administer: the expertise is built into the product, so the whole team can investigate.
* **Configured your way** – Automatic first, then as code, from the dashboard, or both.
* **Open, and yours** – The engine is open source, speaks open standards and runs on your infrastructure.

Read [Netdata Design](https://learn.netdata.cloud/docs/welcome-to-netdata/netdata-design) for the reasoning behind each decision, and the [Complete Product Description](https://learn.netdata.cloud/docs/welcome-to-netdata/complete-product-description) for every capability.

### Most Energy-Efficient Monitoring Tool

<p align="center">
<a href="https://www.ivanomalavolta.com/files/papers/ICSOC_2023.pdf#gh-dark-mode-only">
  <img src="https://github.com/netdata/netdata/assets/139226121/7118757a-38fb-48d7-b12a-53e709a8e8c0" alt="Energy Efficiency" width="800"/>
</a>
<a href="https://www.ivanomalavolta.com/files/papers/ICSOC_2023.pdf#gh-light-mode-only">
  <img src="https://github.com/netdata/netdata/assets/139226121/4f64cbb6-05e4-48e3-b7c0-d1b79e37e219" alt="Energy efficiency" width="800"/>
</a>
</p>

According to the [University of Amsterdam study](https://www.ivanomalavolta.com/files/papers/ICSOC_2023.pdf), Netdata is the most energy-efficient tool for monitoring Docker-based systems. The study also shows Netdata excels in CPU usage, RAM usage, and execution time compared to other monitoring solutions.

---

## Key Features

| Area                                    | What you get                                                                                                     |
|-----------------------------------------|------------------------------------------------------------------------------------------------------------------|
| **Infrastructure monitoring**           | Systems, containers, Kubernetes, virtualization and hardware through 2,000+ integrations, per second             |
| **Application performance monitoring**  | Find any trace by ID, filter traces by any attribute, and see latency percentiles and errors for any slice of traffic, with service maps and browser sessions linked to backend traces. Traces stay on your infrastructure. |
| **Digital experience monitoring**       | Real user monitoring with Core Web Vitals and JavaScript errors, Playwright journeys, Lighthouse audits, uptime   |
| **Network performance monitoring**      | Zero-configuration SNMP with 1,750+ device profiles, L2/L3 topology, NetFlow/IPFIX/sFlow, SNMP traps, BGP         |
| **Database monitoring**                 | 50+ databases and managed services, with live top-query views on 12 engines                                     |
| **Logs monitoring**                     | systemd-journal, Windows Event Log and macOS unified logs queried in place on every node, with no shipping or extra storage |
| **Logs management**                      | Collect logs from any source through OpenTelemetry and syslog, search every field, and explore with facets and histograms. Logs are acknowledged only once safely on disk and are searchable as they arrive, with retention and offload to your own S3 storage. |
| **Cloud monitoring**                    | 31 AWS services through CloudWatch, 38 Azure services through Azure Monitor, and Google Cloud                     |
| **Endpoint and edge monitoring**        | Windows, macOS, IoT and embedded devices with the same Agent                                                     |
| **Alerting**                            | 1,100+ stock alerts, your own alerts as code, from the UI or generated by AI, 30+ notification integrations      |
| **Machine learning and AI**             | Anomaly detection on every metric, Netdata AI investigations, MCP servers for any AI assistant                   |
| **Long-term retention**                 | About 0.6 bytes per sample, with tiered storage that keeps per-second, per-minute and per-hour history            |
| **Scalability**                         | Independent hubs (Parents for metrics; Agents for devices, logs, traces and flows) unified by Netdata Cloud, 100,000+ nodes |

---

## Netdata Ecosystem

This three-part architecture scales from a single node to fleets of 100,000+ nodes:

| Component         | Description                                                                                                                                                                                         | License                                         |
|-------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-------------------------------------------------|
| **Netdata Agent** | • Core engine: collection, storage, machine learning, alerting, streaming, export<br>• Also a hub: Parents for metrics; Agents for SNMP, flows, traps, logs and traces<br>• Runs on servers, cloud, Kubernetes, Windows, macOS and edge devices<br>• MCP server built in | [GPL v3+](https://www.gnu.org/licenses/gpl-3.0) |
| **Netdata Cloud** | • Unifies and correlates all hubs in one view<br>• Netdata AI, traces explorer, Network Monitor, digital experience and uptime views<br>• RBAC, SSO, SCIM<br>• SaaS or Enterprise On-Prem<br>• Free community tier<br>• Data stays on your infrastructure |                                                 |
| **Netdata UI**    | • Dashboards and visualizations<br>• Free to use<br>• Included in standard packages<br>• Latest version via CDN                                                                                     | [NCUL1](https://app.netdata.cloud/LICENSE.txt)  |

## What You Can Monitor

With Netdata you can monitor all these components across platforms:

|                                                                                                   Component |              Linux               | FreeBSD | macOS |                      Windows                      |
|------------------------------------------------------------------------------------------------------------:|:--------------------------------:|:-------:|:-----:|:-------------------------------------------------:|
|                             **System Resources**<small><br/>CPU, Memory and system shared resources</small> |               Full               |   Yes   |  Yes  |                        Yes                        |
|                                **Storage**<small><br/>Disks, Mount points, Filesystems, RAID arrays</small> |               Full               |   Yes   |  Yes  |                        Yes                        |
|                                 **Network**<small><br/>Interfaces, Protocols, Firewall, etc</small> |               Full               |   Yes   |  Yes  |                        Yes                        |
|                        **Hardware & Sensors**<small><br/>Fans, Temperatures, Controllers, GPUs, etc</small> |               Full               |  Some   | Some  |                       Some                        |
|                                       **O/S Services**<small><br/>Resources, Performance and Status</small> | Yes<small><br/>`systemd`</small> |    -    |   -   |         Yes<small><br/>`Windows Services`</small>         |
|                                      **Processes**<small><br/>Resources, Performance, OOM, and more</small> |               Yes                |   Yes   |  Yes  |                        Yes                        |
|                                                                             **Logs Monitoring**<small><br/>Logs queried where the OS stores them</small> | Yes<small><br/>`systemd`-journal |    -    | Yes<small><br/>unified logs</small> | Yes<small><br/>`Windows Event Log`, `ETW`</small> |
|                **Logs Management**<small><br/>OpenTelemetry logs and syslog, collected and indexed</small> |               Yes                |    -    |  Yes  |                         -                         |
|                                 **Network Connections**<small><br/>Live TCP and UDP sockets per PID</small> |               Yes                |   Yes   |  Yes  |                        Yes                        |
|        **Network Performance Monitoring**<small><br/>SNMP devices, Topology, Traps, BGP</small> |               Full               |  Full   | Full  |                       Full                        |
|                                 **Network Flows**<small><br/>NetFlow, IPFIX, sFlow</small> |               Yes                |    -    |   -   |                         -                         |
|                               **Containers**<small><br/>Docker/containerd, LXC/LXD, Kubernetes, etc</small> |               Yes                |    -    |   -   |                         -                         |
|                                 **VMs** (from the host)<small><br/>KVM, qemu, libvirt, Proxmox, etc</small> | Yes<small><br/>`cgroups`</small> |    -    |   -   |         Yes<small><br/>`Hyper-V`</small>          |
|                       **Synthetic Checks**<small><br/>Test APIs, TCP ports, Ping, Certificates, etc</small> |               Yes                |   Yes   |  Yes  |                        Yes                        |
| **Packaged Applications**<small><br/>nginx, apache, postgres, redis, mongodb,<br/>and hundreds more</small> |               Yes                |   Yes   |  Yes  |                        Yes                        |
|                              **Cloud Services**<small><br/>AWS, Azure and Google Cloud</small> |               Yes                |   Yes   |  Yes  |                        Yes                        |
|                       **Custom Applications**<small><br/>OpenMetrics and StatsD</small> |               Yes                |   Yes   |  Yes  |                        Yes                        |
|                      **OpenTelemetry (OTLP)**<small><br/>Metrics, logs and traces over gRPC and HTTP</small>|               Yes                |    -    |  Yes  |                         -                         |

On Linux, you can continuously monitor all kernel features and hardware sensors for errors, including Intel/AMD/Nvidia GPUs, PCI AER, RAM EDAC, IPMI, S.M.A.R.T, Intel RAPL, NVMe, fans, power supplies, and voltage readings.

---

## Getting Started

You can install Netdata on all major operating systems. To begin:

### 1. Install Netdata

Choose your platform and follow the installation guide:

* [Linux Installation](https://learn.netdata.cloud/docs/installing/one-line-installer-for-all-linux-systems)
* [macOS](https://learn.netdata.cloud/docs/installing/macos)
* [FreeBSD](https://learn.netdata.cloud/docs/installing/freebsd)
* [Windows](https://learn.netdata.cloud/docs/netdata-agent/installation/windows)
* [Docker Guide](/packaging/docker/README.md)
* [Kubernetes Setup](https://learn.netdata.cloud/docs/installation/install-on-specific-environments/kubernetes)

> [!NOTE]
> You can access the Netdata UI at `http://localhost:19999` (or `http://NODE:19999` if remote).

### 2. Configure Collectors

Netdata auto-discovers most metrics, but you can manually configure some collectors:

* [All collectors](https://learn.netdata.cloud/docs/data-collection/)
* [SNMP monitoring](https://learn.netdata.cloud/docs/data-collection/monitor-anything/networking/snmp)

### 3. Configure Alerts

You can use 1,100+ stock alerts, add your own, and send notifications to:

`email`, `Slack`, `Telegram`, `PagerDuty`, `Discord`, `Microsoft Teams`, and more.

> [!NOTE]  
> Email alerts work by default if there's a configured MTA.

### 4. Configure Parents

You can centralize dashboards, alerts, and storage with Netdata Parents:

* [Streaming Reference](https://learn.netdata.cloud/docs/streaming/streaming-configuration-reference)

> [!NOTE]  
> You can use Netdata Parents for central dashboards, longer retention, and alert configuration.

### 5. Connect to Netdata Cloud

[Sign in to Netdata Cloud](https://app.netdata.cloud/sign-in) and connect your nodes for:

* Access from anywhere
* Horizontal scalability and multi-node dashboards
* UI configuration for alerts and data collection
* Role-based access control
* Free tier available

> [!NOTE]  
> Netdata Cloud is optional. Your data stays in your infrastructure.

## Live Demo Sites

<p align="center">
  <b>See Netdata in action</b><br/>
  <a href="https://frankfurt.netdata.rocks"><b>FRANKFURT</b></a> |
  <a href="https://newyork.netdata.rocks"><b>NEWYORK</b></a> |
  <a href="https://atlanta.netdata.rocks"><b>ATLANTA</b></a> |
  <a href="https://sanfrancisco.netdata.rocks"><b>SANFRANCISCO</b></a> |
  <a href="https://toronto.netdata.rocks"><b>TORONTO</b></a> |
  <a href="https://singapore.netdata.rocks"><b>SINGAPORE</b></a> |
  <a href="https://bangalore.netdata.rocks"><b>BANGALORE</b></a>
  <br/>
  <i>These demo clusters run with default configuration and show real monitoring data.</i>
  <br/>
  <i>Choose the instance closest to you for the best performance.</i>
</p>

---

## How It Works

Netdata Agents collect, store, analyze and alert where data is created. Hubs concentrate each kind of data where you choose, and Netdata Cloud unifies them into one view.

```mermaid
flowchart LR
  subgraph Sources[What you monitor]
    S1[Servers, VMs, containers, Kubernetes]
    S2[Network devices: SNMP, flows, traps]
    S3[Hardware: IPMI, Redfish]
    S4[Applications: OpenTelemetry traces and logs]
    S5[Browsers and synthetic checks]
  end
  subgraph Hubs[Hubs you place per site, region or team]
    H1[Agents and Parents: metrics]
    H2[Agents: network devices and hardware]
    H3[Agents: logs, traces, flows, RUM]
  end
  C[Netdata Cloud: one view, correlated]:::mainNode
  S1 --> H1
  S2 --> H2
  S3 --> H2
  S4 --> H3
  S5 --> H3
  H1 <--> C
  H2 <--> C
  H3 <--> C

  classDef mainNode fill:#f0f0f0,stroke:#333,stroke-width:1px,color:#333
```

* **Agents** run on every system and collect, store, analyze and alert where the data is created.
* **Hubs** concentrate each kind of data where you choose. They are independent: capacity grows by adding hubs, and a problem in one hub stays in that hub.
* **Netdata Cloud** routes every query to the hubs that hold the answer and correlates the results into one view. It is not in the data path.

### Inside every Agent

```mermaid
flowchart TB
  A[Netdata Agent]
  A1(Collect) --> A
  A2(Store) --> A
  A3(Learn) --> A
  A4(Detect) --> A
  A5(Check) --> A
  A6(Stream) --> A
  A7(Archive) --> A
  A8(Query) --> A
  A9(Score) --> A
```

With each Agent you can:

1. **Collect** – Gather metrics from systems, containers, apps, logs, APIs, and synthetic checks.
2. **Store** – Save metrics to a high-efficiency, tiered time-series database.
3. **Learn** – Train ML models per metric using recent behavior.
4. **Detect** – Identify anomalies using trained ML models.
5. **Check** – Evaluate metrics against pre-set or custom alert rules.
6. **Stream** – Send metrics to Netdata Parents in real time.
7. **Archive** – Export metrics to Prometheus, InfluxDB, OpenTSDB, Graphite, and others.
8. **Query** – Access metrics via an API for dashboards or third-party tools.
9. **Score** – Use a scoring engine to find patterns and correlations across metrics.

> [!NOTE]  
> Learn more: [Netdata's architecture](https://learn.netdata.cloud/docs/netdata-agent/#distributed-observability-pipeline)

## Agent Capabilities

With the Netdata Agent, you can use these core capabilities out-of-the-box:

| Capability                   | Description                                                                                                                                   |
|------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------|
| **Collection**               | • 2,000+ integrations<br>• Systems, containers, VMs, hardware sensors<br>• SNMP devices, NetFlow/IPFIX/sFlow, SNMP traps<br>• OpenMetrics, StatsD, and OpenTelemetry (OTLP metrics, logs and traces)<br>• Real user monitoring and synthetic checks |
| **Resolution**               | • Per-second collection<br>• Real-time visualization with 1-second latency<br>• High-resolution metrics                                       |
| **Machine Learning**         | • ML models trained at the edge<br>• Automatic anomaly detection per metric<br>• Scoring engine that ranks what changed in any time window     |
| **Logs Monitoring**          | • systemd-journal, Windows Event Log and macOS unified logs queried in place<br>• No log shipping, pipelines or extra storage<br>• Rich log visualization |
| **Logs Management**          | • OpenTelemetry logs and syslog from any source<br>• Every field searchable, with facets and histograms<br>• Acknowledged only once safely on disk<br>• Searchable as logs arrive<br>• Retention by size, age and count; offload to your own S3 storage |
| **APM and Traces**           | • Find any trace by ID<br>• Filter by any attribute<br>• Latency percentiles and errors for any slice of traffic<br>• Per-service request rate, errors and duration<br>• Service map |
| **Streaming and Replication** | • Parent-Child relationships<br>• Flexible centralization<br>• Multi-level replication and retention                                          |
| **Visualization**            | • NIDL data model<br>• Auto-generated dashboards<br>• No query language needed                                                                |
| **Alerting**                 | • 1,100+ stock alerts and your own<br>• Multiple notification methods<br>• Evaluated on every Agent, without Netdata Cloud                    |
| **AI and MCP**               | • MCP server on every Agent and Parent<br>• Any AI assistant can query metrics, logs, alerts and live views                                   |
| **Operations**               | • Auto-detection<br>• Zero-touch ML<br>• Scales to 100,000+ nodes<br>• CI/CD friendly                                                                 |
| **Extensibility**            | • Modular architecture<br>• Configurable as code, from the UI, or both<br>• Integrates with existing tools                                                             |

---

## CNCF Membership

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/cncf/artwork/master/other/cncf/horizontal/white/cncf-white.svg">
    <source media="(prefers-color-scheme: light)" srcset="https://raw.githubusercontent.com/cncf/artwork/master/other/cncf/horizontal/color/cncf-color.svg">
    <img alt="CNCF Logo" src="https://raw.githubusercontent.com/cncf/artwork/master/other/cncf/horizontal/color/cncf-color.svg" width="300">
  </picture>
  <br />
  Netdata actively supports and is a member of the Cloud Native Computing Foundation (CNCF).<br />
  It is one of the most starred projects in the <a href="https://landscape.cncf.io/?item=observability-and-analysis--observability--netdata">CNCF landscape</a>.
</p>

---

## Our Story

<details>
  <summary><strong>All heroes have a great origin story. Click to discover ours.</strong></summary>
  <br/>

In 2013, at the company where Costa Tsaousis was COO, a significant percentage of their cloud-based transactions failed silently, severely impacting business performance.

Costa and his team tried every troubleshooting tool available at the time. None could identify the root cause. As Costa later wrote:

“*I couldn’t believe that monitoring systems provide so few metrics and with such low resolution, scale so badly, and cost so much to run.*”

Frustrated, he decided to build his own monitoring tool, starting from scratch.

That decision led to countless late nights and weekends. It also sparked a fundamental shift in how infrastructure monitoring and troubleshooting are approached, both in method and in cost.
</details>

---

## FAQ

<details>
<summary><strong>Is Netdata secure?</strong></summary>
<br/>

Yes. Netdata follows [OpenSSF best practices](https://bestpractices.coreinfrastructure.org/en/projects/2231), has a security-first design, and is regularly audited by the community.

* [Security design](https://learn.netdata.cloud/docs/security-and-privacy-design)
* [Security policies and advisories](https://github.com/netdata/netdata/security)

</details>

<details>
<summary><strong>Does Netdata use a lot of resources?</strong></summary>
<br/>

No. Even with ML and per-second metrics, Netdata uses minimal resources.

* Under 5% of one CPU core and about 200 MB of RAM on a typical server, with machine learning enabled
* About 30 MiB of RAM on edge devices configured as a lightweight Child
* A Parent handles about 500 nodes or 2 million metrics per second; add Parents to grow

> You can use the **Netdata Monitoring** section in the dashboard to inspect its resource usage.

</details>

<details>
<summary><strong>How much data retention is possible?</strong></summary>
<br/>

As much as your disk allows.

With Netdata you can use tiered retention:

* Tier 0: per-second resolution
* Tier 1: per-minute resolution
* Tier 2: per-hour resolution

These are queried automatically based on the zoom level.
</details>

<details>
<summary><strong>Can Netdata scale to many servers?</strong></summary>
<br/>

Yes. With Netdata you can:

* Scale out with independent hubs: Parents for metrics, and Agents for network devices, logs, traces and flows
* Add capacity by adding hubs; queries run in parallel where the data is
* Scale to 100,000+ nodes; Netdata Cloud SaaS serves more than 100,000 reachable nodes

> Netdata Cloud unifies and correlates all hubs into one view.

</details>

<details>
<summary><strong>Is disk I/O a concern?</strong></summary>
<br/>

No. Netdata minimizes disk usage:

* Metrics are flushed to disk every 17 minutes, spread out evenly
* Uses direct I/O and compression (ZSTD)
* Can run entirely in RAM or stream to a Parent

> You can use `alloc` or `ram` mode for no disk writes.

</details>

<details>
<summary><strong>Can Netdata replace my current monitoring stack?</strong></summary>
<br/>

Yes. Netdata is one product for infrastructure, applications, digital experience, networks, databases, logs and cloud.

* No manual setup or dashboards needed
* Built-in machine learning, alerts, dashboards, correlations and AI
* Takes in Prometheus, OpenTelemetry, Nagios and StatsD data, so existing exporters and instrumentation keep working

</details>

<details>
<summary><strong>How is Netdata different from commercial SaaS tools?</strong></summary>
<br/>

With Netdata you can store all metrics on your infrastructure—no sampling, no aggregation, no loss.

* High-resolution metrics by default
* ML per metric, not shared models
* Scales to 100,000+ nodes without skyrocketing cost

</details>

<details>
<summary><strong>What if I feel overwhelmed?</strong></summary>
<br/>

You can start small:

* Use the dashboard's table of contents and search
* Explore anomaly scoring ("AR" toggle)
* Create custom dashboards in Netdata Cloud

> [Docs and guides](https://learn.netdata.cloud/guides)

</details>

<details>
<summary><strong>Do I have to use Netdata Cloud?</strong></summary>
<br/>

No. Netdata Cloud is optional.

Netdata works without it, but with Cloud you can:

* Access remotely with SSO
* Save dashboard customizations
* Configure alerts centrally
* Collaborate with role-based access

</details>

<details>
<summary><strong>What telemetry does Netdata collect?</strong></summary>
<br/>

Anonymous telemetry helps improve the product. You can disable it:

* Add `--disable-telemetry` to the installer, or
* Create `/etc/netdata/.opt-out-from-anonymous-statistics` and restart Netdata

> Telemetry helps us understand usage, not track users. No private data is collected.

</details>

<details>
<summary><strong>Who uses Netdata?</strong></summary>
<br/>

You'll join users including:

* Major companies (Amazon, ABN AMRO Bank, Facebook, Google, IBM, Intel, Netflix, Samsung)
* Universities (NYU, Columbia, Seoul National, UCL)
* Government organizations worldwide
* Infrastructure-intensive organizations
* Technology operators
* Startups and freelancers
* SysAdmins and DevOps professionals

</details>

---

## \:book: Documentation

Visit [Netdata Learn](https://learn.netdata.cloud) for full documentation and guides. Start with the core documents:

* [Welcome to Netdata](https://learn.netdata.cloud/docs/welcome-to-netdata): what Netdata is and the outcomes it delivers
* [Netdata Design](https://learn.netdata.cloud/docs/welcome-to-netdata/netdata-design): what Netdata believes about observability and how its design delivers it
* [Complete Product Description](https://learn.netdata.cloud/docs/welcome-to-netdata/complete-product-description): every capability

> [!NOTE]  
> Includes deployment, configuration, alerting, exporting, troubleshooting, and more.

---

## \:tada: Community

Join the Netdata community:

* [Discord](https://discord.com/invite/2mEmfW735j)
* [Forum](https://community.netdata.cloud)
* [GitHub Discussions](https://github.com/netdata/netdata/discussions)

> [!NOTE]  
> [Code of Conduct](https://github.com/netdata/.github/blob/main/CODE_OF_CONDUCT.md)

Follow us on:
[Twitter](https://twitter.com/netdatahq) | [Reddit](https://www.reddit.com/r/netdata/) | [YouTube](https://www.youtube.com/c/Netdata) | [LinkedIn](https://www.linkedin.com/company/netdata-cloud/)

---

## \:pray: Contribute

We welcome your contributions.

Ways you help us stay sharp:

* Share best practices and monitoring insights
* Report issues or missing features
* Improve documentation
* Develop new integrations or collectors
* Help users in forums and chats

> [!NOTE]  
> [Contribution guide](https://github.com/netdata/.github/blob/main/CONTRIBUTING.md)

---

## \:scroll: License

The Netdata ecosystem includes:

* **Netdata Agent** – Open-source core (GPLv3+). **Includes** data collection, storage, ML, alerting, APIs and **redistributes** several other open-source tools and libraries.
    * [Netdata Agent License](https://github.com/netdata/netdata/blob/master/LICENSE)
    * [Netdata Agent Redistributed](https://github.com/netdata/netdata/blob/master/REDISTRIBUTED.md)
* **Netdata UI** – Closed-source but free to use with Netdata Agent and Cloud. Delivered via CDN. It integrates third-party open-source components.
    * [Netdata Cloud UI License](https://app.netdata.cloud/LICENSE.txt)
    * [Netdata UI third-party licenses](https://app.netdata.cloud/3D_PARTY_LICENSES.txt)
* **Netdata Cloud** – Closed-source, with free and paid tiers. Adds remote access, SSO, scalability.
