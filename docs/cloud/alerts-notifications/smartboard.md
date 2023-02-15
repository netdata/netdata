<!--
title: "Alerts smartboard"
description: ""
type: "reference"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/smartboard.md"
sidebar_label: "Alerts smartboard"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Integrations/Notify/Cloud alert notifications"
-->

The Alerts view gives you a high level of availability and performance information for every node you're
monitoring with Netdata Cloud. We expect it to become the "home base" for many Netdata Cloud users who want to instantly
understand what's going on with their infrastructure and exactly where issues might be.

The Alerts view is available entirely for free to all users and for any number of nodes.

## Alerts table and filtering

The Alerts view shows all active alerts in your War Room, including the alert's name, the most recent value, a
timestamp of when it became active, and the relevant node.

You can use the checkboxes in the filter pane on the right side of the screen to filter the alerts displayed in the
table
by Status, Class, Type & Componenet, Role, Operating System, or Node.

Click on any of the alert names to see the alert.

## View active alerts

In the `Active` subtab, you can see exactly how many **critical** and **warning** alerts are active across your nodes.

## View configured alerts

You can view all the configured alerts on all the agents that belong to a War Room in the `Alert Configurations` subtab.
From within the Alerts view, you can click the `Alert Configurations` subtab to see a high level view of the states of
the alerts on the nodes within this War Room and drill down to the node level where each alert is configured with their
latest status.








