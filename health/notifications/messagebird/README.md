# Messagebird

The messagebird notifications will look like this on your Android device:

![image](https://cloud.githubusercontent.com/assets/17090999/20034652/620b6100-a39b-11e6-96af-4f83b8e830e2.png)

You will need:

1. Signup and Login to messagebird.com
2. Pick an SMS capable number after sign up to get some free credits
3. Go to <https://www.messagebird.com/app/settings/developers/access>
4. Create a new access key under 'API ACCESS (REST)' (you will want a live key)
3. Fill in MESSAGEBIRD_ACCESS_KEY="XXXXXXXX" MESSAGEBIRD_NUMBER="+XXXXXXXXXXX"
4. Add the recipient phone numbers to DEFAULT_RECIPIENT_MESSAGEBIRD="+XXXXXXXXXXX"

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

# Without an access key, netdata cannot send Messagebird text messages.
MESSAGEBIRD_ACCESS_KEY="XXXXXXXX"
MESSAGEBIRD_NUMBER="XXXXXXX"
DEFAULT_RECIPIENT_MESSAGEBIRD="XXXXXXX"

```

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fhealth%2Fnotifications%2Fmessagebird%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
