// BEGIN dashboard_info_context_linux.js

    'system.cpu': {
        info: 'Total CPU utilization (all cores). 100% here means there is no CPU idle time at all. You can get per core usage at the <a href="#menu_cpu">CPUs</a> section and per application usage at the <a href="#menu_apps">Applications Monitoring</a> section.<br/>Keep an eye on <b>iowait</b> ' + sparkline('system.cpu', 'iowait', '%') + '. If it is constantly high, your disks are a bottleneck and they slow your system down.<br/>Another important metric worth monitoring, is <b>softirq</b> ' + sparkline('system.cpu', 'softirq', '%') + '. A constantly high percentage of softirq may indicate network driver issues.',
        valueRange: "[0, 100]"
    },

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

// END dashboard_info_context_linux.js
