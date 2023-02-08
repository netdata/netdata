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

Netdata Cloud provides the Events feed which is a powerful feature that tracks events that happen on your infrastructure, or in your Space. The feed lets you investigate events that occurred in the past, which is obviously invaluable for troubleshooting. Common use cases are ones like when a node goes offline, and you want to understand what events happened before that. A detailed event history can also assist in attributing sudden pattern changes in a time series to specific changes in your environment.

#### What are the available events?

At a high-level view, these are the domains from which the Events feed will provide visibility into.

> ⚠️ Based on your space's plan different allowances are defined to query past data.

| **Domains of events** | **Community** | **Pro** | **Business** |
| :-- | :-- | :-- | :-- |
| **Auditing events** - COMING SOON<p>Events related to actions done on your Space, e.g. invite user, change user role or change plan.</p>| 4 hours | 7 days | 90 days |
| **[Topology events](#topology-events)**<p>Node state transition events, e.g. live or offline.</p>| 4 hours | 7 days | 14 days |
| **[Alert events](#alert-events)**<p>Alert state transition events, can be seen as an alert history log.</p>| 4 hours | 7 days | 90 days |

##### Topology events

| **Event name** | **Description** |  **Example** |
| :-- | :-- | :-- |
| Node Become Live | The node is collecting and streaming metrics to Cloud.| Node `netdata-k8s-state-xyz` is on live **state** |
| Node Become Stale | The node is offline and not streaming metrics to Cloud. It can show historical data from a parent node. | Node `ip-xyz.ec2.internal` is on **stale** state |
| Node Become Offline | The node is offline, not streaming metrics to Cloud and not available in any parent node.| Node `ip-xyz.ec2.internal` is on **offline** state |
| Node Created | The node is created but it is still Unseen on Cloud, didn't establish a successful connection yet.| Node `ip-xyz.ec2.internal` is on **unseen** state |
| Node Removed | The node was removed from the Space through the `Delete` action, if it becomes Live again it will be automatically added. | Node `ip-xyz.ec2.internal` was **deleted(soft)** |
| Node Restored | The node is restored, if node becomes Live after a remove action it is re-added to the Space. | Node `ip-xyz.ec2.internal` was **restored** |
| Node Deleted | The node is deleted from the Space, see this as a hard delete and won't be re-added to the Space if it becomes live. | Node `ip-xyz.ec2.internal` was **deleted(hard)** |
| Agent Claimed | <p>The agent was successfully registered to Netdata Cloud and is able to connect.</p><p>These events can only be seen on _All nodes_ War Room.</p> | Agent with claim ID `7d87bqs9-cv42-4823-8sd4-3614548850c7` was claimed. |
| Agent Connected | <p>The agent connected to the Netdata Cloud MQTT server (Agent-Cloud Link established).</p><p>These events can only be seen on _All nodes_ War Room.</p>  | Agent with claim ID `7d87bqs9-cv42-4823-8sd4-3614548850c7` has connected to Cloud. |
| Agent Disconnected | <p>The agent disconnected from the Netdata Cloud MQTT server (Agent-Cloud Link severed).</p><p>These events can only be seen on _All nodes_ War Room.</p> | Agent with claim ID `7d87bqs9-cv42-4823-8sd4-3614548850c7` has disconnected from Cloud: **Connection Timeout**. |
| Agent Authenticated | <p>The agent successfully authenticated itself to Netdata Cloud.</p><p>These events can only be seen on _All nodes_ War Room.</p> | Agent with claim ID `7d87bqs9-cv42-4823-8sd4-3614548850c7` has successfully authenticated. |
| Agent Authentication Failed | <p>The agent failed to authenticate itself to Netdata Cloud.</p><p>These events can only be seen on _All nodes_ War Room.</p> | Agent with claim ID `7d87bqs9-cv42-4823-8sd4-3614548850c7` has failed to authenticate. |
| Space Statistics | <p>Daily snapshot of space node statistics.</p><p>These events can only be seen on _All nodes_ War Room.</p> | Space statistics. Nodes: **22 live**, **21 stale**, **18 removed**, **61 total**. |


##### Alert events

| **Event name** | **Description** |  **Example** |
| :-- | :-- | :-- |
| Node Alert State Changed | These are node alert state transition events and can be seen as an alert history log. You will be able to see transitions to or from any of these states: Cleared, Warning, Critical, Removed, Error or Unknown | <p>Transition to Cleared</p><p>`httpcheck_web_service_bad_status` for `httpcheck_netdata_cloud.request_status` on `netdata-parent-xyz` recovered with value **8.33%** </p><p>Transition from Cleared to Warning or Critical</p><p> `httpcheck_web_service_bad_status` for `httpcheck_netdata_cloud.request_status` on `netdata-parent-xyz` was raised to **WARNING** with value **10%**</p><p>Transition from Warning to Critical</p><p>`httpcheck_web_service_bad_status` for `httpcheck_netdata_cloud.request_status` on `netdata-parent-xyz` escalated to **CRITICAL** with value **25%**</p>Transition from Critical to Warning</p><p>`httpcheck_web_service_bad_status` for `httpcheck_netdata_cloud.request_status` on `netdata-parent-xyz` was demoted to **WARNING** with value **10%**</p><p>Transition to Removed</p><p>Alert `httpcheck_web_service_bad_status` for `httpcheck_netdata_cloud.request_status` on `netdata-parent-xyz` is no longer available, state can't be assessed.</p><p>Transition to Error</p><p>For this alert `httpcheck_web_service_bad_status` related to `httpcheck_netdata_cloud.request_status` on `netdata-parent-xyz` we couldn't calculate the current value ⓘ</p> |

#### Who can access the events?

All users will be able to see events from the Topology and Alerts domain but Auditing events, once these are added, only be accessible to administrators.

## Related Topics

### **Related Concepts**

### Related Tasks

- [How to use the Events feed?](https://github.com/netdata/netdata/blob/master/docs/cloud/view-evetns-feed.md)
