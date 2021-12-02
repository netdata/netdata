netdata (1.10.0) - 2018-03-27

 Please check full changelog at github.
 <https://github.com/netdata/netdata/releases>

netdata (1.9.0) - 2017-12-17

 Please check full changelog at github.
 <https://github.com/netdata/netdata/releases>

netdata (1.8.0) - 2017-09-17

 This is mainly a bugfix release.
 Please check full changelog at github.

netdata (1.7.0) - 2017-07-16

-   netdata is still spreading fast

    we are at 320.000 users and 132.000 servers

    Almost 100k new users, 52k new installations and 800k docker pulls
    since the previous release, 4 and a half months ago.

    netdata user base grows at about 1000 new users and 600 new servers
    per day. Thank you. You are awesome.

-   The next release (v1.8) will be focused on providing a global health
    monitoring service, for all netdata users, for free.

-   netdata is now a (very fast) fully featured statsd server and the
    only one with automatic visualization: push a statsd metric and hit
    F5 on the netdata dashboard: your metric visualized. It also supports
    synthetic charts, defined by you, so that you can correlate and
    visualize your application the way you like it.

-   netdata got new installation options
    It is now easier than ever to install netdata - we also distribute a
    statically linked netdata x86_64 binary, including key dependencies
    (like bash, curl, etc) that can run everywhere a Linux kernel runs
    (CoreOS, CirrOS, etc).

-   metrics streaming and replication has been improved significantly.
    All known issues have been solved and key enhancements have been added.
    Headless collectors and proxies can now send metrics to backends when
    data source = as collected.

-   backends have got quite a few enhancements, including host tags and
    metrics filtering at the netdata side;
    prometheus support has been re-written to utilize more prometheus
    features and provide more flexibility and integration options.

-   netdata now monitors ZFS (on Linux and FreeBSD), ElasticSearch,
    RabbitMQ, Go applications (via expvar), ipfw (on FreeBSD 11), samba,
    squid logs (with web_log plugin).

-   netdata dashboard loading times have been improved significantly
    (hit F5 a few times on a netdata dashboard - it is now amazingly fast),
    to support dashboards with thousands of charts.

-   netdata alarms now support custom hooks, so you can run whatever you
    like in parallel with netdata alarms.

-   As usual, this release brings dozens of more improvements, enhancements
    and compatibility fixes.

netdata (1.6.0) - 2017-03-20

-   birthday release: 1 year netdata

    netdata was first published on March 30th, 2016.
    It has been a crazy year since then:

      225.000 unique netdata users
              currently, at 1.000 new unique users per day

       80.000 unique netdata installations
              currently, at 500 new installation per day

      610.000 docker pulls on docker hub

    4.000.000 netdata sessions served
              currently, at 15.000 sessions served per day

       20.000 github stars

    ```
          Thank you!
       You are awesome!
    ```

-   central netdata is here

    This is the first release that supports real-time streaming of
    metrics between netdata servers.

    netdata can now be:

    -   autonomous host monitoring
        (like it always has been)

    -   headless data collector
        (collect and stream metrics in real-time to another netdata)

    -   headless proxy
        (collect metrics from multiple netdata and stream them to another netdata)

    -   store and forward proxy
        (like headless proxy, but with a local database)

    -   central database
        (metrics from multiple hosts are aggregated)

    metrics databases can be configured on all nodes and each node maintaining
    a database may have a different retention policy and possibly run
    (even different) alarms on them.

-   monitoring ephemeral nodes

    netdata now supports monitoring autoscaled ephemeral nodes,
    that are started and stopped on demand (their IP is not known).

    When the ephemeral nodes start streaming metrics to the central
    netdata, the central netdata will show register them at "my-netdata"
    menu on the dashboard.

    For more information check:
    <https://github.com/netdata/netdata/tree/master/streaming#monitoring-ephemeral-nodes>

