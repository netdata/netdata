<!--
title: "Role-Based Access model"
sidebar_label: "Role-Based Access model"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/cloud/manage/role-based-access-model.md"
sidebar_position: "1"
learn_status: "Published"
learn_topic_type: "Concepts"
learn_rel_path: "Concepts"
learn_docs_purpose: "Explanation of Netdata roles and permissions linked to them"
-->

Netdata Cloud's role-based-access mechanism allows you to control what functionalities in the app users can access. Each user can be assigned only one role, which fully specifies all the capabilities they are afforded.

#### What roles are available?

With the advent of the paid plans we revamped the roles to cover needs expressed by Netdata users, like providing more limited access to their customers, or
being able to join any room. We also aligned the offered roles to the target audience of each plan. The end result is the following:

| **Role** | **Community** | **Pro** | **Business** |
| :-- | :--: | :--: | :--: |
| **Administrators**<p>This role allows users to manage Spaces, War Rooms, Nodes, and Users, this includes the Plan & Billing settings.</p><p>Provides access to all War Rooms in the space</p> | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| **Managers**<p>This role allows users to manage War Rooms and Users.</p><p>Provides access to all War Rooms and Nodes in the space.</p> | - | - | :heavy_check_mark: |
| **Troubleshooters**<p>This role is for users that will be just focused on using Netdata to troubleshoot, not manage entities.</p><p>Provides access to all War Rooms and Nodes in the space.</p> | - | :heavy_check_mark: | :heavy_check_mark: |
| **Observers**<p>This role is for read-only access with restricted access to explicit War Rooms and only the Nodes that appear in those War Rooms.</p>💡 Ideal for restricting your customer's access to their own dedicated rooms.<p></p> | - | - | :heavy_check_mark: |
| **Billing**<p>This role is for users that need to manage billing options and see invoices, with no further access to the system.</p> | - | - | :heavy_check_mark: |

#### What happens to the previous Member role?

We will maintain a Legacy Community plan for existing users, which will continue to provide access to the Member role.

#### Which functionalities are available for each role?

In more detail, you can find on the following tables which functionalities are available for each role on each domain.

##### Space Management

| **Functionality** | **Administrator** | **Manager** | **Troubleshooter** | **Observer** | **Billing** |
| :-- | :--: | :--: | :--: | :--: | :--: |
| See Space | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Leave Space  | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Delete Space | :heavy_check_mark: | - | - | - | - |
| Change name | :heavy_check_mark: | - | - | - | - |
| Change description | :heavy_check_mark: | - | - | - | - |

##### Node Management

| **Functionality** | **Administrator** | **Manager** | **Troubleshooter** | **Observer** | **Billing** |
| :-- | :--: | :--: | :--: | :--: | :--: |
| See all Nodes in Space (_All Nodes_ room) | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - | - |
| Connect Node to Space | :heavy_check_mark: | - | - | - | - |
| Delete Node from Space | :heavy_check_mark: | - | - | - | - |

##### User Management

| **Functionality** | **Administrator** | **Manager** | **Troubleshooter** | **Observer** | **Billing** | Notes |
| :-- | :--: | :--: | :--: | :--: | :--: | :-- |
| See all Users in Space | :heavy_check_mark: | :heavy_check_mark: | - | - | - | |
| Invite new User to Space | :heavy_check_mark: | :heavy_check_mark: | - | - | - | You can't invite a user with a role you don't have permissions to appoint to (see below) |
| Delete User from Space | :heavy_check_mark: | :heavy_check_mark: | - | - | - | You can't delete a user if he has a role you don't have permissions to appoint to (see below) |
| Appoint Administrators | :heavy_check_mark: | - | - | - | - | |
| Appoint Billing user | :heavy_check_mark: | - | - | - | - | |
| Appoint Managers | :heavy_check_mark: | :heavy_check_mark: | - | - | - | |
| Appoint Troubleshooters | :heavy_check_mark: | :heavy_check_mark: | - | - | - | |
| Appoint Observer | :heavy_check_mark: | :heavy_check_mark: | - | - | - | |
| See all Users in a Room | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - | |
| Invite existing user to Room | :heavy_check_mark: | :heavy_check_mark: | - | - | - | User already invited to the Space |
| Remove user from Room | :heavy_check_mark: | :heavy_check_mark: | - | - | - | |

