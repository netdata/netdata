# 6: Stock Alerts

Netdata ships with a comprehensive library of pre-configured alerts covering system resources, applications, containers, and hardware monitoring. These alerts are enabled by default and require no configuration to provide immediate visibility into common problems.

:::note

Stock alerts follow the principle of conservative defaults. They are tuned to detect genuine issues while avoiding noise from normal operational variation. However, every environment is unique—use these as a starting point and adjust thresholds based on your specific requirements.

:::

## Here

| Section | Focus Area |
|---------|------------|
| **[6.1 System Resource Alerts](#61-system-resource-alerts)** | CPU, memory, disk space, network, load averages |
| **[6.2 Container Alerts](#62-container-and-orchestration-alerts)** | Docker containers, Kubernetes pods, cgroup metrics |
| **[6.3 Application Alerts](#63-application-alerts)** | MySQL, PostgreSQL, Redis, nginx, Apache, and more |
| **[6.4 Network Alerts](#64-network-and-connectivity-alerts)** | Interface errors, packet drops, bandwidth utilization |
| **[6.5 Hardware Alerts](#65-hardware-and-sensor-alerts)** | RAID controllers, UPS battery, SMART disk status |

## 6.1 System Resource Alerts

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

## 6.2 Container and Orchestration Alerts

Container and orchestration alerts address the unique monitoring requirements of dynamic infrastructure.

:::note

Container alerts require the appropriate collector to be enabled. Check the **Collectors** tab in the dashboard to verify your container collectors are running.

:::

### Docker Container Alerts

Docker container alerts monitor both the runtime state and resource consumption.

| Alert | Description | Context | Thresholds |
|-------|-------------|---------|------------|
| **docker_container_unhealthy** | Tracks container health status from Docker daemon | `docker.container_health_status` | CRIT != 0 |
| **docker_container_down** | Tracks running state of each container | `docker.container_state` | CRIT not running |

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

## 6.3 Application Alerts

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
| [RabbitMQ](/docs/src/go/plugin/go.d/collector/rabbitmq/integrations/rabbitmq.md) | Queue depth, node health, memory, partition |
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
| [Etcd](/docs/src/go/plugin/go.d/collector/prometheus/integrations/etcd.md) | Leader, disk, RPC,瓦尔纳 |
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

## 6.4 Network and Connectivity Alerts

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

## 6.5 Hardware and Sensor Alerts

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

## Related Sections

- **[Chapter 2: Creating and Managing Alerts](../creating-alerts-pages/index.md)** - Creating and editing alerts via configuration files
- **[Chapter 12: Best Practices for Designing Effective Alerts](../best-practices/index.md)** - Best practices for designing effective alerts
- **[Chapter 3: Alert Configuration Syntax](../alert-configuration-syntax/index.md)** - Alert configuration syntax reference