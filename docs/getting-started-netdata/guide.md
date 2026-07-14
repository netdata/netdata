# Getting Started with Netdata

Use this guide to install Netdata, connect your first node, explore its data, configure alerts, and organize access for your team.

You can use the Netdata Agent dashboard directly or connect Agents to Netdata Cloud for a centralized view of multiple nodes.

## 1. Install or Connect a Node

Choose the path that matches your deployment:

- [Install the Netdata Agent](/packaging/installer/README.md) on a new node.
- [Connect an existing Agent to Netdata Cloud](/src/claim/README.md).
- Open a standalone Agent dashboard at `http://NODE:19999` if you do not want to connect it to Cloud.

When using Netdata Cloud, sign in at [app.netdata.cloud](https://app.netdata.cloud/) and follow the connection instructions shown in your Space. Keep generated claim tokens private.

## 2. Confirm Data Collection

After the Agent starts, verify that the node appears online and charts begin to populate. Netdata automatically collects system metrics and discovers many supported services without manual dashboard configuration.

Start with:

- CPU, memory, disk, and network charts.
- The [Nodes tab](/docs/dashboards-and-charts/nodes-tab.md) for infrastructure status.
- The [Metrics tab and node dashboards](/docs/dashboards-and-charts/metrics-tab-and-single-node-tabs.md) for detailed charts.
- The [collectors catalog](/src/collectors/COLLECTORS.md) to enable additional integrations.

Collection frequency and available charts depend on the collector and its configuration.

## 3. Investigate Your Data

Use [Dashboards and Charts](/docs/dashboards-and-charts/README.md) to move from an infrastructure overview to individual metrics. During an investigation, you can also use:

- [Metric Correlations](/docs/metric-correlations.md) to find metrics that changed during a selected period.
- [Netdata Functions](/docs/top-monitoring-netdata-functions.md) for on-demand process, database, network, log, and system information.
- [Machine learning and assisted troubleshooting](/docs/category-overview-pages/machine-learning-and-assisted-troubleshooting.md) for anomaly detection and AI-assisted workflows.

## 4. Review and Configure Alerts

Netdata ships with alert definitions for common infrastructure problems. Review active alerts first, then adjust only the definitions that do not match your environment.

- Use the [Alerts Configuration Manager](/docs/alerts-and-notifications/creating-alerts-with-netdata-alerts-configuration-manager.md) where available.
- Use the [alert configuration reference](/src/health/REFERENCE.md) for file-based configuration.
- Configure [alert notifications](/docs/alerts-and-notifications/notifications/README.md) for the services your team uses.

Before disabling a stock alert, identify which stock definition provides it and use the supported override mechanism. Editing or deleting packaged definitions can be lost during upgrades.

## 5. Organize Nodes and Team Access

In Netdata Cloud, use Spaces and Rooms to group nodes by service, environment, location, or team. Invite users and assign the minimum role needed for their work.

See [Organize your infrastructure and invite your team](/docs/netdata-cloud/organize-your-infrastructure-invite-your-team.md) for the current workflow and role descriptions.

## Next Steps

- Compare current Cloud plans and entitlements on the [pricing page](https://www.netdata.cloud/pricing/).
- Review [standalone Agent deployments](/docs/deployment-guides/standalone-deployment.md).
- Review [Netdata Parents](/docs/deployment-guides/deployment-with-centralization-points.md) before centralizing storage or streaming many Agents.
- Explore [Netdata AI](/docs/category-overview-pages/machine-learning-and-assisted-troubleshooting.md) for supported AI and MCP workflows.
