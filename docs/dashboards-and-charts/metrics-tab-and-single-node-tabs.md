# Metrics Tab and Single Node Tabs

The **Metrics tab** provides real-time, per-second time series charts for all nodes in a Room. It helps you visualize, explore, and troubleshoot metrics across your entire infrastructure in one place.

You can also view **single-node dashboards**, which offer the same charts but are focused on a single node. You can access these dashboards from most places in the Netdata UI, often by clicking the name of a node.

From the Metrics tab, you can also access:

- The **Integrations tab**
- **Metric Correlations** to identify related metrics and uncover patterns across your infrastructure

:::note

Learn more: [Metric Correlations documentation](/docs/metric-correlations.md)

:::

## Metrics Tab Structure Overview

```mermaid
flowchart TD
    A("Metrics Tab - Multi-node")
    A --> B("Integrations Tab")
    A --> C("Metric Correlations")
    A --> D("Single Node Tabs")
    D --> E("Node-specific charts")

    %% Style definitions
    classDef alert fill:#ffeb3b,stroke:#000000,stroke-width:3px,color:#000000,font-size:14px
    classDef neutral fill:#f9f9f9,stroke:#000000,stroke-width:3px,color:#000000,font-size:14px
    classDef complete fill:#4caf50,stroke:#000000,stroke-width:3px,color:#000000,font-size:14px
    classDef database fill:#2196F3,stroke:#000000,stroke-width:3px,color:#000000,font-size:14px

    %% Apply styles
    class A alert
    class B,C neutral
    class D,E complete
```

:::tip

The diagram above shows how the Metrics tab connects to related features and single-node dashboards, making it easier to navigate between views.

:::

## How the Dashboard is Organized

The dashboard displays various charts organized by their [context](/docs/dashboards-and-charts/netdata-charts.md#contexts). At the beginning of each section, there is a predefined arrangement of charts that provides an overview for that particular group of metrics.

The available chart types and grouping options allow flexible data visualization for troubleshooting and analysis.

:::tip

Use the chart arrangement at the start of each section to quickly identify patterns, spikes, or anomalies before diving into detailed chart filtering.

:::

## System Overview

The **System Overview** is the predefined section that appears at the top of every single-node dashboard. It populates automatically as soon as a node connects — there is nothing to configure — and gives you an at-a-glance summary of the node's core resources before you scroll into the detailed, per-topic sections below. You'll also find it in the right-hand [Chart Navigation Menu](#chart-navigation-menu).

The section opens with gauges and charts that summarize overall node health, including CPU utilization, memory, and network activity. Notable items include:

- **Net Inbound** and **Net Outbound** — the two network gauges, showing the aggregate rate of data received and sent across all of the node's physical network interfaces. A sudden, sustained change is often the first sign of a new workload, a backup or sync job, or unexpected traffic worth investigating.
- **CPU utilization** — how busy the node's processors are. Sustained saturation is a cue to check which processes are consuming CPU in the sections below.

:::note

The exact gauges and charts in the System Overview depend on what the node's hardware and enabled collectors expose, so you may see additional resource summaries beyond these.

:::

Use these at-a-glance indicators to decide where to dig in next: a spike in network traffic or CPU directs you to the matching detailed section for root-cause analysis.

## Chart Navigation Menu

The **Chart Navigation Menu**, located on the right-hand side of the dashboard, helps you navigate through sections, filter charts, and view active alerts.

| Feature                       | Description                                                                                                    |
|-------------------------------|----------------------------------------------------------------------------------------------------------------|
| **Section Navigation**        | Navigate quickly through the dashboard sections.                                                               |
| **Chart Filtering Options**   | Filter charts by:  <br/> - Host labels  <br/> - Node status  <br/> - Netdata version  <br/> - Individual nodes |
| **Active Alerts Display**     | View active alerts for the Room.                                                                               |
| **Anomaly Rate (AR%) Button** | Check the maximum chart anomaly rate for each section by clicking the `AR%` button.                            |

:::tip

Use chart filtering to reduce visual noise and focus on the nodes, labels, or statuses that matter most to your investigation.

:::

:::tip

The **AR% button** shows the maximum anomaly rate for each dashboard section, helping you quickly identify where issues may be occurring.

:::