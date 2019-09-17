# Health monitoring

Each Netdata node runs an independent thread evaluating health monitoring checks.
This thread has lock free access to the database, so that it can operate as a watchdog.

Health checks (alarms) are attached to Netdata charts, allowing Netdata to automatically
activate an alarm as soon as a chart is created. This is very important for
netdata, since many charts are dynamically created during runtime (for example, the
chart tracking network interface packet drops, is automatically created on the first
packet dropped).

Netdata also supports alarm **templates**, so that an alarm can be attached to all the charts of the same context (i.e. all network interfaces, or all disks, or all mysql servers, etc.).  

Each alarm can execute a single query to the database using statistical algorithms against past data,
but alarms can be combined. So, if you need 2 queries in the database, you can combine
2 alarms together (both will run a query to the database, and the results can be combined).

Each alarm has unlimited access to all the metrics collected. So, a single alarm can
use expressions combining the latest value of any number of metrics.

## Health configuration reference

Stock Netdata health configuration is in `/usr/lib/netdata/conf.d/health.d`.
These files can be overwritten by copying them and editing them in `/etc/netdata/health.d`
(run `/etc/netdata/edit-config` to edit them).

In `/etc/netdata/health.d` you can also put any number of files (in any number of sub-directories)
with a suffix `.conf` to have them processed by Netdata.

Health configuration can be reloaded at any time, without restarting Netdata.
Just send Netdata the SIGUSR2 signal, like this:

```sh
killall -USR2 netdata
```

### Entities in the health files

There are 2 entities:

1.  **alarms**, which are attached to specific charts, and

2.  **templates**, which define rules that should be applied to all charts having a
    specific `context`. You can use this feature to apply **alarms** to all disks,
    all network interfaces, all mysql databases, all nginx web servers, etc.

Both of these entities have exactly the same format and feature set.
The only difference is the label `alarm` or `template`.

Netdata supports overriding **templates** with **alarms**.
For example, when a template is defined for a set of charts, an alarm with exactly the
same name attached to the same chart the template matches, will have higher precedence
(i.e. Netdata will use the alarm on this chart and prevent the template from being applied
to it).

### The format

The following lines are parsed.

#### Alarm line `alarm` or `template`

This line starts an alarm or alarm template.

```
alarm: NAME
```

or

```
template: NAME
```

This line has to be first on each alarm or template.
`NAME` is anything you would like to name it (the only symbols allowed are `.` and `_`).

---

#### Alarm line `on`

This line defines the data the alarm should be attached to.

For alarms:

```
on: CHART
```

For `CHART` you can use a chart `id` or `name` of the chart, as shown on the dashboard.

For alarm templates:

```
on: CONTEXT
```

`CONTEXT` is the template of a chart. For example the charts `mysql_local.net` and
`mysql_server2.net` have the same context: `mysql.net`. So, you can use this to apply
alarms to all `mysql.net` charts.

To find the `CONTEXT` of a chart hover over its date, above the legend. A tooltip will
appear with this format `plugin:nodule, context`. For example, the bandwidth chart of
a network interface says:

```
proc:/proc/dev/dev, net.net
```

So, `plugin = proc`, `module = /proc/net/dev` and `context = net.net`.

---

#### Alarm line `os`

This alarm or template will be used only if the O/S of the host loading it, matches this
pattern list. The value is a space separated list of simple patterns (use `*` as wildcard,
prefix with `!` for a negative match, order is important).

```
os: linux freebsd macos
```

---

#### Alarm line `hosts`

This alarm or template will be used only if the hostname of the host loading it, matches
this pattern list. The value is a space separated list of simple patterns (use `*` as wildcard,
prefix with `!` for a negative match, order is important).

```
hosts: server1 server2 database* !redis3 redis*
```

The above says: use this alarm on all hosts named `server1`, `server2`, `database*`, and
all `redis*` except `redis3`.

