# Dashboards and Charts

This guide covers how to access both Agent and Cloud dashboards, along with links to explore specific sections in more detail.

When you access the Netdata dashboard through the Cloud, you'll always have the latest version available.

By default, the Agent dashboard shows the latest version (matching Netdata Cloud). However, there are a few exceptions:

- Without internet access, the Agent can't download the newest dashboards. In this case, it will automatically use the bundled version.
- Users have defined, e.g. through URL bookmark, that they want to see the previous version of the dashboard (accessible `http://NODE:19999/v1`, replacing `NODE` with the IP address or hostname of your Agent).

## Main sections

The Netdata dashboard consists of the following main sections:

- [Home tab](https://github.com/netdata/netdata/blob/master/docs/dashboard/home-tab.md)
- [Nodes tab](https://github.com/netdata/netdata/blob/master/docs/dashboard/nodes-tab.md)
- [Netdata charts](https://github.com/netdata/netdata/blob/master/docs/dashboard/netdata-charts.md)
- [Metrics tab and single node tabs](https://github.com/netdata/netdata/blob/master/docs/dashboard/metrics-tab-and-single-node-tabs.md)
- [Top tab](https://github.com/netdata/netdata/blob/master/docs/cloud/runtime-troubleshooting-with-functions.md)
- [Logs tab](https://github.com/netdata/netdata/blob/master/docs/dashboard/logs-tab.md)
- [Dashboards tab](https://github.com/netdata/netdata/blob/master/docs/dashboard/dashboards-tab.md)
- [Alerts tab](https://github.com/netdata/netdata/blob/master/docs/monitor/view-active-alerts.md)
- [Events tab](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/events-feed.md)

> **Note**
>
> Some sections of the dashboard, when accessed through the agent, may require the user to be signed in to Netdata Cloud or have the Agent claimed to Netdata Cloud for their full functionality. Examples include saving visualization settings on charts or custom dashboards, claiming the node to Netdata Cloud, or executing functions on an Agent.

## How to access the dashboards?

### Netdata Cloud

You can access the dashboard at <https://app.netdata.cloud/> and [sign-in with an account or sign-up](https://github.com/netdata/netdata/blob/master/docs/category-overview-pages/authentication-and-authorization.md) if you don't have an account yet.

### Netdata Agent

To view your Netdata dashboard, open a web browser and enter the address `http://NODE:19999`  - replace `NODE` with your Agent's IP address or hostname. If the Agent is on the same machine, use http://localhost:19999.

Documentation for previous Agent dashboard can still be found [here](https://github.com/netdata/netdata/blob/master/src/web/gui/README.md).
