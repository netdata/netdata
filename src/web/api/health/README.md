# Health API Calls

## Health Read API

### Enabled Alerts

Netdata enables alerts on demand, i.e. when the chart they should be linked to starts collecting data. So, although many
more alerts are configured, only the useful ones are enabled.

To get the list of all enabled alerts, open your browser and navigate to `http://NODE:19999/api/v1/alarms?all`,
replacing `NODE` with the IP address or hostname for your Agent dashboard.

### Raised Alerts

This API call will return the alerts currently in WARNING or CRITICAL state.

`http://NODE:19999/api/v1/alarms`

### Event Log

The size of the alert log is configured in `netdata.conf`. There are 2 settings: the event history kept in the DB (in seconds), and the in memory size of the alert log.

```
[health]
	in memory max health log entries = 1000
	health log retention = 5d
```

The API call retrieves all entries of the alert log:

`http://NODE:19999/api/v1/alarm_log`

### Alert Log Incremental Updates

`http://NODE:19999/api/v1/alarm_log?after=UNIQUEID`

The above returns all the events in the alert log that occurred after UNIQUEID (you poll it once without `after=`, remember the last UNIQUEID of the returned set, which you give back to get incrementally the next events).

### Alert badges

The following will return an SVG badge of the alert named `NAME`, attached to the chart named `CHART`.

`http://NODE:19999/api/v1/badge.svg?alarm=NAME&chart=CHART`

## Health Management API

