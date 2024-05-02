# Role-Based Access model

Netdata Cloud's role-based-access mechanism allows you to control what functionalities in the app users can access. Each user can be assigned only one role, which fully specifies all the capabilities they are afforded.

## What roles are available?

With the advent of the paid plans we revamped the roles to cover needs expressed by Netdata users, like providing more limited access to their customers, or
being able to join any room. We also aligned the offered roles to the target audience of each plan. The end result is the following:

| **Role**                                                                                                                                                                                          | **Community**      | **Homelab**        | **Business**       | **Enterprise On-Premise** |
|:--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|:-------------------|:-------------------|:-------------------|:--------------------------|
| **Admins**<p>Users with this role can control Spaces, War Rooms, Nodes, Users and Billing.</p><p>They can also access any War Room in the Space.</p>                                              | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark:        |
| **Managers**<p>Users with this role can manage War Rooms and Users.</p><p>They can access any War Room in the Space.</p>                                                                          | -                  | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark:        |
| **Troubleshooters**<p>Users with this role can use Netdata to troubleshoot, not manage entities.</p><p>They can access any War Room in the Space.</p>                                             | -                  | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark:        |
| **Observers**<p>Users with this role can only view data in specific War Rooms.</p>💡 Ideal for restricting your customer's access to their own dedicated rooms.<p></p>                            | -                  | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark:        |
| **Billing**<p>Users with this role can handle billing options and invoices.</p>                                                                                                                   | -                  | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark:        |
| **Member** ⚠️ Legacy role<p>Users with this role you can create War Rooms and invite other Members.</p><p>They can only see the War Rooms they belong to and all Nodes in the All Nodes room.</p> | -                  | -                  | -                  | -                         |

## Which functionalities are available for each role?

In more detail, you can find on the following tables which functionalities are available for each role on each domain.

### Space Management

| **Functionality**      |     **Admin**      |    **Manager**     | **Troubleshooter** |    **Observer**    |    **Billing**     |     **Member**     |
|:-----------------------|:------------------:|:------------------:|:------------------:|:------------------:|:------------------:|:------------------:|
| See Space              | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Leave Space            | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Delete Space           | :heavy_check_mark: |         -          |         -          |         -          |         -          |         -          |
| Change name            | :heavy_check_mark: |         -          |         -          |         -          |         -          |         -          |
| Change description     | :heavy_check_mark: |         -          |         -          |         -          |         -          |         -          |
| Change slug            | :heavy_check_mark: |         -          |         -          |         -          |         -          |         -          |
| Change preferred nodes | :heavy_check_mark: |         -          |         -          |         -          |         -          |         -          |

### Node Management

| **Functionality**                         |     **Admin**      |    **Manager**     | **Troubleshooter** | **Observer** | **Billing** |     **Member**     | Notes                                      |
|:------------------------------------------|:------------------:|:------------------:|:------------------:|:------------:|:-----------:|:------------------:|:-------------------------------------------|
| See all Nodes in Space (_All Nodes_ room) | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -       |      -      | :heavy_check_mark: | Members are always on the _All Nodes_ room |
| Connect Node to Space                     | :heavy_check_mark: |         -          |         -          |      -       |      -      |         -          | -                                          |
| Delete Node from Space                    | :heavy_check_mark: |         -          |         -          |      -       |      -      |         -          | -                                          |

### User Management

