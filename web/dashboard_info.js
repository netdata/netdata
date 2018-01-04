
var netdataDashboard = window.netdataDashboard || {};

// ----------------------------------------------------------------------------
// menus

// information about the main menus

netdataDashboard.menu = {
    'system': {
        title: 'System Overview',
        icon: '<i class="fas fa-bookmark"></i>',
        info: 'Overview of the key system metrics.'
    },

    'services': {
        title: 'systemd Services',
        icon: '<i class="fas fa-cogs"></i>',
        info: 'Resources utilization of systemd services. netdata monitors all systemd services via cgroups (the resources accounting used by containers). '
    },

    'ap': {
        title: 'Access Points',
        icon: '<i class="fas fa-wifi"></i>',
        info: 'Performance metrics for the access points (i.e. wireless interfaces in AP mode) found on the system.'
    },

    'tc': {
        title: 'Quality of Service',
        icon: '<i class="fas fa-globe"></i>',
        info: 'Netdata collects and visualizes <code>tc</code> class utilization using its <a href="https://github.com/firehol/netdata/blob/master/plugins.d/tc-qos-helper.sh" target="_blank">tc-helper plugin</a>. If you also use <a href="http://firehol.org/#fireqos" target="_blank">FireQOS</a> for setting up QoS, netdata automatically collects interface and class names. If your QoS configuration includes overheads calculation, the values shown here will include these overheads (the total bandwidth for the same interface as reported in the Network Interfaces section, will be lower than the total bandwidth reported here). QoS data collection may have a slight time difference compared to the interface (QoS data collection uses a BASH script, so a shift in data collection of a few milliseconds should be justified).'
    },

    'net': {
        title: 'Network Interfaces',
        icon: '<i class="fas fa-sitemap"></i>',
        info: 'Performance metrics for network interfaces.'
    },

    'ipv4': {
        title: 'IPv4 Networking',
        icon: '<i class="fas fa-cloud"></i>',
        info: 'Metrics for the IPv4 stack of the system. <a href="https://en.wikipedia.org/wiki/IPv4" target="_blank">Internet Protocol version 4 (IPv4)</a> is the fourth version of the Internet Protocol (IP). It is one of the core protocols of standards-based internetworking methods in the Internet. IPv4 is a connectionless protocol for use on packet-switched networks. It operates on a best effort delivery model, in that it does not guarantee delivery, nor does it assure proper sequencing or avoidance of duplicate delivery. These aspects, including data integrity, are addressed by an upper layer transport protocol, such as the Transmission Control Protocol (TCP).'
    },

    'ipv6': {
        title: 'IPv6 Networking',
        icon: '<i class="fas fa-cloud"></i>',
        info: 'Metrics for the IPv6 stack of the system. <a href="https://en.wikipedia.org/wiki/IPv6" target="_blank">Internet Protocol version 6 (IPv6)</a> is the most recent version of the Internet Protocol (IP), the communications protocol that provides an identification and location system for computers on networks and routes traffic across the Internet. IPv6 was developed by the Internet Engineering Task Force (IETF) to deal with the long-anticipated problem of IPv4 address exhaustion. IPv6 is intended to replace IPv4.'
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
        info: 'Performance metrics of the ZFS filesystem. The following charts visualize all metrics reported by <a href="https://github.com/zfsonlinux/zfs/blob/master/cmd/arcstat/arcstat.py" target="_blank">arcstat.py</a> and <a href="https://github.com/zfsonlinux/zfs/blob/master/cmd/arc_summary/arc_summary.py" target="_blank">arc_summary.py</a>.'
    },

    'btrfs': {
        title: 'BTRFS filesystem',
        icon: '<i class="fas fa-folder-open"></i>',
        info: 'Disk space metrics for the BTRFS filesystem.'
    },

    'apps': {
        title: 'Applications',
        icon: '<i class="fas fa-heartbeat"></i>',
        info: 'Per application statistics are collected using netdata\'s <code>apps.plugin</code>. This plugin walks through all processes and aggregates statistics for applications of interest, defined in <code>/etc/netdata/apps_groups.conf</code> (the default is <a href="https://github.com/firehol/netdata/blob/master/conf.d/apps_groups.conf" target="_blank">here</a>). The plugin internally builds a process tree (much like <code>ps fax</code> does), and groups processes together (evaluating both child and parent processes) so that the result is always a chart with a predefined set of dimensions (of course, only application groups found running are reported). The reported values are compatible with <code>top</code>, although the netdata plugin counts also the resources of exited children (unlike <code>top</code> which shows only the resources of the currently running processes). So for processes like shell scripts, the reported values include the resources used by the commands these scripts run within each timeframe.',
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

    'memcached': {
        title: 'memcached',
        icon: '<i class="fas fa-database"></i>',
        info: 'Performance metrics for <b>memcached</b>. Memcached is a general-purpose distributed memory caching system. It is often used to speed up dynamic database-driven websites by caching data and objects in RAM to reduce the number of times an external data source (such as a database or API) must be read.'
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

    'retroshare': {
        title: 'RetroShare',
        icon: '<i class="fas fa-share-alt"></i>',
        info: 'Performance metrics for <b>RetroShare</b>. RetroShare is open source software for encrypted filesharing, serverless email, instant messaging, online chat, and BBS, based on a friend-to-friend network built on GNU Privacy Guard (GPG).'
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
        info: 'Information extracted from a server log file. <code>web_log</code> plugin incrementally parses the server log file to provide, in real-time, a break down of key server performance metrics. For web servers, an extended log file format may optionally be used (for <code>nginx</code> and <code>apache</code>) offering timing information and bandwidth for both requests and responses. <code>web_log</code> plugin may also be configured to provide a break down of requests per URL pattern (check <a href="https://github.com/firehol/netdata/blob/master/conf.d/python.d/web_log.conf" target="_blank"><code>/etc/netdata/python.d/web_log.conf</code></a>).'
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
        info: 'Number of requests for each <code>URL pattern</code> defined in <a href="https://github.com/firehol/netdata/blob/master/conf.d/python.d/web_log.conf" target="_blank"><code>/etc/netdata/python.d/web_log.conf</code></a>. This chart counts all requests matching the URL patterns defined, independently of the web server response codes (i.e. both successful and unsuccessful).'
    },

    'web_log.clients': {
        info: 'Charts showing the number of unique client IPs, accessing the web server.'
    },

    'web_log.timings': {
        info: 'Web server response timings - the time the web server needed to prepare and respond to requests. This requires an extended log format and its meaning is web server specific. For most web servers this accounts the time from the reception of a complete request, to the dispatch of the last byte of the response. So, it includes the network delays of responses, but it does not include the network delays of requests.'
    },

    'mem.ksm': {
        title: 'Memory Deduper',
        info: 'Kernel Same-page Merging (KSM) performance monitoring, read from several files in <code>/sys/kernel/mm/ksm/</code>. KSM is a memory-saving de-duplication feature in the Linux kernel (since version 2.6.32). The KSM daemon ksmd periodically scans those areas of user memory which have been registered with it, looking for pages of identical content which can be replaced by a single write-protected page (which is automatically copied if a process later wants to update its content). KSM was originally developed for use with KVM (where it was known as Kernel Shared Memory), to fit more virtual machines into physical memory, by sharing the data common between them.  But it can be useful to any application which generates many instances of the same data.'
    },

    'mem.numa': {
        info: 'Non-Uniform Memory Access (NUMA) is a hierarchical memory design the memory access time is dependent on locality. Under NUMA, a processor can access its own local memory faster than non-local memory (memory local to another processor or memory shared between processors). The individual metrics are described in the <a href="https://www.kernel.org/doc/Documentation/numastat.txt" target="_blank">Linux kernel documentation</a>.'
    },

    'ipv4.ecn': {
        info: '<a href="https://en.wikipedia.org/wiki/Explicit_Congestion_Notification" target="_blank">Explicit Congestion Notification (ECN)</a> is a TCP extension that allows end-to-end notification of network congestion without dropping packets. ECN is an optional feature that may be used between two ECN-enabled endpoints when the underlying network infrastructure also supports it.'
    },

    'netfilter.conntrack': {
        title: 'Connection Tracker',
        info: 'Netfilter Connection Tracker performance metrics. The connection tracker keeps track of all connections of the machine, inbound and outbound. It works by keeping a database with all open connections, tracking network and address translation and connection expectations.'
    },

    'netfilter.nfacct': {
        title: 'Bandwidth Accounting',
        info: 'The following information is read using the <code>nfacct.plugin</code>.'
    },

    'netfilter.synproxy': {
        title: 'DDoS Protection',
        info: 'DDoS protection performance metrics. <a href="https://github.com/firehol/firehol/wiki/Working-with-SYNPROXY" target="_blank">SYNPROXY</a> is a TCP SYN packets proxy. It is used to protect any TCP server (like a web server) from SYN floods and similar DDoS attacks. It is a netfilter module, in the Linux kernel (since version 3.12). It is optimized to handle millions of packets per second utilizing all CPUs available without any concurrency locking between the connections. It can be used for any kind of TCP traffic (even encrypted), since it does not interfere with the content itself.'
    },

    'ipfw.dynamic_rules': {
        title: 'dynamic rules',
        info: 'Number of dynamic rules, created by correspondent stateful firewall rules.'
    },

    'system.softnet_stat': {
        title: 'softnet',
        info: function(os) {
            if(os === 'linux')
                return 'Statistics for CPUs SoftIRQs related to network receive work. Break down per CPU core can be found at <a href="#menu_cpu_submenu_softnet_stat">CPU / softnet statistics</a>. <b>processed</b> states the number of packets processed, <b>dropped</b> is the number packets dropped because the network device backlog was full (to fix them on Linux use <code>sysctl</code> to increase <code>net.core.netdev_max_backlog</code>), <b>squeezed</b> is the number of packets dropped because the network device budget ran out (to fix them on Linux use <code>sysctl</code> to increase <code>net.core.netdev_budget</code>). More information about identifying and troubleshooting network driver related issues can be found at <a href="https://access.redhat.com/sites/default/files/attachments/20150325_network_performance_tuning.pdf" target="_blank">Red Hat Enterprise Linux Network Performance Tuning Guide</a>.';
            else
                return 'Statistics for CPUs SoftIRQs related to network receive work.';
        }
    },

    'cpu.softnet_stat': {
        title: 'softnet',
        info: function(os) {
            if(os === 'linux')
                return 'Statistics for per CPUs core SoftIRQs related to network receive work. Total for all CPU cores can be found at <a href="#menu_system_submenu_softnet_stat">System / softnet statistics</a>. <b>processed</b> states the number of packets processed, <b>dropped</b> is the number packets dropped because the network device backlog was full (to fix them on Linux use <code>sysctl</code> to increase <code>net.core.netdev_max_backlog</code>), <b>squeezed</b> is the number of packets dropped because the network device budget ran out (to fix them on Linux use <code>sysctl</code> to increase <code>net.core.netdev_budget</code>). More information about identifying and troubleshooting network driver related issues can be found at <a href="https://access.redhat.com/sites/default/files/attachments/20150325_network_performance_tuning.pdf" target="_blank">Red Hat Enterprise Linux Network Performance Tuning Guide</a>.';
            else
                return 'Statistics for per CPUs core SoftIRQs related to network receive work. Total for all CPU cores can be found at <a href="#menu_system_submenu_softnet_stat">System / softnet statistics</a>.';
        }
    },

    'go_expvar.memstats': {
        title: 'Memory statistics',
        info: 'Go runtime memory statistics. See <a href="https://golang.org/pkg/runtime/#MemStats" target="_blank">runtime.MemStats</a> documentation for more info about each chart and the values.'
    },

    'couchdb.dbactivity': {
        title: 'DB activity',
        info: 'Overall database reads and writes for the entire server. This includes any external HTTP traffic, as well as internal replication traffic performed in a cluster to ensure node consistency.'
    },

    'couchdb.httptraffic': {
        title: 'HTTP traffic breakdown',
        info: 'All HTTP traffic, broken down by type of request (<tt>GET</tt>, <tt>PUT</tt>, <tt>POST</tt>, etc.) and response status code (<tt>200</tt>, <tt>201</tt>, <tt>4xx</tt>, etc.)<br/><br/>Any <tt>5xx</tt> errors here indicate a likely CouchDB bug; check the logfile for further information.'
    },

    'couchdb.ops': {
        title: 'Server operations'
    },

    'couchdb.perdbstats': {
        title: 'Per-DB statistics',
        info: 'Statistics per database. This includes <a href="http://docs.couchdb.org/en/latest/api/database/common.html#get--db">3 size graphs per database</a>: active (the size of live data in the database), external (the uncompressed size of the database contents), and file (the size of the file on disk, exclusive of any views and indexes). It also includes the number of documents and number of deleted documents per database.'
    },

    'couchdb.erlang': {
        title: 'Erlang statistics',
        info: 'Detailed information about the status of the Erlang VM that hosts CouchDB. These are intended for advanced users only. High values of the peak message queue (>10e6) generally indicate an overload condition.'
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
netdataDashboard.context = {
    'system.cpu': {
        info: function(os) {
            void(os);
            return 'Total CPU utilization (all cores). 100% here means there is no CPU idle time at all. You can get per core usage at the <a href="#menu_cpu">CPUs</a> section and per application usage at the <a href="#menu_apps">Applications Monitoring</a> section.'
                + netdataDashboard.sparkline('<br/>Keep an eye on <b>iowait</b> ', 'system.cpu', 'iowait', '%', '. If it is constantly high, your disks are a bottleneck and they slow your system down.')
                + netdataDashboard.sparkline('<br/>An important metric worth monitoring, is <b>softirq</b> ', 'system.cpu', 'softirq', '%', '. A constantly high percentage of softirq may indicate network driver issues.');
        },
        valueRange: "[0, 100]"
    },

    'system.load': {
        info: 'Current system load, i.e. the number of processes using CPU or waiting for system resources (usually CPU and disk). The 3 metrics refer to 1, 5 and 15 minute averages. The system calculates this once every 5 seconds. For more information check <a href="https://en.wikipedia.org/wiki/Load_(computing)" target="_blank">this wikipedia article</a>',
        height: 0.7
    },

    'system.io': {
        info: function(os) {
            var s = 'Total Disk I/O, for all physical disks. You can get detailed information about each disk at the <a href="#menu_disk">Disks</a> section and per application Disk usage at the <a href="#menu_apps">Applications Monitoring</a> section.';

            if(os === 'linux')
                return s + ' Physical are all the disks that are listed in <code>/sys/block</code>, but do not exist in <code>/sys/devices/virtual/block</code>.';
            else
                return s;
        }
    },

    'system.pgpgio': {
        info: 'Memory paged from/to disk. This is usually the total disk I/O of the system.'
    },

    'system.swapio': {
        info: 'Total Swap I/O. (netdata measures both <code>in</code> and <code>out</code>. If either of them is not shown in the chart, it is because it is zero - you can change the page settings to always render all the available dimensions on all charts).'
    },

    'system.pgfaults': {
        info: 'Total page faults. <b>Major page faults</b> indicates that the system is using its swap. You can find which applications use the swap at the <a href="#menu_apps">Applications Monitoring</a> section.'
    },

    'system.entropy': {
        colors: '#CC22AA',
        info: '<a href="https://en.wikipedia.org/wiki/Entropy_(computing)" target="_blank">Entropy</a>, is a pool of random numbers (<a href="https://en.wikipedia.org/wiki//dev/random" target="_blank">/dev/random</a>) that is mainly used in cryptography. If the pool of entropy gets empty, processes requiring random numbers may run a lot slower (it depends on the interface each program uses), waiting for the pool to be replenished. Ideally a system with high entropy demands should have a hardware device for that purpose (TPM is one such device). There are also several software-only options you may install, like <code>haveged</code>, although these are generally useful only in servers.'
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
        info: function(os) {
            var s = 'Total bandwidth of all physical network interfaces. This does not include <code>lo</code>, VPNs, network bridges, IFB devices, bond interfaces, etc. Only the bandwidth of physical network interfaces is aggregated.';

            if(os === 'linux')
                return s + ' Physical are all the network interfaces that are listed in <code>/proc/net/dev</code>, but do not exist in <code>/sys/devices/virtual/net</code>.';
            else
                return s;
        }
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
            function(os, id) {
                void(os);
                return  '<div data-netdata="' + id + '"'
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

    'mem.pgfaults': {
    	info: 'A <a href="https://en.wikipedia.org/wiki/Page_fault" target="_blank">page fault</a> is a type of interrupt, called trap, raised by computer hardware when a running program accesses a memory page that is mapped into the virtual address space, but not actually loaded into main memory. If the page is loaded in memory at the time the fault is generated, but is not marked in the memory management unit as being loaded in memory, then it is called a <b>minor</b> or soft page fault. A <b>major</b> page fault is generated when the system needs to load the memory page from disk or swap memory.'
    },

    'mem.committed': {
        colors: NETDATA.colors[3],
        info: 'Committed Memory, is the sum of all memory which has been allocated by processes.'
    },

    'mem.writeback': {
        info: '<b>Dirty</b> is the amount of memory waiting to be written to disk. <b>Writeback</b> is how much memory is actively being written to disk.'
    },

    'mem.kernel': {
        info: 'The total amount of memory being used by the kernel. <b>Slab</b> is the amount of memory used by the kernel to cache data structures for its own use. <b>KernelStack</b> is the amount of memory allocated for each task done by the kernel. <b>PageTables</b> is the amount of memory decicated to the lowest level of page tables (A page table is used to turn a virtual address into a physical memory address). <b>VmallocUsed</b> is the amount of memory being used as virtual address space.'
    },

    'mem.slab': {
        info: '<b>Reclaimable</b> is the amount of memory which the kernel can reuse. <b>Unreclaimable</b> can not be reused even when the kernel is lacking memory.'
    },

    // ------------------------------------------------------------------------
    // network interfaces

    'net.drops': {
        info: 'Packets that have been dropped at the network interface level. These are the same counters reported by <code>ifconfig</code> as <code>RX dropped</code> (inbound) and <code>TX dropped</code> (outbound). <b>inbound</b> packets can be dropped at the network interface level due to <a href="#menu_system_submenu_softnet_stat">softnet backlog</a> overflow, bad / unintented VLAN tags, unknown or unregistered protocols, IPv6 frames when the server is not configured for IPv6. Check <a href="https://www.novell.com/support/kb/doc.php?id=7007165" target="_blank">this document</a> for more information.'
    },

    // ------------------------------------------------------------------------
    // IPv4

    'ipv4.tcpmemorypressures': {
        info: 'Number of times a socket was put in <b>memory pressure</b> due to a non fatal memory allocation failure (the kernel attempts to work around this situation by reducing the send buffers, etc).'
    },

    'ipv4.tcpconnaborts': {
        info: 'TCP connection aborts. <b>baddata</b> (<code>TCPAbortOnData</code>) happens while the connection is on <code>FIN_WAIT1</code> and the kernel receives a packet with a sequence number beyond the last one for this connection - the kernel responds with <code>RST</code> (closes the connection). <b>userclosed</b> (<code>TCPAbortOnClose</code>) happens when the kernel receives data on an already closed connection and responds with <code>RST</code>. <b>nomemory</b> (<code>TCPAbortOnMemory</code> happens when there are too many orphaned sockets (not attached to an fd) and the kernel has to drop a connection - sometimes it will send an <code>RST</code>, sometimes it won\'t. <b>timeout</b> (<code>TCPAbortOnTimeout</code>) happens when a connection times out. <b>linger</b> (<code>TCPAbortOnLinger</code>) happens when the kernel killed a socket that was already closed by the application and lingered around for long enough. <b>failed</b> (<code>TCPAbortFailed</code>) happens when the kernel attempted to send an <code>RST</code> but failed because there was no memory available.'
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
        info: 'Virtual memory allocated by applications. Please check <a href="https://github.com/firehol/netdata/wiki/netdata-virtual-memory-size" target="_blank">this article</a> for more information.'
    },

    'apps.preads': {
        height: 2.0
    },

    'apps.pwrites': {
        height: 2.0
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
        info: 'Virtual memory allocated per user. Please check <a href="https://github.com/firehol/netdata/wiki/netdata-virtual-memory-size" target="_blank">this article</a> for more information.'
    },

    'users.preads': {
        height: 2.0
    },

    'users.pwrites': {
        height: 2.0
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
        info: 'Virtual memory allocated per user group. Please check <a href="https://github.com/firehol/netdata/wiki/netdata-virtual-memory-size" target="_blank">this article</a> for more information.'
    },

    'groups.preads': {
        height: 2.0
    },

    'groups.pwrites': {
        height: 2.0
    },

    // ------------------------------------------------------------------------
    // NETWORK QoS

    'tc.qos': {
        heads: [
            function(os, id) {
                void(os);

                if(id.match(/.*-ifb$/))
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
            function(os, id) {
                void(os);
                if(id.match(/^cgroup_.*/)) {
                    var iface;
                    try {
                        iface = ' ' + id.substring(id.lastIndexOf('.net_') + 5, id.length);
                    }
                    catch (e) {
                        iface = '';
                    }
                    return netdataDashboard.gaugeChart('Received' + iface, '12%', 'received');
                }
                else
                    return '';
            },
            function(os, id) {
                void(os);
                if(id.match(/^cgroup_.*/)) {
                    var iface;
                    try {
                        iface = ' ' + id.substring(id.lastIndexOf('.net_') + 5, id.length);
                    }
                    catch (e) {
                        iface = '';
                    }
                    return netdataDashboard.gaugeChart('Sent' + iface, '12%', 'sent');
                }
                else
                    return '';
            }
        ],
        heads: [
            function(os, id) {
                void(os);
                if(!id.match(/^cgroup_.*/))
                    return netdataDashboard.gaugeChart('Received', '12%', 'received');
                else
                    return '';
            },
            function(os, id) {
                void(os);
                if(!id.match(/^cgroup_.*/))
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
    'disk.mops': {
        height: 0.5,
        info: 'The number of merged disk operations. The system is able to merge adjacent I/O operations, for example two 4KB reads can become one 8KB read before given to disk.'
    },
    'disk.svctm': {
        height: 0.5,
        info: 'The average service time for completed I/O operations. This metric is calculated using the total busy time of the disk and the number of completed operations. If the disk is able to execute multiple parallel operations the reporting average service time will be misleading.'
    },
    'disk.avgsz': {
        height: 0.5,
        info: 'The average I/O operation size.'
    },
    'disk.await': {
        height: 0.5,
        info: 'The average time for I/O requests issued to the device to be served. This includes the time spent by the requests in queue and the time spent servicing them.'
    },

    'disk.space': {
        info: 'Disk space utilization. reserved for root is automatically reserved by the system to prevent the root user from getting out of space.'
    },
    'disk.inodes': {
        info: 'inodes (or index nodes) are filesystem objects (e.g. files and directories). On many types of file system implementations, the maximum number of inodes is fixed at filesystem creation, limiting the maximum number of files the filesystem can hold. It is possible for a device to run out of inodes. When this happens, new files cannot be created on the device, even though there may be free space available.'
    },

    'mysql.net': {
        info: 'The amount of data sent to mysql clients (<strong>out</strong>) and received from mysql clients (<strong>in</strong>).'
    },

    // ------------------------------------------------------------------------
    // MYSQL

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
            function(os, id) {
                void(os);
                return  '<div data-netdata="' + id + '"'
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
            function(os, id) {
                void(os);
                return  '<div data-netdata="' + id + '"'
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
            function(os, id) {
                void(os);
                return  '<div data-netdata="' + id + '"'
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

    'cgroup.cpu': {
        mainheads: [
            function(os, id) {
                void(os);
                return  '<div data-netdata="' + id + '"'
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

    'cgroup.mem_usage': {
        mainheads: [
            function(os, id) {
                void(os);
                return  '<div data-netdata="' + id + '"'
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
            }
        ]
    },

    'cgroup.throttle_io': {
        mainheads: [
            function(os, id) {
                void(os);
                return  '<div data-netdata="' + id + '"'
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
            function(os, id) {
                void(os);
                return  '<div data-netdata="' + id + '"'
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
        info: 'Tthe rate of connections opened to beanstalkd.'
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
    // web_log

    'web_log.response_statuses': {
        info: 'Web server responses by type. <code>success</code> includes <b>1xx</b>, <b>2xx</b> and <b>304</b>, <code>error</code> includes <b>5xx</b>, <code>redirect</code> includes <b>3xx</b> except <b>304</b>, <code>bad</code> includes <b>4xx</b>, <code>other</code> are all the other responses.',
        mainheads: [
            function(os, id) {
                void(os);
                return  '<div data-netdata="' + id + '"'
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

            function(os, id) {
                void(os);
                return  '<div data-netdata="' + id + '"'
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

            function(os, id) {
                void(os);
                return  '<div data-netdata="' + id + '"'
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

            function(os, id) {
                void(os);
                return  '<div data-netdata="' + id + '"'
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
        '<code>unmatched</code> counts the lines in the log file that are not matched by the plugin (<a href="https://github.com/firehol/netdata/issues/new?title=web_log%20reports%20unmatched%20lines&body=web_log%20plugin%20reports%20unmatched%20lines.%0A%0AThis%20is%20my%20log:%0A%0A%60%60%60txt%0A%0Aplease%20paste%20your%20web%20server%20log%20here%0A%0A%60%60%60" target="_blank">let us know</a> if you have any unmatched).'
    },

    'web_log.response_time': {
        mainheads: [
            function(os, id) {
                void(os);
                return  '<div data-netdata="' + id + '"'
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
        info: 'Unique client IPs accessing the web server since the last restart of netdata. This plugin keeps in memory all the unique IPs that have accessed the web server. On very busy web servers (several millions of unique IPs) you may want to disable this chart (check <a href="https://github.com/firehol/netdata/blob/master/conf.d/python.d/web_log.conf" target="_blank"><code>/etc/netdata/python.d/web_log.conf</code></a>).'
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
            function(os, id) {
                void(os);
                return  '<div data-netdata="' + id + '"'
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

            function(os, id) {
                void(os);
                return  '<div data-netdata="' + id + '"'
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

            function(os, id) {
                void(os);
                return  '<div data-netdata="' + id + '"'
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

            function(os, id) {
                void(os);
                return  '<div data-netdata="' + id + '"'
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
        '<code>unmatched</code> counts the lines in the log file that are not matched by the plugin (<a href="https://github.com/firehol/netdata/issues/new?title=web_log%20reports%20unmatched%20lines&body=web_log%20plugin%20reports%20unmatched%20lines.%0A%0AThis%20is%20my%20log:%0A%0A%60%60%60txt%0A%0Aplease%20paste%20your%20web%20server%20log%20here%0A%0A%60%60%60" target="_blank">let us know</a> if you have any unmatched).'
    },

    'web_log.squid_duration': {
        mainheads: [
            function(os, id) {
                void(os);
                return  '<div data-netdata="' + id + '"'
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
        info: 'Unique client IPs accessing squid since the last restart of netdata. This plugin keeps in memory all the unique IPs that have accessed the server. On very busy squid servers (several millions of unique IPs) you may want to disable this chart (check <a href="https://github.com/firehol/netdata/blob/master/conf.d/python.d/web_log.conf" target="_blank"><code>/etc/netdata/python.d/web_log.conf</code></a>).'
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
        info: 'These tags are optional and describe some error conditions which occured during response delivery (if any). ' +
        '<code>ABORTED</code> when the response was not completed due to the connection being aborted (usually by the client). ' +
        '<code>TIMEOUT</code>, when the response was not completed due to a connection timeout.'
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
        info: 'This shows the <code>residual frequency</code> for the currently selected reference source. It reflects any difference between what the measurements from the reference source indicate the frequency should be and the frequency currently being used.' +
        'The reason this is not always zero is that a smoothing procedure is applied to the frequency. Each time a measurement from the reference source is obtained and a new residual frequency computed, the estimated accuracy of this residual is compared with the estimated accuracy (see <code>skew</code>) of the existing frequency value. A weighted average is computed for the new frequency, with weights depending on these accuracies. If the measurements from the reference source follow a consistent trend, the residual will be driven to zero over time.',
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
        info: 'Physical disk usage of BTRFS. The disk space reported here is the raw physical disk space assigned to the BTRFS volume (i.e. <b>before any RAID levels</b>). BTRFS uses a two-stage allocator, first allocating large regions of disk space for one type of block (data, metadata, or system), and then using a regular block allocator inside those regions. <code>unallocated</code> is the physical disk space that is not allocated yet and is available to become data, metdata or system on demand. When <code>unallocated</code> is zero, all available disk space has been allocated to a specific function. Healthy volumes should ideally have at least five percent of their total space <code>unallocated</code>. You can keep your volume healthy by running the <code>btrfs balance</code> command on it regularly (check <code>man btrfs-balance</code> for more info).'
    },

    'btrfs.data': {
        info: 'Logical disk usage for BTRFS data. Data chunks are used to store the actual file data (file contents). The disk space reported here is the usable allocation (i.e. after any striping or replication). Healthy volumes should ideally have no more than a few GB of free space reported here persistently. Running <code>btrfs balance</code> can help here.'
    },

    'btrfs.metadata': {
        info: 'Logical disk usage for BTRFS metadata. Metadata chunks store most of the filesystem interal structures, as well as information like directory structure and file names. The disk space reported here is the usable allocation (i.e. after any striping or replication). Healthy volumes should ideally have no more than a few GB of free space reported here persistently. Running <code>btrfs balance</code> can help here.'
    },

    'btrfs.system': {
        info: 'Logical disk usage for BTRFS system. System chunks store information aobut the allocation of other chunks. The disk space reported here is the usable allocation (i.e. after any striping or replication). The values reported here should be relatively small compared to Data and Metadata, and will scale with the volume size and overall space usage.'
    }
};
