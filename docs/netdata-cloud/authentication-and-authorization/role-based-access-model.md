# Role-Based Access model (RBAC)

Netdata Cloud's Role-Based Access mechanism allows you to control what functionalities a user can access.

## Roles

| **Role**                                                                                                                               | **Community**      | **Homelab**        | **Business**       | **Enterprise On-Prem** |
|:---------------------------------------------------------------------------------------------------------------------------------------|:-------------------|:-------------------|:-------------------|:-----------------------|
| **Admins** can control Spaces, Rooms, Nodes, Users and Billing.They can also access any Room in the Space.                             | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark:     |
| **Managers** can manage Rooms and Users. They can access any Room in the Space.                                                        | -                  | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark:     |
| **Troubleshooters** can only use Netdata to troubleshoot, not manage entities. They need to be assigned to Rooms in the Space.         | -                  | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark:     |
| **Observers** can only view data in specific Rooms.<br/> 💡 Ideal for restricting your customer's access to their own dedicated Rooms. | -                  | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark:     |
| **Billing** can handle billing options and invoices.                                                                                   | -                  | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark:     |

## Features

### Space Management

| **Functionality**      |     **Admin**      |    **Manager**     | **Troubleshooter** |    **Observer**    |    **Billing**     |
|:-----------------------|:------------------:|:------------------:|:------------------:|:------------------:|:------------------:|
| See Space              | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Leave Space            | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Delete Space           | :heavy_check_mark: |         -          |         -          |         -          |         -          |
| Change name            | :heavy_check_mark: |         -          |         -          |         -          |         -          |
| Change description     | :heavy_check_mark: |         -          |         -          |         -          |         -          |
| Change slug            | :heavy_check_mark: |         -          |         -          |         -          |         -          |
| Change preferred nodes | :heavy_check_mark: |         -          |         -          |         -          |         -          |

### Node Management

| **Functionality**                         |     **Admin**      |    **Manager**     | **Troubleshooter** | **Observer** | **Billing** |
|:------------------------------------------|:------------------:|:------------------:|:------------------:|:------------:|:-----------:|
| See all Nodes in Space (_All Nodes_ Room) | :heavy_check_mark: | :heavy_check_mark: |         -          |      -       |      -      |
| Connect Node to Space                     | :heavy_check_mark: |         -          |         -          |      -       |      -      |
| Delete Node from Space                    | :heavy_check_mark: |         -          |         -          |      -       |      -      |

### User Management

| **Functionality**                  |     **Admin**      |    **Manager**     | **Troubleshooter** |    **Observer**    | **Billing** |
|:-----------------------------------|:------------------:|:------------------:|:------------------:|:------------------:|:-----------:|
| See all Users in Space             | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |
| Invite new User to Space           | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |
| Delete Pending Invitation to Space | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |
| Delete User from Space             | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |
| Appoint Administrators             | :heavy_check_mark: |         -          |         -          |         -          |      -      |
| Appoint Billing user               | :heavy_check_mark: |         -          |         -          |         -          |      -      |
| Appoint Managers                   | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |
| Appoint Troubleshooters            | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |
| Appoint Observer                   | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |
| Appoint Member                     | :heavy_check_mark: |         -          |         -          |         -          |      -      |
| See all Users in a Room            | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      |
| Invite existing user to Room       | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |
| Remove user from Room              | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |

### Room Management

| **Functionality**            |     **Admin**      |    **Manager**     | **Troubleshooter** |    **Observer**    | **Billing** |
|:-----------------------------|:------------------:|:------------------:|:------------------:|:------------------:|:-----------:|
| See all Rooms in a Space     | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |
| Join any Room in a Space     | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |
| Leave Room                   | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      |
| Create a new Room in a Space | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |
| Delete Room                  | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |
| Change Room name             | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |
| Change Room description      | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |
| Add existing Nodes to Room   | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |
| Remove Nodes from Room       | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |

### Notification Management

