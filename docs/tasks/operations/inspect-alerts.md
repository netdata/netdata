<!--
title: "Inspect alerts"
sidebar_label: "Inspect alerts"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/tasks/operations/inspect-alerts.md"
sidebar_position: "1"
learn_status: "Unpublished"
learn_topic_type: "Tasks"
learn_rel_path: "Operations"
learn_docs_purpose: "Instructions on how the user can see their active alerts"
-->

From the Cloud interface, you can see the active Alerts of a War Room. In this Task you will learn how to inspect a War
Room's Alerts, and how to further filter the results to your liking.

#### Prerequisites

To inspect your alerts, you will need the following:

- A Cloud account with at least one node connected to one of its Spaces.

#### Steps

1. Click on the **Alerts** view
2. Click on the **Active** tab
3. You will be presented with a table of the active alerts (if any), containing info about the:
    - Alert Status
    - Alert name
    - Latest Value
    - Latest Updated timestamp
    - Triggered Value
    - Triggered Node
    - Chart Id
4. Inspect the alerts by sorting the results by these columns, or filtering from the right tab in the interface, using the following filters.
    - Status (Critical/Warning)
    - Class (Errors, Latency, Utilization, Workload)
    - Type & Component
    - Role
    - OS
    - Node (Select any node from the War Room)

#### Related topics

- [Alerts](https://github.com/netdata/netdata/blob/master/docs/concepts/health-monitoring/alerts.md)
- [Alerts Configuration](https://github.com/netdata/netdata/blob/master/health/README.md)
