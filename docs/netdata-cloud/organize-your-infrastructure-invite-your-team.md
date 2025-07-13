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
2. Set appropriate access levels:
    - Rooms
    - User roles

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
2. Click the ⚙️ in the lower left corner
3. Access settings for room management, node configuration, integration setup, and general Space settings

### Room Configuration

1. Click the ⚙️ next to the Room name
2. Manage room access, node grouping, dashboard settings, and monitoring configurations
