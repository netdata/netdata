<!--
title: "Set up Spaces and War Rooms"
description: ""
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/configure/spaces-war-rooms.md
-->

# Set up Spaces and War Rooms

Spaces and War Rooms help you organize your real-time infrastructure monitoring experience in Netdata Cloud. You already
created a Space and War Room when you first signed in to Cloud, assuming you weren't invited to an existing Space by
someone else.

In either case, you can always create new Spaces and War Rooms based on your changing needs or a scaled-up
infrastructure. Let's talk through some strategies for creating the most intuitive Cloud experience for your team.

> This guide assumes you've already [signed in](https://app.netdata.cloud) to Netdata Cloud and finished creating your
> account. If you're not interested in Netdata Cloud's features, you can skip ahead to [node configuration
> basics](/docs/configure/nodes.md).

## Spaces

Spaces are high-level containers to help you organize your team members and the nodes they're able to view in each War
Room. You already have one Space in your Netdata Cloud account.

To create a new Space, click the **+** icon, and enter its name. Switch between existing Spaces using the Spaces
navigation. Spaces are distinguished by an abbreviated version of their name.

The organization you choose will likely be based on two factors:

1.  The fact that any node can be claimed to a single Space.
2.  The size of your team and the complexity of the infrastructure you monitor. 

A single Space is best for smaller infrastructures, as it puts all your metrics in one easily-accessible place. For
larger organizations and operations, multiple siloed Spaces may be easier to manage.

For example, a large organization might have one SRE team for the user-face SaaS application, and a second IT team for
managing employee's hardware. Since these teams don't monitor the same nodes, they can work in separate Spaces and then
further organize their nodes into War Rooms.

## War Rooms

War Rooms are where users can view key metrics in real-time and monitor the health of many nodes with their alarm
status. 

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
using the [Pulsar collector](/docs/agent/collectors/go.d.plugin/modules/pulsar) begins reporting a suspiciously low
messages rate. You can create a War Room called `$year-$month-$day-pulsar-rate`, add all your Pulsar nodes in addition
to nodes they connect to, and begin diagnosing the root cause in a War Room optimized for getting to resolution as fast
as possible.

## What's next?

Once you've figured out an organizational structure that works for your infrastructure, it's time to [invite your
team](/docs/configure/invite-collaborate.md). You can invite any number of colleages to help you collectively
troubleshoot the most complex of infrastructure-wide performace issues.

If you don't have a team to collaborate with, or aren't ready to onboard them your new real-time infrastructure
monitoring dashboard in Netdata Cloud, you can skip ahead to learn the [basics of node
configuration](/docs/configure/nodes.md).

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fconfigure%2Fspaces-war-rooms&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
