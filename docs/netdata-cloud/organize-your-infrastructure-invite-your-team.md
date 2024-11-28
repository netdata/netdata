# Spaces and Rooms

Netdata Cloud works with [Spaces](#spaces) and [Rooms](#rooms). They allow you to better organize your infrastructure and provide the right access to your team.

## Spaces

A Space is a high-level container. It's a collaboration environment where you can organize team members, access levels and connect the nodes you want to monitor.

### Best practices

You can use any number of Spaces you want, but keep in mind that you can only add any given node to a **single** Space.

We recommend sticking to one Space so that you can keep all your nodes and their respective metrics in one place. You can then use multiple [Rooms](#rooms) to further organize your infrastructure monitoring.

### Navigation

You can navigate through your different Spaces by using the left-most bar of the interface. From there you can also create a new Space by clicking the plus **+** icon.

![image](https://github.com/netdata/netdata/assets/70198089/74f622ac-07bf-40c7-81ba-f3907ed16c42)

### Management

Manage your spaces by selecting a particular space and clicking on the gear icon in the lower left-hand corner. This will open the Space's settings view, where you can take a multitude of actions regarding the Space's Rooms, nodes, integrations, configurations, and more.

## Rooms

Spaces use Rooms to organize your connected nodes and provide infrastructure-wide dashboards using real-time metrics and visualizations.

**A node can be in N Rooms.**

Once you add nodes to a Space, all of them will be visible in the **All nodes** Room. Then you can group your nodes into more Rooms. Every Room has its own dashboards, navigation, indicators, and management tools.

### Best Practices

We recommend a few strategies for organizing your Rooms.

- **Service, purpose, location, etc.**

  You can group Rooms by a service (Nginx, MySQL, Pulsar, and so on), their purpose (webserver, database, application), their physical location, whether they're "bare metal" or a Docker container, the PaaS/cloud provider it runs on, and much more. This allows you to see entire slices of your infrastructure by moving from one Room to another.

- **End-to-end apps/services**

  If you have a user-facing SaaS product, or an internal service that this product relies on, you may want to monitor that entire stack in a single Room. This might include Kubernetes clusters, Docker containers, proxies, databases, web servers, brokers, and more. End-to-end Rooms are valuable tools for ensuring the health and performance of your organization's essential services.

- **Incident response**

  You can also create new Rooms as one of the first steps in your incident response process. For example, you have a user-facing web app that relies on Apache Pulsar for a message queue, and one of your nodes using the [Pulsar collector](/src/go/plugin/go.d/collector/pulsar/README.md) begins reporting a suspiciously low messages rate. You can create a Room called `$year-$month-$day-pulsar-rate`, add all your Pulsar nodes in addition to nodes they connect to, and begin diagnosing the root cause in a Room optimized for getting to resolution as fast as possible.

### Navigation

You can navigate through your Rooms from the Space's sidebar.

<img src="https://github.com/user-attachments/assets/16958ba8-53ac-4e78-a51f-7ea328e97f31" height="400px" alt="Individual Space sidebar">

To add new Rooms to any Space, click on the green plus icon **+** next to the **Rooms** heading on the Room's sidebar.

### Management

All the users and nodes involved in a particular Space can be part of a Room.

Click on the gear icon next to the Room's name in the top of the page to open the Room settings. From there you can take the same actions as with the Spaces settings, but now catered towards the specific Room.

## Invite your team

Invite your entire SRE, DevOPs, or ITOps team to your Space, to give everyone access into your infrastructure from a single pane of glass.

To do so, click on **Invite Users** in the Space's sidebar or any other such prompt around the UI.

Follow the instructions on screen, to provide the right Room access and roles to the invitees.
