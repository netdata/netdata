# Twilio

Will look like this on your Android device:

![image](https://cloud.githubusercontent.com/assets/17090999/20034652/620b6100-a39b-11e6-96af-4f83b8e830e2.png)

You will need:

1. Signup and Login to twilio.com
2. Pick an SMS capable number during sign up.
3. Get your SID, and Token from <https://www.twilio.com/console>
3. Fill in TWILIO_ACCOUNT_SID="XXXXXXXX" TWILIO_ACCOUNT_TOKEN="XXXXXXXXX" TWILIO_NUMBER="+XXXXXXXXXXX"
4. Add the recipient phone numbers to DEFAULT_RECIPIENT_TWILIO="+XXXXXXXXXXX"

!!PLEASE NOTE THAT IF YOUR ACCOUNT IS A TRIAL ACCOUNT YOU WILL ONLY BE ABLE TO SEND NOTIFICATIONS TO THE NUMBER YOU SIGNED UP WITH

Set them in `/etc/netdata/health_alarm_notify.conf` (to edit it on your system run `/etc/netdata/edit-config health_alarm_notify.conf`), like this:

```
###############################################################################
# Twilio (twilio.com) SMS options

# multiple recipients can be given like this:
#                  "+15555555555 +17777777777"

# enable/disable sending twilio SMS
SEND_TWILIO="YES"

# Signup for free trial and select a SMS capable Twilio Number
# To get your Account SID and Token, go to https://www.twilio.com/console
# Place your sid, token and number below.
# Then just set the recipients' phone numbers.
# The trial account is only allowed to use the number specified when set up.

# Without an account sid and token, netdata cannot send Twilio text messages.
TWILIO_ACCOUNT_SID="xxxxxxxxx"
TWILIO_ACCOUNT_TOKEN="xxxxxxxxxx"
TWILIO_NUMBER="xxxxxxxxxxx"
DEFAULT_RECIPIENT_TWILIO="+15555555555"
```
