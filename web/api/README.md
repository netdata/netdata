# netdata REST API

The complete documentation of the netdata API is available at the **[Swagger Editor](https://editor.swagger.io/?url=https://raw.githubusercontent.com/netdata/netdata/master/web/netdata-swagger.yaml)**.

If your prefer it over the Swagger Editor, you can also use **[Swagger UI](https://registry.my-netdata.io/swagger/#!/default/get_data)**. This however does not provide all the information available.

## google charts

netdata is a [Google Visualization API datatable and datasource provider](https://developers.google.com/chart/interactive/docs/reference), so it can directly be used with [Google Charts](https://developers.google.com/chart/interactive/docs/).

Check this [single chart, jsfiddle example](https://jsfiddle.net/ktsaou/ensu4uws/9/):

![image](https://cloud.githubusercontent.com/assets/2662304/23824762/1e236b84-0685-11e7-89f4-06fdf67d873a.png)

and this [multi chart, jsfiddle example](https://jsfiddle.net/ktsaou/L5y2eqp2/):

![image](https://cloud.githubusercontent.com/assets/2662304/23824766/31a4a68c-0685-11e7-8429-8327cab64be2.png)


## using the api from shell scripts

Shell scripts can now query netdata easily:

```sh
eval "$(curl -s 'http://localhost:19999/api/v1/allmetrics')"
```

after this command, all the netdata metrics are exposed to shell. Check:

```sh
# source the metrics
eval "$(curl -s 'http://localhost:19999/api/v1/allmetrics')"

# let's see if there are variables exposed by netdata for system.cpu
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

# what about alarms?
set | grep "^NETDATA_ALARM_SYSTEM_SWAP_"
NETDATA_ALARM_SYSTEM_SWAP_RAM_IN_SWAP_STATUS=CRITICAL
NETDATA_ALARM_SYSTEM_SWAP_RAM_IN_SWAP_VALUE=53
NETDATA_ALARM_SYSTEM_SWAP_USED_SWAP_STATUS=CLEAR
NETDATA_ALARM_SYSTEM_SWAP_USED_SWAP_VALUE=51

# let's get the current status of the alarm 'ram in swap'
echo ${NETDATA_ALARM_SYSTEM_SWAP_RAM_IN_SWAP_STATUS}
CRITICAL

# is it fast?
time curl -s 'http://localhost:19999/api/v1/allmetrics' >/dev/null

real  0m0,070s
user  0m0,000s
sys   0m0,007s

# it is...
# 0.07 seconds for curl to be loaded, connect to netdata and fetch the response back...
```

The `_VISIBLETOTAL` variable sums up all the dimensions of each chart.

The format of the variables is:

```sh
NETDATA_${chart_id^^}_${dimension_id^^}="${value}"
```

The value is rounded to the closest integer, since shell script cannot process decimal numbers.
