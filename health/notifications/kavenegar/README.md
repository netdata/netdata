# Kavenegar

[Kavenegar](https://www.kavenegar.com/) as service for software developers, based in Iran, provides send and receive SMS, calling voice by using its APIs.

Will look like this on your Android device:

![image](https://cloud.githubusercontent.com/assets/17090999/20034652/620b6100-a39b-11e6-96af-4f83b8e830e2.png)

You will need:

1. Signup and Login to kavenegar.com
2. Get your APIKEY and Sender from http://panel.kavenegar.com/client/setting/account
3. Fill in KAVENEGAR_API_KEY="" KAVENEGAR_SENDER=""
4. Add the recipient phone numbers to DEFAULT_RECIPIENT_KAVENEGAR=""

Set them in `/etc/netdata/health_alarm_notify.conf` (to edit it on your system run `/etc/netdata/edit-config health_alarm_notify.conf`), like this:

```
###############################################################################
# Kavenegar (kavenegar.com) SMS options

# multiple recipients can be given like this:
#                 "09155555555 09177777777"

# enable/disable sending kavenegar SMS
SEND_KAVENEGAR="YES"

# to get an access key, after selecting and purchasing your desired service
# at http://kavenegar.com/pricing.html
# login to your account, go to your dashboard and my account are
# https://panel.kavenegar.com/Client/setting/account from API Key
# copy your api key. You can generate new API Key too.
# You can find and select kevenegar sender number from this place.

# Without an API key, netdata cannot send KAVENEGAR text messages.
KAVENEGAR_API_KEY=""
KAVENEGAR_SENDER=""
DEFAULT_RECIPIENT_KAVENEGAR=""
```
