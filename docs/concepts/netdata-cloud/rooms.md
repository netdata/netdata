<!--
title: "War Rooms"
sidebar_label: "War Rooms"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-cloud/rooms.md"
sidebar_position: "1700"
learn_status: "Published"
learn_topic_type: "Concepts"
learn_rel_path: "Concepts/Netdata cloud"
learn_docs_purpose: "Present the concept of a War Room, it's purpose and use cases"
-->

### War Room definition

War Rooms organize your connected nodes and provide infrastructure-wide dashboards using real-time metrics and
visualizations.

Once you add nodes to a Space, all of your nodes will be visible in the All nodes War Room. This is a special War Room
which gives you an overview of all of your nodes in this particular space. Then you can create functional separations of
your nodes into more War Rooms. Every War Room has its own dashboards, navigation, indicators, and management tools.

### **All nodes** War room

The **All nodes** War room is an unique War Room for your each of your Spaces. All the Nodes that are claimed to a
particular
space belong to this War Room So you have a comprehensive overview of the Nodes in your Space.

### Play, pause, force play, and timeframe selector

A War Room has three different states: playing, paused, and force playing. The default playing state refreshes charts
every second as long as the browser tab is in
focus. [Interacting with a chart](https://github.com/netdata/netdata/blob/master/docs/tasks/operations/interact-with-the-charts.md)
pauses
the War Room. Once the tab loses focus, charts pause automatically.

The top navigation bar features a play/pause button to quickly change the state, and a dropdown to select **Force Play**
, which keeps charts refreshing, potentially at the expense of system performance.

Next to the play/pause button is the timeframe selector, which helps you select a precise window of metrics data to
visualize. By default, all visualizations in Netdata Cloud show the last 15 minutes of metrics data.

Use the **Quick Selector** to visualize metrics from predefined timeframes, or use the input field below to enter a
number and an appropriate unit of time. The calendar allows you to select multiple days of metrics data.

Click **Apply** to re-render all visualizations with new metrics data streamed to your browser from each distributed
node. Click **Clear** to remove any changes and apply the default 15-minute timeframe.

The fields beneath the calendar display the beginning and ending timestamps your selected timeframe.

### Node filter

The node filter allows you to quickly filter the nodes visualized in a War Room's views. It appears on all views, but
not on single-node dashboards.

![The node filter](https://user-images.githubusercontent.com/12612986/172674440-df224058-2b2c-41da-bb45-f4eb82e342e5.png)

### War Room Organization

You are free to organize your War Rooms to your liking. We do, however, offer a few recommended trategies for organizing
your War Rooms.

**Service, purpose, location, etc.**: You can group War Rooms by a service (think Nginx, MySQL, Pulsar, and so on),
their purpose (webserver, database, application), their physical location, whether they're baremetal or a Docker
container, the PaaS/cloud provider it runs on, and much more. This allows you to see entire slices of your
infrastructure by moving from one War Room to another.

**End-to-end apps/services**: If you have a user-facing SaaS product, or an internal service that said product relies
on, you may want to monitor that entire stack in a single War Room. This might include Kubernetes clusters, Docker
containers, proxies, databases, web servers, brokers, and more. End-to-end War Rooms are valuable tools for ensuring the
health and performance of your organization's essential services.

**Incident response**: You can also create new War Rooms as one of the first steps in your incident response process.
For example, you have a user-facing web app that relies on Apache Pulsar for a message queue, and one of your nodes
using the Pulsar collector begins reporting a suspiciously low
messages rate. You can create a War Room called `$year-$month-$day-pulsar-rate`, add all your Pulsar nodes in addition
to nodes they connect to, and begin diagnosing the root cause in a War Room optimized for getting to resolution as fast
as possible.

### Bookmarks for  essential resources

When an anomaly or outage strikes, your team needs to access other essential resources quickly. You can use Netdata
Cloud's bookmarks to put these tools in one accessible place. Bookmarks are shared between all War Rooms in a Space, so
any users in your Space will be able to see and use them.

Bookmarks can link to both internal and external resources. You can bookmark your app's status page for quick updates
during an outage, a messaging system on your organization's intranet, or other tools your team uses to respond to
changes in your infrastructure.

To add a new bookmark, click on the **Add bookmark** link. In the panel, name the bookmark, include its URL, and write a
short description for your team's reference.

## Related Topics

### **Related Concepts**

- [Spaces](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-cloud/spaces.md)
- [Netdata Views](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-cloud/netdata-views.md)
- [Dashboards](https://github.com/netdata/netdata/blob/master/docs/concepts/visualizations/dashboards.md)
- [From raw metrics to visualizations](https://github.com/netdata/master/blob/master/docs/concepts/visualizations/from-raw-metrics-to-visualization.md)

### Related Tasks

- [Room Management](https://github.com/netdata/netdata/blob/master/docs/tasks/setup/space-administration/rooms.md)
- [Claiming an existing agent to Cloud](https://github.com/netdata/netdata/blob/master/docs/tasks/setup/claim-existing-agent-to-cloud.md)
- [Interact with charts](https://github.com/netdata/netdata/blob/master/docs/tasks/operations/interact-with-the-charts.md)
