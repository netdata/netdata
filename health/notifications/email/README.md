<!--
title: "Email"
custom_edit_url: https://github.com/netdata/netdata/edit/master/health/notifications/email/README.md
-->

# Email

You need a working `sendmail` command for email alerts to work. Almost all MTAs provide a `sendmail` interface.

Netdata sends all emails as user `netdata`, so make sure your `sendmail` works for local users.

email notifications look like this:

![image](https://user-images.githubusercontent.com/1905463/133216974-a2ca0e4f-787b-4dce-b1b2-9996a8c5f718.png)

## Configuration

To edit `health_alarm_notify.conf` on your system run `/etc/netdata/edit-config health_alarm_notify.conf`.

You can configure recipients in [`/etc/netdata/health_alarm_notify.conf`](https://github.com/netdata/netdata/blob/99d44b7d0c4e006b11318a28ba4a7e7d3f9b3bae/conf.d/health_alarm_notify.conf#L101).

You can also configure per role recipients [in the same file, a few lines below](https://github.com/netdata/netdata/blob/99d44b7d0c4e006b11318a28ba4a7e7d3f9b3bae/conf.d/health_alarm_notify.conf#L313).

Changes to this file do not require a Netdata restart.

You can test your configuration by issuing the commands:

```sh
# become user netdata
sudo su -s /bin/bash netdata

# send a test alarm
/usr/libexec/netdata/plugins.d/alarm-notify.sh test [ROLE]
```

Where `[ROLE]` is the role you want to test. The default (if you don't give a `[ROLE]`) is `sysadmin`.

Note that in versions before 1.16, the plugins.d directory may be installed in a different location in certain OSs (e.g. under `/usr/lib/netdata`). 
You can always find the location of the alarm-notify.sh script in `netdata.conf`.

## Filtering

Every notification email (both the plain text and the rich html versions) from the Netdata agent, contain a set of custom email headers that can be used for filtering using an email client. Example:

```
X-Netdata-Severity: warning
X-Netdata-Alert-Name: inbound_packets_dropped_ratio
X-Netdata-Chart: net_packets.enp2s0
X-Netdata-Family: enp2s0
X-Netdata-Classification: System
X-Netdata-Host: winterland
X-Netdata-Role: sysadmin
```

## Simple SMTP transport configuration

If you want an alternative to `sendmail` in order to have a simple MTA configuration for sending emails and auth to an existing SMTP server, you can do the following:

- Install `msmtp`.
- Modify the `sendmail` path in `health_alarm_notify.conf` to point to the location of `msmtp`:
```
# The full path to the sendmail command.
# If empty, the system $PATH will be searched for it.
# If not found, email notifications will be disabled (silently).
sendmail="/usr/bin/msmtp"
```
- Login as netdata :
```sh
(sudo) su -s /bin/bash netdata
```
- Configure `~/.msmtprc` as shown [in the documentation](https://marlam.de/msmtp/documentation/).
- Finally set the appropriate permissions on the `.msmtprc` file :
```sh
chmod 600 ~/.msmtprc
```

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fhealth%2Fnotifications%2Femail%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
