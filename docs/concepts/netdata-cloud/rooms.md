<!--
title: "Rooms"
sidebar_label: "Rooms"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-cloud/rooms.md"
sidebar_position: "1700"
learn_status: "Published"
learn_topic_type: "Concepts"
learn_rel_path: "Concepts/Netdata cloud"
learn_docs_purpose: "Present the concept of a room, it's purpose and use cases"
-->


**********************************************************************

War Rooms organize your connected nodes and provide infrastructure-wide dashboards using real-time metrics and
visualizations.

Once you add nodes to a Space, all of your nodes will be visible in the _All nodes_ War Room. This is a special War Room
which gives you an overview of all of your nodes in this particular space. Then you can create functional separations of
your nodes into more War Rooms. 

Every War Room has its own dashboards, navigation, indicators, and management tools.

## War Room navigation

### Switching between views - static tabs

Every War Rooms provides multiple views. Each view focus on a particular area/subject of the nodes which you monitor in
this War Rooms. Let's explore what view you have available:

- The default view for any War Room is the [**Home** tab](/docs/concepts/visualize/overview#home), which give you an overview
  of this space. Here you can see the number of Nodes claimed, data retention statics, user particate, alerts and more

- The second and most important view is the [**Overview** tab](/docs/concepts/visualize/overview#overview) which uses composite
  charts to display real-time metrics from every available node in a given War Room.

- The [**Nodes** tab](/docs/concepts/visualize/nodes) gives you the ability to see the status (offline or online), host details
  , alarm status and also a short overview of some key metrics from all your nodes at a glance.

- [**Kubernetes** tab](/docs/concepts/visualize/kubernetes) is a logical grouping of charts regards to your Kubernetes clusters.
  It contains a subset of the charts available in the _Overview tab_

- The [**Dashboards** tab](/docs/concepts/visualize/dashboards) gives you the ability to have tailored made views of
  specific/targeted interfaces for your infrastructure using any number of charts from any number of nodes.

- The **Alerts** tab provides you with an overview for all the active alerts you receive for the nodes in this War Room,
  you can also see alla the alerts that are configured to be triggered in any given moment.

- The **Anomalies** tab is dedicated to the [Anomaly Advisor](/docs/cloud/insights/anomaly-advisor) tool

### Non static tabs

If you open a [new dashboard](/docs/concepts/visualize/dashboards), jump to a single-node dashboard, or navigate to a dedicated
alert page, each of these will open in a new War Room tab.

Tabs can be rearranged with drag-and-drop or closed with the **X** button. Open tabs persist between sessions, so you
can always come right back to your preferred setup.

### Play, pause, force play, and timeframe selector

A War Room has three different states: playing, paused, and force playing. The default playing state refreshes charts
every second as long as the browser tab is in focus. [Interacting with a chart](/docs/dashboard/interact-charts) pauses
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


## Tips for organizing your War Room

You are free to organize your War Rooms to your liking. We do, however, offer a few recommended trategies for organizing your War Rooms.

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
using the [Pulsar collector](/docs/agent/collectors/go.d.plugin/modules/pulsar) begins reporting a suspiciously low
messages rate. You can create a War Room called `$year-$month-$day-pulsar-rate`, add all your Pulsar nodes in addition
to nodes they connect to, and begin diagnosing the root cause in a War Room optimized for getting to resolution as fast
as possible.

## Adding All the War Rooms you want

To add new War Rooms to any Space, click on the green plus icon **+** next the **War Rooms** heading. on the left (
space's) sidebar.

In the panel, give the War Room a name and description, and choose whether it's public or private. Anyone in your Space
can join public War Rooms, but can only join private War Rooms with an invitation.

## Managing War Rooms

All the users and nodes involved in a particular space can potentially be part of a War Room.

Any user can change simple settings of a War Room, like the name or the users participating in it. Click on the gear 
icon of the War Room's name in the top of the page to do that. A sidebar will open with options for this War Room:

1. To _change a War Room's name, description, or public/private status_, click on **War Room** tab of the sidebar.

2. To _include an existing node_ to a War Room or _connect a new node*_ click on **Nodes** tab of the sidebar. Choose any
connected node you want to add to this War Room by clicking on the checkbox next to its hostname, then click **+ Add**
at the top of the panel.

3. To _add existing users to a War Room_, click on **Add Users**. See our [invite doc](/docs/cloud/manage/invite-your-team)
for details on inviting new users to your Space in Netdata Cloud.

:::note
 \* This action requires admin rights for this space
:::

### Bookmarks for  essential resources

When an anomaly or outage strikes, your team needs to access other essential resources quickly. You can use Netdata
Cloud's bookmarks to put these tools in one accessible place. Bookmarks are shared between all War Rooms in a Space, so
any users in your Space will be able to see and use them.

Bookmarks can link to both internal and external resources. You can bookmark your app's status page for quick updates
during an outage, a messaging system on your organization's intranet, or other tools your team uses to respond to
changes in your infrastructure.

To add a new bookmark, click on the **Add bookmark** link. In the panel, name the bookmark, include its URL, and write a
short description for your team's reference.

### More actions

To _view or remove nodes_ in a War Room, click on **Nodes view**. To remove a node from the current War Room, click on
the **🗑** icon. 

:::Note
 Removing a node from a War Room does not remove it from your Space.
:::

## Related Topics

### **Related Concepts**
- [Spaces](https://github.com/netdata/learn/blob/master/docs/concepts/netdata-cloud/spaces.md)
- [Netdata Views](https://github.com/netdata/learn/blob/master/docs/concepts/netdata-cloud/netdata-views.md)
- [Dashboards](https://github.com/netdata/learn/blob/master/docs/concepts/visualizations/dashboards.md)
- [From raw metrics to visualizations](https://github.com/netdata/learn/blob/rework-learn/docs/concepts/visualizations/from-raw-metrics-to-visualization.md)

### Related Tasks
- [Room Management](https://github.com/netdata/learn/blob/rework-learn/docs/tasks/setup/space-administration/room-management.md)
- [Setting up spaces and rooms](https://github.com/netdata/learn/blob/master/docs/tasks/setup/setup-spaces-and-rooms.md#how-to-organize-your-netdata-cloud)
- [Claiming an existing agent to Cloud](https://github.com/netdata/netdata/blob/rework-learn/docs/tasks/setup/claim-existing-agent-to-cloud.md)
- [Interact with charts](https://github.com/netdata/learn/blob/rework-learn/docs/tasks/interact-with-the-charts.md)

*******************************************************************************
