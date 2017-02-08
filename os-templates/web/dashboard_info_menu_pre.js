// BEGIN dashboard_info_menu_pre.js

// menu
// information for the main menus

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

    'net': {
        title: 'Network Interfaces',
        icon: '<i class="fa fa-share-alt" aria-hidden="true"></i>',
        info: 'Statistics for network interfaces.'
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
        info: 'Per application statistics are collected using netdata\'s <code>apps.plugin</code>. This plugin walks through the entire process tree and aggregates statistics for applications of interest, defined in <code>/etc/netdata/apps_groups.conf</code> (the default is <a href="https://github.com/firehol/netdata/blob/master/conf.d/apps_groups.conf" target="_blank">here</a>). The plugin internally builds an hierarchical process tree and groups processes together (evaluating both child and parent processes) so that the result is always a chart with a predefined set of dimensions (of course, only application groups found running are reported). The reported values are compatible with <code>top</code>, although the netdata plugin counts also the resources of exited children (unlike <code>top</code> which shows only the resources of the currently running processes). So for processes like shell scripts, the reported values include the resources used by the commands these scripts run within each timeframe.',
        height: 1.5
    },

    'users': {
        title: 'Users',
        icon: '<i class="fa fa-user" aria-hidden="true"></i>',
        info: 'Per user statistics are collected using netdata\'s <code>apps.plugin</code>. This plugin walks through the entire process tree and aggregates statistics per user. The reported values are compatible with <code>top</code>, although the netdata plugin counts also the resources of exited children (unlike <code>top</code> which shows only the resources of the currently running processes). So for processes like shell scripts, the reported values include the resources used by the commands these scripts run within each timeframe.',
        height: 1.5
    },

    'groups': {
        title: 'User Groups',
        icon: '<i class="fa fa-users" aria-hidden="true"></i>',
        info: 'Per user group statistics are collected using netdata\'s <code>apps.plugin</code>. This plugin walks through the entire process tree and aggregates statistics per user group. The reported values are compatible with <code>top</code>, although the netdata plugin counts also the resources of exited children (unlike <code>top</code> which shows only the resources of the currently running processes). So for processes like shell scripts, the reported values include the resources used by the commands these scripts run within each timeframe.',
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
        info: 'Network latency statistics, using <code>fping</code>.'
    },

    'memcached': {
        title: 'memcached',
        icon: '<i class="fa fa-database" aria-hidden="true"></i>',
        info: undefined
    },

    'mysql': {
        title: 'MySQL',
        icon: '<i class="fa fa-database" aria-hidden="true"></i>',
        info: 'MySQL/MariaDB SQL database performance metrics.'
    },

    'postgres': {
        title: 'Postgres',
        icon: '<i class="fa fa-database" aria-hidden="true"></i>',
        info: 'Postgres SQL database performance metrics.'
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
        info: 'Nginx web server performance metrics.'
    },

    'apache': {
        title: 'Apache',
        icon: '<i class="fa fa-eye" aria-hidden="true"></i>',
        info: 'Apache web server performance metrics.'
    },

    'named': {
        title: 'named',
        icon: '<i class="fa fa-tag" aria-hidden="true"></i>',
        info: 'ISC-Bind domain name server performance metrics.'
    },

    'squid': {
        title: 'squid',
        icon: '<i class="fa fa-exchange" aria-hidden="true"></i>',
        info: 'Squid proxy server performance metrics.'
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
    },

// END dashboard_info_menu_pre.js
