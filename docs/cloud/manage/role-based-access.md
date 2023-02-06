<!--
title: "Role-Based Access model"
sidebar_label: "Role-Based Access model"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/cloud/manage/role-based-access-model.md)"
sidebar_position: "1"
learn_status: "Published"
learn_topic_type: "Concepts"
learn_rel_path: "Concepts"
learn_docs_purpose: "Explanation of Netdata roles and permissions linked to them"
-->

Netdata Cloud provides out-of-the-box a role-based-access mechanism that allows you to control what functionalities in the app users can access.
This is achieved through the availability of a set of pre-defined roles that have associated permissions depending on the purpose of each one of them.

#### What roles are available?

Depending on the plan associated to your space you will have different roles available:

| **Role** | **Community** | **Pro** | **Business** |
| :-- | :-- | :-- | :-- |
| **Administrators**<p>This role allows users to manage Spaces, War Rooms, Nodes, and Users, this includes the Plan & Billing settings. It also allows access to all War Rooms in the space</p> | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| **Managers**<p>This role allows users to manage War Rooms and Users. It also allows access to all War Rooms in the space.</p> | - | - | :heavy_check_mark: |
| **Troubleshooters**<p>This role is for users that will be just focused on using Netdata to troubleshoot, not manage entities. It also allows access to all War Rooms in the space.</p> | - | :heavy_check_mark: | :heavy_check_mark: |
| **Observers**<p>This role is for read-only access with restricted access to explicit War Rooms.</p> | - | - | :heavy_check_mark: |
| **Billing**<p>This role is for users that need to manage billing options and see invoices.</p> | - | - | :heavy_check_mark: |

#### Which functionalities are available for each role?

In more detail, you can find on the following table which functionalities are available for each role.

##### Space Management

| **Functionality** | **Administrator** | **Manager** | **Troubleshooter** | **Observer** | **Billing** |
| :-- | :--: | :--: | :--: | :--: | :--: |
| See Space | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Delete Space | :heavy_check_mark: | - | - | - | - |
| Change name | :heavy_check_mark: | - | - | - | - |
| Change description | :heavy_check_mark: | - | - | - | - |
| Leave Space  | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |

##### Node Management

| **Functionality** | **Administrator** | **Manager** | **Troubleshooter** | **Observer** | **Billing** |
| :-- | :--: | :--: | :--: | :--: | :--: |
| See all Nodes in Space (All Nodes room) | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - | - |
| Connect Node to Space | :heavy_check_mark: | - | - | - | - |
| Delete Node from Space | :heavy_check_mark: | - | - | - | - |

##### User Management

| **Functionality** | **Administrator** | **Manager** | **Troubleshooter** | **Observer** | **Billing** | Notes |
| :-- | :--: | :--: | :--: | :--: | :--: | :-- |
| See all Users in Space | :heavy_check_mark: | :heavy_check_mark: | - | - | - | |
| Invite new User to Space | :heavy_check_mark: | :heavy_check_mark: | - | - | - | You can't invite a user with are role you don't have permissions to appoint to |
| Delete User from Space | :heavy_check_mark: | :heavy_check_mark: | - | - | - | You can't delete a user if he has a role you don't have permissions to appoint to |
| Appoint Administrators | :heavy_check_mark: | - | - | - | - | |
| Appoint Billing user | :heavy_check_mark: | - | - | - | - | |
| Appoint Managers | :heavy_check_mark: | :heavy_check_mark: | - | - | - | |
| Appoint Troubleshooters | :heavy_check_mark: | :heavy_check_mark: | - | - | - | |
| Appoint Observer | :heavy_check_mark: | :heavy_check_mark: | - | - | - | |
| See all Users in a Room | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - | |
| Invite existing user to Room | :heavy_check_mark: | :heavy_check_mark: | - | - | - | |
| Remove user from Room | :heavy_check_mark: | :heavy_check_mark: | - | - | - | |

##### Room Management