| **Functionality**                  |     **Admin**      |    **Manager**     | **Troubleshooter** |    **Observer**    | **Billing** |     **Member**     | Notes                                                                                         |
|:-----------------------------------|:------------------:|:------------------:|:------------------:|:------------------:|:-----------:|:------------------:|:----------------------------------------------------------------------------------------------|
| See all Users in Space             | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      | :heavy_check_mark: |                                                                                               |
| Invite new User to Space           | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      | :heavy_check_mark: | You can't invite a user with a role you don't have permissions to appoint to (see below)      |
| Delete Pending Invitation to Space | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      | :heavy_check_mark: |                                                                                               |
| Delete User from Space             | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |         -          | You can't delete a user if he has a role you don't have permissions to appoint to (see below) |
| Appoint Administrators             | :heavy_check_mark: |         -          |         -          |         -          |      -      |         -          |                                                                                               |
| Appoint Billing user               | :heavy_check_mark: |         -          |         -          |         -          |      -      |         -          |                                                                                               |
| Appoint Managers                   | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |         -          |                                                                                               |
| Appoint Troubleshooters            | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |         -          |                                                                                               |
| Appoint Observer                   | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |         -          |                                                                                               |
| Appoint Member                     | :heavy_check_mark: |         -          |         -          |         -          |      -      | :heavy_check_mark: | Only available on Early Bird plans                                                            |
| See all Users in a Room            | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      | :heavy_check_mark: |                                                                                               |
| Invite existing user to Room       | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      | :heavy_check_mark: | User already invited to the Space                                                             |
| Remove user from Room              | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |         -          |                                                                                               |

### Room Management

| **Functionality**            |     **Admin**      |    **Manager**     | **Troubleshooter** |    **Observer**    | **Billing** |     **Member**     | Notes                                                                              |
|:-----------------------------|:------------------:|:------------------:|:------------------:|:------------------:|:-----------:|:------------------:|:-----------------------------------------------------------------------------------|
| See all Rooms in a Space     | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |         -          |      -      |         -          |                                                                                    |
| Join any Room in a Space     | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |         -          |      -      |         -          | By joining a room you will be enabled to get notifications from nodes on that room |
| Leave Room                   | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      | :heavy_check_mark: |                                                                                    |
| Create a new Room in a Space | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      | :heavy_check_mark: |                                                                                    |
| Delete Room                  | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      |         -          |                                                                                    |
| Change Room name             | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      | :heavy_check_mark: | If not the _All Nodes_ room                                                        |
| Change Room description      | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      | :heavy_check_mark: |                                                                                    |
| Add existing Nodes to Room   | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      | :heavy_check_mark: | Node already connected to the Space                                                |
| Remove Nodes from Room       | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |      -      | :heavy_check_mark: |                                                                                    |

### Notifications Management

