// SPDX-License-Identifier: GPL-3.0-or-later

// Codacy declarations
/* global NETDATA */

var netdataDashboard = window.netdataDashboard || {};

// Informational content for the various sections of the GUI (menus, sections, charts, etc.)

// ----------------------------------------------------------------------------
// Menus

netdataDashboard.menu = {
    'system': {
        title: 'System Overview',
        icon: '<i class="fas fa-bookmark"></i>',
        info: 'Overview of the key system metrics.'
    },

    'services': {
        title: 'systemd Services',
        icon: '<i class="fas fa-cogs"></i>',
        info: 'Resources utilization of systemd services. netdata monitors all systemd services via CGROUPS ' +
            '(the resources accounting used by containers). '
    },

    'ap': {
        title: 'Access Points',
        icon: '<i class="fas fa-wifi"></i>',
        info: 'Performance metrics for the access points (i.e. wireless interfaces in AP mode) found on the system.'
    },

    'tc': {
        title: 'Quality of Service',
        icon: '<i class="fas fa-globe"></i>',
        info: 'Netdata collects and visualizes <code>tc</code> class utilization using its ' +
            '<a href="https://github.com/netdata/netdata/blob/master/collectors/tc.plugin/tc-qos-helper.sh.in" target="_blank">tc-helper plugin</a>. ' +
            'If you also use <a href="http://firehol.org/#fireqos" target="_blank">FireQOS</a> for setting up QoS, ' +
            'netdata automatically collects interface and class names. If your QoS configuration includes overheads ' +
            'calculation, the values shown here will include these overheads (the total bandwidth for the same ' +
            'interface as reported in the Network Interfaces section, will be lower than the total bandwidth ' +
            'reported here). QoS data collection may have a slight time difference compared to the interface ' +
            '(QoS data collection uses a BASH script, so a shift in data collection of a few milliseconds ' +
            'should be justified).'
    },

    'net': {
        title: 'Network Interfaces',
        icon: '<i class="fas fa-sitemap"></i>',
        info: 'Performance metrics for network interfaces.'
    },

    'wireless': {
        title: 'Wireless Interfaces',
        icon: '<i class="fas fa-wifi"></i>',
        info: 'Performance metrics for wireless interfaces.'
    },

    'ip': {
        title: 'Networking Stack',
        icon: '<i class="fas fa-cloud"></i>',
        info: function (os) {
            if (os === "linux")
                return 'Metrics for the networking stack of the system. These metrics are collected from <code>/proc/net/netstat</code>, apply to both IPv4 and IPv6 traffic and are related to operation of the kernel networking stack.';
            else
                return 'Metrics for the networking stack of the system.';
        }
    },

    'ipv4': {
        title: 'IPv4 Networking',
        icon: '<i class="fas fa-cloud"></i>',
        info: 'Metrics for the IPv4 stack of the system. ' +
            '<a href="https://en.wikipedia.org/wiki/IPv4" target="_blank">Internet Protocol version 4 (IPv4)</a> is ' +
            'the fourth version of the Internet Protocol (IP). It is one of the core protocols of standards-based ' +
            'internetworking methods in the Internet. IPv4 is a connectionless protocol for use on packet-switched ' +
            'networks. It operates on a best effort delivery model, in that it does not guarantee delivery, nor does ' +
            'it assure proper sequencing or avoidance of duplicate delivery. These aspects, including data integrity, ' +
            'are addressed by an upper layer transport protocol, such as the Transmission Control Protocol (TCP).'
    },

    'ipv6': {
        title: 'IPv6 Networking',
        icon: '<i class="fas fa-cloud"></i>',
        info: 'Metrics for the IPv6 stack of the system. <a href="https://en.wikipedia.org/wiki/IPv6" target="_blank">Internet Protocol version 6 (IPv6)</a> is the most recent version of the Internet Protocol (IP), the communications protocol that provides an identification and location system for computers on networks and routes traffic across the Internet. IPv6 was developed by the Internet Engineering Task Force (IETF) to deal with the long-anticipated problem of IPv4 address exhaustion. IPv6 is intended to replace IPv4.'
    },

    'sctp': {
        title: 'SCTP Networking',
        icon: '<i class="fas fa-cloud"></i>',
        info: '<a href="https://en.wikipedia.org/wiki/Stream_Control_Transmission_Protocol" target="_blank">Stream Control Transmission Protocol (SCTP)</a> is a computer network protocol which operates at the transport layer and serves a role similar to the popular protocols TCP and UDP. SCTP provides some of the features of both UDP and TCP: it is message-oriented like UDP and ensures reliable, in-sequence transport of messages with congestion control like TCP. It differs from those protocols by providing multi-homing and redundant paths to increase resilience and reliability.'
    },

    'ipvs': {
        title: 'IP Virtual Server',
        icon: '<i class="fas fa-eye"></i>',
        info: '<a href="http://www.linuxvirtualserver.org/software/ipvs.html" target="_blank">IPVS (IP Virtual Server)</a> implements transport-layer load balancing inside the Linux kernel, so called Layer-4 switching. IPVS running on a host acts as a load balancer at the front of a cluster of real servers, it can direct requests for TCP/UDP based services to the real servers, and makes services of the real servers to appear as a virtual service on a single IP address.'
    },

    'netfilter': {
        title: 'Firewall (netfilter)',
        icon: '<i class="fas fa-shield-alt"></i>',
        info: 'Performance metrics of the netfilter components.'
    },

    'ipfw': {
        title: 'Firewall (ipfw)',
        icon: '<i class="fas fa-shield-alt"></i>',
        info: 'Counters and memory usage for the ipfw rules.'
    },

    'cpu': {
        title: 'CPUs',
        icon: '<i class="fas fa-bolt"></i>',
        info: 'Detailed information for each CPU of the system. A summary of the system for all CPUs can be found at the <a href="#menu_system">System Overview</a> section.'
    },

    'mem': {
        title: 'Memory',
        icon: '<i class="fas fa-microchip"></i>',
        info: 'Detailed information about the memory management of the system.'
    },

    'disk': {
        title: 'Disks',
        icon: '<i class="fas fa-hdd"></i>',
        info: 'Charts with performance information for all the system disks. Special care has been given to present disk performance metrics in a way compatible with <code>iostat -x</code>. netdata by default prevents rendering performance charts for individual partitions and unmounted virtual disks. Disabled charts can still be enabled by configuring the relative settings in the netdata configuration file.'
    },

    'mdstat': {
        title: 'MD arrays',
        icon: '<i class="fas fa-hdd"></i>'
    },

    'sensors': {
        title: 'Sensors',
        icon: '<i class="fas fa-leaf"></i>',
        info: 'Readings of the configured system sensors.'
    },

    'ipmi': {
        title: 'IPMI',
        icon: '<i class="fas fa-leaf"></i>',
        info: 'The Intelligent Platform Management Interface (IPMI) is a set of computer interface specifications for an autonomous computer subsystem that provides management and monitoring capabilities independently of the host system\'s CPU, firmware (BIOS or UEFI) and operating system.'
    },

    'samba': {
        title: 'Samba',
        icon: '<i class="fas fa-folder-open"></i>',
        info: 'Performance metrics of the Samba file share operations of this system. Samba is a implementation of Windows services, including Windows SMB protocol file shares.'
    },

    'nfsd': {
        title: 'NFS Server',
        icon: '<i class="fas fa-folder-open"></i>',
        info: 'Performance metrics of the Network File Server. NFS is a distributed file system protocol, allowing a user on a client computer to access files over a network, much like local storage is accessed. NFS, like many other protocols, builds on the Open Network Computing Remote Procedure Call (ONC RPC) system. The NFS is an open standard defined in Request for Comments (RFC).'
    },

    'nfs': {
        title: 'NFS Client',
        icon: '<i class="fas fa-folder-open"></i>',
        info: 'Performance metrics of the NFS operations of this system, acting as an NFS client.'
    },

    'zfs': {
        title: 'ZFS filesystem',
        icon: '<i class="fas fa-folder-open"></i>',
        info: 'Performance metrics of the ZFS filesystem. The following charts visualize all metrics reported by <a href="https://github.com/zfsonlinux/zfs/blob/master/cmd/arcstat/arcstat" target="_blank">arcstat.py</a> and <a href="https://github.com/zfsonlinux/zfs/blob/master/cmd/arc_summary/arc_summary3" target="_blank">arc_summary.py</a>.'
    },

    'zfspool': {
        title: 'ZFS pools',
        icon: '<i class="fas fa-database"></i>',
        info: 'State of ZFS pools.'
    },

    'btrfs': {
        title: 'BTRFS filesystem',
        icon: '<i class="fas fa-folder-open"></i>',
        info: 'Disk space metrics for the BTRFS filesystem.'
    },

    'apps': {
        title: 'Applications',
        icon: '<i class="fas fa-heartbeat"></i>',
        info: 'Per application statistics are collected using netdata\'s <code>apps.plugin</code>. This plugin walks through all processes and aggregates statistics for applications of interest, defined in <code>/etc/netdata/apps_groups.conf</code>, which can be edited by running <code>$ /etc/netdata/edit-config apps_groups.conf</code> (the default is <a href="https://github.com/netdata/netdata/blob/master/collectors/apps.plugin/apps_groups.conf" target="_blank">here</a>). The plugin internally builds a process tree (much like <code>ps fax</code> does), and groups processes together (evaluating both child and parent processes) so that the result is always a chart with a predefined set of dimensions (of course, only application groups found running are reported). The reported values are compatible with <code>top</code>, although the netdata plugin counts also the resources of exited children (unlike <code>top</code> which shows only the resources of the currently running processes). So for processes like shell scripts, the reported values include the resources used by the commands these scripts run within each timeframe.',
        height: 1.5
    },

    'users': {
        title: 'Users',
        icon: '<i class="fas fa-user"></i>',
        info: 'Per user statistics are collected using netdata\'s <code>apps.plugin</code>. This plugin walks through all processes and aggregates statistics per user. The reported values are compatible with <code>top</code>, although the netdata plugin counts also the resources of exited children (unlike <code>top</code> which shows only the resources of the currently running processes). So for processes like shell scripts, the reported values include the resources used by the commands these scripts run within each timeframe.',
        height: 1.5
    },

    'groups': {
        title: 'User Groups',
        icon: '<i class="fas fa-users"></i>',
        info: 'Per user group statistics are collected using netdata\'s <code>apps.plugin</code>. This plugin walks through all processes and aggregates statistics per user group. The reported values are compatible with <code>top</code>, although the netdata plugin counts also the resources of exited children (unlike <code>top</code> which shows only the resources of the currently running processes). So for processes like shell scripts, the reported values include the resources used by the commands these scripts run within each timeframe.',
        height: 1.5
    },

    'netdata': {
        title: 'Netdata Monitoring',
        icon: '<i class="fas fa-chart-bar"></i>',
        info: 'Performance metrics for the operation of netdata itself and its plugins.'
    },

    'aclk_test': {
        title: 'ACLK Test Generator',
        info: 'For internal use to perform integration testing.'
    },

    'example': {
        title: 'Example Charts',
        info: 'Example charts, demonstrating the external plugin architecture.'
    },

    'cgroup': {
        title: '',
        icon: '<i class="fas fa-th"></i>',
        info: 'Container resource utilization metrics. Netdata reads this information from <b>cgroups</b> (abbreviated from <b>control groups</b>), a Linux kernel feature that limits and accounts resource usage (CPU, memory, disk I/O, network, etc.) of a collection of processes. <b>cgroups</b> together with <b>namespaces</b> (that offer isolation between processes) provide what we usually call: <b>containers</b>.'
    },

    'cgqemu': {
        title: '',
        icon: '<i class="fas fa-th-large"></i>',
        info: 'QEMU virtual machine resource utilization metrics. QEMU (short for Quick Emulator) is a free and open-source hosted hypervisor that performs hardware virtualization.'
    },

    'fping': {
        title: 'fping',
        icon: '<i class="fas fa-exchange-alt"></i>',
        info: 'Network latency statistics, via <b>fping</b>. <b>fping</b> is a program to send ICMP echo probes to network hosts, similar to <code>ping</code>, but much better performing when pinging multiple hosts. fping versions after 3.15 can be directly used as netdata plugins.'
    },

    'gearman': {
        title: 'Gearman',
        icon: '<i class="fas fa-tasks"></i>',
        info: 'Gearman is a job server that allows you to do work in parallel, to load balance processing, and to call functions between languages.'
    },

    'ioping': {
        title: 'ioping',
        icon: '<i class="fas fa-exchange-alt"></i>',
        info: 'Disk latency statistics, via <b>ioping</b>. <b>ioping</b> is a program to read/write data probes from/to a disk.'
    },

    'httpcheck': {
        title: 'Http Check',
        icon: '<i class="fas fa-heartbeat"></i>',
        info: 'Web Service availability and latency monitoring using HTTP checks. This plugin is a specialized version of the port check plugin.'
    },

    'memcached': {
        title: 'memcached',
        icon: '<i class="fas fa-database"></i>',
        info: 'Performance metrics for <b>memcached</b>. Memcached is a general-purpose distributed memory caching system. It is often used to speed up dynamic database-driven websites by caching data and objects in RAM to reduce the number of times an external data source (such as a database or API) must be read.'
    },

    'monit': {
        title: 'monit',
        icon: '<i class="fas fa-database"></i>',
        info: 'Statuses of checks in <b>monit</b>. Monit is a utility for managing and monitoring processes, programs, files, directories and filesystems on a Unix system. Monit conducts automatic maintenance and repair and can execute meaningful causal actions in error situations.'
    },

    'mysql': {
        title: 'MySQL',
        icon: '<i class="fas fa-database"></i>',
        info: 'Performance metrics for <b>mysql</b>, the open-source relational database management system (RDBMS).'
    },

    'postgres': {
        title: 'Postgres',
        icon: '<i class="fas fa-database"></i>',
        info: 'Performance metrics for <b>PostgresSQL</b>, the object-relational database (ORDBMS).'
    },

    'redis': {
        title: 'Redis',
        icon: '<i class="fas fa-database"></i>',
        info: 'Performance metrics for <b>redis</b>. Redis (REmote DIctionary Server) is a software project that implements data structure servers. It is open-source, networked, in-memory, and stores keys with optional durability.'
    },

    'rethinkdbs': {
        title: 'RethinkDB',
        icon: '<i class="fas fa-database"></i>',
        info: 'Performance metrics for <b>rethinkdb</b>. RethinkDB is the first open-source scalable database built for realtime applications'
    },

    'retroshare': {
        title: 'RetroShare',
        icon: '<i class="fas fa-share-alt"></i>',
        info: 'Performance metrics for <b>RetroShare</b>. RetroShare is open source software for encrypted filesharing, serverless email, instant messaging, online chat, and BBS, based on a friend-to-friend network built on GNU Privacy Guard (GPG).'
    },

    'riakkv': {
        title: 'Riak KV',
        icon: '<i class="fas fa-database"></i>',
        info: 'Metrics for <b>Riak KV</b>, the distributed key-value store.'
    },

    'ipfs': {
        title: 'IPFS',
        icon: '<i class="fas fa-folder-open"></i>',
        info: 'Performance metrics for the InterPlanetary File System (IPFS), a content-addressable, peer-to-peer hypermedia distribution protocol.'
    },

    'phpfpm': {
        title: 'PHP-FPM',
        icon: '<i class="fas fa-eye"></i>',
        info: 'Performance metrics for <b>PHP-FPM</b>, an alternative FastCGI implementation for PHP.'
    },

    'pihole': {
        title: 'Pi-hole',
        icon: '<i class="fas fa-ban"></i>',
        info: 'Metrics for <a href="https://pi-hole.net/" target="_blank">Pi-hole</a>, a black hole for Internet advertisements.' +
            ' The metrics returned by Pi-Hole API is all from the last 24 hours.'
    },

    'portcheck': {
        title: 'Port Check',
        icon: '<i class="fas fa-heartbeat"></i>',
        info: 'Service availability and latency monitoring using port checks.'
    },

    'postfix': {
        title: 'postfix',
        icon: '<i class="fas fa-envelope"></i>',
        info: undefined
    },

    'dovecot': {
        title: 'Dovecot',
        icon: '<i class="fas fa-envelope"></i>',
        info: undefined
    },

    'hddtemp': {
        title: 'HDD Temp',
        icon: '<i class="fas fa-thermometer-half"></i>',
        info: undefined
    },

    'nginx': {
        title: 'nginx',
        icon: '<i class="fas fa-eye"></i>',
        info: undefined
    },

    'apache': {
        title: 'Apache',
        icon: '<i class="fas fa-eye"></i>',
        info: undefined
    },

    'lighttpd': {
        title: 'Lighttpd',
        icon: '<i class="fas fa-eye"></i>',
        info: undefined
    },

    'web_log': {
        title: undefined,
        icon: '<i class="fas fa-file-alt"></i>',
        info: 'Information extracted from a server log file. <code>web_log</code> plugin incrementally parses the server log file to provide, in real-time, a break down of key server performance metrics. For web servers, an extended log file format may optionally be used (for <code>nginx</code> and <code>apache</code>) offering timing information and bandwidth for both requests and responses. <code>web_log</code> plugin may also be configured to provide a break down of requests per URL pattern (check <a href="https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/web_log/web_log.conf" target="_blank"><code>/etc/netdata/python.d/web_log.conf</code></a>).'
    },

    'named': {
        title: 'named',
        icon: '<i class="fas fa-tag"></i>',
        info: undefined
    },

    'squid': {
        title: 'squid',
        icon: '<i class="fas fa-exchange-alt"></i>',
        info: undefined
    },

    'nut': {
        title: 'UPS',
        icon: '<i class="fas fa-battery-half"></i>',
        info: undefined
    },

    'apcupsd': {
        title: 'UPS',
        icon: '<i class="fas fa-battery-half"></i>',
        info: undefined
    },

    'smawebbox': {
        title: 'Solar Power',
        icon: '<i class="fas fa-sun"></i>',
        info: undefined
    },

    'fronius': {
        title: 'Fronius',
        icon: '<i class="fas fa-sun"></i>',
        info: undefined
    },

    'stiebeleltron': {
        title: 'Stiebel Eltron',
        icon: '<i class="fas fa-thermometer-half"></i>',
        info: undefined
    },

    'snmp': {
        title: 'SNMP',
        icon: '<i class="fas fa-random"></i>',
        info: undefined
    },

    'go_expvar': {
        title: 'Go - expvars',
        icon: '<i class="fas fa-eye"></i>',
        info: 'Statistics about running Go applications exposed by the <a href="https://golang.org/pkg/expvar/" target="_blank">expvar package</a>.'
    },

    'chrony': {
        icon: '<i class="fas fa-clock"></i>',
        info: 'chronyd parameters about the system’s clock performance.'
    },

    'couchdb': {
        icon: '<i class="fas fa-database"></i>',
        info: 'Performance metrics for <b><a href="https://couchdb.apache.org/">CouchDB</a></b>, the open-source, JSON document-based database with an HTTP API and multi-master replication.'
    },

    'beanstalk': {
        title: 'Beanstalkd',
        icon: '<i class="fas fa-tasks"></i>',
        info: 'Provides statistics on the <b><a href="http://kr.github.io/beanstalkd/">beanstalkd</a></b> server and any tubes available on that server using data pulled from beanstalkc'
    },

    'rabbitmq': {
        title: 'RabbitMQ',
        icon: '<i class="fas fa-comments"></i>',
        info: 'Performance data for the <b><a href="https://www.rabbitmq.com/">RabbitMQ</a></b> open-source message broker.'
    },

    'ceph': {
        title: 'Ceph',
        icon: '<i class="fas fa-database"></i>',
        info: 'Provides statistics on the <b><a href="http://ceph.com/">ceph</a></b> cluster server, the open-source distributed storage system.'
    },

    'ntpd': {
        title: 'ntpd',
        icon: '<i class="fas fa-clock"></i>',
        info: 'Provides statistics for the internal variables of the Network Time Protocol daemon <b><a href="http://www.ntp.org/">ntpd</a></b> and optional including the configured peers (if enabled in the module configuration). The module presents the performance metrics as shown by <b><a href="http://doc.ntp.org/current-stable/ntpq.html">ntpq</a></b> (the standard NTP query program) using NTP mode 6 UDP packets to communicate with the NTP server.'
    },

    'spigotmc': {
        title: 'Spigot MC',
        icon: '<i class="fas fa-eye"></i>',
        info: 'Provides basic performance statistics for the <b><a href="https://www.spigotmc.org/">Spigot Minecraft</a></b> server.'
    },

    'unbound': {
        title: 'Unbound',
        icon: '<i class="fas fa-tag"></i>',
        info: undefined
    },

    'boinc': {
        title: 'BOINC',
        icon: '<i class="fas fa-microchip"></i>',
        info: 'Provides task counts for <b><a href="http://boinc.berkeley.edu/">BOINC</a></b> distributed computing clients.'
    },

    'w1sensor': {
        title: '1-Wire Sensors',
        icon: '<i class="fas fa-thermometer-half"></i>',
        info: 'Data derived from <a href="https://en.wikipedia.org/wiki/1-Wire">1-Wire</a> sensors.  Currently temperature sensors are automatically detected.'
    },

    'logind': {
        title: 'Logind',
        icon: '<i class="fas fa-user"></i>',
        info: undefined
    },

    'powersupply': {
        title: 'Power Supply',
        icon: '<i class="fas fa-battery-half"></i>',
        info: 'Statistics for the various system power supplies. Data collected from <a href="https://www.kernel.org/doc/Documentation/power/power_supply_class.txt">Linux power supply class</a>.'
    },

    'xenstat': {
        title: 'Xen Node',
        icon: '<i class="fas fa-server"></i>',
        info: 'General statistics for the Xen node. Data collected using <b>xenstat</b> library</a>.'
    },

    'xendomain': {
        title: '',
        icon: '<i class="fas fa-th-large"></i>',
        info: 'Xen domain resource utilization metrics. Netdata reads this information using <b>xenstat</b> library which gives access to the resource usage information (CPU, memory, disk I/O, network) for a virtual machine.'
    },

    'wmi': {
        title: 'wmi',
        icon: '<i class="fas fa-server"></i>',
        info: undefined
    },

    'perf': {
        title: 'Perf Counters',
        icon: '<i class="fas fa-tachometer-alt"></i>',
        info: 'Performance Monitoring Counters (PMC). Data collected using <b>perf_event_open()</b> system call which utilises Hardware Performance Monitoring Units (PMU).'
    },

    'vsphere': {
        title: 'vSphere',
        icon: '<i class="fas fa-server"></i>',
        info: 'Performance statistics for ESXI hosts and virtual machines. Data collected from <a href="https://www.vmware.com/products/vcenter-server.html">VMware vCenter Server</a> using <code><a href="https://github.com/vmware/govmomi"> govmomi</a></code>  library.'
    },

    'vcsa': {
        title: 'VCSA',
        icon: '<i class="fas fa-server"></i>',
        info: 'vCenter Server Appliance health statistics. Data collected from <a href="https://vmware.github.io/vsphere-automation-sdk-rest/vsphere/index.html#SVC_com.vmware.appliance.health">Health API</a>.'
    },

    'zookeeper': {
        title: 'Zookeeper',
        icon: '<i class="fas fa-database"></i>',
        info: 'Provides health statistics for <b><a href="https://zookeeper.apache.org/">Zookeeper</a></b> server. Data collected through the command port using <code><a href="https://zookeeper.apache.org/doc/r3.5.5/zookeeperAdmin.html#sc_zkCommands">mntr</a></code> command.'
    },

    'hdfs': {
        title: 'HDFS',
        icon: '<i class="fas fa-folder-open"></i>',
        info: 'Provides <b><a href="https://hadoop.apache.org/docs/r3.2.0/hadoop-project-dist/hadoop-hdfs/HdfsDesign.html">Hadoop Distributed File System</a></b> performance statistics. Module collects metrics over <code>Java Management Extensions</code> through the web interface of an <code>HDFS</code> daemon.'
    },

    'am2320': {
        title: 'AM2320 Sensor',
        icon: '<i class="fas fa-thermometer-half"></i>',
        info: 'Readings from the external AM2320 Sensor.'
    },

    'scaleio': {
        title: 'ScaleIO',
        icon: '<i class="fas fa-database"></i>',
        info: 'Performance and health statistics for various ScaleIO components. Data collected via VxFlex OS Gateway REST API.'
    },

    'squidlog': {
        title: 'Squid log',
        icon: '<i class="fas fa-file-alt"></i>',
        info: undefined
    },

    'cockroachdb': {
        title: 'CockroachDB',
        icon: '<i class="fas fa-database"></i>',
        info: 'Performance and health statistics for various <code>CockroachDB</code> components.'
    },

    'ebpf': {
        title: 'eBPF',
        icon: '<i class="fas fa-heartbeat"></i>',
        info: 'Monitor system calls, internal functions, bytes read, bytes written and errors using <code>eBPF</code>.'
    },

    'filesystem': {
        title: 'Filesystem',
        icon: '<i class="fas fa-hdd"></i>',
    },

    'vernemq': {
        title: 'VerneMQ',
        icon: '<i class="fas fa-comments"></i>',
        info: 'Performance data for the <b><a href="https://vernemq.com/">VerneMQ</a></b> open-source MQTT broker.'
    },

    'pulsar': {
        title: 'Pulsar',
        icon: '<i class="fas fa-comments"></i>',
        info: 'Summary, namespaces and topics performance data for the <b><a href="http://pulsar.apache.org/">Apache Pulsar</a></b> pub-sub messaging system.'
    },

    'anomalies': {
        title: 'Anomalies',
        icon: '<i class="fas fa-flask"></i>',
        info: 'Anomaly scores relating to key system metrics. A high anomaly probability indicates strange behaviour and may trigger an anomaly prediction from the trained models. Read the <a href="https://github.com/netdata/netdata/tree/master/collectors/python.d.plugin/anomalies" target="_blank">anomalies collector docs</a> for more details.'
    },

    'alarms': {
        title: 'Alarms',
        icon: '<i class="fas fa-bell"></i>',
        info: 'Charts showing alarm status over time. More details <a href="https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/alarms/README.md" target="_blank">here</a>.'
    },

    'statsd': { 
        title: 'StatsD',
        icon: '<i class="fas fa-chart-line"></i>',
        info:'StatsD is an industry-standard technology stack for monitoring applications and instrumenting any piece of software to deliver custom metrics. Netdata allows the user to organize the metrics in different charts and visualize any application metric easily. Read more on <a href="https://learn.netdata.cloud/docs/agent/collectors/statsd.plugin">Netdata Learn</a>.'
    },

    'supervisord': {
        title: 'Supervisord',
        icon: '<i class="fas fa-tasks"></i>',
        info: 'Detailed statistics for each group of processes controlled by <b><a href="http://supervisord.org/">Supervisor</a></b>. ' +
        'Netdata collects these metrics using <a href="http://supervisord.org/api.html#supervisor.rpcinterface.SupervisorNamespaceRPCInterface.getAllProcessInfo" target="_blank"><code>getAllProcessInfo</code></a> method.'
    },

    'systemdunits': {
        title: 'systemd units',
        icon: '<i class="fas fa-cogs"></i>',
        info: '<b>systemd</b> provides a dependency system between various entities called "units" of 11 different types. ' +
        'Units encapsulate various objects that are relevant for system boot-up and maintenance. ' +
        'Units may be <code>active</code> (meaning started, bound, plugged in, depending on the unit type), ' +
        'or <code>inactive</code> (meaning stopped, unbound, unplugged), ' +
        'as well as in the process of being activated or deactivated, i.e. between the two states (these states are called <code>activating</code>, <code>deactivating</code>). ' +
        'A special <code>failed</code> state is available as well, which is very similar to <code>inactive</code> and is entered when the service failed in some way (process returned error code on exit, or crashed, an operation timed out, or after too many restarts). ' +
        'For detailes, see <a href="https://www.freedesktop.org/software/systemd/man/systemd.html" target="_blank"> systemd(1)</a>.'
    },
    
    'changefinder': {
        title: 'ChangeFinder',
        icon: '<i class="fas fa-flask"></i>',
        info: 'Online changepoint detection using machine learning. More details <a href="https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/changefinder/README.md" target="_blank">here</a>.'
    },

    'zscores': {
        title: 'Z-Scores',
        icon: '<i class="fas fa-exclamation"></i>',
        info: 'Z scores scores relating to key system metrics.'
    },

};


