---
title: "How to use the Events Feed"
sidebar_label: "How to use the Events Feed"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/cloud/insights/view-events-feed.md"
learn_status: "Published"
sidebar_position: "5"
learn_topic_type: "Tasks"
learn_rel_path: "Operations"
learn_docs_purpose: "Instructions on how to use Events Feed"
---

Netdata Cloud Events feed allows you to have insights into events that occurred on our infrastructure and on your Space.

### Prerequisites

To access the Events feed you need:
- A Netdata Cloud account

> ⚠️ Based on your space's plan different allowances are defined to query past data. See more details at [What are the available events?](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/events-feed.md#what-are-the-available-events)
> on the events feed documentation.

### Steps

1. Click on the **Events** tab (located near the top of your screen)
1. You will be presented with a table listing the events that occurred from the timeframe defined on the date time picker
1. You can use the filtering capabilities available on right-hand bar to slice through the results provided. See more details on event types and filters 

Note: When you try to query a longer period than what your space allows you will see an error message highlighting that you are querying data outside of your plan.

#### Event types and filters

| Event type | Tags | Nodes | Alert Status | Alert Names | Chart Names | 
| :-- | :-- | :-- | :-- | :-- | :-- |
| Node Become Live | node, lifecycle | Node name | - | - | - |
| Node Become Stale | node, lifecycle | Node name | - | - | - |
| Node Become Offline | node, lifecycle | Node name | - | - | - |
| Node Created | node, lifecycle | Node name | - | - | - |
| Node Removed | node, lifecycle | Node name | - | - | - |
| Node Restored | node, lifecycle | Node name | - | - | - |
| Node Deleted | node, lifecycle | Node name | - | - | - |
| Agent Claimed | agent | - | - | - | - | 
| Agent Connected | agent | - | - | - | - | 
| Agent Disconnected | agent | - | - | - | - | 
| Agent Authenticated | agent | - | - | - | - | 
| Agent Authentication Failed | agent | - | - | - | - | 
| Space Statistics | space, node, statistics | Node name | - | - | - |
| Node Alert State Changed | alert, node | Node name | Cleared, Warning, Critical, Removed, Error or Unknown | Alert name | Chart name |



## Related Topics

### **Related Concepts**
- [Events feed](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/events-feed.md)
