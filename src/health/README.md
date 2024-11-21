# Alerts and notifications

Netdata offers two ways to receive alert notifications on external integrations. These methods work independently, which means you can enable both at the same time to send alert notifications to any number of endpoints.

Both methods use a node's health alerts to generate the content of a notification.

Read our documentation on [configuring alerts](/src/health/REFERENCE.md) to change the pre-configured thresholds or to create tailored alerts for your infrastructure.

<!-- virtual links below, should not lead anywhere outside of the rendered Learn doc -->

- Netdata Cloud provides centralized alert notifications, utilizing the health status data already sent to Netdata Cloud from connected nodes to send alerts to configured integrations. [Supported integrations](/docs/alerts-&-notifications/notifications/centralized-cloud-notifications) include Amazon SNS, Discord, Slack, Splunk, and others.

- The Netdata Agent offers a [wider range of notification options](/docs/alerts-&-notifications/notifications/agent-dispatched-notifications) directly from the Agent itself. You can choose from over a dozen services, including email, Slack, PagerDuty, Twilio, and others, for more granular control over notifications on each node.

The Netdata Agent is a health watchdog for the health and performance of your systems, services, and applications. We've worked closely with our community of DevOps engineers, SREs, and developers to define hundreds of production-ready alerts that work without any configuration.

The Agent's health monitoring system is also dynamic and fully customizable. You can write entirely new alerts, tune the pre-configured alerts for every app/service [the Agent collects metrics from](/src/collectors/COLLECTORS.md), or silence anything you're not interested in. You can even power complex lookups by running statistical algorithms against your metrics.

You can [use various alert notification methods](/docs/alerts-and-notifications/notifications/README.md), [customize alerts](/src/health/REFERENCE.md), and [disable/silence](/src/health/REFERENCE.md#disable-or-silence-alerts) alerts.
