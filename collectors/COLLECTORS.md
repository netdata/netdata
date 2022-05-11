<!--
title: "Supported collectors list"
description: "Netdata gathers real-time metrics from hundreds of data sources using collectors. Most require zero configuration and are pre-configured out of the box."
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/COLLECTORS.md
-->

# Supported collectors list

Netdata uses collectors to help you gather metrics from your favorite applications and services and view them in
real-time, interactive charts. The following list includes collectors for both external services/applications and
internal system metrics.

Learn more about [how collectors work](/docs/collect/how-collectors-work.md), and then learn how to [enable or
configure](/docs/collect/enable-configure.md) any of the below collectors using the same process.

Some collectors have both Go and Python versions as we continue our effort to migrate all collectors to Go. In these
cases, _Netdata always prioritizes the Go version_, and we highly recommend you use the Go versions for the best
experience.

If you want to use a Python version of a collector, you need to explicitly [disable the Go
version](/docs/collect/enable-configure.md), and enable the Python version. Netdata then skips the Go version and
attempts to load the Python version and its accompanying configuration file.

If you don't see the app/service you'd like to monitor in this list, check out our [GitHub
issues](https://github.com/netdata/netdata/issues). Use the search bar to look for previous discussions about that
collector—we may be looking for contributions from users such as yourself! If you don't see the collector there, make a
[feature request](https://community.netdata.cloud/c/feature-requests/7/none) on our community forums.

- [Supported collectors list](#supported-collectors-list)
  - [Service and application collectors](#service-and-application-collectors)
    - [Generic](#generic)
    - [APM (application performance monitoring)](#apm-application-performance-monitoring)
    - [Containers and VMs](#containers-and-vms)
    - [Data stores](#data-stores)
    - [Distributed computing](#distributed-computing)
    - [Email](#email)
    - [Kubernetes](#kubernetes)
    - [Logs](#logs)
    - [Messaging](#messaging)
    - [Network](#network)
    - [Provisioning](#provisioning)
    - [Remote devices](#remote-devices)
    - [Search](#search)
    - [Storage](#storage)
    - [Web](#web)
  - [System collectors](#system-collectors)
    - [Applications](#applications)
    - [Disks and filesystems](#disks-and-filesystems)
    - [eBPF](#ebpf)
    - [Hardware](#hardware)
    - [Memory](#memory)
    - [Networks](#networks)
    - [Operating systems](#operating-systems)
    - [Processes](#processes)
    - [Resources](#resources)
    - [Users](#users)
  - [Netdata collectors](#netdata-collectors)
  - [Orchestrators](#orchestrators)
  - [Third-party collectors](#third-party-collectors)
  - [Etc](#etc)

## Service and application collectors

The Netdata Agent auto-detects and collects metrics from all of the services and applications below. You can also
configure any of these collectors according to your setup and infrastructure.

### Generic

- [Prometheus endpoints](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/prometheus): Gathers
  metrics from any number of Prometheus endpoints, with support to autodetect more than 600 services and applications.

### APM (application performance monitoring)

- [Go applications](/collectors/python.d.plugin/go_expvar/README.md): Monitor any Go application that exposes its
  metrics with the  `expvar` package from the Go standard library.
- [Java Spring Boot 2
  applications](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/springboot2/) (Go version):
  Monitor running Java Spring Boot 2 applications that expose their metrics with the use of the Spring Boot Actuator.
- [Java Spring Boot 2 applications](/collectors/python.d.plugin/springboot/README.md) (Python version): Monitor
  running Java Spring Boot applications that expose their metrics with the use of the Spring Boot Actuator.
- [statsd](/collectors/statsd.plugin/README.md): Implement a high performance `statsd` server for Netdata.
- [phpDaemon](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/phpdaemon/): Collect worker
  statistics (total, active, idle), and uptime for web and network applications.
- [uWSGI](/collectors/python.d.plugin/uwsgi/README.md): Monitor performance metrics exposed by the uWSGI Stats
  Server.

### Containers and VMs

- [Docker containers](/collectors/cgroups.plugin/README.md): Monitor the health and performance of individual Docker
  containers using the cgroups collector plugin.
- [DockerD](/collectors/python.d.plugin/dockerd/README.md): Collect container health statistics.
- [Docker Engine](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/docker_engine/): Collect
  runtime statistics from the `docker` daemon using the `metrics-address` feature.
- [Docker Hub](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/dockerhub/): Collect statistics
  about Docker repositories, such as pulls, starts, status, time since last update, and more.
- [Libvirt](/collectors/cgroups.plugin/README.md): Monitor the health and performance of individual Libvirt containers
  using the cgroups collector plugin.
- [LXC](/collectors/cgroups.plugin/README.md): Monitor the health and performance of individual LXC containers using
  the cgroups collector plugin.
- [LXD](/collectors/cgroups.plugin/README.md): Monitor the health and performance of individual LXD containers using
  the cgroups collector plugin.
- [systemd-nspawn](/collectors/cgroups.plugin/README.md): Monitor the health and performance of individual
  systemd-nspawn containers using the cgroups collector plugin.
- [vCenter Server Appliance](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/vcsa/): Monitor
  appliance system, components, and software update health statuses via the Health API.
- [vSphere](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/vsphere/): Collect host and virtual
  machine performance metrics.
- [Xen/XCP-ng](/collectors/xenstat.plugin/README.md): Collect XenServer and XCP-ng metrics using `libxenstat`.

### Data stores

- [CockroachDB](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/cockroachdb/): Monitor various
  database components using `_status/vars` endpoint.
- [Consul](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/consul/): Capture service and unbound
  checks status (passing, warning, critical, maintenance).
- [Couchbase](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/couchbase/): Gather per-bucket
  metrics from any number of instances of the distributed JSON document database.
- [CouchDB](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/couchdb): Monitor database health and
  performance metrics
  (reads/writes, HTTP traffic, replication status, etc).
- [MongoDB](/collectors/python.d.plugin/mongodb/README.md): Collect memory-caching system performance metrics and
  reads the server's response to `stats` command (stats interface).
- [MySQL](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/mysql/): Collect database global,
  replication and per user statistics.
- [OracleDB](/collectors/python.d.plugin/oracledb/README.md): Monitor database performance and health metrics.
- [Pika](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/pika/): Gather metric, such as clients,
  memory usage, queries, and more from the Redis interface-compatible database.
- [Postgres](/collectors/python.d.plugin/postgres/README.md): Collect database health and performance metrics.
- [ProxySQL](/collectors/python.d.plugin/proxysql/README.md): Monitor database backend and frontend performance
  metrics.
- [Redis](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/redis/): Monitor status from any
  number of database instances by reading the server's response to the `INFO ALL` command.
- [RethinkDB](/collectors/python.d.plugin/rethinkdbs/README.md): Collect database server and cluster statistics.
- [Riak KV](/collectors/python.d.plugin/riakkv/README.md): Collect database stats from the `/stats` endpoint.
- [Zookeeper](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/zookeeper/): Monitor application
  health metrics reading the server's response to the `mntr` command.
- [Memcached](/collectors/python.d.plugin/memcached/README.md): Collect memory-caching system performance metrics.

### Distributed computing

- [BOINC](/collectors/python.d.plugin/boinc/README.md): Monitor the total number of tasks, open tasks, and task
  states for the distributed computing client.
- [Gearman](/collectors/python.d.plugin/gearman/README.md): Collect application summary (queued, running) and per-job
  worker statistics (queued, idle, running).

### Email

- [Dovecot](/collectors/python.d.plugin/dovecot/README.md): Collect email server performance metrics by reading the
  server's response to the `EXPORT global` command.
- [EXIM](/collectors/python.d.plugin/exim/README.md): Uses the `exim` tool to monitor the queue length of a
  mail/message transfer agent (MTA).
- [Postfix](/collectors/python.d.plugin/postfix/README.md): Uses the `postqueue` tool to monitor the queue length of a
  mail/message transfer agent (MTA).

### Kubernetes

- [Kubelet](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/k8s_kubelet/): Monitor one or more
  instances of the Kubelet agent and collects metrics on number of pods/containers running, volume of Docker
  operations, and more.
- [kube-proxy](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/k8s_kubeproxy/): Collect
  metrics, such as syncing proxy rules and REST client requests, from one or more instances of `kube-proxy`.
- [Service discovery](https://github.com/netdata/agent-service-discovery/): Find what services are running on a
  cluster's pods, converts that into configuration files, and exports them so they can be monitored by Netdata.

### Logs

- [Fluentd](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/fluentd/): Gather application
  plugins metrics from an endpoint provided by `in_monitor plugin`.
- [Logstash](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/logstash/): Monitor JVM threads,
  memory usage, garbage collection statistics, and more.
- [OpenVPN status logs](/collectors/python.d.plugin/ovpn_status_log/README.md): Parse server log files and provide
  summary
  (client, traffic) metrics.
- [Squid web server logs](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/squidlog/): Tail Squid
  access logs to return the volume of requests, types of requests, bandwidth, and much more.
- [Web server logs (Go version for Apache,
  NGINX)](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/weblog/): Tail access logs and provide
  very detailed web server performance statistics. This module is able to parse 200k+ rows in less than half a second.
- [Web server logs (Apache, NGINX)](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/weblog): Tail
  access log
  file and collect web server/caching proxy metrics.

### Messaging

- [ActiveMQ](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/activemq/): Collect message broker
  queues and topics statistics using the ActiveMQ Console API.
- [Beanstalk](/collectors/python.d.plugin/beanstalk/README.md): Collect server and tube-level statistics, such as CPU
  usage, jobs rates, commands, and more.
- [Pulsar](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/pulsar/): Collect summary,
  namespaces, and topics performance statistics.
- [RabbitMQ (Go)](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/rabbitmq/): Collect message
  broker overview, system and per virtual host metrics.
- [RabbitMQ (Python)](/collectors/python.d.plugin/rabbitmq/README.md): Collect message broker global and per virtual
  host metrics.
- [VerneMQ](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/vernemq/): Monitor MQTT broker
  health and performance metrics. It collects all available info for both MQTTv3 and v5 communication

### Network

- [Bind 9](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/bind/): Collect nameserver summary
  performance statistics via a web interface (`statistics-channels` feature).
- [Chrony](/collectors/python.d.plugin/chrony/README.md): Monitor the precision and statistics of a local `chronyd`
  server.
- [CoreDNS](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/coredns/): Measure DNS query round
  trip time.
- [Dnsmasq](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/dnsmasq_dhcp/): Automatically
  detects all configured `Dnsmasq` DHCP ranges and Monitor their utilization.
- [DNSdist](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/dnsdist/): Collect
  load-balancer performance and health metrics.
- [Dnsmasq DNS Forwarder](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/dnsmasq/): Gather
  queries, entries, operations, and events for the lightweight DNS forwarder.
- [DNS Query Time](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/dnsquery/): Monitor the round
  trip time for DNS queries in milliseconds.
- [Freeradius](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/freeradius/): Collect
  server authentication and accounting statistics from the `status server`.
- [Libreswan](/collectors/charts.d.plugin/libreswan/README.md): Collect bytes-in, bytes-out, and uptime metrics.
- [Icecast](/collectors/python.d.plugin/icecast/README.md): Monitor the number of listeners for active sources.
- [ISC Bind (RDNC)](/collectors/python.d.plugin/bind_rndc/README.md): Collect nameserver summary performance
  statistics using the `rndc` tool.
- [ISC DHCP](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/isc_dhcpd): Reads a
  `dhcpd.leases` file and collects metrics on total active leases, pool active leases, and pool utilization.
- [OpenLDAP](/collectors/python.d.plugin/openldap/README.md): Provides statistics information from the OpenLDAP
  (`slapd`) server.
- [NSD](/collectors/python.d.plugin/nsd/README.md): Monitor nameserver performance metrics using the `nsd-control`
  tool.
- [NTP daemon](/collectors/python.d.plugin/ntpd/README.md): Monitor the system variables of the local `ntpd` daemon
  (optionally including variables of the polled peers) using the NTP Control Message Protocol via a UDP socket.
- [OpenSIPS](/collectors/charts.d.plugin/opensips/README.md): Collect server health and performance metrics using the
  `opensipsctl` tool.
- [OpenVPN](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/openvpn/): Gather server summary
  (client, traffic) and per user metrics (traffic, connection time) stats using `management-interface`.
- [Pi-hole](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/pihole/): Monitor basic (DNS
  queries, clients, blocklist) and extended (top clients, top permitted, and blocked domains) statistics using the PHP
  API.
- [PowerDNS Authoritative Server](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/powerdns):
  Monitor one or more instances of the nameserver software to collect questions, events, and latency metrics.
- [PowerDNS Recursor](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/powerdns_recursor):
  Gather incoming/outgoing questions, drops, timeouts, and cache usage from any number of DNS recursor instances.
- [RetroShare](/collectors/python.d.plugin/retroshare/README.md): Monitor application bandwidth, peers, and DHT
  metrics.
- [Tor](/collectors/python.d.plugin/tor/README.md): Capture traffic usage statistics using the Tor control port.
- [Unbound](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/unbound/): Collect DNS resolver
  summary and extended system and per thread metrics via the `remote-control` interface.

### Provisioning

- [Puppet](/collectors/python.d.plugin/puppet/README.md): Monitor the status of Puppet Server and Puppet DB.

### Remote devices

- [AM2320](/collectors/python.d.plugin/am2320/README.md): Monitor sensor temperature and humidity.
- [Access point](/collectors/charts.d.plugin/ap/README.md): Monitor client, traffic and signal metrics using the `aw`
    tool.
- [APC UPS](/collectors/charts.d.plugin/apcupsd/README.md): Capture status information using the `apcaccess` tool.
- [Energi Core](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/energid): Monitor
    blockchain indexes, memory usage, network usage, and transactions of wallet instances.
- [UPS/PDU](/collectors/charts.d.plugin/nut/README.md): Read the status of UPS/PDU devices using the `upsc` tool.
- [SNMP devices](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/snmp): Gather data using the SNMP protocol.
-   [1-Wire sensors](/collectors/python.d.plugin/w1sensor/README.md): Monitor sensor temperature.

### Search

- [Elasticsearch](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/elasticsearch): Collect
  dozens of metrics on search engine performance from local nodes and local indices. Includes cluster health and
  statistics.
- [Solr](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/solr/): Collect application search
  requests, search errors, update requests, and update errors statistics.

### Storage

- [Ceph](/collectors/python.d.plugin/ceph/README.md): Monitor the Ceph cluster usage and server data consumption.
- [HDFS](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/hdfs/): Monitor health and performance
  metrics for filesystem datanodes and namenodes.
- [IPFS](/collectors/python.d.plugin/ipfs/README.md): Collect file system bandwidth, peers, and repo metrics.
- [Scaleio](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/scaleio/): Monitor storage system,
  storage pools, and SDCS health and performance metrics via VxFlex OS Gateway API.
- [Samba](/collectors/python.d.plugin/samba/README.md): Collect file sharing metrics using the `smbstatus` tool.

### Web

- [Apache](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/apache/): Collect Apache web
  server performance metrics via the `server-status?auto` endpoint.
- [HAProxy](/collectors/python.d.plugin/haproxy/README.md): Collect frontend, backend, and health metrics.
- [HTTP endpoints](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/httpcheck/): Monitor
  any HTTP endpoint's availability and response time.
- [Lighttpd](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/lighttpd/): Collect web server
  performance metrics using the `server-status?auto` endpoint.
- [Lighttpd2](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/lighttpd2/): Collect web server
  performance metrics using the `server-status?format=plain` endpoint.
- [Litespeed](/collectors/python.d.plugin/litespeed/README.md): Collect web server data (network, connection,
  requests, cache) by reading `.rtreport*` files.
- [Nginx](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/nginx/): Monitor web server
  status information by gathering metrics via `ngx_http_stub_status_module`.
- [Nginx VTS](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/nginxvts/): Gathers metrics from
  any Nginx deployment with the _virtual host traffic status module_ enabled, including metrics on uptime, memory
  usage, and cache, and more.
- [PHP-FPM](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/phpfpm/): Collect application
  summary and processes health metrics by scraping the status page (`/status?full`).
- [TCP endpoints](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/portcheck/): Monitor any
  TCP endpoint's availability and response time.
- [Spigot Minecraft servers](/collectors/python.d.plugin/spigotmc/README.md): Monitor average ticket rate and number
  of users.
- [Squid](/collectors/python.d.plugin/squid/README.md): Monitor client and server bandwidth/requests by gathering
  data from the Cache Manager component.
- [Tengine](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/tengine/): Monitor web server
  statistics using information provided by `ngx_http_reqstat_module`.
- [Tomcat](/collectors/python.d.plugin/tomcat/README.md): Collect web server performance metrics from the Manager App
  (`/manager/status?XML=true`).
- [Traefik](/collectors/python.d.plugin/traefik/README.md): Uses Traefik's Health API to provide statistics.
- [Varnish](/collectors/python.d.plugin/varnish/README.md): Provides HTTP accelerator global, backends (VBE), and
  disks (SMF) statistics using the `varnishstat` tool.
- [x509 check](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/x509check/): Monitor certificate
  expiration time.
- [Whois domain expiry](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/whoisquery/): Checks the
  remaining time until a given domain is expired.

## System collectors

The Netdata Agent can collect these system- and hardware-level metrics using a variety of collectors, some of which
(such as `proc.plugin`) collect multiple types of metrics simultaneously.

### Applications

- [Fail2ban](/collectors/python.d.plugin/fail2ban/README.md): Parses configuration files to detect all jails, then
  uses log files to report ban rates and volume of banned IPs.
- [Monit](/collectors/python.d.plugin/monit/README.md): Monitor statuses of targets (service-checks) using the XML
  stats interface.
- [WMI (Windows Management Instrumentation)
  exporter](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/wmi/): Collect CPU, memory,
  network, disk, OS, system, and log-in metrics scraping `wmi_exporter`.

### Disks and filesystems

- [BCACHE](/collectors/proc.plugin/README.md): Monitor BCACHE statistics with the the `proc.plugin` collector.
- [Block devices](/collectors/proc.plugin/README.md): Gather metrics about the health and performance of block
  devices using the the `proc.plugin` collector.
- [Btrfs](/collectors/proc.plugin/README.md): Monitors Btrfs filesystems with the the `proc.plugin` collector.
- [Device mapper](/collectors/proc.plugin/README.md): Gather metrics about the Linux device mapper with the proc
  collector.
- [Disk space](/collectors/diskspace.plugin/README.md): Collect disk space usage metrics on Linux mount points.
- [Clock synchronization](/collectors/timex.plugin/README.md): Collect the system clock synchronization status on Linux.
- [Files and directories](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/filecheck): Gather
  metrics about the existence, modification time, and size of files or directories.
- [ioping.plugin](/collectors/ioping.plugin/README.md): Measure disk read/write latency.
- [NFS file servers and clients](/collectors/proc.plugin/README.md): Gather operations, utilization, and space usage
  using the the `proc.plugin` collector.
- [RAID arrays](/collectors/proc.plugin/README.md): Collect health, disk status, operation status, and more with the
  the `proc.plugin` collector.
- [Veritas Volume Manager](/collectors/proc.plugin/README.md): Gather metrics about the Veritas Volume Manager (VVM).
- [ZFS](/collectors/proc.plugin/README.md): Monitor bandwidth and utilization of ZFS disks/partitions using the proc
  collector.

### eBPF

- [Files](/collectors/ebpf.plugin/README.md): Provides information about how often a system calls kernel
  functions related to file descriptors using the eBPF collector.
- [Virtual file system (VFS)](/collectors/ebpf.plugin/README.md): Monitor IO, errors, deleted objects, and
  more for kernel virtual file systems (VFS) using the eBPF collector.
- [Processes](/collectors/ebpf.plugin/README.md): Monitor threads, task exits, and errors using the eBPF collector.

### Hardware

- [Adaptec RAID](/collectors/python.d.plugin/adaptec_raid/README.md): Monitor logical and physical devices health
  metrics using the `arcconf` tool.
- [CUPS](/collectors/cups.plugin/README.md): Monitor CUPS.
- [FreeIPMI](/collectors/freeipmi.plugin/README.md): Uses `libipmimonitoring-dev` or `libipmimonitoring-devel` to
  monitor the number of sensors, temperatures, voltages, currents, and more.
- [Hard drive temperature](/collectors/python.d.plugin/hddtemp/README.md): Monitor the temperature of storage
  devices.
- [HP Smart Storage Arrays](/collectors/python.d.plugin/hpssa/README.md): Monitor controller, cache module, logical
  and physical drive state, and temperature using the `ssacli` tool.
- [MegaRAID controllers](/collectors/python.d.plugin/megacli/README.md): Collect adapter, physical drives, and
  battery stats using the `megacli` tool.
- [NVIDIA GPU](/collectors/python.d.plugin/nvidia_smi/README.md): Monitor performance metrics (memory usage, fan
  speed, pcie bandwidth utilization, temperature, and more) using the `nvidia-smi` tool.
- [Sensors](/collectors/python.d.plugin/sensors/README.md): Reads system sensors information (temperature, voltage,
  electric current, power, and more) from `/sys/devices/`.
- [S.M.A.R.T](/collectors/python.d.plugin/smartd_log/README.md): Reads SMART Disk Monitoring daemon logs.

### Memory

- [Available memory](/collectors/proc.plugin/README.md): Tracks changes in available RAM using the the `proc.plugin`
  collector.
- [Committed memory](/collectors/proc.plugin/README.md): Monitor committed memory using the `proc.plugin` collector.
- [Huge pages](/collectors/proc.plugin/README.md): Gather metrics about huge pages in Linux and FreeBSD with the
  `proc.plugin` collector.
- [KSM](/collectors/proc.plugin/README.md): Measure the amount of merging, savings, and effectiveness using the
  `proc.plugin` collector.
- [Numa](/collectors/proc.plugin/README.md): Gather metrics on the number of non-uniform memory access (NUMA) events
  every second using the `proc.plugin` collector.
- [Page faults](/collectors/proc.plugin/README.md): Collect the number of memory page faults per second using the
  `proc.plugin` collector.
- [RAM](/collectors/proc.plugin/README.md): Collect metrics on system RAM, available RAM, and more using the
  `proc.plugin` collector.
- [SLAB](/collectors/slabinfo.plugin/README.md): Collect kernel SLAB details on Linux systems.
- [swap](/collectors/proc.plugin/README.md): Monitor the amount of free and used swap at every second using the
  `proc.plugin` collector.
- [Writeback memory](/collectors/proc.plugin/README.md): Collect how much memory is actively being written to disk at
  every second using the `proc.plugin` collector.

### Networks

- [Access points](/collectors/charts.d.plugin/ap/README.md): Visualizes data related to access points.
- [fping.plugin](fping.plugin/README.md): Measure network latency, jitter and packet loss between the monitored node
  and any number of remote network end points.
- [Netfilter](/collectors/nfacct.plugin/README.md): Collect netfilter firewall, connection tracker, and accounting
  metrics using `libmnl` and `libnetfilter_acct`.
- [Network stack](/collectors/proc.plugin/README.md): Monitor the networking stack for errors, TCP connection aborts,
  bandwidth, and more.
- [Network QoS](/collectors/tc.plugin/README.md): Collect traffic QoS metrics (`tc`) of Linux network interfaces.
- [SYNPROXY](/collectors/proc.plugin/README.md): Monitor entries uses, SYN packets received, TCP cookies, and more.

### Operating systems

- [freebsd.plugin](freebsd.plugin/README.md): Collect resource usage and performance data on FreeBSD systems.
- [macOS](/collectors/macos.plugin/README.md): Collect resource usage and performance data on macOS systems.

### Processes

- [Applications](/collectors/apps.plugin/README.md): Gather CPU, disk, memory, network, eBPF, and other metrics per
  application using the `apps.plugin` collector.
- [systemd](/collectors/cgroups.plugin/README.md): Monitor the CPU and memory usage of systemd services using the
  `cgroups.plugin` collector.
- [systemd unit states](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/systemdunits): See the
  state (active, inactive, activating, deactivating, failed) of various systemd unit types.
- [System processes](/collectors/proc.plugin/README.md): Collect metrics on system load and total processes running
  using `/proc/loadavg` and the `proc.plugin` collector.
- [Uptime](/collectors/proc.plugin/README.md): Monitor the uptime of a system using the `proc.plugin` collector.

### Resources

- [CPU frequency](/collectors/proc.plugin/README.md): Monitor CPU frequency, as set by the `cpufreq` kernel module,
  using the `proc.plugin` collector.
- [CPU idle](/collectors/proc.plugin/README.md): Measure CPU idle every second using the `proc.plugin` collector.
- [CPU performance](/collectors/perf.plugin/README.md): Collect CPU performance metrics using performance monitoring
  units (PMU).
- [CPU throttling](/collectors/proc.plugin/README.md): Gather metrics about thermal throttling using the `/proc/stat`
  module and the `proc.plugin` collector.
- [CPU utilization](/collectors/proc.plugin/README.md): Capture CPU utilization, both system-wide and per-core, using
  the `/proc/stat` module and the `proc.plugin` collector.
- [Entropy](/collectors/proc.plugin/README.md): Monitor the available entropy on a system using the `proc.plugin`
  collector.
- [Interprocess Communication (IPC)](/collectors/proc.plugin/README.md): Monitor IPC semaphores and shared memory
  using the `proc.plugin` collector.
- [Interrupts](/collectors/proc.plugin/README.md): Monitor interrupts per second using the `proc.plugin` collector.
- [IdleJitter](/collectors/idlejitter.plugin/README.md): Measure CPU latency and jitter on all operating systems.
- [SoftIRQs](/collectors/proc.plugin/README.md): Collect metrics on SoftIRQs, both system-wide and per-core, using the
  `proc.plugin` collector.
- [SoftNet](/collectors/proc.plugin/README.md): Capture SoftNet events per second, both system-wide and per-core,
  using the `proc.plugin` collector.

### Users

- [systemd-logind](/collectors/python.d.plugin/logind/README.md): Monitor active sessions, users, and seats tracked
  by `systemd-logind` or `elogind`.
- [User/group usage](/collectors/apps.plugin/README.md): Gather CPU, disk, memory, network, and other metrics per user
  and user group using the `apps.plugin` collector.

## Netdata collectors

These collectors are recursive in nature, in that they monitor some function of the Netdata Agent itself. Some
collectors are described only in code and associated charts in Netdata dashboards.

- [ACLK (code only)](https://github.com/netdata/netdata/blob/master/aclk/legacy/aclk_stats.c): View whether a Netdata
  Agent is connected to Netdata Cloud via the [ACLK](/aclk/README.md), the volume of queries, process times, and more.
- [Alarms](https://learn.netdata.cloud/docs/agent/collectors/python.d.plugin/alarms): This collector creates an
  **Alarms** menu with one line plot showing the alarm states of a Netdata Agent over time.
- [Anomalies](https://learn.netdata.cloud/docs/agent/collectors/python.d.plugin/anomalies): This collector uses the
  Python PyOD library to perform unsupervised anomaly detection on your Netdata charts and/or dimensions.
- [Exporting (code only)](https://github.com/netdata/netdata/blob/master/exporting/send_internal_metrics.c): Gather
  metrics on CPU utilization for the [exporting engine](/exporting/README.md), and specific metrics for each enabled
  exporting connector.
- [Global statistics (code only)](https://github.com/netdata/netdata/blob/master/daemon/global_statistics.c): See
  metrics on the CPU utilization, network traffic, volume of web clients, API responses, database engine usage, and
  more.

## Orchestrators

Plugin orchestrators organize and run many of the above collectors.

If you're interested in developing a new collector that you'd like to contribute to Netdata, we highly recommend using
the `go.d.plugin`.

-   [go.d.plugin](https://github.com/netdata/go.d.plugin): An orchestrator for data collection modules written in `go`.
-   [python.d.plugin](python.d.plugin/README.md): An orchestrator for data collection modules written in `python` v2/v3.
-   [charts.d.plugin](charts.d.plugin/README.md): An orchestrator for data collection modules written in `bash` v4+.

## Third-party collectors

These collectors are developed and maintained by third parties and, unlike the other collectors, are not installed by
default. To use a third-party collector, visit their GitHub/documentation page and follow their installation procedures.

- [CyberPower UPS](https://github.com/HawtDogFlvrWtr/netdata_cyberpwrups_plugin): Polls CyberPower UPS data using
  PowerPanel® Personal Linux.
- [Logged-in users](https://github.com/veksh/netdata-numsessions): Collect the number of currently logged-on users.
- [nextcloud](https://github.com/arnowelzel/netdata-nextcloud): Monitor Nextcloud servers.
- [nim-netdata-plugin](https://github.com/FedericoCeratto/nim-netdata-plugin): A helper to create native Netdata
  plugins using Nim.
- [Nvidia GPUs](https://github.com/coraxx/netdata_nv_plugin): Monitor Nvidia GPUs.
- [Teamspeak 3](https://github.com/coraxx/netdata_ts3_plugin): Pulls active users and bandwidth from TeamSpeak 3
  servers.
- [SSH](https://github.com/Yaser-Amiri/netdata-ssh-module): Monitor failed authentication requests of an SSH server.

## Etc

- [checks.plugin](checks.plugin/README.md): A debugging collector, disabled by default.
- [charts.d example](charts.d.plugin/example/README.md): An example `charts.d` collector.
- [python.d example](python.d.plugin/example/README.md): An example `python.d` collector.
