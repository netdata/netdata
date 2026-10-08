# Netdata: Complete Product Description

## Executive Summary

Netdata is a distributed, real-time observability platform for metrics, logs and traces. Netdata Agents collect, store and analyze data where it is generated — per second, with machine learning anomaly detection on every metric — and Netdata Parents and Netdata Cloud combine them into one view, without centralizing the data.

Netdata covers the full stack in one product: infrastructure monitoring (systems, containers, Kubernetes, virtualization, hardware, applications) through 800+ integrations and the Prometheus, OpenTelemetry, Nagios and StatsD ecosystems; network performance monitoring (devices, topology, flows, traps, BGP, licenses, application dependencies); database monitoring for 50+ databases and managed services with live query views; logs, from in-place querying on every node to OpenTelemetry-based logs management; OpenTelemetry traces; AWS and Azure monitoring; Windows, macOS and edge devices; and digital experience monitoring with RUM and synthetic journeys.

Everything is automatic: discovery, dashboards, 1,100+ stock alerts and anomaly detection start without configuration, and every metric is explorable without a query language. Netdata AI and MCP servers make the whole infrastructure available to AI for troubleshooting and root-cause analysis. Data stays on the customer's infrastructure, with Netdata Cloud available as SaaS or Enterprise On-Prem.

## Platform Architecture

Netdata is a distributed observability platform. Netdata Agents collect, store and analyze data on the systems where it is generated; Netdata Parents centralize data from many Agents; Netdata Cloud (SaaS or On-Prem) provides a unified view, access control and collaboration without storing metric data. The design keeps per-second data, machine learning and alerting at the edge, and scales by adding Agents and Parents instead of growing a central database.

