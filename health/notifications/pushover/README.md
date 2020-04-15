<!--
---
title: "PushOver"
custom_edit_url: https://github.com/netdata/netdata/edit/master/health/notifications/pushover/README.md
---
-->

# PushOver

pushover.net allows you to receive push notifications on your mobile phone. The service seems free for up to 7.500 messages per month.

Netdata will send warning messages with priority `0` and critical messages with priority `1`. pushover.net allows you to select do-not-disturb hours. The way this is configured, critical notifications will ring and vibrate your phone, even during the do-not-disturb-hours. All other notifications will be delivered silently.

You need:

1.  APP TOKEN. You can use the same on all your Netdata servers.
2.  USER TOKEN for each user you are going to send notifications to. This is the actual recipient of the notification.

The configuration is like above (slack messages).

pushover.net notifications look like this:

![image](https://cloud.githubusercontent.com/assets/2662304/18407319/839c10c4-7715-11e6-92c0-12f8215128d3.png)

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fhealth%2Fnotifications%2Fpushover%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
