// BEGIN dashboard_info_menu_linux.js

    'services': {
        title: 'systemd Services',
        icon: '<i class="fa fa-cogs" aria-hidden="true"></i>',
        info: 'Resources utilization of systemd services.'
    },

    'tc': {
        title: 'Quality of Service',
        icon: '<i class="fa fa-globe" aria-hidden="true"></i>',
        info: 'Netdata collects and visualizes tc class utilization using its <a href="https://github.com/firehol/netdata/blob/master/plugins.d/tc-qos-helper.sh" target="_blank">tc-helper plugin</a>. If you also use <a href="http://firehol.org/#fireqos" target="_blank">FireQOS</a> for setting up QoS, netdata automatically collects interface and class names. If your QoS configuration includes overheads calculation, the values shown here will include these overheads (the total bandwidth for the same interface as reported in the Network Interfaces section, will be lower than the total bandwidth reported here). Also, data collection may have a slight time difference compared to the interface (QoS data collection is implemented with a BASH script, so a shift in data collection of a few milliseconds should be justified).'
    },

    'netfilter': {
        title: 'Firewall (netfilter)',
        icon: '<i class="fa fa-shield" aria-hidden="true"></i>',
        info: undefined
    },

    'ipvs': {
        title: 'IP Virtual Server',
        icon: '<i class="fa fa-eye" aria-hidden="true"></i>',
        info: undefined
    },

// END dashboard_info_menu_linux.js
