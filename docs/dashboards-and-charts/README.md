# Dashboards and Charts

This guide explains how to access both **Agent** and **Cloud** dashboards in Netdata, with direct links to explore each section in more detail.

When you access the Netdata dashboard through Cloud, you always see the latest available version.

By default, the **Agent dashboard** also shows the latest version (matching Netdata Cloud). However, there are a few exceptions:

- If the Agent has **no internet access**, it will automatically use the bundled dashboard version.
- Users can manually load the previous dashboard version (for example, through a bookmark) using:  
  `http://NODE:19999/v1`  
  Replace `NODE` with the IP address or hostname of your Agent.

## Main Sections

The Netdata dashboard consists of the following main sections:

- [Home Tab](/docs/dashboards-and-charts/home-tab.md)
- [Nodes Tab](/docs/dashboards-and-charts/nodes-tab.md)
- [Netdata Charts](/docs/dashboards-and-charts/netdata-charts.md)
- [Metrics Tab and Single Node Tabs](/docs/dashboards-and-charts/metrics-tab-and-single-node-tabs.md)
- [Top Tab](/docs/dashboards-and-charts/top-tab.md)
- [Logs Tab](/docs/dashboards-and-charts/logs-tab.md)
- [Dashboards Tab](/docs/dashboards-and-charts/dashboards-tab.md)
- [Alerts Tab](/docs/dashboards-and-charts/alerts-tab.md)
- [Events Tab](/docs/dashboards-and-charts/events-feed.md)

:::tip

Some dashboard features require a Netdata Cloud login and a connected Agent — such as saving chart preferences, accessing custom dashboards, or executing node Functions.

:::

## How to Access the Dashboards

### Netdata Cloud

Access the Cloud dashboard at:  
**<https://app.netdata.cloud/>**

[Sign in or sign up](https://app.netdata.cloud/sign-in) if you don’t have an account yet.

### Netdata Agent

To view your local Netdata Agent dashboard:

- Open a browser and go to:  
  `http://NODE:19999`  
  Replace `NODE` with your Agent’s IP address or hostname.
- If viewing from the same machine where the Agent is installed, use:  
  `http://localhost:19999`
