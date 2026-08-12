# Shell exporter

The shell format of the `allmetrics` API exports the latest values collected by one Netdata Agent as Bash-compatible
variable assignments. It is intended for small local automation tasks that need current metric or alert values without
parsing JSON.

Query a trusted local Agent and evaluate the response:

```sh
eval "$(curl --fail --silent --show-error 'http://localhost:19999/api/v3/allmetrics?format=shell')"
```

After this command, the latest values are available as shell variables:

```sh
# source the metrics
eval "$(curl --fail --silent --show-error 'http://localhost:19999/api/v3/allmetrics?format=shell')"

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

The response contains the current stored value for each active, non-obsolete dimension. Empty or non-finite values become an
empty string. It also includes two variables for every running alert: its current numeric value and its status.

The `_VISIBLETOTAL` variable sums all non-hidden dimensions in each chart. A dimension displayed as negative for visual
direction is converted to its absolute direction before it contributes to that total. Decide whether this total is
meaningful for the chart before using it in automation; not every set of dimensions represents additive quantities.

The format of the variables is:

```sh
NETDATA_${chart_id^^}_${dimension_id^^}="${value}"
```

Chart and dimension names are converted to uppercase, and every non-alphanumeric character becomes an underscore. Each name
is limited to 100 characters. Values are rounded to the nearest integer because POSIX shell arithmetic does not process
decimal numbers directly.

Alert variables use these forms:

```text
NETDATA_ALARM_<CHART>_<ALERT>_VALUE="<value>"
NETDATA_ALARM_<CHART>_<ALERT>_STATUS="<status>"
```

## Limit the response

Use the `filter` parameter to select chart names with a Netdata
[simple pattern](/src/libnetdata/simple_pattern/README.md). This reduces response size and avoids importing variables that
the script does not need.

```sh
eval "$(curl --fail --silent --show-error \
  'http://localhost:19999/api/v3/allmetrics?format=shell&filter=system.*')"
```

Patterns can include exact chart names, wildcards, multiple space-separated alternatives, and negated alternatives. URL
encode the parameter when it contains spaces or other reserved characters.

## Security and operational limits

`eval` executes its input as shell code. Use it only with a Netdata endpoint you control and trust, preferably the local
Agent. Do not evaluate output fetched through an untrusted proxy, redirect, or user-supplied URL. `--fail` prevents curl from
passing ordinary HTTP error responses to `eval`, but it cannot make an untrusted successful response safe.

The endpoint returns a snapshot, not a subscription and not historical data. A script that needs changes over time must poll
again and handle collection gaps, Agent restarts, renamed charts, and alerts that appear or disappear. For structured data,
decimal values, metadata, or multi-host processing, use the JSON or Prometheus formats instead of shell variables.

The API is subject to the Agent's dashboard access controls and optional bearer protection. If bearer protection is enabled,
authenticate the request using the supported API mechanism; never embed a long-lived credential in a script that can be read
by untrusted users.

