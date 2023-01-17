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

A Space is a high-level container. It's a virtual collaboration area where you can organize team members, access
levels, and the nodes you want to monitor.

You can use any number of Spaces you want, but as you organize your Cloud experience, keep in mind that you can only add
any given node to a single Space. This 1:1 relationship between node and Space may dictate whether you use one
encompassing Space for your entire team and separate them by War Rooms, or use different Spaces for teams monitoring
discrete parts of your infrastructure.

If you have been invited to Netdata Cloud by another user by default you will be able to see this space. If you are a new
user the first space is already created.

The other consideration for the number of Spaces you use to organize your Netdata Cloud experience is the size and
complexity of your organization.

For small team and infrastructures we recommend sticking to a single Space so that you can keep all your nodes and their
respective metrics in one place. You can then use
multiple [War Rooms](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-cloud/rooms.md) to further
organize your infrastructure monitoring.

Enterprises may want to create multiple Spaces for each of their larger teams, particularly if those teams have
different responsibilities or parts of the overall infrastructure to monitor. For example, you might have one SRE team
for your user-facing SaaS application and a second team for infrastructure tooling. If they don't need to monitor the
same nodes, you can create separate Spaces for each team.

We advise you to also explore the [recommended strategies](https://github.com/netdata/netdata/blob/master/docs/tasks/setup/setup-spaces-and-rooms.md#how-to-organize-your-netdata-cloud) for creating the most intuitive Cloud experience for your team.

### Related Topics

#### **Related Concepts**

- [Netdata Views](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-cloud/netdata-views.md)
- [Rooms](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-cloud/rooms.md)
- [Dashboards](https://github.com/netdata/netdata/blob/master/docs/concepts/visualizations/dashboards.md)
- [From raw metrics to visualizations](https://github.com/netdata/netdata/blob/master/docs/concepts/visualizations/from-raw-metrics-to-visualization.md)

#### Related Tasks

- [Space Administration](https://github.com/netdata/netdata/blob/master/docs/tasks/setup/space-administration/spaces.md)
- [Claiming an existing agent to Cloud](https://github.com/netdata/netdata/blob/master/docs/tasks/setup/claim-existing-agent-to-cloud.md)
- [Interact with charts](https://github.com/netdata/netdata/blob/master/docs/tasks/operations/interact-with-the-charts.md)
