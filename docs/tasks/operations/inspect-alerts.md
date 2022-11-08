<!--
title: "Inspect alerts"
sidebar_label: "Inspect alerts"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/tasks/alerting/inspect-alerts.md"
sidebar_position: "1"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Operations"
learn_docs_purpose: "Instructions on how the user can see their active alerts"
-->

From the Cloud interface, you can see the active Alerts of a War Room. In this Task you will learn how to inspect a War
Room's Alerts, and how to further filter the results to your liking.

## Prerequisites

- A Cloud account with at least one node connected to one of its Spaces.

## Steps

1. Click on the **Alerts** view
2. Click on the **Active** tab

You will then be presented with a table of the active alerts (if any), containing info about the:

- Alert Status
- Alert name
- Latest Value
- Latest Updated timestamp
- Triggered Value
- Triggered Node
- Chart Id

You can sort the results by these columns, and you can also filter the Alerts, from the right tab in the interface.  
You can filter by:

- Status (Critical/Warning)
- Class (Errors, Latency, Utilization, Workload)
- Type & Component
- Role
- OS
- Node (Select any node from the War Room)

## Related topics

1. [Alerts Concept](https://github.com/netdata/netdata/blob/master/docs/concepts/health-monitoring/alerts.md)
2. [Alerts Configuration Reference](https://github.com/netdata/netdata/blob/master/health/README.md)