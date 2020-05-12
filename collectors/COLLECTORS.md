<!--
title: "Supported collectors list"
date: 2020-05-12
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

-   [Go applications](/collectors/python.d.plugin/go_expvar/README.md): 
-   [Java Spring Boot 2
    applications](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/springboot2/) (Go version): 
-   [Java Spring Boot 2 applications](/collectors/python.d.plugin/springboot/README.md) (Python version): 
-   [statsd](/collectors/statsd.plugin/README.md): 
-   [phpDaemon](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/phpdaemon/): 
-   [uWSGI](/collectors/python.d.plugin/uwsgi/README.md): 

### Containers and VMs

-   [Docker containers](/collectors/cgroups.plugin/README.md): 
-   [DockerD](/collectors/python.d.plugin/dockerd/README.md): 
-   [Docker Engine](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/docker_engine/): 
-   [Docker Hub](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/dockerhub/): 
-   [Kubernetes](https://github.com/netdata/helmchart): 
-   [Kubelet](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/k8s_kubelet/): 
-   [kube-proxy](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/k8s_kubeproxy/): 
-   [Libvirt](/collectors/cgroups.plugin/README.md): 
-   [LXC](/collectors/cgroups.plugin/README.md): 
-   [LXD](/collectors/cgroups.plugin/README.md): 
-   [systemd-nspawn](/collectors/cgroups.plugin/README.md): 
-   [vCenter Server Appliance](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/vcsa/): 
-   [vSphere](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/vsphere/): 
-   [Xen/XCP-ng](/collectors/xenstat.plugin/README.md): 

### Data stores

-   [CockroachDB](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/cockroachdb/): 
-   [Consul](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/consul/): 
-   [CouchDB](/collectors/python.d.plugin/couchdb/README.md): 
-   [MongoDB](/collectors/python.d.plugin/mongodb/README.md): 
-   [MySQL](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/mysql/): 
-   [OracleDB](/collectors/python.d.plugin/oracledb/README.md): 
-   [Postgres](/collectors/python.d.plugin/postgres/README.md): 
-   [ProxySQL](/collectors/python.d.plugin/proxysql/README.md): 
-   [Redis](/collectors/python.d.plugin/redis/): 
-   [RethinkDB](/collectors/python.d.plugin/rethinkdbs/README.md): 
-   [Riak KV](/collectors/python.d.plugin/riakkv/README.md): 
-   [Zookeeper](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/zookeeper/): 

### Distributed computing

-   [BOINC](/collectors/python.d.plugin/boinc/README.md): 
-   [Gearman](/collectors/python.d.plugin/gearman/README.md): 

### Email

-   [Dovecot](/collectors/python.d.plugin/dovecot/README.md): 
-   [EXIM](/collectors/python.d.plugin/exim/README.md): 
-   [Postfix](/collectors/python.d.plugin/postfix/README.md): 

### Logs

-   [Fluentd](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/fluentd/): 
-   [Logstash](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/logstash/): 
-   [OpenVPN status logs](/collectors/python.d.plugin/ovpn_status_log/): 
-   [Web server logs (Apache, NGINX)](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/weblog/): 
-   [Web server logs (Apache, NGINX, Squid)](/collectors/python.d.plugin/web_log/): 

### Messaging

-   [ActiveMQ](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/activemq/): 
-   [Pulsar](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/pulsar/): 
-   [Beanstalk](/collectors/python.d.plugin/beanstalk/README.md): 
-   [RabbitMQ (Go)](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/rabbitmq/): 
-   [RabbitMQ (Python)](/collectors/python.d.plugin/rabbitmq/README.md): 
-   [VerneMQ](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/vernemq/): 

### Network

-   [Bind 9](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/bind/): 
-   [Chrony](/collectors/python.d.plugin/chrony/README.md): 
-   [CoreDNS](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/coredns/): 
-   [Dnsmasq](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/dnsmasq_dhcp/): 
-   [dnsdist](/collectors/python.d.plugin/dnsdist/README.md): 
-   [dns_query](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/dnsquery/): 
-   [DNS Query Time](/collectors/python.d.plugin/dns_query_time/README.md): 
-   [Freeradius (Go)](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/freeradius/): 
-   [Freeradius (Python)](/collectors/python.d.plugin/freeradius/README.md): 
-   [Libreswan](https://learn.netdata.cloud/docs/agent/collectors/charts.d.plugin/libreswan/): 
-   [Icecast](/collectors/python.d.plugin/icecast/README.md): 
-   [ISC BIND](https://learn.netdata.cloud/docs/agent/collectors/node.d.plugin/named/README.md): 
-   [ISC Bind (RDNC)](/collectors/python.d.plugin/bind_rndc/README.md): 
-   [ISC DHCP](/collectors/python.d.plugin/isc_dhcpd/README.md): 
-   [OpenLDAP](/collectors/python.d.plugin/openldap/README.md): 
-   [NSD](/collectors/python.d.plugin/nsd/README.md): 
-   [NTP daemon](/collectors/python.d.plugin/ntpd/README.md): 
-   [OpenSIPS](/collectors/charts.d.plugin/opensips/README.md): 
-   [OpenVPN](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/openvpn/): 
-   [Pi-hole](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/pihole/): 
-   [PowerDNS](/collectors/python.d.plugin/powerdns/README.md): 
-   [Tor](/collectors/python.d.plugin/tor/README.md): 
-   [Unbound](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/unbound/): 

### Provisioning

-   [Puppet](/collectors/python.d.plugin/puppet/README.md): 

### Remote devices

-   [AM2320](/collectors/python.d.plugin/am2320/README.md): 
-   [Access point](/collectors/charts.d.plugin/ap/README.md): 
-   [APC UPS](/collectors/charts.d.plugin/apcupsd/README.md): 
-   [Energi Core](/collectors/python.d.plugin/energid/README.md): 
-   [Fronius Symo](/collectors/node.d.plugin/fronius/): 
-   [UPS/PDU](/collectors/charts.d.plugin/nut/README.md): 
-   [SMA Sunny WebBox](/collectors/node.d.plugin/sma_webbox/README.md): 
-   [SNMP devices](/collectors/node.d.plugin/snmp/README.md): 
-   [Stiebel Eltron ISG](/collectors/node.d.plugin/stiebeleltron/README.md): 
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

-   [Apache (Go)](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/apache/): 
-   [Apache (Python)](/collectors/python.d.plugin/apache/README.md): 
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
-   [Disk space](/collectors/diskspace.plugin/README.md): 
-   [megacli](/collectors/proc.plugin/README.md): 
-   [NFS file servers and clients](/collectors/proc.plugin/README.md): 
-   [RAID arrays](/collectors/proc.plugin/README.md): 
-   [Veritas volume manager](/collectors/proc.plugin/README.md): 
-   [ZFS](/collectors/proc.plugin/README.md): 

### eBPF

-   [Files](https://learn.netdata.cloud/docs/agent/collectors/ebpf_process.plugin/): 
-   [Virtual file system (VFS)](https://learn.netdata.cloud/docs/agent/collectors/ebpf_process.plugin/): 
-   [Processes](https://learn.netdata.cloud/docs/agent/collectors/ebpf_process.plugin/): 

### Hardware

-   [Adaptec RAID](/collectors/python.d.plugin/adaptec_raid/): 
-   [CUPS](https://learn.netdata.cloud/docs/agent/collectors/cups.plugin/): 
-   [FreeIPMI](https://learn.netdata.cloud/docs/agent/collectors/freeipmi.plugin/): 
-   [Hard drive temperature](/collectors/python.d.plugin/hddtemp/): 
-   [HP Smart Storage Arrays](/collectors/python.d.plugin/hpssa/): 
-   [macOS](https://learn.netdata.cloud/docs/agent/collectors/macos.plugin/): 
-   [MegaRAID](/collectors/python.d.plugin/megacli/): 
-   [NVIDIA GPU](/collectors/python.d.plugin/nvidia_smi/): 
-   [Sensors](/collectors/python.d.plugin/sensors/): 
-   [S.M.A.R.T](/collectors/python.d.plugin/smartd_log/): 

### Memory

-   [Available memory](https://learn.netdata.cloud/docs/agent/collectors/proc.plugin/): 
-   [Committed memory](https://learn.netdata.cloud/docs/agent/collectors/proc.plugin/): 
-   [Huge pages](https://learn.netdata.cloud/docs/agent/collectors/proc.plugin/): 
-   [KSM](https://learn.netdata.cloud/docs/agent/collectors/proc.plugin/): 
-   [Memcached](/collectors/python.d.plugin/memcached/): 
-   [Numa](https://learn.netdata.cloud/docs/agent/collectors/proc.plugin/): 
-   [Page faults](https://learn.netdata.cloud/docs/agent/collectors/proc.plugin/): 
-   [RAM](https://learn.netdata.cloud/docs/agent/collectors/proc.plugin/): 
-   [SLAB](https://learn.netdata.cloud/docs/agent/collectors/proc.plugin/): 
-   [swap](https://learn.netdata.cloud/docs/agent/collectors/proc.plugin/): 
-   [Writeback memory](https://learn.netdata.cloud/docs/agent/collectors/proc.plugin/): 

### Networks

-   [Access points](https://learn.netdata.cloud/docs/agent/collectors/charts.d.plugin/ap/): 
-   [fping](https://learn.netdata.cloud/docs/agent/collectors/fping.plugin/): 
-   [Netfilter](https://learn.netdata.cloud/docs/agent/collectors/nfacct.plugin/): 
-   [Network stack](https://learn.netdata.cloud/docs/agent/collectors/proc.plugin/): 
-   [Network QoS](https://learn.netdata.cloud/docs/agent/collectors/tc.plugin/): 
-   [SYNPROXY](https://learn.netdata.cloud/docs/agent/collectors/proc.plugin/): 

### Processes

-   [Applications](https://learn.netdata.cloud/docs/agent/collectors/apps.plugin/): 
-   [systemd](https://learn.netdata.cloud/docs/agent/collectors/cgroups.plugin/): 
-   [System processes](https://learn.netdata.cloud/docs/agent/collectors/proc.plugin/): 

### Resources

-   [CPU frequency](https://learn.netdata.cloud/docs/agent/collectors/proc.plugin/): 
-   [CPU idle](https://learn.netdata.cloud/docs/agent/collectors/proc.plugin/): 
-   [CPU performance](https://learn.netdata.cloud/docs/agent/collectors/perf.plugin/): 
-   [CPU throttling](https://learn.netdata.cloud/docs/agent/collectors/proc.plugin/): 
-   [CPU utilization](https://learn.netdata.cloud/docs/agent/collectors/proc.plugin/): 
-   [Entropy](https://learn.netdata.cloud/docs/agent/collectors/proc.plugin/): 
-   [Interprocess Communication](https://learn.netdata.cloud/docs/agent/collectors/proc.plugin/): 
-   [Interrupts](https://learn.netdata.cloud/docs/agent/collectors/proc.plugin/): 
-   [IdleJitter](https://learn.netdata.cloud/docs/agent/collectors/idlejitter.plugin/): 
-   [SoftIRQs](https://learn.netdata.cloud/docs/agent/collectors/proc.plugin/): 
-   [SoftNet](https://learn.netdata.cloud/docs/agent/collectors/proc.plugin/): 

### Users

-   [systemd-logind](/collectors/python.d.plugin/logind): 
-   [User/group usage](https://learn.netdata.cloud/docs/agent/collectors/apps.plugin/): 

## Third-party collectors

These collectors are developed and maintined by third parties and, unlike the other collectors, are not installed by
default. To use a third-party collector, visit their GitHub/documentation page and follow their installation procedures.

-   [CyberPower UPS](https://github.com/HawtDogFlvrWtr/netdata_cyberpwrups_plugin):
-   [Logged-in users](https://github.com/veksh/netdata-numsessions): Collects the number of currently logged-on users.
-   [nim-netdata-plugin](https://github.com/FedericoCeratto/nim-netdata-plugin): A helper to create native Netdata
    plugins using Nim.
-   [Nvidia GPUs](https://github.com/coraxx/netdata_nv_plugin): Monitors nvidia GPUs. 
-   [Teamspeak 3](https://github.com/coraxx/netdata_ts3_plugin): Plls active users and bandwidth from TeamSpeak 3
    servers.
-   [SSH](https://github.com/Yaser-Amiri/netdata-ssh-module): Monitors failed authentication requests of an SSH server.

---

| plugin                                           | O/S     | Description                                                                                |
| :------------------------------------------------| :-------| :------------------------------------------------------------------------------------------|
| [cgroups.plugin](cgroups.plugin/README.md)       | Linux   | Collects resource usage of containers, libvirt VMs, and systemd services on Linux systems. |
| [checks.plugin](checks.plugin/README.md)         | any     | A debugging plugin.                                                                        |
| [diskspace.plugin](diskspace.plugin/README.md)   | Linux   | Collects disk space usage metrics on Linux mount points.                                   |
| [freebsd.plugin](freebsd.plugin/README.md)       | FreeBSD | Collects resource usage and performance data on FreeBSD systems.                           |
| [idlejitter.plugin](idlejitter.plugin/README.md) | any     | Measures CPU latency and jitter on all operating systems.                                  |
| [macos.plugin](macos.plugin/README.md)           | macos   | Collects resource usage and performance data on macOS systems.                             |
| [proc.plugin](proc.plugin/README.md)             | Linux   | Collects resource usage and performance data on Linux systems.                             |
| [slabinfo.plugin](slabinfo.plugin/README.md)     | Linux   | Collects kernel SLAB details on Linux systems.                                             |
| [statsd.plugin](statsd.plugin/README.md)         | any     | Implements a high performance `statsd` server for Netdata.                                 |
| [tc.plugin](tc.plugin/README.md)                 | Linux   | Collects traffic QoS metrics (`tc`) of Linux network interfaces.                           |
| [xenstat.plugin](xenstat.plugin/README.md)       | Linux   | Collects XenServer and XCP-ng metrics using `libxenstat`.                                  |

## External plugins

| plugin                                                | O/S            | Description                                                                                                                 |
| :-----------------------------------------------------| :------------- | :-------------------------------------------------------------------------------------------------------------------------- |
| [apps.plugin](apps.plugin/README.md)                  | Linux, FreeBSD | Monitors the whole process tree on Linux and FreeBSD and breaks down system resource usage by process, user and user group. |
| [charts.d.plugin](charts.d.plugin/README.md)          | any            | A plugin orchestrator for data collection modules written in `bash` v4+.                                                    |
| [cups.plugin](cups.plugin/README.md)                  | any            | Monitors CUPS.                                                                                                              |
| [fping.plugin](fping.plugin/README.md)                | any            | Measures network latency, jitter and packet loss between the monitored node and any number of remote network end points.    |
| [freeipmi.plugin](freeipmi.plugin/README.md)          | Linux, FreeBSD | Collects metrics from enterprise hardware sensors, on Linux and FreeBSD servers.                                            |
| [go.d.plugin](https://github.com/netdata/go.d.plugin) | any            | A plugin orchestrator for data collection modules written in `go`.                                                          |
| [ioping.plugin](ioping.plugin/README.md)              | any            | Measures disk read/write latency.                                                                                           |
| [nfacct.plugin](nfacct.plugin/README.md)              | Linux          | Collects netfilter firewall, connection tracker and accounting metrics using `libmnl` and `libnetfilter_acct`.              |
| [node.d.plugin](node.d.plugin/README.md)              | any            | A plugin orchestrator for data collection modules written in `node.js`.                                                     |
| [perf.plugin](perf.plugin/README.md)                  | Linux          | Collects CPU performance metrics using performance monitoring units (PMU).                                                  |
| [python.d.plugin](python.d.plugin/README.md)          | any            | A plugin orchestrator for data collection modules written in `python` v2/v3.                                                |

## Collector modules (via plugin orchestrators)

### Bash (`charts.d`)

| Name                                             | Monitors                                | Description                                                                                                  |
| :----------------------------------------------- | :-------------------------------------- | :----------------------------------------------------------------------------------------------------------- |
| [ap](charts.d.plugin/ap/README.md)               | `Access Points`                         | Monitors client, traffic and signal metrics using `aw` tool.                                                 |
| [apcupsd](charts.d.plugin/apcupsd/README.md)     | `APC UPSes`                             | Retrieves status information using `apcaccess` tool.                                                         |
| [example](charts.d.plugin/example/README.md)     | -                                       | -                                                                                                            |
| [libreswan](charts.d.plugin/libreswan/README.md) | `Libreswan IPSEC Tunnels`               | Collects bytes-in, bytes-out and uptime metrics.                                                             |
| [nut](charts.d.plugin/nut/README.md)             | `UPS Servers`                           | Polls the status using `upsc` tool.                                                                          |
| [opensips](charts.d.plugin/opensips/README.md)   | [`OpenSIPS`](https://www.opensips.org/) | Collects server health and performance metrics using the `opensipsctl` tool.                                 |
| [sensors](charts.d.plugin/sensors/README.md)     | `Linux Machines Sensors`                | reads system sensors information (temperature, voltage, electric current, power, etc.) from `/sys/devices/`. |

### Go (`go.d`)

| Name                                                         | Monitors                                                                                                                                               | Description                                                                                                                                             |
| :----------------------------------------------------------- | :----------------------------------------------------------------------------------------------------------------------------------------------------- | :-------------------------------------------------------------------------------------------------------------------------------------------------------|
| [activemq](https://github.com/netdata/go.d.plugin/tree/master/modules/activemq)           | [`ActiveMQ`](https://activemq.apache.org/)                                                                                                             | Collects message broker queues and topics statistics using ActiveMQ Console API.                                                                        |
| [apache](https://github.com/netdata/go.d.plugin/tree/master/modules/apache)               | [`Apache`](https://httpd.apache.org/)                                                                                                                  | Collects web server performance metrics via `server-status?auto` endpoint.                                                                              |
| [bind](https://github.com/netdata/go.d.plugin/tree/master/modules/bind/)                   | [`ISC Bind`](https://www.isc.org/bind/)                                                                                                                | Collects Name server summary performance statistics via web interface (`statistics-channels` feature).                                                  |
| [cockroachdb](https://github.com/netdata/go.d.plugin/tree/master/modules/cockroachdb)     | [`CockroachDB`](https://www.cockroachlabs.com/)                                                                                                        | Monitors various database components using `_status/vars` endpoint.                                                                                     |
| [consul](https://github.com/netdata/go.d.plugin/tree/master/modules/consul)               | [`Consul`](https://www.consul.io/)                                                                                                                     | Reports service and unbound checks status (passing, warning, critical, maintenance).                                                                    |
| [coredns](https://github.com/netdata/go.d.plugin/tree/master/modules/coredns)             | [`CoreDNS`](https://coredns.io/)                                                                                                                       | Collects Name server summary, per server and per zone metrics.                                                                                          |
| [dns_query](https://github.com/netdata/go.d.plugin/tree/master/modules/dnsquery)          | `DNS Query RTT`                                                                                                                                        | Measures DNS query round trip time.                                                                                                                     |
| [dnsmasq_dhcp](https://github.com/netdata/go.d.plugin/tree/master/modules/dnsmasq_dhcp)   | [`Dnsmasq`](http://www.thekelleys.org.uk/dnsmasq/doc.html)                                                                                             | Automatically detects all configured `Dnsmasq` DHCP ranges and Monitors their utilization.                                                              |
| [docker_engine](https://github.com/netdata/go.d.plugin/tree/master/modules/docker_engine) | [`Docker Engine`](https://docs.docker.com/engine/)                                                                                                     | Collects runtime statistics from `Docker` daemon (`metrics-address` feature).                                                                           |
| [dockerhub](https://github.com/netdata/go.d.plugin/tree/master/modules/dockerhub)         | [`Docker Hub`](https://hub.docker.com/)                                                                                                                | Collects docker repositories statistics (pulls, starts, status, time since last update).                                                                |
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
| [phpdaemon](https://github.com/netdata/go.d.plugin/tree/master/modules/phpdaemon)         | [`phpDaemon`](https://daemon.io/)                                                                                                                      | Collects workers statistics (total, active, idle).                                                                                                      |
| [phpfpm](https://github.com/netdata/go.d.plugin/tree/master/modules/phpfpm)               | [`PHP-FPM`](https://php-fpm.org/)                                                                                                                      | Collects application summary and processes health metrics scraping status page (`/status?full`).                                                        |
| [pihole](https://github.com/netdata/go.d.plugin/tree/master/modules/pihole)               | [`Pi-hole`](https://pi-hole.net/)                                                                                                                      | Monitors basic (dns queries, clients, blocklist) and extended (top clients, top permitted and blocked domains) statistics using PHP API.                |
| [portcheck](https://github.com/netdata/go.d.plugin/tree/master/modules/portcheck)         | `TCP Endpoint`                                                                                                                                         | Monitors tcp endpoint availability and response time.                                                                                                   |
| [pulsar](https://github.com/netdata/go.d.plugin/tree/master/modules/pulsar)               | [`Apache Pulsar`](http://pulsar.apache.org/)                                                                                                           | Collects summary, namespaces and topics performance statistics.                                                                                         |
| [rabbitmq](https://github.com/netdata/go.d.plugin/tree/master/modules/rabbitmq)           | [`RabbitMQ`](https://www.rabbitmq.com/)                                                                                                                | Collects message broker overview, system and per virtual host metrics.                                                                                  |
| [scaleio](https://github.com/netdata/go.d.plugin/tree/master/modules/scaleio)             | [`Dell EMC ScaleIO`](https://www.delltechnologies.com/en-us/storage/data-storage/software-defined-storage.htm)                                         | Monitors storage system, storage pools and sdcs health and performance metrics via VxFlex OS Gateway API.                                               |
| [solr](https://github.com/netdata/go.d.plugin/tree/master/modules/solr)                   | [`Solr`](https://lucene.apache.org/solr/)                                                                                                              | Collects application search requests, search errors, update requests and update errors statistics.                                                      |
| [springboot2](https://github.com/netdata/go.d.plugin/tree/master/modules/springboot2)     | [`Spring Boot2`](https://spring.io/)                                                                                                                   | Monitors running Java Spring Boot 2 applications that expose their metrics with the use of the Spring Boot Actuator.                                    |
| [squidlog](https://github.com/netdata/go.d.plugin/tree/master/modules/squidlog)           | [`Squid`](http://www.squid-cache.org/)                                                                                                                 | Tails access logs and provides very detailed caching proxy performance statistics. This module is able to parse 200k+ rows for less then half a second. |
| [tengine](https://github.com/netdata/go.d.plugin/tree/master/modules/tengine)             | [`Tengine`](https://tengine.taobao.org/)                                                                                                               | Monitors web server statistics using information provided by `ngx_http_reqstat_module`.                                                                 |
| [unbound](https://github.com/netdata/go.d.plugin/tree/master/modules/unbound)             | [`Unbound`](https://nlnetlabs.nl/projects/unbound/about/)                                                                                              | Collects dns resolver summary and extended system and per thread metrics via `remote-control` interface.                                                |
| [vcsa](https://github.com/netdata/go.d.plugin/tree/master/modules/vcsa)                   | [`vCenter Server Appliance`](https://docs.vmware.com/en/VMware-vSphere/6.5/com.vmware.vsphere.vcsa.doc/GUID-223C2821-BD98-4C7A-936B-7DBE96291BA4.html) | Monitors appliance system, components and software updates health statuses via Health API.                                                              |
| [vernemq](https://github.com/netdata/go.d.plugin/tree/master/modules/vernemq)             | [`VerneMQ`](https://vernemq.com/)                                                                                                                      | Monitors MQTT broker health and performance metrics. It collects all available info for both MQTTv3 and v5 communication.                               |
| [vsphere](https://github.com/netdata/go.d.plugin/tree/master/modules/vsphere)             | [`VMware vCenter Server`](https://www.vmware.com/products/vcenter-server.html)                                                                         | Collects hosts and virtual machines performance metrics.                                                                                                |
| [web_log](https://github.com/netdata/go.d.plugin/tree/master/modules/weblog)              | `Apache/NGINX`                                                                                                                                         | Tails access logs and provides very detailed web server performance statistics. This module is able to parse 200k+ rows for less then half a second.    |
| [wmi](https://github.com/netdata/go.d.plugin/tree/master/modules/wmi)                     | `Windows Machines`                                                                                                                                     | Collects cpu, memory, network, disk, os, system and logon metrics scraping `wmi_exporter`.                                                              |
| [x509check](https://github.com/netdata/go.d.plugin/tree/master/modules/x509check)         | `Digital Certificates`                                                                                                                                 | Monitors certificate expiration time.                                                                                                                   |
| [zookeeper](https://github.com/netdata/go.d.plugin/tree/master/modules/zookeeper)         | [`ZooKeeper`](https://zookeeper.apache.org/)                                                                                                           | Monitors application health metrics reading server response to `mntr` command.                                                                          |

### NodeJS (`node.d`)

| Name                                                   | Monitors                                | Description                                                                                            |
| :----------------------------------------------------- | :-------------------------------------- | :------------------------------------------------------------------------------------------------------|
| [named](node.d.plugin/named/README.md)                 | [`ISC Bind`](https://www.isc.org/bind/) | Collects Name server summary performance statistics via web interface (`statistics-channels` feature). |
| [fronius](node.d.plugin/fronius/README.md)             | `Fronius Symo Solar Power Products`     | Collects power, consumption, autonomy, energy and inverter statistics.                                 |
| [sma_webbox](node.d.plugin/sma_webbox/README.md)       | `SMA Sunny WebBox`                      | Collects power statistics.                                                                             |
| [snmp](node.d.plugin/snmp/README.md)                   | `SNMP Devices`                          | Gathers data using SNMP protocol. All protocol versions are supported.                                 |
| [stiebeleltron](node.d.plugin/stiebeleltron/README.md) | `Stiebel Eltron ISG Products`           | Collects heat pumps and how water installations metrics.                                               |

### Python (`python.d`)

| Name                                                         | Monitors                                                                      | Description                                                                                                                                                                                          |
| :----------------------------------------------------------- | :---------------------------------------------------------------------------- | :----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [adaptec_raid](python.d.plugin/adaptec_raid/README.md)       | `Adaptec RAID Controller`                                                     | Monitors logical and physical devices health metrics using `arcconf` tool.                                                                                                                           |
| [am2320](python.d.plugin/am2320/README.md)                   | `AM2320 Sensor`                                                               | Monitors sensor temperature and humidity                                                                                                                                                             |
| [apache](python.d.plugin/apache/README.md)                   | [`Apache`](https://httpd.apache.org/)                                         | Collects web server performance metrics via `server-status?auto` endpoint.                                                                                                                           |
| [beanstalk](python.d.plugin/beanstalk/README.md)             | [`Beanstalk`](https://beanstalkapp.com/)                                      | Collects server summary and per tube metrics                                                                                                                                                         |
| [bind_rndc](python.d.plugin/bind_rndc/README.md)             | [`ISC Bind`](https://www.isc.org/bind/)                                       | Collects Name server summary performance statistics using `rndc` tool.                                                                                                                               |
| [boinc](python.d.plugin/boinc/README.md)                     | [`BOINC`](https://boinc.berkeley.edu/)                                        | Monitors task counts.                                                                                                                                                                                |
| [ceph](python.d.plugin/ceph/README.md)                       | [`CEPH`](https://ceph.io/)                                                    | Monitors the ceph cluster usage and server data consumption.                                                                                                                                         |
| [chrony](python.d.plugin/chrony/README.md)                   | [`Chrony`](https://chrony.tuxfamily.org/)                                     | Monitors the precision and statistics of a local `chronyd` server.                                                                                                                                   |
| [couchdb](python.d.plugin/couchdb/README.md)                 | [`Apache CouchDB`](https://couchdb.apache.org/)                               | Monitors database health and performance metrics (reads/writes, HTTP traffic, replication status, etc).                                                                                              |
| [dns_query_time](python.d.plugin/dns_query_time/README.md)   | `DNS Query RTT`                                                               | measures DNS query round trip time.                                                                                                                                                                  |
| [dnsdist](python.d.plugin/dnsdist/README.md)                 | [`PowerDNS dnsdist`](https://dnsdist.org/)                                    | Collects load-balancer performance and health metrics.                                                                                                                                               |
| [dockerd](python.d.plugin/dockerd/README.md)                 | [`Docker Engine`](https://docs.docker.com/engine/)                            | Collects container health statistics.                                                                                                                                                                |
| [dovecot](python.d.plugin/dovecot/README.md)                 | [`Dovecot`](https://www.dovecot.org/)                                         | Collects email server performance metrics. It reads server response to `EXPORT global` command.                                                                                                      |
| [elasticsearch](python.d.plugin/elasticsearch/README.md)     | [`Elasticseach`](https://www.elastic.co/elasticsearch)                        | Collects search engine performance and health statistics. Optionally Collects per index metrics.                                                                                                     |
| [energid](python.d.plugin/energid/README.md)                 | [`Energi Core Node`](https://github.com/energicryptocurrency/energi)          | Monitors blockchain, memory, network and unspent transactions statistics.                                                                                                                            |
| [example](python.d.plugin/example/README.md)                 | -                                                                             | just an data collector example.                                                                                                                                                                      |
| [exim](python.d.plugin/exim/README.md)                       | [`Exim`](https://www.exim.org/)                                               | reports MTA emails queue length using `exim` tool.                                                                                                                                                   |
| [fail2ban](python.d.plugin/fail2ban/README.md)               | [`Fail2ban`](https://www.fail2ban.org/wiki/index.php/Main_Page)               | parses log file and reports ban rate and number of banned IPS (since the last restart of Netdata) for every jail. It automatically detects all configured jails from `Fail2ban` configuration files. |
| [freeradius](python.d.plugin/freeradius/README.md)           | [`FreeRADIUS`](https://freeradius.org/)                                       | Collects server authentication and accounting statistics from `status server` using `radclient` tool.                                                                                                |
| [gearman](python.d.plugin/gearman/README.md)                 | [`Gearman`](http://gearman.org/)                                              | Collects application summary (queued, running) and per job worker statistics (queued, idle, running)                                                                                                 |
| [go_expvar](python.d.plugin/go_expvar/README.md)             | `Go Application`                                                              | Monitors Go application that exposes its metrics with the use of `expvar` package from the Go standard library.                                                                                      |
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
| [memcached](python.d.plugin/memcached/README.md)             | [`Memcached`](https://memcached.org/)                                         | Collects memory-caching system performance metrics. It reads server response to `stats` command (stats interface).                                                                                   |
| [mongodb](python.d.plugin/mongodb/README.md)                 | [`MongoDB`](https://www.mongodb.com/)                                         | Monitors database health, performance and replication metrics.                                                                                                                                       |
| [monit](python.d.plugin/monit/README.md)                     | [`Monit`](https://mmonit.com/monit/)                                          | Monitors statuses of targets (service-checks) using XML stats interface.                                                                                                                             |
| [mysql](python.d.plugin/mysql/README.md)                     | [`MySQL`](https://www.mysql.com/)                                             | Collects database global, replication and per user statistics.                                                                                                                                       |
| [nginx](python.d.plugin/nginx/README.md)                     | [`NGINX`](https://www.nginx.com/)                                             | Monitors web server status information. Information is provided by `ngx_http_stub_status_module`.                                                                                                    |
| [nginx_plus](python.d.plugin/nginx_plus/README.md)           | [`NGINX Plus`](https://www.nginx.com/products/nginx/)                         | Collects web server global, and per server zone/upstream/cache metrics.                                                                                                                              |
| [nsd](python.d.plugin/nsd/README.md)                         | [`NSD`](https://www.nlnetlabs.nl/projects/nsd/about/)                         | Monitors Name server performance metrics using `nsd-control` tool.                                                                                                                                   |
| [ntpd](python.d.plugin/ntpd/README.md)                       | `NTPd`                                                                        | Monitors the system variables of the local `ntpd` daemon (optional incl. variables of the polled peers) using the NTP Control Message Protocol via UDP socket.                                       |
| [nvidia_smi](python.d.plugin/nvidia_smi/README.md)           | `Nvidia GPU`                                                                  | Monitors performance metrics (memory usage, fan speed, pcie bandwidth utilization, temperature, etc.) using `nvidia-smi` tool.                                                                       |
| [openldap](python.d.plugin/openldap/README.md)               | [`OpenLDAP`](https://www.openldap.org/)                                       | provides statistics information from openldap (slapd) server. Statistics are taken from LDAP monitoring interface.                                                                                   |
| [oracledb](python.d.plugin/oracledb/README.md)               | [`OracleDB`](https://www.oracle.com/database/)                                | Monitors database performance and health metrics.                                                                                                                                                    |
| [ovpn_status_log](python.d.plugin/ovpn_status_log/README.md) | [`OpenVPN`](https://openvpn.net/)                                             | parses server log files and provides summary (client, traffic) metrics.                                                                                                                              |
| [phpfpm](python.d.plugin/phpfpm/README.md)                   | [`PHP-FPM`](https://php-fpm.org/)                                             | Collects application summary and processes health metrics scraping status page (`/status?full`).                                                                                                     |
| [portcheck](python.d.plugin/portcheck/README.md)             | `TCP Endpoint`                                                                | Monitors tcp endpoint availability and response time.                                                                                                                                                |
| [postfix](python.d.plugin/postfix/README.md)                 | [`Postfix`](http://www.postfix.org/)                                          | Monitors MTA email queue statistics using `postqueue` tool.                                                                                                                                          |
| [postgres](python.d.plugin/postgres/README.md)               | [`PostgreSQL`](https://www.postgresql.org/)                                   | Collects database health and performance metrics.                                                                                                                                                    |
| [powerdns](python.d.plugin/powerdns/README.md)               | [`PowerDNS`](https://www.powerdns.com/)                                       | Monitors authoritative server and recursor statistics.                                                                                                                                               |
| [proxysql](python.d.plugin/proxysql/README.md)               | [`ProxySQL`](https://www.proxysql.com/)                                       | Monitors database backend and frontend performance metrics.                                                                                                                                          |
| [puppet](python.d.plugin/puppet/README.md)                   | [`Puppet`](https://puppet.com/)                                               | Monitors status of Puppet Server and Puppet DB.                                                                                                                                                      |
| [rabbitmq](python.d.plugin/rabbitmq/README.md)               | [`RabbitMQ`](https://www.rabbitmq.com/)                                       | Collects message broker global and per virtual host metrics.                                                                                                                                         |
| [redis](python.d.plugin/redis/README.md)                     | [`Redis`](https://redis.io/)                                                  | Monitors database status. It reads server response to `INFO` command.                                                                                                                                |
| [rethinkdbs](python.d.plugin/rethinkdbs/README.md)           | [`RethinkDB`](https://rethinkdb.com/)                                         | Collects database server and cluster statistics.                                                                                                                                                     |
| [retroshare](python.d.plugin/retroshare/README.md)           | [`RetroShare`](https://retroshare.cc/)                                        | Monitors application bandwidth, peers and DHT metrics.                                                                                                                                               |
| [riakkv](python.d.plugin/riakkv/README.md)                   | [`RiakKV`](https://riak.com/products/riak-kv/index.html)                      | Collects database stats from `/stats` endpoint.                                                                                                                                                      |
| [samba](python.d.plugin/samba/README.md)                     | [`Samba`](https://www.samba.org/)                                             | Collects file sharing metrics using `smbstatus` tool.                                                                                                                                                |
| [sensors](python.d.plugin/sensors/README.md)                 | `Linux Machines Sensors`                                                      | reads system sensors information (temperature, voltage, electric current, power, etc.).                                                                                                              |
| [smartd_log](python.d.plugin/smartd_log/README.md)           | `Storage Devices`                                                             | reads SMART Disk Monitoring Daemon logs.                                                                                                                                                             |
| [spigotmc](python.d.plugin/spigotmc/README.md)               | [`SpigotMC`](https://www.spigotmc.org/)                                       | Monitors average ticket rate and number of users.                                                                                                                                                    |
| [springboot](python.d.plugin/springboot/README.md)           | [`Spring Boot2`](https://spring.io/)                                          | Monitors running Java Spring Boot applications that expose their metrics with the use of the Spring Boot Actuator.                                                                                   |
| [squid](python.d.plugin/squid/README.md)                     | [`Squid`](http://www.squid-cache.org/)                                        | Monitors client and server bandwidth/requests. This module Gathers data from Cache Manager component.                                                                                                |
| [tomcat](python.d.plugin/tomcat/README.md)                   | [`Apache Tomcat`](http://tomcat.apache.org/)                                  | Collects web server performance metrics from Manager App (`/manager/status?XML=true`).                                                                                                               |
| [tor](python.d.plugin/tor/README.md)                         | [`Tor`](https://www.torproject.org/)                                          | reports traffic usage statistics. It uses `Tor` control port to gather the data.                                                                                                                     |
| [traefik](python.d.plugin/traefik/README.md)                 | [`Traefic`](https://docs.traefik.io/)                                         | uses Health API to provide statistics.                                                                                                                                                               |
| [uwsgi](python.d.plugin/uwsgi/README.md)                     | `uWSGI`                                                                       | Monitors performance metrics exposed by `Stats Server`.                                                                                                                                              |
| [varnish](python.d.plugin/varnish/README.md)                 | [`Varnish Cache`](https://varnish-cache.org/)                                 | provides HTTP accelerator global, backends (VBE) and disks (SMF) statistics using `varnishstat` tool.                                                                                                |
| [w1sensor](python.d.plugin/w1sensor/README.md)               | `1-Wire Sensors`                                                              | Monitors sensor temperature.                                                                                                                                                                         |
| [web_log](python.d.plugin/web_log/README.md)                 | `Apache/NGINX/Squid`                                                          | tails access log file and Collects web server/caching proxy metrics.                                                                                                                                 |

## Third-party plugins

Third-party plugins are distributed by their developers, and are not installed by default with Netdata. To use a
third-party plugin, you must visit their documentation and follow the installation steps.

| Name                                                                                       | Monitors       | Description                                               |
| :------------------------------------------------------------------------------------------| :------------- | :-------------------------------------------------------- |
| [netdata_nv_plugin](https://github.com/coraxx/netdata_nv_plugin)                           | Nvidia GPUs    | Monitors nvidia GPUs.                                     |
| [netdata_ts3_plugin](https://github.com/coraxx/netdata_ts3_plugin)                         | Teamspeak 3    | polls active users and bandwidth from TeamSpeak 3 servers |
| [netdata-ssh-module](https://github.com/Yaser-Amiri/netdata-ssh-module)                    | SSH            | Monitors failed authentication requests of an SSH server  |
| [netdata-numsessions](https://github.com/veksh/netdata-numsessions)                        | `uptime`       | Collects the number of currently logged-on users.         |
| [netdata_cyberpwrups_plugin](https://github.com/HawtDogFlvrWtr/netdata_cyberpwrups_plugin) | CyberPower UPS |                                                           |
| [nim-netdata-plugin](https://github.com/FedericoCeratto/nim-netdata-plugin)                | helper         | A helper to create native Netdata plugins using Nim.      |