// ----------------------------------------------------------------------------
// submenus

// information to be shown, just below each submenu

// information about the submenus
netdataDashboard.submenu = {
    'web_log.squid_bandwidth': {
        title: 'bandwidth',
        info: 'Bandwidth of responses (<code>sent</code>) by squid. This chart may present unusual spikes, since the bandwidth is accounted at the time the log line is saved by the server, even if the time needed to serve it spans across a longer duration. We suggest to use QoS (e.g. <a href="http://firehol.org/#fireqos" target="_blank">FireQOS</a>) for accurate accounting of the server bandwidth.'
    },

    'web_log.squid_responses': {
        title: 'responses',
        info: 'Information related to the responses sent by squid.'
    },

    'web_log.squid_requests': {
        title: 'requests',
        info: 'Information related to the requests squid has received.'
    },

    'web_log.squid_hierarchy': {
        title: 'hierarchy',
        info: 'Performance metrics for the squid hierarchy used to serve the requests.'
    },

    'web_log.squid_squid_transport': {
        title: 'transport'
    },

    'web_log.squid_squid_cache': {
        title: 'cache',
        info: 'Performance metrics for the performance of the squid cache.'
    },

    'web_log.squid_timings': {
        title: 'timings',
        info: 'Duration of squid requests. Unrealistic spikes may be reported, since squid logs the total time of the requests, when they complete. Especially for HTTPS, the clients get a tunnel from the proxy and exchange requests directly with the upstream servers, so squid cannot evaluate the individual requests and reports the total time the tunnel was open.'
    },

    'web_log.squid_clients': {
        title: 'clients'
    },

    'web_log.bandwidth': {
        info: 'Bandwidth of requests (<code>received</code>) and responses (<code>sent</code>). <code>received</code> requires an extended log format (without it, the web server log does not have this information). This chart may present unusual spikes, since the bandwidth is accounted at the time the log line is saved by the web server, even if the time needed to serve it spans across a longer duration. We suggest to use QoS (e.g. <a href="http://firehol.org/#fireqos" target="_blank">FireQOS</a>) for accurate accounting of the web server bandwidth.'
    },

    'web_log.urls': {
        info: 'Number of requests for each <code>URL pattern</code> defined in <a href="https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/web_log/web_log.conf" target="_blank"><code>/etc/netdata/python.d/web_log.conf</code></a>. This chart counts all requests matching the URL patterns defined, independently of the web server response codes (i.e. both successful and unsuccessful).'
    },

    'web_log.clients': {
        info: 'Charts showing the number of unique client IPs, accessing the web server.'
    },

    'web_log.timings': {
        info: 'Web server response timings - the time the web server needed to prepare and respond to requests. This requires an extended log format and its meaning is web server specific. For most web servers this accounts the time from the reception of a complete request, to the dispatch of the last byte of the response. So, it includes the network delays of responses, but it does not include the network delays of requests.'
    },

    'mem.ksm': {
        title: 'deduper (ksm)',
        info: 'Kernel Same-page Merging (KSM) performance monitoring, read from several files in <code>/sys/kernel/mm/ksm/</code>. KSM is a memory-saving de-duplication feature in the Linux kernel (since version 2.6.32). The KSM daemon ksmd periodically scans those areas of user memory which have been registered with it, looking for pages of identical content which can be replaced by a single write-protected page (which is automatically copied if a process later wants to update its content). KSM was originally developed for use with KVM (where it was known as Kernel Shared Memory), to fit more virtual machines into physical memory, by sharing the data common between them.  But it can be useful to any application which generates many instances of the same data.'
    },

    'mem.hugepages': {
        info: 'Hugepages is a feature that allows the kernel to utilize the multiple page size capabilities of modern hardware architectures. The kernel creates multiple pages of virtual memory, mapped from both physical RAM and swap. There is a mechanism in the CPU architecture called "Translation Lookaside Buffers" (TLB) to manage the mapping of virtual memory pages to actual physical memory addresses. The TLB is a limited hardware resource, so utilizing a large amount of physical memory with the default page size consumes the TLB and adds processing overhead. By utilizing Huge Pages, the kernel is able to create pages of much larger sizes, each page consuming a single resource in the TLB. Huge Pages are pinned to physical RAM and cannot be swapped/paged out.'
    },

    'mem.numa': {
        info: 'Non-Uniform Memory Access (NUMA) is a hierarchical memory design the memory access time is dependent on locality. Under NUMA, a processor can access its own local memory faster than non-local memory (memory local to another processor or memory shared between processors). The individual metrics are described in the <a href="https://www.kernel.org/doc/Documentation/numastat.txt" target="_blank">Linux kernel documentation</a>.'
    },

    'ip.ecn': {
        info: '<a href="https://en.wikipedia.org/wiki/Explicit_Congestion_Notification" target="_blank">Explicit Congestion Notification (ECN)</a> is a TCP extension that allows end-to-end notification of network congestion without dropping packets. ECN is an optional feature that may be used between two ECN-enabled endpoints when the underlying network infrastructure also supports it.'
    },

    'netfilter.conntrack': {
        title: 'connection tracker',
        info: 'Netfilter Connection Tracker performance metrics. The connection tracker keeps track of all connections of the machine, inbound and outbound. It works by keeping a database with all open connections, tracking network and address translation and connection expectations.'
    },

    'netfilter.nfacct': {
        title: 'bandwidth accounting',
        info: 'The following information is read using the <code>nfacct.plugin</code>.'
    },

    'netfilter.synproxy': {
        title: 'DDoS protection',
        info: 'DDoS protection performance metrics. <a href="https://github.com/firehol/firehol/wiki/Working-with-SYNPROXY" target="_blank">SYNPROXY</a> is a TCP SYN packets proxy. It is used to protect any TCP server (like a web server) from SYN floods and similar DDoS attacks. It is a netfilter module, in the Linux kernel (since version 3.12). It is optimized to handle millions of packets per second utilizing all CPUs available without any concurrency locking between the connections. It can be used for any kind of TCP traffic (even encrypted), since it does not interfere with the content itself.'
    },

    'ipfw.dynamic_rules': {
        title: 'dynamic rules',
        info: 'Number of dynamic rules, created by correspondent stateful firewall rules.'
    },

    'system.softnet_stat': {
        title: 'softnet',
        info: function (os) {
            if (os === 'linux')
                return 'Statistics for CPUs SoftIRQs related to network receive work. Break down per CPU core can be found at <a href="#menu_cpu_submenu_softnet_stat">CPU / softnet statistics</a>. <b>processed</b> states the number of packets processed, <b>dropped</b> is the number packets dropped because the network device backlog was full (to fix them on Linux use <code>sysctl</code> to increase <code>net.core.netdev_max_backlog</code>), <b>squeezed</b> is the number of packets dropped because the network device budget ran out (to fix them on Linux use <code>sysctl</code> to increase <code>net.core.netdev_budget</code> and/or <code>net.core.netdev_budget_usecs</code>). More information about identifying and troubleshooting network driver related issues can be found at <a href="https://access.redhat.com/sites/default/files/attachments/20150325_network_performance_tuning.pdf" target="_blank">Red Hat Enterprise Linux Network Performance Tuning Guide</a>.';
            else
                return 'Statistics for CPUs SoftIRQs related to network receive work.';
        }
    },

    'cpu.softnet_stat': {
        title: 'softnet',
        info: function (os) {
            if (os === 'linux')
                return 'Statistics for per CPUs core SoftIRQs related to network receive work. Total for all CPU cores can be found at <a href="#menu_system_submenu_softnet_stat">System / softnet statistics</a>. <b>processed</b> states the number of packets processed, <b>dropped</b> is the number packets dropped because the network device backlog was full (to fix them on Linux use <code>sysctl</code> to increase <code>net.core.netdev_max_backlog</code>), <b>squeezed</b> is the number of packets dropped because the network device budget ran out (to fix them on Linux use <code>sysctl</code> to increase <code>net.core.netdev_budget</code> and/or <code>net.core.netdev_budget_usecs</code>). More information about identifying and troubleshooting network driver related issues can be found at <a href="https://access.redhat.com/sites/default/files/attachments/20150325_network_performance_tuning.pdf" target="_blank">Red Hat Enterprise Linux Network Performance Tuning Guide</a>.';
            else
                return 'Statistics for per CPUs core SoftIRQs related to network receive work. Total for all CPU cores can be found at <a href="#menu_system_submenu_softnet_stat">System / softnet statistics</a>.';
        }
    },

    'go_expvar.memstats': {
        title: 'memory statistics',
        info: 'Go runtime memory statistics. See <a href="https://golang.org/pkg/runtime/#MemStats" target="_blank">runtime.MemStats</a> documentation for more info about each chart and the values.'
    },

    'couchdb.dbactivity': {
        title: 'db activity',
        info: 'Overall database reads and writes for the entire server. This includes any external HTTP traffic, as well as internal replication traffic performed in a cluster to ensure node consistency.'
    },

    'couchdb.httptraffic': {
        title: 'http traffic breakdown',
        info: 'All HTTP traffic, broken down by type of request (<tt>GET</tt>, <tt>PUT</tt>, <tt>POST</tt>, etc.) and response status code (<tt>200</tt>, <tt>201</tt>, <tt>4xx</tt>, etc.)<br/><br/>Any <tt>5xx</tt> errors here indicate a likely CouchDB bug; check the logfile for further information.'
    },

    'couchdb.ops': {
        title: 'server operations'
    },

    'couchdb.perdbstats': {
        title: 'per db statistics',
        info: 'Statistics per database. This includes <a href="http://docs.couchdb.org/en/latest/api/database/common.html#get--db">3 size graphs per database</a>: active (the size of live data in the database), external (the uncompressed size of the database contents), and file (the size of the file on disk, exclusive of any views and indexes). It also includes the number of documents and number of deleted documents per database.'
    },

    'couchdb.erlang': {
        title: 'erlang statistics',
        info: 'Detailed information about the status of the Erlang VM that hosts CouchDB. These are intended for advanced users only. High values of the peak message queue (>10e6) generally indicate an overload condition.'
    },

    'ntpd.system': {
        title: 'system',
        info: 'Statistics of the system variables as shown by the readlist billboard <code>ntpq -c rl</code>. System variables are assigned an association ID of zero and can also be shown in the readvar billboard <code>ntpq -c "rv 0"</code>. These variables are used in the <a href="http://doc.ntp.org/current-stable/discipline.html">Clock Discipline Algorithm</a>, to calculate the lowest and most stable offset.'
    },

    'ntpd.peers': {
        title: 'peers',
        info: 'Statistics of the peer variables for each peer configured in <code>/etc/ntp.conf</code> as shown by the readvar billboard <code>ntpq -c "rv &lt;association&gt;"</code>, while each peer is assigned a nonzero association ID as shown by <code>ntpq -c "apeers"</code>. The module periodically scans for new/changed peers (default: every 60s). <b>ntpd</b> selects the best possible peer from the available peers to synchronize the clock. A minimum of at least 3 peers is required to properly identify the best possible peer.'
    }
};


// ----------------------------------------------------------------------------
// chart

// information works on the context of a chart
// Its purpose is to set:
//
// info: the text above the charts
// heads: the representation of the chart at the top the subsection (second level menu)
// mainheads: the representation of the chart at the top of the section (first level menu)
// colors: the dimension colors of the chart (the default colors are appended)
// height: the ratio of the chart height relative to the default
//

var cgroupCPULimitIsSet = 0;
var cgroupMemLimitIsSet = 0;