##### Room Management

| **Functionality** | **Administrator** | **Manager** | **Troubleshooter** | **Observer** | **Billing** | Notes |
| :-- | :--: | :--: | :--: | :--: | :--: | :-- |
| See all Rooms in a Space | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - | - | |
| Join any Room in a Space | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - | - | By joining a room you will be enabled to get notifications from nodes on that room |
| Leave Room  | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - | |
| Create a new Room in a Space | :heavy_check_mark: | :heavy_check_mark: | - | - | - | |
| Delete Room | :heavy_check_mark: | :heavy_check_mark: | - | - | - | |
| Change Room name | :heavy_check_mark: | :heavy_check_mark: | - | - | - | If not the _All Nodes_ room |
| Change Room description | :heavy_check_mark: | :heavy_check_mark: | - | - | - | |
| Add existing Nodes to Room | :heavy_check_mark: | :heavy_check_mark: | - | - | - | Node already connected to the Space |
| Remove Nodes from Room | :heavy_check_mark: | :heavy_check_mark: | - | - | - | |

##### Notifications Management

| **Functionality** | **Administrator** | **Manager** | **Troubleshooter** | **Observer** | **Billing** | Notes |
| :-- | :--: | :--: | :--: | :--: | :--: | :-- |
| See all configured notifications on a Space  | :heavy_check_mark: | - | - | - | - | |
| Add new configuration | :heavy_check_mark: | - | - | - | - | |
| Enable/Disable configuration | :heavy_check_mark: | - | - | - | - |  |
| Edit configuration | :heavy_check_mark: | - | - | - | - | Some exceptions apply depending on [service level](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/manage-notification-methods.md#available-actions-per-notification-methods-based-on-service-level) |
| Delete configuration | :heavy_check_mark: | - | - | - | - | |
| Edit personal level notification settings | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | [Manage user notification settings](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/manage-notification-methods.md#manage-user-notification-settings) |

Notes:
* Enable, Edit and Add actions over specific notification methods will only be allowed if your plan has access to those ([service classification](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/notifications.mdx#service-classification))

##### Dashboards

| **Functionality** | **Administrator** | **Manager** | **Troubleshooter** | **Observer** | **Billing** |
| :-- | :--: | :--: | :--: | :--: | :--: |
| See all dashboards in Room | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - |
| Add new dashboard to Room | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - |
| Edit any dashboard in Room | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - | - |
| Edit own dashboard in Room | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - |
| Delete any dashboard in Room | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - | - |
| Delete own dashboard in Room | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - |

##### Functions

| **Functionality** | **Administrator** | **Manager** | **Troubleshooter** | **Observer** | **Billing** | Notes |
| :-- | :--: | :--: | :--: | :--: | :--: | :-- |
| See all functions in Room | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - |
| Run any function in Room | :heavy_check_mark: | :heavy_check_mark: | - | - | - |
| Run read-only function in Room | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - | |
| Run sensitive function in Room | :heavy_check_mark: | :heavy_check_mark: | - | - | - | There isn't any function on this category yet, so subject to change. |

##### Events feed

| **Functionality** | **Administrator** | **Manager** | **Troubleshooter** | **Observer** | **Billing** | Notes |
| :-- | :--: | :--: | :--: | :--: | :--: | :-- |
| See Alert or Topology events | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - | |
| See Auditing events | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - | These are coming soon, not currently available  |

##### Billing

| **Functionality** | **Administrator** | **Manager** | **Troubleshooter** | **Observer** | **Billing** | Notes |
| :-- | :--: | :--: | :--: | :--: | :--: | :-- |
| See Plan & Billing details | :heavy_check_mark: | - | - | - | :heavy_check_mark: | Current plan and usage figures |
| Update plans | :heavy_check_mark: | - | - | - | - | This includes cancelling current plan (going to Community plan) |
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
