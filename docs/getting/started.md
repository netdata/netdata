# Getting Started with Netdata

Welcome to Netdata! This guide will help you set up comprehensive infrastructure monitoring in just a few steps. You'll go from installation to having a fully operational monitoring system that gives you insights into your entire infrastructure.

:::warning

**People get addicted to Netdata**. Once you start using it on your systems, there's no going back.

:::

## Quick Start: 4 Essential Steps

### Step 1: Install Netdata

You can install Netdata on all major operating systems. Choose your platform and follow the installation guide:

- <span style="color: green;">[Linux Installation](https://learn.netdata.cloud/docs/netdata-agent/installation/linux/)</span>
- <span style="color: green;">[macOS](https://learn.netdata.cloud/docs/netdata-agent/installation/macos)</span>
- <span style="color: green;">[AWS](https://learn.netdata.cloud/docs/netdata-agent/installation/aws)</span>
- <span style="color: green;">[Windows](https://learn.netdata.cloud/docs/netdata-agent/installation/windows)</span>
- <span style="color: green;">[Docker Guide](https://learn.netdata.cloud/docs/netdata-agent/installation/docker)</span>
- <span style="color: green;">[Kubernetes Setup](https://learn.netdata.cloud/docs/netdata-agent/installation/kubernetes)</span>

:::note
You can access the Netdata UI at `http://localhost:19999` (or `http://NODE:19999` if remote).
:::

### Step 2: Connect Your Nodes & See the Magic

Whether it's a **server, container, or virtual machine, Netdata makes it easy to bring your infrastructure under observation**. Once you install the Agent, you'll immediately see your system metrics flowing in.

:::info

**Auto-Discovery in Action**: Netdata automatically discovers most system components and starts monitoring them without any configuration. Your dashboards populate instantly with CPU, memory, disk, network, and application metrics.

:::

### Step 3: Configure Data Collection & Integrations

Netdata auto-discovers most metrics, but you can expand monitoring with our extensive integration library:

- <span style="color: green;">[All Collectors](https://learn.netdata.cloud/docs/collecting-metrics/)</span> - Browse 1000+ integrations
- <span style="color: green;">[SNMP Monitoring](https://learn.netdata.cloud/docs/collecting-metrics/generic-collecting-metrics/snmp-devices)</span> - Monitor network devices
- <span style="color: green;">[Custom Applications](https://learn.netdata.cloud/docs/collecting-metrics/monitor-anything)</span> - Monitor your specific applications

:::tip

**Netdata auto-discovers most system components**. For everything else, explore our 1000+ integrations to monitor your entire network, databases, web servers, and custom applications.

:::

### Step 4: Set Up Alerts & Notifications

You can use hundreds of built-in alerts and integrate with your preferred notification channels:

**Configure Alert Notifications**:
- `Email` (works by default with configured MTA)
- `Slack`
- `Telegram` 
- `PagerDuty`
- `Discord`
- `Microsoft Teams`
- <span style="color: green;">[And dozens more](https://learn.netdata.cloud/docs/alerts-&-notifications/alert-configuration-reference)</span>

**Customize Your Alerts**: Explore and customize alerts to ensure you and your team receive the most relevant notifications for your infrastructure.

:::note

**Email alerts work by default** if there's a configured MTA. You can customize alert thresholds and notification methods to fit your exact monitoring needs.

:::

<details>
<summary><strong>Advanced Setup (Optional)</strong></summary><br/>

### Centralize with Netdata Parents

You can centralize dashboards, alerts, and storage with Netdata Parents for:

- Central dashboards across multiple nodes
- Longer data retention
- Centralized alert configuration
- Reduced resource usage on monitored systems

Check our <span style="color: green;">[Deployment Guides](https://learn.netdata.cloud/docs/deployment-guides/)</span> for more info.

### Connect to Netdata Cloud

Sign in to Netdata Cloud and connect your nodes for enhanced capabilities:

| **Key Features** | Description |
|------------------|-------------|
| **Access from Anywhere** | Monitor your infrastructure remotely |
| **Horizontal Scalability** | Multi-node dashboards and views |
| **Team Collaboration** | Organize infrastructure and invite team members |
| **Advanced Features** | UI configuration for alerts and data collection |
| **Role-based Access Control** | Manage team permissions |
| **Free Tier Available** | Get started without cost |

**Organize Your Infrastructure**: Group your infrastructure into Spaces and Rooms based on location, service, or team. Invite your teammates for seamless online collaboration.

:::important

**Netdata Cloud is optional. Your data stays in your infrastructure**. We provide the interface and collaboration features while your metrics remain under your control.

:::

</details>

## What's Next?

Once you have Netdata running, you can:

1. **<span style="color: green;">[Explore your dashboards](https://learn.netdata.cloud/docs/dashboards-and-charts/)</span>** - Navigate through the automatically generated charts and metrics
2. **<span style="color: green;">[Set up custom alerts](https://learn.netdata.cloud/docs/alerts-&-notifications/alert-configuration-reference)</span>** - Tailor notifications to your specific infrastructure needs  
3. **<span style="color: green;">[Invite your team](https://www.netdata.cloud/blog/introducing-the-all-new-netdata-cloud/)</span>** - Share insights and troubleshoot collaboratively
4. **<span style="color: green;">[Optimize performance](https://learn.netdata.cloud/docs/netdata-agent/configuration/performance-optimization)</span>** - Use insights to improve your system performance

:::tip

Check out our <span style="color: green;">[documentation](https://learn.netdata.cloud/docs/deployment-guides)</span> or join our <span style="color: green;">[community forums](https://community.netdata.cloud/)</span> for support and best practices.

:::