<!--
title: "Health monitoring"
custom_edit_url: https://github.com/netdata/netdata/edit/master/health/README.md
-->

# Health monitoring

The Netdata Agent is a health watchdog for the health and performance of your systems, services, and applications. We've
worked closely with our community of DevOps engineers, SREs, and developers to define hundreds of production-ready
alarms that work without any configuration.

The Agent's health monitoring system is also dynamic and fully customizable. You can write entirely new alarms, tune the
community-configured alarms for every app/service [the Agent collects metrics from](/collectors/COLLECTORS.md), or
silence anything you're not interested in. You can even power complex lookups by running statistical algorithms against
your metrics.

Ready to take the next steps with health monitoring?

[Quickstart](/health/QUICKSTART.md)

[Configuration reference](/health/REFERENCE.md)

## Guides

Every infrastructure is different, so we're not interested in mandating how you should configure Netdata's health
monitoring features. Instead, these guides should give you the details you need to tweak alarms to your heart's
content.

[Stopping notifications for individual alarms](/docs/guides/monitor/stop-notifications-alarms.md)

[Use dimension templates to create dynamic alarms](/docs/guides/monitor/dimension-templates.md)

## Related features

**[Notifications](/health/notifications/README.md)**: Get notified about ongoing alarms from your Agents via your
favorite platform(s), such as Slack, Discord, PagerDuty, email, and much more.


