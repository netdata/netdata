# Health API Calls

## Health Read API

### Enabled Alarms

NetData enables alarms on demand, i.e. when the chart they should be linked to starts collecting data. So, although many more alarms are configured, only the useful ones are enabled.

To get the list of all enabled alarms:

`http://your.netdata.ip:19999/api/v1/alarms?all`

### Raised Alarms

This API call will return the alarms currently in WARNING or CRITICAL state.

`http://your.netdata.ip:19999/api/v1/alarms`

### Event Log

The size of the alarm log is configured in `netdata.conf`. There are 2 settings: the rotation of the alarm log file and the in memory size of the alarm log.

```
[health]
	in memory max health log entries = 1000
	rotate log every lines = 2000
```

The API call retrieves all entries of the alarm log:

`http://your.netdata.ip:19999/api/v1/alarm_log`

### Alarm Log Incremental Updates

`http://your.netdata.ip:19999/api/v1/alarm_log?after=UNIQUEID`

The above returns all the events in the alarm log that occurred after UNIQUEID (you poll it once without `after=`, remember the last UNIQUEID of the returned set, which you give back to get incrementally the next events).

### Alarm badges

The following will return an SVG badge of the alarm named `NAME`, attached to the chart named `CHART`.

`http://your.netdata.ip:19999/api/v1/badge.svg?alarm=NAME&chart=CHART`

## Health Management API

