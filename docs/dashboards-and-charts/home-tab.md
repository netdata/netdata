# Home tab

The **Home tab** offers a **real-time overview of your Room** in Netdata Cloud. Use it to quickly assess your infrastructure's health and key metrics at a glance.

| Feature | Description |
|---------|-------------|
| **Total nodes** | Shows the total number of nodes, broken down by state: **Live**, **Offline**, or **Stale** |
| **Active alerts** | Displays the number of active alerts in a donut chart, with counters for both **Critical** and **Warning** alerts |
| **Nodes map** | Interactive hexagonal map with color-coded node statuses and hoverable details. Classify nodes by **Status**, **OS**, **Technology**, **Agent version**, **Replication factor**, **Cloud provider**, **Cloud region**, **Instance type**, or custom **Labels**. Configure color-coding by **Status** or **Connection stability** |
| **Parents/Children/Standalone** | Shows node role distribution across your infrastructure, breaking down how many nodes operate as Parents, Children, or Standalone |
| **Data replication** | Displays replication factors across your nodes categorized as **None**, **Single**, and **Multi** for high availability monitoring |
| **Nodes with the most alerts (last 24h)** | Bar chart showing the top nodes with the highest alert count in the last 24 hours, broken down by **Warning** and **Critical** severity. Summary shows total Warning and Critical alerts across all nodes |
| **Top alerts (last 24h)** | Table of the most frequently triggered alerts, with instance name, number of occurrences, and total duration (in seconds). Sortable by occurrences to identify persistent issues |
| **Space metrics** | Displays key statistics: **Metrics available**, **Charts visualized**, and **Alerts configured** across your entire Space |
| **Data retention per node** | Bar chart showing the number of nodes grouped by retention period (less than 1 week, 1-2 weeks, 2-4 weeks, 1-3 months, 3-6 months, 6-12 months, 1-2 years, more than 2 years), helping you assess historical data availability across your infrastructure |

:::note

The Home tab provides critical infrastructure visibility through real-time node status, alert trends, and deployment topology. The **Parents/Children/Standalone** breakdown shows your streaming architecture at a glance, helping you understand your data collection hierarchy. The **Data replication** metrics indicate how many nodes have no replication, single replication, or multi-replication for high availability. Use the **Nodes with the most alerts** chart to quickly identify problematic hosts requiring immediate attention, and the **Top alerts** table to spot recurring issues that may need alert tuning or infrastructure remediation.

:::

:::tip

Use the Home tab regularly to stay ahead of infrastructure issues and monitor alert trends at a glance.

:::
