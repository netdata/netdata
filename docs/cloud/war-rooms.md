# Netdata Cloud War rooms

Netdata Cloud uses War Rooms to organize your connected nodes and provide infrastructure-wide dashboards using real-time metrics and visualizations.

Once you add nodes to a Space, all of your nodes will be visible in the **All nodes** War Room. This is a special War Room
which gives you an overview of all of your nodes in this particular Space. Then you can create functional separations of
your nodes into more War Rooms. Every War Room has its own dashboards, navigation, indicators, and management tools.

![An example War Room](https://user-images.githubusercontent.com/43294513/225355998-f16730ba-06d4-4953-8fd3-f1c2751e102d.png)

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

To **view or remove nodes** in a War Room, click on the **Nodes tab**. To remove a node from the current War Room, click on
the **🗑** icon.

> ### Info
>
> Removing a node from a War Room does not remove it from your Space.
