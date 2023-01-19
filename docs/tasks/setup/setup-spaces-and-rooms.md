<!--
title: "Setup your Space and rooms"
sidebar_label: "Setup your Space and rooms"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/tasks/setup/setup-spaces-and-rooms.md"
sidebar_position : "10"
learn_status: "Unpublished"
learn_topic_type: "Tasks"
learn_rel_path: "Setup"
learn_docs_purpose: "Your first step in your Netdata Space"
-->

## Introduction

Welcome to the centralized observability view of your infrastructure. Once signed in with your preferred method, a Space
named for your login email is automatically created. Every Space has a static War Room named `All nodes`.

## Steps

Follow our on-boarding guides to walk through the app.

## How to organize your Netdata Cloud

You can configure more Spaces and War Rooms to help you organize your team and the many systems that make up your
infrastructure. For example, you can put product and infrastructure SRE teams in separate Spaces, and then use War Rooms
to group nodes by their service (nginx), purpose (webservers), or physical location (IAD).

Don't worry! You can always add more Spaces and War Rooms later if you decide to reorganize how you use Netdata Cloud.

You can use any number of Spaces you want, but as you organize your Cloud experience, keep in mind that you can only add
any given node to a single Space. This 1:1 relationship between node and Space may dictate whether you use one
encompassing Space for your entire team and separate them by War Rooms, or use different Spaces for teams monitoring
discrete parts of your infrastructure.

If you have been invited to Netdata Cloud by another user by default you will be able to see this space. If you are a
new user the first space is already created.

The other consideration for the number of Spaces you use to organize your Netdata Cloud experience is the size and
complexity of your organization.

For small team and infrastructures we recommend sticking to a single Space so that you can keep all your nodes and their
respective metrics in one place. You can then use multiple War Rooms to further organize your infrastructure monitoring.

Enterprises may want to create multiple Spaces for each of their larger teams, particularly if those teams have
different responsibilities or parts of the overall infrastructure to monitor. For example, you might have one SRE team
for your user-facing SaaS application and a second team for infrastructure tooling. If they don't need to monitor the
same nodes, you can create separate Spaces for each team.
