# Monitor anything with Netdata

**850+ integrations. Zero configuration. Deploy anywhere.**

Netdata uses collectors to help you gather metrics from your favorite applications and services and view them in real-time, interactive charts. The following list includes all the integrations where Netdata can gather metrics from.

Learn more about [how collectors work](/src/collectors/README.md), and then learn how to [enable or configure](/src/collectors/REFERENCE.md#enable-or-disable-collectors-and-plugins) a specific collector.

### Why Teams Choose Us

- ✅ **850+ integrations** automatically discovered and configured
- ✅ **Zero configuration** required - monitors start collecting data immediately
- ✅ **No vendor lock-in** - Deploy anywhere, own your data
- ✅ **1-second resolution** - Real-time visibility, not delayed averages
- ✅ **Flexible deployment** - On-premise, cloud, or hybrid

### Find Your Technology

**Select your primary infrastructure to jump directly to relevant integrations:**

**Cloud & Infrastructure:**
[AWS](#cloud-provider-managed) • [Azure](#cloud-provider-managed) • [GCP](#cloud-provider-managed) • [Kubernetes](#kubernetes) • [Docker](#containers-and-vms) • [VMware](#containers-and-vms)

**Databases & Caching:**
[MySQL](#databases) • [PostgreSQL](#databases) • [MongoDB](#databases) • [Redis](#databases) • [Elasticsearch](#search-engines) • [Oracle](#databases)

**Web & Application:**
[NGINX](#web-servers-and-web-proxies) • [Apache](#web-servers-and-web-proxies) • [HAProxy](#web-servers-and-web-proxies) • [Tomcat](#web-servers-and-web-proxies) • [PHP-FPM](#web-servers-and-web-proxies)

**Message Queues:**
[Kafka](#message-brokers) • [RabbitMQ](#message-brokers) • [ActiveMQ](#message-brokers) • [NATS](#message-brokers) • [Pulsar](#message-brokers)

**Operating Systems:**
[Linux](#linux-systems) • [Windows](#windows-systems) • [macOS](#macos-systems) • [FreeBSD](#freebsd)

**Don't see what you need?** We support [Prometheus endpoints](#generic-data-collection), [SNMP devices](#generic-data-collection), [StatsD](#add-your-application-to-netdata), and [custom data sources](#generic-data-collection).

## Beyond the 850+ integrations

Netdata can monitor virtually any application through generic collectors:

- **[Prometheus collector](/src/go/plugin/go.d/collector/prometheus/README.md)** - Any application exposing Prometheus metrics
- **[StatsD collector](/src/collectors/statsd.plugin/README.md)** - Applications instrumented with [StatsD](https://blog.netdata.cloud/introduction-to-statsd/)
- **[Pandas collector](/src/collectors/python.d.plugin/pandas/README.md)** - Structured data from CSV, JSON, XML, and more

Need a dedicated integration? [Submit a feature request](https://github.com/netdata/netdata/issues/new/choose) on GitHub.

## Available Data Collection Integrations

### Linux Systems

| Integration | Description |
|-------------|-------------|
| [CPU performance](https://github.com/netdata/netdata/blob/master/src/collectors/perf.plugin/integrations/cpu_performance.md) | Monitor CPU performance counters for hardware-level insights |
| [Disk space](https://github.com/netdata/netdata/blob/master/src/collectors/diskspace.plugin/integrations/disk_space.md) | Track disk space usage across all mount points |
| [OpenRC](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/openrc.md) | Monitor OpenRC init system service status |

#### CPU

| Integration | Description |
|-------------|-------------|
| [Interrupts](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/interrupts.md) | Track hardware and software interrupt rates |
| [SoftIRQ statistics](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/softirq_statistics.md) | Monitor software interrupt handling performance |

#### Disk

| Integration | Description |
|-------------|-------------|
| [Disk Statistics](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/disk_statistics.md) | Track disk I/O operations, throughput, and latency |
| [MD RAID](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/md_raid.md) | Monitor Linux software RAID arrays health and performance |

##### BTRFS

| Integration | Description |
|-------------|-------------|
| [BTRFS](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/btrfs.md) | Track BTRFS filesystem allocation and usage |

##### NFS

| Integration | Description |
|-------------|-------------|
| [NFS Client](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/nfs_client.md) | Monitor NFS client operations and performance |
| [NFS Server](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/nfs_server.md) | Track NFS server requests and throughput |

##### ZFS

| Integration | Description |
|-------------|-------------|
| [ZFS Adaptive Replacement Cache](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/zfs_adaptive_replacement_cache.md) | Monitor ZFS ARC memory cache efficiency |

#### Firewall

| Integration | Description |
|-------------|-------------|
| [Conntrack](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/conntrack.md) | Track connection tracking table usage and limits |
| [Netfilter](https://github.com/netdata/netdata/blob/master/src/collectors/nfacct.plugin/integrations/netfilter.md) | Monitor iptables packet and byte counters |
| [Synproxy](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/synproxy.md) | Track SYN flood protection statistics |
| [nftables](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/nftables.md) | Monitor nftables firewall rules and counters |

#### IPC

| Integration | Description |
|-------------|-------------|
| [Inter Process Communication](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/inter_process_communication.md) | Track System V IPC resources usage |

#### Kernel

| Integration | Description |
|-------------|-------------|
| [Linux kernel SLAB allocator statistics](https://github.com/netdata/netdata/blob/master/src/collectors/slabinfo.plugin/integrations/linux_kernel_slab_allocator_statistics.md) | Monitor kernel memory allocation efficiency |
| [Power Capping](https://github.com/netdata/netdata/blob/master/src/collectors/debugfs.plugin/integrations/power_capping.md) | Track CPU power consumption limits and usage |

#### Memory

| Integration | Description |
|-------------|-------------|
| [Kernel Same-Page Merging](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/kernel_same-page_merging.md) | Monitor KSM memory deduplication efficiency |
| [Linux ZSwap](https://github.com/netdata/netdata/blob/master/src/collectors/debugfs.plugin/integrations/linux_zswap.md) | Track compressed swap cache performance |
| [Memory Statistics](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/memory_statistics.md) | Monitor detailed memory allocation and usage |
| [Memory Usage](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/memory_usage.md) | Track RAM utilization across categories |
| [Memory modules (DIMMs)](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/memory_modules_dimms.md) | Monitor physical RAM module errors |
| [Non-Uniform Memory Access](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/non-uniform_memory_access.md) | Track NUMA node memory allocation |
| [Page types](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/page_types.md) | Monitor memory page classifications |
| [System Memory Fragmentation](https://github.com/netdata/netdata/blob/master/src/collectors/debugfs.plugin/integrations/system_memory_fragmentation.md) | Track memory fragmentation levels |
| [ZRAM](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/zram.md) | Monitor compressed RAM block device usage |

#### Network

| Integration | Description |
|-------------|-------------|
| [Access Points](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/ap/integrations/access_points.md) | Monitor wireless access point connections |
| [IP Virtual Server](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/ip_virtual_server.md) | Track IPVS load balancer connections |
| [IPv6 Socket Statistics](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/ipv6_socket_statistics.md) | Monitor IPv6 network connections |
| [InfiniBand](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/infiniband.md) | Track InfiniBand network adapter performance |
| [Network interfaces](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/network_interfaces.md) | Monitor network interface traffic and errors |
| [Network statistics](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/network_statistics.md) | Track TCP/UDP/IP protocol statistics |
| [SCTP Statistics](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/sctp_statistics.md) | Monitor Stream Control Transmission Protocol |
| [Socket statistics](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/socket_statistics.md) | Track socket state distribution |
| [Softnet Statistics](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/softnet_statistics.md) | Monitor network packet processing queues |
| [Wireless network interfaces](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/wireless_network_interfaces.md) | Track WiFi signal strength and quality |
| [tc QoS classes](https://github.com/netdata/netdata/blob/master/src/collectors/tc.plugin/integrations/tc_qos_classes.md) | Monitor traffic control QoS performance |

#### Power Supply

| Integration | Description |
|-------------|-------------|
| [Power Supply](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/power_supply.md) | Track battery and power adapter status |

#### Pressure

| Integration | Description |
|-------------|-------------|
| [Pressure Stall Information](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/pressure_stall_information.md) | Monitor CPU, memory, and I/O contention |

#### System

| Integration | Description |
|-------------|-------------|
| [Entropy](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/entropy.md) | Track system entropy pool for cryptography |
| [System Load Average](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/system_load_average.md) | Monitor system load across time periods |
| [System Uptime](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/system_uptime.md) | Track system uptime and boot time |
| [System statistics](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/system_statistics.md) | Monitor context switches and process creation |

### Containers and VMs

| Integration | Description |
|-------------|-------------|
| [Containers](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/integrations/containers.md) | Monitor containerized applications resource usage |
| [Docker Engine](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/docker_engine/integrations/docker_engine.md) | Track Docker daemon metrics and container stats |
| [Docker Hub repository](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/dockerhub/integrations/docker_hub_repository.md) | Monitor Docker Hub image pull statistics |
| [Docker](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/docker/integrations/docker.md) | Track Docker container resource consumption |
| [LXC Containers](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/integrations/lxc_containers.md) | Monitor LXC container CPU and memory usage |
| [Libvirt Containers](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/integrations/libvirt_containers.md) | Track libvirt managed containers |
| [NSX-T](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/nsx-t.md) | Monitor VMware NSX-T virtualization platform |
| [Podman](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/podman.md) | Track Podman container engine metrics |
| [Proxmox Containers](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/integrations/proxmox_containers.md) | Monitor Proxmox LXC containers |
| [Proxmox VE](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/proxmox_ve.md) | Track Proxmox virtual environment performance |
| [VMware vCenter Server](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/vsphere/integrations/vmware_vcenter_server.md) | Monitor VMware ESXi hosts and VMs |
| [Virtual Machines](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/integrations/virtual_machines.md) | Track VM resource usage across hypervisors |
| [Xen XCP-ng](https://github.com/netdata/netdata/blob/master/src/collectors/xenstat.plugin/integrations/xen_xcp-ng.md) | Monitor Xen hypervisor and VMs |
| [cAdvisor](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/cadvisor.md) | Track container metrics via Google cAdvisor |
| [oVirt Containers](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/integrations/ovirt_containers.md) | Monitor oVirt virtualization containers |
| [vCenter Server Appliance](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/vcsa/integrations/vcenter_server_appliance.md) | Track VMware VCSA health and performance |

### Kubernetes

| Integration | Description |
|-------------|-------------|
| [Cilium Agent](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/cilium_agent.md) | Monitor Cilium CNI agent network flows |
| [Cilium Operator](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/cilium_operator.md) | Track Cilium operator resource management |
| [Cilium Proxy](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/cilium_proxy.md) | Monitor Cilium L7 proxy connections |
| [Kubelet](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/k8s_kubelet/integrations/kubelet.md) | Track Kubelet pod and container metrics |
| [Kubeproxy](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/k8s_kubeproxy/integrations/kubeproxy.md) | Monitor kube-proxy network rules |
| [Kubernetes Cluster Cloud Cost](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/kubernetes_cluster_cloud_cost.md) | Track K8s cluster cloud spending |
| [Kubernetes Cluster State](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/k8s_state/integrations/kubernetes_cluster_state.md) | Monitor K8s cluster resources via kube-state-metrics |
| [Kubernetes Containers](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/integrations/kubernetes_containers.md) | Track K8s pod and container resource usage |
| [Rancher](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/rancher.md) | Monitor Rancher Kubernetes management platform |

### Web Servers and Web Proxies

| Integration | Description |
|-------------|-------------|
| [APIcast](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/apicast.md) | Monitor Red Hat 3scale APIcast gateway |
| [Apache](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/apache/integrations/apache.md) | Track Apache HTTP Server requests and workers |
| [Clash](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/clash.md) | Monitor Clash proxy server connections |
| [Cloudflare PCAP](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/cloudflare_pcap.md) | Track Cloudflare packet capture analytics |
| [Envoy](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/envoy/integrations/envoy.md) | Monitor Envoy proxy traffic and clusters |
| [Gobetween](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/gobetween.md) | Track Gobetween load balancer backends |
| [HAProxy](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/haproxy/integrations/haproxy.md) | Monitor HAProxy load balancer health and traffic |
| [HHVM](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/hhvm.md) | Track HipHop Virtual Machine PHP execution |
| [HTTPD](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/apache/integrations/httpd.md) | Monitor Apache HTTPD server performance |
| [Lighttpd](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/lighttpd/integrations/lighttpd.md) | Track Lighttpd web server connections |
| [Litespeed](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/litespeed/integrations/litespeed.md) | Monitor LiteSpeed web server metrics |
| [NGINX Plus](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/nginxplus/integrations/nginx_plus.md) | Track NGINX Plus advanced metrics |
| [NGINX Unit](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/nginxunit/integrations/nginx_unit.md) | Monitor NGINX Unit application server |
| [NGINX VTS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/nginxvts/integrations/nginx_vts.md) | Track NGINX with Virtual Host Traffic Status |
| [NGINX](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/nginx/integrations/nginx.md) | Monitor NGINX web server requests and connections |
| [PHP-FPM](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/phpfpm/integrations/php-fpm.md) | Track PHP-FPM process pool performance |
| [Squid log files](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/squidlog/integrations/squid_log_files.md) | Parse Squid proxy access logs |
| [Squid](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/squid/integrations/squid.md) | Monitor Squid proxy cache performance |
| [Tengine](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/tengine/integrations/tengine.md) | Track Tengine web server metrics |
| [Tomcat](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/tomcat/integrations/tomcat.md) | Monitor Apache Tomcat servlet container |
| [Traefik](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/traefik/integrations/traefik.md) | Track Traefik reverse proxy requests |
| [Varnish](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/varnish/integrations/varnish.md) | Monitor Varnish HTTP cache hit rates |
| [Web server log files](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/weblog/integrations/web_server_log_files.md) | Parse web server access logs in real-time |
| [uWSGI](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/uwsgi/integrations/uwsgi.md) | Track uWSGI application server workers |

### Databases

| Integration | Description |
|-------------|-------------|
| [4D Server](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/4d_server.md) | Monitor 4D database server connections |
| [AWS RDS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/aws_rds.md) | Track Amazon RDS database instances |
| [BOINC](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/boinc/integrations/boinc.md) | Monitor BOINC distributed computing tasks |
| [Cassandra](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/cassandra/integrations/cassandra.md) | Track Apache Cassandra NoSQL database |
| [ClickHouse](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/clickhouse/integrations/clickhouse.md) | Monitor ClickHouse analytical database |
| [ClusterControl CMON](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/clustercontrol_cmon.md) | Track database cluster management |
| [CockroachDB](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/cockroachdb/integrations/cockroachdb.md) | Monitor CockroachDB distributed SQL database |
| [CouchDB](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/couchdb/integrations/couchdb.md) | Track CouchDB document database |
| [Couchbase](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/couchbase/integrations/couchbase.md) | Monitor Couchbase NoSQL database cluster |
| [HANA](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/hana.md) | Track SAP HANA in-memory database |
| [Hasura GraphQL Server](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/hasura_graphql_server.md) | Monitor Hasura GraphQL engine queries |
| [InfluxDB](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/influxdb.md) | Track InfluxDB time-series database |
| [Machbase](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/machbase.md) | Monitor Machbase time-series database |
| [MariaDB](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/mysql/integrations/mariadb.md) | Track MariaDB database queries and replication |
| [MaxScale](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/maxscale/integrations/maxscale.md) | Monitor MariaDB MaxScale database proxy |
| [Memcached (community)](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/memcached_community.md) | Track Memcached cache via community exporter |
| [Memcached](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/memcached/integrations/memcached.md) | Monitor Memcached in-memory key-value cache |
| [MongoDB](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/mongodb/integrations/mongodb.md) | Track MongoDB NoSQL document database |
| [MySQL](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/mysql/integrations/mysql.md) | Monitor MySQL relational database performance |
| [ODBC](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/odbc.md) | Track databases via ODBC connections |
| [Oracle DB (community)](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/oracle_db_community.md) | Monitor Oracle database via community exporter |
| [Oracle DB](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/oracledb/integrations/oracle_db.md) | Track Oracle database performance metrics |
| [Patroni](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/patroni.md) | Monitor PostgreSQL HA with Patroni |
| [Percona MySQL](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/mysql/integrations/percona_mysql.md) | Track Percona Server for MySQL |
| [PgBouncer](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/pgbouncer/integrations/pgbouncer.md) | Monitor PgBouncer PostgreSQL connection pooler |
| [Pgpool-II](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/pgpool-ii.md) | Track Pgpool-II PostgreSQL middleware |
| [Pika](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/pika/integrations/pika.md) | Monitor Pika Redis-compatible database |
| [PostgreSQL](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/postgres/integrations/postgresql.md) | Track PostgreSQL database queries and connections |
| [ProxySQL](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/proxysql/integrations/proxysql.md) | Monitor ProxySQL MySQL proxy performance |
| [Redis](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/redis/integrations/redis.md) | Track Redis in-memory data store operations |
| [RethinkDB](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/rethinkdb/integrations/rethinkdb.md) | Monitor RethinkDB realtime database |
| [Riak KV](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/riakkv/integrations/riak_kv.md) | Track Riak KV distributed database |
| [SQL Database agnostic](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/sql_database_agnostic.md) | Monitor SQL databases via generic exporter |
| [Vertica](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/vertica.md) | Track Vertica analytics platform |
| [Warp10](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/warp10.md) | Monitor Warp10 time-series platform |
| [YugabyteDB](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/yugabytedb/integrations/yugabytedb.md) | Track YugabyteDB distributed SQL database |
| [pgBackRest](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/pgbackrest.md) | Monitor PostgreSQL backup and restore |

### Cloud Provider Managed

| Integration | Description |
|-------------|-------------|
| [AWS EC2 Compute instances](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/aws_ec2_compute_instances.md) | Monitor Amazon EC2 instance metrics |
| [AWS EC2 Spot Instance](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/aws_ec2_spot_instance.md) | Track EC2 Spot Instance pricing |
| [AWS ECS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/aws_ecs.md) | Monitor Amazon ECS container service |
| [AWS Health events](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/aws_health_events.md) | Track AWS Health service status |
| [AWS Quota](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/aws_quota.md) | Monitor AWS service quota usage |
| [AWS S3 buckets](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/aws_s3_buckets.md) | Track S3 bucket storage and requests |
| [AWS SQS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/aws_sqs.md) | Monitor Amazon SQS queue metrics |
| [AWS instance health](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/aws_instance_health.md) | Track AWS instance health checks |
| [Akamai Global Traffic Management](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/akamai_global_traffic_management.md) | Monitor Akamai GTM traffic routing |
| [Akami Cloudmonitor](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/akami_cloudmonitor.md) | Track Akamai CDN performance |
| [Alibaba Cloud](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/alibaba_cloud.md) | Monitor Alibaba Cloud services |
| [ArvanCloud CDN](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/arvancloud_cdn.md) | Track ArvanCloud CDN metrics |
| [Azure AD App passwords](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/azure_ad_app_passwords.md) | Monitor Azure AD app password expiration |
| [Azure Elastic Pool SQL](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/azure_elastic_pool_sql.md) | Track Azure SQL elastic pool resources |
| [Azure Resources](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/azure_resources.md) | Monitor Azure cloud resources |
| [Azure SQL](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/azure_sql.md) | Track Azure SQL database performance |
| [Azure Service Bus](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/azure_service_bus.md) | Monitor Azure Service Bus messaging |
| [Azure application](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/azure_application.md) | Track Azure application metrics |
| [BigQuery](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/bigquery.md) | Monitor Google BigQuery data warehouse |
| [CloudWatch](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/cloudwatch.md) | Track AWS CloudWatch metrics |
| [Dell EMC ECS cluster](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/dell_emc_ecs_cluster.md) | Monitor Dell EMC ECS object storage |
| [DigitalOcean](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/digitalocean.md) | Track DigitalOcean droplet metrics |
| [GCP GCE](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/gcp_gce.md) | Monitor Google Compute Engine instances |
| [GCP Quota](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/gcp_quota.md) | Track Google Cloud service quotas |
| [Google Cloud Platform](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/google_cloud_platform.md) | Monitor GCP services |
| [Google Stackdriver](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/google_stackdriver.md) | Track Google Cloud monitoring metrics |
| [Linode](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/linode.md) | Monitor Linode VPS instances |
| [Lustre metadata](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/lustre_metadata.md) | Track Lustre parallel filesystem |
| [Nextcloud servers](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/nextcloud_servers.md) | Monitor Nextcloud server health |
| [OpenStack](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/openstack.md) | Track OpenStack cloud platform |
| [Zerto](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/zerto.md) | Monitor Zerto disaster recovery |

### Message Brokers

| Integration | Description |
|-------------|-------------|
| [ActiveMQ](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/activemq/integrations/activemq.md) | Monitor ActiveMQ message broker queues |
| [Apache Pulsar](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/pulsar/integrations/apache_pulsar.md) | Track Pulsar pub-sub messaging platform |
| [Beanstalk](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/beanstalk/integrations/beanstalk.md) | Monitor Beanstalkd work queue |
| [IBM MQ](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/ibm_mq.md) | Track IBM MQ enterprise messaging |
| [Kafka Connect](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/kafka_connect.md) | Monitor Kafka Connect data pipelines |
| [Kafka ZooKeeper](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/kafka_zookeeper.md) | Track Kafka ZooKeeper coordination |
| [Kafka](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/kafka.md) | Monitor Apache Kafka message streams |
| [MQTT Blackbox](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/mqtt_blackbox.md) | Track MQTT broker connectivity |
| [NATS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/nats/integrations/nats.md) | Monitor NATS messaging system |
| [RabbitMQ](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/rabbitmq/integrations/rabbitmq.md) | Track RabbitMQ message broker queues |
| [Redis Queue](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/redis_queue.md) | Monitor Redis-based message queues |
| [VerneMQ](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/vernemq/integrations/vernemq.md) | Track VerneMQ MQTT broker |
| [XMPP Server](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/xmpp_server.md) | Monitor XMPP messaging server |
| [mosquitto](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/mosquitto.md) | Track Eclipse Mosquitto MQTT broker |

### DNS and DHCP Servers

| Integration | Description |
|-------------|-------------|
| [Akamai Edge DNS Traffic](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/akamai_edge_dns_traffic.md) | Monitor Akamai DNS query traffic |
| [CoreDNS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/coredns/integrations/coredns.md) | Track CoreDNS server queries and cache |
| [DNS query](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/dnsquery/integrations/dns_query.md) | Monitor DNS query response times |
| [DNSBL](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/dnsbl.md) | Track DNS blacklist queries |
| [DNSdist](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/dnsdist/integrations/dnsdist.md) | Monitor DNSdist load balancer |
| [Dnsmasq DHCP](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/dnsmasq_dhcp/integrations/dnsmasq_dhcp.md) | Track Dnsmasq DHCP leases |
| [Dnsmasq](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/dnsmasq/integrations/dnsmasq.md) | Monitor Dnsmasq DNS/DHCP server |
| [ISC DHCP](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/isc_dhcpd/integrations/isc_dhcp.md) | Track ISC DHCP server leases |
| [NSD](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/nsd/integrations/nsd.md) | Monitor NSD authoritative DNS server |
| [NextDNS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/nextdns.md) | Track NextDNS service metrics |
| [Pi-hole](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/pihole/integrations/pi-hole.md) | Monitor Pi-hole DNS ad-blocker |
| [PowerDNS Authoritative Server](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/powerdns/integrations/powerdns_authoritative_server.md) | Track PowerDNS authoritative server |
| [PowerDNS Recursor](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/powerdns_recursor/integrations/powerdns_recursor.md) | Monitor PowerDNS recursive resolver |
| [Unbound](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/unbound/integrations/unbound.md) | Track Unbound DNS resolver queries |

### System Clock and NTP

| Integration | Description |
|-------------|-------------|
| [Chrony](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/chrony/integrations/chrony.md) | Monitor Chrony NTP daemon time sync |
| [NTPd](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/ntpd/integrations/ntpd.md) | Track NTP daemon clock offset |
| [Timex](https://github.com/netdata/netdata/blob/master/src/collectors/timex.plugin/integrations/timex.md) | Monitor system clock synchronization |

### Systemd

| Integration | Description |
|-------------|-------------|
| [Systemd Services](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/integrations/systemd_services.md) | Monitor systemd service resource usage |
| [Systemd Units](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/systemdunits/integrations/systemd_units.md) | Track systemd unit states |
| [systemd-logind users](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/logind/integrations/systemd-logind_users.md) | Monitor logged-in user sessions |

### Observability

| Integration | Description |
|-------------|-------------|
| [Collectd](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/collectd.md) | Integrate with Collectd monitoring daemon |
| [Dynatrace](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/dynatrace.md) | Connect to Dynatrace monitoring platform |
| [Grafana](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/grafana.md) | Monitor Grafana dashboard server |
| [Hubble](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/hubble.md) | Track Cilium Hubble network observability |
| [Naemon](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/naemon.md) | Monitor Naemon core checks |
| [Nagios](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/nagios.md) | Integrate with Nagios monitoring |
| [New Relic](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/new_relic.md) | Connect to New Relic APM |

### Incident Management

| Integration | Description |
|-------------|-------------|
| [OTRS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/otrs.md) | Monitor OTRS ticketing system metrics |
| [StatusPage](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/statuspage.md) | Track StatusPage incident status |

### Storage, Mount Points and Filesystems

| Integration | Description |
|-------------|-------------|
| [Adaptec RAID](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/adaptecraid/integrations/adaptec_raid.md) | Monitor Adaptec RAID controller health |
| [Altaro Backup](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/altaro_backup.md) | Track Altaro VM Backup status |
| [Borg backup](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/borg_backup.md) | Monitor BorgBackup repository status |
| [CVMFS clients](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/cvmfs_clients.md) | Track CernVM-FS cache performance |
| [Ceph](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/ceph/integrations/ceph.md) | Monitor Ceph distributed storage cluster |
| [DMCache devices](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/dmcache/integrations/dmcache_devices.md) | Track Linux device-mapper cache |
| [Dell EMC Isilon cluster](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/dell_emc_isilon_cluster.md) | Monitor Dell EMC Isilon NAS |
| [Dell EMC ScaleIO](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/scaleio/integrations/dell_emc_scaleio.md) | Track Dell EMC ScaleIO storage |
| [Dell EMC XtremIO cluster](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/dell_emc_xtremio_cluster.md) | Monitor Dell EMC XtremIO array |
| [Dell PowerMax](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/dell_powermax.md) | Track Dell PowerMax storage array |
| [EOS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/eos.md) | Monitor EOS distributed storage |
| [Generic storage enclosure tool](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/generic_storage_enclosure_tool.md) | Track storage enclosure metrics |
| [HDSentinel](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/hdsentinel.md) | Monitor hard drive health with HDSentinel |
| [HPE Smart Arrays](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/hpssa/integrations/hpe_smart_arrays.md) | Track HPE Smart Array RAID controllers |
| [Hadoop Distributed File System (HDFS)](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/hdfs/integrations/hadoop_distributed_file_system_hdfs.md) | Monitor HDFS cluster capacity and health |
| [IBM Spectrum Virtualize](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/ibm_spectrum_virtualize.md) | Track IBM Spectrum Virtualize storage |
| [IBM Spectrum](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/ibm_spectrum.md) | Monitor IBM Spectrum Scale filesystem |
| [IPFS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/ipfs/integrations/ipfs.md) | Track InterPlanetary File System nodes |
| [LVM logical volumes](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/lvm/integrations/lvm_logical_volumes.md) | Monitor LVM volume usage |
| [Lagerist Disk latency](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/lagerist_disk_latency.md) | Track disk latency metrics |
| [MegaCLI MegaRAID](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/megacli/integrations/megacli_megaraid.md) | Monitor LSI MegaRAID controllers |
| [MogileFS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/mogilefs.md) | Track MogileFS distributed filesystem |
| [NVMe devices](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/nvme/integrations/nvme_devices.md) | Monitor NVMe SSD health and performance |
| [NetApp Solidfire](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/netapp_solidfire.md) | Track NetApp SolidFire storage |
| [Netapp ONTAP API](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/netapp_ontap_api.md) | Monitor NetApp ONTAP storage systems |
| [Samba](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/samba/integrations/samba.md) | Track Samba file server connections |
| [Starwind VSAN VSphere Edition](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/starwind_vsan_vsphere_edition.md) | Monitor Starwind VSAN storage |
| [StoreCLI RAID](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/storcli/integrations/storecli_raid.md) | Track StoreCLI RAID controllers |
| [Storidge](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/storidge.md) | Monitor Storidge container storage |
| [Synology ActiveBackup](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/synology_activebackup.md) | Track Synology backup status |
| [ZFS Pools](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/zfspool/integrations/zfs_pools.md) | Monitor ZFS pool capacity and health |

### Logs Servers

| Integration | Description |
|-------------|-------------|
| [AuthLog](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/authlog.md) | Parse authentication logs for failed logins |
| [Fluentd](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/fluentd/integrations/fluentd.md) | Monitor Fluentd log collector buffers |
| [Graylog Server](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/graylog_server.md) | Track Graylog log management platform |
| [Logstash](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/logstash/integrations/logstash.md) | Monitor Logstash pipeline performance |
| [journald](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/journald.md) | Track systemd journal log volume |
| [loki](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/loki.md) | Monitor Grafana Loki log aggregation |
| [mtail](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/mtail.md) | Parse logs with mtail log processor |

### Search Engines

| Integration | Description |
|-------------|-------------|
| [Elasticsearch](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/elasticsearch/integrations/elasticsearch.md) | Monitor Elasticsearch cluster health and indices |
| [Meilisearch](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/meilisearch.md) | Track Meilisearch search engine |
| [OpenSearch](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/elasticsearch/integrations/opensearch.md) | Monitor OpenSearch cluster performance |
| [Sphinx](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/sphinx.md) | Track Sphinx search engine queries |
| [Typesense](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/typesense/integrations/typesense.md) | Monitor Typesense search server |

### Authentication and Authorization

| Integration | Description |
|-------------|-------------|
| [Fail2ban](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/fail2ban/integrations/fail2ban.md) | Monitor Fail2ban banned IP addresses |
| [FreeRADIUS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/freeradius/integrations/freeradius.md) | Track FreeRADIUS authentication requests |
| [HashiCorp Vault secrets](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/hashicorp_vault_secrets.md) | Monitor Vault secrets engine |
| [LDAP](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/ldap.md) | Track LDAP directory service |
| [OpenLDAP (community)](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/openldap_community.md) | Monitor OpenLDAP via community exporter |
| [OpenLDAP](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/openldap/integrations/openldap.md) | Track OpenLDAP directory operations |
| [RADIUS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/radius.md) | Monitor RADIUS authentication server |
| [SSH](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/ssh.md) | Track SSH service availability |
| [TACACS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/tacacs.md) | Monitor TACACS+ authentication |

### CICD Platforms

| Integration | Description |
|-------------|-------------|
| [Concourse](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/concourse.md) | Monitor Concourse CI pipeline jobs |
| [GitLab Runner](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/gitlab_runner.md) | Track GitLab CI/CD runner jobs |
| [Jenkins](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/jenkins.md) | Monitor Jenkins build server jobs |
| [Puppet](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/puppet/integrations/puppet.md) | Track Puppet configuration management |

### Networking Stack and Network Interfaces

| Integration | Description |
|-------------|-------------|
| [8430FT modem](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/8430ft_modem.md) | Monitor 8430FT cable modem metrics |
| [A10 ACOS network devices](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/a10_acos_network_devices.md) | Track A10 Networks load balancers |
| [Aruba devices](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/aruba_devices.md) | Track Aruba network equipment |
| [Cisco ACI](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/cisco_aci.md) | Monitor Cisco ACI fabric health |
| [Fortigate firewall](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/fortigate_firewall.md) | Monitor Fortinet FortiGate firewalls |
| [Fritzbox network devices](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/fritzbox_network_devices.md) | Track FRITZ!Box router metrics |
| [MikroTik devices](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/mikrotik_devices.md) | Monitor MikroTik RouterOS devices |
| [Open vSwitch](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/open_vswitch.md) | Track Open vSwitch virtual networking |
| [Optical modules](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/ethtool/integrations/optical_modules.md) | Monitor SFP/SFP+ optical transceivers |
| [SNMP devices](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/snmp/integrations/snmp_devices.md) | Monitor any SNMP-enabled network devices |
| [Starlink (SpaceX)](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/starlink_spacex.md) | Track Starlink satellite internet metrics |

### Processes and System Services

| Integration | Description |
|-------------|-------------|
| [Applications](https://github.com/netdata/netdata/blob/master/src/collectors/apps.plugin/integrations/applications.md) | Monitor per-application CPU, memory, disk, and network usage |
| [Supervisor](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/supervisord/integrations/supervisor.md) | Track Supervisord process control system |
| [User Groups](https://github.com/netdata/netdata/blob/master/src/collectors/apps.plugin/integrations/user_groups.md) | Monitor resource usage by user groups |
| [Users](https://github.com/netdata/netdata/blob/master/src/collectors/apps.plugin/integrations/users.md) | Track resource usage by individual users |

### UPS

| Integration | Description |
|-------------|-------------|
| [APC UPS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/apcupsd/integrations/apc_ups.md) | Monitor APC UPS battery and power metrics |
| [Eaton UPS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/eaton_ups.md) | Track Eaton UPS power protection systems |
| [UPS (NUT)](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/upsd/integrations/ups_nut.md) | Monitor UPS devices via Network UPS Tools |

### VPNs

| Integration | Description |
|-------------|-------------|
| [OpenVPN status log](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/openvpn_status_log/integrations/openvpn_status_log.md) | Parse OpenVPN status logs for connections |
| [OpenVPN](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/openvpn/integrations/openvpn.md) | Monitor OpenVPN server connections and traffic |
| [Tor](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/tor/integrations/tor.md) | Track Tor anonymity network relay metrics |
| [WireGuard](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/wireguard/integrations/wireguard.md) | Monitor WireGuard VPN tunnel connections |

### Hardware Devices and Sensors

| Integration | Description |
|-------------|-------------|
| [1-Wire Sensors](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/w1sensor/integrations/1-wire_sensors.md) | Monitor 1-Wire temperature and humidity sensors |
| [CUPS](https://github.com/netdata/netdata/blob/master/src/collectors/cups.plugin/integrations/cups.md) | Track print queue and printer status |
| [HDD temperature](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/hddtemp/integrations/hdd_temperature.md) | Monitor hard drive temperature sensors |
| [Intel GPU](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/intelgpu/integrations/intel_gpu.md) | Track Intel integrated GPU utilization |
| [Intelligent Platform Management Interface (IPMI)](https://github.com/netdata/netdata/blob/master/src/collectors/freeipmi.plugin/integrations/intelligent_platform_management_interface_ipmi.md) | Monitor server hardware health via IPMI |
| [Linux Sensors](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/sensors/integrations/linux_sensors.md) | Track temperature, voltage, and fan sensors |
| [Nvidia GPU](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/nvidia_smi/integrations/nvidia_gpu.md) | Monitor Nvidia GPU utilization and memory |
| [S.M.A.R.T.](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/smartctl/integrations/s.m.a.r.t..md) | Track disk health using S.M.A.R.T. attributes |

### Synthetic Checks

| Integration | Description |
|-------------|-------------|
| [Blackbox](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/blackbox.md) | Probe endpoints via HTTP, TCP, ICMP, and DNS |
| [Domain expiration date](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/whoisquery/integrations/domain_expiration_date.md) | Check domain name expiration dates |
| [HTTP Endpoints](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/httpcheck/integrations/http_endpoints.md) | Monitor HTTP/HTTPS endpoint availability and response time |
| [Idle OS Jitter](https://github.com/netdata/netdata/blob/master/src/collectors/idlejitter.plugin/integrations/idle_os_jitter.md) | Measure CPU scheduling latency |
| [Ping](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/ping/integrations/ping.md) | Check host reachability and latency via ICMP |
| [TCP/UDP Endpoints](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/portcheck/integrations/tcp-udp_endpoints.md) | Monitor TCP/UDP port availability |
| [X.509 certificate](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/x509check/integrations/x.509_certificate.md) | Check SSL/TLS certificate expiration |

### eBPF

| Integration | Description |
|-------------|-------------|
| [eBPF Cachestat](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_cachestat.md) | Monitor Linux page cache statistics |
| [eBPF Disk](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_disk.md) | Track disk I/O operations at kernel level |
| [eBPF Filesystem](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_filesystem.md) | Monitor filesystem operations performance |
| [eBPF Process](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_process.md) | Track process lifecycle and resource usage |
| [eBPF Socket](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_socket.md) | Monitor network socket operations |
| [eBPF VFS](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_vfs.md) | Track virtual filesystem layer operations |

### Windows Systems

| Integration | Description |
|-------------|-------------|
| [Active Directory](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/active_directory.md) | Monitor AD domain controller performance |
| [Hyper-V](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/hyper-v.md) | Track Hyper-V hypervisor and VM metrics |
| [IIS](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/iis.md) | Monitor Internet Information Services web server |
| [MS SQL Server](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/ms_sql_server.md) | Track SQL Server database performance |
| [Memory statistics](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/memory_statistics.md) | Monitor Windows memory allocation |
| [Network Subsystem](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/network_subsystem.md) | Track Windows network interface metrics |
| [Physical and Logical Disk Performance Metrics](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/physical_and_logical_disk_performance_metrics.md) | Monitor disk I/O performance |
| [Processor](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/processor.md) | Track CPU utilization and context switches |
| [System statistics](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/system_statistics.md) | Monitor Windows system-level metrics |
| [Windows Services](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/windows_services.md) | Track Windows service status and health |

### macOS Systems

| Integration | Description |
|-------------|-------------|
| [macOS](https://github.com/netdata/netdata/blob/master/src/collectors/macos.plugin/integrations/macos.md) | Monitor macOS system metrics including CPU, memory, disk, and network |

### FreeBSD

| Integration | Description |
|-------------|-------------|
| [devstat](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/devstat.md) | Monitor FreeBSD disk I/O statistics |
| [getifaddrs](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/getifaddrs.md) | Track FreeBSD network interface metrics |
| [getmntinfo](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/getmntinfo.md) | Monitor FreeBSD filesystem mount points |
| [ipfw](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/ipfw.md) | Track FreeBSD firewall rules and traffic |
| [system.ram](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/system.ram.md) | Monitor FreeBSD RAM usage |
| [zfs](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/zfs.md) | Track FreeBSD ZFS filesystem metrics |

### APM

| Integration | Description |
|-------------|-------------|
| [Apache Airflow](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/apache_airflow.md) | Monitor Airflow workflow orchestration |
| [Apache Flink](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/apache_flink.md) | Track Apache Flink stream processing |
| [Go applications (EXPVAR)](https://github.com/netdata/netdata/blob/master/src/collectors/python.d.plugin/go_expvar/integrations/go_applications_expvar.md) | Monitor Go application metrics via expvar |
| [JMX](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/jmx.md) | Track Java Management Extensions metrics |
| [phpDaemon](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/phpdaemon/integrations/phpdaemon.md) | Monitor phpDaemon application server |

### Mail Servers

| Integration | Description |
|-------------|-------------|
| [Dovecot](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/dovecot/integrations/dovecot.md) | Monitor Dovecot IMAP/POP3 server connections |
| [Exim](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/exim/integrations/exim.md) | Track Exim mail transfer agent queues |
| [Postfix](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/postfix/integrations/postfix.md) | Monitor Postfix mail server queue size |

### Security Systems

| Integration | Description |
|-------------|-------------|
| [Crowdsec](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/crowdsec.md) | Monitor CrowdSec security engine decisions |
| [Fail2ban](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/fail2ban/integrations/fail2ban.md) | Track banned IP addresses and jails |
| [Rspamd](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/rspamd/integrations/rspamd.md) | Monitor Rspamd spam filtering engine |

### Service Discovery / Registry

| Integration | Description |
|-------------|-------------|
| [Consul](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/consul/integrations/consul.md) | Monitor Consul service mesh health checks |
| [ZooKeeper](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/zookeeper/integrations/zookeeper.md) | Track ZooKeeper distributed coordination |
| [etcd](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/etcd.md) | Monitor etcd distributed key-value store |

### Distributed Computing Systems

| Integration | Description |
|-------------|-------------|
| [Gearman](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/gearman/integrations/gearman.md) | Monitor Gearman job server queues |

### Task Queues

| Integration | Description |
|-------------|-------------|
| [Celery](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/celery.md) | Monitor Celery distributed task queue |

### Media Services

| Integration | Description |
|-------------|-------------|
| [Icecast](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/icecast/integrations/icecast.md) | Monitor Icecast streaming media server listeners |

### Gaming

| Integration | Description |
|-------------|-------------|
| [Minecraft](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/minecraft.md) | Monitor Minecraft server players and performance |
| [SpigotMC](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/spigotmc/integrations/spigotmc.md) | Track SpigotMC Minecraft server metrics |

### FTP Servers

| Integration | Description |
|-------------|-------------|
| [ProFTPD](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/proftpd.md) | Monitor ProFTPD server connections |

### Blockchain Servers

| Integration | Description |
|-------------|-------------|
| [Go-ethereum](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/geth/integrations/go-ethereum.md) | Monitor Ethereum node synchronization |

### Provisioning Systems

| Integration | Description |
|-------------|-------------|
| [Puppet](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/puppet/integrations/puppet.md) | Track Puppet configuration management runs |

### IoT Devices

| Integration | Description |
|-------------|-------------|
| [Modbus protocol](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/modbus_protocol.md) | Monitor industrial devices via Modbus |

### Telephony Servers

| Integration | Description |
|-------------|-------------|
| [OpenSIPS](https://github.com/netdata/netdata/blob/master/src/collectors/charts.d.plugin/opensips/integrations/opensips.md) | Monitor OpenSIPS SIP server calls |

### Generic Data Collection

| Integration | Description |
|-------------|-------------|
| [Pandas](https://github.com/netdata/netdata/blob/master/src/collectors/python.d.plugin/pandas/integrations/pandas.md) | Parse structured data from CSV, JSON, XML files |
| [Prometheus endpoint](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/prometheus_endpoint.md) | Collect metrics from any Prometheus exporter |
| [SNMP devices](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/snmp/integrations/snmp_devices.md) | Monitor any SNMP-enabled device |
| [Shell command](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/shell_command.md) | Execute custom scripts and parse output |

### Other

| Integration | Description |
|-------------|-------------|
| [Files and directories](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/filecheck/integrations/files_and_directories.md) | Monitor file existence, modification time, and size |