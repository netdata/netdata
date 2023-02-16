<!--
title: "Send alert notifications to PagerDuty"
description: "Send alerts to your PagerDuty dashboard any time an anomaly or performance issue strikes a node in your infrastructure."
sidebar_label: "PagerDuty"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/health/notifications/pagerduty/README.md"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Integrations/Notify/Agent alert notifications"
learn_autogeneration_metadata: "{'part_of_cloud': False, 'part_of_agent': True}"
-->

# Send alert notifications to PagerDuty

[PagerDuty](https://www.pagerduty.com/company/) is an enterprise incident resolution service that integrates with ITOps
and DevOps monitoring stacks to improve operational reliability and agility. From enriching and aggregating events to
correlating them into incidents, PagerDuty streamlines the incident management process by reducing alert noise and
resolution times.

## What you need to get started

- An installation of the open-source [Netdata](https://github.com/netdata/netdata/blob/master/packaging/installer/README.md) monitoring agent.
- An installation of the [PagerDuty agent](https://www.pagerduty.com/docs/guides/agent-install-guide/) on the node
  running Netdata.
- A PagerDuty `Generic API` service using either the `Events API v2` or `Events API v1`.

## Setup

[Add a new service](https://support.pagerduty.com/docs/services-and-integrations#section-configuring-services-and-integrations)
to PagerDuty. Click **Use our API directly** and select either `Events API v2` or `Events API v1`. Once you finish
creating the service, click on the **Integrations** tab to find your **Integration Key**.

Navigate to the [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory) and use
[`edit-config`](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) to open
`health_alarm_notify.conf`.

```bash
cd /etc/netdata
sudo ./edit-config health_alarm_notify.conf
```

Scroll down to the `# pagerduty.com notification options` section.

Ensure `SEND_PD` is set to `YES`, then copy your Integration Key into `DEFAULT_RECIPIENT_ID`. Change `USE_PD_VERSION` to
`2` if you chose `Events API v2` during service setup on PagerDuty. Minus comments, the section should look like this:

```conf
SEND_PD="YES"
DEFAULT_RECIPIENT_PD="xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
USE_PD_VERSION="2"
```

## Testing

To test alert notifications to PagerDuty, run the following:

```bash
sudo su -s /bin/bash netdata
/usr/libexec/netdata/plugins.d/alarm-notify.sh test
```

## Configuration

Aside from the three values set in `health_alarm_notify.conf`, there is no further configuration required to send alert
notifications to PagerDuty.

To configure individual alarms, read our [alert configuration](https://github.com/netdata/netdata/blob/master/docs/monitor/configure-alarms.md) doc or
the [health entity reference](https://github.com/netdata/netdata/blob/master/health/REFERENCE.md) doc.
