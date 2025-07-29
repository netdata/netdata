# Role-Based Access Control (RBAC)

## Overview

You can control what functionalities users can access in Netdata Cloud through the Role-Based Access mechanism. RBAC helps you secure your monitoring infrastructure by ensuring team members only access the data and features they need for their specific responsibilities.

**What RBAC enables you to do:**

- Restrict access to sensitive monitoring data
- Control who can modify configurations and settings
- Manage billing and subscription access
- Organize teams with appropriate permission levels
- Maintain audit trails of user actions

## Choose the Right Role

### Role Selection Guide

**When assigning roles, consider:**

| **If the user needs to...**                                                                      | **Recommended Role** |
|:-------------------------------------------------------------------------------------------------|:---------------------|
| **Full system control** - manage everything including billing, users, and all configurations     | **Admin**            |
| **Team and infrastructure management** - manage users, rooms, and configurations but not billing | **Manager**          |
| **Active troubleshooting** - investigate issues, run diagnostics, create dashboards              | **Troubleshooter**   |
| **View-only access** - monitor specific systems without making changes                           | **Observer**         |
| **Billing management** - handle invoices and payments without system access                      | **Billing**          |

## Quick Reference

<details>
<summary><strong>Role Comparison by Plan</strong></summary><br/>

| **Role**                                                                                                                               |   **Community**    |    **Homelab**     |    **Business**    | **Enterprise On-Prem** |
|:---------------------------------------------------------------------------------------------------------------------------------------|:------------------:|:------------------:|:------------------:|:----------------------:|
| **Admins** can control Spaces, Rooms, Nodes, Users and Billing. They can also access any Room in the Space.                            | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |   :heavy_check_mark:   |
| **Managers** can manage Rooms and Users. They can access any Room in the Space.                                                        |         -          | :heavy_check_mark: | :heavy_check_mark: |   :heavy_check_mark:   |
| **Troubleshooters** can only use Netdata to troubleshoot, not manage entities. They need to be assigned to Rooms in the Space.         |         -          | :heavy_check_mark: | :heavy_check_mark: |   :heavy_check_mark:   |
| **Observers** can only view data in specific Rooms.<br/> 💡 Ideal for restricting your customer's access to their own dedicated Rooms. |         -          | :heavy_check_mark: | :heavy_check_mark: |   :heavy_check_mark:   |
| **Billing** can handle billing options and invoices.                                                                                   |         -          | :heavy_check_mark: | :heavy_check_mark: |   :heavy_check_mark:   |

</details>

### Key Permissions Summary

| **Area**             |  **Admin**   |   **Manager**    | **Troubleshooter**  |    **Observer**     | **Billing**  |
|:---------------------|:------------:|:----------------:|:-------------------:|:-------------------:|:------------:|
| **Space Management** | Full control |    View only     |      View only      |      View only      |  View only   |
| **User Management**  | Full control | Most permissions | View users in rooms | View users in rooms |     None     |
| **Room Management**  | Full control |   Full control   | View assigned rooms | View assigned rooms |     None     |
| **Node Management**  | Full control |  View all nodes  |        None         |        None         |     None     |
| **Billing Access**   | Full control |       None       |        None         |        None         | Full control |
| **Notifications**    | Full control |    View only     |      View only      |      View only      |     None     |

## Detailed Permissions

<details>
<summary><strong>Space Management</strong></summary><br/>

| **Functionality**          |     **Admin**      |    **Manager**     | **Troubleshooter** |    **Observer**    |    **Billing**     | **Notes** |
|:---------------------------|:------------------:|:------------------:|:------------------:|:------------------:|:------------------:|:----------|
| **See Space**              | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |           |
| **Leave Space**            | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |           |
| **Delete Space**           | :heavy_check_mark: |         -          |         -          |         -          |         -          |           |
| **Change name**            | :heavy_check_mark: |         -          |         -          |         -          |         -          |           |
| **Change description**     | :heavy_check_mark: |         -          |         -          |         -          |         -          |           |
| **Change slug**            | :heavy_check_mark: |         -          |         -          |         -          |         -          |           |
| **Change preferred nodes** | :heavy_check_mark: |         -          |         -          |         -          |         -          |           |

</details>

<details>
<summary><strong>Node Management</strong></summary><br/>

