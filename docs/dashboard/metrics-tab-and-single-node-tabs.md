# Metrics tab and single node tabs

The Metrics tab is where all the time series [charts](https://github.com/netdata/netdata/blob/master/docs/dashboard/netdata-charts.md) for all the nodes of a War Room are located.

You can also see single-node dashboards, essentially the same dashboard the Metrics tab offers but only for one node. They are reached from most places in the UI, often by clicking the name of a node.

From this tab, a user can also reach the Integrations tab and run [Metric Correlations](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/metric-correlations.md)

## Dashboard structure

The dashboard consists of various charts presented in different chart types. They are categorized based on their [context](https://github.com/netdata/netdata/blob/master/docs/dashboard/netdata-charts.md#contexts) and at the beginning of each section, there is a predefined arrangement of charts helping you to get an overview for that particular section.

## Chart navigation Menu

On the right-hand side, there is a bar that:

- Allows for quick navigation through the sections of the dashboard
- Provides a filtering mechanism that can filter charts by:
  - Host labels
  - Node status
  - Netdata version
  - Individual nodes
- Presents the active alerts for the War Room

From this bar you can also view the maximum chart anomaly rate on each menu section by clicking the `AR%` button.
