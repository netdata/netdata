# Alerts and notifications

The Netdata Agent is a health watchdog for the health and performance of your systems, services, and applications. We've
worked closely with our community of DevOps engineers, SREs, and developers to define hundreds of production-ready
alarms that work without any configuration.

The Agent's health monitoring system is also dynamic and fully customizable. You can write entirely new alarms, tune the
community-configured alarms for every app/service [the Agent collects metrics from](https://github.com/netdata/netdata/blob/master/collectors/COLLECTORS.md), or
silence anything you're not interested in. You can even power complex lookups by running statistical algorithms against
your metrics.

You can [use various alert notification methods](https://github.com/netdata/netdata/edit/master/docs/monitor/enable-notifications.md), 
[customize alerts](https://github.com/netdata/netdata/blob/master/health/REFERENCE.md), and 
[disable/silence](https://github.com/netdata/netdata/blob/master/health/REFERENCE.md#disable-or-silence-alerts) alerts.
