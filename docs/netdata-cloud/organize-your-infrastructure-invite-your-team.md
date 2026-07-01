# Spaces and Rooms

This guide explains how you can effectively organize your infrastructure monitoring using Netdata Cloud.

## Overview

You can organize your monitoring with two primary concepts that work together.

**Spaces** serve as your primary collaboration environment where you organize team members and manage access levels, connect nodes for monitoring, and create a unified monitoring environment.

**Rooms** function as organizational units within Spaces that provide infrastructure-wide dashboards, real-time metrics visualization, focused monitoring views, and flexible node grouping.

**Key Relationship:** Each node can only belong to **one** Space, but you can assign a node to **multiple** Rooms within that Space.

## Getting Started

### Create Your Space

1. Use the left-most sidebar to switch between Spaces
2. Click the plus (**+**) icon to create a new Space

:::tip

You can create multiple Spaces, but we recommend using a single Space for most use cases. All team members in a Space can access its monitoring data based on their assigned roles.

:::

### Set Up Team Access

1. Click "Invite Users" in the Space's sidebar
2. Assign a role to each invited user. Space-level roles determine what someone can do across the Space:

    - **Admin** — full control of the Space, including managing users, Rooms, nodes, notifications, and billing, plus access to every Room. Assign **Admin** to give the invited user the same level of control over the Space that you have.
    - **Manager** — manage users, Rooms, and most configuration, but cannot manage billing or assign the Admin role.
    - **Troubleshooter** — investigate issues and build dashboards in the Rooms they are assigned to, without managing the Space.
    - **Observer** — view-only access to specific Rooms.
    - **Billing** — manage invoices and payments, without access to monitoring management.

   Which roles you can assign depends on your plan. Only an existing Admin can grant the Admin role, so you must already be an Admin to assign it to someone else. Role assignments take effect immediately.

   For the full permission matrix and role availability by plan, see the [role-based access model](/docs/netdata-cloud/authentication-and-authorization/role-based-access-model.md).

:::tip

**Best Practices for Team Access:** Invite all relevant team members (SRE, DevOps, ITOps) and configure role-based access control. Maintain clear permission hierarchies and conduct regular access reviews and updates.

:::

### Create Your First Room

1. Access Rooms through the Space's sidebar
2. Click the green plus (**+**) icon next to "Rooms" to create new Rooms

   <img src="https://github.com/user-attachments/assets/16958ba8-53ac-4e78-a51f-7ea328e97f31" height="400px" alt="Individual Space sidebar"/>

:::info

All nodes automatically appear in the "All nodes" Room. Each Room has independent dashboards and monitoring tools.

:::

## Organize Your Infrastructure

### Room Organization Strategies

| Strategy                              | Use Case                                                                                    | Examples                                                                                                                                                                                                                        |
|---------------------------------------|---------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Service-Based Organization**        | Group nodes by **specific services**, **purpose**, **location**, or **infrastructure type** | **Nginx**, **MySQL**, **Pulsar**, **webserver**, **database**, **application**, **physical location**, **bare metal**, **containers**, **cloud provider**                                                                       |
| **End-to-End Application Monitoring** | Create Rooms for **complete application stacks** and **service dependencies**               | **Complete SaaS product stacks**, **internal service dependencies**, **full application ecosystems** including **Kubernetes clusters**, **Docker containers**, **Proxies**, **Databases**, **Web servers**, **Message brokers** |
| **Incident Response**                 | Create dedicated Rooms for **troubleshooting** and **problem resolution**                   | **Active incident investigation**, **problem diagnosis**, **performance troubleshooting**, **root cause analysis**                                                                                                              |

## Manage Your Setup

### Space Configuration

1. Select your Space
2. Click **Space Settings** (⚙️) on the left sidebar below the spaces list
3. Access settings for room management, node configuration, integration setup, and general Space settings

### Room Configuration

1. Click the ⚙️ next to the Room name
2. Manage room access, node grouping, dashboard settings, and monitoring configurations

### Leaving a Room

Any user with Room access — **Admin**, **Manager**, **Troubleshooter**, or **Observer** — can leave a Room. **Billing** users do not have Room access and cannot leave a Room.

Leaving a Room removes your access to that Room's nodes, dashboards, metrics, and functions. It does not delete the Room or affect other members.

Rejoining a Room depends on your role:

- **Admins** and **Managers** can rejoin any Room at any time on their own.
- **Troubleshooters** and **Observers** cannot rejoin on their own and must be re-added to the Room by an Admin or Manager.

For the full permission breakdown, see the [Room Management table in the RBAC reference](/docs/netdata-cloud/authentication-and-authorization/role-based-access-model.md).

### Delete a Room

Only **Admins** and **Managers** can delete a Room. See the [Room Management table in the RBAC reference](/docs/netdata-cloud/authentication-and-authorization/role-based-access-model.md) for the full permissions matrix.

:::warning

Deleting a Room is permanent and cannot be undone. The Room and its dashboards are removed.

:::

The nodes in a deleted Room are not deleted — they remain available in the "All nodes" Room and any other Rooms they belong to within the Space.

**Steps to delete a Room:**

1. Click the ⚙️ next to the Room name to open the Room's settings.
2. Click **Delete Room**.

### Delete a Space

Only users with the **Admin** role can delete a Space. See the [role-based access model](/docs/netdata-cloud/authentication-and-authorization/role-based-access-model.md) for full permission details.

:::warning

Deleting a Space is permanent and cannot be undone. You cannot delete the only Space on your account — each account must have at least one Space.

:::

**Steps to delete a Space:**

1. Navigate to **Space Settings** (⚙️) on the left sidebar below the spaces list.
2. Select the **Info** tab.
3. Click **Delete Space**.
