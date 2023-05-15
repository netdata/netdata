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
        info: 'Resources utilization of systemd services. '+
        'Netdata monitors all systemd services via '+
        '<a href="https://en.wikipedia.org/wiki/Cgroups" target="_blank">cgroups</a> ' +
        '(the resources accounting used by containers).'
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
        info: '<p>Performance <a href="https://www.kernel.org/doc/html/latest/networking/statistics.html" target="_blank">metrics for network interfaces</a>.</p>'+
        '<p>Netdata retrieves this data reading the <code>/proc/net/dev</code> file and <code>/sys/class/net/</code> directory.</p>'
    },

    'Infiniband': {
        title: 'Infiniband ports',
        icon: '<i class="fas fa-sitemap"></i>',
        info: '<p>Performance and exception statistics for '+
        '<a href="https://en.wikipedia.org/wiki/InfiniBand" target="_blank">Infiniband</a> ports. '+
        'The individual port and hardware counter descriptions can be found in the '+
        '<a href="https://community.mellanox.com/s/article/understanding-mlx5-linux-counters-and-status-parameters" target="_blank">Mellanox knowledge base</a>.'
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
                return 'Metrics for the networking stack of the system. These metrics are collected from <code>/proc/net/netstat</code> or attaching <code>kprobes</code> to kernel functions, apply to both IPv4 and IPv6 traffic and are related to operation of the kernel networking stack.';
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
        info: '<p><a href="https://en.wikipedia.org/wiki/Stream_Control_Transmission_Protocol" target="_blank">Stream Control Transmission Protocol (SCTP)</a> '+
        'is a computer network protocol which operates at the transport layer and serves a role similar to the popular '+
        'protocols TCP and UDP. SCTP provides some of the features of both UDP and TCP: it is message-oriented like UDP '+
        'and ensures reliable, in-sequence transport of messages with congestion control like TCP. '+
        'It differs from those protocols by providing multi-homing and redundant paths to increase resilience and reliability.</p>'+
        '<p>Netdata collects SCTP metrics reading the <code>/proc/net/sctp/snmp</code> file.</p>'
    },

    'ipvs': {
        title: 'IP Virtual Server',
        icon: '<i class="fas fa-eye"></i>',
        info: '<p><a href="http://www.linuxvirtualserver.org/software/ipvs.html" target="_blank">IPVS (IP Virtual Server)</a> '+
        'implements transport-layer load balancing inside the Linux kernel, so called Layer-4 switching. '+
        'IPVS running on a host acts as a load balancer at the front of a cluster of real servers, '+
        'it can direct requests for TCP/UDP based services to the real servers, '+
        'and makes services of the real servers to appear as a virtual service on a single IP address.</p>'+
        '<p>Netdata collects summary statistics, reading <code>/proc/net/ip_vs_stats</code>. '+
        'To display the statistics information of services and their servers, run <code>ipvsadm -Ln --stats</code> '+
        'or <code>ipvsadm -Ln --rate</code> for the rate statistics. '+
        'For details, see <a href="https://linux.die.net/man/8/ipvsadm" target="_blank">ipvsadm(8)</a>.</p>'
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

    'mount': {
        title: 'Mount Points',
        icon: '<i class="fas fa-hdd"></i>',
        info: ''
    },

    'mdstat': {
        title: 'MD arrays',
        icon: '<i class="fas fa-hdd"></i>',
        info: '<p>RAID devices are virtual devices created from two or more real block devices. '+
        '<a href="https://man7.org/linux/man-pages/man4/md.4.html" target="_blank">Linux Software RAID</a> devices are '+
        'implemented through the md (Multiple Devices) device driver.</p>'+
        '<p>Netdata monitors the current status of MD arrays reading <a href="https://raid.wiki.kernel.org/index.php/Mdstat" target="_blank">/proc/mdstat</a> and '+
        '<code>/sys/block/%s/md/mismatch_cnt</code> files.</p>'
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
        info: 'Performance metrics of the Network File Server. '+
        '<a href="https://en.wikipedia.org/wiki/Network_File_System" target="_blank">NFS</a> '+
        'is a distributed file system protocol, allowing a user on a client computer to access files over a network, '+
        'much like local storage is accessed. '+
        'NFS, like many other protocols, builds on the Open Network Computing Remote Procedure Call (ONC RPC) system.'
    },

    'nfs': {
        title: 'NFS Client',
        icon: '<i class="fas fa-folder-open"></i>',
        info: 'Performance metrics of the '+
        '<a href="https://en.wikipedia.org/wiki/Network_File_System" target="_blank">NFS</a> '+
        'operations of this system, acting as an NFS client.'
    },

    'zfs': {
        title: 'ZFS Cache',
        icon: '<i class="fas fa-folder-open"></i>',
        info: 'Performance metrics of the '+
        '<a href="https://en.wikipedia.org/wiki/ZFS#Caching_mechanisms" target="_blank">ZFS ARC and L2ARC</a>. '+
        'The following charts visualize all metrics reported by '+
        '<a href="https://github.com/openzfs/zfs/blob/master/cmd/arcstat/arcstat.in" target="_blank">arcstat.py</a> and '+
        '<a href="https://github.com/openzfs/zfs/blob/master/cmd/arc_summary/arc_summary3" target="_blank">arc_summary.py</a>.'
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
        info: 'Per application statistics are collected using '+
        '<a href="https://learn.netdata.cloud/docs/agent/collectors/apps.plugin" target="_blank">apps.plugin</a>. '+
        'This plugin walks through all processes and aggregates statistics for '+
        '<a href="https://learn.netdata.cloud/docs/agent/collectors/apps.plugin#configuration" target="_blank">application groups</a>. '+
        'The plugin also counts the resources of exited children. '+
        'So for processes like shell scripts, the reported values include the resources used by the commands '+
        'these scripts run within each timeframe.',
        height: 1.5
    },

    'groups': {
        title: 'User Groups',
        icon: '<i class="fas fa-user"></i>',
        info: 'Per user group statistics are collected using '+
        '<a href="https://learn.netdata.cloud/docs/agent/collectors/apps.plugin" target="_blank">apps.plugin</a>. '+
        'This plugin walks through all processes and aggregates statistics per user group. '+
        'The plugin also counts the resources of exited children. '+
        'So for processes like shell scripts, the reported values include the resources used by the commands '+
        'these scripts run within each timeframe.',
        height: 1.5
    },

    'users': {
        title: 'Users',
        icon: '<i class="fas fa-users"></i>',
        info: 'Per user statistics are collected using '+
        '<a href="https://learn.netdata.cloud/docs/agent/collectors/apps.plugin" target="_blank">apps.plugin</a>. '+
        'This plugin walks through all processes and aggregates statistics per user. '+
        'The plugin also counts the resources of exited children. '+
        'So for processes like shell scripts, the reported values include the resources used by the commands '+
        'these scripts run within each timeframe.',
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

    'docker': {
        title: 'Docker',
        icon: '<i class="fas fa-cube"></i>',
        info: 'Docker containers state and disk usage.'
    },

    'ping': {
        title: 'Ping',
        icon: '<i class="fas fa-exchange-alt"></i>',
        info: 'Measures round-trip time and packet loss by sending ping messages to network hosts.'
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

    'nvme': {
        title: 'NVMe',
        icon: '<i class="fas fa-hdd"></i>',
        info: 'NVMe devices SMART and health metrics. Additional information on metrics can be found in the <a href="https://nvmexpress.org/developers/nvme-specification/" target="_blank">NVM Express Base Specification</a>.'
    },

    'postgres': {
        title: 'PostgreSQL',
        icon: '<i class="fas fa-database"></i>',
        info: 'Performance metrics for <b>PostgreSQL</b>, the open source object-relational database management system (ORDBMS).'
    },

    'proxysql': {
        title: 'ProxySQL',
        icon: '<i class="fas fa-database"></i>',
        info: 'Performance metrics for <b>ProxySQL</b>, a high-performance open-source MySQL proxy.'
    },

    'pgbouncer': {
        title: 'PgBouncer',
        icon: '<i class="fas fa-exchange-alt"></i>',
        info: 'Performance metrics for PgBouncer, an open source connection pooler for PostgreSQL.'
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

    'nginxplus': {
        title: 'Nginx Plus',
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
        info: 'Information extracted from a server log file. <code>web_log</code> plugin incrementally parses the server log file to provide, in real-time, a break down of key server performance metrics. For web servers, an extended log file format may optionally be used (for <code>nginx</code> and <code>apache</code>) offering timing information and bandwidth for both requests and responses. <code>web_log</code> plugin may also be configured to provide a break down of requests per URL pattern (check <a href="https://github.com/netdata/go.d.plugin/blob/master/config/go.d/web_log.conf" target="_blank"><code>/etc/netdata/go.d/web_log.conf</code></a>).'
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
        title: 'Chrony',
        icon: '<i class="fas fa-clock"></i>',
        info: 'The system’s clock performance and peers activity status.'
    },

    'couchdb': {
        icon: '<i class="fas fa-database"></i>',
        info: 'Performance metrics for <b><a href="https://couchdb.apache.org/" target="_blank">CouchDB</a></b>, the open-source, JSON document-based database with an HTTP API and multi-master replication.'
    },

    'beanstalk': {
        title: 'Beanstalkd',
        icon: '<i class="fas fa-tasks"></i>',
        info: 'Provides statistics on the <b><a href="http://kr.github.io/beanstalkd/" target="_blank">beanstalkd</a></b> server and any tubes available on that server using data pulled from beanstalkc'
    },

    'rabbitmq': {
        title: 'RabbitMQ',
        icon: '<i class="fas fa-comments"></i>',
        info: 'Performance data for the <b><a href="https://www.rabbitmq.com/" target="_blank">RabbitMQ</a></b> open-source message broker.'
    },

    'ceph': {
        title: 'Ceph',
        icon: '<i class="fas fa-database"></i>',
        info: 'Provides statistics on the <b><a href="http://ceph.com/" target="_blank">ceph</a></b> cluster server, the open-source distributed storage system.'
    },

    'ntpd': {
        title: 'ntpd',
        icon: '<i class="fas fa-clock"></i>',
        info: 'Provides statistics for the internal variables of the Network Time Protocol daemon <b><a href="http://www.ntp.org/" target="_blank">ntpd</a></b> and optional including the configured peers (if enabled in the module configuration). The module presents the performance metrics as shown by <b><a href="http://doc.ntp.org/current-stable/ntpq.html">ntpq</a></b> (the standard NTP query program) using NTP mode 6 UDP packets to communicate with the NTP server.'
    },

    'spigotmc': {
        title: 'Spigot MC',
        icon: '<i class="fas fa-eye"></i>',
        info: 'Provides basic performance statistics for the <b><a href="https://www.spigotmc.org/" target="_blank">Spigot Minecraft</a></b> server.'
    },

    'unbound': {
        title: 'Unbound',
        icon: '<i class="fas fa-tag"></i>',
        info: undefined
    },

    'boinc': {
        title: 'BOINC',
        icon: '<i class="fas fa-microchip"></i>',
        info: 'Provides task counts for <b><a href="http://boinc.berkeley.edu/" target="_blank">BOINC</a></b> distributed computing clients.'
    },

    'w1sensor': {
        title: '1-Wire Sensors',
        icon: '<i class="fas fa-thermometer-half"></i>',
        info: 'Data derived from <a href="https://en.wikipedia.org/wiki/1-Wire" target="_blank">1-Wire</a> sensors.  Currently temperature sensors are automatically detected.'
    },

    'logind': {
        title: 'Logind',
        icon: '<i class="fas fa-user"></i>',
        info: 'Keeps track of user logins and sessions by querying the <a href="https://www.freedesktop.org/software/systemd/man/org.freedesktop.login1.html" target="_blank">systemd-logind API</a>.'
    },

    'powersupply': {
        title: 'Power Supply',
        icon: '<i class="fas fa-battery-half"></i>',
        info: 'Statistics for the various system power supplies. Data collected from <a href="https://www.kernel.org/doc/Documentation/power/power_supply_class.txt" target="_blank">Linux power supply class</a>.'
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

    'windows': {
        title: 'Windows',
        icon: '<i class="fab fa-windows"></i>',
        info: undefined
    },

    'iis': {
        title: 'IIS',
        icon: '<i class="fas fa-eye"></i>',
        info: undefined
    },

    'mssql': {
        title: 'MS SQL Server',
        icon: '<i class="fas fa-database"></i>',
        info: undefined
    },

    'ad': {
        title: 'AD Domain Service',
        icon: '<i class="fab fa-windows"></i>',
        info: undefined
    },

    'adcs': {
        title: 'AD Certification Service',
        icon: '<i class="fab fa-windows"></i>',
        info: undefined
    },

    'adfs': {
        title: 'AD Federation Service',
        icon: '<i class="fab fa-windows"></i>',
        info: undefined
    },

    'netframework': {
        title: '.NET Framework',
        icon: '<i class="fas fa-laptop-code"></i>',
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
        info: 'Performance statistics for ESXI hosts and virtual machines. Data collected from <a href="https://www.vmware.com/products/vcenter-server.html" target="_blank">VMware vCenter Server</a> using <code><a href="https://github.com/vmware/govmomi"> govmomi</a></code>  library.'
    },

    'vcsa': {
        title: 'VCSA',
        icon: '<i class="fas fa-server"></i>',
        info: 'vCenter Server Appliance health statistics. Data collected from <a href="https://vmware.github.io/vsphere-automation-sdk-rest/vsphere/index.html#SVC_com.vmware.appliance.health" target="_blank">Health API</a>.'
    },

    'zookeeper': {
        title: 'Zookeeper',
        icon: '<i class="fas fa-database"></i>',
        info: 'Provides health statistics for <b><a href="https://zookeeper.apache.org/" target="_blank">Zookeeper</a></b> server. Data collected through the command port using <code><a href="https://zookeeper.apache.org/doc/r3.5.5/zookeeperAdmin.html#sc_zkCommands">mntr</a></code> command.'
    },

    'hdfs': {
        title: 'HDFS',
        icon: '<i class="fas fa-folder-open"></i>',
        info: 'Provides <b><a href="https://hadoop.apache.org/docs/r3.2.0/hadoop-project-dist/hadoop-hdfs/HdfsDesign.html" target="_blank">Hadoop Distributed File System</a></b> performance statistics. Module collects metrics over <code>Java Management Extensions</code> through the web interface of an <code>HDFS</code> daemon.'
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
        info: 'Number of filesystem events for <a href="#menu_filesystem_submenu_vfs">Virtual File System</a>, <a href="#menu_filesystem_submenu_file_access">File Access</a>, <a href="#menu_filesystem_submenu_directory_cache__eBPF_">Directory cache</a>, and file system latency (<a href="#menu_filesystem_submenu_btrfs_latency">BTRFS</a>, <a href="#menu_filesystem_submenu_ext4_latency">EXT4</a>, <a href="#menu_filesystem_submenu_nfs_latency">NFS</a>, <a href="#menu_filesystem_submenu_xfs_latency">XFS</a>, and <a href="#menu_filesystem_submenu_xfs_latency">ZFS</a>) when your disk has the file system. Filesystem charts have relationship with <a href="#menu_system_submenu_swap">SWAP</a>, <a href="#menu_disk">Disk</a>, <a href="#menu_mem_submenu_synchronization__eBPF_">Sync</a>, and <a href="#menu_mount">Mount Points</a>.'
    },

    'vernemq': {
        title: 'VerneMQ',
        icon: '<i class="fas fa-comments"></i>',
        info: 'Performance data for the <b><a href="https://vernemq.com/" target="_blank">VerneMQ</a></b> open-source MQTT broker.'
    },

    'pulsar': {
        title: 'Pulsar',
        icon: '<i class="fas fa-comments"></i>',
        info: 'Summary, namespaces and topics performance data for the <b><a href="http://pulsar.apache.org/" target="_blank">Apache Pulsar</a></b> pub-sub messaging system.'
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
        info:'StatsD is an industry-standard technology stack for monitoring applications and instrumenting any piece of software to deliver custom metrics. Netdata allows the user to organize the metrics in different charts and visualize any application metric easily. Read more on <a href="https://learn.netdata.cloud/docs/agent/collectors/statsd.plugin" target="_blank">Netdata Learn</a>.'
    },

    'supervisord': {
        title: 'Supervisord',
        icon: '<i class="fas fa-tasks"></i>',
        info: 'Detailed statistics for each group of processes controlled by <b><a href="http://supervisord.org/" target="_blank">Supervisor</a></b>. ' +
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
        'For details, see <a href="https://www.freedesktop.org/software/systemd/man/systemd.html" target="_blank"> systemd(1)</a>.'
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

    'anomaly_detection': {
        title: 'Anomaly Detection',
        icon: '<i class="fas fa-brain"></i>',
        info: 'Charts relating to anomaly detection, increased <code>anomalous</code> dimensions or a higher than usual <code>anomaly_rate</code> could be signs of some abnormal behaviour. Read our <a href="https://learn.netdata.cloud/guides/monitor/anomaly-detection" target="_blank">anomaly detection guide</a> for more details.'
    },

    'fail2ban': {
        title: 'Fail2ban',
        icon: '<i class="fas fa-shield-alt"></i>',
        info: 'Netdata keeps track of the current jail status by reading the Fail2ban log file.'
    },

    'wireguard': {
        title: 'WireGuard',
        icon: '<i class="fas fa-dragon"></i>',
        info: 'VPN network interfaces and peers traffic.'
    },

    'pandas': {
        icon: '<i class="fas fa-teddy-bear"></i>'
    },

    'cassandra': {
        title: 'Cassandra',
        icon: '<i class="fas fa-database"></i>',
        info: 'Performance metrics for Cassandra, the open source distributed NoSQL database management system'
    },

    'consul': {
        title: 'Consul',
        icon: '<i class="fas fa-circle-notch"></i>',
        info: 'Consul performance and health metrics. For details, see <a href="https://developer.hashicorp.com/consul/docs/agent/telemetry#key-metrics" target="_blank">Key Metrics</a>.'
    }
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
        info: 'Number of requests for each <code>URL pattern</code> defined in <a href="https://github.com/netdata/go.d.plugin/blob/master/config/go.d/web_log.conf" target="_blank"><code>/etc/netdata/go.d/web_log.conf</code></a>. This chart counts all requests matching the URL patterns defined, independently of the web server response codes (i.e. both successful and unsuccessful).'
    },

    'web_log.clients': {
        info: 'Charts showing the number of unique client IPs, accessing the web server.'
    },

    'web_log.timings': {
        info: 'Web server response timings - the time the web server needed to prepare and respond to requests. This requires an extended log format and its meaning is web server specific. For most web servers this accounts the time from the reception of a complete request, to the dispatch of the last byte of the response. So, it includes the network delays of responses, but it does not include the network delays of requests.'
    },

    'mem.ksm': {
        title: 'deduper (ksm)',
        info: '<a href="https://en.wikipedia.org/wiki/Kernel_same-page_merging" target="_blank">Kernel Same-page Merging</a> '+
        '(KSM) performance monitoring, read from several files in <code>/sys/kernel/mm/ksm/</code>. '+
        'KSM is a memory-saving de-duplication feature in the Linux kernel. '+
        'The KSM daemon ksmd periodically scans those areas of user memory which have been registered with it, '+
        'looking for pages of identical content which can be replaced by a single write-protected page.'
    },

    'mem.hugepages': {
        info: 'Hugepages is a feature that allows the kernel to utilize the multiple page size capabilities of modern hardware architectures. The kernel creates multiple pages of virtual memory, mapped from both physical RAM and swap. There is a mechanism in the CPU architecture called "Translation Lookaside Buffers" (TLB) to manage the mapping of virtual memory pages to actual physical memory addresses. The TLB is a limited hardware resource, so utilizing a large amount of physical memory with the default page size consumes the TLB and adds processing overhead. By utilizing Huge Pages, the kernel is able to create pages of much larger sizes, each page consuming a single resource in the TLB. Huge Pages are pinned to physical RAM and cannot be swapped/paged out.'
    },

    'mem.numa': {
        info: 'Non-Uniform Memory Access (NUMA) is a hierarchical memory design the memory access time is dependent on locality. Under NUMA, a processor can access its own local memory faster than non-local memory (memory local to another processor or memory shared between processors). The individual metrics are described in the <a href="https://www.kernel.org/doc/Documentation/numastat.txt" target="_blank">Linux kernel documentation</a>.'
    },

    'mem.ecc': {
        info: '<p><a href="https://en.wikipedia.org/wiki/ECC_memory" target="_blank">ECC memory</a> '+
        'is a type of computer data storage that uses an error correction code (ECC) to detect '+
        'and correct n-bit data corruption which occurs in memory. '+
        'Typically, ECC memory maintains a memory system immune to single-bit errors: '+
        'the data that is read from each word is always the same as the data that had been written to it, '+
        'even if one of the bits actually stored has been flipped to the wrong state.</p>'+
        '<p>Memory errors can be classified into two types: '+
        '<b>Soft errors</b>, which randomly corrupt bits but do not leave physical damage. '+
        'Soft errors are transient in nature and are not repeatable, can be because of electrical or '+
        'magnetic interference. '+
        '<b>Hard errors</b>, which corrupt bits in a repeatable manner because '+
        'of a physical/hardware defect or an environmental problem.'
    },

    'mem.pagetype': {
        info: 'Statistics of free memory available from '+
        '<a href="https://en.wikipedia.org/wiki/Buddy_memory_allocation" target="_blank">memory buddy allocator</a>. '+
        'The buddy allocator is the system memory allocator. '+
        'The whole memory space is split in physical pages, which are grouped by '+
        'NUMA node, zone, '+
        '<a href="https://lwn.net/Articles/224254/" target="_blank">migrate type</a>, and size of the block. '+
        'By keeping pages grouped based on their ability to move, '+
        'the kernel can reclaim pages within a page block to satisfy a high-order allocation. '+
        'When the kernel or an application requests some memory, the buddy allocator provides a page that matches closest the request.'
    },

    'mem.fragmentation': {
        info: 'These charts show whether the kernel will compact memory or direct reclaim to satisfy a high-order allocation. '+
            'The extfrag/extfrag_index file in debugfs shows what the fragmentation index for each order is in each zone in the system.' +
            'Values tending towards 0 imply allocations would fail due to lack of memory, values towards 1000 imply failures are due to ' +
            'fragmentation and -1 implies that the allocation will succeed as long as watermarks are met.'
    },

    'system.zswap': {
        info : 'Zswap is a backend for frontswap that takes pages that are in the process of being swapped out and attempts to compress and store them in a ' +
            'RAM-based memory pool.  This can result in a significant I/O reduction on the swap device and, in the case where decompressing from RAM is faster ' +
            'than reading from the swap device, can also improve workload performance.'
    },

    'ip.ecn': {
        info: '<a href="https://en.wikipedia.org/wiki/Explicit_Congestion_Notification" target="_blank">Explicit Congestion Notification (ECN)</a> '+
        'is an extension to the IP and to the TCP that allows end-to-end notification of network congestion without dropping packets. '+
        'ECN is an optional feature that may be used between two ECN-enabled endpoints when '+
        'the underlying network infrastructure also supports it.'
    },

    'ip.multicast': {
        info: '<a href="https://en.wikipedia.org/wiki/Multicast" target="_blank">IP multicast</a> is a technique for '+
        'one-to-many communication over an IP network. '+
        'Multicast uses network infrastructure efficiently by requiring the source to send a packet only once, '+
        'even if it needs to be delivered to a large number of receivers. '+
        'The nodes in the network take care of replicating the packet to reach multiple receivers only when necessary.'
    },
    'ip.broadcast': {
        info: 'In computer networking, '+
        '<a href="https://en.wikipedia.org/wiki/Broadcasting_(networking)" target="_blank">broadcasting</a> refers to transmitting a packet that will be received by every device on the network. '+
        'In practice, the scope of the broadcast is limited to a broadcast domain.'
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
        info: 'DDoS protection performance metrics. <a href="https://github.com/firehol/firehol/wiki/Working-with-SYNPROXY" target="_blank">SYNPROXY</a> '+
        'is a TCP SYN packets proxy. '+
        'It is used to protect any TCP server (like a web server) from SYN floods and similar DDoS attacks. '+
        'SYNPROXY intercepts new TCP connections and handles the initial 3-way handshake using syncookies '+
        'instead of conntrack to establish the connection. '+
        'It is optimized to handle millions of packets per second utilizing all CPUs available without '+
        'any concurrency locking between the connections. '+
        'It can be used for any kind of TCP traffic (even encrypted), '+
        'since it does not interfere with the content itself.'
    },

    'ipfw.dynamic_rules': {
        title: 'dynamic rules',
        info: 'Number of dynamic rules, created by correspondent stateful firewall rules.'
    },

    'system.softnet_stat': {
        title: 'softnet',
        info: function (os) {
            if (os === 'linux')
                return '<p>Statistics for CPUs SoftIRQs related to network receive work. '+
                'Break down per CPU core can be found at <a href="#menu_cpu_submenu_softnet_stat">CPU / softnet statistics</a>. '+
                'More information about identifying and troubleshooting network driver related issues can be found at '+
                '<a href="https://access.redhat.com/sites/default/files/attachments/20150325_network_performance_tuning.pdf" target="_blank">Red Hat Enterprise Linux Network Performance Tuning Guide</a>.</p>'+
                '<p><b>Processed</b> - packets processed. '+
                '<b>Dropped</b> - packets dropped because the network device backlog was full. '+
                '<b>Squeezed</b> - number of times the network device budget was consumed or the time limit was reached, '+
                'but more work was available. '+
                '<b>ReceivedRPS</b> - number of times this CPU has been woken up to process packets via an Inter-processor Interrupt. '+
                '<b>FlowLimitCount</b> - number of times the flow limit has been reached (flow limiting is an optional '+
                'Receive Packet Steering feature).</p>';
            else
                return 'Statistics for CPUs SoftIRQs related to network receive work.';
        }
    },

    'system.clock synchronization': {
        info: '<a href="https://en.wikipedia.org/wiki/Network_Time_Protocol" target="_blank">NTP</a> '+
        'lets you automatically sync your system time with a remote server. '+
        'This keeps your machine’s time accurate by syncing with servers that are known to have accurate times.'
    },

    'cpu.softnet_stat': {
        title: 'softnet',
        info: function (os) {
            if (os === 'linux')
                return '<p>Statistics for CPUs SoftIRQs related to network receive work. '+
                'Total for all CPU cores can be found at <a href="#menu_system_submenu_softnet_stat">System / softnet statistics</a>. '+
                'More information about identifying and troubleshooting network driver related issues can be found at '+
                '<a href="https://access.redhat.com/sites/default/files/attachments/20150325_network_performance_tuning.pdf" target="_blank">Red Hat Enterprise Linux Network Performance Tuning Guide</a>.</p>'+
                '<p><b>Processed</b> - packets processed. '+
                '<b>Dropped</b> - packets dropped because the network device backlog was full. '+
                '<b>Squeezed</b> - number of times the network device budget was consumed or the time limit was reached, '+
                'but more work was available. '+
                '<b>ReceivedRPS</b> - number of times this CPU has been woken up to process packets via an Inter-processor Interrupt. '+
                '<b>FlowLimitCount</b> - number of times the flow limit has been reached (flow limiting is an optional '+
                'Receive Packet Steering feature).</p>';
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
        info: 'Statistics per database. This includes <a href="http://docs.couchdb.org/en/latest/api/database/common.html#get--db" target="_blank">3 size graphs per database</a>: active (the size of live data in the database), external (the uncompressed size of the database contents), and file (the size of the file on disk, exclusive of any views and indexes). It also includes the number of documents and number of deleted documents per database.'
    },

    'couchdb.erlang': {
        title: 'erlang statistics',
        info: 'Detailed information about the status of the Erlang VM that hosts CouchDB. These are intended for advanced users only. High values of the peak message queue (>10e6) generally indicate an overload condition.'
    },

    'ntpd.system': {
        title: 'system',
        info: 'Statistics of the system variables as shown by the readlist billboard <code>ntpq -c rl</code>. System variables are assigned an association ID of zero and can also be shown in the readvar billboard <code>ntpq -c "rv 0"</code>. These variables are used in the <a href="http://doc.ntp.org/current-stable/discipline.html" target="_blank">Clock Discipline Algorithm</a>, to calculate the lowest and most stable offset.'
    },

    'ntpd.peers': {
        title: 'peers',
        info: 'Statistics of the peer variables for each peer configured in <code>/etc/ntp.conf</code> as shown by the readvar billboard <code>ntpq -c "rv &lt;association&gt;"</code>, while each peer is assigned a nonzero association ID as shown by <code>ntpq -c "apeers"</code>. The module periodically scans for new/changed peers (default: every 60s). <b>ntpd</b> selects the best possible peer from the available peers to synchronize the clock. A minimum of at least 3 peers is required to properly identify the best possible peer.'
    },

    'mem.page_cache': {
        title: 'page cache (eBPF)',
        info: 'Number of calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#memory" target="_blank">functions</a> used to manipulate the <a href="https://en.wikipedia.org/wiki/Page_cache" target="_blank">Linux page cache</a>. This chart has a relationship with <a href="#menu_filesystem">File Systems</a>, <a href="#menu_mem_submenu_synchronization__eBPF_">Sync</a>, and <a href="#menu_disk">Hard Disk</a>.'
    },

    'apps.page_cache': {
        title: 'page cache (eBPF)',
        info: 'Netdata also gives a summary for these charts in <a href="#menu_mem_submenu_page_cache">Memory submenu</a>.'
    },

    'filesystem.vfs': {
        title: 'vfs (eBPF)',
        info: 'Number of calls to Virtual File System <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">functions</a> used to manipulate <a href="#menu_filesystem">File Systems</a>.'
    },

    'apps.vfs': {
        title: 'vfs (eBPF)',
        info: 'Netdata also gives a summary for these charts in <a href="#menu_filesystem_submenu_vfs">Filesystem submenu</a>.'
    },

    'filesystem.ext4_latency': {
        title: 'ext4 latency (eBPF)',
        info: '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#latency-algorithm" target="_blank">Latency</a> is the time it takes for an event to be completed. Based on the <a href="http://www.brendangregg.com/blog/2016-10-06/linux-bcc-ext4dist-ext4slower.html" target="_blank">eBPF ext4dist</a> from BCC tools. This chart is provided by the <a href="#menu_netdata_submenu_ebpf">eBPF plugin</a> to monitor <a href="#menu_filesystem">File systems</a>.'
    },

    'filesystem.xfs_latency': {
        title: 'xfs latency (eBPF)',
        info: '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#latency-algorithm" target="_blank">Latency</a> is the time it takes for an event to be completed. Based on the <a href="https://github.com/iovisor/bcc/blob/master/tools/xfsdist_example.txt" target="_blank">xfsdist</a> from BCC tools. This chart is provided by the <a href="#menu_netdata_submenu_ebpf">eBPF plugin</a> to monitor <a href="#menu_filesystem">File systems</a>.'
    },

    'filesystem.nfs_latency': {
        title: 'nfs latency (eBPF)',
        info: '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#latency-algorithm" target="_blank">Latency</a> is the time it takes for an event to be completed. Based on the <a href="https://github.com/iovisor/bcc/blob/master/tools/nfsdist_example.txt" target="_blank">nfsdist</a> from BCC tools. This chart is provided by the <a href="#menu_netdata_submenu_ebpf">eBPF plugin</a> to monitor <a href="#menu_filesystem">File systems</a>.'
    },

    'filesystem.zfs_latency': {
        title: 'zfs latency (eBPF)',
        info: '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#latency-algorithm" target="_blank">Latency</a> is the time it takes for an event to be completed. Based on the <a href="https://github.com/iovisor/bcc/blob/master/tools/zfsdist_example.txt" target="_blank">zfsdist</a> from BCC tools. This chart is provided by the <a href="#menu_netdata_submenu_ebpf">eBPF plugin</a> to monitor <a href="#menu_filesystem">File systems</a>.'
    },

    'filesystem.btrfs_latency': {
        title: 'btrfs latency (eBPF)',
        info: '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#latency-algorithm" target="_blank">Latency</a> is the time it takes for an event to be completed. Based on the <a href="https://github.com/iovisor/bcc/blob/master/tools/btrfsdist_example.txt" target="_blank">btrfsdist</a> from BCC tools. This chart is provided by the <a href="#menu_netdata_submenu_ebpf">eBPF plugin</a> to monitor <a href="#menu_filesystem">File systems</a>.'
    },

    'filesystem.file_access': {
        title: 'file access (eBPF)',
    },

    'apps.file_access': {
        title: 'file access (eBPF)',
        info: 'Netdata also gives a summary for this chart on <a href="#menu_filesystem_submenu_file_access">Filesystem submenu</a> (more details on <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#file-descriptor" target="_blank">eBPF plugin file chart section</a>).'
    },

    'ip.kernel': {
        title: 'kernel functions (eBPF)',
    },

    'apps.net': {
        title: 'network',
        info: 'Netdata also gives a summary for eBPF charts in <a href="#menu_ip_submenu_kernel">Networking Stack submenu</a>.'
    },

    'system.ipc semaphores': {
        info: 'System V semaphores is an inter-process communication (IPC) mechanism. '+
        'It allows processes or threads within a process to synchronize their actions. '+
        'They are often used to monitor and control the availability of system resources such as shared memory segments. ' +
        'For details, see <a href="https://man7.org/linux/man-pages/man7/svipc.7.html" target="_blank">svipc(7)</a>. ' +
        'To see the host IPC semaphore information, run <code>ipcs -us</code>. For limits, run <code>ipcs -ls</code>.'
    },

    'system.ipc shared memory': {
        info: 'System V shared memory is an inter-process communication (IPC) mechanism. '+
        'It allows processes to communicate information by sharing a region of memory. '+
        'It is the fastest form of inter-process communication available since no kernel involvement occurs when data is passed between the processes (no copying). '+
        'Typically, processes must synchronize their access to a shared memory object, using, for example, POSIX semaphores. '+
        'For details, see <a href="https://man7.org/linux/man-pages/man7/svipc.7.html" target="_blank">svipc(7)</a>. '+
        'To see the host IPC shared memory information, run <code>ipcs -um</code>. For limits, run <code>ipcs -lm</code>.'
    },

    'system.ipc message queues': {
        info: 'System V message queues is an inter-process communication (IPC) mechanism. '+
        'It allow processes to exchange data in the form of messages. '+
        'For details, see <a href="https://man7.org/linux/man-pages/man7/svipc.7.html" target="_blank">svipc(7)</a>. ' +
        'To see the host IPC messages information, run <code>ipcs -uq</code>. For limits, run <code>ipcs -lq</code>.'
    },

    'system.interrupts': {
        info: '<a href="https://en.wikipedia.org/wiki/Interrupt" target="_blank"><b>Interrupts</b></a> are signals '+
        'sent to the CPU by external devices (normally I/O devices) or programs (running processes). '+
        'They tell the CPU to stop its current activities and execute the appropriate part of the operating system. '+
        'Interrupt types are '+
        '<b>hardware</b> (generated by hardware devices to signal that they need some attention from the OS), '+
        '<b>software</b> (generated by programs when they want to request a system call to be performed by the operating system), and '+
        '<b>traps</b> (generated by the CPU itself to indicate that some error or condition occurred for which assistance from the operating system is needed).'
    },

    'system.softirqs': {
        info: 'Software interrupts (or "softirqs") are one of the oldest deferred-execution mechanisms in the kernel. '+
        'Several tasks among those executed by the kernel are not critical: '+
        'they can be deferred for a long period of time, if necessary. '+
        'The deferrable tasks can execute with all interrupts enabled '+
        '(softirqs are patterned after hardware interrupts). '+
        'Taking them out of the interrupt handler helps keep kernel response time small.'
    },

    'cpu.softirqs': {
        info: 'Total number of software interrupts per CPU. '+
        'To see the total number for the system check the <a href="#menu_system_submenu_softirqs">softirqs</a> section.'
    },

    'cpu.interrupts': {
        info: 'Total number of interrupts per CPU. '+
        'To see the total number for the system check the <a href="#menu_system_submenu_interrupts">interrupts</a> section. '+
        'The last column in <code>/proc/interrupts</code> provides an interrupt description or the device name that registered the handler for that interrupt.'
    },

    'cpu.throttling': {
        info: ' CPU throttling is commonly used to automatically slow down the computer '+
        'when possible to use less energy and conserve battery.'
    },

    'cpu.cpuidle': {
        info: '<a href="https://en.wikipedia.org/wiki/Advanced_Configuration_and_Power_Interface#Processor_states" target="_blank">Idle States (C-states)</a> '+
        'are used to save power when the processor is idle.'
    },

    'services.net': {
        title: 'network (eBPF)',
    },

    'services.page_cache': {
        title: 'pache cache (eBPF)',
    },

    'netdata.ebpf': {
        title: 'eBPF.plugin',
        info: 'eBPF (extended Berkeley Packet Filter) is used to collect metrics from inside Linux kernel giving a zoom inside your <a href="#ebpf_system_process_thread">Process</a>, '+
              '<a href="#menu_disk">Hard Disk</a>, <a href="#menu_filesystem">File systems</a> (<a href="#menu_filesystem_submenu_file_access">File Access</a>, and ' +
              '<a href="#menu_filesystem_submenu_directory_cache__eBPF_">Directory Cache</a>), Memory (<a href="#ebpf_global_swap">Swap I/O</a>, <a href="#menu_mem_submenu_page_cache">Page Cache</a>), ' +
              'IRQ (<a href="#ebpf_global_hard_irq">Hard IRQ</a> and <a href="#ebpf_global_soft_irq">Soft IRQ</a> ), <a href="#ebpf_global_shm">Shared Memory</a>, ' +
              'Syscalls (<a href="#menu_mem_submenu_synchronization__eBPF_">Sync</a>, <a href="#menu_mount_submenu_mount__eBPF_">Mount</a>), and <a href="#menu_ip_submenu_kernel">Network</a>.'
    },

    'postgres.connections': {
        info: 'A connection is an established line of communication between a client and the PostgreSQL server. Each connection adds to the load on the PostgreSQL server. To guard against running out of memory or overloading the database the <i>max_connections</i> parameter (default = 100) defines the maximum number of concurrent connections to the database server. A separate parameter, <i>superuser_reserved_connections</i> (default = 3), defines the quota for superuser connections (so that superusers can connect even if all other connection slots are blocked).'
    },

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

const netBytesInfo = 'The amount of traffic transferred by the network interface.'
const netPacketsInfo = 'The number of packets transferred by the network interface. ' +
    'Received <a href="https://en.wikipedia.org/wiki/Multicast" target="_blank">multicast</a> counter is ' +
    'commonly calculated at the device level (unlike <b>received</b>) and therefore may include packets which did not reach the host.'
const netErrorsInfo = '<p>The number of errors encountered by the network interface.</p>' +
    '<p><b>Inbound</b> - bad packets received on this interface. ' +
    'It includes dropped packets due to invalid length, CRC, frame alignment, and other errors. ' +
    '<b>Outbound</b> - transmit problems. ' +
    'It includes frames transmission errors due to loss of carrier, FIFO underrun/underflow, heartbeat, ' +
    'late collisions, and other problems.</p>'
const netFIFOInfo = '<p>The number of FIFO errors encountered by the network interface.</p>' +
    '<p><b>Inbound</b> - packets dropped because they did not fit into buffers provided by the host, ' +
    'e.g. packets larger than MTU or next buffer in the ring was not available for a scatter transfer. ' +
    '<b>Outbound</b> - frame transmission errors due to device FIFO underrun/underflow. ' +
    'This condition occurs when the device begins transmission of a frame ' +
    'but is unable to deliver the entire frame to the transmitter in time for transmission.</p>'
const netDropsInfo = '<p>The number of packets that have been dropped at the network interface level.</p>' +
    '<p><b>Inbound</b> - packets received but not processed, e.g. due to ' +
    '<a href="#menu_system_submenu_softnet_stat">softnet backlog</a> overflow, bad/unintended VLAN tags, ' +
    'unknown or unregistered protocols, IPv6 frames when the server is not configured for IPv6. ' +
    '<b>Outbound</b> - packets dropped on their way to transmission, e.g. due to lack of resources.</p>'
const netCompressedInfo = 'The number of correctly transferred compressed packets by the network interface. ' +
    'These counters are only meaningful for interfaces which support packet compression (e.g. CSLIP, PPP).'
const netEventsInfo = '<p>The number of errors encountered by the network interface.</p>' +
    '<p><b>Frames</b> - aggregated counter for dropped packets due to ' +
    'invalid length, FIFO overflow, CRC, and frame alignment errors. ' +
    '<b>Collisions</b> - ' +
    '<a href="https://en.wikipedia.org/wiki/Collision_(telecommunications)" target="blank">collisions</a> during packet transmissions. ' +
    '<b>Carrier</b> - aggregated counter for frame transmission errors due to ' +
    'excessive collisions, loss of carrier, device FIFO underrun/underflow, Heartbeat/SQE Test errors, and  late collisions.</p>'
const netDuplexInfo = '<p>The interface\'s latest or current ' +
    '<a href="https://en.wikipedia.org/wiki/Duplex_(telecommunications)" target="_blank">duplex</a> that the network adapter ' +
    '<a href="https://en.wikipedia.org/wiki/Autonegotiation" target="_blank">negotiated</a> with the device it is connected to.</p>' +
    '<p><b>Unknown</b> - the duplex mode can not be determined. ' +
    '<b>Half duplex</b> - the communication is one direction at a time. ' +
    '<b>Full duplex</b> - the interface is able to send and receive data simultaneously.</p>'
const netOperstateInfo = '<p>The current ' +
    '<a href="https://datatracker.ietf.org/doc/html/rfc2863" target="_blank">operational state</a> of the interface.</p>' +
    '<p><b>Unknown</b> - the state can not be determined. ' +
    '<b>NotPresent</b> - the interface has missing (typically, hardware) components. ' +
    '<b>Down</b> - the interface is unable to transfer data on L1, e.g. ethernet is not plugged or interface is administratively down. ' +
    '<b>LowerLayerDown</b> - the interface is down due to state of lower-layer interface(s). ' +
    '<b>Testing</b> - the interface is in testing mode, e.g. cable test. It can’t be used for normal traffic until tests complete. ' +
    '<b>Dormant</b> - the interface is L1 up, but waiting for an external event, e.g. for a protocol to establish. ' +
    '<b>Up</b> - the interface is ready to pass packets and can be used.</p>'
const netCarrierInfo = 'The current physical link state of the interface.'
const netSpeedInfo = 'The interface\'s latest or current speed that the network adapter ' +
    '<a href="https://en.wikipedia.org/wiki/Autonegotiation" target="_blank">negotiated</a> with the device it is connected to. ' +
    'This does not give the max supported speed of the NIC.'
const netMTUInfo = 'The interface\'s currently configured ' +
    '<a href="https://en.wikipedia.org/wiki/Maximum_transmission_unit" target="_blank">Maximum transmission unit</a> (MTU) value. ' +
    'MTU is the size of the largest protocol data unit that can be communicated in a single network layer transaction.'
// eBPF constants
const ebpfChartProvides = ' This chart is provided by the <a href="#menu_netdata_submenu_ebpf">eBPF plugin</a>.'
const ebpfProcessCreate = 'Number of times <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#processes" target="_blank">a function</a> ' +
    'that starts a process is called. Netdata gives a summary for this chart in <a href="#ebpf_system_process_thread">Process</a>, and when the integration ' +
    'is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, Netdata shows process per ' +
    '<a href="#ebpf_apps_process_create">application</a>.' + ebpfChartProvides
const ebpfThreadCreate = 'Number of times <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#processes" target="_blank">a function</a> ' +
    'that starts a thread is called. Netdata gives a summary for this chart in <a href="#ebpf_system_process_thread">Process</a>, and when the integration ' +
    'is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, Netdata shows process per ' +
    '<a href="#ebpf_apps_thread_create">application</a>.' + ebpfChartProvides
const ebpfTaskExit = 'Number of times <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#processes" target="_blank">a function</a> ' +
    'that responsible for closing tasks is called. Netdata gives a summary for this chart in <a href="#ebpf_system_process_exit">Process</a>, and when the integration ' +
    'is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, Netdata shows process per ' +
    '<a href="#ebpf_apps_process_exit">application</a>.' + ebpfChartProvides
const ebpfTaskClose = 'Number of times <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#processes" target="_blank">a function</a> ' +
    'that responsible for releasing tasks is called. Netdata gives a summary for this chart in <a href="#ebpf_system_process_exit">Process</a>, and when the integration ' +
    'is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, Netdata shows process per ' +
    '<a href="#ebpf_apps_task_release">application</a>.' + ebpfChartProvides
const ebpfTaskError = 'Number of errors to create a new <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#task-error" target="_blank">task</a>. Netdata gives a ' +
    'summary for this chart in <a href="#ebpf_system_task_error">Process</a>, and when the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows process per <a href="#ebpf_apps_task_error">application</a>.' + ebpfChartProvides
const ebpfFileOpen = 'Number of calls for internal functions on the Linux kernel responsible <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#file-descriptor" target="_blank">to open files</a>. ' +
    'Netdata gives a summary for this chart in <a href="#menu_filesystem_submenu_file_access">file access</a>, and when the integration is ' +
    '<a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows virtual file system per <a href="#ebpf_apps_file_open">application</a>.' + ebpfChartProvides
const ebpfFileOpenError = 'Number of failed calls for internal functions on the Linux kernel responsible <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#file-descriptor" target="_blank">to open files</a>. ' +
    'Netdata gives a summary for this chart in <a href="#menu_filesystem_submenu_file_error">file access</a>, and when the integration is ' +
    '<a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows virtual file system per <a href="#ebpf_apps_file_open_error">application</a>.' + ebpfChartProvides
const ebpfFileClosed = 'Number of calls for internal functions on the Linux kernel responsible <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#file-descriptor" target="_blank">to close files</a>. ' +
    'Netdata gives a summary for this chart in <a href="#menu_filesystem_submenu_file_access">file access</a>, and when the integration is ' +
    '<a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows virtual file system per <a href="#ebpf_apps_file_closed">application</a>.' + ebpfChartProvides
const ebpfFileCloseError = 'Number of failed calls for internal functions on the Linux kernel responsible <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#file-descriptor" target="_blank">to close files</a>. ' +
    'Netdata gives a summary for this chart in <a href="#menu_filesystem_submenu_file_error">file access</a>, and when the integration is ' +
    '<a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows virtual file system per <a href="#ebpf_apps_file_close_error">application</a>.' + ebpfChartProvides
const ebpfDCHit = 'Percentage of file accesses that were present in the <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#directory-cache" target="_blank">directory cache</a>. ' +
    'Netdata gives a summary for this chart in <a href="#ebpf_dc_hit_ratio">directory cache</a>, and when the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows directory cache per <a href="#ebpf_apps_dc_hit">application</a>.' + ebpfChartProvides
const ebpfDCReference = 'Number of times a file is accessed inside <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#directory-cache" target="_blank">directory cache</a>. ' +
    'Netdata gives a summary for this chart in <a href="#ebpf_dc_reference">directory cache</a>, and when the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows directory cache per <a href="#ebpf_apps_dc_reference">application</a>.' + ebpfChartProvides
const ebpfDCNotCache = 'Number of times a file is accessed in the file system, because it is not present inside the <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#directory-cache" target="_blank">directory cache</a>. ' +
    'Netdata gives a summary for this chart in <a href="#ebpf_dc_reference">directory cache</a>, and when the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows directory cache per <a href="#ebpf_apps_dc_not_cache">application</a>.' + ebpfChartProvides
const ebpfDCNotFound = 'Number of times a file was not found on the file system. Netdata gives a summary for this chart in <a href="#ebpf_dc_reference">directory cache</a>, ' +
    'and when the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows directory cache per <a href="#ebpf_apps_dc_not_found">application</a>.' + ebpfChartProvides
const ebpfVFSWrite = 'Number of successful calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS writer function</a>. Netdata gives a summary for this chart in ' +
    '<a href="#ebpf_global_vfs_io">Virtual File System</a>, and when the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows virtual file system per <a href="#ebpf_apps_vfs_write">application</a>.' + ebpfChartProvides
const ebpfVFSRead = 'Number of successful calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS reader function</a>. Netdata gives a summary for this chart in ' +
    '<a href="#ebpf_global_vfs_io">Virtual File System</a>, and when the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows virtual file system per <a href="#ebpf_apps_vfs_read">application</a>.' + ebpfChartProvides
const ebpfVFSWriteError = 'Number of failed calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS writer function</a>. Netdata gives a summary for this chart in ' +
    '<a href="#ebpf_global_vfs_io_error">Virtual File System</a>, and when the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows virtual file system per <a href="#ebpf_apps_vfs_write_error">application</a>.' + ebpfChartProvides
const ebpfVFSReadError = 'Number of failed calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS reader function</a>. Netdata gives a summary for this chart in ' +
    '<a href="#ebpf_global_vfs_io_error">Virtual File System</a>, and when the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows virtual file system per <a href="#ebpf_apps_vfs_read_error">application</a>.' + ebpfChartProvides
const ebpfVFSWriteBytes = 'Total of bytes successfully written using the <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS writer function</a>. Netdata gives a summary for this chart in ' +
    '<a href="#ebpf_global_vfs_io_bytes">Virtual File System</a>, and when the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows virtual file system per <a href="#ebpf_apps_vfs_write_bytes">application</a>.' + ebpfChartProvides
const ebpfVFSReadBytes = 'Total of bytes successfully read using the <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS reader function</a>. Netdata gives a summary for this chart in ' +
    '<a href="#ebpf_global_vfs_io_bytes">Virtual File System</a>, and when the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows virtual file system per <a href="#ebpf_apps_vfs_read_bytes">application</a>.' + ebpfChartProvides
const ebpfVFSUnlink = 'Number of calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS unlinker function</a>. Netdata gives a summary for this chart in ' +
    '<a href="#ebpf_global_vfs_unlink">Virtual File System</a>, and when the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows virtual file system per <a href="#ebpf_apps_vfs_unlink">application</a>.' + ebpfChartProvides
const ebpfVFSSync = 'Number of calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS syncer function</a>. Netdata gives a summary for this chart in ' +
    '<a href="#ebpf_global_vfs_sync">Virtual File System</a>, and when the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows virtual file system per <a href="#ebpf_apps_vfs_sync">application</a>.' + ebpfChartProvides
const ebpfVFSSyncError = 'Number of failed calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS syncer function</a>. Netdata gives a summary for this chart in ' +
    '<a href="#ebpf_global_vfs_sync_error">Virtual File System</a>, and when the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows virtual file system per <a href="#ebpf_apps_vfs_sync_error">application</a>.' + ebpfChartProvides
const ebpfVFSOpen = 'Number of calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS opener function</a>. Netdata gives a summary for this chart in ' +
    '<a href="#ebpf_global_vfs_open">Virtual File System</a>, and when the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows virtual file system per <a href="#ebpf_apps_vfs_open">application</a>.' + ebpfChartProvides
const ebpfVFSOpenError = 'Number of failed calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS opener function</a>. Netdata gives a summary for this chart in ' +
    '<a href="#ebpf_global_vfs_open_error">Virtual File System</a>, and when the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows virtual file system per <a href="#ebpf_apps_vfs_open_error">application</a>.' + ebpfChartProvides
const ebpfVFSCreate = 'Number of calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS creator function</a>. Netdata gives a summary for this chart in ' +
    '<a href="#ebpf_global_vfs_create">Virtual File System</a>, and when the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows virtual file system per <a href="#ebpf_apps_vfs_create">application</a>.' + ebpfChartProvides
const ebpfVFSCreateError = 'Number of failed calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS creator function</a>. Netdata gives a summary for this chart in ' +
    '<a href="#ebpf_global_vfs_create_error">Virtual File System</a>, and when the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows virtual file system per <a href="#ebpf_apps_vfs_create_error">application</a>.' + ebpfChartProvides
const ebpfSwapRead = 'Number of failed calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#swap" target="_blank">swap reader function</a>. Netdata gives a summary for this chart in ' +
    '<a href="#ebpf_global_swap">System Overview</a>, and when the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows swap per <a href="#ebpf_apps_swap_read">application</a>.' + ebpfChartProvides
const ebpfSwapWrite = 'Number of failed calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#swap" target="_blank">swap writer function</a>. Netdata gives a summary for this chart in ' +
    '<a href="#ebpf_global_swap">System Overview</a>, and when the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows swap per <a href="#ebpf_apps_swap_write">application</a>.' + ebpfChartProvides
const ebpfCachestatRatio = 'The <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#page-cache-ratio" target="_blank">ratio</a> shows the percentage of data accessed directly in memory. ' +
    'Netdata gives a summary for this chart in <a href="#menu_mem_submenu_page_cache">Memory</a>, and when the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows page cache hit per <a href="#ebpf_apps_cachestat_ratio">application</a>.' + ebpfChartProvides
const ebpfCachestatDirties = 'Number of <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#dirty-pages" target="_blank">modified pages</a> in <a href="https://en.wikipedia.org/wiki/Page_cache" target="_blank">Linux page cache</a>. ' +
    'Netdata gives a summary for this chart in <a href="#ebpf_global_cachestat_dirty">Memory</a>, and when the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows page cache hit per <a href="#ebpf_apps_cachestat_dirties">application</a>.' + ebpfChartProvides
const ebpfCachestatHits = 'Number of <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#page-cache-hits" target="_blank">access</a> to data in <a href="https://en.wikipedia.org/wiki/Page_cache" target="_blank">Linux page cache</a>. ' +
    'Netdata gives a summary for this chart in <a href="#ebpf_global_cachestat_hits">Memory</a>, and when the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows page cache hit per <a href="#ebpf_apps_cachestat_hits">application</a>.' + ebpfChartProvides
const ebpfCachestatMisses = 'Number of <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#page-cache-misses" target="_blank">access</a> to data was not present in <a href="https://en.wikipedia.org/wiki/Page_cache" target="_blank">Linux page cache</a>. ' +
    'Netdata gives a summary for this chart in <a href="#ebpf_global_cachestat_misses">Memory</a>, and when the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows page cache misses per <a href="#ebpf_apps_cachestat_misses">application</a>.' + ebpfChartProvides
const ebpfSHMget = 'Number of calls to <code>shmget</code>. Netdata gives a summary for this chart in <a href="#ebpf_global_shm">System Overview</a>, and when the integration is ' +
    '<a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, Netdata shows shared memory metrics per <a href="#ebpf_apps_shm_get">application</a>.' + ebpfChartProvides
const ebpfSHMat = 'Number of calls to <code>shmat</code>. Netdata gives a summary for this chart in <a href="#ebpf_global_shm">System Overview</a>, and when the integration is ' +
    '<a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, Netdata shows shared memory metrics per <a href="#ebpf_apps_shm_at">application</a>.' + ebpfChartProvides
const ebpfSHMctl = 'Number of calls to <code>shmctl</code>. Netdata gives a summary for this chart in <a href="#ebpf_global_shm">System Overview</a>, and when the integration is ' +
    '<a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, Netdata shows shared memory metrics per <a href="#ebpf_apps_shm_ctl">application</a>.' + ebpfChartProvides
const ebpfSHMdt = 'Number of calls to <code>shmdt</code>. Netdata gives a summary for this chart in <a href="#ebpf_global_shm">System Overview</a>, and when the integration is ' +
    '<a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, Netdata shows shared memory metrics per <a href="#ebpf_apps_shm_dt">application</a>.' + ebpfChartProvides
const ebpfIPV4conn = 'Number of calls to IPV4 TCP function responsible for <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#tcp-outbound-connections" target="_blank">starting connections</a>. ' +
    'Netdata gives a summary for this chart in <a href="#ebpf_global_outbound_conn">Network Stack</a>. When the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows outbound connections per <a href="#ebpf_apps_outbound_conn_ipv4">application</a>.' + ebpfChartProvides
const ebpfIPV6conn = 'Number of calls to IPV6 TCP function responsible for <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#tcp-outbound-connections" target="_blank">starting connections</a>. ' +
    'Netdata gives a summary for this chart in <a href="#ebpf_global_outbound_conn">Network Stack</a>. When the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows outbound connections per <a href="#ebpf_apps_outbound_conn_ipv6">application</a>.' + ebpfChartProvides
const ebpfBandwidthSent = 'Total bytes sent with <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#tcp-bandwidth" target="_blank">TCP</a> or <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#udp-functions" target="_blank">UDP</a> internal functions. ' +
    'Netdata gives a summary for this chart in <a href="#ebpf_global_bandwidth_tcp_bytes">Network Stack</a>. When the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows bandwidth per <a href="#ebpf_apps_bandwidth_sent">application</a>.' + ebpfChartProvides
const ebpfBandwidthRecv = 'Total bytes received with <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#tcp-bandwidth" target="_blank">TCP</a> or <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#udp-functions" target="_blank">UDP</a> internal functions. ' +
    'Netdata gives a summary for this chart in <a href="#ebpf_global_bandwidth_tcp_bytes">Network Stack</a>. When the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows bandwidth per <a href="#ebpf_apps_bandwidth_received">application</a>.' + ebpfChartProvides
const ebpfTCPSendCall = 'Number of calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#tcp-bandwidth" target="_blank">TCP</a> functions responsible to send data. ' +
    'Netdata gives a summary for this chart in <a href="#ebpf_global_tcp_bandwidth_call">Network Stack</a>. ' +
    'When the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, Netdata shows TCP calls per <a href="#ebpf_apps_bandwidth_tcp_sent">application</a>.' + ebpfChartProvides
const ebpfTCPRecvCall = 'Number of calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#tcp-bandwidth" target="_blank">TCP</a> functions responsible to receive data. ' +
    'Netdata gives a summary for this chart in <a href="#ebpf_global_tcp_bandwidth_call">Network Stack</a>. ' +
    'When the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, ' +
    'Netdata shows TCP calls per <a href="#ebpf_apps_bandwidth_tcp_received">application</a>.' + ebpfChartProvides
const ebpfTCPRetransmit = 'Number of times a <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#tcp-retransmit" target="_blank">TCP</a> packet was retransmitted. ' +
    'Netdata gives a summary for this chart in <a href="#ebpf_global_tcp_retransmit">Network Stack</a>. ' +
    'When the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, Netdata shows TCP calls per <a href="#ebpf_apps_tcp_retransmit">application</a>.' + ebpfChartProvides
const ebpfUDPsend = 'Number of calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#udp-functions" target="_blank">UDP</a> functions responsible to send data. ' +
    'Netdata gives a summary for this chart in <a href="#ebpf_global_udp_bandwidth_call">Network Stack</a>. ' +
    'When the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, Netdata shows UDP calls per <a href="#ebpf_apps_udp_sendmsg">application</a>.' + ebpfChartProvides
const ebpfUDPrecv = 'Number of calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#udp-functions" target="_blank">UDP</a> functions responsible to receive data. ' +
    'Netdata gives a summary for this chart in <a href="#ebpf_global_udp_bandwidth_call">Network Stack</a>. ' +
    'When the integration is <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">enabled</a>, Netdata shows UDP calls per <a href="#ebpf_apps_udp_recv">application</a>.' + ebpfChartProvides

const cgroupCPULimit = 'Total CPU utilization within the configured or system-wide (if not set) limits. When the CPU utilization of a cgroup exceeds the limit for the configured period, the tasks belonging to its hierarchy will be throttled and are not allowed to run again until the next period.'
const cgroupCPU = 'Total CPU utilization within the system-wide CPU resources (all cores). The amount of time spent by tasks of the cgroup in <a href="https://en.wikipedia.org/wiki/CPU_modes#Mode_types" target="_blank">user and kernel</a> modes.'
const cgroupThrottled = 'The percentage of runnable periods when tasks in a cgroup have been throttled. The tasks have not been allowed to run because they have exhausted all of the available time as specified by their CPU quota.'
const cgroupThrottledDuration = 'The total time duration for which tasks in a cgroup have been throttled. When an application has used its allotted CPU quota for a given period, it gets throttled until the next period.'
const cgroupCPUShared = '<p>The weight of each group living in the same hierarchy, that translates into the amount of CPU it is expected to get. The percentage of CPU assigned to the cgroup is the value of shares divided by the sum of all shares in all cgroups in the same level.</p> <p>For example, tasks in two cgroups that have <b>cpu.shares</b> set to 100 will receive equal CPU time, but tasks in a cgroup that has <b>cpu.shares</b> set to 200 receive twice the CPU time of tasks in a cgroup where <b>cpu.shares</b> is set to 100.</p>'
const cgroupCPUPerCore = 'Total CPU utilization per core within the system-wide CPU resources.'
const cgroupCPUSomePressure = 'CPU <a href="https://www.kernel.org/doc/html/latest/accounting/psi.html" target="_blank">Pressure Stall Information</a>. <b>Some</b> indicates the share of time in which at least <b>some tasks</b> are stalled on CPU. The ratios are tracked as recent trends over 10-, 60-, and 300-second windows.'
const cgroupCPUSomePressureStallTime = 'The amount of time some processes have been waiting for CPU time.'
const cgroupCPUFullPressure = 'CPU <a href="https://www.kernel.org/doc/html/latest/accounting/psi.html" target="_blank">Pressure Stall Information</a>. <b>Full</b> indicates the share of time in which <b>all non-idle tasks</b> are stalled on CPU resource simultaneously. The ratios are tracked as recent trends over 10-, 60-, and 300-second windows.'
const cgroupCPUFullPressureStallTime = 'The amount of time all non-idle processes have been stalled due to CPU congestion.'

const cgroupMemUtilization = 'RAM utilization within the configured or system-wide (if not set) limits. When the RAM utilization of a cgroup exceeds the limit, OOM killer will start killing the tasks belonging to the cgroup.'
const cgroupMemUsageLimit = 'RAM usage within the configured or system-wide (if not set) limits. When the RAM usage of a cgroup exceeds the limit, OOM killer will start killing the tasks belonging to the cgroup.'
const cgroupMemUsage = 'The amount of used RAM and swap memory.'
const cgroupMem = 'Memory usage statistics. The individual metrics are described in the memory.stat section for <a href="https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v1/memory.html#per-memory-cgroup-local-status" target="_blank">cgroup-v1</a> and <a href="https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html#memory-interface-files" target="_blank">cgroup-v2</a>.'
const cgroupMemFailCnt = 'The number of memory usage hits limits.'
const cgroupWriteback = '<b>Dirty</b> is the amount of memory waiting to be written to disk. <b>Writeback</b> is how much memory is actively being written to disk.'
const cgroupMemActivity = '<p>Memory accounting statistics.</p><p><b>In</b> - a page is accounted as either mapped anon page (RSS) or cache page (Page Cache) to the cgroup. <b>Out</b> - a page is unaccounted from the cgroup.</p>'
const cgroupPgFaults = '<p>Memory <a href="https://en.wikipedia.org/wiki/Page_fault" target="_blank">page fault</a> statistics.</p><p><b>Pgfault</b> - all page faults. <b>Swap</b> - major page faults.</p>'
const cgroupMemorySomePressure = 'Memory <a href="https://www.kernel.org/doc/html/latest/accounting/psi.html" target="_blank">Pressure Stall Information</a>. <b>Some</b> indicates the share of time in which at least <b>some tasks</b> are stalled on memory. In this state the CPU is still doing productive work. The ratios are tracked as recent trends over 10-, 60-, and 300-second windows.'
const cgroupMemorySomePressureStallTime = 'The amount of time some processes have been waiting due to memory congestion.'
const cgroupMemoryFullPressure = 'Memory <a href="https://www.kernel.org/doc/html/latest/accounting/psi.html" target="_blank">Pressure Stall Information</a>. <b>Full</b> indicates the share of time in which <b>all non-idle tasks</b> are stalled on memory resource simultaneously. In this state actual CPU cycles are going to waste, and a workload that spends extended time in this state is considered to be thrashing. This has severe impact on performance. The ratios are tracked as recent trends over 10-, 60-, and 300-second windows.'
const cgroupMemoryFullPressureStallTime = 'The amount of time all non-idle processes have been stalled due to memory congestion.'

const cgroupIO = 'The amount of data transferred to and from specific devices as seen by the CFQ scheduler. It is not updated when the CFQ scheduler is operating on a request queue.'
const cgroupServicedOps = 'The number of I/O operations performed on specific devices as seen by the CFQ scheduler.'
const cgroupQueuedOps = 'The number of requests queued for I/O operations.'
const cgroupMergedOps = 'The number of BIOS requests merged into requests for I/O operations.'
const cgroupThrottleIO = 'The amount of data transferred to and from specific devices as seen by the throttling policy.'
const cgroupThrottleIOServicesOps = 'The number of I/O operations performed on specific devices as seen by the throttling policy.'
const cgroupIOSomePressure = 'I/O <a href="https://www.kernel.org/doc/html/latest/accounting/psi.html" target="_blank">Pressure Stall Information</a>. <b>Some</b> indicates the share of time in which at least <b>some tasks</b> are stalled on I/O. In this state the CPU is still doing productive work. The ratios are tracked as recent trends over 10-, 60-, and 300-second windows.'
const cgroupIOSomePRessureStallTime = 'The amount of time some processes have been waiting due to I/O congestion.'
const cgroupIOFullPressure = 'I/O <a href="https://www.kernel.org/doc/html/latest/accounting/psi.html" target="_blank">Pressure Stall Information</a>. <b>Full</b> line indicates the share of time in which <b>all non-idle tasks</b> are stalled on I/O resource simultaneously. In this state actual CPU cycles are going to waste, and a workload that spends extended time in this state is considered to be thrashing. This has severe impact on performance. The ratios are tracked as recent trends over 10-, 60-, and 300-second windows.'
const cgroupIOFullPressureStallTime = 'The amount of time all non-idle processes have been stalled due to I/O congestion.'

netdataDashboard.context = {
    'system.cpu': {
        info: function (os) {
            void (os);
            return 'Total CPU utilization (all cores). 100% here means there is no CPU idle time at all. You can get per core usage at the <a href="#menu_cpu">CPUs</a> section and per application usage at the <a href="#menu_apps">Applications Monitoring</a> section.'
                + netdataDashboard.sparkline('<br/>Keep an eye on <b>iowait</b> ', 'system.cpu', 'iowait', '%', '. If it is constantly high, your disks are a bottleneck and they slow your system down.')
                + netdataDashboard.sparkline(
                '<br/>An important metric worth monitoring, is <b>softirq</b> ',
                'system.cpu',
                'softirq',
                '%',
                '. A constantly high percentage of softirq may indicate network driver issues. '+
                'The individual metrics can be found in the '+
                '<a href="https://www.kernel.org/doc/html/latest/filesystems/proc.html#miscellaneous-kernel-statistics-in-proc-stat" target="_blank">kernel documentation</a>.');
        },
        valueRange: "[0, 100]"
    },

    'system.load': {
        info: 'Current system load, i.e. the number of processes using CPU or waiting for system resources (usually CPU and disk). The 3 metrics refer to 1, 5 and 15 minute averages. The system calculates this once every 5 seconds. For more information check <a href="https://en.wikipedia.org/wiki/Load_(computing)" target="_blank">this wikipedia article</a>.',
        height: 0.7
    },

    'system.cpu_some_pressure': {
        info: 'CPU <a href="https://www.kernel.org/doc/html/latest/accounting/psi.html" target="_blank">Pressure Stall Information</a>. '+
            '<b>Some</b> indicates the share of time in which at least <b>some tasks</b> are stalled on CPU. ' +
            'The ratios are tracked as recent trends over 10-, 60-, and 300-second windows.'
    },
    'system.cpu_some_pressure_stall_time': {
        info: 'The amount of time some processes have been waiting for CPU time.'
    },
    'system.cpu_full_pressure': {
        info: 'CPU <a href="https://www.kernel.org/doc/html/latest/accounting/psi.html" target="_blank">Pressure Stall Information</a>. ' +
            '<b>Full</b> indicates the share of time in which <b>all non-idle tasks</b> are stalled on CPU resource simultaneously. ' +
            'The ratios are tracked as recent trends over 10-, 60-, and 300-second windows.'
    },
    'system.cpu_full_pressure_stall_time': {
        info: 'The amount of time all non-idle processes have been stalled due to CPU congestion.'
    },

    'system.memory_some_pressure': {
        info: 'Memory <a href="https://www.kernel.org/doc/html/latest/accounting/psi.html" target="_blank">Pressure Stall Information</a>. '+
            '<b>Some</b> indicates the share of time in which at least <b>some tasks</b> are stalled on memory. ' +
            'In this state the CPU is still doing productive work. '+
            'The ratios are tracked as recent trends over 10-, 60-, and 300-second windows.'
    },
    'system.memory_some_pressure_stall_time': {
        info: 'The amount of time some processes have been waiting due to memory congestion.'
    },
    'system.memory_full_pressure': {
        info: 'Memory <a href="https://www.kernel.org/doc/html/latest/accounting/psi.html" target="_blank">Pressure Stall Information</a>. ' +
            '<b>Full</b> indicates the share of time in which <b>all non-idle tasks</b> are stalled on memory resource simultaneously. ' +
            'In this state actual CPU cycles are going to waste, and a workload that spends extended time in this state is considered to be thrashing. '+
            'This has severe impact on performance. '+
            'The ratios are tracked as recent trends over 10-, 60-, and 300-second windows.'
    },
    'system.memory_full_pressure_stall_time': {
        info: 'The amount of time all non-idle processes have been stalled due to memory congestion.'
    },

    'system.io_some_pressure': {
        info: 'I/O <a href="https://www.kernel.org/doc/html/latest/accounting/psi.html" target="_blank">Pressure Stall Information</a>. '+
            '<b>Some</b> indicates the share of time in which at least <b>some tasks</b> are stalled on I/O. ' +
            'In this state the CPU is still doing productive work. '+
            'The ratios are tracked as recent trends over 10-, 60-, and 300-second windows.'
    },
    'system.io_some_pressure_stall_time': {
        info: 'The amount of time some processes have been waiting due to I/O congestion.'
    },
    'system.io_full_pressure': {
        info: 'I/O <a href="https://www.kernel.org/doc/html/latest/accounting/psi.html" target="_blank">Pressure Stall Information</a>. ' +
            '<b>Full</b> line indicates the share of time in which <b>all non-idle tasks</b> are stalled on I/O resource simultaneously. ' +
            'In this state actual CPU cycles are going to waste, and a workload that spends extended time in this state is considered to be thrashing. '+
            'This has severe impact on performance. '+
            'The ratios are tracked as recent trends over 10-, 60-, and 300-second windows.'
    },
    'system.io_full_pressure_stall_time': {
        info: 'The amount of time all non-idle processes have been stalled due to I/O congestion.'
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
        info: '<p>System swap I/O.</p>'+
        '<b>In</b> - pages the system has swapped in from disk to RAM. '+
        '<b>Out</b> - pages the system has swapped out from RAM to disk.'
    },

    'system.pgfaults': {
        info: 'Total page faults. <b>Major page faults</b> indicates that the system is using its swap. You can find which applications use the swap at the <a href="#menu_apps">Applications Monitoring</a> section.'
    },

    'system.entropy': {
        colors: '#CC22AA',
        info: '<a href="https://en.wikipedia.org/wiki/Entropy_(computing)" target="_blank">Entropy</a>, is a pool of random numbers (<a href="https://en.wikipedia.org/wiki//dev/random" target="_blank">/dev/random</a>) that is mainly used in cryptography. If the pool of entropy gets empty, processes requiring random numbers may run a lot slower (it depends on the interface each program uses), waiting for the pool to be replenished. Ideally a system with high entropy demands should have a hardware device for that purpose (TPM is one such device). There are also several software-only options you may install, like <code>haveged</code>, although these are generally useful only in servers.'
    },

    'system.zswap_rejections': {
        info: '<p>Zswap rejected pages per access.</p>' +
            '<p><b>CompressPoor</b> - compressed page was too big for the allocator to store. ' +
            '<b>KmemcacheFail</b> - number of entry metadata that could not be allocated. ' +
            '<b>AllocFail</b> - allocator could not get memory. ' +
            '<b>ReclaimFail</b> - memory cannot be reclaimed (pool limit was reached).</p>'
    },

    'system.clock_sync_state': {
        info:'<p>The system clock synchronization state as provided by the <a href="https://man7.org/linux/man-pages/man2/adjtimex.2.html" target="_blank">ntp_adjtime()</a> system call. '+
        'An unsynchronized clock may be the result of synchronization issues by the NTP daemon or a hardware clock fault. '+
        'It can take several minutes (usually up to 17) before NTP daemon selects a server to synchronize with. '+
        '<p><b>State map</b>: 0 - not synchronized, 1 - synchronized.</p>'
    },

    'system.clock_status': {
        info:'<p>The kernel code can operate in various modes and with various features enabled or disabled, as selected by the '+
        '<a href="https://man7.org/linux/man-pages/man2/adjtimex.2.html" target="_blank">ntp_adjtime()</a> system call. '+
        'The system clock status shows the value of the <b>time_status</b> variable in the kernel. '+
        'The bits of the variable are used to control these functions and record error conditions as they exist.</p>'+
        '<p><b>UNSYNC</b> - set/cleared by the caller to indicate clock unsynchronized (e.g., when no peers are reachable). '+
        'This flag is usually controlled by an application program, but the operating system may also set it. '+
        '<b>CLOCKERR</b> - set/cleared by the external hardware clock driver to indicate hardware fault.</p>'+
        '<p><b>Status map</b>: 0 - bit unset, 1 - bit set.</p>'
    },

    'system.clock_sync_offset': {
        info: 'A typical NTP client regularly polls one or more NTP servers. '+
        'The client must compute its '+
        '<a href="https://en.wikipedia.org/wiki/Network_Time_Protocol#Clock_synchronization_algorithm" target="_blank">time offset</a> '+
        'and round-trip delay. '+
        'Time offset is the difference in absolute time between the two clocks.'
    },

    'system.forks': {
        colors: '#5555DD',
        info: 'The number of new processes created.'
    },

    'system.intr': {
        colors: '#DD5555',
        info: 'Total number of CPU interrupts. Check <code>system.interrupts</code> that gives more detail about each interrupt and also the <a href="#menu_cpu">CPUs</a> section where interrupts are analyzed <a href="#menu_cpu_submenu_interrupts">per CPU core</a>.'
    },

    'system.interrupts': {
        info: 'CPU interrupts in detail. At the <a href="#menu_cpu">CPUs</a> section, interrupts are analyzed <a href="#menu_cpu_submenu_interrupts">per CPU core</a>. '+
        'The last column in <code>/proc/interrupts</code> provides an interrupt description or the device name that registered the handler for that interrupt.'
    },

    'system.hardirq_latency': {
        info: 'Total time spent servicing <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#hard-irq" target="_blank">hardware interrupts</a>. Based on the eBPF <a href="https://github.com/iovisor/bcc/blob/master/tools/hardirqs_example.txt" target="_blank">hardirqs</a> from BCC tools.' + ebpfChartProvides + '<div id="ebpf_global_hard_irq"></div>'
    },

    'system.softirqs': {
        info: '<p>Total number of software interrupts in the system. '+
        'At the <a href="#menu_cpu">CPUs</a> section, softirqs are analyzed <a href="#menu_cpu_submenu_softirqs">per CPU core</a>.</p>'+
        '<p><b>HI</b> - high priority tasklets. '+
        '<b>TIMER</b> - tasklets related to timer interrupts. '+
        '<b>NET_TX</b>, <b>NET_RX</b> - used for network transmit and receive processing. '+
        '<b>BLOCK</b> - handles block I/O completion events. '+
        '<b>IRQ_POLL</b> - used by the IO subsystem to increase performance (a NAPI like approach for block devices). '+
        '<b>TASKLET</b> - handles regular tasklets. '+
        '<b>SCHED</b> - used by the scheduler to perform load-balancing and other scheduling tasks. '+
        '<b>HRTIMER</b> - used for high-resolution timers. '+
        '<b>RCU</b> - performs read-copy-update (RCU) processing.</p>'

    },

    'system.softirq_latency': {
        info: 'Total time spent servicing <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#soft-irq" target="_blank">software interrupts</a>. Based on the eBPF <a href="https://github.com/iovisor/bcc/blob/master/tools/softirqs_example.txt" target="_blank">softirqs</a> from BCC tools.' + ebpfChartProvides + '<div id="ebpf_global_soft_irq"></div>'
    },

    'system.processes': {
        info: '<p>System processes.</p>'+
        '<p><b>Running</b> - running or ready to run (runnable). '+
        '<b>Blocked</b> - currently blocked, waiting for I/O to complete.</p>'
    },

    'system.processes_state': {
        info: '<p>The number of processes in different states. </p> '+
        '<p><b>Running</b> - Process using the CPU at a particular moment. '+
        '<b>Sleeping (uninterruptible)</b> - Process will wake when a waited-upon resource becomes available or after a time-out occurs during that wait. '+
        'Mostly used by device drivers waiting for disk or network I/O. '+
        '<b>Sleeping (interruptible)</b> - Process is waiting either for a particular time slot or for a particular event to occur. '+
        '<b>Zombie</b> - Process that has completed its execution, released the system resources, but its entry is not removed from the process table. '+
        'Usually occurs in child processes when the parent process still needs to read its child’s exit status. '+
        'A process that stays a zombie for a long time is generally an error and causes system PID space leak. '+
        '<b>Stopped</b> - Process is suspended from proceeding further due to STOP or TSTP signals. ' +
        'In this state, a process will not do anything (not even terminate) until it receives a CONT signal.</p>'
    },

    'system.active_processes': {
        info: 'The total number of processes in the system.'
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

    'system.swapcalls': {
        info: 'Number of calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#swap" target="_blank">functions</a> used to manipulate swap data. Netdata shows swap metrics per <a href="#ebpf_apps_swap_read">application</a> and <a href="#ebpf_services_swap_read">cgroup (systemd Services)</a> if <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">apps</a> or <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">cgroup (systemd Services)</a> plugins are enabled.' + ebpfChartProvides + '<div id="ebpf_global_swap"></div>'
    },

    'system.ipc_semaphores': {
        info: 'Number of allocated System V IPC semaphores. '+
        'The system-wide limit on the number of semaphores in all semaphore sets is specified in <code>/proc/sys/kernel/sem</code> file (2nd field).'
    },

    'system.ipc_semaphore_arrays': {
        info: 'Number of used System V IPC semaphore arrays (sets). Semaphores support semaphore sets where each one is a counting semaphore. '+
        'So when an application requests semaphores, the kernel releases them in sets. '+
        'The system-wide limit on the maximum number of semaphore sets is specified in <code>/proc/sys/kernel/sem</code> file (4th field).'
    },

    'system.shared_memory_segments': {
        info: 'Number of allocated System V IPC memory segments. '+
        'The system-wide maximum number of shared memory segments that can be created is specified in <code>/proc/sys/kernel/shmmni</code> file.'
    },

    'system.shared_memory_bytes': {
        info: 'Amount of memory currently used by System V IPC memory segments. '+
        'The run-time limit on the maximum  shared memory segment size that can be created is specified in <code>/proc/sys/kernel/shmmax</code> file.'
    },

    'system.shared_memory_calls': {
        info: 'Number of calls to syscalls responsible to manipulate <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#ipc-shared-memory" target="_blank">shared memories</a>. Netdata shows shared memory metrics per <a href="#ebpf_apps_shm_get">application</a> and <a href="#ebpf_services_shm_get">cgroup (systemd Services)</a> if <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">apps</a> or <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">cgroup (systemd Services)</a> plugins are enabled.' + ebpfChartProvides + '<div id="ebpf_global_shm"></div>'
    },

    'system.message_queue_messages': {
        info: 'Number of messages that are currently present in System V IPC message queues.'
    },

    'system.message_queue_bytes': {
        info: 'Amount of memory currently used by messages in System V IPC message queues.'
    },

    'system.uptime': {
        info: 'The amount of time the system has been running, including time spent in suspend.'
    },

    'system.process_thread': {
        title : 'Task creation',
        info: 'Number of times <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#processes" target="_blank">a function</a> that starts a process or thread is called. Netdata shows process metrics per <a href="#ebpf_apps_process_create">application</a> and <a href="#ebpf_services_process_create">cgroup (systemd Services)</a> if <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">apps</a> or <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">cgroup (systemd Services)</a> plugins are enabled.' + ebpfChartProvides + '<div id="ebpf_system_process_thread"></div>'
    },

    'system.exit': {
        title : 'Exit monitoring',
        info: 'Number of times <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#process-exit" target="_blank">a function</a> responsible to close a process or thread is called. Netdata shows process metrics per <a href="#ebpf_apps_process_exit">application</a> and <a href="#ebpf_services_process_exit">cgroup (systemd Services)</a> if <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">apps</a> or <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">cgroup (systemd Services)</a> plugins are enabled.' + ebpfChartProvides + '<div id="ebpf_system_process_exit"></div>'
    },

    'system.task_error': {
        title : 'Task error',
        info: 'Number of times <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#task-error" target="_blank">a function</a> that starts a process or thread failed. Netdata shows process metrics per <a href="#ebpf_apps_task_error">application</a> and <a href="#ebpf_services_task_error">cgroup (systemd Services)</a> if <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">apps</a> or <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">cgroup (systemd Services)</a> plugins are enabled.' + ebpfChartProvides + '<div id="ebpf_system_task_error"></div>'
    },

    'system.process_status': {
        title : 'Task status',
        info: 'Difference between the number of calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#process-exit" target="_blank">functions</a> that close a task and release a task.'+ ebpfChartProvides
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

    'cpu.core_throttling': {
        info: 'The number of adjustments made to the clock speed of the CPU based on it\'s core temperature.'
    },

    'cpu.package_throttling': {
        info: 'The number of adjustments made to the clock speed of the CPU based on it\'s package (chip) temperature.'
    },

    'cpufreq.cpufreq': {
        info: 'The frequency measures the number of cycles your CPU executes per second.'
    },

    'cpuidle.cpuidle': {
        info: 'The percentage of time spent in C-states.'
    },

    // ------------------------------------------------------------------------
    // MEMORY

    'mem.ksm': {
        info: '<p>Memory pages merging statistics. '+
        'A high ratio of <b>Sharing</b> to <b>Shared</b> indicates good sharing, '+
        'but a high ratio of <b>Unshared</b> to <b>Sharing</b> indicates wasted effort.</p>'+
        '<p><b>Shared</b> - used shared pages. '+
        '<b>Unshared</b> - memory no longer shared (pages are unique but repeatedly checked for merging). '+
        '<b>Sharing</b> - memory currently shared (how many more sites are sharing the pages, i.e. how much saved). '+
        '<b>Volatile</b> - volatile pages (changing too fast to be placed in a tree).</p>'
    },

    'mem.ksm_savings': {
        heads: [
            netdataDashboard.gaugeChart('Saved', '12%', 'savings', '#0099CC')
        ],
        info: '<p>The amount of memory saved by KSM.</p>'+
        '<p><b>Savings</b> - saved memory. '+
        '<b>Offered</b> - memory marked as mergeable.</p>'
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
        ],
        info: 'The effectiveness of KSM. '+
        'This is the percentage of the mergeable pages that are currently merged.'
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
        info: '<p>A <a href="https://en.wikipedia.org/wiki/Page_fault" target="_blank">page fault</a> is a type of interrupt, '+
        'called trap, raised by computer hardware when a running program accesses a memory page '+
        'that is mapped into the virtual address space, but not actually loaded into main memory.</p>'+
        '</p><b>Minor</b> - the page is loaded in memory at the time the fault is generated, '+
        'but is not marked in the memory management unit as being loaded in memory. '+
        '<b>Major</b> - generated when the system needs to load the memory page from disk or swap memory.</p>'
    },

    'mem.committed': {
        colors: NETDATA.colors[3],
        info: 'Committed Memory, is the sum of all memory which has been allocated by processes.'
    },

    'mem.real': {
        colors: NETDATA.colors[3],
        info: 'Total amount of real (physical) memory used.'
    },

    'mem.oom_kill': {
        info: 'The number of processes killed by '+
        '<a href="https://en.wikipedia.org/wiki/Out_of_memory" target="_blank">Out of Memory</a> Killer. '+
        'The kernel\'s OOM killer is summoned when the system runs short of free memory and '+
        'is unable to proceed without killing one or more processes. '+
        'It tries to pick the process whose demise will free the most memory while '+
        'causing the least misery for users of the system. '+
        'This counter also includes processes within containers that have exceeded the memory limit.'
    },

    'mem.numa': {
        info: '<p>NUMA balancing statistics.</p>'+
        '<p><b>Local</b> - pages successfully allocated on this node, by a process on this node. '+
        '<b>Foreign</b> - pages initially intended for this node that were allocated to another node instead. '+
        '<b>Interleave</b> - interleave policy pages successfully allocated to this node. '+
        '<b>Other</b> - pages allocated on this node, by a process on another node. '+
        '<b>PteUpdates</b> - base pages that were marked for NUMA hinting faults. '+
        '<b>HugePteUpdates</b> - transparent huge pages that were marked for NUMA hinting faults. '+
        'In Combination with <b>pte_updates</b> the total address space that was marked can be calculated. '+
        '<b>HintFaults</b> - NUMA hinting faults that were trapped. '+
        '<b>HintFaultsLocal</b> - hinting faults that were to local nodes. '+
        'In combination with <b>HintFaults</b>, the percentage of local versus remote faults can be calculated. '+
        'A high percentage of local hinting faults indicates that the workload is closer to being converged. '+
        '<b>PagesMigrated</b> - pages were migrated because they were misplaced. '+
        'As migration is a copying operation, it contributes the largest part of the overhead created by NUMA balancing.</p>'
    },

    'mem.available': {
        info: function (os) {
            if (os === "freebsd")
                return 'The amount of memory that can be used by user-space processes without causing swapping. Calculated as the sum of free, cached, and inactive memory.';
            else
                return 'Available Memory is estimated by the kernel, as the amount of RAM that can be used by userspace processes, without causing swapping.';
        }
    },

    'mem.writeback': {
        info: '<b>Dirty</b> is the amount of memory waiting to be written to disk. <b>Writeback</b> is how much memory is actively being written to disk.'
    },

    'mem.kernel': {
        info: '<p>The total amount of memory being used by the kernel.</p>'+
        '<p><b>Slab</b> - used by the kernel to cache data structures for its own use. '+
        '<b>KernelStack</b> - allocated for each task done by the kernel. '+
        '<b>PageTables</b> - dedicated to the lowest level of page tables (A page table is used to turn a virtual address into a physical memory address). '+
        '<b>VmallocUsed</b> - being used as virtual address space. '+
        '<b>Percpu</b> - allocated to the per-CPU allocator used to back per-CPU allocations (excludes the cost of metadata). '+
        'When you create a per-CPU variable, each processor on the system gets its own copy of that variable.</p>'
    },

    'mem.slab': {
        info: '<p><a href="https://en.wikipedia.org/wiki/Slab_allocation" target="_blank">Slab memory</a> statistics.<p>'+
        '<p><b>Reclaimable</b> - amount of memory which the kernel can reuse. '+
        '<b>Unreclaimable</b> - can not be reused even when the kernel is lacking memory.</p>'
    },

    'mem.hugepages': {
        info: 'Dedicated (or Direct) HugePages is memory reserved for applications configured to utilize huge pages. Hugepages are <b>used</b> memory, even if there are free hugepages available.'
    },

    'mem.transparent_hugepages': {
        info: 'Transparent HugePages (THP) is backing virtual memory with huge pages, supporting automatic promotion and demotion of page sizes. It works for all applications for anonymous memory mappings and tmpfs/shmem.'
    },

    'mem.hwcorrupt': {
        info: 'The amount of memory with physical corruption problems, identified by <a href="https://en.wikipedia.org/wiki/ECC_memory" target="_blank">ECC</a> and set aside by the kernel so it does not get used.'
    },

    'mem.ecc_ce': {
        info: 'The number of correctable (single-bit) ECC errors. '+
        'These errors do not affect the normal operation of the system '+
        'because they are still being corrected. '+
        'Periodic correctable errors may indicate that one of the memory modules is slowly failing.'
    },

    'mem.ecc_ue': {
        info: 'The number of uncorrectable (multi-bit) ECC errors. '+
        'An uncorrectable error is a fatal issue that will typically lead to an OS crash.'
    },

    'mem.pagetype_global': {
        info: 'The amount of memory available in blocks of certain size.'
    },

    'mem.cachestat_ratio': {
        info: 'The <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#page-cache-ratio" target="_blank">ratio</a> shows the percentage of data accessed directly in memory. Netdata shows the ratio per <a href="#ebpf_apps_cachestat_ratio">application</a> and <a href="#ebpf_services_cachestat_ratio">cgroup (systemd Services)</a> if <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">apps</a> or <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">cgroup (systemd Services)</a> plugins are enabled.' + ebpfChartProvides
    },

    'mem.cachestat_dirties': {
        info: 'Number of <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#dirty-pages" target="_blank">modified pages</a> in <a href="https://en.wikipedia.org/wiki/Page_cache" target="_blank">Linux page cache</a>. Netdata shows the dity page <a href="#ebpf_apps_cachestat_dirties">application</a> and <a href="#ebpf_services_cachestat_dirties">cgroup (systemd Services)</a> if <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">apps</a> or <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">cgroup (systemd Services)</a> plugins are enabled.' + ebpfChartProvides + '<div id="ebpf_global_cachestat_dirty"></div>'
    },

    'mem.cachestat_hits': {
        info: 'Number of <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#page-cache-hits" target="_blank">access</a> to data in <a href="https://en.wikipedia.org/wiki/Page_cache" target="_blank">Linux page cache</a>. Netdata shows the hits per <a href="#ebpf_apps_cachestat_hits">application</a> and <a href="#ebpf_services_cachestat_hits">cgroup (systemd Services)</a> if <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">apps</a> or <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">cgroup (systemd Services)</a> plugins are enabled.' + ebpfChartProvides + '<div id="ebpf_global_cachestat_hits"></div>'
    },

    'mem.cachestat_misses': {
        info: 'Number of <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#page-cache-misses" target="_blank">access</a> to data that was not present in <a href="https://en.wikipedia.org/wiki/Page_cache" target="_blank">Linux page cache</a>. Netdata shows the missed access per <a href="#ebpf_apps_cachestat_misses">application</a> and <a href="#ebpf_services_cachestat_misses">cgroup (systemd Services)</a> if <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">apps</a> or <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">cgroup (systemd Services)</a> plugins are enabled.' + ebpfChartProvides + '<div id="ebpf_global_cachestat_misses"></div>'
    },

    'mem.sync': {
        info: 'Number of calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#file-system-sync" target="_blank">syscalls</a> that sync filesystem metadata or cached. This chart has a relationship with <a href="#menu_filesystem">File systems</a> and Linux <a href="#menu_mem_submenu_page_cache">Page Cache</a>.' + ebpfChartProvides
    },

    'mem.file_sync': {
        info: 'Number of calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#file-sync" target="_blank">syscalls</a> responsible to transfer modified Linux page cache to disk. This chart has a relationship with <a href="#menu_filesystem">File systems</a> and Linux <a href="#menu_mem_submenu_page_cache">Page Cache</a>.' + ebpfChartProvides
    },

    'mem.memory_map': {
        info: 'Number of calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#memory-map-sync" target="_blank">syscall</a> responsible to the in-core copy of a file that was mapped. This chart has a relationship with <a href="#menu_filesystem">File systems</a> and Linux <a href="#menu_mem_submenu_page_cache">Page Cache</a>.' + ebpfChartProvides
    },

    'mem.file_segment': {
        info: 'Number of calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#file-range-sync" target="_blank">syscall</a> responsible to sync file segments. This chart has a relationship with <a href="#menu_filesystem">File systems</a> and Linux <a href="#menu_mem_submenu_page_cache">Page Cache</a>.' + ebpfChartProvides
    },

    'filesystem.dc_hit_ratio': {
        info: 'Percentage of file accesses that were present in the <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#directory-cache" target="_blank">directory cache</a>. Netdata shows directory cache metrics per <a href="#ebpf_apps_dc_hit">application</a> and <a href="#ebpf_services_dc_hit">cgroup (systemd Services)</a> if <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">apps</a> or <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">cgroup (systemd Services)</a> plugins are enabled.' + ebpfChartProvides + '<div id="ebpf_dc_hit_ratio"></div>'
    },

    'filesystem.dc_reference': {
        info: 'Counters of file accesses. <code>Reference</code> is when there is a file access and the file is not present in the directory cache. <code>Miss</code> is when there is file access and the file is not found in the filesystem. <code>Slow</code> is when there is a file access and the file is present in the filesystem but not in the directory cache. Netdata shows directory cache metrics per <a href="#ebpf_apps_dc_reference">application</a> and <a href="#ebpf_services_dc_reference">cgroup (systemd Services)</a> if <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">apps</a> or <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">cgroup (systemd Services)</a> plugins are enabled.' + ebpfChartProvides + '<div id="ebpf_dc_reference"></div>'
    },

    'md.health': {
        info: 'Number of failed devices per MD array. '+
        'Netdata retrieves this data from the <b>[n/m]</b> field of the md status line. '+
        'It means that ideally the array would have <b>n</b> devices however, currently, <b>m</b> devices are in use. '+
        '<code>failed disks</code> is <b>n-m</b>.'
    },
    'md.disks': {
        info: 'Number of devices in use and in the down state. '+
        'Netdata retrieves this data from the <b>[n/m]</b> field of the md status line. '+
        'It means that ideally the array would have <b>n</b> devices however, currently, <b>m</b> devices are in use. '+
        '<code>inuse</code> is <b>m</b>, <code>down</code> is <b>n-m</b>.'
    },
    'md.status': {
        info: 'Completion progress of the ongoing operation.'
    },
    'md.expected_time_until_operation_finish': {
        info: 'Estimated time to complete the ongoing operation. '+
        'The time is only an approximation since the operation speed will vary according to other I/O demands.'
    },
    'md.operation_speed': {
        info: 'Speed of the ongoing operation. '+
        'The system-wide rebuild speed limits are specified in <code>/proc/sys/dev/raid/{speed_limit_min,speed_limit_max}</code> files. '+
        'These options are good for tweaking rebuilt process and may increase overall system load, cpu and memory usage.'
    },
    'md.mismatch_cnt': {
        info: 'When performing <b>check</b> and <b>repair</b>, and possibly when performing <b>resync</b>, md will count the number of errors that are found. '+
        'A count of mismatches is recorded in the <code>sysfs</code> file <code>md/mismatch_cnt</code>. '+
        'This value is the number of sectors that were re-written, or (for <b>check</b>) would have been re-written. '+
        'It may be larger than the number of actual errors by a factor of the number of sectors in a page. '+
        'Mismatches can not be interpreted very reliably on RAID1 or RAID10, especially when the device is used for swap. '+
        'On a truly clean RAID5 or RAID6 array, any mismatches should indicate a hardware problem at some level - '+
        'software issues should never cause such a mismatch. '+
        'For details, see <a href="https://man7.org/linux/man-pages/man4/md.4.html" target="_blank">md(4)</a>.'
    },
    'md.flush': {
        info: 'Number of flush counts per MD array. Based on the eBPF <a href="https://github.com/iovisor/bcc/blob/master/tools/mdflush_example.txt" target="_blank">mdflush</a> from BCC tools.'
    },

    // ------------------------------------------------------------------------
    // IP

    'ip.inerrors': {
        info: '<p>The number of errors encountered during the reception of IP packets.</p>' +
            '</p><b>NoRoutes</b> - packets that were dropped because there was no route to send them. ' +
            '<b>Truncated</b> - packets which is being discarded because the datagram frame didn\'t carry enough data. ' +
            '<b>Checksum</b> - packets that were dropped because they had wrong checksum.</p>'
    },

    'ip.mcast': {
        info: 'Total multicast traffic in the system.'
    },

    'ip.mcastpkts': {
        info: 'Total transferred multicast packets in the system.'
    },

    'ip.bcast': {
        info: 'Total broadcast traffic in the system.'
    },

    'ip.bcastpkts': {
        info: 'Total transferred broadcast packets in the system.'
    },

    'ip.ecnpkts': {
        info: '<p>Total number of received IP packets with ECN bits set in the system.</p>'+
        '<p><b>CEP</b> - congestion encountered. '+
        '<b>NoECTP</b> - non ECN-capable transport. '+
        '<b>ECTP0</b> and <b>ECTP1</b> - ECN capable transport.</p>'
    },

    'ip.inbound_conn': {
        info: 'Number of calls to functions responsible for <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#tcp-inbound-connections" target="_blank">receiving connections</a>.' + ebpfChartProvides
    },

    'ip.tcp_outbound_conn': {
        info: 'Number of calls to TCP functions responsible for <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#tcp-outbound-connections" target="_blank">starting connections</a>. ' +
        'Netdata shows TCP outbound connections metrics per <a href="#ebpf_apps_outbound_conn_ipv4">application</a> and <a href="#ebpf_services_outbound_conn_ipv4">cgroup (systemd Services)</a> if ' +
        '<a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">apps</a> or ' +
        '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">cgroup (systemd Services)</a> plugins are enabled.' + ebpfChartProvides + '<div id="ebpf_global_outbound_conn"></div>'
    },

    'ip.tcp_functions': {
        info: 'Number of calls to TCP functions responsible for <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#tcp-bandwidth-functions" target="_blank">exchanging data</a>. ' +
       'Netdata shows TCP outbound connections metrics per <a href="#ebpf_apps_bandwidth_tcp_sent">application</a> and <a href="#ebpf_services_bandwidth_tcp_sent">cgroup (systemd Services)</a> if ' +
       '<a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">apps</a> or ' +
       '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">cgroup (systemd Services)</a> plugins are enabled.' + ebpfChartProvides + '<div id="ebpf_global_tcp_bandwidth_call"></div>'
    },

    'ip.total_tcp_bandwidth': {
        info: 'Total bytes sent and received with <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#tcp-bandwidth" target="_blank">TCP internal functions</a>. ' +
        'Netdata shows TCP bandwidth metrics per <a href="#ebpf_apps_bandwidth_sent">application</a> and <a href="#ebpf_services_bandwidth_sent">cgroup (systemd Services)</a> if ' +
        '<a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">apps</a> or ' +
        '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">cgroup (systemd Services)</a> plugins are enabled.' + ebpfChartProvides + '<div id="ebpf_global_bandwidth_tcp_bytes"></div>'
    },

    'ip.tcp_error': {
        info: 'Number of failed calls to TCP functions responsible for <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#tcp-bandwidth" target="_blank">TCP bandwidth</a>. ' +
        'Netdata shows TCP error per <a href="#ebpf_apps_tcp_sendmsg_error">application</a> and <a href="#ebpf_services_tcp_sendmsg_error">cgroup (systemd Services)</a> if ' +
        '<a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">apps</a> or ' +
        '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">cgroup (systemd Services)</a> plugins are enabled.' + ebpfChartProvides + '<div id="ebpf_global_tcp_bandwidth_error"></div>'
    },

    'ip.tcp_retransmit': {
        info: 'Number of times a TCP packet was <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#tcp-retransmit" target="_blank">retransmitted</a>. ' +
        'Netdata shows TCP retransmit per <a href="#ebpf_apps_tcp_retransmit">application</a> and <a href="#ebpf_services_tcp_retransmit">cgroup (systemd Services)</a> if ' +
        '<a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">apps</a> or ' +
        '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">cgroup (systemd Services)</a> plugins are enabled.' + ebpfChartProvides + '<div id="ebpf_global_tcp_retransmit"></div>'
    },

    'ip.udp_functions': {
        info: 'Number of calls to UDP functions responsible for <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#udp-functions" target="_blank">exchanging data</a>. ' +
        'Netdata shows TCP outbound connections metrics per <a href="#ebpf_apps_udp_sendmsg">application</a> and <a href="#ebpf_services_udp_sendmsg">cgroup (systemd Services)</a> if ' +
        '<a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">apps</a> or ' +
        '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">cgroup (systemd Services)</a> plugins are enabled.' + ebpfChartProvides + '<div id="ebpf_global_udp_bandwidth_call"></div>'
    },

    'ip.total_udp_bandwidth': {
        info: 'Total bytes sent and received with <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#udp-bandwidth" target="_blank">UDP internal functions</a>. ' +
        'Netdata shows UDP bandwidth metrics per <a href="#ebpf_apps_bandwidth_udp_sendmsg">application</a> and <a href="#ebpf_services_bandwidth_udp_sendmsg">cgroup (systemd Services)</a> if ' +
        '<a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">apps</a> or ' +
        '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">cgroup (systemd Services)</a> plugins are enabled.' + ebpfChartProvides + '<div id="ebpf_global_bandwidth_udp_sendmsg"></div>'
    },

    'ip.udp_error': {
        info: 'Number of failed calls to UDP functions responsible for <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#udp-bandwidth" target="_blank">UDP bandwidth</a>. ' +
        'Netdata shows UDP error per <a href="#ebpf_apps_udp_sendmsg_error">application</a> and <a href="#ebpf_services_udp_sendmsg_error">cgroup (systemd Services)</a> if ' +
        '<a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">apps</a> or ' +
        '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">cgroup (systemd Services)</a> plugins are enabled.' + ebpfChartProvides + '<div id="ebpf_global_udp_bandwidth_error"></div>'
    },

    'ip.tcpreorders': {
        info: '<p>TCP prevents out-of-order packets by either sequencing them in the correct order or '+
        'by requesting the retransmission of out-of-order packets.</p>'+
        '<p><b>Timestamp</b> - detected re-ordering using the timestamp option. '+
        '<b>SACK</b> - detected re-ordering using Selective Acknowledgment algorithm. '+
        '<b>FACK</b> - detected re-ordering using Forward Acknowledgment algorithm. '+
        '<b>Reno</b> - detected re-ordering using Fast Retransmit algorithm.</p>'
    },

    'ip.tcpofo': {
        info: '<p>TCP maintains an out-of-order queue to keep the out-of-order packets in the TCP communication.</p>'+
        '<p><b>InQueue</b> - the TCP layer receives an out-of-order packet and has enough memory to queue it. '+
        '<b>Dropped</b> - the TCP layer receives an out-of-order packet but does not have enough memory, so drops it. '+
        '<b>Merged</b> - the received out-of-order packet has an overlay with the previous packet. '+
        'The overlay part will be dropped. All these packets will also be counted into <b>InQueue</b>. '+
        '<b>Pruned</b> - packets dropped from out-of-order queue because of socket buffer overrun.</p>'
    },

    'ip.tcpsyncookies': {
        info: '<p><a href="https://en.wikipedia.org/wiki/SYN_cookies" target="_blank">SYN cookies</a> '+
        'are used to mitigate SYN flood.</p>'+
        '<p><b>Received</b> - after sending a SYN cookie, it came back to us and passed the check. '+
        '<b>Sent</b> - an application was not able to accept a connection fast enough, so the kernel could not store '+
        'an entry in the queue for this connection. Instead of dropping it, it sent a SYN cookie to the client. '+
        '<b>Failed</b> - the MSS decoded from the SYN cookie is invalid. When this counter is incremented, '+
        'the received packet won’t be treated as a SYN cookie.</p>'
    },

    'ip.tcpmemorypressures': {
        info: 'The number of times a socket was put in memory pressure due to a non fatal memory allocation failure '+
        '(the kernel attempts to work around this situation by reducing the send buffers, etc).'
    },

    'ip.tcpconnaborts': {
        info: '<p>TCP connection aborts.</p>'+
        '<p><b>BadData</b> - happens while the connection is on FIN_WAIT1 and the kernel receives a packet '+
        'with a sequence number beyond the last one for this connection - '+
        'the kernel responds with RST (closes the connection). '+
        '<b>UserClosed</b> - happens when the kernel receives data on an already closed connection and '+
        'responds with RST. '+
        '<b>NoMemory</b> - happens when there are too many orphaned sockets (not attached to an fd) and '+
        'the kernel has to drop a connection - sometimes it will send an RST, sometimes it won\'t. '+
        '<b>Timeout</b> - happens when a connection times out. '+
        '<b>Linger</b> - happens when the kernel killed a socket that was already closed by the application and '+
        'lingered around for long enough. '+
        '<b>Failed</b> - happens when the kernel attempted to send an RST but failed because there was no memory available.</p>'
    },

    'ip.tcp_syn_queue': {
        info: '<p>The SYN queue of the kernel tracks TCP handshakes until connections get fully established. ' +
            'It overflows when too many incoming TCP connection requests hang in the half-open state and the server ' +
            'is not configured to fall back to SYN cookies. Overflows are usually caused by SYN flood DoS attacks.</p>' +
            '<p><b>Drops</b> - number of connections dropped because the SYN queue was full and SYN cookies were disabled. ' +
            '<b>Cookies</b> - number of SYN cookies sent because the SYN queue was full.</p>'
    },

    'ip.tcp_accept_queue': {
        info: '<p>The accept queue of the kernel holds the fully established TCP connections, waiting to be handled ' +
            'by the listening application.</p>'+
            '<b>Overflows</b> - the number of established connections that could not be handled because '+
            'the receive queue of the listening application was full. '+
            '<b>Drops</b> - number of incoming connections that could not be handled, including SYN floods, '+
            'overflows, out of memory, security issues, no route to destination, reception of related ICMP messages, '+
            'socket is broadcast or multicast.</p>'
    },


    // ------------------------------------------------------------------------
    // IPv4

    'ipv4.packets': {
        info: '<p>IPv4 packets statistics for this host.</p>'+
        '<p><b>Received</b> - packets received by the IP layer. '+
        'This counter will be increased even if the packet is dropped later. '+
        '<b>Sent</b> - packets sent via IP layer, for both single cast and multicast packets. '+
        'This counter does not include any packets counted in <b>Forwarded</b>. '+
        '<b>Forwarded</b> - input packets for which this host was not their final IP destination, '+
        'as a result of which an attempt was made to find a route to forward them to that final destination. '+
        'In hosts which do not act as IP Gateways, this counter will include only those packets which were '+
        '<a href="https://en.wikipedia.org/wiki/Source_routing" target="_blank">Source-Routed</a> '+
        'and the Source-Route option processing was successful. '+
        '<b>Delivered</b> - packets delivered to the upper layer protocols, e.g. TCP, UDP, ICMP, and so on.</p>'
    },

    'ipv4.fragsout': {
        info: '<p><a href="https://en.wikipedia.org/wiki/IPv4#Fragmentation" target="_blank">IPv4 fragmentation</a> '+
        'statistics for this system.</p>'+
        '<p><b>OK</b> - packets that have been successfully fragmented. '+
        '<b>Failed</b> - packets that have been discarded because they needed to be fragmented '+
        'but could not be, e.g. due to <i>Don\'t Fragment</i> (DF) flag was set. '+
        '<b>Created</b> - fragments that have been generated as a result of fragmentation.</p>'
    },

    'ipv4.fragsin': {
        info: '<p><a href="https://en.wikipedia.org/wiki/IPv4#Reassembly" target="_blank">IPv4 reassembly</a> '+
        'statistics for this system.</p>'+
        '<p><b>OK</b> - packets that have been successfully reassembled. '+
        '<b>Failed</b> - failures detected by the IP reassembly algorithm. '+
        'This is not necessarily a count of discarded IP fragments since some algorithms '+
        'can lose track of the number of fragments by combining them as they are received. '+
        '<b>All</b> - received IP fragments which needed to be reassembled.</p>'
    },

    'ipv4.errors': {
        info: '<p>The number of discarded IPv4 packets.</p>'+
        '<p><b>InDiscards</b>, <b>OutDiscards</b> - inbound and outbound packets which were chosen '+
        'to be discarded even though no errors had been '+
        'detected to prevent their being deliverable to a higher-layer protocol. '+
        '<b>InHdrErrors</b> - input packets that have been discarded due to errors in their IP headers, including '+
        'bad checksums, version number mismatch, other format errors, time-to-live exceeded, '+
        'errors discovered in processing their IP options, etc. '+
        '<b>OutNoRoutes</b> - packets that have been discarded because no route could be found '+
        'to transmit them to their destination. This includes any packets which a host cannot route '+
        'because all of its default gateways are down. '+
        '<b>InAddrErrors</b> - input packets that have been discarded due to invalid IP address or '+
        'the destination IP address is not a local address and IP forwarding is not enabled. '+
        '<b>InUnknownProtos</b> - input packets which were discarded because of an unknown or unsupported protocol.</p>'
    },

    'ipv4.icmp': {
        info: '<p>The number of transferred IPv4 ICMP messages.</p>'+
        '<p><b>Received</b>, <b>Sent</b> - ICMP messages which the host received and attempted to send. '+
        'Both these counters include errors.</p>'
    },

    'ipv4.icmp_errors': {
        info: '<p>The number of IPv4 ICMP errors.</p>'+
        '<p><b>InErrors</b> - received ICMP messages but determined as having ICMP-specific errors, '+
        'e.g. bad ICMP checksums, bad length, etc. '+
        '<b>OutErrors</b> - ICMP messages which this host did not send due to '+
        'problems discovered within ICMP such as a lack of buffers. '+
        'This counter does not include errors discovered outside the ICMP layer '+
        'such as the inability of IP to route the resultant datagram. '+
        '<b>InCsumErrors</b> - received ICMP messages with bad checksum.</p>'
    },

    'ipv4.icmpmsg': {
        info: 'The number of transferred '+
        '<a href="https://www.iana.org/assignments/icmp-parameters/icmp-parameters.xhtml" target="_blank">IPv4 ICMP control messages</a>.'
    },

    'ipv4.udppackets': {
        info: 'The number of transferred UDP packets.'
    },

    'ipv4.udperrors': {
        info: '<p>The number of errors encountered during transferring UDP packets.</p>'+
        '<b>RcvbufErrors</b> - receive buffer is full. '+
        '<b>SndbufErrors</b> - send buffer is full, no kernel memory available, or '+
        'the IP layer reported an error when trying to send the packet and no error queue has been setup. '+
        '<b>InErrors</b> - that is an aggregated counter for all errors, excluding <b>NoPorts</b>. '+
        '<b>NoPorts</b> - no application is listening at the destination port. '+
        '<b>InCsumErrors</b> - a UDP checksum failure is detected. '+
        '<b>IgnoredMulti</b> - ignored multicast packets.'
    },

    'ipv4.udplite': {
        info: 'The number of transferred UDP-Lite packets.'
    },

    'ipv4.udplite_errors': {
        info: '<p>The number of errors encountered during transferring UDP-Lite packets.</p>'+
        '<b>RcvbufErrors</b> - receive buffer is full. '+
        '<b>SndbufErrors</b> - send buffer is full, no kernel memory available, or '+
        'the IP layer reported an error when trying to send the packet and no error queue has been setup. '+
        '<b>InErrors</b> - that is an aggregated counter for all errors, excluding <b>NoPorts</b>. '+
        '<b>NoPorts</b> - no application is listening at the destination port. '+
        '<b>InCsumErrors</b> - a UDP checksum failure is detected. '+
        '<b>IgnoredMulti</b> - ignored multicast packets.'
    },

    'ipv4.tcppackets': {
        info: '<p>The number of packets transferred by the TCP layer.</p>'+
        '</p><b>Received</b> - received packets, including those received in error, '+
        'such as checksum error, invalid TCP header, and so on. '+
        '<b>Sent</b> - sent packets, excluding the retransmitted packets. '+
        'But it includes the SYN, ACK, and RST packets.</p>'
    },

    'ipv4.tcpsock': {
        info: 'The number of TCP connections for which the current state is either ESTABLISHED or CLOSE-WAIT. '+
        'This is a snapshot of the established connections at the time of measurement '+
        '(i.e. a connection established and a connection disconnected within the same iteration will not affect this metric).'
    },

    'ipv4.tcpopens': {
        info: '<p>TCP connection statistics.</p>'+
        '<p><b>Active</b> - number of outgoing TCP connections attempted by this host. '+
         '<b>Passive</b> - number of incoming TCP connections accepted by this host.</p>'
    },

    'ipv4.tcperrors': {
        info: '<p>TCP errors.</p>'+
        '<p><b>InErrs</b> - TCP segments received in error '+
        '(including header too small, checksum errors, sequence errors, bad packets - for both IPv4 and IPv6). '+
        '<b>InCsumErrors</b> - TCP segments received with checksum errors (for both IPv4 and IPv6). '+
        '<b>RetransSegs</b> - TCP segments retransmitted.</p>'
    },

    'ipv4.tcphandshake': {
        info: '<p>TCP handshake statistics.</p>'+
        '<p><b>EstabResets</b> - established connections resets '+
        '(i.e. connections that made a direct transition from ESTABLISHED or CLOSE_WAIT to CLOSED). '+
        '<b>OutRsts</b> - TCP segments sent, with the RST flag set (for both IPv4 and IPv6). '+
        '<b>AttemptFails</b> - number of times TCP connections made a direct transition from either '+
        'SYN_SENT or SYN_RECV to CLOSED, plus the number of times TCP connections made a direct transition '+
        'from the SYN_RECV to LISTEN. '+
        '<b>SynRetrans</b> - shows retries for new outbound TCP connections, '+
        'which can indicate general connectivity issues or backlog on the remote host.</p>'
    },

    'ipv4.sockstat_sockets': {
        info: 'The total number of used sockets for all '+
        '<a href="https://man7.org/linux/man-pages/man7/address_families.7.html" target="_blank">address families</a> '+
        'in this system.'
    },

    'ipv4.sockstat_tcp_sockets': {
        info: '<p>The number of TCP sockets in the system in certain '+
        '<a href="https://en.wikipedia.org/wiki/Transmission_Control_Protocol#Protocol_operation" target="_blank">states</a>.</p>'+
        '<p><b>Alloc</b> - in any TCP state. '+
        '<b>Orphan</b> - no longer attached to a socket descriptor in any user processes, '+
        'but for which the kernel is still required to maintain state in order to complete the transport protocol. '+
        '<b>InUse</b> - in any TCP state, excluding TIME-WAIT and CLOSED. '+
        '<b>TimeWait</b> - in the TIME-WAIT state.</p>'
    },

    'ipv4.sockstat_tcp_mem': {
        info: 'The amount of memory used by allocated TCP sockets.'
    },

    'ipv4.sockstat_udp_sockets': {
        info: 'The number of used UDP sockets.'
    },

    'ipv4.sockstat_udp_mem': {
        info: 'The amount of memory used by allocated UDP sockets.'
    },

    'ipv4.sockstat_udplite_sockets': {
        info: 'The number of used UDP-Lite sockets.'
    },

    'ipv4.sockstat_raw_sockets': {
        info: 'The number of used <a href="https://en.wikipedia.org/wiki/Network_socket#Types" target="_blank"> raw sockets</a>.'
    },

    'ipv4.sockstat_frag_sockets': {
        info: 'The number of entries in hash tables that are used for packet reassembly.'
    },

    'ipv4.sockstat_frag_mem': {
        info: 'The amount of memory used for packet reassembly.'
    },

    // ------------------------------------------------------------------------
    // IPv6

    'ipv6.packets': {
        info: '<p>IPv6 packet statistics for this host.</p>'+
        '<p><b>Received</b> - packets received by the IP layer. '+
        'This counter will be increased even if the packet is dropped later. '+
        '<b>Sent</b> - packets sent via IP layer, for both single cast and multicast packets. '+
        'This counter does not include any packets counted in <b>Forwarded</b>. '+
        '<b>Forwarded</b> - input packets for which this host was not their final IP destination, '+
        'as a result of which an attempt was made to find a route to forward them to that final destination. '+
        'In hosts which do not act as IP Gateways, this counter will include only those packets which were '+
        '<a href="https://en.wikipedia.org/wiki/Source_routing" target="_blank">Source-Routed</a> '+
        'and the Source-Route option processing was successful. '+
        '<b>Delivers</b> - packets delivered to the upper layer protocols, e.g. TCP, UDP, ICMP, and so on.</p>'
    },

    'ipv6.fragsout': {
        info: '<p><a href="https://en.wikipedia.org/wiki/IP_fragmentation" target="_blank">IPv6 fragmentation</a> '+
        'statistics for this system.</p>'+
        '<p><b>OK</b> - packets that have been successfully fragmented. '+
        '<b>Failed</b> - packets that have been discarded because they needed to be fragmented '+
        'but could not be, e.g. due to <i>Don\'t Fragment</i> (DF) flag was set. '+
        '<b>All</b> - fragments that have been generated as a result of fragmentation.</p>'
    },

    'ipv6.fragsin': {
        info: '<p><a href="https://en.wikipedia.org/wiki/IP_fragmentation" target="_blank">IPv6 reassembly</a> '+
        'statistics for this system.</p>'+
        '<p><b>OK</b> - packets that have been successfully reassembled. '+
        '<b>Failed</b> - failures detected by the IP reassembly algorithm. '+
        'This is not necessarily a count of discarded IP fragments since some algorithms '+
        'can lose track of the number of fragments by combining them as they are received. '+
        '<b>Timeout</b> - reassembly timeouts detected. '+
        '<b>All</b> - received IP fragments which needed to be reassembled.</p>'
    },

    'ipv6.errors': {
        info: '<p>The number of discarded IPv6 packets.</p>'+
        '<p><b>InDiscards</b>, <b>OutDiscards</b> - packets which were chosen to be discarded even though '+
        'no errors had been detected to prevent their being deliverable to a higher-layer protocol. '+
        '<b>InHdrErrors</b> - errors in IP headers, including bad checksums, version number mismatch, '+
        'other format errors, time-to-live exceeded, etc. '+
        '<b>InAddrErrors</b> - invalid IP address or the destination IP address is not a local address and '+
        'IP forwarding is not enabled. '+
        '<b>InUnknownProtos</b> - unknown or unsupported protocol. '+
        '<b>InTooBigErrors</b> - the size exceeded the link MTU. '+
        '<b>InTruncatedPkts</b> - packet frame did not carry enough data. '+
        '<b>InNoRoutes</b> - no route could be found while forwarding. '+
        '<b>OutNoRoutes</b> - no route could be found for packets generated by this host.</p>'
    },

    'ipv6.udppackets': {
        info: 'The number of transferred UDP packets.'
    },

    'ipv6.udperrors': {
        info: '<p>The number of errors encountered during transferring UDP packets.</p>'+
        '<b>RcvbufErrors</b> - receive buffer is full. '+
        '<b>SndbufErrors</b> - send buffer is full, no kernel memory available, or '+
        'the IP layer reported an error when trying to send the packet and no error queue has been setup. '+
        '<b>InErrors</b> - that is an aggregated counter for all errors, excluding <b>NoPorts</b>. '+
        '<b>NoPorts</b> - no application is listening at the destination port. '+
        '<b>InCsumErrors</b> - a UDP checksum failure is detected. '+
        '<b>IgnoredMulti</b> - ignored multicast packets.'
    },

    'ipv6.udplitepackets': {
        info: 'The number of transferred UDP-Lite packets.'
    },

    'ipv6.udpliteerrors': {
        info: '<p>The number of errors encountered during transferring UDP-Lite packets.</p>'+
        '<p><b>RcvbufErrors</b> - receive buffer is full. '+
        '<b>SndbufErrors</b> - send buffer is full, no kernel memory available, or '+
        'the IP layer reported an error when trying to send the packet and no error queue has been setup. '+
        '<b>InErrors</b> - that is an aggregated counter for all errors, excluding <b>NoPorts</b>. '+
        '<b>NoPorts</b> - no application is listening at the destination port. '+
        '<b>InCsumErrors</b> - a UDP checksum failure is detected.</p>'
    },

    'ipv6.mcast': {
        info: 'Total IPv6 multicast traffic.'
    },

    'ipv6.bcast': {
        info: 'Total IPv6 broadcast traffic.'
    },

    'ipv6.mcastpkts': {
        info: 'Total transferred IPv6 multicast packets.'
    },

    'ipv6.icmp': {
        info: '<p>The number of transferred ICMPv6 messages.</p>'+
        '<p><b>Received</b>, <b>Sent</b> - ICMP messages which the host received and attempted to send. '+
        'Both these counters include errors.</p>'
    },

    'ipv6.icmpredir': {
        info: 'The number of transferred ICMPv6 Redirect messages. '+
        'These messages inform a host to update its routing information (to send packets on an alternative route).'
    },

    'ipv6.icmpechos': {
        info: 'The number of ICMPv6 Echo messages.'
    },

    'ipv6.icmperrors': {
        info: '<p>The number of ICMPv6 errors and '+
        '<a href="https://www.rfc-editor.org/rfc/rfc4443.html#section-3" target="_blank">error messages</a>.</p>'+
        '<p><b>InErrors</b>, <b>OutErrors</b> - bad ICMP messages (bad ICMP checksums, bad length, etc.). '+
        '<b>InCsumErrors</b> - wrong checksum.</p>'
    },

    'ipv6.groupmemb': {
        info: '<p>The number of transferred ICMPv6 Group Membership messages.</p>'+
        '<p> Multicast routers send Group Membership Query messages to learn which groups have members on each of their '+
        'attached physical networks. Host computers respond by sending a Group Membership Report for each '+
        'multicast group joined by the host. A host computer can also send a Group Membership Report when '+
        'it joins a new multicast group. '+
        'Group Membership Reduction messages are sent when a host computer leaves a multicast group.</p>'
    },

    'ipv6.icmprouter': {
        info: '<p>The number of transferred ICMPv6 '+
        '<a href="https://en.wikipedia.org/wiki/Neighbor_Discovery_Protocol" target="_blank">Router Discovery</a> messages.</p>'+
        '<p>Router <b>Solicitations</b> message is sent from a computer host to any routers on the local area network '+
        'to request that they advertise their presence on the network. '+
        'Router <b>Advertisement</b> message is sent by a router on the local area network to announce its IP address '+
        'as available for routing.</p>'
    },

    'ipv6.icmpneighbor': {
        info: '<p>The number of transferred ICMPv6 '+
        '<a href="https://en.wikipedia.org/wiki/Neighbor_Discovery_Protocol" target="_blank">Neighbour Discovery</a> messages.</p>'+
        '<p>Neighbor <b>Solicitations</b> are used by nodes to determine the link layer address '+
        'of a neighbor, or to verify that a neighbor is still reachable via a cached link layer address. '+
        'Neighbor <b>Advertisements</b> are used by nodes to respond to a Neighbor Solicitation message.</p>'
    },

    'ipv6.icmpmldv2': {
        info: 'The number of transferred ICMPv6 '+
        '<a href="https://en.wikipedia.org/wiki/Multicast_Listener_Discovery" target="_blank">Multicast Listener Discovery</a> (MLD) messages.'
    },

    'ipv6.icmptypes': {
        info: 'The number of transferred ICMPv6 messages of '+
        '<a href="https://en.wikipedia.org/wiki/Internet_Control_Message_Protocol_for_IPv6#Types" target="_blank">certain types</a>.'
    },

    'ipv6.ect': {
        info: '<p>Total number of received IPv6 packets with ECN bits set in the system.</p>'+
        '<p><b>CEP</b> - congestion encountered. '+
        '<b>NoECTP</b> - non ECN-capable transport. '+
        '<b>ECTP0</b> and <b>ECTP1</b> - ECN capable transport.</p>'
    },

    'ipv6.sockstat6_tcp_sockets': {
        info: 'The number of TCP sockets in any '+
        '<a href="https://en.wikipedia.org/wiki/Transmission_Control_Protocol#Protocol_operation" target="_blank">state</a>, '+
        'excluding TIME-WAIT and CLOSED.'
    },

    'ipv6.sockstat6_udp_sockets': {
        info: 'The number of used UDP sockets.'
    },

    'ipv6.sockstat6_udplite_sockets': {
        info: 'The number of used UDP-Lite sockets.'
    },

    'ipv6.sockstat6_raw_sockets': {
        info: 'The number of used <a href="https://en.wikipedia.org/wiki/Network_socket#Types" target="_blank"> raw sockets</a>.'
    },

    'ipv6.sockstat6_frag_sockets': {
        info: 'The number of entries in hash tables that are used for packet reassembly.'
    },


    // ------------------------------------------------------------------------
    // SCTP

    'sctp.established': {
        info: 'The number of associations for which the current state is either '+
        'ESTABLISHED, SHUTDOWN-RECEIVED or SHUTDOWN-PENDING.'
    },

    'sctp.transitions': {
        info: '<p>The number of times that associations have made a direct transition between states.</p>'+
        '<p><b>Active</b> - from COOKIE-ECHOED to ESTABLISHED. The upper layer initiated the association attempt. '+
        '<b>Passive</b> - from CLOSED to ESTABLISHED. The remote endpoint initiated the association attempt. '+
        '<b>Aborted</b> - from any state to CLOSED using the primitive ABORT. Ungraceful termination of the association. '+
        '<b>Shutdown</b> - from SHUTDOWN-SENT or SHUTDOWN-ACK-SENT to CLOSED. Graceful termination of the association.</p>'
    },

    'sctp.packets': {
        info: '<p>The number of transferred SCTP packets.</p>'+
        '<p><b>Received</b> - includes duplicate packets. '+
        '<b>Sent</b> - includes retransmitted DATA chunks.</p>'
    },

    'sctp.packet_errors': {
        info: '<p>The number of errors encountered during receiving SCTP packets.</p>'+
        '<p><b>Invalid</b> - packets for which the receiver was unable to identify an appropriate association. '+
        '<b>Checksum</b> - packets with an invalid checksum.</p>'
    },

    'sctp.fragmentation': {
        info: '<p>The number of fragmented and reassembled SCTP messages.</p>'+
        '<p><b>Reassembled</b> - reassembled user messages, after conversion into DATA chunks. '+
        '<b>Fragmented</b> - user messages that have to be fragmented because of the MTU.</p>'
    },

    'sctp.chunks': {
        info: 'The number of transferred control, ordered, and unordered DATA chunks. '+
        'Retransmissions and duplicates are not included.'
    },

    // ------------------------------------------------------------------------
    // Netfilter Connection Tracker

    'netfilter.conntrack_sockets': {
        info: 'The number of entries in the conntrack table.'
    },

    'netfilter.conntrack_new': {
        info: '<p>Packet tracking statistics. <b>New</b> (since v4.9) and <b>Ignore</b> (since v5.10) are hardcoded to zeros in the latest kernel.</p>'+
        '<p><b>New</b> - conntrack entries added which were not expected before. '+
        '<b>Ignore</b> - packets seen which are already connected to a conntrack entry. '+
        '<b>Invalid</b> - packets seen which can not be tracked.</p>'
    },

    'netfilter.conntrack_changes': {
        info: '<p>The number of changes in conntrack tables.</p>'+
        '<p><b>Inserted</b>, <b>Deleted</b> - conntrack entries which were inserted or removed. '+
        '<b>Delete-list</b> - conntrack entries which were put to dying list.</p>'
    },

    'netfilter.conntrack_expect': {
        info: '<p>The number of events in the "expect" table. '+
        'Connection tracking expectations are the mechanism used to "expect" RELATED connections to existing ones. '+
        'An expectation is a connection that is expected to happen in a period of time.</p>'+
        '<p><b>Created</b>, <b>Deleted</b> - conntrack entries which were inserted or removed. '+
        '<b>New</b> - conntrack entries added after an expectation for them was already present.</p>'
    },

    'netfilter.conntrack_search': {
        info: '<p>Conntrack table lookup statistics.</p>'+
        '<p><b>Searched</b> - conntrack table lookups performed. '+
        '<b>Restarted</b> - conntrack table lookups which had to be restarted due to hashtable resizes. '+
        '<b>Found</b> - conntrack table lookups which were successful.</p>'
    },

    'netfilter.conntrack_errors': {
        info: '<p>Conntrack errors.</p>'+
        '<p><b>IcmpError</b> - packets which could not be tracked due to error situation. '+
        '<b>InsertFailed</b> - entries for which list insertion was attempted but failed '+
        '(happens if the same entry is already present). '+
        '<b>Drop</b> - packets dropped due to conntrack failure. '+
        'Either new conntrack entry allocation failed, or protocol helper dropped the packet. '+
        '<b>EarlyDrop</b> - dropped conntrack entries to make room for new ones, if maximum table size was reached.</p>'
    },

    'netfilter.synproxy_syn_received': {
        info: 'The number of initial TCP SYN packets received from clients.'
    },

    'netfilter.synproxy_conn_reopened': {
        info: 'The number of reopened connections by new TCP SYN packets directly from the TIME-WAIT state.'
    },

    'netfilter.synproxy_cookies': {
        info: '<p>SYNPROXY cookie statistics.</p>'+
        '<p><b>Valid</b>, <b>Invalid</b> - result of cookie validation in TCP ACK packets received from clients. '+
        '<b>Retransmits</b> - TCP SYN packets retransmitted to the server. '+
        'It happens when the client repeats TCP ACK and the connection to the server is not yet established.</p>'
    },

    // ------------------------------------------------------------------------
    // APPS (Applications, Groups, Users)

    // APPS cpu
    'apps.cpu': {
        info: 'Total CPU utilization (all cores). It includes user, system and guest time.'
    },
    'groups.cpu': {
        info: 'Total CPU utilization (all cores). It includes user, system and guest time.'
    },
    'users.cpu': {
        info: 'Total CPU utilization (all cores). It includes user, system and guest time.'
    },

    'apps.cpu_user': {
        info: 'The amount of time the CPU was busy executing code in '+
        '<a href="https://en.wikipedia.org/wiki/CPU_modes#Mode_types" target="_blank">user mode</a> (all cores).'
    },
    'groups.cpu_user': {
        info: 'The amount of time the CPU was busy executing code in '+
        '<a href="https://en.wikipedia.org/wiki/CPU_modes#Mode_types" target="_blank">user mode</a> (all cores).'
    },
    'users.cpu_user': {
        info: 'The amount of time the CPU was busy executing code in '+
        '<a href="https://en.wikipedia.org/wiki/CPU_modes#Mode_types" target="_blank">user mode</a> (all cores).'
    },

    'apps.cpu_system': {
        info: 'The amount of time the CPU was busy executing code in '+
        '<a href="https://en.wikipedia.org/wiki/CPU_modes#Mode_types" target="_blank">kernel mode</a> (all cores).'
    },
    'groups.cpu_system': {
        info: 'The amount of time the CPU was busy executing code in '+
        '<a href="https://en.wikipedia.org/wiki/CPU_modes#Mode_types" target="_blank">kernel mode</a> (all cores).'
    },
    'users.cpu_system': {
        info: 'The amount of time the CPU was busy executing code in '+
        '<a href="https://en.wikipedia.org/wiki/CPU_modes#Mode_types" target="_blank">kernel mode</a> (all cores).'
    },

    'apps.cpu_guest': {
        info: 'The amount of time spent running a virtual CPU for a guest operating system (all cores).'
    },
    'groups.cpu_guest': {
        info: 'The amount of time spent running a virtual CPU for a guest operating system (all cores).'
    },
    'users.cpu_guest': {
        info: 'The amount of time spent running a virtual CPU for a guest operating system (all cores).'
    },

    // APPS disk
    'apps.preads': {
        info: 'The amount of data that has been read from the storage layer. '+
        'Actual physical disk I/O was required.'
    },
    'groups.preads': {
        info: 'The amount of data that has been read from the storage layer. '+
        'Actual physical disk I/O was required.'
    },
    'users.preads': {
        info: 'The amount of data that has been read from the storage layer. '+
        'Actual physical disk I/O was required.'
    },

    'apps.pwrites': {
        info: 'The amount of data that has been written to the storage layer. '+
        'Actual physical disk I/O was required.'
    },
    'groups.pwrites': {
        info: 'The amount of data that has been written to the storage layer. '+
        'Actual physical disk I/O was required.'
    },
    'users.pwrites': {
        info: 'The amount of data that has been written to the storage layer. '+
        'Actual physical disk I/O was required.'
    },

    'apps.lreads': {
        info: 'The amount of data that has been read from the storage layer. '+
        'It includes things such as terminal I/O and is unaffected by whether or '+
        'not actual physical disk I/O was required '+
        '(the read might have been satisfied from pagecache).'
    },
    'groups.lreads': {
        info: 'The amount of data that has been read from the storage layer. '+
        'It includes things such as terminal I/O and is unaffected by whether or '+
        'not actual physical disk I/O was required '+
        '(the read might have been satisfied from pagecache).'
    },
    'users.lreads': {
        info: 'The amount of data that has been read from the storage layer. '+
        'It includes things such as terminal I/O and is unaffected by whether or '+
        'not actual physical disk I/O was required '+
        '(the read might have been satisfied from pagecache).'
    },

    'apps.lwrites': {
        info: 'The amount of data that has been written or shall be written to the storage layer. '+
        'It includes things such as terminal I/O and is unaffected by whether or '+
        'not actual physical disk I/O was required.'
    },
    'groups.lwrites': {
        info: 'The amount of data that has been written or shall be written to the storage layer. '+
        'It includes things such as terminal I/O and is unaffected by whether or '+
        'not actual physical disk I/O was required.'
    },
    'users.lwrites': {
        info: 'The amount of data that has been written or shall be written to the storage layer. '+
        'It includes things such as terminal I/O and is unaffected by whether or '+
        'not actual physical disk I/O was required.'
    },

    'apps.files': {
        info: 'The number of open files and directories.'
    },
    'groups.files': {
        info: 'The number of open files and directories.'
    },
    'users.files': {
        info: 'The number of open files and directories.'
    },

    // APPS mem
    'apps.mem': {
        info: 'Real memory (RAM) used by applications. This does not include shared memory.'
    },
    'groups.mem': {
        info: 'Real memory (RAM) used per user group. This does not include shared memory.'
    },
    'users.mem': {
        info: 'Real memory (RAM) used per user. This does not include shared memory.'
    },

    'apps.vmem': {
        info: 'Virtual memory allocated by applications. '+
        'Check <a href="https://github.com/netdata/netdata/tree/master/daemon#virtual-memory" target="_blank">this article</a> for more information.'
    },
    'groups.vmem': {
        info: 'Virtual memory allocated per user group since the Netdata restart. Please check <a href="https://github.com/netdata/netdata/tree/master/daemon#virtual-memory" target="_blank">this article</a> for more information.'
    },
    'users.vmem': {
        info: 'Virtual memory allocated per user since the Netdata restart. Please check <a href="https://github.com/netdata/netdata/tree/master/daemon#virtual-memory" target="_blank">this article</a> for more information.'
    },

    'apps.minor_faults': {
        info: 'The number of <a href="https://en.wikipedia.org/wiki/Page_fault#Minor" target="_blank">minor faults</a> '+
        'which have not required loading a memory page from the disk. '+
        'Minor page faults occur when a process needs data that is in memory and is assigned to another process. '+
        'They share memory pages between multiple processes – '+
        'no additional data needs to be read from disk to memory.'
    },
    'groups.minor_faults': {
        info: 'The number of <a href="https://en.wikipedia.org/wiki/Page_fault#Minor" target="_blank">minor faults</a> '+
        'which have not required loading a memory page from the disk. '+
        'Minor page faults occur when a process needs data that is in memory and is assigned to another process. '+
        'They share memory pages between multiple processes – '+
        'no additional data needs to be read from disk to memory.'
    },
    'users.minor_faults': {
        info: 'The number of <a href="https://en.wikipedia.org/wiki/Page_fault#Minor" target="_blank">minor faults</a> '+
        'which have not required loading a memory page from the disk. '+
        'Minor page faults occur when a process needs data that is in memory and is assigned to another process. '+
        'They share memory pages between multiple processes – '+
        'no additional data needs to be read from disk to memory.'
    },

    // APPS processes
    'apps.threads': {
        info: 'The number of <a href="https://en.wikipedia.org/wiki/Thread_(computing)" target="_blank">threads</a>.'
    },
    'groups.threads': {
        info: 'The number of <a href="https://en.wikipedia.org/wiki/Thread_(computing)" target="_blank">threads</a>.'
    },
    'users.threads': {
        info: 'The number of <a href="https://en.wikipedia.org/wiki/Thread_(computing)" target="_blank">threads</a>.'
    },

    'apps.processes': {
        info: 'The number of <a href="https://en.wikipedia.org/wiki/Process_(computing)" target="_blank">processes</a>.'
    },
    'groups.processes': {
        info: 'The number of <a href="https://en.wikipedia.org/wiki/Process_(computing)" target="_blank">processes</a>.'
    },
    'users.processes': {
        info: 'The number of <a href="https://en.wikipedia.org/wiki/Process_(computing)" target="_blank">processes</a>.'
    },

    'apps.uptime': {
        info: 'The period of time within which at least one process in the group has been running.'
    },
    'groups.uptime': {
        info: 'The period of time within which at least one process in the group has been running.'
    },
    'users.uptime': {
        info: 'The period of time within which at least one process in the group has been running.'
    },

    'apps.uptime_min': {
        info: 'The shortest uptime among processes in the group.'
    },
    'groups.uptime_min': {
        info: 'The shortest uptime among processes in the group.'
    },
    'users.uptime_min': {
        info: 'The shortest uptime among processes in the group.'
    },

    'apps.uptime_avg': {
        info: 'The average uptime of processes in the group.'
    },
    'groups.uptime_avg': {
        info: 'The average uptime of processes in the group.'
    },
    'users.uptime_avg': {
        info: 'The average uptime of processes in the group.'
    },

    'apps.uptime_max': {
        info: 'The longest uptime among processes in the group.'
    },
    'groups.uptime_max': {
        info: 'The longest uptime among processes in the group.'
    },
    'users.uptime_max': {
        info: 'The longest uptime among processes in the group.'
    },

    'apps.pipes': {
        info: 'The number of open '+
        '<a href="https://en.wikipedia.org/wiki/Anonymous_pipe#Unix" target="_blank">pipes</a>. '+
        'A pipe is a unidirectional data channel that can be used for interprocess communication.'
    },
    'groups.pipes': {
        info: 'The number of open '+
        '<a href="https://en.wikipedia.org/wiki/Anonymous_pipe#Unix" target="_blank">pipes</a>. '+
        'A pipe is a unidirectional data channel that can be used for interprocess communication.'
    },
    'users.pipes': {
        info: 'The number of open '+
        '<a href="https://en.wikipedia.org/wiki/Anonymous_pipe#Unix" target="_blank">pipes</a>. '+
        'A pipe is a unidirectional data channel that can be used for interprocess communication.'
    },

    // APPS swap
    'apps.swap': {
        info: 'The amount of swapped-out virtual memory by anonymous private pages. '+
        'This does not include shared swap memory.'
    },
    'groups.swap': {
        info: 'The amount of swapped-out virtual memory by anonymous private pages. '+
        'This does not include shared swap memory.'
    },
    'users.swap': {
        info: 'The amount of swapped-out virtual memory by anonymous private pages. '+
        'This does not include shared swap memory.'
    },

    'apps.major_faults': {
        info: 'The number of <a href="https://en.wikipedia.org/wiki/Page_fault#Major" target="_blank">major faults</a> '+
        'which have required loading a memory page from the disk. '+
        'Major page faults occur because of the absence of the required page from the RAM. '+
        'They are expected when a process starts or needs to read in additional data and '+
        'in these cases do not indicate a problem condition. '+
        'However, a major page fault can also be the result of reading memory pages that have been written out '+
        'to the swap file, which could indicate a memory shortage.'
    },
    'groups.major_faults': {
        info: 'The number of <a href="https://en.wikipedia.org/wiki/Page_fault#Major" target="_blank">major faults</a> '+
        'which have required loading a memory page from the disk. '+
        'Major page faults occur because of the absence of the required page from the RAM. '+
        'They are expected when a process starts or needs to read in additional data and '+
        'in these cases do not indicate a problem condition. '+
        'However, a major page fault can also be the result of reading memory pages that have been written out '+
        'to the swap file, which could indicate a memory shortage.'
    },
    'users.major_faults': {
        info: 'The number of <a href="https://en.wikipedia.org/wiki/Page_fault#Major" target="_blank">major faults</a> '+
        'which have required loading a memory page from the disk. '+
        'Major page faults occur because of the absence of the required page from the RAM. '+
        'They are expected when a process starts or needs to read in additional data and '+
        'in these cases do not indicate a problem condition. '+
        'However, a major page fault can also be the result of reading memory pages that have been written out '+
        'to the swap file, which could indicate a memory shortage.'
    },

    // APPS net
    'apps.sockets': {
        info: 'The number of open sockets. '+
        'Sockets are a way to enable inter-process communication between programs running on a server, '+
        'or between programs running on separate servers. This includes both network and UNIX sockets.'
    },
    'groups.sockets': {
        info: 'The number of open sockets. '+
        'Sockets are a way to enable inter-process communication between programs running on a server, '+
        'or between programs running on separate servers. This includes both network and UNIX sockets.'
    },
    'users.sockets': {
        info: 'The number of open sockets. '+
        'Sockets are a way to enable inter-process communication between programs running on a server, '+
        'or between programs running on separate servers. This includes both network and UNIX sockets.'
    },

   // Apps eBPF stuff

    'apps.file_open': {
        info: 'Number of calls for internal functions on the Linux kernel responsible <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#file-descriptor" target="_blank">to open files</a>. ' +
              'Netdata gives a summary for this chart in <a href="#menu_filesystem_submenu_file_access">file access</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, ' +
              'Netdata shows virtual file system per <a href="#ebpf_services_file_open">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_file_open"></div>'
    },

    'apps.file_open_error': {
        info: 'Number of failed calls for internal functions on the Linux kernel responsible <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#file-descriptor" target="_blank">to open files</a>. ' +
              'Netdata gives a summary for this chart in <a href="#menu_filesystem_submenu_file_access">file access</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, ' +
              'Netdata shows virtual file system per <a href="#ebpf_services_file_open_error">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_file_open_error"></div>'
    },

    'apps.file_closed': {
        info: 'Number of calls for internal functions on the Linux kernel responsible <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#file-descriptor" target="_blank">to close files</a>. ' +
              'Netdata gives a summary for this chart in <a href="#menu_filesystem_submenu_file_access">file access</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, ' +
              'Netdata shows virtual file system per <a href="#ebpf_services_file_closed">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_file_closed"></div>'
    },

    'apps.file_close_error': {
        info: 'Number of failed calls for internal functions on the Linux kernel responsible <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#file-descriptor" target="_blank">to close files</a>. ' +
            'Netdata gives a summary for this chart in <a href="#menu_filesystem_submenu_file_access">file access</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, ' +
            'Netdata shows virtual file system per <a href="#ebpf_services_file_close_error">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_file_close_error"></div>'
    },

    'apps.file_deleted': {
        info: 'Number of calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS unlinker function</a>. Netdata gives a summary for this chart in ' +
            '<a href="#ebpf_global_vfs_unlink">Virtual File System</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, ' +
            'Netdata shows virtual file system per <a href="#ebpf_services_vfs_unlink">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_vfs_unlink"></div>'
    },

    'apps.vfs_write_call': {
        info: 'Number of successful calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS writer function</a>. Netdata gives a summary for this chart in ' +
              '<a href="#ebpf_global_vfs_io">Virtual File System</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, ' +
              'Netdata shows virtual file system per <a href="#ebpf_services_vfs_write">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_vfs_write"></div>'
    },

    'apps.vfs_write_error': {
        info: 'Number of failed calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS writer function</a>. Netdata gives a summary for this chart in ' +
            '<a href="#ebpf_global_vfs_io_error">Virtual File System</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, ' +
            'Netdata shows virtual file system per <a href="#ebpf_services_vfs_write_error">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_vfs_write_error"></div>'
    },

    'apps.vfs_read_call': {
        info: 'Number of successful calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS reader function</a>. Netdata gives a summary for this chart in ' +
              '<a href="#ebpf_global_vfs_io">Virtual File System</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, ' +
              'Netdata shows virtual file system per <a href="#ebpf_services_vfs_read">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_vfs_read"></div>'
    },

    'apps.vfs_read_error': {
        info: 'Number of failed calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS reader function</a>. Netdata gives a summary for this chart in ' +
            '<a href="#ebpf_global_vfs_io_error">Virtual File System</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, ' +
            'Netdata shows virtual file system per <a href="#ebpf_services_vfs_read_error">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_vfs_read_error"></div>'
    },

    'apps.vfs_write_bytes': {
        info: 'Total of bytes successfully written using the <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS writer function</a>. Netdata gives a summary for this chart in ' +
            '<a href="#ebpf_global_vfs_io_bytes">Virtual File System</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, ' +
            'Netdata shows virtual file system per <a href="#ebpf_services_vfs_write_bytes">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_vfs_write_bytes"></div>'
    },

    'apps.vfs_read_bytes': {
        info: 'Total of bytes successfully written using the <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS reader function</a>. Netdata gives a summary for this chart in ' +
            '<a href="#ebpf_global_vfs_io_bytes">Virtual File System</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, ' +
            'Netdata shows virtual file system per <a href="#ebpf_services_vfs_read_bytes">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_vfs_read_bytes"></div>'
    },

    'apps.vfs_fsync': {
        info: 'Number of calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS syncer function</a>. Netdata gives a summary for this chart in ' +
            '<a href="#ebpf_global_vfs_sync">Virtual File System</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, ' +
            'Netdata shows virtual file system per <a href="#ebpf_services_vfs_sync">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_vfs_sync"></div>'
    },

    'apps.vfs_fsync_error': {
        info: 'Number of failed calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS syncer function</a>. Netdata gives a summary for this chart in ' +
            '<a href="#ebpf_global_vfs_sync_error">Virtual File System</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, ' +
            'Netdata shows virtual file system per <a href="#ebpf_services_vfs_sync_error">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_vfs_sync_error"></div>'
    },

    'apps.vfs_open': {
        info: 'Number of calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS opener function</a>. Netdata gives a summary for this chart in ' +
            '<a href="#ebpf_global_vfs_open">Virtual File System</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, ' +
            'Netdata shows virtual file system per <a href="#ebpf_services_vfs_open">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_vfs_open"></div>'
    },

    'apps.vfs_open_error': {
        info: 'Number of failed calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS opener function</a>. Netdata gives a summary for this chart in ' +
            '<a href="#ebpf_global_vfs_open_error">Virtual File System</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, ' +
            'Netdata shows virtual file system per <a href="#ebpf_services_vfs_open_error">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_vfs_open_error"></div>'
    },

    'apps.vfs_create': {
        info: 'Number of calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS creator function</a>. Netdata gives a summary for this chart in ' +
            '<a href="#ebpf_global_vfs_create">Virtual File System</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, ' +
            'Netdata shows virtual file system per <a href="#ebpf_services_vfs_create">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_vfs_create"></div>'
    },

    'apps.vfs_create_error': {
        info: 'Number of failed calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS creator function</a>. Netdata gives a summary for this chart in ' +
            '<a href="#ebpf_global_vfs_create_error">Virtual File System</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, ' +
            'Netdata shows virtual file system per <a href="#ebpf_services_vfs_create_error">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_vfs_create_error"></div>'
    },

    'apps.process_create': {
        info: 'Number of times <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#processes" target="_blank">a function</a> that starts a process is called. Netdata gives a summary for this chart in <a href="#ebpf_system_process_thread">Process</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, Netdata shows process per <a href="#ebpf_services_process_create">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_process_create"></div>'
    },

    'apps.thread_create': {
        info: 'Number of times <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#processes" target="_blank">a function</a> that starts a thread is called. Netdata gives a summary for this chart in <a href="#ebpf_system_process_thread">Process</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, Netdata shows process per <a href="#ebpf_services_thread_create">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_thread_create"></div>'
    },

    'apps.task_exit': {
        info: 'Number of times <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#process-exit" target="_blank">a function</a> responsible for closing tasks is called. Netdata gives a summary for this chart in <a href="#ebpf_system_process_exit">Process</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, Netdata shows process per <a href="#ebpf_services_process_exit">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_process_exit"></div>'
    },

    'apps.task_close': {
        info: 'Number of times <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#process-exit" target="_blank">a function</a> responsible for releasing tasks is called. Netdata gives a summary for this chart in <a href="#ebpf_system_process_exit">Process</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, Netdata shows process per <a href="#ebpf_services_task_releease">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_task_release"></div>'
    },

    'apps.task_error': {
        info: 'Number of errors to create a new <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#process-exit" target="_blank">task</a>. Netdata gives a summary for this chart in <a href="#ebpf_system_task_error">Process</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, Netdata shows process per <a href="#ebpf_services_task_error">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_task_error"></div>'
    },

    'apps.outbound_conn_v4': {
        info: 'Number of calls to IPV4 TCP function responsible for <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#tcp-outbound-connections" target="_blank">starting connections</a>. Netdata gives a summary for this chart in <a href="#ebpf_global_outbound_conn">Network Stack</a>. When the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, Netdata shows outbound connections per <a href="#ebpf_services_outbound_conn_ipv4">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_outbound_conn_ipv4"></div>'
    },

    'apps.outbound_conn_v6': {
        info: 'Number of calls to IPV6 TCP function responsible for <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#tcp-outbound-connections" target="_blank">starting connections</a>. Netdata gives a summary for this chart in <a href="#ebpf_global_outbound_conn">Network Stack</a>. When the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, Netdata shows outbound connections per <a href="#ebpf_services_outbound_conn_ipv6">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_outbound_conn_ipv6"></div>'
    },

    'apps.total_bandwidth_sent': {
        info: 'Total bytes sent with <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#tcp-bandwidth" target="_blank">TCP</a> or <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#udp-functions" target="_blank">UDP</a> internal functions. Netdata gives a summary for this chart in <a href="#ebpf_global_bandwidth_tcp_bytes">Network Stack</a>. When the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, Netdata shows bandwidth per <a href="#ebpf_services_bandwidth_sent">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_bandwidth_sent"></div>'
    },

    'apps.total_bandwidth_recv': {
        info: 'Total bytes received with <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#tcp-bandwidth" target="_blank">TCP</a> or <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#udp-functions" target="_blank">UDP</a> internal functions. Netdata gives a summary for this chart in <a href="#ebpf_global_bandwidth_tcp_bytes">Network Stack</a>. When the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, Netdata shows bandwidth per <a href="#ebpf_services_bandwidth_received">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_bandwidth_received"></div>'
    },

    'apps.bandwidth_tcp_send': {
        info: 'Number of calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#tcp-bandwidth" target="_blank">TCP</a> functions responsible to send data. Netdata gives a summary for this chart in <a href="#ebpf_global_tcp_bandwidth_call">Network Stack</a>. When the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, Netdata shows TCP calls per <a href="#ebpf_services_bandwidth_tcp_sent">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_bandwidth_tcp_sent"></div>'
    },

    'apps.bandwidth_tcp_recv': {
        info: 'Number of calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#tcp-bandwidth" target="_blank">TCP</a> functions responsible to receive data. Netdata gives a summary for this chart in <a href="#ebpf_global_tcp_bandwidth_call">Network Stack</a>. When the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, Netdata shows TCP calls per <a href="#ebpf_services_bandwidth_tcp_received">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_bandwidth_tcp_received"></div>'
    },

    'apps.bandwidth_tcp_retransmit': {
        info: 'Number of times a <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#tcp-retransmit" target="_blank">TCP</a> packet was retransmitted. Netdata gives a summary for this chart in <a href="#ebpf_global_tcp_retransmit">Network Stack</a>. When the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, Netdata shows TCP calls per <a href="#ebpf_services_tcp_retransmit">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_tcp_retransmit"></div>'
    },

    'apps.bandwidth_udp_send': {
        info: 'Number of calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#udp-functions" target="_blank">UDP</a> functions responsible to send data. Netdata gives a summary for this chart in <a href="#ebpf_global_udp_bandwidth_call">Network Stack</a>. When the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, Netdata shows UDP calls per <a href="#ebpf_services_udp_sendmsg">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_udp_sendmsg"></div>'
    },

    'apps.bandwidth_udp_recv': {
        info: 'Number of calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#udp-functions" target="_blank">UDP</a> functions responsible to receive data. Netdata gives a summary for this chart in <a href="#ebpf_global_udp_bandwidth_call">Network Stack</a>. When the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, Netdata shows UDP calls per <a href="#ebpf_services_udp_recv">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_udp_recv"></div>'
    },

    'apps.cachestat_ratio' : {
        info: 'The <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#page-cache-ratio" target="_blank">ratio</a> shows the percentage of data accessed directly in memory. Netdata gives a summary for this chart in <a href="#menu_mem_submenu_page_cache">Memory</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, Netdata shows page cache hit per <a href="#ebpf_services_cachestat_ratio">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_cachestat_ratio"></div>'
    },

    'apps.cachestat_dirties' : {
        info: 'Number of <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#dirty-pages" target="_blank">modified pages</a> in <a href="https://en.wikipedia.org/wiki/Page_cache" target="_blank">Linux page cache</a>. Netdata gives a summary for this chart in <a href="#ebpf_global_cachestat_dirty">Memory</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, Netdata shows page cache hit per <a href="#ebpf_services_cachestat_dirties">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_cachestat_dirties"></div>'
    },

    'apps.cachestat_hits' : {
        info: 'Number of <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#page-cache-hits" target="_blank">access</a> to data in <a href="https://en.wikipedia.org/wiki/Page_cache" target="_blank">Linux page cache</a>. Netdata gives a summary for this chart in <a href="#ebpf_global_cachestat_hits">Memory</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, Netdata shows page cache hit per <a href="#ebpf_services_cachestat_hits">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_cachestat_hits"></div>'
    },

    'apps.cachestat_misses' : {
        info: 'Number of <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#page-cache-misses" target="_blank">access</a> to data was not present in <a href="https://en.wikipedia.org/wiki/Page_cache" target="_blank">Linux page cache</a>. Netdata gives a summary for this chart in <a href="#ebpf_global_cachestat_misses">Memory</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, Netdata shows page cache misses per <a href="#ebpf_services_cachestat_misses">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_cachestat_misses"></div>'
    },

    'apps.dc_hit_ratio': {
        info: 'Percentage of file accesses that were present in the <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#directory-cache" target="_blank">directory cache</a>. Netdata gives a summary for this chart in <a href="#ebpf_dc_hit_ratio">directory cache</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, Netdata shows directory cache per <a href="#ebpf_services_dc_hit">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_dc_hit"></div>'
    },

    'apps.dc_reference': {
        info: 'Number of times a file is accessed inside <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#directory-cache" target="_blank">directory cache</a>. Netdata gives a summary for this chart in <a href="#ebpf_dc_reference">directory cache</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, Netdata shows directory cache per <a href="#ebpf_services_dc_reference">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_dc_reference"></div>'
    },

    'apps.dc_not_cache': {
        info: 'Number of times a file is accessed in the file system, because it is not present inside the <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#directory-cache" target="_blank">directory cache</a>. Netdata gives a summary for this chart in <a href="#ebpf_dc_reference">directory cache</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, Netdata shows directory cache per <a href="#ebpf_services_dc_not_cache">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_dc_not_cache"></div>'
    },

    'apps.dc_not_found': {
        info: 'Number of times a file was not found on the file system. Netdata gives a summary for this chart in <a href="#ebpf_dc_reference">directory cache</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, Netdata shows directory cache per <a href="#ebpf_services_dc_not_found">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_dc_not_found"></div>'
    },

    'apps.swap_read_call': {
        info: 'Number of calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#swap">swap reader function</a>. Netdata gives a summary for this chart in <a href="#ebpf_global_swap">System Overview</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, Netdata shows swap metrics per <a href="#ebpf_services_swap_read">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_swap_read"></div>'
    },

    'apps.swap_write_call': {
        info: 'Number of calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#swap">swap writer function</a>. Netdata gives a summary for this chart in <a href="#ebpf_global_swap">System Overview</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, Netdata shows swap metrics per <a href="#ebpf_services_swap_write">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_swap_write"></div>'
    },

    'apps.shmget_call': {
        info: 'Number of calls to <code>shmget</code>. Netdata gives a summary for this chart in <a href="#ebpf_global_shm">System Overview</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, Netdata shows shared memory metrics per <a href="#ebpf_services_shm_get">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_shm_get"></div>'
    },

    'apps.shmat_call': {
        info: 'Number of calls to <code>shmat</code>. Netdata gives a summary for this chart in <a href="#ebpf_global_shm">System Overview</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, Netdata shows shared memory metrics per <a href="#ebpf_services_shm_at">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_shm_at"></div>'
    },

    'apps.shmdt_call': {
        info: 'Number of calls to <code>shmdt</code>. Netdata gives a summary for this chart in <a href="#ebpf_global_shm">System Overview</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, Netdata shows shared memory metrics per <a href="#ebpf_services_shm_dt">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_shm_dt"></div>'
    },

    'apps.shmctl_call': {
        info: 'Number of calls to <code>shmctl</code>. Netdata gives a summary for this chart in <a href="#ebpf_global_shm">System Overview</a>, and when the integration is <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">enabled</a>, Netdata shows shared memory metrics per <a href="#ebpf_services_shm_ctl">cgroup (systemd Services)</a>.' + ebpfChartProvides + '<div id="ebpf_apps_shm_ctl"></div>'
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
        heads: [
            netdataDashboard.gaugeChart('Received', '12%', 'received'),
            netdataDashboard.gaugeChart('Sent', '12%', 'sent'),
        ],
        info: netBytesInfo
    },
    'net.packets': {
        info: netPacketsInfo
    },
    'net.errors': {
        info: netErrorsInfo
    },
    'net.fifo': {
        info: netFIFOInfo
    },
    'net.drops': {
        info: netDropsInfo
    },
    'net.compressed': {
        info: netCompressedInfo
    },
    'net.events': {
        info: netEventsInfo
    },
    'net.duplex': {
        info: netDuplexInfo
    },
    'net.operstate': {
        info: netOperstateInfo
    },
    'net.carrier': {
        info: netCarrierInfo
    },
    'net.speed': {
        info: netSpeedInfo
    },
    'net.mtu': {
        info: netMTUInfo
    },

    // ------------------------------------------------------------------------
    // CGROUP NETWORK INTERFACES

    'cgroup.net_net': {
        mainheads: [
            function (os, id) {
                void (os);
                var iface;
                try {
                    iface = ' ' + id.substring(id.lastIndexOf('.net_') + 5, id.length);
                } catch (e) {
                    iface = '';
                }
                return netdataDashboard.gaugeChart('Received' + iface, '12%', 'received');

            },
            function (os, id) {
                void (os);
                var iface;
                try {
                    iface = ' ' + id.substring(id.lastIndexOf('.net_') + 5, id.length);
                } catch (e) {
                    iface = '';
                }
                return netdataDashboard.gaugeChart('Sent' + iface, '12%', 'sent');
            }
        ],
        info: netBytesInfo
    },
    'cgroup.net_packets': {
        info: netPacketsInfo
    },
    'cgroup.net_errors': {
        info: netErrorsInfo
    },
    'cgroup.net_fifo': {
        info: netFIFOInfo
    },
    'cgroup.net_drops': {
        info: netDropsInfo
    },
    'cgroup.net_compressed': {
        info: netCompressedInfo
    },
    'cgroup.net_events': {
        info: netEventsInfo
    },
    'cgroup.net_duplex': {
        info: netDuplexInfo
    },
    'cgroup.net_operstate': {
        info: netOperstateInfo
    },
    'cgroup.net_carrier': {
        info: netCarrierInfo
    },
    'cgroup.net_speed': {
        info: netSpeedInfo
    },
    'cgroup.net_mtu': {
        info: netMTUInfo
    },

    'k8s.cgroup.net_net': {
        mainheads: [
            function (_, id) {
                var iface;
                try {
                    iface = ' ' + id.substring(id.lastIndexOf('.net_') + 5, id.length);
                } catch (e) {
                    iface = '';
                }
                return netdataDashboard.gaugeChart('Received' + iface, '12%', 'received');

            },
            function (_, id) {
                var iface;
                try {
                    iface = ' ' + id.substring(id.lastIndexOf('.net_') + 5, id.length);
                } catch (e) {
                    iface = '';
                }
                return netdataDashboard.gaugeChart('Sent' + iface, '12%', 'sent');
            }
        ],
        info: netBytesInfo
    },
    'k8s.cgroup.net_packets': {
        info: netPacketsInfo
    },
    'k8s.cgroup.net_errors': {
        info: netErrorsInfo
    },
    'k8s.cgroup.net_fifo': {
        info: netFIFOInfo
    },
    'k8s.cgroup.net_drops': {
        info: netDropsInfo
    },
    'k8s.cgroup.net_compressed': {
        info: netCompressedInfo
    },
    'k8s.cgroup.net_events': {
        info: netEventsInfo
    },
    'k8s.cgroup.net_operstate': {
        info: netOperstateInfo
    },
    'k8s.cgroup.net_duplex': {
        info: netDuplexInfo
    },
    'k8s.cgroup.net_carrier': {
        info: netCarrierInfo
    },
    'k8s.cgroup.net_speed': {
        info: netSpeedInfo
    },
    'k8s.cgroup.net_mtu': {
        info: netMTUInfo
    },

    // ------------------------------------------------------------------------
    // WIRELESS NETWORK INTERFACES

    'wireless.link_quality': {
        info: 'Overall quality of the link. '+
        'May be based on the level of contention or interference, the bit or frame error rate, '+
        'how good the received signal is, some timing synchronisation, or other hardware metric.'
    },

    'wireless.signal_level': {
        info: 'Received signal strength '+
        '(<a href="https://en.wikipedia.org/wiki/Received_signal_strength_indication" target="_blank">RSSI</a>).'
    },

    'wireless.noise_level': {
        info: 'Background noise level (when no packet is transmitted).'
    },

    'wireless.discarded_packets': {
        info: '<p>The number of discarded packets.</p>'+
        '</p><b>NWID</b> - received packets with a different NWID or ESSID. '+
        'Used to detect configuration problems or adjacent network existence (on the same frequency). '+
        '<b>Crypt</b> - received packets that the hardware was unable to code/encode. '+
        'This can be used to detect invalid encryption settings. '+
        '<b>Frag</b> - received packets for which the hardware was not able to properly re-assemble '+
        'the link layer fragments (most likely one was missing). '+
        '<b>Retry</b> - packets that the hardware failed to deliver. '+
        'Most MAC protocols will retry the packet a number of times before giving up. '+
        '<b>Misc</b> - other packets lost in relation with specific wireless operations.</p>'
    },

    'wireless.missed_beacons': {
        info: 'The number of periodic '+
        '<a href="https://en.wikipedia.org/wiki/Beacon_frame" target="_blank">beacons</a> '+
        'from the Cell or the Access Point have been missed. '+
        'Beacons are sent at regular intervals to maintain the cell coordination, '+
        'failure to receive them usually indicates that the card is out of range.'
    },

    // ------------------------------------------------------------------------
    // INFINIBAND

    'ib.bytes': {
        info: 'The amount of traffic transferred by the port.'
    },

    'ib.packets': {
        info: 'The number of packets transferred by the port.'
    },

    'ib.errors': {
        info: 'The number of errors encountered by the port.'
    },

    'ib.hwerrors': {
        info: 'The number of hardware errors encountered by the port.'
    },

    'ib.hwpackets': {
        info: 'The number of hardware packets transferred by the port.'
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
    // IPVS
    'ipvs.sockets': {
        info: 'Total created connections for all services and their servers. '+
        'To see the IPVS connection table, run <code>ipvsadm -Lnc</code>.'
    },
    'ipvs.packets': {
        info: 'Total transferred packets for all services and their servers.'
    },
    'ipvs.net': {
        info: 'Total network traffic for all services and their servers.'
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
        info: 'The amount of data transferred to and from disk.'
    },

    'disk_ext.io': {
        info: 'The amount of discarded data that are no longer in use by a mounted file system.'
    },

    'disk.ops': {
        info: 'Completed disk I/O operations. Keep in mind the number of operations requested might be higher, since the system is able to merge adjacent to each other (see merged operations chart).'
    },

    'disk_ext.ops': {
        info: '<p>The number (after merges) of completed discard/flush requests.</p>'+
        '<p><b>Discard</b> commands inform disks which blocks of data are no longer considered to be in use and therefore can be erased internally. '+
        'They are useful for solid-state drivers (SSDs) and thinly-provisioned storage. '+
        'Discarding/trimming enables the SSD to handle garbage collection more efficiently, '+
        'which would otherwise slow future write operations to the involved blocks down.</p>'+
        '<p><b>Flush</b> operations transfer all modified in-core data (i.e., modified buffer cache pages) to the disk device '+
        'so that all changed information can be retrieved even if the system crashes or is rebooted. '+
        'Flush requests are executed by disks. Flush requests are not tracked for partitions. '+
        'Before being merged, flush operations are counted as writes.</p>'
    },

    'disk.qops': {
        info: 'I/O operations currently in progress. This metric is a snapshot - it is not an average over the last interval.'
    },

    'disk.iotime': {
        height: 0.5,
        info: 'The sum of the duration of all completed I/O operations. This number can exceed the interval if the disk is able to execute I/O operations in parallel.'
    },
    'disk_ext.iotime': {
        height: 0.5,
        info: 'The sum of the duration of all completed discard/flush operations. This number can exceed the interval if the disk is able to execute discard/flush operations in parallel.'
    },
    'disk.mops': {
        height: 0.5,
        info: 'The number of merged disk operations. The system is able to merge adjacent I/O operations, for example two 4KB reads can become one 8KB read before given to disk.'
    },
    'disk_ext.mops': {
        height: 0.5,
        info: 'The number of merged discard disk operations. Discard operations which are adjacent to each other may be merged for efficiency.'
    },
    'disk.svctm': {
        height: 0.5,
        info: 'The average service time for completed I/O operations. This metric is calculated using the total busy time of the disk and the number of completed operations. If the disk is able to execute multiple parallel operations the reporting average service time will be misleading.'
    },
    'disk.latency_io': {
        height: 0.5,
        info: 'Disk I/O <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#disk-latency" target="_blank">latency</a> is the time it takes for an I/O request to be completed. Disk chart has a relationship with <a href="#filesystem">Filesystem</a> charts. This chart is based on the <a href="https://github.com/cloudflare/ebpf_exporter/blob/master/examples/bio-tracepoints.yaml" target="_blank">bio_tracepoints</a> tool of the ebpf_exporter.' + ebpfChartProvides
    },
    'disk.avgsz': {
        height: 0.5,
        info: 'The average I/O operation size.'
    },
    'disk_ext.avgsz': {
        height: 0.5,
        info: 'The average discard operation size.'
    },
    'disk.await': {
        height: 0.5,
        info: 'The average time for I/O requests issued to the device to be served. This includes the time spent by the requests in queue and the time spent servicing them.'
    },
    'disk_ext.await': {
        height: 0.5,
        info: 'The average time for discard/flush requests issued to the device to be served. This includes the time spent by the requests in queue and the time spent servicing them.'
    },

    'disk.space': {
        info: 'Disk space utilization. reserved for root is automatically reserved by the system to prevent the root user from getting out of space.'
    },
    'disk.inodes': {
        info: 'Inodes (or index nodes) are filesystem objects (e.g. files and directories). On many types of file system implementations, the maximum number of inodes is fixed at filesystem creation, limiting the maximum number of files the filesystem can hold. It is possible for a device to run out of inodes. When this happens, new files cannot be created on the device, even though there may be free space available.'
    },

    'disk.bcache_hit_ratio': {
        info: '<p><b>Bcache (block cache)</b> is a cache in the block layer of Linux kernel, '+
        'which is used for accessing secondary storage devices. '+
        'It allows one or more fast storage devices, such as flash-based solid-state drives (SSDs), '+
        'to act as a cache for one or more slower storage devices, such as hard disk drives (HDDs).</p>'+
        '<p>Percentage of data requests that were fulfilled right from the block cache. '+
        'Hits and misses are counted per individual IO as bcache sees them. '+
        'A partial hit is counted as a miss.</p>'
    },
    'disk.bcache_rates': {
        info: 'Throttling rates. '+
        'To avoid congestions bcache tracks latency to the cache device, and gradually throttles traffic if the latency exceeds a threshold. ' +
        'If the writeback percentage is nonzero, bcache tries to keep around this percentage of the cache dirty by '+
        'throttling background writeback and using a PD controller to smoothly adjust the rate.'
    },
    'disk.bcache_size': {
        info: 'The amount of dirty data for this backing device in the cache.'
    },
    'disk.bcache_usage': {
        info: 'The percentage of cache device which does not contain dirty data, and could potentially be used for writeback.'
    },
    'disk.bcache_cache_read_races': {
        info: '<b>Read races</b> happen when a bucket was reused and invalidated while data was being read from the cache. '+
        'When this occurs the data is reread from the backing device. '+
        '<b>IO errors</b> are decayed by the half life. '+
        'If the decaying count reaches the limit, dirty data is written out and the cache is disabled.'
    },
    'disk.bcache': {
        info: 'Hits and misses are counted per individual IO as bcache sees them; a partial hit is counted as a miss. '+
        'Collisions happen when data was going to be inserted into the cache from a cache miss, '+
        'but raced with a write and data was already present. '+
        'Cache miss reads are rounded up to the readahead size, but without overlapping existing cache entries.'
    },
    'disk.bcache_bypass': {
        info: 'Hits and misses for IO that is intended to skip the cache.'
    },
    'disk.bcache_cache_alloc': {
        info: '<p>Working set size.</p>'+
        '<p><b>Unused</b> is the percentage of the cache that does not contain any data. '+
        '<b>Dirty</b> is the data that is modified in the cache but not yet written to the permanent storage. '+
        '<b>Clean</b> data matches the data stored on the permanent storage. '+
        '<b>Metadata</b> is bcache\'s metadata overhead.</p>'
    },

    // ------------------------------------------------------------------------
    // NFS client

    'nfs.net': {
        info: 'The number of received UDP and TCP packets.'
    },

    'nfs.rpc': {
        info: '<p>Remote Procedure Call (RPC) statistics.</p>'+
        '</p><b>Calls</b> - all RPC calls. '+
        '<b>Retransmits</b> - retransmitted calls. '+
        '<b>AuthRefresh</b> - authentication refresh calls (validating credentials with the server).</p>'
    },

    'nfs.proc2': {
        info: 'NFSv2 RPC calls. The individual metrics are described in '+
        '<a href="https://datatracker.ietf.org/doc/html/rfc1094#section-2.2" target="_blank">RFC1094</a>.'
    },

    'nfs.proc3': {
        info: 'NFSv3 RPC calls. The individual metrics are described in '+
        '<a href="https://datatracker.ietf.org/doc/html/rfc1813#section-3" target="_blank">RFC1813</a>.'
    },

    'nfs.proc4': {
        info: 'NFSv4 RPC calls. The individual metrics are described in '+
        '<a href="https://datatracker.ietf.org/doc/html/rfc8881#section-18" target="_blank">RFC8881</a>.'
    },

    // ------------------------------------------------------------------------
    // NFS server

    'nfsd.readcache': {
        info: '<p>Reply cache statistics. '+
        'The reply cache keeps track of responses to recently performed non-idempotent transactions, and '+
        'in case of a replay, the cached response is sent instead of attempting to perform the operation again.</p>'+
        '<b>Hits</b> - client did not receive a reply and re-transmitted its request. This event is undesirable. '+
        '<b>Misses</b> - an operation that requires caching (idempotent). '+
        '<b>Nocache</b> - an operation that does not require caching (non-idempotent).'
    },

    'nfsd.filehandles': {
        info: '<p>File handle statistics. '+
        'File handles are small pieces of memory that keep track of what file is opened.</p>'+
        '<p><b>Stale</b> - happen when a file handle references a location that has been recycled. '+
        'This also occurs when the server loses connection and '+
        'applications are still using files that are no longer accessible.'
    },

    'nfsd.io': {
        info: 'The amount of data transferred to and from disk.'
    },

    'nfsd.threads': {
        info: 'The number of threads used by the NFS daemon.'
    },

    'nfsd.readahead': {
        info: '<p>Read-ahead cache statistics. '+
        'NFS read-ahead predictively requests blocks from a file in advance of I/O requests by the application. '+
        'It is designed to improve client sequential read throughput.</p>'+
        '<p><b>10%</b>-<b>100%</b> - histogram of depth the block was found. '+
        'This means how far the cached block is from the original block that was first requested. '+
        '<b>Misses</b> - not found in the read-ahead cache.</p>'
    },

    'nfsd.net': {
        info: 'The number of received UDP and TCP packets.'
    },

    'nfsd.rpc': {
        info: '<p>Remote Procedure Call (RPC) statistics.</p>'+
        '</p><b>Calls</b> - all RPC calls. '+
        '<b>BadAuth</b> - bad authentication. '+
        'It does not count if you try to mount from a machine that it\'s not in your exports file. '+
        '<b>BadFormat</b> - other errors.</p>'
    },

    'nfsd.proc2': {
        info: 'NFSv2 RPC calls. The individual metrics are described in '+
        '<a href="https://datatracker.ietf.org/doc/html/rfc1094#section-2.2" target="_blank">RFC1094</a>.'
    },

    'nfsd.proc3': {
        info: 'NFSv3 RPC calls. The individual metrics are described in '+
        '<a href="https://datatracker.ietf.org/doc/html/rfc1813#section-3" target="_blank">RFC1813</a>.'
    },

    'nfsd.proc4': {
        info: 'NFSv4 RPC calls. The individual metrics are described in '+
        '<a href="https://datatracker.ietf.org/doc/html/rfc8881#section-18" target="_blank">RFC8881</a>.'
    },

    'nfsd.proc4ops': {
        info: 'NFSv4 RPC operations. The individual metrics are described in '+
        '<a href="https://datatracker.ietf.org/doc/html/rfc8881#section-18" target="_blank">RFC8881</a>.'
    },

    // ------------------------------------------------------------------------
    // ZFS

    'zfs.arc_size': {
        info: '<p>The size of the ARC.</p>'+
        '<p><b>Arcsz</b> - actual size. '+
        '<b>Target</b> - target size that the ARC is attempting to maintain (adaptive). '+
        '<b>Min</b> - minimum size limit. When the ARC is asked to shrink, it will stop shrinking at this value. '+
        '<b>Max</b> - maximum size limit.</p>'
    },

    'zfs.l2_size': {
        info: '<p>The size of the L2ARC.</p>'+
        '<p><b>Actual</b> - size of compressed data. '+
        '<b>Size</b> - size of uncompressed data.</p>'
    },

    'zfs.reads': {
        info: '<p>The number of read requests.</p>'+
        '<p><b>ARC</b> - all prefetch and demand requests. '+
        '<b>Demand</b> - triggered by an application request. '+
        '<b>Prefetch</b> - triggered by the prefetch mechanism, not directly from an application request. '+
        '<b>Metadata</b> - metadata read requests. '+
        '<b>L2</b> - L2ARC read requests.</p>'
    },

    'zfs.bytes': {
        info: 'The amount of data transferred to and from the L2ARC cache devices.'
    },

    'zfs.hits': {
        info: '<p>Hit rate of the ARC read requests.</p>'+
        '<p><b>Hits</b> - a data block was in the ARC DRAM cache and returned. '+
        '<b>Misses</b> - a data block was not in the ARC DRAM cache. '+
        'It will be read from the L2ARC cache devices (if available and the data is cached on them) or the pool disks.</p>'
    },

    'zfs.dhits': {
        info: '<p>Hit rate of the ARC data and metadata demand read requests. '+
        'Demand requests are triggered by an application request.</p>'+
        '<p><b>Hits</b> - a data block was in the ARC DRAM cache and returned. '+
        '<b>Misses</b> - a data block was not in the ARC DRAM cache. '+
        'It will be read from the L2ARC cache devices (if available and the data is cached on them) or the pool disks.</p>'
    },

    'zfs.phits': {
        info: '<p>Hit rate of the ARC data and metadata prefetch read requests. '+
        'Prefetch requests are triggered by the prefetch mechanism, not directly from an application request.</p>'+
        '<p><b>Hits</b> - a data block was in the ARC DRAM cache and returned. '+
        '<b>Misses</b> - a data block was not in the ARC DRAM cache. '+
        'It will be read from the L2ARC cache devices (if available and the data is cached on them) or the pool disks.</p>'
    },

    'zfs.mhits': {
        info: '<p>Hit rate of the ARC metadata read requests.</p>'+
        '<p><b>Hits</b> - a data block was in the ARC DRAM cache and returned. '+
        '<b>Misses</b> - a data block was not in the ARC DRAM cache. '+
        'It will be read from the L2ARC cache devices (if available and the data is cached on them) or the pool disks.</p>'
    },

    'zfs.l2hits': {
        info: '<p>Hit rate of the L2ARC lookups.</p>'+
        '</p><b>Hits</b> - a data block was in the L2ARC cache and returned. '+
        '<b>Misses</b> - a data block was not in the L2ARC cache. '+
        'It will be read from the pool disks.</p>'
    },

    'zfs.demand_data_hits': {
        info: '<p>Hit rate of the ARC data demand read requests. '+
        'Demand requests are triggered by an application request.</p>'+
        '<b>Hits</b> - a data block was in the ARC DRAM cache and returned. '+
        '<b>Misses</b> - a data block was not in the ARC DRAM cache. '+
        'It will be read from the L2ARC cache devices (if available and the data is cached on them) or the pool disks.</p>'
    },

    'zfs.prefetch_data_hits': {
        info: '<p>Hit rate of the ARC data prefetch read requests. '+
        'Prefetch requests are triggered by the prefetch mechanism, not directly from an application request.</p>'+
        '<p><b>Hits</b> - a data block was in the ARC DRAM cache and returned. '+
        '<b>Misses</b> - a data block was not in the ARC DRAM cache. '+
        'It will be read from the L2ARC cache devices (if available and the data is cached on them) or the pool disks.</p>'
    },

    'zfs.list_hits': {
        info: 'MRU (most recently used) and MFU (most frequently used) cache list hits. '+
        'MRU and MFU lists contain metadata for requested blocks which are cached. '+
        'Ghost lists contain metadata of the evicted pages on disk.'
    },

    'zfs.arc_size_breakdown': {
        info: 'The size of MRU (most recently used) and MFU (most frequently used) cache.'
    },

    'zfs.memory_ops': {
        info: '<p>Memory operation statistics.</p>'+
        '<p><b>Direct</b> - synchronous memory reclaim. Data is evicted from the ARC and free slabs reaped. '+
        '<b>Throttled</b> - number of times that ZFS had to limit the ARC growth. '+
        'A constant increasing of the this value can indicate excessive pressure to evict data from the ARC. '+
        '<b>Indirect</b> - asynchronous memory reclaim. It reaps free slabs from the ARC cache.</p>'
    },

    'zfs.important_ops': {
        info: '<p>Eviction and insertion operation statistics.</p>'+
        '<p><b>EvictSkip</b> - skipped data eviction operations. '+
        '<b>Deleted</b> - old data is evicted (deleted) from the cache. '+
        '<b>MutexMiss</b> - an attempt to get hash or data block mutex when it is locked during eviction. '+
        '<b>HashCollisions</b> - occurs when two distinct data block numbers have the same hash value.</p>'
    },

    'zfs.actual_hits': {
        info: '<p>MRU and MFU cache hit rate.</p>'+
        '<p><b>Hits</b> - a data block was in the ARC DRAM cache and returned. '+
        '<b>Misses</b> - a data block was not in the ARC DRAM cache. '+
        'It will be read from the L2ARC cache devices (if available and the data is cached on them) or the pool disks.</p>'
    },

    'zfs.hash_elements': {
        info: '<p>Data Virtual Address (DVA) hash table element statistics.</p>'+
        '<p><b>Current</b> - current number of elements. '+
        '<b>Max</b> - maximum number of elements seen.</p>'
    },

    'zfs.hash_chains': {
        info: '<p>Data Virtual Address (DVA) hash table chain statistics. '+
        'A chain is formed when two or more distinct data block numbers have the same hash value.</p>'+
        '<p><b>Current</b> - current number of chains. '+
        '<b>Max</b> - longest length seen for a chain. '+
        'If the value is high, performance may degrade as the hash locks are held longer while the chains are walked.</p>'
    },

    // ------------------------------------------------------------------------
    // ZFS pools
    'zfspool.state': {
        info: 'ZFS pool state. '+
        'The overall health of a pool, as reported by <code>zpool status</code>, '+
        'is determined by the aggregate state of all devices within the pool. ' +
        'For states description, '+
        'see <a href="https://openzfs.github.io/openzfs-docs/man/7/zpoolconcepts.7.html#Device_Failure_and_Recovery" target="_blank"> ZFS documentation</a>.'
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
        info: "<p>Status of this cluster component.</p><p><b>Primary</b> - primary group configuration, quorum present. <b>Non-Primary</b> - non-primary group configuration, quorum lost. <b>Disconnected</b> - not connected to group, retrying.</p>"
    },

    'mysql.galera_cluster_state': {
        info: "<p>Membership state of this cluster component.</p><p><b>Undefined</b> - undefined state. <b>Joining</b> - the node is attempting to join the cluster. <b>Donor</b> - the node has blocked itself while it sends a State Snapshot Transfer (SST) to bring a new node up to date with the cluster. <b>Joined</b> - the node has successfully joined the cluster. <b>Synced</b> - the node has established a connection with the cluster and synchronized its local databases with those of the cluster. <b>Error</b> - the node is not part of the cluster and does not replicate transactions. This state is provider-specific, check <i>wsrep_local_state_comment</i> variable for a description.</p>"
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
    'postgres.connections_utilization': {
        room: { 
            mainheads: [
                function (_, id) {
                    return '<div data-netdata="' + id + '"'
                        + ' data-append-options="percentage"'
                        + ' data-gauge-max-value="100"'
                        + ' data-chart-library="gauge"'
                        + ' data-title="Connections Utilization"'
                        + ' data-units="%"'
                        + ' data-gauge-adjust="width"'
                        + ' data-width="12%"'
                        + ' data-before="0"'
                        + ' data-after="-CHART_DURATION"'
                        + ' data-points="CHART_DURATION"'
                        + ' data-colors="' + NETDATA.colors[1] + '"'
                        + ' role="application"></div>';
                }
            ],
        },
        info: '<b>Total connection utilization</b> across all databases. Utilization is measured as a percentage of (<i>max_connections</i> - <i>superuser_reserved_connections</i>). If the utilization is 100% no more new connections will be accepted (superuser connections will still be accepted if superuser quota is available).'
    },
    'postgres.connections_usage': {
        info: '<p><b>Connections usage</b> across all databases. The maximum number of concurrent connections to the database server is (<i>max_connections</i> - <i>superuser_reserved_connections</i>). As a general rule, if you need more than 200 connections it is advisable to use connection pooling.</p><p><b>Available</b> - new connections allowed. <b>Used</b> - connections currently in use.</p>'
    },
    'postgres.connections_state_count': {
        info: '<p>Number of connections in each state across all databases.</p><p><b>Active</b> - the backend is executing query. <b>Idle</b> - the backend is waiting for a new client command. <b>IdleInTransaction</b> - the backend is in a transaction, but is not currently executing a query. <b>IdleInTransactionAborted</b> - the backend is in a transaction, and not currently executing a query, but one of the statements in the transaction caused an error. <b>FastPathFunctionCall</b> - the backend is executing a fast-path function. <b>Disabled</b> - is reported if <a href="https://www.postgresql.org/docs/current/runtime-config-statistics.html#GUC-TRACK-ACTIVITIES" target="_blank"><i>track_activities</i></a> is disabled in this backend.</p>'
    },
    'postgres.transactions_duration': {
        info: 'Running transactions duration histogram. The bins are specified as consecutive, non-overlapping intervals. The value is the number of observed transactions that fall into each interval.'
    },
    'postgres.queries_duration': {
        info: 'Active queries duration histogram. The bins are specified as consecutive, non-overlapping intervals. The value is the number of observed active queries that fall into each interval.'
    },
    'postgres.checkpoints_rate': {
        info: '<p>Number of checkpoints that have been performed. Checkpoints are periodic maintenance operations the database performs to make sure that everything it\'s been caching in memory has been synchronized with the disk. Ideally checkpoints should be time-driven (scheduled) as opposed to load-driven (requested).</p><p><b>Scheduled</b> - checkpoints triggered as per schedule when time elapsed from the previous checkpoint is greater than <a href="https://www.postgresql.org/docs/current/runtime-config-wal.html#GUC-CHECKPOINT-TIMEOUT" target="_blank"><i>checkpoint_timeout</i></a>. <b>Requested</b> - checkpoints triggered due to WAL updates reaching the <a href="https://www.postgresql.org/docs/current/runtime-config-wal.html#GUC-MAX-WAL-SIZE" target="_blank"><i>max_wal_size</i></a> before the <i>checkpoint_timeout</i> is reached.</p>'
    },
    'postgres.checkpoints_time': {
        info: '<p>Checkpoint timing information. An important indicator of how well checkpoint I/O is performing is the amount of time taken to sync files to disk.</p><p><b>Write</b> - amount of time spent writing files to disk during checkpoint processing. <b>Sync</b> - amount of time spent synchronizing files to disk during checkpoint processing.</p>'
    },
    'postgres.buffers_allocated_rate': {
        info: 'Allocated and re-allocated buffers. If a backend process requests data it is either found in a block in shared buffer cache or the block has to be allocated (read from disk). The latter is counted as <b>Allocated</b>.'
    },
    'postgres.buffers_io_rate': {
        info: '<p>Amount of data flushed from memory to disk.</p><p><b>Checkpoint</b> - buffers written during checkpoints. <b>Backend</b> -  buffers written directly by a backend. It may happen that a dirty page is requested by a backend process. In this case the page is synced to disk before the page is returned to the client. <b>BgWriter</b> - buffers written by the background writer. PostgreSQL may clear pages with a low usage count in advance. The process scans for dirty pages with a low usage count so that they could be cleared if necessary. Buffers written by this process increment the counter.</p>'
    },
    'postgres.bgwriter_halts_rate': {
        info: 'Number of times the background writer stopped a cleaning scan because it had written too many buffers (exceeding the value of <a href="https://www.postgresql.org/docs/current/runtime-config-resource.html#RUNTIME-CONFIG-RESOURCE-BACKGROUND-WRITER" target="_blank"><i>bgwriter_lru_maxpages</i></a>).'
    },
    'postgres.buffers_backend_fsync_rate': {
        info: 'Number of times a backend had to execute its own fsync call (normally the background writer handles those even when the backend does its own write). Any values above zero can indicate problems with storage when fsync queue is completely filled.'
    },
    'postgres.wal_io_rate': {
        info: 'Write-Ahead Logging (WAL) ensures data integrity by ensuring that changes to data files (where tables and indexes reside) are written only after log records describing the changes have been flushed to permanent storage.'
    },
    'postgres.wal_files_count': {
        info: '<p>Number of WAL logs stored in the directory <i>pg_wal</i> under the data directory.</p><p><b>Written</b> - generated log segments files. <b>Recycled</b> - old log segment files that are no longer needed. Renamed to become future segments in the numbered sequence to avoid the need to create new ones.</p>'
    },
    'postgres.wal_archiving_files_count': {
        info: '<p>WAL archiving.</p><p><b>Ready</b> - WAL files waiting to be archived. A non-zero value can indicate <i>archive_command</i> is in error, see <a href="https://www.postgresql.org/docs/current/static/continuous-archiving.html" target="_blank">Continuous Archiving and Point-in-Time Recovery</a>. <b>Done</b> - WAL files successfully archived.'
    },
    'postgres.autovacuum_workers_count': {
        info: 'PostgreSQL databases require periodic maintenance known as vacuuming. For many installations, it is sufficient to let vacuuming be performed by the autovacuum daemon. For more information see <a href="https://www.postgresql.org/docs/current/static/routine-vacuuming.html#AUTOVACUUM" target="_blank">The Autovacuum Daemon</a>.'
    },
    'postgres.txid_exhaustion_towards_autovacuum_perc': {
        info: 'Percentage towards emergency autovacuum for one or more tables. A forced autovacuum will run once this value reaches 100%. For more information see <a href="https://www.postgresql.org/docs/current/routine-vacuuming.html#VACUUM-FOR-WRAPAROUND" target="_blank">Preventing Transaction ID Wraparound Failures</a>.'
    },
    'postgres.txid_exhaustion_perc': {
        info: 'Percentage towards transaction wraparound. A transaction wraparound may occur when this value reaches 100%. For more information see <a href="https://www.postgresql.org/docs/current/routine-vacuuming.html#VACUUM-FOR-WRAPAROUND" target="_blank">Preventing Transaction ID Wraparound Failures</a>.'
    },
    'postgres.txid_exhaustion_oldest_txid_num': {
        info: 'The oldest current transaction ID (XID). If for some reason autovacuum fails to clear old XIDs from a table, the system will begin to emit warning messages when the database\'s oldest XIDs reach eleven million transactions from the wraparound point. For more information see <a href="https://www.postgresql.org/docs/current/routine-vacuuming.html#VACUUM-FOR-WRAPAROUND" target="_blank">Preventing Transaction ID Wraparound Failures</a>.'
    },
    'postgres.uptime': {
        room: { 
            mainheads: [
                function (os, id) {
                    void (os);
                    return '<div data-netdata="' + id + '"'
                        + ' data-chart-library="easypiechart"'
                        + ' data-title="Uptime"'
                        + ' data-units="Seconds"'
                        + ' data-gauge-adjust="width"'
                        + ' data-width="10%"'
                        + ' data-before="0"'
                        + ' data-after="-CHART_DURATION"'
                        + ' data-points="CHART_DURATION"'
                        + ' role="application"></div>';
                }
            ],
        },
        info: 'The time elapsed since the Postgres process was started.'
    },

    'postgres.replication_app_wal_lag_size': {
        info: '<p>Replication WAL lag size.</p><p><b>SentLag</b> - sent over the network. <b>WriteLag</b> - written to disk. <b>FlushLag</b> - flushed to disk. <b>ReplayLag</b> - replayed into the database.</p>'
    },
    'postgres.replication_app_wal_lag_time': {
        info: '<p>Replication WAL lag time.</p><p><b>WriteLag</b> - time elapsed between flushing recent WAL locally and receiving notification that the standby server has written it, but not yet flushed it or applied it. <b>FlushLag</b> - time elapsed between flushing recent WAL locally and receiving notification that the standby server has written and flushed it, but not yet applied it. <b>ReplayLag</b> - time elapsed between flushing recent WAL locally and receiving notification that the standby server has written, flushed and applied it.</p>'
    },
    'postgres.replication_slot_files_count': {
        info: '<p>Replication slot files. For more information see <a href="https://www.postgresql.org/docs/current/static/warm-standby.html#STREAMING-REPLICATION-SLOTS" target="_blank">Replication Slots</a>.</p><p><b>WalKeep</b> - WAL files retained by the replication slot. <b>PgReplslotFiles</b> - files present in pg_replslot.</p>'
    },

    'postgres.db_transactions_ratio': {
        info: 'Percentage of committed/rollback transactions.'
    },
    'postgres.db_transactions_rate': {
        info: '<p>Number of transactions that have been performed</p><p><b>Committed</b> - transactions that have been committed. All changes made by the committed transaction become visible to others and are guaranteed to be durable if a crash occurs. <b>Rollback</b> - transactions that have been rolled back. Rollback aborts the current transaction and causes all the updates made by the transaction to be discarded. Single queries that have failed outside the transactions are also accounted as rollbacks.</p>'
    },
    'postgres.db_connections_utilization': {
        info: 'Connection utilization per database. Utilization is measured as a percentage of <i>CONNECTION LIMIT</i> per database (if set) or <i>max_connections</i> (if <i>CONNECTION LIMIT</i> is not set).'
    },
    'postgres.db_connections_count': {
        info: 'Number of current connections per database.'
    },    
    'postgres.db_cache_io_ratio': {
        room: { 
            mainheads: [
                function (_, id) {
                    return '<div data-netdata="' + id + '"'
                        + ' data-append-options="percentage"'
                        + ' data-gauge-max-value="100"'
                        + ' data-chart-library="gauge"'
                        + ' data-title="Cache Miss Ratio"'
                        + ' data-units="%"'
                        + ' data-gauge-adjust="width"'
                        + ' data-width="12%"'
                        + ' data-before="0"'
                        + ' data-after="-CHART_DURATION"'
                        + ' data-points="CHART_DURATION"'
                        + ' data-colors="' + NETDATA.colors[1] + '"'
                        + ' role="application"></div>';
                }
            ],
        },
        info: 'PostgreSQL uses a <b>shared buffer cache</b> to store frequently accessed data in memory, and avoid slower disk reads. If you are seeing performance issues, consider increasing the <a href="https://www.postgresql.org/docs/current/runtime-config-resource.html#GUC-SHARED-BUFFERS" target="_blank"><i>shared_buffers</i></a> size or tuning <a href="https://www.postgresql.org/docs/current/runtime-config-query.html#GUC-EFFECTIVE-CACHE-SIZE" target="_blank"><i>effective_cache_size</i></a>.'
    },
    'postgres.db_io_rate': {
        info: '<p>Amount of data read from shared buffer cache or from disk.</p><p><b>Disk</b> - data read from disk. <b>Memory</b> - data read from buffer cache (this only includes hits in the PostgreSQL buffer cache, not the operating system\'s file system cache).</p>'
    },
    'postgres.db_ops_fetched_rows_ratio': {
        room: {
            mainheads: [
                function (_, id) {
                    return '<div data-netdata="' + id + '"'
                        + ' data-append-options="percentage"'
                        + ' data-gauge-max-value="100"'
                        + ' data-chart-library="gauge"'
                        + ' data-title="Rows Fetched vs Returned"'
                        + ' data-units="%"'
                        + ' data-gauge-adjust="width"'
                        + ' data-width="12%"'
                        + ' data-before="0"'
                        + ' data-after="-CHART_DURATION"'
                        + ' data-points="CHART_DURATION"'
                        + ' data-colors="' + NETDATA.colors[1] + '"'
                        + ' role="application"></div>';
                }
            ],
        }, 
        info: 'The percentage of rows that contain data needed to execute the query, out of the total number of rows scanned. A high value indicates that the database is executing queries efficiently, while a low value indicates that the database is performing extra work by scanning a large number of rows that aren\'t required to process the query. Low values may be caused by missing indexes or inefficient queries.'
    },
    'postgres.db_ops_read_rows_rate': {
        info: '<p>Read queries throughput.</p><p><b>Returned</b> - Total number of rows scanned by queries. This value indicates rows returned by the storage layer to be scanned, not rows returned to the client. <b>Fetched</b> - Subset of scanned rows (<b>Returned</b>) that contained data needed to execute the query.</p>'
    },
    'postgres.db_ops_write_rows_rate': {
        info: '<p>Write queries throughput.</p><p><b>Inserted</b> - number of rows inserted by queries. <b>Deleted</b> - number of rows deleted by queries. <b>Updated</b> - number of rows updated by queries.</p>'
    },
    'postgres.db_conflicts_rate': {
        info: 'Number of queries canceled due to conflict with recovery on standby servers. To minimize query cancels caused by cleanup records consider configuring <a href="https://www.postgresql.org/docs/current/runtime-config-replication.html#GUC-HOT-STANDBY-FEEDBACK" target="_blank"><i>hot_standby_feedback</i></a>.'
    },
    'postgres.db_conflicts_reason_rate': {
        info: '<p>Statistics about queries canceled due to various types of conflicts on standby servers.</p><p><b>Tablespace</b> - queries that have been canceled due to dropped tablespaces. <b>Lock</b> - queries that have been canceled due to lock timeouts. <b>Snapshot</b> - queries that have been canceled due to old snapshots. <b>Bufferpin</b> - queries that have been canceled due to pinned buffers. <b>Deadlock</b> - queries that have been canceled due to deadlocks.</p>'
    },
    'postgres.db_deadlocks_rate': {
        info: 'Number of detected deadlocks. When a transaction cannot acquire the requested lock within a certain amount of time (configured by <b>deadlock_timeout</b>), it begins deadlock detection.'
    },
    'postgres.db_locks_held_count': {
        info: 'Number of held locks. Some of these lock modes are acquired by PostgreSQL automatically before statement execution, while others are provided to be used by applications. All lock modes acquired in a transaction are held for the duration of the transaction. For lock modes details, see <a href="https://www.postgresql.org/docs/current/explicit-locking.html#LOCKING-TABLES" target="_blank">table-level locks</a>.'
    },
    'postgres.db_locks_awaited_count': {
        info: 'Number of awaited locks. It indicates that some transaction is currently waiting to acquire a lock, which implies that some other transaction is holding a conflicting lock mode on the same lockable object. For lock modes details, see <a href="https://www.postgresql.org/docs/current/explicit-locking.html#LOCKING-TABLES" target="_blank">table-level locks</a>.'
    },
    'postgres.db_temp_files_created_rate': {
        info: 'Number of temporary files created by queries. Complex queries may require more memory than is available (specified by <b>work_mem</b>). When this happens, Postgres reverts to using temporary files - they are actually stored on disk, but only exist for the duration of the request. After the request returns, the temporary files are deleted.'
    },
    'postgres.db_temp_files_io_rate': {
        info: 'Amount of data written temporarily to disk to execute queries.'
    },
    'postgres.db_size': {
        room: {
            mainheads: [
                function (os, id) {
                    void (os);
                    return '<div data-netdata="' + id + '"'
                        + ' data-chart-library="easypiechart"'
                        + ' data-title="DB Size"'
                        + ' data-units="MiB"'
                        + ' data-gauge-adjust="width"'
                        + ' data-width="10%"'
                        + ' data-before="0"'
                        + ' data-after="-CHART_DURATION"'
                        + ' data-points="CHART_DURATION"'
                        + ' role="application"></div>';
                }
            ],
        },        
        info: 'Actual on-disk usage of the database\'s data directory and any associated tablespaces.'
    },
    'postgres.table_rows_dead_ratio': {
        info: 'Percentage of dead rows. An increase in dead rows indicates a problem with VACUUM processes, which can slow down your queries.'
    },
    'postgres.table_rows_count': {
        info: '<p>Number of rows. When you do an UPDATE or DELETE, the row is not actually physically deleted. For a DELETE, the database simply marks the row as unavailable for future transactions, and for UPDATE, under the hood it is a combined INSERT then DELETE, where the previous version of the row is marked unavailable.</p><p><b>Live</b> - rows that currently in use and can be queried. <b>Dead</b> - deleted rows that will later be reused for new rows from INSERT or UPDATE.</p>'
    },
    'postgres.table_ops_rows_rate': {
        info: 'Write queries throughput. If you see a large number of updated and deleted rows, keep an eye on the number of dead rows, as a high percentage of dead rows can slow down your queries.'
    },
    'postgres.table_ops_rows_hot_ratio': {
        info: 'Percentage of HOT (Heap Only Tuple) updated rows. HOT updates are much more efficient than ordinary updates: less write operations, less WAL writes, vacuum operation has less work to do, increased read efficiency (help to limit table and index bloat).'
    },
    'postgres.table_ops_rows_hot_rate': {
        info: 'Number of HOT (Heap Only Tuple) updated rows.'
    },
    'postgres.table_cache_io_ratio': {
        info: 'Table cache inefficiency. Percentage of data read from disk. Lower is better.'
    },
    'postgres.table_io_rate': {
        info: '<p>Amount of data read from shared buffer cache or from disk.</p><p><b>Disk</b> - data read from disk. <b>Memory</b> - data read from buffer cache (this only includes hits in the PostgreSQL buffer cache, not the operating system\'s file system cache).</p>'
    },
    'postgres.table_index_cache_io_ratio': {
        info: 'Table indexes cache inefficiency. Percentage of data read from disk. Lower is better.'
    },
    'postgres.table_index_io_rate': {
        info: '<p>Amount of data read from all indexes from shared buffer cache or from disk.</p><p><b>Disk</b> - data read from disk. <b>Memory</b> - data read from buffer cache (this only includes hits in the PostgreSQL buffer cache, not the operating system\'s file system cache).</p>'
    },
    'postgres.table_toast_cache_io_ratio': {
        info: 'Table TOAST cache inefficiency. Percentage of data read from disk. Lower is better.'
    },
    'postgres.table_toast_io_rate': {
        info: '<p>Amount of data read from TOAST table from shared buffer cache or from disk.</p><p><b>Disk</b> - data read from disk. <b>Memory</b> - data read from buffer cache (this only includes hits in the PostgreSQL buffer cache, not the operating system\'s file system cache).</p>'
    },
    'postgres.table_toast_index_cache_io_ratio': {
        info: 'Table TOAST indexes cache inefficiency. Percentage of data read from disk. Lower is better.'
    },
    'postgres.table_toast_index_io_rate': {
        info: '<p>Amount of data read from this table\'s TOAST table indexes from shared buffer cache or from disk.</p><p><b>Disk</b> - data read from disk. <b>Memory</b> - data read from buffer cache (this only includes hits in the PostgreSQL buffer cache, not the operating system\'s file system cache).</p>'
    },
    'postgres.table_scans_rate': {
        info: '<p>Number of scans initiated on this table. If you see that your database regularly performs more sequential scans over time, you can improve its performance by creating an index on data that is frequently accessed.</p><p><b>Index</b> - relying on an index to point to the location of specific rows. <b>Sequential</b> - have to scan through each row of a table sequentially. Typically, take longer than index scans.</p>'
    },
    'postgres.table_scans_rows_rate': {
        info: 'Number of live rows fetched by scans.'
    },
    'postgres.table_autovacuum_since_time': {
        info: 'Time elapsed since this table was vacuumed by the autovacuum daemon.'
    },
    'postgres.table_vacuum_since_time': {
        info: 'Time elapsed since this table was manually vacuumed (not counting VACUUM FULL).'
    },
    'postgres.table_autoanalyze_since_time': {
        info: 'Time elapsed this table was analyzed by the autovacuum daemon.'
    },
    'postgres.table_analyze_since_time': {
        info: 'Time elapsed since this table was manually analyzed.'
    },
    'postgres.table_null_columns': {
        info: 'Number of table columns that contain only NULLs.'
    },
    'postgres.table_total_size': {
        info: 'Actual on-disk size of the table.'
    },
    'postgres.table_bloat_size_perc': {
        info: 'Estimated percentage of bloat in the table. It is normal for tables that are updated frequently to have a small to moderate amount of bloat.'
    },
    'postgres.table_bloat_size': {
        info: 'Disk space that was used by the table and is available for reuse by the database but has not been reclaimed. Bloated tables require more disk storage and additional I/O that can slow down query execution. Running <a href="https://www.postgresql.org/docs/current/sql-vacuum.html" target="_blank">VACUUM</a> regularly on a table that is updated frequently results in fast reuse of space occupied by expired rows, which prevents the table from growing too large.'
    },
    'postgres.index_size': {
        info: 'Actual on-disk size of the index.'
    },
    'postgres.index_bloat_size_perc': {
        info: 'Estimated percentage of bloat in the index.'
    },
    'postgres.index_bloat_size': {
        info: 'Disk space that was used by the index and is available for reuse by the database but has not been reclaimed. Bloat slows down your database and eats up more storage than needed. To recover the space from indexes, recreate them using the <a href="https://www.postgresql.org/docs/current/sql-reindex.html" target="_blank">REINDEX</a> command.'
    },
    'postgres.index_usage_status': {
        info: 'An index is considered unused if no scans have been initiated on that index.'
    },


    // ------------------------------------------------------------------------
    // PgBouncer
    'pgbouncer.client_connections_utilization': {
        info: 'Client connections in use as percentage of <i>max_client_conn</i> (default 100).'
    },
    'pgbouncer.db_client_connections': {
        info: '<p>Client connections in different states.</p><p><b>Active</b> - linked to server connection and can process queries. <b>Waiting</b> - have sent queries but have not yet got a server connection. <b>CancelReq</b> - have not forwarded query cancellations to the server yet.</p>'
    },
    'pgbouncer.db_server_connections': {
        info: '<p>Server connections in different states.</p><p><b>Active</b> - linked to a client. <b>Idle</b> - unused and immediately usable for client queries. <b>Used</b> - have been idle for more than <i>server_check_delay</i>, so they need <i>server_check_query</i> to run on them before they can be used again. <b>Tested</b> - currently running either <i>server_reset_query</i> or <i>server_check_query</i>. <b>Login</b> - currently in the process of logging in.</p>'
    },
    'pgbouncer.db_server_connections_utilization': {
        info: 'Server connections in use as percentage of <i>max_db_connections</i> (default 0 - unlimited). This considers the PgBouncer database that the client has connected to, not the PostgreSQL database of the outgoing connection.'
    },
    'pgbouncer.db_clients_wait_time': {
        info: 'Time spent by clients waiting for a server connection. This shows if the decrease in database performance from the client\'s point of view was due to exhaustion of the corresponding PgBouncer pool.'
    },
    'pgbouncer.db_client_max_wait_time': {
        info: 'Waiting time for the first (oldest) client in the queue. If this starts increasing, then the current pool of servers does not handle requests quickly enough.'
    },
    'pgbouncer.db_transactions': {
        info: 'SQL transactions pooled (proxied) by pgbouncer.'
    },
    'pgbouncer.db_transactions_time': {
        info: 'Time spent by pgbouncer when connected to PostgreSQL in a transaction, either idle in transaction or executing queries.'
    },
    'pgbouncer.db_transaction_avg_time': {
        info: 'Average transaction duration.'
    },
    'pgbouncer.db_queries': {
        info: 'SQL queries pooled (proxied) by pgbouncer.'
    },
    'pgbouncer.db_queries_time': {
        info: 'Time spent by pgbouncer when actively connected to PostgreSQL, executing queries.'
    },
    'pgbouncer.db_query_avg_time': {
        info: 'Average query duration.'
    },
    'pgbouncer.db_network_io': {
        info: '<p>Network traffic received and sent by pgbouncer.</p><p><b>Received</b> - received from clients. <b>Sent</b> - sent to servers.</p>'
    },

    // ------------------------------------------------------------------------
    // CASSANDRA

    'cassandra.client_requests_rate': {
        info: 'Client requests received per second. Consider whether your workload is read-heavy or write-heavy while choosing a compaction strategy.'
    },
    'cassandra.client_requests_latency': {
        info: 'Response latency of requests received per second. Latency could be impacted by disk access, network latency or replication configuration.'
    },
    'cassandra.key_cache_hit_ratio': {
        info: 'Key cache hit ratio indicates the efficiency of the key cache. If ratio is consistently < 80% consider increasing cache size.'
    },
    'cassandra.key_cache_hit_rate': {
        info: 'Key cache hit rate measures the cache hits and misses per second.'
    },
    'cassandra.storage_live_disk_space_used': {
        info: 'Amount of live disk space used. This does not include obsolete data waiting to be garbage collected.'
    },
    'cassandra.compaction_completed_tasks_rate': {
        info: 'Compaction tasks completed per second.'
    },
    'cassandra.compaction_pending_tasks_count': {
        info: 'Total compaction tasks in queue.'
    },
    'cassandra.thread_pool_active_tasks_count': {
        info: 'Total tasks currently being processed.'
    },
    'cassandra.thread_pool_pending_tasks_count': {
        info: 'Total tasks in queue awaiting a thread for processing.'
    },
    'cassandra.thread_pool_blocked_tasks_rate': {
        info: 'Tasks that cannot be queued for processing yet.'
    },
    'cassandra.thread_pool_blocked_tasks_count': {
        info: 'Total tasks that cannot yet be queued for processing.'
    },
    'cassandra.jvm_gc_rate': {
        info: 'Rate of garbage collections.</p><p><b>ParNew</b> - young-generation. <b>cms (ConcurrentMarkSweep)</b> - old-generation.</p>'
    },
    'cassandra.jvm_gc_time': {
        info: 'Elapsed time of garbage collection.</p><p><b>ParNew</b> - young-generation. <b>cms (ConcurrentMarkSweep)</b> - old-generation.</p>'
    },
    'cassandra.client_requests_timeouts_rate': {
        info: 'Requests which were not acknowledged within the configurable timeout window.'
    },
    'cassandra.client_requests_unavailables_rate': {
        info: 'Requests for which the required number of nodes was unavailable.'
    },
    'cassandra.storage_exceptions_rate': {
        info: 'Requests for which a storage exception was encountered.'
    },

    // ------------------------------------------------------------------------
    // Consul
    'consul.node_health_check_status': {
        info: 'The current status of the <a href="https://developer.hashicorp.com/consul/tutorials/developer-discovery/service-registration-health-checks#monitor-a-node" target="_blank">node health check</a>. A node health check monitors the health of the entire node. If the node health check fails, Consul marks the node as unhealthy.'
    },
    'consul.service_health_check_status': {
        info: 'The current status of the <a href="https://developer.hashicorp.com/consul/tutorials/developer-discovery/service-registration-health-checks#monitor-a-service" target="_blank">service health check</a>. A service check only affects the health of the service it is associated with. If the service health check fails, the DNS interface stops returning that service.'
    },
    'consul.client_rpc_requests_rate': {
        info: 'The number of RPC requests to a Consul server.'
    },
    'consul.client_rpc_requests_exceeded_rate': {
        info: 'The number of rate-limited RPC requests to a Consul server. An Increase of this metric either indicates the load is getting high enough to limit the rate or a <a href="https://developer.hashicorp.com/consul/docs/agent/config/config-files#limits" target="_blank">incorrectly configured</a> Consul agent.'
    },
    'consul.client_rpc_requests_failed_rate': {
        info: 'The number of failed RPC requests to a Consul server.'
    },
    'consul.memory_allocated': {
        info: 'The amount of memory allocated by the Consul process.'
    },
    'consul.memory_sys': {
        info: 'The amount of memory obtained from the OS.'
    },
    'consul.gc_pause_time': {
        info: 'The amount of time spent in garbage collection (GC) pauses. GC pause is a "stop-the-world" event, meaning that all runtime threads are blocked until GC completes. If memory usage is high, the Go runtime may GC so frequently that it starts to slow down Consul.'
    },
    'consul.kvs_apply_time': {
        info: 'The time it takes to complete an update to the KV store.'
    },
    'consul.kvs_apply_operations_rate': {
        info: 'The number of KV store updates.'
    },
    'consul.txn_apply_time': {
        info: 'The time spent applying a transaction operation.'
    },
    'consul.txn_apply_operations_rate': {
        info: 'The number of applied transaction operations.'
    },
    'consul.raft_commit_time': {
        info: 'The time it takes to commit a new entry to the Raft log on the leader.'
    },
    'consul.raft_commits_rate': {
        info: 'The number of applied Raft transactions.'
    },
    'consul.autopilot_health_status': {
        info: 'The overall health of the local server cluster. The status is healthy if <b>all servers</b> are considered healthy by Autopilot.'
    },
    'consul.autopilot_server_health_status': {
        info: 'Whether the server is healthy according to the current <a href="https://developer.hashicorp.com/consul/tutorials/datacenter-operations/autopilot-datacenter-operations#server-health-checking", target="_blank">Autopilot configuration</a>.'
    },
    'consul.autopilot_server_stable_time': {
        info: 'The time this server has been in its current state.'
    },
    'consul.autopilot_server_serf_status': {
        info: 'The SerfHealth check status for the server.'
    },
    'consul.autopilot_server_voter_status': {
        info: 'Whether the server is a voting member of the Raft cluster.'
    },
    'consul.autopilot_failure_tolerance': {
        info: 'The number of voting servers that the cluster can lose while continuing to function.'
    },
    'consul.network_lan_rtt': {
        info: '<a href="https://developer.hashicorp.com/consul/docs/architecture/coordinates#working-with-coordinates" target="_blank">Estimated</a> network round-trip time between this node and other nodes of the cluster.'
    },
    'consul.raft_leader_last_contact_time': {
        info: 'The time since the leader was last able to contact the follower nodes when checking its leader lease.'
    },
    'consul.raft_follower_last_contact_leader_time': {
        info: 'The time elapsed since this server last contacted the leader.'
    },
    'consul.raft_leader_elections_rate': {
        info: 'The number of leadership elections. Increments whenever a Consul server starts an election.'
    },
    'consul.raft_leadership_transitions_rate': {
        info: 'The number of leadership elections. Increments whenever a Consul server becomes a leader.'
    },
    'consul.server_leadership_status': {
        info: 'The Consul server leadership status.'
    },
    'consul.raft_thread_main_saturation_perc': {
        info: 'An approximate measurement of the proportion of time the main Raft goroutine is busy and unavailable to accept new work.'
    },
    'consul.raft_thread_fsm_saturation_perc': {
        info: 'An approximate measurement of the proportion of time the Raft FSM goroutine is busy and unavailable to accept new work.'
    },
    'consul.raft_fsm_last_restore_duration': {
        info: 'The time taken to restore the FSM from a snapshot on an agent restart or from the leader calling <i>installSnapshot</i>.'
    },
    'consul.raft_leader_oldest_log_age': {
        info: 'The time elapsed since the oldest journal was written to the leader\'s journal storage. This can be important for the health of replication when the write rate is high and the snapshot is large, because followers may not be able to recover from a restart if recovery takes longer than the minimum for the current leader.'
    },
    'consul.raft_rpc_install_snapshot_time': {
        info: 'The time it takes to process the <i>installSnapshot</i> RPC call.'
    },
    'consul.raft_boltdb_freelist_bytes': {
        info: 'The number of bytes necessary to encode the freelist metadata. When <a href="https://developer.hashicorp.com/consul/docs/agent/config/config-files#NoFreelistSync" target="_blank">raft_boltdb.NoFreelistSync</a> is set to <i>false</i> these metadata bytes must also be written to disk for each committed log.'
    },
    'consul.raft_boltdb_logs_per_batch_rate': {
        info: 'The number of logs written per batch to the database.'
    },
    'consul.raft_boltdb_store_logs_time': {
        info: 'The amount of time spent writing logs to the database.'
    },
    'consul.license_expiration_time': {
        info: 'The amount of time remaining before Consul Enterprise license expires. When the license expires, some Consul Enterprise features will stop working.'
    },

    // ------------------------------------------------------------------------
    // Windows (Process)

    'windows.processes_cpu_time': {
        info: 'Total CPU utilization. The amount of time spent by the process in <a href="https://en.wikipedia.org/wiki/CPU_modes#Mode_types" target="_blank">user and privileged</a> modes.'
    },
    'windows.processes_handles': {
        info: 'Total number of <a href="https://learn.microsoft.com/en-us/windows/win32/sysinfo/handles-and-objects" target="_blank">handles</a> the process has open. This number is the sum of the handles currently open by each thread in the process.'
    },
    'windows.processes_io_bytes': {
        info: 'Bytes issued to I/O operations in different modes (read, write, other). This property counts all I/O activity generated by the process to include file, network, and device I/Os. Read and write mode includes data operations; other mode includes those that do not involve data, such as control operations.'
    },
    'windows.processes_io_operations': {
        info: 'I/O operations issued in different modes (read, write, other). This property counts all I/O activity generated by the process to include file, network, and device I/Os. Read and write mode includes data operations; other mode includes those that do not involve data, such as control operations.'
    },
    'windows.processes_page_faults': {
        info: 'Page faults by the threads executing in this process. A page fault occurs when a thread refers to a virtual memory page that is not in its working set in main memory. This can cause the page not to be fetched from disk if it is on the standby list and hence already in main memory, or if it is in use by another process with which the page is shared.'
    },
    'windows.processes_file_bytes': {
        info: 'Current number of bytes this process has used in the paging file(s). Paging files are used to store pages of memory used by the process that are not contained in other files. Paging files are shared by all processes, and lack of space in paging files can prevent other processes from allocating memory.'
    },
    'windows.processes_pool_bytes': {
        info: 'Pool Bytes is the last observed number of bytes in the paged or nonpaged pool. The nonpaged pool is an area of system memory (physical memory used by the operating system) for objects that cannot be written to disk, but must remain in physical memory as long as they are allocated. The paged pool is an area of system memory (physical memory used by the operating system) for objects that can be written to disk when they are not being used.'
    },
    'windows.processes_threads': {
        info: 'Number of threads currently active in this process. An instruction is the basic unit of execution in a processor, and a thread is the object that executes instructions. Every running process has at least one thread.'
    },

    // ------------------------------------------------------------------------
    // Windows (TCP)

    'windows.tcp_conns_active': {
        info: 'Number of times TCP connections have made a direct transition from the CLOSED state to the SYN-SENT state.'
    },
    'windows.tcp_conns_established': {
        info: 'Number of TCP connections for which the current state is either ESTABLISHED or CLOSE-WAIT.'
    },
    'windows.tcp_conns_failures': {
        info: 'Number of times TCP connections have made a direct transition to the CLOSED state from the SYN-SENT state or the SYN-RCVD state, plus the number of times TCP connections have made a direct transition from the SYN-RCVD state to the LISTEN state.'
    },
    'windows.tcp_conns_passive': {
        info: 'Number of times TCP connections have made a direct transition from the LISTEN state to the SYN-RCVD state.'
    },
    'windows.tcp_conns_resets': {
        info: 'Number of times TCP connections have made a direct transition from the LISTEN state to the SYN-RCVD state.'
    },
    'windows.tcp_segments_received': {
        info: 'Rate at which segments are received, including those received in error. This count includes segments received on currently established connections.'
    },
    'windows.tcp_segments_retransmitted': {
        info: 'Rate at which segments are retransmitted, that is, segments transmitted that contain one or more previously transmitted bytes.'
    },
    'windows.tcp_segments_sent': {
        info: 'Rate at which segments are sent, including those on current connections, but excluding those containing only retransmitted bytes.'
    },

    // ------------------------------------------------------------------------
    // Windows (IIS)

    'iis.website_isapi_extension_requests_count': {
        info: 'The number of <a href="https://learn.microsoft.com/en-us/previous-versions/iis/6.0-sdk/ms525282(v=vs.90)" target="_blank">ISAPI extension</a> requests that are processed concurrently by the web service.'
    },
    'iis.website_errors_rate': {
        info: '<p>The number of requests that cannot be satisfied by the server.</p><p><b>DocumentLocked</b> - the requested document was locked. Usually reported as HTTP error 423. <b>DocumentNotFound</b> - the requested document was not found. Usually reported as HTTP error 404.</p>'
    },

    // ------------------------------------------------------------------------
    // Windows (Service)

    'windows.service_status': {
        info: 'The current <a href="https://learn.microsoft.com/en-us/windows/win32/services/service-status-transitions" target="_blank">status</a> of the service.'
    },

    // ------------------------------------------------------------------------
    // Windows (MSSQL)

    'mssql.instance_accessmethods_page_splits': {
        info : 'Page split happens when the page does not have more space. This chart shows the number of page splits per second that occur as the result of overflowing index pages.'
    },

    'mssql.instance_cache_hit_ratio': {
        info : 'Indicates the percentage of pages found in the buffer cache without having to read from disk. The ratio is the total number of cache hits divided by the total number of cache lookups over the last few thousand page accesses. After a long period of time, the ratio moves very little. Because reading from the cache is much less expensive than reading from disk, you want this ratio to be high.'
    },

    'mssql.instance_bufman_checkpoint_pages': {
        info : 'Indicates the number of pages flushed to disk per second by a checkpoint or other operation that require all dirty pages to be flushed.'
    },

    'mssql.instance_bufman_page_life_expectancy': {
        info : 'Indicates the number of seconds a page will stay in the buffer pool without references.'
    },

    'mssql.instance_memmgr_external_benefit_of_memory': {
        info : 'It is used by the engine to balance memory usage between cache and is useful to support when troubleshooting cases with unexpected cache growth. The value is presented as an integer based on an internal calculation.'
    },

    'mssql.instance_sql_errors': {
        info: 'Errors in Microsoft SQL Server.</p><p><b>Db_offline</b> - Tracks severe errors that cause SQL Server to take the current database offline. <b>Info</b> - Information related to error messages that provide information to users but do not cause errors. <b>Kill_connection</b> - Tracks severe errors that cause SQL Server to kill the current connection. <b>User</b> - User errors.</p>'
    },

    'mssql.instance_sqlstats_auto_parameterization_attempts': {
        info: 'Auto-parameterization occurs when an instance of SQL Server tries to parameterize a Transact-SQL request by replacing some literals with parameters so that reuse of the resulting cached execution plan across multiple similar-looking requests is possible. Note that auto-parameterizations are also known as simple parameterizations in newer versions of SQL Server. This counter does not include forced parameterizations.'
    },

    'mssql.instance_sqlstats_batch_requests': {
        info: 'This statistic is affected by all constraints (such as I/O, number of users, cache size, complexity of requests, and so on). High batch requests mean good throughput.'
    },

    'mssql.instance_sqlstats_safe_auto_parameterization_attempts': {
        info: 'Note that auto-parameterizations are also known as simple parameterizations in later versions of SQL Server.'
    },

    'mssql.instance_sqlstats_sql_compilations': {
        info: 'Indicates the number of times the compile code path is entered. Includes compiles caused by statement-level recompilations in SQL Server. After SQL Server user activity is stable, this value reaches a steady state.'
    },

    // ------------------------------------------------------------------------
    // Windows (AD)

    'ad.dra_replication_intersite_compressed_traffic': {
        info: 'The compressed size, in bytes, of inbound and outbound compressed replication data (size after compression, from DSAs in other sites).'
    },

    'ad.dra_replication_intrasite_compressed_traffic': {
        info: 'The number of bytes replicated that were not compressed (that is., from DSAs in the same site).'
    },

    'ad.dra_replication_properties_updated': {
        info: 'The number of properties that are updated due to incoming property winning the reconciliation logic that determines the final value to be replicated.'
    },

    'ad.dra_replication_objects_filtered': {
        info: 'The number of objects received from inbound replication partners that contained no updates that needed to be applied.'
    },

    'ad.dra_replication_pending_syncs': {
        info: 'The number of directory synchronizations that are queued for this server but not yet processed.'
    },

    'ad.dra_replication_sync_requests': {
        info: 'The number of directory synchronizations that are queued for this server but not yet processed.'
    },

    // ------------------------------------------------------------------------
    // Windows (NET Framework: Exception)

    'netframework.clrexception_thrown': {
        info: 'The exceptions include both .NET exceptions and unmanaged exceptions that are converted into .NET exceptions.'
    },

    'netframework.clrexception_filters': {
        info: 'An exception filter evaluates regardless of whether an exception is handled.'
    },

    'netframework.clrexception_finallys': {
        info: 'The metric counts only the finally blocks executed for an exception; finally blocks on normal code paths are not counted by this counter.'
    },

    // ------------------------------------------------------------------------
    // Windows (NET Framework: Interop)

    'netframework.clrinterop_com_callable_wrappers': {
        info: 'A COM callable wrappers (CCW) is a proxy for a managed object being referenced from an unmanaged COM client.'
    },

    'netframework.clrinterop_interop_stubs_created': {
        info: 'The Stubs are responsible for marshaling arguments and return values from managed to unmanaged code, and vice versa, during a COM interop call or a platform invoke call.'
    },

    // ------------------------------------------------------------------------
    // Windows (NET Framework: JIT)

    'netframework.clrjit_methods': {
        info: 'The metric does not include pre-JIT-compiled methods.'
    },

    'netframework.clrjit_time': {
        info: 'The metric is updated at the end of every JIT compilation phase. A JIT compilation phase occurs when a method and its dependencies are compiled.'
    },

    'netframework.clrjit_standard_failures': {
        info: 'The failure can occur if the MSIL cannot be verified or if there is an internal error in the JIT compiler.'
    },

    // ------------------------------------------------------------------------
    // Windows (NET Framework: Loading)

    'netframework.clrloading_loader_heap_size': {
        info: 'The memory committed by the class loader across all application domains is the physical space reserved in the disk paging file.'
    },

    'netframework.clrloading_assemblies_loaded': {
        info: 'If the assembly is loaded as domain-neutral from multiple application domains, the metric is incremented only once.'
    },

    // ------------------------------------------------------------------------
    // Windows (NET Framework: Locks and Threads)

    'netframework.clrlocksandthreads_recognized_threads': {
        info: 'Displays the total number of threads that have been recognized by the runtime since the application started. These threads are associated with a corresponding managed thread object. The runtime does not create these threads, but they have run inside the runtime at least once.'
    },

    // ------------------------------------------------------------------------
    // Windows (NET Framework: Memory)

    'netframework.clrmemory_heap_size': {
        info: 'The metric shows maximum bytes that can be allocated, but it does not indicate the current number of bytes allocated.'
    },

    'netframework.clrmemory_promoted': {
        info: 'Memory is promoted when it survives a garbage collection.'
    },

    'netframework.clrmemory_number_gc_handles': {
        info: 'Garbage collection handles are handles to resources external to the common language runtime and the managed environment.'
    },

    'netframework.clrmemory_induced_gc': {
        info: 'The metric is updated when an explicit call to GC.Collect happens.'
    },

    'netframework.clrmemory_number_sink_blocks_in_use': {
        info: 'Synchronization blocks are per-object data structures allocated for storing synchronization information. They hold weak references to managed objects and must be scanned by the garbage collector.'
    },

    'netframework.clrmemory_committed': {
        info: 'Committed memory is the physical memory for which space has been reserved in the disk paging file.'
    },

    'netframework.clrmemory_reserved': {
        info: 'Reserved memory is the virtual memory space reserved for the application when no disk or main memory pages have been used.'
    },

    'netframework.clrmemory_gc_time': {
        info: 'Displays the percentage of time that was spent performing a garbage collection in the last sample.'
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
    // NGINX Plus
    'nginxplus.client_connections_rate': {
        info: 'Accepted and dropped (not handled) connections. A connection is considered <b>dropped</b> if the worker process is unable to get a connection for the request by establishing a new connection or reusing an open one.'
    },
    'nginxplus.client_connections_count': {
        info: 'The current number of client connections. A connection is considered <b>idle</b> if there are currently no active requests.'
    },
    'nginxplus.ssl_handshakes_rate': {
        info: 'Successful and failed SSL handshakes.'
    },
    'nginxplus.ssl_session_reuses_rate': {
        info: 'The number of session reuses during SSL handshake.'
    },
    'nginxplus.ssl_handshakes_failures_rate': {
        info: '<p>SSL handshake failures.</p><p><b>NoCommonProtocol</b> - failed because of no common protocol. <b>NoCommonCipher</b> - failed because of no shared cipher. <b>Timeout</b> - failed because of a timeout. <b>PeerRejectedCert</b> - failed because a client rejected the certificate.</p>'
    },
    'nginxplus.ssl_verification_errors_rate': {
        info: '<p>SSL verification errors.</p><p><b>NoCert</b> - a client did not provide the required certificate. <b>ExpiredCert</b> - an expired or not yet valid certificate was presented by a client. <b>RevokedCert</b> - a revoked certificate was presented by a client. <b>HostnameMismatch</b> - server\'s certificate does not match the hostname. <b>Other</b> - other SSL certificate verification errors.</p>'
    },
    'nginxplus.http_requests_rate': {
        info: 'The number of HTTP requests received from clients.'
    },
    'nginxplus.http_requests_count': {
        info: 'The current number of client requests.'
    },
    'nginxplus.uptime': {
        info: 'The time elapsed since the NGINX process was started.'
    },
    'nginxplus.http_server_zone_requests_rate': {
        info: 'The number of requests to the HTTP Server Zone.'
    },
    'nginxplus.http_server_zone_responses_per_code_class_rate': {
        info: 'The number of responses from the HTTP Server Zone. Responses grouped by HTTP status code class.'
    },
    'nginxplus.http_server_zone_traffic_rate': {
        info: 'The amount of data transferred to and from the HTTP Server Zone.'
    },
    'nginxplus.http_server_zone_requests_processing_count': {
        info: 'The number of client requests that are currently being processed by the HTTP Server Zone.'
    },
    'nginxplus.http_server_zone_requests_discarded_rate': {
        info: 'The number of requests to the HTTP Server Zone completed without sending a response.'
    },
    'nginxplus.http_location_zone_requests_rate': {
        info: 'The number of requests to the HTTP Location Zone.'
    },
    'nginxplus.http_location_zone_responses_per_code_class_rate': {
        info: 'The number of responses from the HTTP Location Zone. Responses grouped by HTTP status code class.'
    },
    'nginxplus.http_location_zone_traffic_rate': {
        info: 'The amount of data transferred to and from the HTTP Location Zone.'
    },
    'nginxplus.http_location_zone_requests_discarded_rate': {
        info: 'The number of requests to the HTTP Location Zone completed without sending a response.'
    },
    'nginxplus.http_upstream_peers_count': {
        info: 'The number of HTTP Upstream servers.'
    },
    'nginxplus.http_upstream_zombies_count': {
        info: 'The current number of HTTP Upstream servers removed from the group but still processing active client requests.'
    },
    'nginxplus.http_upstream_keepalive_count': {
        info: 'The current number of idle keepalive connections to the HTTP Upstream.'
    },
    'nginxplus.http_upstream_server_requests_rate': {
        info: 'The number of client requests forwarded to the HTTP Upstream Server.'
    },
    'nginxplus.http_upstream_server_responses_per_code_class_rate': {
        info: 'The number of responses received from the HTTP Upstream Server. Responses grouped by HTTP status code class.'
    },
    'nginxplus.http_upstream_server_response_time': {
        info: 'The average time to get a complete response from the HTTP Upstream Server.'
    },
    'nginxplus.http_upstream_server_response_header_time': {
        info: 'The average time to get a response header from the HTTP Upstream Server.'
    },
    'nginxplus.http_upstream_server_traffic_rate': {
        info: 'The amount of traffic transferred to and from the HTTP Upstream Server.'
    },
    'nginxplus.http_upstream_server_state': {
        info: 'The current state of the HTTP Upstream Server. Status active if set to 1.'
    },
    'nginxplus.http_upstream_server_connections_count': {
        info: 'The current number of active connections to the HTTP Upstream Server.'
    },
    'nginxplus.http_upstream_server_downtime': {
        info: 'The time the HTTP Upstream Server has spent in the <b>unavail</b>, <b>checking</b>, and <b>unhealthy</b> states.'
    },
    'nginxplus.http_cache_state': {
        info: 'HTTP cache current state. <b>Cold</b> means that the cache loader process is still loading data from disk into the cache.'
    },
    'nginxplus.http_cache_iops': {
        info: '<p>HTTP cache IOPS.</p><p><b>Served</b> - valid, expired, and revalidated responses read from the cache. <b>Written</b> - miss, expired, and bypassed responses written to the cache. <b>Bypassed</b> - miss, expired, and bypass responses.</p>'
    },
    'nginxplus.http_cache_io': {
        info: '<p>HTTP cache IO.</p><p><b>Served</b> - valid, expired, and revalidated responses read from the cache. <b>Written</b> - miss, expired, and bypassed responses written to the cache. <b>Bypassed</b> - miss, expired, and bypass responses.</p>'
    },
    'nginxplus.http_cache_size': {
        info: 'The current size of the cache.'
    },
    'nginxplus.stream_server_zone_connections_rate': {
        info: 'The number of accepted connections to the Stream Server Zone.'
    },
    'nginxplus.stream_server_zone_sessions_per_code_class_rate': {
        info: 'The number of completed sessions for the Stream Server Zone. Sessions grouped by status code class.'
    },
    'nginxplus.stream_server_zone_traffic_rate': {
        info: 'The amount of data transferred to and from the Stream Server Zone.'
    },
    'nginxplus.stream_server_zone_connections_processing_count': {
        info: 'The number of client connections to the Stream Server Zone that are currently being processed.'
    },
    'nginxplus.stream_server_zone_connections_discarded_rate': {
        info: 'The number of connections to the Stream Server Zone completed without creating a session.'
    },
    'nginxplus.stream_upstream_peers_count': {
        info: 'The number of Stream Upstream servers.'
    },
    'nginxplus.stream_upstream_zombies_count': {
        info: 'The current number of HTTP Upstream servers removed from the group but still processing active client connections.'
    },
    'nginxplus.stream_upstream_server_connections_rate': {
        info: 'The number of connections forwarded to the Stream Upstream Server.'
    },
    'nginxplus.stream_upstream_server_traffic_rate': {
        info: 'The amount of traffic transferred to and from the Stream Upstream Server.'
    },
    'nginxplus.stream_upstream_server_state': {
        info: 'The current state of the Stream Upstream Server. Status active if set to 1.'
    },
    'nginxplus.stream_upstream_server_downtime': {
        info: 'The time the Stream Upstream Server has spent in the <b>unavail</b>, <b>checking</b>, and <b>unhealthy</b> states.'
    },
    'nginxplus.stream_upstream_server_connections_count': {
        info: 'The current number of connections to the Stream Upstream Server.'
    },
    'nginxplus.resolver_zone_requests_rate': {
        info: '<p>Resolver zone DNS requests.</p><p><b>Name</b> - requests to resolve names to addresses. <b>Srv</b> - requests to resolve SRV records. <b>Addr</b> - requests to resolve addresses to names.</p>'
    },
    'nginxplus.resolver_zone_responses_rate': {
        info: '<p>Resolver zone DNS responses.</p><p><b>NoError</b> - successful responses. <b>FormErr</b> - format error responses. <b>ServFail</b> - server failure responses. <b>NXDomain</b> - host not found responses. <b>NotImp</b> - unimplemented responses. <b>Refused</b> - operation refused responses. <b>TimedOut</b> - timed out requests. <b>Unknown</b> - requests completed with an unknown error.</p>'
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

    'netdata.ebpf_threads': {
        info: 'Show total number of threads and number of active threads. For more details about the threads, see the <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#ebpf-programs-configuration-options" target="_blank">official documentation</a>.'
    },

    'netdata.ebpf_load_methods': {
        info: 'Show number of threads loaded using legacy code (independent binary) or <code>CO-RE (Compile Once Run Everywhere)</code>.'
    },

    'netdata.ebpf_kernel_memory': {
        info: 'Show amount of memory allocated inside kernel ring for hash tables. This chart shows the same information displayed by command `bpftool map show`.'
    },

    'netdata.ebpf_hash_tables_count': {
        info: 'Show total number of hash tables loaded by eBPF.plugin`.'
    },

    'netdata.ebpf_aral_stat_size': {
        info: 'Show total memory allocated for the specific ARAL.'
    },

    'netdata.ebpf_aral_stat_alloc': {
        info: 'Show total memory of calls to get a specific region of memory inside an ARAL region.'
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
    // containers

    'cgroup.cpu_limit': {
        valueRange: "[0, null]",
        mainheads: [
            function (_, id) {
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
        ],
        info: cgroupCPULimit
    },
    'cgroup.cpu': {
        mainheads: [
            function (_, id) {
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
        ],
        info: cgroupCPU
    },
    'cgroup.throttled': {
        info: cgroupThrottled
    },
    'cgroup.throttled_duration': {
        info: cgroupThrottledDuration
    },
    'cgroup.cpu_shares': {
        info: cgroupCPUShared
    },
    'cgroup.cpu_per_core': {
        info: cgroupCPUPerCore
    },
    'cgroup.cpu_some_pressure': {
        info: cgroupCPUSomePressure
    },
    'cgroup.cpu_some_pressure_stall_time': {
        info: cgroupCPUSomePressureStallTime
    },
    'cgroup.cpu_full_pressure': {
        info: cgroupCPUFullPressure
    },
    'cgroup.cpu_full_pressure_stall_time': {
        info: cgroupCPUFullPressureStallTime
    },

    'k8s.cgroup.cpu_limit': {
        valueRange: "[0, null]",
        mainheads: [
            function (_, id) {
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
        ],
        info: cgroupCPULimit
    },
    'k8s.cgroup.cpu': {
        mainheads: [
            function (_, id) {
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
        ],
        info: cgroupCPU
    },
    'k8s.cgroup.throttled': {
        info: cgroupThrottled
    },
    'k8s.cgroup.throttled_duration': {
        info: cgroupThrottledDuration
    },
    'k8s.cgroup.cpu_shares': {
        info: cgroupCPUShared
    },
    'k8s.cgroup.cpu_per_core': {
        info: cgroupCPUPerCore
    },
    'k8s.cgroup.cpu_some_pressure': {
        info: cgroupCPUSomePressure
    },
    'k8s.cgroup.cpu_some_pressure_stall_time': {
        info: cgroupCPUSomePressureStallTime
    },
    'k8s.cgroup.cpu_full_pressure': {
        info: cgroupCPUFullPressure
    },
    'k8s.cgroup.cpu_full_pressure_stall_time': {
        info: cgroupCPUFullPressureStallTime
    },

    'cgroup.mem_utilization': {
        info: cgroupMemUtilization
    },
    'cgroup.mem_usage_limit': {
        mainheads: [
            function (_, id) {
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
        ],
        info: cgroupMemUsageLimit
    },
    'cgroup.mem_usage': {
        mainheads: [
            function (_, id) {
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
        ],
        info: cgroupMemUsage
    },
    'cgroup.mem': {
        info: cgroupMem
    },
    'cgroup.mem_failcnt': {
        info: cgroupMemFailCnt
    },
    'cgroup.writeback': {
        info: cgroupWriteback
    },
    'cgroup.mem_activity': {
        info: cgroupMemActivity
    },
    'cgroup.pgfaults': {
        info: cgroupPgFaults
    },
    'cgroup.memory_some_pressure': {
        info: cgroupMemorySomePressure
    },
    'cgroup.memory_some_pressure_stall_time': {
        info: cgroupMemorySomePressureStallTime
    },
    'cgroup.memory_full_pressure': {
        info: cgroupMemoryFullPressure
    },
    'cgroup.memory_full_pressure_stall_time': {
        info: cgroupMemoryFullPressureStallTime
    },

    'k8s.cgroup.mem_utilization': {
        info: cgroupMemUtilization
    },
    'k8s.cgroup.mem_usage_limit': {
        mainheads: [
            function (_, id) {
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
        ],
        info: cgroupMemUsageLimit
    },
    'k8s.cgroup.mem_usage': {
        mainheads: [
            function (_, id) {
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
        ],
        info: cgroupMemUsage
    },
    'k8s.cgroup.mem': {
        info: cgroupMem
    },
    'k8s.cgroup.mem_failcnt': {
        info: cgroupMemFailCnt
    },
    'k8s.cgroup.writeback': {
        info: cgroupWriteback
    },
    'k8s.cgroup.mem_activity': {
        info: cgroupMemActivity
    },
    'k8s.cgroup.pgfaults': {
        info: cgroupPgFaults
    },
    'k8s.cgroup.memory_some_pressure': {
        info: cgroupMemorySomePressure
    },
    'k8s.cgroup.memory_some_pressure_stall_time': {
        info: cgroupMemorySomePressureStallTime
    },
    'k8s.cgroup.memory_full_pressure': {
        info: cgroupMemoryFullPressure
    },
    'k8s.cgroup.memory_full_pressure_stall_time': {
        info: cgroupMemoryFullPressureStallTime
    },

    'cgroup.io': {
        info: cgroupIO
    },
    'cgroup.serviced_ops': {
        info: cgroupServicedOps
    },
    'cgroup.queued_ops': {
        info: cgroupQueuedOps
    },
    'cgroup.merged_ops': {
        info: cgroupMergedOps
    },
    'cgroup.throttle_io': {
        mainheads: [
            function (_, id) {
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
            function (_, id) {
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
        ],
        info: cgroupThrottleIO
    },
    'cgroup.throttle_serviced_ops': {
        info: cgroupThrottleIOServicesOps
    },
    'cgroup.io_some_pressure': {
        info: cgroupIOSomePressure
    },
    'cgroup.io_some_pressure_stall_time': {
        info: cgroupIOSomePRessureStallTime
    },
    'cgroup.io_full_pressure': {
        info: cgroupIOFullPressure
    },
    'cgroup.io_full_pressure_stall_time': {
        info: cgroupIOFullPressureStallTime
    },

    'k8s.cgroup.io': {
        info: cgroupIO
    },
    'k8s.cgroup.serviced_ops': {
        info: cgroupServicedOps
    },
    'k8s.cgroup.queued_ops': {
        info: cgroupQueuedOps
    },
    'k8s.cgroup.merged_ops': {
        info: cgroupMergedOps
    },
    'k8s.cgroup.throttle_io': {
        mainheads: [
            function (_, id) {
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
            function (_, id) {
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
        ],
        info: cgroupThrottleIO
    },
    'k8s.cgroup.throttle_serviced_ops': {
        info: cgroupThrottleIOServicesOps
    },
    'k8s.cgroup.io_some_pressure': {
        info: cgroupIOSomePressure
    },
    'k8s.cgroup.io_some_pressure_stall_time': {
        info: cgroupIOSomePRessureStallTime
    },
    'k8s.cgroup.io_full_pressure': {
        info: cgroupIOFullPressure
    },
    'k8s.cgroup.io_full_pressure_stall_time': {
        info: cgroupIOFullPressureStallTime
    },

    'cgroup.swap_read': {
        info: ebpfSwapRead
    },
    'cgroup.swap_write': {
        info: ebpfSwapWrite
    },
    'cgroup.fd_open': {
        info: ebpfFileOpen
    },
    'cgroup.fd_open_error': {
        info: ebpfFileOpenError
    },
    'cgroup.fd_close': {
        info: ebpfFileClosed
    },
    'cgroup.fd_close_error': {
        info: ebpfFileCloseError
    },
    'cgroup.vfs_unlink': {
        info: ebpfVFSUnlink
    },
    'cgroup.vfs_write': {
        info: ebpfVFSWrite
    },
    'cgroup.vfs_write_error': {
        info: ebpfVFSWriteError
    },
    'cgroup.vfs_read': {
        info: ebpfVFSRead
    },
    'cgroup.vfs_read_error': {
        info: ebpfVFSReadError
    },
    'cgroup.vfs_write_bytes': {
        info: ebpfVFSWriteBytes
    },
    'cgroup.vfs_read_bytes': {
        info: ebpfVFSReadBytes
    },
    'cgroup.vfs_fsync': {
        info: ebpfVFSSync
    },
    'cgroup.vfs_fsync_error': {
        info: ebpfVFSSyncError
    },
    'cgroup.vfs_open': {
        info: ebpfVFSOpen
    },
    'cgroup.vfs_open_error': {
        info: ebpfVFSOpenError
    },
    'cgroup.vfs_create': {
        info: ebpfVFSCreate
    },
    'cgroup.vfs_create_error': {
        info: ebpfVFSCreateError
    },
    'cgroup.process_create': {
        info: ebpfProcessCreate
    },
    'cgroup.thread_create': {
        info: ebpfThreadCreate
    },
    'cgroup.task_exit': {
        info: ebpfTaskExit
    },
    'cgroup.task_close': {
        info: ebpfTaskClose
    },
    'cgroup.task_error': {
        info: ebpfTaskError
    },
    'cgroup.dc_ratio': {
        info: 'Percentage of file accesses that were present in the directory cache. 100% means that every file that was accessed was present in the directory cache. If files are not present in the directory cache 1) they are not present in the file system, 2) the files were not accessed before. Read more about <a href="https://www.kernel.org/doc/htmldocs/filesystems/the_directory_cache.html" target="_blank">directory cache</a>. Netdata also gives a summary for these charts in <a href="#menu_filesystem_submenu_directory_cache__eBPF_">Filesystem submenu</a>.'
    },
    'cgroup.shmget': {
        info: ebpfSHMget
    },
    'cgroup.shmat': {
        info: ebpfSHMat
    },
    'cgroup.shmdt': {
        info: ebpfSHMdt
    },
    'cgroup.shmctl': {
        info: ebpfSHMctl
    },
    'cgroup.outbound_conn_v4': {
        info: ebpfIPV4conn
    },
    'cgroup.outbound_conn_v6': {
        info: ebpfIPV6conn
    },
    'cgroup.net_bytes_send': {
        info: ebpfBandwidthSent
    },
    'cgroup.net_bytes_recv': {
        info: ebpfBandwidthRecv
    },
    'cgroup.net_tcp_send': {
        info: ebpfTCPSendCall
    },
    'cgroup.net_tcp_recv': {
        info: ebpfTCPRecvCall
    },
    'cgroup.net_retransmit': {
        info: ebpfTCPRetransmit
    },
    'cgroup.net_udp_send': {
        info: ebpfUDPsend
    },
    'cgroup.net_udp_recv': {
        info: ebpfUDPrecv
    },
    'cgroup.dc_hit_ratio': {
        info: ebpfDCHit
    },
    'cgroup.dc_reference': {
        info: ebpfDCReference
    },
    'cgroup.dc_not_cache': {
        info: ebpfDCNotCache
    },
    'cgroup.dc_not_found': {
        info: ebpfDCNotFound
    },
    'cgroup.cachestat_ratio': {
        info: ebpfCachestatRatio
    },
    'cgroup.cachestat_dirties': {
        info: ebpfCachestatDirties
    },
    'cgroup.cachestat_hits': {
        info: ebpfCachestatHits
    },
    'cgroup.cachestat_misses': {
        info: ebpfCachestatMisses
    },

    // ------------------------------------------------------------------------
    // containers (systemd)

    'services.cpu': {
        info: 'Total CPU utilization within the system-wide CPU resources (all cores). '+
        'The amount of time spent by tasks of the cgroup in '+
        '<a href="https://en.wikipedia.org/wiki/CPU_modes#Mode_types" target="_blank">user and kernel</a> modes.'
    },

    'services.mem_usage': {
        info: 'The amount of used RAM.'
    },

    'services.mem_rss': {
        info: 'The amount of used '+
        '<a href="https://en.wikipedia.org/wiki/Resident_set_size" target="_blank">RSS</a> memory. '+
        'It includes transparent hugepages.'
    },

    'services.mem_mapped': {
        info: 'The size of '+
        '<a href="https://en.wikipedia.org/wiki/Memory-mapped_file" target="_blank">memory-mapped</a> files.'
    },

    'services.mem_cache': {
        info: 'The amount of used '+
        '<a href="https://en.wikipedia.org/wiki/Page_cache" target="_blank">page cache</a> memory.'
    },

    'services.mem_writeback': {
        info: 'The amount of file/anon cache that is '+
        '<a href="https://en.wikipedia.org/wiki/Cache_(computing)#Writing_policies" target="_blank">queued for syncing</a> '+
        'to disk.'
    },

    'services.mem_pgfault': {
        info: 'The number of '+
        '<a href="https://en.wikipedia.org/wiki/Page_fault#Types" target="_blank">page faults</a>. '+
        'It includes both minor and major page faults.'
    },

    'services.mem_pgmajfault': {
        info: 'The number of '+
        '<a href="https://en.wikipedia.org/wiki/Page_fault#Major" target="_blank">major</a> '+
        'page faults.'
    },

    'services.mem_pgpgin': {
        info: 'The amount of memory charged to the cgroup. '+
        'The charging event happens each time a page is accounted as either '+
        'mapped anon page(RSS) or cache page(Page Cache) to the cgroup.'
    },

    'services.mem_pgpgout': {
        info: 'The amount of memory uncharged from the cgroup. '+
        'The uncharging event happens each time a page is unaccounted from the cgroup.'
    },

    'services.mem_failcnt': {
        info: 'The number of memory usage hits limits.'
    },

    'services.swap_usage': {
        info: 'The amount of used '+
        '<a href="https://en.wikipedia.org/wiki/Memory_paging#Unix_and_Unix-like_systems" target="_blank">swap</a> '+
        'memory.'
    },

    'services.io_read': {
        info: 'The amount of data transferred from specific devices as seen by the CFQ scheduler. '+
        'It is not updated when the CFQ scheduler is operating on a request queue.'
    },

    'services.io_write': {
        info: 'The amount of data transferred to specific devices as seen by the CFQ scheduler. '+
        'It is not updated when the CFQ scheduler is operating on a request queue.'
    },

    'services.io_ops_read': {
        info: 'The number of read operations performed on specific devices as seen by the CFQ scheduler.'
    },

    'services.io_ops_write': {
        info: 'The number write operations performed on specific devices as seen by the CFQ scheduler.'
    },

    'services.throttle_io_read': {
        info: 'The amount of data transferred from specific devices as seen by the throttling policy.'
    },

    'services.throttle_io_write': {
        info: 'The amount of data transferred to specific devices as seen by the throttling policy.'
    },

    'services.throttle_io_ops_read': {
        info: 'The number of read operations performed on specific devices as seen by the throttling policy.'
    },

    'services.throttle_io_ops_write': {
        info: 'The number of write operations performed on specific devices as seen by the throttling policy.'
    },

    'services.queued_io_ops_read': {
        info: 'The number of queued read requests.'
    },

    'services.queued_io_ops_write': {
        info: 'The number of queued write requests.'
    },

    'services.merged_io_ops_read': {
        info: 'The number of read requests merged.'
    },

    'services.merged_io_ops_write': {
        info: 'The number of write requests merged.'
    },

    'services.swap_read': {
        info: ebpfSwapRead + '<div id="ebpf_services_swap_read"></div>'
    },

    'services.swap_write': {
        info: ebpfSwapWrite + '<div id="ebpf_services_swap_write"></div>'
    },

    'services.fd_open': {
        info: ebpfFileOpen + '<div id="ebpf_services_file_open"></div>'
    },

    'services.fd_open_error': {
        info: ebpfFileOpenError + '<div id="ebpf_services_file_open_error"></div>'
    },

    'services.fd_close': {
        info: ebpfFileClosed + '<div id="ebpf_services_file_closed"></div>'
    },

    'services.fd_close_error': {
        info: ebpfFileCloseError + '<div id="ebpf_services_file_close_error"></div>'
    },

    'services.vfs_unlink': {
        info: ebpfVFSUnlink + '<div id="ebpf_services_vfs_unlink"></div>'
    },

    'services.vfs_write': {
        info: ebpfVFSWrite + '<div id="ebpf_services_vfs_write"></div>'
    },

    'services.vfs_write_error': {
        info: ebpfVFSWriteError + '<div id="ebpf_services_vfs_write_error"></div>'
    },

    'services.vfs_read': {
        info: ebpfVFSRead + '<div id="ebpf_services_vfs_read"></div>'
    },

    'services.vfs_read_error': {
        info: ebpfVFSReadError + '<div id="ebpf_services_vfs_read_error"></div>'
    },

    'services.vfs_write_bytes': {
        info: ebpfVFSWriteBytes + '<div id="ebpf_services_vfs_write_bytes"></div>'
    },

    'services.vfs_read_bytes': {
        info: ebpfVFSReadBytes + '<div id="ebpf_services_vfs_read_bytes"></div>'
    },

    'services.vfs_fsync': {
        info: ebpfVFSSync + '<div id="ebpf_services_vfs_sync"></div>'
    },

    'services.vfs_fsync_error': {
        info: ebpfVFSSyncError + '<div id="ebpf_services_vfs_sync_error"></div>'
    },

    'services.vfs_open': {
        info: ebpfVFSOpen + '<div id="ebpf_services_vfs_open"></div>'
    },

    'services.vfs_open_error': {
        info: ebpfVFSOpenError + '<div id="ebpf_services_vfs_open_error"></div>'
    },

    'services.vfs_create': {
        info: ebpfVFSCreate + '<div id="ebpf_services_vfs_create"></div>'
    },

    'services.vfs_create_error': {
        info: ebpfVFSCreateError + '<div id="ebpf_services_vfs_create_error"></div>'
    },

    'services.process_create': {
        info: ebpfProcessCreate + '<div id="ebpf_services_process_create"></div>'
    },

    'services.thread_create': {
        info: ebpfThreadCreate + '<div id="ebpf_services_thread_create"></div>'
    },

    'services.task_exit': {
        info: ebpfTaskExit + '<div id="ebpf_services_process_exit"></div>'
    },

    'services.task_close': {
        info: ebpfTaskClose + '<div id="ebpf_services_task_release"></div>'
    },

    'services.task_error': {
        info: ebpfTaskError + '<div id="ebpf_services_task_error"></div>'
    },

    'services.dc_hit_ratio': {
        info: ebpfDCHit + '<div id="ebpf_services_dc_hit"></div>'
    },

    'services.dc_reference': {
        info: ebpfDCReference + '<div id="ebpf_services_dc_reference"></div>'
    },

    'services.dc_not_cache': {
        info: ebpfDCNotCache + '<div id="ebpf_services_dc_not_cache"></div>'
    },

    'services.dc_not_found': {
        info: ebpfDCNotFound + '<div id="ebpf_services_dc_not_found"></div>'
    },

    'services.cachestat_ratio': {
        info: ebpfCachestatRatio + '<div id="ebpf_services_cachestat_ratio"></div>'
    },

    'services.cachestat_dirties': {
        info: ebpfCachestatDirties + '<div id="ebpf_services_cachestat_dirties"></div>'
    },

    'services.cachestat_hits': {
        info: ebpfCachestatHits + '<div id="ebpf_services_cachestat_hits"></div>'
    },

    'services.cachestat_misses': {
        info: ebpfCachestatMisses + '<div id="ebpf_services_cachestat_misses"></div>'
    },

    'services.shmget': {
        info: ebpfSHMget + '<div id="ebpf_services_shm_get"></div>'
    },

    'services.shmat': {
        info: ebpfSHMat + '<div id="ebpf_services_shm_at"></div>'
    },

    'services.shmdt': {
        info: ebpfSHMdt + '<div id="ebpf_services_shm_dt"></div>'
    },

    'services.shmctl': {
        info: ebpfSHMctl + '<div id="ebpf_services_shm_ctl"></div>'
    },

    'services.outbound_conn_v4': {
        info: ebpfIPV4conn + '<div id="ebpf_services_outbound_conn_ipv4"></div>'
    },

    'services.outbound_conn_v6': {
        info: ebpfIPV6conn + '<div id="ebpf_services_outbound_conn_ipv6"></div>'
    },

    'services.net_bytes_send': {
        info: ebpfBandwidthSent + '<div id="ebpf_services_bandwidth_sent"></div>'
    },

    'services.net_bytes_recv': {
        info: ebpfBandwidthRecv + '<div id="ebpf_services_bandwidth_received"></div>'
    },

    'services.net_tcp_send': {
        info: ebpfTCPSendCall + '<div id="ebpf_services_bandwidth_tcp_sent"></div>'
    },

    'services.net_tcp_recv': {
        info: ebpfTCPRecvCall + '<div id="ebpf_services_bandwidth_tcp_received"></div>'
    },

    'services.net_retransmit': {
        info: ebpfTCPRetransmit + '<div id="ebpf_services_tcp_retransmit"></div>'
    },

    'services.net_udp_send': {
        info: ebpfUDPsend + '<div id="ebpf_services_udp_sendmsg"></div>'
    },

    'services.net_udp_recv': {
       info: ebpfUDPrecv + '<div id="ebpf_services_udp_recv"></div>'
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
        info: 'Unique client IPs accessing the web server since the last restart of netdata. This plugin keeps in memory all the unique IPs that have accessed the web server. On very busy web servers (several millions of unique IPs) you may want to disable this chart (check <a href="https://github.com/netdata/go.d.plugin/blob/master/config/go.d/web_log.conf" target="_blank"><code>/etc/netdata/go.d/web_log.conf</code></a>).'
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
        info: 'Unique client IPs accessing squid since the last restart of netdata. This plugin keeps in memory all the unique IPs that have accessed the server. On very busy squid servers (several millions of unique IPs) you may want to disable this chart (check <a href="https://github.com/netdata/go.d.plugin/blob/master/config/go.d/web_log.conf" target="_blank"><code>/etc/netdata/go.d/web_log.conf</code></a>).'
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
            'Check the <a href="http://wiki.squid-cache.org/SquidFaq/SquidLogs" target="_blank">squid documentation about them</a>.'
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
    // Chrony

    'chrony.stratum': {
        info: 'The stratum indicates the distance (hops) to the computer with the reference clock. The higher the stratum number, the more the timing accuracy and stability degrades.',
    },

    'chrony.current_correction': {
        info: 'Any error in the system clock is corrected by slightly speeding up or slowing down the system clock until the error has been removed, and then returning to the system clock’s normal speed. A consequence of this is that there will be a period when the system clock (as read by other programs) will be different from chronyd\'s estimate of the current true time (which it reports to NTP clients when it is operating as a server). The reported value is the difference due to this effect.',
    },

    'chrony.root_delay': {
        info: 'The total of the network path delays to the stratum-1 computer from which the computer is ultimately synchronised.'
    },

    'chrony.root_dispersion': {
        info: 'The total dispersion accumulated through all the computers back to the stratum-1 computer from which the computer is ultimately synchronised. Dispersion is due to system clock resolution, statistical measurement variations, etc.'
    },

    'chrony.last_offset': {
        info: 'The estimated local offset on the last clock update. A positive value indicates the local time (as previously estimated true time) was ahead of the time sources.',
    },

    'chrony.frequency': {
        info: 'The <b>frequency</b> is the rate by which the system’s clock would be wrong if chronyd was not correcting it. It is expressed in ppm (parts per million). For example, a value of 1 ppm would mean that when the system’s clock thinks it has advanced 1 second, it has actually advanced by 1.000001 seconds relative to true time.',
    },

    'chrony.residual_frequency': {
        info: 'The <b>residual frequency</b> for the currently selected reference source. This reflects any difference between what the measurements from the reference source indicate the frequency should be and the frequency currently being used. The reason this is not always zero is that a smoothing procedure is applied to the frequency.',
    },

    'chrony.skew': {
        info: 'The estimated error bound on the frequency.',
    },

    'chrony.ref_measurement_time': {
        info: 'The time elapsed since the last measurement from the reference source was processed.',
    },

    'chrony.leap_status': {
        info: '<p>The current leap status of the source.</p><p><b>Normal</b> - indicates the normal status (no leap second). <b>InsertSecond</b> - indicates that a leap second will be inserted at the end of the month. <b>DeleteSecond</b> - indicates that a leap second will be deleted at the end of the month. <b>Unsynchronised</b> - the server has not synchronized properly with the NTP server.</p>',
    },

    'chrony.activity': {
        info: '<p>The number of servers and peers that are online and offline.</p><p><b>Online</b> - the server or peer is currently online (i.e. assumed by chronyd to be reachable). <b>Offline</b> - the server or peer is currently offline (i.e. assumed by chronyd to be unreachable, and no measurements from it will be attempted). <b>BurstOnline</b> - a burst command has been initiated for the server or peer and is being performed. After the burst is complete, the server or peer will be returned to the online state. <b>BurstOffline</b> - a burst command has been initiated for the server or peer and is being performed. After the burst is complete, the server or peer will be returned to the offline state. <b>Unresolved</b> - the name of the server or peer was not resolved to an address yet.</p>',
    },

    // ------------------------------------------------------------------------
    // Couchdb

    'couchdb.active_tasks': {
        info: 'Active tasks running on this CouchDB <b>cluster</b>. Four types of tasks currently exist: indexer (view building), replication, database compaction and view compaction.'
    },

    'couchdb.replicator_jobs': {
        info: 'Detailed breakdown of any replication jobs in progress on this node. For more information, see the <a href="http://docs.couchdb.org/en/latest/replication/replicator.html" target="_blank">replicator documentation</a>.'
    },

    'couchdb.open_files': {
        info: 'Count of all files held open by CouchDB. If this value seems pegged at 1024 or 4096, your server process is probably hitting the open file handle limit and <a href="http://docs.couchdb.org/en/latest/maintenance/performance.html#pam-and-ulimit" target="_blank">needs to be increased.</a>'
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

    'btrfs.commits': {
        info: 'Tracks filesystem wide commits. Commits mark fully consistent synchronization points for the filesystem, and are triggered automatically when certain events happen or when enough time has elapsed since the last commit.'
    },

    'btrfs.commits_perc_time': {
        info: 'Tracks commits time share. The reported time share metrics are valid only when BTRFS commit interval is longer than Netdata\'s <b>update_every</b> interval.'
    },

    'btrfs.commit_timings': {
        info: 'Tracks timing information for commits. <b>last</b> dimension metrics are valid only when BTRFS commit interval is longer than Netdata\'s <b>update_every</b> interval.'
    },

    'btrfs.device_errors': {
        info: 'Tracks per-device error counts. Five types of errors are tracked: read errors, write errors, flush errors, corruption errors, and generation errors. <b>Read</b>, <b>write</b>, and <b>flush</b> are errors reported by the underlying block device when trying to perform the associated operations on behalf of BTRFS. <b>Corruption</b> errors count checksum mismatches, which usually are a result of either at-rest data corruption or hardware problems. <b>Generation</b> errors count generational mismatches within the internal data structures of the volume, and are also usually indicative of at-rest data corruption or hardware problems. Note that errors reported here may not trigger an associated IO error in userspace, as BTRFS has relatively robust error recovery that allows it to return correct data in most multi-device setups.'
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
        info: 'Time constants and poll intervals are expressed as exponents of 2. The default poll exponent of 6 corresponds to a poll interval of 64 s. For typical Internet paths, the optimum poll interval is about 64 s. For fast LANs with modern computers, a poll exponent of 4 (16 s) is appropriate. The <a href="http://doc.ntp.org/current-stable/poll.html" target="_blank">poll process</a> sends NTP packets at intervals determined by the clock discipline algorithm.',
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
        info: 'This variable is used in interleaved mode (used only in NTP symmetric and broadcast modes). See <a href="http://doc.ntp.org/current-stable/xleave.html" target="_blank">NTP Interleaved Modes</a>.'
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
        info: 'The number of currently connected users on the monitored Spigot server.'
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
        info: 'Local and remote sessions.'
    },
    'logind.sessions_type': {
        info: '<p>Sessions of each session type.</p><p><b>Graphical</b> - sessions are running under one of X11, Mir, or Wayland. <b>Console</b> - sessions are usually regular text mode local logins, but depending on how the system is configured may have an associated GUI. <b>Other</b> - sessions are those that do not fall into the above categories (such as sessions for cron jobs or systemd timer units).</p>'
    },
    'logind.sessions_state': {
        info: '<p>Sessions in each session state.</p><p><b>Online</b> - logged in and running in the background. <b>Closing</b> - nominally logged out, but some processes belonging to it are still around. <b>Active</b> - logged in and running in the foreground.</p>'
    },
    'logind.users_state': {
        info: '<p>Users in each user state.</p><p><b>Offline</b> - users are not logged in. <b>Closing</b> - users are in the process of logging out without lingering. <b>Online</b> - users are logged in, but have no active sessions. <b>Lingering</b> - users are not logged in, but have one or more services still running. <b>Active</b> - users are logged in, and have at least one active session.</p>'
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
        info: 'The current battery charge.'
    },

    'powersupply.charge': {
        info: '<p>The battery charge in Amp-hours.</p>'+
        '<p><b>now</b> - actual charge value. '+
        '<b>full</b>, <b>empty</b> - last remembered value of charge when battery became full/empty. '+
        'It also could mean "value of charge when battery considered full/empty at given conditions (temperature, age)". '+
        'I.e. these attributes represents real thresholds, not design values. ' +
        '<b>full_design</b>, <b>empty_design</b> - design charge values, when battery considered full/empty.</p>'
    },

    'powersupply.energy': {
        info: '<p>The battery charge in Watt-hours.</p>'+
        '<p><b>now</b> - actual charge value. '+
        '<b>full</b>, <b>empty</b> - last remembered value of charge when battery became full/empty. '+
        'It also could mean "value of charge when battery considered full/empty at given conditions (temperature, age)". '+
        'I.e. these attributes represents real thresholds, not design values. ' +
        '<b>full_design</b>, <b>empty_design</b> - design charge values, when battery considered full/empty.</p>'
    },

    'powersupply.voltage': {
        info: '<p>The power supply voltage.</p>'+
        '<p><b>now</b> - current voltage. '+
        '<b>max</b>, <b>min</b> - voltage values that hardware could only guess (measure and retain) the thresholds '+
        'of a given power supply. '+
        '<b>max_design</b>, <b>min_design</b> - design values for maximal and minimal power supply voltages. '+
        'Maximal/minimal means values of voltages when battery considered "full"/"empty" at normal conditions.</p>'
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
            'For details see <a href="https://docs.vmware.com/en/VMware-vSphere/6.5/com.vmware.vsphere.resmgmt.doc/GUID-BFDC988B-F53D-4E97-9793-A002445AFAE1.html" target="_blank">Measuring and Differentiating Types of Memory Usage</a> and ' +
            '<a href="https://vdc-repo.vmware.com/vmwb-repository/dcr-public/fe08899f-1eec-4d8d-b3bc-a6664c168c2c/7fdf97a1-4c0d-4be0-9d43-2ceebbc174d9/doc/memory_counters.html" target="_blank">Memory Counters</a> articles.'
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
            'For details see <a href="https://docs.vmware.com/en/VMware-vSphere/6.5/com.vmware.vsphere.resmgmt.doc/GUID-BFDC988B-F53D-4E97-9793-A002445AFAE1.html" target="_blank">Measuring and Differentiating Types of Memory Usage</a> and ' +
            '<a href="https://vdc-repo.vmware.com/vmwb-repository/dcr-public/fe08899f-1eec-4d8d-b3bc-a6664c168c2c/7fdf97a1-4c0d-4be0-9d43-2ceebbc174d9/doc/memory_counters.html" target="_blank">Memory Counters</a> articles.'

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
            'Check the <a href="https://wiki.squid-cache.org/SquidFaq/SquidLogs#Squid_result_codes" target="_blank">squid documentation</a> about them.'
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
        info: 'The request method to obtain an object. Please refer to section <a href="https://wiki.squid-cache.org/SquidFaq/SquidLogs#Request_methods" target="_blank">request-methods</a> for available methods and their description.'
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
    // Perf

    'perf.instructions_per_cycle': {
        info: 'An IPC < 1.0 likely means memory bound, and an IPC > 1.0 likely means instruction bound. For more details about the metric take a look at this <a href="https://www.brendangregg.com/blog/2017-05-09/cpu-utilization-is-wrong.html" target="_blank">blog post</a>.'
    },

    // ------------------------------------------------------------------------
    // Filesystem

    'filesystem.vfs_deleted_objects': {
        title : 'VFS remove',
        info: 'Number of calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS unlinker function</a>. This chart may not show all file system events if it uses other functions ' +
            'to store data on disk. Netdata shows virtual file system metrics per <a href="#ebpf_apps_vfs_unlink">application</a> and <a href="#ebpf_services_vfs_unlink">cgroup (systemd Services)</a> ' +
            'if <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">apps</a> or ' +
            '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">cgroup (systemd Services)</a> plugins are enabled.' + ebpfChartProvides +
            ' to monitor <a href="#menu_filesystem">File Systems</a>.<div id="ebpf_global_vfs_unlink"></div>'
    },

    'filesystem.vfs_io': {
        title : 'VFS IO',
        info: 'Number of calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS I/O functions</a>. This chart may not show all file system events if it uses other functions ' +
              'to store data on disk. Netdata shows virtual file system metrics per <a href="#ebpf_apps_vfs_write">application</a> and <a href="#ebpf_services_vfs_write">cgroup (systemd Services)</a> ' +
              'if <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">apps</a> or ' +
              '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">cgroup (systemd Services)</a> plugins are enabled.' + ebpfChartProvides +
              ' to monitor <a href="#menu_filesystem">File Systems</a>.<div id="ebpf_global_vfs_io"></div>'
    },

    'filesystem.vfs_io_bytes': {
        title : 'VFS bytes written',
        info: 'Total of bytes read or written with success using the <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS I/O functions</a>. This chart may not show all file system events if it uses other functions ' +
            'to store data on disk. Netdata shows virtual file system metrics per <a href="#ebpf_apps_vfs_write_bytes">application</a> and <a href="#ebpf_services_vfs_write_bytes">cgroup (systemd Services)</a> ' +
            'if <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">apps</a> or ' +
            '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">cgroup (systemd Services)</a> plugins are enabled.' + ebpfChartProvides +
            ' to monitor <a href="#menu_filesystem">File Systems</a>.<div id="ebpf_global_vfs_io_bytes"></div>'
    },

    'filesystem.vfs_io_error': {
        title : 'VFS IO error',
        info: 'Number of failed calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS I/O functions</a>. This chart may not show all file system events if it uses other functions ' +
            'to store data on disk. Netdata shows virtual file system metrics per <a href="#ebpf_apps_vfs_write_error">application</a> and <a href="#ebpf_services_vfs_write_error">cgroup (systemd Services)</a> ' +
            'if <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">apps</a> or ' +
            '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">cgroup (systemd Services)</a> plugins are enabled.' + ebpfChartProvides +
            ' to monitor <a href="#menu_filesystem">File Systems</a>.<div id="ebpf_global_vfs_io_error"></div>'
    },

    'filesystem.vfs_fsync': {
        info: 'Number of calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS syncer function</a>. This chart may not show all file system events if it uses other functions ' +
            'to sync data on disk. Netdata shows virtual file system metrics per <a href="#ebpf_apps_vfs_sync">application</a> and <a href="#ebpf_services_vfs_sync">cgroup (systemd Services)</a> ' +
            'if <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">apps</a> or ' +
            '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">cgroup (systemd Services)</a> plugins are enabled.' + ebpfChartProvides +
            ' to monitor <a href="#menu_filesystem">File Systems</a>.<div id="ebpf_global_vfs_sync"></div>'
    },

    'filesystem.vfs_fsync_error': {
        info: 'Number of failed calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS syncer function.</a>. This chart may not show all file system events if it uses other functions ' +
            'to sync data on disk. Netdata shows virtual file system metrics per <a href="#ebpf_apps_vfs_sync_error">application</a> and <a href="#ebpf_services_vfs_sync_error">cgroup (systemd Services)</a> ' +
            'if <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">apps</a> or ' +
            '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">cgroup (systemd Services)</a> plugins are enabled.' + ebpfChartProvides +
            ' to monitor <a href="#menu_filesystem">File Systems</a>.<div id="ebpf_global_vfs_sync_error"></div>'
    },

    'filesystem.vfs_open': {
        info: 'Number of calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS opener function</a>. This chart may not show all file system events if it uses other functions ' +
            'to open files. Netdata shows virtual file system metrics per <a href="#ebpf_apps_vfs_open">application</a> and <a href="#ebpf_services_vfs_open">cgroup (systemd Services)</a> ' +
            'if <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">apps</a> or ' +
            '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">cgroup (systemd Services)</a> plugins are enabled.' + ebpfChartProvides +
            ' to monitor <a href="#menu_filesystem">File Systems</a>.<div id="ebpf_global_vfs_open"></div>'
    },

    'filesystem.vfs_open_error': {
        info: 'Number of failed calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS opener function</a>. This chart may not show all file system events if it uses other functions ' +
            'to open files. Netdata shows virtual file system metrics per <a href="#ebpf_apps_vfs_open_error">application</a> and <a href="#ebpf_services_vfs_open_error">cgroup (systemd Services)</a> ' +
            'if <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">apps</a> or ' +
            '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">cgroup (systemd Services)</a> plugins are enabled.' + ebpfChartProvides +
            ' to monitor <a href="#menu_filesystem">File Systems</a>.<div id="ebpf_global_vfs_open_error"></div>'
    },

    'filesystem.vfs_create': {
        info: 'Number of calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS creator function</a>. This chart may not show all file system events if it uses other functions ' +
            'to create files. Netdata shows virtual file system metrics per <a href="#ebpf_apps_vfs_create">application</a> and <a href="#ebpf_services_vfs_create">cgroup (systemd Services)</a> ' +
            'if <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">apps</a> or ' +
            '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">cgroup (systemd Services)</a> plugins are enabled.' + ebpfChartProvides +
            ' to monitor <a href="#menu_filesystem">File Systems</a>.<div id="ebpf_global_vfs_create"></div>'
    },

    'filesystem.vfs_create_error': {
        info: 'Number of failed calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#vfs" target="_blank">VFS creator function</a>. This chart may not show all file system events if it uses other functions ' +
            'to create files. Netdata shows virtual file system metrics per <a href="#ebpf_apps_vfs_craete_error">application</a> and <a href="#ebpf_services_vfs_create_error">cgroup (systemd Services)</a> ' +
            'if <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">apps</a> or ' +
            '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">cgroup (systemd Services)</a> plugins are enabled.' + ebpfChartProvides +
            ' to monitor <a href="#menu_filesystem">File Systems</a>.<div id="ebpf_global_vfs_create_error"></div>'
    },

    'filesystem.ext4_read_latency': {
        info: '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#latency-algorithm" target="_blank">Latency</a> for each read request monitoring ext4 reader <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#ext4" target="_blank">function</a>.' + ebpfChartProvides + 'to monitor <a href="#menu_filesystem">File systems</a>.'
    },

    'filesystem.ext4_write_latency': {
        info: '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#latency-algorithm" target="_blank">Latency</a> for each write request monitoring ext4 writer <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#ext4" target="_blank">function</a>.' + ebpfChartProvides + ' to monitor <a href="#menu_filesystem">File systems</a>.'
    },

    'filesystem.ext4_open_latency': {
        info: '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#latency-algorithm" target="_blank">Latency</a> for each open request monitoring ext4 opener <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#ext4" target="_blank">function</a>.' + ebpfChartProvides + ' to monitor <a href="#menu_filesystem">File systems</a>.'
    },

    'filesystem.ext4_sync_latency': {
        info: '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#latency-algorithm" target="_blank">Latency</a> for each sync request monitoring ext4 syncer <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#ext4" target="_blank">function</a>.' + ebpfChartProvides + ' to monitor <a href="#menu_filesystem">File systems</a>.'
    },

    'filesystem.xfs_read_latency': {
        info: '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#latency-algorithm" target="_blank">Latency</a> for each read request monitoring xfs reader <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#xfs" target="_blank">function</a>.' + ebpfChartProvides + ' to monitor <a href="#menu_filesystem">File systems</a>.'
    },

    'filesystem.xfs_write_latency': {
        info: '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#latency-algorithm" target="_blank">Latency</a> for each write request monitoring xfs writer <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#xfs" target="_blank">function</a>.' + ebpfChartProvides + ' to monitor <a href="#menu_filesystem">File systems</a>.'
    },

    'filesystem.xfs_open_latency': {
        info: '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#latency-algorithm" target="_blank">Latency</a> for each open request monitoring xfs opener <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#xfs" target="_blank">function</a>.' + ebpfChartProvides + ' to monitor <a href="#menu_filesystem">File systems</a>.'
    },

    'filesystem.xfs_sync_latency': {
        info: '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#latency-algorithm" target="_blank">Latency</a> for each sync request monitoring xfs syncer <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#xfs" target="_blank">function</a>.' + ebpfChartProvides + ' to monitor <a href="#menu_filesystem">File systems</a>.'
    },

    'filesystem.nfs_read_latency': {
        info: '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#latency-algorithm" target="_blank">Latency</a> for each read request monitoring nfs reader <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#nfs" target="_blank">function</a>.' + ebpfChartProvides + ' to monitor <a href="#menu_filesystem">File systems</a>.'
    },

    'filesystem.nfs_write_latency': {
        info: '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#latency-algorithm" target="_blank">Latency</a> for each write request monitoring nfs writer <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#nfs" target="_blank">function</a>.' + ebpfChartProvides + ' to monitor <a href="#menu_filesystem">File systems</a>.'
    },

    'filesystem.nfs_open_latency': {
        info: '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#latency-algorithm" target="_blank">Latency</a> for each open request monitoring nfs opener <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#nfs" target="_blank">function</a>.' + ebpfChartProvides + ' to monitor <a href="#menu_filesystem">File systems</a>.'
    },

    'filesystem.nfs_attribute_latency': {
        info: '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#latency-algorithm" target="_blank">Latency</a> for each get attribute request monitoring nfs attribute <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#nfs" target="_blank">function</a>.' + ebpfChartProvides + ' to monitor <a href="#menu_filesystem">File systems</a>.'
    },

    'filesystem.zfs_read_latency': {
        info: '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#latency-algorithm" target="_blank">Latency</a> for each read request monitoring zfs reader <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#zfs" target="_blank">function</a>.' + ebpfChartProvides + ' to monitor <a href="#menu_filesystem">File systems</a>.'
    },

    'filesystem.zfs_write_latency': {
        info: '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#latency-algorithm" target="_blank">Latency</a> for each write request monitoring zfs writer <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#zfs" target="_blank">function</a>.' + ebpfChartProvides + ' to monitor <a href="#menu_filesystem">File systems</a>.'
    },

    'filesystem.zfs_open_latency': {
        info: '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#latency-algorithm" target="_blank">Latency</a> for each open request monitoring zfs opener <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#zfs" target="_blank">function</a>.' + ebpfChartProvides + ' to monitor <a href="#menu_filesystem">File systems</a>.'
    },

    'filesystem.zfs_sync_latency': {
        info: '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#latency-algorithm" target="_blank">Latency</a> for each sync request monitoring zfs syncer <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#zfs" target="_blank">function</a>.' + ebpfChartProvides + ' to monitor <a href="#menu_filesystem">File systems</a>.'
    },

    'filesystem.btrfs_read_latency': {
        info: '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#latency-algorithm" target="_blank">Latency</a> for each read request monitoring btrfs reader <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#btrfs" target="_blank">function</a>.' + ebpfChartProvides + ' to monitor <a href="#menu_filesystem">File systems</a>.'
    },

    'filesystem.btrfs_write_latency': {
        info: '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#latency-algorithm" target="_blank">Latency</a> for each write request monitoring btrfs writer <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#btrfs" target="_blank">function</a>.' + ebpfChartProvides + ' to monitor <a href="#menu_filesystem">File systems</a>.'
    },

    'filesystem.btrfs_open_latency': {
        info: '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#latency-algorithm" target="_blank">Latency</a> for each open request monitoring btrfs opener <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#btrfs" target="_blank">function</a>.' + ebpfChartProvides + ' to monitor <a href="#menu_filesystem">File systems</a>.'
    },

    'filesystem.btrfs_sync_latency': {
        info: '<a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#latency-algorithm" target="_blank">Latency</a> for each sync request monitoring btrfs syncer <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#btrfs" target="_blank">function</a>.' + ebpfChartProvides + ' to monitor <a href="#menu_filesystem">File systems</a>.'
    },

    'mount_points.call': {
        info: 'Monitor calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#mount-points" target="_blank">syscalls</a> that are responsible for attaching (<code>mount(2)</code>) or removing filesystems (<code>umount(2)</code>). This chart has relationship with <a href="#menu_filesystem">File systems</a>.' + ebpfChartProvides
    },

    'mount_points.error': {
        info: 'Monitor errors in calls to <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#mount-points" target="_blank">syscalls</a> that are responsible for attaching (<code>mount(2)</code>) or removing filesystems (<code>umount(2)</code>). This chart has relationship with <a href="#menu_filesystem">File systems</a>.' + ebpfChartProvides
    },

    'filesystem.file_descriptor': {
        info: 'Number of calls for internal functions on the Linux kernel responsible <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#file-descriptor" target="_blank">to open and closing files</a>. ' +
              'Netdata shows file access per <a href="#ebpf_apps_file_open">application</a> and <a href="#ebpf_services_file_open">cgroup (systemd Services)</a> if <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">apps</a> ' +
              'or <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">cgroup (systemd Services)</a> plugins are enabled.' + ebpfChartProvides + ' to monitor <a href="#menu_filesystem">File systems</a>'
    },

    'filesystem.file_error': {
        info: 'Number of failed calls to the kernel internal function responsible <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#file-descriptor" target="_blank">to open and closing files</a>. ' +
              'Netdata shows file error per <a href="#ebpf_apps_file_open_error">application</a> and <a href="#ebpf_services_file_open_error">cgroup (systemd Services)</a> if <a href="https://learn.netdata.cloud/guides/troubleshoot/monitor-debug-applications-ebpf" target="_blank">apps</a> ' +
              'or <a href="https://learn.netdata.cloud/docs/agent/collectors/ebpf.plugin#integration-with-cgroupsplugin" target="_blank">cgroup (systemd Services)</a> plugins are enabled.' + ebpfChartProvides + ' to monitor <a href="#menu_filesystem">File systems</a>.'
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

    'anomaly_detection.dimensions': {
        info: 'Total count of dimensions considered anomalous or normal. '
    },

    'anomaly_detection.anomaly_rate': {
        info: 'Percentage of anomalous dimensions. '
    },

    'anomaly_detection.detector_window': {
        info: 'The length of the active window used by the detector. '
    },

    'anomaly_detection.detector_events': {
        info: 'Flags (0 or 1) to show when an anomaly event has been triggered by the detector. '
    },

    'anomaly_detection.prediction_stats': {
        info: 'Diagnostic metrics relating to prediction time of anomaly detection. '
    },

    'anomaly_detection.training_stats': {
        info: 'Diagnostic metrics relating to training time of anomaly detection. '
    },

    // ------------------------------------------------------------------------
    // Supervisor

    'fail2ban.failed_attempts': {
        info: '<p>The number of failed attempts.</p>'+
        '<p>This chart reflects the number of \'Found\' lines. '+
        'Found means a line in the service’s log file matches the failregex in its filter.</p>'
    },

    'fail2ban.bans': {
        info: '<p>The number of bans.</p>'+
        '<p>This chart reflects the number of \'Ban\' and \'Restore Ban\' lines. '+
        'Ban action happens when the number of failed attempts (maxretry) occurred in the last configured interval (findtime).</p>'
    },

    'fail2ban.banned_ips': {
        info: '<p>The number of banned IP addresses.</p>'
    },

    // ------------------------------------------------------------------------
    // K8s state: Node.

    'k8s_state.node_allocatable_cpu_requests_utilization': {
        info: 'The percentage of allocated CPU resources used by Pod requests. '+
        'A Pod is scheduled to run on a Node only if the Node has enough CPU resources available to satisfy the Pod CPU request.'
    },
    'k8s_state.node_allocatable_cpu_requests_used': {
        info: 'The amount of allocated CPU resources used by Pod requests. ' +
        '1000 millicpu is equivalent to '+
        '<a href="https://kubernetes.io/docs/tasks/configure-pod-container/assign-cpu-resource/#cpu-units" target="_blank">1 physical or virtual CPU core</a>.'
    },
    'k8s_state.node_allocatable_cpu_limits_utilization': {
        info: 'The percentage of allocated CPU resources used by Pod limits. '+
        'Total limits may be over 100 percent (overcommitted).'
    },
    'k8s_state.node_allocatable_cpu_limits_used': {
        info: 'The amount of allocated CPU resources used by Pod limits. ' +
        '1000 millicpu is equivalent to '+
        '<a href="https://kubernetes.io/docs/tasks/configure-pod-container/assign-cpu-resource/#cpu-units" target="_blank">1 physical or virtual CPU core</a>.'
    },
    'k8s_state.node_allocatable_mem_requests_utilization': {
        info: 'The percentage of allocated memory resources used by Pod requests. '+
        'A Pod is scheduled to run on a Node only if the Node has enough memory resources available to satisfy the Pod memory request.'
    },
    'k8s_state.node_allocatable_mem_requests_used': {
        info: 'The amount of allocated memory resources used by Pod requests.'
    },
    'k8s_state.node_allocatable_mem_limits_utilization': {
        info: 'The percentage of allocated memory resources used by Pod limits. '+
        'Total limits may be over 100 percent (overcommitted).'
    },
    'k8s_state.node_allocatable_mem_limits_used': {
        info: 'The amount of allocated memory resources used by Pod limits.'
    },
    'k8s_state.node_allocatable_pods_utilization': {
        info: 'Pods limit utilization.'
    },
    'k8s_state.node_allocatable_pods_usage': {
        info: '<p>Pods limit usage.</p>'+
        '<p><b>Available</b> - the number of Pods available for scheduling. '+
        '<b>Allocated</b> - the number of Pods that have been scheduled.</p>'
    },
    'k8s_state.node_condition': {
        info: 'Health status. '+
        'If the status of the Ready condition remains False for longer than the <code>pod-eviction-timeout</code> (the default is 5 minutes), '+
        'then the node controller triggers API-initiated eviction for all Pods assigned to that node. '+
        '<a href="https://kubernetes.io/docs/concepts/architecture/nodes/#condition" target="_blank">More info.</a>'
    },
    'k8s_state.node_pods_readiness': {
        info: 'The percentage of Pods that are ready to serve requests.'
    },
    'k8s_state.node_pods_readiness_state': {
        info: '<p>Pods readiness state.</p>'+
        '<p><b>Ready</b> - the Pod has passed its readiness probe and ready to serve requests. '+
        '<b>Unready</b> - the Pod has not passed its readiness probe yet.</p>'
    },
    'k8s_state.node_pods_condition': {
        info: '<p>Pods state. '+
        '<a href="https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-conditions" target="_blank">More info.</a></p>'+
        '<b>PodReady</b> -  the Pod is able to serve requests and should be added to the load balancing pools of all matching Services. '+
        '<b>PodScheduled</b> - the Pod has been scheduled to a node. '+
        '<b>PodInitialized</b> - all init containers have completed successfully. '+
        '<b>ContainersReady</b> - all containers in the Pod are ready.</p>'
    },
    'k8s_state.node_pods_phase': {
        info: '<p>Pods phase. The phase of a Pod is a high-level summary of where the Pod is in its lifecycle. '+
        '<a href="https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-phase" target="_blank">More info.</a></p>'+
        '<p><b>Running</b> - the Pod has been bound to a node, and all of the containers have been created. '+
        'At least one container is still running, or is in the process of starting or restarting. ' +
        '<b>Failed</b> - all containers in the Pod have terminated, and at least one container has terminated in failure. '+
        'That is, the container either exited with non-zero status or was terminated by the system. ' +
        '<b>Succedeed</b> - all containers in the Pod have terminated in success, and will not be restarted. ' +
        '<b>Pending</b> - the Pod has been accepted by the Kubernetes cluster, but one or more of the containers has not been set up and made ready to run.</p>'
    },
    'k8s_state.node_containers': {
        info: 'The total number of containers and init containers.'
    },
    'k8s_state.node_containers_state': {
        info: '<p>The number of containers in different lifecycle states. '+
        '<a href="https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#container-states" target="_blank">More info.</a></p>'+
        '<p><b>Running</b> - a container is executing without issues. '+
        '<b>Waiting</b> - a container is still running the operations it requires in order to complete start up. '+
        '<b>Terminated</b> - a container began execution and then either ran to completion or failed for some reason.</p>'
    },
    'k8s_state.node_init_containers_state': {
        info: '<p>The number of init containers in different lifecycle states. '+
        '<a href="https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#container-states" target="_blank">More info.</a></p>'+
        '<p><b>Running</b> - a container is executing without issues. '+
        '<b>Waiting</b> - a container is still running the operations it requires in order to complete start up. '+
        '<b>Terminated</b> - a container began execution and then either ran to completion or failed for some reason.</p>'
    },
    'k8s_state.node_age': {
        info: 'The lifetime of the Node.'
    },

    // K8s state: Pod.

    'k8s_state.pod_cpu_requests_used': {
        info: 'The overall CPU resource requests for a Pod. '+
        'This is the sum of the CPU requests for all the Containers in the Pod. '+
        'Provided the system has CPU time free, a container is guaranteed to be allocated as much CPU as it requests. '+
        '1000 millicpu is equivalent to '+
        '<a href="https://kubernetes.io/docs/tasks/configure-pod-container/assign-cpu-resource/#cpu-units" target="_blank">1 physical or virtual CPU core</a>.'
    },
    'k8s_state.pod_cpu_limits_used': {
        info: 'The overall CPU resource limits for a Pod. '+
        'This is the sum of the CPU limits for all the Containers in the Pod. '+
        'If set, containers cannot use more CPU than the configured limit. '+
        '1000 millicpu is equivalent to '+
        '<a href="https://kubernetes.io/docs/tasks/configure-pod-container/assign-cpu-resource/#cpu-units" target="_blank">1 physical or virtual CPU core</a>.'
    },
    'k8s_state.pod_mem_requests_used': {
        info: 'The overall memory resource requests for a Pod. '+
        'This is the sum of the memory requests for all the Containers in the Pod.'
    },
    'k8s_state.pod_mem_limits_used': {
        info: 'The overall memory resource limits for a Pod. '+
        'This is the sum of the memory limits for all the Containers in the Pod. '+
        'If set, containers cannot use more RAM than the configured limit.'
    },
    'k8s_state.pod_condition': {
        info: 'The current state of the Pod. ' +
        '<a href="https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-conditions" target="_blank">More info.</a></p>'+
        '<p><b>PodReady</b> - the Pod is able to serve requests and should be added to the load balancing pools of all matching Services. ' +
        '<b>PodScheduled</b> - the Pod has been scheduled to a node. ' +
        '<b>PodInitialized</b> - all init containers have completed successfully. ' +
        '<b>ContainersReady</b> - all containers in the Pod are ready. '
    },
    'k8s_state.pod_phase': {
        info: 'High-level summary of where the Pod is in its lifecycle. ' +
        '<a href="https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-phase" target="_blank">More info.</a></p>'+
        '<p><b>Running</b> - the Pod has been bound to a node, and all of the containers have been created. '+
        'At least one container is still running, or is in the process of starting or restarting. ' +
        '<b>Failed</b> - all containers in the Pod have terminated, and at least one container has terminated in failure. '+
        'That is, the container either exited with non-zero status or was terminated by the system. ' +
        '<b>Succedeed</b> - all containers in the Pod have terminated in success, and will not be restarted. ' +
        '<b>Pending</b> - the Pod has been accepted by the Kubernetes cluster, but one or more of the containers has not been set up and made ready to run. '+
        'This includes time a Pod spends waiting to be scheduled as well as the time spent downloading container images over the network. '
    },
    'k8s_state.pod_age': {
        info: 'The <a href="https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-lifetime" target="_blank">lifetime</a> of the Pod. '
    },
    'k8s_state.pod_containers': {
        info: 'The number of containers and init containers belonging to the Pod.'
    },
    'k8s_state.pod_containers_state': {
        info: 'The state of each container inside this Pod. '+
        '<a href="https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#container-states" target="_blank">More info.</a> '+
        '<p><b>Running</b> - a container is executing without issues. '+
        '<b>Waiting</b> - a container is still running the operations it requires in order to complete start up. '+
        '<b>Terminated</b> - a container began execution and then either ran to completion or failed for some reason.</p>'
    },
    'k8s_state.pod_init_containers_state': {
        info: 'The state of each init container inside this Pod. '+
        '<a href="https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#container-states" target="_blank">More info.</a> '+
        '<p><b>Running</b> - a container is executing without issues. '+
        '<b>Waiting</b> - a container is still running the operations it requires in order to complete start up. '+
        '<b>Terminated</b> - a container began execution and then either ran to completion or failed for some reason.</p>'
    },

    // K8s state: Pod container.

    'k8s_state.pod_container_readiness_state': {
        info: 'Specifies whether the container has passed its readiness probe. '+
        'Kubelet uses readiness probes to know when a container is ready to start accepting traffic.'
    },
    'k8s_state.pod_container_restarts': {
        info: 'The number of times the container has been restarted.'
    },
    'k8s_state.pod_container_state': {
        info: 'Current state of the container. '+
        '<a href="https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#container-states" target="_blank">More info.</a> '+
        '<p><b>Running</b> - a container is executing without issues. '+
        '<b>Waiting</b> - a container is still running the operations it requires in order to complete start up. '+
        '<b>Terminated</b> - a container began execution and then either ran to completion or failed for some reason.</p>'
    },
    'k8s_state.pod_container_waiting_state_reason': {
        info: 'Reason the container is not yet running. '+
        '<a href="https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#container-state-waiting" target="_blank">More info.</a> '
    },
    'k8s_state.pod_container_terminated_state_reason': {
        info: 'Reason from the last termination of the container. '+
        '<a href="https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#container-state-terminated" target="_blank">More info.</a>'
    },

    // Ping

    'ping.host_rtt': {
        info: 'Round-trip time (RTT) is the time it takes for a data packet to reach its destination and return back to its original source.'
    },
    'ping.host_std_dev_rtt': {
        info: 'Round-trip time (RTT) standard deviation. The average value of how far each RTT of a ping differs from the average RTT.'
    },
    'ping.host_packet_loss': {
        info: 'Packet loss occurs when one or more transmitted data packets do not reach their destination. Usually caused by data transfer errors, network congestion or firewall blocking. ICMP echo packets are often treated as lower priority by routers and target hosts, so ping test packet loss may not always translate to application packet loss.'
    },
    'ping.host_packets': {
        info: 'Number of ICMP messages sent and received. These counters should be equal if there is no packet loss.'
    },

    // NVMe

    'nvme.device_estimated_endurance_perc': {
        info: 'NVM subsystem lifetime used based on the actual usage and the manufacturer\'s prediction of NVM life. A value of 100 indicates that the estimated endurance of the device has been consumed, but may not indicate a device failure. The value can be greater than 100 if you use the storage beyond its planned lifetime.'
    },
    'nvme.device_available_spare_perc': {
        info: 'Remaining spare capacity that is available. SSDs provide a set of internal spare capacity, called spare blocks, that can be used to replace blocks that have reached their write operation limit. After all of the spare blocks have been used, the next block that reaches its limit causes the disk to fail.'
    },
    'nvme.device_composite_temperature': {
        info: 'The current composite temperature of the controller and namespace(s) associated with that controller. The manner in which this value is computed is implementation specific and may not represent the actual temperature of any physical point in the NVM subsystem.'
    },
    'nvme.device_io_transferred_count': {
        info: 'The total amount of data read and written by the host.'
    },
    'nvme.device_power_cycles_count': {
        info: 'Power cycles reflect the number of times this host has been rebooted or the device has been woken up after sleep. A high number of power cycles does not affect the device\'s life expectancy.'
    },
    'nvme.device_power_on_time': {
        info: '<a href="https://en.wikipedia.org/wiki/Power-on_hours" target="_blank">Power-on time</a> is the length of time the device is supplied with power.'
    },
    'nvme.device_unsafe_shutdowns_count': {
        info: 'The number of times a power outage occurred without a shutdown notification being sent. Depending on the NVMe device you are using, an unsafe shutdown can corrupt user data.'
    },
    'nvme.device_critical_warnings_state': {
        info: '<p>Critical warnings for the status of the controller. Status active if set to 1.</p><p><b>AvailableSpare</b> - the available spare capacity is below the threshold. <b>TempThreshold</b> - the composite temperature is greater than or equal to an over temperature threshold or less than or equal to an under temperature threshold. <b>NvmSubsystemReliability</b> - the NVM subsystem reliability is degraded due to excessive media or internal errors. <b>ReadOnly</b> - media is placed in read-only mode. <b>VolatileMemBackupFailed</b> - the volatile memory backup device has failed. <b>PersistentMemoryReadOnly</b> - the Persistent Memory Region has become read-only or unreliable.</p>'
    },
    'nvme.device_media_errors_rate': {
        info: 'The number of occurrences where the controller detected an unrecovered data integrity error. Errors such as uncorrectable ECC, CRC checksum failure, or LBA tag mismatch are included in this counter.'
    },
    'nvme.device_error_log_entries_rate': {
        info: 'The number of entries in the Error Information Log. By itself, an increase in the number of records is not an indicator of any failure condition.'
    },
    'nvme.device_warning_composite_temperature_time': {
        info: 'The time the device has been operating above the Warning Composite Temperature Threshold (WCTEMP) and below Critical Composite Temperature Threshold (CCTEMP).'
    },
    'nvme.device_critical_composite_temperature_time': {
        info: 'The time the device has been operating above the Critical Composite Temperature Threshold (CCTEMP).'
    },
    'nvme.device_thermal_mgmt_temp1_transitions_rate': {
        info: 'The number of times the controller has entered lower active power states or performed vendor-specific thermal management actions, <b>minimizing performance impact</b>, to attempt to lower the Composite Temperature due to the host-managed thermal management feature.'
    },
    'nvme.device_thermal_mgmt_temp2_transitions_rate': {
        info: 'The number of times the controller has entered lower active power states or performed vendor-specific thermal management actions, <b>regardless of the impact on performance (e.g., heavy throttling)</b>, to attempt to lower the Combined Temperature due to the host-managed thermal management feature.'
    },
    'nvme.device_thermal_mgmt_temp1_time': {
        info: 'The amount of time the controller has entered lower active power states or performed vendor-specific thermal management actions, <b>minimizing performance impact</b>, to attempt to lower the Composite Temperature due to the host-managed thermal management feature.'
    },
    'nvme.device_thermal_mgmt_temp2_time': {
        info: 'The amount of time the controller has entered lower active power states or performed vendor-specific thermal management actions, <b>regardless of the impact on performance (e.g., heavy throttling)</b>, to attempt to lower the Combined Temperature due to the host-managed thermal management feature.'
    },
    // ------------------------------------------------------------------------

};
