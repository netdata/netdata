# 11. Stock Alerts

Netdata ships with a comprehensive library of pre-configured alerts covering system resources, applications, containers, and hardware monitoring. These alerts are enabled by default and require no configuration to provide immediate visibility into common problems.

:::note

Stock alerts follow the principle of conservative defaults. They are tuned to detect genuine issues while avoiding noise from normal operational variation. However, every environment is unique—use these as a starting point and adjust thresholds based on your specific requirements.

:::

## Here

| Section | Focus Area |
|---------|------------|
| **[11.1 System Resource Alerts](#111-system-resource-alerts)** | CPU, memory, disk space, network, load averages |
| **[11.2 Container Alerts](#112-container-and-orchestration-alerts)** | Docker containers, Kubernetes pods, cgroup metrics |
| **[11.3 Application Alerts](#113-application-alerts)** | MySQL, PostgreSQL, Redis, nginx, Apache, and more |
| **[11.4 Network Alerts](#114-network-and-connectivity-alerts)** | Interface errors, packet drops, bandwidth utilization |
| **[11.5 Hardware Alerts](#115-hardware-and-sensor-alerts)** | RAID controllers, UPS battery, SMART disk status |
| **[11.6 Special Monitors](#116-netdata-infrastructure-and-special-monitors)** | DB engine, web logs, IOPing, 100+ platforms |

## 11.1 System Resource Alerts

System resource alerts cover the fundamental building blocks of any server: CPU, memory, disk, network, and load. These alerts apply to every Netdata node and provide the baseline monitoring that every infrastructure should have.

:::note

System resource alerts are enabled by default on all Netdata installations. These alerts are foundational and apply to all nodes regardless of their role.

:::

### CPU Alerts

The CPU alerts monitor utilization, saturation, and scheduling behavior.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **10min_cpu_usage** | Primary CPU alert tracking aggregate usage over 10-minute window | `system.cpu` | WARN > 75%, CRIT > 85% |
| **10min_cpu_iowait** | Monitors time spent waiting for I/O operations | `system.cpu` | WARN > 20%, CRIT > 40% |
| **20min_steal_cpu** | Tracks time stolen by other VMs in cloud/VPS environments | `system.cpu` | WARN > 10%, CRIT > 20% |

### Memory Alerts

Memory monitoring balances three competing concerns: availability for new allocations, pressure on cached data, and swapping activity.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **ram_in_use** | Tracks utilization percentage | `system.ram` | WARN > 80%, CRIT > 90% |
| **ram_available** | Monitors actual available memory | `mem.available` | WARN < 15%, CRIT < 10% |
| **oom_kill** | Number of OOM kills in the last 30 minutes | `mem.oom_kill` | WARN > 0 |
| **30min_ram_swapped_out** | Percentage of RAM swapped in last 30 minutes | `mem.swapio` | WARN > 20%, CRIT > 30% |
| **used_swap** | Swap memory utilization | `mem.swap` | WARN > 80%, CRIT > 90% |

### Disk Space Alerts

Disk space monitoring addresses space availability and inode exhaustion.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **disk_space_usage** | Tracks percentage of allocated space across filesystems | `disk.space` | WARN > 80%, CRIT > 90% |
| **disk_inode_usage** | Tracks inode exhaustion for filesystems with many small files | `disk.inodes` | WARN > 80%, CRIT > 90% |
| **10min_disk_utilization** | Average disk utilization over 10 minutes | `disk.util` | WARN > 68.6%, CRIT > 98% |
| **10min_disk_backlog** | Average disk backlog over 10 minutes | `disk.backlog` | WARN > 3500ms, CRIT > 5000ms |
| **out_of_disk_space_time** | Estimated hours until disk runs out of space | `disk.space` | WARN < 48h, CRIT < 24h |
| **out_of_disk_inodes_time** | Estimated hours until disk runs out of inodes | `disk.inodes` | WARN < 48h, CRIT < 24h |

### Load Average Alerts

System load alerts for capacity planning.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **load_cpu_number** | Number of active CPU cores in the system | `system.load` | Calculation |
| **load_average_15** | System load average for past 15 minutes | `system.load` | WARN > 175% of CPUs |
| **load_average_5** | System load average for past 5 minutes | `system.load` | WARN > 350% of CPUs |
| **load_average_1** | System load average for past 1 minute | `system.load` | WARN > 700% of CPUs |

### Process Alerts

Process and file descriptor monitoring.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **active_processes** | System PID space utilization | `system.active_processes` | WARN > 85%, CRIT > 90% |
| **system_file_descriptors_utilization** | System file descriptors utilization | `system.file_descriptors` | WARN > 80%, CRIT > 90% |

### Kernel and Synchronization Alerts

Low-level kernel and time synchronization alerts.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **lowest_entropy** | Available entropy for cryptographic operations | `kernel.entropy` | WARN < 100 |
| **system_clock_sync_state** | Clock synchronization status | `timex.ntp_offset` | Various |
| **sync_freq** | System clock drift frequency adjustment | `timex.tick_mode` | Various |
| **semaphores_used** | System V semaphores in use | `ipc.semaphore` | Various |
| **semaphore_arrays_used** | System V semaphore arrays in use | `ipc.semaphore_array` | Various |

## 11.2 Container and Orchestration Alerts

Container and orchestration alerts address the unique monitoring requirements of dynamic infrastructure.

:::note

Container alerts require the appropriate collector to be enabled. Check the **Collectors** tab in the dashboard to verify your container collectors are running.

:::

### Docker Container Alerts

Docker container alerts monitor both the runtime state and resource consumption.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **docker_container_unhealthy** | Tracks container health status from Docker daemon | `docker.container_health_status` | WARN > 0 |
| **docker_container_down** | Tracks running state of each container | `docker.container_state` | WARN > 0 (exited) |

### Kubernetes Pod Alerts

Kubernetes pod alerts require the Netdata Kubernetes collector.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **cgroup_10min_cpu_usage** | Container CPU utilization over 10 minutes | `cgroup.cpu` | WARN/CRIT based on limits |
| **cgroup_ram_in_use** | Container memory utilization | `cgroup.mem_usage` | WARN/CRIT based on limits |
| **k8s_cgroup_10min_cpu_usage** | K8s container CPU usage approaching limit | `cgroup.cpu_limit` | WARN > 80%, CRIT > 90% |
| **k8s_cgroup_ram_in_use** | K8s container memory usage approaching limit | `cgroup.mem_usage` | WARN > 80%, CRIT > 90% |

### Kubernetes State Alerts

Alerts for Kubernetes workload health and deployment status.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **k8s_state_cronjob_last_execution_failed** | CronJob last execution status | `k8s_state.cronjob` | CRIT failed |
| **k8s_state_deployment_condition_available** | Deployment availability condition | `k8s_state.deployment` | CRIT not available |

### Kubernetes Node (Kubelet) Alerts

Kubernetes node alerts available through kubelet metrics.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **kubelet_node_config_error** | Kubelet node configuration error status | `k8s_kubelet.kubelet_node_config_error` | CRIT > 0 (config error) |
| **kubelet_10s_pleg_relist_latency_quantile_05** | PLEG relist latency 50th percentile | `k8s_kubelet.pleg` | WARN/C |
| **kubelet_10s_pleg_relist_latency_quantile_09** | PLEG relist latency 90th percentile | `k8s_kubelet.pleg` | WARN/C |
| **kubelet_10s_pleg_relist_latency_quantile_099** | PLEG relist latency 99th percentile | `k8s_kubelet.pleg` | WARN/C |
| **kubelet_1m_pleg_relist_latency_quantile_05** | PLEG relist latency 50th percentile (1m avg) | `k8s_kubelet.pleg` | WARN/C |
| **kubelet_1m_pleg_relist_latency_quantile_09** | PLEG relist latency 90th percentile (1m avg) | `k8s_kubelet.pleg` | WARN/C |
| **kubelet_1m_pleg_relist_latency_quantile_099** | PLEG relist latency 99th percentile (1m avg) | `k8s_kubelet.pleg` | WARN/C |
| **kubelet_operations_error** | Kubelet operations error count | `k8s_kubelet.operations` | WARN > 0 |
| **kubelet_token_requests** | Kubelet token request metrics | `k8s_kubelet.token` | Various |

## 11.3 Application Alerts

Application alerts cover databases, web servers, message queues, caching systems, and other infrastructure services. Each application integration includes domain-specific alerts for health, performance, and availability.

:::note

Each collector integration below documents all its available alerts. Visit the linked integration pages to see detailed alert configurations.

:::

### Database Integrations

Full alert catalogs for database systems.

| Integration | Alerts Covered |
|------------|---------------|
| [MySQL & MariaDB](/docs/src/go/plugin/go.d/collector/mysql/integrations/mysql.md) | Slow queries, table locks, connections, replication lag, Galera cluster |
| [PostgreSQL](/docs/src/go/plugin/go.d/collector/postgres/integrations/postgres.md) | Connections, locks, deadlocks, cache IO, rollbacks, txid exhaustion |
| [MongoDB](/docs/src/go/plugin/go.d/collector/mongodb/integrations/mongodb.md) | Connections, memory, operations, replication |
| [Redis](/docs/src/go/plugin/go.d/collector/redis/integrations/redis.md) | Connected clients, memory, eviction, replication, persistence |
| [ClickHouse](/docs/src/go/plugin/go.d/collector/clickhouse/integrations/clickhouse.md) | Delayed inserts, queries, partitions, replication |
| [Cassandra](/docs/src/go/plugin/go.d/collector/cassandra/integrations/cassandra.md) | Key cache, row cache, compaction, threads |
| [CockroachDB](/docs/src/go/plugin/go.d/collector/cockroachdb/integrations/cockroachdb.md) | File descriptors, ranges, storage |

### Web Server Integrations

HTTP server and reverse proxy monitoring.

| Integration | Alerts Covered |
|------------|---------------|
| [nginx](/docs/src/go/plugin/go.d/collector/nginx/integrations/nginx.md) | Requests, connections, response status |
| [Apache](/docs/src/go/plugin/go.d/collector/apache/integrations/apache.md) | Requests, workers, idle workers, bytes |
| [Traefik](/docs/src/go/plugin/go.d/collector/traefik/integrations/traefik.md) | Requests, response time, retries |
| [HAProxy](/docs/src/go/plugin/go.d/collector/haproxy/integrations/haproxy.md) | Backend health, requests, sessions |
| [Varnish](/docs/src/go/plugin/go.d/collector/varnish/integrations/varnish.md) | Client requests, backend fetches, cache hits |
| [Envoy](/docs/src/go/plugin/go.d/collector/envoy/integrations/envoy.md) | Downstream/upstream connections, requests |

### Message Queue & Streaming

Queues, pub/sub, and streaming platform alerts.

| Integration | Alerts Covered |
|------------|---------------|
| [RabbitMQ](/docs/src/go/plugin/go.d/collector/rabbitmq/integrations/rabbitmq.md) | Node down, network partition, memory/disk alarms, queue minority, unhealthy status |
| [Kafka](/docs/src/go/plugin/go.d/collector/prometheus/integrations/kafka.md) | Consumer lag, under-replicated partitions |
| [Pulsar](/docs/src/go/plugin/go.d/collector/pulsar/integrations/pulsar.md) | Messages, subscriptions, ledgers |
| [NATS](/docs/src/go/plugin/go.d/collector/nats/integrations/nats.md) | Connections, subs, pubs, errors |
| [Vernemq](/docs/src/go/plugin/go.d/collector/vernemq/integrations/vernemq.md) | MQTT connections, publishes, drops |

### Caching & Key-Value Stores

Fast data access layer alerting.

| Integration | Alerts Covered |
|------------|---------------|
| [Memcached](/docs/src/go/plugin/go.d/collector/memcached/integrations/memcached.md) | Hit rate, memory, evictions |
| [Elasticsearch](/docs/src/go/plugin/go.d/collector/elasticsearch/integrations/elasticsearch.md) | Cluster health, indices, search time |
| [Consul](/docs/src/go/plugin/go.d/collector/consul/integrations/consul.md) | Raft leader, health checks, license |
| [Etcd](/docs/src/go/plugin/go.d/collector/prometheus/integrations/etcd.md) | Leader, disk, RPC, WAL |
| [ZooKeeper](/docs/src/go/plugin/go.d/collector/zookeeper/integrations/zookeeper.md) | Requests, connections, znodes |

### DNS & DHCP

Name resolution and address assignment.

| Integration | Alerts Covered |
|------------|---------------|
| [DNS Query](/docs/src/go/plugin/go.d/collector/dnsquery/integrations/dnsquery.md) | Query status, resolution failures |
| [Unbound](/docs/src/go/plugin/go.d/collector/unbound/integrations/unbound.md) | Request list, cache, threads |
| [PowerDNS Recursor](/docs/src/go/plugin/go.d/collector/powerdns_recursor/integrations/powerdns_recursor.md) | Queries, cache, DNSSEC |
| [ISC DHCP](/docs/src/go/plugin/go.d/collector/isc_dhcpd/integrations/isc_dhcpd.md) | Pool utilization, lease time |
| [Dnsmasq](/docs/src/go/plugin/go.d/collector/dnsmasq/integrations/dnsmasq.md) | Cache, queries, DHCP |

### Infrastructure Services

Additional infrastructure monitoring.

| Integration | Alerts Covered |
|------------|---------------|
| [ProxySQL](/docs/src/go/plugin/go.d/collector/proxysql/integrations/proxysql.md) | Backend connections, shunning |
| [PgBouncer](/docs/src/go/plugin/go.d/collector/pgbouncer/integrations/pgbouncer.md) | Client connections, pooler stats |
| [HDFS](/docs/src/go/plugin/go.d/collector/hdfs/integrations/hdfs.md) | Capacity, blocks, nodes |
| [Prometheus](/docs/src/go/plugin/go.d/collector/prometheus/integrations/prometheus.md) | Remote write, queries, targets |
| [SSH](/docs/src/go/plugin/go.d/collector/prometheus/integrations/ssh.md) | Connection, auth, sessions |
| [Consul](/docs/src/go/plugin/go.d/collector/consul/integrations/consul.md) | Autopilot, raft leader, GC pause, health checks |
| [ClickHouse](/docs/src/go/plugin/go.d/collector/clickhouse/integrations/clickhouse.md) | Inserts, distributed queries, replication lag, partitions |
| [CockroachDB](/docs/src/go/plugin/go.d/collector/cockroachdb/integrations/cockroachdb.md) | File descriptors, unavailable/underreplicated ranges, storage |
| [Beanstalk](/docs/src/go/plugin/go.d/collector/beanstalk/integrations/beanstalk.md) | Buried jobs |
| [Gearman](/docs/src/go/plugin/go.d/collector/gearman/integrations/gearman.md) | Job queue status |
| [Freeradius](/docs/src/go/plugin/go.d/collector/freeradius/integrations/freeradius.md) | Authentication requests, responses, rejects |
| [IPFS](/docs/src/go/plugin/go.d/collector/ipfs/integrations/ipfs.md) | Datastore usage, bandwidth, peers |
| [PostFix](/docs/src/go/plugin/go.d/collector/postfix/integrations/postfix.md) | Mail queue statistics |
| [Squid](/docs/src/go/plugin/go.d/collector/squid/integrations/squid.md) | Cache utilization, HTTP traffic |
| [Squid Logs](/docs/src/go/plugin/go.d/collector/squidlog/integrations/squidlog.md) | Access log parsing |
| [LiteSpeed](/docs/src/go/plugin/go.d/collector/litespeed/integrations/litespeed.md) | Requests, connections, hit ratio |
| [CoreDNS](/docs/src/go/plugin/go.d/collector/coredns/integrations/coredns.md) | DNS requests, responses, errors |
| [Fail2Ban](/docs/src/go/plugin/go.d/collector/fail2ban/integrations/fail2ban.md) | Jail status, banned IPs |
| [Supervisord](/docs/src/go/plugin/go.d/collector/supervisord/integrations/supervisord.md) | Supervised process health |

## 11.4 Network and Connectivity Alerts

Network alerts cover interface statistics, endpoint monitoring, firewall state, and connectivity health.

:::note

Network connectivity alerts (ping, port check, HTTP) require specific endpoints to be configured. Interface and TCP/UDP alerts apply to all systems.

:::

### ICMP and Latency Monitoring

Host reachability and network latency monitoring.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **ping_host_latency** | Round-trip time to target host | `ping.host_rtt` | WARN > 500ms, CRIT > 1000ms |
| **ping_packet_loss** | Percentage of packets without responses | `ping.host_packet_loss` | WARN > 5%, CRIT > 10% |
| **ping_host_reachable** | Host reachability status | `ping.host_packet_loss` | CRIT == 0 (not reachable) |

### Port and Service Monitoring

TCP connection testing and service availability.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **portcheck_connection_fails** | Failed connection attempts | `portcheck.status` | CRIT > 40% failed |
| **portcheck_connection_timeouts** | Connection establishment timeouts | `portcheck.status` | WARN > 10% timeout, CRIT > 40% timeout |
| **portcheck_service_reachable** | Overall service reachability | `portcheck.status` | CRIT < 75% success |

### SSL Certificate Monitoring

TLS certificate validity and expiration.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **x509check_days_until_expiration** | Days until certificate expires | `x509check.time_until_expiration` | WARN < 14 days, CRIT < 7 days |
| **x509check_revocation_status** | Certificate revocation status | `x509check.revocation_status` | CRIT revoked |
| **whoisquery_days_until_expiration** | Domain registration expiry | `whoisquery.days_left` | WARN < 30 days, CRIT < 7 days |

### DNS Resolution Monitoring

DNS query and resolution health.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **dns_query_query_status** | DNS resolution success/failure | `dns_query.query_status` | WARN != 1 (failed) |
| **unbound_request_list_dropped** | Dropped requests due to cache full | `unbound.request_list` | WARN > 0 |
| **unbound_request_list_overwritten** | Overwritten cache entries | `unbound.request_list` | WARN > 0 |

### HTTP Endpoint Monitoring

Web service availability and response validation.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **httpcheck_web_service_bad_status** | Non-2xx HTTP responses | `httpcheck.status` | WARN >= 10% bad, CRIT >= 40% bad |
| **httpcheck_web_service_timeouts** | HTTP request timeouts | `httpcheck.status` | WARN >= 10% timeout, CRIT >= 40% |
| **httpcheck_web_service_up** | Overall HTTP service availability | `httpcheck.status` | CRIT not responding |
| **httpcheck_web_service_bad_content** | Unexpected response content | `httpcheck.status` | CRIT bad content |
| **httpcheck_web_service_bad_header** | Malformed response headers | `httpcheck.header` | CRIT != 0 |
| **httpcheck_web_service_no_connection** | Unable to connect | `httpcheck.connect` | CRIT != 0 |

### Network Interface Statistics

Interface-level traffic and error monitoring.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **1m_received_traffic_overflow** | Inbound utilization percentage | `net.net` | WARN > 85%, CRIT > 90% |
| **1m_sent_traffic_overflow** | Outbound utilization percentage | `net.net` | WARN > 85%, CRIT > 90% |
| **inbound_packets_dropped_ratio** | Dropped inbound packets ratio | `net.drops` | WARN >= 2% |
| **outbound_packets_dropped_ratio** | Dropped outbound packets ratio | `net.drops` | WARN >= 2% |
| **wifi_inbound_packets_dropped_ratio** | WiFi inbound drop ratio | `net.drops` | WARN >= 10% |
| **wifi_outbound_packets_dropped_ratio** | WiFi outbound drop ratio | `net.drops` | WARN >= 10% |
| **10min_fifo_errors** | Network FIFO buffer errors | `net.fifo` | WARN > 0 |
| **1m_received_packets_storm** | Sudden spike in received packets | `net.packets` | WARN > 200%, CRIT > 5000% |

### TCP Connection and Queue Monitoring

Connection pool, queue overflow, and socket statistics.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **tcp_connections** | TCP connections utilization | `ip.tcpsock` | WARN > 60%, CRIT > 80% |
| **tcp_memory** | TCP memory consumption | `ip.tcpmemory` | Various |
| **tcp_orphans** | Orphan TCP sockets | `ip.tcporphan` | Various |
| **1m_tcp_accept_queue_overflows** | TCP accept queue overflows | `ip.tcp_accept_queue` | WARN > 1 |
| **1m_tcp_accept_queue_drops** | TCP accept queue drops | `ip.tcp_accept_queue` | WARN > 1 |
| **1m_tcp_syn_queue_drops** | SYN queue drops (no cookies) | `ip.tcp_syn_queue` | CRIT > 0 |
| **1m_tcp_syn_queue_cookies** | SYN cookies issued | `ip.tcp_syn_queue` | WARN > 1 |

### TCP Reset and Error Rates

Connection failure and reset monitoring.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **1m_ip_tcp_resets_sent** | TCP RSTs sent (1-min avg) | `ip.tcphandshake` | Various |
| **10s_ip_tcp_resets_sent** | TCP RSTs sent (10-sec burst) | `ip.tcphandshake` | Dynamic threshold |
| **1m_ip_tcp_resets_received** | TCP RSTs received (1-min avg) | `ip.tcphandshake` | Various |
| **10s_ip_tcp_resets_received** | TCP RSTs received (10-sec burst) | `ip.tcphandshake` | Dynamic threshold |

### UDP Error Monitoring

UDP buffer and transmission errors.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **1m_ipv4_udp_receive_buffer_errors** | UDP receive buffer errors | `ipv4.udperrors` | WARN > 0 |
| **1m_ipv4_udp_send_buffer_errors** | UDP send buffer errors | `ipv4.udperrors` | WARN > 0 |

### Firewall and Connection Tracking

Netfilter connection tracking and state.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **netfilter_conntrack_full** | Connection tracker nearly full | `nf.conntrack` | WARN > 90% of max |

### Softnet and Processing Backlogs

Kernel network processing queue monitoring.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **1min_netdev_backlog_exceeded** | Dropped packets (netdev backlog) | `system.softnet_stat` | WARN > 0 |
| **1min_netdev_budget_ran_outs** | Budget run-outs (softirq) | `system.softnet_stat` | WARN > 0 |
| **10min_netisr_backlog_exceeded** | NetISR queue drops (FreeBSD) | `system.softnet_stat` | WARN > 0 |

## 11.5 Hardware and Sensor Alerts

Hardware monitoring protects physical infrastructure health: storage arrays, power supplies, cooling systems, and drive SMART status.

:::note

Hardware monitoring requires collector support for your specific platform. Enable IPMI, SMART, and sensor collectors appropriate to your hardware.

:::

### RAID Array Controllers

Enterprise RAID controller health and disk status.

| Integration | Alerts Covered |
|------------|---------------|
| [Adaptec RAID](/docs/src/go/plugin/go.d/collector/adaptecraid/integrations/adaptecraid.md) | Logical device health, physical disk state, battery backup |
| [MegaRAID](/docs/src/go/plugin/go.d/collector/megacli/integrations/megacli.md) | Adapter health, BBU charge/recharge cycles, media/predictive errors |
| [StorCLI](/docs/src/go/plugin/go.d/collector/storcli/integrations/storcli.md) | Controller health, battery status, disk errors, predictive failures |

### Software RAID and MDADM

Linux software RAID monitoring.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **mdstat_disks** | Number of healthy disks in array | `mdstat.md` | CRIT degraded |
| **mdstat_mismatch_cnt** | Resync mismatch count | `mdstat.md` | WARN > 0 |
| **mdstat_nonredundant_last_collected** | Array running without redundancy | `mdstat.md` | CRIT non-redundant |

### Storage Pools and Volume Managers

LVM, BCACHE, and ZFS storage pool monitoring.

| Integration | Alerts Covered |
|------------|---------------|
| [LVM](/docs/src/go/plugin/go.d/collector/lvm/integrations/lvm.md) | LV data space, metadata space utilization |
| [BCACHE](/docs/src/go/plugin/go.d/collector/dmcache/integrations/dmcache.md) | Cache dirty data, cache errors |
| [ZFS](/docs/src/health/health.d/zfs.conf) | Pool health state, space utilization, vdev health |

### SSD and NVMe Monitoring

NVMe drive health and SMART status.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **nvme_device_critical_warnings_state** | Critical warning flags from NVMe SMART | `nvme.health` | CRIT > 0 |

### SMART Disk Monitoring

Drive self-monitoring and predictive failure indicators.

| Integration | Alerts Covered |
|------------|---------------|
| [smartctl](/docs/src/go/plugin/go.d/collector/smartctl/integrations/smartctl.md) | Drive health, predictive failures, temperature |

### Uninterruptible Power Supplies

UPS battery and power status monitoring.

#### APCUPSD (APC UPS)

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **apcupsd_ups_battery_charge** | Remaining battery capacity | `apcupsd.ups_battery_charge` | WARN < 25%, CRIT < 10% |
| **apcupsd_ups_status_onbatt** | Operating on battery power | `apcupsd.ups_status` | CRIT on battery |
| **apcupsd_ups_status_lowbatt** | Low battery warning | `apcupsd.ups_status` | CRIT low battery |
| **apcupsd_ups_load_capacity** | UPS load percentage | `apcupsd.ups_load_capacity_utilization` | WARN > 80%, CRIT > 90% |
| **apcupsd_ups_status_commlost** | Communication lost with UPS | `apcupsd.ups_status` | CRIT comm lost |
| **apcupsd_ups_selftest_warning** | UPS selftest warning | `apcupsd.ups` | WARN/C |
| **apcupsd_last_collected_secs** | Stale data from UPS | `apcupsd.ups` | WARN > 300s |

#### NUT (Network UPS Tools)

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **upsd_ups_battery_charge** | Battery charge level | `upsd.ups_battery_charge` | WARN < 25%, CRIT < 10% |
| **upsd_10min_ups_load** | UPS load percentage | `upsd.ups_load` | WARN > 80%, CRIT > 90% |
| **upsd_ups_last_collected_secs** | Stale data from UPS daemon | `upsd.ups` | WARN > 300s |

### Baseboard Management Controller (BMC) and IPMI

Server BMC/IPMI sensor monitoring.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **ipmi_sensor_state** | Individual BMC sensor states | `ipmi.sensor_state` | WARN/CRIT per sensor |
| **ipmi_events** | IPMI event log entries | `ipmi.events` | Any events logged |

### Hardware Sensors

Temperature, fan speed, voltage, and environmental sensors.

| Integration | Alerts Covered |
|------------|---------------|
| [Sensors](/docs/src/go/plugin/go.d/collector/sensors/integrations/sensors.md) | Temperature, fan speed, voltage alarms |
| [LM-Sensors](/docs/src/go/plugin/go.d/collector/sensors/integrations/lm_sensors.md) | Core temp, ambient temp, fan speeds |
| [NVidia SMI](/docs/src/go/plugin/go.d/collector/nvidia_smi/integrations/nvidia_smi.md) | GPU temperature, memory, utilization |
| [Intel GPU](/docs/src/go/plugin/go.d/collector/intelgpu/integrations/intelgpu.md) | GPU frequency, temperature, render engines |

### Hardware Memory and ECC

ECC memory error counting and DIMM status.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **ecc_memory_mc_correctable** | Memory controller correctable errors | `ecc.mc` | Various |
| **ecc_memory_mc_uncorrectable** | Memory controller uncorrectable errors | `ecc.mc` | Any uncorrectable |
| **ecc_memory_dimm_correctable** | DIMM correctable ECC errors | `ecc.dimm` | Various |
| **ecc_memory_dimm_uncorrectable** | DIMM uncorrectable ECC errors | `ecc.dimm` | Any uncorrectable |
| **1hour_memory_hw_corrupted** | Memory region flagged corrupted | `hardware.corrupted` | CRIT > 0 |
| **power_supply_capacity** | Power supply capacity utilization | `power_supply.capacity` | WARN > 90% |

### VMware vCenter Server Appliance (VCSA)

VMware VCSA health and resource monitoring.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **vcsa_system_health_crit** | Critical system health | `vcsa.applmgmt` | CRIT |
| **vcsa_system_health_warn** | Warning system health | `vcsa.applmgmt` | WARN |
| **vcsa_load_health_crit** | Critical load average | `vcsa.system` | CRIT |
| **vcsa_load_health_warn** | Warning load average | `vcsa.system` | WARN |
| **vcsa_mem_health_crit** | Critical memory usage | `vcsa.system` | CRIT |
| **vcsa_mem_health_warn** | Warning memory usage | `vcsa.system` | WARN |
| **vcsa_swap_health_crit** | Critical swap usage | `vcsa.system` | CRIT |
| **vcsa_swap_health_warn** | Warning swap usage | `vcsa.system` | WARN |
| **vcsa_storage_health_crit** | Critical storage | `vcsa.storage` | CRIT |
| **vcsa_storage_health_warn** | Warning storage | `vcsa.storage` | WARN |
| **vcsa_database_storage_health_crit** | Critical DB storage | `vcsa.database` | CRIT |
| **vcsa_database_storage_health_warn** | Warning DB storage | `vcsa.database` | WARN |
| **vcsa_software_packages_health_warn** | Software packages issues | `vcsa.software` | WARN |

### IBM AS/400 System Monitoring

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **as400_cpu_utilization** | CPU utilization percentage | `as400.cpu` | WARN/C |
| **as400_disk_busy_average** | Disk busy average | `as400.disk` | WARN/C |
| **as400_job_queue_waiting** | Jobs waiting in queue | `as400.jobs` | WARN/C |
| **as400_system_asp_usage** | Auxiliary storage pool usage | `as400.asp` | WARN/C |

### BOINC Distributed Computing

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **boinc_active_tasks** | Number of active BOINC tasks | `boinc.tasks` | WARN/C |
| **boinc_total_tasks** | Total BOINC tasks | `boinc.tasks` | WARN/C |
| **boinc_compute_errors** | Computation errors | `boinc.errors` | WARN > 0 |
| **boinc_upload_errors** | Upload failures | `boinc.upload` | WARN > 0 |

### Systemd Unit Monitoring

Service, socket, timer, and systemd unit health.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **systemd_service_unit_failed_state** | Service unit failed status | `systemd.units` | CRIT failed |
| **systemd_socket_unit_failed_state** | Socket unit failed status | `systemd.units` | CRIT failed |
| **systemd_timer_unit_failed_state** | Timer unit failed status | `systemd.units` | CRIT failed |
| **systemd_mount_unit_failed_state** | Mount unit failed status | `systemd.units` | CRIT failed |
| **systemd_swap_unit_failed_state** | Swap unit failed status | `systemd.units` | CRIT failed |
| **systemd_automount_unit_failed_state** | Automount unit failed | `systemd.units` | CRIT failed |
| **systemd_device_unit_failed_state** | Device unit failed status | `systemd.units` | CRIT failed |
| **systemd_path_unit_failed_state** | Path unit failed status | `systemd.units` | CRIT failed |
| **systemd_scope_unit_failed_state** | Scope unit failed status | `systemd.units` | CRIT failed |
| **systemd_slice_unit_failed_state** | Slice unit failed status | `systemd.units` | CRIT failed |
| **systemd_target_unit_failed_state** | Target unit failed status | `systemd.units` | CRIT failed |

### Exporting and Streaming

Metrics export and streaming pipeline health.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **exporting_last_buffering** | Last buffering timestamp | `exporting.buffer` | Stale > 5m |
| **exporting_metrics_sent** | Export pipeline status | `exporting.sent` | Export failure |
| **streaming_disconnected** | Upstream connection lost | `streaming.remote` | CRIT disconnected |
| **streaming_never_connected** | Never connected upstream | `streaming.remote` | CRIT never connected |
| **plugin_availability_status** | Collector plugin status | `plugins.available` | Disabled |
| **plugin_data_collection_status** | Data collection failures | `plugins.collection` | Failures |
| **python.d_job_last_collected_secs** | Python collector staleness | `python.d.job` | WARN > 300s |

### BTRFS Filesystem

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **btrfs_allocated** | BTRFS space allocation | `btrfs.allocation` | WARN/C |
| **btrfs_data** | BTRFS data pool status | `btrfs.data` | WARN/C |
| **btrfs_metadata** | BTRFS metadata pool status | `btrfs.metadata` | WARN/C |
| **btrfs_system** | BTRFS system pool status | `btrfs.system` | WARN/C |
| **btrfs_device_corruption_errors** | Device corruption errors | `btrfs.device` | CRIT > 0 |
| **btrfs_device_flush_errors** | Device flush errors | `btrfs.device` | CRIT > 0 |
| **btrfs_device_generation_errors** | Generation errors | `btrfs.device` | CRIT > 0 |
| **btrfs_device_read_errors** | Device read errors | `btrfs.device` | CRIT > 0 |
| **btrfs_device_write_errors** | Device write errors | `btrfs.device` | CRIT > 0 |

### CEPH Storage Cluster

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **ceph_cluster_physical_capacity_utilization** | Ceph cluster capacity utilization | `ceph.cluster` | WARN > 80%, CRIT > 90% |

## 11.6 Netdata Infrastructure and Special Monitors

Specialized monitoring for Netdata internals, web logging, I/O latency, and additional platforms.

:::note

These are specialized monitoring categories that extend beyond standard infrastructure alerting.

:::

### Netdata Database Engine

Monitoring for Netdata's own time-series database performance and health.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **10min_dbengine_global_fs_errors** | Filesystem errors affecting DB engine | `netdata.dbengine_global_errors` | CRIT > 0 |
| **10min_dbengine_global_io_errors** | IO errors (CRC, disk, space) | `netdata.dbengine_global_errors` | CRIT > 0 |
| **10min_dbengine_global_flushing_warnings** | Page cache flushing warnings | `netdata.dbengine_global_errors` | WARN > 0 |
| **10min_dbengine_global_flushing_errors** | Page cache flushing errors | `netdata.dbengine_global_errors` | CRIT > 0 |
| **zfs_memory_throttle** | ZFS memory throttling pressure | `zfs.arc` | WARN/C |

### Web Server Access Log Monitoring

Parse and alert on Apache, Nginx, and other web server access logs.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **web_log_1m_requests** | Requests per minute | `web_log.requests` | WARN/C |
| **web_log_1m_successful** | Successful (2xx) requests | `web_log.responses` | WARN/C |
| **web_log_1m_bad_requests** | Bad requests (4xx) | `web_log.responses` | WARN/C |
| **web_log_1m_internal_errors** | Internal errors (5xx) | `web_log.responses` | CRIT > 0 |
| **web_log_10m_response_time** | Response time percentiles | `web_log.latency` | WARN/C |
| **web_log_5m_successful_old** | Legacy successful response check | `web_log.responses` | WARN/C |
| **web_log_5m_requests_ratio** | Request ratio between sections | `web_log.requests` | WARN/C |
| **web_log_1m_redirects** | Redirect responses (3xx) | `web_log.responses` | WARN/C |
| **web_log_1m_unmatched** | Lines not matching log format | `web_log.unmatched` | WARN > 0 |

### Disk I/O Latency Monitoring (IOPing)

Block device latency measurements and thresholds.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **ioping_disk_latency** | Average I/O latency | `ioping.latency` | WARN > 1s, CRIT > 5s |

### Additional Platforms and Specialized Monitoring

| Integration | Alerts Covered |
|------------|---------------|
| [Riak KV](/docs/src/go/plugin/go.d/collector/riakkv/integrations/riakkv.md) | KV get/put latency, slow ops, VM process count |
| [Retroshare](/docs/src/go/plugin/go.d/collector/retroshare/integrations/retroshare.md) | DHT working status |
| [Monit](/docs/src/go/plugin/go.d/collector/monit/integrations/monit.md) | Monit service status |
| [Whois Query](/docs/src/go/plugin/go.d/collector/whoisquery/integrations/whoisquery.md) | Domain expiration tracking |
| [WireGuard](/docs/src/go/plugin/go.d/collector/wireguard/integrations/wireguard.md) | WireGuard tunnel status |
| [Machine Learning](/docs/src/go/plugin/go.d/collector/prometheus/integrations/ml.md) | ML model anomalies (ml_1min_node_ar) |
| [PHP-FPM](/docs/src/go/plugin/go.d/collector/phpfpm/integrations/phpfpm.md) | Process manager, requests, performance |
| [PHP Daemon](/docs/src/go/plugin/go.d/collector/phpdaemon/integrations/phpdaemon.md) | Workers, uptime, memory |
| [Tor](/docs/src/go/plugin/go.d/collector/tor/integrations/tor.md) | Bandwidth, circuits, relay status |
| [Spigot MC](/docs/src/go/plugin/go.d/collector/spigotmc/integrations/spigotmc.md) | Minecraft player count, latency |
| [BOINC](/docs/src/go/plugin/go.d/collector/boinc/integrations/boinc.md) | Active tasks, compute/upload errors |
| [SMB/CIFS](/docs/src/go/plugin/go.d/collector/samba/integrations/samba.md) | SMB shares, connections, guest access |
| [vSphere](/docs/src/go/plugin/go.d/collector/vsphere/integrations/vsphere.md) | ESXi hosts, VM CPU/memory utilization |
| [WebSphere PMI](/docs/src/go/plugin/go.d/collector/websphere_pmi/integrations/websphere_pmi.md) | JDBC pool, thread pool, JMS queues |
| [WebSphere JMX](/docs/src/go/plugin/go.d/collector/websphere_jmx/integrations/websphere_jmx.md) | JVM heap, application response, JDBC wait |
| [WebSphere MP](/docs/src/go/plugin/go.d/collector/websphere_mp/integrations/websphere_mp.md) | REST response time, thread pool, heap |
| [NSDNS](/docs/src/go/plugin/go.d/collector/nsd/integrations/nsd.md) | NSD zone transfers, query statistics |
| [Chrony](/docs/src/go/plugin/go.d/collector/chrony/integrations/chrony.md) | Clock stratum, offset, sync status |
| [NTPD](/docs/src/go/plugin/go.d/collector/ntpd/integrations/ntpd.md) | NTP daemon, poll interval, dispersion |
| [PowerDNS](/docs/src/go/plugin/go.d/collector/powerdns/integrations/powerdns.md) | Queries, cache, DNSSEC validations |
| [NTPdate](/docs/src/go/plugin/go.d/collector/prometheus/integrations/ntpdate.md) | NTP sync status, offset |
| [Icecast](/docs/src/go/plugin/go.d/collector/icecast/integrations/icecast.md) | Icecast streams, listeners, source stats |
| [Dovecot](/docs/src/go/plugin/go.d/collector/dovecot/integrations/dovecot.md) | IMAP/POP3 sessions, commands, read/write |
| [Exim](/docs/src/go/plugin/go.d/collector/exim/integrations/exim.md) | Mail queue, frozen messages, SMTP stats |
| [Rspamd](/docs/src/go/plugin/go.d/collector/rspamd/integrations/rspamd.md) | Actions, scores, learn/block actions |
| [OpenLDAP](/docs/src/go/plugin/go.d/collector/openldap/integrations/openldap.md) | Bind operations, entries, referrals |
| [NGINX Unit](/docs/src/go/plugin/go.d/collector/nginxunit/integrations/nginxunit.md) | Applications, routes, listener status |
| [NGINX VTS](/docs/src/go/plugin/go.d/collector/nginxvts/integrations/nginxvts.md) | Server zones, upstream peers, requests |
| [NGINX Plus](/docs/src/go/plugin/go.d/collector/nginxplus/integrations/nginxplus.md) | HTTP upstreams, SSL certificates |
| [Tengine](/docs/src/go/plugin/go.d/collector/tengine/integrations/tengine.md) | Requests, connections, cache status |
| [Envoy](/docs/src/go/plugin/go.d/collector/envoy/integrations/envoy.md) | Downstream/upstream connections, requests |
| [Varnish](/docs/src/go/plugin/go.d/collector/varnish/integrations/varnish.md) | Client requests, backend fetches, cache hits |
| [UWSGI](/docs/src/go/plugin/go.d/collector/uwsgi/integrations/uwsgi.md) | Requests, rss, total runtime |
| [Tomcat](/docs/src/go/plugin/go.d/collector/tomcat/integrations/tomcat.md) | JSPs, servlets, thread pool |
| [Spring Boot](/docs/src/go/plugin/go.d/collector/spring_boot/integrations/spring_boot.md) | Heap, gc pauses, HTTP requests |
| [Activemq](/docs/src/go/plugin/go.d/collector/activemq/integrations/activemq.md) | Topics, queues, producers, consumers |
| [Typesense](/docs/src/go/plugin/go.d/collector/typesense/integrations/typesense.md) | Search requests,latency, memory |
| [YugaByte DB](/docs/src/go/plugin/go.d/collector/yugabytedb/integrations/yugabytedb.md) | YSQL connections, tablet servers |
| [MaxScale](/docs/src/go/plugin/go.d/collector/maxscale/integrations/maxscale.md) | MaxScale routers, connections, threads |
| [Oracledb](/docs/src/go/plugin/go.d/collector/oracledb/integrations/oracledb.md) | PDB status, ASM disk groups |
| [Pika](/docs/src/go/plugin/go.d/collector/pika/integrations/pika.md) | Compaction, memory, compression |
| [Pulsar](/docs/src/go/plugin/go.d/collector/pulsar/integrations/pulsar.md) | Messages published, replication lag |
| [Solr](/docs/src/go/plugin/go.d/collector/solr/integrations/solr.md) | Solr core stats, request latency |
| [SolrCloud](/docs/src/go/plugin/go.d/collector/solr/integrations/solrcloud.md) | SolrCloud collections, shards |
| [Cassandra](/docs/src/go/plugin/go.d/collector/cassandra/integrations/cassandra.md) | Key/row cache, compaction, threads |
| [Couchbase](/docs/src/go/plugin/go.d/collector/couchbase/integrations/couchbase.md) | Bucket quota, views, XDCR |
| [CouchDB](/docs/src/go/plugin/go.d/collector/couchdb/integrations/couchdb.md) | Request methods, status codes |
| [InfluxDB](/docs/src/go/plugin/go.d/collector/influxdb/integrations/influxdb.md) | InfluxDB write throughput, query latency |
| [Druid](/docs/src/go/plugin/go.d/collector/druid/integrations/druid.md) | Broker/Historical/Realtime services |
| [QuestDB](/docs/src/go/plugin/go.d/collector/questdb/integrations/questdb.md) | QuestDB TCP/JVM metrics |
| [TDEngine](/docs/src/go/plugin/go.d/collector/tdengine/integrations/tdengine.md) | TDEngine insert, queries, vgroups |
| [VictoriaMetrics](/docs/src/go/plugin/go.d/collector/victoriametrics/integrations/victoriametrics.md) | vminsert/vmselect efficiency |
| [TimescaleDB](/docs/src/go/plugin/go.d/collector/prometheus/integrations/timescaledb.md) | Compression, bgw jobs, tuples |
| [Crushmap](/docs/src/go/plugin/go.d/collector/crushmap/integrations/crushmap.md) | Crush map rule analysis |
| [ScaleIO](/docs/src/go/plugin/go.d/collector/scaleio/integrations/scaleio.md) | SDC/MDM connection, storage pools |
| [EMC PowerPath](/docs/src/go/plugin/go.d/collector/emcpower/integrations/emcpower.md) | PowerPath device paths |
| [NetApp](/docs/src/go/plugin/go.d/collector/netapp/integrations/netapp.md) | Aggregate, lun, volume utilization |
| [Oracle AX](/docs/src/go/plugin/go.d/collector/oracle/integrations/oracle.md) | Oracle DB tablespace, sessions |
| [Spring Cloud](/docs/src/go/plugin/go.d/collector/spring_cloud/integrations/spring_cloud.md) | Eureka services, Zuul routes |
| [Istio](/docs/src/go/plugin/go.d/collector/istio/integrations/istio.md) | Mesh traffic, request durations |
| [Linkerd](/docs/src/go/plugin/go.d/collector/linkerd/integrations/linkerd.md) | Linkerd proxy, destination services |
| [Kong](/docs/src/go/plugin/go.d/collector/kong/integrations/kong.md) | Kong requests, database connections |
| [Traefik](/docs/src/go/plugin/go.d/collector/traefik/integrations/traefik.md) | Requests, retries, error codes |
| [Cilium](/docs/src/go/plugin/go.d/collector/cilium/integrations/cilium.md) | Endpoints, identity, BPF maps |
| [CNI Metrics](/docs/src/go/plugin/go.d/collector/cni/integrations/cni.md) | CNI bandwidth, errors, drops |
| [NVIDIA DCGM](/docs/src/go/plugin/go.d/collector/dcgm/integrations/dcgm.md) | GPU utilization, memory, temperature |
| [AMD ROCm](/docs/src/go/plugin/go.d/collector/amdgpu/integrations/amdgpu.md) | GPU memory, engine usage |
| [OpenVPN](/docs/src/go/plugin/go.d/collector/openvpn/integrations/openvpn.md) | Traffic, link quality, session count |
| [OpenVPN Status](/docs/src/go/plugin/go.d/collector/openvpn_status_log/integrations/openvpn_status_log.md) | VPN client connections, bandwidth |
| [WireGuard](/docs/src/go/plugin/go.d/collector/wireguard/integrations/wireguard.md) | Tunnel status, handshake, traffic |
| [DNSCrypt](/docs/src/go/plugin/go.d/collector/dnscrypt/integrations/dnscrypt.md) | DNSCrypt resolver queries |
| [NDP](/docs/src/go/plugin/go.d/collector/ndp/integrations/ndp.md) | NDP neighbor discovery |
| [Bird](/docs/src/go/plugin/go.d/collector/bird/integrations/bird.md) | BGP routes, protocols, communities |
| [FRR](/docs/src/go/plugin/go.d/collector/frr/integrations/frr.md) | FRRouting BGP/OSPF neighbors |
| [Quagga](/docs/src/go/plugin/go.d/collector/quagga/integrations/quagga.md) | Zebra routing daemon |
| [Exabgp](/docs/src/go/plugin/go.d/collector/exabgp/integrations/exabgp.md) | ExaBGP session, updates |
| [DHCP Relay](/docs/src/go/plugin/go.d/collector/dhcrelay/integrations/dhcrelay.md) | DHCP relay packets |
| [KeA DHCP](/docs/src/go/plugin/go.d/collector/kea/integrations/kea.md) | Kea DHCP leases, DDNS updates |
| [Go.D CoAP](/docs/src/go/plugin/go.d/collector/coap/integrations/coap.md) | CoAP server requests, payload size |
| [SigNoz](/docs/src/go/plugin/go.d/collector/signoz/integrations/signoz.md) | SigNoz traces, spans |

## Related Sections

- **[Chapter 2: Creating and Managing Alerts](/docs/creating-alerts-pages/README.md)** - Creating and editing alerts via configuration files
- **[Chapter 12: Best Practices for Designing Effective Alerts](/docs/best-practices/README.md)** - Best practices for designing effective alerts
- **[Chapter 3: Alert Configuration Syntax](/docs/alert-configuration-syntax/README.md)** - Alert configuration syntax reference