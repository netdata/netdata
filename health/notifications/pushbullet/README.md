<!--
title: "PushBullet"
custom_edit_url: https://github.com/netdata/netdata/edit/master/health/notifications/pushbullet/README.md
-->

# PushBullet

Will look like this on your browser:
![image](https://cloud.githubusercontent.com/assets/4300670/19109636/278b1c0c-8aee-11e6-8a09-7fc94fdbfec8.png)

And like this on your Android device:

![image](https://cloud.githubusercontent.com/assets/4300670/19109635/278a1dde-8aee-11e6-9984-0bc87a13312d.png)

You will need:

1.  Sign up and log in to [pushbullet.com](https://www.pushbullet.com/)
2.  Create a new access token in your [account settings](https://www.pushbullet.com/#settings/account).
3.  Fill in the `PUSHBULLET_ACCESS_TOKEN` with the newly generated access token.
4.  Add the recipient emails or channel tags (each channel tag must be prefixed with #, e.g. #channeltag) to `DEFAULT_RECIPIENT_PUSHBULLET`.
    > 🚨 The pushbullet notification service will send emails to the email recipient, regardless of if they have a pushbullet account.

To add notification channels, run `/etc/netdata/edit-config health_alarm_notify.conf` 

You can change the configuration like this:

```
###############################################################################
# pushbullet (pushbullet.com) push notification options

# multiple recipients (a combination of email addresses or channel tags) can be given like this:
#                  "user1@email.com user2@mail.com #channel1 #channel2"

# enable/disable sending pushbullet notifications
SEND_PUSHBULLET="YES"

# Signup and Login to pushbullet.com
# To get your Access Token, go to https://www.pushbullet.com/#settings/account
# And create a new access token
# Then just set the recipients emails and/or channel tags (channel tags must be prefixed with #)
# Please note that the if an email in the DEFAULT_RECIPIENT_PUSHBULLET does
# not have a pushbullet account, the pushbullet service will send an email
# to that address instead

# Without an access token, Netdata cannot send pushbullet notifications.
PUSHBULLET_ACCESS_TOKEN="o.Sometokenhere"
DEFAULT_RECIPIENT_PUSHBULLET="admin1@example.com admin3@somemail.com #examplechanneltag #anotherchanneltag"
```


