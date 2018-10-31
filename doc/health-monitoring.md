# Health Monitoring

> New to netdata? Check its demo: **[http://my-netdata.io/](http://my-netdata.io/)**
>
> [![User Base](https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_entries&dimensions=persons&label=user%20base&units=null&value_color=blue&precision=0&v41)](https://registry.my-netdata.io/#netdata_registry) [![Monitored Servers](https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_entries&dimensions=machines&label=servers%20monitored&units=null&value_color=orange&precision=0&v41)](https://registry.my-netdata.io/#netdata_registry) [![Sessions Served](https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_sessions&label=sessions%20served&units=null&value_color=yellowgreen&precision=0&v41)](https://registry.my-netdata.io/#netdata_registry)
> 
> [![New Users Today](http://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_entries&dimensions=persons&after=-86400&options=unaligned&group=incremental-sum&label=new%20users%20today&units=null&value_color=blue&precision=0&v40)](https://registry.my-netdata.io/#netdata_registry) [![New Machines Today](https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_entries&dimensions=machines&group=incremental-sum&after=-86400&options=unaligned&label=servers%20added%20today&units=null&value_color=orange&precision=0&v40)](https://registry.my-netdata.io/#netdata_registry) [![Sessions Today](https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_sessions&after=-86400&group=incremental-sum&options=unaligned&label=sessions%20served%20today&units=null&value_color=yellowgreen&precision=0&v40)](https://registry.my-netdata.io/#netdata_registry)

---

## table of contents

- [Overview](#netdata-got-health-monitoring)
- [Examples](https://github.com/netdata/netdata/wiki/health-configuration-examples)
- [Health Configuration](https://github.com/netdata/netdata/wiki/health-configuration-reference)
  - [Entities in the Health files](https://github.com/netdata/netdata/wiki/health-configuration-reference#entities-in-the-health-files)
  - [The Format](https://github.com/netdata/netdata/wiki/health-configuration-reference#the-format)
  - [Expressions](https://github.com/netdata/netdata/wiki/health-configuration-reference#expressions)
  - [Variables](https://github.com/netdata/netdata/wiki/health-configuration-reference#variables)
- [Alarm Actions](https://github.com/netdata/netdata/wiki/health-configuration-reference#alarm-actions)
  - [web browser notifications](https://github.com/netdata/netdata/wiki/web-browser-notifications)
  - [email messages](https://github.com/netdata/netdata/wiki/email-notifications)
  - [slack.com team collaboration](https://github.com/netdata/netdata/wiki/slack-notifications)
  - [Discord team collaboration](https://github.com/netdata/netdata/wiki/discord-notifications)
  - [pushover.net push notifications](https://github.com/netdata/netdata/wiki/pushover-notifications)
  - [pushbullet.com push notifications](https://github.com/netdata/netdata/wiki/pushbullet-notifications)
  - [telegram.org push messages](https://github.com/netdata/netdata/wiki/telegram-notifications)
  - [PagerDuty notifications](https://github.com/netdata/netdata/wiki/pagerduty-notifications)
  - [Twilio SMS notifications](https://github.com/netdata/netdata/wiki/twilio-notifications)
  - [Messagebird SMS notifications](https://github.com/netdata/netdata/wiki/messagebird-notifications)
- [Alarm Statuses](https://github.com/netdata/netdata/wiki/health-configuration-reference#alarm-statuses)
- [API Calls](https://github.com/netdata/netdata/wiki/health-API-calls)
- [Troubleshooting](https://github.com/netdata/netdata/wiki/troubleshooting-alarms)

---

# netdata got health monitoring!

Dear dev-ops and sys-admins, **netdata got alarms**!

A few months ago, when [I decided to let the netdata users decide the features they need us to develop](https://github.com/netdata/netdata/issues/436), I was somewhat surprised that most users wanted **[health monitoring](https://github.com/netdata/netdata/issues/436#issuecomment-220832546)**.

I think I get it now.

Health monitoring is problematic for most people. I have not seen a single sys-admin or dev-op totally happy with the tools he/she has.

So, I decided to build a health monitoring system in netdata that will overcome most of the problems other systems have:

Of course an alarm is just a threshold, like `A > 90`.

But netdata goes a lot beyond that...

netdata allows you to correlate different metrics. It is not just `A > 90`. It can be: `(A > 90 AND (B > 80 OR C < 40)) OR (D > 50 AND E < 30))`. This means for example that you can raise an alarm on the number of database requests only when the disk is congested, or when cpu utilization is too high too.

netdata allows you to take into account the values of the same metric **some time in the past**. So you can say: `A(now) > 90 AND A(30 mins ago) < 30`.

You can even calculate rates: `A(now) - A(30 mins ago) / (30 * 60)` = the rate A changes over the last 30 minutes. Then you can raise an alarm when: `A(rate of the last 30 mins) > 10`. This means you can detect if your web server is facing an abnormal request flood, or if your web server although operational is getting way too low requests.
  
You can calculate a percentage of change over the last period: `(A(now) - A(1 min ago)) * 100 / (A(1 min ago) - A(2 min ago))` = the percentage of the volume of the last minute compared to the volume of its previous minute. This means you can track your servers minute-by-minute and trigger alarms based on their changes.

netdata also allows you to evaluate simple expressions on any timeframe of a metric. For example, you can use in expressions the `min`, `max`, `average` and `sum` of any metric for any timeframe.

Then we come to configuration. All of you that use netdata already, know I hate configuration. I find absolutely no joy in configuring applications. Although netdata provides tons of configuration options, I always do my best so that most installations will need to configure nothing.

So, netdata comes with pre-defined alarms for detecting the most common problems. Out of the box:

- it will trigger alarms when the applications it monitors stops
- it will detect network interfaces errors
- it will detect disks not catching up with the load they are offered
- it will detect low disk space on any disk
- it will even **predict in how many hours your system is going to be out of disk space** and notify you if it is less than 48 hours.
- it will alert you if your system is running low on entropy (random numbers pool)

Even when you need to configure alarms by hand, netdata offers **Alarm Templates**. Once you have configured an alarm, netdata can apply this alarm on all similar charts/metrics. So, if for example, you build an alarm to detect a web server request flood, netdata can apply this alarm to all your web servers automatically.

netdata also offers **Context Based Variables**. When you configure an alarm to a chart, netdata automatically brings you as variables all the chart dimensions and all the dimensions and alarms of the charts that belong to the same family (e.g. family = `eth0` or `sda` or `mysql server 1`). This creates a context where similar things are available to be used with their "first" name.

netdata alarms are based on **expressions**. These expressions can use data from any chart, any dimension, any metric. If you want, you can correlate the backlog of the disk, to the number of database queries, to the packets rate of a network interface, to number of requests to a web server. This together with powerful **database lookup and reduce functions** allow you to create alarms for everything imaginable.
