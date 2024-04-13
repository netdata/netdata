# Home tab

The Home tab allows users to see an overview of their Room.

## Total nodes

The total number of nodes is presented and dissected by their state, Live, Offline or Stale.

## Active alerts

The number of active alerts is presented in a donut chart, while also having counters for both Critical and Warning alerts.

## Nodes map

A map consisting of node entries allows for quick hoverable information about each node, while also presenting node status in a color-coded way.

The map classification can be altered, allowing the categorization of nodes by:

- Status (e.g. Live)
- OS (e.g. Ubuntu)
- Technology (e.g. Container)
- Agent version (e.g. v1.45.2)
- Replication factor (e.g. Single, Multi)
- Cloud provider (e.g AWS)
- Cloud region (e.g. us-east-1)
- Instance type (e.g. c6a.xlarge)

Color-coding can also be configured between:

- Status (e.g. Live, Offline)
- Connection stability (e.g. Stable, Unstable)
- Replication factor (e.g. None, Single)

## Data replication

There are two views about data replication in the Home tab:

The first bar chart presents the amount of **Parents**, **Children** and **Standalone** nodes.

The second bar chart presents the number of nodes depending on their Replication factor, **None**, **Single** and **Multi**.

## Alerts overview over the last 24h

There are two views that display information about nodes that produced the most alerts and top alerts in the last 24 hours.

The first bar chart presents the nodes that produced the most alerts in a time window of the last 24 hours.

The second table contains the top alerts in the last 24 hours, along with their instance, the occurrences and their duration in seconds.

## Netdata Assistant shortcut

In the Home tab there is a shortcut button in order to start an instant conversation with the [Netdata Assistant](https://github.com/netdata/netdata/edit/master/docs/cloud/netdata-assistant.md).

## Space metrics

There are three key metrics that are displayed in the Home tab, **Metrics collected**, **Charts visualized** and **Alerts configured**.

## Data retention per Nodes

This bar chart shows the number of nodes based on their retention period.