Netdata v1.12 and beyond provides a command API to control health checks and notifications at runtime. The feature is especially useful for maintenance periods, during which you receive meaningless alerts.
From Netdata v1.16.0 and beyond, the configuration controlled via the API commands is [persisted across Netdata restarts](#persistence).

Specifically, the API allows you to:

-   Disable health checks completely. Alert conditions will not be evaluated at all and no entries will be added to the alert log.
-   Silence alert notifications. Alert conditions will be evaluated, the alerts will appear in the log and the Netdata UI will show the alerts as active, but no notifications will be sent.
-   Disable or Silence specific alerts that match selectors on alert/template name, chart, context, and host.

The API is available by default, but it is protected by an `api authorization token` that is stored in the file you will see in the following entry of `http://NODE:19999/netdata.conf`:

```
[registry]
    # netdata management api key file = /var/lib/netdata/netdata.api.key
```

You can access the API via GET requests, by adding the bearer token to an `Authorization` http header, like this:

```
curl "http://NODE:19999/api/v1/manage/health?cmd=RESET" -H "X-Auth-Token: Mytoken"
```

By default access to the health management API is only allowed from `localhost`. Accessing the API from anything else will return a 403 error with the message `You are not allowed to access this resource.`. You can change permissions by editing the `allow management from` variable in `netdata.conf` within the [web] section. See [web server access lists](/src/web/server/README.md#access-lists) for more information.

The command `RESET` just returns Netdata to the default operation, with all health checks and notifications enabled.
If you've configured and entered your token correctly, you should see the plain text response `All health checks and notifications are enabled`.

### Disable or silence all alerts

If all you need is temporarily disable all health checks, then you issue the following before your maintenance period starts:

```sh
curl "http://NODE:19999/api/v1/manage/health?cmd=DISABLE%20ALL" -H "X-Auth-Token: Mytoken"
```

The effect of disabling health checks is that the alert criteria are not evaluated at all and nothing is written in the alert log.
If you want the health checks to be running but to not receive any notifications during your maintenance period, you can instead use this:

```sh
curl "http://NODE:19999/api/v1/manage/health?cmd=SILENCE%20ALL" -H "X-Auth-Token: Mytoken"
```

Alerts may then still be raised and logged in Netdata, so you'll be able to see them via the UI.  

Regardless of the option you choose, at the end of your maintenance period you revert to the normal state via the RESET command.

```sh
 curl "http://NODE:19999/api/v1/manage/health?cmd=RESET" -H "X-Auth-Token: Mytoken"
```

### Disable or silence specific alerts

If you do not wish to disable/silence all alerts, then the `DISABLE ALL` and `SILENCE ALL` commands can't be used.
Instead, the following commands expect that one or more alert selectors will be added, so that only alerts that match the selectors are disabled or silenced.  

-   `DISABLE` : Set the mode to disable health checks.
-   `SILENCE` : Set the mode to silence notifications.

You will normally put one of these commands in the same request with your first alert selector, but it's possible to issue them separately as well.
You will get a warning in the response, if a selector was added without a SILENCE/DISABLE command, or vice versa.

Each request can specify a single alert `selector`, with one or more `selection criteria`.
A single alert will match a `selector` if all selection criteria match the alert.
You can add as many selectors as you like.
In essence, the rule is: IF (alert matches all the criteria in selector1 OR all the criteria in selector2 OR ...) THEN apply the DISABLE or SILENCE command.

To clear all selectors and reset the mode to default, use the `RESET` command.

The following example silences notifications for all the alerts with context=load:

```
curl "http://NODE:19999/api/v1/manage/health?cmd=SILENCE&context=load" -H "X-Auth-Token: Mytoken"
```

#### Selection criteria

The `selection criteria` are key/value pairs, in the format `key : value`, where value is a Netdata [simple pattern](/src/libnetdata/simple_pattern/README.md). This means that you can create very powerful selectors (you will rarely need more than one or two).

The accepted keys for the `selection criteria` are the following:

-   `alarm`    : The expression provided will match both `alarm` and `template` names.
-   `chart`    : Chart ids/names, as shown on the dashboard. These will match the `on` entry of a configured `alarm`.
-   `context`  : Chart context, as shown on the dashboard. These will match the `on` entry of a configured `template`.
-   `hosts`    : The hostnames that will need to match.

You can add any of the selection criteria you need on the request, to ensure that only the alerts you are interested in are matched and disabled/silenced. e.g. there is no reason to add `hosts: *`, if you want the criteria to be applied to alerts for all hosts.

Example 1: Disable all health checks for context = `random`

```
http://NODE:19999/api/v1/manage/health?cmd=DISABLE&context=random
```

Example 2: Silence all alerts and templates with name starting with `out_of` on host `myhost`

```
http://NODE:19999/api/v1/manage/health?cmd=SILENCE&alarm=out_of*&hosts=myhost
```

### List silencers

The command `LIST` was added in Netdata v1.16.0 and returns a JSON with the current status of the silencers.

```
 curl "http://NODE:19999/api/v1/manage/health?cmd=LIST" -H "X-Auth-Token: Mytoken"
```

As an example, the following response shows that we have two silencers configured, one for an alert called `samplealert` and one for alerts with context `random` on host `myhost`

```
json
{
        "all": false,
        "type": "SILENCE",
        "silencers": [
                {
                        "alarm": "samplealert"
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

-   "Auth Error" : Token authentication failed
-   "All alarm notifications are silenced" : Successful response to cmd=SILENCE ALL
-   "All health checks are disabled" : Successful response to cmd=DISABLE ALL
-   "All health checks and notifications are enabled" : Successful response to cmd=RESET
-   "Health checks disabled for alarms matching the selectors" : Added to the response for a cmd=DISABLE
-   "Alarm notifications silenced for alarms matching the selectors" : Added to the response for a cmd=SILENCE
-   "Alarm selector added" : Added to the response when a new selector is added
-   "Invalid key. Ignoring it." : Wrong name of a parameter. Added to the response and ignored.
-   "WARNING: Added alarm selector to silence/disable alarms without a SILENCE or DISABLE command." : Added to the response if a selector is added without a selector-specific command.
-   "WARNING: SILENCE or DISABLE command is ineffective without defining any alarm selectors." : Added to the response if a selector-specific command is issued without a selector.

### Persistence

From Netdata v1.16.0 and beyond, the silencers configuration is persisted to disk and loaded when Netdata starts.
The JSON string returned by the [LIST command](#list-silencers) is automatically saved to the `silencers file`, every time a command alters the silencers configuration.
The file's location is configurable in `netdata.conf`. The default is shown below:

```
[health]
        # silencers file = /var/lib/netdata/health.silencers.json
```

### Further reading

The test script under [tests/health_mgmtapi](/tests/health_mgmtapi/README.md) contains a series of tests that you can either run or read through to understand the various calls and responses better.


