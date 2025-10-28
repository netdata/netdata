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

### Linux Systems

| Integration | Description |
|-------------|-------------|
| [Access Points](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/ap/integrations/access_points.md) | Metrics for Access Points |
| [BTRFS](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/btrfs.md) | Metrics for BTRFS |
| [Conntrack](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/conntrack.md) | Metrics for Conntrack |
| [CPU performance](https://github.com/netdata/netdata/blob/master/src/collectors/perf.plugin/integrations/cpu_performance.md) | Metrics for CPU performance |
| [Disk space](https://github.com/netdata/netdata/blob/master/src/collectors/diskspace.plugin/integrations/disk_space.md) | Metrics for Disk space |
| [Disk Statistics](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/disk_statistics.md) | Metrics for Disk Statistics |
| [Entropy](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/entropy.md) | Metrics for Entropy |
| [InfiniBand](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/infiniband.md) | Metrics for InfiniBand |
| [Inter Process Communication](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/inter_process_communication.md) | Metrics for Inter Process Communication |
| [Interrupts](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/interrupts.md) | Metrics for Interrupts |
| [IP Virtual Server](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/ip_virtual_server.md) | Metrics for IP Virtual Server |
| [IPv6 Socket Statistics](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/ipv6_socket_statistics.md) | Metrics for IPv6 Socket Statistics |
| [Kernel Same-Page Merging](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/kernel_same-page_merging.md) | Metrics for Kernel Same-Page Merging |
| [Linux kernel SLAB allocator statistics](https://github.com/netdata/netdata/blob/master/src/collectors/slabinfo.plugin/integrations/linux_kernel_slab_allocator_statistics.md) | Metrics for Linux kernel SLAB allocator statistics |
| [Linux ZSwap](https://github.com/netdata/netdata/blob/master/src/collectors/debugfs.plugin/integrations/linux_zswap.md) | Metrics for Linux ZSwap |
| [MD RAID](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/md_raid.md) | Metrics for MD RAID |
| [Memory modules (DIMMs)](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/memory_modules_dimms.md) | Metrics for Memory modules (DIMMs) |
| [Memory Statistics](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/memory_statistics.md) | Metrics for Memory Statistics |
| [Memory Usage](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/memory_usage.md) | Metrics for Memory Usage |
| [Netfilter](https://github.com/netdata/netdata/blob/master/src/collectors/nfacct.plugin/integrations/netfilter.md) | Metrics for Netfilter |
| [Network interfaces](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/network_interfaces.md) | Metrics for Network interfaces |
| [Network statistics](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/network_statistics.md) | Metrics for Network statistics |
| [NFS Client](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/nfs_client.md) | Metrics for NFS Client |
| [NFS Server](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/nfs_server.md) | Metrics for NFS Server |
| [nftables](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/nftables.md) | Metrics for nftables |
| [Non-Uniform Memory Access](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/non-uniform_memory_access.md) | Metrics for Non-Uniform Memory Access |
| [OpenRC](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/openrc.md) | Metrics for OpenRC |
| [Page types](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/page_types.md) | Metrics for Page types |
| [Power Capping](https://github.com/netdata/netdata/blob/master/src/collectors/debugfs.plugin/integrations/power_capping.md) | Metrics for Power Capping |
| [Power Supply](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/power_supply.md) | Metrics for Power Supply |
| [Pressure Stall Information](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/pressure_stall_information.md) | Metrics for Pressure Stall Information |
| [SCTP Statistics](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/sctp_statistics.md) | Metrics for SCTP Statistics |
| [Socket statistics](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/socket_statistics.md) | Metrics for Socket statistics |
| [SoftIRQ statistics](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/softirq_statistics.md) | Metrics for SoftIRQ statistics |
| [Softnet Statistics](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/softnet_statistics.md) | Metrics for Softnet Statistics |
| [Synproxy](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/synproxy.md) | Metrics for Synproxy |
| [System Load Average](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/system_load_average.md) | Metrics for System Load Average |
| [System Memory Fragmentation](https://github.com/netdata/netdata/blob/master/src/collectors/debugfs.plugin/integrations/system_memory_fragmentation.md) | Metrics for System Memory Fragmentation |
| [System statistics](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/system_statistics.md) | Metrics for System statistics |
| [System Uptime](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/system_uptime.md) | Metrics for System Uptime |
| [tc QoS classes](https://github.com/netdata/netdata/blob/master/src/collectors/tc.plugin/integrations/tc_qos_classes.md) | Metrics for tc QoS classes |
| [Wireless network interfaces](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/wireless_network_interfaces.md) | Metrics for Wireless network interfaces |
| [ZFS Adaptive Replacement Cache](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/zfs_adaptive_replacement_cache.md) | Metrics for ZFS Adaptive Replacement Cache |
| [ZRAM](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/zram.md) | Metrics for ZRAM |

### eBPF

| Integration | Description |
|-------------|-------------|
| [eBPF Cachestat](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_cachestat.md) | Metrics for eBPF Cachestat |
| [eBPF DCstat](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_dcstat.md) | Metrics for eBPF DCstat |
| [eBPF Disk](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_disk.md) | Metrics for eBPF Disk |
| [eBPF Filedescriptor](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_filedescriptor.md) | Metrics for eBPF Filedescriptor |
| [eBPF Filesystem](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_filesystem.md) | Metrics for eBPF Filesystem |
| [eBPF Hardirq](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_hardirq.md) | Metrics for eBPF Hardirq |
| [eBPF MDflush](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_mdflush.md) | Metrics for eBPF MDflush |
| [eBPF Mount](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_mount.md) | Metrics for eBPF Mount |
| [eBPF OOMkill](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_oomkill.md) | Metrics for eBPF OOMkill |
| [eBPF Process](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_process.md) | Metrics for eBPF Process |
| [eBPF Processes](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_processes.md) | Metrics for eBPF Processes |
| [eBPF SHM](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_shm.md) | Metrics for eBPF SHM |
| [eBPF Socket](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_socket.md) | Metrics for eBPF Socket |
| [eBPF SoftIRQ](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_softirq.md) | Metrics for eBPF SoftIRQ |
| [eBPF SWAP](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_swap.md) | Metrics for eBPF SWAP |
| [eBPF Sync](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_sync.md) | Metrics for eBPF Sync |
| [eBPF VFS](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_vfs.md) | Metrics for eBPF VFS |

### FreeBSD

| Integration | Description |
|-------------|-------------|
| [dev.cpu.0.freq](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/dev.cpu.0.freq.md) | Metrics for dev.cpu.0.freq |
| [dev.cpu.temperature](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/dev.cpu.temperature.md) | Metrics for dev.cpu.temperature |
| [devstat](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/devstat.md) | Metrics for devstat |
| [FreeBSD NFS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/freebsd_nfs.md) | Metrics for FreeBSD NFS |
| [FreeBSD RCTL-RACCT](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/freebsd_rctl-racct.md) | Metrics for FreeBSD RCTL-RACCT |
| [getifaddrs](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/getifaddrs.md) | Metrics for getifaddrs |
| [getmntinfo](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/getmntinfo.md) | Metrics for getmntinfo |
| [hw.intrcnt](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/hw.intrcnt.md) | Metrics for hw.intrcnt |
| [ipfw](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/ipfw.md) | Metrics for ipfw |
| [kern.cp_time](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/kern.cp_time.md) | Metrics for kern.cp_time |
| [kern.ipc.msq](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/kern.ipc.msq.md) | Metrics for kern.ipc.msq |
| [kern.ipc.sem](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/kern.ipc.sem.md) | Metrics for kern.ipc.sem |
| [kern.ipc.shm](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/kern.ipc.shm.md) | Metrics for kern.ipc.shm |
| [net.inet.icmp.stats](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/net.inet.icmp.stats.md) | Metrics for net.inet.icmp.stats |
| [net.inet.ip.stats](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/net.inet.ip.stats.md) | Metrics for net.inet.ip.stats |
| [net.inet.tcp.states](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/net.inet.tcp.states.md) | Metrics for net.inet.tcp.states |
| [net.inet.tcp.stats](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/net.inet.tcp.stats.md) | Metrics for net.inet.tcp.stats |
| [net.inet.udp.stats](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/net.inet.udp.stats.md) | Metrics for net.inet.udp.stats |
| [net.inet6.icmp6.stats](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/net.inet6.icmp6.stats.md) | Metrics for net.inet6.icmp6.stats |
| [net.inet6.ip6.stats](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/net.inet6.ip6.stats.md) | Metrics for net.inet6.ip6.stats |
| [net.isr](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/net.isr.md) | Metrics for net.isr |
| [system.ram](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/system.ram.md) | Metrics for system.ram |
| [uptime](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/uptime.md) | Metrics for uptime |
| [vm.loadavg](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/vm.loadavg.md) | Metrics for vm.loadavg |
| [vm.stats.sys.v_intr](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/vm.stats.sys.v_intr.md) | Metrics for vm.stats.sys.v_intr |
| [vm.stats.sys.v_soft](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/vm.stats.sys.v_soft.md) | Metrics for vm.stats.sys.v_soft |
| [vm.stats.sys.v_swtch](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/vm.stats.sys.v_swtch.md) | Metrics for vm.stats.sys.v_swtch |
| [vm.stats.vm.v_pgfaults](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/vm.stats.vm.v_pgfaults.md) | Metrics for vm.stats.vm.v_pgfaults |
| [vm.stats.vm.v_swappgs](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/vm.stats.vm.v_swappgs.md) | Metrics for vm.stats.vm.v_swappgs |
| [vm.swap_info](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/vm.swap_info.md) | Metrics for vm.swap_info |
| [vm.vmtotal](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/vm.vmtotal.md) | Metrics for vm.vmtotal |
| [zfs](https://github.com/netdata/netdata/blob/master/src/collectors/freebsd.plugin/integrations/zfs.md) | Metrics for zfs |

### Containers and VMs

| Integration | Description |
|-------------|-------------|
| [cAdvisor](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/cadvisor.md) | Metrics for cAdvisor |
| [Containers](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/integrations/containers.md) | Metrics for Containers |
| [Docker](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/docker/integrations/docker.md) | Metrics for Docker |
| [Docker Engine](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/docker_engine/integrations/docker_engine.md) | Metrics for Docker Engine |
| [Docker Hub repository](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/dockerhub/integrations/docker_hub_repository.md) | Metrics for Docker Hub repository |
| [Libvirt Containers](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/integrations/libvirt_containers.md) | Metrics for Libvirt Containers |
| [LXC Containers](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/integrations/lxc_containers.md) | Metrics for LXC Containers |
| [NSX-T](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/nsx-t.md) | Metrics for NSX-T |
| [oVirt Containers](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/integrations/ovirt_containers.md) | Metrics for oVirt Containers |
| [Podman](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/podman.md) | Metrics for Podman |
| [Proxmox Containers](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/integrations/proxmox_containers.md) | Metrics for Proxmox Containers |
| [Proxmox VE](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/proxmox_ve.md) | Metrics for Proxmox VE |
| [vCenter Server Appliance](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/vcsa/integrations/vcenter_server_appliance.md) | Metrics for vCenter Server Appliance |
| [Virtual Machines](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/integrations/virtual_machines.md) | Metrics for Virtual Machines |
| [VMware vCenter Server](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/vsphere/integrations/vmware_vcenter_server.md) | Metrics for VMware vCenter Server |
| [Xen XCP-ng](https://github.com/netdata/netdata/blob/master/src/collectors/xenstat.plugin/integrations/xen_xcp-ng.md) | Metrics for Xen XCP-ng |

### Databases

| Integration | Description |
|-------------|-------------|
| [4D Server](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/4d_server.md) | Metrics for 4D Server |
| [AWS RDS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/aws_rds.md) | Metrics for AWS RDS |
| [BOINC](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/boinc/integrations/boinc.md) | Metrics for BOINC |
| [Cassandra](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/cassandra/integrations/cassandra.md) | Metrics for Cassandra |
| [ClickHouse](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/clickhouse/integrations/clickhouse.md) | Metrics for ClickHouse |
| [ClusterControl CMON](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/clustercontrol_cmon.md) | Metrics for ClusterControl CMON |
| [CockroachDB](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/cockroachdb/integrations/cockroachdb.md) | Metrics for CockroachDB |
| [Couchbase](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/couchbase/integrations/couchbase.md) | Metrics for Couchbase |
| [CouchDB](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/couchdb/integrations/couchdb.md) | Metrics for CouchDB |
| [HANA](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/hana.md) | Metrics for HANA |
| [Hasura GraphQL Server](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/hasura_graphql_server.md) | Metrics for Hasura GraphQL Server |
| [InfluxDB](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/influxdb.md) | Metrics for InfluxDB |
| [Machbase](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/machbase.md) | Metrics for Machbase |
| [MariaDB](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/mysql/integrations/mariadb.md) | Metrics for MariaDB |
| [MaxScale](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/maxscale/integrations/maxscale.md) | Metrics for MaxScale |
| [Memcached](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/memcached/integrations/memcached.md) | Metrics for Memcached |
| [Memcached (community)](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/memcached_community.md) | Metrics for Memcached (community) |
| [MongoDB](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/mongodb/integrations/mongodb.md) | Metrics for MongoDB |
| [MySQL](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/mysql/integrations/mysql.md) | Metrics for MySQL |
| [ODBC](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/odbc.md) | Metrics for ODBC |
| [Oracle DB](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/oracledb/integrations/oracle_db.md) | Metrics for Oracle DB |
| [Oracle DB (community)](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/oracle_db_community.md) | Metrics for Oracle DB (community) |
| [Patroni](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/patroni.md) | Metrics for Patroni |
| [Percona MySQL](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/mysql/integrations/percona_mysql.md) | Metrics for Percona MySQL |
| [pgBackRest](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/pgbackrest.md) | Metrics for pgBackRest |
| [PgBouncer](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/pgbouncer/integrations/pgbouncer.md) | Metrics for PgBouncer |
| [Pgpool-II](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/pgpool-ii.md) | Metrics for Pgpool-II |
| [Pika](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/pika/integrations/pika.md) | Metrics for Pika |
| [PostgreSQL](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/postgres/integrations/postgresql.md) | Metrics for PostgreSQL |
| [ProxySQL](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/proxysql/integrations/proxysql.md) | Metrics for ProxySQL |
| [Redis](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/redis/integrations/redis.md) | Metrics for Redis |
| [RethinkDB](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/rethinkdb/integrations/rethinkdb.md) | Metrics for RethinkDB |
| [Riak KV](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/riakkv/integrations/riak_kv.md) | Metrics for Riak KV |
| [SQL Database agnostic](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/sql_database_agnostic.md) | Metrics for SQL Database agnostic |
| [Vertica](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/vertica.md) | Metrics for Vertica |
| [Warp10](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/warp10.md) | Metrics for Warp10 |
| [YugabyteDB](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/yugabytedb/integrations/yugabytedb.md) | Metrics for YugabyteDB |

### Kubernetes

| Integration | Description |
|-------------|-------------|
| [Cilium Agent](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/cilium_agent.md) | Metrics for Cilium Agent |
| [Cilium Operator](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/cilium_operator.md) | Metrics for Cilium Operator |
| [Cilium Proxy](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/cilium_proxy.md) | Metrics for Cilium Proxy |
| [Kubelet](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/k8s_kubelet/integrations/kubelet.md) | Metrics for Kubelet |
| [Kubeproxy](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/k8s_kubeproxy/integrations/kubeproxy.md) | Metrics for Kubeproxy |
| [Kubernetes Cluster Cloud Cost](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/kubernetes_cluster_cloud_cost.md) | Metrics for Kubernetes Cluster Cloud Cost |
| [Kubernetes Cluster State](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/k8s_state/integrations/kubernetes_cluster_state.md) | Metrics for Kubernetes Cluster State |
| [Kubernetes Containers](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/integrations/kubernetes_containers.md) | Metrics for Kubernetes Containers |
| [Rancher](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/rancher.md) | Metrics for Rancher |

### Incident Management

| Integration | Description |
|-------------|-------------|
| [OTRS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/otrs.md) | Metrics for OTRS |
| [StatusPage](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/statuspage.md) | Metrics for StatusPage |

### Service Discovery / Registry

| Integration | Description |
|-------------|-------------|
| [Consul](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/consul/integrations/consul.md) | Metrics for Consul |
| [etcd](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/etcd.md) | Metrics for etcd |
| [Kafka Consumer Lag](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/kafka_consumer_lag.md) | Metrics for Kafka Consumer Lag |
| [ZooKeeper](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/zookeeper/integrations/zookeeper.md) | Metrics for ZooKeeper |

### Web Servers and Web Proxies

| Integration | Description |
|-------------|-------------|
| [Apache](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/apache/integrations/apache.md) | Metrics for Apache |
| [APIcast](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/apicast.md) | Metrics for APIcast |
| [Clash](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/clash.md) | Metrics for Clash |
| [Cloudflare PCAP](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/cloudflare_pcap.md) | Metrics for Cloudflare PCAP |
| [Envoy](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/envoy/integrations/envoy.md) | Metrics for Envoy |
| [Gobetween](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/gobetween.md) | Metrics for Gobetween |
| [HAProxy](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/haproxy/integrations/haproxy.md) | Metrics for HAProxy |
| [HHVM](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/hhvm.md) | Metrics for HHVM |
| [HTTPD](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/apache/integrations/httpd.md) | Metrics for HTTPD |
| [Lighttpd](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/lighttpd/integrations/lighttpd.md) | Metrics for Lighttpd |
| [Litespeed](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/litespeed/integrations/litespeed.md) | Metrics for Litespeed |
| [NGINX](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/nginx/integrations/nginx.md) | Metrics for NGINX |
| [NGINX Plus](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/nginxplus/integrations/nginx_plus.md) | Metrics for NGINX Plus |
| [NGINX Unit](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/nginxunit/integrations/nginx_unit.md) | Metrics for NGINX Unit |
| [NGINX VTS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/nginxvts/integrations/nginx_vts.md) | Metrics for NGINX VTS |
| [PHP-FPM](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/phpfpm/integrations/php-fpm.md) | Metrics for PHP-FPM |
| [Squid](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/squid/integrations/squid.md) | Metrics for Squid |
| [Squid log files](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/squidlog/integrations/squid_log_files.md) | Metrics for Squid log files |
| [Tengine](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/tengine/integrations/tengine.md) | Metrics for Tengine |
| [Tomcat](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/tomcat/integrations/tomcat.md) | Metrics for Tomcat |
| [Traefik](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/traefik/integrations/traefik.md) | Metrics for Traefik |
| [uWSGI](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/uwsgi/integrations/uwsgi.md) | Metrics for uWSGI |
| [Varnish](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/varnish/integrations/varnish.md) | Metrics for Varnish |
| [Web server log files](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/weblog/integrations/web_server_log_files.md) | Metrics for Web server log files |

### Cloud Provider Managed

| Integration | Description |
|-------------|-------------|
| [Akamai Global Traffic Management](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/akamai_global_traffic_management.md) | Metrics for Akamai Global Traffic Management |
| [Akami Cloudmonitor](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/akami_cloudmonitor.md) | Metrics for Akami Cloudmonitor |
| [Alibaba Cloud](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/alibaba_cloud.md) | Metrics for Alibaba Cloud |
| [ArvanCloud CDN](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/arvancloud_cdn.md) | Metrics for ArvanCloud CDN |
| [AWS EC2 Compute instances](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/aws_ec2_compute_instances.md) | Metrics for AWS EC2 Compute instances |
| [AWS EC2 Spot Instance](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/aws_ec2_spot_instance.md) | Metrics for AWS EC2 Spot Instance |
| [AWS ECS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/aws_ecs.md) | Metrics for AWS ECS |
| [AWS Health events](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/aws_health_events.md) | Metrics for AWS Health events |
| [AWS instance health](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/aws_instance_health.md) | Metrics for AWS instance health |
| [AWS Quota](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/aws_quota.md) | Metrics for AWS Quota |
| [AWS S3 buckets](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/aws_s3_buckets.md) | Metrics for AWS S3 buckets |
| [AWS SQS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/aws_sqs.md) | Metrics for AWS SQS |
| [Azure AD App passwords](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/azure_ad_app_passwords.md) | Metrics for Azure AD App passwords |
| [Azure application](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/azure_application.md) | Metrics for Azure application |
| [Azure Elastic Pool SQL](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/azure_elastic_pool_sql.md) | Metrics for Azure Elastic Pool SQL |
| [Azure Resources](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/azure_resources.md) | Metrics for Azure Resources |
| [Azure Service Bus](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/azure_service_bus.md) | Metrics for Azure Service Bus |
| [Azure SQL](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/azure_sql.md) | Metrics for Azure SQL |
| [BigQuery](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/bigquery.md) | Metrics for BigQuery |
| [CloudWatch](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/cloudwatch.md) | Metrics for CloudWatch |
| [Dell EMC ECS cluster](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/dell_emc_ecs_cluster.md) | Metrics for Dell EMC ECS cluster |
| [DigitalOcean](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/digitalocean.md) | Metrics for DigitalOcean |
| [GCP GCE](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/gcp_gce.md) | Metrics for GCP GCE |
| [GCP Quota](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/gcp_quota.md) | Metrics for GCP Quota |
| [Google Cloud Platform](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/google_cloud_platform.md) | Metrics for Google Cloud Platform |
| [Google Stackdriver](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/google_stackdriver.md) | Metrics for Google Stackdriver |
| [Linode](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/linode.md) | Metrics for Linode |
| [Lustre metadata](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/lustre_metadata.md) | Metrics for Lustre metadata |
| [Nextcloud servers](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/nextcloud_servers.md) | Metrics for Nextcloud servers |
| [OpenStack](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/openstack.md) | Metrics for OpenStack |
| [Zerto](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/zerto.md) | Metrics for Zerto |

### Windows Systems

| Integration | Description |
|-------------|-------------|
| [Active Directory](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/active_directory.md) | Metrics for Active Directory |
| [Active Directory Certificate Service](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/active_directory_certificate_service.md) | Metrics for Active Directory Certificate Service |
| [Active Directory Federation Service](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/active_directory_federation_service.md) | Metrics for Active Directory Federation Service |
| [ASP.NET](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/asp.net.md) | Metrics for ASP.NET |
| [Hyper-V](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/hyper-v.md) | Metrics for Hyper-V |
| [IIS](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/iis.md) | Metrics for IIS |
| [Memory statistics](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/memory_statistics.md) | Metrics for Memory statistics |
| [MS Exchange](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/ms_exchange.md) | Metrics for MS Exchange |
| [MS SQL Server](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/ms_sql_server.md) | Metrics for MS SQL Server |
| [NET Framework](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/net_framework.md) | Metrics for NET Framework |
| [Network Subsystem](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/network_subsystem.md) | Metrics for Network Subsystem |
| [NUMA Architecture](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/numa_architecture.md) | Metrics for NUMA Architecture |
| [Physical and Logical Disk Performance Metrics](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/physical_and_logical_disk_performance_metrics.md) | Metrics for Physical and Logical Disk Performance Metrics |
| [Power supply](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/power_supply.md) | Metrics for Power supply |
| [Processor](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/processor.md) | Metrics for Processor |
| [Semaphore statistics](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/semaphore_statistics.md) | Metrics for Semaphore statistics |
| [Sensors](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/sensors.md) | Metrics for Sensors |
| [System statistics](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/system_statistics.md) | Metrics for System statistics |
| [System thermal zone](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/system_thermal_zone.md) | Metrics for System thermal zone |
| [Windows Services](https://github.com/netdata/netdata/blob/master/src/collectors/windows.plugin/integrations/windows_services.md) | Metrics for Windows Services |

### APM

| Integration | Description |
|-------------|-------------|
| [Alamos FE2 server](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/alamos_fe2_server.md) | Metrics for Alamos FE2 server |
| [Apache Airflow](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/apache_airflow.md) | Metrics for Apache Airflow |
| [Apache Flink](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/apache_flink.md) | Metrics for Apache Flink |
| [Audisto](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/audisto.md) | Metrics for Audisto |
| [bpftrace variables](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/bpftrace_variables.md) | Metrics for bpftrace variables |
| [Dependency-Track](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/dependency-track.md) | Metrics for Dependency-Track |
| [Go applications (EXPVAR)](https://github.com/netdata/netdata/blob/master/src/collectors/python.d.plugin/go_expvar/integrations/go_applications_expvar.md) | Metrics for Go applications (EXPVAR) |
| [Google Pagespeed](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/google_pagespeed.md) | Metrics for Google Pagespeed |
| [gpsd](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/gpsd.md) | Metrics for gpsd |
| [IBM AIX systems Njmon](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/ibm_aix_systems_njmon.md) | Metrics for IBM AIX systems Njmon |
| [JMX](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/jmx.md) | Metrics for JMX |
| [jolokia](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/jolokia.md) | Metrics for jolokia |
| [NRPE daemon](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/nrpe_daemon.md) | Metrics for NRPE daemon |
| [phpDaemon](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/phpdaemon/integrations/phpdaemon.md) | Metrics for phpDaemon |
| [Sentry](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/sentry.md) | Metrics for Sentry |
| [Sysload](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/sysload.md) | Metrics for Sysload |
| [VSCode](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/vscode.md) | Metrics for VSCode |
| [YOURLS URL Shortener](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/yourls_url_shortener.md) | Metrics for YOURLS URL Shortener |

### Hardware Devices and Sensors

| Integration | Description |
|-------------|-------------|
| [1-Wire Sensors](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/w1sensor/integrations/1-wire_sensors.md) | Metrics for 1-Wire Sensors |
| [AM2320](https://github.com/netdata/netdata/blob/master/src/collectors/python.d.plugin/am2320/integrations/am2320.md) | Metrics for AM2320 |
| [AMD CPU & GPU](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/amd_cpu_&_gpu.md) | Metrics for AMD CPU & GPU |
| [AMD GPU](https://github.com/netdata/netdata/blob/master/src/collectors/proc.plugin/integrations/amd_gpu.md) | Metrics for AMD GPU |
| [ARM HWCPipe](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/arm_hwcpipe.md) | Metrics for ARM HWCPipe |
| [CUPS](https://github.com/netdata/netdata/blob/master/src/collectors/cups.plugin/integrations/cups.md) | Metrics for CUPS |
| [HDD temperature](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/hddtemp/integrations/hdd_temperature.md) | Metrics for HDD temperature |
| [HP iLO](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/hp_ilo.md) | Metrics for HP iLO |
| [IBM CryptoExpress (CEX) cards](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/ibm_cryptoexpress_cex_cards.md) | Metrics for IBM CryptoExpress (CEX) cards |
| [IBM Z Hardware Management Console](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/ibm_z_hardware_management_console.md) | Metrics for IBM Z Hardware Management Console |
| [Intel GPU](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/intelgpu/integrations/intel_gpu.md) | Metrics for Intel GPU |
| [Intelligent Platform Management Interface (IPMI)](https://github.com/netdata/netdata/blob/master/src/collectors/freeipmi.plugin/integrations/intelligent_platform_management_interface_ipmi.md) | Metrics for Intelligent Platform Management Interface (IPMI) |
| [IPMI (By SoundCloud)](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/ipmi_by_soundcloud.md) | Metrics for IPMI (By SoundCloud) |
| [Linux Sensors](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/sensors/integrations/linux_sensors.md) | Metrics for Linux Sensors |
| [Nvidia GPU](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/nvidia_smi/integrations/nvidia_gpu.md) | Metrics for Nvidia GPU |
| [NVML](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/nvml.md) | Metrics for NVML |
| [Raritan PDU](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/raritan_pdu.md) | Metrics for Raritan PDU |
| [S.M.A.R.T.](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/smartctl/integrations/s.m.a.r.t..md) | Metrics for S.M.A.R.T. |
| [ServerTech](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/servertech.md) | Metrics for ServerTech |
| [Siemens S7 PLC](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/siemens_s7_plc.md) | Metrics for Siemens S7 PLC |
| [T-Rex NVIDIA GPU Miner](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/t-rex_nvidia_gpu_miner.md) | Metrics for T-Rex NVIDIA GPU Miner |

### macOS Systems

| Integration | Description |
|-------------|-------------|
| [Apple Time Machine](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/apple_time_machine.md) | Metrics for Apple Time Machine |
| [macOS](https://github.com/netdata/netdata/blob/master/src/collectors/macos.plugin/integrations/macos.md) | Metrics for macOS |

### Message Brokers

| Integration | Description |
|-------------|-------------|
| [ActiveMQ](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/activemq/integrations/activemq.md) | Metrics for ActiveMQ |
| [Apache Pulsar](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/pulsar/integrations/apache_pulsar.md) | Metrics for Apache Pulsar |
| [Beanstalk](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/beanstalk/integrations/beanstalk.md) | Metrics for Beanstalk |
| [IBM MQ](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/ibm_mq.md) | Metrics for IBM MQ |
| [Kafka](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/kafka.md) | Metrics for Kafka |
| [Kafka Connect](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/kafka_connect.md) | Metrics for Kafka Connect |
| [Kafka ZooKeeper](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/kafka_zookeeper.md) | Metrics for Kafka ZooKeeper |
| [mosquitto](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/mosquitto.md) | Metrics for mosquitto |
| [MQTT Blackbox](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/mqtt_blackbox.md) | Metrics for MQTT Blackbox |
| [NATS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/nats/integrations/nats.md) | Metrics for NATS |
| [RabbitMQ](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/rabbitmq/integrations/rabbitmq.md) | Metrics for RabbitMQ |
| [Redis Queue](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/redis_queue.md) | Metrics for Redis Queue |
| [VerneMQ](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/vernemq/integrations/vernemq.md) | Metrics for VerneMQ |
| [XMPP Server](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/xmpp_server.md) | Metrics for XMPP Server |

### Provisioning Systems

| Integration | Description |
|-------------|-------------|
| [BOSH](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/bosh.md) | Metrics for BOSH |
| [Cloud Foundry](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/cloud_foundry.md) | Metrics for Cloud Foundry |
| [Cloud Foundry Firehose](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/cloud_foundry_firehose.md) | Metrics for Cloud Foundry Firehose |
| [Spacelift](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/spacelift.md) | Metrics for Spacelift |

### Search Engines

| Integration | Description |
|-------------|-------------|
| [Elasticsearch](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/elasticsearch/integrations/elasticsearch.md) | Metrics for Elasticsearch |
| [Meilisearch](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/meilisearch.md) | Metrics for Meilisearch |
| [OpenSearch](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/elasticsearch/integrations/opensearch.md) | Metrics for OpenSearch |
| [Sphinx](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/sphinx.md) | Metrics for Sphinx |
| [Typesense](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/typesense/integrations/typesense.md) | Metrics for Typesense |

### Networking Stack and Network Interfaces

| Integration | Description |
|-------------|-------------|
| [8430FT modem](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/8430ft_modem.md) | Metrics for 8430FT modem |
| [A10 ACOS network devices](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/a10_acos_network_devices.md) | Metrics for A10 ACOS network devices |
| [Andrews & Arnold line status](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/andrews_&_arnold_line_status.md) | Metrics for Andrews & Arnold line status |
| [Aruba devices](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/aruba_devices.md) | Metrics for Aruba devices |
| [Bird Routing Daemon](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/bird_routing_daemon.md) | Metrics for Bird Routing Daemon |
| [Checkpoint device](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/checkpoint_device.md) | Metrics for Checkpoint device |
| [Cisco ACI](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/cisco_aci.md) | Metrics for Cisco ACI |
| [Citrix NetScaler](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/citrix_netscaler.md) | Metrics for Citrix NetScaler |
| [DDWRT Routers](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/ddwrt_routers.md) | Metrics for DDWRT Routers |
| [Fortigate firewall](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/fortigate_firewall.md) | Metrics for Fortigate firewall |
| [Freifunk network](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/freifunk_network.md) | Metrics for Freifunk network |
| [Fritzbox network devices](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/fritzbox_network_devices.md) | Metrics for Fritzbox network devices |
| [FRRouting](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/frrouting.md) | Metrics for FRRouting |
| [Hitron CGN series CPE](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/hitron_cgn_series_cpe.md) | Metrics for Hitron CGN series CPE |
| [Hitron CODA Cable Modem](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/hitron_coda_cable_modem.md) | Metrics for Hitron CODA Cable Modem |
| [Huawei devices](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/huawei_devices.md) | Metrics for Huawei devices |
| [Keepalived](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/keepalived.md) | Metrics for Keepalived |
| [Meraki dashboard](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/meraki_dashboard.md) | Metrics for Meraki dashboard |
| [MikroTik devices](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/mikrotik_devices.md) | Metrics for MikroTik devices |
| [Mikrotik RouterOS devices](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/mikrotik_routeros_devices.md) | Metrics for Mikrotik RouterOS devices |
| [NetFlow](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/netflow.md) | Metrics for NetFlow |
| [NetMeter](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/netmeter.md) | Metrics for NetMeter |
| [Open vSwitch](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/open_vswitch.md) | Metrics for Open vSwitch |
| [OpenROADM devices](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/openroadm_devices.md) | Metrics for OpenROADM devices |
| [Optical modules](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/ethtool/integrations/optical_modules.md) | Metrics for Optical modules |
| [RIPE Atlas](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/ripe_atlas.md) | Metrics for RIPE Atlas |
| [SmartRG 808AC Cable Modem](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/smartrg_808ac_cable_modem.md) | Metrics for SmartRG 808AC Cable Modem |
| [SONiC NOS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/sonic_nos.md) | Metrics for SONiC NOS |
| [Starlink (SpaceX)](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/starlink_spacex.md) | Metrics for Starlink (SpaceX) |
| [Traceroute](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/traceroute.md) | Metrics for Traceroute |
| [Ubiquiti UFiber OLT](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/ubiquiti_ufiber_olt.md) | Metrics for Ubiquiti UFiber OLT |
| [Zyxel GS1200-8](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/zyxel_gs1200-8.md) | Metrics for Zyxel GS1200-8 |

### Synthetic Checks

| Integration | Description |
|-------------|-------------|
| [Blackbox](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/blackbox.md) | Metrics for Blackbox |
| [Domain expiration date](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/whoisquery/integrations/domain_expiration_date.md) | Metrics for Domain expiration date |
| [HTTP Endpoints](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/httpcheck/integrations/http_endpoints.md) | Metrics for HTTP Endpoints |
| [Idle OS Jitter](https://github.com/netdata/netdata/blob/master/src/collectors/idlejitter.plugin/integrations/idle_os_jitter.md) | Metrics for Idle OS Jitter |
| [IOPing](https://github.com/netdata/netdata/blob/master/src/collectors/ioping.plugin/integrations/ioping.md) | Metrics for IOPing |
| [Monit](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/monit/integrations/monit.md) | Metrics for Monit |
| [Ping](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/ping/integrations/ping.md) | Metrics for Ping |
| [Pingdom](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/pingdom.md) | Metrics for Pingdom |
| [Site 24x7](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/site_24x7.md) | Metrics for Site 24x7 |
| [TCP/UDP Endpoints](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/portcheck/integrations/tcp-udp_endpoints.md) | Metrics for TCP/UDP Endpoints |
| [Uptimerobot](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/uptimerobot.md) | Metrics for Uptimerobot |
| [X.509 certificate](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/x509check/integrations/x.509_certificate.md) | Metrics for X.509 certificate |

### CICD Platforms

| Integration | Description |
|-------------|-------------|
| [Concourse](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/concourse.md) | Metrics for Concourse |
| [GitLab Runner](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/gitlab_runner.md) | Metrics for GitLab Runner |
| [Jenkins](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/jenkins.md) | Metrics for Jenkins |
| [Puppet](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/puppet/integrations/puppet.md) | Metrics for Puppet |

### UPS

| Integration | Description |
|-------------|-------------|
| [APC UPS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/apcupsd/integrations/apc_ups.md) | Metrics for APC UPS |
| [Eaton UPS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/eaton_ups.md) | Metrics for Eaton UPS |
| [UPS (NUT)](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/upsd/integrations/ups_nut.md) | Metrics for UPS (NUT) |

### Logs Servers

| Integration | Description |
|-------------|-------------|
| [AuthLog](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/authlog.md) | Metrics for AuthLog |
| [Fluentd](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/fluentd/integrations/fluentd.md) | Metrics for Fluentd |
| [Graylog Server](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/graylog_server.md) | Metrics for Graylog Server |
| [journald](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/journald.md) | Metrics for journald |
| [Logstash](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/logstash/integrations/logstash.md) | Metrics for Logstash |
| [loki](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/loki.md) | Metrics for loki |
| [mtail](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/mtail.md) | Metrics for mtail |

### Security Systems

| Integration | Description |
|-------------|-------------|
| [Certificate Transparency](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/certificate_transparency.md) | Metrics for Certificate Transparency |
| [ClamAV daemon](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/clamav_daemon.md) | Metrics for ClamAV daemon |
| [Clamscan results](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/clamscan_results.md) | Metrics for Clamscan results |
| [Crowdsec](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/crowdsec.md) | Metrics for Crowdsec |
| [Honeypot](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/honeypot.md) | Metrics for Honeypot |
| [Lynis audit reports](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/lynis_audit_reports.md) | Metrics for Lynis audit reports |
| [OpenVAS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/openvas.md) | Metrics for OpenVAS |
| [Rspamd](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/rspamd/integrations/rspamd.md) | Metrics for Rspamd |
| [SSL Certificate](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/ssl_certificate.md) | Metrics for SSL Certificate |
| [Suricata](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/suricata.md) | Metrics for Suricata |
| [Vault PKI](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/vault_pki.md) | Metrics for Vault PKI |

### Observability

| Integration | Description |
|-------------|-------------|
| [Collectd](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/collectd.md) | Metrics for Collectd |
| [Dynatrace](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/dynatrace.md) | Metrics for Dynatrace |
| [Grafana](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/grafana.md) | Metrics for Grafana |
| [Hubble](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/hubble.md) | Metrics for Hubble |
| [Naemon](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/naemon.md) | Metrics for Naemon |
| [Nagios](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/nagios.md) | Metrics for Nagios |
| [New Relic](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/new_relic.md) | Metrics for New Relic |

### Gaming

| Integration | Description |
|-------------|-------------|
| [BungeeCord](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/bungeecord.md) | Metrics for BungeeCord |
| [Minecraft](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/minecraft.md) | Metrics for Minecraft |
| [OpenRCT2](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/openrct2.md) | Metrics for OpenRCT2 |
| [SpigotMC](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/spigotmc/integrations/spigotmc.md) | Metrics for SpigotMC |
| [Steam](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/steam.md) | Metrics for Steam |

### IoT Devices

| Integration | Description |
|-------------|-------------|
| [Airthings Waveplus air sensor](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/airthings_waveplus_air_sensor.md) | Metrics for Airthings Waveplus air sensor |
| [Bobcat Miner 300](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/bobcat_miner_300.md) | Metrics for Bobcat Miner 300 |
| [Christ Elektronik CLM5IP power panel](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/christ_elektronik_clm5ip_power_panel.md) | Metrics for Christ Elektronik CLM5IP power panel |
| [CraftBeerPi](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/craftbeerpi.md) | Metrics for CraftBeerPi |
| [Dutch Electricity Smart Meter](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/dutch_electricity_smart_meter.md) | Metrics for Dutch Electricity Smart Meter |
| [Elgato Key Light devices.](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/elgato_key_light_devices..md) | Metrics for Elgato Key Light devices. |
| [Energomera smart power meters](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/energomera_smart_power_meters.md) | Metrics for Energomera smart power meters |
| [Helium hotspot](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/helium_hotspot.md) | Metrics for Helium hotspot |
| [Homebridge](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/homebridge.md) | Metrics for Homebridge |
| [Homey](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/homey.md) | Metrics for Homey |
| [iqAir AirVisual air quality monitors](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/iqair_airvisual_air_quality_monitors.md) | Metrics for iqAir AirVisual air quality monitors |
| [Jarvis Standing Desk](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/jarvis_standing_desk.md) | Metrics for Jarvis Standing Desk |
| [Modbus protocol](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/modbus_protocol.md) | Metrics for Modbus protocol |
| [Monnit Sensors MQTT](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/monnit_sensors_mqtt.md) | Metrics for Monnit Sensors MQTT |
| [MP707 USB thermometer](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/mp707_usb_thermometer.md) | Metrics for MP707 USB thermometer |
| [Nature Remo E lite devices](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/nature_remo_e_lite_devices.md) | Metrics for Nature Remo E lite devices |
| [Netatmo sensors](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/netatmo_sensors.md) | Metrics for Netatmo sensors |
| [OpenHAB](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/openhab.md) | Metrics for OpenHAB |
| [Personal Weather Station](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/personal_weather_station.md) | Metrics for Personal Weather Station |
| [Philips Hue](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/philips_hue.md) | Metrics for Philips Hue |
| [Pimoroni Enviro+](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/pimoroni_enviro+.md) | Metrics for Pimoroni Enviro+ |
| [Powerpal devices](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/powerpal_devices.md) | Metrics for Powerpal devices |
| [Radio Thermostat](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/radio_thermostat.md) | Metrics for Radio Thermostat |
| [Salicru EQX inverter](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/salicru_eqx_inverter.md) | Metrics for Salicru EQX inverter |
| [Sense Energy](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/sense_energy.md) | Metrics for Sense Energy |
| [Shelly humidity sensor](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/shelly_humidity_sensor.md) | Metrics for Shelly humidity sensor |
| [SMA Inverters](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/sma_inverters.md) | Metrics for SMA Inverters |
| [Smart meters SML](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/smart_meters_sml.md) | Metrics for Smart meters SML |
| [Solar logging stick](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/solar_logging_stick.md) | Metrics for Solar logging stick |
| [SolarEdge inverters](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/solaredge_inverters.md) | Metrics for SolarEdge inverters |
| [Solis Ginlong 5G inverters](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/solis_ginlong_5g_inverters.md) | Metrics for Solis Ginlong 5G inverters |
| [Sunspec Solar Energy](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/sunspec_solar_energy.md) | Metrics for Sunspec Solar Energy |
| [Tado smart heating solution](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/tado_smart_heating_solution.md) | Metrics for Tado smart heating solution |
| [Tesla Powerwall](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/tesla_powerwall.md) | Metrics for Tesla Powerwall |
| [Tesla vehicle](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/tesla_vehicle.md) | Metrics for Tesla vehicle |
| [Tesla Wall Connector](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/tesla_wall_connector.md) | Metrics for Tesla Wall Connector |
| [TP-Link P110](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/tp-link_p110.md) | Metrics for TP-Link P110 |
| [Xiaomi Mi Flora](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/xiaomi_mi_flora.md) | Metrics for Xiaomi Mi Flora |

### Media Services

| Integration | Description |
|-------------|-------------|
| [Discourse](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/discourse.md) | Metrics for Discourse |
| [Icecast](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/icecast/integrations/icecast.md) | Metrics for Icecast |
| [OBS Studio](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/obs_studio.md) | Metrics for OBS Studio |
| [SABnzbd](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/sabnzbd.md) | Metrics for SABnzbd |
| [Stream](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/stream.md) | Metrics for Stream |
| [Twitch](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/twitch.md) | Metrics for Twitch |
| [Zulip](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/zulip.md) | Metrics for Zulip |

### Authentication and Authorization

| Integration | Description |
|-------------|-------------|
| [Fail2ban](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/fail2ban/integrations/fail2ban.md) | Metrics for Fail2ban |
| [FreeRADIUS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/freeradius/integrations/freeradius.md) | Metrics for FreeRADIUS |
| [HashiCorp Vault secrets](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/hashicorp_vault_secrets.md) | Metrics for HashiCorp Vault secrets |
| [LDAP](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/ldap.md) | Metrics for LDAP |
| [OpenLDAP](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/openldap/integrations/openldap.md) | Metrics for OpenLDAP |
| [OpenLDAP (community)](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/openldap_community.md) | Metrics for OpenLDAP (community) |
| [RADIUS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/radius.md) | Metrics for RADIUS |
| [SSH](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/ssh.md) | Metrics for SSH |
| [TACACS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/tacacs.md) | Metrics for TACACS |

### DNS and DHCP Servers

| Integration | Description |
|-------------|-------------|
| [Akamai Edge DNS Traffic](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/akamai_edge_dns_traffic.md) | Metrics for Akamai Edge DNS Traffic |
| [CoreDNS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/coredns/integrations/coredns.md) | Metrics for CoreDNS |
| [DNS query](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/dnsquery/integrations/dns_query.md) | Metrics for DNS query |
| [DNSBL](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/dnsbl.md) | Metrics for DNSBL |
| [DNSdist](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/dnsdist/integrations/dnsdist.md) | Metrics for DNSdist |
| [Dnsmasq](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/dnsmasq/integrations/dnsmasq.md) | Metrics for Dnsmasq |
| [Dnsmasq DHCP](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/dnsmasq_dhcp/integrations/dnsmasq_dhcp.md) | Metrics for Dnsmasq DHCP |
| [ISC DHCP](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/isc_dhcpd/integrations/isc_dhcp.md) | Metrics for ISC DHCP |
| [NextDNS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/nextdns.md) | Metrics for NextDNS |
| [NSD](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/nsd/integrations/nsd.md) | Metrics for NSD |
| [Pi-hole](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/pihole/integrations/pi-hole.md) | Metrics for Pi-hole |
| [PowerDNS Authoritative Server](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/powerdns/integrations/powerdns_authoritative_server.md) | Metrics for PowerDNS Authoritative Server |
| [PowerDNS Recursor](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/powerdns_recursor/integrations/powerdns_recursor.md) | Metrics for PowerDNS Recursor |
| [Unbound](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/unbound/integrations/unbound.md) | Metrics for Unbound |

### Mail Servers

| Integration | Description |
|-------------|-------------|
| [DMARC](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/dmarc.md) | Metrics for DMARC |
| [Dovecot](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/dovecot/integrations/dovecot.md) | Metrics for Dovecot |
| [Exim](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/exim/integrations/exim.md) | Metrics for Exim |
| [Halon](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/halon.md) | Metrics for Halon |
| [Maildir](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/maildir.md) | Metrics for Maildir |
| [Postfix](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/postfix/integrations/postfix.md) | Metrics for Postfix |

### Processes and System Services

| Integration | Description |
|-------------|-------------|
| [Applications](https://github.com/netdata/netdata/blob/master/src/collectors/apps.plugin/integrations/applications.md) | Metrics for Applications |
| [Supervisor](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/supervisord/integrations/supervisor.md) | Metrics for Supervisor |
| [User Groups](https://github.com/netdata/netdata/blob/master/src/collectors/apps.plugin/integrations/user_groups.md) | Metrics for User Groups |
| [Users](https://github.com/netdata/netdata/blob/master/src/collectors/apps.plugin/integrations/users.md) | Metrics for Users |

### Storage, Mount Points and Filesystems

| Integration | Description |
|-------------|-------------|
| [Adaptec RAID](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/adaptecraid/integrations/adaptec_raid.md) | Metrics for Adaptec RAID |
| [Altaro Backup](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/altaro_backup.md) | Metrics for Altaro Backup |
| [Borg backup](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/borg_backup.md) | Metrics for Borg backup |
| [Ceph](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/ceph/integrations/ceph.md) | Metrics for Ceph |
| [CVMFS clients](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/cvmfs_clients.md) | Metrics for CVMFS clients |
| [Dell EMC Isilon cluster](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/dell_emc_isilon_cluster.md) | Metrics for Dell EMC Isilon cluster |
| [Dell EMC ScaleIO](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/scaleio/integrations/dell_emc_scaleio.md) | Metrics for Dell EMC ScaleIO |
| [Dell EMC XtremIO cluster](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/dell_emc_xtremio_cluster.md) | Metrics for Dell EMC XtremIO cluster |
| [Dell PowerMax](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/dell_powermax.md) | Metrics for Dell PowerMax |
| [DMCache devices](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/dmcache/integrations/dmcache_devices.md) | Metrics for DMCache devices |
| [EOS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/eos.md) | Metrics for EOS |
| [Generic storage enclosure tool](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/generic_storage_enclosure_tool.md) | Metrics for Generic storage enclosure tool |
| [Hadoop Distributed File System (HDFS)](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/hdfs/integrations/hadoop_distributed_file_system_hdfs.md) | Metrics for Hadoop Distributed File System (HDFS) |
| [HDSentinel](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/hdsentinel.md) | Metrics for HDSentinel |
| [HPE Smart Arrays](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/hpssa/integrations/hpe_smart_arrays.md) | Metrics for HPE Smart Arrays |
| [IBM Spectrum](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/ibm_spectrum.md) | Metrics for IBM Spectrum |
| [IBM Spectrum Virtualize](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/ibm_spectrum_virtualize.md) | Metrics for IBM Spectrum Virtualize |
| [IPFS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/ipfs/integrations/ipfs.md) | Metrics for IPFS |
| [Lagerist Disk latency](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/lagerist_disk_latency.md) | Metrics for Lagerist Disk latency |
| [LVM logical volumes](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/lvm/integrations/lvm_logical_volumes.md) | Metrics for LVM logical volumes |
| [MegaCLI MegaRAID](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/megacli/integrations/megacli_megaraid.md) | Metrics for MegaCLI MegaRAID |
| [MogileFS](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/mogilefs.md) | Metrics for MogileFS |
| [Netapp ONTAP API](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/netapp_ontap_api.md) | Metrics for Netapp ONTAP API |
| [NetApp Solidfire](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/netapp_solidfire.md) | Metrics for NetApp Solidfire |
| [NVMe devices](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/nvme/integrations/nvme_devices.md) | Metrics for NVMe devices |
| [Samba](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/samba/integrations/samba.md) | Metrics for Samba |
| [Starwind VSAN VSphere Edition](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/starwind_vsan_vsphere_edition.md) | Metrics for Starwind VSAN VSphere Edition |
| [StoreCLI RAID](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/storcli/integrations/storecli_raid.md) | Metrics for StoreCLI RAID |
| [Storidge](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/storidge.md) | Metrics for Storidge |
| [Synology ActiveBackup](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/synology_activebackup.md) | Metrics for Synology ActiveBackup |
| [ZFS Pools](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/zfspool/integrations/zfs_pools.md) | Metrics for ZFS Pools |

### Systemd

| Integration | Description |
|-------------|-------------|
| [Systemd Services](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/integrations/systemd_services.md) | Metrics for Systemd Services |
| [Systemd Units](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/systemdunits/integrations/systemd_units.md) | Metrics for Systemd Units |
| [systemd-logind users](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/logind/integrations/systemd-logind_users.md) | Metrics for systemd-logind users |

### Telephony Servers

| Integration | Description |
|-------------|-------------|
| [GTP](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/gtp.md) | Metrics for GTP |
| [Kannel](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/kannel.md) | Metrics for Kannel |
| [OpenSIPS](https://github.com/netdata/netdata/blob/master/src/collectors/charts.d.plugin/opensips/integrations/opensips.md) | Metrics for OpenSIPS |

### VPNs

| Integration | Description |
|-------------|-------------|
| [Fastd](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/fastd.md) | Metrics for Fastd |
| [Libreswan](https://github.com/netdata/netdata/blob/master/src/collectors/charts.d.plugin/libreswan/integrations/libreswan.md) | Metrics for Libreswan |
| [OpenVPN](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/openvpn/integrations/openvpn.md) | Metrics for OpenVPN |
| [OpenVPN status log](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/openvpn_status_log/integrations/openvpn_status_log.md) | Metrics for OpenVPN status log |
| [SoftEther VPN Server](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/softether_vpn_server.md) | Metrics for SoftEther VPN Server |
| [Speedify CLI](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/speedify_cli.md) | Metrics for Speedify CLI |
| [strongSwan](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/strongswan.md) | Metrics for strongSwan |
| [Tor](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/tor/integrations/tor.md) | Metrics for Tor |
| [WireGuard](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/wireguard/integrations/wireguard.md) | Metrics for WireGuard |

### Blockchain Servers

| Integration | Description |
|-------------|-------------|
| [Chia](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/chia.md) | Metrics for Chia |
| [Crypto exchanges](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/crypto_exchanges.md) | Metrics for Crypto exchanges |
| [Cryptowatch](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/cryptowatch.md) | Metrics for Cryptowatch |
| [Go-ethereum](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/geth/integrations/go-ethereum.md) | Metrics for Go-ethereum |
| [Helium miner (validator)](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/helium_miner_validator.md) | Metrics for Helium miner (validator) |
| [IOTA full node](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/iota_full_node.md) | Metrics for IOTA full node |
| [Sia](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/sia.md) | Metrics for Sia |

### Distributed Computing Systems

| Integration | Description |
|-------------|-------------|
| [Gearman](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/gearman/integrations/gearman.md) | Metrics for Gearman |

### Generic Data Collection

| Integration | Description |
|-------------|-------------|
| [Custom Exporter](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/custom_exporter.md) | Metrics for Custom Exporter |
| [Excel spreadsheet](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/excel_spreadsheet.md) | Metrics for Excel spreadsheet |
| [Generic Command Line Output](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/generic_command_line_output.md) | Metrics for Generic Command Line Output |
| [JetBrains Floating License Server](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/jetbrains_floating_license_server.md) | Metrics for JetBrains Floating License Server |
| [OpenWeatherMap](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/openweathermap.md) | Metrics for OpenWeatherMap |
| [Pandas](https://github.com/netdata/netdata/blob/master/src/collectors/python.d.plugin/pandas/integrations/pandas.md) | Metrics for Pandas |
| [Prometheus endpoint](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/prometheus_endpoint.md) | Metrics for Prometheus endpoint |
| [Shell command](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/shell_command.md) | Metrics for Shell command |
| [SNMP devices](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/snmp/integrations/snmp_devices.md) | Metrics for SNMP devices |
| [Tankerkoenig API](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/tankerkoenig_api.md) | Metrics for Tankerkoenig API |
| [TwinCAT ADS Web Service](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/twincat_ads_web_service.md) | Metrics for TwinCAT ADS Web Service |

### System Clock and NTP

| Integration | Description |
|-------------|-------------|
| [Chrony](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/chrony/integrations/chrony.md) | Metrics for Chrony |
| [NTPd](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/ntpd/integrations/ntpd.md) | Metrics for NTPd |
| [Timex](https://github.com/netdata/netdata/blob/master/src/collectors/timex.plugin/integrations/timex.md) | Metrics for Timex |

### Task Queues

| Integration | Description |
|-------------|-------------|
| [Celery](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/celery.md) | Metrics for Celery |
| [Mesos](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/mesos.md) | Metrics for Mesos |
| [Slurm](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/slurm.md) | Metrics for Slurm |

### FTP Servers

| Integration | Description |
|-------------|-------------|
| [ProFTPD](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/proftpd.md) | Metrics for ProFTPD |

### Other

| Integration | Description |
|-------------|-------------|
| [Files and directories](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/filecheck/integrations/files_and_directories.md) | Metrics for Files and directories |
| [GitHub API rate limit](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/github_api_rate_limit.md) | Metrics for GitHub API rate limit |
| [GitHub repository](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/prometheus/integrations/github_repository.md) | Metrics for GitHub repository |
