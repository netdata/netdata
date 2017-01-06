
var netdataDashboard = window.netdataDashboard || {};

// menu
// information about the main menus

netdataDashboard.menu = {
    'system': {
        title: 'System Overview',
        icon: '<i class="fa fa-bookmark" aria-hidden="true"></i>',
        info: 'Overview of the key system metrics.'
    },

    'ap': {
        title: 'Access Points',
        icon: '<i class="fa fa-wifi" aria-hidden="true"></i>',
        info: undefined
    },

    'tc': {
        title: 'Quality of Service',
        icon: '<i class="fa fa-globe" aria-hidden="true"></i>',
        info: 'Netdata collects and visualizes tc class utilization using its <a href="https://github.com/firehol/netdata/blob/master/plugins.d/tc-qos-helper.sh" target="_blank">tc-helper plugin</a>. If you also use <a href="http://firehol.org/#fireqos" target="_blank">FireQOS</a> for setting up QoS, netdata automatically collects interface and class names. If your QoS configuration includes overheads calculation, the values shown here will include these overheads (the total bandwidth for the same interface as reported in the Network Interfaces section, will be lower than the total bandwidth reported here). Also, data collection may have a slight time difference compared to the interface (QoS data collection is implemented with a BASH script, so a shift in data collection of a few milliseconds should be justified).'
    },

    'net': {
        title: 'Network Interfaces',
        icon: '<i class="fa fa-share-alt" aria-hidden="true"></i>',
        info: 'Per network interface statistics collected from <code>/proc/net/dev</code>.'
    },

    'ipv4': {
        title: 'IPv4 Networking',
        icon: '<i class="fa fa-cloud" aria-hidden="true"></i>',
        info: undefined
    },

    'ipv6': {
        title: 'IPv6 Networking',
        icon: '<i class="fa fa-cloud" aria-hidden="true"></i>',
        info: undefined
    },

    'ipvs': {
        title: 'IP Virtual Server',
        icon: '<i class="fa fa-eye" aria-hidden="true"></i>',
        info: undefined
    },

    'netfilter': {
        title: 'Firewall (netfilter)',
        icon: '<i class="fa fa-shield" aria-hidden="true"></i>',
        info: undefined
    },

    'cpu': {
        title: 'CPUs',
        icon: '<i class="fa fa-bolt" aria-hidden="true"></i>',
        info: undefined
    },

    'mem': {
        title: 'Memory',
        icon: '<i class="fa fa-bolt" aria-hidden="true"></i>',
        info: undefined
    },

    'disk': {
        title: 'Disks',
        icon: '<i class="fa fa-folder" aria-hidden="true"></i>',
        info: 'Charts with performance information for all the system disks. Special care has been given to present disk performance metrics in a way compatible with <code>iostat -x</code>. netdata by default prevents rendering performance charts for individual partitions and unmounted virtual disks. Disabled charts can still be enabled by altering the relative settings in the netdata configuration file.'
    },

    'sensors': {
        title: 'Sensors',
        icon: '<i class="fa fa-leaf" aria-hidden="true"></i>',
        info: undefined
    },

    'nfsd': {
        title: 'NFS Server',
        icon: '<i class="fa fa-folder-open" aria-hidden="true"></i>',
        info: undefined
    },

    'nfs': {
        title: 'NFS Client',
        icon: '<i class="fa fa-folder-open" aria-hidden="true"></i>',
        info: undefined
    },

    'apps': {
        title: 'Applications',
        icon: '<i class="fa fa-heartbeat" aria-hidden="true"></i>',
        info: 'Per application statistics are collected using netdata\'s <code>apps.plugin</code>. This plugin walks through the entire <code>/proc</code> filesystem and aggregates statistics for applications of interest, defined in <code>/etc/netdata/apps_groups.conf</code> (the default is <a href="https://github.com/firehol/netdata/blob/master/conf.d/apps_groups.conf" target="_blank">here</a>). The plugin internally builds a process tree (much like <code>ps fax</code> does), and groups processes together (evaluating both child and parent processes) so that the result is always a chart with a predefined set of dimensions (of course, only application groups found running are reported). The reported values are compatible with <code>top</code>, although the netdata plugin counts also the resources of exited children (unlike <code>top</code> which shows only the resources of the currently running processes). So for processes like shell scripts, the reported values include the resources used by the commands these scripts run within each timeframe.',
        height: 1.5
    },

    'users': {
        title: 'Users',
        icon: '<i class="fa fa-user" aria-hidden="true"></i>',
        info: 'Per user statistics are collected using netdata\'s <code>apps.plugin</code>. This plugin walks through the entire <code>/proc</code> filesystem and aggregates statistics per user. The reported values are compatible with <code>top</code>, although the netdata plugin counts also the resources of exited children (unlike <code>top</code> which shows only the resources of the currently running processes). So for processes like shell scripts, the reported values include the resources used by the commands these scripts run within each timeframe.',
        height: 1.5
    },

    'groups': {
        title: 'User Groups',
        icon: '<i class="fa fa-users" aria-hidden="true"></i>',
        info: 'Per user group statistics are collected using netdata\'s <code>apps.plugin</code>. This plugin walks through the entire <code>/proc</code> filesystem and aggregates statistics per user group. The reported values are compatible with <code>top</code>, although the netdata plugin counts also the resources of exited children (unlike <code>top</code> which shows only the resources of the currently running processes). So for processes like shell scripts, the reported values include the resources used by the commands these scripts run within each timeframe.',
        height: 1.5
    },

    'netdata': {
        title: 'Netdata Monitoring',
        icon: '<i class="fa fa-bar-chart" aria-hidden="true"></i>',
        info: undefined
    },

    'example': {
        title: 'Example Charts',
        info: undefined
    },

    'cgroup': {
        title: '',
        icon: '<i class="fa fa-th" aria-hidden="true"></i>',
        info: undefined
    },

    'cgqemu': {
        title: '',
        icon: '<i class="fa fa-th-large" aria-hidden="true"></i>',
        info: undefined
    },

    'fping': {
        title: 'fping',
        icon: '<i class="fa fa-exchange" aria-hidden="true"></i>',
        info: undefined
    },

    'memcached': {
        title: 'memcached',
        icon: '<i class="fa fa-database" aria-hidden="true"></i>',
        info: undefined
    },

    'mysql': {
        title: 'MySQL',
        icon: '<i class="fa fa-database" aria-hidden="true"></i>',
        info: undefined
    },

    'postgres': {
        title: 'Postgres',
        icon: '<i class="fa fa-database" aria-hidden="true"></i>',
        info: undefined
    },

    'redis': {
        title: 'Redis',
        icon: '<i class="fa fa-database" aria-hidden="true"></i>',
        info: undefined
    },

    'retroshare': {
        title: 'RetroShare',
        icon: '<i class="fa fa-share-alt" aria-hidden="true"></i>',
        info: undefined
    },

    'ipfs': {
        title: 'IPFS',
        icon: '<i class="fa fa-folder-open" aria-hidden="true"></i>',
        info: undefined
    },

    'phpfpm': {
        title: 'PHP-FPM',
        icon: '<i class="fa fa-eye" aria-hidden="true"></i>',
        info: undefined
    },

    'postfix': {
        title: 'postfix',
        icon: '<i class="fa fa-envelope" aria-hidden="true"></i>',
        info: undefined
    },

    'dovecot': {
        title: 'Dovecot',
        icon: '<i class="fa fa-envelope" aria-hidden="true"></i>',
        info: undefined
    },

    'hddtemp': {
        title: 'HDD Temp',
        icon: '<i class="fa fa-thermometer-full" aria-hidden="true"></i>',
        info: undefined
    },

    'nginx': {
        title: 'nginx',
        icon: '<i class="fa fa-eye" aria-hidden="true"></i>',
        info: undefined
    },

    'apache': {
        title: 'Apache',
        icon: '<i class="fa fa-eye" aria-hidden="true"></i>',
        info: undefined
    },

    'named': {
        title: 'named',
        icon: '<i class="fa fa-tag" aria-hidden="true"></i>',
        info: undefined
    },

    'squid': {
        title: 'squid',
        icon: '<i class="fa fa-exchange" aria-hidden="true"></i>',
        info: undefined
    },

    'nut': {
        title: 'UPS',
        icon: '<i class="fa fa-battery-half" aria-hidden="true"></i>',
        info: undefined
    },

    'apcupsd': {
        title: 'UPS',
        icon: '<i class="fa fa-battery-half" aria-hidden="true"></i>',
        info: undefined
    },

    'smawebbox': {
        title: 'Solar Power',
        icon: '<i class="fa fa-sun-o" aria-hidden="true"></i>',
        info: undefined
    },

    'snmp': {
        title: 'SNMP',
        icon: '<i class="fa fa-random" aria-hidden="true"></i>',
        info: undefined
    }
};