| **Functionality**                                                         |     **Admin**      |    **Manager**     | **Troubleshooter** |    **Observer**    |    **Billing**     |     **Member**     | Notes                                                                                                                                                                                                                               |
|:--------------------------------------------------------------------------|:------------------:|:------------------:|:------------------:|:------------------:|:------------------:|:------------------:|:------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| See all configured notifications on a Space                               | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |         -          | :heavy_check_mark: |                                                                                                                                                                                                                                     |
| Add new configuration                                                     | :heavy_check_mark: |         -          |         -          |         -          |         -          |         -          |                                                                                                                                                                                                                                     |
| Enable/Disable configuration                                              | :heavy_check_mark: |         -          |         -          |         -          |         -          |         -          |                                                                                                                                                                                                                                     |
| Edit configuration                                                        | :heavy_check_mark: |         -          |         -          |         -          |         -          |         -          | Some exceptions apply depending on [service level](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/manage-notification-methods.md#available-actions-per-notification-methods-based-on-service-level) |
| Delete configuration                                                      | :heavy_check_mark: |         -          |         -          |         -          |         -          |         -          |                                                                                                                                                                                                                                     |
| Edit personal level notification settings                                 | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | [Manage user notification settings](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/manage-notification-methods.md#manage-user-notification-settings)                                                |
| See space alert notification silencing rules                              | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |         -          |                                                                                                                                                                                                                                     |
| Add new space alert notification silencing rule                           | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |         -          |         -          |                                                                                                                                                                                                                                     |
| Enable/Disable space alert notification silencing rule                    | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |         -          |         -          |                                                                                                                                                                                                                                     |
| Edit space alert notification silencing rule                              | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |         -          |         -          |                                                                                                                                                                                                                                     |
| Delete space alert notification silencing rule                            | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |         -          |         -          |                                                                                                                                                                                                                                     |
| See, add, edit or delete personal level alert notification silencing rule | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |         -          |         -          |                                                                                                                                                                                                                                     |

> **Note**
>
> Enable, Edit and Add actions over specific notification methods will only be allowed if your plan has access to those ([service classification](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/notifications.md#service-classification))

### Dashboards

| **Functionality**            |     **Admin**      |    **Manager**     | **Troubleshooter** |    **Observer**    | **Billing** |     **Member**     |
|:-----------------------------|:------------------:|:------------------:|:------------------:|:------------------:|:-----------:|:------------------:|
| See all dashboards in Room   | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      | :heavy_check_mark: |
| Add new dashboard to Room    | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      | :heavy_check_mark: |
| Edit any dashboard in Room   | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |         -          |      -      | :heavy_check_mark: |
| Edit own dashboard in Room   | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      | :heavy_check_mark: |
| Delete any dashboard in Room | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |         -          |      -      | :heavy_check_mark: |
| Delete own dashboard in Room | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      | :heavy_check_mark: |

### Functions

| **Functionality** | **Admin** | **Manager** | **Troubleshooter** | **Observer** | **Billing** | **Member** | Notes |
| :-- | :--: | :--: | :--: | :--: | :--: |  :--: | :-- |
| See all functions in Room | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - | :heavy_check_mark: |
| Run any function in Room | :heavy_check_mark: | :heavy_check_mark: | - | - | - | - |
| Run read-only function in Room | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | - | :heavy_check_mark: |  |
| Run sensitive function in Room | :heavy_check_mark: | :heavy_check_mark: | - | - | - | - | There isn't any function on this category yet, so subject to change. |

### Events feed

| **Functionality**            |     **Admin**      |    **Manager**     | **Troubleshooter** |    **Observer**    | **Billing** |     **Member**     | Notes                                          |
|:-----------------------------|:------------------:|:------------------:|:------------------:|:------------------:|:-----------:|:------------------:|:-----------------------------------------------|
| See Alert or Topology events | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      | :heavy_check_mark: |                                                |
| See Auditing events          | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      |         -          | These are coming soon, not currently available |

### Billing

| **Functionality**          |     **Admin**      | **Manager** | **Troubleshooter** | **Observer** |    **Billing**     | **Member** | Notes                                                           |
|:---------------------------|:------------------:|:-----------:|:------------------:|:------------:|:------------------:|:----------:|:----------------------------------------------------------------|
| See Plan & Billing details | :heavy_check_mark: |      -      |         -          |      -       | :heavy_check_mark: |     -      | Current plan and usage figures                                  |
| Update plans               | :heavy_check_mark: |      -      |         -          |      -       |         -          |     -      | This includes cancelling current plan (going to Community plan) |
| See invoices               | :heavy_check_mark: |      -      |         -          |      -       | :heavy_check_mark: |     -      |                                                                 |
| Manage payment methods     | :heavy_check_mark: |      -      |         -          |      -       | :heavy_check_mark: |     -      |                                                                 |
| Update billing email       | :heavy_check_mark: |      -      |         -          |      -       | :heavy_check_mark: |     -      |                                                                 |

### Other permissions

| **Functionality**          |     **Admin**      |    **Manager**     | **Troubleshooter** |    **Observer**    | **Billing** |     **Member**     |
|:---------------------------|:------------------:|:------------------:|:------------------:|:------------------:|:-----------:|:------------------:|
| See Bookmarks in Space     | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      | :heavy_check_mark: |
| Add Bookmark to Space      | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |         -          |      -      | :heavy_check_mark: |
| Delete Bookmark from Space | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |         -          |      -      | :heavy_check_mark: |
| See Visited Nodes          | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      | :heavy_check_mark: |
| Update Visited Nodes       | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |      -      | :heavy_check_mark: |
