<!--
---
title: "prowl"
custom_edit_url: https://github.com/netdata/netdata/edit/master/health/notifications/prowl/README.md
---
-->

# prowl

(Prowl)[1] is a push notification service for iOS devices.  Netdata
supprots delivering notifications to iOS devices through Prowl.

Because of how Netdata integrates with Prowl, there is a hard limit of
at most 1000 notifications per hour (starting from the first notification
sent).  Any alerts beyond the first thousand in an hour will be dropped.

Warning messages will be sent with the 'High' priority, critical messages
will be sent with the 'Emergency' priority, and all other messages will
be sent with the normal priority.  Opening the notification's associated
URL will take you to the Netdata dashboard of the system that issued
the alert, directly to the chart that it triggered on.

## configuration

To use this, you will need a Prowl API key, which can be rquested through
the Prowl website after registering.

Once you have an API key, simply specify that as a recipient for Prowl
notifications.
