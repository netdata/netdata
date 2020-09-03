<!--
title: "Invite your team and collaborate"
description: "Invite your SRE, DevOPs, or ITOps teams to Netdata Cloud to give everyone insights into your infrastructure from a single pane of glass."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/configure/invite-collaborate.md
-->

# Invite your team and collaborate

Netdata is designed to make an infrastructure's real-time metrics available and actionable to all members of an
organization. Through inviting others, you can better synchronize with your team or colleagues on understanding the
heartbeat of your infrastructure. When something goes wrong, you'll be ready to collaboratively troubleshoot complex
performance problems from a single pane of glass.

## Invite new members

Invite new users by clicking on your Space's name in the top navigation, and then **Invite more users**, to open the
invitation pane.

![Opening and navigating the invitation
panel](https://user-images.githubusercontent.com/1153921/92025596-a618e680-ed14-11ea-9c1f-a61fdcb8aa4e.png)

Enter their email address and name. They can change this name once they accept your invitation.

Choose which War Rooms you want to add this user to, then click the plus **+** button to add the invitation to the
**New invitations to be sent** queue. Repeat the process with everyone you want to invite to your Space.

When you're ready to send the new invitations you created, hit the **Send** button. Netdata Cloud sends these
invitations as soon as it can and moves them to the **Invitations awaiting response** category.

Your team will receive their invitations momentarily and prompt them to sign in to join your Space.

## Collaboration with Netdata Cloud

Netdata Cloud gives teams a single interface to view real-time metrics from across the entire infrastructure. Having all
the metrics, alarm statuses, dashboards, and people in one place is a powerful asset for any infrastructure monitoring
team.

Assets like dashboards and bookmarks are shared between members of a War Room. As soon as one member creates a
dashboard, for example, other members of the same War Room can see it in the War Room's dropdown and supplement it with
additional charts/text.

Let's say you get an alert from your nodes about an excess of 500-type errors in your Nginx logs. Your team can hop on a
Slack call to begin working together. While one engineer handles creating a new dashboard with a half-dozen relevant
Nginx log metrics, another can dive into the real-time node dashboard investigate correlated charts in graunular detail.

## What's next?

If your team members have trouble signing in, direct them to the [Netdata Cloud sign in
doc](https://learn.netdata.cloud/docs/cloud/manage/sign-in). Or, find answers to other common questions about Netdata
Cloud in our [FAQ](https://learn.netdata.cloud/docs/cloud/faq-glossary).

Next, we recommend you learn the [basics of node configuration](/docs/configure/nodes.md). While the Netdata Agent is
proudly zero-configuration in most cases, it's important that you understand how to tweak its settings to give you the
best Netdata, for example, to increase metrics retention and improve security.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fconfigure%2Finvite-collaborate&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