| **Functionality** | **Administrator** | **Manager** | **Troubleshooter** | **Observer** | **Billing** | Notes |
| :-- | :--: | :--: | :--: | :--: | :--: | :-- |
| See all Rooms in a Space | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - | - | |
| Join any Room in a Space | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - | - | This will enable you to get notifications from nodes on the room you joined |
| Create a new Room in a Space | :heavy_check_mark: | :heavy_check_mark: | - | - | - | |
| Delete Room | :heavy_check_mark: | :heavy_check_mark: | - | - | - | |
| Change Room name | :heavy_check_mark: | :heavy_check_mark: | - | - | - | If not the _All Nodes_ room |
| Change Room description | :heavy_check_mark: | :heavy_check_mark: | - | - | - | |
| Add existing Nodes to Room | :heavy_check_mark: | :heavy_check_mark: | - | - | - | |
| Remove Nodes from Room | :heavy_check_mark: | :heavy_check_mark: | - | - | - | |
| Leave Room  | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - | |

##### Notifications Management

| **Functionality** | **Administrator** | **Manager** | **Troubleshooter** | **Observer** | **Billing** | Notes |
| :-- | :--: | :--: | :--: | :--: | :--: | :-- |
| See all configured notifications on a Space  | :heavy_check_mark: | - | - | - | - | |
| Add new configuration | :heavy_check_mark: | - | - | - | - | |
| Enable/Disable configuration | :heavy_check_mark: | - | - | - | - |  |
| Edit configuration | :heavy_check_mark: | - | - | - | - | Some exceptions apply depending on [service level](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/manage-notification-methods.md#available-actions-per-notification-methods-based-on-service-level)) |
| Delete configuration | :heavy_check_mark: | - | - | - | - | |
| Edit personal level notification settings | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | [Manage user notification settings](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/manage-notification-methods.md#manage-user-notification-settings) |

Notes:
* Enable, Edit and Add actions over specific notification methods will only be allowed if your plan has access to those ([service classification](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/notifications.mdx#service-classification))

##### Dashboards

| **Functionality** | **Administrator** | **Manager** | **Troubleshooter** | **Observer** | **Billing** |
| :-- | :--: | :--: | :--: | :--: | :--: |
| See all dashboards in Room | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - |
| Add new dashboards to Room | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - |
| Edit any dashboards in Room | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - | - |
| Edit own dashboards in Room | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - |
| Delete any dashboards in Room | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - | - |
| Delete own dashboards in Room | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - |

##### Functions

| **Functionality** | **Administrator** | **Manager** | **Troubleshooter** | **Observer** | **Billing** | Notes |
| :-- | :--: | :--: | :--: | :--: | :--: | :-- |
| See all functions in Room | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - |
| Run any function in Room | :heavy_check_mark: | :heavy_check_mark: | - | - | - |
| Run read-only function in Room | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - | There isn't any function on this category, subject to change. |
| Run sensitive function in Room | :heavy_check_mark: | :heavy_check_mark: | - | - | - | There isn't any function on this category, subject to change. |

##### Events feed

| **Functionality** | **Administrator** | **Manager** | **Troubleshooter** | **Observer** | **Billing** | Notes |
| :-- | :--: | :--: | :--: | :--: | :--: | :-- |
| See Alert or Topology events | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - | |
| See Auditing events | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - | These are coming soon, not currently available  |

##### Billing

| **Functionality** | **Administrator** | **Manager** | **Troubleshooter** | **Observer** | **Billing** | Notes |
| :-- | :--: | :--: | :--: | :--: | :--: | :-- |
| See Plan & Billing details | :heavy_check_mark: | - | - | - | :heavy_check_mark: | Current plan and usage figures |
| Update plans | :heavy_check_mark: | - | - | - | - | This includes cancelling |
| See invoices | :heavy_check_mark: | - | - | - | :heavy_check_mark: | |
| Manage payment methods | :heavy_check_mark: | - | - | - | :heavy_check_mark: | |
| Update billing email | :heavy_check_mark: | - | - | - | :heavy_check_mark: | |

##### Other permissions

| **Functionality** | **Administrator** | **Manager** | **Troubleshooter** | **Observer** | **Billing** |
| :-- | :--: | :--: | :--: | :--: | :--: |
| See Bookmarks in Space | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - |
| Add Bookmark to Space | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - | - |
| Delete Bookmark from Space | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - | - |
| See Visited Nodes | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - |
| Update Visited Nodes | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - |
