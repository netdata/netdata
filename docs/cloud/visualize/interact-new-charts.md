<!--
title: "Interact with charts"
description: >-
    "Learn how to get the most out of Netdata's charts. These charts will help you make sense of all the
    metrics at your disposal, helping you troubleshoot with real-time, per-second metric data"
type: "how-to"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/interact-new-charts.md"
sidebar_label: "Interact with charts"
learn_status: "Published"
learn_topic_type: "Concepts"
learn_rel_path: "Operations/Visualizations"
-->

> ⚠️ This new version of charts is currently **only** available on Netdata Cloud. We didn't want to keep this valuable
> feature from you, so after we get this into your hands on the Cloud, we will collect and implement your feedback.
> Together, we will be able to provide the best possible version of charts on the Netdata Agent dashboard, as quickly as
> possible.

Netdata excels in collecting, storing, and organizing metrics in out-of-the-box dashboards.
To make sense of all the metrics, Netdata offers an enhanced version of charts that update every second.

These charts provide a lot of useful information, so that you can:

- Enjoy the high-resolution, granular metrics collected by Netdata
- Explore visualization with more options such as _line_, _stacked_ and _area_ types (other types like _bar_, _pie_ and
  _gauges_ are to be added shortly)
- Examine all the metrics by hovering over them with your cursor
- Use intuitive tooling and shortcuts to pan, zoom or highlight your charts
- On highlight, ease access
  to [Metric Correlations](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/metric-correlations.md) to
  see other metrics with similar patterns
- Have the dimensions sorted based on name or value
- View information about the chart, its plugin, context, and type
- Get the chart status and possible errors. On top, reload functionality

These charts will available
on [Overview tab](https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/overview.md), Single Node view and
on your [Custom Dashboards](https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/dashboards.md).

## Overview

Have a look at the can see the overall look and feel of the charts for both with a composite chart from
the [Overview tab](https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/overview.md) and a simple chart
from the single node view:

![NRve6zr325.gif](https://images.zenhubusercontent.com/60b4ebb03f4163193ec31819/5ecaf5ec-1229-480e-b122-62f63e9df227)

With a quick glance you have immediate information available at your disposal:

- Chart title and units
- Action bars
- Chart area
- Legend with dimensions

## Play, Pause and Reset

Your charts are controlled using the
available [Time controls](https://github.com/netdata/netdata/blob/master/docs/dashboard/visualization-date-and-time-controls.md#time-controls).
Besides these, when interacting with the chart you can also activate these controls by:

- hovering over any chart to temporarily pause it - this momentarily switches time control to Pause, so that you can
  hover over a specific timeframe. When moving out of the chart time control will go back to Play (if it was it's
  previous state)
- clicking on the chart to lock it - this enables the Pause option on the time controls, to the current timeframe. This
  is if you want to jump to a different chart to look for possible correlations.
- double clicking to release a previously locked chart - move the time control back to Play

  ![23CHKCPnnJ.gif](https://images.zenhubusercontent.com/60b4ebb03f4163193ec31819/0b1e111e-df44-4d92-b2e3-be5cfd9db8df)

| Interaction       | Keyboard/mouse | Touchpad/touchscreen | Time control          |
|:------------------|:---------------|:---------------------|:----------------------|
| **Pause** a chart | `hover`        | `n/a`                | Temporarily **Pause** |
| **Stop** a chart  | `click`        | `tap`                | **Pause**             |
| **Reset** a chart | `double click` | `n/a`                | **Play**              |

Note: These interactions are available when the default "Pan" action is used. Other actions are accessible via
the [Exploration action bar](#exploration-action-bar).

## Title and chart action bar

When you start interacting with a chart, you'll notice valuable information on the top bar. You will see information
from the chart title to a chart action bar.

The elements that you can find on this top bar are:

- Netdata icon: this indicates that data is continuously being updated, this happens
  if [Time controls](https://github.com/netdata/netdata/blob/master/docs/dashboard/visualization-date-and-time-controls.md#time-controls)
  are in Play or Force Play mode
- Chart status icon: indicates the status of the chart. Possible values are: Loading, Timeout, Error or No data
- Chart title: on the chart title you can see the title together with the metric being displayed, as well as the unit of
  measurement
- Chart action bar: here you'll have access to chart info, change chart types, enables fullscreen mode, and the ability
  to add the chart to a custom dashboard

![image.png](https://images.zenhubusercontent.com/60b4ebb03f4163193ec31819/c8f5f0bd-5f84-4812-970b-0e4340f4773b)

### Chart action bar

On this bar you have access to immediate actions over the chart, the available actions are:

- Chart info: you will be able to get more information relevant to the chart you are interacting with
- Chart type: change the chart type from _line_, _stacked_ or _area_
- Enter fullscreen mode: allows you expand the current chart to the full size of your screen
- Add chart to dashboard: This allows you to add the chart to an existing custom dashboard or directly create a new one
  that includes the chart.

<img src="https://images.zenhubusercontent.com/60b4ebb03f4163193ec31819/65ac4fc8-3d8d-4617-8234-dbb9b31b4264" width="40%" height="40%" />

## Exploration action bar

When exploring the chart you will see a second action bar. This action bar is there to support you on this task. The
available actions that you can see are:

- Pan
- Highlight
- Horizontal and Vertical zooms
- In-context zoom in and out

<img src="https://images.zenhubusercontent.com/60b4ebb03f4163193ec31819/0417ad66-fcf6-42d5-9a24-e9392ec51f87" width="40%" height="40%" />

### Pan

Drag your mouse/finger to the right to pan backward through time, or drag to the left to pan forward in time. Think of
it like pushing the current timeframe off the screen to see what came before or after.

| Interaction | Keyboard | Mouse          | Touchpad/touchscreen |
|:------------|:---------|:---------------|:---------------------|
| **Pan**     | `n/a`    | `click + drag` | `touch drag`         |

### Highlight

Selecting timeframes is useful when you see an interesting spike or change in a chart and want to investigate further,
from looking at the same period of time on other charts/sections or triggering actions to help you troubleshoot with an
in-context action bar to help you troubleshoot (currently only available on
Single Node view). The available actions:

-

run [Metric Correlations](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/metric-correlations.md)

- zoom in on the selected timeframe

[Metric Correlations](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/metric-correlations.md)
will only be available if you respect the timeframe selection limitations. The selected duration pill together with the
button state helps visualize this.

<img src="https://images.zenhubusercontent.com/60b4ebb03f4163193ec31819/2ffc157d-0f0f-402e-80bb-5ffa8a2091d5" width="50%" height="50%" />

<p/>

| Interaction                        | Keyboard/mouse                                           | Touchpad/touchscreen |
|:-----------------------------------|:---------------------------------------------------------|:---------------------|
| **Highlight** a specific timeframe | `Alt + mouse selection` or `⌘ + mouse selection` (macOS) | `n/a`                |

### Zoom

Zooming in helps you see metrics with maximum granularity, which is useful when you're trying to diagnose the root cause
of an anomaly or outage. Zooming out lets you see metrics within the larger context, such as the last hour, day, or
week, which is useful in understanding what "normal" looks like, or to identify long-term trends, like a slow creep in
memory usage.

The actions above are _normal_ vertical zoom actions. We also provide an horizontal zoom action that helps you focus on
a
specific Y-axis area to further investigate a spike or dive on your charts.

![Y5IESOjD3s.gif](https://images.zenhubusercontent.com/60b4ebb03f4163193ec31819/f8722ee8-e69b-426c-8bcb-6cb79897c177)

| Interaction                                | Keyboard/mouse                       | Touchpad/touchscreen                                 |
|:-------------------------------------------|:-------------------------------------|:-----------------------------------------------------|
| **Zoom** in or out                         | `Shift + mouse scrollwheel`          | `two-finger pinch` <br />`Shift + two-finger scroll` |
| **Zoom** to a specific timeframe           | `Shift + mouse vertical selection`   | `n/a`                                                |
| **Horizontal Zoom** a specific Y-axis area | `Shift + mouse horizontal selection` | `n/a`                                                |

You also have two direct action buttons on the exploration action bar for in-context `Zoom in` and `Zoom out`.

## Other interactions

### Order dimensions legend

The bottom legend of the chart where you can see the dimensions of the chart can now be ordered by:

- Dimension name (Ascending or Descending)
- Dimension value (Ascending or Descending)

<img src="https://images.zenhubusercontent.com/60b4ebb03f4163193ec31819/d3031c35-37bc-46c1-bcf9-be29dea0b476" width="50%" height="50%" />

### Show and hide dimensions

Hiding dimensions simplifies the chart and can help you better discover exactly which aspect of your system might be
behaving strangely.

| Interaction                            | Keyboard/mouse  | Touchpad/touchscreen |
|:---------------------------------------|:----------------|:---------------------|
| **Show one** dimension and hide others | `click`         | `tap`                |
| **Toggle (show/hide)** one dimension   | `Shift + click` | `n/a`                |

### Resize

To resize the chart, click-and-drag the icon on the bottom-right corner of any chart. To restore the chart to its
original height,
double-click the same icon.

![AjqnkIHB9H.gif](https://images.zenhubusercontent.com/60b4ebb03f4163193ec31819/1bcc6a0a-a58e-457b-8a0c-e5d361a3083c)

## What's next?

We recommend you read up on the differences
between [chart dimensions, contexts, and families](https://github.com/netdata/netdata/blob/master/docs/dashboard/dimensions-contexts-families.md)
to strengthen your understanding of how Netdata organizes its dashboards. Another valuable way to interact with charts
is to use
the [date and time controls](https://github.com/netdata/netdata/blob/master/docs/dashboard/visualization-date-and-time-controls.md),
which helps you visualize specific moments of historical metrics.

### Further reading & related information

- Dashboard
    - [How the dashboard works](https://github.com/netdata/netdata/blob/master/docs/dashboard/how-dashboard-works.md)
    - [Chart dimensions, contexts, and families](https://github.com/netdata/netdata/blob/master/docs/dashboard/dimensions-contexts-families.md)
    - [Date and Time controls](https://github.com/netdata/netdata/blob/master/docs/dashboard/visualization-date-and-time-controls.md)
    - [Customize the standard dashboard](https://github.com/netdata/netdata/blob/master/docs/dashboard/customize.md)
    - [Metric Correlations](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/metric-correlations.md)
    - [Netdata Agent - Interact with charts](https://github.com/netdata/netdata/blob/master/docs/dashboard/interact-charts.md)