netdataDashboard.context = {
    'system.cpu': {
        info: function (os) {
            void (os);
            return 'Total CPU utilization (all cores). 100% here means there is no CPU idle time at all. You can get per core usage at the <a href="#menu_cpu">CPUs</a> section and per application usage at the <a href="#menu_apps">Applications Monitoring</a> section.'
                + netdataDashboard.sparkline('<br/>Keep an eye on <b>iowait</b> ', 'system.cpu', 'iowait', '%', '. If it is constantly high, your disks are a bottleneck and they slow your system down.')
                + netdataDashboard.sparkline('<br/>An important metric worth monitoring, is <b>softirq</b> ', 'system.cpu', 'softirq', '%', '. A constantly high percentage of softirq may indicate network driver issues.');
        },
        valueRange: "[0, 100]"
    },

    'system.load': {
        info: 'Current system load, i.e. the number of processes using CPU or waiting for system resources (usually CPU and disk). The 3 metrics refer to 1, 5 and 15 minute averages. The system calculates this once every 5 seconds. For more information check <a href="https://en.wikipedia.org/wiki/Load_(computing)" target="_blank">this wikipedia article</a>.',
        height: 0.7
    },

    'system.cpu_pressure': {
        info: '<a href="https://www.kernel.org/doc/html/latest/accounting/psi.html">Pressure Stall Information</a> ' +
            'identifies and quantifies the disruptions caused by resource contentions. ' +
            'The "some" line indicates the share of time in which at least <b>some</b> tasks are stalled on CPU. ' +
            'The ratios (in %) are tracked as recent trends over 10-, 60-, and 300-second windows.'
    },

    'system.memory_some_pressure': {
        info: '<a href="https://www.kernel.org/doc/html/latest/accounting/psi.html">Pressure Stall Information</a> ' +
            'identifies and quantifies the disruptions caused by resource contentions. ' +
            'The "some" line indicates the share of time in which at least <b>some</b> tasks are stalled on memory. ' +
            'The "full" line indicates the share of time in which <b>all non-idle</b> tasks are stalled on memory simultaneously. ' +
            'In this state actual CPU cycles are going to waste, and a workload that spends extended time in this state is considered to be thrashing. ' +
            'The ratios (in %) are tracked as recent trends over 10-, 60-, and 300-second windows.'
    },

    'system.io_some_pressure': {
        info: '<a href="https://www.kernel.org/doc/html/latest/accounting/psi.html">Pressure Stall Information</a> ' +
            'identifies and quantifies the disruptions caused by resource contentions. ' +
            'The "some" line indicates the share of time in which at least <b>some</b> tasks are stalled on I/O. ' +
            'The "full" line indicates the share of time in which <b>all non-idle</b> tasks are stalled on I/O simultaneously. ' +
            'In this state actual CPU cycles are going to waste, and a workload that spends extended time in this state is considered to be thrashing. ' +
            'The ratios (in %) are tracked as recent trends over 10-, 60-, and 300-second windows.'
    },

    'system.io': {
        info: function (os) {
            var s = 'Total Disk I/O, for all physical disks. You can get detailed information about each disk at the <a href="#menu_disk">Disks</a> section and per application Disk usage at the <a href="#menu_apps">Applications Monitoring</a> section.';

            if (os === 'linux')
                return s + ' Physical are all the disks that are listed in <code>/sys/block</code>, but do not exist in <code>/sys/devices/virtual/block</code>.';
            else
                return s;
        }
    },

    'system.pgpgio': {
        info: 'Memory paged from/to disk. This is usually the total disk I/O of the system.'
    },

    'system.swapio': {
        info: 'Total Swap I/O. (netdata measures both <code>in</code> and <code>out</code>. If either of the metrics <code>in</code> or <code>out</code> is not shown in the chart, the reason is that the metric is zero. - you can change the page settings to always render all the available dimensions on all charts).'
    },

    'system.pgfaults': {
        info: 'Total page faults. <b>Major page faults</b> indicates that the system is using its swap. You can find which applications use the swap at the <a href="#menu_apps">Applications Monitoring</a> section.'
    },

    'system.entropy': {
        colors: '#CC22AA',
        info: '<a href="https://en.wikipedia.org/wiki/Entropy_(computing)" target="_blank">Entropy</a>, is a pool of random numbers (<a href="https://en.wikipedia.org/wiki//dev/random" target="_blank">/dev/random</a>) that is mainly used in cryptography. If the pool of entropy gets empty, processes requiring random numbers may run a lot slower (it depends on the interface each program uses), waiting for the pool to be replenished. Ideally a system with high entropy demands should have a hardware device for that purpose (TPM is one such device). There are also several software-only options you may install, like <code>haveged</code>, although these are generally useful only in servers.'
    },

    'system.clock_sync_state': {
        info: 'State map: 0 - not synchronized, 1 - synchronized'
    },

    'system.forks': {
        colors: '#5555DD',
        info: 'Number of new processes created.'
    },

    'system.intr': {
        colors: '#DD5555',
        info: 'Total number of CPU interrupts. Check <code>system.interrupts</code> that gives more detail about each interrupt and also the <a href="#menu_cpu">CPUs</a> section where interrupts are analyzed per CPU core.'
    },

    'system.interrupts': {
        info: 'CPU interrupts in detail. At the <a href="#menu_cpu">CPUs</a> section, interrupts are analyzed per CPU core.'
    },

    'system.softirqs': {
        info: 'CPU softirqs in detail. At the <a href="#menu_cpu">CPUs</a> section, softirqs are analyzed per CPU core.'
    },

    'system.processes': {
        info: 'System processes. <b>Running</b> are the processes in the CPU. <b>Blocked</b> are processes that are willing to enter the CPU, but they cannot, e.g. because they wait for disk activity.'
    },

    'system.active_processes': {
        info: 'All system processes.'
    },

    'system.ctxt': {
        info: '<a href="https://en.wikipedia.org/wiki/Context_switch" target="_blank">Context Switches</a>, is the switching of the CPU from one process, task or thread to another. If there are many processes or threads willing to execute and very few CPU cores available to handle them, the system is making more context switching to balance the CPU resources among them. The whole process is computationally intensive. The more the context switches, the slower the system gets.'
    },

    'system.idlejitter': {
        info: 'Idle jitter is calculated by netdata. A thread is spawned that requests to sleep for a few microseconds. When the system wakes it up, it measures how many microseconds have passed. The difference between the requested and the actual duration of the sleep, is the <b>idle jitter</b>. This number is useful in real-time environments, where CPU jitter can affect the quality of the service (like VoIP media gateways).'
    },

    'system.net': {
        info: function (os) {
            var s = 'Total bandwidth of all physical network interfaces. This does not include <code>lo</code>, VPNs, network bridges, IFB devices, bond interfaces, etc. Only the bandwidth of physical network interfaces is aggregated.';

            if (os === 'linux')
                return s + ' Physical are all the network interfaces that are listed in <code>/proc/net/dev</code>, but do not exist in <code>/sys/devices/virtual/net</code>.';
            else
                return s;
        }
    },

    'system.ip': {
        info: 'Total IP traffic in the system.'
    },

    'system.ipv4': {
        info: 'Total IPv4 Traffic.'
    },

    'system.ipv6': {
        info: 'Total IPv6 Traffic.'
    },

    'system.ram': {
        info: 'System Random Access Memory (i.e. physical memory) usage.'
    },

    'system.swap': {
        info: 'System swap memory usage. Swap space is used when the amount of physical memory (RAM) is full. When the system needs more memory resources and the RAM is full, inactive pages in memory are moved to the swap space (usually a disk, a disk partition or a file).'
    },

    // ------------------------------------------------------------------------
    // CPU charts

    'cpu.cpu': {
        commonMin: true,
        commonMax: true,
        valueRange: "[0, 100]"
    },

    'cpu.interrupts': {
        commonMin: true,
        commonMax: true
    },

    'cpu.softirqs': {
        commonMin: true,
        commonMax: true
    },

    'cpu.softnet_stat': {
        commonMin: true,
        commonMax: true
    },

    // ------------------------------------------------------------------------
    // MEMORY

    'mem.ksm_savings': {
        heads: [
            netdataDashboard.gaugeChart('Saved', '12%', 'savings', '#0099CC')
        ]
    },

    'mem.ksm_ratios': {
        heads: [
            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-gauge-max-value="100"'
                    + ' data-chart-library="gauge"'
                    + ' data-title="Savings"'
                    + ' data-units="percentage %"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="12%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' role="application"></div>';
            }
        ]
    },

    'mem.zram_usage': {
        info: 'ZRAM total RAM usage metrics. ZRAM uses some memory to store metadata about stored memory pages, thus introducing an overhead which is proportional to disk size. It excludes same-element-filled-pages since no memory is allocated for them.'
    },

    'mem.zram_savings': {
        info: 'Displays original and compressed memory data sizes.'
    },

    'mem.zram_ratio': {
        heads: [
            netdataDashboard.gaugeChart('Compression Ratio', '12%', 'ratio', '#0099CC')
        ],
        info: 'Compression ratio, calculated as <code>100 * original_size / compressed_size</code>. More means better compression and more RAM savings.'
    },

    'mem.zram_efficiency': {
        heads: [
            netdataDashboard.gaugeChart('Efficiency', '12%', 'percent', NETDATA.colors[0])
        ],
        commonMin: true,
        commonMax: true,
        valueRange: "[0, 100]",
        info: 'Memory usage efficiency, calculated as <code>100 * compressed_size / total_mem_used</code>.'
    },


    'mem.pgfaults': {
        info: 'A <a href="https://en.wikipedia.org/wiki/Page_fault" target="_blank">page fault</a> is a type of interrupt, called trap, raised by computer hardware when a running program accesses a memory page that is mapped into the virtual address space, but not actually loaded into main memory. If the page is loaded in memory at the time the fault is generated, but is not marked in the memory management unit as being loaded in memory, then it is called a <b>minor</b> or soft page fault. A <b>major</b> page fault is generated when the system needs to load the memory page from disk or swap memory.'
    },

    'mem.committed': {
        colors: NETDATA.colors[3],
        info: 'Committed Memory, is the sum of all memory which has been allocated by processes.'
    },

    'mem.available': {
        info: 'Available Memory is estimated by the kernel, as the amount of RAM that can be used by userspace processes, without causing swapping.'
    },

    'mem.writeback': {
        info: '<b>Dirty</b> is the amount of memory waiting to be written to disk. <b>Writeback</b> is how much memory is actively being written to disk.'
    },

    'mem.kernel': {
        info: 'The total amount of memory being used by the kernel. <b>Slab</b> is the amount of memory used by the kernel to cache data structures for its own use. <b>KernelStack</b> is the amount of memory allocated for each task done by the kernel. <b>PageTables</b> is the amount of memory dedicated to the lowest level of page tables (A page table is used to turn a virtual address into a physical memory address). <b>VmallocUsed</b> is the amount of memory being used as virtual address space.'
    },

    'mem.slab': {
        info: '<b>Reclaimable</b> is the amount of memory which the kernel can reuse. <b>Unreclaimable</b> can not be reused even when the kernel is lacking memory.'
    },

    'mem.hugepages': {
        info: 'Dedicated (or Direct) HugePages is memory reserved for applications configured to utilize huge pages. Hugepages are <b>used</b> memory, even if there are free hugepages available.'
    },

    'mem.transparent_hugepages': {
        info: 'Transparent HugePages (THP) is backing virtual memory with huge pages, supporting automatic promotion and demotion of page sizes. It works for all applications for anonymous memory mappings and tmpfs/shmem.'
    },

    'mem.cachestat_ratio': {
        info: 'When the processor needs to read or write a location in main memory, it checks for a corresponding entry in the page cache. If the entry is there, a page cache hit has occurred and the read is from the cache. If the entry is not there, a page cache miss has occurred and the kernel allocates a new entry and copies in data from the disk. Netdata calculates the percentage of accessed files that are cached on memory. <a href="https://github.com/iovisor/bcc/blob/master/tools/cachestat.py#L126-L138" target="_blank">The ratio</a> is calculated counting the accessed cached pages (without counting dirty pages and pages added because of read misses) divided by total access without dirty pages.'
    },

    'mem.cachestat_dirties': {
        info: 'Number of <a href="https://en.wikipedia.org/wiki/Page_cache#Memory_conservation" target="_blank">dirty(modified) pages</a> cache. Pages in the page cache modified after being brought in are called dirty pages. Since non-dirty pages in the page cache have identical copies in <a href="https://en.wikipedia.org/wiki/Secondary_storage" target="_blank">secondary storage</a> (e.g. hard disk drive or solid-state drive), discarding and reusing their space is much quicker than paging out application memory, and is often preferred over flushing the dirty pages into secondary storage and reusing their space.'
    },

    'mem.cachestat_hits': {
        info: 'When the processor needs to read or write a location in main memory, it checks for a corresponding entry in the page cache. If the entry is there, a page cache hit has occurred and the read is from the cache. Hits show pages accessed that were not modified (we are excluding dirty pages), this counting also excludes the recent pages inserted for read.'
    },

    'mem.cachestat_misses': {
        info: 'When the processor needs to read or write a location in main memory, it checks for a corresponding entry in the page cache. If the entry is not there, a page cache miss has occurred and the cache allocates a new entry and copies in data for the main memory. Misses count page insertions to the memory not related to writing.'
    },

    'mem.sync': {
        info: 'System calls for <a href="https://man7.org/linux/man-pages/man2/sync.2.html" target="_blank">sync() and syncfs()</a> which flush the file system buffers to storage devices. Performance perturbations might be caused by these calls. The <code>sync()</code> calls are based on the eBPF <a href="https://github.com/iovisor/bcc/blob/master/tools/syncsnoop.py" target="_blank">syncsnoop</a> from BCC tools.'
    },

    'mem.file_sync': {
        info: 'System calls for <a href="https://man7.org/linux/man-pages/man2/fsync.2.html" target="_blank">fsync() and fdatasync()</a> transfer all modified page caches for the files on disk devices. These calls block until the device reports that the transfer has been completed.'
    },

    'mem.memory_map': {
        info: 'System calls for <a href="https://man7.org/linux/man-pages/man2/msync.2.html" target="_blank">msync()</a> which flushes changes made to the in-core copy of a file that was mapped.'
    },

    'mem.file_segment': {
        info: 'System calls for <a href="https://man7.org/linux/man-pages/man2/sync_file_range.2.html" target="_blank">sync_file_range()</a> permits fine control when synchronizing the open file referred to by the file descriptor fd with disk. This system call is extremely dangerous and should not be used in portable programs.'
    },

    'filesystem.dc_hit_ratio': {
        info: 'Percentage of file accesses that were present in the directory cache. 100% means that every file that was accessed was present in the directory cache. If files are not present in the directory cache 1) they are not present in the file system, 2) the files were not accessed before. Read more about <a href="https://www.kernel.org/doc/htmldocs/filesystems/the_directory_cache.html" target="_blank">directory cache</a>.'
    },

    'filesystem.dc_reference': {
        info: 'Counters of file accesses. <code>Reference</code> is when there is a file access and the file is not present in the directory cache. <code>Miss</code> is when there is file access and the file is not found in the filesystem. <code>Slow</code> is when there is a file access and the file is present in the filesystem but not in the directory cache. Read more about <a href="https://www.kernel.org/doc/htmldocs/filesystems/the_directory_cache.html" target="_blank">directory cache</a>.'
    },

    // ------------------------------------------------------------------------
    // network interfaces

    'net.drops': {
        info: 'Packets that have been dropped at the network interface level. These are the same counters reported by <code>ifconfig</code> as <code>RX dropped</code> (inbound) and <code>TX dropped</code> (outbound). <b>inbound</b> packets can be dropped at the network interface level due to <a href="#menu_system_submenu_softnet_stat">softnet backlog</a> overflow, bad / unintended VLAN tags, unknown or unregistered protocols, IPv6 frames when the server is not configured for IPv6. Check <a href="https://www.novell.com/support/kb/doc.php?id=7007165" target="_blank">this document</a> for more information.'
    },

    'net.duplex': {
        info: 'State map: 0 - unknown, 1 - half duplex, 2 - full duplex'
    },

    'net.operstate': {
        info: 'State map: 0 - unknown, 1 - notpresent, 2 - down, 3 - lowerlayerdown, 4 - testing, 5 - dormant, 6 - up'
    },

    'net.carrier': {
        info: 'State map: 0 - down, 1 - up'
    },

    // ------------------------------------------------------------------------
    // IP

    'ip.inerrors': {
        info: 'Errors encountered during the reception of IP packets. ' +
            '<code>noroutes</code> (<code>InNoRoutes</code>) counts packets that were dropped because there was no route to send them. ' +
            '<code>truncated</code> (<code>InTruncatedPkts</code>) counts packets which is being discarded because the datagram frame didn\'t carry enough data. ' +
            '<code>checksum</code> (<code>InCsumErrors</code>) counts packets that were dropped because they had wrong checksum. '
    },

    'ip.tcpmemorypressures': {
        info: 'Number of times a socket was put in <b>memory pressure</b> due to a non fatal memory allocation failure (the kernel attempts to work around this situation by reducing the send buffers, etc).'
    },

    'ip.tcpconnaborts': {
        info: 'TCP connection aborts. <b>baddata</b> (<code>TCPAbortOnData</code>) happens while the connection is on <code>FIN_WAIT1</code> and the kernel receives a packet with a sequence number beyond the last one for this connection - the kernel responds with <code>RST</code> (closes the connection). <b>userclosed</b> (<code>TCPAbortOnClose</code>) happens when the kernel receives data on an already closed connection and responds with <code>RST</code>. <b>nomemory</b> (<code>TCPAbortOnMemory</code> happens when there are too many orphaned sockets (not attached to an fd) and the kernel has to drop a connection - sometimes it will send an <code>RST</code>, sometimes it won\'t. <b>timeout</b> (<code>TCPAbortOnTimeout</code>) happens when a connection times out. <b>linger</b> (<code>TCPAbortOnLinger</code>) happens when the kernel killed a socket that was already closed by the application and lingered around for long enough. <b>failed</b> (<code>TCPAbortFailed</code>) happens when the kernel attempted to send an <code>RST</code> but failed because there was no memory available.'
    },

    'ip.tcp_syn_queue': {
        info: 'The <b>SYN queue</b> of the kernel tracks TCP handshakes until connections get fully established. ' +
            'It overflows when too many incoming TCP connection requests hang in the half-open state and the server ' +
            'is not configured to fall back to SYN cookies*. Overflows are usually caused by SYN flood DoS attacks ' +
            '(i.e. someone sends lots of SYN packets and never completes the handshakes). ' +
            '<b>drops</b> (or <code>TcpExtTCPReqQFullDrop</code>) is the number of connections dropped because the ' +
            'SYN queue was full and SYN cookies were disabled. ' +
            '<b>cookies</b> (or <code>TcpExtTCPReqQFullDoCookies</code>) is the number of SYN cookies sent because the ' +
            'SYN queue was full.'
    },

    'ip.tcp_accept_queue': {
        info: 'The <b>accept queue</b> of the kernel holds the fully established TCP connections, waiting to be handled ' +
            'by the listening application. <b>overflows</b> (or <code>ListenOverflows</code>) is the number of ' +
            'established connections that could not be handled because the receive queue of the listening application ' +
            'was full. <b>drops</b> (or <code>ListenDrops</code>) is the number of incoming ' +
            'connections that could not be handled, including SYN floods, overflows, out of memory, security issues, ' +
            'no route to destination, reception of related ICMP messages, socket is broadcast or multicast.'
    },


    // ------------------------------------------------------------------------
    // IPv4

    'ipv4.tcpsock': {
        info: 'The number of established TCP connections (known as <code>CurrEstab</code>). This is a snapshot of the established connections at the time of measurement (i.e. a connection established and a connection disconnected within the same iteration will not affect this metric).'
    },

    'ipv4.tcpopens': {
        info: '<b>active</b> or <code>ActiveOpens</code> is the number of outgoing TCP <b>connections attempted</b> by this host.'
            + ' <b>passive</b> or <code>PassiveOpens</code> is the number of incoming TCP <b>connections accepted</b> by this host.'
    },

    'ipv4.tcperrors': {
        info: '<code>InErrs</code> is the number of TCP segments received in error (including header too small, checksum errors, sequence errors, bad packets - for both IPv4 and IPv6).'
            + ' <code>InCsumErrors</code> is the number of TCP segments received with checksum errors (for both IPv4 and IPv6).'
            + ' <code>RetransSegs</code> is the number of TCP segments retransmitted.'
    },

    'ipv4.tcphandshake': {
        info: '<code>EstabResets</code> is the number of established connections resets (i.e. connections that made a direct transition from <code>ESTABLISHED</code> or <code>CLOSE_WAIT</code> to <code>CLOSED</code>).'
            + ' <code>OutRsts</code> is the number of TCP segments sent, with the <code>RST</code> flag set (for both IPv4 and IPv6).'
            + ' <code>AttemptFails</code> is the number of times TCP connections made a direct transition from either <code>SYN_SENT</code> or <code>SYN_RECV</code> to <code>CLOSED</code>, plus the number of times TCP connections made a direct transition from the <code>SYN_RECV</code> to <code>LISTEN</code>.'
            + ' <code>TCPSynRetrans</code> shows retries for new outbound TCP connections, which can indicate general connectivity issues or backlog on the remote host.'
    },

    // ------------------------------------------------------------------------
    // APPS

    'apps.cpu': {
        height: 2.0
    },

    'apps.mem': {
        info: 'Real memory (RAM) used by applications. This does not include shared memory.'
    },

    'apps.vmem': {
        info: 'Virtual memory allocated by applications. Please check <a href="https://github.com/netdata/netdata/tree/master/daemon#virtual-memory" target="_blank">this article</a> for more information.'
    },

    'apps.preads': {
        height: 2.0
    },

    'apps.pwrites': {
        height: 2.0
    },

    'apps.uptime': {
        info: 'Carried over process group uptime since the Netdata restart. The period of time within which at least one process in the group was running.'
    },

    'apps.file_open': {
        info: 'Calls to the internal function <code>do_sys_open</code> ( For kernels newer than <code>5.5.19</code> we add a kprobe to <code>do_sys_openat2</code>. ), which is the common function called from' +
            ' <a href="https://www.man7.org/linux/man-pages/man2/open.2.html" target="_blank">open(2)</a> ' +
            ' and <a href="https://www.man7.org/linux/man-pages/man2/openat.2.html" target="_blank">openat(2)</a>. '
    },

    'apps.file_open_error': {
        info: 'Failed calls to the internal function <code>do_sys_open</code> ( For kernels newer than <code>5.5.19</code> we add a kprobe to <code>do_sys_openat2</code>. ).'
    },

    'apps.file_closed': {
        info: 'Calls to the internal function <a href="https://elixir.bootlin.com/linux/v5.10/source/fs/file.c#L665" target="_blank">__close_fd</a> or <a href="https://elixir.bootlin.com/linux/v5.11/source/fs/file.c#L617" target="_blank">close_fd</a> according to your kernel version, which is called from' +
            ' <a href="https://www.man7.org/linux/man-pages/man2/close.2.html" target="_blank">close(2)</a>. '
    },

    'apps.file_close_error': {
        info: 'Failed calls to the internal function <a href="https://elixir.bootlin.com/linux/v5.10/source/fs/file.c#L665" target="_blank">__close_fd</a> or <a href="https://elixir.bootlin.com/linux/v5.11/source/fs/file.c#L617" target="_blank">close_fd</a> according to your kernel version.'
    },

    'apps.file_deleted': {
        info: 'Calls to the function <a href="https://www.kernel.org/doc/htmldocs/filesystems/API-vfs-unlink.html" target="_blank">vfs_unlink</a>. This chart does not show all events that remove files from the filesystem, because filesystems can create their own functions to remove files.'
    },

    'apps.vfs_write_call': {
        info: 'Successful calls to the function <a href="https://topic.alibabacloud.com/a/kernel-state-file-operation-__-work-information-kernel_8_8_20287135.html" target="_blank">vfs_write</a>. This chart may not show all filesystem events if it uses other functions to store data on disk.'
    },

    'apps.vfs_write_error': {
        info: 'Failed calls to the function <a href="https://topic.alibabacloud.com/a/kernel-state-file-operation-__-work-information-kernel_8_8_20287135.html" target="_blank">vfs_write</a>. This chart may not show all filesystem events if it uses other functions to store data on disk.'
    },

    'apps.vfs_read_call': {
        info: 'Successful calls to the function <a href="https://topic.alibabacloud.com/a/kernel-state-file-operation-__-work-information-kernel_8_8_20287135.html" target="_blank">vfs_read</a>. This chart may not show all filesystem events if it uses other functions to store data on disk.'
    },

    'apps.vfs_read_error': {
        info: 'Failed calls to the function <a href="https://topic.alibabacloud.com/a/kernel-state-file-operation-__-work-information-kernel_8_8_20287135.html" target="_blank">vfs_read</a>. This chart may not show all filesystem events if it uses other functions to store data on disk.'
    },

    'apps.vfs_write_bytes': {
        info: 'Total of bytes successfully written using the function <a href="https://topic.alibabacloud.com/a/kernel-state-file-operation-__-work-information-kernel_8_8_20287135.html" target="_blank">vfs_write</a>.'
    },

    'apps.vfs_read_bytes': {
        info: 'Total of bytes successfully read using the function <a href="https://topic.alibabacloud.com/a/kernel-state-file-operation-__-work-information-kernel_8_8_20287135.html" target="_blank">vfs_read</a>.'
    },

    'apps.process_create': {
        info: 'Calls to either <a href="https://www.ece.uic.edu/~yshi1/linux/lkse/node4.html#SECTION00421000000000000000" target="_blank">do_fork</a>, or <code>kernel_clone</code> if you are running kernel newer than 5.9.16, to create a new task, which is the common name used to define process and tasks inside the kernel. Netdata identifies the process by counting the number of calls to <a href="https://linux.die.net/man/2/clone" target="_blank">sys_clone</a> that do not have the flag <code>CLONE_THREAD</code> set.'
    },

    'apps.thread_create': {
        info: 'Calls to either <a href="https://www.ece.uic.edu/~yshi1/linux/lkse/node4.html#SECTION00421000000000000000" target="_blank">do_fork</a>, or <code>kernel_clone</code> if you are running kernel newer than 5.9.16, to create a new task, which is the common name used to define process and tasks inside the kernel. Netdata identifies the threads by counting the number of calls to <a  href="https://linux.die.net/man/2/clone" target="_blank">sys_clone</a> that have the flag <code>CLONE_THREAD</code> set.'
    },

    'apps.task_close': {
        info: 'Calls to the functions responsible for closing (<a href="https://www.informit.com/articles/article.aspx?p=370047&seqNum=4" target="_blank">do_exit</a>) and releasing (<a  href="https://www.informit.com/articles/article.aspx?p=370047&seqNum=4" target="_blank">release_task</a>) tasks.'
    },

    'apps.bandwidth_sent': {
        info: 'Bytes sent by functions <code>tcp_sendmsg</code> and <code>udp_sendmsg</code>.'
    },

    'apps.bandwidth_recv': {
        info: 'Bytes received by functions <code>tcp_cleanup_rbuf</code> and <code>udp_recvmsg</code>. We use <code>tcp_cleanup_rbuf</code> instead <code>tcp_recvmsg</code>, because this last misses <code>tcp_read_sock()</code> traffic and we would also need to have more probes to get the socket and package size.'
    },

    'apps.bandwidth_tcp_send': {
        info: 'Calls for function <code>tcp_sendmsg</code>.'
    },

    'apps.bandwidth_tcp_recv': {
        info: 'Calls for functions <code>tcp_cleanup_rbuf</code>. We use <code>tcp_cleanup_rbuf</code> instead <code>tcp_recvmsg</code>, because this last misses <code>tcp_read_sock()</code> traffic and we would also need to have more probes to get the socket and package size.'
    },

    'apps.bandwidth_tcp_retransmit': {
        info: 'Calls for functions <code>tcp_retransmit_skb</code>.'
    },

    'apps.bandwidth_udp_send': {
        info: 'Calls for function <code>udp_sendmsg</code>.'
    },

    'apps.bandwidth_udp_recv': {
        info: 'Calls for function <code>udp_recvmsg</code>.'
    },

    'apps.dc_hit_ratio': {
        info: 'Percentage of file accesses that were present in the directory cache. 100% means that every file that was accessed was present in the directory cache. If files are not present in the directory cache 1) they are not present in the file system, 2) the files were not accessed before. Read more about <a href="https://www.kernel.org/doc/htmldocs/filesystems/the_directory_cache.html" target="_blank">directory cache</a>.'
    },

    'apps.dc_reference': {
        info: 'Counters of file accesses. <code>Reference</code> is when there is a file access, see the <code>filesystem.dc_reference</code> chart for more context. Read more about <a href="https://www.kernel.org/doc/htmldocs/filesystems/the_directory_cache.html" target="_blank">directory cache</a>.'
    },

    'apps.dc_not_cache': {
        info: 'Counters of file accesses. <code>Slow</code> is when there is a file access and the file is not present in the directory cache, see the <code>filesystem.dc_reference</code> chart for more context. Read more about <a href="https://www.kernel.org/doc/htmldocs/filesystems/the_directory_cache.html" target="_blank">directory cache</a>.'
    },

    'apps.dc_not_found': {
        info: 'Counters of file accesses. <code>Miss</code> is when there is file access and the file is not found in the filesystem, see the <code>filesystem.dc_reference</code> chart for more context. Read more about <a href="https://www.kernel.org/doc/htmldocs/filesystems/the_directory_cache.html" target="_blank">directory cache</a>.'
    },

    // ------------------------------------------------------------------------
    // USERS

    'users.cpu': {
        height: 2.0
    },

    'users.mem': {
        info: 'Real memory (RAM) used per user. This does not include shared memory.'
    },

    'users.vmem': {
        info: 'Virtual memory allocated per user. Please check <a href="https://github.com/netdata/netdata/tree/master/daemon#virtual-memory" target="_blank">this article</a> for more information.'
    },

    'users.preads': {
        height: 2.0
    },

    'users.pwrites': {
        height: 2.0
    },

    'users.uptime': {
        info: 'Carried over process group uptime since the Netdata restart. The period of time within which at least one process in the group was running.'
    },

    // ------------------------------------------------------------------------
    // GROUPS

    'groups.cpu': {
        height: 2.0
    },

    'groups.mem': {
        info: 'Real memory (RAM) used per user group. This does not include shared memory.'
    },

    'groups.vmem': {
        info: 'Virtual memory allocated per user group since the Netdata restart. Please check <a href="https://github.com/netdata/netdata/tree/master/daemon#virtual-memory" target="_blank">this article</a> for more information.'
    },

    'groups.preads': {
        height: 2.0
    },

    'groups.pwrites': {
        height: 2.0
    },

    'groups.uptime': {
        info: 'Carried over process group uptime. The period of time within which at least one process in the group was running.'
    },

    // ------------------------------------------------------------------------
    // NETWORK QoS

    'tc.qos': {
        heads: [
            function (os, id) {
                void (os);

                if (id.match(/.*-ifb$/))
                    return netdataDashboard.gaugeChart('Inbound', '12%', '', '#5555AA');
                else
                    return netdataDashboard.gaugeChart('Outbound', '12%', '', '#AA9900');
            }
        ]
    },

    // ------------------------------------------------------------------------
    // NETWORK INTERFACES

    'net.net': {
        mainheads: [
            function (os, id) {
                void (os);
                if (id.match(/^cgroup_.*/)) {
                    var iface;
                    try {
                        iface = ' ' + id.substring(id.lastIndexOf('.net_') + 5, id.length);
                    } catch (e) {
                        iface = '';
                    }
                    return netdataDashboard.gaugeChart('Received' + iface, '12%', 'received');
                } else
                    return '';
            },
            function (os, id) {
                void (os);
                if (id.match(/^cgroup_.*/)) {
                    var iface;
                    try {
                        iface = ' ' + id.substring(id.lastIndexOf('.net_') + 5, id.length);
                    } catch (e) {
                        iface = '';
                    }
                    return netdataDashboard.gaugeChart('Sent' + iface, '12%', 'sent');
                } else
                    return '';
            }
        ],
        heads: [
            function (os, id) {
                void (os);
                if (!id.match(/^cgroup_.*/))
                    return netdataDashboard.gaugeChart('Received', '12%', 'received');
                else
                    return '';
            },
            function (os, id) {
                void (os);
                if (!id.match(/^cgroup_.*/))
                    return netdataDashboard.gaugeChart('Sent', '12%', 'sent');
                else
                    return '';
            }
        ]
    },

    // ------------------------------------------------------------------------
    // NETFILTER

    'netfilter.sockets': {
        colors: '#88AA00',
        heads: [
            netdataDashboard.gaugeChart('Active Connections', '12%', '', '#88AA00')
        ]
    },

    'netfilter.new': {
        heads: [
            netdataDashboard.gaugeChart('New Connections', '12%', 'new', '#5555AA')
        ]
    },

    // ------------------------------------------------------------------------
    // DISKS

    'disk.util': {
        colors: '#FF5588',
        heads: [
            netdataDashboard.gaugeChart('Utilization', '12%', '', '#FF5588')
        ],
        info: 'Disk Utilization measures the amount of time the disk was busy with something. This is not related to its performance. 100% means that the system always had an outstanding operation on the disk. Keep in mind that depending on the underlying technology of the disk, 100% here may or may not be an indication of congestion.'
    },

    'disk.busy': {
        colors: '#FF5588',
        info: 'Disk Busy Time measures the amount of time the disk was busy with something.'
    },
    
    'disk.backlog': {
        colors: '#0099CC',
        info: 'Backlog is an indication of the duration of pending disk operations. On every I/O event the system is multiplying the time spent doing I/O since the last update of this field with the number of pending operations. While not accurate, this metric can provide an indication of the expected completion time of the operations in progress.'
    },

    'disk.io': {
        heads: [
            netdataDashboard.gaugeChart('Read', '12%', 'reads'),
            netdataDashboard.gaugeChart('Write', '12%', 'writes')
        ],
        info: 'Amount of data transferred to and from disk.'
    },

    'disk.ops': {
        info: 'Completed disk I/O operations. Keep in mind the number of operations requested might be higher, since the system is able to merge adjacent to each other (see merged operations chart).'
    },

    'disk.qops': {
        info: 'I/O operations currently in progress. This metric is a snapshot - it is not an average over the last interval.'
    },

    'disk.iotime': {
        height: 0.5,
        info: 'The sum of the duration of all completed I/O operations. This number can exceed the interval if the disk is able to execute I/O operations in parallel.'
    },
    'disk_ext.iotime': {
        height: 0.5
    },
    'disk.mops': {
        height: 0.5,
        info: 'The number of merged disk operations. The system is able to merge adjacent I/O operations, for example two 4KB reads can become one 8KB read before given to disk.'
    },
    'disk_ext.mops': {
        height: 0.5
    },
    'disk.svctm': {
        height: 0.5,
        info: 'The average service time for completed I/O operations. This metric is calculated using the total busy time of the disk and the number of completed operations. If the disk is able to execute multiple parallel operations the reporting average service time will be misleading.'
    },
    'disk.latency_io': {
        height: 0.5,
        info: 'Disk I/O latency is the time it takes for an I/O request to be completed. Latency is the single most important metric to focus on when it comes to storage performance, under most circumstances. For hard drives, an average latency somewhere between 10 to 20 ms can be considered acceptable. For SSD (Solid State Drives), depending on the workload it should never reach higher than 1-3 ms. In most cases, workloads will experience less than 1ms latency numbers. The dimensions refer to time intervals. This chart is based on the <a href="https://github.com/cloudflare/ebpf_exporter/blob/master/examples/bio-tracepoints.yaml" target="_blank">bio_tracepoints</a> tool of the ebpf_exporter.'
    },
    'disk.avgsz': {
        height: 0.5,
        info: 'The average I/O operation size.'
    },
    'disk_ext.avgsz': {
        height: 0.5
    },
    'disk.await': {
        height: 0.5,
        info: 'The average time for I/O requests issued to the device to be served. This includes the time spent by the requests in queue and the time spent servicing them.'
    },
    'disk_ext.await': {
        height: 0.5,
        info: 'The average time for extended I/O requests issued to the device to be served. This includes the time spent by the requests in queue and the time spent servicing them.'
    },

    'disk.space': {
        info: 'Disk space utilization. reserved for root is automatically reserved by the system to prevent the root user from getting out of space.'
    },
    'disk.inodes': {
        info: 'inodes (or index nodes) are filesystem objects (e.g. files and directories). On many types of file system implementations, the maximum number of inodes is fixed at filesystem creation, limiting the maximum number of files the filesystem can hold. It is possible for a device to run out of inodes. When this happens, new files cannot be created on the device, even though there may be free space available.'
    },

    // ------------------------------------------------------------------------
    // ZFS pools
    'zfspool.state': {
        info: 'ZFS pool state. The overall health of a pool, as reported by <code>zpool status</code>, is determined by the aggregate state of all devices within the pool. ' +
            'For details, see <a href="https://openzfs.github.io/openzfs-docs/man/8/zpoolconcepts.8.html?#Device_Failure_and_Recovery" target="_blank"> ZFS documentation</a>.'
    },

    // ------------------------------------------------------------------------
    // MYSQL

    'mysql.net': {
        info: 'The amount of data sent to mysql clients (<strong>out</strong>) and received from mysql clients (<strong>in</strong>).'
    },

    'mysql.queries': {
        info: 'The number of statements executed by the server.<ul>' +
            '<li><strong>queries</strong> counts the statements executed within stored SQL programs.</li>' +
            '<li><strong>questions</strong> counts the statements sent to the mysql server by mysql clients.</li>' +
            '<li><strong>slow queries</strong> counts the number of statements that took more than <a href="http://dev.mysql.com/doc/refman/5.7/en/server-system-variables.html#sysvar_long_query_time" target="_blank">long_query_time</a> seconds to be executed.' +
            ' For more information about slow queries check the mysql <a href="http://dev.mysql.com/doc/refman/5.7/en/slow-query-log.html" target="_blank">slow query log</a>.</li>' +
            '</ul>'
    },

    'mysql.handlers': {
        info: 'Usage of the internal handlers of mysql. This chart provides very good insights of what the mysql server is actually doing.' +
            ' (if the chart is not showing all these dimensions it is because they are zero - set <strong>Which dimensions to show?</strong> to <strong>All</strong> from the dashboard settings, to render even the zero values)<ul>' +
            '<li><strong>commit</strong>, the number of internal <a href="http://dev.mysql.com/doc/refman/5.7/en/commit.html" target="_blank">COMMIT</a> statements.</li>' +
            '<li><strong>delete</strong>, the number of times that rows have been deleted from tables.</li>' +
            '<li><strong>prepare</strong>, a counter for the prepare phase of two-phase commit operations.</li>' +
            '<li><strong>read first</strong>, the number of times the first entry in an index was read. A high value suggests that the server is doing a lot of full index scans; e.g. <strong>SELECT col1 FROM foo</strong>, with col1 indexed.</li>' +
            '<li><strong>read key</strong>, the number of requests to read a row based on a key. If this value is high, it is a good indication that your tables are properly indexed for your queries.</li>' +
            '<li><strong>read next</strong>, the number of requests to read the next row in key order. This value is incremented if you are querying an index column with a range constraint or if you are doing an index scan.</li>' +
            '<li><strong>read prev</strong>, the number of requests to read the previous row in key order. This read method is mainly used to optimize <strong>ORDER BY ... DESC</strong>.</li>' +
            '<li><strong>read rnd</strong>, the number of requests to read a row based on a fixed position. A high value indicates you are doing a lot of queries that require sorting of the result. You probably have a lot of queries that require MySQL to scan entire tables or you have joins that do not use keys properly.</li>' +
            '<li><strong>read rnd next</strong>, the number of requests to read the next row in the data file. This value is high if you are doing a lot of table scans. Generally this suggests that your tables are not properly indexed or that your queries are not written to take advantage of the indexes you have.</li>' +
            '<li><strong>rollback</strong>, the number of requests for a storage engine to perform a rollback operation.</li>' +
            '<li><strong>savepoint</strong>, the number of requests for a storage engine to place a savepoint.</li>' +
            '<li><strong>savepoint rollback</strong>, the number of requests for a storage engine to roll back to a savepoint.</li>' +
            '<li><strong>update</strong>, the number of requests to update a row in a table.</li>' +
            '<li><strong>write</strong>, the number of requests to insert a row in a table.</li>' +
            '</ul>'
    },

    'mysql.table_locks': {
        info: 'MySQL table locks counters: <ul>' +
            '<li><strong>immediate</strong>, the number of times that a request for a table lock could be granted immediately.</li>' +
            '<li><strong>waited</strong>, the number of times that a request for a table lock could not be granted immediately and a wait was needed. If this is high and you have performance problems, you should first optimize your queries, and then either split your table or tables or use replication.</li>' +
            '</ul>'
    },

    'mysql.innodb_deadlocks': {
        info: 'A deadlock happens when two or more transactions mutually hold and request for locks, creating a cycle of dependencies. For more information about <a href="https://dev.mysql.com/doc/refman/5.7/en/innodb-deadlocks-handling.html" target="_blank">how to minimize and handle deadlocks</a>.'
    },

    'mysql.galera_cluster_status': {
        info:
            '<code>-1</code>: unknown, ' +
            '<code>0</code>: primary (primary group configuration, quorum present), ' +
            '<code>1</code>: non-primary (non-primary group configuration, quorum lost), ' +
            '<code>2</code>: disconnected(not connected to group, retrying).'
    },

    'mysql.galera_cluster_state': {
        info:
            '<code>0</code>: Undefined, ' +
            '<code>1</code>: Joining, ' +
            '<code>2</code>: Donor/Desynced, ' +
            '<code>3</code>: Joined, ' +
            '<code>4</code>: Synced, ' +
            '<code>5</code>: Inconsistent.'
    },

    'mysql.galera_cluster_weight': {
        info: 'The value is counted as a sum of <code>pc.weight</code> of the nodes in the current Primary Component.'
    },

    'mysql.galera_connected': {
        info: '<code>0</code> means that the node has not yet connected to any of the cluster components. ' +
            'This may be due to misconfiguration.'
    },

    'mysql.open_transactions': {
        info: 'The number of locally running transactions which have been registered inside the wsrep provider. ' +
            'This means transactions which have made operations which have caused write set population to happen. ' +
            'Transactions which are read only are not counted.'
    },


    // ------------------------------------------------------------------------
    // POSTGRESQL


    'postgres.db_stat_blks': {
        info: 'Blocks reads from disk or cache.<ul>' +
            '<li><strong>blks_read:</strong> number of disk blocks read in this database.</li>' +
            '<li><strong>blks_hit:</strong> number of times disk blocks were found already in the buffer cache, so that a read was not necessary (this only includes hits in the PostgreSQL buffer cache, not the operating system&#39;s file system cache)</li>' +
            '</ul>'
    },
    'postgres.db_stat_tuple_write': {
        info: '<ul><li>Number of rows inserted/updated/deleted.</li>' +
            '<li><strong>conflicts:</strong> number of queries canceled due to conflicts with recovery in this database. (Conflicts occur only on standby servers; see <a href="https://www.postgresql.org/docs/10/static/monitoring-stats.html#PG-STAT-DATABASE-CONFLICTS-VIEW" target="_blank">pg_stat_database_conflicts</a> for details.)</li>' +
            '</ul>'
    },
    'postgres.db_stat_temp_bytes': {
        info: 'Temporary files can be created on disk for sorts, hashes, and temporary query results.'
    },
    'postgres.db_stat_temp_files': {
        info: '<ul>' +
            '<li><strong>files:</strong> number of temporary files created by queries. All temporary files are counted, regardless of why the temporary file was created (e.g., sorting or hashing).</li>' +
            '</ul>'
    },
    'postgres.archive_wal': {
        info: 'WAL archiving.<ul>' +
            '<li><strong>total:</strong> total files.</li>' +
            '<li><strong>ready:</strong> WAL waiting to be archived.</li>' +
            '<li><strong>done:</strong> WAL successfully archived. ' +
            'Ready WAL can indicate archive_command is in error, see <a href="https://www.postgresql.org/docs/current/static/continuous-archiving.html" target="_blank">Continuous Archiving and Point-in-Time Recovery</a>.</li>' +
            '</ul>'
    },
    'postgres.checkpointer': {
        info: 'Number of checkpoints.<ul>' +
            '<li><strong>scheduled:</strong> when checkpoint_timeout is reached.</li>' +
            '<li><strong>requested:</strong> when max_wal_size is reached.</li>' +
            '</ul>' +
            'For more information see <a href="https://www.postgresql.org/docs/current/static/wal-configuration.html" target="_blank">WAL Configuration</a>.'
    },
    'postgres.autovacuum': {
        info: 'PostgreSQL databases require periodic maintenance known as vacuuming. For many installations, it is sufficient to let vacuuming be performed by the autovacuum daemon. ' +
            'For more information see <a href="https://www.postgresql.org/docs/current/static/routine-vacuuming.html#AUTOVACUUM" target="_blank">The Autovacuum Daemon</a>.'
    },
    'postgres.standby_delta': {
        info: 'Streaming replication delta.<ul>' +
            '<li><strong>sent_delta:</strong> replication delta sent to standby.</li>' +
            '<li><strong>write_delta:</strong> replication delta written to disk by this standby.</li>' +
            '<li><strong>flush_delta:</strong> replication delta flushed to disk by this standby server.</li>' +
            '<li><strong>replay_delta:</strong> replication delta replayed into the database on this standby server.</li>' +
            '</ul>' +
            'For more information see <a href="https://www.postgresql.org/docs/current/static/warm-standby.html#SYNCHRONOUS-REPLICATION" target="_blank">Synchronous Replication</a>.'
    },
    'postgres.replication_slot': {
        info: 'Replication slot files.<ul>' +
            '<li><strong>wal_keeped:</strong> WAL files retained by each replication slots.</li>' +
            '<li><strong>pg_replslot_files:</strong> files present in pg_replslot.</li>' +
            '</ul>' +
            'For more information see <a href="https://www.postgresql.org/docs/current/static/warm-standby.html#STREAMING-REPLICATION-SLOTS" target="_blank">Replication Slots</a>.'
    },
    'postgres.backend_usage': {
        info: 'Connections usage against maximum connections allowed, as defined in the <i>max_connections</i> setting.<ul>' +
            '<li><strong>available:</strong> maximum new connections allowed.</li>' +
            '<li><strong>used:</strong> connections currently in use.</li>' +
            '</ul>' +
            'Assuming non-superuser accounts are being used to connect to Postgres (so <i>superuser_reserved_connections</i> are subtracted from <i>max_connections</i>).<br/>' +
            'For more information see <a href="https://www.postgresql.org/docs/current/runtime-config-connection.html" target="_blank">Connections and Authentication</a>.'
    },
    'postgres.forced_autovacuum': {
        info: 'Percent towards forced autovacuum for one or more tables.<ul>' +
            '<li><strong>percent_towards_forced_autovacuum:</strong> a forced autovacuum will run once this value reaches 100.</li>' +
            '</ul>' +
            'For more information see <a href="https://www.postgresql.org/docs/current/routine-vacuuming.html" target="_blank">Preventing Transaction ID Wraparound Failures</a>.'
    },
    'postgres.tx_wraparound_oldest_current_xid': {
        info: 'The oldest current transaction id (xid).<ul>' +
            '<li><strong>oldest_current_xid:</strong> oldest current transaction id.</li>' +
            '</ul>' +
            'If for some reason autovacuum fails to clear old XIDs from a table, the system will begin to emit warning messages when the database\'s oldest XIDs reach eleven million transactions from the wraparound point.<br/>' +
            'For more information see <a href="https://www.postgresql.org/docs/current/routine-vacuuming.html" target="_blank">Preventing Transaction ID Wraparound Failures</a>.'
    },
    'postgres.percent_towards_wraparound': {
        info: 'Percent towards transaction wraparound.<ul>' +
            '<li><strong>percent_towards_wraparound:</strong> transaction wraparound may occur when this value reaches 100.</li>' +
            '</ul>' +
            'For more information see <a href="https://www.postgresql.org/docs/current/routine-vacuuming.html" target="_blank">Preventing Transaction ID Wraparound Failures</a>.'
    },


    // ------------------------------------------------------------------------
    // APACHE

    'apache.connections': {
        colors: NETDATA.colors[4],
        mainheads: [
            netdataDashboard.gaugeChart('Connections', '12%', '', NETDATA.colors[4])
        ]
    },

    'apache.requests': {
        colors: NETDATA.colors[0],
        mainheads: [
            netdataDashboard.gaugeChart('Requests', '12%', '', NETDATA.colors[0])
        ]
    },

    'apache.net': {
        colors: NETDATA.colors[3],
        mainheads: [
            netdataDashboard.gaugeChart('Bandwidth', '12%', '', NETDATA.colors[3])
        ]
    },

    'apache.workers': {
        mainheads: [
            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="busy"'
                    + ' data-append-options="percentage"'
                    + ' data-gauge-max-value="100"'
                    + ' data-chart-library="gauge"'
                    + ' data-title="Workers Utilization"'
                    + ' data-units="percentage %"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="12%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' role="application"></div>';
            }
        ]
    },

    'apache.bytesperreq': {
        colors: NETDATA.colors[3],
        height: 0.5
    },

    'apache.reqpersec': {
        colors: NETDATA.colors[4],
        height: 0.5
    },

    'apache.bytespersec': {
        colors: NETDATA.colors[6],
        height: 0.5
    },


    // ------------------------------------------------------------------------
    // LIGHTTPD

    'lighttpd.connections': {
        colors: NETDATA.colors[4],
        mainheads: [
            netdataDashboard.gaugeChart('Connections', '12%', '', NETDATA.colors[4])
        ]
    },

    'lighttpd.requests': {
        colors: NETDATA.colors[0],
        mainheads: [
            netdataDashboard.gaugeChart('Requests', '12%', '', NETDATA.colors[0])
        ]
    },

    'lighttpd.net': {
        colors: NETDATA.colors[3],
        mainheads: [
            netdataDashboard.gaugeChart('Bandwidth', '12%', '', NETDATA.colors[3])
        ]
    },

    'lighttpd.workers': {
        mainheads: [
            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="busy"'
                    + ' data-append-options="percentage"'
                    + ' data-gauge-max-value="100"'
                    + ' data-chart-library="gauge"'
                    + ' data-title="Servers Utilization"'
                    + ' data-units="percentage %"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="12%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' role="application"></div>';
            }
        ]
    },

    'lighttpd.bytesperreq': {
        colors: NETDATA.colors[3],
        height: 0.5
    },

    'lighttpd.reqpersec': {
        colors: NETDATA.colors[4],
        height: 0.5
    },

    'lighttpd.bytespersec': {
        colors: NETDATA.colors[6],
        height: 0.5
    },

    // ------------------------------------------------------------------------
    // NGINX

    'nginx.connections': {
        colors: NETDATA.colors[4],
        mainheads: [
            netdataDashboard.gaugeChart('Connections', '12%', '', NETDATA.colors[4])
        ]
    },

    'nginx.requests': {
        colors: NETDATA.colors[0],
        mainheads: [
            netdataDashboard.gaugeChart('Requests', '12%', '', NETDATA.colors[0])
        ]
    },

    // ------------------------------------------------------------------------
    // HTTP check

    'httpcheck.responsetime': {
        info: 'The <code>response time</code> describes the time passed between request and response. ' +
            'Currently, the accuracy of the response time is low and should be used as reference only.'
    },

    'httpcheck.responselength': {
        info: 'The <code>response length</code> counts the number of characters in the response body. For static pages, this should be mostly constant.'
    },

    'httpcheck.status': {
        valueRange: "[0, 1]",
        info: 'This chart verifies the response of the webserver. Each status dimension will have a value of <code>1</code> if triggered. ' +
            'Dimension <code>success</code> is <code>1</code> only if all constraints are satisfied. ' +
            'This chart is most useful for alarms or third-party apps.'
    },

    // ------------------------------------------------------------------------
    // NETDATA

    'netdata.response_time': {
        info: 'The netdata API response time measures the time netdata needed to serve requests. This time includes everything, from the reception of the first byte of a request, to the dispatch of the last byte of its reply, therefore it includes all network latencies involved (i.e. a client over a slow network will influence these metrics).'
    },

    // ------------------------------------------------------------------------
    // RETROSHARE

    'retroshare.bandwidth': {
        info: 'RetroShare inbound and outbound traffic.',
        mainheads: [
            netdataDashboard.gaugeChart('Received', '12%', 'bandwidth_down_kb'),
            netdataDashboard.gaugeChart('Sent', '12%', 'bandwidth_up_kb')
        ]
    },

    'retroshare.peers': {
        info: 'Number of (connected) RetroShare friends.',
        mainheads: [
            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="peers_connected"'
                    + ' data-append-options="friends"'
                    + ' data-chart-library="easypiechart"'
                    + ' data-title="connected friends"'
                    + ' data-units=""'
                    + ' data-width="8%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' role="application"></div>';
            }
        ]
    },

    'retroshare.dht': {
        info: 'Statistics about RetroShare\'s DHT. These values are estimated!'
    },

    // ------------------------------------------------------------------------
    // fping

    'fping.quality': {
        colors: NETDATA.colors[10],
        height: 0.5
    },

    'fping.packets': {
        height: 0.5
    },


    // ------------------------------------------------------------------------
    // containers

    'cgroup.cpu_limit': {
        valueRange: "[0, null]",
        mainheads: [
            function (os, id) {
                void (os);
                cgroupCPULimitIsSet = 1;
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="used"'
                    + ' data-gauge-max-value="100"'
                    + ' data-chart-library="gauge"'
                    + ' data-title="CPU"'
                    + ' data-units="%"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="12%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-colors="' + NETDATA.colors[4] + '"'
                    + ' role="application"></div>';
            }
        ]
    },

    'cgroup.cpu': {
        mainheads: [
            function (os, id) {
                void (os);
                if (cgroupCPULimitIsSet === 0) {
                    return '<div data-netdata="' + id + '"'
                        + ' data-chart-library="gauge"'
                        + ' data-title="CPU"'
                        + ' data-units="%"'
                        + ' data-gauge-adjust="width"'
                        + ' data-width="12%"'
                        + ' data-before="0"'
                        + ' data-after="-CHART_DURATION"'
                        + ' data-points="CHART_DURATION"'
                        + ' data-colors="' + NETDATA.colors[4] + '"'
                        + ' role="application"></div>';
                } else
                    return '';
            }
        ]
    },

    'cgroup.mem_usage_limit': {
        mainheads: [
            function (os, id) {
                void (os);
                cgroupMemLimitIsSet = 1;
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="used"'
                    + ' data-append-options="percentage"'
                    + ' data-gauge-max-value="100"'
                    + ' data-chart-library="gauge"'
                    + ' data-title="Memory"'
                    + ' data-units="%"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="12%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-colors="' + NETDATA.colors[1] + '"'
                    + ' role="application"></div>';
            }
        ]
    },

    'cgroup.mem_usage': {
        mainheads: [
            function (os, id) {
                void (os);
                if (cgroupMemLimitIsSet === 0) {
                    return '<div data-netdata="' + id + '"'
                        + ' data-chart-library="gauge"'
                        + ' data-title="Memory"'
                        + ' data-units="MB"'
                        + ' data-gauge-adjust="width"'
                        + ' data-width="12%"'
                        + ' data-before="0"'
                        + ' data-after="-CHART_DURATION"'
                        + ' data-points="CHART_DURATION"'
                        + ' data-colors="' + NETDATA.colors[1] + '"'
                        + ' role="application"></div>';
                } else
                    return '';
            }
        ]
    },

    'cgroup.throttle_io': {
        mainheads: [
            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="read"'
                    + ' data-chart-library="gauge"'
                    + ' data-title="Read Disk I/O"'
                    + ' data-units="KB/s"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="12%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-colors="' + NETDATA.colors[2] + '"'
                    + ' role="application"></div>';
            },
            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="write"'
                    + ' data-chart-library="gauge"'
                    + ' data-title="Write Disk I/O"'
                    + ' data-units="KB/s"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="12%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-colors="' + NETDATA.colors[3] + '"'
                    + ' role="application"></div>';
            }
        ]
    },

    // ------------------------------------------------------------------------
    // beanstalkd
    // system charts
    'beanstalk.cpu_usage': {
        info: 'Amount of CPU Time for user and system used by beanstalkd.'
    },

    // This is also a per-tube stat
    'beanstalk.jobs_rate': {
        info: 'The rate of jobs processed by the beanstalkd served.'
    },

    'beanstalk.connections_rate': {
        info: 'The rate of connections opened to beanstalkd.'
    },

    'beanstalk.commands_rate': {
        info: 'The rate of commands received by beanstalkd.'
    },

    'beanstalk.current_tubes': {
        info: 'Total number of current tubes on the server including the default tube (which always exists).'
    },

    'beanstalk.current_jobs': {
        info: 'Current number of jobs in all tubes grouped by status: urgent, ready, reserved, delayed and buried.'
    },

    'beanstalk.current_connections': {
        info: 'Current number of connections group by connection type: written, producers, workers, waiting.'
    },

    'beanstalk.binlog': {
        info: 'The rate of records <code>written</code> to binlog and <code>migrated</code> as part of compaction.'
    },

    'beanstalk.uptime': {
        info: 'Total time beanstalkd server has been up for.'
    },

    // tube charts
    'beanstalk.jobs': {
        info: 'Number of jobs currently in the tube grouped by status: urgent, ready, reserved, delayed and buried.'
    },

    'beanstalk.connections': {
        info: 'The current number of connections to this tube grouped by connection type; using, waiting and watching.'
    },

    'beanstalk.commands': {
        info: 'The rate of <code>delete</code> and <code>pause</code> commands executed by beanstalkd.'
    },

    'beanstalk.pause': {
        info: 'Shows info on how long the tube has been paused for, and how long is left remaining on the pause.'
    },

    // ------------------------------------------------------------------------
    // ceph

    'ceph.general_usage': {
        info: 'The usage and available space in all ceph cluster.'
    },

    'ceph.general_objects': {
        info: 'Total number of objects storage on ceph cluster.'
    },

    'ceph.general_bytes': {
        info: 'Cluster read and write data per second.'
    },

    'ceph.general_operations': {
        info: 'Number of read and write operations per second.'
    },

    'ceph.general_latency': {
        info: 'Total of apply and commit latency in all OSDs. The apply latency is the total time taken to flush an update to disk. The commit latency is the total time taken to commit an operation to the journal.'
    },

    'ceph.pool_usage': {
        info: 'The usage space in each pool.'
    },

    'ceph.pool_objects': {
        info: 'Number of objects presents in each pool.'
    },

    'ceph.pool_read_bytes': {
        info: 'The rate of read data per second in each pool.'
    },

    'ceph.pool_write_bytes': {
        info: 'The rate of write data per second in each pool.'
    },

    'ceph.pool_read_objects': {
        info: 'Number of read objects per second in each pool.'
    },

    'ceph.pool_write_objects': {
        info: 'Number of write objects per second in each pool.'
    },

    'ceph.osd_usage': {
        info: 'The usage space in each OSD.'
    },

    'ceph.osd_size': {
        info: "Each OSD's size"
    },

    'ceph.apply_latency': {
        info: 'Time taken to flush an update in each OSD.'
    },

    'ceph.commit_latency': {
        info: 'Time taken to commit an operation to the journal in each OSD.'
    },

    // ------------------------------------------------------------------------
    // web_log

    'web_log.response_statuses': {
        info: 'Web server responses by type. <code>success</code> includes <b>1xx</b>, <b>2xx</b>, <b>304</b> and <b>401</b>, <code>error</code> includes <b>5xx</b>, <code>redirect</code> includes <b>3xx</b> except <b>304</b>, <code>bad</code> includes <b>4xx</b> except <b>401</b>, <code>other</code> are all the other responses.',
        mainheads: [
            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="success"'
                    + ' data-chart-library="gauge"'
                    + ' data-title="Successful"'
                    + ' data-units="requests/s"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="12%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-common-max="' + id + '"'
                    + ' data-colors="' + NETDATA.colors[0] + '"'
                    + ' data-decimal-digits="0"'
                    + ' role="application"></div>';
            },

            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="redirect"'
                    + ' data-chart-library="gauge"'
                    + ' data-title="Redirects"'
                    + ' data-units="requests/s"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="12%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-common-max="' + id + '"'
                    + ' data-colors="' + NETDATA.colors[2] + '"'
                    + ' data-decimal-digits="0"'
                    + ' role="application"></div>';
            },

            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="bad"'
                    + ' data-chart-library="gauge"'
                    + ' data-title="Bad Requests"'
                    + ' data-units="requests/s"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="12%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-common-max="' + id + '"'
                    + ' data-colors="' + NETDATA.colors[3] + '"'
                    + ' data-decimal-digits="0"'
                    + ' role="application"></div>';
            },

            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="error"'
                    + ' data-chart-library="gauge"'
                    + ' data-title="Server Errors"'
                    + ' data-units="requests/s"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="12%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-common-max="' + id + '"'
                    + ' data-colors="' + NETDATA.colors[1] + '"'
                    + ' data-decimal-digits="0"'
                    + ' role="application"></div>';
            }
        ]
    },

    'web_log.response_codes': {
        info: 'Web server responses by code family. ' +
            'According to the standards <code>1xx</code> are informational responses, ' +
            '<code>2xx</code> are successful responses, ' +
            '<code>3xx</code> are redirects (although they include <b>304</b> which is used as "<b>not modified</b>"), ' +
            '<code>4xx</code> are bad requests, ' +
            '<code>5xx</code> are internal server errors, ' +
            '<code>other</code> are non-standard responses, ' +
            '<code>unmatched</code> counts the lines in the log file that are not matched by the plugin (<a href="https://github.com/netdata/netdata/issues/new?title=web_log%20reports%20unmatched%20lines&body=web_log%20plugin%20reports%20unmatched%20lines.%0A%0AThis%20is%20my%20log:%0A%0A%60%60%60txt%0A%0Aplease%20paste%20your%20web%20server%20log%20here%0A%0A%60%60%60" target="_blank">let us know</a> if you have any unmatched).'
    },

    'web_log.response_time': {
        mainheads: [
            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="avg"'
                    + ' data-chart-library="gauge"'
                    + ' data-title="Average Response Time"'
                    + ' data-units="milliseconds"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="12%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-colors="' + NETDATA.colors[4] + '"'
                    + ' data-decimal-digits="2"'
                    + ' role="application"></div>';
            }
        ]
    },

    'web_log.detailed_response_codes': {
        info: 'Number of responses for each response code individually.'
    },

    'web_log.requests_per_ipproto': {
        info: 'Web server requests received per IP protocol version.'
    },

    'web_log.clients': {
        info: 'Unique client IPs accessing the web server, within each data collection iteration. If data collection is <b>per second</b>, this chart shows <b>unique client IPs per second</b>.'
    },

    'web_log.clients_all': {
        info: 'Unique client IPs accessing the web server since the last restart of netdata. This plugin keeps in memory all the unique IPs that have accessed the web server. On very busy web servers (several millions of unique IPs) you may want to disable this chart (check <a href="https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/web_log/web_log.conf" target="_blank"><code>/etc/netdata/python.d/web_log.conf</code></a>).'
    },

    // ------------------------------------------------------------------------
    // web_log for squid

    'web_log.squid_response_statuses': {
        info: 'Squid responses by type. ' +
            '<code>success</code> includes <b>1xx</b>, <b>2xx</b>, <b>000</b>, <b>304</b>, ' +
            '<code>error</code> includes <b>5xx</b> and <b>6xx</b>, ' +
            '<code>redirect</code> includes <b>3xx</b> except <b>304</b>, ' +
            '<code>bad</code> includes <b>4xx</b>, ' +
            '<code>other</code> are all the other responses.',
        mainheads: [
            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="success"'
                    + ' data-chart-library="gauge"'
                    + ' data-title="Successful"'
                    + ' data-units="requests/s"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="12%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-common-max="' + id + '"'
                    + ' data-colors="' + NETDATA.colors[0] + '"'
                    + ' data-decimal-digits="0"'
                    + ' role="application"></div>';
            },

            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="redirect"'
                    + ' data-chart-library="gauge"'
                    + ' data-title="Redirects"'
                    + ' data-units="requests/s"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="12%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-common-max="' + id + '"'
                    + ' data-colors="' + NETDATA.colors[2] + '"'
                    + ' data-decimal-digits="0"'
                    + ' role="application"></div>';
            },

            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="bad"'
                    + ' data-chart-library="gauge"'
                    + ' data-title="Bad Requests"'
                    + ' data-units="requests/s"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="12%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-common-max="' + id + '"'
                    + ' data-colors="' + NETDATA.colors[3] + '"'
                    + ' data-decimal-digits="0"'
                    + ' role="application"></div>';
            },

            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="error"'
                    + ' data-chart-library="gauge"'
                    + ' data-title="Server Errors"'
                    + ' data-units="requests/s"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="12%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-common-max="' + id + '"'
                    + ' data-colors="' + NETDATA.colors[1] + '"'
                    + ' data-decimal-digits="0"'
                    + ' role="application"></div>';
            }
        ]
    },

    'web_log.squid_response_codes': {
        info: 'Web server responses by code family. ' +
            'According to HTTP standards <code>1xx</code> are informational responses, ' +
            '<code>2xx</code> are successful responses, ' +
            '<code>3xx</code> are redirects (although they include <b>304</b> which is used as "<b>not modified</b>"), ' +
            '<code>4xx</code> are bad requests, ' +
            '<code>5xx</code> are internal server errors. ' +
            'Squid also defines <code>000</code> mostly for UDP requests, and ' +
            '<code>6xx</code> for broken upstream servers sending wrong headers. ' +
            'Finally, <code>other</code> are non-standard responses, and ' +
            '<code>unmatched</code> counts the lines in the log file that are not matched by the plugin (<a href="https://github.com/netdata/netdata/issues/new?title=web_log%20reports%20unmatched%20lines&body=web_log%20plugin%20reports%20unmatched%20lines.%0A%0AThis%20is%20my%20log:%0A%0A%60%60%60txt%0A%0Aplease%20paste%20your%20web%20server%20log%20here%0A%0A%60%60%60" target="_blank">let us know</a> if you have any unmatched).'
    },

    'web_log.squid_duration': {
        mainheads: [
            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="avg"'
                    + ' data-chart-library="gauge"'
                    + ' data-title="Average Response Time"'
                    + ' data-units="milliseconds"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="12%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-colors="' + NETDATA.colors[4] + '"'
                    + ' data-decimal-digits="2"'
                    + ' role="application"></div>';
            }
        ]
    },

    'web_log.squid_detailed_response_codes': {
        info: 'Number of responses for each response code individually.'
    },

    'web_log.squid_clients': {
        info: 'Unique client IPs accessing squid, within each data collection iteration. If data collection is <b>per second</b>, this chart shows <b>unique client IPs per second</b>.'
    },

    'web_log.squid_clients_all': {
        info: 'Unique client IPs accessing squid since the last restart of netdata. This plugin keeps in memory all the unique IPs that have accessed the server. On very busy squid servers (several millions of unique IPs) you may want to disable this chart (check <a href="https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/web_log/web_log.conf" target="_blank"><code>/etc/netdata/python.d/web_log.conf</code></a>).'
    },

    'web_log.squid_transport_methods': {
        info: 'Break down per delivery method: <code>TCP</code> are requests on the HTTP port (usually 3128), ' +
            '<code>UDP</code> are requests on the ICP port (usually 3130), or HTCP port (usually 4128). ' +
            'If ICP logging was disabled using the log_icp_queries option, no ICP replies will be logged. ' +
            '<code>NONE</code> are used to state that squid delivered an unusual response or no response at all. ' +
            'Seen with cachemgr requests and errors, usually when the transaction fails before being classified into one of the above outcomes. ' +
            'Also seen with responses to <code>CONNECT</code> requests.'
    },

    'web_log.squid_code': {
        info: 'These are combined squid result status codes. A break down per component is given in the following charts. ' +
            'Check the <a href="http://wiki.squid-cache.org/SquidFaq/SquidLogs">squid documentation about them</a>.'
    },

    'web_log.squid_handling_opts': {
        info: 'These tags are optional and describe why the particular handling was performed or where the request came from. ' +
            '<code>CLIENT</code> means that the client request placed limits affecting the response. Usually seen with client issued a <b>no-cache</b>, or analogous cache control command along with the request. Thus, the cache has to validate the object.' +
            '<code>IMS</code> states that the client sent a revalidation (conditional) request. ' +
            '<code>ASYNC</code>, is used when the request was generated internally by Squid. Usually this is background fetches for cache information exchanges, background revalidation from stale-while-revalidate cache controls, or ESI sub-objects being loaded. ' +
            '<code>SWAPFAIL</code> is assigned when the object was believed to be in the cache, but could not be accessed. A new copy was requested from the server. ' +
            '<code>REFRESH</code> when a revalidation (conditional) request was sent to the server. ' +
            '<code>SHARED</code> when this request was combined with an existing transaction by collapsed forwarding. NOTE: the existing request is not marked as SHARED. ' +
            '<code>REPLY</code> when particular handling was requested in the HTTP reply from server or peer. Usually seen on DENIED due to http_reply_access ACLs preventing delivery of servers response object to the client.'
    },

    'web_log.squid_object_types': {
        info: 'These tags are optional and describe what type of object was produced. ' +
            '<code>NEGATIVE</code> is only seen on HIT responses, indicating the response was a cached error response. e.g. <b>404 not found</b>. ' +
            '<code>STALE</code> means the object was cached and served stale. This is usually caused by stale-while-revalidate or stale-if-error cache controls. ' +
            '<code>OFFLINE</code> when the requested object was retrieved from the cache during offline_mode. The offline mode never validates any object. ' +
            '<code>INVALID</code> when an invalid request was received. An error response was delivered indicating what the problem was. ' +
            '<code>FAIL</code> is only seen on <code>REFRESH</code> to indicate the revalidation request failed. The response object may be the server provided network error or the stale object which was being revalidated depending on stale-if-error cache control. ' +
            '<code>MODIFIED</code> is only seen on <code>REFRESH</code> responses to indicate revalidation produced a new modified object. ' +
            '<code>UNMODIFIED</code> is only seen on <code>REFRESH</code> responses to indicate revalidation produced a <b>304</b> (Not Modified) status, which was relayed to the client. ' +
            '<code>REDIRECT</code> when squid generated an HTTP redirect response to this request.'
    },

    'web_log.squid_cache_events': {
        info: 'These tags are optional and describe whether the response was loaded from cache, network, or otherwise. ' +
            '<code>HIT</code> when the response object delivered was the local cache object. ' +
            '<code>MEM</code> when the response object came from memory cache, avoiding disk accesses. Only seen on HIT responses. ' +
            '<code>MISS</code> when the response object delivered was the network response object. ' +
            '<code>DENIED</code> when the request was denied by access controls. ' +
            '<code>NOFETCH</code> an ICP specific type, indicating service is alive, but not to be used for this request (sent during "-Y" startup, or during frequent failures, a cache in hit only mode will return either UDP_HIT or UDP_MISS_NOFETCH. Neighbours will thus only fetch hits). ' +
            '<code>TUNNEL</code> when a binary tunnel was established for this transaction.'
    },

    'web_log.squid_transport_errors': {
        info: 'These tags are optional and describe some error conditions which occurred during response delivery (if any). ' +
            '<code>ABORTED</code> when the response was not completed due to the connection being aborted (usually by the client). ' +
            '<code>TIMEOUT</code>, when the response was not completed due to a connection timeout.'
    },

     // ------------------------------------------------------------------------
    // go web_log

    'web_log.type_requests': {
        info: 'Web server responses by type. <code>success</code> includes <b>1xx</b>, <b>2xx</b>, <b>304</b> and <b>401</b>, <code>error</code> includes <b>5xx</b>, <code>redirect</code> includes <b>3xx</b> except <b>304</b>, <code>bad</code> includes <b>4xx</b> except <b>401</b>, <code>other</code> are all the other responses.',
        mainheads: [
            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="success"'
                    + ' data-chart-library="gauge"'
                    + ' data-title="Successful"'
                    + ' data-units="requests/s"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="12%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-common-max="' + id + '"'
                    + ' data-colors="' + NETDATA.colors[0] + '"'
                    + ' data-decimal-digits="0"'
                    + ' role="application"></div>';
            },

            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="redirect"'
                    + ' data-chart-library="gauge"'
                    + ' data-title="Redirects"'
                    + ' data-units="requests/s"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="12%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-common-max="' + id + '"'
                    + ' data-colors="' + NETDATA.colors[2] + '"'
                    + ' data-decimal-digits="0"'
                    + ' role="application"></div>';
            },

            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="bad"'
                    + ' data-chart-library="gauge"'
                    + ' data-title="Bad Requests"'
                    + ' data-units="requests/s"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="12%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-common-max="' + id + '"'
                    + ' data-colors="' + NETDATA.colors[3] + '"'
                    + ' data-decimal-digits="0"'
                    + ' role="application"></div>';
            },

            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="error"'
                    + ' data-chart-library="gauge"'
                    + ' data-title="Server Errors"'
                    + ' data-units="requests/s"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="12%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-common-max="' + id + '"'
                    + ' data-colors="' + NETDATA.colors[1] + '"'
                    + ' data-decimal-digits="0"'
                    + ' role="application"></div>';
            }
        ]
    },

    'web_log.request_processing_time': {
        mainheads: [
            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="avg"'
                    + ' data-chart-library="gauge"'
                    + ' data-title="Average Response Time"'
                    + ' data-units="milliseconds"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="12%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-colors="' + NETDATA.colors[4] + '"'
                    + ' data-decimal-digits="2"'
                    + ' role="application"></div>';
            }
        ]
    },
    // ------------------------------------------------------------------------
    // Fronius Solar Power

    'fronius.power': {
        info: 'Positive <code>Grid</code> values mean that power is coming from the grid. Negative values are excess power that is going back into the grid, possibly selling it. ' +
            '<code>Photovoltaics</code> is the power generated from the solar panels. ' +
            '<code>Accumulator</code> is the stored power in the accumulator, if one is present.'
    },

    'fronius.autonomy': {
        commonMin: true,
        commonMax: true,
        valueRange: "[0, 100]",
        info: 'The <code>Autonomy</code> is the percentage of how autonomous the installation is. An autonomy of 100 % means that the installation is producing more energy than it is needed. ' +
            'The <code>Self consumption</code> indicates the ratio between the current power generated and the current load. When it reaches 100 %, the <code>Autonomy</code> declines, since the solar panels can not produce enough energy and need support from the grid.'
    },

    'fronius.energy.today': {
        commonMin: true,
        commonMax: true,
        valueRange: "[0, null]"
    },

    // ------------------------------------------------------------------------
    // Stiebel Eltron Heat pump installation

    'stiebeleltron.system.roomtemp': {
        commonMin: true,
        commonMax: true,
        valueRange: "[0, null]"
    },

    // ------------------------------------------------------------------------
    // Port check

    'portcheck.latency': {
        info: 'The <code>latency</code> describes the time spent connecting to a TCP port. No data is sent or received. ' +
            'Currently, the accuracy of the latency is low and should be used as reference only.'
    },

    'portcheck.status': {
        valueRange: "[0, 1]",
        info: 'The <code>status</code> chart verifies the availability of the service. ' +
            'Each status dimension will have a value of <code>1</code> if triggered. Dimension <code>success</code> is <code>1</code> only if connection could be established. ' +
            'This chart is most useful for alarms and third-party apps.'
    },

    // ------------------------------------------------------------------------

    'chrony.system': {
        info: 'In normal operation, chronyd never steps the system clock, because any jump in the timescale can have adverse consequences for certain application programs. Instead, any error in the system clock is corrected by slightly speeding up or slowing down the system clock until the error has been removed, and then returning to the system clock’s normal speed. A consequence of this is that there will be a period when the system clock (as read by other programs using the <code>gettimeofday()</code> system call, or by the <code>date</code> command in the shell) will be different from chronyd\'s estimate of the current true time (which it reports to NTP clients when it is operating in server mode). The value reported on this line is the difference due to this effect.',
        colors: NETDATA.colors[3]
    },

    'chrony.offsets': {
        info: '<code>last offset</code> is the estimated local offset on the last clock update. <code>RMS offset</code> is a long-term average of the offset value.',
        height: 0.5
    },

    'chrony.stratum': {
        info: 'The <code>stratum</code> indicates how many hops away from a computer with an attached reference clock we are. Such a computer is a stratum-1 computer.',
        decimalDigits: 0,
        height: 0.5
    },

    'chrony.root': {
        info: 'Estimated delays against the root time server this system is synchronized with. <code>delay</code> is the total of the network path delays to the stratum-1 computer from which the computer is ultimately synchronised. <code>dispersion</code> is the total dispersion accumulated through all the computers back to the stratum-1 computer from which the computer is ultimately synchronised. Dispersion is due to system clock resolution, statistical measurement variations etc.'
    },

    'chrony.frequency': {
        info: 'The <code>frequency</code> is the rate by which the system\'s clock would be would be wrong if chronyd was not correcting it. It is expressed in ppm (parts per million). For example, a value of 1ppm would mean that when the system\'s clock thinks it has advanced 1 second, it has actually advanced by 1.000001 seconds relative to true time.',
        colors: NETDATA.colors[0]
    },

    'chrony.residualfreq': {
        info: 'This shows the <code>residual frequency</code> for the currently selected reference source. ' +
            'It reflects any difference between what the measurements from the reference source indicate the ' +
            'frequency should be and the frequency currently being used. The reason this is not always zero is ' +
            'that a smoothing procedure is applied to the frequency. Each time a measurement from the reference ' +
            'source is obtained and a new residual frequency computed, the estimated accuracy of this residual ' +
            'is compared with the estimated accuracy (see <code>skew</code>) of the existing frequency value. ' +
            'A weighted average is computed for the new frequency, with weights depending on these accuracies. ' +
            'If the measurements from the reference source follow a consistent trend, the residual will be ' +
            'driven to zero over time.',
        height: 0.5,
        colors: NETDATA.colors[3]
    },

    'chrony.skew': {
        info: 'The estimated error bound on the frequency.',
        height: 0.5,
        colors: NETDATA.colors[5]
    },

    'couchdb.active_tasks': {
        info: 'Active tasks running on this CouchDB <b>cluster</b>. Four types of tasks currently exist: indexer (view building), replication, database compaction and view compaction.'
    },

    'couchdb.replicator_jobs': {
        info: 'Detailed breakdown of any replication jobs in progress on this node. For more information, see the <a href="http://docs.couchdb.org/en/latest/replication/replicator.html">replicator documentation</a>.'
    },

    'couchdb.open_files': {
        info: 'Count of all files held open by CouchDB. If this value seems pegged at 1024 or 4096, your server process is probably hitting the open file handle limit and <a href="http://docs.couchdb.org/en/latest/maintenance/performance.html#pam-and-ulimit">needs to be increased.</a>'
    },

    'btrfs.disk': {
        info: 'Physical disk usage of BTRFS. The disk space reported here is the raw physical disk space assigned to the BTRFS volume (i.e. <b>before any RAID levels</b>). BTRFS uses a two-stage allocator, first allocating large regions of disk space for one type of block (data, metadata, or system), and then using a regular block allocator inside those regions. <code>unallocated</code> is the physical disk space that is not allocated yet and is available to become data, metadata or system on demand. When <code>unallocated</code> is zero, all available disk space has been allocated to a specific function. Healthy volumes should ideally have at least five percent of their total space <code>unallocated</code>. You can keep your volume healthy by running the <code>btrfs balance</code> command on it regularly (check <code>man btrfs-balance</code> for more info).  Note that some of the space listed as <code>unallocated</code> may not actually be usable if the volume uses devices of different sizes.',
        colors: [NETDATA.colors[12]]
    },

    'btrfs.data': {
        info: 'Logical disk usage for BTRFS data. Data chunks are used to store the actual file data (file contents). The disk space reported here is the usable allocation (i.e. after any striping or replication). Healthy volumes should ideally have no more than a few GB of free space reported here persistently. Running <code>btrfs balance</code> can help here.'
    },

    'btrfs.metadata': {
        info: 'Logical disk usage for BTRFS metadata. Metadata chunks store most of the filesystem internal structures, as well as information like directory structure and file names. The disk space reported here is the usable allocation (i.e. after any striping or replication). Healthy volumes should ideally have no more than a few GB of free space reported here persistently. Running <code>btrfs balance</code> can help here.'
    },

    'btrfs.system': {
        info: 'Logical disk usage for BTRFS system. System chunks store information about the allocation of other chunks. The disk space reported here is the usable allocation (i.e. after any striping or replication). The values reported here should be relatively small compared to Data and Metadata, and will scale with the volume size and overall space usage.'
    },

    // ------------------------------------------------------------------------
    // RabbitMQ

    // info: the text above the charts
    // heads: the representation of the chart at the top the subsection (second level menu)
    // mainheads: the representation of the chart at the top of the section (first level menu)
    // colors: the dimension colors of the chart (the default colors are appended)
    // height: the ratio of the chart height relative to the default

    'rabbitmq.queued_messages': {
        info: 'Overall total of ready and unacknowledged queued messages.  Messages that are delivered immediately are not counted here.'
    },

    'rabbitmq.message_rates': {
        info: 'Overall messaging rates including acknowledgements, deliveries, redeliveries, and publishes.'
    },

    'rabbitmq.global_counts': {
        info: 'Overall totals for channels, consumers, connections, queues and exchanges.'
    },

    'rabbitmq.file_descriptors': {
        info: 'Total number of used filed descriptors. See <code><a href="https://www.rabbitmq.com/production-checklist.html#resource-limits-file-handle-limit" target="_blank">Open File Limits</a></code> for further details.',
        colors: NETDATA.colors[3]
    },

    'rabbitmq.sockets': {
        info: 'Total number of used socket descriptors.  Each used socket also counts as a used file descriptor.  See <code><a href="https://www.rabbitmq.com/production-checklist.html#resource-limits-file-handle-limit" target="_blank">Open File Limits</a></code> for further details.',
        colors: NETDATA.colors[3]
    },

    'rabbitmq.processes': {
        info: 'Total number of processes running within the Erlang VM.  This is not the same as the number of processes running on the host.',
        colors: NETDATA.colors[3]
    },

    'rabbitmq.erlang_run_queue': {
        info: 'Number of Erlang processes the Erlang schedulers have queued to run.',
        colors: NETDATA.colors[3]
    },

    'rabbitmq.memory': {
        info: 'Total amount of memory used by the RabbitMQ.  This is a complex statistic that can be further analyzed in the management UI.  See <code><a href="https://www.rabbitmq.com/production-checklist.html#resource-limits-ram" target="_blank">Memory</a></code> for further details.',
        colors: NETDATA.colors[3]
    },

    'rabbitmq.disk_space': {
        info: 'Total amount of disk space consumed by the message store(s).  See <code><a href="https://www.rabbitmq.com/production-checklist.html#resource-limits-disk-space" target=_"blank">Disk Space Limits</a></code> for further details.',
        colors: NETDATA.colors[3]
    },

    'rabbitmq.queue_messages': {
        info: 'Total amount of messages and their states in this queue.',
        colors: NETDATA.colors[3]
    },

    'rabbitmq.queue_messages_stats': {
        info: 'Overall messaging rates including acknowledgements, deliveries, redeliveries, and publishes.',
        colors: NETDATA.colors[3]
    },

    // ------------------------------------------------------------------------
    // ntpd

    'ntpd.sys_offset': {
        info: 'For hosts without any time critical services an offset of &lt; 100 ms should be acceptable even with high network latencies. For hosts with time critical services an offset of about 0.01 ms or less can be achieved by using peers with low delays and configuring optimal <b>poll exponent</b> values.',
        colors: NETDATA.colors[4]
    },

    'ntpd.sys_jitter': {
        info: 'The jitter statistics are exponentially-weighted RMS averages. The system jitter is defined in the NTPv4 specification; the clock jitter statistic is computed by the clock discipline module.'
    },

    'ntpd.sys_frequency': {
        info: 'The frequency offset is shown in ppm (parts per million) relative to the frequency of the system. The frequency correction needed for the clock can vary significantly between boots and also due to external influences like temperature or radiation.',
        colors: NETDATA.colors[2],
        height: 0.6
    },

    'ntpd.sys_wander': {
        info: 'The wander statistics are exponentially-weighted RMS averages.',
        colors: NETDATA.colors[3],
        height: 0.6
    },

    'ntpd.sys_rootdelay': {
        info: 'The rootdelay is the round-trip delay to the primary reference clock, similar to the delay shown by the <code>ping</code> command. A lower delay should result in a lower clock offset.',
        colors: NETDATA.colors[1]
    },

    'ntpd.sys_stratum': {
        info: 'The distance in "hops" to the primary reference clock',
        colors: NETDATA.colors[5],
        height: 0.3
    },

    'ntpd.sys_tc': {
        info: 'Time constants and poll intervals are expressed as exponents of 2. The default poll exponent of 6 corresponds to a poll interval of 64 s. For typical Internet paths, the optimum poll interval is about 64 s. For fast LANs with modern computers, a poll exponent of 4 (16 s) is appropriate. The <a href="http://doc.ntp.org/current-stable/poll.html">poll process</a> sends NTP packets at intervals determined by the clock discipline algorithm.',
        height: 0.5
    },

    'ntpd.sys_precision': {
        colors: NETDATA.colors[6],
        height: 0.2
    },

    'ntpd.peer_offset': {
        info: 'The offset of the peer clock relative to the system clock in milliseconds. Smaller values here weight peers more heavily for selection after the initial synchronization of the local clock. For a system providing time service to other systems, these should be as low as possible.'
    },

    'ntpd.peer_delay': {
        info: 'The round-trip time (RTT) for communication with the peer, similar to the delay shown by the <code>ping</code> command. Not as critical as either the offset or jitter, but still factored into the selection algorithm (because as a general rule, lower delay means more accurate time). In most cases, it should be below 100ms.'
    },

    'ntpd.peer_dispersion': {
        info: 'This is a measure of the estimated error between the peer and the local system. Lower values here are better.'
    },

    'ntpd.peer_jitter': {
        info: 'This is essentially a remote estimate of the peer\'s <code>system_jitter</code> value. Lower values here weight highly in favor of peer selection, and this is a good indicator of overall quality of a given time server (good servers will have values not exceeding single digit milliseconds here, with high quality stratum one servers regularly having sub-millisecond jitter).'
    },

    'ntpd.peer_xleave': {
        info: 'This variable is used in interleaved mode (used only in NTP symmetric and broadcast modes). See <a href="http://doc.ntp.org/current-stable/xleave.html">NTP Interleaved Modes</a>.'
    },

    'ntpd.peer_rootdelay': {
        info: 'For a stratum 1 server, this is the access latency for the reference clock. For lower stratum servers, it is the sum of the <code>peer_delay</code> and <code>peer_rootdelay</code> for the system they are syncing off of. Similarly to <code>peer_delay</code>, lower values here are technically better, but have limited influence in peer selection.'
    },

    'ntpd.peer_rootdisp': {
        info: 'Is the same as <code>peer_rootdelay</code>, but measures accumulated <code>peer_dispersion</code> instead of accumulated <code>peer_delay</code>.'
    },

    'ntpd.peer_hmode': {
        info: 'The <code>peer_hmode</code> and <code>peer_pmode</code> variables give info about what mode the packets being sent to and received from a given peer are. Mode 1 is symmetric active (both the local system and the remote peer have each other declared as peers in <code>/etc/ntp.conf</code>), Mode 2 is symmetric passive (only one side has the other declared as a peer), Mode 3 is client, Mode 4 is server, and Mode 5 is broadcast (also used for multicast and manycast operation).',
        height: 0.2
    },

    'ntpd.peer_pmode': {
        height: 0.2
    },

    'ntpd.peer_hpoll': {
        info: 'The <code>peer_hpoll</code> and <code>peer_ppoll</code> variables are log2 representations of the polling interval in seconds.',
        height: 0.5
    },

    'ntpd.peer_ppoll': {
        height: 0.5
    },

    'ntpd.peer_precision': {
        height: 0.2
    },

    'spigotmc.tps': {
        info: 'The running 1, 5, and 15 minute average number of server ticks per second.  An idealized server will show 20.0 for all values, but in practice this almost never happens.  Typical servers should show approximately 19.98-20.0 here.  Lower values indicate progressively more server-side lag (and thus that you need better hardware for your server or a lower user limit).  For every 0.05 ticks below 20, redstone clocks will lag behind by approximately 0.25%.  Values below approximately 19.50 may interfere with complex free-running redstone circuits and will noticeably slow down growth.'
    },

    'spigotmc.users': {
        info: 'The number of currently connect users on the monitored Spigot server.'
    },

    'boinc.tasks': {
        info: 'The total number of tasks and the number of active tasks.  Active tasks are those which are either currently being processed, or are partially processed but suspended.'
    },

    'boinc.states': {
        info: 'Counts of tasks in each task state.  The normal sequence of states is <code>New</code>, <code>Downloading</code>, <code>Ready to Run</code>, <code>Uploading</code>, <code>Uploaded</code>.  Tasks which are marked <code>Ready to Run</code> may be actively running, or may be waiting to be scheduled.  <code>Compute Errors</code> are tasks which failed for some reason during execution.  <code>Aborted</code> tasks were manually cancelled, and will not be processed.  <code>Failed Uploads</code> are otherwise finished tasks which failed to upload to the server, and usually indicate networking issues.'
    },

    'boinc.sched': {
        info: 'Counts of active tasks in each scheduling state.  <code>Scheduled</code> tasks are the ones which will run if the system is permitted to process tasks.  <code>Preempted</code> tasks are on standby, and will run if a <code>Scheduled</code> task stops running for some reason.  <code>Uninitialized</code> tasks should never be present, and indicate tha the scheduler has not tried to schedule them yet.'
    },

    'boinc.process': {
        info: 'Counts of active tasks in each process state.  <code>Executing</code> tasks are running right now.  <code>Suspended</code> tasks have an associated process, but are not currently running (either because the system isn\'t processing any tasks right now, or because they have been preempted by higher priority tasks).  <code>Quit</code> tasks are exiting gracefully.  <code>Aborted</code> tasks exceeded some resource limit, and are being shut down.  <code>Copy Pending</code> tasks are waiting on a background file transfer to finish.  <code>Uninitialized</code> tasks do not have an associated process yet.'
    },

    'w1sensor.temp': {
        info: 'Temperature derived from 1-Wire temperature sensors.'
    },

    'logind.sessions': {
        info: 'Shows the number of active sessions of each type tracked by logind.'
    },

    'logind.users': {
        info: 'Shows the number of active users of each type tracked by logind.'
    },

    'logind.seats': {
        info: 'Shows the number of active seats tracked by logind.  Each seat corresponds to a combination of a display device and input device providing a physical presence for the system.'
    },

    // ------------------------------------------------------------------------
    // ProxySQL

    'proxysql.pool_status': {
        info: 'The status of the backend servers. ' +
            '<code>1=ONLINE</code> backend server is fully operational, ' +
            '<code>2=SHUNNED</code> backend sever is temporarily taken out of use because of either too many connection errors in a time that was too short, or replication lag exceeded the allowed threshold, ' +
            '<code>3=OFFLINE_SOFT</code> when a server is put into OFFLINE_SOFT mode, new incoming connections aren\'t accepted anymore, while the existing connections are kept until they became inactive. In other words, connections are kept in use until the current transaction is completed. This allows to gracefully detach a backend, ' +
            '<code>4=OFFLINE_HARD</code> when a server is put into OFFLINE_HARD mode, the existing connections are dropped, while new incoming connections aren\'t accepted either. This is equivalent to deleting the server from a hostgroup, or temporarily taking it out of the hostgroup for maintenance work, ' +
            '<code>-1</code> Unknown status.'
    },

    'proxysql.pool_net': {
        info: 'The amount of data sent to/received from the backend ' +
            '(This does not include metadata (packets\' headers, OK/ERR packets, fields\' description, etc).'
    },

    'proxysql.pool_overall_net': {
        info: 'The amount of data sent to/received from the all backends ' +
            '(This does not include metadata (packets\' headers, OK/ERR packets, fields\' description, etc).'
    },

    'proxysql.questions': {
        info: '<code>questions</code> total number of queries sent from frontends, ' +
            '<code>slow_queries</code> number of queries that ran for longer than the threshold in milliseconds defined in global variable <code>mysql-long_query_time</code>. '
    },

    'proxysql.connections': {
        info: '<code>aborted</code> number of frontend connections aborted due to invalid credential or max_connections reached, ' +
            '<code>connected</code> number of frontend connections currently connected, ' +
            '<code>created</code> number of frontend connections created, ' +
            '<code>non_idle</code> number of frontend connections that are not currently idle. '
    },

    'proxysql.pool_latency': {
        info: 'The currently ping time in microseconds, as reported from Monitor.'
    },

    'proxysql.queries': {
        info: 'The number of queries routed towards this particular backend server.'
    },

    'proxysql.pool_used_connections': {
        info: 'The number of connections are currently used by ProxySQL for sending queries to the backend server.'
    },

    'proxysql.pool_free_connections': {
        info: 'The number of connections are currently free. They are kept open in order to minimize the time cost of sending a query to the backend server.'
    },

    'proxysql.pool_ok_connections': {
        info: 'The number of connections were established successfully.'
    },

    'proxysql.pool_error_connections': {
        info: 'The number of connections weren\'t established successfully.'
    },

    'proxysql.commands_count': {
        info: 'The total number of commands of that type executed'
    },

    'proxysql.commands_duration': {
        info: 'The total time spent executing commands of that type, in ms'
    },

    // ------------------------------------------------------------------------
    // Power Supplies

    'powersupply.capacity': {
        info: undefined
    },

    'powersupply.charge': {
        info: undefined
    },

    'powersupply.energy': {
        info: undefined
    },

    'powersupply.voltage': {
        info: undefined
    },

    // ------------------------------------------------------------------------
    // VMware vSphere

    // Host specific
    'vsphere.host_mem_usage_percentage': {
        info: 'Percentage of used machine memory: <code>consumed</code> / <code>machine-memory-size</code>.'
    },

    'vsphere.host_mem_usage': {
        info:
            '<code>granted</code> is amount of machine memory that is mapped for a host, ' +
            'it equals sum of all granted metrics for all powered-on virtual machines, plus machine memory for vSphere services on the host. ' +
            '<code>consumed</code> is amount of machine memory used on the host, it includes memory used by the Service Console, the VMkernel, vSphere services, plus the total consumed metrics for all running virtual machines. ' +
            '<code>consumed</code> = <code>total host memory</code> - <code>free host memory</code>.' +
            '<code>active</code> is sum of all active metrics for all powered-on virtual machines plus vSphere services (such as COS, vpxa) on the host.' +
            '<code>shared</code> is sum of all shared metrics for all powered-on virtual machines, plus amount for vSphere services on the host. ' +
            '<code>sharedcommon</code> is amount of machine memory that is shared by all powered-on virtual machines and vSphere services on the host. ' +
            '<code>shared</code> - <code>sharedcommon</code> = machine memory (host memory) savings (KB). ' +
            'For details see <a href="https://docs.vmware.com/en/VMware-vSphere/6.5/com.vmware.vsphere.resmgmt.doc/GUID-BFDC988B-F53D-4E97-9793-A002445AFAE1.html">Measuring and Differentiating Types of Memory Usage</a> and ' +
            '<a href="https://www.vmware.com/support/developer/converter-sdk/conv51_apireference/memory_counters.html">Memory Counters</a> articles.'
    },

    'vsphere.host_mem_swap_rate': {
        info:
            'This statistic refers to VMkernel swapping and not to guest OS swapping. ' +
            '<code>in</code> is sum of <code>swapinRate</code> values for all powered-on virtual machines on the host.' +
            '<code>swapinRate</code> is rate at which VMKernel reads data into machine memory from the swap file. ' +
            '<code>out</code> is sum of <code>swapoutRate</code> values for all powered-on virtual machines on the host.' +
            '<code>swapoutRate</code> is rate at which VMkernel writes to the virtual machine’s swap file from machine memory.'
    },

    // VM specific
    'vsphere.vm_mem_usage_percentage': {
        info: 'Percentage of used virtual machine “physical” memory: <code>active</code> / <code>virtual machine configured size</code>.'
    },

    'vsphere.vm_mem_usage': {
        info:
            '<code>granted</code> is amount of guest “physical” memory that is mapped to machine memory, it includes <code>shared</code> memory amount. ' +
            '<code>consumed</code> is amount of guest “physical” memory consumed by the virtual machine for guest memory, ' +
            '<code>consumed</code> = <code>granted</code> - <code>memory saved due to memory sharing</code>. ' +
            '<code>active</code> is amount of memory that is actively used, as estimated by VMkernel based on recently touched memory pages. ' +
            '<code>shared</code> is amount of guest “physical” memory shared with other virtual machines (through the VMkernel’s transparent page-sharing mechanism, a RAM de-duplication technique). ' +
            'For details see <a href="https://docs.vmware.com/en/VMware-vSphere/6.5/com.vmware.vsphere.resmgmt.doc/GUID-BFDC988B-F53D-4E97-9793-A002445AFAE1.html">Measuring and Differentiating Types of Memory Usage</a> and ' +
            '<a href="https://www.vmware.com/support/developer/converter-sdk/conv51_apireference/memory_counters.html">Memory Counters</a> articles.'

    },

    'vsphere.vm_mem_swap_rate': {
        info:
            'This statistic refers to VMkernel swapping and not to guest OS swapping. ' +
            '<code>in</code> is rate at which VMKernel reads data into machine memory from the swap file. ' +
            '<code>out</code> is rate at which VMkernel writes to the virtual machine’s swap file from machine memory.'
    },

    'vsphere.vm_mem_swap': {
        info:
            'This statistic refers to VMkernel swapping and not to guest OS swapping. ' +
            '<code>swapped</code> is amount of guest physical memory swapped out to the virtual machine\'s swap file by the VMkernel. ' +
            'Swapped memory stays on disk until the virtual machine needs it.'
    },

    // Common
    'vsphere.cpu_usage_total': {
        info: 'Summary CPU usage statistics across all CPUs/cores.'
    },

    'vsphere.net_bandwidth_total': {
        info: 'Summary receive/transmit statistics across all network interfaces.'
    },

    'vsphere.net_packets_total': {
        info: 'Summary receive/transmit statistics across all network interfaces.'
    },

    'vsphere.net_errors_total': {
        info: 'Summary receive/transmit statistics across all network interfaces.'
    },

    'vsphere.net_drops_total': {
        info: 'Summary receive/transmit statistics across all network interfaces.'
    },

    'vsphere.disk_usage_total': {
        info: 'Summary read/write statistics across all disks.'
    },

    'vsphere.disk_max_latency': {
        info: '<code>latency</code> is highest latency value across all disks.'
    },

    'vsphere.overall_status': {
        info: '<code>0</code> is unknown, <code>1</code> is OK, <code>2</code> is might have a problem, <code>3</code> is definitely has a problem.'
    },

    // ------------------------------------------------------------------------
    // VCSA
    'vcsa.system_health': {
        info:
            '<code>-1</code>: unknown; ' +
            '<code>0</code>: all components are healthy; ' +
            '<code>1</code>: one or more components might become overloaded soon; ' +
            '<code>2</code>: one or more components in the appliance might be degraded; ' +
            '<code>3</code>: one or more components might be in an unusable status and the appliance might become unresponsive soon; ' +
            '<code>4</code>: no health data is available.'
    },

    'vcsa.components_health': {
        info:
            '<code>-1</code>: unknown; ' +
            '<code>0</code>: healthy; ' +
            '<code>1</code>: healthy, but may have some problems; ' +
            '<code>2</code>: degraded, and may have serious problems; ' +
            '<code>3</code>: unavailable, or will stop functioning soon; ' +
            '<code>4</code>: no health data is available.'
    },

    'vcsa.software_updates_health': {
        info:
            '<code>softwarepackages</code> represents information on available software updates available in the remote vSphere Update Manager repository.<br>' +
            '<code>-1</code>: unknown; ' +
            '<code>0</code>: no updates available; ' +
            '<code>2</code>: non-security updates are available; ' +
            '<code>3</code>: security updates are available; ' +
            '<code>4</code>: an error retrieving information on software updates.'
    },

    // ------------------------------------------------------------------------
    // Zookeeper

    'zookeeper.server_state': {
        info:
            '<code>0</code>: unknown, ' +
            '<code>1</code>: leader, ' +
            '<code>2</code>: follower, ' +
            '<code>3</code>: observer, ' +
            '<code>4</code>: standalone.'
    },

    // ------------------------------------------------------------------------
    // Squidlog

    'squidlog.requests': {
        info: 'Total number of requests (log lines read). It includes <code>unmatched</code>.'
    },

    'squidlog.excluded_requests': {
        info: '<code>unmatched</code> counts the lines in the log file that are not matched by the plugin parser (<a href="https://github.com/netdata/netdata/issues/new?title=squidlog%20reports%20unmatched%20lines&body=squidlog%20plugin%20reports%20unmatched%20lines.%0A%0AThis%20is%20my%20log:%0A%0A%60%60%60txt%0A%0Aplease%20paste%20your%20squid%20server%20log%20here%0A%0A%60%60%60" target="_blank">let us know</a> if you have any unmatched).'
    },

    'squidlog.type_requests': {
        info: 'Requests by response type:<br>' +
            '<ul>' +
            ' <li><code>success</code> includes 1xx, 2xx, 0, 304, 401.</li>' +
            ' <li><code>error</code> includes 5xx and 6xx.</li>' +
            ' <li><code>redirect</code> includes 3xx except 304.</li>' +
            ' <li><code>bad</code> includes 4xx except 401.</li>' +
            ' </ul>'
    },

    'squidlog.http_status_code_class_responses': {
        info: 'The HTTP response status code classes. According to <a href="https://tools.ietf.org/html/rfc7231" target="_blank">rfc7231</a>:<br>' +
            ' <li><code>1xx</code> is informational responses.</li>' +
            ' <li><code>2xx</code> is successful responses.</li>' +
            ' <li><code>3xx</code> is redirects.</li>' +
            ' <li><code>4xx</code> is bad requests.</li>' +
            ' <li><code>5xx</code> is internal server errors.</li>' +
            ' </ul>' +
            'Squid also uses <code>0</code> for a result code being unavailable, and <code>6xx</code> to signal an invalid header, a proxy error.'
    },

    'squidlog.http_status_code_responses': {
        info: 'Number of responses for each http response status code individually.'
    },

    'squidlog.uniq_clients': {
        info: 'Unique clients (requesting instances), within each data collection iteration. If data collection is <b>per second</b>, this chart shows <b>unique clients per second</b>.'
    },

    'squidlog.bandwidth': {
        info: 'The size is the amount of data delivered to the clients. Mind that this does not constitute the net object size, as headers are also counted. ' +
            'Also, failed requests may deliver an error page, the size of which is also logged here.'
    },

    'squidlog.response_time': {
        info: 'The elapsed time considers how many milliseconds the transaction busied the cache. It differs in interpretation between TCP and UDP:' +
            '<ul>' +
            ' <li><code>TCP</code> this is basically the time from having received the request to when Squid finishes sending the last byte of the response.</li>' +
            ' <li><code>UDP</code> this is the time between scheduling a reply and actually sending it.</li>' +
            ' </ul>' +
            'Please note that <b>the entries are logged after the reply finished being sent</b>, not during the lifetime of the transaction.'
    },

    'squidlog.cache_result_code_requests': {
        info: 'The Squid result code is composed of several tags (separated by underscore characters) which describe the response sent to the client. ' +
            'Check the <a href="https://wiki.squid-cache.org/SquidFaq/SquidLogs#Squid_result_codes">squid documentation</a> about them.'
    },

    'squidlog.cache_result_code_transport_tag_requests': {
        info: 'These tags are always present and describe delivery method.<br>' +
            '<ul>' +
            ' <li><code>TCP</code> requests on the HTTP port (usually 3128).</li>' +
            ' <li><code>UDP</code> requests on the ICP port (usually 3130) or HTCP port (usually 4128).</li>' +
            ' <li><code>NONE</code> Squid delivered an unusual response or no response at all. Seen with cachemgr requests and errors, usually when the transaction fails before being classified into one of the above outcomes. Also seen with responses to CONNECT requests.</li>' +
            ' </ul>'
    },

    'squidlog.cache_result_code_handling_tag_requests': {
        info: 'These tags are optional and describe why the particular handling was performed or where the request came from.<br>' +
            '<ul>' +
            ' <li><code>CF</code> at least one request in this transaction was collapsed. See <a href="http://www.squid-cache.org/Doc/config/collapsed_forwarding/" target="_blank">collapsed_forwarding</a>  for more details about request collapsing.</li>' +
            ' <li><code>CLIENT</code> usually seen with client issued a "no-cache", or analogous cache control command along with the request. Thus, the cache has to validate the object.</li>' +
            ' <li><code>IMS</code> the client sent a revalidation (conditional) request.</li>' +
            ' <li><code>ASYNC</code> the request was generated internally by Squid. Usually this is background fetches for cache information exchanges, background revalidation from <i>stale-while-revalidate</i> cache controls, or ESI sub-objects being loaded.</li>' +
            ' <li><code>SWAPFAIL</code> the object was believed to be in the cache, but could not be accessed. A new copy was requested from the server.</li>' +
            ' <li><code>REFRESH</code> a revalidation (conditional) request was sent to the server.</li>' +
            ' <li><code>SHARED</code> this request was combined with an existing transaction by collapsed forwarding.</li>' +
            ' <li><code>REPLY</code> the HTTP reply from server or peer. Usually seen on <code>DENIED</code> due to <a href="http://www.squid-cache.org/Doc/config/http_reply_access/" target="_blank">http_reply_access</a> ACLs preventing delivery of servers response object to the client.</li>' +
            ' </ul>'
    },

    'squidlog.cache_code_object_tag_requests': {
        info: 'These tags are optional and describe what type of object was produced.<br>' +
            '<ul>' +
            ' <li><code>NEGATIVE</code> only seen on HIT responses, indicating the response was a cached error response. e.g. <b>404 not found</b>.</li>' +
            ' <li><code>STALE</code> the object was cached and served stale. This is usually caused by <i>stale-while-revalidate</i> or <i>stale-if-error</i> cache controls.</li>' +
            ' <li><code>OFFLINE</code> the requested object was retrieved from the cache during <a href="http://www.squid-cache.org/Doc/config/offline_mode/" target="_blank">offline_mode</a>. The offline mode never validates any object.</li>' +
            ' <li><code>INVALID</code> an invalid request was received. An error response was delivered indicating what the problem was.</li>' +
            ' <li><code>FAILED</code> only seen on <code>REFRESH</code> to indicate the revalidation request failed. The response object may be the server provided network error or the stale object which was being revalidated depending on stale-if-error cache control.</li>' +
            ' <li><code>MODIFIED</code> only seen on <code>REFRESH</code> responses to indicate revalidation produced a new modified object.</li>' +
            ' <li><code>UNMODIFIED</code> only seen on <code>REFRESH</code> responses to indicate revalidation produced a 304 (Not Modified) status. The client gets either a full 200 (OK), a 304 (Not Modified), or (in theory) another response, depending on the client request and other details.</li>' +
            ' <li><code>REDIRECT</code> Squid generated an HTTP redirect response to this request.</li>' +
            ' </ul>'
    },

    'squidlog.cache_code_load_source_tag_requests': {
        info: 'These tags are optional and describe whether the response was loaded from cache, network, or otherwise.<br>' +
            '<ul>' +
            ' <li><code>HIT</code> the response object delivered was the local cache object.</li>' +
            ' <li><code>MEM</code> the response object came from memory cache, avoiding disk accesses. Only seen on HIT responses.</li>' +
            ' <li><code>MISS</code> the response object delivered was the network response object.</li>' +
            ' <li><code>DENIED</code> the request was denied by access controls.</li>' +
            ' <li><code>NOFETCH</code> an ICP specific type, indicating service is alive, but not to be used for this request.</li>' +
            ' <li><code>TUNNEL</code> a binary tunnel was established for this transaction.</li>' +
            ' </ul>'
    },

    'squidlog.cache_code_error_tag_requests': {
        info: 'These tags are optional and describe some error conditions which occurred during response delivery.<br>' +
            '<ul>' +
            ' <li><code>ABORTED</code> the response was not completed due to the connection being aborted (usually by the client).</li>' +
            ' <li><code>TIMEOUT</code> the response was not completed due to a connection timeout.</li>' +
            ' <li><code>IGNORED</code> while refreshing a previously cached response A, Squid got a response B that was older than A (as determined by the Date header field). Squid ignored response B (and attempted to use A instead).</li>' +
            ' </ul>'
    },

    'squidlog.http_method_requests': {
        info: 'The request method to obtain an object. Please refer to section <a href="https://wiki.squid-cache.org/SquidFaq/SquidLogs#Request_methods">request-methods</a> for available methods and their description.'
    },

    'squidlog.hier_code_requests': {
        info: 'A code that explains how the request was handled, e.g. by forwarding it to a peer, or going straight to the source. ' +
            'Any hierarchy tag may be prefixed with <code>TIMEOUT_</code>, if the timeout occurs waiting for all ICP replies to return from the neighbours. The timeout is either dynamic, if the <a href="http://www.squid-cache.org/Doc/config/icp_query_timeout/" target="_blank">icp_query_timeout</a> was not set, or the time configured there has run up. ' +
            'Refer to <a href="https://wiki.squid-cache.org/SquidFaq/SquidLogs#Hierarchy_Codes" target="_blank">Hierarchy Codes</a> for details on hierarchy codes.'
    },

    'squidlog.server_address_forwarded_requests': {
        info: 'The IP address or hostname where the request (if a miss) was forwarded. For requests sent to origin servers, this is the origin server\'s IP address. ' +
            'For requests sent to a neighbor cache, this is the neighbor\'s hostname. NOTE: older versions of Squid would put the origin server hostname here.'
    },

    'squidlog.mime_type_requests': {
        info: 'The content type of the object as seen in the HTTP reply header. Please note that ICP exchanges usually don\'t have any content type.'
    },

    // ------------------------------------------------------------------------
    // CockroachDB

    'cockroachdb.process_cpu_time_combined_percentage': {
        info: 'Current combined cpu utilization, calculated as <code>(user+system)/num of logical cpus</code>.'
    },

    'cockroachdb.host_disk_bandwidth': {
        info: 'Summary disk bandwidth statistics across all system host disks.'
    },

    'cockroachdb.host_disk_operations': {
        info: 'Summary disk operations statistics across all system host disks.'
    },

    'cockroachdb.host_disk_iops_in_progress': {
        info: 'Summary disk iops in progress statistics across all system host disks.'
    },

    'cockroachdb.host_network_bandwidth': {
        info: 'Summary network bandwidth statistics across all system host network interfaces.'
    },

    'cockroachdb.host_network_packets': {
        info: 'Summary network packets statistics across all system host network interfaces.'
    },

    'cockroachdb.live_nodes': {
        info: 'Will be <code>0</code> if this node is not itself live.'
    },

    'cockroachdb.total_storage_capacity': {
        info: 'Entire disk capacity. It includes non-CR data, CR data, and empty space.'
    },

    'cockroachdb.storage_capacity_usability': {
        info: '<code>usable</code> is sum of empty space and CR data, <code>unusable</code> is space used by non-CR data.'
    },

    'cockroachdb.storage_usable_capacity': {
        info: 'Breakdown of <code>usable</code> space.'
    },

    'cockroachdb.storage_used_capacity_percentage': {
        info: '<code>total</code> is % of <b>total</b> space used, <code>usable</code> is % of <b>usable</b> space used.'
    },

    'cockroachdb.sql_bandwidth': {
        info: 'The total amount of SQL client network traffic.'
    },

    'cockroachdb.sql_errors': {
        info: '<code>statement</code> is statements resulting in a planning or runtime error, ' +
            '<code>transaction</code> is SQL transactions abort errors.'
    },

    'cockroachdb.sql_started_ddl_statements': {
        info: 'The amount of <b>started</b> DDL (Data Definition Language) statements. ' +
            'This type means database schema changes. ' +
            'It includes <code>CREATE</code>, <code>ALTER</code>, <code>DROP</code>, <code>RENAME</code>, <code>TRUNCATE</code> and <code>COMMENT</code> statements.'
    },

    'cockroachdb.sql_executed_ddl_statements': {
        info: 'The amount of <b>executed</b> DDL (Data Definition Language) statements. ' +
            'This type means database schema changes. ' +
            'It includes <code>CREATE</code>, <code>ALTER</code>, <code>DROP</code>, <code>RENAME</code>, <code>TRUNCATE</code> and <code>COMMENT</code> statements.'
    },

    'cockroachdb.sql_started_dml_statements': {
        info: 'The amount of <b>started</b> DML (Data Manipulation Language) statements.'
    },

    'cockroachdb.sql_executed_dml_statements': {
        info: 'The amount of <b>executed</b> DML (Data Manipulation Language) statements.'
    },

    'cockroachdb.sql_started_tcl_statements': {
        info: 'The amount of <b>started</b> TCL (Transaction Control Language) statements.'
    },

    'cockroachdb.sql_executed_tcl_statements': {
        info: 'The amount of <b>executed</b> TCL (Transaction Control Language) statements.'
    },

    'cockroachdb.live_bytes': {
        info: 'The amount of live data used by both applications and the CockroachDB system.'
    },

    'cockroachdb.kv_transactions': {
        info: 'KV transactions breakdown:<br>' +
            '<ul>' +
            ' <li><code>committed</code> committed KV transactions (including 1PC).</li>' +
            ' <li><code>fast-path_committed</code> KV transaction on-phase commit attempts.</li>' +
            ' <li><code>aborted</code> aborted KV transactions.</li>' +
            ' </ul>'
    },

    'cockroachdb.kv_transaction_restarts': {
        info: 'KV transactions restarts breakdown:<br>' +
            '<ul>' +
            ' <li><code>write too old</code> restarts due to a concurrent writer committing first.</li>' +
            ' <li><code>write too old (multiple)</code> restarts due to multiple concurrent writers committing first.</li>' +
            ' <li><code>forwarded timestamp (iso=serializable)</code> restarts due to a forwarded commit timestamp and isolation=SERIALIZABLE".</li>' +
            ' <li><code>possible replay</code> restarts due to possible replays of command batches at the storage layer.</li>' +
            ' <li><code>async consensus failure</code> restarts due to async consensus writes that failed to leave intents.</li>' +
            ' <li><code>read within uncertainty interval</code> restarts due to reading a new value within the uncertainty interval.</li>' +
            ' <li><code>aborted</code> restarts due to an abort by a concurrent transaction (usually due to deadlock).</li>' +
            ' <li><code>push failure</code> restarts due to a transaction push failure.</li>' +
            ' <li><code>unknown</code> restarts due to a unknown reasons.</li>' +
            ' </ul>'
    },

    'cockroachdb.ranges': {
        info: 'CockroachDB stores all user data (tables, indexes, etc.) and almost all system data in a giant sorted map of key-value pairs. ' +
            'This keyspace is divided into "ranges", contiguous chunks of the keyspace, so that every key can always be found in a single range.'
    },

    'cockroachdb.ranges_replication_problem': {
        info: 'Ranges with not optimal number of replicas:<br>' +
            '<ul>' +
            ' <li><code>unavailable</code> ranges with fewer live replicas than needed for quorum.</li>' +
            ' <li><code>under replicated</code> ranges with fewer live replicas than the replication target.</li>' +
            ' <li><code>over replicated</code> ranges with more live replicas than the replication target.</li>' +
            ' </ul>'
    },

    'cockroachdb.replicas': {
        info: 'CockroachDB replicates each range (3 times by default) and stores each replica on a different node.'
    },

    'cockroachdb.replicas_leaders': {
        info: 'For each range, one of the replicas is the <code>leader</code> for write requests, <code>not leaseholders</code> is the number of Raft leaders whose range lease is held by another store.'
    },

    'cockroachdb.replicas_leaseholders': {
        info: 'For each range, one of the replicas holds the "range lease". This replica, referred to as the <code>leaseholder</code>, is the one that receives and coordinates all read and write requests for the range.'
    },

    'cockroachdb.queue_processing_failures': {
        info: 'Failed replicas breakdown by queue:<br>' +
            '<ul>' +
            ' <li><code>gc</code> replicas which failed processing in the GC queue.</li>' +
            ' <li><code>replica gc</code> replicas which failed processing in the replica GC queue.</li>' +
            ' <li><code>replication</code> replicas which failed processing in the replicate queue.</li>' +
            ' <li><code>split</code> replicas which failed processing in the split queue.</li>' +
            ' <li><code>consistency</code> replicas which failed processing in the consistency checker queue.</li>' +
            ' <li><code>raft log</code> replicas which failed processing in the Raft log queue.</li>' +
            ' <li><code>raft snapshot</code> replicas which failed processing in the Raft repair queue.</li>' +
            ' <li><code>time series maintenance</code> replicas which failed processing in the time series maintenance queue.</li>' +
            ' </ul>'
    },

    'cockroachdb.rebalancing_queries': {
        info: 'Number of kv-level requests received per second by the store, averaged over a large time period as used in rebalancing decisions.'
    },

    'cockroachdb.rebalancing_writes': {
        info: 'Number of keys written (i.e. applied by raft) per second to the store, averaged over a large time period as used in rebalancing decisions.'
    },

    'cockroachdb.slow_requests': {
        info: 'Requests that have been stuck for a long time.'
    },

    'cockroachdb.timeseries_samples': {
        info: 'The amount of metric samples written to disk.'
    },

    'cockroachdb.timeseries_write_errors': {
        info: 'The amount of errors encountered while attempting to write metrics to disk.'
    },

    'cockroachdb.timeseries_write_bytes': {
        info: 'Size of metric samples written to disk.'
    },

    // ------------------------------------------------------------------------
    // Filesystem

    'filesystem.vfs_deleted_objects': {
        title : 'VFS remove',
        info: 'This chart does not show all events that remove files from the file system, because file systems can create their own functions to remove files, it shows calls for the function <code>vfs_unlink</code>. '
    },

    'filesystem.vfs_io': {
        title : 'VFS IO',
        info: 'Successful or failed calls to functions <code>vfs_read</code> and <code>vfs_write</code>. This chart may not show all file system events if it uses other functions to store data on disk.'
    },

    'filesystem.vfs_io_bytes': {
        title : 'VFS bytes written',
        info: 'Total of bytes read or written with success using the functions <code>vfs_read</code> and <code>vfs_write</code>.'
    },

    'filesystem.vfs_io_error': {
        title : 'VFS IO error',
        info: 'Failed calls to functions <code>vfs_read</code> and <code>vfs_write</code>.'
    },

    'filesystem.vfs_fsync': {
        info: 'Successful or failed calls to functions <code>vfs_fsync</code>.'
    },

    'filesystem.vfs_fsync_error': {
        info: 'Failed calls to functions <code>vfs_fsync</code>.'
    },

    'filesystem.vfs_open': {
        info: 'Successful or failed calls to functions <code>vfs_open</code>.'
    },

    'filesystem.vfs_open_error': {
        info: 'Failed calls to functions <code>vfs_open</code>.'
    },

    'filesystem.vfs_create': {
        info: 'Successful or failed calls to functions <code>vfs_create</code>.'
    },

    'filesystem.vfs_create_error': {
        info: 'Failed calls to functions <code>vfs_create</code>.'
    },

    'filesystem.ext4_read_latency': {
        info: 'Latency is the time it takes for an event to be completed. Netdata is attaching a kprobe for when the function <code>ext4_file_read_iter</code> is called and another for when it finishes the execution. We calculate the difference between the calling and return times, we get the logarithmic for the final result and we sum one value to the respective bin. Based on the eBPF <a href="http://www.brendangregg.com/blog/2016-10-06/linux-bcc-ext4dist-ext4slower.html" target="_blank">ext4dist</a> from BCC tools.'
    },

    'filesystem.ext4_write_latency': {
        info: 'Latency is the time it takes for an event to be completed. Netdata is attaching a kprobe for when the function <code>ext4_file_write_iter</code> is called and another for when it finishes the execution. We calculate the difference between the calling and return times, we get the logarithmic for the final result and we sum one value to the respective bin. Based on the eBPF <a href="http://www.brendangregg.com/blog/2016-10-06/linux-bcc-ext4dist-ext4slower.html" target="_blank">ext4dist</a> from BCC tools.'
    },

    'filesystem.ext4_open_latency': {
        info: 'Latency is the time it takes for an event to be completed. Netdata is attaching a kprobe for when the function <code>ext4_file_open</code> is called and another for when it finishes the execution. We calculate the difference between the calling and return times, we get the logarithmic for the final result and we sum one value to the respective bin. Based on the eBPF <a href="http://www.brendangregg.com/blog/2016-10-06/linux-bcc-ext4dist-ext4slower.html" target="_blank">ext4dist</a> from BCC tools.'
    },

    'filesystem.ext4_sync_latency': {
        info: 'Latency is the time it takes for an event to be completed. Netdata is attaching a kprobe for when the function <code>ext4_sync_file</code> is called and another for when it finishes the execution. We calculate the difference between the calling and return times, we get the logarithmic for the final result and we sum one value to the respective bin. Based on the eBPF <a href="http://www.brendangregg.com/blog/2016-10-06/linux-bcc-ext4dist-ext4slower.html" target="_blank">ext4dist</a> from BCC tools.'
    },

    'filesystem.xfs_read_latency': {
        info: 'Latency is the time it takes for an event to be completed. Netdata is attaching a kprobe for when the function <code>xfs_file_read_iter</code> is called and another for when it finishes the execution. We calculate the difference between the calling and return times, we get the logarithmic for the final result and we sum one value to the respective bin. Based on the eBPF <a href="https://github.com/iovisor/bcc/blob/master/tools/xfsdist_example.txt" target="_blank">xfsdist</a> from BCC tools.'
    },

    'filesystem.xfs_write_latency': {
        info: 'Latency is the time it takes for an event to be completed. Netdata is attaching a kprobe for when the function <code>xfs_file_write_iter</code> is called and another for when it finishes the execution. We calculate the difference between the calling and return times, we get the logarithmic for the final result and we sum one value to the respective bin. Based on the eBPF <a href="https://github.com/iovisor/bcc/blob/master/tools/xfsdist_example.txt" target="_blank">xfsdist</a> from BCC tools.'
    },

    'filesystem.xfs_open_latency': {
        info: 'Latency is the time it takes for an event to be completed. Netdata is attaching a kprobe for when the function <code>xfs_file_open</code> is called and another for when it finishes the execution. We calculate the difference between the calling and return times, we get the logarithmic for the final result and we sum one value to the respective bin. Based on the eBPF <a href="https://github.com/iovisor/bcc/blob/master/tools/xfsdist_example.txt" target="_blank">xfsdist</a> from BCC tools.'
    },

    'filesystem.xfs_sync_latency': {
        info: 'Latency is the time it takes for an event to be completed. Netdata is attaching a kprobe for when the function <code>xfs_file_sync</code> is called and another for when it finishes the execution. We calculate the difference between the calling and return times, we get the logarithmic for the final result and we sum one value to the respective bin. Based on the eBPF <a href="https://github.com/iovisor/bcc/blob/master/tools/xfsdist_example.txt" target="_blank">xfsdist</a> from BCC tools.'
    },

    'filesystem.nfs_read_latency': {
        info: 'Latency is the time it takes for an event to be completed. Netdata is attaching a kprobe for when the function <code>nfs_file_read</code> is called and another for when it finishes the execution. We calculate the difference between the calling and return times, we get the logarithmic for the final result and we sum one value to the respective bin. Based on the eBPF <a href="https://github.com/iovisor/bcc/blob/master/tools/nfsdist_example.txt" target="_blank">nfsdist</a> from BCC tools.'
    },

    'filesystem.nfs_write_latency': {
        info: 'Latency is the time it takes for an event to be completed. Netdata is attaching a kprobe for when the function <code>nfs_file_write</code> is called and another for when it finishes the execution. We calculate the difference between the calling and return times, we get the logarithmic for the final result and we sum one value to the respective bin. Based on the eBPF <a href="https://github.com/iovisor/bcc/blob/master/tools/nfsdist_example.txt" target="_blank">nfsdist</a> from BCC tools.'
    },

    'filesystem.nfs_open_latency': {
        info: 'Latency is the time it takes for an event to be completed. Netdata is attaching a kprobe for when functions <code>nfs_file_open</code> and <code>nfs4_file_open</code> are called and another for when they finish the execution. We calculate the difference between the calling and return times, we get the logarithmic for the final result and we sum one value to the respective bin. Based on the eBPF <a href="https://github.com/iovisor/bcc/blob/master/tools/nfsdist_example.txt" target="_blank">nfsdist</a> from BCC tools.'
    },

    'filesystem.nfs_attribute_latency': {
        info: 'Latency is the time it takes for an event to be completed. Netdata is attaching a kprobe for when the function <code>nfs_getattr</code> is called and another for when it finishes the execution. We calculate the difference between the calling and return times, we get the logarithmic for the final result and we sum one value to the respective bin. Based on the eBPF <a href="https://github.com/iovisor/bcc/blob/master/tools/nfsdist_example.txt" target="_blank">nfsdist</a> from BCC tools.'
    },

    'filesystem.zfs_read_latency': {
        info: 'Latency is the time it takes for an event to be completed. Netdata is attaching a kprobe for when the function <code>zpl_iter_read</code> is called and another for when it finishes the execution. We calculate the difference between the calling and return times, we get the logarithmic for the final result and we sum one value to the respective bin. Based on the eBPF <a href="https://github.com/iovisor/bcc/blob/master/tools/zfsdist_example.txt" target="_blank">zfsdist</a> from BCC tools.'
    },

    'filesystem.zfs_write_latency': {
        info: 'Latency is the time it takes for an event to be completed. Netdata is attaching a kprobe for when the function <code>zpl_iter_write</code> is called and another for when it finishes the execution. We calculate the difference between the calling and return times, we get the logarithmic for the final result and we sum one value to the respective bin. Based on the eBPF <a href="https://github.com/iovisor/bcc/blob/master/tools/zfsdist_example.txt" target="_blank">zfsdist</a> from BCC tools.'
    },

    'filesystem.zfs_open_latency': {
        info: 'Latency is the time it takes for an event to be completed. Netdata is attaching a kprobe for when the function <code>zpl_open</code> is called and another for when it finishes the execution. We calculate the difference between the calling and return times, we get the logarithmic for the final result and we sum one value to the respective bin. Based on the eBPF <a href="https://github.com/iovisor/bcc/blob/master/tools/zfsdist_example.txt" target="_blank">zfsdist</a> from BCC tools.'
    },

    'filesystem.zfs_sync_latency': {
        info: 'Latency is the time it takes for an event to be completed. Netdata is attaching a kprobe for when the function <code>zpl_fsync</code> is called and another for when it finishes the execution. We calculate the difference between the calling and return times, we get the logarithmic for the final result and we sum one value to the respective bin. Based on the eBPF <a href="https://github.com/iovisor/bcc/blob/master/tools/zfsdist_example.txt" target="_blank">zfsdist</a> from BCC tools.'
    },

    'filesystem.btrfs_read_latency': {
        info: 'Latency is the time it takes for an event to be completed. Netdata is attaching a kprobe for when the function <code>btrfs_file_read_iter</code> (kernel newer than 5.9.16) or the function <code>generic_file_read_iter</code> (old kernels) is called and another for when it finishes the execution. We calculate the difference between the calling and return times, we get the logarithmic for the final result and we sum one value to the respective bin. Based on the eBPF <a href="https://github.com/iovisor/bcc/blob/master/tools/btrfsdist_example.txt" target="_blank">btrfsdist</a> from BCC tools.'
    },

    'filesystem.btrfs_write_latency': {
        info: 'Latency is the time it takes for an event to be completed. Netdata is attaching a kprobe for when the function <code>btrfs_file_write_iter</code> is called and another for when it finishes the execution. We calculate the difference between the calling and return times, we get the logarithmic for the final result and we sum one value to the respective bin. Based on the eBPF <a href="https://github.com/iovisor/bcc/blob/master/tools/btrfsdist_example.txt" target="_blank">btrfsdist</a> from BCC tools.'
    },

    'filesystem.btrfs_open_latency': {
        info: 'Latency is the time it takes for an event to be completed. Netdata is attaching a kprobe for when the function <code>btrfs_file_open</code> is called and another for when it finishes the execution. We calculate the difference between the calling and return times, we get the logarithmic for the final result and we sum one value to the respective bin. Based on the eBPF <a href="https://github.com/iovisor/bcc/blob/master/tools/btrfsdist_example.txt" target="_blank">btrfsdist</a> from BCC tools.'
    },

    'filesystem.btrfs_sync_latency': {
        info: 'Latency is the time it takes for an event to be completed. Netdata is attaching a kprobe for when the function <code>btrfs_sync_file</code> is called and another for when it finishes the execution. We calculate the difference between the calling and return times, we get the logarithmic for the final result and we sum one value to the respective bin. Based on the eBPF <a href="https://github.com/iovisor/bcc/blob/master/tools/btrfsdist_example.txt" target="_blank">btrfsdist</a> from BCC tools.'
    },

    // ------------------------------------------------------------------------
    // eBPF

    'ebpf.tcp_functions': {
        title : 'TCP calls',
        info: 'Successful or failed calls to functions <code>tcp_sendmsg</code>, <code>tcp_cleanup_rbuf</code> and <code>tcp_close</code>.'
    },

    'ebpf.tcp_bandwidth': {
        title : 'TCP bandwidth',
        info: 'Bytes sent and received for functions <code>tcp_sendmsg</code> and <code>tcp_cleanup_rbuf</code>. We use <code>tcp_cleanup_rbuf</code> instead <code>tcp_recvmsg</code>, because this last misses <code>tcp_read_sock()</code> traffic and we would also need to have more probes to get the socket and package size.'
    },

    'ebpf.tcp_retransmit': {
        title : 'TCP retransmit',
        info: 'Number of packets retransmitted for function <code>tcp_retransmit_skb</code>.'
    },

    'ebpf.tcp_error': {
        title : 'TCP errors',
        info: 'Failed calls that to functions <code>tcp_sendmsg</code>, <code>tcp_cleanup_rbuf</code> and <code>tcp_close</code>.'
    },

    'ebpf.udp_functions': {
        title : 'UDP calls',
        info: 'Successful or failed calls to  functions <code>udp_sendmsg</code> and <code>udp_recvmsg</code>.'
    },

    'ebpf.udp_bandwidth': {
        title : 'UDP bandwidth',
        info: 'Bytes sent and received for functions <code>udp_sendmsg</code> and <code>udp_recvmsg</code>.'
    },

    'ebpf.file_descriptor': {
        title : 'File access',
        info: 'Calls for internal functions on Linux kernel. The open dimension is attached to the kernel internal function <code>do_sys_open</code> ( For kernels newer than <code>5.5.19</code> we add a kprobe to <code>do_sys_openat2</code>. ), which is the common function called from'+
            ' <a href="https://www.man7.org/linux/man-pages/man2/open.2.html" target="_blank">open(2)</a> ' +
            ' and <a href="https://www.man7.org/linux/man-pages/man2/openat.2.html" target="_blank">openat(2)</a>. ' +
            ' The close dimension is attached to the function <code>__close_fd</code> or <code>close_fd</code> according to your kernel version, which is called from system call' +
            ' <a href="https://www.man7.org/linux/man-pages/man2/close.2.html" target="_blank">close(2)</a>. '
    },

    'ebpf.file_error': {
        title : 'File access error',
        info: 'Failed calls to the kernel internal function <code>do_sys_open</code> ( For kernels newer than <code>5.5.19</code> we add a kprobe to <code>do_sys_openat2</code>. ), which is the common function called from'+
            ' <a href="https://www.man7.org/linux/man-pages/man2/open.2.html" target="_blank">open(2)</a> ' +
            ' and <a href="https://www.man7.org/linux/man-pages/man2/openat.2.html" target="_blank">openat(2)</a>. ' +
            ' The close dimension is attached to the function <code>__close_fd</code> or <code>close_fd</code> according to your kernel version, which is called from system call' +
            ' <a href="https://www.man7.org/linux/man-pages/man2/close.2.html" target="_blank">close(2)</a>. '
    },

    'ebpf.process_thread': {
        title : 'Task creation',
        info: 'Number of times that either <a href="https://www.ece.uic.edu/~yshi1/linux/lkse/node4.html#SECTION00421000000000000000" target="_blank">do_fork</a>, or <code>kernel_clone</code> if you are running kernel newer than 5.9.16, is called to create a new task, which is the common name used to define process and tasks inside the kernel. Netdata identifies the threads by counting the number of calls for <a href="https://linux.die.net/man/2/clone" target="_blank">sys_clone</a> that has the flag <code>CLONE_THREAD</code> set.'
    },

    'ebpf.exit': {
        title : 'Exit monitoring',
        info: 'Calls for the functions responsible for closing (<a href="https://www.informit.com/articles/article.aspx?p=370047&seqNum=4" target="_blank">do_exit</a>) and releasing (<a href="https://www.informit.com/articles/article.aspx?p=370047&seqNum=4" target="_blank">release_task</a>) tasks.'
    },

    'ebpf.task_error': {
        title : 'Task error',
        info: 'Number of errors to create a new process or thread.'
    },

    'ebpf.process_status': {
        title : 'Task status',
        info: 'Difference between the number of process created and the number of threads created per period(<code>process</code> dimension), it also shows the number of possible zombie process running on system.'
    },

    'apps.swap_read_call': {
        info: 'The function <code>swap_readpage</code> is called when the kernel reads a page from swap memory.'
    },

    'apps.swap_write_call': {
        info: 'The function <code>swap_writepage</code> is called when the kernel writes a page to swap memory.'
    },

    // ------------------------------------------------------------------------
    // ACLK Internal Stats
    'netdata.aclk_status': {
        valueRange: "[0, 1]",
        info: 'This chart shows if ACLK was online during entirety of the sample duration.'
    },

    'netdata.aclk_query_per_second': {
        info: 'This chart shows how many queries were added for ACLK_query thread to process and how many it was actually able to process.'
    },

    'netdata.aclk_latency_mqtt': {
        info: 'Measures latency between MQTT publish of the message and it\'s PUB_ACK being received'
    },

    // ------------------------------------------------------------------------
    // VerneMQ

    'vernemq.sockets': {
        mainheads: [
            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="open_sockets"'
                    + ' data-chart-library="gauge"'
                    + ' data-title="Connected Clients"'
                    + ' data-units="clients"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="16%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-colors="' + NETDATA.colors[4] + '"'
                    + ' data-decimal-digits="2"'
                    + ' role="application"></div>';
            }
        ]
    },
    'vernemq.queue_processes': {
        mainheads: [
            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="queue_processes"'
                    + ' data-chart-library="gauge"'
                    + ' data-title="Queues Processes"'
                    + ' data-units="processes"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="16%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-colors="' + NETDATA.colors[4] + '"'
                    + ' data-decimal-digits="2"'
                    + ' role="application"></div>';
            }
        ]
    },
    'vernemq.queue_messages_in_queues': {
        mainheads: [
            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="queue_messages_current"'
                    + ' data-chart-library="gauge"'
                    + ' data-title="Messages in the Queues"'
                    + ' data-units="messages"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="16%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-colors="' + NETDATA.colors[2] + '"'
                    + ' data-decimal-digits="2"'
                    + ' role="application"></div>';
            }
        ]
    },
    'vernemq.queue_messages': {
        mainheads: [
            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="queue_message_in"'
                    + ' data-chart-library="easypiechart"'
                    + ' data-title="MQTT Receive Rate"'
                    + ' data-units="messages/s"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="14%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-colors="' + NETDATA.colors[0] + '"'
                    + ' data-decimal-digits="2"'
                    + ' role="application"></div>';
            },
            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="queue_message_out"'
                    + ' data-chart-library="easypiechart"'
                    + ' data-title="MQTT Send Rate"'
                    + ' data-units="messages/s"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="14%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-colors="' + NETDATA.colors[1] + '"'
                    + ' data-decimal-digits="2"'
                    + ' role="application"></div>';
            },
        ]
    },
    'vernemq.average_scheduler_utilization': {
        mainheads: [
            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="system_utilization"'
                    + ' data-chart-library="gauge"'
                    + ' data-gauge-max-value="100"'
                    + ' data-title="Average Scheduler Utilization"'
                    + ' data-units="percentage"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="16%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-colors="' + NETDATA.colors[3] + '"'
                    + ' data-decimal-digits="2"'
                    + ' role="application"></div>';
            }
        ]
    },

    // ------------------------------------------------------------------------
    // Apache Pulsar
    'pulsar.messages_rate': {
        mainheads: [
            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="pulsar_rate_in"'
                    + ' data-chart-library="easypiechart"'
                    + ' data-title="Publish"'
                    + ' data-units="messages/s"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="12%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-colors="' + NETDATA.colors[0] + '"'
                    + ' data-decimal-digits="2"'
                    + ' role="application"></div>';
            },
            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="pulsar_rate_out"'
                    + ' data-chart-library="easypiechart"'
                    + ' data-title="Dispatch"'
                    + ' data-units="messages/s"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="12%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-colors="' + NETDATA.colors[1] + '"'
                    + ' data-decimal-digits="2"'
                    + ' role="application"></div>';
            },
        ]
    },
    'pulsar.subscription_msg_rate_redeliver': {
        mainheads: [
            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="pulsar_subscription_msg_rate_redeliver"'
                    + ' data-chart-library="gauge"'
                    + ' data-gauge-max-value="100"'
                    + ' data-title="Redelivered"'
                    + ' data-units="messages/s"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="14%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-colors="' + NETDATA.colors[3] + '"'
                    + ' data-decimal-digits="2"'
                    + ' role="application"></div>';
            }
        ]
    },
    'pulsar.subscription_blocked_on_unacked_messages': {
        mainheads: [
            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="pulsar_subscription_blocked_on_unacked_messages"'
                    + ' data-chart-library="gauge"'
                    + ' data-gauge-max-value="100"'
                    + ' data-title="Blocked On Unacked"'
                    + ' data-units="subscriptions"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="14%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-colors="' + NETDATA.colors[3] + '"'
                    + ' data-decimal-digits="2"'
                    + ' role="application"></div>';
            }
        ]
    },
    'pulsar.msg_backlog': {
        mainheads: [
            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="pulsar_msg_backlog"'
                    + ' data-chart-library="gauge"'
                    + ' data-gauge-max-value="100"'
                    + ' data-title="Messages Backlog"'
                    + ' data-units="messages"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="14%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-colors="' + NETDATA.colors[2] + '"'
                    + ' data-decimal-digits="2"'
                    + ' role="application"></div>';
            }
        ]
    },

    'pulsar.namespace_messages_rate': {
        heads: [
            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="publish"'
                    + ' data-chart-library="easypiechart"'
                    + ' data-title="Publish"'
                    + ' data-units="messages/s"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="12%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-colors="' + NETDATA.colors[0] + '"'
                    + ' data-decimal-digits="2"'
                    + ' role="application"></div>';
            },
            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="dispatch"'
                    + ' data-chart-library="easypiechart"'
                    + ' data-title="Dispatch"'
                    + ' data-units="messages/s"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="12%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-colors="' + NETDATA.colors[1] + '"'
                    + ' data-decimal-digits="2"'
                    + ' role="application"></div>';
            },
        ]
    },
    'pulsar.namespace_subscription_msg_rate_redeliver': {
        heads: [
            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="redelivered"'
                    + ' data-chart-library="gauge"'
                    + ' data-gauge-max-value="100"'
                    + ' data-title="Redelivered"'
                    + ' data-units="messages/s"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="14%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-colors="' + NETDATA.colors[3] + '"'
                    + ' data-decimal-digits="2"'
                    + ' role="application"></div>';
            }
        ]
    },
    'pulsar.namespace_subscription_blocked_on_unacked_messages': {
        heads: [
            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="blocked"'
                    + ' data-chart-library="gauge"'
                    + ' data-gauge-max-value="100"'
                    + ' data-title="Blocked On Unacked"'
                    + ' data-units="subscriptions"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="14%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-colors="' + NETDATA.colors[3] + '"'
                    + ' data-decimal-digits="2"'
                    + ' role="application"></div>';
            }
        ]
    },
    'pulsar.namespace_msg_backlog': {
        heads: [
            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="backlog"'
                    + ' data-chart-library="gauge"'
                    + ' data-gauge-max-value="100"'
                    + ' data-title="Messages Backlog"'
                    + ' data-units="messages"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="14%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-colors="' + NETDATA.colors[2] + '"'
                    + ' data-decimal-digits="2"'
                    + ' role="application"></div>';
            },
        ],
    },

    // ------------------------------------------------------------------------
    // Nvidia-smi

    'nvidia_smi.fan_speed': {
        heads: [
            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="speed"'
                    + ' data-chart-library="easypiechart"'
                    + ' data-title="Fan Speed"'
                    + ' data-units="percentage"'
                    + ' data-easypiechart-max-value="100"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="12%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-colors="' + NETDATA.colors[4] + '"'
                    + ' data-decimal-digits="2"'
                    + ' role="application"></div>';
            }
        ]
    },
    'nvidia_smi.temperature': {
        heads: [
            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="temp"'
                    + ' data-chart-library="easypiechart"'
                    + ' data-title="Temperature"'
                    + ' data-units="celsius"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="12%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-colors="' + NETDATA.colors[3] + '"'
                    + ' data-decimal-digits="2"'
                    + ' role="application"></div>';
            }
        ]
    },
    'nvidia_smi.memory_allocated': {
        heads: [
            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="used"'
                    + ' data-chart-library="easypiechart"'
                    + ' data-title="Used Memory"'
                    + ' data-units="MiB"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="12%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-colors="' + NETDATA.colors[4] + '"'
                    + ' data-decimal-digits="2"'
                    + ' role="application"></div>';
            }
        ]
    },
    'nvidia_smi.power': {
        heads: [
            function (os, id) {
                void (os);
                return '<div data-netdata="' + id + '"'
                    + ' data-dimensions="power"'
                    + ' data-chart-library="easypiechart"'
                    + ' data-title="Power Utilization"'
                    + ' data-units="watts"'
                    + ' data-gauge-adjust="width"'
                    + ' data-width="12%"'
                    + ' data-before="0"'
                    + ' data-after="-CHART_DURATION"'
                    + ' data-points="CHART_DURATION"'
                    + ' data-colors="' + NETDATA.colors[2] + '"'
                    + ' data-decimal-digits="2"'
                    + ' role="application"></div>';
            }
        ]
    },

    // ------------------------------------------------------------------------
    // Supervisor

    'supervisord.process_state_code': {
        info: '<a href="http://supervisord.org/subprocess.html#process-states" target="_blank">Process states map</a>: ' +
        '<code>0</code> - stopped, <code>10</code> - starting, <code>20</code> - running, <code>30</code> - backoff,' +
        '<code>40</code> - stopping, <code>100</code> - exited, <code>200</code> - fatal, <code>1000</code> - unknown.'
    },

    // ------------------------------------------------------------------------
    // Systemd units

    'systemd.service_units_state': {
        info: 'Service units start and control daemons and the processes they consist of. ' +
        'For details, see <a href="https://www.freedesktop.org/software/systemd/man/systemd.service.html#" target="_blank"> systemd.service(5)</a>'
    },

    'systemd.socket_unit_state': {
        info: 'Socket units encapsulate local IPC or network sockets in the system, useful for socket-based activation. ' +
        'For details about socket units, see <a href="https://www.freedesktop.org/software/systemd/man/systemd.socket.html#" target="_blank"> systemd.socket(5)</a>, ' +
        'for details on socket-based activation and other forms of activation, see <a href="https://www.freedesktop.org/software/systemd/man/daemon.html#" target="_blank"> daemon(7)</a>.'
    },

    'systemd.target_unit_state': {
        info: 'Target units are useful to group units, or provide well-known synchronization points during boot-up, ' +
        'see <a href="https://www.freedesktop.org/software/systemd/man/systemd.target.html#" target="_blank"> systemd.target(5)</a>.'
    },

    'systemd.path_unit_state': {
        info: 'Path units may be used to activate other services when file system objects change or are modified. ' +
        'See <a href="https://www.freedesktop.org/software/systemd/man/systemd.path.html#" target="_blank"> systemd.path(5)</a>.'
    },

    'systemd.device_unit_state': {
        info: 'Device units expose kernel devices in systemd and may be used to implement device-based activation. ' +
        'For details, see <a href="https://www.freedesktop.org/software/systemd/man/systemd.device.html#" target="_blank"> systemd.device(5)</a>.'
    },

    'systemd.mount_unit_state': {
        info: 'Mount units control mount points in the file system. ' +
        'For details, see <a href="https://www.freedesktop.org/software/systemd/man/systemd.mount.html#" target="_blank"> systemd.mount(5)</a>.'
    },

    'systemd.automount_unit_state': {
        info: 'Automount units provide automount capabilities, for on-demand mounting of file systems as well as parallelized boot-up. ' +
        'See <a href="https://www.freedesktop.org/software/systemd/man/systemd.automount.html#" target="_blank"> systemd.automount(5)</a>.'
    },

    'systemd.swap_unit_state': {
        info: 'Swap units are very similar to mount units and encapsulate memory swap partitions or files of the operating system. ' +
        'They are described in <a href="https://www.freedesktop.org/software/systemd/man/systemd.swap.html#" target="_blank"> systemd.swap(5)</a>.'
    },

    'systemd.timer_unit_state': {
        info: 'Timer units are useful for triggering activation of other units based on timers. ' +
        'You may find details in <a href="https://www.freedesktop.org/software/systemd/man/systemd.timer.html#" target="_blank"> systemd.timer(5)</a>.'
    },

    'systemd.scope_unit_state': {
        info: 'Slice units may be used to group units which manage system processes (such as service and scope units) ' +
        'in a hierarchical tree for resource management purposes. ' +
        'See <a href="https://www.freedesktop.org/software/systemd/man/systemd.scope.html#" target="_blank"> systemd.scope(5)</a>.'
    },

    'systemd.slice_unit_state': {
        info: 'Scope units are similar to service units, but manage foreign processes instead of starting them as well. ' +
        'See <a href="https://www.freedesktop.org/software/systemd/man/systemd.slice.html#" target="_blank"> systemd.slice(5)</a>.'
    },

};
