# Alerts and notifications

Netdata offers two ways to receive alert notifications on external integrations. These methods work independently, which means you can enable both at the same time to send alert notifications to any number of endpoints.

Both methods use a node's health alerts to generate the content of a notification. 

Read our documentation on [configuring alerts](https://github.com/netdata/netdata/blob/master/src/health/REFERENCE.md) to change the preconfigured thresholds or to create tailored alerts for your infrastructure.

- Netdata Cloud offers [centralized alert notifications](https://learn.netdata.cloud/docs/alerts-&-notifications/notifications/centralized-cloud-notifications) which leverages the health status information already streamed to Netdata Cloud from connected nodes to send notifications to the configured integrations. Supported integrations are Amazon SNS, Discord, Slack, Splunk and more.

- [The Netdata Agent can dispatch its own notifications](https://learn.netdata.cloud/docs/alerts-&-notifications/notifications/agent-dispatched-notifications) and supports more than a dozen services, such as email, Slack, PagerDuty, Twilio, Amazon SNS, Discord, and much more.

The Netdata Agent is a health watchdog for the health and performance of your systems, services, and applications. We've worked closely with our community of DevOps engineers, SREs, and developers to define hundreds of production-ready alerts that work without any configuration.

The Agent's health monitoring system is also dynamic and fully customizable. You can write entirely new alerts, tune the pre-configured alerts for every app/service [the Agent collects metrics from](https://github.com/netdata/netdata/blob/master/src/collectors/COLLECTORS.md), or silence anything you're not interested in. You can even power complex lookups by running statistical algorithms against your metrics.

You can [use various alert notification methods](https://github.com/netdata/netdata/blob/master/docs/monitor/enable-notifications.md), [customize alerts](https://github.com/netdata/netdata/blob/master/src/health/REFERENCE.md), and [disable/silence](https://github.com/netdata/netdata/blob/master/src/health/REFERENCE.md#disable-or-silence-alerts) alerts.
