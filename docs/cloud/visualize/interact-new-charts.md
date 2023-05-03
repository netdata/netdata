# Interact with charts

Learn how to use Netdata's powerful charts to troubleshoot with real-time, per-second metric data.

Netdata excels in collecting, storing, and organizing metrics in out-of-the-box dashboards.
To make sense of all the metrics, Netdata offers an enhanced version of charts that update every second.

These charts provide a lot of useful information, so that you can:

- Enjoy the high-resolution, granular metrics collected by Netdata
- Explore visualization with more options such as _line_, _stacked_, _area_, _pie_, _gauges_ types and more
- Examine all the metrics by hovering over them with your cursor
- Use intuitive tooling and shortcuts to pan, zoom or highlight your charts
- On highlight, ease access
  to [Metric Correlations](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/metric-correlations.md) to
  see other metrics with similar patterns
- Have the dimensions sorted based on name or value
- View information about the chart, its plugin, context, and type
- Get the chart status and possible errors. On top, reload functionality

These charts are available on Netdata Cloud's
[Overview tab](https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/overview.md), Single Node tab and
on your [Custom Dashboards](https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/dashboards.md).

## Overview

A Netdata chart looks like this:  

<img src=https://user-images.githubusercontent.com/70198089/235935382-6f3d5db1-672c-43e3-a0a1-8157902d7138.png  width="60%"/>

With a quick glance you have immediate information available at your disposal:

- Chart title and units
- Anomaly Rate ribbon
- Definition bar
- Action bars
- Chart area
- Legend with dimensions

## Title and chart action bar

## Title bar

When you start interacting with a chart, you'll notice valuable information on the top bar.

The elements that you can find on this top bar are:

