<!--
title: "Email agent alert notifications"
sidebar_label: "Email"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/health/notifications/email/README.md"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Integrations/Notify/Agent alert notifications"
learn_autogeneration_metadata: "{'part_of_cloud': True, 'part_of_agent': True}"
-->

# Email agent alert notifications

You need a working `sendmail` command for email alerts to work. Almost all MTAs provide a `sendmail` interface.

Netdata sends all emails as user `netdata`, so make sure your `sendmail` works for local users.

If you are using our Docker images, or are running Netdata on a system that does not have a working `sendmail`
command, see [the section below about using msmtp in place of sendmail](#using-msmtp-instead-of-sendmail).

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

## Using msmtp instead of sendmail

[msmtp](https://marlam.de/msmtp/) provides a simple alternative to a full-blown local mail server and `sendmail`
that will still allow you to send email notifications. It comes pre-installed in our Docker images, and is available
on most distributions in the system package repositories.

To use msmtp with Netdata for sending email alerts:

1. If it’s not already installed, install msmtp. Most distributions have it in their package repositories with the
   package name `msmtp`.
2. Modify the `sendmail` path in `health_alarm_notify.conf` to point to the location of `msmtp`:
```
# The full path to the sendmail command.
# If empty, the system $PATH will be searched for it.
# If not found, email notifications will be disabled (silently).
sendmail="/usr/bin/msmtp"
```
3. Login as netdata:
```sh
(sudo) su -s /bin/bash netdata
```
4. Configure `~/.msmtprc` as shown [in the documentation](https://marlam.de/msmtp/documentation/).
5. Finally set the appropriate permissions on the `.msmtprc` file :
```sh
chmod 600 ~/.msmtprc
```
