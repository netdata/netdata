# email

You need a working `sendmail` command for email alerts to work. Almost all MTAs provide a `sendmail` interface.

netdata sends all emails as user `netdata`, so make sure your `sendmail` works for local users.

email notifications look like this:

![image](https://cloud.githubusercontent.com/assets/2662304/18407294/e9218c68-7714-11e6-8739-e4dd8a498252.png)

## configuration

To edit `health_alarm_notify.conf` on your system run `/etc/netdata/edit-config health_alarm_notify.conf`.

You can configure recipients in [`/etc/netdata/health_alarm_notify.conf`](https://github.com/netdata/netdata/blob/99d44b7d0c4e006b11318a28ba4a7e7d3f9b3bae/conf.d/health_alarm_notify.conf#L101).

You can also configure per role recipients [in the same file, a few lines below](https://github.com/netdata/netdata/blob/99d44b7d0c4e006b11318a28ba4a7e7d3f9b3bae/conf.d/health_alarm_notify.conf#L313).

Changes to this file do not require netdata restart.

You can test your configuration by issuing the commands:

```sh
# become user netdata
sudo su -s /bin/bash netdata

# send a test alarm
/usr/libexec/netdata/plugins.d/alarm-notify.sh test [ROLE]
```

Where `[ROLE]` is the role you want to test. The default (if you don't give a `[ROLE]`) is `sysadmin`.
