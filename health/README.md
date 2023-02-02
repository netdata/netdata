<!--
title: "Health monitoring"
custom_edit_url: https://github.com/netdata/netdata/edit/master/health/README.md
sidebar_label: "Health monitoring"
learn_status: "Published"
learn_topic_type: "Concepts"
learn_rel_path: "Concepts"
-->

# Health monitoring

The Netdata Agent is a health watchdog for the health and performance of your systems, services, and applications. We've
worked closely with our community of DevOps engineers, SREs, and developers to define hundreds of production-ready
alarms that work without any configuration.

The Agent's health monitoring system is also dynamic and fully customizable. You can write entirely new alarms, tune the
community-configured alarms for every app/service [the Agent collects metrics from](https://github.com/netdata/netdata/blob/master/collectors/COLLECTORS.md), or
silence anything you're not interested in. You can even power complex lookups by running statistical algorithms against
your metrics.

Ready to take the next steps with health monitoring?

[Configuration reference](https://github.com/netdata/netdata/blob/master/health/REFERENCE.md)

## Guides

Every infrastructure is different, so we're not interested in mandating how you should configure Netdata's health
monitoring features. Instead, these guides should give you the details you need to tweak alarms to your heart's
content.

[Stopping notifications for individual alarms](https://github.com/netdata/netdata/blob/master/docs/guides/monitor/stop-notifications-alarms.md)

[Use dimension templates to create dynamic alarms](https://github.com/netdata/netdata/blob/master/docs/guides/monitor/dimension-templates.md)

## Related features

**[Notifications](https://github.com/netdata/netdata/blob/master/health/notifications/README.md)**: Get notified about ongoing alarms from your Agents via your
favorite platform(s), such as Slack, Discord, PagerDuty, email, and much more.


