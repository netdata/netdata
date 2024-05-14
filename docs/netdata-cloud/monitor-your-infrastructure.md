import { Grid, Box, BoxList, BoxListItemRegexLink } from '@site/src/components/Grid/'
import { RiExternalLinkLine } from 'react-icons/ri'

# Monitor your infrastructure

Learn how to view key metrics, insightful charts, and active alerts from all your nodes, with Netdata Cloud's real-time infrastructure monitoring.

[Netdata Cloud](https://app.netdata.cloud) provides scalable infrastructure monitoring for any number of distributed
nodes running the Netdata Agent. A node is any system in your infrastructure that you want to monitor, whether it's a
physical or virtual machine (VM), container, cloud deployment, or edge/IoT device.

The Netdata Agent uses zero-configuration collectors to gather metrics from every application and container instantly,
and uses Netdata's distributed data architecture to store metrics
locally. Without a slow and troublesome centralized data lake for your infrastructure's metrics, you reduce the
resources you need to invest in, and the complexity of, monitoring your infrastructure.

Netdata Cloud unifies infrastructure monitoring by _centralizing the interface_ you use to query and visualize your
nodes' metrics, not the data. By streaming metrics values to your browser, with Netdata Cloud acting as the secure proxy
between them, you can monitor your infrastructure using customizable, interactive, and real-time visualizations from any
number of distributed nodes.

In this quickstart guide, you'll learn the basics of using Netdata Cloud to monitor an infrastructure with dashboards,
composite charts, and alert viewing. You'll then learn about the most critical ways to configure the Agent on each of
your nodes to maximize the value you get from Netdata.

This quickstart assumes you've [installed Netdata](https://github.com/netdata/netdata/edit/master/packaging/installer/README.md)
on more than one node in your infrastructure, and connected those nodes to your Space in Netdata Cloud.

## Set up your Netdata Cloud experience

Start your infrastructure monitoring experience by setting up your Netdata Cloud account.

### Organize Spaces and War Rooms

Spaces are high-level containers to help you organize your team members and the nodes they can view in each War Room.
You already have at least one Space in your Netdata Cloud account.

A single Space puts all your metrics in one easily-accessible place, while multiple Spaces creates logical division
between different users and different pieces of a large infrastructure. For example, a large organization might have one
SRE team for the user-facing SaaS application, and a second IT team for managing employees' hardware. Since these teams
don't monitor the same nodes, they can work in separate Spaces and then further organize their nodes into War Rooms.

Next, set up War Rooms. Netdata Cloud creates dashboards and visualizations based on the nodes added to a given War
Room. You can [organize War Rooms](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/organize-your-infrastrucutre-invite-your-team.md#war-room-organization) in any way
you want, such as by the application type, for end-to-end application monitoring, or as an incident response tool.

Learn more about [Spaces](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/organize-your-infrastrucutre-invite-your-team.md#netdata-cloud-spaces) and [War
Rooms](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/organize-your-infrastrucutre-invite-your-team.md#netdata-cloud-war-rooms), including how to manage each, in their respective reference
documentation.

### Invite your team

Netdata Cloud makes an infrastructure's real-time metrics available and actionable to all organization members. By
inviting others, you can better synchronize with your team or colleagues to understand your infrastructure's heartbeat.
When something goes wrong, you'll be ready to collaboratively troubleshoot complex performance problems from a single
pane of glass.

To [invite new users](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/organize-your-infrastrucutre-invite-your-team.md#invite-your-team), click on **Invite Users** in the
Space management Area. Choose which War Rooms to add this user to, then click **Send**.

### See an overview of your infrastructure

Netdata Cloud utilizes "tabs" in order to provide you with informative sections based on your infrastructure.  
These tabs can be separated into "static", meaning they are by default presented, and "non-static" which are tabs that get presented by user action (e.g clicking on a custom dashboard)

#### Static tabs

- The default tab for any War Room is the [Home tab](https://github.com/netdata/netdata/blob/master/docs/dashboard/home-tab.md), which gives you an overview of this Space.  
  Here you can see the number of Nodes claimed, data retention statics, users by role, alerts and more.

- The [Nodes tab](https://github.com/netdata/netdata/blob/master/docs/dashboard/nodes-tab.md) gives you the ability to see the status (offline or online), host details, alert status and also a short overview of some key metrics from all your nodes at a glance.

- The third and most important tab is the [Metrics tab](https://github.com/netdata/netdata/blob/master/docs/dashboard/metrics-tab-and-single-node-tabs.md) which uses composite charts to display real-time metrics from every available node in a given War Room.

- [Kubernetes tab](https://github.com/netdata/netdata/blob/master/docs/dashboard/kubernetes-tab.md) is a logical grouping of charts regarding your Kubernetes clusters. It contains a subset of the charts available in the **Overview tab**.

- The [Dashboards tab](https://github.com/netdata/netdata/blob/master/docs/dashboard/dashboards-tab.md) gives you the ability to have tailored made views of specific/targeted interfaces for your infrastructure using any number of charts from any number of nodes.

- The [Alerts tab](https://github.com/netdata/netdata/blob/master/docs/monitor/view-active-alerts.md) provides you with an overview for all the active alerts you receive for the nodes in this War Room, you can also see all the alerts that are configured to be triggered in any given moment.

- The [Anomalies tab](https://github.com/netdata/netdata/blob/master/docs/dashboard/anomaly-advisor-tab.md) is dedicated to the Anomaly Advisor tool.

- The [Functions tab](https://github.com/netdata/netdata/blob/master/docs/cloud/netdata-functions.md) gives you the ability to visualize functions that the Netdata Agent collectors are able to expose.

- The [Feed & events](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/events-feed.md) tab lets you investigate events that occurred in the past, which is invaluable for troubleshooting.

#### Dynamic tabs

If you open a [new dashboard](https://github.com/netdata/netdata/blob/master/docs/dashboard/dashboards-tab.md), jump to a single-node dashboard, or navigate to a dedicated alert page, a new tab will open in War Room bar.

Tabs can be rearranged with drag-and-drop or closed with the **X** button. Open tabs persist between sessions, so you can always come right back to your preferred setup.

### Drill down to specific nodes

Both the Overview and the Nodes tab offer easy access to **single-node dashboards** for targeted analysis. You can use
single-node dashboards in Netdata Cloud to drill down on specific issues, scrub backward in time to investigate
historical data, and see like metrics presented meaningfully to help you troubleshoot performance problems.

Learn more about [interacting with
dashboards and charts](https://github.com/netdata/netdata/blob/master/docs/dashboard/netdata-charts.md) to get the most from all of Netdata's real-time
metrics.

### Create new dashboards

You can use Netdata Cloud to create new dashboards that match your infrastructure's topology or help you diagnose
complex issues by aggregating correlated charts from any number of nodes. For example, you could monitor the system CPU
from every node in your infrastructure on a single dashboard.

![An example system CPU
dashboard](https://user-images.githubusercontent.com/1153921/108732974-4b09c480-74eb-11eb-87a2-c67e569c08b6.png)

Read more about [creating new dashboards](https://github.com/netdata/netdata/blob/master/docs/dashboard/dashboards-tab.md) for more details about the process and
additional tips on best leveraging the feature to help you troubleshoot complex performance problems.

## Set up your nodes

You get the most value out of Netdata Cloud's infrastructure monitoring capabilities if each node collects every
possible metric. For example, if a node in your infrastructure is responsible for serving a MySQL database, you should
ensure that the Netdata Agent on that node is properly collecting and streaming all MySQL-related metrics.

In most cases, collectors autodetect their data source and require no configuration, but you may need to configure
certain behaviors based on your infrastructure. Or, you may want to enable/configure advanced functionality, such as
longer metrics retention or streaming.

### Configure the Netdata Agent on your nodes

You can configure any node in your infrastructure if you need to, although most users will find the default settings
work extremely well for monitoring their infrastructures.

Each node has a configuration file called `netdata.conf`, which is typically at `/etc/netdata/netdata.conf`. The best
way to edit this file is using the `edit-config` script, which ensures updates to the Netdata Agent do not overwrite
your changes. For example:

```bash
cd /etc/netdata
sudo ./edit-config netdata.conf
```

Our [configuration basics doc](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md) contains more information about `netdata.conf`, `edit-config`,
along with simple examples to get you familiar with editing your node's configuration.

After you've learned the basics, you should [secure your infrastructure's nodes](https://github.com/netdata/netdata/blob/master/docs/security-and-privacy-design/README.md) using
one of our recommended methods. These security best practices ensure no untrusted parties gain access to the metrics
collected on any of your nodes.

### Collect metrics from systems and applications

Netdata has [300+ pre-installed collectors](https://github.com/netdata/netdata/blob/master/src/collectors/COLLECTORS.md) that gather thousands of metrics with zero
configuration. Collectors search each of your nodes in default locations and ports to find running applications and
gather as many metrics as they can without you having to configure them individually.

Most collectors work without configuration, should you want more info, you can read more on [how Netdata's metrics collectors work](https://github.com/netdata/netdata/blob/master/src/collectors/README.md) and the [Collectors configuration reference](https://github.com/netdata/netdata/blob/master/src/collectors/REFERENCE.md) documentation.

In addition, find detailed information about which [system](https://github.com/netdata/netdata/blob/master/docs/collect/system-metrics.md),
[container](https://github.com/netdata/netdata/blob/master/docs/collect/container-metrics.md), and [application](https://github.com/netdata/netdata/blob/master/docs/collect/application-metrics.md) metrics you can
collect from across your infrastructure with Netdata.

## Netdata Cloud features

<Grid columns="2">
  <Box
    title="Spaces and War Rooms">
    <BoxList>
      <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/organize-your-infrastrucutre-invite-your-team.md#netdata-cloud-spaces)" title="Spaces" />
      <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/organize-your-infrastrucutre-invite-your-team.md#netdata-cloud-war-rooms)" title="War Rooms" />
    </BoxList>
  </Box>
  <Box
    title="Dashboards">
    <BoxList>
      <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/dashboard/metrics-tab-and-single-node-tabs.md)" title="Metrics tab" />
      <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/dashboard/nodes-tab.md)" title="Nodes tab" />
      <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/dashboard/kubernetes-tab.md)" title="Kubernetes" />
      <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/dashboard/dashboards-tab.md)" title="Create new dashboards" />
    </BoxList>
  </Box>
  <Box
    title="Alerts and notifications">
    <BoxList>
      <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/monitor/view-active-alerts.md#netdata-cloud)" title="View active alerts" />
      <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/notifications.md)" title="Alert notifications" />
    </BoxList>
  </Box>
  <Box
    title="Troubleshooting with Netdata Cloud">
    <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/metric-correlations.md)" title="Metric Correlations" />
    <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/dashboard/anomaly-advisor-tab.md)" title="Anomaly Advisor" />
    <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/events-feed.md)" title="Events Feed" />
  </Box>
  <Box
    title="Management and settings">
    <BoxList>
      <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/category-overview-pages/authentication-and-authorization.md)" title="Sign in with email, Google, GitHub or with an SSO tool" />
      <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/organize-your-infrastrucutre-invite-your-team.md#invite-your-team)" title="Invite your team" />
      <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/themes.md)" title="Choose your Netdata Cloud theme" />
      <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/role-based-access.md)" title="Role-Based Access" />
      <BoxListItemRegexLink to="[](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/plans.md)" title="Paid Plans" />
    </BoxList>
  </Box>
</Grid>

- Spaces and War Rooms
  - [Spaces](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/organize-your-infrastrucutre-invite-your-team.md#netdata-cloud-spaces)
  - [War Rooms](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/organize-your-infrastrucutre-invite-your-team.md#netdata-cloud-war-rooms)
- Dashboards
  - [Metrics tab](https://github.com/netdata/netdata/blob/master/docs/dashboard/metrics-tab-and-single-node-tabs.md)
  - [Nodes tab](https://github.com/netdata/netdata/blob/master/docs/dashboard/nodes-tab.md)
  - [Kubernetes](https://github.com/netdata/netdata/blob/master/docs/dashboard/kubernetes-tab.md)
  - [Create new dashboards](https://github.com/netdata/netdata/blob/master/docs/dashboard/dashboards-tab.md)
- Alerts and notifications
  - [View active alerts](https://github.com/netdata/netdata/blob/master/docs/monitor/view-active-alerts.md#netdata-cloud)
  - [Alert notifications](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/notifications.md)
- Troubleshooting with Netdata Cloud
  - [Metric Correlations](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/metric-correlations.md)
  - [Anomaly Advisor](https://github.com/netdata/netdata/blob/master/docs/dashboard/anomaly-advisor-tab.md)
  - [Events Feed](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/events-feed.md)
- Management and settings
  - [Sign in with email, Google, GitHub or with an SSO tool](https://github.com/netdata/netdata/blob/master/docs/category-overview-pages/authentication-and-authorization.md)
  - [Invite your team](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/organize-your-infrastrucutre-invite-your-team.md#invite-your-team)
  - [Choose your Netdata Cloud theme](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/themes.md)
  - [Role-Based Access](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/role-based-access.md)
  - [Paid Plans](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/plans.md)
