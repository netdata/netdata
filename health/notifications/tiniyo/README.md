# Tiniyo

Will look like this on your Android device:

![image](https://cloud.githubusercontent.com/assets/17090999/20034652/620b6100-a39b-11e6-96af-4f83b8e830e2.png)

You will need:

1.  Signup and Login to tiniyo.com
2.  Pick an SMS capable number during sign up.
3.  Get your AuthID, and AuthSecretID from <https://www.tiniyo.com/accounts/home>
4.  Fill in TINIYO_ACCOUNT_AUTHID="XXXXXXXX" TINIYO_ACCOUNT_AUTHSECRETID="XXXXXXXXX" TINIYO_NUMBER="+XXXXXXXXXXX"
5.  Add the recipient phone numbers to DEFAULT_RECIPIENT_TINIYO="+XXXXXXXXXXX"

!!PLEASE NOTE THAT IF YOUR ACCOUNT IS A DEVELOPER ACCOUNT YOU WILL ONLY BE ABLE TO SEND NOTIFICATIONS TO THE NUMBER YOU SIGNED UP WITH

Set them in `/etc/netdata/health_alarm_notify.conf` (to edit it on your system run `/etc/netdata/edit-config health_alarm_notify.conf`), like this:

```
###############################################################################
# Tiniyo (tiniyo.com) SMS options

# multiple recipients can be given like this:
#                  "+15555555555 +17777777777"

# enable/disable sending tiniyo SMS
SEND_TINIYO="YES"

# Signup for free trial and select a SMS capable Tiniyo Number
# To get your AuthID and AuthSecretID, go to https://www.tiniyo.com/accounts/home
# Place your AuthID, AuthSecretID and number below.
# Then just set the recipients' phone numbers.
# The trial account is only allowed to use the number specified when set up.

# Without an account sid and token, Netdata cannot send Tiniyo text messages.
TINIYO_ACCOUNT_SID="xxxxxxxxx"
TINIYO_ACCOUNT_TOKEN="xxxxxxxxxx"
TINIYO_NUMBER="xxxxxxxxxxx"
DEFAULT_RECIPIENT_TINIYO="+15555555555"
```
