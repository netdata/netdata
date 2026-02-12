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

**Don't see what you need?** We support [Prometheus endpoints](#generic-data-collection), [SNMP devices](#generic-data-collection), [StatsD](#beyond-the-850-integrations), and [custom data sources](#generic-data-collection).


## Beyond the 850+ integrations

Netdata can monitor virtually any application through generic collectors:

- **[Prometheus collector](/src/go/plugin/go.d/collector/prometheus/README.md)** - Any application exposing Prometheus metrics
- **[StatsD collector](/src/collectors/statsd.plugin/README.md)** - Applications instrumented with [StatsD](https://blog.netdata.cloud/introduction-to-statsd/)
- **[Pandas collector](/src/collectors/python.d.plugin/pandas/README.md)** - Structured data from CSV, JSON, XML, and more

Need a dedicated integration? [Submit a feature request](https://github.com/netdata/netdata/issues/new/choose) on GitHub.


## Available Data Collection Integrations

### Databases

| Integration | Description |
|-------------|-------------|
| [4D Server](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/4d_server.md) | Monitor 4D Server performance metrics for efficient application management and optimization. |
| [ActiveMQ](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/activemq/integrations/activemq.md) | This collector monitors ActiveMQ queues and topics. |
| [Apache Pulsar](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/pulsar/integrations/apache_pulsar.md) | This collector monitors Pulsar servers. |
| [AWS RDS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/aws_rds.md) | Monitor Amazon RDS (Relational Database Service) metrics for efficient cloud database management and performance. |
| [Beanstalk](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/beanstalk/integrations/beanstalk.md) | This collector monitors Beanstalk server performance and provides detailed statistics for each tube. |
| [Cassandra](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/cassandra/integrations/cassandra.md) | This collector gathers metrics about client requests, cache hits, and many more, while also providing metrics per each thread pool. |
| [ClickHouse](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/clickhouse/integrations/clickhouse.md) | This collector retrieves performance data from ClickHouse for connections, queries, resources, replication, IO, and data operations (inserts, selects, merges) using HTTP requests and ClickHouse system tables. |
| [ClusterControl CMON](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/clustercontrol_cmon.md) | Track CMON metrics for Severalnines Cluster Control for efficient monitoring and management of database operations. |
| [CockroachDB](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/cockroachdb/integrations/cockroachdb.md) | This collector monitors CockroachDB servers. |
| [Couchbase](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/couchbase/integrations/couchbase.md) | This collector monitors Couchbase servers. |
| [CouchDB](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/couchdb/integrations/couchdb.md) | This collector monitors CouchDB servers. |
| [Elasticsearch](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/elasticsearch/integrations/elasticsearch.md) | This collector monitors the performance and health of the Elasticsearch cluster. |
| [HANA](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/hana.md) | Track SAP HANA database metrics for efficient data storage and query performance. |
| [IBM MQ](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/ibm_mq.md) | Keep tabs on IBM MQ message queue metrics for efficient message transport and performance. |
| [InfluxDB](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/influxdb.md) | Monitor InfluxDB time-series database metrics for efficient data storage and query performance. |
| [Kafka](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/kafka.md) | Keep an eye on Kafka message queue metrics for optimized data streaming and performance. |
| [Kafka Consumer Lag](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/kafka_consumer_lag.md) | Monitor Kafka consumer lag metrics for efficient message queue management and performance. |
| [Kafka ZooKeeper](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/kafka_zookeeper.md) | Monitor Kafka ZooKeeper metrics for optimized distributed coordination and management. |
| [MariaDB](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/mysql/integrations/mariadb.md) | This collector monitors the health and performance of MySQL servers and collects general statistics, replication and user metrics. |
| [MaxScale](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/maxscale/integrations/maxscale.md) | This collector monitors the activity and performance of MaxScale servers. |
| [Meilisearch](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/meilisearch.md) | Track Meilisearch search engine metrics for efficient search performance and management. |
| [Memcached](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/memcached/integrations/memcached.md) | Monitor Memcached metrics for proficient in-memory key-value store operations. |
| [Microsoft SQL Server](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/mssql/integrations/microsoft_sql_server.md) | This collector monitors the health and performance of Microsoft SQL Server instances. |
| [MongoDB](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/mongodb/integrations/mongodb.md) | This collector monitors MongoDB servers. |
| [mosquitto](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/mosquitto.md) | Keep an eye on Mosquitto MQTT broker metrics for efficient IoT message transport and performance. |
| [MS SQL Server](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/ms_sql_server.md) | This collector monitors Microsoft SQL Server statistics. |
| [MySQL](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/mysql/integrations/mysql.md) | This collector monitors the health and performance of MySQL servers and collects general statistics, replication and user metrics. |
| [NATS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/nats/integrations/nats.md) | This collector monitors the activity and performance of NATS servers. |
| [OpenSearch](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/elasticsearch/integrations/opensearch.md) | This collector monitors the performance and health of the Elasticsearch cluster. |
| [Oracle DB](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/oracledb/integrations/oracle_db.md) | This collector monitors the health and performance of Oracle DB servers and collects general statistics, replication and user metrics. |
| [Pandas](https://github.com/netdata/netdata/blob/master/src/collectors/python.d.plugin/pandas/integrations/pandas.md) | [Pandas](https://pandas.pydata.org/) is a de-facto standard in reading and processing most types of structured data in Python. |
| [Patroni](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/patroni.md) | Keep tabs on Patroni PostgreSQL high-availability metrics for efficient database management and performance. |
| [Percona MySQL](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/mysql/integrations/percona_mysql.md) | This collector monitors the health and performance of MySQL servers and collects general statistics, replication and user metrics. |
| [pgBackRest](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/pgbackrest.md) | Monitor pgBackRest PostgreSQL backup metrics for efficient database backup and management. |
| [PgBouncer](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/pgbouncer/integrations/pgbouncer.md) | This collector monitors PgBouncer servers. |
| [Pgpool-II](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/pgpool-ii.md) | Track Pgpool-II PostgreSQL middleware metrics for efficient database connection management and performance. |
| [Pika](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/pika/integrations/pika.md) | This collector monitors Pika servers. |
| [PostgreSQL](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/postgres/integrations/postgresql.md) | This collector monitors the activity and performance of Postgres servers, collects replication statistics, metrics for each database, table and index, and more. |
| [ProxySQL](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/proxysql/integrations/proxysql.md) | This collector monitors ProxySQL servers. |
| [RabbitMQ](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/rabbitmq/integrations/rabbitmq.md) | This collector monitors RabbitMQ instances. |
| [Redis](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/redis/integrations/redis.md) | This collector monitors the health and performance of Redis servers and collects general statistics, CPU and memory consumption, replication information, command statistics, and more. |
| [Redis Queue](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/redis_queue.md) | Monitor Python RQ (Redis Queue) job queue metrics for efficient task management and performance. |
| [RethinkDB](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/rethinkdb/integrations/rethinkdb.md) | It collects cluster-wide metrics such as server status, client connections, active clients, query rate, and document read/write rates. |
| [Riak KV](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/riakkv/integrations/riak_kv.md) | This collector monitors RiakKV metrics about throughput, latency, resources and more. |
| [ScyllaDB](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/scylladb.md) | Track ScyllaDB NoSQL database metrics for efficient database management and performance with Netdata's Prometheus integration. |
| [Sphinx](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/sphinx.md) | Monitor Sphinx search engine metrics for efficient search and indexing performance. |
| [SQL databases (generic)](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/sql/integrations/sql_databases_generic.md) | Metrics and charts for this collector are **entirely defined by your SQL configuration**. |
| [Typesense](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/typesense/integrations/typesense.md) | This collector monitors the overall health status and performance of your Typesense servers. |
| [VerneMQ](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/vernemq/integrations/vernemq.md) | This collector monitors VerneMQ instances. |
| [Vertica](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/vertica.md) | Monitor Vertica analytics database platform metrics for efficient database performance and management. |
| [Warp10](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/warp10.md) | Monitor Warp 10 time-series database metrics for efficient time-series data management and performance. |
| [YugabyteDB](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/yugabytedb/integrations/yugabytedb.md) | This collector monitors the activity and performance of YugabyteDB servers. |

### Web Servers and Proxies

| Integration | Description |
|-------------|-------------|
| [Apache](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/apache/integrations/apache.md) | This collector monitors the activity and performance of Apache servers, and collects metrics such as the number of connections, workers, requests and more. |
| [APIcast](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/apicast.md) | Monitor APIcast performance metrics to optimize API gateway operations and management. |
| [ASP.NET](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/asp.net.md) | This collector monitors ASP.NET applications. |
| [Envoy](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/envoy/integrations/envoy.md) | This collector monitors Envoy proxies. |
| [Gobetween](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/gobetween.md) | Track Gobetween load balancer metrics for optimized network traffic management and performance. |
| [HAProxy](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/haproxy/integrations/haproxy.md) | This collector monitors HAProxy servers. |
| [HTTPD](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/apache/integrations/httpd.md) | This collector monitors the activity and performance of Apache servers, and collects metrics such as the number of connections, workers, requests and more. |
| [IIS](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/iis.md) | This collector monitors website requests and logins. |
| [Lighttpd](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/lighttpd/integrations/lighttpd.md) | This collector monitors the activity and performance of Lighttpd servers, and collects metrics such as the number of connections, workers, requests and more. |
| [Litespeed](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/litespeed/integrations/litespeed.md) | Examine Litespeed metrics for insights into web server operations. |
| [NGINX](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/nginx/integrations/nginx.md) | This collector monitors the activity and performance of NGINX servers, and collects metrics such as the number of connections, their status, and client requests. |
| [NGINX Plus](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/nginxplus/integrations/nginx_plus.md) | This collector monitors NGINX Plus servers. |
| [NGINX Unit](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/nginxunit/integrations/nginx_unit.md) | This collector monitors the activity and performance of NGINX Unit servers, and collects metrics such as the number of connections, their status, and client requests. |
| [NGINX VTS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/nginxvts/integrations/nginx_vts.md) | This collector monitors NGINX servers with [virtual host traffic status module](https://github.com/vozlt/nginx-module-vts). |
| [PHP-FPM](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/phpfpm/integrations/php-fpm.md) | This collector monitors PHP-FPM instances. |
| [Squid](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/squid/integrations/squid.md) | This collector monitors statistics about the Squid Clients and Servers, like bandwidth and requests. |
| [Squid log files](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/squidlog/integrations/squid_log_files.md) | his collector monitors Squid servers by parsing their access log files. |
| [Tengine](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/tengine/integrations/tengine.md) | This collector monitors Tengine servers. |
| [Tomcat](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/tomcat/integrations/tomcat.md) | This collector monitors Tomcat metrics about bandwidth, processing time, threads and more. |
| [Traefik](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/traefik/integrations/traefik.md) | This collector monitors Traefik servers. |
| [uWSGI](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/uwsgi/integrations/uwsgi.md) | Monitors UWSGI worker health and performance by collecting metrics like requests, transmitted data, exceptions, and harakiris. |
| [Varnish](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/varnish/integrations/varnish.md) | This collector monitors Varnish instances, supporting both the open-source Varnish-Cache and the commercial Varnish-Plus. |
| [Web server log files](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/weblog/integrations/web_server_log_files.md) | This collector monitors web servers by parsing their log files. |

### Containers and VMs

| Integration | Description |
|-------------|-------------|
| [AWS ECS Containers](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/integrations/aws_ecs_containers.md) | Monitor AWS ECS container resource utilization — CPU, memory, disk I/O, and network — via Linux cgroups. |
| [Cilium Agent](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/cilium_agent.md) | Keep an eye on Cilium Agent metrics for optimized network security and connectivity. |
| [Cilium Operator](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/cilium_operator.md) | Monitor Cilium Operator metrics for efficient Kubernetes network security management. |
| [Cilium Proxy](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/cilium_proxy.md) | Track Cilium Proxy metrics for enhanced network security and performance. |
| [containerd Containers](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/integrations/containerd_containers.md) | Monitor containerd container resource utilization — CPU, memory, disk I/O, and network — via Linux cgroups. |
| [Containers](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/integrations/containers.md) | Monitor containers and virtual machines resource utilization — CPU, memory, disk I/O, and network — via Linux cgroups. |
| [Docker](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/docker/integrations/docker.md) | This collector monitors Docker containers state, health status and more. |
| [Docker Containers](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/integrations/docker_containers.md) | Monitor Docker container resource utilization — CPU, memory, disk I/O, and network — via Linux cgroups. |
| [Docker Engine](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/docker_engine/integrations/docker_engine.md) | This collector monitors the activity and health of Docker Engine and Docker Swarm. |
| [Docker Hub repository](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/dockerhub/integrations/docker_hub_repository.md) | This collector keeps track of DockerHub repositories statistics such as the number of stars, pulls, current status, and more. |
| [Hyper-V](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/hyper-v.md) | This collector monitors website requests and logins. |
| [Kubelet](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/k8s_kubelet/integrations/kubelet.md) | This collector monitors Kubelet instances. |
| [Kubeproxy](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/k8s_kubeproxy/integrations/kubeproxy.md) | This collector monitors Kubeproxy instances. |
| [Kubernetes API Server](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/k8s_apiserver/integrations/kubernetes_api_server.md) | This collector monitors Kubernetes API Server health, performance, and request metrics. |
| [Kubernetes Cluster State](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/k8s_state/integrations/kubernetes_cluster_state.md) | This collector monitors Kubernetes Nodes, Pods and Containers. |
| [Kubernetes Containers](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/integrations/kubernetes_containers.md) | Monitor containers and virtual machines resource utilization — CPU, memory, disk I/O, and network — via Linux cgroups. |
| [Libvirt VMs and Containers](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/integrations/libvirt_vms_and_containers.md) | Monitor libvirt-managed VM and container resource utilization — CPU, memory, disk I/O, and network — via Linux cgroups. |
| [LXC Containers](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/integrations/lxc_containers.md) | Monitor LXC/LXD/Incus container resource utilization — CPU, memory, disk I/O, and network — via Linux cgroups. |
| [Mesos](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/mesos.md) | Monitor Apache Mesos cluster manager metrics for efficient resource management and performance. |
| [Nomad Containers](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/integrations/nomad_containers.md) | Monitor HashiCorp Nomad container resource utilization — CPU, memory, disk I/O, and network — via Linux cgroups. |
| [OpenShift Containers](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/integrations/openshift_containers.md) | Monitor Red Hat OpenShift container resource utilization — CPU, memory, disk I/O, and network — via Linux cgroups. |
| [OpenStack VMs](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/integrations/openstack_vms.md) | Monitor OpenStack Nova virtual machine resource utilization — CPU, memory, disk I/O, and network — via Linux cgroups. |
| [oVirt VMs](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/integrations/ovirt_vms.md) | Monitor oVirt/RHEV virtual machine resource utilization — CPU, memory, disk I/O, and network — via Linux cgroups. |
| [Podman](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/podman.md) | Keep tabs on Podman container runtime metrics for efficient container management and performance. |
| [Podman Containers](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/integrations/podman_containers.md) | Monitor Podman container resource utilization — CPU, memory, disk I/O, and network — via Linux cgroups. |
| [Proxmox VE](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/proxmox_ve.md) | Keep tabs on Proxmox Virtual Environment metrics for efficient virtualization and container management. |
| [Proxmox VE Monitoring](https://github.com/netdata/netdata/blob/master/src/collectors/guides/proxmox/integrations/proxmox_ve_monitoring.md) | This guide describes how Netdata monitors Proxmox VE hypervisors. |
| [Proxmox VMs and Containers](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/integrations/proxmox_vms_and_containers.md) | Monitor Proxmox VE virtual machine and container resource utilization — CPU, memory, disk I/O, and network — via Linux cgroups. |
| [systemd-nspawn Containers](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/integrations/systemd-nspawn_containers.md) | Monitor systemd-nspawn container resource utilization — CPU, memory, disk I/O, and network — via Linux cgroups. |
| [vCenter Server Appliance](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/vcsa/integrations/vcenter_server_appliance.md) | This collector monitors [health statistics](https://developer.vmware.com/apis/vsphere-automation/latest/appliance/health/) of vCenter Server Appliance servers. |
| [Virtual Machines](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/integrations/virtual_machines.md) | Monitor virtual machine resource utilization — CPU, memory, disk I/O, and network — via Linux cgroups. |
| [VMware vCenter Server](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/vsphere/integrations/vmware_vcenter_server.md) | This collector monitors hosts and vms performance statistics from `vCenter` servers. |
| [Xen XCP-ng](https://github.com/netdata/netdata/blob/master/src/collectors/xenstat.plugin/integrations/xen_xcp-ng.md) | This collector monitors XenServer and XCP-ng host and domains statistics. |

### Operating Systems

| Integration | Description |
|-------------|-------------|
| [Applications](https://github.com/netdata/netdata/blob/master/src/collectors/apps.plugin/integrations/applications.md) | Monitor Applications for optimal software performance and resource usage. |
| [CPU performance](https://github.com/netdata/netdata/blob/master/src/collectors/perf.plugin/integrations/cpu_performance.md) | This collector monitors CPU performance metrics about cycles, instructions, migrations, cache operations and more. |
| [dev.cpu.0.freq](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/dev.cpu.0.freq.md) | Read current CPU Scaling frequency. |
| [eBPF Cachestat](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_cachestat.md) | Monitor Linux page cache events giving for users a general vision about how his kernel is manipulating files. |
| [eBPF DCstat](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_dcstat.md) | Monitor directory cache events per application given an overall vision about files on memory or storage device. |
| [eBPF Filedescriptor](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_filedescriptor.md) | Monitor calls for functions responsible to open or close a file descriptor and possible errors. |
| [eBPF Hardirq](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_hardirq.md) | Monitor latency for each HardIRQ available. |
| [eBPF OOMkill](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_oomkill.md) | Monitor applications that reach out of memory. |
| [eBPF Process](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_process.md) | Monitor internal memory usage. |
| [eBPF Processes](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_processes.md) | Monitor calls for function creating tasks (threads and processes) inside Linux kernel. |
| [eBPF SHM](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_shm.md) | Monitor syscall responsible to manipulate shared memory. |
| [eBPF SoftIRQ](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_softirq.md) | Monitor latency for each SoftIRQ available. |
| [eBPF SWAP](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_swap.md) | Monitors when swap has I/O events and applications executing events. |
| [Entropy](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/entropy.md) | Entropy, a measure of the randomness or unpredictability of data. |
| [FreeBSD RCTL-RACCT](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/freebsd_rctl-racct.md) | Keep an eye on FreeBSD Resource Container metrics for optimized resource management and performance. |
| [hw.intrcnt](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/hw.intrcnt.md) | Get total number of interrupts |
| [Inter Process Communication](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/inter_process_communication.md) | IPC stands for Inter-Process Communication. |
| [Interrupts](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/interrupts.md) | Monitors `/proc/interrupts`, a file organized by CPU and then by the type of interrupt. |
| [kern.cp_time](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/kern.cp_time.md) | Total CPU utilization |
| [kern.ipc.msq](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/kern.ipc.msq.md) | Collect number of IPC message Queues |
| [kern.ipc.sem](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/kern.ipc.sem.md) | Collect information about semaphore. |
| [kern.ipc.shm](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/kern.ipc.shm.md) | Collect shared memory information. |
| [Kernel Same-Page Merging](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/kernel_same-page_merging.md) | Kernel Samepage Merging (KSM) is a memory-saving feature in Linux that enables the kernel to examine the memory of different processes and identify identical pages. |
| [Linux kernel SLAB allocator statistics](https://github.com/netdata/netdata/blob/master/src/collectors/slabinfo.plugin/integrations/linux_kernel_slab_allocator_statistics.md) | Collects metrics on kernel SLAB cache utilization to monitor the low-level performance impact of workloads in the kernel. |
| [Linux ZSwap](https://github.com/netdata/netdata/blob/master/src/collectors/debugfs.plugin/integrations/linux_zswap.md) | Collects zswap performance metrics on Linux systems. |
| [macOS](https://github.com/netdata/netdata/blob/master/src/collectors/macos.plugin/integrations/macos.md) | Monitor macOS metrics for efficient operating system performance. |
| [Memory Statistics](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/memory_statistics.md) | Linux Virtual memory subsystem. |
| [Memory statistics](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/memory_statistics.md) | This collector monitors swap and memory pool statistics on Windows systems. |
| [Memory Usage](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/memory_usage.md) | `/proc/meminfo` provides detailed information about the system's current memory usage. |
| [Non-Uniform Memory Access](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/non-uniform_memory_access.md) | Information about NUMA (Non-Uniform Memory Access) nodes on the system. |
| [NUMA Architecture](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/numa_architecture.md) | This collector monitors NUMA Architecture on Windows. |
| [OpenRC](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/openrc.md) | Keep tabs on OpenRC init system metrics for efficient system startup and service management. |
| [Page types](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/page_types.md) | This integration provides metrics about the system's memory page types |
| [Pressure Stall Information](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/pressure_stall_information.md) | Introduced in Linux kernel 4.20, `/proc/pressure` provides information about system pressure stall information (PSI). |
| [Processor](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/processor.md) | This collector monitors processors statistics on host. |
| [Semaphore statistics](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/semaphore_statistics.md) | Inter-Process Communication (IPC) enables different processes to communicate and coordinate with each other. |
| [SoftIRQ statistics](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/softirq_statistics.md) | In the Linux kernel, handling of hardware interrupts is split into two halves: the top half and the bottom half. |
| [Supervisor](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/supervisord/integrations/supervisor.md) | This collector monitors Supervisor instances. |
| [System Load Average](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/system_load_average.md) | The `/proc/loadavg` file provides information about the system load average. |
| [System Memory Fragmentation](https://github.com/netdata/netdata/blob/master/src/collectors/debugfs.plugin/integrations/system_memory_fragmentation.md) | Collects memory fragmentation statistics from the Linux kernel |
| [System statistics](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/system_statistics.md) | CPU utilization, states and frequencies and key Linux system performance metrics. |
| [System statistics](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/system_statistics.md) | This collector monitors the current number of processes, threads, and context switches on Windows systems. |
| [System Uptime](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/system_uptime.md) | The amount of time the system has been up (running). |
| [system.ram](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/system.ram.md) | Show information about system memory usage. |
| [Systemd Services](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/integrations/systemd_services.md) | Monitor containers and virtual machines resource utilization — CPU, memory, disk I/O, and network — via Linux cgroups. |
| [Systemd Units](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/systemdunits/integrations/systemd_units.md) | This collector monitors the state of Systemd units and unit files. |
| [systemd-logind users](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/logind/integrations/systemd-logind_users.md) | This collector monitors number of sessions and users as reported by the `org.freedesktop.login1` DBus API. |
| [uptime](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/uptime.md) | Show period of time server is up. |
| [User Groups](https://github.com/netdata/netdata/blob/master/src/collectors/apps.plugin/integrations/user_groups.md) | This integration monitors resource utilization on a user groups context. |
| [Users](https://github.com/netdata/netdata/blob/master/src/collectors/apps.plugin/integrations/users.md) | This integration monitors resource utilization on a user context. |
| [vm.loadavg](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/vm.loadavg.md) | System Load Average |
| [vm.stats.sys.v_intr](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/vm.stats.sys.v_intr.md) | Device interrupts |
| [vm.stats.sys.v_soft](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/vm.stats.sys.v_soft.md) | Software Interrupt |
| [vm.stats.sys.v_swtch](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/vm.stats.sys.v_swtch.md) | CPU context switch |
| [vm.stats.vm.v_pgfaults](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/vm.stats.vm.v_pgfaults.md) | Collect memory page faults events. |
| [vm.stats.vm.v_swappgs](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/vm.stats.vm.v_swappgs.md) | The metric swap amount of data read from and written to SWAP. |
| [vm.swap_info](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/vm.swap_info.md) | Collect information about SWAP memory. |
| [vm.vmtotal](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/vm.vmtotal.md) | Collect Virtual Memory information from host. |
| [Windows Services](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/windows_services.md) | This collector monitors Windows Services Status and States. |
| [ZRAM](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/zram.md) | zRAM, or compressed RAM, is a block device that uses a portion of your system's RAM as a block device. |

### Networking

| Integration | Description |
|-------------|-------------|
| [8430FT modem](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/8430ft_modem.md) | Keep track of vital metrics from the MTS 8430FT modem for streamlined network performance and diagnostics. |
| [Access Points](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/ap/integrations/access_points.md) | This collector monitors various wireless access point metrics like connected clients, bandwidth, packets, transmit issues, signal strength, and bitrate for each device and its associated SSID. |
| [Bird Routing Daemon](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/bird_routing_daemon.md) | Keep an eye on Bird Routing Daemon metrics for optimized network routing and management. |
| [Chrony](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/chrony/integrations/chrony.md) | This collector monitors the system's clock performance and peers activity status |
| [Clash](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/clash.md) | Keep an eye on Clash proxy server metrics for optimized network performance and management. |
| [Conntrack](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/conntrack.md) | This integration monitors the connection tracking mechanism of Netfilter in the Linux Kernel. |
| [CoreDNS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/coredns/integrations/coredns.md) | This collector monitors CoreDNS instances. |
| [DNSBL](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/dnsbl.md) | Monitor DNSBL metrics for efficient domain reputation and security management. |
| [DNSdist](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/dnsdist/integrations/dnsdist.md) | This collector monitors DNSDist servers. |
| [Dnsmasq](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/dnsmasq/integrations/dnsmasq.md) | This collector monitors Dnsmasq servers. |
| [Dnsmasq DHCP](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/dnsmasq_dhcp/integrations/dnsmasq_dhcp.md) | This collector monitors Dnsmasq DHCP leases databases, depending on your configuration. |
| [eBPF Socket](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_socket.md) | Monitor bandwidth consumption per application for protocols TCP and UDP. |
| [Fastd](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/fastd.md) | Monitor Fastd VPN metrics for efficient virtual private network management and performance. |
| [Freifunk network](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/freifunk_network.md) | Keep tabs on Freifunk community network metrics for optimized network performance and management. |
| [FRRouting](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/frrouting.md) | Monitor Free Range Routing (FRR) metrics for optimized network routing and management. |
| [getifaddrs](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/getifaddrs.md) | Collect traffic per network interface. |
| [Hitron CODA Cable Modem](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/hitron_coda_cable_modem.md) | Track Hitron CODA cable modem metrics for optimized internet connectivity and performance. |
| [InfiniBand](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/infiniband.md) | This integration monitors InfiniBand network inteface statistics. |
| [IP Virtual Server](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/ip_virtual_server.md) | This integration monitors IP Virtual Server statistics |
| [ipfw](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/ipfw.md) | Collect information about FreeBSD firewall. |
| [IPv6 Socket Statistics](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/ipv6_socket_statistics.md) | This integration provides IPv6 socket statistics. |
| [ISC DHCP](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/isc_dhcpd/integrations/isc_dhcp.md) | This collector monitors ISC DHCP lease usage by reading the DHCP client lease database (dhcpd.leases). |
| [Keepalived](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/keepalived.md) | Track Keepalived metrics for efficient high-availability and load balancing management. |
| [Libreswan](https://github.com/netdata/netdata/blob/master/src/collectors/charts.d.plugin/libreswan/integrations/libreswan.md) | Monitor Libreswan performance for optimal IPsec VPN operations. |
| [net.inet.icmp.stats](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/net.inet.icmp.stats.md) | Collect information about ICMP traffic. |
| [net.inet.ip.stats](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/net.inet.ip.stats.md) | Collect IP stats |
| [net.inet.tcp.states](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/net.inet.tcp.states.md) | This collector is supported on all platforms. |
| [net.inet.tcp.stats](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/net.inet.tcp.stats.md) | Collect overall information about TCP connections. |
| [net.inet.udp.stats](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/net.inet.udp.stats.md) | Collect information about UDP connections. |
| [net.inet6.icmp6.stats](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/net.inet6.icmp6.stats.md) | Collect information abou IPv6 ICMP |
| [net.inet6.ip6.stats](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/net.inet6.ip6.stats.md) | Collect information abou IPv6 stats. |
| [net.isr](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/net.isr.md) | Collect information about system softnet stat. |
| [Netfilter](https://github.com/netdata/netdata/blob/master/src/collectors/nfacct.plugin/integrations/netfilter.md) | Monitor Netfilter metrics for optimal packet filtering and manipulation. |
| [Network interfaces](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/network_interfaces.md) | Monitor network interface metrics about bandwidth, state, errors and more. |
| [Network statistics](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/network_statistics.md) | This integration provides metrics from the `netstat`, `snmp` and `snmp6` modules. |
| [Network Subsystem](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/network_subsystem.md) | Monitor network interface metrics about bandwidth, state, errors and more. |
| [NextDNS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/nextdns.md) | Track NextDNS DNS resolver and security platform metrics for efficient DNS management and security. |
| [NSD](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/nsd/integrations/nsd.md) | This collector monitors NSD statistics like queries, zones, protocols, query types and more. |
| [NTPd](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/ntpd/integrations/ntpd.md) | This collector monitors the system variables of the local `ntpd` daemon (optional incl. |
| [Open vSwitch](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/open_vswitch.md) | Keep an eye on Open vSwitch software-defined networking metrics for efficient network virtualization and performance. |
| [OpenROADM devices](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/openroadm_devices.md) | Monitor OpenROADM optical transport network metrics using the NETCONF protocol for efficient network management and performance. |
| [OpenVPN](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/openvpn/integrations/openvpn.md) | This collector monitors OpenVPN servers. |
| [OpenVPN status log](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/openvpn_status_log/integrations/openvpn_status_log.md) | This collector monitors OpenVPN server. |
| [Optical modules](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/ethtool/integrations/optical_modules.md) | This collector monitors optical transceiver modules' diagnostic parameters (temperature, voltage, laser bias current, transmit/receive power levels) from network interfaces equipped with modules that support Digital Diagnostic Monitoring (DDM). |
| [Pi-hole](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/pihole/integrations/pi-hole.md) | This collector monitors Pi-hole instances using [Pi-hole API 6.0](https://ftl.pi-hole.net/master/docs/). |
| [PowerDNS Authoritative Server](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/powerdns/integrations/powerdns_authoritative_server.md) | This collector monitors PowerDNS Authoritative Server instances. |
| [PowerDNS Recursor](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/powerdns_recursor/integrations/powerdns_recursor.md) | This collector monitors PowerDNS Recursor instances. |
| [RIPE Atlas](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/ripe_atlas.md) | Keep tabs on RIPE Atlas Internet measurement platform metrics for efficient network monitoring and performance. |
| [SCTP Statistics](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/sctp_statistics.md) | This integration provides statistics about the Stream Control Transmission Protocol (SCTP). |
| [SNMP devices](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/snmp/integrations/snmp_devices.md) | This collector discovers and monitors any SNMP-enabled network device. |
| [Socket statistics](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/socket_statistics.md) | This integration provides socket statistics. |
| [SoftEther VPN Server](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/softether_vpn_server.md) | Monitor SoftEther VPN Server metrics for efficient virtual private network (VPN) management and performance. |
| [Softnet Statistics](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/softnet_statistics.md) | `/proc/net/softnet_stat` provides statistics that relate to the handling of network packets by softirq. |
| [SONiC NOS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/sonic_nos.md) | Keep tabs on Software for Open Networking in the Cloud (SONiC) metrics for efficient network switch management and performance. |
| [Starlink (SpaceX)](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/starlink_spacex.md) | Monitor SpaceX Starlink satellite internet metrics for efficient internet service management and performance. |
| [strongSwan](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/strongswan.md) | Track strongSwan VPN and IPSec metrics using the vici interface for efficient virtual private network (VPN) management and performance. |
| [Synproxy](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/synproxy.md) | This integration provides statistics about the Synproxy netfilter module. |
| [tc QoS classes](https://github.com/netdata/netdata/blob/master/src/collectors/tc.plugin/integrations/tc_qos_classes.md) | Examine tc metrics to gain insights into Linux traffic control operations. |
| [Timex](https://github.com/netdata/netdata/blob/master/src/collectors/timex.plugin/integrations/timex.md) | Examine Timex metrics to gain insights into system clock operations. |
| [Tor](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/tor/integrations/tor.md) | Tracks Tor's download and upload traffic, as well as its uptime. |
| [Ubiquiti UFiber OLT](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/ubiquiti_ufiber_olt.md) | Track Ubiquiti UFiber GPON (Gigabit Passive Optical Network) device metrics for efficient fiber-optic network management and performance. |
| [Unbound](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/unbound/integrations/unbound.md) | This collector monitors Unbound servers. |
| [WireGuard](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/wireguard/integrations/wireguard.md) | This collector monitors WireGuard VPN devices and peers traffic. |
| [Wireless network interfaces](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/wireless_network_interfaces.md) | Monitor wireless devices with metrics about status, link quality, signal level, noise level and more. |

### Cloud and DevOps

| Integration | Description |
|-------------|-------------|
| [AWS EC2 Compute instances](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/aws_ec2_compute_instances.md) | Track AWS EC2 instances key metrics for optimized performance and cost management. |
| [AWS Quota](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/aws_quota.md) | Monitor AWS service quotas for effective resource usage and cost management. |
| [BOSH](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/bosh.md) | Keep an eye on BOSH deployment metrics for improved cloud orchestration and resource management. |
| [Cloud Foundry](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/cloud_foundry.md) | Track Cloud Foundry platform metrics for optimized application deployment and management. |
| [Cloud Foundry Firehose](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/cloud_foundry_firehose.md) | Monitor Cloud Foundry Firehose metrics for comprehensive platform diagnostics and management. |
| [CloudWatch](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/cloudwatch.md) | Monitor AWS CloudWatch metrics for comprehensive AWS resource management and performance optimization. |
| [Concourse](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/concourse.md) | Monitor Concourse CI/CD pipeline metrics for optimized workflow management and deployment. |
| [Dynatrace](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/dynatrace.md) | Monitor Dynatrace APM metrics for comprehensive application performance management. |
| [GCP GCE](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/gcp_gce.md) | Keep an eye on Google Cloud Platform Compute Engine metrics for efficient cloud resource management and performance. |
| [GitLab Runner](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/gitlab_runner.md) | Keep an eye on GitLab CI/CD job metrics for efficient development and deployment management. |
| [Google Cloud Platform](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/google_cloud_platform.md) | Monitor Google Cloud Platform metrics for comprehensive cloud resource management and performance optimization. |
| [Google Stackdriver](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/google_stackdriver.md) | Track Google Stackdriver monitoring metrics for optimized cloud performance and diagnostics. |
| [Hubble](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/hubble.md) | Monitor Hubble network observability metrics for efficient network visibility and management. |
| [Jenkins](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/jenkins.md) | Track Jenkins continuous integration server metrics for efficient development and build management. |
| [Linode](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/linode.md) | Monitor Linode cloud hosting metrics for efficient virtual server management and performance. |
| [Puppet](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/puppet/integrations/puppet.md) | This collector monitors Puppet metrics, including JVM heap and non-heap memory, CPU usage, and file descriptors. |
| [Spacelift](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/spacelift.md) | Track Spacelift infrastructure-as-code (IaC) platform metrics for efficient infrastructure automation and management. |
| [Zerto](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/zerto.md) | Monitor Zerto disaster recovery and data protection metrics for efficient backup and recovery management. |

### Hardware and Sensors

| Integration | Description |
|-------------|-------------|
| [1-Wire Sensors](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/w1sensor/integrations/1-wire_sensors.md) | Monitor 1-Wire Sensors metrics with Netdata for optimal environmental conditions monitoring. |
| [AM2320](https://github.com/netdata/netdata/blob/master/src/collectors/python.d.plugin/am2320/integrations/am2320.md) | This collector monitors AM2320 sensor metrics about temperature and humidity. |
| [AMD CPU & GPU](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/amd_cpu_&_gpu.md) | Monitor AMD System Management Interface performance for optimized hardware management. |
| [AMD GPU](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/amd_gpu.md) | This integration monitors AMD GPU metrics, such as utilization, clock frequency and memory usage. |
| [APC UPS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/apcupsd/integrations/apc_ups.md) | This collector monitors Uninterruptible Power Supplies by polling the Apcupsd daemon. |
| [Christ Elektronik CLM5IP power panel](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/christ_elektronik_clm5ip_power_panel.md) | Monitor Christ Elektronik CLM5IP device metrics for efficient performance and diagnostics. |
| [CraftBeerPi](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/craftbeerpi.md) | Keep an eye on CraftBeerPi homebrewing metrics for optimized brewing process management. |
| [dev.cpu.temperature](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/dev.cpu.temperature.md) | Get current CPU temperature |
| [Dutch Electricity Smart Meter](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/dutch_electricity_smart_meter.md) | Keep tabs on Dutch smart meter P1 port metrics for efficient energy management and monitoring. |
| [Elgato Key Light devices.](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/elgato_key_light_devices..md) | Keep tabs on Elgato Key Light metrics for optimized lighting control and management. |
| [Energomera smart power meters](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/energomera_smart_power_meters.md) | Track Energomera electricity meter metrics for efficient energy management and monitoring. |
| [Hardware information collected from kernel ring.](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/hardware_information_collected_from_kernel_ring..md) | This collector monitors cpu temperature on Windows systems. |
| [IBM CryptoExpress (CEX) cards](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/ibm_cryptoexpress_cex_cards.md) | Track IBM Z Crypto Express device metrics for optimized cryptographic performance and management. |
| [IBM Z Hardware Management Console](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/ibm_z_hardware_management_console.md) | Monitor IBM Z Hardware Management Console metrics for efficient mainframe management and performance. |
| [Intel GPU](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/intelgpu/integrations/intel_gpu.md) | This collector gathers performance metrics for Intel integrated GPUs. |
| [Intelligent Platform Management Interface (IPMI)](https://github.com/netdata/netdata/blob/master/src/collectors/freeipmi.plugin/integrations/intelligent_platform_management_interface_ipmi.md) | "Monitor enterprise server sensor readings, event log entries, and hardware statuses to ensure reliable server operations." |
| [Jarvis Standing Desk](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/jarvis_standing_desk.md) | Track Jarvis standing desk usage metrics for efficient workspace ergonomics and management. |
| [Linux Sensors](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/sensors/integrations/linux_sensors.md) | This collector gathers real-time system sensor statistics using the [sysfs](https://www.kernel.org/doc/Documentation/hwmon/sysfs-interface) interface. |
| [Memory modules (DIMMs)](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/memory_modules_dimms.md) | The Error Detection and Correction (EDAC) subsystem is detecting and reporting errors in the system's memory, primarily ECC (Error-Correcting Code) memory errors. |
| [Modbus protocol](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/modbus_protocol.md) | Track Modbus RTU protocol metrics for efficient industrial automation and control performance. |
| [Nature Remo E lite devices](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/nature_remo_e_lite_devices.md) | Monitor Nature Remo E series smart home device metrics for efficient home automation and energy management. |
| [Netatmo sensors](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/netatmo_sensors.md) | Keep an eye on Netatmo smart home device metrics for efficient home automation and energy management. |
| [Nvidia Data Center GPU Manager (DCGM)](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/dcgm/integrations/nvidia_data_center_gpu_manager_dcgm.md) | This collector gathers NVIDIA GPU telemetry from a `dcgm-exporter` endpoint. |
| [Nvidia GPU](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/nvidia_smi/integrations/nvidia_gpu.md) | This collector monitors GPUs performance metrics using the [nvidia-smi](https://developer.nvidia.com/nvidia-system-management-interface) CLI tool. |
| [Personal Weather Station](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/personal_weather_station.md) | Track personal weather station metrics for efficient weather monitoring and management. |
| [Philips Hue](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/philips_hue.md) | Keep an eye on Philips Hue smart lighting metrics for efficient home automation and energy management. |
| [Pimoroni Enviro+](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/pimoroni_enviro+.md) | Track Pimoroni Enviro+ air quality and environmental metrics for efficient environmental monitoring and analysis. |
| [Power Capping](https://github.com/netdata/netdata/blob/master/src/collectors/debugfs.plugin/integrations/power_capping.md) | Collects power capping performance metrics on Linux systems. |
| [Power Supply](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/power_supply.md) | This integration monitors Power supply metrics, such as battery status, AC power status and more. |
| [Power supply](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/power_supply.md) | This collector monitors power supply statistics on Windows systems. |
| [Powerpal devices](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/powerpal_devices.md) | Keep an eye on Powerpal smart meter metrics for efficient energy management and monitoring. |
| [Radio Thermostat](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/radio_thermostat.md) | Monitor Radio Thermostat smart thermostat metrics for efficient home automation and energy management. |
| [Raritan PDU](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/raritan_pdu.md) | Monitor Raritan Power Distribution Unit (PDU) metrics for efficient power management and monitoring. |
| [Salicru EQX inverter](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/salicru_eqx_inverter.md) | Keep tabs on Salicru EQX solar inverter metrics for efficient solar energy management and monitoring. |
| [Sense Energy](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/sense_energy.md) | Keep tabs on Sense Energy smart meter metrics for efficient energy management and monitoring. |
| [Sensors](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/sensors.md) | This collector monitors sensors on Windows systems. |
| [Shelly humidity sensor](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/shelly_humidity_sensor.md) | Monitor Shelly smart home device metrics for efficient home automation and energy management. |
| [Siemens S7 PLC](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/siemens_s7_plc.md) | Monitor Siemens S7 Programmable Logic Controller (PLC) metrics for efficient industrial automation and control. |
| [SMA Inverters](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/sma_inverters.md) | Monitor SMA solar inverter metrics for efficient solar energy management and monitoring. |
| [Smart meters SML](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/smart_meters_sml.md) | Monitor Smart Message Language (SML) metrics for efficient smart metering and energy management. |
| [Solar logging stick](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/solar_logging_stick.md) | Monitor solar energy metrics using a solar logging stick for efficient solar energy management and monitoring. |
| [Solis Ginlong 5G inverters](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/solis_ginlong_5g_inverters.md) | Monitor Solis solar inverter metrics for efficient solar energy management and monitoring. |
| [Sunspec Solar Energy](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/sunspec_solar_energy.md) | Monitor SunSpec Alliance solar energy metrics for efficient solar energy management and monitoring. |
| [System thermal zone](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/system_thermal_zone.md) | This collector monitors thermal zone statistics on Windows systems. |
| [Tado smart heating solution](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/tado_smart_heating_solution.md) | Monitor Tado smart thermostat metrics for efficient home heating and cooling management. |
| [Tesla vehicle](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/tesla_vehicle.md) | Track Tesla vehicle metrics for efficient electric vehicle management and monitoring. |
| [Tesla Wall Connector](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/tesla_wall_connector.md) | Monitor Tesla Wall Connector charging station metrics for efficient electric vehicle charging management. |
| [UPS (NUT)](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/upsd/integrations/ups_nut.md) | This collector monitors Uninterruptible Power Supplies by polling the UPS daemon using the NUT network protocol. |
| [Xiaomi Mi Flora](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/xiaomi_mi_flora.md) | Keep tabs on MiFlora plant monitor metrics for efficient plant care and growth management. |

### Applications

| Integration | Description |
|-------------|-------------|
| [Active Directory](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/active_directory.md) | This collector monitors Active Directory IO and queries. |
| [Active Directory Certificate Service](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/active_directory_certificate_service.md) | This collector monitors Active Directory Certificate Services statistics. |
| [Active Directory Federation Service](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/active_directory_federation_service.md) | This collector monitors Active Directory Federation Services statistics. |
| [Alamos FE2 server](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/alamos_fe2_server.md) | Keep tabs on Alamos FE2 systems for improved performance and management. |
| [AuthLog](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/authlog.md) | Monitor authentication logs for security insights and efficient access management. |
| [BOINC](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/boinc/integrations/boinc.md) | This collector monitors task counts for the Berkeley Open Infrastructure Networking Computing (BOINC) distributed computing client. |
| [BungeeCord](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/bungeecord.md) | Track BungeeCord proxy server metrics for efficient load balancing and performance management. |
| [Celery](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/celery.md) | Keep an eye on Celery task queue metrics for optimized task processing and resource management. |
| [Chia](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/chia.md) | Track Chia blockchain metrics for optimized farming and resource allocation. |
| [ClamAV daemon](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/clamav_daemon.md) | Track ClamAV antivirus metrics for enhanced threat detection and management. |
| [Clamscan results](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/clamscan_results.md) | Monitor ClamAV scanning performance metrics for efficient malware detection and analysis. |
| [Collectd](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/collectd.md) | Monitor system and application metrics with Collectd for comprehensive performance analysis. |
| [Consul](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/consul/integrations/consul.md) | This collector monitors [key metrics](https://developer.hashicorp.com/consul/docs/agent/telemetry#key-metrics) of Consul Agents: transaction timings, leadership changes, memory usage and more. |
| [Crowdsec](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/crowdsec.md) | Monitor Crowdsec security metrics for efficient threat detection and response. |
| [Cryptowatch](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/cryptowatch.md) | Keep tabs on Cryptowatch market data metrics for comprehensive cryptocurrency market analysis. |
| [CUPS](https://github.com/netdata/netdata/blob/master/src/collectors/cups.plugin/integrations/cups.md) | Monitor CUPS performance for achieving optimal printing system operations. |
| [Discourse](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/discourse.md) | Monitor Discourse forum metrics for efficient community management and engagement. |
| [DMARC](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/dmarc.md) | Track DMARC email authentication metrics for improved email security and deliverability. |
| [Dovecot](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/dovecot/integrations/dovecot.md) | This collector monitors Dovecot metrics about sessions, logins, commands, page faults and more. |
| [etcd](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/etcd.md) | Track etcd database metrics for optimized distributed key-value store management and performance. |
| [Exim](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/exim/integrations/exim.md) | This collector monitors Exim mail queue. |
| [Fail2ban](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/fail2ban/integrations/fail2ban.md) | This collector tracks two main metrics for each jail: currently banned IPs and active failure incidents. |
| [Fluentd](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/fluentd/integrations/fluentd.md) | This collector monitors Fluentd servers. |
| [FreeRADIUS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/freeradius/integrations/freeradius.md) | This collector monitors FreeRADIUS servers. |
| [Gearman](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/gearman/integrations/gearman.md) | Monitors jobs activity, priority and available workers. |
| [GitHub API rate limit](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/github_api_rate_limit.md) | Monitor GitHub API rate limit metrics for efficient API usage and management. |
| [GitHub repository](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/github_repository.md) | Track GitHub repository metrics for optimized project and user analytics monitoring. |
| [Go applications (EXPVAR)](https://github.com/netdata/netdata/blob/master/src/collectors/python.d.plugin/go_expvar/integrations/go_applications_expvar.md) | This collector monitors Go applications that expose their metrics with the use of the `expvar` package from the Go standard library. |
| [Go-ethereum](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/geth/integrations/go-ethereum.md) | This collector monitors Go-ethereum instances. |
| [Google Pagespeed](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/google_pagespeed.md) | Keep an eye on Google PageSpeed Insights performance metrics for efficient web page optimization and performance. |
| [gpsd](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/gpsd.md) | Monitor GPSD (GPS daemon) metrics for efficient GPS data management and performance. |
| [Grafana](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/grafana.md) | Keep tabs on Grafana dashboard and visualization metrics for optimized monitoring and data analysis. |
| [Graylog Server](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/graylog_server.md) | Monitor Graylog server metrics for efficient log management and analysis. |
| [Halon](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/halon.md) | Monitor Halon email security and delivery metrics for optimized email management and protection. |
| [Homebridge](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/homebridge.md) | Monitor Homebridge smart home metrics for efficient home automation management and performance. |
| [Homey](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/homey.md) | Track Homey smart home controller metrics for efficient home automation and performance. |
| [Honeypot](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/honeypot.md) | Monitor honeypot metrics for efficient threat detection and management. |
| [IBM AIX systems Njmon](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/ibm_aix_systems_njmon.md) | Keep an eye on NJmon system performance monitoring metrics for efficient IT infrastructure management and performance. |
| [Icecast](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/icecast/integrations/icecast.md) | This collector monitors Icecast listener counts. |
| [JMX](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/jmx.md) | Track Java Management Extensions (JMX) metrics for efficient Java application management and performance. |
| [journald](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/journald.md) | Keep an eye on systemd-journald metrics for efficient log management and analysis. |
| [Kannel](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/kannel.md) | Keep tabs on Kannel SMS gateway and WAP gateway metrics for efficient mobile communication and performance. |
| [Logstash](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/logstash/integrations/logstash.md) | This collector monitors Logstash instances. |
| [loki](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/loki.md) | Track Loki metrics. |
| [Lynis audit reports](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/lynis_audit_reports.md) | Track Lynis security auditing tool metrics for efficient system security and compliance management. |
| [Minecraft](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/minecraft.md) | Track Minecraft server metrics for efficient game server management and performance. |
| [MS Exchange](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/ms_exchange.md) | This collector monitors Microsoft Exchange. |
| [mtail](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/mtail.md) | Monitor log data metrics using mtail log data extractor and parser. |
| [Nagios](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/nagios.md) | Keep tabs on Nagios network monitoring metrics for efficient IT infrastructure management and performance. |
| [NET Framework](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/net_framework.md) | This collector monitors application built with .NET |
| [Nextcloud servers](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/nextcloud_servers.md) | Keep an eye on Nextcloud cloud storage metrics for efficient file hosting and management. |
| [NRPE daemon](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/nrpe_daemon.md) | Monitor Nagios Remote Plugin Executor (NRPE) metrics for efficient system and network monitoring. |
| [OBS Studio](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/obs_studio.md) | Track OBS Studio live streaming and recording software metrics for efficient video production and performance. |
| [OpenLDAP](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/openldap/integrations/openldap.md) | This collector monitors OpenLDAP metrics about connections, operations, referrals and more. |
| [OpenSIPS](https://github.com/netdata/netdata/blob/master/src/collectors/charts.d.plugin/opensips/integrations/opensips.md) | Examine OpenSIPS metrics for insights into SIP server operations. |
| [OpenWeatherMap](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/openweathermap.md) | Track OpenWeatherMap weather data and air pollution metrics for efficient environmental monitoring and analysis. |
| [phpDaemon](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/phpdaemon/integrations/phpdaemon.md) | This collector monitors phpDaemon instances. |
| [Postfix](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/postfix/integrations/postfix.md) | This collector retrieves statistics about the Postfix mail queue using the [postqueue](https://www.postfix.org/postqueue.1.html) command-line tool. |
| [ProFTPD](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/proftpd.md) | Monitor ProFTPD FTP server metrics for efficient file transfer and server performance. |
| [Prometheus endpoint](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/prometheus_endpoint.md) | This generic Prometheus collector gathers metrics from any [`Prometheus`](https://prometheus.io/) endpoints. |
| [RADIUS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/radius.md) | Keep tabs on RADIUS (Remote Authentication Dial-In User Service) protocol metrics for efficient authentication and access management. |
| [Rspamd](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/rspamd/integrations/rspamd.md) | This collector monitors the activity and performance of Rspamd servers. |
| [SABnzbd](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/sabnzbd.md) | Monitor SABnzbd Usenet client metrics for efficient file downloads and resource management. |
| [scripts.d Scheduler](https://github.com/netdata/netdata/blob/master/src/go/plugin/scripts.d/modules/scheduler/integrations/scripts.d_scheduler.md) | The scheduler module manages the execution of jobs defined by the nagios and zabbix modules. |
| [Slurm](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/slurm.md) | Track Slurm workload manager metrics for efficient high-performance computing (HPC) and cluster management. |
| [SpigotMC](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/spigotmc/integrations/spigotmc.md) | This collector monitors SpigotMC server server performance, in the form of ticks per second average, memory utilization, and active users. |
| [StatusPage](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/statuspage.md) | Monitor StatusPage.io incident and status metrics for efficient incident management and communication. |
| [Steam](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/steam.md) | Gain insights into Steam A2S-supported game servers for performance and availability through real-time metric monitoring. |
| [Suricata](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/suricata.md) | Keep an eye on Suricata network intrusion detection and prevention system (IDS/IPS) metrics for efficient network security and performance. |
| [Sysload](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/sysload.md) | Monitor system load metrics for efficient system performance and resource management. |
| [TACACS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/tacacs.md) | Track Terminal Access Controller Access-Control System (TACACS) protocol metrics for efficient network authentication and authorization management. |
| [Tankerkoenig API](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/tankerkoenig_api.md) | Track Tankerknig API fuel price metrics for efficient fuel price monitoring and management. |
| [Twitch](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/twitch.md) | Track Twitch streaming platform metrics for efficient live streaming management and performance. |
| [Vault PKI](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/vault_pki.md) | Monitor HashiCorp Vault Public Key Infrastructure (PKI) metrics for efficient certificate management and security. |
| [VSCode](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/vscode.md) | Track Visual Studio Code editor metrics for efficient development environment management and performance. |
| [YOURLS URL Shortener](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/yourls_url_shortener.md) | Monitor YOURLS (Your Own URL Shortener) metrics for efficient URL shortening service management and performance. |
| [Zabbix Preprocessing](https://github.com/netdata/netdata/blob/master/src/go/plugin/scripts.d/modules/zabbix/integrations/zabbix_preprocessing.md) | This module runs Zabbix-style data collection jobs natively inside Netdata. |
| [ZooKeeper](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/zookeeper/integrations/zookeeper.md) | It connects to the Zookeeper instance via a TCP and executes the following commands: |

### Storage and Filesystems

| Integration | Description |
|-------------|-------------|
| [Adaptec RAID](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/adaptecraid/integrations/adaptec_raid.md) | Monitors the health of Adaptec Hardware RAID by tracking the status of logical and physical devices in your storage system. |
| [BCache](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/bcache.md) | Statistics for BCache (block layer cache) devices, including cache hit ratios, I/O operations, cache allocations, and bypass activity. |
| [BTRFS](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/btrfs.md) | This integration provides usage and error statistics from the BTRFS filesystem. |
| [Ceph](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/ceph/integrations/ceph.md) | This collector monitors the overall health status and performance of your Ceph clusters. |
| [Dell EMC ScaleIO](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/scaleio/integrations/dell_emc_scaleio.md) | This collector monitors ScaleIO (VxFlex OS) instances via VxFlex OS Gateway API. |
| [devstat](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/devstat.md) | Collect information per hard disk available on host. |
| [Disk space](https://github.com/netdata/netdata/blob/master/src/collectors/diskspace.plugin/integrations/disk_space.md) | Monitor Disk space metrics for proficient storage management. |
| [Disk Statistics](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/disk_statistics.md) | Detailed statistics for each of your system's disk devices and partitions. |
| [DMCache devices](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/dmcache/integrations/dmcache_devices.md) | This collector monitors DMCache, providing insights into capacity usage, efficiency, and activity. |
| [eBPF Disk](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_disk.md) | Measure latency for I/O events on disk. |
| [eBPF Filesystem](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_filesystem.md) | Monitor latency for main actions on filesystem like I/O events. |
| [eBPF MDflush](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_mdflush.md) | Monitor when flush events happen between disks. |
| [eBPF Mount](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_mount.md) | Monitor calls for mount and umount syscall. |
| [eBPF Sync](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_sync.md) | Monitor syscall responsible to move data from memory to storage device. |
| [eBPF VFS](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_vfs.md) | Monitor I/O events on Linux Virtual Filesystem. |
| [EOS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/eos.md) | Monitor CERN EOS metrics for efficient storage management. |
| [FreeBSD NFS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/freebsd_nfs.md) | Monitor FreeBSD Network File System metrics for efficient file sharing management and performance. |
| [Generic storage enclosure tool](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/generic_storage_enclosure_tool.md) | Monitor storage enclosure metrics for efficient storage device management and performance. |
| [getmntinfo](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/getmntinfo.md) | Collect information per mount point. |
| [Hadoop Distributed File System (HDFS)](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/hdfs/integrations/hadoop_distributed_file_system_hdfs.md) | This collector monitors HDFS nodes. |
| [HDD temperature](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/hddtemp/integrations/hdd_temperature.md) | This collector monitors disk temperatures. |
| [HPE Smart Arrays](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/hpssa/integrations/hpe_smart_arrays.md) | Monitors the health of HPE Smart Arrays by tracking the status of controllers, arrays, logical and physical drives in your storage system. |
| [IBM Spectrum](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/ibm_spectrum.md) | Monitor IBM Spectrum storage metrics for efficient data management and performance. |
| [IBM Spectrum Virtualize](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/ibm_spectrum_virtualize.md) | Monitor IBM Spectrum Virtualize metrics for efficient storage virtualization and performance. |
| [IPFS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/ipfs/integrations/ipfs.md) | This collector monitors IPFS daemon health and network activity. |
| [Lustre metadata](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/lustre_metadata.md) | Keep tabs on Lustre clustered file system for efficient management and performance. |
| [LVM logical volumes](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/lvm/integrations/lvm_logical_volumes.md) | This collector monitors the health of LVM logical volumes. |
| [MD RAID](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/md_raid.md) | This integration monitors the status of MD RAID devices. |
| [MegaCLI MegaRAID](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/megacli/integrations/megacli_megaraid.md) | Monitors the health of MegaCLI Hardware RAID by tracking the status of RAID adapters, physical drives, and backup batteries in your storage system. |
| [MogileFS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/mogilefs.md) | Monitor MogileFS distributed file system metrics for efficient storage management and performance. |
| [Netapp ONTAP API](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/netapp_ontap_api.md) | Keep tabs on NetApp ONTAP storage system metrics for efficient data storage management and performance. |
| [NetApp Solidfire](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/netapp_solidfire.md) | Track NetApp Solidfire storage system metrics for efficient data storage management and performance. |
| [NFS Client](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/nfs_client.md) | This integration provides statistics from the Linux kernel's NFS Client. |
| [NFS Server](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/nfs_server.md) | This integration provides statistics from the Linux kernel's NFS Server. |
| [NVMe devices](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/nvme/integrations/nvme_devices.md) | This collector monitors the health of NVMe devices. |
| [Physical and Logical Disk Performance Metrics](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/physical_and_logical_disk_performance_metrics.md) | Detailed statistics for all disk devices and volumes. |
| [S.M.A.R.T.](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/smartctl/integrations/s.m.a.r.t..md) | This collector monitors the health status of storage devices by analyzing S.M.A.R.T. |
| [Samba](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/samba/integrations/samba.md) | This collector monitors Samba syscalls and SMB2 calls. |
| [StoreCLI RAID](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/storcli/integrations/storecli_raid.md) | Monitors the health of StoreCLI Hardware RAID by tracking the status of RAID adapters, physical drives, and backup batteries in your storage system. |
| [Storidge](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/storidge.md) | Keep an eye on Storidge storage metrics for efficient storage management and performance. |
| [Synology ActiveBackup](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/synology_activebackup.md) | Track Synology Active Backup metrics for efficient backup and data protection management. |
| [zfs](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/zfs.md) | Collect metrics for ZFS filesystem |
| [ZFS Adaptive Replacement Cache](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/zfs_adaptive_replacement_cache.md) | This integration monitors ZFS Adadptive Replacement Cache (ARC) statistics. |
| [ZFS Pools](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/zfspool/integrations/zfs_pools.md) | This collector monitors the health and space usage of ZFS pools using the command line tool [zpool](https://openzfs.github.io/openzfs-docs/man/master/8/zpool-list.8.html). |

### Synthetic Testing

| Integration | Description |
|-------------|-------------|
| [Blackbox](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/blackbox.md) | Track external service availability and response times with Blackbox monitoring. |
| [DNS query](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/dnsquery/integrations/dns_query.md) | This module monitors DNS query round-trip time (RTT). |
| [Domain expiration date](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/whoisquery/integrations/domain_expiration_date.md) | This collector monitors the remaining time before the domain expires. |
| [Files and directories](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/filecheck/integrations/files_and_directories.md) | This collector monitors the existence, last modification time, and size of arbitrary files and directories on the system. |
| [HTTP Endpoints](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/httpcheck/integrations/http_endpoints.md) | This collector monitors HTTP servers availability status and response time. |
| [Idle OS Jitter](https://github.com/netdata/netdata/blob/master/src/collectors/idlejitter.plugin/integrations/idle_os_jitter.md) | Monitor delays in timing for user processes caused by scheduling limitations to optimize the system to run latency sensitive applications with minimal jitter, improving consistency and quality of service. |
| [IOPing](https://github.com/netdata/netdata/blob/master/src/collectors/ioping.plugin/integrations/ioping.md) | Monitor IOPing metrics for efficient disk I/O latency tracking. |
| [Monit](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/monit/integrations/monit.md) | This collector monitors status of Monit's service checks. |
| [MQTT Blackbox](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/mqtt_blackbox.md) | Track MQTT message transport performance using blackbox testing methods. |
| [Nagios Plugins](https://github.com/netdata/netdata/blob/master/src/go/plugin/scripts.d/modules/nagios/integrations/nagios_plugins.md) | This module runs unmodified [Nagios plugins](https://www.nagios-plugins.org/) inside Netdata without any changes to the plugins themselves. |
| [Ping](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/ping/integrations/ping.md) | This module measures round-trip time and packet loss by sending ping messages to network hosts. |
| [Site 24x7](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/site_24x7.md) | Monitor Site24x7 website and infrastructure monitoring metrics for efficient performance tracking and management. |
| [TCP/UDP Endpoints](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/portcheck/integrations/tcp-udp_endpoints.md) | Collector for monitoring service availability and response time. |
| [Uptimerobot](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/uptimerobot.md) | Monitor UptimeRobot website uptime monitoring metrics for efficient website availability tracking and management. |
| [X.509 certificate](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/x509check/integrations/x.509_certificate.md) | This collectors monitors x509 certificates expiration time and revocation status. |