Netdata v1.12 and beyond provides a command API to control health checks and notifications at runtime. The feature is especially useful for maintenance periods, during which you receive meaningless alarms.
From Netdata v1.16.0 and beyond, the configuration controlled via the API commands is [persisted across Netdata restarts](#persistence).

Specifically, the API allows you to:
 - Disable health checks completely. Alarm conditions will not be evaluated at all and no entries will be added to the alarm log.
 - Silence alarm notifications. Alarm conditions will be evaluated, the alarms will appear in the log and the Netdata UI will show the alarms as active, but no notifications will be sent.
 - Disable or Silence specific alarms that match selectors on alarm/template name, chart, context, host and family.

The API is available by default, but it is protected by an `api authorization token` that is stored in the file you will see in the following entry of `http://localhost:19999/netdata.conf`:

```
[registry]
    # netdata management api key file = /var/lib/netdata/netdata.api.key
```

You can access the API via GET requests, by adding the bearer token to an `Authorization` http header, like this:

```
curl "http://myserver/api/v1/manage/health?cmd=RESET" -H "X-Auth-Token: Mytoken"
```

By default access to the health management API is only allowed from `localhost`. Accessing the API from anything else will return a 403 error with the message `You are not allowed to access this resource.`. You can change permissions by editing the `allow management from` variable in `netdata.conf` within the [web] section. See [web server access lists](../../server/#access-lists) for more information.

The command `RESET` just returns Netdata to the default operation, with all health checks and notifications enabled.
If you've configured and entered your token correclty, you should see the plain text response `All health checks and notifications are enabled`.

### Disable or silence all alarms

If all you need is temporarily disable all health checks, then you issue the following before your maintenance period starts:

```
curl "http://myserver/api/v1/manage/health?cmd=DISABLE ALL" -H "X-Auth-Token: Mytoken"
```

The effect of disabling health checks is that the alarm criteria are not evaluated at all and nothing is written in the alarm log.
If you want the health checks to be running but to not receive any notifications during your maintenance period, you can instead use this:

```
curl "http://myserver/api/v1/manage/health?cmd=SILENCE ALL" -H "X-Auth-Token: Mytoken"
```

Alarms may then still be raised and logged in Netdata, so you'll be able to see them via the UI.  

Regardless of the option you choose, at the end of your maintenance period you revert to the normal state via the RESET command.

```
 curl "http://myserver/api/v1/manage/health?cmd=RESET" -H "X-Auth-Token: Mytoken"
```

### Disable or silence specific alarms

If you do not wish to disable/silence all alarms, then the `DISABLE ALL` and `SILENCE ALL` commands can't be used.
Instead, the following commands expect that one or more alarm selectors will be added, so that only alarms that match the selectors are disabled or silenced.  
- `DISABLE` : Set the mode to disable health checks.
- `SILENCE` : Set the mode to silence notifications.

You will normally put one of these commands in the same request with your first alarm selector, but it's possible to issue them separately as well.
You will get a warning in the response, if a selector was added without a SILENCE/DISABLE command, or vice versa.

Each request can specify a single alarm `selector`, with one or more `selection criteria`.
A single alarm will match a `selector` if all selection criteria match the alarm.
You can add as many selectors as you like.
In essence, the rule is: IF (alarm matches all the criteria in selector1 OR all the criteria in selector2 OR ...) THEN apply the DISABLE or SILENCE command.

To clear all selectors and reset the mode to default, use the `RESET` command.

The following example silences notifications for all the alarms with context=load:

```
curl "http://myserver/api/v1/manage/health?cmd=SILENCE&context=load" -H "X-Auth-Token: Mytoken"
```

#### Selection criteria

The `selection criteria` are key/value pairs, in the format `key : value`, where value is a Netdata [simple pattern](../../../libnetdata/simple_pattern/). This means that you can create very powerful selectors (you will rarely need more than one or two).

The accepted keys for the `selection criteria` are the following:
- `alarm`    : The expression provided will match both `alarm` and `template` names.
- `chart`    : Chart ids/names, as shown on the dashboard. These will match the `on` entry of a configured `alarm`.
- `context`  : Chart context, as shown on the dashboard. These will match the `on` entry of a configured `template`.
- `hosts`    : The hostnames that will need to match.
- `families` : The alarm families.

You can add any of the selection criteria you need on the request, to ensure that only the alarms you are interested in are matched and disabled/silenced. e.g. there is no reason to add `hosts: *`, if you want the criteria to be applied to alarms for all hosts.

Example 1: Disable all health checks for context = `random`

```
http://localhost/api/v1/manage/health?cmd=DISABLE&context=random
```

Example 2: Silence all alarms and templates with name starting with `out_of` on host `myhost`

```
http://localhost/api/v1/manage/health?cmd=SILENCE&alarm=out_of*&hosts=myhost
```

Example 2.2: Add one more selector, to also silence alarms for cpu1 and cpu2

```
http://localhost/api/v1/manage/health?families=cpu1 cpu2
```

### List silencers

The command `LIST` was added in Netdata v1.16.0 and returns a JSON with the current status of the silencers.

```
 curl "http://myserver/api/v1/manage/health?cmd=LIST" -H "X-Auth-Token: Mytoken"
```

As an example, the following response shows that we have two silencers configured, one for an alarm called `samplealarm` and one for alarms with context `random` on host `myhost`

```
json
{
        "all": false,
        "type": "SILENCE",
        "silencers": [
                {
                        "alarm": "samplealarm"
                },
                {
                        "context": "random",
                        "hosts": "myhost"
                }
        ]
}
```

The response below shows that we have disabled all health checks.

```
json
{
        "all": true,
        "type": "DISABLE",
        "silencers": []
}
```

### Responses

- "Auth Error" : Token authentication failed
- "All alarm notifications are silenced" : Successful response to cmd=SILENCE ALL
- "All health checks are disabled" : Successful response to cmd=DISABLE ALL
- "All health checks and notifications are enabled" : Successful response to cmd=RESET
- "Health checks disabled for alarms matching the selectors" : Added to the response for a cmd=DISABLE
- "Alarm notifications silenced for alarms matching the selectors" : Added to the response for a cmd=SILENCE
- "Alarm selector added" : Added to the response when a new selector is added
- "Invalid key. Ignoring it." : Wrong name of a parameter. Added to the response and ignored.
- "WARNING: Added alarm selector to silence/disable alarms without a SILENCE or DISABLE command." : Added to the response if a selector is added without a selector-specific command.
- "WARNING: SILENCE or DISABLE command is ineffective without defining any alarm selectors." : Added to the response if a selector-specific command is issued without a selector.

### Persistence

From Netdata v1.16.0 and beyond, the silencers configuration is persisted to disk and loaded when Netdata starts.
The JSON string returned by the [LIST command](#list-silencers) is automatically saved to the `silencers file`, every time a command alters the silencers configuration.
The file's location is configurable in `netdata.conf`. The default is shown below:

```
[health]
        # silencers file = /var/lib/netdata/health.silencers.json
```

### Further reading

The test script under [tests/health_mgmtapi](../../../tests/health_mgmtapi) contains a series of tests that you can either run or read through to understand the various calls and responses better.


[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fweb%2Fapi%2Fhealth%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
