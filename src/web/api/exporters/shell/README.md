# Shell exporter

Shell scripts can now query Netdata:

```sh
eval "$(curl -s 'http://localhost:19999/api/v3/allmetrics')"
```

after this command, all the Netdata metrics are exposed to shell. Check:

```sh
# source the metrics
eval "$(curl -s 'http://localhost:19999/api/v3/allmetrics')"

# let's see if there are variables exposed by Netdata for system.cpu
set | grep "^NETDATA_SYSTEM_CPU"

NETDATA_SYSTEM_CPU_GUEST=0
NETDATA_SYSTEM_CPU_GUEST_NICE=0
NETDATA_SYSTEM_CPU_IDLE=95
NETDATA_SYSTEM_CPU_IOWAIT=0
NETDATA_SYSTEM_CPU_IRQ=0
NETDATA_SYSTEM_CPU_NICE=0
NETDATA_SYSTEM_CPU_SOFTIRQ=0
NETDATA_SYSTEM_CPU_STEAL=0
NETDATA_SYSTEM_CPU_SYSTEM=1
NETDATA_SYSTEM_CPU_USER=4
NETDATA_SYSTEM_CPU_VISIBLETOTAL=5

# let's see the total cpu utilization of the system
echo ${NETDATA_SYSTEM_CPU_VISIBLETOTAL}
5

# what about alerts?
set | grep "^NETDATA_ALARM_SYSTEM_SWAP_"
NETDATA_ALARM_SYSTEM_SWAP_USED_SWAP_STATUS=CLEAR
NETDATA_ALARM_SYSTEM_SWAP_USED_SWAP_VALUE=51

# let's get the current status of the alert 'used swap'
echo ${NETDATA_ALARM_SYSTEM_SWAP_USED_SWAP_STATUS}
CLEAR

# is it fast?
time curl -s 'http://localhost:19999/api/v3/allmetrics' >/dev/null

real  0m0,070s
user  0m0,000s
sys   0m0,007s

# it is...
# 0.07 seconds for curl to be loaded, connect to Netdata and fetch the response back...
```

The `_VISIBLETOTAL` variable sums up all the dimensions of each chart.

The format of the variables is:

```sh
NETDATA_${chart_id^^}_${dimension_id^^}="${value}"
```

The value is rounded to the closest integer, since shell script cannot process decimal numbers.


