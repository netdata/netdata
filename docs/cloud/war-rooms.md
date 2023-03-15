# War rooms

War Rooms organize your connected nodes and provide infrastructure-wide dashboards using real-time metrics and
visualizations.

Once you add nodes to a Space, all of your nodes will be visible in the **All nodes** War Room. This is a special War Room
which gives you an overview of all of your nodes in this particular Space. Then you can create functional separations of
your nodes into more War Rooms. Every War Room has its own dashboards, navigation, indicators, and management tools.

![An example War Room](https://user-images.githubusercontent.com/70198089/225249330-46a252c5-ce95-4cac-a6b5-3223f31ed74c.png)

## Navigation

### Switching between views - static tabs

Every War Room provides multiple views. Each view focuses on a particular area/subject of the nodes which you monitor in this War Room.
Let's explore what views you have available:

- The default view for any War Room is
  the [Home tab](https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/overview.md#home), which gives you an overview of this Space.
  Here you can see the number of Nodes claimed, data retention statics, users by role, alerts and more.

- The second and most important view is the [Overview tab](https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/overview.md#overview-and-single-node-view) which uses composite charts to display real-time metrics from every available node in a given War Room.

- The [Nodes tab](https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/nodes.md) gives you the ability to see the status (offline or online), host details, alarm status and also a short overview of some key metrics from all your nodes at a glance.

- [Kubernetes tab](https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/kubernetes.md) is a logical grouping of charts regarding your Kubernetes clusters. It contains a subset of the charts available in the **Overview tab**.

- The [Dashboards tab](https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/dashboards.md) gives you the ability to have tailored made views of specific/targeted interfaces for your infrastructure using any number of charts from any number of nodes.

- The [Alerts tab](https://github.com/netdata/netdata/blob/master/docs/monitor/view-active-alarms.md) provides you with an overview for all the active alerts you receive for the nodes in this War Room, you can also see all the alerts that are configured to be triggered in any given moment.

- The [Anomalies tab](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/anomaly-advisor.md) is dedicated to the Anomaly Advisor tool.

- The [Functions tab](https://github.com/netdata/netdata/blob/master/docs/cloud/netdata-functions.md) gives you the ability to visualize functions that the Netdata Agent collectors are able to expose.

- The [Feed & events](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/events-feed.md) tab lets you investigate events that occurred in the past, which is invaluable for troubleshooting.

### Non static tabs

If you open a [new dashboard](https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/dashboards.md), jump to a single-node dashboard, or navigate to a dedicated alert page they will open in a new War Room tab.

Tabs can be rearranged with drag-and-drop or closed with the **X** button. Open tabs persist between sessions, so you can always come right back to your preferred setup.

### Play, pause, force play, and timeframe selector

A War Room has three different states: playing, paused, and force playing. The default playing state refreshes charts every second as long as the browser tab is in focus.

[Interacting with a chart](https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/interact-new-charts.md) pauses the War Room. Once the tab loses focus, charts pause automatically.

The top navigation bar features a play/pause button to quickly change the state, and a dropdown to select **Force Play**, which keeps charts refreshing, potentially at the expense of system performance.

Next to the play/pause button is the timeframe selector, which helps you select a precise window of metrics data to visualize. By default, all visualizations in Netdata Cloud show the last 15 minutes of metrics data.

Use the **Quick Selector** to visualize metrics from predefined timeframes, or use the input field below to enter a number and an appropriate unit of time. The calendar allows you to select multiple days of metrics data.

Click **Apply** to re-render all visualizations with new metrics data streamed to your browser from each distributed node. Click **Clear** to remove any changes and apply the default 15-minute timeframe.

The fields beneath the calendar display the beginning and ending timestamps your selected timeframe.

### Node filter

The node filter allows you to quickly filter the nodes visualized in a War Room's views. It appears on all views, but not on single-node dashboards.

![The node filter](https://user-images.githubusercontent.com/70198089/225249850-60ce4fcc-4398-4412-a6b5-6082308f4e60.png)

## War Room organization

We recommend a few strategies for organizing your War Rooms.

- **Service, purpose, location, etc.**  
   You can group War Rooms by a service (Nginx, MySQL, Pulsar, and so on), their purpose (webserver, database, application), their physical location, whether they're "bare metal" or a Docker container, the PaaS/cloud provider it runs on, and much more.
   This allows you to see entire slices of your infrastructure by moving from one War Room to another.

- **End-to-end apps/services**  
  If you have a user-facing SaaS product, or an internal service that this said product relies on, you may want to monitor that entire stack in a single War Room. This might include Kubernetes clusters, Docker containers, proxies, databases, web servers, brokers, and more.
  End-to-end War Rooms are valuable tools for ensuring the health and performance of your organization's essential services.

- **Incident response**  
  You can also create new War Rooms as one of the first steps in your incident response process.
   For example, you have a user-facing web app that relies on Apache Pulsar for a message queue, and one of your nodes using the [Pulsar collector](https://github.com/netdata/go.d.plugin/blob/master/modules/pulsar/README.md) begins reporting a suspiciously low messages rate.
   You can create a War Room called `$year-$month-$day-pulsar-rate`, add all your Pulsar nodes in addition to nodes they connect to, and begin diagnosing the root cause in a War Room optimized for getting to resolution as fast as possible.

## Add War Rooms

To add new War Rooms to any Space, click on the green plus icon **+** next the **War Rooms** heading on the left (Space's) sidebar.

In the panel, give the War Room a name and description, and choose whether it's public or private.
Anyone in your Space can join public War Rooms, but can only join private War Rooms with an invitation.

## Manage War Rooms

All the users and nodes involved in a particular Space can be part of a War Room.

Any user can change simple settings of a War room, like the name or the users participating in it.
Click on the gear icon of the War Room's name in the top of the page to do that. A sidebar will open with options for this War Room:

1. To **change a War Room's name, description, or public/private status**, click on **War Room** tab.

2. To **include an existing node** to a War Room or **connect a new node\*** click on **Nodes** tab. Choose any connected node you want to add to this War Room by clicking on the checkbox next to its hostname, then click **+ Add** at the top of the panel.

3. To **add existing users to a War Room**, click on **Add Users**.
   See our [invite doc](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/invite-your-team.md) for details on inviting new users to your Space in Netdata Cloud.

> ### Note
>
>\* This action requires **admin** rights for this Space

### More actions

To **view or remove nodes** in a War Room, click on **Nodes view**. To remove a node from the current War Room, click on
the **🗑** icon.

> ### Info
>
> Removing a node from a War Room does not remove it from your Space.