This is useful when you centralize metrics from multiple hosts, to one Netdata.

---

#### Alarm line `families`

This line is only used in alarm templates. It filters the charts. So, if you need to create
an alarm template for a few of a kind of chart (a few of your disks, or a few of your network
interfaces, or a few your mysql servers, etc), you can create an alarm template that would
normally be applied to all of them, and filter them by [family](../web/README.md#families).

The format is:

```
families: SIMPLE PATTERN LIST
```

The simple pattern syntax and operation is explained in [simple patterns](../libnetdata/simple_pattern/).

---

#### Alarm line `lookup`

This line makes a database lookup to find a value. This result of this lookup is available as `$this`.

The format is:

```
lookup: METHOD AFTER [at BEFORE] [every DURATION] [OPTIONS] [of DIMENSIONS]
```

Everything is the same with [badges](../web/api/badges/). In short:

-   `METHOD` is one of `average`, `min`, `max`, `sum`, `incremental-sum`.
     This is required.

-   `AFTER` is a relative number of seconds, but it also accepts a single letter for changing
     the units, like `-1s` = 1 second in the past, `-1m` = 1 minute in the past, `-1h` = 1 hour
     in the past, `-1d` = 1 day in the past. You need a negative number (i.e. how far in the past
     to look for the value). **This is required**.

-   `at BEFORE` is by default 0 and is not required. Using this you can define the end of the
     lookup. So data will be evaluated between `AFTER` and `BEFORE`.

-   `every DURATION` sets the updated frequency of the lookup (supports single letter units as
     above too).

-   `OPTIONS` is a space separated list of `percentage`, `absolute`, `min2max`, `unaligned`,
     `match-ids`, `match-names`. Check the badges documentation for more info.

-   `of DIMENSIONS` is optional and has to be the last parameter. Dimensions have to be separated
     by `,` or `|`. The space characters found in dimensions will be kept as-is (a few dimensions
     have spaces in their names). This accepts Netdata simple patterns and the `match-ids` and
     `match-names` options affect the searches for dimensions.

The result of the lookup will be available as `$this` and `$NAME` in expressions.
The timestamps of the timeframe evaluated by the database lookup is available as variables
`$after` and `$before` (both are unix timestamps).

---

#### Alarm line `calc`

This expression is evaluated just after the `lookup` (if any). Its purpose is to apply some
calculation before using the value looked up from the db.

You can also have an expression without a lookup, using other variables that are available.

The result of the calculation will be available as `$this` in warning and critical expressions
(overwriting the `lookup` one).

Format:

```
calc: EXPRESSION
```

Check [Expressions](#expressions) for more information.

---

#### Alarm line `every`

Sets the update frequency of this alarm.  This is the same to the `every DURATION` given
in the `lookup` lines.

Format:

```
every: DURATION
```

`DURATION` accepts `s` for seconds, `m` is minutes, `h` for hours, `d` for days.

---

#### Alarm lines `green` and `red`

Set the green and red thresholds of a chart. Both are available as `$green` and `$red` in
expressions. If multiple alarms define different thresholds, the ones defined by the first
alarm will be used. These will eventually visualized on the dashboard, so only one set of
them is allowed. If you need multiple sets of them in different alarms, use absolute numbers
instead of `$red` and `$green`.

Format:

```
green: NUMBER
red: NUMBER
```

---

#### Alarm lines `warn` and `crit`

These expressions should evaluate to true or false (alternatively non-zero or zero).
They trigger the alarm. Both are optional.

Format:

```
warn: EXPRESSION
crit: EXPRESSION
```

Check [Expressions](#expressions) for more information.

---

#### Alarm line `to`

This will be the first parameter of the script to be executed when the alarm switches status.
Its meaning is left up to the `exec` script.

The default `exec` script, `alarm-notify.sh`, uses this field as a space separated list of roles,
which are then consulted to find the exact recipients per notification method.

Format:

```
to: ROLE1 ROLE2 ROLE3 ...
```

---

#### Alarm line `exec`

The script that will be executed when the alarm changes status.

Format:

```
exec: SCRIPT
```

The default `SCRIPT` is Netdata's `alarm-notify.sh`, which supports all the notifications
methods Netdata supports, including custom hooks.

---

#### Alarm line `delay`

This is used to provide optional hysteresis settings for the notifications, to defend
against notification floods. These settings do not affect the actual alarm - only the time
the `exec` script is executed.

Format:

```
delay: [[[up U] [down D] multiplier M] max X]
```

-   `up U` defines the delay to be applied to a notification for an alarm that raised its status
     (i.e. CLEAR to WARNING, CLEAR to CRITICAL, WARNING to CRITICAL). For example, `up 10s`, the
     notification for this event will be sent 10 seconds after the actual event. This is used in
     hope the alarm will get back to its previous state within the duration given. The default `U`
     is zero.

-   `down D` defines the delay to be applied to a notification for an alarm that moves to lower
     state (i.e. CRITICAL to WARNING, CRITICAL to CLEAR, WARNING to CLEAR). For example, `down 1m`
     will delay the notification by 1 minute. This is used to prevent notifications for flapping
     alarms. The default `D` is zero.

-   `mutliplier M` multiplies `U` and `D` when an alarm changes state, while a notification is
     delayed. The default multiplier is `1.0`.

-   `max X`  defines the maximum absolute notification delay an alarm may get. The default `X`
     is `max(U * M, D * M)` (i.e. the max duration of `U` or `D` multiplied once with `M`).

    Example:

    `delay: up 10s down 15m multiplier 2 max 1h`

    The time is `00:00:00` and the status of the alarm is CLEAR.

    | time of event | new status | delay               | notification will be sent|why|
    |-------------|----------|:---:|-------------------------|---|
    | 00:00:01      | WARNING    | `up 10s`            | 00:00:11|first state switch|
    | 00:00:05      | CLEAR      | `down 15m x2`       | 00:30:05|the alarm changes state while a notification is delayed, so it was multiplied|
    | 00:00:06      | WARNING    | `up 10s x2 x2`      | 00:00:26|multiplied twice|
    | 00:00:07      | CLEAR      | `down 15m x2 x2 x2` | 00:45:07|multiplied 3 times.|

    So:

    -   `U` and `D` are multiplied by `M` every time the alarm changes state (any state, not just
        their matching one) and a delay is in place.
    -   All are reset to their defaults when the alarm switches state without a delay in place.

---

#### Alarm line `repeat`

Defines the interval between repeating notifications for the alarms in CRITICAL or WARNING mode. This will override the default interval settings inherited from health settings in `netdata.conf`. The default settings for repeating notifications are `default repeat warning = DURATION` and `default repeat critical = DURATION` which can be found in health stock configuration, when one of these interval is bigger than 0, Netdata will activate the repeat notification for `CRITICAL`, `CLEAR` and `WARNING` messages.
`

Format:

```
repeat: [off] [warning DURATION] [critical DURATION]
```

-   `off`: Turns off the repeating feature for the current alarm. This is effective when the default repeat settings has been enabled in health configuration.
-   `warning DURATION`: Defines the interval when the alarm is in WARNING state. Use `0s` to turn off the repeating notification for WARNING mode.
-   `critical DURATION`: Defines the interval when the alarm is in CRITICAL state. Use `0s` to turn off the repeating notification for CRITICAL mode.

---

#### Alarm line `option`

The only possible value for the `option` line is

```
option: no-clear-notification
```

For some alarms we need compare two time-frames, to detect anomalies. For example, `health.d/httpcheck.conf` has an alarm template called `web_service_slow` that compares the average http call response time over the last 3 minutes, compared to the average over the last hour. It triggers a warning alarm when the average of the last 3 minutes is twice the average of the last hour. In such cases, it is easy to trigger the alarm, but difficult to tell when the alarm is cleared. As time passes, the newest window moves into the older, so the average response time of the last hour will keep increasing. Eventually, the comparison will find the averages in the two time-frames close enough to clear the alarm. However, the issue was not resolved, it's just a matter of the newer data "polluting" the old. For such alarms, it's a good idea to tell Netdata to not clear the notification, by using the `no-clear-notification` option.

---

### Expressions

Netdata has an internal [infix expression parser](../libnetdata/eval).
This parses expressions and creates an internal structure that allows fast execution of them.

These operators are supported `+`, `-`, `*`, `/`, `<`, `<=`, `<>`, `!=`, `>`, `>=`, `&&`, `||`,
`!`, `AND`, `OR`, `NOT`. Boolean operators result in either `1` (true) or `0` (false).

The conditional evaluation operator `?` is supported too. Using this operator IF-THEN-ELSE conditional statements can be specified. The format is: `(condition) ? (true expression) :(false expression)`. So, Netdata will first evaluate the `condition` and based on the result will either evaluate `true expression` or `false expression`.

Example: `($this > 0) ? ($avail * 2) : ($used / 2)`.

Nested such expressions are also supported (i.e. `true expression` and `false expression` can contain conditional evaluations).

Expressions also support the `abs()` function.

Expressions can have variables. Variables start with `$`. Check below for more information.

There are two special values you can use:

-   `nan`, for example `$this != nan` will check if the variable `this` is available. A variable can be `nan` if the database lookup failed. All calculations (i.e. addition, multiplication, etc) with a `nan` result in a `nan`.

-   `inf`, for example `$this != inf` will check if `this` is not infinite. A value or variable can be infinite if divided by zero. All calculations (i.e. addition, multiplication, etc) with a `inf` result in a `inf`.

---

### Special use of the conditional operator

A common (but not necessarily obvious) use of the conditional evaluation operator is
to provide [hysteresis](https://en.wikipedia.org/wiki/Hysteresis) around the critical
or warning thresholds.  This usage helps to avoid bogus messages resulting from small
variations in the value when it is varying regularly but staying close to the threshold
value, without needing to delay sending messages at all.

An example of such usage from the default CPU usage alarms bundled with Netdata is:

```
warn: $this > (($status >= $WARNING)  ? (75) : (85))
crit: $this > (($status == $CRITICAL) ? (85) : (95))
```

The above say:

-   If the alarm is currently a warning, then the threshold for being considered a warning
     is 75, otherwise it's 85.

-   If the alarm is currently critical, then the threshold for being considered critical
     is 85, otherwise it's 95.

Which in turn, results in the following behavior:

-   While the value is rising, it will trigger a warning when it exceeds 85, and a critical
     alert when it exceeds 95.

-   While the value is falling, it will return to a warning state when it goes below 85,
     and a normal state when it goes below 75.

-   If the value is constantly varying between 80 and 90, then it will trigger a warning the
     first time it goes above 85, but will remain a warning until it goes below 75 (or goes above 85).

-   If the value is constantly varying between 90 and 100, then it will trigger a critical alert
     the first time it goes above 95, but will remain a critical alert goes below 85 (at which
     point it will return to being a warning).

---

### Variables

You can find all the variables that can be used for a given chart, using
`http://your.netdata.ip:19999/api/v1/alarm_variables?chart=CHART_NAME`
Example: [variables for the `system.cpu` chart of the registry](https://registry.my-netdata.io/api/v1/alarm_variables?chart=system.cpu).

_Hint: If you don't know how to find the CHART_NAME, you can read about it [here](../web/README.md#charts)._

Netdata supports 3 internal indexes for variables that will be used in health monitoring.

<details markdown="1"><summary>The variables below can be used in both chart alarms and context templates.</summary>

Although the `alarm_variables` link shows you variables for a particular chart, the same variables can also be used in templates for charts belonging to a given [context](../web/README.md#contexts). The reason is that all charts of a given context are essentially identical, with the only difference being the [family](../web/README.md#families) that identifies a particular hardware or software instance. Charts and templates do not apply to specific families anyway, unless if you explicitly limit an alarm with the [alarm line `families`](#alarm-line-families).
</details>

-   **chart local variables**. All the dimensions of the chart are exposed as local variables. The value of $this for the other configured alarms of the chart also appears, under the name of each configured alarm.

     Charts also define a few special variables:

    -   `$last_collected_t` is the unix timestamp of the last data collection
    -   `$collected_total_raw` is the sum of all the dimensions (their last collected values)
    -   `$update_every` is the update frequency of the chart
    -   `$green` and `$red` the threshold defined in alarms (these are per chart - the charts
         inherits them from the the first alarm that defined them)

     Chart dimensions define their last calculated (i.e. interpolated) value, exactly as
     shown on the charts, but also a variable with their name and suffix `_raw` that resolves
     to the last collected value - as collected and another with suffix `_last_collected_t`
     that resolves to unix timestamp the dimension was last collected (there may be dimensions
     that fail to be collected while others continue normally).

-   **family variables**. Families are used to group charts together. For example all `eth0`
     charts, have `family = eth0`. This index includes all local variables, but if there are
     overlapping variables, only the first are exposed.

-   **host variables**. All the dimensions of all charts, including all alarms, in fullname.
     Fullname is `CHART.VARIABLE`, where `CHART` is either the chart id or the chart name (both
     are supported).

-   **special variables\*** are:

    -   `$this`, which is resolved to the value of the current alarm.

    -   `$status`, which is resolved to the current status of the alarm (the current = the last
         status, i.e. before the current database lookup and the evaluation of the `calc` line).
         This values can be compared with `$REMOVED`, `$UNINITIALIZED`, `$UNDEFINED`, `$CLEAR`,
         `$WARNING`, `$CRITICAL`. These values are incremental, ie. `$status > $CLEAR` works as
         expected.

    -   `$now`, which is resolved to current unix timestamp.

## Alarm Statuses

Alarms can have the following statuses:

-   `REMOVED` - the alarm has been deleted (this happens when a SIGUSR2 is sent to Netdata
     to reload health configuration)

-   `UNINITIALIZED` - the alarm is not initialized yet

-   `UNDEFINED` - the alarm failed to be calculated (i.e. the database lookup failed,
     a division by zero occurred, etc)

-   `CLEAR` - the alarm is not armed / raised (i.e. is OK)

-   `WARNING` - the warning expression resulted in true or non-zero

-   `CRITICAL` - the critical expression resulted in true or non-zero

The external script will be called for all status changes.

## Examples

Check the `health/health.d/` directory for all alarms shipped with Netdata.

Here are a few examples:

### Example 1

A simple check if an apache server is alive:

```
template: apache_last_collected_secs
      on: apache.requests
    calc: $now - $last_collected_t
   every: 10s
    warn: $this > ( 5 * $update_every)
    crit: $this > (10 * $update_every)
```

The above checks that Netdata is able to collect data from apache. In detail:

```
template: apache_last_collected_secs
```

The above defines a **template** named `apache_last_collected_secs`.
The name is important since `$apache_last_collected_secs` resolves to the `calc` line.
So, try to give something descriptive.

```
      on: apache.requests
```

The above applies the **template** to all charts that have `context = apache.requests`
(i.e. all your apache servers).

```
    calc: $now - $last_collected_t
```

-   `$now` is a standard variable that resolves to the current timestamp.

-   `$last_collected_t` is the last data collection timestamp of the chart.
     So this calculation gives the number of seconds passed since the last data collection.

```
   every: 10s
```

The alarm will be evaluated every 10 seconds.

```
    warn: $this > ( 5 * $update_every)
    crit: $this > (10 * $update_every)
```

If these result in non-zero or true, they trigger the alarm.

-   `$this` refers to the value of this alarm (i.e. the result of the `calc` line.
     We could also use `$apache_last_collected_secs`.

`$update_every` is the update frequency of the chart, in seconds.

So, the warning condition checks if we have not collected data from apache for 5
iterations and the critical condition checks for 10 iterations.

### Example 2

Check if any of the disks is critically low on disk space:

```
template: disk_full_percent
      on: disk.space
    calc: $used * 100 / ($avail + $used)
   every: 1m
    warn: $this > 80
    crit: $this > 95
  repeat: warning 120s critical 10s
```

`$used` and `$avail`  are the `used` and `avail` chart dimensions as shown on the dashboard.

So, the `calc` line finds the percentage of used space. `$this` resolves to this percentage.

This is a repeating alarm and if the alarm becomes CRITICAL it repeats the notifications every 10 seconds. It also repeats notifications every 2 minutes if the alarm goes into WARNING mode.

### Example 3

Predict if any disk will run out of space in the near future.

We do this in 2 steps:

Calculate the disk fill rate:

```
    template: disk_fill_rate
          on: disk.space
      lookup: max -1s at -30m unaligned of avail
        calc: ($this - $avail) / (30 * 60)
       every: 15s
```

In the `calc` line: `$this` is the result of the `lookup` line (i.e. the free space 30 minutes
ago) and `$avail` is the current disk free space. So the `calc` line will either have a positive
number of GB/second if the disk if filling up, or a negative number of GB/second if the disk is
freeing up space.

There is no `warn` or `crit` lines here. So, this template will just do the calculation and
nothing more.

Predict the hours after which the disk will run out of space:

```
    template: disk_full_after_hours
          on: disk.space
        calc: $avail / $disk_fill_rate / 3600
       every: 10s
        warn: $this > 0 and $this < 48
        crit: $this > 0 and $this < 24
```

The `calc` line estimates the time in hours, we will run out of disk space. Of course, only
positive values are interesting for this check, so the warning and critical conditions check
for positive values and that we have enough free space for 48 and 24 hours respectively.

Once this alarm triggers we will receive an email like this:

![image](https://cloud.githubusercontent.com/assets/2662304/17839993/87872b32-6802-11e6-8e08-b2e4afef93bb.png)

### Example 4

Check if any network interface is dropping packets:

```
template: 30min_packet_drops
      on: net.drops
  lookup: sum -30m unaligned absolute
   every: 10s
    crit: $this > 0
```

The `lookup` line will calculate the sum of the all dropped packets in the last 30 minutes.

The `crit` line will issue a critical alarm if even a single packet has been dropped.

Note that the drops chart does not exist if a network interface has never dropped a single packet.
When Netdata detects a dropped packet, it will add the chart and it will automatically attach this
alarm to it.

## Troubleshooting

You can compile Netdata with [debugging](../daemon#debugging) and then set in `netdata.conf`:

```
[global]
   debug flags = 0x0000000000800000
```

Then check your `/var/log/netdata/debug.log`. It will show you how it works.
Important: this will generate a lot of output in debug.log.

You can find the context of charts by looking up the chart in either
`http://your.netdata:19999/netdata.conf` or `http://your.netdata:19999/api/v1/charts`.

You can find how Netdata interpreted the expressions by examining the alarm at `http://your.netdata:19999/api/v1/alarms?all`. For each expression, Netdata will return the expression as given in its config file, and the same expression with additional parentheses added to indicate the evaluation flow of the expression.

## Disabling health checks or silencing notifications at runtime

It's currently not possible to schedule notifications from within the alarm template. For those scenarios where you need to temporary disable notifications (for instance when running backups triggers a disk alert) you can disable or silence notifications are runtime. The health checks can be controlled at runtime via the [health management api](../web/api/health/#health-management-api).

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fhealth%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
