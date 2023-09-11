<!--
title: "Events feed"
sidebar_label: "Events feed"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/cloud/insights/events-feed.md"
sidebar_position: "2800"
learn_status: "Published"
learn_topic_type: "Concepts"
learn_rel_path: "Concepts"
learn_docs_purpose: "Present the Netdata Events feed."
-->

# Events feed

Netdata Cloud provides the Events feed which is a powerful feature that tracks events that happen on your infrastructure, or in your Space. The feed lets you investigate events that occurred in the past, which is invaluable for troubleshooting. Common use cases are ones like when a node goes offline, and you want to understand what events happened before that. A detailed event history can also assist in attributing sudden pattern changes in a time series to specific changes in your environment.

## What are the available events?

At a high-level view, these are the domains from which the Events feed will provide visibility into.

> ⚠️ Based on your space's plan, different allowances are defined to query past data.

| **Domains of events** | **Community** | **Pro** | **Business** |
| :-- | :-- | :-- | :-- |
| **[Auditing events](#auditing-events)** - <br/>Events related to actions done on your Space, e.g. invite user, change user role or change plan.| 4 hours | 7 days | 90 days |
| **[Topology events](#topology-events)**<br/>Node state transition events, e.g. live or offline.| 4 hours | 7 days | 14 days |
| **[Alert events](#alert-events)**<br/>Alert state transition events, can be seen as an alert history log.| 4 hours | 7 days | 90 days |

### Auditing events

| **Event name** | **Description** |  **Example** |
| :-- | :-- | :-- |
| Space Created | The space was created.| Space `Acme Space` was **created** |
| Room Created | A room was created on the Space.| 	Room `DB Servers` was **created** by `John Doe` |
| Room Deleted | A room was deleted from the Space. | Room `DB servers` was **deleted** by `John Doe` |
| User Invited to Space | A user was invited to join the Space.| User `John Smith` was **invited** to this space by `Alan Doe` |
| User Uninvited from Space | An invitation for a user to join the space was revoked.| User `John Smith` was **uninvited** from this space |
| User Added to Space | A user was added to the Space from an invitation (user accepted the invitation).| User `John Smith` was **added** to this space by invite of `Alan Doe` |
| User Removed from Space | A user was added to the Space from an invitation. | User `John Smith` was **removed** from this space by `Alan Doe` |
| User Added to Room | A user was added to a room on the Space. | User `John Smith` was **added** to room `DB servers` |
| User Removed from Room | A user was removed from a room on the Space. | User `John Smith` was **removed** from room `DB Servers` by `Alan Doe` |
| User Space Properties Changed | The properties of a user on the Space have changed, e.g. change user role | User role for `John Smith` was **changed** to `troubleshooter` by `Alan Doe` |
| Node Added To Room | The node was added to a room on the Space. | Node `ip-xyz.ec2.internal` was **added** to room `DB Servers` by `John Doe` |
| Node Removed To Room | The node was removed from a room on the Space. | Node `ip-xyz.ec2.internal` was **removed** from room `DB Servers` by `John Doe` |
| Silencing Rule Created | A new alert notification silencing rule was created on the Space. | Silencing rule `DB Servers schedule silencing` on rooms `All nodes` and `DB Servers` was **created** by `John Smith` |
| Silencing Rule Changed | An existing alert notification silencing rule was modified on the Space. | Silencing rule `DB Servers schedule silencing` on rooms `All nodes` and `DB Servers` was **changed** by `John Doe` |
| Silencing Rule Deleted | An existing alert notifications silencing rule was removed from the Space. | Silencing rule `DB Servers schedule silencing` on rooms `All nodes` and `DB Servers` was **changed** by `Alan Smith` |

### Topology events

| **Event name** | **Description** |  **Example** |
| :-- | :-- | :-- |
| Node Became Live | The node is collecting and streaming metrics to Cloud.| Node `netdata-k8s-state-xyz` was **live** |
| Node Became Stale | The node is offline and not streaming metrics to Cloud. It can show historical data from a parent node. | Node `ip-xyz.ec2.internal` was **stale** |
| Node Became Offline | The node is offline, not streaming metrics to Cloud and not available in any parent node.| Node `ip-xyz.ec2.internal` was **offline** |
| Node Created | The node is created but it is still `Unseen` on Cloud, didn't establish a successful connection yet.| Node `ip-xyz.ec2.internal` was **created** |
| Node Removed |The node was removed from the Space, for example by using the `Delete` action on the node. This is a soft delete in that the node gets marked as deleted, but retains the association with this space. If it becomes live again, it will be restored (see `Node Restored` below) and reappear in this space as before. | Node `ip-xyz.ec2.internal` was **deleted (soft)** |
| Node Restored | The node was restored. See `Node Removed` above. | Node `ip-xyz.ec2.internal` was **restored** |
| Node Deleted | The node was deleted from the Space. This is a hard delete and no information on the node is retained. | Node `ip-xyz.ec2.internal` was **deleted (hard)** |
| Agent Connected | The agent connected to the Cloud MQTT server (Agent-Cloud Link established).<br/>These events can only be seen on _All nodes_ War Room. | Agent with claim ID `7d87bqs9-cv42-4823-8sd4-3614548850c7` has connected to Cloud. |
| Agent Disconnected | The agent disconnected from the Cloud MQTT server (Agent-Cloud Link severed).<br/>These events can only be seen on _All nodes_ War Room. | Agent with claim ID `7d87bqs9-cv42-4823-8sd4-3614548850c7` has disconnected from Cloud: **Connection Timeout**. |
| Space Statistics | Daily snapshot of space node statistics.<br/>These events can only be seen on _All nodes_ War Room. | Space statistics. Nodes: **22 live**, **21 stale**, **18 removed**, **61 total**. |


### Alert events

| **Event name** | **Description** |  **Example** |
| :-- | :-- | :-- |
| Node Alert State Changed | These are node alert state transition events and can be seen as an alert history log. You will be able to see transitions to or from any of these states: Cleared, Warning, Critical, Removed, Error or Unknown | Transition to Cleared:<br/>`httpcheck_web_service_bad_status` for `httpcheck_netdata_cloud.request_status` on `netdata-parent-xyz` recovered with value **8.33%**<br/><br/>Transition from Cleared to Warning or Critical:<br/>`httpcheck_web_service_bad_status` for `httpcheck_netdata_cloud.request_status` on `netdata-parent-xyz` was raised to **WARNING** with value **10%**<br/><br/>Transition from Warning to Critical:<br/>`httpcheck_web_service_bad_status` for `httpcheck_netdata_cloud.request_status` on `netdata-parent-xyz` escalated to **CRITICAL** with value **25%**<br/><br/>Transition from Critical to Warning:<br/>`httpcheck_web_service_bad_status` for `httpcheck_netdata_cloud.request_status` on `netdata-parent-xyz` was demoted to **WARNING** with value **10%**<br/><br/>Transition to Removed:<br/>Alert `httpcheck_web_service_bad_status` for `httpcheck_netdata_cloud.request_status` on `netdata-parent-xyz` is no longer available, state can't be assessed.<br/><br/>Transition to Error:<br/>For this alert `httpcheck_web_service_bad_status` related to `httpcheck_netdata_cloud.request_status` on `netdata-parent-xyz` we couldn't calculate the current value ⓘ|

## Who can access the events?

All users will be able to see events from the Topology and Alerts domain but Auditing events, once these are added, only be accessible to administrators. For more details checkout [Netdata Role-Based Access model](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/role-based-access.md).

## How to use the events feed

1. Click on the **Events** tab (located near the top of your screen)
1. You will be presented with a table listing the events that occurred from the timeframe defined on the date time picker
1. You can use the filtering capabilities available on right-hand bar to slice through the results provided. See more details on event types and filters 

Note: When you try to query a longer period than what your space allows you will see an error message highlighting that you are querying data outside of your plan.

### Event types and filters

| Event type | Tags | Nodes | Alert Status | Alert Names | Chart Names | 
| :-- | :-- | :-- | :-- | :-- | :-- |
| Node Became Live | node, lifecycle | Node name | - | - | - |
| Node Became Stale | node, lifecycle | Node name | - | - | - |
| Node Became Offline | node, lifecycle | Node name | - | - | - |
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
