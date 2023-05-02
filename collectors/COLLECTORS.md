# Monitor anything with Netdata

Netdata uses collectors to help you gather metrics from your favorite applications and services and view them in
real-time, interactive charts. The following list includes collectors for both external services/applications and
internal system metrics.

Learn more
about [how collectors work](https://github.com/netdata/netdata/blob/master/collectors/README.md), and
then learn how to [enable or
configure](https://github.com/netdata/netdata/blob/master/collectors/REFERENCE.md#enable-and-disable-a-specific-collection-module) any of the below collectors using the same process.

Some collectors have both Go and Python versions as we continue our effort to migrate all collectors to Go. In these
cases, _Netdata always prioritizes the Go version_, and we highly recommend you use the Go versions for the best
experience.

If you want to use a Python version of a collector, you need to
explicitly [disable the Go version](https://github.com/netdata/netdata/blob/master/collectors/REFERENCE.md#enable-and-disable-a-specific-collection-module),
and enable the Python version. Netdata then skips the Go version and attempts to load the Python version and its
accompanying configuration file.

## Add your application to Netdata

If you don't see the app/service you'd like to monitor in this list:

- If your application has a Prometheus endpoint, Netdata can monitor it! Look at our
  [generic Prometheus collector](https://github.com/netdata/go.d.plugin/blob/master/modules/prometheus/README.md).

- If your application is instrumented to expose [StatsD](https://blog.netdata.cloud/introduction-to-statsd/) metrics,
  see our [generic StatsD collector](https://github.com/netdata/netdata/blob/master/collectors/statsd.plugin/README.md).

- If you have data in CSV, JSON, XML or other popular formats, you may be able to use our
  [generic structured data (Pandas) collector](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/pandas/README.md),

- Check out our [GitHub issues](https://github.com/netdata/netdata/issues). Use the search bar to look for previous
  discussions about that collector—we may be looking for assistance from users such as yourself!

- If you don't see the collector there, you can make
  a [feature request](https://github.com/netdata/netdata/issues/new/choose) on GitHub.

- If you have basic software development skills, you can add your own plugin
  in [Go](https://github.com/netdata/go.d.plugin/blob/master/README.md#how-to-develop-a-collector)
  or [Python](https://github.com/netdata/netdata/blob/master/docs/guides/python-collector.md)

## Available Collectors

- [Monitor anything with Netdata](#monitor-anything-with-netdata)
  - [Add your application to Netdata](#add-your-application-to-netdata)
  - [Available Collectors](#available-collectors)
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

- [Prometheus endpoints](https://github.com/netdata/go.d.plugin/blob/master/modules/prometheus/README.md): Gathers
  metrics from any number of Prometheus endpoints, with support to autodetect more than 600 services and applications.
- [Pandas](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/pandas/README.md): A Python
  collector that gathers
  metrics from a [pandas](https://pandas.pydata.org/) dataframe. Pandas is a high level data processing library in
  Python that can read various formats of data from local files or web endpoints. Custom processing and transformation
  logic can also be expressed as part of the collector configuration.

### APM (application performance monitoring)

- [Go applications](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/go_expvar/README.md):
  Monitor any Go application that exposes its
  metrics with the  `expvar` package from the Go standard library.
- [Java Spring Boot 2 applications](https://github.com/netdata/go.d.plugin/blob/master/modules/springboot2/README.md):
  Monitor running Java Spring Boot 2 applications that expose their metrics with the use of the Spring Boot Actuator.
- [statsd](https://github.com/netdata/netdata/blob/master/collectors/statsd.plugin/README.md): Implement a high
  performance `statsd` server for Netdata.
- [phpDaemon](https://github.com/netdata/go.d.plugin/blob/master/modules/phpdaemon/README.md): Collect worker
  statistics (total, active, idle), and uptime for web and network applications.
- [uWSGI](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/uwsgi/README.md): Monitor
  performance metrics exposed by the uWSGI Stats
  Server.

### Containers and VMs

- [Docker containers](https://github.com/netdata/netdata/blob/master/collectors/cgroups.plugin/README.md): Monitor the
  health and performance of individual Docker containers using the cgroups collector plugin.
- [DockerD](https://github.com/netdata/go.d.plugin/blob/master/modules/docker/README.md): Collect container health
  statistics.
- [Docker Engine](https://github.com/netdata/go.d.plugin/blob/master/modules/docker_engine/README.md): Collect
  runtime statistics from the `docker` daemon using the `metrics-address` feature.
- [Docker Hub](https://github.com/netdata/go.d.plugin/blob/master/modules/dockerhub/README.md): Collect statistics
  about Docker repositories, such as pulls, starts, status, time since last update, and more.
- [Libvirt](https://github.com/netdata/netdata/blob/master/collectors/cgroups.plugin/README.md): Monitor the health and
  performance of individual Libvirt containers
  using the cgroups collector plugin.
- [LXC](https://github.com/netdata/netdata/blob/master/collectors/cgroups.plugin/README.md): Monitor the health and
  performance of individual LXC containers using
  the cgroups collector plugin.
- [LXD](https://github.com/netdata/netdata/blob/master/collectors/cgroups.plugin/README.md): Monitor the health and
  performance of individual LXD containers using
  the cgroups collector plugin.
- [systemd-nspawn](https://github.com/netdata/netdata/blob/master/collectors/cgroups.plugin/README.md): Monitor the
  health and performance of individual
  systemd-nspawn containers using the cgroups collector plugin.
- [vCenter Server Appliance](https://github.com/netdata/go.d.plugin/blob/master/modules/vcsa/README.md): Monitor
  appliance system, components, and software update health statuses via the Health API.
- [vSphere](https://github.com/netdata/go.d.plugin/blob/master/modules/vsphere/README.md): Collect host and virtual
  machine performance metrics.
- [Xen/XCP-ng](https://github.com/netdata/netdata/blob/master/collectors/xenstat.plugin/README.md): Collect XenServer
  and XCP-ng metrics using `libxenstat`.

### Data stores

- [CockroachDB](https://github.com/netdata/go.d.plugin/blob/master/modules/cockroachdb/README.md): Monitor various
  database components using `_status/vars` endpoint.
- [Consul](https://github.com/netdata/go.d.plugin/blob/master/modules/consul/README.md): Capture service and unbound
  checks status (passing, warning, critical, maintenance).
- [Couchbase](https://github.com/netdata/go.d.plugin/blob/master/modules/couchbase/README.md): Gather per-bucket
  metrics from any number of instances of the distributed JSON document database.
- [CouchDB](https://github.com/netdata/go.d.plugin/blob/master/modules/couchdb/README.md): Monitor database health and
  performance metrics
  (reads/writes, HTTP traffic, replication status, etc).
- [MongoDB](https://github.com/netdata/go.d.plugin/blob/master/modules/mongodb/README.md): Collect server, database,
  replication and sharding performance and health metrics.
- [MySQL](https://github.com/netdata/go.d.plugin/blob/master/modules/mysql/README.md): Collect database global,
  replication and per user statistics.
- [OracleDB](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/oracledb/README.md): Monitor
  database performance and health metrics.
- [Pika](https://github.com/netdata/go.d.plugin/blob/master/modules/pika/README.md): Gather metric, such as clients,
  memory usage, queries, and more from the Redis interface-compatible database.
- [Postgres](https://github.com/netdata/go.d.plugin/blob/master/modules/postgres/README.md): Collect database health
  and performance metrics.
- [ProxySQL](https://github.com/netdata/go.d.plugin/blob/master/modules/proxysql/README.md): Monitor database backend
  and frontend performance metrics.
- [Redis](https://github.com/netdata/go.d.plugin/blob/master/modules/redis/README.md): Monitor status from any
  number of database instances by reading the server's response to the `INFO ALL` command.
- [RethinkDB](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/rethinkdbs/README.md): Collect
  database server and cluster statistics.
- [Riak KV](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/riakkv/README.md): Collect
  database stats from the `/stats` endpoint.
- [Zookeeper](https://github.com/netdata/go.d.plugin/blob/master/modules/zookeeper/README.md): Monitor application
  health metrics reading the server's response to the `mntr` command.
- [Memcached](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/memcached/README.md): Collect
  memory-caching system performance metrics.

### Distributed computing

- [BOINC](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/boinc/README.md): Monitor the total
  number of tasks, open tasks, and task
  states for the distributed computing client.
- [Gearman](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/gearman/README.md): Collect
  application summary (queued, running) and per-job
  worker statistics (queued, idle, running).

### Email

- [Dovecot](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/dovecot/README.md): Collect email
  server performance metrics by reading the
  server's response to the `EXPORT global` command.
- [EXIM](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/exim/README.md): Uses the `exim` tool
  to monitor the queue length of a
  mail/message transfer agent (MTA).
- [Postfix](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/postfix/README.md): Uses
  the `postqueue` tool to monitor the queue length of a
  mail/message transfer agent (MTA).

### Kubernetes

- [Kubelet](https://github.com/netdata/go.d.plugin/blob/master/modules/k8s_kubelet/README.md): Monitor one or more
  instances of the Kubelet agent and collects metrics on number of pods/containers running, volume of Docker
  operations, and more.
- [kube-proxy](https://github.com/netdata/go.d.plugin/blob/master/modules/k8s_kubeproxy/README.md): Collect
  metrics, such as syncing proxy rules and REST client requests, from one or more instances of `kube-proxy`.
- [Service discovery](https://github.com/netdata/agent-service-discovery/blob/master/README.md): Find what services are running on a
  cluster's pods, converts that into configuration files, and exports them so they can be monitored by Netdata.

### Logs

- [Fluentd](https://github.com/netdata/go.d.plugin/blob/master/modules/fluentd/README.md): Gather application
  plugins metrics from an endpoint provided by `in_monitor plugin`.
- [Logstash](https://github.com/netdata/go.d.plugin/blob/master/modules/logstash/README.md): Monitor JVM threads,
  memory usage, garbage collection statistics, and more.
- [OpenVPN status logs](https://github.com/netdata/go.d.plugin/blob/master/modules/openvpn_status_log/README.md): Parse
  server log files and provide summary (client, traffic) metrics.
- [Squid web server logs](https://github.com/netdata/go.d.plugin/blob/master/modules/squidlog/README.md): Tail Squid
  access logs to return the volume of requests, types of requests, bandwidth, and much more.
- [Web server logs (Go version for Apache, NGINX)](https://github.com/netdata/go.d.plugin/blob/master/modules/weblog/README.md): Tail access logs and provide
  very detailed web server performance statistics. This module is able to parse 200k+ rows in less than half a second.
- [Web server logs (Apache, NGINX)](https://github.com/netdata/go.d.plugin/blob/master/modules/weblog/README.md): Tail
  access log
  file and collect web server/caching proxy metrics.

### Messaging

- [ActiveMQ](https://github.com/netdata/go.d.plugin/blob/master/modules/activemq/README.md): Collect message broker
  queues and topics statistics using the ActiveMQ Console API.
- [Beanstalk](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/beanstalk/README.md): Collect
  server and tube-level statistics, such as CPU
  usage, jobs rates, commands, and more.
- [Pulsar](https://github.com/netdata/go.d.plugin/blob/master/modules/pulsar/README.md): Collect summary,
  namespaces, and topics performance statistics.
- [RabbitMQ](https://github.com/netdata/go.d.plugin/blob/master/modules/rabbitmq/README.md): Collect message
  broker overview, system and per virtual host metrics.
- [VerneMQ](https://github.com/netdata/go.d.plugin/blob/master/modules/vernemq/README.md): Monitor MQTT broker
  health and performance metrics. It collects all available info for both MQTTv3 and v5 communication

### Network

- [Bind 9](https://github.com/netdata/go.d.plugin/blob/master/modules/bind/README.md): Collect nameserver summary
  performance statistics via a web interface (`statistics-channels` feature).
- [Chrony](https://github.com/netdata/go.d.plugin/blob/master/modules/chrony/README.md): Monitor the precision and
  statistics of a local `chronyd` server.
- [CoreDNS](https://github.com/netdata/go.d.plugin/blob/master/modules/coredns/README.md): Measure DNS query round
  trip time.
- [Dnsmasq](https://github.com/netdata/go.d.plugin/blob/master/modules/dnsmasq_dhcp/README.md): Automatically
  detects all configured `Dnsmasq` DHCP ranges and Monitor their utilization.
- [DNSdist](https://github.com/netdata/go.d.plugin/blob/master/modules/dnsdist/README.md): Collect
  load-balancer performance and health metrics.
- [Dnsmasq DNS Forwarder](https://github.com/netdata/go.d.plugin/blob/master/modules/dnsmasq/README.md): Gather
  queries, entries, operations, and events for the lightweight DNS forwarder.
- [DNS Query Time](https://github.com/netdata/go.d.plugin/blob/master/modules/dnsquery/README.md): Monitor the round
  trip time for DNS queries in milliseconds.
- [Freeradius](https://github.com/netdata/go.d.plugin/blob/master/modules/freeradius/README.md): Collect
  server authentication and accounting statistics from the `status server`.
- [Libreswan](https://github.com/netdata/netdata/blob/master/collectors/charts.d.plugin/libreswan/README.md): Collect
  bytes-in, bytes-out, and uptime metrics.
- [Icecast](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/icecast/README.md): Monitor the
  number of listeners for active sources.
- [ISC Bind (RDNC)](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/bind_rndc/README.md):
  Collect nameserver summary performance
  statistics using the `rndc` tool.
- [ISC DHCP](https://github.com/netdata/go.d.plugin/blob/master/modules/isc_dhcpd/README.md): Reads a
  `dhcpd.leases` file and collects metrics on total active leases, pool active leases, and pool utilization.
- [OpenLDAP](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/openldap/README.md): Provides
  statistics information from the OpenLDAP
  (`slapd`) server.
- [NSD](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/nsd/README.md): Monitor nameserver
  performance metrics using the `nsd-control`
  tool.
- [NTP daemon](https://github.com/netdata/go.d.plugin/blob/master/modules/ntpd/README.md): Monitor the system variables
  of the local `ntpd` daemon (optionally including variables of the polled peers) using the NTP Control Message Protocol
  via a UDP socket.
- [OpenSIPS](https://github.com/netdata/netdata/blob/master/collectors/charts.d.plugin/opensips/README.md): Collect
  server health and performance metrics using the
  `opensipsctl` tool.
- [OpenVPN](https://github.com/netdata/go.d.plugin/blob/master/modules/openvpn/README.md): Gather server summary
  (client, traffic) and per user metrics (traffic, connection time) stats using `management-interface`.
- [Pi-hole](https://github.com/netdata/go.d.plugin/blob/master/modules/pihole/README.md): Monitor basic (DNS
  queries, clients, blocklist) and extended (top clients, top permitted, and blocked domains) statistics using the PHP
  API.
- [PowerDNS Authoritative Server](https://github.com/netdata/go.d.plugin/blob/master/modules/powerdns/README.md):
  Monitor one or more instances of the nameserver software to collect questions, events, and latency metrics.
- [PowerDNS Recursor](https://github.com/netdata/go.d.plugin/blob/master/modules/powerdns/README.md#recursor):
  Gather incoming/outgoing questions, drops, timeouts, and cache usage from any number of DNS recursor instances.
- [RetroShare](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/retroshare/README.md): Monitor
  application bandwidth, peers, and DHT
  metrics.
- [Tor](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/tor/README.md): Capture traffic usage
  statistics using the Tor control port.
- [Unbound](https://github.com/netdata/go.d.plugin/blob/master/modules/unbound/README.md): Collect DNS resolver
  summary and extended system and per thread metrics via the `remote-control` interface.

### Provisioning

- [Puppet](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/puppet/README.md): Monitor the
  status of Puppet Server and Puppet DB.

### Remote devices

- [AM2320](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/am2320/README.md): Monitor sensor
  temperature and humidity.
- [Access point](https://github.com/netdata/netdata/blob/master/collectors/charts.d.plugin/ap/README.md): Monitor
  client, traffic and signal metrics using the `aw`
  tool.
- [APC UPS](https://github.com/netdata/netdata/blob/master/collectors/charts.d.plugin/apcupsd/README.md): Capture status
  information using the `apcaccess` tool.
- [Energi Core](https://github.com/netdata/go.d.plugin/blob/master/modules/energid/README.md): Monitor
  blockchain indexes, memory usage, network usage, and transactions of wallet instances.
- [UPS/PDU](https://github.com/netdata/netdata/blob/master/collectors/charts.d.plugin/nut/README.md): Read the status of
  UPS/PDU devices using the `upsc` tool.
- [SNMP devices](https://github.com/netdata/go.d.plugin/blob/master/modules/snmp/README.md): Gather data using the SNMP
  protocol.
- [1-Wire sensors](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/w1sensor/README.md):
  Monitor sensor temperature.

### Search

- [Elasticsearch](https://github.com/netdata/go.d.plugin/blob/master/modules/elasticsearch/README.md): Collect
  dozens of metrics on search engine performance from local nodes and local indices. Includes cluster health and
  statistics.
- [Solr](https://github.com/netdata/go.d.plugin/blob/master/modules/solr/README.md): Collect application search
  requests, search errors, update requests, and update errors statistics.

### Storage

- [Ceph](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/ceph/README.md): Monitor the Ceph
  cluster usage and server data consumption.
- [HDFS](https://github.com/netdata/go.d.plugin/blob/master/modules/hdfs/README.md): Monitor health and performance
  metrics for filesystem datanodes and namenodes.
- [IPFS](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/ipfs/README.md): Collect file system
  bandwidth, peers, and repo metrics.
- [Scaleio](https://github.com/netdata/go.d.plugin/blob/master/modules/scaleio/README.md): Monitor storage system,
  storage pools, and SDCS health and performance metrics via VxFlex OS Gateway API.
- [Samba](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/samba/README.md): Collect file
  sharing metrics using the `smbstatus` tool.

### Web

- [Apache](https://github.com/netdata/go.d.plugin/blob/master/modules/apache/README.md): Collect Apache web
  server performance metrics via the `server-status?auto` endpoint.
- [HAProxy](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/haproxy/README.md): Collect
  frontend, backend, and health metrics.
- [HTTP endpoints](https://github.com/netdata/go.d.plugin/blob/master/modules/httpcheck/README.md): Monitor
  any HTTP endpoint's availability and response time.
- [Lighttpd](https://github.com/netdata/go.d.plugin/blob/master/modules/lighttpd/README.md): Collect web server
  performance metrics using the `server-status?auto` endpoint.
- [Litespeed](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/litespeed/README.md): Collect
  web server data (network, connection,
  requests, cache) by reading `.rtreport*` files.
- [Nginx](https://github.com/netdata/go.d.plugin/blob/master/modules/nginx/README.md): Monitor web server
  status information by gathering metrics via `ngx_http_stub_status_module`.
- [Nginx VTS](https://github.com/netdata/go.d.plugin/blob/master/modules/nginxvts/README.md): Gathers metrics from
  any Nginx deployment with the _virtual host traffic status module_ enabled, including metrics on uptime, memory
  usage, and cache, and more.
- [PHP-FPM](https://github.com/netdata/go.d.plugin/blob/master/modules/phpfpm/README.md): Collect application
  summary and processes health metrics by scraping the status page (`/status?full`).
- [TCP endpoints](https://github.com/netdata/go.d.plugin/blob/master/modules/portcheck/README.md): Monitor any
  TCP endpoint's availability and response time.
- [Spigot Minecraft servers](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/spigotmc/README.md):
  Monitor average ticket rate and number
  of users.
- [Squid](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/squid/README.md): Monitor client and
  server bandwidth/requests by gathering
  data from the Cache Manager component.
- [Tengine](https://github.com/netdata/go.d.plugin/blob/master/modules/tengine/README.md): Monitor web server
  statistics using information provided by `ngx_http_reqstat_module`.
- [Tomcat](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/tomcat/README.md): Collect web
  server performance metrics from the Manager App
  (`/manager/status?XML=true`).
- [Traefik](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/traefik/README.md): Uses Traefik's
  Health API to provide statistics.
- [Varnish](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/varnish/README.md): Provides HTTP
  accelerator global, backends (VBE), and
  disks (SMF) statistics using the `varnishstat` tool.
- [x509 check](https://github.com/netdata/go.d.plugin/blob/master/modules/x509check/README.md): Monitor certificate
  expiration time.
- [Whois domain expiry](https://github.com/netdata/go.d.plugin/blob/master/modules/whoisquery/README.md): Checks the
  remaining time until a given domain is expired.

## System collectors

The Netdata Agent can collect these system- and hardware-level metrics using a variety of collectors, some of which
(such as `proc.plugin`) collect multiple types of metrics simultaneously.

### Applications

- [Fail2ban](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/fail2ban/README.md): Parses
  configuration files to detect all jails, then
  uses log files to report ban rates and volume of banned IPs.
- [Monit](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/monit/README.md): Monitor statuses
  of targets (service-checks) using the XML
  stats interface.
- [Windows](https://github.com/netdata/go.d.plugin/blob/master/modules/windows/README.md): Collect CPU, memory,
  network, disk, OS, system, and log-in metrics scraping [windows_exporter](https://github.com/prometheus-community/windows_exporter).

### Disks and filesystems

- [BCACHE](https://github.com/netdata/netdata/blob/master/collectors/proc.plugin/README.md): Monitor BCACHE statistics
  with the `proc.plugin` collector.
- [Block devices](https://github.com/netdata/netdata/blob/master/collectors/proc.plugin/README.md): Gather metrics about
  the health and performance of block
  devices using the `proc.plugin` collector.
- [Btrfs](https://github.com/netdata/netdata/blob/master/collectors/proc.plugin/README.md): Monitors Btrfs filesystems
  with the `proc.plugin` collector.
- [Device mapper](https://github.com/netdata/netdata/blob/master/collectors/proc.plugin/README.md): Gather metrics about
  the Linux device mapper with the proc
  collector.
- [Disk space](https://github.com/netdata/netdata/blob/master/collectors/diskspace.plugin/README.md): Collect disk space
  usage metrics on Linux mount points.
- [Clock synchronization](https://github.com/netdata/netdata/blob/master/collectors/timex.plugin/README.md): Collect the
  system clock synchronization status on Linux.
- [Files and directories](https://github.com/netdata/go.d.plugin/blob/master/modules/filecheck/README.md): Gather
  metrics about the existence, modification time, and size of files or directories.
- [ioping.plugin](https://github.com/netdata/netdata/blob/master/collectors/ioping.plugin/README.md): Measure disk
  read/write latency.
- [NFS file servers and clients](https://github.com/netdata/netdata/blob/master/collectors/proc.plugin/README.md):
  Gather operations, utilization, and space usage
  using the `proc.plugin` collector.
- [RAID arrays](https://github.com/netdata/netdata/blob/master/collectors/proc.plugin/README.md): Collect health, disk
  status, operation status, and more with the `proc.plugin` collector.
- [Veritas Volume Manager](https://github.com/netdata/netdata/blob/master/collectors/proc.plugin/README.md): Gather
  metrics about the Veritas Volume Manager (VVM).
- [ZFS](https://github.com/netdata/netdata/blob/master/collectors/proc.plugin/README.md): Monitor bandwidth and
  utilization of ZFS disks/partitions using the proc
  collector.

### eBPF

- [Files](https://github.com/netdata/netdata/blob/master/collectors/ebpf.plugin/README.md): Provides information about
  how often a system calls kernel
  functions related to file descriptors using the eBPF collector.
- [Virtual file system (VFS)](https://github.com/netdata/netdata/blob/master/collectors/ebpf.plugin/README.md): Monitor
  IO, errors, deleted objects, and
  more for kernel virtual file systems (VFS) using the eBPF collector.
- [Processes](https://github.com/netdata/netdata/blob/master/collectors/ebpf.plugin/README.md): Monitor threads, task
  exits, and errors using the eBPF collector.

### Hardware

- [Adaptec RAID](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/adaptec_raid/README.md):
  Monitor logical and physical devices health
  metrics using the `arcconf` tool.
- [CUPS](https://github.com/netdata/netdata/blob/master/collectors/cups.plugin/README.md): Monitor CUPS.
- [FreeIPMI](https://github.com/netdata/netdata/blob/master/collectors/freeipmi.plugin/README.md):
  Uses `libipmimonitoring-dev` or `libipmimonitoring-devel` to
  monitor the number of sensors, temperatures, voltages, currents, and more.
- [Hard drive temperature](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/hddtemp/README.md):
  Monitor the temperature of storage
  devices.
- [HP Smart Storage Arrays](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/hpssa/README.md):
  Monitor controller, cache module, logical
  and physical drive state, and temperature using the `ssacli` tool.
- [MegaRAID controllers](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/megacli/README.md):
  Collect adapter, physical drives, and
  battery stats using the `megacli` tool.
- [NVIDIA GPU](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/nvidia_smi/README.md): Monitor
  performance metrics (memory usage, fan
  speed, pcie bandwidth utilization, temperature, and more) using the `nvidia-smi` tool.
- [Sensors](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/sensors/README.md): Reads system
  sensors information (temperature, voltage,
  electric current, power, and more) from `/sys/devices/`.
- [S.M.A.R.T](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/smartd_log/README.md): Reads
  SMART Disk Monitoring daemon logs.

### Memory

- [Available memory](https://github.com/netdata/netdata/blob/master/collectors/proc.plugin/README.md): Tracks changes in
  available RAM using the `proc.plugin` collector.
- [Committed memory](https://github.com/netdata/netdata/blob/master/collectors/proc.plugin/README.md): Monitor committed
  memory using the `proc.plugin` collector.
- [Huge pages](https://github.com/netdata/netdata/blob/master/collectors/proc.plugin/README.md): Gather metrics about
  huge pages in Linux and FreeBSD with the
  `proc.plugin` collector.
- [KSM](https://github.com/netdata/netdata/blob/master/collectors/proc.plugin/README.md): Measure the amount of merging,
  savings, and effectiveness using the
  `proc.plugin` collector.
- [Numa](https://github.com/netdata/netdata/blob/master/collectors/proc.plugin/README.md): Gather metrics on the number
  of non-uniform memory access (NUMA) events
  every second using the `proc.plugin` collector.
- [Page faults](https://github.com/netdata/netdata/blob/master/collectors/proc.plugin/README.md): Collect the number of
  memory page faults per second using the
  `proc.plugin` collector.
- [RAM](https://github.com/netdata/netdata/blob/master/collectors/proc.plugin/README.md): Collect metrics on system RAM,
  available RAM, and more using the
  `proc.plugin` collector.
- [SLAB](https://github.com/netdata/netdata/blob/master/collectors/slabinfo.plugin/README.md): Collect kernel SLAB
  details on Linux systems.
- [swap](https://github.com/netdata/netdata/blob/master/collectors/proc.plugin/README.md): Monitor the amount of free
  and used swap at every second using the
  `proc.plugin` collector.
- [Writeback memory](https://github.com/netdata/netdata/blob/master/collectors/proc.plugin/README.md): Collect how much
  memory is actively being written to disk at
  every second using the `proc.plugin` collector.

### Networks

- [Access points](https://github.com/netdata/netdata/blob/master/collectors/charts.d.plugin/ap/README.md): Visualizes
  data related to access points.
- [Ping](https://github.com/netdata/go.d.plugin/blob/master/modules/ping/README.md): Measure network latency, jitter and
  packet loss between the monitored node
  and any number of remote network end points.
- [Netfilter](https://github.com/netdata/netdata/blob/master/collectors/nfacct.plugin/README.md): Collect netfilter
  firewall, connection tracker, and accounting
  metrics using `libmnl` and `libnetfilter_acct`.
- [Network stack](https://github.com/netdata/netdata/blob/master/collectors/proc.plugin/README.md): Monitor the
  networking stack for errors, TCP connection aborts,
  bandwidth, and more.
- [Network QoS](https://github.com/netdata/netdata/blob/master/collectors/tc.plugin/README.md): Collect traffic QoS
  metrics (`tc`) of Linux network interfaces.
- [SYNPROXY](https://github.com/netdata/netdata/blob/master/collectors/proc.plugin/README.md): Monitor entries uses, SYN
  packets received, TCP cookies, and more.

### Operating systems

- [freebsd.plugin](https://github.com/netdata/netdata/blob/master/collectors/freebsd.plugin/README.md): Collect resource
  usage and performance data on FreeBSD systems.
- [macOS](https://github.com/netdata/netdata/blob/master/collectors/macos.plugin/README.md): Collect resource usage and
  performance data on macOS systems.

### Processes

- [Applications](https://github.com/netdata/netdata/blob/master/collectors/apps.plugin/README.md): Gather CPU, disk,
  memory, network, eBPF, and other metrics per
  application using the `apps.plugin` collector.
- [systemd](https://github.com/netdata/netdata/blob/master/collectors/cgroups.plugin/README.md): Monitor the CPU and
  memory usage of systemd services using the
  `cgroups.plugin` collector.
- [systemd unit states](https://github.com/netdata/go.d.plugin/blob/master/modules/systemdunits/README.md): See the
  state (active, inactive, activating, deactivating, failed) of various systemd unit types.
- [System processes](https://github.com/netdata/netdata/blob/master/collectors/proc.plugin/README.md): Collect metrics
  on system load and total processes running
  using `/proc/loadavg` and the `proc.plugin` collector.
- [Uptime](https://github.com/netdata/netdata/blob/master/collectors/proc.plugin/README.md): Monitor the uptime of a
  system using the `proc.plugin` collector.

### Resources

- [CPU frequency](https://github.com/netdata/netdata/blob/master/collectors/proc.plugin/README.md): Monitor CPU
  frequency, as set by the `cpufreq` kernel module,
  using the `proc.plugin` collector.
- [CPU idle](https://github.com/netdata/netdata/blob/master/collectors/proc.plugin/README.md): Measure CPU idle every
  second using the `proc.plugin` collector.
- [CPU performance](https://github.com/netdata/netdata/blob/master/collectors/perf.plugin/README.md): Collect CPU
  performance metrics using performance monitoring
  units (PMU).
- [CPU throttling](https://github.com/netdata/netdata/blob/master/collectors/proc.plugin/README.md): Gather metrics
  about thermal throttling using the `/proc/stat`
  module and the `proc.plugin` collector.
- [CPU utilization](https://github.com/netdata/netdata/blob/master/collectors/proc.plugin/README.md): Capture CPU
  utilization, both system-wide and per-core, using
  the `/proc/stat` module and the `proc.plugin` collector.
- [Entropy](https://github.com/netdata/netdata/blob/master/collectors/proc.plugin/README.md): Monitor the available
  entropy on a system using the `proc.plugin`
  collector.
- [Interprocess Communication (IPC)](https://github.com/netdata/netdata/blob/master/collectors/proc.plugin/README.md):
  Monitor IPC semaphores and shared memory
  using the `proc.plugin` collector.
- [Interrupts](https://github.com/netdata/netdata/blob/master/collectors/proc.plugin/README.md): Monitor interrupts per
  second using the `proc.plugin` collector.
- [IdleJitter](https://github.com/netdata/netdata/blob/master/collectors/idlejitter.plugin/README.md): Measure CPU
  latency and jitter on all operating systems.
- [SoftIRQs](https://github.com/netdata/netdata/blob/master/collectors/proc.plugin/README.md): Collect metrics on
  SoftIRQs, both system-wide and per-core, using the
  `proc.plugin` collector.
- [SoftNet](https://github.com/netdata/netdata/blob/master/collectors/proc.plugin/README.md): Capture SoftNet events per
  second, both system-wide and per-core,
  using the `proc.plugin` collector.

### Users

- [systemd-logind](https://github.com/netdata/go.d.plugin/blob/master/modules/logind/README.md): Monitor active
  sessions, users, and seats tracked
  by `systemd-logind` or `elogind`.
- [User/group usage](https://github.com/netdata/netdata/blob/master/collectors/apps.plugin/README.md): Gather CPU, disk,
  memory, network, and other metrics per user
  and user group using the `apps.plugin` collector.

## Netdata collectors

These collectors are recursive in nature, in that they monitor some function of the Netdata Agent itself. Some
collectors are described only in code and associated charts in Netdata dashboards.

- [ACLK (code only)](https://github.com/netdata/netdata/blob/master/aclk/legacy/aclk_stats.c): View whether a Netdata
  Agent is connected to Netdata Cloud via the [ACLK](https://github.com/netdata/netdata/blob/master/aclk/README.md), the
  volume of queries, process times, and more.
- [Alarms](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/alarms/README.md): This collector
  creates an
  **Alarms** menu with one line plot showing the alarm states of a Netdata Agent over time.
- [Anomalies](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/anomalies/README.md): This
  collector uses the
  Python PyOD library to perform unsupervised anomaly detection on your Netdata charts and/or dimensions.
- [Exporting (code only)](https://github.com/netdata/netdata/blob/master/exporting/send_internal_metrics.c): Gather
  metrics on CPU utilization for
  the [exporting engine](https://github.com/netdata/netdata/blob/master/exporting/README.md), and specific metrics for
  each enabled
  exporting connector.
- [Global statistics (code only)](https://github.com/netdata/netdata/blob/master/daemon/global_statistics.c): See
  metrics on the CPU utilization, network traffic, volume of web clients, API responses, database engine usage, and
  more.

## Orchestrators

Plugin orchestrators organize and run many of the above collectors.

If you're interested in developing a new collector that you'd like to contribute to Netdata, we highly recommend using
the `go.d.plugin`.

- [go.d.plugin](https://github.com/netdata/go.d.plugin): An orchestrator for data collection modules written in `go`.
- [python.d.plugin](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/README.md): An
  orchestrator for data collection modules written in `python` v2/v3.
- [charts.d.plugin](https://github.com/netdata/netdata/blob/master/collectors/charts.d.plugin/README.md): An
  orchestrator for data collection modules written in `bash` v4+.

## Third-party collectors

These collectors are developed and maintained by third parties and, unlike the other collectors, are not installed by
default. To use a third-party collector, visit their GitHub/documentation page and follow their installation procedures.

<details>
<summary>Typical third party Python collector installation instructions</summary>

In general the below steps should be sufficient to use a third party collector.

1. Download collector code file
   into [folder expected by Netdata](https://github.com/netdata/netdata/blob/master/collectors/plugins.d/README.md#environment-variables).
2. Download default collector configuration file
   into [folder expected by Netdata](https://github.com/netdata/netdata/blob/master/collectors/plugins.d/README.md#environment-variables).
3. [Edit configuration file](https://github.com/netdata/netdata/blob/master/docs/collect/enable-configure#configure-a-collector)
   from step 2 if required.
4. [Enable collector](https://github.com/netdata/netdata/blob/master/docs/collect/enable-configure#enable-a-collector-or-its-orchestrator).
5. [Restart Netdata](https://github.com/netdata/netdata/blob/master/docs/configure/start-stop-restart.md)

For example below are the steps to enable
the [Python ClickHouse collector](https://github.com/netdata/community/tree/main/collectors/python.d.plugin/clickhouse).

```bash
# download python collector script to /usr/libexec/netdata/python.d/
$ sudo wget https://raw.githubusercontent.com/netdata/community/main/collectors/python.d.plugin/clickhouse/clickhouse.chart.py -O /usr/libexec/netdata/python.d/clickhouse.chart.py

# (optional) download default .conf to /etc/netdata/python.d/
$ sudo wget https://raw.githubusercontent.com/netdata/community/main/collectors/python.d.plugin/clickhouse/clickhouse.conf -O /etc/netdata/python.d/clickhouse.conf

# enable collector by adding line a new line with "clickhouse: yes" to /etc/netdata/python.d.conf file
# this will append to the file if it already exists or create it if not
$ sudo echo "clickhouse: yes" >> /etc/netdata/python.d.conf

# (optional) edit clickhouse.conf if needed
$ sudo vi /etc/netdata/python.d/clickhouse.conf

# restart netdata 
# see docs for more information: https://github.com/netdata/netdata/blob/master/docs/configure/start-stop-restart.md
$ sudo systemctl restart netdata
```

</details>

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
- [ClickHouse](https://github.com/netdata/community/tree/main/collectors/python.d.plugin/clickhouse):
  Monitor [ClickHouse](https://clickhouse.com/) database.
- [Ethtool](https://github.com/ghanapunq/netdata_ethtool_plugin): Monitor network interfaces with ethtool.
- [netdata-needrestart](https://github.com/nodiscc/netdata-needrestart) - Check/graph the number of processes/services/kernels that should be restarted after upgrading packages.
- [netdata-debsecan](https://github.com/nodiscc/netdata-debsecan) - Check/graph the number of CVEs in currently installed packages.
- [netdata-logcount](https://github.com/nodiscc/netdata-logcount) - Check/graph the number of syslog messages, by level over time.
- [netdata-apt](https://github.com/nodiscc/netdata-apt) - Check/graph and alert on the number of upgradeable packages, and available distribution upgrades.

## Etc

- [charts.d example](https://github.com/netdata/netdata/blob/master/collectors/charts.d.plugin/example/README.md): An
  example `charts.d` collector.
- [python.d example](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/example/README.md): An
  example `python.d` collector.
- [go.d example](https://github.com/netdata/go.d.plugin/blob/master/modules/example/README.md): An
  example `go.d` collector.
