# Notifications

Netdata offers two ways to receive alert notifications on external integrations. These methods work independently, which means you can enable both at the same time to send alert notifications to any number of endpoints.

Both methods use a node's health alerts to generate the content of a notification. 

Read our documentation on [configuring alerts](https://github.com/netdata/netdata/blob/master/src/health/REFERENCE.md) to change the preconfigured thresholds or to create tailored alerts for your infrastructure.

- Netdata Cloud offers [centralized alert notifications](https://learn.netdata.cloud/docs/alerts-&-notifications/notifications/centralized-cloud-notifications) which leverages the health status information already streamed to Netdata Cloud from connected nodes to send notifications to the configured integrations. Supported integrations are Amazon SNS, Discord, Slack, Splunk and more.

- [The Netdata Agent can dispatch its own notifications](https://learn.netdata.cloud/docs/alerts-&-notifications/notifications/agent-dispatched-notifications) and supports more than a dozen services, such as email, Slack, PagerDuty, Twilio, Amazon SNS, Discord, and much more.