| **Functionality**                             |     **Admin**      |    **Manager**     | **Troubleshooter** | **Observer** | **Billing** | **Notes** |
|:----------------------------------------------|:------------------:|:------------------:|:------------------:|:------------:|:-----------:|:----------|
| **See all Nodes in Space (_All Nodes_ Room)** | :heavy_check_mark: | :heavy_check_mark: |         -          |      -       |      -      |           |
| **Connect Node to Space**                     | :heavy_check_mark: |         -          |         -          |      -       |      -      |           |
| **Delete Node from Space**                    | :heavy_check_mark: |         -          |         -          |      -       |      -      |           |

</details>

<details>
<summary><strong>User Management</strong></summary><br/>

| **Functionality**                      |     **Admin**      |    **Manager**     | **Troubleshooter** |    **Observer**    | **Billing** | **Notes** |
|:---------------------------------------|:------------------:|:------------------:|:------------------:|:------------------:|:-----------:|:----------|
| **See all Users in Space**             | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |           |
| **Invite new User to Space**           | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |           |
| **Delete Pending Invitation to Space** | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |           |
| **Delete User from Space**             | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |           |
| **Appoint Administrators**             | :heavy_check_mark: |         -          |         -          |         -          |      -      |           |
| **Appoint Billing user**               | :heavy_check_mark: |         -          |         -          |         -          |      -      |           |
| **Appoint Managers**                   | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |           |
| **Appoint Troubleshooters**            | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |           |
| **Appoint Observer**                   | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |           |
| **Appoint Member**                     | :heavy_check_mark: |         -          |         -          |         -          |      -      |           |
| **See all Users in a Room**            | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      |           |
| **Invite existing user to Room**       | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |           |
| **Remove user from Room**              | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |           |

</details>

<details>
<summary><strong>Room Management</strong></summary><br/>

| **Functionality**                |     **Admin**      |    **Manager**     | **Troubleshooter** |    **Observer**    | **Billing** | **Notes** |
|:---------------------------------|:------------------:|:------------------:|:------------------:|:------------------:|:-----------:|:----------|
| **See all Rooms in a Space**     | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |           |
| **Join any Room in a Space**     | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |           |
| **Leave Room**                   | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      |           |
| **Create a new Room in a Space** | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |           |
| **Delete Room**                  | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |           |
| **Change Room name**             | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |           |
| **Change Room description**      | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |           |
| **Add existing Nodes to Room**   | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |           |
| **Remove Nodes from Room**       | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |           |

</details>

<details>
<summary><strong>Notification Management</strong></summary><br/>

