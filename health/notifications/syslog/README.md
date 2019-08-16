# Syslog

You need a working `logger` command for this to work.  This is the case on pretty much every Linux system in existence, and most BSD systems.

Logged messages will look like this:

```
netdata WARNING on hostname at Tue Apr 3 09:00:00 EDT 2018: disk_space._ out of disk space time = 5h
```

## configuration

System log targets are configured as recipients in [`/etc/netdata/health_alarm_notify.conf`](https://github.com/netdata/netdata/blob/36bedc044584dea791fd29455bdcd287c3306cb2/conf.d/health_alarm_notify.conf#L534) (to edit it on your system run `/etc/netdata/edit-config health_alarm_notify.conf`).

You can als configure per-role targets in the same file a bit further down.

Targets are defined as follows:

```
[[facility.level][@host[:port]]/]prefix
```

`prefix` defines what the log messages are prefixed with.  By default, all lines are prefixed with 'netdata'.

The `facility` and `level` are the standard syslog facility and level options, for more info on them see your local `logger` and `syslog` documentation.  By default, Netdata will log to the `local6` facility, with a log level dependent on the type of message (`crit` for CRITICAL, `warning` for WARNING, and `info` for everything else).

You can configure sending directly to remote log servers by specifying a host (and optionally a port).  However, this has a somewhat high overhead, so it is much preferred to use your local syslog daemon to handle the forwarding of messages to remote systems (pretty much all of them allow at least simple forwarding, and most of the really popular ones support complex queueing and routing of messages to remote log servers).

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fhealth%2Fnotifications%2Fsyslog%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
