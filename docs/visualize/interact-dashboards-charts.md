<!--
title: "Interact with dashboards and charts"
description: "Zoom, highlight, and pan through time on hundreds of real-time, interactive charts to quickly discover the root cause of any anomaly."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/visualize/interact-dashboards-charts.md
-->

# Interact with dashboards and charts

You can find Netdata's dashboards in two place: locally served at `http://NODE:19999` by the Netdata Agent, and in
Netdata Cloud. While you access these dashboards differently, they have similar interfaces, identical charts and
metrics, and you interact with both of them the same way.

> If you're not sure which option is best for you, see our [single-node](/docs/quickstart/single-node.md) and
> [infrastructure](/docs/quickstart/infrastructure.md) quickstart guides.

Netdata dashboards are a single, scrollable pages with many charts stacked on top of one another. As you scroll up or
down, charts appearing in your browser's viewport automatically load and update every second.

![A single-node dashboard in Netdata
Cloud](https://user-images.githubusercontent.com/1153921/93402406-82d45800-f839-11ea-8bf0-b6faa9411567.png)

The dashboard is broken up into multiple **sections**, such as **System Overview**, **CPU**, **Disk**, which are
automatically generated based on which [collectors](/docs/collect/how-collectors-work.md) begin collecting metrics when
Netdata starts up. These sections are also represented in the menu on the right-hand side of the dashboard, which also
features submenus based on the contexts and families Netdata creates for your node.

## Charts, dimensions, families, and contexts

A **chart** is an interactive visualization of one or more collected/calculated metrics. You can see the name (also
known as its unique ID) of a chart by looking at the top-left corner of a chart and finding the parenthesized text. On a
Linux system, one of the first charts on the dashboard will be the system CPU chart, with the name `system.cpu`.

A **dimension** is any value that gets shown on a chart. The value can be raw data or calculated values, such as
percentages, aggregates, and more. Most charts will have more than one dimension, in which case it will display each in
a different color. You can disable or enable showing these dimensions by clicking on them.

A **family** is _one_ instance of a monitored hardware or software resource that needs to be monitored and displayed
separately from similar instances. For example, if your node has multiple partitions, Netdata will create different
families for `/`, `/boot`, `/home`, and so on. Same goes for entire disks, network devices, and more.

A **context** groups several charts based on the types of metrics being collected and displayed. For example, the
**Disk** section often has many contexts: `disk.io`, `disk.ops`, `disk.backlog`, `disk.util`, and so on. Netdata uses
this context to create individual charts and then groups them by family. You can always see the context of any chart by
looking at its name or hovering over the chart's date.

See our [dashboard docs](/web/README.mdweb#charts-contexts-families) for more information about the above distinctions
and how they're used across Netdata to meaningfully organize and present metrics.

## Interact with charts

Netdata's charts are fully interactive to help you find meaningful information about complex problems. You can pan
through historical metrics, zoom in and out, select specific timeframes for further analysis, resize charts, and more.
Whenever you use a chart in this way, Netdata synchronizes all the other charts to match it.

| Change                                            | Method #1                           | Method #2                                                 | Method #3                                                  |
| ------------------------------------------------- | ----------------------------------- | --------------------------------------------------------- | ---------------------------------------------------------- |
| **Stop** a chart from updating                    | `click`                             |                                                           |                                                            |
| **Reset** charts to default auto-refreshing state | `double click`                      | `double tap` (touchpad/touchscreen)                       |                                                            |
| **Select** a certain timeframe                    | `ALT` + `mouse selection`           | `⌘` + `mouse selection` (macOS)                           |                                                            |
| **Pan** forward or back in time                   | `click and drag`                    | `touch and drag` (touchpad/touchscreen)                   |                                                            |
| **Zoom** to a specific timeframe                  | `SHIFT` + `mouse selection`         |                                                           |                                                            |
| **Zoom** in/out                                   | `SHIFT`/`ALT` + `mouse scrollwheel` | `SHIFT`/`ALT` + `two-finger pinch` (touchpad/touchscreen) | `SHIFT`/`ALT` + `two-finger scroll` (touchpad/touchscreen) |

![Animated GIF of the standard Netdata dashboard being manipulated and synchronizing
charts](https://user-images.githubusercontent.com/1153921/80839230-b034a800-8baf-11ea-9cb2-99c1e10f0f85.gif)

These interactions can also be triggered using the icons on the bottom-right corner of every chart. They are,
respectively, `Pan Left`, `Reset`, `Pan Right`, `Zoom In`, and `Zoom Out`.

You can show and hide individual dimensions by clicking on their names. Hiding dimensions simplifies the chart and can
help you better discover exactly which aspect of your system is behaving strangely.

You can resize any chart by clicking-and-dragging the icon on the bottom-right corner of any chart. To restore the chart
to its original height, double-click the same icon.

![Animated GIF of resizing a chart and resetting it to the default
height](https://user-images.githubusercontent.com/1153921/80842459-7d41e280-8bb6-11ea-9488-1bc29f94d7f2.gif)

## What's next?

Netdata Cloud users can [build new dashboards](/docs/visualize/create-dashboards.md) in just a few clicks. By
aggregating relevant metrics from any number of nodes onto a single interface, you can respond faster to anomalies,
perform more targeted troubleshooting, or keep tabs on a bird's eye view of your infrastructure.

If you're finished with dashboards for now, skip to Netdata's health watchdog for information on [creating or
configuring](/docs/monitor/configure-alarms.md) alarms, and [send notifications](/docs/monitor/enable-notifications.md)
to get informed when something goes wrong in your infrastructure.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fvisualize%2Finteract-dashboards-charts&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