- **Netdata icon**: this indicates that data is continuously being updated, this happens if [Time controls](https://github.com/netdata/netdata/blob/master/docs/dashboard/visualization-date-and-time-controls.md#time-controls) are in Play or Force Play mode.
- **Chart title**: on the chart title you can see the title together with the metric being displayed, as well as the unit of measurement.

![image](https://user-images.githubusercontent.com/70198089/222689197-f9506ca7-a869-40a9-871f-8c4e1fa4b927.png)

Along with seeing chart type, context and units, on this bar you have access to immediate actions over the chart:

- **Chart info**: get more information relevant to the chart you are interacting with.
- **Chart type**: change the chart type from _line_, _stacked_ or _area_.
- **Enter fullscreen mode**: expand the current chart to the full size of your screen.
- **Add chart to dashboard**: add the chart to an existing custom dashboard or directly create a new one that includes the chart.

<img src="https://user-images.githubusercontent.com/70198089/222689501-4116f5fe-e447-4359-83b5-62dadb33f4ef.png" width="40%" height="40%" />

## Definition bar

Each composite chart has a definition bar to provide information and options about the following:

- Group by option
- Aggregate function to be applied in case multiple data sources exist
- Nodes
- Instances
- Dimensions
- Labels
- The aggregate function over time to be applied if one point in the chart consists of multiple data points aggregated

### NIDL framework

<img src="https://user-images.githubusercontent.com/2662304/235475061-44628011-3b1f-4c44-9528-34452018eb89.png" width=200/>

To help users instantly understand and validate the data they see on charts, we developed the NIDL (Nodes, Instances, Dimensions, Labels) framework. This information is visualized on all charts.

You can rapidly access condensed information for collected metrics, grouped by node, monitored instances, dimension, or any key/value label pair.

At the Definition bar of each chart, there are a few drop-down menus. These drop-down menus have 2 functions:

1. Provide additional information about the visualized chart, to help us understand the data we see.
2. Provide filtering and grouping capabilities, altering the query on the fly, to help us get different views of the dataset.

The NIDL framework attaches metadata to every metric we collect to provide for each of them the following consolidated data for the visible time frame:

1. The volume contribution of each metric into the final query. So even if a query comes from 1000 nodes, we can instantly see the contribution of each node in the result. The same goes for instances, dimensions and labels. Especially for labels, Netdata also provides the volume contribution of each label `key:value` pair to the final query, so that we can immediately see for all label values involved in the query how much they affected the chart.
2. The anomaly rate of each of them for the time-frame of the query. This is used to quickly spot which of the nodes, instances, dimensions or labels have anomalies in the requested time-frame.
3. The minimum, average and maximum values of all the points used for the query. This is used to quickly spot which of the nodes, instances, dimensions or labels are responsible for a spike or a dive in the chart.

All of these drop-down menus can be used for instantly filtering the information shown, by including or excluding specific nodes, instances, dimensions or labels. Directly from the drop-down menu, without the need to edit a query string and without any additional knowledge of the underlying data.

![image](https://user-images.githubusercontent.com/43294513/235470150-62a3b9ac-51ca-4c0d-81de-8804e3d733eb.png)

### Group by dropdown

The "Group by" drop-down menu allows selecting 1 or more groupings to be applied at once on the same dataset. Currently it supports:

1. **Group by Node**, to summarize the data of each node, and provide one dimension on the chart for each of the nodes involved. Filtering nodes is supported at the same time, using the nodes drop-down menu.
2. **Group by Instance**, to summarize the data of each instance and provide one dimension on the chart for each of the instances involved. Filtering instances is supported at the same time, using the instances drop-down menu.
3. **Group by Dimension**, so that each metric in the visualization is the aggregation of a single dimension. This provides a per dimension view of the data from all the nodes in the War Room, taking into account filtering criteria if defined.
4. **Group by Label**, to summarize the data for each label value. Multiple label keys can be selected at the same time.

Using this menu, you can slice and dice the data in any possible way, to quickly get different views of it, without the need to edit a query string and without any need to better understand the format of the underlying data.

<img src="https://user-images.githubusercontent.com/43294513/235468819-3af5a1d3-8619-48fb-a8b7-8e8b4cf6a8ff.png" width=800/>

<!-- 
A composite chart grouped by _node_ visualizes a single metric across contributing nodes. If the composite chart has
five
contributing nodes, there will be five lines/areas. This is typically an absolute value of the sum of the dimensions
over each node but there
are some opinionated-but-valuable exceptions where a specific dimension is selected.
Grouping by nodes allows you to quickly understand which nodes in your infrastructure are experiencing anomalous
behavior. -->
<!-- 
A composite chart grouped by _instance_ visualizes each instance of one software or hardware on a node and displays
these as a separate dimension. By grouping the
`disk.io` chart by _instance_, you can visualize the activity of each disk on each node that contributes to the
composite
chart. -->

A very pertinent example is composite charts over contexts related to cgroups (VMs and containers).
You have the means to change the default group by or apply filtering to get a better view into what data your are trying to analyze.
For example, if you change the group by to _instance_ you get a view with the data of all the instances (cgroups) that contribute to that chart.
Then you can use further filtering tools to focus the data that is important to you and even save the result to your own dashboards.

<!-- ![image](https://user-images.githubusercontent.com/82235632/201902017-04b76701-0ff9-4498-aa9b-6d507b567bea.png) -->

### Aggregate functions over data sources dropdown

Each chart uses an opinionated-but-valuable default aggregate function over the data sources.

For example, the `system.cpu` chart shows the average for each dimension from every contributing chart, while the `net.net` chart shows the sum for each dimension from every contributing chart, which can also come from multiple networking interfaces.

The following aggregate functions are available for each selected dimension:

- **Average**: Displays the average value from contributing nodes. If a composite chart has 5 nodes with the following
  values for the `out` dimension&mdash;`-2.1`, `-5.5`, `-10.2`, `-15`, `-0.1`&mdash;the composite chart displays a
  value of `−6.58`.
- **Sum**: Displays the sum of contributed values. Using the same nodes, dimension, and values as above, the composite
  chart displays a metric value of `-32.9`.
- **Min**: Displays a minimum value. For dimensions with positive values, the min is the value closest to zero. For
  charts with negative values, the min is the value with the largest magnitude.
- **Max**: Displays a maximum value. For dimensions with positive values, the max is the value with the largest
  magnitude. For charts with negative values, the max is the value closet to zero.

### Nodes dropdown

Click on **X Nodes** to display a dropdown of nodes contributing to that composite chart. Each line displays a hostname
to help you identify which nodes contribute to a chart. You can also use this component to filter nodes directly on the
chart.

If one or more nodes can't contribute to a given chart, the definition bar shows a warning symbol plus the number of
affected nodes, then lists them in the dropdown along with the associated error. Nodes might return errors because of
networking issues, a stopped `netdata` service, or because that node does not have any metrics for that context.

### Instances dropdown

Click on **X Instances** to display a dropdown of instances and nodes contributing to that composite chart. Each line in
the dropdown displays an instance name and the associated node's hostname.

### Dimensions dropdown

Select which dimensions to display on the composite chart. You can choose **All dimensions**, a single dimension, or any
number of dimensions available on that context.

### Aggregate functions over time

When the granularity of the data collected is higher than the plotted points on the chart an aggregation function over
time is applied.

By default the aggregation applied is _average_ but the user can choose different options from the following:

- Min
- Max
- Average
- Sum
- Incremental sum (Delta)
- Standard deviation
- Median
- Single exponential smoothing
- Double exponential smoothing
- Coefficient variation
- Trimmed Median `*`
- Trimmed Mean `*`
- Percentile `**`

> ### Info
>
> - `*` For **Trimmed Median and Mean** you can choose the percentage of data tha you want to focus on: 1%, 2%, 3%, 5%, 10%, 15%, 20% and 25%.
> - `**` For **Percentile** you can specify the percentile you want to focus on: 25th, 50th, 75th, 80th, 90th, 95th, 97th, 98th and 99th.

For more details on each, you can refer to our Agent's HTTP API details
on [Data Queries - Data Grouping](https://github.com/netdata/netdata/blob/master/web/api/queries/README.md#data-grouping).

### Reset to defaults

Finally, you can reset everything to its defaults by clicking the green "Reset" prompt at the end of the definition bar.

## Anomaly Rate ribbon

Netdata's unsupervised machine learning algorithm creates a unique model for each metric collected by your agents, using exclusively the metric's past data.
It then uses these unique models during data collection to predict the value that should be collected and check if the collected value is within the range of acceptable values based on past patterns and behavior.

If the value collected is an outlier, it is marked as anomalous.

This unmatched capability of real-time predictions as data is collected allows you to **detect anomalies for potentially millions of metrics across your entire infrastructure within a second of occurrence**.

The Anomaly Rate ribbon on top of each chart visualizes the combined anomaly rate of all the underlying data, highlighting areas of interest that may not be easily visible to the naked eye.

Hovering over the Anomaly Rate ribbon provides a histogram of the anomaly rates per presented dimension, for the specific point in time.  
<img src="https://user-images.githubusercontent.com/43294513/235494042-2c2a0c17-3681-4709-8f39-577e23aebfe6.png" width=500/>

Anomaly Rate visualization does not make Netdata slower. Anomaly rate is saved in the the Netdata database, together with metric values, and due to the smart design of Netdata, it does not even incur a disk footprint penalty.

## Hover over the chart

Hovering over any point in the chart will reveal a more informative overlay.
It includes a bar indicating the volume percentage of each time series compared to the total, the anomaly rate, and a notification on if there are data collection issues (annotations from the info ribbon).

This overlay sorts all dimensions by value, makes bold the closest dimension to the mouse and presents a histogram based on the values of the dimensions.

<img src="https://user-images.githubusercontent.com/43294513/235470493-49cf7890-5d21-4aca-9e8d-5d0874d56389.png" width=600/>

When hovering the anomaly ribbon, the overlay sorts all dimensions by anomaly rate, and presents a histogram of these anomaly rates.

#### Annotations

Additionally, when hovering over the chart, the overlay may display an indication in the "Annotations" column.

Currently, annotations are used to inform users of any data collection issues that might affect the chart.
Below each chart, we added an information ribbon. This ribbon currently shows 3 states related to the points presented in the chart:

1. **[P]: Partial Data**
   At least one of the dimensions in the chart has partial data, meaning that not all instances available contributed data to this point. This can happen when a container is stopped, or when a node is restarted. This indicator helps to gain confidence of the dataset, in situations when unusual spikes or dives appear due to infrastructure maintenance, or due to failures to part of the infrastructure.

2. **[O]: Overflown**
   At least one of the datasources included in the chart was a counter that has overflown that exactly that point.

3. **[E]: Empty Data**
   At least one of the dimensions included in the chart has no data at all for the given points.

All these indicators are also visualized per dimension, in the pop-over that appears when hovering the chart.

<img src="https://user-images.githubusercontent.com/43294513/235477397-753941a6-66ae-4979-b982-dfe3bd9ab598.png" width=600/>

## Play, Pause and Reset

Your charts are controlled using the available [Time controls](https://github.com/netdata/netdata/blob/master/docs/dashboard/visualization-date-and-time-controls.md#time-controls).
Besides these, when interacting with the chart you can also activate these controls by:

- Hovering over any chart to temporarily pause it - this momentarily switches time control to Pause, so that you can
  hover over a specific timeframe. When moving out of the chart time control will go back to Play (if it was it's
  previous state)
- Clicking on the chart to lock it - this enables the Pause option on the time controls, to the current timeframe. This
  is if you want to jump to a different chart to look for possible correlations.
- Double clicking to release a previously locked chart - move the time control back to Play

| Interaction       | Keyboard/mouse | Touchpad/touchscreen | Time control          |
|:------------------|:---------------|:---------------------|:----------------------|
| **Pause** a chart | `hover`        | `n/a`                | Temporarily **Pause** |
| **Stop** a chart  | `click`        | `tap`                | **Pause**             |
| **Reset** a chart | `double click` | `n/a`                | **Play**              |

Note: These interactions are available when the default "Pan" action is used. Other actions are accessible via
the [Tool Bar](#tool-bar).

## Jump to single-node dashboards

Click on **X Charts**/**X Nodes** to display one of the two dropdowns that list the charts and nodes contributing to a
given composite chart. For example, the nodes dropdown.

![The nodes dropdown in a composite chart](https://user-images.githubusercontent.com/1153921/99305049-7c019b80-2810-11eb-942a-8ebfcf236b7f.png)

To jump to a single-node dashboard, click on the link icon
<img class="img__inline img__inline--link" src="https://user-images.githubusercontent.com/1153921/95762109-1d219300-0c62-11eb-8daa-9ba509a8e71c.png" /> next to the
node you're interested in.

The single-node dashboard opens in a new tab. From there, you can continue to troubleshoot or run
[Metric Correlations](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/metric-correlations.md) for faster root
cause analysis.

## Add composite charts to a dashboard

Click on the 3-dot icon (**⋮**) on any chart, then click on **Add to Dashboard**. Click the **+** button for any
dashboard you'd like to add this composite chart to, or create a new dashboard an initiate it with your chosen chart by
entering the name and clicking **New Dashboard**.

## Tool bar

While exploring the chart, a tool bar will appear. This tool bar is there to support you on this task.
The available manipulation tools you can select are:

- Pan
- Highlight
- Select and zoom
- Chart zoom
- Reset zoom

<img src="https://user-images.githubusercontent.com/70198089/222689556-58ad77bc-924f-4c3f-b38b-fc63de2f5773.png" width="40%" height="40%" />

### Pan

Drag your mouse/finger to the right to pan backward through time, or drag to the left to pan forward in time. Think of
it like pushing the current timeframe off the screen to see what came before or after.

| Interaction | Keyboard | Mouse          | Touchpad/touchscreen |
|:------------|:---------|:---------------|:---------------------|
| **Pan**     | `n/a`    | `click + drag` | `touch drag`         |

### Highlight

Selecting timeframes is useful when you see an interesting spike or change in a chart and want to investigate further by:

- Looking at the same period of time on other charts/sections
- Running [metric correlations](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/metric-correlations.md) to filter metrics that also show something different in the selected period, vs the previous one

<img alt="image" src="https://user-images.githubusercontent.com/43294513/221365853-1142944a-ace5-484a-a108-a205d050c594.png" />

| Interaction                        | Keyboard/mouse                                           | Touchpad/touchscreen |
|:-----------------------------------|:---------------------------------------------------------|:---------------------|
| **Highlight** a specific timeframe | `Alt + mouse selection` or `⌘ + mouse selection` (macOS) | `n/a`                |

### Select and zoom

The actions above are _normal_ vertical zoom actions. We also provide an horizontal zoom action that helps you focus on
a specific Y-axis area to further investigate a spike or dive on your charts.

| Interaction                                | Keyboard/mouse                       | Touchpad/touchscreen                                 |
|:-------------------------------------------|:-------------------------------------|:-----------------------------------------------------|
| **Zoom** to a specific timeframe           | `Shift + mouse vertical selection`   | `n/a`                                                |
| **Horizontal Zoom** a specific Y-axis area | `Shift + mouse horizontal selection` | `n/a`                                                |

<img alt="gif" src=https://user-images.githubusercontent.com/70198089/222689676-ad16a2a0-3c3d-48fa-87af-c40ae142dd79.gif width="70%"/>

### Chart zoom

Zooming in helps you see metrics with maximum granularity, which is useful when you're trying to diagnose the root cause
of an anomaly or outage. Zooming out lets you see metrics within the larger context, such as the last hour, day, or
week, which is useful in understanding what "normal" looks like, or to identify long-term trends, like a slow creep in
memory usage.

| Interaction                                | Keyboard/mouse                       | Touchpad/touchscreen                                 |
|:-------------------------------------------|:-------------------------------------|:-----------------------------------------------------|
| **Zoom** in or out                         | `Shift + mouse scrollwheel`          | `two-finger pinch` <br />`Shift + two-finger scroll` |

## Dimensions bar

### Order dimensions legend

The bottom legend of the chart where you can see the dimensions of the chart can now be ordered by:

- Dimension name (Ascending or Descending)
- Dimension value (Ascending or Descending)

<img src="https://user-images.githubusercontent.com/70198089/222689791-48c77890-1093-4beb-84c2-7598353ca049.png" width="50%" height="50%" />

### Show and hide dimensions

Hiding dimensions simplifies the chart and can help you better discover exactly which aspect of your system might be
behaving strangely.

| Interaction                            | Keyboard/mouse  | Touchpad/touchscreen |
|:---------------------------------------|:----------------|:---------------------|
| **Show one** dimension and hide others | `click`         | `tap`                |
| **Toggle (show/hide)** one dimension   | `Shift + click` | `n/a`                |

## Resize a chart

To resize the chart, click-and-drag the icon on the bottom-right corner of any chart. To restore the chart to its
original height,
double-click the same icon.

![1bcc6a0a-a58e-457b-8a0c-e5d361a3083c](https://user-images.githubusercontent.com/70198089/222689845-51a9c054-a57d-49dc-925d-39b924dae2f8.gif)
