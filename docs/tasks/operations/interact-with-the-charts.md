<!--
title: "Interact with the charts"
sidebar_label: "Interact with the charts"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/tasks/operations/interact-with-the-charts.md"
learn_status: "Published"
learn_topic_type: "Tasks"
sidebar_position: "1"
learn_rel_path: "Operations"
learn_docs_purpose: "Instructions on how to interact with the charts (buttons, etc)"
learn_repo_doc: "True"
-->

In this task, you will learn how to interact with Netdata's many charts.

Before we begin, let's explain some of the terminology we are going to use:

- "Tooltip" is the small text that appears upon hovering over a button. They help in giving more info about a button's
  function
- "Toolbox" is the box that appears in every chart's top right corner, and it contains tools used in manipulating the
  chart
- The button names used in this task will resemble the tooltips shown while hovering above the individual button

We will begin analyzing all the buttons and the actions from the top of the chart to the bottom, in a left-to-right
fashion.

## Prerequisites

To interact with charts, you will need the following:

- A Netdata Cloud Space with at least one live node claimed to it

## Stopping a chart from updating (and restarting)

You can stop a chart from updating by clicking on it. This will pause all the charts in order to take a better look!

You can then resume updating by double-clicking the chart, or pressing on the **Pause** button on the top bar of the
interface, which will then turn into a **Play** button.

## Top bar

The top tab of every chart consists of five buttons. They allow you to:

- [See the anomalies based on a chart's data](#see-the-anomalies-based-on-a-charts-data)
- [See a chart's information](#see-a-charts-information)
- [Change a chart's line style](#change-a-charts-line-style)
- [Toggle Fullscreen](#toggle-fullscreen)
- [Add a chart to a custom dashboard](#add-a-chart-to-a-custom-dashboard)

### See the anomalies based on a chart's data

You can see the anomaly rate of the current selected dimension (or all of them if none is selected) by clicking
the **See anomalies based on chart data** button.

### See a chart's information

You can see a chart's information by clicking the **Information** button.

After clicking this button, you will be presented with:

- the chart's name
- the plugin name
- the context name
- the chart type

### Change a chart's line style

Clicking the **Line** button allows you to select between **Line**, **Stacked** and **Area** styles, which resemble different ways
of rendering the metrics on the chart.

### Toggle Fullscreen

You can set a chart to fullscreen to have better visibility or focus on one specific charts by clicking
the **Maximize** button.

To bring the chart back to normal size, you can click the same button, now with the tooltip **Minimize**.

### Add a chart to a custom dashboard

You can add a chart to one of your custom dashboards by clicking the **add to dashboard** button.

To read more about custom dashboards, check our Task
on [how to create custom Dashboards](https://github.com/netdata/learn/blob/master/docs/tasks/setup/space-administration/room-management.md#createdelete-custom-dashboards).

## Metrics management bar

This bar allows you to customise what metrics are presented on the chart.

In detail, you can:

- [Group metrics](#group-metrics)
- [Change the way the metrics are presented](#change-the-way-the-metrics-are-presented)

### Group Metrics

To group metrics:

1. Press the **Group by** tab
2. Select any of the available options

### Change the way the metrics are presented

You can change how the grouped metrics are presented, by altering the fields in the sentence that follows the **Group by**
tab.

Depending on how metrics are currently grouped, you are going to have different options, correlated to the current
grouping.

You can always reset the options by pressing the **Reset** button.

## Chart toolbox

The chart toolbox is located in the top right corner of every chart and contains some very handy tools!

With it, you can:

- [Zoom in and out on a chart](#zoom-in-and-out-on-a-chart)
- [Inspect back in time](#inspect-back-in-time)
- [Zoom to a specific timeframe](#zoom-to-a-specific-timeframe)
- [Zoom vertically](#zoom-vertically)
- [Highlight a certain timeframe](#highlight-a-certain-timeframe)
- [Reset to default auto refreshing and zoom levels](#reset-to-default-auto-refreshing-and-zoom-levels)

### Zoom in and out on a chart

You can zoom in and out on a chart by clicking the **zoom in** and **zoom out** buttons on its toolbox.

To reset the zoom level, click the **reset zoom** button.

### Inspect back in time

You can pan forward or backward in time:

1. Select the **Pan** tool from the toolbox
2. Click and drag

Furthermore, you can also define a timeframe to zoom to, by clicking the **date and time** field on the top bar of the
interface, next to the "play/pause" indicator.

From the tab revealed you can:

- Select the preferred timezone on the bottom left of the panel
- Select a predefined timeframe to zoom to from the left bar
- Input a value in the `Last X` and then identify it by minutes, hours, days or months
- Select "from" and "to" dates on the main calendar panel
- Precisely input your preferred date and time in `September 1 2022,7:00` format into the two boxes indicating the start
  and the ending date and time for zooming

CLick **Apply** to zoom all the charts to the selected timeframe.

To reset the view mode, go back to the bar and click the **Clear** button and then click **Apply**.

### Zoom to a specific timeframe

You can zoom into a specific timeframe within the chart:

1. Click the **select and zoom** button in the toolbox
2. Perform a horizontal mouse selection

You can then manipulate the zoom by [zooming in and out on a chart](#zoom-in-and-out-on-a-chart), or you can reset the
zoom by clicking the **reset zoom** button on the toolbox.

### Zoom vertically

You can zoom vertically on a chart:

1. Click the arrow next to **select and zoom** button
2. Click the **select vertical and zoom button**
3. Perform a vertical mouse selection

You can reset the zoom by clicking the **reset zoom** button on the toolbox.

### Highlight a certain timeframe

You can highlight a certain timeframe:

1. Click the **Highlight** button
2. Perform a horizontal mouse selection

While having the **Highlight** tool selected, you can click anywhere in the chart to remove the highlight.

### Reset to default auto refreshing and zoom levels

You can reset the zoom level and enable auto refreshing at any time by clicking the **reset zoom** button on the toolbox.

## Bottom bar

The bottom bar allows you to select which metrics are shown on the chart, the metrics presented here depend on
the **Group by** tab's value from the [Metrics management bar](#metrics-management-bar)
The colors used represent the colors of the currently shown metrics on the chart.

In this bar you can:

- shorten dimensions/metrics by name or value
- click on any given field to show only that on the chart
- use `ctrl` and click to select more than one dimension/metric
- Expand or contract the chart for better visibility.

## Related topics

### Related Concepts

- [From raw metrics to visualization](https://github.com/netdata/learn/blob/master/docs/concepts/visualizations/from-raw-metrics-to-visualization.md)
