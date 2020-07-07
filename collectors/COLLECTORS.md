<!--
title: "Supported collectors list"
description: "Netdata supports 
custom_edit_url](https://github.com/netdata/netdata/edit/master/collectors/COLLECTORS.md
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

-   [Dovecot](/collectors/python.d.plugin/dovecot/README.md): 
-   [EXIM](/collectors/python.d.plugin/exim/README.md): 
-   [Postfix](/collectors/python.d.plugin/postfix/README.md): 

### Kubernetes

-   [Kubelet](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/k8s_kubelet/): 
-   [kube-proxy](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/k8s_kubeproxy/): 
-   [Service discovery](https://github.com/netdata/agent-service-discovery/): Finds what services are running on a
    cluster's pods, converts that into configuration files, and exports them so they can be monitored by Netdata.

### Logs

-   [Fluentd](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/fluentd/): 
-   [Logstash](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/logstash/): 
-   [OpenVPN status logs](/collectors/python.d.plugin/ovpn_status_log/): 
-   [Web server logs (Apache, NGINX)](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/weblog/): 
-   [Web server logs (Apache, NGINX, Squid)](/collectors/python.d.plugin/web_log/): 

### Messaging

-   [ActiveMQ](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/activemq/): Collects message broker
    queues and topics statistics using the ActiveMQ Console API.
-   [Pulsar](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/pulsar/): 
-   [Beanstalk](/collectors/python.d.plugin/beanstalk/README.md): 
-   [RabbitMQ (Go)](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/rabbitmq/): 
-   [RabbitMQ (Python)](/collectors/python.d.plugin/rabbitmq/README.md): 
-   [VerneMQ](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/vernemq/): 

### Network

-   [Bind 9](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/bind/): Collects nameserver summary
    performance statistics via a web interface (`statistics-channels` feature).
-   [Chrony](/collectors/python.d.plugin/chrony/README.md): 
-   [CoreDNS](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/coredns/): 
-   [Dnsmasq](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/dnsmasq_dhcp/): 
-   [dnsdist](/collectors/python.d.plugin/dnsdist/README.md): 
-   [dns_query](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/dnsquery/): 
-   [DNS Query Time](/collectors/python.d.plugin/dns_query_time/README.md): 
-   [Freeradius (Go)](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/freeradius/): 
-   [Freeradius (Python)](/collectors/python.d.plugin/freeradius/README.md): 
-   [Libreswan](/collectors/charts.d.plugin/libreswan/): Collects bytes-in, bytes-out, and uptime metrics.
-   [Icecast](/collectors/python.d.plugin/icecast/README.md): 
-   [ISC BIND](/collectors/node.d.plugin/named/README.md): Collects nameserver summary performance statistics via a web
    interface (`statistics-channels` feature).
-   [ISC Bind (RDNC)](/collectors/python.d.plugin/bind_rndc/README.md): 
-   [ISC DHCP](/collectors/python.d.plugin/isc_dhcpd/README.md): 
-   [OpenLDAP](/collectors/python.d.plugin/openldap/README.md): 
-   [NSD](/collectors/python.d.plugin/nsd/README.md): 
-   [NTP daemon](/collectors/python.d.plugin/ntpd/README.md): 
-   [OpenSIPS](/collectors/charts.d.plugin/opensips/README.md): Collects server health and performance metrics using the
    `opensipsctl` tool.
-   [OpenVPN](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/openvpn/): 
-   [Pi-hole](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/pihole/): 
-   [PowerDNS](/collectors/python.d.plugin/powerdns/README.md): 
-   [Tor](/collectors/python.d.plugin/tor/README.md): 
-   [Unbound](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/unbound/): 

### Provisioning

-   [Puppet](/collectors/python.d.plugin/puppet/README.md): 

### Remote devices

-   [AM2320](/collectors/python.d.plugin/am2320/README.md): 
-   [Access point](/collectors/charts.d.plugin/ap/README.md): Monitors client, traffic and signal metrics using the `aw`
    tool.
-   [APC UPS](/collectors/charts.d.plugin/apcupsd/README.md): Retrieves status information using the `apcaccess` tool.
-   [Energi Core](/collectors/python.d.plugin/energid/README.md): 
-   [Fronius Symo](/collectors/node.d.plugin/fronius/): Collects power, consumption, autonomy, energy, and inverter
    statistics.
-   [UPS/PDU](/collectors/charts.d.plugin/nut/README.md): Polls the status of UPS/PDU devices using the `upsc` tool.
-   [SMA Sunny WebBox](/collectors/node.d.plugin/sma_webbox/README.md): Collects power statistics.
-   [SNMP devices](/collectors/node.d.plugin/snmp/README.md): Gathers data using the SNMP protocol.
-   [Stiebel Eltron ISG](/collectors/node.d.plugin/stiebeleltron/README.md): Collects metrics from heat pump and hot
    water installations.
-   [1-Wire sensors](/collectors/python.d.plugin/w1sensor/README.md): 

### Search

-   [ElasticSearch](/collectors/python.d.plugin/elasticsearch/README.md): 
-   [Solr](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/solr/): 

### Storage

-   [Ceph](/collectors/python.d.plugin/ceph/README.md): 
-   [HDFS](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/hdfs/): 
-   [IPFS](/collectors/python.d.plugin/ipfs/README.md): 
-   [Scaleio](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/scaleio/): 
-   [Samba](/collectors/python.d.plugin/samba/README.md): 

### Web

-   [Apache (Go)](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/apache/): Collects Apache web server performance metrics via the `server-status?auto` endpoint.
-   [Apache (Python)](/collectors/python.d.plugin/apache/README.md): Collects Apache web server performance metrics via the `server-status?auto` endpoint.
-   [HAProxy](/collectors/python.d.plugin/haproxy/README.md): 
-   [HTTP endpoints (Go)](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/httpcheck/): 
-   [HTTP endpoints (Python)](/collectors/python.d.plugin/httpcheck/README.md): 
-   [Lighttpd](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/lighttpd/): 
-   [Lighttpd2](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/lighttpd2/): 
-   [Litespeed](/collectors/python.d.plugin/litespeed/README.md): 
-   [Nginx (Go)](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/nginx/): 
-   [Nginx (Python)](/collectors/python.d.plugin/nginx/README.md): 
-   [Nginx Plus](/collectors/python.d.plugin/nginx_plus/README.md): 
-   [PHP-FPM (Go)](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/phpfpm/): 
-   [PHP-FPM (Python)](/collectors/python.d.plugin/phpfpm/README.md): 
-   [TCP endpoints](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/portcheck/): 
-   [Spigot Minecraft servers](/collectors/python.d.plugin/spigotmc/README.md): 
-   [Squid](/collectors/python.d.plugin/squid/README.md): 
-   [Tengine](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/tengine/): 
-   [Tomcat](/collectors/python.d.plugin/tomcat/README.md): 
-   [Traefik](/collectors/python.d.plugin/traefik/README.md): 
-   [Varnish](/collectors/python.d.plugin/varnish/README.md): 
-   [x509 check](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/x509check/): 
-   [Whois domain expiry](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/whoisquery/): 

## System collectors

The Netdata Agent can collect these system- and hardware-level metrics using a variety of collectors, some of which
(such as `proc.plugin`) collect multiple types of metrics simultaneously.

### Applications

-   [Fail2ban](/collectors/python.d.plugin/fail2ban/README.md): 
-   [Monit](/collectors/python.d.plugin/monit/README.md): 
-   [WMI (Windows Management Instrumentation)
    exporter](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/wmi/): 

### Disks and filesystems

-   [BCACHE](/collectors/proc.plugin/README.md): 
-   [Block devices](/collectors/proc.plugin/README.md): 
-   [BTRFS](/collectors/proc.plugin/README.md): 
-   [Device mapper](/collectors/proc.plugin/README.md): 
-   [ioping.plugin](ioping.plugin/README.md): Measures disk read/write latency.
-   [Disk space](/collectors/diskspace.plugin/README.md): Collects disk space usage metrics on Linux mount points.
-   [megacli](/collectors/proc.plugin/README.md): 
-   [NFS file servers and clients](/collectors/proc.plugin/README.md): 
-   [RAID arrays](/collectors/proc.plugin/README.md): 
-   [Veritas volume manager](/collectors/proc.plugin/README.md): 
-   [ZFS](/collectors/proc.plugin/README.md): 

### eBPF

-   [Files](/collectors/ebpf_process.plugin/): 
-   [Virtual file system (VFS)](/collectors/ebpf_process.plugin/): 
-   [Processes](/collectors/ebpf_process.plugin/): 

### Hardware

-   [Adaptec RAID](/collectors/python.d.plugin/adaptec_raid/): 
-   [CUPS](/collectors/cups.plugin/): Monitors CUPS.
-   [FreeIPMI](/collectors/freeipmi.plugin/): 
-   [Hard drive temperature](/collectors/python.d.plugin/hddtemp/): 
-   [HP Smart Storage Arrays](/collectors/python.d.plugin/hpssa/): 
-   [MegaRAID](/collectors/python.d.plugin/megacli/): 
-   [NVIDIA GPU](/collectors/python.d.plugin/nvidia_smi/): Monitors performance metrics (memory usage, fan speed, pcie
    bandwidth utilization, temperature, and more) using the `nvidia-smi` tool.
-   [Sensors](/collectors/python.d.plugin/sensors/): Reads system sensors information (temperature, voltage, electric
    current, power, and more) from `/sys/devices/`.
-   [S.M.A.R.T](/collectors/python.d.plugin/smartd_log/): Reads SMART Disk Monitoring daemon logs.                                                                                                                                                          |

### Memory

-   [Available memory](/collectors/proc.plugin/): 
-   [Committed memory](/collectors/proc.plugin/): 
-   [Huge pages](/collectors/proc.plugin/): 
-   [KSM](/collectors/proc.plugin/): 
-   [Memcached](/collectors/python.d.plugin/memcached/): 
-   [Numa](/collectors/proc.plugin/): 
-   [Page faults](/collectors/proc.plugin/): 
-   [RAM](/collectors/proc.plugin/): 
-   [SLAB](/collectors/slabinfo.plugin/): Collects kernel SLAB details on Linux systems.
-   [swap](/collectors/proc.plugin/): 
-   [Writeback memory](/collectors/proc.plugin/): 

### Networks

-   [Access points](/collectors/charts.d.plugin/ap/): 
-   [fping.plugin](fping.plugin/README.md): Measures network latency, jitter and packet loss between the monitored node
    and any number of remote network end points.
-   [Netfilter](/collectors/nfacct.plugin/): Collects netfilter firewall, connection tracker, and accounting metrics
    using `libmnl` and `libnetfilter_acct`.
-   [Network stack](/collectors/proc.plugin/): 
-   [Network QoS](/collectors/tc.plugin/): Collects traffic QoS metrics (`tc`) of
    Linux network interfaces.
-   [SYNPROXY](/collectors/proc.plugin/): 

### Operating systems

-   [freebsd.plugin](freebsd.plugin/README.md): Collects resource usage and performance data on FreeBSD systems.
-   [macOS](/collectors/macos.plugin/): Collects resource usage and performance data on macOS systems.

### Processes

-   [Applications](/collectors/apps.plugin/): 
-   [systemd](/collectors/cgroups.plugin/): 
-   [System processes](/collectors/proc.plugin/): 

### Resources

-   [CPU frequency](/collectors/proc.plugin/): 
-   [CPU idle](/collectors/proc.plugin/): 
-   [CPU performance](/collectors/perf.plugin/): Collects CPU performance metrics using performance monitoring units
    (PMU).
-   [CPU throttling](/collectors/proc.plugin/): 
-   [CPU utilization](/collectors/proc.plugin/): 
-   [Entropy](/collectors/proc.plugin/): 
-   [Interprocess Communication](/collectors/proc.plugin/): 
-   [Interrupts](/collectors/proc.plugin/): 
-   [IdleJitter](/collectors/idlejitter.plugin/): Measures CPU latency and jitter on all operating systems.
-   [SoftIRQs](/collectors/proc.plugin/): 
-   [SoftNet](/collectors/proc.plugin/): 

### Users

-   [systemd-logind](/collectors/python.d.plugin/logind): 
-   [User/group usage](/collectors/apps.plugin/): 

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

---

| [cgroups.plugin](cgroups.plugin/README.md)       | Linux   | Collects resource usage of containers, libvirt VMs, and systemd services on Linux systems. |
| [proc.plugin](proc.plugin/README.md)             | Linux   | Collects resource usage and performance data on Linux systems.                             |
| [apps.plugin](apps.plugin/README.md)                  | Linux, FreeBSD |  |
| [charts.d.plugin](charts.d.plugin/README.md)          | any            | A plugin orchestrator for data collection modules written in `bash` v4+.                                                    |
| [go.d.plugin](https://github.com/netdata/go.d.plugin) | any            | A plugin orchestrator for data collection modules written in `go`.                                                          |
| [node.d.plugin](node.d.plugin/README.md)              | any            | A plugin orchestrator for data collection modules written in `node.js`.                                                     |
| [python.d.plugin](python.d.plugin/README.md)          | any            | A plugin orchestrator for data collection modules written in `python` v2/v3.                                                |


### Go (`go.d`)

| Name                                                         | Monitors                                                                                                                                               | Description                                                                                                                                             |
| :----------------------------------------------------------- | :----------------------------------------------------------------------------------------------------------------------------------------------------- | :-------------------------------------------------------------------------------------------------------------------------------------------------------|
| [consul](https://github.com/netdata/go.d.plugin/tree/master/modules/consul)               | [`Consul`](https://www.consul.io/)                                                                                                                     |                                                                    |
| [coredns](https://github.com/netdata/go.d.plugin/tree/master/modules/coredns)             | [`CoreDNS`](https://coredns.io/)                                                                                                                       | Collects Name server summary, per server and per zone metrics.                                                                                          |
| [dns_query](https://github.com/netdata/go.d.plugin/tree/master/modules/dnsquery)          | `DNS Query RTT`                                                                                                                                        | Measures DNS query round trip time.                                                                                                                     |
| [dnsmasq_dhcp](https://github.com/netdata/go.d.plugin/tree/master/modules/dnsmasq_dhcp)   | [`Dnsmasq`](http://www.thekelleys.org.uk/dnsmasq/doc.html)                                                                                             | Automatically detects all configured `Dnsmasq` DHCP ranges and Monitors their utilization.                                                              |
| [fluentd](https://github.com/netdata/go.d.plugin/tree/master/modules/fluentd)             | [`Fluentd`](https://www.fluentd.org/)                                                                                                                  | Gathers application plugins metrics from endpoint provided by `in_monitor plugin`.                                                                      |
| [freeradius](https://github.com/netdata/go.d.plugin/tree/master/modules/freeradius)       | [`FreeRADIUS`](https://freeradius.org/)                                                                                                                | Collects server authentication and accounting statistics from `status server`.                                                                          |
| [hdfs](https://github.com/netdata/go.d.plugin/tree/master/modules/hdfs)                   | [`HDFS`](https://hadoop.apache.org/docs/r1.2.1/hdfs_design.html)                                                                                       | Monitors file system datanodes and namenodes health and performance metrics.                                                                            |
| [httpcheck](https://github.com/netdata/go.d.plugin/tree/master/modules/httpcheck)         | `HTTP Endpoint`                                                                                                                                        | Monitors http endpoint availability and response time.                                                                                                  |
| [k8s_kubelet](https://github.com/netdata/go.d.plugin/tree/master/modules/k8s_kubelet)     | [`Kubelet`](https://kubernetes.io/docs/concepts/overview/components/#kubelet)                                                                          | Collects application health and performance metrics.                                                                                                    |
| [k8s_kubeproxy](https://github.com/netdata/go.d.plugin/tree/master/modules/k8s_kubeproxy) | [`Kube-proxy`](https://kubernetes.io/docs/concepts/overview/components/#kube-proxy)                                                                    | Collects application health and performance metrics.                                                                                                    |
| [lighttpd](https://github.com/netdata/go.d.plugin/tree/master/modules/lighttpd)           | [`Lighttpd`](https://www.lighttpd.net/)                                                                                                                | Collects web server performance metrics via `server-status?auto` endpoint.                                                                              |
| [lighttpd2](https://github.com/netdata/go.d.plugin/tree/master/modules/lighttpd2)         | [`Lighttpd2`](https://redmine.lighttpd.net/projects/lighttpd2)                                                                                         | Collects web server performance metrics via `erver-status?format=plain` endpoint.                                                                       |
| [logstash](https://github.com/netdata/go.d.plugin/tree/master/modules/logstash)           | [`Logstash`](https://www.elastic.co/logstash)                                                                                                          | Monitors application JVM memory usage ang GC statistics.                                                                                                |
| [mysql](https://github.com/netdata/go.d.plugin/tree/master/modules/mysql)                 | [`MySQL`](https://www.mysql.com/)                                                                                                                      | Collects database global and replication metrics.                                                                                                       |
| [nginx](https://github.com/netdata/go.d.plugin/tree/master/modules/nginx)                 | [`NGINX`](https://www.nginx.com/)                                                                                                                      | Monitors web server status information. Information is provided by `ngx_http_stub_status_module`.                                                       |
| [openvpn](https://github.com/netdata/go.d.plugin/tree/master/modules/openvpn)             | [`OpenVPN`](https://openvpn.net/)                                                                                                                      | Gathers server summary (client, traffic) and per user metrics (traffic, connection time) stats using `management-interface`.                            |
| [phpfpm](https://github.com/netdata/go.d.plugin/tree/master/modules/phpfpm)               | [`PHP-FPM`](https://php-fpm.org/)                                                                                                                      | Collects application summary and processes health metrics scraping status page (`/status?full`).                                                        |
| [pihole](https://github.com/netdata/go.d.plugin/tree/master/modules/pihole)               | [`Pi-hole`](https://pi-hole.net/)                                                                                                                      | Monitors basic (dns queries, clients, blocklist) and extended (top clients, top permitted and blocked domains) statistics using PHP API.                |
| [portcheck](https://github.com/netdata/go.d.plugin/tree/master/modules/portcheck)         | `TCP Endpoint`                                                                                                                                         | Monitors tcp endpoint availability and response time.                                                                                                   |
| [pulsar](https://github.com/netdata/go.d.plugin/tree/master/modules/pulsar)               | [`Apache Pulsar`](http://pulsar.apache.org/)                                                                                                           | Collects summary, namespaces and topics performance statistics.                                                                                         |
| [rabbitmq](https://github.com/netdata/go.d.plugin/tree/master/modules/rabbitmq)           | [`RabbitMQ`](https://www.rabbitmq.com/)                                                                                                                | Collects message broker overview, system and per virtual host metrics.                                                                                  |
| [scaleio](https://github.com/netdata/go.d.plugin/tree/master/modules/scaleio)             | [`Dell EMC ScaleIO`](https://www.delltechnologies.com/en-us/storage/data-storage/software-defined-storage.htm)                                         | Monitors storage system, storage pools and sdcs health and performance metrics via VxFlex OS Gateway API.                                               |
| [solr](https://github.com/netdata/go.d.plugin/tree/master/modules/solr)                   | [`Solr`](https://lucene.apache.org/solr/)                                                                                                              | Collects application search requests, search errors, update requests and update errors statistics.                                                      |
| [squidlog](https://github.com/netdata/go.d.plugin/tree/master/modules/squidlog)           | [`Squid`](http://www.squid-cache.org/)                                                                                                                 | Tails access logs and provides very detailed caching proxy performance statistics. This module is able to parse 200k+ rows for less then half a second. |
| [tengine](https://github.com/netdata/go.d.plugin/tree/master/modules/tengine)             | [`Tengine`](https://tengine.taobao.org/)                                                                                                               | Monitors web server statistics using information provided by `ngx_http_reqstat_module`.                                                                 |
| [unbound](https://github.com/netdata/go.d.plugin/tree/master/modules/unbound)             | [`Unbound`](https://nlnetlabs.nl/projects/unbound/about/)                                                                                              | Collects dns resolver summary and extended system and per thread metrics via `remote-control` interface.                                                |
| [vernemq](https://github.com/netdata/go.d.plugin/tree/master/modules/vernemq)             | [`VerneMQ`](https://vernemq.com/)                                                                                                                      | Monitors MQTT broker health and performance metrics. It collects all available info for both MQTTv3 and v5 communication.                               |
| [web_log](https://github.com/netdata/go.d.plugin/tree/master/modules/weblog)              | `Apache/NGINX`                                                                                                                                         | Tails access logs and provides very detailed web server performance statistics. This module is able to parse 200k+ rows for less then half a second.    |
| [wmi](https://github.com/netdata/go.d.plugin/tree/master/modules/wmi)                     | `Windows Machines`                                                                                                                                     | Collects cpu, memory, network, disk, os, system and logon metrics scraping `wmi_exporter`.                                                              |
| [x509check](https://github.com/netdata/go.d.plugin/tree/master/modules/x509check)         | `Digital Certificates`                                                                                                                                 | Monitors certificate expiration time.                                                                                                                   |

### Python (`python.d`)

| Name                                                         | Monitors                                                                      | Description                                                                                                                                                                                          |
| :----------------------------------------------------------- | :---------------------------------------------------------------------------- | :----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [adaptec_raid](python.d.plugin/adaptec_raid/README.md)       | `Adaptec RAID Controller`                                                     | Monitors logical and physical devices health metrics using `arcconf` tool.                                                                                                                           |
| [am2320](python.d.plugin/am2320/README.md)                   | `AM2320 Sensor`                                                               | Monitors sensor temperature and humidity                                                                                                                                                             |
| [apache](python.d.plugin/apache/README.md)                   | [`Apache`](https://httpd.apache.org/)                                         | Collects web server performance metrics via `server-status?auto` endpoint.                                                                                                                           |
| [beanstalk](python.d.plugin/beanstalk/README.md)             | [`Beanstalk`](https://beanstalkapp.com/)                                      | Collects server summary and per tube metrics                                                                                                                                                         |
| [bind_rndc](python.d.plugin/bind_rndc/README.md)             | [`ISC Bind`](https://www.isc.org/bind/)                                       | Collects Name server summary performance statistics using `rndc` tool.                                                                                                                               |
| [ceph](python.d.plugin/ceph/README.md)                       | [`CEPH`](https://ceph.io/)                                                    | Monitors the ceph cluster usage and server data consumption.                                                                                                                                         |
| [chrony](python.d.plugin/chrony/README.md)                   | [`Chrony`](https://chrony.tuxfamily.org/)                                     | Monitors the precision and statistics of a local `chronyd` server.                                                                                                                                   |
| [dns_query_time](python.d.plugin/dns_query_time/README.md)   | `DNS Query RTT`                                                               | measures DNS query round trip time.                                                                                                                                                                  |
| [dnsdist](python.d.plugin/dnsdist/README.md)                 | [`PowerDNS dnsdist`](https://dnsdist.org/)                                    | Collects load-balancer performance and health metrics.                                                                                                                                               |
| [dovecot](python.d.plugin/dovecot/README.md)                 | [`Dovecot`](https://www.dovecot.org/)                                         | Collects email server performance metrics. It reads server response to `EXPORT global` command.                                                                                                      |
| [elasticsearch](python.d.plugin/elasticsearch/README.md)     | [`Elasticseach`](https://www.elastic.co/elasticsearch)                        | Collects search engine performance and health statistics. Optionally Collects per index metrics.                                                                                                     |
| [energid](python.d.plugin/energid/README.md)                 | [`Energi Core Node`](https://github.com/energicryptocurrency/energi)          | Monitors blockchain, memory, network and unspent transactions statistics.                                                                                                                            |
| [example](python.d.plugin/example/README.md)                 | -                                                                             | just an data collector example.                                                                                                                                                                      |
| [exim](python.d.plugin/exim/README.md)                       | [`Exim`](https://www.exim.org/)                                               | reports MTA emails queue length using `exim` tool.                                                                                                                                                   |
| [fail2ban](python.d.plugin/fail2ban/README.md)               | [`Fail2ban`](https://www.fail2ban.org/wiki/index.php/Main_Page)               | parses log file and reports ban rate and number of banned IPS (since the last restart of Netdata) for every jail. It automatically detects all configured jails from `Fail2ban` configuration files. |
| [freeradius](python.d.plugin/freeradius/README.md)           | [`FreeRADIUS`](https://freeradius.org/)                                       | Collects server authentication and accounting statistics from `status server` using `radclient` tool.                                                                                                |
| [haproxy](python.d.plugin/haproxy/README.md)                 | [`Haproxy`](http://www.haproxy.org/)                                          | Collects frontend, backend and health metrics.                                                                                                                                                       |
| [hddtemp](python.d.plugin/hddtemp/README.md)                 | `HDD Temperature`                                                             | Monitors storage temperature.                                                                                                                                                                        |
| [hpssa](python.d.plugin/hpssa/README.md)                     | `HP Smart Storage Arrays`                                                     | Monitors controller, cache module, logical and physical drive state and temperature using `ssacli` tool.                                                                                             |
| [httpcheck](python.d.plugin/httpcheck/README.md)             | `HTTP Endpoint`                                                               | Monitors http endpoint availability and response time.                                                                                                                                               |
| [icecast](python.d.plugin/icecast/README.md)                 | [`Icecast`](http://icecast.org/)                                              | Monitors server number of listeners for active sources.                                                                                                                                              |
| [ipfs](python.d.plugin/ipfs/README.md)                       | [`IPFS`](https://ipfs.io/)                                                    | Collects file system bandwidth, peers and repo metrics.                                                                                                                                              |
| [isc_dhcpd](python.d.plugin/isc_dhcpd/README.md)             | [`ISC DHCP`](https://www.isc.org/dhcp/)                                       | reads `dhcpd.leases` file and reports DHCP pools utiliation and leases statistics (total number, leases per pool).                                                                                   |
| [litespeed](python.d.plugin/litespeed/README.md)             | [`LiteSpeed`](https://www.litespeedtech.com/products/litespeed-web-server)    | Collects web server data (network, connection, requests, cache) reading `.rtreport*` files.                                                                                                          |
| [logind](python.d.plugin/logind/README.md)                   | [`Systemd-Logind`](https://www.freedesktop.org/wiki/Software/systemd/logind/) | Monitors active sessions, users, and seats tracked by `systemd-logind` or `elogind`.                                                                                                                 |
| [megacli](python.d.plugin/megacli/README.md)                 | `MegaRAID Controller`                                                         | Collects adapter, physical drives and battery stats using `megacli` tool.                                                                                                                            |
| [monit](python.d.plugin/monit/README.md)                     | [`Monit`](https://mmonit.com/monit/)                                          | Monitors statuses of targets (service-checks) using XML stats interface.                                                                                                                             |
| [nginx](python.d.plugin/nginx/README.md)                     | [`NGINX`](https://www.nginx.com/)                                             | Monitors web server status information. Information is provided by `ngx_http_stub_status_module`.                                                                                                    |
| [nginx_plus](python.d.plugin/nginx_plus/README.md)           | [`NGINX Plus`](https://www.nginx.com/products/nginx/)                         | Collects web server global, and per server zone/upstream/cache metrics.                                                                                                                              |
| [nsd](python.d.plugin/nsd/README.md)                         | [`NSD`](https://www.nlnetlabs.nl/projects/nsd/about/)                         | Monitors Name server performance metrics using `nsd-control` tool.                                                                                                                                   |
| [ntpd](python.d.plugin/ntpd/README.md)                       | `NTPd`                                                                        | Monitors the system variables of the local `ntpd` daemon (optional incl. variables of the polled peers) using the NTP Control Message Protocol via UDP socket.                                       |
| [openldap](python.d.plugin/openldap/README.md)               | [`OpenLDAP`](https://www.openldap.org/)                                       | provides statistics information from openldap (slapd) server. Statistics are taken from LDAP monitoring interface.                                                                                   |
| [ovpn_status_log](python.d.plugin/ovpn_status_log/README.md) | [`OpenVPN`](https://openvpn.net/)                                             | parses server log files and provides summary (client, traffic) metrics.                                                                                                                              |
| [phpfpm](python.d.plugin/phpfpm/README.md)                   | [`PHP-FPM`](https://php-fpm.org/)                                             | Collects application summary and processes health metrics scraping status page (`/status?full`).                                                                                                     |
| [portcheck](python.d.plugin/portcheck/README.md)             | `TCP Endpoint`                                                                | Monitors tcp endpoint availability and response time.                                                                                                                                                |
| [postfix](python.d.plugin/postfix/README.md)                 | [`Postfix`](http://www.postfix.org/)                                          | Monitors MTA email queue statistics using `postqueue` tool.                                                                                                                                          |
| [powerdns](python.d.plugin/powerdns/README.md)               | [`PowerDNS`](https://www.powerdns.com/)                                       | Monitors authoritative server and recursor statistics.                                                                                                                                               |
| [puppet](python.d.plugin/puppet/README.md)                   | [`Puppet`](https://puppet.com/)                                               | Monitors status of Puppet Server and Puppet DB.                                                                                                                                                      |
| [rabbitmq](python.d.plugin/rabbitmq/README.md)               | [`RabbitMQ`](https://www.rabbitmq.com/)                                       | Collects message broker global and per virtual host metrics.                                                                                                                                         |
| [retroshare](python.d.plugin/retroshare/README.md)           | [`RetroShare`](https://retroshare.cc/)                                        | Monitors application bandwidth, peers and DHT metrics.                                                                                                                                               |
| [samba](python.d.plugin/samba/README.md)                     | [`Samba`](https://www.samba.org/)                                             | Collects file sharing metrics using `smbstatus` tool.                                                                                                                                                |
| [sensors](python.d.plugin/sensors/README.md)                 | `Linux Machines Sensors`                                                      | reads system sensors information (temperature, voltage, electric current, power, etc.).                                                                                                              |
| [spigotmc](python.d.plugin/spigotmc/README.md)               | [`SpigotMC`](https://www.spigotmc.org/)                                       | Monitors average ticket rate and number of users.                                                                                                                                                    |
| [squid](python.d.plugin/squid/README.md)                     | [`Squid`](http://www.squid-cache.org/)                                        | Monitors client and server bandwidth/requests. This module Gathers data from Cache Manager component.                                                                                                |
| [tomcat](python.d.plugin/tomcat/README.md)                   | [`Apache Tomcat`](http://tomcat.apache.org/)                                  | Collects web server performance metrics from Manager App (`/manager/status?XML=true`).                                                                                                               |
| [tor](python.d.plugin/tor/README.md)                         | [`Tor`](https://www.torproject.org/)                                          | reports traffic usage statistics. It uses `Tor` control port to gather the data.                                                                                                                     |
| [traefik](python.d.plugin/traefik/README.md)                 | [`Traefic`](https://docs.traefik.io/)                                         | uses Health API to provide statistics.                                                                                                                                                               |
| [varnish](python.d.plugin/varnish/README.md)                 | [`Varnish Cache`](https://varnish-cache.org/)                                 | provides HTTP accelerator global, backends (VBE) and disks (SMF) statistics using `varnishstat` tool.                                                                                                |
| [w1sensor](python.d.plugin/w1sensor/README.md)               | `1-Wire Sensors`                                                              | Monitors sensor temperature.                                                                                                                                                                         |
| [web_log](python.d.plugin/web_log/README.md)                 | `Apache/NGINX/Squid`                                                          | tails access log file and Collects web server/caching proxy metrics.                                                                                                                                 |
