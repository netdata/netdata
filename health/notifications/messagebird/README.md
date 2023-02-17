<!--
title: "Messagebird agent alert notifications"
sidebar_label: "Messagebird"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/health/notifications/messagebird/README.md"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Integrations/Notify/Agent alert notifications"
-->

# Messagebird agent alert notifications

The messagebird notifications will look like this on your Android device:

![image](https://cloud.githubusercontent.com/assets/17090999/20034652/620b6100-a39b-11e6-96af-4f83b8e830e2.png)

You will need:

1.  Signup and Login to messagebird.com
2.  Pick an SMS capable number after sign up to get some free credits
3.  Go to <https://www.messagebird.com/app/settings/developers/access>
4.  Create a new access key under 'API ACCESS (REST)' (you will want a live key)
5.  Fill in MESSAGEBIRD_ACCESS_KEY="XXXXXXXX" MESSAGEBIRD_NUMBER="+XXXXXXXXXXX"
6.  Add the recipient phone numbers to DEFAULT_RECIPIENT_MESSAGEBIRD="+XXXXXXXXXXX"

Set them in `/etc/netdata/health_alarm_notify.conf` (to edit it on your system run `/etc/netdata/edit-config health_alarm_notify.conf`), like this:

```
#------------------------------------------------------------------------------
# Messagebird (messagebird.com) SMS options

# multiple recipients can be given like this:
#                  "+15555555555 +17777777777"

# enable/disable sending messagebird SMS
SEND_MESSAGEBIRD="YES"

# to get an access key, create a free account at https://www.messagebird.com
# verify and activate the account (no CC info needed)
# login to your account and enter your phonenumber to get some free credits
# to get the API key, click on 'API' in the sidebar, then 'API Access (REST)' 
# click 'Add access key' and fill in data (you want a live key to send SMS)

# Without an access key, Netdata cannot send Messagebird text messages.
MESSAGEBIRD_ACCESS_KEY="XXXXXXXX"
MESSAGEBIRD_NUMBER="XXXXXXX"
DEFAULT_RECIPIENT_MESSAGEBIRD="XXXXXXX"
```


