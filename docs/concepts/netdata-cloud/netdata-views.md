<!--
title: "Netdata Views"
sidebar_label: "Netdata Views"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-cloud/netdata-views.md"
sidebar_position: "1800"
learn_status: "Unpublished"
learn_topic_type: "Concepts"
learn_rel_path: "Concepts/Netdata cloud"
learn_docs_purpose: "Present the Netdata cloud's views/tabs, not focusing on dashboards which we explain them in depth in visualizations"
-->

### What is a view

Views are "parts" of a [War Room](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-cloud/rooms.md).
Each view focus on a particular area/subject of the nodes which you monitor in this War Rooms. Views are represented by
**Tabs** in you Netdata Cloud Space/War Room.

### Static Views/Tabs

Every War Rooms provides multiple static views

- The default view for any War Room is the **Home** tab, which gives you an overview
  of this space. Here you can see the number of Nodes claimed, data retention statics, user participate, alerts and more

- The second and most important view is the **Overview** tab which uses composite
  charts to display real-time metrics from every available node in a given War Room.

- The **Nodes** tab gives you the ability to see the status (offline or online), host details
  , alarm status and also a short overview of some key metrics from all your nodes at a glance.

- **Kubernetes** tab is a logical grouping of charts regards to your Kubernetes clusters.
  It contains a subset of the charts available in the _Overview tab_

- The **Dashboards** tab gives you the ability to have tailored made views of
  specific/targeted interfaces for your infrastructure using any number of charts from any number of nodes.

- The **Alerts** tab provides you with an overview for all the active alerts you receive for the nodes in this War Room,
  you can also see alla the alerts that are configured to be triggered in any given moment.

- The **Anomalies** tab is dedicated to
  the [Anomaly Advisor](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-architecture/guided-troubleshooting.md#anomaly-advisor)
  tool

### Non-static Tabs

If you open a new dashboard, jump to a single-node dashboard, or navigate to a dedicated alert page, each of these will
open in a new War Room tab.

Tabs can be rearranged with drag-and-drop or closed with the **X** button. Open tabs persist between sessions, so you
can always come right back to your preferred setup.

### Home Tab

The Home tab provides a predefined dashboard of relevant information about entities in the War Room.

This tab will
automatically present summarized information in an easily digestible display. You can see information about your
nodes, data collection and retention stats, alerts, users and dashboards.

### Overview Tab

The Overview tab is another great way to monitor infrastructure using Netdata Cloud. While the interface might look
similar to local
dashboards served by an Agent, or even the single-node dashboards in Netdata Cloud, Overview uses **composite charts**.
These charts display real-time aggregated metrics from all the nodes (or a filtered selection) in a given War Room.

With Overview's composite charts, you can see your infrastructure from a single pane of glass, discover trends or
anomalies, then drill down by grouping metrics by node and jumping to single-node dashboards for root cause analysis.

### Nodes Tab

### Kubernetes Tab

### Dashboards Tab

### Alerts Tab 

### Anomalies Tab


## Related topics

### **Related Concepts**

- [Spaces](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-cloud/spaces.md)
- [Rooms](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-cloud/rooms.md)
- [Dashboards](https://github.com/netdata/netdata/blob/master/docs/concepts/visualizations/dashboards.md)
- [From raw metrics to visualizations](https://github.com/netdata/learn/blob/master/docs/concepts/visualizations/from-raw-metrics-to-visualization.md)

### Related Tasks

- [Claiming an existing agent to Cloud](https://github.com/netdata/netdata/blob/master/docs/tasks/setup/claim-existing-agent-to-cloud.md)
- [Interact with charts](https://github.com/netdata/netdata/blob/master/docs/tasks/operations/interact-with-the-charts.md)

