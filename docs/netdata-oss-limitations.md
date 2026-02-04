# Netdata Access Control and Feature Availability

This document explains the access control policies that govern feature availability in Netdata, and how these change based on your authentication and subscription status.

## Overview

Netdata implements a layered access control system to protect sensitive information while keeping core monitoring capabilities freely available. The system distinguishes between three access levels:

| Access Level | Description |
|--------------|-------------|
| **Anonymous** | Using the Netdata dashboard without signing in |
| **Netdata Cloud Community** | Signed in to Netdata Cloud (free tier) |
| **Netdata Cloud Business** | Signed in with a paid subscription |

## Why Access Controls Exist

Netdata functions can expose sensitive system information:

- **Process details** reveal running applications, command-line arguments (which may contain passwords or tokens), and resource consumption patterns
- **Network connections** expose active services, connected clients, and internal network topology
- **System logs** may contain application errors, security events, and debugging information with sensitive context
- **Database queries** can reveal query patterns, table structures, and potentially sensitive data in error messages

Without authentication, anyone who can reach the Netdata dashboard could access this information. The access control system ensures that sensitive data is only available to authenticated users who belong to the same Netdata Cloud Space as the monitored infrastructure.

## Feature Availability by Access Level

### Metrics and Visualization

| Feature | Anonymous | Community | Business |
|---------|:---------:|:---------:|:--------:|
| Real-time metrics (all collectors) | ✓ | ✓ | ✓ |
| Historical data and retention | ✓ | ✓ | ✓ |
| Charts and dashboards | ✓ | ✓ | ✓ |
| Anomaly detection (ML) | ✓ | ✓ | ✓ |
| Alert notifications | ✓ | ✓ | ✓ |
| Multi-node views | 5 nodes | 5 nodes | Unlimited |
| Custom dashboards | 1 per agent | 1 per room | Unlimited |

### Functions (Top Tab)

Functions provide on-demand, detailed information beyond standard metrics.

| Function | Description | Anonymous | Community | Business |
|----------|-------------|:---------:|:---------:|:--------:|
| **Block Devices** | Disk I/O activity | ✓ | ✓ | ✓ |
| **Containers/VMs** | Container and VM resource usage | ✓ | ✓ | ✓ |
| **IPMI Sensors** | Hardware sensor readings | ✓ | ✓ | ✓ |
| **Mount Points** | Disk usage per mount | ✓ | ✓ | ✓ |
| **Network Interfaces** | Interface traffic and status | ✓ | ✓ | ✓ |
| **Systemd Services** | Service resource usage | ✓ | ✓ | ✓ |
| **Processes** | Running processes, command lines, resources | ✗ | ✓ | ✓ |
| **Network Connections** | Active TCP/UDP connections | ✗ | ✓ | ✓ |
| **Systemd Journal** | System and application logs | ✗ | ✓ | ✓ |
| **Windows Events** | Windows event logs | ✗ | ✓ | ✓ |
| **Systemd Units** | Unit status and configuration | ✗ | ✓ | ✓ |
| **Database Queries** | Top queries, deadlocks, errors | ✗ | ✓ | ✓ |
| **Streaming Status** | Netdata streaming topology | ✗ | ✓ | ✓ |
| **API Call Tracing** | Netdata API request tracing | ✗ | ✓ | ✓ |

### Configuration and Management

| Feature | Anonymous | Community | Business |
|---------|:---------:|:---------:|:--------:|
| View agent configuration | ✗ | ✗ | ✓ |
| Dynamic Configuration (collectors) | ✗ | ✗ | ✓ |
| Dynamic Configuration (alerts) | ✗ | ✗ | ✓ |
| Alert silencing rules | ✗ | ✓ | ✓ |
| Notification configuration | ✗ | ✗ | ✓ |

### AI-Powered Features

| Feature | Anonymous | Community | Business |
|---------|:---------:|:---------:|:--------:|
| Alert explanations | ✗ | ✓ | ✓ |
| Alert configuration suggestions | ✗ | ✓ | ✓ |
| AI-powered insights | ✗ | ✓ | ✓ |

### Organization Features

| Feature | Anonymous | Community | Business |
|---------|:---------:|:---------:|:--------:|
| Role-based access control (RBAC) | N/A | ✗ | ✓ |
| Single Sign-On (SSO) | N/A | ✗ | ✓ |
| Team management | N/A | Limited | Full |

## MCP (Model Context Protocol)

Netdata's MCP server is available directly at Netdata Agents and Parents, independent of Netdata Cloud authentication. This allows AI assistants and tools to query metrics and execute functions through the MCP protocol.

When accessing Netdata via MCP:

- **Without Cloud connection**: MCP can access public functions and metrics, but sensitive functions follow the same restrictions as the dashboard
- **With Cloud connection**: MCP inherits the user's Cloud permissions, enabling access to sensitive functions for authenticated users

For MCP setup and configuration, see the [MCP documentation](/docs/netdata-ai/mcp/README.md).

## How to Enable Features

### Enable Sensitive Functions

1. **Sign in to Netdata Cloud** at [app.netdata.cloud](https://app.netdata.cloud)
2. **Connect your nodes** to your Netdata Cloud Space
3. **Access the dashboard** through Netdata Cloud

Once signed in, you'll have access to all sensitive functions (processes, logs, network connections, etc.) on nodes within your Space.

### Enable Dynamic Configuration

Dynamic Configuration requires a Business subscription:

1. **Sign in to Netdata Cloud**
2. **Upgrade to Business** from the billing settings
3. **Access Dynamic Configuration** from the settings menu on any connected node

### Increase Node Limits

The 5-node limit on multi-node dashboards applies to Community plans:

1. **Upgrade to Business** for unlimited nodes
2. **Or select preferred nodes** in Space settings to choose which 5 nodes appear in multi-node views

## Summary

| What You Get | Anonymous | Community | Business |
|--------------|-----------|-----------|----------|
| **Metrics & Charts** | Full access | Full access | Full access |
| **Anomaly Detection** | Full access | Full access | Full access |
| **Alert Notifications** | Full access | Full access | Full access |
| **Public Functions** | Full access | Full access | Full access |
| **Sensitive Functions** | Blocked | Full access | Full access |
| **AI Features** | Blocked | Full access | Full access |
| **Dynamic Configuration** | Blocked | Blocked | Full access |
| **Multi-node Limit** | 5 nodes | 5 nodes | Unlimited |
| **Custom Dashboards** | 1 per agent | 1 per room | Unlimited |
| **RBAC & SSO** | N/A | Not available | Full access |

Netdata's access control model ensures that sensitive system information is protected while keeping powerful monitoring capabilities freely available. Sign in to Netdata Cloud to unlock sensitive functions, or upgrade to Business for full configuration control and unlimited scale.
