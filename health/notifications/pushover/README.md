# PushOver

pushover.net allows you to receive push notifications on your mobile phone. The service seems free for up to 7.500 messages per month.

netdata will send warning messages with priority `0` and critical messages with priority `1`. pushover.net allows you to select do-not-disturb hours. The way this is configured, critical notifications will ring and vibrate your phone, even during the do-not-disturb-hours. All other notifications will be delivered silently.

You need:

1. APP TOKEN. You can use the same on all your netdata servers.
2. USER TOKEN for each user you are going to send notifications to. This is the actual recipient of the notification.

The configuration is like above (slack messages).

pushover.net notifications look like this:

![image](https://cloud.githubusercontent.com/assets/2662304/18407319/839c10c4-7715-11e6-92c0-12f8215128d3.png)
