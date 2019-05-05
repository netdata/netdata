# SMS Server Tools 3

The [SMS Server Tools 3](http://smstools3.kekekasvi.com/) is a SMS Gateway software which can send and receive short messages through GSM modems and mobile phones.

To have netdata send notifications via SMS Server Tools 3, you'll first need to [install](http://smstools3.kekekasvi.com/index.php?p=compiling) and [configure](http://smstools3.kekekasvi.com/index.php?p=configure) smsd.

Ensure that the user `netdata` can execute `sendsms`. Any user executing `sendsms` needs to:

* Have write permissions to `/tmp` and `/var/spool/sms/outgoing`
* Be a member of group `smsd`

To ensure that the steps above are successful, just `su netdata` and execute `sendsms phone message`.

You then just need to configure the recipient phone numbers in `health_alarm_notify.conf`:

```sh
#------------------------------------------------------------------------------
# SMS Server Tools 3 (smstools3) global notification options

# enable/disable sending SMS Server Tools 3 SMS notifications
SEND_SMS="YES"

# if a role's recipients are not configured, a notification will be sent to
# this SMS channel (empty = do not send a notification for unconfigured
# roles). Multiple recipients can be given like this: "PHONE1 PHONE2 ..."

DEFAULT_RECIPIENT_SMS=""
```

Netdata uses the script `sendsms` that is installed by `smstools3` and just passes a phone number and a message to it. If `sendsms` is not in `$PATH`, you can pass its location in `health_alarm_notify.conf`:

```sh
# The full path of the sendsms command (smstools3).
# If empty, the system $PATH will be searched for it.
# If not found, SMS notifications will be silently disabled.
sendsms=""
```

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fhealth%2Fnotifications%2Fsmstools3%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
