<!--
title: "Supported collectors list"
description: "Netdata gathers real-time metrics from hundreds of data sources using collectors. Most require zero configuration and are pre-configured out of the box."
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/COLLECTORS.md
-->

# Supported collectors list

Netdata uses collectors to help you gather metrics from your favorite applications and services and view them in
real-time, interactive charts. The following list includes collectors for both external services/applications and
internal system metrics.

Read more about collectors and how to enable them in our [collectors documentation](/collectors/README.md), or use the
[collector quickstart](/collectors/QUICKSTART.md) to figure out how to collect metrics from your favorite app/service
with auto-detection and minimal configuration.

Some collectors have both Go and Python versions. While the Go verisons are newer, they are most often disabled by
default as we continue our effort to make them enabled by default. See the [Go plugin
documentation](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/#why-disabled-how-to-enable) for details on
how to disable the Python collector and enable the Go equivalent.

If you don't see the app/service you'd like to monitor here, check out our [GitHub
issues](https://github.com/netdata/netdata/issues). Use the search bar to look for previous discussions about that
collector—we may be looking for contributions from users such as yourself!

-   [Service and application collectors](#service-and-application-collectors)
    -   [APM (application performance monitoring)](#apm-application-performance-monitoring)
    -   [Containers and VMs](#containers-and-vms)
    -   [Data stores](#data-stores)
    -   [Distributed computing](#distributed-computing)
    -   [Email](#email)
    -   [Kubernetes](#kubernetes)
    -   [Logs](#logs)
    -   [Messaging](#messaging)
    -   [Network](#network)
    -   [Provisioning](#provisioning)
    -   [Remote devices](#remote-devices)
    -   [Search](#search)
    -   [Storage](#storage)
    -   [Web](#web)
-   [System collectors](#system-collectors)
    -   [Applications](#applications)
    -   [Disks and filesystems](#disks-and-filesystems)
    -   [eBPF (extended Berkely Backet Filter)](#ebpf)
    -   [Hardware](#hardware)
    -   [Memory](#memory)
    -   [Networks](#networks)
    -   [Processes](#processes)
    -   [Resources](#resources)
    -   [Users](#users)
-   [Third-party collectors](#third-party-collectors)
-   [Etc](#etc)

## Service and application collectors

The Netdata Agent auto-detects and collects metrics from all of the services and applications below. You can also
configure any of these collectors according to your setup and infrastructure.

### APM (application performance monitoring)

-   [Go applications](/collectors/python.d.plugin/go_expvar/README.md): Monitors any Go application that exposes its
    metrics with the  `expvar` package from the Go standard library.
-   [Java Spring Boot 2
    applications](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/springboot2/) (Go version):
    Monitors running Java Spring Boot 2 applications that expose their metrics with the use of the Spring Boot Actuator.
-   [Java Spring Boot 2 applications](/collectors/python.d.plugin/springboot/README.md) (Python version): Monitors
    running Java Spring Boot applications that expose their metrics with the use of the Spring Boot Actuator.
-   [statsd](/collectors/statsd.plugin/README.md): Implements a high performance `statsd` server for Netdata.
-   [phpDaemon](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/phpdaemon/): Collects worker
    statistics (total, active, idle), and uptime for web and network applications.
-   [uWSGI](/collectors/python.d.plugin/uwsgi/README.md): Monitors performance metrics exposed by the uWSGI Stats
    Server.

### Containers and VMs

-   [Docker containers](/collectors/cgroups.plugin/README.md): Monitor the health and performance of individual Docker
    containers using the cgroups collector plugin.
-   [DockerD](/collectors/python.d.plugin/dockerd/README.md): Collects container health statistics.
-   [Docker Engine](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/docker_engine/): Collects
    runtime statistics from the `docker` daemon using the `metrics-address` feature.
-   [Docker Hub](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/dockerhub/): Collects statistics
    about Docker repositories, such as pulls, starts, status, time since last update, and more.
-   [Libvirt](/collectors/cgroups.plugin/README.md): Monitor the health and performance of individual Libvirt containers
    using the cgroups collector plugin.
-   [LXC](/collectors/cgroups.plugin/README.md): Monitor the health and performance of individual LXC containers using
    the cgroups collector plugin.
-   [LXD](/collectors/cgroups.plugin/README.md): Monitor the health and performance of individual LXD containers using
    the cgroups collector plugin.
-   [systemd-nspawn](/collectors/cgroups.plugin/README.md): Monitor the health and performance of individual
    systemd-nspawn containers using the cgroups collector plugin.
-   [vCenter Server Appliance](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/vcsa/): Monitors
    appliance system, components, and software update health statuses via the Health API.
-   [vSphere](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/vsphere/): Collects host and virtual
    machine performance metrics.
-   [Xen/XCP-ng](/collectors/xenstat.plugin/README.md): Collects XenServer and XCP-ng metrics using `libxenstat`.

### Data stores

-   [CockroachDB](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/cockroachdb/): Monitors various
    database components using `_status/vars` endpoint.
-   [Consul](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/consul/): Reports service and unbound
    checks status (passing, warning, critical, maintenance). 
-   [CouchDB](/collectors/python.d.plugin/couchdb/README.md): Monitors database health and performance metrics
    (reads/writes, HTTP traffic, replication status, etc).
-   [MongoDB](/collectors/python.d.plugin/mongodb/README.md): Collects memory-caching system performance metrics and
    reads the server's response to `stats` command (stats interface).
-   [MySQL](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/mysql/): Collects database global,
    replication and per user statistics.
-   [OracleDB](/collectors/python.d.plugin/oracledb/README.md): Monitors database performance and health metrics.
-   [Postgres](/collectors/python.d.plugin/postgres/README.md): Collects database health and performance metrics. 
-   [ProxySQL](/collectors/python.d.plugin/proxysql/README.md): Monitors database backend and frontend performance
    metrics.
-   [Redis](/collectors/python.d.plugin/redis/): Monitors database status by reading the server's response to the `INFO`
    command.
-   [RethinkDB](/collectors/python.d.plugin/rethinkdbs/README.md): Collects database server and cluster statistics.
-   [Riak KV](/collectors/python.d.plugin/riakkv/README.md): Collects database stats from the `/stats` endpoint.
-   [Zookeeper](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/zookeeper/): Monitors application
    health metrics reading the server's response to the `mntr` command.

### Distributed computing

-   [BOINC](/collectors/python.d.plugin/boinc/README.md): Monitors the total number of tasks, open tasks, and task
    states for the distributed computing client.
-   [Gearman](/collectors/python.d.plugin/gearman/README.md): Collects application summary (queued, running) and per-job
    worker statistics (queued, idle, running).

### Email

-   [Dovecot](/collectors/python.d.plugin/dovecot/README.md): Collects email server performance metrics by reading the
    server's response to the `EXPORT global` command.
-   [EXIM](/collectors/python.d.plugin/exim/README.md): Uses the `exim` tool to monitor the queue length of a
    mail/message transfer agent (MTA).
-   [Postfix](/collectors/python.d.plugin/postfix/README.md): Uses the `postqueue` tool to monitor the queue length of a
    mail/message transfer agent (MTA).

### Kubernetes

-   [Kubelet](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/k8s_kubelet/): Monitors one or more
    instances of the Kubelet agent and collects metrics on number of pods/containers running, volume of Docker
    operations, and more.
-   [kube-proxy](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/k8s_kubeproxy/): Collects
    metrics, such as syncing proxy rules and REST client requests, from one or more instances of `kube-proxy`.
-   [Service discovery](https://github.com/netdata/agent-service-discovery/): Finds what services are running on a
    cluster's pods, converts that into configuration files, and exports them so they can be monitored by Netdata.

### Logs

-   [Fluentd](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/fluentd/): Gathers application
    plugins metrics from an endpoint provided by `in_monitor plugin`.
-   [Logstash](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/logstash/): Monitors JVM threads,
    memory usage, garbage collection statistics, and more.
-   [OpenVPN status logs](/collectors/python.d.plugin/ovpn_status_log/): Parses server log files and provides summary
    (client, traffic) metrics.
-   [Web server logs (Go version for Apache,
    NGINX)](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/weblog/): Tails access logs and
    provides very detailed web server performance statistics. This module is able to parse 200k+ rows in less than half
    a second.
-   [Web server logs (Python version for Apache, NGINX, Squid)](/collectors/python.d.plugin/web_log/): Tails access log
    file and collects web server/caching proxy metrics.

### Messaging

-   [ActiveMQ](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/activemq/): Collects message broker
    queues and topics statistics using the ActiveMQ Console API.
-   [Beanstalk](/collectors/python.d.plugin/beanstalk/README.md): Collects server and tube-level statistics, such as CPU
    usage, jobs rates, commands, and more.
-   [Pulsar](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/pulsar/): Collects summary,
    namespaces, and topics performance statistics.
-   [RabbitMQ (Go)](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/rabbitmq/): Collects message
    broker overview, system and per virtual host metrics.
-   [RabbitMQ (Python)](/collectors/python.d.plugin/rabbitmq/README.md): Collects message broker global and per virtual
    host metrics. 
-   [VerneMQ](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/vernemq/): Monitors MQTT broker
    health and performance metrics. It collects all available info for both MQTTv3 and v5 communication

### Network

-   [Bind 9](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/bind/): Collects nameserver summary
    performance statistics via a web interface (`statistics-channels` feature).
-   [Chrony](/collectors/python.d.plugin/chrony/README.md): Monitors the precision and statistics of a local `chronyd`
    server.
-   [CoreDNS](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/coredns/): Measures DNS query round
    trip time.
-   [Dnsmasq](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/dnsmasq_dhcp/): Automatically
    detects all configured `Dnsmasq` DHCP ranges and Monitors their utilization.
-   [dnsdist](/collectors/python.d.plugin/dnsdist/README.md): Collects load-balancer performance and health metrics.
-   [dns_query](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/dnsquery/): Monitors the round
    trip time for DNS queries in milliseconds.
-   [DNS Query Time](/collectors/python.d.plugin/dns_query_time/README.md): Measures DNS query round trip time.
-   [Freeradius (Go)](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/freeradius/): Collects
    server authentication and accounting statistics from the `status server`.
-   [Freeradius (Python)](/collectors/python.d.plugin/freeradius/README.md): Collects server authentication and
    accounting statistics from the `status server` using the `radclient` tool.
-   [Libreswan](/collectors/charts.d.plugin/libreswan/): Collects bytes-in, bytes-out, and uptime metrics.
-   [Icecast](/collectors/python.d.plugin/icecast/README.md): Monitors the number of listeners for active sources.
-   [ISC BIND](/collectors/node.d.plugin/named/README.md): Collects nameserver summary performance statistics via a web
    interface (`statistics-channels` feature).
-   [ISC Bind (RDNC)](/collectors/python.d.plugin/bind_rndc/README.md): Collects nameserver summary performance
    statistics using the `rndc` tool.
-   [ISC DHCP](/collectors/python.d.plugin/isc_dhcpd/README.md): Reads `dhcpd.leases` file and reports DHCP pools
    utiliation and leases statistics (total number, leases per pool).
-   [OpenLDAP](/collectors/python.d.plugin/openldap/README.md): Provides statistics information from the OpenLDAP
    (`slapd`) server.
-   [NSD](/collectors/python.d.plugin/nsd/README.md): Monitors nameserver performance metrics using the `nsd-control`
    tool.
-   [NTP daemon](/collectors/python.d.plugin/ntpd/README.md): Monitors the system variables of the local `ntpd` daemon
    (optionally including variables of the polled peers) using the NTP Control Message Protocol via a UDP socket.
-   [OpenSIPS](/collectors/charts.d.plugin/opensips/README.md): Collects server health and performance metrics using the
    `opensipsctl` tool.
-   [OpenVPN](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/openvpn/): Gathers server summary
    (client, traffic) and per user metrics (traffic, connection time) stats using `management-interface`.
-   [Pi-hole](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/pihole/): Monitors basic (DNS
    queries, clients, blocklist) and extended (top clients, top permitted, and blocked domains) statistics using the PHP
    API.
-   [PowerDNS](/collectors/python.d.plugin/powerdns/README.md): Monitors authoritative server and recursor statistics.
-   [RetroShare](/collectors/python.d.plugin/retroshare/README.md): Monitors application bandwidth, peers, and DHT
    metrics.
-   [Tor](/collectors/python.d.plugin/tor/README.md): Reports traffic usage statistics using the Tor control port.
-   [Unbound](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/unbound/): Collects DNS resolver
    summary and extended system and per thread metrics via the `remote-control` interface.

### Provisioning

-   [Puppet](/collectors/python.d.plugin/puppet/README.md): Monitors the status of Puppet Server and Puppet DB.

### Remote devices

-   [AM2320](/collectors/python.d.plugin/am2320/README.md): Monitors sensor temperature and humidity.
-   [Access point](/collectors/charts.d.plugin/ap/README.md): Monitors client, traffic and signal metrics using the `aw`
    tool.
-   [APC UPS](/collectors/charts.d.plugin/apcupsd/README.md): Retrieves status information using the `apcaccess` tool.
-   [Energi Core](/collectors/python.d.plugin/energid/README.md): Monitors blockchain, memory, network, and unspent
    transactions statistics.
-   [Fronius Symo](/collectors/node.d.plugin/fronius/): Collects power, consumption, autonomy, energy, and inverter
    statistics.
-   [UPS/PDU](/collectors/charts.d.plugin/nut/README.md): Polls the status of UPS/PDU devices using the `upsc` tool.
-   [SMA Sunny WebBox](/collectors/node.d.plugin/sma_webbox/README.md): Collects power statistics.
-   [SNMP devices](/collectors/node.d.plugin/snmp/README.md): Gathers data using the SNMP protocol.
-   [Stiebel Eltron ISG](/collectors/node.d.plugin/stiebeleltron/README.md): Collects metrics from heat pump and hot
    water installations.
-   [1-Wire sensors](/collectors/python.d.plugin/w1sensor/README.md): Monitors sensor temperature.

### Search

-   [ElasticSearch](/collectors/python.d.plugin/elasticsearch/README.md): Collects search engine performance and health
    statistics. Optionally collects per-index metrics.
-   [Solr](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/solr/): Collects application search
    requests, search errors, update requests, and update errors statistics.

### Storage

-   [Ceph](/collectors/python.d.plugin/ceph/README.md): Monitors the Ceph cluster usage and server data consumption.
-   [HDFS](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/hdfs/): Monitors health and performance
    metrics for filesystem datanodes and namenodes.
-   [IPFS](/collectors/python.d.plugin/ipfs/README.md): Collects file system bandwidth, peers, and repo metrics.
-   [Scaleio](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/scaleio/): Monitors storage system,
    storage pools, and SDCS health and performance metrics via VxFlex OS Gateway API.
-   [Samba](/collectors/python.d.plugin/samba/README.md): Collects file sharing metrics using the `smbstatus` tool.

### Web

-   [Apache (Go)](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/apache/): Collects Apache web
    server performance metrics via the `server-status?auto` endpoint.
-   [Apache (Python)](/collectors/python.d.plugin/apache/README.md): Collects Apache web server performance metrics via
    the `server-status?auto` endpoint.
-   [HAProxy](/collectors/python.d.plugin/haproxy/README.md): Collects frontend, backend, and health metrics.
-   [HTTP endpoints (Go)](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/httpcheck/): Monitors
    any HTTP endpoint's availability and response time.
-   [HTTP endpoints (Python)](/collectors/python.d.plugin/httpcheck/README.md): Monitors any HTTP endpoint's
    availability and response time.
-   [Lighttpd](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/lighttpd/): Collects web server
    performance metrics using the `server-status?auto` endpoint.
-   [Lighttpd2](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/lighttpd2/): Collects web server
    performance metrics using the `server-status?format=plain` endpoint.
-   [Litespeed](/collectors/python.d.plugin/litespeed/README.md): Collects web server data (network, connection,
    requests, cache) by reading `.rtreport*` files.
-   [Nginx (Go)](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/nginx/): Monitors web server
    status information by gathering metrics via `ngx_http_stub_status_module`.
-   [Nginx (Python)](/collectors/python.d.plugin/nginx/README.md): Monitors web server status information by gathering
    metrics via `ngx_http_stub_status_module`.
-   [Nginx Plus](/collectors/python.d.plugin/nginx_plus/README.md): Collects global and per-server zone, upstream, and
    cache metrics.
-   [PHP-FPM (Go)](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/phpfpm/): Collects application
    summary and processes health metrics by scraping the status page (`/status?full`).
-   [PHP-FPM (Python)](/collectors/python.d.plugin/phpfpm/README.md): Collects application summary and processes health
    metrics by scraping the status page (`/status?full`).
-   [TCP endpoints (Go)](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/portcheck/): Monitors any TCP
    endpoint's availability and response time.
-   [TCP endpoints (Python)](/collectors/python.d.plugin/portcheck/README.md): Monitors any TCP endpoint's availability
    and response time.
-   [Spigot Minecraft servers](/collectors/python.d.plugin/spigotmc/README.md): Monitors average ticket rate and number
    of users.
-   [Squid](/collectors/python.d.plugin/squid/README.md): Monitors client and server bandwidth/requests by gathering
    data from the Cache Manager component.
-   [Tengine](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/tengine/): Monitors web server
    statistics using information provided by `ngx_http_reqstat_module`.
-   [Tomcat](/collectors/python.d.plugin/tomcat/README.md): Collects web server performance metrics from the Manager App
    (`/manager/status?XML=true`).
-   [Traefik](/collectors/python.d.plugin/traefik/README.md): Uses Trafik's Health API to provide statistics.
-   [Varnish](/collectors/python.d.plugin/varnish/README.md): Provides HTTP accelerator global, backends (VBE), and
    disks (SMF) statistics using the `varnishstat` tool.
-   [x509 check](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/x509check/): Monitors certificate
    expiration time.
-   [Whois domain expiry](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/whoisquery/): Checks the
    remaining time until a given domain is expired.

## System collectors

The Netdata Agent can collect these system- and hardware-level metrics using a variety of collectors, some of which
(such as `proc.plugin`) collect multiple types of metrics simultaneously.

### Applications

-   [Fail2ban](/collectors/python.d.plugin/fail2ban/README.md): Parses configuration files to detect all jails, then
    uses log files to report ban rates and volume of banned IPs.
-   [Monit](/collectors/python.d.plugin/monit/README.md): Monitors statuses of targets (service-checks) using the XML
    stats interface.
-   [WMI (Windows Management Instrumentation)
    exporter](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/wmi/): Collects CPU, memory,
    network, disk, OS, system, and log-in metrics scraping `wmi_exporter`.

### Disks and filesystems

-   [BCACHE](/collectors/proc.plugin/README.md): Monitors BCACHE statistics with the proc collector.
-   [Block devices](/collectors/proc.plugin/README.md): Gathers metrics about the health and performance of block
    devices using the proc collector.
-   [Btrfs](/collectors/proc.plugin/README.md): Montiors Btrfs filesystems with the proc collector.
-   [Device mapper](/collectors/proc.plugin/README.md): Gathers metrics about the Linux device mapper with the proc
    collector.
-   [ioping.plugin](/collectors/ioping.plugin/README.md): Measures disk read/write latency.
-   [Disk space](/collectors/diskspace.plugin/README.md): Collects disk space usage metrics on Linux mount points.
-   [NFS file servers and clients](/collectors/proc.plugin/README.md): Gathers operations, utilization, and space usage
    using the proc collector.
-   [RAID arrays](/collectors/proc.plugin/README.md): Collects health, disk status, operation status, and more with the
    proc collector.
-   [Veritas Volume Manager](/collectors/proc.plugin/README.md): Gathers metrics about the Veritas Volume Manager (VVM).
-   [ZFS](/collectors/proc.plugin/README.md): Monitors bandwidth and utilization of ZFS disks/partitions using the proc
    collector.

### eBPF

-   [Files](/collectors/ebpf_process.plugin/README.md): Provides information about how often a system calls kernel
    functions related to file descriptors using the eBPF collector.
-   [Virtual file system (VFS)](/collectors/ebpf_process.plugin/README.md): Monitors IO, errors, deleted objects, and
    more for kernel virtual file systems (VFS) using the eBPF collector.
-   [Processes](/collectors/ebpf_process.plugin/README.md/): Monitors threads, task exits, and errors using the eBPF
    collector.

### Hardware

-   [Adaptec RAID](/collectors/python.d.plugin/adaptec_raid/README.md): Monitors logical and physical devices health
    metrics using the `arcconf` tool. 
-   [CUPS](/collectors/cups.plugin/README.md): Monitors CUPS.
-   [FreeIPMI](/collectors/freeipmi.plugin/README.md): Uses `libipmimonitoring-dev` or `libipmimonitoring-devel` to
    monitor the number of sensors, temperatures, voltages, currents, and more.
-   [Hard drive temperature](/collectors/python.d.plugin/hddtemp/README.md): Monitors the temperature of storage
    devices.
-   [HP Smart Storage Arrays](/collectors/python.d.plugin/hpssa/README.md): Monitors controller, cache module, logical
    and physical drive state, and temperature using the `ssacli` tool.
-   [MegaRAID controllers](/collectors/python.d.plugin/megacli/README.md): Collects adapter, physical drives, and
    battery stats using the `megacli` tool.
-   [NVIDIA GPU](/collectors/python.d.plugin/nvidia_smi/README.md): Monitors performance metrics (memory usage, fan
    speed, pcie bandwidth utilization, temperature, and more) using the `nvidia-smi` tool.
-   [Sensors](/collectors/python.d.plugin/sensors/README.md): Reads system sensors information (temperature, voltage,
    electric current, power, and more) from `/sys/devices/`.
-   [S.M.A.R.T](/collectors/python.d.plugin/smartd_log/README.md): Reads SMART Disk Monitoring daemon logs.

### Memory

-   [Available memory](/collectors/proc.plugin/README.md): Tracks changes in available RAM using the proc collector.
-   [Committed memory](/collectors/proc.plugin/README.md): Monitors committed memory using the proc collector.
-   [Huge pages](/collectors/proc.plugin/README.md): Gathers 
-   [KSM](/collectors/proc.plugin/README.md): 
-   [Memcached](/collectors/python.d.plugin/memcached/README.md): 
-   [Numa](/collectors/proc.plugin/README.md): 
-   [Page faults](/collectors/proc.plugin/README.md): 
-   [RAM](/collectors/proc.plugin/README.md): 
-   [SLAB](/collectors/slabinfo.plugin/README.md): Collects kernel SLAB details on Linux systems.
-   [swap](/collectors/proc.plugin/README.md): 
-   [Writeback memory](/collectors/proc.plugin/README.md): 

### Networks

-   [Access points](/collectors/charts.d.plugin/ap/README.md): Visualizes data related to access points.
-   [fping.plugin](fping.plugin/README.md): Measures network latency, jitter and packet loss between the monitored node
    and any number of remote network end points.
-   [Netfilter](/collectors/nfacct.plugin/README.md): Collects netfilter firewall, connection tracker, and accounting
    metrics using `libmnl` and `libnetfilter_acct`.
-   [Network stack](/collectors/proc.plugin/README.md): 
-   [Network QoS](/collectors/tc.plugin/README.md): Collects traffic QoS metrics (`tc`) of Linux network interfaces.
-   [SYNPROXY](/collectors/proc.plugin/README.md): 

### Operating systems

-   [freebsd.plugin](freebsd.plugin/README.md): Collects resource usage and performance data on FreeBSD systems.
-   [macOS](/collectors/macos.plugin/README.md): Collects resource usage and performance data on macOS systems.

### Processes

-   [Applications](/collectors/apps.plugin/README.md): 
-   [systemd](/collectors/cgroups.plugin/README.md): 
-   [System processes](/collectors/proc.plugin/README.md): 

### Resources

-   [CPU frequency](/collectors/proc.plugin/README.md): 
-   [CPU idle](/collectors/proc.plugin/README.md): 
-   [CPU performance](/collectors/perf.plugin/README.md): Collects CPU performance metrics using performance monitoring
    units (PMU).
-   [CPU throttling](/collectors/proc.plugin/README.md): 
-   [CPU utilization](/collectors/proc.plugin/README.md): 
-   [Entropy](/collectors/proc.plugin/README.md): 
-   [Interprocess Communication](/collectors/proc.plugin/README.md): 
-   [Interrupts](/collectors/proc.plugin/README.md): 
-   [IdleJitter](/collectors/idlejitter.plugin/README.md): Measures CPU latency and jitter on all operating systems.
-   [SoftIRQs](/collectors/proc.plugin/README.md): 
-   [SoftNet](/collectors/proc.plugin/README.md): 

### Users

-   [systemd-logind](/collectors/python.d.plugin/logind/README.md): Monitors active sessions, users, and seats tracked
    by `systemd-logind` or `elogind`.
-   [User/group usage](/collectors/apps.plugin/README.md): 

## Third-party collectors

These collectors are developed and maintined by third parties and, unlike the other collectors, are not installed by
default. To use a third-party collector, visit their GitHub/documentation page and follow their installation procedures.

-   [CyberPower UPS](https://github.com/HawtDogFlvrWtr/netdata_cyberpwrups_plugin): Polls Cyberpower UPS data using
    PowerPanel® Personal Linux.
-   [Logged-in users](https://github.com/veksh/netdata-numsessions): Collects the number of currently logged-on users.
-   [nim-netdata-plugin](https://github.com/FedericoCeratto/nim-netdata-plugin): A helper to create native Netdata
    plugins using Nim.
-   [Nvidia GPUs](https://github.com/coraxx/netdata_nv_plugin): Monitors Nvidia GPUs.
-   [Teamspeak 3](https://github.com/coraxx/netdata_ts3_plugin): Plls active users and bandwidth from TeamSpeak 3
    servers.
-   [SSH](https://github.com/Yaser-Amiri/netdata-ssh-module): Monitors failed authentication requests of an SSH server.

## Etc

-   [checks.plugin](checks.plugin/README.md): A debugging collector, disabled by default.
-   [charts.d example](charts.d.plugin/example/README.md): An example `charts.d` collector.
-   [python.d example](python.d.plugin/example/README.md): An example `python.d` collector.

## Orchestrators

-   [charts.d.plugin](charts.d.plugin/README.md): An orchestrator for data collection modules written in `bash` v4+.
-   [go.d.plugin](https://github.com/netdata/go.d.plugin): An orchestrator for data collection modules written in `go`.
-   [node.d.plugin](node.d.plugin/README.md): An orchestrator for data collection modules written in `node.js`.
-   [python.d.plugin](python.d.plugin/README.md): An orchestrator for data collection modules written in `python` v2/v3.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2FCOLLECTORS&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)