// submenu
// information about the submenus
netdataDashboard.submenu = {
    'mem.ksm': {
        title: 'Memory Deduper',
        info: 'Kernel Same-page Merging (KSM) performance monitoring, read from several files in <code>/sys/kernel/mm/ksm/</code>. KSM is a memory-saving de-duplication feature in the Linux kernel (since version 2.6.32). The KSM daemon ksmd periodically scans those areas of user memory which have been registered with it, looking for pages of identical content which can be replaced by a single write-protected page (which is automatically copied if a process later wants to update its content). KSM was originally developed for use with KVM (where it was known as Kernel Shared Memory), to fit more virtual machines into physical memory, by sharing the data common between them.  But it can be useful to any application which generates many instances of the same data.'
    },

    'ipv4.ecn': {
        info: '<a href="https://en.wikipedia.org/wiki/Explicit_Congestion_Notification" target="_blank">Explicit Congestion Notification (ECN)</a> is a TCP extension that allows end-to-end notification of network congestion without dropping packets. ECN is an optional feature that may be used between two ECN-enabled endpoints when the underlying network infrastructure also supports it.'
    },

    'netfilter.conntrack': {
        title: 'Connection Tracker',
        info: 'Netfilter Connection Tracker performance monitoring, read from <code>/proc/net/stat/nf_conntrack</code>. The connection tracker keeps track of all connections of the machine, inbound and outbound. It works by keeping a database with all open connections, tracking network and address translation and connection expectations.'
    },

    'netfilter.nfacct': {
        title: 'Bandwidth Accounting',
        info: 'The following information is read using the <code>nfacct.plugin</code>.'
    },

    'netfilter.synproxy': {
        title: 'DDoS Protection',
        info: 'DDoS Protection performance monitoring read from <code>/proc/net/stat/synproxy</code>. <a href="https://github.com/firehol/firehol/wiki/Working-with-SYNPROXY" target="_blank">SYNPROXY</a> is a TCP SYN packets proxy. It is used to protect any TCP server (like a web server) from SYN floods and similar DDoS attacks. It is a netfilter module, in the Linux kernel (since version 3.12). It is optimized to handle millions of packets per second utilizing all CPUs available without any concurrency locking between the connections. It can be used for any kind of TCP traffic (even encrypted), since it does not interfere with the content itself.'
    },

    'system.softnet_stat': {
        title: 'softnet',
        info: 'Statistics for CPUs SoftIRQs related to network receive work, read from <code>/proc/net/softnet_stat</code>. Break down per CPU core can be found at <a href="#menu_cpu_submenu_softnet_stat">CPU / softnet statistics</a>. <b>processed</b> states the number of packets processed, <b>dropped</b> is the number packets dropped because the network device backlog was full (to fix them use <code>sysctl</code> to increase <code>net.core.netdev_max_backlog</code>), <b>squeezed</b> is the number of packets dropped because the network device budget ran out (to fix them use <code>sysctl</code> to increase <code>net.core.netdev_budget</code>). More information about identifying and troubleshooting network driver related issues can be found at <a href="https://access.redhat.com/sites/default/files/attachments/20150325_network_performance_tuning.pdf" target="_blank">Red Hat Enterprise Linux Network Performance Tuning Guide</a>.'
    },

    'cpu.softnet_stat': {
        title: 'softnet',
        info: 'Statistics for per CPUs core SoftIRQs related to network receive work, read from <code>/proc/net/softnet_stat</code>. Total for all CPU cores can be found at <a href="#menu_system_submenu_softnet_stat">System / softnet statistics</a>. <b>processed</b> states the number of packets processed, <b>dropped</b> is the number packets dropped because the network device backlog was full (to fix them use <code>sysctl</code> to increase <code>net.core.netdev_max_backlog</code>), <b>squeezed</b> is the number of packets dropped because the network device budget ran out (to fix them use <code>sysctl</code> to increase <code>net.core.netdev_budget</code>). More information about identifying and troubleshooting network driver related issues can be found at <a href="https://access.redhat.com/sites/default/files/attachments/20150325_network_performance_tuning.pdf" target="_blank">Red Hat Enterprise Linux Network Performance Tuning Guide</a>.'
    }
};

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
        info: 'Total CPU utilization (all cores). 100% here means there is no CPU idle time at all. You can get per core usage at the <a href="#menu_cpu">CPUs</a> section and per application usage at the <a href="#menu_apps">Applications Monitoring</a> section.<br/>Keep an eye on <b>iowait</b> ' + sparkline('system.cpu', 'iowait', '%') + '. If it is constantly high, your disks are a bottleneck and they slow your system down.<br/>Another important metric worth monitoring, is <b>softirq</b> ' + sparkline('system.cpu', 'softirq', '%') + '. A constantly high percentage of softirq may indicate network driver issues.',
        valueRange: "[0, 100]"
    },

    'system.load': {
        info: 'Current system load, i.e. the number of processes using CPU or waiting for system resources (usually CPU and disk). The 3 metrics refer to 1, 5 and 15 minute averages. Linux calculates this once every 5 seconds. Netdata reads them from <code>/proc/loadavg</code>. For more information check <a href="https://en.wikipedia.org/wiki/Load_(computing)" target="_blank">this wikipedia article</a>',
        height: 0.7
    },

    'system.io': {
        info: 'Total Disk I/O, for all disks, read from <code>/proc/vmstat</code>. You can get detailed information about each disk at the <a href="#menu_disk">Disks</a> section and per application Disk usage at the <a href="#menu_apps">Applications Monitoring</a> section.'
    },

    'system.swapio': {
        info: 'Total Swap I/O, read from <code>/proc/vmstat</code>. (netdata measures both <code>in</code> and <code>out</code>. If either of them is not shown in the chart, it is because it is zero - you can change the page settings to always render all the available dimensions on all charts).'
    },

    'system.pgfaults': {
        info: 'Total page faults, read from <code>/proc/vmstat</code>. <b>Major page faults</b> indicates that the system is using its swap. You can find which applications use the swap at the <a href="#menu_apps">Applications Monitoring</a> section.'
    },

    'system.entropy': {
        colors: '#CC22AA',
        info: '<a href="https://en.wikipedia.org/wiki/Entropy_(computing)" target="_blank">Entropy</a>, read from <code>/proc/sys/kernel/random/entropy_avail</code>, is like a pool of random numbers (<a href="https://en.wikipedia.org/wiki//dev/random" target="_blank">/dev/random</a>) that are mainly used in cryptography. It is advised that the pool remains always <a href="https://blog.cloudflare.com/ensuring-randomness-with-linuxs-random-number-generator/" target="_blank">above 200</a>. If the pool of entropy gets empty, you risk your security to be predictable and you should install a user-space random numbers generating daemon, like <code>haveged</code> or <code>rng-tools</code> (i.e. <b>rngd</b>), to keep the pool in healthy levels.'
    },

    'system.forks': {
        colors: '#5555DD',
        info: 'The number of new processes created per second, read from <code>/proc/stat</code>.'
    },

    'system.intr': {
        colors: '#DD5555',
        info: 'Total number of CPU interrupts, read from <code>/proc/stat</code>. Check <code>system.interrupts</code> that gives more detail about each interrupt and also the <a href="#menu_cpu">CPUs</a> section where interrupts are analyzed per CPU core.'
    },

    'system.interrupts': {
        info: 'CPU interrupts in detail, read from <code>/proc/interrupts</code>. At the <a href="#menu_cpu">CPUs</a> section, interrupts are analyzed per CPU core.'
    },

    'system.softirqs': {
        info: 'CPU softirqs in detail, read from <code>/proc/softirqs</code>. At the <a href="#menu_cpu">CPUs</a> section, softirqs are analyzed per CPU core.'
    },

    'system.processes': {
        info: 'System processes, read from <code>/proc/stat</code>. <b>Running</b> are the processes in the CPU. <b>Blocked</b> are processes that are willing to enter the CPU, but they cannot, e.g. because they wait for disk activity.'
    },

    'system.active_processes': {
        info: 'All system processes, read from <code>/proc/loadavg</code>.'
    },

    'system.ctxt': {
        info: '<a href="https://en.wikipedia.org/wiki/Context_switch" target="_blank">Context Switches</a>, read from <code>/proc/stat</code>, is the switching of the CPU from one process, task or thread to another. If there are many processes or threads willing to execute and very few CPU cores available to handle them, the system is making more context switching to balance the CPU resources among them. The whole process is computationally intensive. The more the context switches, the slower the system gets.'
    },

    'system.idlejitter': {
        colors: '#5555AA',
        info: 'Idle jitter is calculated by netdata. A thread is spawned that requests to sleep for a few microseconds. When the system wakes it up, it measures how many microseconds have passed. The difference between the requested and the actual duration of the sleep, is the <b>idle jitter</b>. This number is useful in real-time environments, where CPU jitter can affect the quality of the service (like VoIP media gateways).'
    },

    'system.ipv4': {
        info: 'Total IPv4 Traffic, read from <code>/proc/net/netstat</code>.'
    },

    'system.ipv6': {
        info: 'Total IPv6 Traffic, read from <code>/proc/net/snmp6</code>.'
    },

    'system.ram': {
        info: 'System memory, read from <code>/proc/meminfo</code>.'
    },

    'system.swap': {
        info: 'System swap memory, read from <code>/proc/meminfo</code>.'
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
            function(id) {
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

    'mem.committed': {
        colors: NETDATA.colors[3]
    },
    
    'mem.pgfaults': {
    	info: 'A <a href="https://en.wikipedia.org/wiki/Page_fault" target="_blank">page fault</a> is a type of interrupt, called trap, raised by computer hardware when a running program accesses a memory page that is mapped into the virtual address space, but not actually loaded into main memory. If the page is loaded in memory at the time the fault is generated, but is not marked in the memory management unit as being loaded in memory, then it is called a <b>minor</b> or soft page fault. A <b>major</b> page fault is generated when the system needs to load the memory page from disk or swap memory. These values are read from <code>/proc/vmstat</code>.'
    },

    'mem.committed': {
        info: 'Committed Memory, read from <code>/proc/meminfo</code>, is the sum of all memory which has been allocated by processes.'
    },

    'mem.writeback': {
        info: 'Read from <code>/proc/meminfo</code>, <b>Dirty</b> is the amount of memory waiting to be written to disk. <b>Writeback</b> is how much memory is actively being written to disk.'
    },

    'mem.kernel': {
        info: 'Read from <code>/proc/meminfo</code>, This chart displays the total ammount of memory being used by the kernel. <b>Slab</b> is the amount of memory used by the kernel to cache data structures for its own use. <b>KernelStack</b> is the amount of memory allocated for each task done by the kernel. <b>PageTables</b> is the amount of memory decicated to the lowest level of page tables (A page table is used to turn a virtual address into a physical memory address). <b>VmallocUsed</b> is the amount of memory being used as virtual address space.'
    },

    'mem.slab': {
        info: 'Read from <code>/proc/meminfo</code>, <b>reclaimable</b> is the amount of memory which the kernel can reuse. <b>unreclaimable</b> can not be reused even when the kernel is lacking memory.'
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
            function(id) {
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
        heads: [
            netdataDashboard.gaugeChart('Received', '12%', 'received'),
            netdataDashboard.gaugeChart('Sent', '12%', 'sent')
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
        info: 'Disk Utilization measures the amount of time the disk was busy with something. This is not related to its performance. 100% means that the Linux kernel always had an outstanding operation on the disk. Keep in mind that depending on the underlying technology of the disk, 100% here may or may not be an indication of congestion.'
    },

    'disk.backlog': {
        colors: '#0099CC',
        info: 'Backlog is an indication of the duration of pending disk operations. On every I/O event the Linux kernel is multiplying the time spent doing I/O since the last update of this field with the number of pending operations. While not accurate, this metric can provide an indication of the expected completion time of the operations in progress.'
    },

    'disk.io': {
        heads: [
            netdataDashboard.gaugeChart('Read', '12%', 'reads'),
            netdataDashboard.gaugeChart('Write', '12%', 'writes')
        ],
        info: 'Amount of data transferred to and from disk.'
    },

    'disk.ops': {
        info: 'Completed disk I/O operations. Keep in mind the number of operations requested might be higher, since the Linux kernel is able to merge adjacent to each other (see merged operations chart).'
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
        info: 'The number of merged disk operations. The Linux kernel is able to merge adjacent I/O operations, for example two 4KB reads can become one 8KB read before given to disk.'
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
            function(id) {
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
            function(id) {
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
    }

};
