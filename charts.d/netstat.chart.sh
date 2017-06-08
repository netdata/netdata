# no need for shebang - this file is loaded from charts.d.plugin

# netdata
# real-time performance and health monitoring, done right!
# (C) 2016 Costa Tsaousis <costa@tsaousis.gr>
# GPL v3+
#

# if this chart is called X.chart.sh, then all functions and global variables
# must start with X_

# _update_every is a special variable - it holds the number of seconds
# between the calls of the _update() function
netstat_update_every=1

# the priority is used to sort the charts on the dashboard
# 1 = the first chart
netstat_priority=5000

# global variables to store our collected data
# remember: they need to start with the module name netstat_

# _check is called once, to find out if this chart should be enabled or not
netstat_check() {
    if ! command -v netstat >/dev/null 2>&1; then
        error "command netstat not found"
        return 1
    fi
    return 0
}

# _create is called once, to create the charts
netstat_create() {
# create the chart with 3 dimensions
    cat <<EOF
CHART netstat.tcp '' "TCP netstat" "connections" netstat netstat.tcp area $netstat_priority $netstat_update_every
DIMENSION ESTABLISHED  '' absolute 1 1
DIMENSION SYN_SENT     '' absolute 1 1
DIMENSION SYN_RECV     '' absolute 1 1
DIMENSION FIN_WAIT1    '' absolute 1 1
DIMENSION FIN_WAIT2    '' absolute 1 1
DIMENSION TIME_WAIT    '' absolute 1 1
DIMENSION CLOSE        '' absolute 1 1
DIMENSION CLOSE_WAIT   '' absolute 1 1
DIMENSION LISTEN       '' absolute 1 1
DIMENSION CLOSING      '' absolute 1 1
EOF
    return 0
}

netstat_update() {
    echo "BEGIN netstat.tcp $1"
    netstat -tan | grep tcp | awk '{print $6}' | sort | uniq -c | awk '{print "SET",$2,"=",$1}'
    echo END

    return 0
}
