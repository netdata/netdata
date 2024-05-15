# Agent alert notifications

This is a reference documentation for Netdata's Agent alert notification feature, which supports dozens of endpoints, user roles, and more.

The `script to execute on alarm` line in `netdata.conf` defines the external script that will be called once the alert is triggered.

The default script is `alarm-notify.sh`.

> ### Info
>
> This file mentions editing configuration files.  
>
> - To edit configuration files in a safe way, we provide the [`edit config` script](https://github.com/netdata/netdata/blob/master/docs/netdata-agent/configuration/README.md#edit-netdataconf)located in your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/netdata-agent/configuration/README.md#the-netdata-config-directory) (typically is `/etc/netdata`) that creates the proper file and opens it in an editor automatically.  
> Note that to run the script you need to be inside your Netdata config directory.
>
> - Please also note that after most configuration changes you will need to [restart the Agent](https://github.com/netdata/netdata/blob/master/packaging/installer/README.md#maintaining-a-netdata-agent-installation) for the changes to take effect.
>
> It is recommended to use this way for configuring Netdata.

You can change the default script globally by editing `netdata.conf` and changing the `script to execute on alarm` in the `[health]` section.

`alarm-notify.sh` is capable of sending notifications:

- to multiple recipients
- using multiple notification methods
- filtering severity per recipient

It uses **roles**. For example `sysadmin`, `webmaster`, `dba`, etc.

Each alert is assigned to one or more roles, using the `to` line of the alert  configuration. For example, here is the alert configuration for `ram.conf` that defaults to the role `sysadmin`:

```conf
    alarm: ram_in_use
       on: system.ram
    class: Utilization
     type: System
component: Memory
       os: linux
    hosts: *
     calc: $used * 100 / ($used + $cached + $free + $buffers)
    units: %
    every: 10s
     warn: $this > (($status >= $WARNING)  ? (80) : (90))
     crit: $this > (($status == $CRITICAL) ? (90) : (98))
    delay: down 15m multiplier 1.5 max 1h
     info: system memory utilization
       to: sysadmin
```

Then `alarm-notify.sh` uses its own configuration file `health_alarm_notify.conf`, which at the bottom of the file stores the recipients per role, for all notification methods.

Here is an example, of the `sysadmin`'s role recipients for the email notification.  
You can send the notification to multiple recipients by separating the emails with a space.

```conf

###############################################################################
# RECIPIENTS PER ROLE

# -----------------------------------------------------------------------------
# generic system alerts
# CPU, disks, network interfaces, entropy, etc

role_recipients_email[sysadmin]="someone@exaple.com someoneelse@example.com"
```

Each role may have one or more destinations and one or more notification methods.

So, for example the `sysadmin` role may send:

1. emails to admin1@example.com and admin2@example.com
2. pushover.net notifications to USERTOKENS `A`, `B` and `C`.
3. pushbullet.com push notifications to admin1@example.com and admin2@example.com
4. messages to the `#alerts` and `#systems` channels of a Slack workspace.
5. messages to Discord channels `#alerts` and `#systems`.

## Configuration

You can edit `health_alarm_notify.conf` using the `edit-config` script to configure:

- **Settings** per notification method:

     All notification methods except email, require some configuration (i.e. API keys, tokens, destination rooms, channels, etc). Please check this section's content to find the configuration guides for your notification option of choice

- **Recipients** per role per notification method

     ```conf
     role_recipients_email[sysadmin]="${DEFAULT_RECIPIENT_EMAIL}"
     role_recipients_pushover[sysadmin]="${DEFAULT_RECIPIENT_PUSHOVER}"
     role_recipients_pushbullet[sysadmin]="${DEFAULT_RECIPIENT_PUSHBULLET}"
     role_recipients_telegram[sysadmin]="${DEFAULT_RECIPIENT_TELEGRAM}"
     role_recipients_slack[sysadmin]="${DEFAULT_RECIPIENT_SLACK}"
     ...
     ```

     Here you can change the `${DEFAULT_...}` values to the values of the recipients you want, separated by a space if you have multiple recipients.

## Testing Alert Notifications

You can run the following command by hand, to test alerts configuration:

```sh
# become user netdata
sudo su -s /bin/bash netdata

# enable debugging info on the console
export NETDATA_ALARM_NOTIFY_DEBUG=1

# send test alerts to sysadmin
/usr/libexec/netdata/plugins.d/alarm-notify.sh test

# send test alerts to any role
/usr/libexec/netdata/plugins.d/alarm-notify.sh test "ROLE"
```

If you are [running your own registry](https://github.com/netdata/netdata/blob/master/src/registry/README.md#run-your-own-registry), add `export NETDATA_REGISTRY_URL=[YOUR_URL]` before calling `alarm-notify.sh`.

> If you need to dig even deeper, you can trace the execution with `bash -x`. Note that in test mode, `alarm-notify.sh` calls itself with many more arguments. So first do:
>
>```sh
>bash -x /usr/libexec/netdata/plugins.d/alarm-notify.sh test
>```
>
> And then look in the output for the alarm-notify.sh calls and run the one you want to trace with `bash -x`.

## Global configuration options

### Notification Filtering

When you define recipients per role for notification methods, you can append `|critical` to limit the notifications that are sent.

In the following examples, the first recipient receives all the alerts, while the second one receives only notifications for alerts that have at some point become critical.
The second user may still receive warning and clear notifications, but only for the event that previously caused a critical alert.

```conf
 email      : "user1@example.com user2@example.com|critical"
 pushover   : "2987343...9437837 8756278...2362736|critical"
 telegram   : "111827421 112746832|critical"
 slack      : "alerts disasters|critical"
 alerta     : "alerts disasters|critical"
 flock      : "alerts disasters|critical"
 discord    : "alerts disasters|critical"
 twilio     : "+15555555555 +17777777777|critical"
 messagebird: "+15555555555 +17777777777|critical"
 kavenegar  : "09155555555 09177777777|critical"
 pd         : "<pd_service_key_1> <pd_service_key_2>|critical"
 irc        : "<irc_channel_1> <irc_channel_2>|critical"
```

If a per role recipient is set to an empty string, the default recipient of the given
notification method (email, pushover, telegram, slack, alerta, etc.) will be used.

To disable a notification, use the recipient called: disabled
This works for all notification methods (including the default recipients).

### Proxy configuration

If you need to send curl based notifications (pushover, pushbullet, slack, alerta,
flock, discord, telegram) via a proxy, you should set these variables to your proxy address:

```conf
export http_proxy="http://10.0.0.1:3128/"
export https_proxy="http://10.0.0.1:3128/"
```

### Notification images

Images in notifications need to be downloaded from an Internet facing site.

To allow notification providers to fetch the icons/images, by default we set the URL of the global public netdata registry.

If you have an Internet facing netdata (or you have copied the images/ folder
of netdata to your web server), set its URL here, to fetch the notification
images from it.

```conf
images_base_url="http://my.public.netdata.server:19999"
```

### Date handling

You can configure netdata alerts to send dates in any format you want via editing the `date_format` variable.

This uses standard `date` command format strings. See `man date` for
more info on what formats are supported.

Note that this has to start with a '+', otherwise it won't work.

- For ISO 8601 dates, use `+%FT%T%z`
- For RFC 5322 dates, use `+%a, %d %b %Y %H:%M:%S %z`
- For RFC 3339 dates, use `+%F %T%:z`
- For RFC 1123 dates, use `+%a, %d %b %Y %H:%M:%S %Z`
- For RFC 1036 dates, use `+%A, %d-%b-%y %H:%M:%S %Z`
- For a reasonably local date and time (in that order), use `+%x %X`
- For the old default behavior (compatible with ANSI C's `asctime()` function), leave the `date_format` field empty.

### Hostname handling

By default, Netdata will use the simple hostname for the system (the hostname with everything after the first `.` removed) when displaying the hostname in alert notifications.

If you instead prefer to have Netdata use the host's fully qualified domain name, you can set `use_fdqn` to `YES`.

This setting does not account for child systems for which the system you are configuring is a parent.

> ### Note
>
> If the system's host name is overridden in `/etc/netdata.conf` with the `hostname` option, that name will be used unconditionally.
