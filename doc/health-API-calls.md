# Health API Calls

## all enabled alarms

netdata enables alarms on demand, i.e. when the chart they should be linked to starts collecting data. So, although many more alarms are configured, only the useful ones are enabled.

To get the list of all enabled alarms:

`http://your.netdata.ip:19999/api/v1/alarms?all`

## all currently raised alarms

This API call will return the alarms currently in WARNING or CRITICAL state.

`http://your.netdata.ip:19999/api/v1/alarms`

## all the event log

The size of the alarm log is configured in `netdata.conf`. There are 2 settings: the rotation of the alarm log file and the in memory size of the alarm log.

```
[health]
	health db file = /var/lib/netdata/health/health-log.db
	in memory max health log entries = 1000
	rotate log every lines = 2000
```

The API call the get all entries of the alarm log:

`http://your.netdata.ip:19999/api/v1/alarm_log`

## incremental updates of the alarm log

`http://your.netdata.ip:19999/api/v1/alarm_log?after=UNIQUEID`

The above returns all the events in the alarm log that occurred after UNIQUEID (you poll it once without `after=`, remember the last UNIQUEID of the returned set, which you give back to get incrementally the next events).

## alarm badges

The following will return an SVG badge of the alarm named `NAME`, attached to the chart named `CHART`.

`http://your.netdata.ip:19999/api/v1/badge.svg?alarm=NAME&chart=CHART`

