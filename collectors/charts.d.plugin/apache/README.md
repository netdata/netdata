# Apache

> THIS MODULE IS OBSOLETE.
> USE [THE PYTHON ONE](../../python.d.plugin/apache) - IT SUPPORTS MULTIPLE JOBS AND IT IS MORE EFFICIENT

---

The `apache` collector visualizes key performance data for an apache web server.

## Example netdata charts

For apache 2.2:

![image](https://cloud.githubusercontent.com/assets/2662304/12530273/421c4d14-c1e2-11e5-9fb6-ca6d6dd3b1dd.png)

For apache 2.4:

![image](https://cloud.githubusercontent.com/assets/2662304/12530376/29ec26de-c1e6-11e5-9af1-e48aaf781795.png)

## How it works

It runs `curl "http://apache.host/server-status?auto` to fetch the current status of apache.

It has been tested with apache 2.2 and apache 2.4. The latter also provides connections information (total and break down by status).

Apache 2.2 response:

```sh
$ curl "http://127.0.0.1/server-status?auto"
Total Accesses: 80057
Total kBytes: 223017
CPULoad: .018287
Uptime: 64472
ReqPerSec: 1.24173
BytesPerSec: 3542.15
BytesPerReq: 2852.59
BusyWorkers: 1
IdleWorkers: 49
Scoreboard: _________________________......................................._W_______________________.......................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................
```

Apache 2.4 response:

```sh
$ curl "http://127.0.0.1/server-status?auto"
127.0.0.1
ServerVersion: Apache/2.4.18 (Unix)
ServerMPM: event
Server Built: Dec 14 2015 08:05:54
CurrentTime: Saturday, 23-Jan-2016 14:42:06 EET
RestartTime: Saturday, 23-Jan-2016 04:57:13 EET
ParentServerConfigGeneration: 2
ParentServerMPMGeneration: 1
ServerUptimeSeconds: 35092
ServerUptime: 9 hours 44 minutes 52 seconds
Load1: 0.32
Load5: 0.32
Load15: 0.27
Total Accesses: 32403
Total kBytes: 34464
CPUUser: 30.37
CPUSystem: 29.55
CPUChildrenUser: 0
CPUChildrenSystem: 0
CPULoad: .170751
Uptime: 35092
ReqPerSec: .923373
BytesPerSec: 1005.67
BytesPerReq: 1089.13
BusyWorkers: 1
IdleWorkers: 99
ConnsTotal: 0
ConnsAsyncWriting: 0
ConnsAsyncKeepAlive: 0
ConnsAsyncClosing: 0
Scoreboard: __________________________________________________________________________________________W_________............................................................................................................................................................................................................................................................................................................
```

From the apache status output it collects:

 - total accesses (incremental value, rendered as requests/s)
 - total bandwidth (incremental value, rendered as bandwidth/s)
 - requests per second (this appears to be calculated by apache as an average for its lifetime, while the one calculated by netdata using the total accesses counter is real-time)
 - bytes per second (average for the lifetime of the apache server)
 - bytes per request (average for the lifetime of the apache server)
 - workers by status (`busy` and `idle`)
 - total connections (currently active connections - offered by apache 2.4+)
 - async connections per status (`keepalive`, `writing`, `closing` - offered by apache 2.4+)

## Configuration

The configuration is stored in `/etc/netdata/charts.d/apache.conf`.
To edit this file on your system run `/etc/netdata/edit-config charts.d/apache.conf`.

The internal default is:

```sh
# the URL your apache server is responding with mod_status information.
apache_url="http://127.0.0.1:80/server-status?auto"

# use this to set custom curl options you may need
apache_curl_opts=

# set this to a NUMBER to overwrite the update frequency
# it is in seconds
apache_update_every=
```

The default `apache_update_every` is configured in netdata.

## Auto-detection

If you have configured your apache server to offer server-status information on localhost clients, the defaults should work fine.

## Apache Configuration

Apache configuration differs between distributions. Please check your distribution's documentation for information on enabling apache's `mod_status` module.

If you are able to run successfully, by hand this command:

```sh
curl "http://127.0.0.1:80/server-status?auto"
```

netdata will be able to do it too.

Notice: You may need to have the default `000-default.conf ` website enabled in order for the status mod to work.