| **Functionality**                                                         |     **Admin**      |    **Manager**     | **Troubleshooter** |    **Observer**    |    **Billing**     | Notes                                                                                                                                                                                                                            |
|:--------------------------------------------------------------------------|:------------------:|:------------------:|:------------------:|:------------------:|:------------------:|:---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| See all configured notifications on a Space                               | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |         -          |                                                                                                                                                                                                                                  |
| Add new configuration                                                     | :heavy_check_mark: |         -          |         -          |         -          |         -          |                                                                                                                                                                                                                                  |
| Enable/Disable configuration                                              | :heavy_check_mark: |         -          |         -          |         -          |         -          |                                                                                                                                                                                                                                  |
| Edit configuration                                                        | :heavy_check_mark: |         -          |         -          |         -          |         -          | Some exceptions apply depending on [service level](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/manage-notification-methods.md#available-actions-per-notification-method-based-on-service-level) |
| Delete configuration                                                      | :heavy_check_mark: |         -          |         -          |         -          |         -          |                                                                                                                                                                                                                                  |
| Edit personal level notification settings                                 | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | [Manage user notification settings](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/manage-notification-methods.md#manage-user-notification-settings)                                               |
| See Space Alert notification silencing rules                              | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |                                                                                                                                                                                                                                  |
| Add new Space Alert notification silencing rule                           | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |         -          |                                                                                                                                                                                                                                  |
| Enable/Disable Space Alert notification silencing rule                    | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |         -          |                                                                                                                                                                                                                                  |
| Edit Space Alert notification silencing rule                              | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |         -          |                                                                                                                                                                                                                                  |
| Delete Space Alert notification silencing rule                            | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |         -          |                                                                                                                                                                                                                                  |
| See, add, edit or delete personal level Alert notification silencing rule | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |         -          |                                                                                                                                                                                                                                  |

> **Note**
>
> Enable, Edit and Add actions over specific notification methods will only be allowed if your plan has access to those (see [service classification](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/centralized-cloud-notifications-reference.md#service-classification))

### Dashboards

| **Functionality**            |     **Admin**      |    **Manager**     | **Troubleshooter** |    **Observer**    | **Billing** |
|:-----------------------------|:------------------:|:------------------:|:------------------:|:------------------:|:-----------:|
| See all dashboards in Room   | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      |
| Add new dashboard to Room    | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      |
| Edit any dashboard in Room   | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |         -          |      -      |
| Edit own dashboard in Room   | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      |
| Delete any dashboard in Room | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |         -          |      -      |
| Delete own dashboard in Room | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      |

### Functions

| **Functionality**              |     **Admin**      |    **Manager**     | **Troubleshooter** |    **Observer**    | **Billing** |
|:-------------------------------|:------------------:|:------------------:|:------------------:|:------------------:|:-----------:|
| See all functions in Room      | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      |
| Run any function in Room       | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |
| Run read-only function in Room | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      |
| Run sensitive function in Room | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |

### Events feed

| **Functionality**            |     **Admin**      |    **Manager**     | **Troubleshooter** |    **Observer**    | **Billing** |
|:-----------------------------|:------------------:|:------------------:|:------------------:|:------------------:|:-----------:|
| See Alert or Topology events | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      |
| See Auditing events          | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      |

### Billing

| **Functionality**          |     **Admin**      | **Manager** | **Troubleshooter** | **Observer** |    **Billing**     | Notes                                                           |
|:---------------------------|:------------------:|:-----------:|:------------------:|:------------:|:------------------:|:----------------------------------------------------------------|
| See Plan & Billing details | :heavy_check_mark: |      -      |         -          |      -       | :heavy_check_mark: | Current plan and usage figures                                  |
| Update plans               | :heavy_check_mark: |      -      |         -          |      -       |         -          | This includes cancelling current plan (going to Community plan) |
| See invoices               | :heavy_check_mark: |      -      |         -          |      -       | :heavy_check_mark: |                                                                 |
| Manage payment methods     | :heavy_check_mark: |      -      |         -          |      -       | :heavy_check_mark: |                                                                 |
| Update billing email       | :heavy_check_mark: |      -      |         -          |      -       | :heavy_check_mark: |                                                                 |

### Dynamic Configuration Manager

> **Note**
>
> Netdata Cloud paid subscription required for all actions except "List All".

| **Functionality**                     |     **Admin**      |    **Manager**     | **Troubleshooter** |    **Observer**    |    **Billing**     |
|:--------------------------------------|:------------------:|:------------------:|:------------------:|:------------------:|:------------------:|
| List All (see all configurable items) | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Enable/Disable                        | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |         -          |
| Add                                   | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |         -          |
| Update                                | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |         -          |
| Remove                                | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |         -          |
| Test                                  | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |         -          |
| View                                  | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |         -          |
| View File Format                      | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |         -          |

### Other permissions

| **Functionality**          |     **Admin**      |    **Manager**     | **Troubleshooter** |    **Observer**    | **Billing** |
|:---------------------------|:------------------:|:------------------:|:------------------:|:------------------:|:-----------:|
| See Bookmarks in Space     | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      |
| Add Bookmark to Space      | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |         -          |      -      |
| Delete Bookmark from Space | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |         -          |      -      |
| See Visited Nodes          | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      |
| Update Visited Nodes       | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      |
