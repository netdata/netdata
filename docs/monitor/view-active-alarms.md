<!--
title: "View active health alarms"
description: "View active alarms and their rich data to discover and resolve anomalies and performance issues across your infrastructure."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/monitor/view-active-alarms.md
-->

# View active health alarms

Every Netdata Agent comes with hundreds of pre-installed health alarms designed to notify you when an anomaly or
performance issue affects your node or the applications it runs.

## Netdata Cloud

A War Room's [alarms indicator](https://learn.netdata.cloud/docs/cloud/war-rooms#indicators) displays the number of
active `critical` (red) and `warning` (yellow) alerts for the nodes in this War Room. Click on either the critical or
warning badges to open a pre-filtered modal displaying only those types of [active
alarms](https://learn.netdata.cloud/docs/cloud/alerts-notifications/view-active-alerts).

![The Alarms panel in Netdata
Cloud](https://user-images.githubusercontent.com/1153921/108564747-d2bfbb00-72c0-11eb-97b9-5863ad3324eb.png)

The Alarms panel lists all active alarms for nodes within that War Room, and tells you which chart triggered the alarm,
what that chart's current value is, the alarm that triggered it, and when the alarm status first began.

Use the input field in the Alarms panel to filter active alarms. You can sort by the node's name, alarm, status, chart
that triggered the alarm, or the operating system. Read more about the [filtering
syntax](https://learn.netdata.cloud/docs/cloud/war-rooms#node-filter) to build valuable filters for your infrastructure.

Click on the 3-dot icon (`⋮`) to view active alarm information or navigate directly to the offending chart in that
node's Cloud dashboard with the **Go to chart** button.

The active alarm information gives you details about the alarm that's been triggered. You can see the alarm's
configuration, how it calculates warning or critical alarms, and which configuration file you could edit on that node if
you want to tweak or disable the alarm to better suit your needs.

![Active alarm details in Netdata
Cloud](https://user-images.githubusercontent.com/1153921/108564813-f08d2000-72c0-11eb-80c8-b2af22a751fd.png)

## Local Netdata Agent dashboard

Find the alarms icon ![Alarms
icon](https://raw.githubusercontent.com/netdata/netdata-ui/98e31799c1ec0983f433537ff16d2ac2b0d994aa/src/components/icon/assets/alarm.svg)
in the top navigation to bring up a modal that shows currently raised alarms, all running alarms, and the alarms log.
Here is an example of a raised `system.cpu` alarm, followed by the full list and alarm log:

![Animated GIF of looking at raised alarms and the alarm
log](https://user-images.githubusercontent.com/1153921/80842482-8c289500-8bb6-11ea-9791-600cfdbe82ce.gif)

And a static screenshot of the raised CPU alarm: 

![Screenshot of a raised system CPU
alarm](https://user-images.githubusercontent.com/1153921/80842330-2dfbb200-8bb6-11ea-8147-3cd366eb0f37.png)

The alarm itself is named **system - cpu**, and its context is `system.cpu`. Beneath that is an auto-updating badge that
shows the latest value of the chart that triggered the alarm.

With the three icons beneath that and the **role** designation, you can:

1.  Scroll to the chart associated with this raised alarm.
2.  Copy a link to the badge to your clipboard.
3.  Copy the code to embed the badge onto another web page using an `<embed>` element.

The table on the right-hand side displays information about the health entity that triggered the alarm, which you can
use as a reference to [configure alarms](/docs/monitor/configure-alarms.md).

## What's next?

With the information that appears on Netdata Cloud and the local dashboard about active alarms, you can [configure
alarms](/docs/monitor/configure-alarms.md) to match your infrastructure's needs or your team's goals.

If you're happy with the pre-configured alarms, skip ahead to [enable
notifications](/docs/monitor/enable-notifications.md) to use Netdata Cloud's centralized alarm notifications and/or
per-node notifications to endpoints like Slack, PagerDuty, Twilio, and more.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fmonitor%2Fview-active-alarms&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