-   monitoring ephemeral containers and VM guests

    netdata now cleans up container, guest VM, network interfaces and mounted
    disk metrics, disabling automatically their alarms too.

    For more information check:
    <https://github.com/netdata/netdata/tree/master/collectors/cgroups.plugin#monitoring-ephemeral-containers>

-   apps.plugin ported for FreeBSD

    @vlvkobal has ported "apps.plugin" to FreeBSD. netdata can now provide
    "Applications", "Users" and "User Groups" on FreeBSD.

-   web_log plugin

    @l2isbad has done a wonderful job creating a unified web log parsing plugin
    for all kinds of web server logs. With it, netdata provides real-time
    performance information and health monitoring alarms for web applications
    and web sites!

    For more information check:
    <https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/web_log#web_log>

-   backends

    netdata can now archive metrics to `JSON` backends
    (both push, by @lfdominguez, and pull modes).

-   IPMI monitoring

    netdata now has an IPMI plugin (based on freeipmi)
    for monitoring server hardware.

    The plugin creates (up to) 8 charts:

    1.  number of sensors by state
    2.  number of events in SEL
    3.  Temperatures CELSIUS
    4.  Temperatures FAHRENHEIT
    5.  Voltages
    6.  Currents
    7.  Power
    8.  Fans

    It also supports alarms (including the number of sensors in critical state).

    For more information, check:
    <https://github.com/netdata/netdata/tree/master/collectors/freeipmi.plugin>

