<!--
title: "Spaces"
sidebar_label: "Spaces"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-cloud/spaces.md"
sidebar_position: "1600"
learn_status: "Published"
learn_topic_type: "Concepts"
learn_rel_path: "Concepts/Netdata cloud"
learn_docs_purpose: "Present the purpose of Spaces"
-->


**********************************************************************

A Space is a high-level container. It's a virtual collaboration area  where you can organize team members, access levels,and the
nodes you want to monitor.

[Read here](https://github.com/netdata/learn/blob/master/docs/tasks/setup/setup-spaces-and-rooms.md#how-to-organize-your-netdata-cloud) to learn some recommended strategies for creating the most intuitive Cloud experience for your team.

## Space Navigation

Click on any of the boxes to switch between available Spaces.

Netdata Cloud abbreviates each Space to the first letter of the name, or the first two letters if the name is two words
or more. Hover over each icon to see the full name in a tooltip.

To add a new Space, click on the green **+** button . Enter the name of the Space and click **Save**.


## Space Management

Manage your spaces by selecting in a particular space and clicking in the small gear icon in the lower left corner. This
will open a side tab in which you can:

1. _Configure this Space*_, in the first tab (**Space**) you can change the name, description or/and some privilege
   options of this space

2. _Edit the War Rooms*_, click on the **War rooms** tab to add or remove War Rooms.

3. _Connect nodes*_, click on **Nodes** tab. Copy the claiming script to your node and run it. See the
   [connect to Cloud doc](/docs/agent/claim) for details.

4. _Manage the users*_, click on **Users**. The [invitation doc](/docs/cloud/manage/invite-your-team)
   details the invitation process.

5. _Manage notification setting*_, click on **Notifications** tab to turn off/on notification methods.

6. _Manage your bookmarks*_, click on the **Bookmarks** tab to add or remove bookmarks that you need.

:::note \* This action requires admin rights for this space
:::

## Obsolete Offline Nodes

Netdata admin users now have the ability to remove obsolete nodes from a space, with the following conditions recognized:

- Only offline nodes can be marked obsolete (Live nodes and stale nodes cannot be obsoleted)
- Node obsoletion works across the entire space, so the obsoleted node will be removed from all rooms belonging to the
  space
- If the obsoleted nodes eventually become live or online once more they will be automatically re-added to the space

![Obsoleting an offline node](https://user-images.githubusercontent.com/24860547/173087202-70abfd2d-f0eb-4959-bd0f-74aeee2a2a5a.gif)

## Related Topics

### **Related Concepts**
- [Netdata Views](https://github.com/netdata/learn/blob/master/docs/concepts/netdata-cloud/netdata-views.md)
- [Rooms](https://github.com/netdata/learn/blob/master/docs/concepts/netdata-cloud/rooms.md)
- [Dashboards](https://github.com/netdata/learn/blob/master/docs/concepts/visualizations/dashboards.md)
- [From raw metrics to visualizations](https://github.com/netdata/learn/blob/rework-learn/docs/concepts/visualizations/from-raw-metrics-to-visualization.md)

### Related Tasks
- [Space Administration](https://github.com/netdata/learn/blob/master/docs/tasks/setup/space-administration/space-administration.md)
- [Set up spaces and war rooms](https://github.com/netdata/learn/blob/master/docs/tasks/setup/setup-spaces-and-rooms.md)
- [Claiming an existing agent to Cloud](https://github.com/netdata/netdata/blob/master/docs/tasks/setup/claim-existing-agent-to-cloud.md)
- [Interact with charts](https://github.com/netdata/learn/blob/master/docs/tasks/interact-with-the-charts.md)
*******************************************************************************