**Learn more:** [Architecture and scalability](/docs/scalability.md) · [Distributed observability pipeline on netdata.cloud](https://www.netdata.cloud/features/architecture/distributed-observability-data-pipeline/)

**Design**

- **[Netdata Agent](/docs/deployment-guides/standalone-deployment.md).** One process per system: collects metrics (per second for system metrics), stores them in a local multi-tier time-series database, trains machine learning models per metric, evaluates alerts, serves dashboards, Functions and an MCP server, and streams to Parents.
- **[Netdata Parents](/docs/deployment-guides/deployment-with-centralization-points.md).** Agents that receive streams from other Agents (Children), storing their metrics, running machine learning and alerting on their behalf, and serving their dashboards and queries. Parents form active-active clusters; Children stream to one Parent at a time and fail over to others automatically.
- **[Netdata Cloud](/docs/netdata-cloud/README.md).** The control plane: fleet-wide dashboards, Rooms, users and roles, alert notifications, AI features and Functions routing. Netdata Cloud receives metadata and status changes, not metric samples; dashboards query Agents and Parents in real time. Available as SaaS or Enterprise On-Prem (including air-gapped).
- **Distributed queries.** Netdata Cloud fans queries out to the Agents and Parents holding the data and merges the results, so fleet-wide views do not require centralizing the data.

**Capabilities**

**Storage — [DBENGINE](/src/database/README.md)**
- Multi-tier time-series database, default for Agents and Parents: tier 0 per-second, tier 1 per-minute, tier 2 per-hour (up to 5 tiers supported)
- [Default retention](/docs/netdata-agent/sizing-netdata-agents/disk-requirements-and-retention.md) per tier by time and size, whichever comes first: tier 0 14 days, tier 1 3 months, tier 2 2 years, each capped at 1 GiB (about 4 GiB per Agent with metadata); configurable per tier
- About 0.6 bytes per sample on disk at tier 0 (Gorilla and ZSTD compression); about 6 and 18 bytes per sample at tiers 1 and 2, which store sum, min, max, count and anomaly count per interval
- Anomaly bit stored with every sample, in real-time, at collection time
- Alternative modes: `ram` (in memory), `alloc`, `none` (streaming only)
- [Extreme cardinality protection](/docs/extreme-cardinality-protection.md): automatic cleanup of archived, ephemeral metric instances, without touching collected metrics

**[Streaming](/src/streaming/README.md) and Parents**
- Real-time streaming from Children to Parents with compression (ZSTD default; LZ4, gzip, Brotli) and TLS
- Multiple Parent destinations with [automatic failover](/docs/streaming-routing.md); balancing to the Parent with the freshest data
- [Replication](/docs/observability-centralization-points/metrics-centralization-points/replication-of-past-samples.md): after a disconnection, the Parent back-fills the gap from the Child's retained data
- [Active-active Parent clusters](/docs/observability-centralization-points/metrics-centralization-points/clustering-and-high-availability-of-netdata-parents.md): every Parent receives all data; machine learning models are trained where the data is first stored and shared — Children stream trained models to their Parents, and Parent clusters train once and share models — so no node retrains models another has trained
- [Streaming topology Function](/src/go/plugin/go.d/collector/snmp/npm-catalog/integrations/netdata_streaming_topology.md) (`topology:streaming`) and per-Child streaming statistics on Parents

**Fleet and configuration**
- [Dynamic configuration](/docs/netdata-agent/configuration/dynamic-configuration.md) of collectors, service discovery, alerts and virtual nodes from the UI, without restarts
- [Secrets management](/src/collectors/SECRETS.md): references to environment variables, files, commands and secret stores — HashiCorp Vault, AWS Secrets Manager, Azure Key Vault, Google Secret Manager
- [Virtual nodes](/docs/netdata-agent/configuration/organize-systems-metrics-and-alerts.md) for remote and API-monitored targets; [ephemeral nodes](/docs/nodes-ephemerality.md) with automatic cleanup in Netdata Cloud
- Host labels (automatic and user-defined)

**Deployment**
- Linux: [one-line installer](/packaging/installer/methods/kickstart.md) (`kickstart.sh`), native packages (DEB, RPM) for current Debian, Ubuntu, RHEL-compatible, Fedora, Amazon Linux, openSUSE releases; static builds for x86_64, ARM64, ARMv7, ARMv6
- [Docker images](/packaging/docker/README.md) (Docker Hub, GHCR, Quay); [Kubernetes](/packaging/installer/methods/kubernetes.md) through the Netdata Helm chart (Child DaemonSet, Parents, cluster-state collector, optional OpenTelemetry deployment)
- [Windows](/packaging/windows/WINDOWS_INSTALLER.md) (MSI installer), [macOS](/packaging/installer/methods/macos.md) (installer, Homebrew), [FreeBSD](/packaging/installer/methods/freebsd.md)
- [Release channels](/docs/learn/switching-install-types.md): stable and nightly; [automatic updates](/packaging/installer/UPDATE.md)
- [Enterprise On-Prem](https://www.netdata.cloud/product/cloud-on-premises/): the full Netdata Cloud on customer infrastructure, deployed with Helm, including air-gapped environments

**Scale and resources**
- From single nodes to 100,000+ nodes; Netdata Cloud SaaS serves more than 100,000 reachable nodes
- Parent sizing: about 500 nodes or 2 million metrics per second per Parent (each Parent in a cluster holds all data; about 20 CPU cores, 80 GB RAM); 100 nodes ≈ 0.5M metrics/s on 5 cores and 20 GB; 250 nodes ≈ 1M metrics/s on 10 cores and 40 GB
- [Agent resource use](/docs/netdata-agent/sizing-netdata-agents/README.md): \<5% CPU and 150MB RAM (standalone, ML enabled); in detail, 1–5% CPU and 100–200 MB RAM on an idle system; 5–20% of one CPU core and 250–350 MB RAM in production workloads; machine learning and alerting can be offloaded to Parents, special tuning for IoT
- [Netdata benchmark (2025)](https://www.netdata.cloud/blog/netdata-vs-prometheus-2025/), Netdata vs a Prometheus stack at 4.6 million metrics per second: 37% less CPU, 88% less RAM, 97% less disk I/O, 13% less bandwidth, 15× longer per-second retention (up to 40× in lower tiers), 22× faster on large queries, 100% sample completeness vs 93.7%

**Value**

- **No central bottleneck.** Collection, storage, machine learning and alerting run where the data is; adding systems adds capacity instead of loading a central database.
- **Per-second data kept affordably.** Compression of about 0.6 bytes per sample and tiered retention keep per-second detail and long-term history on modest storage.
- **Resilient by design.** Parent clusters, automatic failover and replication keep monitoring available through Parent failures and network outages.
- **Data stays on the customer's infrastructure.** Metric samples are stored on Agents and Parents; Netdata Cloud receives only metadata, with an On-Prem option for full control.
- **Operated from the UI.** Collectors, alerts and virtual nodes are configured centrally, with secrets kept in the customer's vaults.
- **Predictable scaling.** Published sizing for Parents and Agents makes capacity planning straightforward from a few nodes to 100,000+ nodes.

## Infrastructure Monitoring

Netdata monitors the full infrastructure stack — operating systems, processes, containers, Kubernetes, virtualization, hardware, applications and services — through 800+ integrations and the Prometheus, OpenTelemetry, Nagios and StatsD ecosystems. Every collected metric is visualized automatically and analyzed by machine learning anomaly detection, at per-second resolution for system metrics, and the entire infrastructure is available to AI assistants for troubleshooting. Each monitored system runs a Netdata Agent that collects locally; Netdata Parents and Netdata Cloud combine the fleet into one view.

**Learn more:** [Collecting metrics documentation](/src/collectors/README.md) · [Infrastructure monitoring on netdata.cloud](https://www.netdata.cloud/solutions/use-cases/infrastructure-monitoring/)

**Design**

- **[Every metric visualized and analyzed automatically](/docs/dashboards-and-charts/netdata-charts.md).** Metrics from native collectors, Prometheus exporters, OpenTelemetry sources, Nagios plugins and StatsD clients are charted, grouped by their labels and analyzed by machine learning the moment they are collected — no dashboard design, no per-chart queries, no query language. Native collectors and profiles add full structure (contexts, instances, dimensions, families) declaratively.
- **[Machine learning on every metric](/docs/ml-ai/ml-anomaly-detection/ml-anomaly-detection.md), [AI over everything](/docs/netdata-ai/mcp/README.md).** Every metric gets unsupervised anomaly detection trained locally on its own behavior, with anomaly rates shown on every chart and used to find anomalous and correlated metrics across the infrastructure. Through Netdata's MCP server and Netdata AI, AI assistants query metrics, anomalies, correlations, alerts and live Functions (processes, containers, connections, logs) to investigate and explain issues.
- **[Local collection on every system](/docs/realtime-monitoring.md).** The Netdata Agent collects from the kernel, processes, containers, hardware and local applications on the system it runs on, at one-second resolution by default for system metrics; remote and API-based targets are collected as frequently as needed.
- **[Automatic discovery](/src/collectors/SERVICE-DISCOVERY.md).** Service discovery finds local applications (listening ports and processes), Docker containers and Kubernetes workloads, and starts the matching collectors; 650+ known Prometheus exporter ports are recognized automatically.
- **[Structured metrics, automatic dashboards](/docs/NIDL-Framework.md).** Collectors organize metrics into contexts, instances, dimensions and labels (the NIDL model), so every chart can be sliced, filtered and grouped from the dashboard without query languages or dashboard building.
- **[Configuration from the UI](/docs/netdata-agent/configuration/dynamic-configuration.md).** Every go.d collector, service-discovery pipeline and alert can be configured from the Netdata UI through dynamic configuration, without editing files or restarting the Agent.
- **[Functions for live inspection](/docs/top-monitoring-netdata-functions.md).** Collectors expose Functions — live tables queried on demand from the node (processes, containers, services, connections, sensors, hardware, queries) — through the dashboard, Netdata Cloud and MCP.

**Capabilities**

**Operating systems**
- **Linux:** CPU (per core, frequency, idle states, interrupts, softirqs), pressure stall information (PSI), memory (NUMA, KSM, zram, zswap, huge pages, fragmentation, ECC errors), disks and filesystems (I/O, latency, utilization, mount points, inodes, btrfs, ZFS ARC, mdstat), network interfaces and protocols (IP, TCP, UDP, SCTP, softnet, conntrack, SYNPROXY, IPVS, QoS classes, netfilter accounting, wireless, InfiniBand), PCIe errors (AER), power (RAPL), audit, entropy, time synchronization
- **[eBPF](/docs/developer-and-contributor-corner/monitor-debug-applications-ebpf.md):** kernel-level process, file, socket, DNS, page cache, directory cache, mount and OOM-kill monitoring
- **Windows, macOS, FreeBSD:** see Endpoint and Edge Monitoring
- **[systemd](/src/collectors/cgroups.plugin/integrations/systemd_services.md):** service resource usage, unit states, logind sessions

**Processes**
- [Per-process, per-application group](/src/collectors/apps.plugin/integrations/applications.md), per-user and per-group CPU, memory, disk I/O, open files, sockets, threads, uptime and more (`apps.plugin`), with configurable application groups
- [Live process table](/docs/functions/processes.md) (Function `processes`)

**Containers and Kubernetes**
- [Container and VM resource monitoring](/src/collectors/cgroups.plugin/README.md) through cgroups: Docker, Podman, containerd, LXC/Incus, systemd-nspawn, Nomad, Amazon ECS, Kubernetes (including OpenShift and managed Kubernetes services), Proxmox, libvirt/KVM, OpenStack, oVirt
- Kubernetes metadata on container metrics: namespace, pod, controller, container, QoS class and more; pod-level name overrides through annotations
- Kubernetes collectors: [cluster state](/src/go/plugin/go.d/collector/k8s_state/integrations/kubernetes_cluster_state.md) (`k8s_state`), kubelet, kube-proxy, API server
- [Docker engine](/src/go/plugin/go.d/collector/docker/integrations/docker.md) and Docker Hub collectors; live container table (Function `docker:container-ls`)
- [Kubernetes Explorer](/docs/dashboards-and-charts/kubernetes-tab.md) in Netdata Cloud: workloads and nodes views; Kubernetes and container overview dashboards
- Deployment on Kubernetes through the [Netdata Helm chart](https://github.com/netdata/helmchart/blob/master/charts/netdata/README.md)

**Virtualization**
- [VMware vSphere](/src/go/plugin/go.d/collector/vsphere/integrations/vmware_vcenter_server.md) (hosts, VMs, datastores, clusters, resource pools; readiness and topology Functions) and vCenter Server Appliance health
- [Proxmox](/src/collectors/guides/proxmox/integrations/proxmox_ve_monitoring.md) (VMs and containers through cgroups; Proxmox VE API through a Prometheus profile), KVM/libvirt, Xen (`xenstat`), Hyper-V (Windows), LXC

**Hardware**
- [Redfish BMCs](/src/go/plugin/go.d/collector/redfish/integrations/redfish.md): sensors, hardware inventory and BMC event logs (Functions `redfish:sensors`, `redfish:hardware`, `redfish:logs`)
- [IPMI](/src/collectors/freeipmi.plugin/integrations/intelligent_platform_management_interface_ipmi.md) sensors and system event log (Function `ipmi-sensors`)
- [Hardware sensors](/src/collectors/debugfs.plugin/integrations/linux_hardware_sensors_libsensors.md) (temperature, voltage, fans, power; Function `sensors`), [memory module inventory](/src/go/plugin/go.d/collector/smbios_memory/integrations/smbios_memory.md) with drift detection (Function `smbios-memory-inventory`)
- [Disk health](/src/go/plugin/go.d/collector/smartctl/integrations/s.m.a.r.t..md): SMART (`smartctl`), NVMe, `hddtemp`; RAID controllers: MegaCLI, StorCLI, Adaptec, HPE Smart Array; Linux software RAID, LVM, dm-cache, ZFS pools
- [GPUs](/src/go/plugin/go.d/collector/nvidia_smi/integrations/nvidia_gpu.md): NVIDIA (`nvidia_smi`, DCGM with 183 metrics), Intel, AMD (kernel DRM), Apple Silicon (macOS)
- UPS: NUT (`upsd`), APC (`apcupsd`); NIC optics (`ethtool`); 1-Wire sensors
- Enterprise storage: Dell PowerStore, PowerVault, ScaleIO; HDFS; S3-compatible object storage checks (`s3check`)
- **[Ceph](/docs/guides/ceph/README.md):** dedicated collector (cluster status, hosts, monitors, managers, object and iSCSI gateways, physical capacity, objects and copy health, placement groups, client I/O, recovery, scrubbing; per-OSD status, space, I/O and latency; per-pool space, objects and I/O), 100 stock alerts, a Prometheus profile for the Ceph exporter (569 chart templates), and live `ceph:osds` and `ceph:pools` Functions

**Applications and services** (dedicated collectors)
- **Web servers and proxies:** Apache, NGINX (including NGINX Plus, Unit, VTS), Envoy, Traefik, lighttpd, LiteSpeed, Tengine, Tomcat, uWSGI, PHP-FPM, Squid, Varnish; web server and Squid log parsing (`weblog`, `squidlog`); HAProxy and Caddy through Prometheus
- **Message brokers:** RabbitMQ, ActiveMQ, NATS, Pulsar, VerneMQ, Beanstalk; IBM MQ; Kafka and Mosquitto through Prometheus
- **DNS, DHCP and time:** CoreDNS, PowerDNS (authoritative and recursor), Unbound, dnsdist, dnsmasq (DNS and DHCP), NSD, Pi-hole, ISC DHCP, chrony, NTPd
- **VPN:** OpenVPN, WireGuard, Libreswan, Tor
- **Mail and identity:** Postfix, Dovecot, Exim, Rspamd, OpenLDAP, FreeRADIUS, fail2ban
- **Application servers and middleware:** IBM WebSphere (JMX, MicroProfile, PMI), IBM i, ZooKeeper, Consul, Gearman, Logstash, Fluentd
- **Process supervisors and DevOps:** Supervisord, Monit, Puppet; Jenkins, GitLab Runner and others through Prometheus
- **[AI serving](/src/go/plugin/go.d/collector/prometheus/integrations/vllm.md):** vLLM and LiteLLM through Prometheus profiles
- **Other:** Go-Ethereum, Icecast, SpigotMC, BOINC, Go expvar, OpenSIPS
- Databases, network devices, cloud services and OpenTelemetry: see their sections

**Endpoint checks**
- [HTTP endpoints](/src/go/plugin/go.d/collector/httpcheck/integrations/http_endpoints.md) (`httpcheck`), TCP/UDP ports (`portcheck`), ICMP (`ping`), DNS queries (`dnsquery`), TLS certificate expiry (`x509check`), domain expiry (`whoisquery`), files and directories (`filecheck`), S3-compatible storage (`s3check`)

**Monitoring ecosystems**
- **[Prometheus and OpenMetrics](/src/go/plugin/go.d/collector/prometheus/integrations/prometheus_endpoint.md):** any exporter or `/metrics` endpoint; 160+ cataloged exporters and automatic discovery of 650+ known exporter ports. Every metric is charted automatically, with machine learning and alerting. **[Prometheus profiles](/src/go/plugin/go.d/collector/prometheus/profile-format.md)** turn any exporter into a structured Netdata dashboard by declaring a few attributes per metric family (context, instance, dimension, family, units) — replacing per-panel dashboard building; stock profiles for Ceph (569 chart templates), HAProxy, vLLM, LiteLLM, FastAPI, process runtime and Python GC; user-defined profiles; relabeling engine; counters, gauges, histograms and summaries handled natively
- **[OpenTelemetry](/docs/category-overview-pages/opentelemetry.md):** metrics, logs and traces from any OpenTelemetry SDK or Collector receiver (see OpenTelemetry)
- **[Nagios](/src/go/plugin/scripts.d/collector/nagios/integrations/nagios_plugins_and_custom_scripts.md):** any of the thousands of Nagios-compatible check plugins; check results and performance data become Netdata metrics and alerts (`scripts.d` `nagios` collector)
- **[StatsD](/src/collectors/statsd.plugin/README.md):** counters, gauges, timers, histograms, meters, sets and dictionaries from any application, with synthetic charts

**Custom collection**
- Generic [`sql` collector](/src/go/plugin/go.d/collector/sql/integrations/sql_databases_generic.md) (any SQL query as metrics and live tables) and the [external plugin protocol](/src/plugins.d/README.md) for collectors in any language

**Functions** (live inspection, on demand)
- System: `processes`, `containers-vms`, `systemd-services`, `systemd-list-units`, `mount-points`, `block-devices`, `network-interfaces`, `network-connections`, `network-protocols`, `dns-queries`, `topology:network-connections`
- Hardware: `sensors`, `ipmi-sensors`, `redfish:sensors`, `redfish:hardware`, `redfish:logs`, `smbios-memory-inventory`
- Containers, storage, virtualization: `docker:container-ls`, `ceph:osds`, `ceph:pools`, `vsphere:readiness`, `topology:vsphere`
- Netdata itself: `netdata-streaming`, `topology:streaming`, `netdata-api-calls`, `netdata-metrics-cardinality`, `config`
- Logs, databases, network and OpenTelemetry Functions: see their sections

**Fleet organization**
- 37 [automatic host labels](/docs/netdata-agent/configuration/organize-systems-metrics-and-alerts.md) (operating system, kernel, architecture, virtualization, container, cloud provider and instance details, Kubernetes, and more), plus user-defined host labels
- Virtual nodes for remote and API-monitored targets

**Value**

- **Coverage of the whole stack without assembling a toolchain.** One Agent monitors the operating system, processes, containers, Kubernetes, hardware and applications, replacing separate exporters, agents and dashboard projects per technology.
- **Monitoring that starts on its own.** Applications, containers and Kubernetes workloads are discovered and charted automatically; collectors are configured from the UI when needed.
- **Every second, every metric.** Per-second system metrics capture short spikes, saturation and contention that 10–60 second monitoring averages away.
- **Anomalies surfaced across the whole stack.** Machine learning watches every metric of every system, container, device and application; unusual behavior is detected without thresholds, and correlated anomalies point to where a problem started.
- **AI-assisted troubleshooting on live data.** AI assistants investigate incidents using the infrastructure's real metrics, anomalies, alerts and live Functions, reducing the time from question to root cause.
- **Down to the process, container and device.** Metrics resolve to individual processes, containers, pods, disks, interfaces, GPUs and sensors, so problems are located to their source.
- **Live answers from the node.** Functions return live process, container, service, connection, sensor and hardware tables on demand, without logging into the machine.
- **Any metric source, with dashboards from day one.** Existing Prometheus exporters, OpenTelemetry instrumentation, Nagios checks and StatsD clients work as they are; their metrics are charted and analyzed automatically, and profiles turn them into structured dashboards without dashboard design or query languages.
- **Organized automatically.** Automatic host labels group the fleet by operating system, cloud, virtualization and Kubernetes context without manual tagging.

## Endpoint and Edge Monitoring

Netdata monitors Windows and macOS endpoints and servers, and edge and embedded devices — IoT devices and gateways, robots, embedded systems, digital signage players and in-vehicle computers — with the same Agent, per-second metrics, logs, machine learning and alerting it uses for servers. On constrained devices the Agent adapts its footprint automatically; on intermittent links it streams to a Netdata Parent and back-fills data after reconnecting.

**Learn more:** [Edge device monitoring documentation](/docs/fleet-management/README.md) · [Edge fleet monitoring on netdata.cloud](https://www.netdata.cloud/solutions/use-cases/edge-fleet-monitoring/)

**Design**

- **One Agent from servers to devices.** The same Netdata Agent runs on servers, Windows and macOS endpoints, and ARM and x86 edge devices; all appear as nodes with the same dashboards, alerts and anomaly detection.
- **[Automatic footprint adaptation](/docs/fleet-management/minimize-cpu-and-memory.md).** On devices with one CPU core or less than 1 GiB of RAM, the Agent applies its `iot` profile automatically: machine learning off, reduced memory use, lower streaming compression effort.
- **[Thin devices, central storage](/docs/fleet-management/minimize-disk-footprint.md).** Devices can keep metrics in RAM only, or not store them at all, and stream to a Netdata Parent that stores, analyzes and alerts on their behalf (machine learning and alerting run on the Parent).
- **[Resilience on intermittent links](/docs/fleet-management/disconnected-devices-and-failover.md).** After a disconnection, the device back-fills missed data to its Parent (by default, the Parent requests up to one day of missed data; longer windows are configurable), so gaps in connectivity do not become gaps in monitoring.
- **[Cellular data budgets](/docs/fleet-management/minimize-cellular-traffic.md).** Sparse collection (down to five or ten minutes), compressed streaming and keepalives that the Parent automatically matches to the collection cadence keep a device connected for live troubleshooting at about 4–7 MiB of streaming traffic per month.
- **[Reduced device images](/packaging/makeself/README.md).** A build-host tool produces reduced static installers that keep only the capabilities each device class needs, with symbols stripped (ARMv7 payload 676 MiB → 41 MiB, installer 19.3 MiB).

**Capabilities**

**Windows** (desktops, laptops, servers)
- 24 native modules (423 chart types): CPU, memory, disks (including Cluster Shared Volumes), network, services, processes, Remote Desktop sessions, battery and power supply, hardware sensors, CPU temperature, thermal zones, Hyper-V, IIS, Active Directory, AD Certificate Services, AD Federation Services, Exchange, ASP.NET, .NET Framework
- Per-application process monitoring, with [grouping by Windows service](/src/collectors/apps.plugin/integrations/windows_services.md)
- Live network connections and application dependency mapping
- [Windows Event Log explorer](/src/collectors/windows-events.plugin/README.md) (event logs, including ETW and TraceLogging events routed to the Event Log)
- Cross-platform collectors on Windows: SQL Server, disk health, RAID controllers, NVIDIA GPUs and others
- [MSI installer](/packaging/windows/WINDOWS_INSTALLER.md) with silent, offline and interactive installation, for standard software-distribution tools

**macOS**
- [8 native modules](/src/collectors/macos.plugin/integrations/macos.md): CPU, memory, disks, network, battery and UPS, Apple Silicon GPU, temperature, fan, voltage and power sensors, thermal pressure, NVMe health
- [macOS unified log explorer](/src/collectors/macos-logs.plugin/README.md)
- Per-application process monitoring; live network connections and application dependency mapping
- OpenTelemetry ingestion
- Installation through the [one-line installer or Homebrew](/packaging/installer/methods/macos.md)

**FreeBSD**
- 30 native modules, per-application process monitoring, live network connections, system-call monitoring

**Edge and embedded devices**
- [Linux builds](/packaging/PLATFORM_SUPPORT.md) for x86_64, ARM64, ARMv7 and ARMv6 (including Raspberry Pi devices); native packages for ARM64 and ARMv7 on Debian and Ubuntu
- Automatic `iot` profile for constrained devices; RAM-only and no-storage modes; thin children streaming to Parents; back-fill after reconnect
- Device hardware: hardware sensors (temperature, voltage, fans), 1-Wire sensors, I²C temperature and humidity sensors (with Raspberry Pi wiring guide), battery and power supply, Wi-Fi, UPS, disk and SD/NVMe health, GPUs
- Industrial and field devices through Prometheus exporters: [Modbus](/src/go/plugin/go.d/collector/prometheus/integrations/modbus_protocol.md), Siemens S7 PLCs, GPS (gpsd), vehicle telemetry and others; cellular routers through SNMP (see Network Performance Monitoring)

**Robotics, vehicle and AI-edge devices**
- NVIDIA Jetson: native monitoring of GPU, CPU, memory, power rails and temperatures, plus full operating-system monitoring (Jetson runs Ubuntu-based L4T on ARM64)
- ROS application telemetry through ROS-topic Prometheus exporters, via Netdata's Prometheus collector
- CAN bus interfaces (SocketCAN, e.g. `can0`) monitored as network interfaces — frames, errors, drops; decoded signals through a decode pipeline (DBC → Prometheus, StatsD or OTLP)

**Fleet footprint and connectivity**
- Collection intervals from one second to ten minutes, per device and per collector; streaming filters keep charts local without sending them
- Automatic TCP keepalives matched to collection cadence; [per-device streaming statistics](/docs/fleet-management/monitor-the-fleet.md) (bytes, reconnects, replication) on the Parent
- Reduced static installers (`prepare-fleet.py`): capability bundles, symbol stripping, checksums and manifest; ARMv6 and ARMv7 examples
- SD-card protection: metric history in RAM, quiet metadata database, optional Agent logs off
- Measured footprint: Raspberry Pi 3 B+ under 1% of one CPU core and about 30 MiB of memory
- Outage recovery from a RAM buffer (about 3.6 days at five-minute collection, with the Parent's replication period extended to match) with Parent back-fill; automatic failover between Parents; ephemeral nodes for sleeping devices
- [Nodes Map](/docs/dashboards-and-charts/home-tab.md): devices grouped and colored by labels and metrics, node notes, geographic location
- Application operating metrics through local StatsD; local alert actions that run while disconnected; image-managed updates with preserved device identity

**Fleet operations**
- [Host labels](/docs/fleet-management/deployment-and-identity.md) for grouping devices by site, model, customer or role; [Rooms filled from labels](/docs/netdata-cloud/node-rule-based-room-assignment.md)
- [Ephemeral nodes](/docs/nodes-ephemerality.md) for devices that come and go
- Configuration changes from the Netdata UI without restarts (dynamic configuration)
- [Automatic updates](/docs/fleet-management/updates-and-troubleshooting.md)

**Value**

- **Performance data for every endpoint.** Windows and macOS machines report per-second processes, resources, hardware health and logs, so slow-machine and user-experience issues are investigated with data.
- **The same tool for servers, endpoints and devices.** One Agent, one data model and one set of dashboards and alerts from data-center servers to laptops and field devices.
- **Runs on constrained hardware without tuning.** The Agent detects constrained devices and adapts its footprint automatically.
- **No data loss on unreliable links.** Back-fill after reconnect keeps device history complete across connectivity gaps.
- **Device health visibility.** Temperature, power, battery, storage wear and sensors are monitored alongside software metrics, exposing hardware-driven failures in the field.
- **Predictable cellular cost.** Devices stay connected and troubleshootable within a few MiB of streaming traffic per month.
- **Fits small images and protects flash storage.** Reduced installers of about 20 MiB, and no continuous metric writes to SD cards.
- **Fleet-scale organization.** Labels, Rooms and ephemeral nodes keep thousands of devices organized by site, customer or role.

## Cloud Monitoring (AWS and Azure)

Netdata monitors AWS and Azure managed services through their native monitoring APIs: 31 AWS services (47 service profiles, 440+ metrics) through Amazon CloudWatch, and 38 Azure services (38 service profiles, 1,300+ metrics) through Azure Monitor, with 490+ stock alerts. Managed services are monitored next to the virtual machines, containers and applications Netdata monitors with its Agents.

**Learn more:** [AWS CloudWatch collector documentation](/src/go/plugin/go.d/collector/cloudwatch/integrations/amazon_cloudwatch.md) · [Cloud monitoring on netdata.cloud](https://www.netdata.cloud/solutions/use-cases/cloud-monitoring/)

**Design**

- **Collection through the cloud provider's monitoring API.** The `cloudwatch` collector uses batched CloudWatch `GetMetricData` requests with metric discovery; the `azure_monitor` collector uses the Azure Monitor Metrics batch API with resource discovery through Azure Resource Graph. No agent runs inside managed services.
- **[Service profiles](/src/go/plugin/go.d/collector/cloudwatch/profile-format.md).** Each service is described by a declarative profile that defines its metrics, charts and grouping; profiles are documented, and users can add or modify profiles for additional services or metrics without code changes.
- **Resources as chart instances.** Each cloud resource (instance, database, load balancer, queue, function, bucket) is an instance of its service's charts, labelled with account or subscription, region, resource group, resource identifiers and selected tags, so resources can be filtered, grouped and compared without building dashboards. On Azure, resources can also be grouped into workload virtual nodes by tag.
- **Many accounts and subscriptions per collector.** One collector job covers multiple AWS accounts and regions (up to 64 targets, each with its own assumed role) or multiple Azure subscriptions; public, government and China partitions are supported.
- **API cost visibility.** The AWS collector charts its own CloudWatch API usage as it happens.

**Capabilities**

**AWS** — [Amazon CloudWatch](/src/go/plugin/go.d/collector/cloudwatch/integrations/amazon_cloudwatch.md) (`cloudwatch` collector)
- **Compute and serverless:** EC2, Auto Scaling, Lambda, Step Functions
- **Containers:** ECS, EKS (control plane)
- **Databases and analytics:** RDS, DynamoDB (including per-operation metrics), ElastiCache, DocumentDB, Redshift, OpenSearch Service
- **Storage:** S3 (storage and request metrics), EBS (including stalled I/O checks), EFS
- **Networking and delivery:** Classic, Application and Network Load Balancers (including target-group health and detail), NAT Gateway, Site-to-Site VPN, CloudFront, PrivateLink (endpoints and endpoint services, by subnet, availability zone, load balancer and consumer VPC endpoint)
- **Messaging, streaming and events:** SQS, SNS, Kinesis Data Streams, Data Firehose, MSK (brokers and clusters), EventBridge
- **Application services and AI:** API Gateway, Bedrock
- **Cost:** AWS Billing estimated charges — total, by service, by linked account
- **Authentication:** AWS SDK default chain (environment, shared profiles, instance roles, IRSA), static keys, role assumption with external ID per target
- **Scope and filtering:** multiple accounts and regions per job; resource filtering and labelling by tags
- 36 stock alerts

**Azure** — [Azure Monitor](/src/go/plugin/go.d/collector/azure_monitor/integrations/azure_monitor.md) (`azure_monitor` collector)
- **Compute:** Virtual Machines, VM Scale Sets, App Service and Azure Functions
- **Containers:** AKS, Container Apps, Container Instances, Container Registry
- **Databases:** SQL Database, SQL Elastic Pool, SQL Managed Instance, Database for PostgreSQL Flexible Server, Database for MySQL Flexible Server, Cosmos DB, Cache for Redis
- **Storage:** Storage Accounts
- **Networking:** Load Balancer, Application Gateway, Front Door, Firewall, NAT Gateway, VPN Gateway, ExpressRoute circuits and gateways
- **Integration and messaging:** API Management, Logic Apps, Service Bus, Event Hubs, Event Grid, IoT Hub
- **Analytics:** Data Explorer, Data Factory, Synapse Analytics, Stream Analytics
- **AI:** Azure AI services (Cognitive Services), Machine Learning
- **Observability and security:** Application Insights, Log Analytics, Key Vault
- **Authentication:** service principal, managed identity, Azure SDK default chain
- **Scope and filtering:** multiple subscriptions per job; discovery scoped by resource group, region, tag or custom Resource Graph (KQL) query; profiles activate automatically for discovered resource types
- 454 stock alerts

**Related cloud integrations**
- Entra ID (Azure AD) authentication for SQL Server, PostgreSQL and the generic `sql` collector
- [Secrets](/src/collectors/SECRETS.md) from AWS Secrets Manager and Azure Key Vault in collector configurations
- Alert notifications through [AWS SNS](/integrations/cloud-notifications/integrations/amazon_sns.md); metrics export to [AWS Kinesis](/src/exporting/aws_kinesis/integrations/aws_kinesis.md)
- Network flows [labelled with AWS and Azure published IP ranges](/docs/npm/network-flows/enrichment.md) (see Network Performance Monitoring)

**Value**

- **Managed services and infrastructure in one place.** Cloud services appear next to the VMs, containers and applications monitored by Netdata Agents, on the same timeline, removing the need to switch between cloud consoles and the monitoring tool.
- **One view across accounts and subscriptions.** Multiple AWS accounts and regions, and multiple Azure subscriptions, are monitored together instead of per-console.
- **Coverage without building dashboards.** Profiles define metrics, charts and alerts for each service; resources are discovered and charted automatically.
- **Alerting and anomaly detection on cloud metrics.** 490+ stock alerts, and machine learning anomaly detection on every cloud metric.
- **Cost awareness.** AWS Billing charges are monitored by service and linked account, and the collector shows its own CloudWatch API usage.
- **Extensible to any service.** Documented profiles let users add services and metrics the stock profiles do not cover.

## Database Monitoring

Netdata monitors 50+ databases and managed database services — relational, NoSQL, caches, search engines, connection poolers, and AWS and Azure managed databases — with per-object metrics (databases, tables, indexes, replicas, wait types), top-query views on 12 engines, replication and high-availability monitoring, and machine learning anomaly detection on every metric.

**Learn more:** [Database query views documentation](/docs/functions/databases.md) · [Database monitoring on netdata.cloud](https://www.netdata.cloud/solutions/use-cases/database-monitoring/)

**Design**

- **Collection next to the database.** The Netdata Agent on the database host (or the site's Agent, for remote and cloud-managed databases) detects database instances and collects from them locally, with low overhead and no central poller reaching into production databases over the network.
- **Every database object is explorable.** Databases, tables, indexes, replicas, replication slots, tablespaces, wait types, users and shards are instances of their charts: slice, filter, group and compare them from the dashboard, without building per-object dashboards or writing queries.
- **Query views run live at the source.** Top queries, running queries, deadlocks and query errors are fetched on request from the database's own node and delivered encrypted to the user's browser. Query text and statistics are never stored outside the customer's infrastructure, and are visible only to signed-in Space members with permission to view sensitive data.

**Capabilities**

**Relational databases**
- **[PostgreSQL](/src/go/plugin/go.d/collector/postgres/integrations/postgresql.md)**: per database, table, index, replica and replication slot — connections, transactions, locks, buffer cache, checkpoints, WAL, vacuum and autovacuum, bloat, table and index I/O and size, replication lag
- **[MySQL, MariaDB, Percona Server](/src/go/plugin/go.d/collector/mysql/integrations/mysql.md)**: per user — connections, queries, InnoDB buffer pool and I/O, locks, replication, Galera cluster
- **[Microsoft SQL Server](/src/go/plugin/go.d/collector/mssql/integrations/microsoft_sql_server.md)**: per database, wait type, lock resource, SQL Agent job, Always On availability group and replica, Windows failover cluster; transaction log; Azure SQL Managed Instance
- **[Oracle Database](/src/go/plugin/go.d/collector/oracledb/integrations/oracle_db.md)**: sessions, cursors, parse and execute rates, logical and physical I/O, buffer and library cache, enqueues; per tablespace and wait class
- **[IBM Db2](/src/go/plugin/ibm.d/modules/db2/integrations/ibm_db2.md)**: per buffer pool, tablespace, table, index, connection, memory pool and prefetcher; locks, deadlocks, sort overflows, log space, backup age and status, long-running queries
- **[IBM i (AS/400)](/src/go/plugin/ibm.d/modules/as400/integrations/ibm_i_as-400.md)**: system, jobs, storage, SQL plan cache
- **[ClickHouse](/src/go/plugin/go.d/collector/clickhouse/integrations/clickhouse.md)**: queries, inserts, merges, parts, replication lag and read-only replicas, distributed sends; per disk and per table
- **[CockroachDB](/src/go/plugin/go.d/collector/cockroachdb/integrations/cockroachdb.md)**: SQL connections, statements and errors, KV transactions and restarts, ranges and replication problems, replicas, leaseholders and rebalancing, storage capacity, RocksDB engine, node liveness, slow requests
- **[YugabyteDB](/src/go/plugin/go.d/collector/yugabytedb/integrations/yugabytedb.md)**: master and tablet server RPC services, Raft, YSQL and YCQL statements
- **Connection poolers and proxies**: [PgBouncer](/src/go/plugin/go.d/collector/pgbouncer/integrations/pgbouncer.md) (per database), [ProxySQL](/src/go/plugin/go.d/collector/proxysql/integrations/proxysql.md) (per user, hostgroup and backend), [MaxScale](/src/go/plugin/go.d/collector/maxscale/integrations/maxscale.md) (per server)

**NoSQL, caches and search**
- **[MongoDB](/src/go/plugin/go.d/collector/mongodb/integrations/mongodb.md)**: operations, connections, WiredTiger cache and transactions, locks; per database (collections, documents, indexes, sizes), replica set member state, health and lag, sharding
- **[Redis](/src/go/plugin/go.d/collector/redis/integrations/redis.md)** and Redis-compatible servers (**Valkey, Dragonfly, KeyDB, Kvrocks, Garnet**): memory, clients, operations, keyspace per database, hit rate, per-command calls and latency, persistence (RDB, AOF), replication
- **[Elasticsearch, OpenSearch](/src/go/plugin/go.d/collector/elasticsearch/integrations/elasticsearch.md)**: per node (JVM, indexing, search, GC, thread pools, circuit breakers), cluster health and shards, per index (health, shards, documents, size)
- **[Memcached](/src/go/plugin/go.d/collector/memcached/integrations/memcached.md)**, **[Cassandra](/src/go/plugin/go.d/collector/cassandra/integrations/cassandra.md)**, **[Couchbase](/src/go/plugin/go.d/collector/couchbase/integrations/couchbase.md)** (per bucket), **[CouchDB](/src/go/plugin/go.d/collector/couchdb/integrations/couchdb.md)**, **[RethinkDB](/src/go/plugin/go.d/collector/rethinkdb/integrations/rethinkdb.md)**, **[Riak KV](/src/go/plugin/go.d/collector/riakkv/integrations/riak_kv.md)**, **[Pika](/src/go/plugin/go.d/collector/pika/integrations/pika.md)**, **[Typesense](/src/go/plugin/go.d/collector/typesense/integrations/typesense.md)**

**Replication and high availability**
- PostgreSQL replicas and replication slots, MySQL/MariaDB replication and Galera cluster, SQL Server Always On availability groups and Windows failover clusters, MongoDB replica sets and shards, ClickHouse replication and read-only replicas, CockroachDB and YugabyteDB replication
- Patroni and pgBackRest through their Prometheus exporters

**Cloud-managed databases**
- **AWS** ([CloudWatch](/src/go/plugin/go.d/collector/cloudwatch/integrations/amazon_cloudwatch.md)): RDS, DynamoDB (including per-operation metrics), DocumentDB, ElastiCache (Redis, Valkey, Memcached), Redshift, OpenSearch Service
- **Azure** ([Azure Monitor](/src/go/plugin/go.d/collector/azure_monitor/integrations/azure_monitor.md)): SQL Database, SQL Elastic Pool, SQL Managed Instance, Database for PostgreSQL Flexible Server, Database for MySQL Flexible Server, Cosmos DB, Cache for Redis, Data Explorer, Synapse Analytics

**Any database with SQL**
- Generic [`sql` collector](/src/go/plugin/go.d/collector/sql/integrations/sql_databases_generic.md): turns any SQL query into metrics and live tables — MySQL, PostgreSQL, Oracle, SQL Server and Azure SQL drivers (including Azure AD authentication)

**More engines through exporters and OpenTelemetry**
- Prometheus exporters: ScyllaDB, SAP HANA, InfluxDB, Vertica, Pgpool-II, Meilisearch and others
- [OpenTelemetry Collector database receivers](/docs/opentelemetry/metrics-collection.md), through Netdata's OTLP ingestion

**Live query views** (Netdata Functions)
- **Top queries** on 12 engines — PostgreSQL, MySQL/MariaDB/Percona, SQL Server, Oracle, MongoDB, ClickHouse, CockroachDB, YugabyteDB, Couchbase, Elasticsearch/OpenSearch, ProxySQL, Redis (and Redis-compatible servers)
  - Per-query execution counts, total/mean/max time, rows, buffer and disk I/O, temp usage, WAL, JIT, errors, user, database and client, depending on the engine (PostgreSQL: 54 columns; SQL Server: 69 columns from Query Store, with plan cache fallback)
  - Normalized query text where the engine provides it (PostgreSQL, YugabyteDB, MySQL digests, ProxySQL)
  - Sort by any column, up to 5,000 rows
- **Running queries** — PostgreSQL, Oracle, CockroachDB, YugabyteDB, RethinkDB
- **Deadlocks** — MySQL/MariaDB/Percona, SQL Server
- **Query errors** — MySQL/MariaDB/Percona, SQL Server; PostgreSQL error attribution with `pg_stat_monitor`
- **Custom live tables** — any SQL query, through the generic `sql` collector
- Available to AI assistants through [Netdata's MCP server](/docs/netdata-ai/mcp/README.md)

**Alerts**
- Stock alerts for PostgreSQL, MySQL/MariaDB (including Galera), SQL Server, Db2, IBM i, ClickHouse, CockroachDB, ProxySQL, Redis, Memcached, Riak KV, Elasticsearch, and AWS/Azure managed databases

**Anomaly detection**
- Machine learning anomaly detection on every metric of every engine

**Value**

- **Single tool across engines.** 50+ databases and managed services, self-hosted, containerized or cloud-managed, are monitored with the same data model, dashboards and alerting, removing the need for per-engine monitoring tools.
- **Coverage without a setup project.** Database instances are detected and monitored automatically; no exporters, dashboards or queries are required.
- **Problems located to the object.** Per-object metrics identify the specific table, index, replica, wait type or shard behind a problem.
- **Query-level root cause.** Live top queries, running queries, deadlocks and query errors identify the statements behind current database load and failures.
- **Replication and high-availability assurance.** Replication lag, replica health, availability groups and cluster state are monitored across engines.
- **Short events captured.** Per-second collection on self-hosted databases records lock storms, connection spikes and replication stalls lasting seconds.
- **Anomalies detected without thresholds.** Machine learning flags unusual behavior on every database metric, with no alert tuning.
- **Database and infrastructure correlated.** Database metrics share a timeline with the host, disk, container and applications, which separates query, storage and application causes.
- **AI-assisted investigation.** AI assistants query live database views through Netdata's MCP server, e.g. which queries are loading a PostgreSQL instance right now.
- **Query data sovereignty.** Query text and statistics are fetched live, delivered encrypted only to authorized users, and never stored outside the customer's infrastructure.
- **Database operations without per-engine expertise.** Built-in metrics selection, alerts and query views let platform, SRE and DevOps teams operate engines they are not specialists in.

## Network Performance Monitoring

Netdata provides network performance monitoring covering network devices (SNMP), topology (SNMP L2/L3, application dependencies, vSphere, Cato Networks), traffic flows (NetFlow, IPFIX, sFlow), SNMP traps, BGP, network licenses and network device syslog, correlated with the servers, containers and applications Netdata monitors. Any SNMP device is monitored with zero configuration: devices are identified by vendor from the complete IANA registry (65,000+ vendors), 1,700+ device models from 100+ vendors have dedicated profiles, and all other devices are monitored through standard MIBs. SNMP traps are decoded against 150,000+ trap definitions from 800+ vendors and 6,000+ MIBs. Device profiles are open, declarative and documented, and can be extended without code.

**Learn more:** [Network performance monitoring documentation](/docs/npm/README.md) · [Network monitoring on netdata.cloud](https://www.netdata.cloud/solutions/use-cases/network-monitoring/)

**Design**

- **[Distributed collection that scales](/docs/npm/device-metrics/sizing-and-scaling.md).** Netdata Agents collect network data where it is produced: one Agent per site polls devices, receives flows, traps and syslog; every Agent on a server maps that server's connections. There is no central poller to outgrow: adding Agents adds capacity, so polling stays on schedule at any network size and as frequently as needed, down to every second where devices support it and it is useful.
- **Read-only.** Netdata only reads from network devices (SNMP GET/WALK); it never changes device configuration.
- **Every device is a node.** Each SNMP device, Cato site or vSphere object appears as its own (virtual) node, with its own dashboards, alerts and anomaly detection — the same as a server.
- **[One topology model](/docs/npm/topology/README.md).** All topology sources (SNMP L2/L3, application dependencies, Netdata streaming, vSphere, Cato Networks) are published through the same topology schema and explored in the same topology view.
- **Standards first.** SNMP v1/v2c/v3, NetFlow/IPFIX/sFlow, BMP, OTLP. Syslog from network devices enters through Netdata's OpenTelemetry logs pipeline (see Logs Management); traps can be forwarded over OTLP to any OpenTelemetry-compatible destination (e.g., SIEM).
- **Network Monitor.** Netdata Cloud brings devices, topology, flows and trap events together on one page.

**Capabilities**

**[Network device monitoring (SNMP)](/docs/npm/device-metrics/README.md)** — go.d `snmp` collector
- SNMP v1, v2c, v3 (authentication up to SHA-512, privacy up to AES-256; context names)
- Read-only collection (SNMP GET/WALK only)
- Automatic device recognition by sysObjectID/sysDescr
- Collection interval configurable per device, as frequently as needed, down to every second
- Vendor identification for any SNMP device (full IANA enterprise registry, 65,000+ vendors)
- Dedicated profiles: 1,700+ device models, 100+ vendors — routers, switches, firewalls, load balancers, wireless controllers and access points, SD-WAN, UPS, PDUs, environmental sensors, storage, servers (BMC), printers, HSMs and more
- [Generic profile](/src/go/plugin/go.d/collector/snmp/npm-catalog/integrations/generic_snmp_device.md) for any SNMP device: standard MIBs (IF-MIB, IP-MIB, TCP/UDP-MIB, HOST-RESOURCES-MIB, UCD-MIB, ENTITY-SENSOR-MIB, UPS-MIB, LAG-MIB, LLDP-MIB, OSPF-MIB, BGP4-MIB and more)
- Interfaces (traffic, errors, discards, status, speed), CPU, memory, environmental sensors, vendor hardware and protocol metrics (e.g., HA, VPN tunnels, SD-WAN links, PoE, vPC, HSRP, BFD, IPS)
- [ICMP reachability](/docs/npm/device-metrics/configuration.md), latency and packet loss alongside SNMP (on by default); ping-only monitoring for devices without SNMP
- Functions: `snmp:interfaces`, `snmp:bgp-peers`, `snmp:licenses`
- [Custom profiles](/src/go/plugin/go.d/collector/snmp/profile-format.md): declarative YAML, documented format; extend or override stock profiles
- Vendor alert packs: FortiGate (HA, SD-WAN, IPS, link monitor, wireless), Cisco Nexus (vPC, HSRP, BFD, NTP)
- [SNMP diagnostics](/docs/npm/device-metrics/collect-snmp-troubleshooting-data.md) included in support bundles

**[Network discovery](/src/go/plugin/go.d/discovery/sdext/discoverer/snmpsd/integrations/snmp.md)** — SNMP service discovery (`snmp` discoverer), for dynamic networks, labs and staging; run continuously, on a schedule, or not at all
- Scans configured subnets in parallel, with SNMP v1/v2c/v3 credentials per subnet
- Identifies each responding device (system name, description, location, contact, vendor, model, category)
- Creates a monitoring job for every discovered device from customizable rules (e.g., per vendor, category or subnet); devices become nodes automatically
- Rescans continuously: new devices are picked up, known devices cached between probes
- Configured from the Netdata UI or `go.d/sd/snmp.conf`

**Network topology**
- **[SNMP L2/L3 topology](/docs/npm/topology/discovery-methods.md)** (`topology:snmp`), built automatically from SNMP device data, including neighbor devices and endpoints (unmanaged ones too) learned from LLDP/CDP, MAC and ARP tables
  - L2: LLDP (including LLDP-V2), CDP, FDB/MAC tables (Q-BRIDGE, per-VLAN), ARP/IP neighbors, STP, Cisco VTP
  - L3: BGP peerings, OSPF adjacencies, connected subnets
  - Map types: managed fabric, LLDP/CDP/managed devices, high-confidence inferred, all devices
  - Link inference strategies for unmanaged segments (FDB, STP, CDP+FDB, STP+FDB)
  - Per-link evidence and confidence; MAC vendor (OUI) lookup; reverse DNS
  - Focus on a device with configurable depth; trace paths, find affected devices, locate endpoints
- **[Application dependency mapping](/docs/npm/topology/dependency-mapping.md)** (`topology:network-connections`) — see below
- **[Netdata streaming topology](/src/go/plugin/go.d/collector/snmp/npm-catalog/integrations/netdata_streaming_topology.md)** (`topology:streaming`): Netdata Agents and Parents and how data flows between them
- **[VMware vSphere topology](/src/go/plugin/go.d/collector/snmp/npm-catalog/integrations/vsphere_topology.md)** (`topology:vsphere`): datacenters, clusters, hosts, VMs, datastores, datastore clusters, resource pools, networks
- **[Cato Networks topology](/src/go/plugin/go.d/collector/snmp/npm-catalog/integrations/cato_networks_topology.md)** (`topology:cato_networks`): sites, devices, PoPs, BGP peers

**Application dependency mapping ([live network connections](/src/collectors/network-viewer.plugin/integrations/network_connections.md))** — `network-viewer.plugin`
- Maps every TCP and UDP connection (IPv4, IPv6, all states) of every system Netdata runs on; zero configuration, enabled by default
- No instrumentation, no sidecars, no eBPF required: reads the operating system's live socket tables (Linux netlink/procfs, Windows IP Helper, macOS libproc, FreeBSD)
- Attributes each connection to its process, command line, user, container and image, Kubernetes pod, namespace and workload, systemd unit
- Classifies direction (inbound, outbound, listening, local) and address space (loopback, private, public, multicast)
- Per-connection TCP round-trip time, receive RTT and retransmissions
- Cross-node dependency map: connections between monitored systems are resolved to process-to-process links (e.g., `web-01 nginx → db-02 postgres`)
- Group by process, container or PID; aggregated or detailed views
- Linux, Windows, macOS, FreeBSD
- Related Functions: `network-connections` (table), `network-protocols`, `dns-queries` (Linux, eBPF)

**[Network flows](/docs/npm/network-flows/README.md)** — `netflow-plugin`
- NetFlow v5, v7, v9 (including Cisco ASA NSEL), IPFIX, sFlow v5 — auto-detected; UDP 2055 and 6343 by default
- Correct aggregation of [mixed sampling rates](/docs/npm/network-flows/validation.md) (per-record rate multiplication; raw counters preserved)
- [Enrichment](/docs/npm/network-flows/enrichment.md):
  - GeoIP and ASN (country, state, city, coordinates, AS number and name); [IP intelligence downloader](/docs/npm/network-flows/intel-downloader.md) for DB-IP, MaxMind GeoLite2, IPtoASN, CAIDA RouteViews, IP2Location and others
  - Cloud provider IP ranges: AWS, GCP, Azure
  - IPAM/CMDB: [NetBox](/src/crates/netflow-plugin/integrations/netbox.md), generic JSON-over-HTTP sources (Infoblox, BlueCat, phpIPAM, CMDBs)
  - Live BGP routing data: [BMP](/src/crates/netflow-plugin/integrations/bmp_bgp_monitoring_protocol.md) and bio-rd RIS (AS path, communities, next hop)
  - [Static exporter, interface and network metadata](/src/crates/netflow-plugin/integrations/static_metadata.md) (names, roles, sites, regions, tenants); classifiers
  - SRv6 and VXLAN [decapsulation](/src/crates/netflow-plugin/integrations/decapsulation.md)
- [Four storage tiers](/docs/npm/network-flows/retention-querying.md) (raw, 1-minute, 5-minute, 1-hour) with independent retention
- [Network Flows dashboard](/docs/npm/network-flows/visualization/overview.md) (Function `flows:netflow`): Sankey, table, time-series, country/state/city maps, 3D globe; facets on any field (including CIDR), regex search, group by up to 10 fields, top-N talkers by bytes or packets, shareable views
- [50,000–100,000 flow records/s per Agent](/docs/npm/network-flows/sizing-capacity.md)

**[SNMP traps](/docs/npm/snmp-traps/README.md)** — go.d `snmp_traps`
- Native trap receiver (no `snmptrapd`): SNMP v1, v2c, v3 traps and INFORMs; full SNMPv3 USM
- [150,000+ trap definitions](/docs/npm/snmp-traps/trap-profiles.md), 800+ vendors, 6,000+ MIBs
- Every trap [classified by category](/docs/npm/snmp-traps/usage-and-output.md) (state change, config change, security, auth, license, mobility, diagnostic) and severity
- [Enrichment](/docs/npm/snmp-traps/enrichment.md): source device, Netdata node, interface names and LLDP/CDP neighbors (from topology), reverse DNS
- [Storm control](/docs/npm/snmp-traps/configuration.md): per-source rate limiting and deduplication
- Stored as structured logs, [explored in the logs explorer](/docs/npm/snmp-traps/journal-and-querying.md) (Function `snmp:traps`); [OTLP export](/docs/npm/snmp-traps/forwarding-to-siem.md) to any OpenTelemetry destination (e.g., SIEM)
- [20 stock alerts](/docs/npm/snmp-traps/alerts.md) on trap severity, storms and receiver health
- [~50,000 traps/s per Agent](/docs/npm/snmp-traps/sizing-and-capacity.md)

**[BGP monitoring](/docs/npm/bgp/README.md)**
- Any router exposing BGP4-MIB, plus vendor-specific BGP MIBs: Alcatel-Lucent, Arista, Cisco, Dell, F5, Huawei (including BGP VPN), Juniper, NEC, Nokia, NVIDIA
- Per peer and per address family: session state, uptime, flaps, prefixes (received, accepted, active, advertised, rejected, suppressed, withdrawn), update recency (detects established-but-stale sessions), last error and down reason, graceful restart
- [Function `snmp:bgp-peers`](/docs/npm/bgp/metrics.md); BGP peerings as topology links
- Alerts: peer down, address family down, and machine-learning anomaly alerts on transitions, updates and accepted prefixes
- Also from Palo Alto PAN-OS and Cato Networks; BIRD and FRRouting through their Prometheus exporters

**[Network license monitoring](/docs/npm/licensing/README.md)**
- Blue Coat ProxySG, Check Point, Cisco (Smart and traditional licensing), Fortinet FortiGate, MikroTik RouterOS, Sophos XGS; Palo Alto PAN-OS
- Time to expiry (license, authorization, certificate, grace period), pool usage, license state
- [Function `snmp:licenses`](/docs/npm/licensing/metrics.md); alerts for expiring licenses, authorizations, certificates, grace periods, degraded state, high usage

**Vendor API collectors**
- **[Palo Alto PAN-OS](/src/go/plugin/go.d/collector/panos/integrations/palo_alto_networks_pan-os.md)** (XML API): BGP, system, HA, environment sensors, licenses, IPsec; alerts
- **[Cato Networks](/src/go/plugin/go.d/collector/cato_networks/integrations/cato_networks.md)** (SASE, GraphQL API): sites, devices, interfaces, traffic and quality, BGP; each site a node; topology; alerts

**[Syslog from network devices](/docs/npm/syslog/README.md)**
- Received through Netdata's OpenTelemetry logs pipeline (syslog receiver, RFC 3164/5424, UDP/TCP), stored and explored in Logs Management; Netdata provides ready collection configurations

**Active network checks**
- `ping` (latency, loss, jitter), `portcheck` (TCP/UDP), `dnsquery`, `httpcheck`, `x509check` (certificate expiry), `whoisquery` (domain expiry), each with stock alerts

**Host networking** (every Netdata Agent)
- Interfaces, IP/TCP/UDP/ICMP/SCTP protocol statistics, sockets, softnet, conntrack, SYNPROXY, IPVS, QoS (tc), netfilter accounting, wireless, InfiniBand, NIC/optics (ethtool), WireGuard, OpenVPN; eBPF socket and DNS monitoring
- Stock alerts for interface drops, errors, saturation, packet storms, conntrack exhaustion, TCP/UDP errors

**Value**

- **Zero-configuration device onboarding.** Device recognition, vendor identification and profile selection are automatic; devices report without OID lists, templates or per-device setup, and network discovery adds new devices automatically in dynamic networks.
- **Coverage with autonomy.** 1,700+ device models have dedicated profiles and every other SNMP device is monitored through standard MIBs; customers can extend profiles themselves, and Netdata adds missing device profiles for customers as required.
- **Collection that scales with the network.** Collection is distributed across Netdata Agents with no central poller; polling stays on schedule at any network size, as frequently as needed, down to every second, and each Agent processes 50,000–100,000 flow records per second.
- **Safe for production devices.** Collection is read-only; Netdata never changes device configuration.
- **Network and applications correlated.** Device metrics, topology, flows, traps and syslog share dashboards and timelines with the servers, containers and applications running over the network; application dependency mapping shows process-to-process communication across systems without instrumentation.
- **Always-current topology.** L2/L3 topology is rebuilt continuously from what devices report (LLDP, CDP, FDB, ARP, STP, BGP, OSPF), removing manual network maps.
- **Traffic attributed to business context.** Flows are enriched with geography, ASNs, cloud provider ranges, IPAM data and live BGP routing, attributing traffic to sites, tenants, providers and networks.
- **Actionable fault events.** Traps are decoded, classified, deduplicated, rate-limited and enriched with device and topology context, turning trap storms into searchable, prioritized events.
- **Routing and license risks detected early.** BGP sessions and prefixes are monitored with machine learning anomaly detection; license, certificate and grace-period expiry are alerted before they cause outages.
- **Anomaly detection on every device metric.** Every network device gets the same dashboards, machine learning anomaly detection and alerting as every other Netdata node.
- **Standards-based collection.** SNMP, NetFlow/IPFIX/sFlow, BMP and OpenTelemetry; no proprietary probes or appliances.

## Logs

Netdata provides logs at two levels: **Logs Monitoring** queries logs where operating systems already store them (systemd-journal, Windows Event Log, macOS unified logs) on every monitored node, and **Logs Management** collects, stores, indexes and queries logs from any source through Netdata's OpenTelemetry logs pipeline. Both use the same logs explorer, alongside metrics, alerts and traces.

**Learn more:** [Logs documentation](/docs/category-overview-pages/working-with-logs.md) · [Logs on netdata.cloud](https://www.netdata.cloud/features/dataplatform/logs-management/)

| | **Logs Monitoring** | **Logs Management** |
|---|---|---|
| What it does | Queries logs where the operating system already stores them | Collects, stores, indexes and queries logs from any source |
| Sources | systemd-journal (Linux), Windows Event Log (Windows), macOS unified logs (macOS) | Any source, through Netdata's OpenTelemetry logs pipeline |
| Storage | None added — logs stay in the OS log store | Netdata log store (field-level indexing, ~1/4 of raw size) |

### Logs Monitoring (query in place)

**Learn more:** [Zero-pipeline logs on netdata.cloud](https://www.netdata.cloud/features/dataplatform/zero-pipeline-logs/)

**Design.** Netdata queries logs directly in the operating system's own log store, on the node where they are generated. No log shipping, no extra storage, no pipeline to operate. Every monitored node is a log explorer for its own logs; Netdata Parents and Netdata Cloud route queries to the right node.

**Capabilities**

- **[systemd-journal](/src/collectors/systemd-journal.plugin/README.md)** (Linux; Function `systemd-journal`)
  - System, user and namespace journals; remote journals received by `systemd-journal-remote` are detected automatically and presented per host (`remote-<host>`)
  - [Journal centralization guides](/docs/observability-centralization-points/logs-centralization-points-with-systemd-journald/README.md): passive and active, with or without TLS (self-signed certificate helper), forward-secure sealing
  - Two readers: libsystemd (native packages) and Netdata's own Rust journal reader (static and Docker builds; no libsystemd required)
  - Monitored journal directories configurable at runtime (DynCfg)
  - Sampling for very large journals: fully evaluates the first 500k rows of a query, then samples up to 1M, with estimated counts for the rest
  - Human-readable field values: priority, facility, errno, message IDs (~130 known), boot IDs (as boot time), UIDs/GIDs (as user and group names), capabilities
- **[Windows Event Log](/src/collectors/windows-events.plugin/README.md)** (Windows; Function `windows-events`)
  - All local channels (Admin, Operational, Analytic, Debug), including events forwarded by Windows Event Forwarding (WEF/WEC); ETW and TraceLogging events routed to the Event Log
  - Source groups by channel type, provider, retention mode, enabled/disabled state
  - Facets: Computer, Provider, Level, Keywords, Opcode, Task, UserAccount, UserDomain, UserSID
- **[macOS unified logs](/src/collectors/macos-logs.plugin/README.md)** (Function `macos-logs`)
  - Reads the system log store through Apple's OSLog framework
  - Facets: level, process, sender, subsystem, category, entry type, signposts
- **[Logs explorer](/docs/dashboards-and-charts/logs-tab.md) (common to all three)**
  - Faceted filtering on any field, with value counts per facet
  - Histogram of log volume over time, by any field
  - Full-text search (substring, `|` for OR, `*` wildcard, `!` negation)
  - Live tail (PLAY mode)
  - Time-range navigation and paging
  - Severity colouring
  - Filters pushed down to the log store where possible (journal slicing, Windows XPath, macOS predicates)
- **[log2journal](/src/collectors/log2journal/README.md)**: converts any text log (JSON, logfmt, or any format matched by a PCRE2 pattern) into structured journal entries, with field extraction, renaming, injection, rewriting and filtering. Ships with configurations for nginx (combined and JSON). Pairs with [`systemd-cat-native`](/src/libnetdata/log/systemd-cat-native.md), which writes to local journald or sends to a remote `systemd-journal-remote` over HTTP/HTTPS.
- **[Netdata's own logs](/src/libnetdata/log/README.md)**: Netdata logs to systemd-journal (Linux) or ETW/Windows Event Log (Windows) with structured fields and message IDs; alert transitions are logged and explorable in the logs explorer.
- **Other log record sources** in the same explorer:
  - [SNMP traps](/docs/logs/snmp-trap-logs.md) (Function `snmp:traps`): traps stored as structured journal files (see Network Performance Monitoring)
  - [Redfish BMC event logs](/src/go/plugin/go.d/collector/redfish/integrations/redfish.md) (Function `redfish:logs`): read live from the BMC

**[Access control](/docs/netdata-oss-limitations.md).** Log queries require a signed-in Netdata Cloud user who is a member of the node's Space and whose role grants permission to view sensitive data; log data is flagged as sensitive. Log data is not stored in Netdata Cloud; it travels encrypted from the node to the user's browser.

### Logs Management (collect and store)

**Learn more:** [Centralizing logs with OpenTelemetry](/docs/logs/centralizing-logs-with-opentelemetry.md)

**Design.** Netdata Logs Management is an OpenTelemetry logs pipeline:

- **Collection stage — OpenTelemetry Collector:** collection from all sources (syslog, Kubernetes, files, applications, systemd-journal, Windows Event Log, macOS unified logs, cloud services), parsing, filtering, enrichment, logs-to-metrics conversion, and forwarding to additional destinations (e.g., SIEM).
- **Backend stage — Netdata:** OTLP ingestion, structured storage with field-level indexing, retention and offload, query and exploration, machine learning anomaly detection and alerting on metrics (including log-derived metrics), and correlation with metrics and traces.

Syslog ingestion, Kubernetes log collection, log-pattern alerting (via logs-to-metrics), and SIEM forwarding are functions of the pipeline's collection stage. Netdata documentation provides [Collector configurations](/docs/opentelemetry/logs-collection.md) for syslog, journald, files, Windows Event Log, Kubernetes pods and macOS unified logs.

**Capabilities**

- **[Ingestion](/docs/opentelemetry/otlp-ingestion.md)** (`otel-plugin`)
  - OTLP over gRPC (default `127.0.0.1:4317`) and over HTTP (`127.0.0.1:4318`)
  - TLS and mutual TLS; gzip compression
  - Durable acknowledgement: records are written to the write-ahead log before the sender is acknowledged; rejected records are reported through OTLP partial success
  - Accepts records up to 24 hours in the past and 10 minutes in the future
  - Logs, traces and metrics through the same endpoint
- **[Storage](/docs/logs/log-storage-and-retention.md)**
  - Netdata log store: write-ahead log, sealed into indexed files
  - Field-level indexing: every field of every record is indexed; indexing strategy adapts per field to its number of distinct values
  - zstd compression
  - Records are queryable immediately, before files are sealed
  - Streams partitioned by `service.namespace` / `service.name`
  - Multi-tenancy: tenant selected by the `X-Scope-OrgID` header (off by default)
- **Retention and offload**
  - Retention by total size, age and file count; per-tenant overrides
  - Offload to S3-compatible object storage (AWS S3, MinIO and others) or a filesystem target; offloaded files are fetched back on demand when queried, through a local read cache
- **Query** (Functions `otel-logs`, `legacy-otel-logs`)
  - Same logs explorer as Logs Monitoring: facets, histograms, filtering on any field, regex search, paging
  - Service selector based on OpenTelemetry service identity
  - Offline command-line query: `otel-plugin logs` (NDJSON output)
- **[Log-derived metrics](/docs/opentelemetry/logs-to-metrics.md)**: metrics produced from logs by the collection stage (e.g., counts per service, severity or status) are ingested as Netdata metrics, with per-second dashboards, machine learning anomaly detection on every series, and alerting

**Storage efficiency (measured).** Tested on several days of Netdata Cloud production logs, all products at default settings. Sizes relative to the raw uncompressed logs:

| | Indexing | Structured | Storage size |
|---|---|---|---|
| **Netdata** | Field-level | Yes | **~1/4** |
| Loki | Labels only | No | ~1/4 |
| systemd-journal | Field-level | Yes | ~1× |
| Elasticsearch | Full-text | Yes | ≥2.5× |

Elasticsearch storage grows with indexing configuration; Loki storage grows with the number of streams. Netdata storage size does not depend on either.

**Value**

- **Log visibility across the infrastructure without shipping logs.** Logs Monitoring queries systemd-journal, Windows Event Log and macOS unified logs in place, providing log exploration on every monitored node without log shipping, pipelines or extra storage.
- **Centralize only what needs centralizing.** Logs Management is used for logs that must be centralized or retained, or that have no local log store (applications, containers, network devices, cloud services).
- **Indexed search at low storage cost.** Every field is indexed and searchable at about 1/4 of raw log size; storage does not grow with stream count or indexing choices.
- **No logging cluster to operate.** Logs are stored and queried by Netdata Agents; there is no separate log cluster to deploy, size, scale or upgrade.
- **OpenTelemetry investment preserved.** Existing OpenTelemetry SDKs, Collector configurations and team skills work unchanged; there is no proprietary agent, protocol or instrumentation lock-in, and the same pipeline can also feed other destinations (e.g., SIEM).
- **Any source through one pipeline.** The OpenTelemetry Collector's receivers and processors bring syslog, Kubernetes, files, applications and cloud services into Netdata, with parsing, enrichment and logs-to-metrics conversion.
- **Logs correlated with metrics, alerts and traces.** Logs are explored on the same timeline as per-second metrics, anomaly detection, alerts and traces; log-derived metrics get machine learning anomaly detection and alerting.
- **Investigation without a query language.** Facets with value counts, histograms and full-text search make logs explorable by every team member.
- **Log data sovereignty.** Logs are stored on Netdata Agents, optionally offloaded to the customer's own object storage, and never stored in Netdata Cloud.

## OpenTelemetry

Netdata is an OpenTelemetry-native observability pipeline for metrics, logs and traces. The OpenTelemetry Collector forms the pipeline's collection stage; Netdata is the backend that ingests OTLP, stores, indexes and queries the data, and applies per-second dashboards, machine learning anomaly detection, alerting and correlation across all three signals.

**Learn more:** [OpenTelemetry documentation](/docs/category-overview-pages/opentelemetry.md) · [OpenTelemetry on netdata.cloud](https://www.netdata.cloud/opentelemetry/)

**Design**

- **Collection stage — OpenTelemetry Collector.** Receivers collect from any source; processors parse, filter and enrich; connectors derive signals (logs → metrics with `count`, traces → RED metrics with `spanmetrics`, service-to-service metrics with `servicegraph`); the `transform` processor computes percentiles from histograms (`extract_percentile_metric`); exporters route copies to additional destinations (e.g., SIEM).
- **Backend stage — Netdata ([`otel-plugin`](/src/crates/otel-plugin/README.md)).** One OTLP endpoint (gRPC and HTTP) for metrics, logs and traces, with TLS/mTLS, gzip and durable acknowledgement (data is written to disk before the sender is acknowledged).
- **Data on the customer's infrastructure.** Logs and traces are stored on Netdata Agents (optionally offloaded to the customer's own S3-compatible or filesystem storage), never in Netdata Cloud.

**Capabilities**

**Metrics**
- [Every OTLP metric becomes a Netdata chart](/docs/opentelemetry/metrics-collection.md) (`otel.<metric>`); resource, scope and data-point attributes become labels
- Gauges; sums (monotonic counters as rates); explicit-bucket histograms (bucket heatmap, count, sum, min/max); summaries (quantiles)
- Chart mappings (`otel.d/v1/metrics`) structure OTLP metrics into Netdata charts; stock mapping for the Collector's `hostmetrics` receiver; user-defined mappings
- Machine learning anomaly detection, alerting, dashboards, streaming to Netdata Parents and MCP access, as for native metrics
- Derived metrics through the collection stage:
  - [Logs → metrics](/docs/opentelemetry/logs-to-metrics.md) (`count` connector)
  - Traces → RED metrics per service and operation — rate, errors, duration including p50/p95/p99 (`spanmetrics` + `transform`)
  - Service-to-service request, failure and latency metrics (`servicegraph`)

**Logs — [Logs Management](/docs/logs/centralizing-logs-with-opentelemetry.md)**

Netdata Logs Management is Netdata's OpenTelemetry logs pipeline: it collects, stores, indexes and queries logs from any source. It is distinct from Logs Monitoring, which queries logs in place in the operating systems' own log stores (systemd-journal, Windows Event Log, macOS unified logs) (see Logs).

- **[Collection stage](/docs/opentelemetry/logs-collection.md):** any source through the OpenTelemetry Collector — syslog, Kubernetes, files, applications, journald, Windows Event Log, macOS unified logs, cloud services — with parsing, filtering, enrichment, logs-to-metrics conversion, and forwarding to additional destinations (e.g., SIEM)
- **Ingestion:** OTLP/gRPC and OTLP/HTTP; durable acknowledgement (records are written to the write-ahead log before the sender is acknowledged); rejected records reported through OTLP partial success; records accepted up to 24 hours in the past and 10 minutes in the future
- **Storage:** write-ahead log sealed into indexed files; field-level indexing of every field of every record; zstd compression; about 1/4 of raw log size; records queryable immediately, before files are sealed
- **Streams and tenancy:** streams partitioned by `service.namespace` / `service.name`; tenant selection with the `X-Scope-OrgID` header; per-tenant retention
- **Retention and offload:** by total size, age and file count; offload to S3-compatible object storage or a filesystem target, with on-demand read-back through a local cache
- **Query:** logs explorer (Functions `otel-logs`, `legacy-otel-logs`) with facets and value counts, histograms, filtering on any field, regex search, service selector; offline command-line query (`otel-plugin logs`)
- **Log-derived metrics:** metrics produced by the collection stage (e.g., counts per service, severity or status) become Netdata charts with machine learning anomaly detection and alerting
- **Correlation:** logs share the timeline with metrics and traces; log records carry `trace_id` / `span_id` from OpenTelemetry instrumentation

**[Traces](/docs/opentelemetry/trace-storage-and-retention.md)**
- Span storage with field-level indexing, trace-ID index and bloom filter, span events and links; retention and offload as for logs
- **Traces explorer** (Netdata Cloud; users turn it on in the Early Access panel):
  - Trace list: heatmap (trace count × duration band, with errors), volume, error and percentile charts over time, trace duration scatter, slowest traces
  - Trace detail: waterfall, span tree, span details, missing-span markers
  - Filters: service, operation, status, duration range, any resource, span, scope, event or link attribute
- **RED analysis at query time:** the explorer computes rate, errors and duration (p50/p95/p99) directly from the stored spans of every matching trace in the selected time range, for any combination of attribute values and duration range. No metrics need to be pre-defined: any slice of the traffic can be analyzed after the fact, without cardinality limits or pre-declared dimensions.
- **RED metrics at collection time:** the collection stage (`spanmetrics`, with `transform` for percentiles) produces persistent RED metrics per service and operation, which Netdata stores long-term with per-second charts, machine learning anomaly detection and alerting.
- Function `otel-traces`, also available to AI assistants through Netdata's MCP server

**Collector configurations in Netdata documentation**
- Metrics: host metrics, Prometheus, Redis, nginx, PostgreSQL
- Logs: journald, files, Windows Event Log, Kubernetes (Helm, `k8sattributes`), macOS unified logs, syslog
- Logs to metrics (`count` connector); transformations; OTLP ingestion; securing the endpoint; trace storage and retention

**[Security and tenancy](/docs/opentelemetry/securing-the-otlp-endpoint.md)**
- TLS and mutual TLS on the OTLP endpoint
- Tenant selection with the `X-Scope-OrgID` header; per-tenant retention
- Logs and traces queryable only by signed-in Space members with permission to view sensitive data

**Netdata as an OpenTelemetry source**
- [SNMP traps exported as OTLP log records](/docs/npm/snmp-traps/forwarding-to-siem.md)

**Value**

- **Standards-based pipeline.** Metrics, logs and traces enter through OTLP; existing OpenTelemetry SDKs, Collector configurations and team skills work unchanged, with no proprietary agent, protocol or instrumentation lock-in.
- **One backend for three signals.** Metrics, logs and traces are stored, explored and correlated in one place, on one timeline, with the infrastructure Netdata monitors.
- **Investigation and monitoring from the same traces.** Query-time RED analysis answers questions about any slice of traffic after the fact; collection-time RED metrics provide long-term trends, machine learning anomaly detection and alerting.
- **OpenTelemetry metrics treated as first-class.** OTLP metrics get the same per-second dashboards, anomaly detection and alerting as Netdata's native metrics.
- **Flexibility of the Collector.** The Collector's receivers, processors and connectors extend what reaches Netdata — new sources, derived metrics, percentiles, routing to other destinations — without changes to Netdata.
- **Data sovereignty.** Logs and traces stay on the customer's infrastructure, never stored in Netdata Cloud.


## Digital Experience and Application Observability

Netdata monitors applications from the user's side as well as the infrastructure's: Real User Monitoring (RUM) measures what real visitors experience in their browsers, synthetic journeys and Lighthouse audits test critical user flows and page performance from real browsers on a schedule, and OpenTelemetry traces connect browser requests to backend services. Digital experience data shares dashboards, alerting and machine learning with the infrastructure behind it.

**Design**

- **Collected by Netdata Agents.** RUM beacons are received, and synthetic browser checks run, by the `dem.plugin` on Netdata Agents and Parents; the customer chooses where data is received and where checks run from.
- **Privacy at ingestion.** URLs are stripped of query strings and fragments, identifiers (emails, UUIDs, IDs, tokens) are redacted from paths, and client IP addresses are used only for rate limiting and geolocation, never stored.
- **Real browsers, real scripts.** Synthetic journeys are Playwright Test scripts executed in sandboxed Chromium, with a fresh browser per run; Lighthouse audits run in the same managed browser environment.
- **Connected to the backend.** RUM can propagate W3C trace context and export browser logs and spans to Netdata's OpenTelemetry pipeline, linking user interactions to backend traces.

**Capabilities**

**Real User Monitoring**
- One script tag per site (a bootstrap served by the Agent loads the open-source browser SDK from a public CDN); beacons to the Agent's RUM endpoint over HTTPS, with origin allow-lists and rate limits
- Core Web Vitals — LCP, INP, CLS — plus FCP and TTFB, as p50/p75/p95 with good / needs-improvement / poor distributions
- Page load time, DOMContentLoaded, first-party API duration, resource timing by host
- Page views, active sessions, JavaScript errors grouped by fingerprint with sample stack traces, rage clicks, dead clicks, error clicks
- Breakdowns by page group, browser, device, country and application version
- Sampling with an investigation rate that always keeps sessions with errors or poor vitals; sampled session history retained on the Agent
- Functions: `rum-sites`, `rum-pages`, `rum-live`, `rum-sessions`, `rum-errors`, `rum-session-events`
- Stock alerts: LCP, INP and CLS p75 against Core Web Vitals thresholds; telemetry export drops
- Optional export of browser logs and spans to the OpenTelemetry pipeline; W3C `traceparent` propagation to backend services
- Digital Experience views in Netdata Cloud: Live, Overview, Pages, Web Vitals, Audience, Errors, Sessions, Events, Sites, Alerts

**Synthetic journeys**
- Playwright Test scripts (JavaScript or TypeScript) with assertions and named steps, executed in sandboxed Chromium on a schedule
- Secrets passed to scripts as environment variables and redacted from diagnostics
- Failure screenshots; run history and artifacts retained on the Agent
- Netdata Agents act as probe locations
- Functions: `synthetics-checks`, `synthetics-runs`, `synthetics-run`, `synthetics-artifact`; stock alert on journey failure
- Netdata Cloud views: Journeys, Overview, Runs, Tests, Alerts

**Lighthouse performance audits**
- Scheduled desktop Lighthouse performance audits: performance score (0–100), FCP, LCP, Total Blocking Time, Speed Index and CLS (lab data); HTML reports

**Endpoint checks** (see Infrastructure Monitoring)
- HTTP endpoints, TCP/UDP ports, ICMP, DNS, TLS certificate and domain expiry, files, S3-compatible storage; Nagios-compatible plugins

**Application traces** (see OpenTelemetry)
- OpenTelemetry traces with the Traces explorer and RED analysis at query time and collection time

**Value**

- **The user's experience, measured.** Core Web Vitals, errors and sessions from real visitors show how applications perform for users, not only for servers.
- **Critical flows tested continuously.** Synthetic journeys catch broken logins, checkouts and sign-ups before users report them, using standard Playwright scripts.
- **From the browser to the root cause.** Digital experience data sits next to backend traces and infrastructure metrics, and trace context links user interactions to the services that served them.
- **Privacy by default.** Identifiers are redacted and IP addresses are not stored, simplifying privacy reviews.
- **Probes where the users are.** Netdata Agents in data centers, cloud regions, offices and branches run synthetic journeys from those locations.
- **Standards-based.** Playwright, Lighthouse, Core Web Vitals, W3C trace context and OpenTelemetry; no proprietary scripting.

## Machine Learning and AI

Netdata applies unsupervised machine learning to every metric it collects, on the Agents and Parents that hold the data, and makes the whole infrastructure — metrics, anomalies, alerts, logs and live Functions — available to AI: Netdata AI in Netdata Cloud (conversations, troubleshooting investigations, reports, alert automation) and Netdata's MCP servers for any AI assistant.

**Learn more:** [Machine learning and AI documentation](/docs/category-overview-pages/machine-learning-and-assisted-troubleshooting.md) · [AI-powered observability on netdata.cloud](https://www.netdata.cloud/features/aiml/aiops/)

**Design**

- **[Unsupervised anomaly detection per metric, at the edge](/docs/ml-ai/ml-anomaly-detection/ml-anomaly-detection.md).** Every metric gets its own models, trained locally on its own recent behavior; no labels, thresholds or configuration are required. Models run where the data is (Agents and Parents), so detection scales with the fleet.
- **[Consensus scoring](/docs/ml-ai/ml-anomaly-detection/ml-accuracy.md).** Each metric is scored by multiple models trained on successive time windows; a sample is flagged anomalous only when all models agree, which suppresses false positives from one-off training artifacts.
- **Anomalies as data.** The anomaly flag is stored with every sample in the database, so anomaly rates can be queried, charted, correlated and alerted on for any time range, like any other metric.
- **AI on live infrastructure data.** Netdata AI and external AI assistants work on real metrics, anomalies, alerts and live Functions queried from the Agents and Parents, not on exported copies.

**Capabilities**

**Machine learning**
- [k-means clustering](/src/ml/ml-configuration.md) (k=2) per metric; 18 models per metric by default (configurable 1–168), trained on 6-hour windows and retrained every 3 hours
- Detection starts on a new metric after about 15 minutes of data (its first model); all 18 default models are in place after about 54 hours (over two days)
- Anomaly flag stored with every sample (tier 0) and anomaly counts in higher tiers
- Anomaly rate per chart, dimension, instance and node; [anomaly ribbon](/docs/dashboards-and-charts/netdata-charts.md) on every chart; node-level anomaly rate
- **[Anomaly Advisor](/docs/ml-ai/anomaly-advisor.md):** ranks the metrics that were anomalous in a selected time range across the infrastructure, to identify where a problem started and what it affected; the root cause is usually among the top 30–50 ranked metrics
- **[Metric Correlations](/docs/metric-correlations.md):** finds the metrics that changed most between a highlighted period and a baseline (KS2 and volume algorithms, on raw values or anomaly rates)
- [Anomaly-based alerting](/src/health/REFERENCE.md): alerts on anomaly rates (stock examples for ML and BGP; user-defined on any metric)
- Enabled by default on Agents and Parents with persistent storage; disabled automatically on constrained devices (`iot` profile)
- Parent clusters train once and share models; Children stream trained models to Parents, which skip retraining

**Netdata AI** (Netdata Cloud)
- **[Conversations (Ask AI)](/docs/netdata-ai/conversations.md):** chat about the infrastructure from anywhere in the UI, with up to five attached context items (charts, alerts, nodes), report mode and sharing
- **[Troubleshoot with AI / Investigations](/docs/netdata-ai/investigations/index.md):** automated troubleshooting and root-cause analysis for any question or time range; custom and scheduled investigations (daily, weekly, monthly); results in about two minutes, with email notification
- **[Insights reports](/docs/ml-ai/ai-insights.md):** Infrastructure Summary, Anomaly Analysis, Capacity Planning, Performance Optimization; scheduled reports; PDF export
- **[Alert AI](/docs/netdata-ai/alerts-automation/alerts-automation.md):** alert investigation reports, AI alert explanations, AI-generated and AI-suggested alert definitions, and alert evaluation against historical data before deployment
- **[Infrastructure Knowledge and AI Memory](/docs/netdata-ai/infrastructure-knowledge.md):** a versioned description of the infrastructure (generated from the infrastructure, from a template or written) and remembered facts that ground AI answers in the customer's environment
- **[MCP Connections](/docs/netdata-ai/mcp/mcp-connections.md):** Netdata AI connects to external MCP servers — GitHub, Atlassian (Jira, Confluence, Bitbucket), PagerDuty and custom HTTPS MCP servers — to combine monitoring data with code, tickets and incidents

**[MCP servers](/docs/netdata-ai/mcp/README.md)** (for any AI assistant)
- **Agent and Parent MCP server:** Streamable HTTP (`/mcp`), SSE (`/sse`) and WebSocket transports; `nd-mcp` bridge for stdio-only clients
- **Netdata Cloud MCP server:** fleet-wide access through Netdata Cloud (Streamable HTTP, API token with MCP scope)
- **Tools:** `list_metrics`, `get_metrics_details`, `list_nodes`, `get_nodes_details`, `list_functions`, `execute_function`, `query_metrics`, `find_correlated_metrics`, `find_anomalous_metrics`, `find_unstable_metrics`, `list_raised_alerts`, `list_running_alerts`, `list_alert_transitions`
- Live Functions through MCP: processes, containers, connections, logs, traces, database queries and more
- [Documented clients](/docs/netdata-ai/mcp/mcp-clients/ai-devops-copilot.md): Claude Desktop, Claude Code, Cursor, VS Code, JetBrains IDEs, OpenAI Codex CLI, Gemini CLI, Crush, OpenCode, and AI DevOps copilots in general
- Security: MCP API key (Bearer), access-control lists (`allow mcp from`), permissions following the user's Netdata Cloud role; sensitive Functions require the corresponding permission
- AI skills published in the Netdata repository for querying Agents and Netdata Cloud

**Value**

- **Anomaly detection on everything, without configuration.** Every metric of every system, container, application and device is monitored for unusual behavior from the moment it is collected.
- **Fewer false alarms.** Consensus across multiple models flags only behavior that is unusual by every model's judgment, eliminating about 99% of false positives.
- **Faster root-cause analysis.** Anomaly Advisor and Metric Correlations narrow thousands of metrics to the few that changed, across the whole infrastructure.
- **Investigations in minutes.** Netdata AI produces troubleshooting and root-cause reports, capacity and performance reviews, and alert explanations from live infrastructure data.
- **AI grounded in the customer's environment.** Infrastructure Knowledge, AI Memory and MCP Connections combine monitoring data with the customer's own context, code and incidents.
- **Open to any AI assistant.** MCP servers on every Agent, Parent and Netdata Cloud let existing AI tools query the infrastructure directly, under the same access controls as the UI.

## Alerting and Notifications

Netdata evaluates alerts on every Agent and Parent, close to the data, with 1,100+ stock alert definitions that activate automatically for the technologies detected, and delivers notifications from the Agent (30 methods) and from Netdata Cloud (16 integrations plus personal email) with role- and room-based routing.

**Learn more:** [Alerts documentation](/src/health/README.md) · [Alerts and notifications on netdata.cloud](https://www.netdata.cloud/features/dataplatform/alerts-notifications/)

**Design**

- **Alerts evaluated at the edge.** Each Agent or Parent evaluates the alerts for the metrics it holds, on each alert's own schedule, so alerting scales with the fleet and keeps working without Netdata Cloud.
- **Stock alerts that apply themselves.** Alert templates attach to every matching chart, so new systems, containers, applications and devices are covered as soon as they are detected.
- **Metrics, anomalies and history in one expression language.** Alerts can evaluate current values, rolling-window statistics over any period, anomaly rates from machine learning, and other alerts' values.
- **Two delivery paths.** Agents send notifications directly (no Cloud dependency); Netdata Cloud sends fleet-wide notifications with user- and room-aware routing.

**Capabilities**

**Alerts**
- 1,184 stock alert definitions across 144 configuration files, covering systems, containers, applications, databases, network devices, cloud services and Netdata itself
- [Alert expressions](/src/health/REFERENCE.md): lookups over any time window (average, min, max, sum, percentiles and more), anomaly-rate lookups, calculations, warning and critical conditions with hysteresis, delays and repeat intervals, per-alert evaluation interval
- Label-based targeting (host labels, chart labels) and templates applied to all matching instances
- Alert states CLEAR, WARNING, CRITICAL (plus UNDEFINED, UNINITIALIZED, REMOVED); alert transitions history
- **[Alerts Configuration Manager](/docs/alerts-and-notifications/creating-alerts-with-netdata-alerts-configuration-manager.md):** create and edit alerts from the Netdata UI, applied without restarts; stock-alert overrides removable without restarts
- **AI alert automation:** AI-generated and AI-suggested alert definitions, AI alert explanations, evaluation against historical data before deployment (see Machine Learning and AI)
- **Alert acknowledgement** and **[silencing rules](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/manage-alert-notification-silencing-rules.md)** (space-wide or personal; hierarchy by space, room, node, alert, host labels; scheduled windows)
- Flood protection

**[Agent-dispatched notifications](/src/health/notifications/README.md)** (30 methods)
- Alerta, Amazon SNS, custom scripts, Discord, Dynatrace, email, Fleep, Flock, Gotify, ilert, IRC, Kafka, Kavenegar, Matrix, MessageBird, Microsoft Teams, ntfy, Opsgenie, PagerDuty, Prowl, Pushbullet, Pushover, Rocket.Chat, SIGNL4, Slack, SMS (SMS Server Tools 3), SMSEagle, syslog (local and remote), Telegram, Twilio
- Role-based recipients, severity filtering, and automation scripts on alert transitions

**[Cloud-dispatched notifications](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/centralized-cloud-notifications-reference.md)** (16 integrations, plus personal email)
- [Netdata Mobile App](/integrations/cloud-notifications/integrations/netdata_mobile_app.md) (iOS, Android), group email, personal email, Discord, Slack, Microsoft Teams, Telegram, Mattermost, Rocket.Chat, PagerDuty, Opsgenie, Splunk, Splunk VictorOps, ilert, ServiceNow, Amazon SNS, webhooks
- Personal and system channels; routing by room, role and severity

**Value**

- **Coverage without writing alerts.** 1,100+ stock alerts apply automatically to every detected technology, including new nodes and containers.
- **Alerting that keeps working.** Evaluation on Agents and Parents continues during network or Cloud outages, with Agent-side delivery as an independent path.
- **Alerts on behavior, not only thresholds.** Anomaly-rate and rolling-window alerts catch unusual behavior and gradual changes that fixed thresholds miss.
- **Less noise.** Acknowledgement, hierarchical silencing rules, hysteresis and flood protection keep notifications actionable.
- **Notifications where teams work.** 30 Agent methods and 16 Cloud integrations reach chat, incident management, ticketing and mobile.

## Dashboards and Visualization

Netdata dashboards are generated automatically from the collected metrics and organized by technology, with every chart interactive through the NIDL model (nodes, instances, dimensions, labels) — no query language and no dashboard building required. Netdata Cloud adds fleet-wide views, custom dashboards, logs, traces, topology, live Functions and AI.

**Learn more:** [Dashboards and charts documentation](/docs/dashboards-and-charts/README.md) · [The Netdata UI on netdata.cloud](https://www.netdata.cloud/product/netdata-ui/)

**Design**

- **[Automatic dashboards](/docs/dashboards-and-charts/metrics-tab-and-single-node-tabs.md).** Every collected metric appears on a dashboard, organized into a table of contents by technology, the moment it is collected.
- **[Query-less analysis (NIDL)](/docs/NIDL-Framework.md).** Every chart can be grouped, filtered and aggregated by node, instance, dimension and label from dropdowns that show each item's contribution, anomaly rate and min/average/max.
- **[One time frame for everything](/docs/dashboards-and-charts/visualization-date-and-time-controls.md).** Charts, logs, traces, events and Functions share a synchronized time selection across the dashboard.

**Capabilities**

**Chart features**
- [NIDL dropdowns](/docs/dashboards-and-charts/netdata-charts.md) with volume contribution, anomaly rate and statistics per item; group-by and aggregation selection
- Anomaly ribbon and info ribbon (gaps, resets, partial data) on every chart
- Highlighting a time range for Metric Correlations and Anomaly Advisor; [expanded chart analysis](/docs/dashboards-and-charts/expanded-chart-analysis.md)
- [Chart annotations](/docs/dashboards-and-charts/chart-annotations.md) (Netdata Cloud)

**Netdata Cloud views**
- [Home](/docs/dashboards-and-charts/home-tab.md) (room overview), [Nodes](/docs/dashboards-and-charts/nodes-tab.md) and **Nodes Map** (devices grouped and colored by labels and metrics, node notes, geographic location), Kubernetes Explorer, Alerts, Anomalies (Anomaly Advisor), Events feed (alert transitions, node state changes and environment events, for attributing changes in time), Logs, Traces, Live (Functions), Top (live top-style tables from Functions — processes, containers, services, queries), Topology, Network Monitor, Digital Experience (RUM and synthetics), Insights (AI), **Uptime** (fleet-wide availability built from every state metric — endpoint checks, services, devices, synthetic journeys — with reliability figures, a severity timeline, per-check and per-node drill-down, curated views and configurable severity semantics)
- Single-node views: [alerts](/docs/dashboards-and-charts/alerts-tab.md), [anomalies](/docs/dashboards-and-charts/anomaly-advisor-tab.md), [events](/docs/dashboards-and-charts/events-feed.md), logs, top, traces
- **[Custom dashboards](/docs/dashboards-and-charts/dashboards-tab.md):** drag-and-drop charts and cards (text, state timeline, alert count, raised alerts, map, uptime), templates and presets, import and export, sharing within the Space
- **TV Mode** (token-authenticated wallboard URLs) and **Playlists**
- Mobile app for notifications

**Agent dashboards**
- The same dashboard served locally by every Agent and Parent, for single nodes or a Parent's Children, usable without Netdata Cloud (the local dashboard of a standalone Windows Agent depends on the plan; see Pricing and Licensing)

**Value**

- **Dashboards from day one.** Every metric is visualized automatically; teams investigate instead of building panels and queries.
- **Anyone can investigate.** Point-and-click grouping and filtering replace query languages, so the whole team can analyze data.
- **Fast navigation from symptom to cause.** Anomaly ribbons, Anomaly Advisor, Metric Correlations and synchronized logs, traces and Functions shorten investigations.
- **Shared situational awareness.** Custom dashboards, TV Mode and Playlists put the same views on every screen.

## Organization and Service Views

Netdata organizes infrastructure and people in Netdata Cloud: Spaces isolate organizations, Rooms group nodes and the people responsible for them, and rule-based Rooms assemble nodes automatically from their labels. Because every system and device is monitored as its own node, any combination of servers, containers, network devices, cloud services and applications can be viewed, alerted on and investigated as one service.

**Learn more:** [Organize your infrastructure](/docs/netdata-cloud/organize-your-infrastructure-invite-your-team.md) · [Team collaboration on netdata.cloud](https://www.netdata.cloud/features/enterprise/team-collaboration/)

**Design**

- **Per-node monitoring as the building block.** Every Agent, Parent Child and virtual node (network device, cloud resource, remote service) is a node with its own metrics, alerts, anomaly detection and Functions. Service-level views are composed from nodes, not configured per metric.
- **Rooms as services.** A Room groups nodes and the users who work on them; dashboards, alerts, notifications and AI features in a Room cover exactly its nodes. A node can belong to many Rooms.
- **[Rule-based Rooms](/docs/netdata-cloud/node-rule-based-room-assignment.md).** Rooms can assemble their nodes automatically from host labels (include and exclude rules), evaluated in real time as labels change — e.g., "all nodes with `service=payments`" brings the payment service's servers, databases, load balancers and network devices together, and new nodes join automatically.
- **Spaces as isolation boundaries.** Each Space has its own nodes, Rooms, users, roles, settings and billing; one login gives access to all Spaces a user belongs to.

**Capabilities**

**Spaces and Rooms**
- Spaces with full isolation of nodes, Rooms, users, permissions, settings and billing; a single session across Spaces, with a different role per Space
- Static Rooms and rule-based Rooms (host-label include/exclude rules, AND conditions, exclusion precedence, real-time evaluation)
- Room-scoped dashboards, alerts, notification routing, silencing rules and AI investigations
- Multi-tenant operation for managed service providers: one Space per customer, customer users with restricted roles

**Users and access**
- [Roles](/docs/netdata-cloud/authentication-and-authorization/role-based-access-model.md): Admin, Manager, Troubleshooter, Observer, Billing; Troubleshooters and Observers limited to their assigned Rooms
- Permission to view sensitive data (logs, query text, connections, processes) controlled per role
- [Enterprise single sign-on](/docs/netdata-cloud/authentication-and-authorization/enterprise-sso-authentication.md): Okta and OpenID Connect (e.g., Microsoft Entra ID, Google); [SCIM](/integrations/cloud-authentication/integrations/scim.md) user provisioning
- User invitations management; [API tokens](/docs/netdata-cloud/authentication-and-authorization/api-tokens.md) with scopes

**Nodes**
- [Node states](/docs/netdata-cloud/node-states-and-transitions.md): Live, Stale (history available, not currently collecting), Offline, Unseen
- [Virtual nodes](/docs/learn/node-identities.md) for remote targets (network devices, cloud resources, remote services)
- [Ephemeral nodes](/docs/nodes-ephemerality.md) with automatic cleanup rules for nodes that come and go
- 37 [automatic host labels](/docs/netdata-agent/configuration/organize-systems-metrics-and-alerts.md) plus user-defined labels; node information including Rooms and machine identity; fleet operations views; Nodes Map

**Value**

- **Service-level monitoring without configuration.** Rule-based Rooms turn labels into live service views that include every related node, and stay current as infrastructure changes.
- **Every team sees its own scope.** Rooms give each team, service or customer its dashboards, alerts and notifications, without separate tools or duplicate configuration.
- **Per-node depth inside every service view.** Because every component is monitored individually, a service view drills down to the exact server, container, device or cloud resource behind a problem.
- **Safe multi-tenancy.** Spaces isolate customers and business units completely, with roles and sensitive-data permissions controlling what each user sees.
- **Identity managed centrally.** SSO and SCIM keep users and access in the customer's identity provider.

## Security and Privacy

Netdata keeps monitoring data on the customer's infrastructure: metric samples, logs, traces and query text are stored on Netdata Agents and Parents, while Netdata Cloud receives metadata and status changes over an encrypted, outbound-only connection. Access is controlled through roles, sensitive-data permissions, single sign-on and Agent-level access controls.

**Learn more:** [Security and privacy design](/docs/security-and-privacy-design/README.md) · [Netdata for CISOs on netdata.cloud](https://www.netdata.cloud/solutions/built-for/cisos/)

**Design**

- **[Data stays at the edge](/docs/security-and-privacy-design/netdata-cloud-security.md).** Metric samples, logs, traces and database query text are stored on Agents and Parents (and the customer's own object storage, when configured); Netdata Cloud stores metadata (nodes, charts, alerts, users) and queries the data in real time when users view it.
- **[Outbound-only Cloud connection](/src/aclk/README.md).** Agents connect to Netdata Cloud over TLS-encrypted WebSockets (MQTT); no inbound ports are required. Proxy support (including SOCKS5).
- **Sensitive data gated by permission.** Logs, query text, processes and connections are available only to signed-in Space members whose role grants permission to view sensitive data.
- **[On-Prem for full control](https://www.netdata.cloud/product/cloud-on-premises/).** Enterprise On-Prem runs the complete Netdata Cloud on customer infrastructure, including air-gapped environments.

**Capabilities**

**Data protection**
- TLS for Cloud connections, streaming between Agents and Parents, and the OTLP endpoint (including mutual TLS)
- Netdata Cloud metadata hosted in US data centers
- [Anonymous Agent telemetry](/docs/netdata-agent/configuration/anonymous-telemetry-events.md) (opt-out available)

**Access control**
- Netdata Cloud: roles (Admin, Manager, Troubleshooter, Observer, Billing), room-scoped access, sensitive-data permission, enterprise SSO (Okta, OpenID Connect), SCIM, scoped API tokens; multi-factor authentication through the customer's identity provider
- Agent: bind addresses, [per-surface access lists](/docs/netdata-agent/securing-netdata-agents.md) (dashboard, streaming, management, MCP), bearer-token protection, TLS
- MCP: API key (Bearer), access lists, permissions following the user's Netdata Cloud role

**Secrets management**
- Credentials in collector configurations [referenced](/src/collectors/SECRETS.md) from environment variables, files, commands or secret stores — HashiCorp Vault, AWS Secrets Manager, Azure Key Vault, Google Secret Manager — resolved without elevated privileges

**Compliance**
- SOC 2 Type 2 certified ([Trust Center](https://trust.netdata.cloud/))
- GDPR-aligned processing
- HIPAA and PCI DSS alignment (not certified); Business Associate Agreements available for HIPAA
- [Vulnerability disclosure](https://github.com/netdata/.github/blob/main/SECURITY.md) through security@netdata.cloud or GitHub security advisories; published security advisories
- Code-signed Windows installer

**Value**

- **Data sovereignty by architecture.** Monitoring data does not leave the customer's infrastructure for storage; On-Prem removes the external dependency entirely.
- **Minimal attack surface.** Outbound-only connections and Agent-level access controls avoid exposing monitoring endpoints.
- **Least-privilege access.** Roles, Room scoping and sensitive-data permissions limit what each user can see.
- **Fits enterprise identity and compliance processes.** SSO, SCIM, SOC 2 Type 2 certification and secrets in existing vaults integrate with established controls.

## Integrations and APIs

Netdata exchanges data with other systems through exporting connectors, Prometheus-compatible endpoints, REST APIs, MCP, infrastructure-as-code tooling and incident-management integrations.

**Learn more:** [Exporting metrics documentation](/docs/exporting-metrics/README.md) · [Integrations on netdata.cloud](https://www.netdata.cloud/integrations/)

**Capabilities**

**Exporting metrics**
- Exporting engine connectors: Graphite, OpenTSDB, JSON (each also over HTTP), Prometheus remote write, AWS Kinesis, Google Cloud Pub/Sub, MongoDB
- [Prometheus remote write](/src/exporting/prometheus/integrations/prometheus_remote_write.md) to 26 documented destinations, including Thanos, Cortex, VictoriaMetrics, Kafka, Elasticsearch and others
- [Prometheus scraping](/src/exporting/prometheus/README.md) of any Agent or Parent (`/api/v1/allmetrics`)

**APIs**
- [Agent REST API](/src/web/api/README.md) (v1, v2, v3) with OpenAPI specification: data, metadata, alerts, Functions, configuration
- [Netdata Cloud API](/docs/netdata-cloud/authentication-and-authorization/api-tokens.md) with scoped tokens
- MCP servers on Agents, Parents and Netdata Cloud (see Machine Learning and AI)

**Visualization and tools**
- [Grafana data source plugin](https://github.com/netdata/netdata-grafana-datasource-plugin/blob/master/README.md) (queries through Netdata Cloud)
- Netdata Mobile App (iOS, Android) for notifications

**Infrastructure as code**
- [Terraform provider](https://registry.terraform.io/providers/netdata/netdata/latest): Spaces, Rooms, members, notification channels, silencing rules
- Helm chart for Kubernetes; [Ansible and configuration-management deployment](/docs/fleet-configuration-management.md); Docker images

**Incident and collaboration integrations**
- Notification integrations with PagerDuty, Opsgenie, ServiceNow, Splunk, ilert and others (see Alerting and Notifications)
- Netdata AI MCP Connections to GitHub, Atlassian and PagerDuty (see Machine Learning and AI)

**Value**

- **Fits existing toolchains.** Metrics flow to existing time-series databases, data lakes and dashboards; incidents flow to existing on-call and ticketing systems.
- **Automatable.** APIs, MCP and Terraform let teams manage Netdata and consume its data programmatically.

## Pricing and Licensing

**Learn more:** [Plans and billing documentation](/docs/netdata-cloud/view-plan-and-billing.md) · [Pricing on netdata.cloud](https://www.netdata.cloud/pricing/)

### Plans

| Plan | Price | For | Highlights |
|---|---|---|---|
| **Community** | $0 | Personal, non-commercial use | Up to 5 active connected nodes, 1 custom dashboard per Room |
| **Homelab** | $90/year or $10/month | Personal, non-commercial use | No hard limit on nodes (fair usage policy), no limit on custom dashboards |
| **Business** | $4.50/node/month billed yearly; $6/node/month billed monthly | Freelancers, professionals, businesses of all sizes | All features: Netdata AI, all roles (RBAC), enterprise SSO (Okta, OpenID Connect), SCIM, enterprise notification integrations, Netdata Cloud MCP, centralized configuration management, no limit on custom dashboards, audit events in the events feed |
| **Enterprise On-Prem** | Custom, from 200 node licenses | Air-gapped facilities, critical infrastructure | Everything in Business, running on customer infrastructure; priority support plans |
| **Open-source Agent** | Free (Agent GPL v3+; dashboard NCUL1) | Self-hosted, any use | Complete Agent and Parents, no limit on metrics, local dashboards |

Community and Homelab are for personal, non-commercial use only. Freelancers, professionals and businesses use the open-source Agent or the Business plan. A 14-day Business trial with no limit on nodes is available on sign-up.

Netdata is also available on [AWS Marketplace](https://aws.amazon.com/marketplace/seller-profile?id=seller-5bbjpj3csb4mw): Netdata Cloud as a pay-as-you-go subscription or an annual contract (SaaS), Netdata Cloud On-Prem (Helm chart, bring your own license) and the Netdata Agent for EKS (Helm chart, free).

### What is billed

- **Billable node:** each running Netdata Agent connected to Netdata Cloud, plus each virtual node. Sub-entities a collector discovers (hosts and VMs behind a hypervisor, cloud resources, containers) are labeled instances of the collecting Agent's node and are not billed separately; an explicit virtual node attached to a collector job adds one node for that job. Collectors that create a virtual node per resource (e.g., one per SNMP device by default, or Azure workload virtual nodes) bill each such virtual node. Containers on a monitored host are included at no extra cost.
- **Active nodes only:** offline and stale nodes (history still queryable on Parents) are not billed.
- **P90 billing:** daily, the node count at the 90th percentile of time-weighted usage; monthly, the 90th percentile of daily values (the top 3 days of a 30-day month are excluded). Short spikes and occasional high-usage days do not increase the bill.
- **Not billed:** metrics volume, Logs Monitoring (querying logs in place), users, data retention, containers.
- **Free Preview:** Logs Management (collecting, storing and indexing logs), Traces (OpenTelemetry trace storage and the Traces explorer), Network Flows (NetFlow, IPFIX, sFlow) and Digital Experience Monitoring (RUM, synthetic journeys, Lighthouse audits) are in Free Preview.
- **Netdata AI:** usage measured in AI credits — one credit per investigation or report, fractional credits for smaller actions (conversations, alert creation and suggestion); alert explanations consume no credits. Business includes 10 AI credits per month; the 14-day trial includes 10 AI credits; additional credits are purchased in the app.
- **Windows:** Windows Agents count as nodes like any other Agent and are monitored on every plan through Netdata Cloud (including Community); only the local dashboard of a standalone Windows Agent is unlocked on paid plans, and Windows Children streaming to a Linux Parent are shown on the Parent's dashboard on any plan.
- **Plan-gated features:** Netdata Cloud MCP, enterprise notification integrations (all Cloud integrations except Discord, personal email and the mobile app), additional roles, SSO and SCIM require a paid plan.

### Support

**Learn more:** [Support plans on netdata.cloud](https://www.netdata.cloud/support/)

Support by plan:
- Community: public forums, GitHub and Discord (all plans include community support)
- Business: Email/ticket support during business hours with SLA
- Enterprise: 24/7 availability, dedicated support team, phone support

**Professional Services:**
- Implementation assistance
- Architecture design
- Migration support
- Custom integrations
- Training programs

### Licensing

**Learn more:** [Open source on netdata.cloud](https://www.netdata.cloud/open-source/)

- Netdata Agent: GPL v3 or later
- Netdata dashboard (UI): Netdata Cloud UI License (NCUL1) — ships with the open-source Agent for use with Netdata Agents and Parents, not open source

## Key Differentiators

### What sets Netdata apart

1. **[Real-time](/docs/realtime-monitoring.md).** Per-second collection, visualization, anomaly detection and alerting for system metrics, with data on the dashboard within about two seconds. Many products described as "real-time" collect every 10–60 seconds.
2. **[Machine learning on every metric](/docs/ml-ai/ml-anomaly-detection/ml-anomaly-detection.md).** 18 unsupervised models per metric, trained locally on every Agent and Parent, with consensus scoring and the anomaly flag stored with every sample. Not ML on a sample of metrics, not ML as an add-on.
3. **[Automatic everything](/src/collectors/SERVICE-DISCOVERY.md).** Discovery, collection, structured dashboards and 1,100+ stock alerts start without configuration; metrics from any source are charted and analyzed the moment they arrive.
4. **[Query-less analysis](/docs/NIDL-Framework.md).** The NIDL model makes every chart explorable by node, instance, dimension and label from dropdowns — no query language required for anyone on the team.
5. **[Distributed by design](/docs/scalability.md).** Collection, storage, machine learning and alerting run on Agents and Parents; Netdata Cloud queries them in real time. No central database to scale, no cardinality limits imposed by a central store.
6. **[Data stays on the customer's infrastructure](/docs/security-and-privacy-design/README.md).** Metric samples, logs, traces and query text are stored on Agents and Parents; Netdata Cloud receives metadata only. Enterprise On-Prem removes the external dependency entirely.
7. **[Live inspection without SSH](/docs/top-monitoring-netdata-functions.md).** Functions return processes, containers, connections, services, sensors, logs, database queries and topology live from any node, to the dashboard and to AI assistants.
8. **[One platform from data center to device](/src/collectors/COLLECTORS.md).** Servers, containers, Kubernetes, databases, network devices, cloud services, logs, traces, endpoints, edge devices and digital experience in one product, one data model and one UI.
9. **Open ecosystems.** Prometheus, OpenTelemetry, Nagios and StatsD sources work as they are; OpenTelemetry is the native pipeline for logs and traces.

### By category

| Compared with | Netdata difference |
|---|---|
| **[Prometheus and Grafana](https://www.netdata.cloud/comparisons/prometheus/)** | Collection, storage, dashboards, alerting and ML in one product with zero configuration; metrics charted automatically instead of panel-by-panel with PromQL; Netdata benchmark (2025, 4.6M metrics/s): 37% less CPU, 88% less RAM, 97% less disk I/O, 13% less bandwidth, 15× longer per-second retention (up to 40× in lower tiers), 22× faster on large queries, 100% sample completeness vs 93.7% |
| **[Datadog](https://www.netdata.cloud/comparisons/datadog/), New Relic, Dynatrace and other SaaS** | Per-second resolution instead of 10–60 s; ML on every metric; data stored on customer infrastructure; per-node pricing without per-metric, per-host-add-on or ingestion charges for metrics; open-source Agent |
| **[Elasticsearch](https://www.netdata.cloud/comparisons/elk/), Loki, Splunk (logs)** | Logs queried in place on every node without shipping; Logs Management with field-level indexing at about 1/4 of raw size (Loki: ~1/4 with label-only indexing; systemd-journal: ~1×; Elasticsearch: ≥2.5×); OpenTelemetry-native pipeline |
| **[SolarWinds](https://www.netdata.cloud/solarwinds-npm-alternative/), PRTG, LibreNMS, Datadog NDM (network)** | Distributed collection with no central poller to outgrow; read-only; 1,700+ device models and 150,000+ trap definitions; L2/L3 topology, flows, BGP, licensing and application dependency mapping in one product, next to servers and applications |
| **pganalyze, Percona PMM, Datadog DBM (databases)** | 50+ databases and managed services in one tool; per-object metrics; top-query views on 12 engines; query text never stored outside the customer's infrastructure |
| **Traditional monitoring ([Nagios](https://www.netdata.cloud/comparisons/nagios/), Zabbix, Checkmk)** | Automatic discovery and dashboards instead of templates; per-second data; ML anomaly detection; existing Nagios plugins keep working |

## Use Cases and Customers

**Learn more:** [Case studies on netdata.cloud](https://www.netdata.cloud/case-studies/)

### Adoption

- Open source with a large community: 80,000+ GitHub stars, 690+ contributors
- Netdata Cloud SaaS serves more than 100,000 reachable nodes
- Customers across technology, financial services, education, government, healthcare, hosting and cloud, gaming and digital media, retail, logistics, manufacturing, transportation and automotive (38 published case studies)

### Standard figures

Netdata's standard figures, used consistently across Netdata materials:

- **90% cost reduction** compared with traditional and SaaS observability stacks (90% lower TCO)
- **80% faster MTTR**
- **~25% reduction in infrastructure downtime**
- **Zero data loss:** per-second samples are collected without sampling, and Parents back-fill gaps through replication
- **1.5 million downloads per day**; **688M+ Docker Hub pulls**
- **4.5+ billion metrics per second** processed globally
- **Logs without pipelines:** Logs Monitoring queries logs where they are generated, with no shipping or pipeline to operate
- **Console replacement:** Functions provide browser-based, live inspection of processes, services, connections, logs and more, replacing SSH sessions for troubleshooting

### Common use cases

- [Infrastructure and Kubernetes monitoring](https://www.netdata.cloud/solutions/use-cases/infrastructure-monitoring/) with per-second resolution and automatic dashboards
- Replacing Prometheus/Grafana stacks or per-host SaaS monitoring to reduce cost and operational effort
- [Network performance monitoring](https://www.netdata.cloud/solutions/use-cases/network-monitoring/): devices, topology, flows, traps, BGP
- Logs exploration across the fleet, and OpenTelemetry-based logs and traces
- [Database monitoring](https://www.netdata.cloud/solutions/use-cases/database-monitoring/) across many engines without a DBA per engine
- [Hybrid and multi-cloud monitoring](https://www.netdata.cloud/solutions/use-cases/hybrid-cloud-observability/) (AWS, Azure, on-premises)
- [Edge and IoT fleets](https://www.netdata.cloud/solutions/use-cases/edge-fleet-monitoring/) on constrained devices and cellular links
- [Air-gapped and regulated environments](https://www.netdata.cloud/product/cloud-on-premises/) with Enterprise On-Prem
- [AI-assisted troubleshooting and root-cause analysis](https://www.netdata.cloud/solutions/use-cases/troubleshooting/) on live data

## Support and Resources

### Documentation

**Official Documentation**: https://learn.netdata.cloud
- Guides, API reference, troubleshooting and best practices

### Community Support

**Community Channels:**
- Forum: https://community.netdata.cloud
- GitHub Discussions: https://github.com/netdata/netdata/discussions
- Discord: https://discord.com/invite/2mEmfW735j
- GitHub Issues: Bug reports and feature requests (https://github.com/netdata/netdata/issues)



## Getting Started

**Learn more:** [Getting started guide](/docs/getting-started-netdata/guide.md)

### Quick Start

**Step 1: Install Agent**
```bash
# Linux/macOS
bash <(curl -Ss https://get.netdata.cloud/kickstart.sh)

# Docker
docker run -d --name=netdata -p 19999:19999 netdata/netdata:stable   # see the Docker guide for recommended host mounts

# Kubernetes
helm repo add netdata https://netdata.github.io/helmchart/
helm install netdata netdata/netdata

# Windows
# MSI installer: see the Windows installation guide
```

Installation guides: [Linux and macOS](/packaging/installer/methods/kickstart.md) · [Docker](/packaging/docker/README.md) · [Kubernetes](/packaging/installer/methods/kubernetes.md) · [Windows](/packaging/windows/WINDOWS_INSTALLER.md)

**Step 2: Access Dashboard**
- Local: http://localhost:19999
- Cloud: https://app.netdata.cloud (after connecting the Agent)

**Step 3: [Connect to Netdata Cloud](/src/claim/README.md) (Optional)**
- Sign in from the local dashboard
- Or connect at install time: `bash <(curl -Ss https://get.netdata.cloud/kickstart.sh) --claim-token <CLAIM_TOKEN> --claim-rooms <ROOM_IDS> --claim-url https://app.netdata.cloud`

**Step 4: Configure (Optional)**
- [Main settings](/docs/netdata-agent/configuration/README.md) in `/etc/netdata/netdata.conf`
- [Configure collectors](/src/collectors/REFERENCE.md) from the UI (dynamic configuration) or in `/etc/netdata/go.d/`, `/etc/netdata/scripts.d/`
- [Set up streaming](/docs/observability-centralization-points/metrics-centralization-points/configuration.md) in `/etc/netdata/stream.conf`

**Step 5: Explore**
- Navigate dashboards
- Review pre-configured alerts
- Explore logs (systemd-journal, Windows Event Log, macOS unified logs)
- Try AI troubleshooting

### Enterprise Deployment

**Learn more:** [Enterprise evaluation guide](/docs/netdata-enterprise-evaluation.md)

**5 Steps:**

1. **Design Topology** (1 day)
   - Decide Parent placement (one cluster per ~500 nodes)
   - Position by geography/provider
   - Teams organize later via Rooms

2. **Deploy Everywhere** (1 day)
   - Use [Ansible/Terraform/Helm](/docs/fleet-configuration-management.md)
   - Install Agents + Parents
   - Same binary, different roles

3. **Add Collector Credentials** (2 hours)
   - UI shows discovered services needing auth
   - Configure databases, cloud accounts, SNMP
   - Enable custom app collectors

4. **Review Alerts** (2 hours)
   - 1,100+ pre-configured alerts already running
   - [Tune thresholds, disable irrelevant ones](/src/health/overriding-stock-alerts.md)
   - ML is already training

5. **Invite Teams** (2 hours)
   - [Create Rooms](/docs/netdata-cloud/organize-your-infrastructure-invite-your-team.md) for team isolation
   - Add users, set permissions
   - Each team sees their infrastructure slice


## Mission

Netdata's mission is to reveal and surface the pulse and breath of systems and applications, making infrastructure health visible and actionable without requiring deep expertise.

Netdata is focused on making observability simple and affordable for operations teams: monitoring should work for operators, not the other way around. Netdata removes the work that makes monitoring hard — choosing and integrating tools, configuring collection, building dashboards, writing alerts, tuning thresholds, scaling central databases — so that teams get complete, real-time visibility and fast answers from the moment they install it, at a cost that lets them monitor everything instead of choosing what to leave out.

The roadmap follows this mission: broader coverage that works without configuration, deeper automation through machine learning and AI, and open standards that let teams bring the tools and instrumentation they already use.

## FAQ

**My device is not in the profile list. What can I do?**
Netdata already monitors it through standard MIBs. For full device-specific coverage, either add it — [profiles are documented YAML files](/src/go/plugin/go.d/collector/snmp/profile-format.md); create a new profile or extend an existing one, no code required — or ask Netdata, which adds missing device profiles for customers as required.

**How does Netdata receive SNMP traps?**
Natively: the [`snmp_traps` collector](/docs/npm/snmp-traps/README.md) receives SNMP v1, v2c and v3 traps and INFORMs directly (no `snmptrapd`), decodes them against 150,000+ trap definitions, and stores them as searchable logs, with optional OTLP forwarding.

**How do I send syslog to Netdata?**
Through Netdata's OpenTelemetry logs pipeline: the OpenTelemetry Collector's [syslog receiver](/docs/npm/syslog/README.md) (RFC 3164/5424, UDP/TCP) forwards logs to Netdata Logs Management. Logs already in systemd-journal, Windows Event Log or macOS unified logs are queried in place by Logs Monitoring.

**How do I forward logs to a SIEM?**
The OpenTelemetry Collector stage of the pipeline routes copies of logs to any destination, including SIEMs; SNMP traps can also be [exported over OTLP](/docs/npm/snmp-traps/forwarding-to-siem.md).

**Does Netdata poll network devices every second?**
Collection intervals are [configurable per device](/docs/npm/device-metrics/configuration.md). Netdata's distributed collection keeps polling on schedule at any network size, as frequently as needed, down to every second where devices support it and it is useful.

**How does Netdata relate to SASE platforms?**
SASE platforms sit in the traffic path and enforce policy; Netdata stays out of the path and measures what happens on systems and networks, providing an independent view. Netdata also monitors [Cato Networks](/src/go/plugin/go.d/collector/cato_networks/integrations/cato_networks.md) SASE sites, devices and BGP through the Cato API.

## Summary

Netdata makes observability simple and affordable for operations teams. One distributed platform collects everything — from servers and Kubernetes to network devices, databases, cloud services, logs, traces, devices and browsers — at per-second resolution for system metrics, analyzes every metric with machine learning, and turns it into dashboards, alerts and AI-assisted answers automatically.

- **Complete:** [800+ integrations](/src/collectors/COLLECTORS.md) plus the Prometheus, OpenTelemetry, Nagios and StatsD ecosystems, in one data model and one UI
- **Real-time:** per-second collection, visualization, anomaly detection and alerting
- **Automatic:** zero-configuration discovery, dashboards and 1,100+ stock alerts
- **Intelligent:** [18 ML models per metric](/docs/category-overview-pages/machine-learning-and-assisted-troubleshooting.md), Anomaly Advisor, Metric Correlations, Netdata AI and MCP
- **Distributed:** no central bottleneck; [scales from one node to 100,000+](/docs/scalability.md)
- **Sovereign:** [data stays on the customer's infrastructure](/docs/security-and-privacy-design/README.md); SaaS or On-Prem
- **Open:** open-source Agent, 80,000+ GitHub stars, standards-based pipelines

## Contact and Resources

- **Website**: https://www.netdata.cloud
- **Documentation**: https://learn.netdata.cloud
- **GitHub**: https://github.com/netdata/netdata
- **Community**: https://community.netdata.cloud
- **Discord**: https://discord.com/invite/2mEmfW735j
- **Pricing**: https://www.netdata.cloud/pricing/
- **ROI Calculator**: https://www.netdata.cloud/value/roi/
- **Sales**: https://www.netdata.cloud/contact-sales/
- **Enterprise**: https://www.netdata.cloud/request-enterprise/
- **Support**: https://www.netdata.cloud/support/
- **Partnerships**: https://www.netdata.cloud/partnerships/
- **Referral Program**: https://www.netdata.cloud/referral/
- **SOC 2 Trust Center**: https://trust.netdata.cloud/

**Netdata Cloud**: https://app.netdata.cloud

---

*This product description is based on Netdata source code, official documentation and publicly available information as of October 2026.*