-   new plugins

    @l2isbad builds python data collection plugins for netdata at an wonderful
    rate! He rocks!

    -   **web_log** for monitoring in real-time all kinds of web server log files @l2isbad
    -   **freeipmi** for monitoring IPMI (server hardware)
    -   **nsd** (the [name server daemon](https://www.nlnetlabs.nl/projects/nsd/)) @383c57
    -   **mongodb** @l2isbad
    -   **smartd_log** (monitoring disk S.M.A.R.T. values) @l2isbad

-   improved plugins

    -   **nfacct** reworked and now collects connection tracker information using netlink.
    -   **ElasticSearch** re-worked @l2isbad
    -   **mysql** re-worked to allow faster development of custom mysql based plugins (MySQLService) @l2isbad
    -   **SNMP**
    -   **tomcat** @NMcCloud
    -   **ap** (monitoring hostapd access points)
    -   **php_fpm** @l2isbad
    -   **postgres** @l2isbad
    -   **isc_dhcpd** @l2isbad
    -   **bind_rndc** @l2isbad
    -   **numa**
    -   **apps.plugin** improvements and freebsd support @vlvkobal
    -   **fail2ban** @l2isbad
    -   **freeradius** @l2isbad
    -   **nut** (monitoring UPSes)
    -   **tc** (Linux QoS) now works on qdiscs instead of classes for the same result (a lot faster) @t-h-e
    -   **varnish** @l2isbad

-   new and improved alarms
    -   **web_log**, many alarms to detect common web site/API issues
    -   **fping**, alarms to detect packet loss, disconnects and unusually high latency
    -   **cpu**, cpu utilization alarm now ignores `nice`

-   new and improved alarm notification methods
    -   **HipChat** to allow hosted HipChat @frei-style
    -   **discordapp** @lowfive

-   dashboard improvements
    -   dashboard now works on HiDPi screens
    -   dashboard now shows version of netdata
    -   dashboard now resets charts properly
    -   dashboard updated to use latest gauge.js release

-   other improvements
    -   thanks to @rlefevre netdata now uses a lot of different high resolution system clocks.

 netdata has received a lot more improvements from many more  contributors!

 Thank you all!

netdata (1.5.0) - 2017-01-22

-   yet another release that makes netdata the fastest
    netdata ever!

-   netdata runs on FreeBSD, FreeNAS and MacOS !

    Vladimir Kobal (@vlvkobal) has done a magnificent work
    porting netdata to FreeBSD and MacOS.

    Everything works: cpu, memory, disks performance, disks space,
    network interfaces, interrupts, IPv4 metrics, IPv6 metrics
    processes, context switches, softnet, IPC queues,
    IPC semaphores, IPC shared memory, uptime, etc. Wow!

-   netdata supports data archiving to backend databases:

    -   Graphite
    -   OpenTSDB
    -   Prometheus

    and of course all the compatible ones
    (KairosDB, InfluxDB, Blueflood, etc)

-   new plugins:

    Ilya Mashchenko (@l2isbad) has created most of the python
    data collection plugins in this release !

    -   systemd Services (using cgroups!)
    -   FPing (yes, network latency in netdata!)
    -   postgres databases            @facetoe, @moumoul
    -   Vanish disk cache (v3 and v4) @l2isbad
    -   ElasticSearch                 @l2isbad
    -   HAproxy                       @l2isbad
    -   FreeRadius                    @l2isbad, @lgz
    -   mdstat (RAID)                 @l2isbad
    -   ISC bind (via rndc)           @l2isbad
    -   ISC dhcpd                     @l2isbad, @lgz
    -   Fail2Ban                      @l2isbad
    -   OpenVPN status log            @l2isbad, @lgz
    -   NUMA memory                   @tycho
    -   CPU Idle                      @tycho
    -   gunicorn log                  @deltaskelta
    -   ECC memory hardware errors
    -   IPC semaphores
    -   uptime plugin (with a nice badge too)

-   improved plugins:

    -   netfilter conntrack
    -   mysql (replication)           @l2isbad
    -   ipfs                          @pjz
    -   cpufreq                       @tycho
    -   hddtemp                       @l2isbad
    -   sensors                       @l2isbad
    -   nginx                         @leolovenet
    -   nginx_log                     @paulfantom
    -   phpfpm                        @leolovenet
    -   redis                         @leolovenet
    -   dovecot                       @justohall
    -   cgroups
    -   disk space
    -   apps.plugin
    -   /proc/interrupts              @rlefevre
    -   /proc/softirqs                @rlefevre
    -   /proc/vmstat       (system memory charts)
    -   /proc/net/snmp6    (IPv6 charts)
    -   /proc/self/meminfo (system memory charts)
    -   /proc/net/dev      (network interfaces)
    -   tc                 (linux QoS)

-   new/improved alarms:

    -   MySQL / MariaDB alarms (incl. replication)
    -   IPFS alarms
    -   HAproxy alarms
    -   UDP buffer alarms
    -   TCP AttemptFails
    -   ECC memory alarms
    -   netfilter connections alarms
    -   SNMP

-   new alarm notifications:

    -   messagebird.com               @tech-no-logical
    -   pagerduty.com                 @jimcooley
    -   pushbullet.com                @tperalta82
    -   twilio.com                    @shadycuz
    -   HipChat
    -   kafka

-   shell integration

    -   shell scripts can now query netdata easily!

-   dashboard improvements:
    -   dashboard is now faster on firefox, safari, opera, edge
        (edge is still the slowest)
    -   dashboard now has a little bigger fonts
    -   SHIFT + mouse wheel to zoom charts, works on all browsers
    -   perfect-scrollbar on the dashboard
    -   dashboard 4K resolution fixes
    -   dashboard compatibility fixes for embedding charts in
        third party web sites
    -   charts on custom dashboards can have common min/max
        even if they come from different netdata servers
    -   alarm log is now saved and loaded back so that
        the alarm history is available at the dashboard

-   other improvements:
    -   python.d.plugin has received way to many improvements
        from many contributors!
    -   charts.d.plugin can now be forked to support
        multiple independent instances
    -   registry has been re-factored to lower its memory
        requirements (required for the public registry)
    -   simple patterns in cgroups, disks and alarms
    -   netdata-installer.sh can now correctly install
        netdata in containers
    -   supplied logrotate script compatibility fixes
    -   spec cleanup                  @breed808
    -   clocks and timers reworked    @rlefevre

 netdata has received a lot more improvements from many more
 contributors!

 Thank you all guys!

netdata (1.4.0) - 2016-10-04

 At a glance:

-   the fastest netdata ever (with a better look too)!

-   improved IoT and containers support!

-   alarms improved in almost every way!

-   new plugins:
       softnet netdev,
       extended TCP metrics,
       UDPLite
       NFS v2, v3 client (server was there already),
       NFS v4 server & client,
       APCUPSd,
       RetroShare

-   improved plugins:
       mysql,
       cgroups,
       hddtemp,
       sensors,
       phpfpm,
       tc (QoS)

 In detail:

-   improved alarms

    Many new alarms have been added to detect common kernel
    configuration errors and old alarms have been re-worked
    to avoid notification floods.

    Alarms now support notification hysteresis (both static
    and dynamic), notification self-cancellation, dynamic
    thresholds based on current alarm status

-   improved alarm notifications

    netdata now supports:

    -   email notifications
    -   slack.com notifications on slack channels
    -   pushover.net notifications (mobile push notifications)
    -   telegram.org notifications

    For all the above methods, netdata supports role-based
    notifications, with multiple recipients for each role
    and severity filtering per recipient!

    Also, netdata support HTML5 notifications, while the
    dashboard is open in a browser window (no need to be
    the active one).

    All notifications are now clickable to get to the chart
    that raised the alarm.

-   improved IoT support!

    netdata builds and runs with musl libc and runs on systems
    based on busybox.

-   improved containers support!

    netdata runs on alpine linux (a low profile linux distribution
    used in containers).

-   Dozens of other improvements and bugfixes

netdata (1.3.0) - 2016-08-28

 At a glance:

-   netdata has health monitoring / alarms!
-   netdata has badges that can be embeded anywhere!
-   netdata plugins are now written in Python!
-   new plugins: redis, memcached, nginx_log, ipfs, apache_cache

 IMPORTANT:
 Since netdata now uses Python plugins, new packages are
 required to be installed on a system to allow it work.
 For more information, please check the installation page:

 <https://github.com/netdata/netdata/tree/master/installer#installation>

 In detail:

-   netdata has alarms!

    Based on the POLL we made on github
    (<https://github.com/netdata/netdata/issues/436>),
    health monitoring was the winner. So here it is!

    netdata now has a powerful health monitoring system embedded.
    Please check the wiki page:

    <https://github.com/netdata/netdata/tree/master/health>

-   netdata has badges!

    netdata can generate badges with live information from the
    collected metrics.
    Please check the wiki page:

    <https://github.com/netdata/netdata/tree/master/web/api/badges>

-   netdata plugins are now written in Python!

    Thanks to the great work of Paweł Krupa (@paulfantom), most BASH
    plugins have been ported to Python.

    The new python.d.plugin supports both python2 and python3 and
    data collection from multiple sources for all modules.

    The following pre-existing modules have been ported to Python:

    -   apache
    -   cpufreq
    -   example
    -   exim
    -   hddtemp
    -   mysql
    -   nginx
    -   phpfpm
    -   postfix
    -   sensors
    -   squid
    -   tomcat

    The following new modules have been added:

    -   apache_cache
    -   dovecot
    -   ipfs
    -   memcached
    -   nginx_log
    -   redis

-   other data collectors:

    -   Thanks to @simonnagl netdata now reports disk space usage.

-   dashboards now transfer a certain settings from server to server
    when changing servers via the my-netdata menu.

    The settings transferred are the dashboard theme, the online
    help status and current pan and zoom timeframe of the dashboard.

-   API improvements:

    -   reduction functions now support 'min', 'sum' and 'incremental-sum'.

    -   netdata now offers a multi-threaded and a single threaded
        web server (single threaded is better for IoT).

-   apps.plugin improvements:

    -   can now run with command line argument 'without-files'
        to prevent it from enumerating all the open files/sockets/pipes
        of all running processes.

    -   apps.plugin now scales the collected values to match the
        the total system usage.

    -   apps.plugin can now report guest CPU usage per process.

    -   repeating errors are now logged once per process.

-   netdata now runs with IDLE process priority (lower than nice 19)

-   netdata now instructs the kernel to kill it first when it starves
    for memory.

-   netdata listens for signals:

    -   SIGHUP to netdata instructs it to re-open its log files
        (new logrotate files added too).

    -   SIGUSR1 to netdata saves the database

    -   SIGUSR2 to netdata reloads health / alarms configuration

-   netdata can now bind to multiple IPs and ports.

-   netdata now has new systemd service file (it starts as user
    netdata and does not fork).

-   Dozens of other improvements and bugfixes

netdata (1.2.0) - 2016-05-16

 At a glance:

-   netdata is now 30% faster
-   netdata now has a registry (my-netdata dashboard menu)
-   netdata now monitors Linux Containers (docker, lxc, etc)

 IMPORTANT:
 This version requires libuuid. The package you need is:

-   uuid-dev (debian/ubuntu), or
-   libuuid-devel (centos/fedora/redhat)

 In detail:

-   netdata is now 30% faster !

    -   Patches submitted by @fredericopissarra improved overall
        netdata performance by 10%.

    -   A new improved search function in the internal indexes
        made all searches faster by 50%, resulting in about
        20% better performance for the core of netdata.

    -   More efficient threads locking in key components
        contributed to the overall efficiency.

-   netdata now has a CENTRAL REGISTRY !

    The central registry tracks all your netdata servers
    and bookmarks them for you at the 'my-netdata' menu
    on all dashboards.

    Every netdata can act as a registry, but there is also
    a global registry provided for free for all netdata users!

-   netdata now monitors CONTAINERS !

    docker, lxc, or anything else. For each container it monitors
    CPU, RAM, DISK I/O (network interfaces were already monitored)

-   apps.plugin: now uses linux capabilities by default
    without setuid to root

-   netdata has now an improved signal handler
    thanks to @simonnagl

-   API: new improved CORS support

-   SNMP: counter64 support fixed

-   MYSQL: more charts, about QCache, MyISAM key cache,
    InnoDB buffer pools, open files

-   DISK charts now show mount point when available

-   Dashboard: improved support for older web browsers
    and mobile web browsers (thanks to @simonnagl)

-   Multi-server dashboards now allow de-coupled refreshes for
    each chart, so that if one netdata has a network latency
    the other charts are not affected

-   Several other minor improvements and bugfixes

netdata (1.1.0) - 2016-04-20

 Dozens of commits that improve netdata in several ways:

-   Data collection: added IPv6 monitoring
-   Data collection: added SYNPROXY DDoS protection monitoring
-   Data collection: apps.plugin: added charts for users and user groups
-   Data collection: apps.plugin: grouping of processes now support patterns
-   Data collection: apps.plugin: now it is faster, after the new features added
-   Data collection: better auto-detection of partitions for disk monitoring
-   Data collection: better fireqos integration for QoS monitoring
-   Data collection: squid monitoring now uses squidclient
-   Data collection: SNMP monitoring now supports 64bit counters
-   API: fixed issues in CSV output generation
-   API: netdata can now be restricted to listen on a specific IP
-   Core and apps.plugin: error log flood protection
-   Dashboard: better error handling when the netdata server is unreachable
-   Dashboard: each chart now has a toolbox
-   Dashboard: on-line help support
-   Dashboard: check for netdata updates button
-   Dashboard: added example /tv.html dashboard
-   Packaging: now compiles with musl libc (alpine linux)
-   Packaging: added debian packaging
-   Packaging: support non-root installations
-   Packaging: the installer generates uninstall script

netdata (1.0.0) - 2016-03-22

-   first public release

netdata (1.0.0-rc.1) - 2015-11-28

-   initial packaging
