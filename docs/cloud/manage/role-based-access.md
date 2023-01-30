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
| See space | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Delete Space | :heavy_check_mark: | - | - | - | - |
| Change Name | :heavy_check_mark: | - | - | - | - |
| Change Description | :heavy_check_mark: | - | - | - | - |

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
| Remove user from to Room | :heavy_check_mark: | :heavy_check_mark: | - | - | - | |


## Related Topics

### **Related Concepts**

- [ACLK](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/aclk.md)
- [plugins.d](https://github.com/netdata/netdata/tree/master/collectors/plugins.d)

### Related Tasks

- [Run-time troubleshooting with Functions](docs/nightly/tasks/operations/runtime-troubleshootting-with-function)
