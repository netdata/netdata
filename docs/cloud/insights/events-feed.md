<!--
title: "Events feed"
sidebar_label: "Events feed"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/cloud/events-feed.md"
sidebar_position: "2800"
learn_status: "Published"
learn_topic_type: "Concepts"
learn_rel_path: "Concepts"
learn_docs_purpose: "Present the Netdata Events feed."
-->

Netdata Cloud provides the Events feed which is a powerful feature that keeps track of events that happen both on our infrastructure and on your Space.
You will be able to use it to investigate events that occurred in that past and can be key on your troubleshooting task.

#### What are the available events?

At an high-level view, these are the domains from which the Events feed will provide visibility into.

> ⚠️ Based on your space's plan different allowances are defined to query past data.

| **Domains of events** | **Community** | **Pro** | **Business** |
| :-- | :-- | :-- | :-- |
| **Auditing events** - COMING SOON<p>Events related to actions done on your Space, e.g. invite user, change user role or change plan.</p>| 4 hours | 7 days | 90 days |
| **[Topology events](#topology-events)**<p>Node state transition events, e.g. live or offline.</p>| 4 hours | 7 days | 14 days |
| **[Alert events](#alert-events)**<p>Alert state transition events, can be seen as an alert history log.</p>| 4 hours | 7 days | 90 days |

##### Topology events

| **Event name** | **Description** |  **Example** |
| :-- | :-- | :-- |
| Node Become Live | The node is collecting and streaming metrics to Cloud.| TO-DO |
| Node Become Stale | The node is offline and not streaming metrics to Cloud. It can show historical data from a parent node. | TO-DO |
| Node Become Offline | The node is offline, not streaming metrics to Cloud and not available in any parent node.| TO-DO |
| Node Created | The node is created but it is still Unseen on Cloud, didn't establish a successful connection yet.| TO-DO |
| Node Removed | The node was removed from the Space through the `Delete` action, if it becomes Live again it will be automatically added. | TO-DO |
| Node Restored | The node is restored, if node become Live after a remove action it re-added to the Space. | TO-DO |
| Node Deleted | The node is deleted from the Space, see this as an hard delete and won't be re-added to the Space if it becomes live. | TO-DO |

TO-DO: Agent events?

##### Alert events

| **Event name** | **Description** |  **Example** |
| :-- | :-- | :-- |
| Node Alert State Changed | These are node alert state transition events and can be seen as an alert history log. You will be able to transitions to or from any of these states: Cleared, Warning, Critical, Removed, Error or Unknown | TO-DO |

#### Who can access the events?

All users will be able to see events from the Topology and Alerts domain but Auditing events, once these are added, only be accessible to administrators.

## Related Topics

### **Related Concepts**

### Related Tasks

- [How to use the Events feed?](TO-DO)