| **Functionality**                                                             |     **Admin**      |    **Manager**     | **Troubleshooter** |    **Observer**    |    **Billing**     | **Notes**                                                                                                                                                                                                                        |
|:------------------------------------------------------------------------------|:------------------:|:------------------:|:------------------:|:------------------:|:------------------:|:---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **See all configured notifications on a Space**                               | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |         -          |                                                                                                                                                                                                                                  |
| **Add new configuration**                                                     | :heavy_check_mark: |         -          |         -          |         -          |         -          |                                                                                                                                                                                                                                  |
| **Enable/Disable configuration**                                              | :heavy_check_mark: |         -          |         -          |         -          |         -          |                                                                                                                                                                                                                                  |
| **Edit configuration**                                                        | :heavy_check_mark: |         -          |         -          |         -          |         -          | Some exceptions apply depending on [service level](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/manage-notification-methods.md#available-actions-per-notification-method-based-on-service-level) |
| **Delete configuration**                                                      | :heavy_check_mark: |         -          |         -          |         -          |         -          |                                                                                                                                                                                                                                  |
| **Edit personal level notification settings**                                 | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | [Manage user notification settings](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/manage-notification-methods.md#manage-user-notification-settings)                                               |
| **See Space Alert notification silencing rules**                              | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |                                                                                                                                                                                                                                  |
| **Add new Space Alert notification silencing rule**                           | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |         -          |                                                                                                                                                                                                                                  |
| **Enable/Disable Space Alert notification silencing rule**                    | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |         -          |                                                                                                                                                                                                                                  |
| **Edit Space Alert notification silencing rule**                              | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |         -          |                                                                                                                                                                                                                                  |
| **Delete Space Alert notification silencing rule**                            | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |         -          |                                                                                                                                                                                                                                  |
| **See, add, edit or delete personal level Alert notification silencing rule** | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |         -          |                                                                                                                                                                                                                                  |

</details>

<details>
<summary><strong>Dashboards</strong></summary><br/>

| **Functionality**                |     **Admin**      |    **Manager**     | **Troubleshooter** |    **Observer**    | **Billing** | **Notes** |
|:---------------------------------|:------------------:|:------------------:|:------------------:|:------------------:|:-----------:|:----------|
| **See all dashboards in Room**   | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      |           |
| **Add new dashboard to Room**    | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      |           |
| **Edit any dashboard in Room**   | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |         -          |      -      |           |
| **Edit own dashboard in Room**   | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      |           |
| **Delete any dashboard in Room** | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |         -          |      -      |           |
| **Delete own dashboard in Room** | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      |           |

</details>

<details>
<summary><strong>Functions</strong></summary><br/>

| **Functionality**                  |     **Admin**      |    **Manager**     | **Troubleshooter** |    **Observer**    | **Billing** | **Notes** |
|:-----------------------------------|:------------------:|:------------------:|:------------------:|:------------------:|:-----------:|:----------|
| **See all functions in Room**      | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      |           |
| **Run any function in Room**       | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |           |
| **Run read-only function in Room** | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      |           |
| **Run sensitive function in Room** | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |           |

</details>

<details>
<summary><strong>Events Tab</strong></summary><br/>

| **Functionality**                |     **Admin**      |    **Manager**     | **Troubleshooter** |    **Observer**    | **Billing** | **Notes** |
|:---------------------------------|:------------------:|:------------------:|:------------------:|:------------------:|:-----------:|:----------|
| **See Alert or Topology events** | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      |           |
| **See Auditing events**          | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      |           |

</details>

<details>
<summary><strong>Billing</strong></summary><br/>

| **Functionality**              |     **Admin**      | **Manager** | **Troubleshooter** | **Observer** |    **Billing**     | **Notes**                                                       |
|:-------------------------------|:------------------:|:-----------:|:------------------:|:------------:|:------------------:|:----------------------------------------------------------------|
| **See Plan & Billing details** | :heavy_check_mark: |      -      |         -          |      -       | :heavy_check_mark: | Current plan and usage figures                                  |
| **Update plans**               | :heavy_check_mark: |      -      |         -          |      -       |         -          | This includes cancelling current plan (going to Community plan) |
| **See invoices**               | :heavy_check_mark: |      -      |         -          |      -       | :heavy_check_mark: |                                                                 |
| **Manage payment methods**     | :heavy_check_mark: |      -      |         -          |      -       | :heavy_check_mark: |                                                                 |
| **Update billing email**       | :heavy_check_mark: |      -      |         -          |      -       | :heavy_check_mark: |                                                                 |

</details>

<details>
<summary><strong>Dynamic Configuration Manager</strong></summary><br/>

| **Functionality**                         |     **Admin**      |    **Manager**     | **Troubleshooter** |    **Observer**    |    **Billing**     | **Notes** |
|:------------------------------------------|:------------------:|:------------------:|:------------------:|:------------------:|:------------------:|:----------|
| **List All (see all configurable items)** | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |           |
| **Enable/Disable**                        | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |         -          |           |
| **Add**                                   | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |         -          |           |
| **Update**                                | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |         -          |           |
| **Remove**                                | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |         -          |           |
| **Test**                                  | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |         -          |           |
| **View**                                  | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |         -          |           |
| **View File Format**                      | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |         -          |           |

</details>

<details>
<summary><strong>Other Permissions</strong></summary><br/>

| **Functionality**              |     **Admin**      |    **Manager**     | **Troubleshooter** |    **Observer**    | **Billing** | **Notes** |
|:-------------------------------|:------------------:|:------------------:|:------------------:|:------------------:|:-----------:|:----------|
| **See Bookmarks in Space**     | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      |           |
| **Add Bookmark to Space**      | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |         -          |      -      |           |
| **Delete Bookmark from Space** | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |         -          |      -      |           |
| **See Visited Nodes**          | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      |           |
| **Update Visited Nodes**       | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      |           |

</details><br/>

:::note

Enable, Edit and Add actions over specific notification methods will only be allowed if your plan has access to those (see [service classification](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/centralized-cloud-notifications-reference.md#service-classification))

:::

:::note

Netdata Cloud paid subscription required for all actions except "List All" in Dynamic Configuration Manager.

:::
