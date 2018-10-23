$ PushBullet notifications

Will look like this on your browser:
![image](https://cloud.githubusercontent.com/assets/4300670/19109636/278b1c0c-8aee-11e6-8a09-7fc94fdbfec8.png)

And like this on your Android device:


![image](https://cloud.githubusercontent.com/assets/4300670/19109635/278a1dde-8aee-11e6-9984-0bc87a13312d.png)

You will need:

1. Signup and Login to pushbullet.com
2. Get your Access Token, go to https://www.pushbullet.com/#settings/account and create a new one
3. Fill in the PUSHBULLET_ACCESS_TOKEN with that value
4. Add the recipient emails to DEFAULT_RECIPIENT_PUSHBULLET
!!PLEASE NOTE THAT IF THE RECIPIENT DOES NOT HAVE A PUSHBULLET ACCOUNT, PUSHBULLET SERVICE WILL SEND AN EMAIL!!

Set them in `/etc/netdata/health_alarm_notify.conf` (to edit it on your system run `/etc/netdata/edit-config health_alarm_notify.conf`), like this:

```
###############################################################################
# pushbullet (pushbullet.com) push notification options

# multiple recipients can be given like this:
#                  "user1@email.com user2@mail.com"

# enable/disable sending pushbullet notifications
SEND_PUSHBULLET="YES"

# Signup and Login to pushbullet.com
# To get your Access Token, go to https://www.pushbullet.com/#settings/account
# And create a new access token
# Then just set the recipients emails
# Please note that the if the email in the DEFAULT_RECIPIENT_PUSHBULLET does
# not have a pushbullet account, the pushbullet service will send an email
# to that address instead

# Without an access token, netdata cannot send pushbullet notifications.
PUSHBULLET_ACCESS_TOKEN="o.Sometokenhere"
DEFAULT_RECIPIENT_PUSHBULLET="admin1@example.com admin3@somemail.com"
```