<!--
title: "View active health alerts"
description: "View active alerts and their rich data to discover and resolve anomalies and performance issues across your infrastructure."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/monitor/view-active-alerts.md
-->

# View active health alerts

Every Netdata Agent comes with hundreds of pre-installed health alerts designed to notify you when an anomaly or
performance issue affects your node or the applications it runs.

## Netdata Cloud

A War Room's [alerts indicator](https://learn.netdata.cloud/docs/cloud/war-rooms#indicators) displays the number of
active `critical` (red) and `warning` (yellow) alerts for the nodes in this War Room. Click on either the critical or
warning badges to open a pre-filtered modal displaying only those types of [active
alerts](https://learn.netdata.cloud/docs/cloud/alerts-notifications/view-active-alerts).

![The alerts panel in Netdata
Cloud](https://user-images.githubusercontent.com/1153921/108564747-d2bfbb00-72c0-11eb-97b9-5863ad3324eb.png)

The Alerts panel lists all active alerts for nodes within that War Room, and tells you which chart triggered the alert,
what that chart's current value is, the alert that triggered it, and when the alert status first began.

Use the input field in the Alerts panel to filter active alerts. You can sort by the node's name, alert, status, chart
that triggered the alert, or the operating system. Read more about the [filtering
syntax](https://learn.netdata.cloud/docs/cloud/war-rooms#node-filter) to build valuable filters for your infrastructure.

Click on the 3-dot icon (`⋮`) to view active alert information or navigate directly to the offending chart in that
node's Cloud dashboard with the **Go to chart** button.

The active alert information gives you details about the alert that's been triggered. You can see the alert's
configuration, how it calculates warning or critical alerts, and which configuration file you could edit on that node if
you want to tweak or disable the alert to better suit your needs.

![Active alert details in Netdata
Cloud](https://user-images.githubusercontent.com/1153921/108564813-f08d2000-72c0-11eb-80c8-b2af22a751fd.png)

## Local Netdata Agent dashboard

Find the alerts icon ![Alerts
icon](https://raw.githubusercontent.com/netdata/netdata-ui/98e31799c1ec0983f433537ff16d2ac2b0d994aa/src/components/icon/assets/alarm.svg)
in the top navigation to bring up a modal that shows currently raised alerts, all running alerts, and the alerts log.
Here is an example of a raised `system.cpu` alert, followed by the full list and alert log:

![Animated GIF of looking at raised alerts and the alert
log](https://user-images.githubusercontent.com/1153921/80842482-8c289500-8bb6-11ea-9791-600cfdbe82ce.gif)

And a static screenshot of the raised CPU alert: 

![Screenshot of a raised system CPU
alert](https://user-images.githubusercontent.com/1153921/80842330-2dfbb200-8bb6-11ea-8147-3cd366eb0f37.png)

The alert itself is named **system - cpu**, and its context is `system.cpu`. Beneath that is an auto-updating badge that
shows the latest value of the chart that triggered the alert.

With the three icons beneath that and the **role** designation, you can:

1.  Scroll to the chart associated with this raised alert.
2.  Copy a link to the badge to your clipboard.
3.  Copy the code to embed the badge onto another web page using an `<embed>` element.

The table on the right-hand side displays information about the health entity that triggered the alert, which you can
use as a reference to [configure alerts](/docs/monitor/configure-alarms.md).

## What's next?

With the information that appears on Netdata Cloud and the local dashboard about active alerts, you can [configure
alerts](/docs/monitor/configure-alarms.md) to match your infrastructure's needs or your team's goals.

If you're happy with the pre-configured alerts, skip ahead to [enable
notifications](/docs/monitor/enable-notifications.md) to use Netdata Cloud's centralized alert notifications and/or
per-node notifications to endpoints like Slack, PagerDuty, Twilio, and more.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fmonitor%2Fview-active-alarms&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
