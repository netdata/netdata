<!--
title: "Rooms"
sidebar_label: "Rooms"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/tasks/setup/space-administration/rooms.md"
learn_status: "Published"
learn_topic_type: "Tasks"
sidebar_position: "1"
learn_rel_path: "Setup/Space administration"
learn_docs_purpose: "Instructions on how an admin/user can manage a room"
-->

import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';
import Admonition from '@theme/Admonition';

War Rooms organize your connected nodes and provide infrastructure-wide dashboards using real-time metrics and
visualizations.

Once you add nodes to a Space, all of your nodes will be visible in the **All nodes War Room**. This is a special War
Room which gives you an overview of all of your nodes in this particular space. Then you can create functional
separations of your nodes into more War Rooms. Every War Room has its own dashboards, navigation, indicators, and
management tools.

:::note
Some of these actions can be configured to be accessible to normal users too instead of being admin-only, you can read
more in
the [Space administration](https://github.com/netdata/netdata/blob/master/docs/tasks/setup/space-administration/spaces.md)
Task.
:::

## Prerequisites

To perform administrative actions on Rooms, you will need the following:

- A Netdata Cloud account with at least one node claimed to one of its Spaces.

## Node management

You can perform the following actions from either :

- The **War Room settings** interface, reachable from the top of the Cloud
  interface, by clicking the **cogwheel** icon next to the War Room's name
- The **Nodes** view accessible from the view bar on the top of the Cloud interface

### Add a Node to a War Room

<Tabs groupId="choice">

<TabItem value="War Room settings" label="War Room settings" default>

1. Click on the **Nodes** tab
2. Click the green **+** icon
3. Then you can:
    - Either proceed on claiming a new Agent to this Space, and adding it to the War Room that you are in. See details
      at
      our [claim an Agent to the Cloud](https://github.com/netdata/netdata/blob/master/docs/tasks/setup/claim-existing-agent-to-cloud.md)
      Task
    - Or, from the bottom tab, you can select to add nodes that are already claimed on your Space.

</TabItem>

<TabItem value="Nodes view" label="Nodes View" default>

1. Click the **+ Add Nodes** button
2. Then you can:
    - Either proceed on claiming a new Agent to this Space, and adding it to the War Room that you are in. See details
      at
      our [claim an Agent to the Cloud](https://github.com/netdata/netdata/blob/master/docs/tasks/setup/claim-existing-agent-to-cloud.md)
      Task
    - Or, from the bottom tab, you can select to add nodes that are already claimed on your Space.

</TabItem>
</Tabs>

### Remove a Node from a War Room

<Tabs groupId="other">

<TabItem value="War Room settings" label="War Room settings" default>

1. Click on the **Nodes** tab
2. You can remove a node from the War Room by clicking the **Remove node from Room** icon in the **Actions** column

</TabItem>

<TabItem value="Nodes view" label="Nodes View" default>

1. Hover over the node to be removed
2. On the right side of the row, click the **Remove Node from the War Room** button

</TabItem>
</Tabs>

## User management

You can add/remove users by following the Tasks below by navigating to the **War Room settings** interface.

### Add users to a War Room

From the **War Room settings** interface:

1. Click on the **Users** tab
2. Click the green **+** icon
3. Select from the list of the Space's users, that are eligible to be added to this War Room.

If you want to add a new User to the War Room that isn't on the Space, check
the [Space administration](https://github.com/netdata/netdata/blob/master/docs/tasks/setup/space-administration/spaces.md)
Task.

### Remove users from a War Room

From the **War Room settings** interface:

1. Click on the **Users** tab
2. Remove a user from the War Room by clicking the trashcan icon in the **Actions** column.

## Create/Delete Custom Dashboards

From the **Dashboards** view of a War Room, you can Create Custom Dashboards by clicking the green **Create new**
button.  
Then you can add charts by following the prompt.

:::tip
You can quickly add a chart to a Custom Dashboard, by clicking the **Add to dashboard** button of the top bar on any
chart. To read more, check
the [interact with the Charts](https://github.com/netdata/netdata/blob/master/docs/tasks/operations/interact-with-the-charts.md)
Task.
:::

To delete a Custom Dashboard, select it from the tick box column, and then click the **Delete** button.

## Related topics

### Related Concepts

1. [War Rooms](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-cloud/rooms.md)
2. [Views Concept](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-cloud/netdata-views.md)
