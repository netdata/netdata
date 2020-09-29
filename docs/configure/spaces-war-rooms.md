<!--
title: "Set up Spaces and War Rooms"
description: "Netdata Cloud allows people and teams of all sizes to organize their infrastructure and collaborate on anomalies or incidents."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/configure/spaces-war-rooms.md
-->

# Set up Spaces and War Rooms

Spaces and War Rooms help you organize your real-time infrastructure monitoring experience in Netdata Cloud. You already
created a Space and War Room when you first signed in to Cloud, assuming you weren't invited to an existing Space by
someone else.

In either case, you can always create new Spaces and War Rooms based on your changing needs or a scaled-up
infrastructure. Let's talk through some strategies for building the most intuitive Cloud experience for your team.

> This guide assumes you've already [signed in](https://app.netdata.cloud) to Netdata Cloud and finished creating your
> account. If you're not interested in Netdata Cloud's features, you can skip ahead to [node configuration
> basics](/docs/configure/nodes.md).

## Spaces

Spaces are high-level containers to help you organize your team members and the nodes they can view in each War Room.
You already have at least one Space in your Netdata Cloud account.

To create a new Space, click the **+** icon, enter its name, and click **Save**. Netdata Cloud distinguishes between
Spaces with abbreviated versions of their name. Click on any of the icons to switch between them.

![Spaces navigation in Netdata
Cloud](https://user-images.githubusercontent.com/1153921/92177439-5b22d000-edf5-11ea-9323-383347f21c8d.png)

The organization you choose will likely be based on two factors:

1.  The fact that any node can be claimed to a single Space.
2.  The size of your team and the complexity of the infrastructure you monitor.

A single Space puts all your metrics in one easily-accessible place, while multiple Spaces creates logical division
between different users and different pieces of a large infrastructure.

For example, a large organization might have one SRE team for the user-facing SaaS application, and a second IT team for
managing employees' hardware. Since these teams don't monitor the same nodes, they can work in separate Spaces and then
further organize their nodes into War Rooms.

You can also use multiple Spaces for different aspects of your monitoring "life," such as your work infrastructure
versus your homelab.

## War Rooms

War Rooms are granular containers for organizing nodes, viewing key metrics in real-time, and monitoring the health and
alarm status of many nodes. 

War Rooms organize the [at-a-glance Node view](/docs/visualize/view-all-nodes.md) and any [new
dashboards](/docs/visualize/create-dashboards.md) you build.

We recommend a few strategies for organizing your War Rooms.

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
using the [Pulsar collector](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/pulsar) begins
reporting a suspiciously low messages rate. You can create a War Room called `$year-$month-$day-pulsar-rate`, add all
your Pulsar nodes in addition to nodes they connect to, and begin diagnosing the root cause in a War Room optimized for
getting to resolution as fast as possible.

For example, here is a War Room based on the node's provider and physical location (**us-east-1**).

![An example War Room based on provider and
location](https://user-images.githubusercontent.com/1153921/92178714-ff0d7b00-edf7-11ea-8411-09b2e75a5529.png)

## What's next?

Once you've figured out an organizational structure that works for your infrastructure, it's time to [invite your
team](/docs/configure/invite-collaborate.md). You can invite any number of colleagues to help you collectively
troubleshoot the most complex of infrastructure-wide performance issues.

If you don't have a team or aren't ready to invite them, you can skip ahead to learn the [basics of node
configuration](/docs/configure/nodes.md).

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fconfigure%2Fspaces-war-rooms&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
