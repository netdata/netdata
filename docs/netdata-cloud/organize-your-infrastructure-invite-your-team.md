# Organize Your Infrastructure and Invite your Team

Netdata Cloud works with [Spaces](#netdata-cloud-spaces) and [War Rooms](#netdata-cloud-war-rooms). They allow you to better organize your infrastructure and provide access to your team.

## Netdata Cloud Spaces

A Space is a high-level container. It's a collaboration environment where you can organize team members, access levels and the nodes you want to monitor.

### How to organize your Netdata Cloud

You can use any number of Spaces you want, but as you organize your Cloud experience, keep in mind that _you can only add any given node to a single Space_.

We recommend sticking to a single Space so that you can keep all your nodes and their respective metrics in one place. You can then use multiple [War Rooms](#netdata-cloud-war-rooms) to further organize your infrastructure monitoring.

### Navigate between Spaces

You can navigate through your different Spaces by using the left-most bar of the interface. From there you can also create a new Space by clicking the plus **+** icon.

![image](https://github.com/netdata/netdata/assets/70198089/74f622ac-07bf-40c7-81ba-f3907ed16c42)

### Manage Spaces

Manage your spaces by selecting a particular space and clicking on the small gear icon in the lower left corner. This will open the Space's settings view, where you can take a multitude of actions regarding the Space's Rooms, nodes, integrations, configurations, and more.

### Obsoleting offline nodes from a Space

Netdata admin users now have the ability to remove obsolete nodes from a space.

- Only admin users have the ability to obsolete nodes
- Only offline nodes can be marked obsolete (Live nodes and stale nodes cannot be obsoleted)
- Node obsoletion works across the entire space, so the obsoleted node will be removed from all rooms belonging to the
  space
- If the obsoleted nodes eventually become live or online once more they will be automatically re-added to the space

## Netdata Cloud War rooms

Spaces use War Rooms to organize your connected nodes and provide infrastructure-wide dashboards using real-time metrics and visualizations.

A node can be in N War Rooms.

Once you add nodes to a Space, all of your nodes will be visible in the **All nodes** War Room. It gives you an overview of all of your nodes in this particular Space. Then you can create functional separations of your nodes into more War Rooms. Every War Room has its own dashboards, navigation, indicators, and management tools.

### War Room organization

We recommend a few strategies for organizing your War Rooms.

- **Service, purpose, location, etc.**  
   You can group War Rooms by a service (Nginx, MySQL, Pulsar, and so on), their purpose (webserver, database, application), their physical location, whether they're "bare metal" or a Docker container, the PaaS/cloud provider it runs on, and much more. This allows you to see entire slices of your infrastructure by moving from one War Room to another.

- **End-to-end apps/services**  
  If you have a user-facing SaaS product, or an internal service that this said product relies on, you may want to monitor that entire stack in a single War Room. This might include Kubernetes clusters, Docker containers, proxies, databases, web servers, brokers, and more. End-to-end War Rooms are valuable tools for ensuring the health and performance of your organization's essential services.

- **Incident response**  
  You can also create new War Rooms as one of the first steps in your incident response process. For example, you have a user-facing web app that relies on Apache Pulsar for a message queue, and one of your nodes using the [Pulsar collector](/src/go/collectors/go.d.plugin/modules/pulsar/README.md) begins reporting a suspiciously low messages rate. You can create a War Room called `$year-$month-$day-pulsar-rate`, add all your Pulsar nodes in addition to nodes they connect to, and begin diagnosing the root cause in a War Room optimized for getting to resolution as fast as possible.

### Add War Rooms

To add new War Rooms to any Space, click on the green plus icon **+** next to the **War Rooms** heading on the Room's sidebar.

### Manage War Rooms

All the users and nodes involved in a particular Space can be part of a War Room.

Click on the gear icon next to the Room's name in the top of the page to do that. This will open the Rooms settings view, where you can take the same actions as with the Spaces settings, now catered towards the specific Room.

## Invite your team

Invite your entire SRE, DevOPs, or ITOps team to your Space, to give everyone access into your infrastructure from a single pane of glass.

Invite new users to your Space by clicking on **Invite Users** in the [Space](#netdata-cloud-spaces) management area.

Follow the instructions on screen, to provide the right access and role to the users you want to invite.
