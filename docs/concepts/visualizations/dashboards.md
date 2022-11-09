<!--
title: "Dashboards"
sidebar_label: "Dashboards"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/concepts/visualizations/dashboards.md"
sidebar_position: "2400"
learn_status: "Published"
learn_topic_type: "Concepts"
learn_rel_path: "Concepts/Visualizations"
learn_docs_purpose: "what we define as a Dashboard, explain the overview dashboard, Kubernetes dashboard, custom dashboards, legacy dashboard of the Agent"
-->


Netdata excels in collecting, storing, and organizing metrics in out-of-the-box dashboards. 
To make sense of all the metrics, Netdata offers charts that update every second. 

These charts provide a lot of useful information, so that you can:

-   Enjoy the high-resolution, granular metrics collected by Netdata
-   Explore visualization with more options such as _line_, _stacked_ and _area_ types (other types like _bar_, _pie_ and _gauges_ are to be added shortly)
-   Examine all the metrics by hovering over them with your cursor
-   Use intuitive tooling and shortcuts to pan, zoom or highlight your charts
-   On highlight, ease access to [Metric Correlations](/docs/cloud/insights/metric-correlations) to see other metrics with similar patterns
-   Have the dimensions sorted based on name or value
-   View information about the chart, its plugin, context, and type
-   Get the chart status and possible errors. On top, reload functionality

These charts will available on [Overview tab](/docs/cloud/visualize/overview), Single Node view and on your [Custom Dashboards](/docs/cloud/visualize/dashboards). 

Now that you're fully connected within your Cloud and Agent, and you understand how Netdata drives raw metrics to useful data, we can break down the basics of how you can employ Netdata's industry-leading visualizations to your monitoring and troubleshooting plan. 


## Interactive composite charts on Cloud

:::note ⚠️ This version of charts is currently **only** available on Netdata Cloud. We didn't want to keep this valuable
> feature from you, so after we get this into your hands on the Cloud, we will collect and implement your feedback. Together, we will be able to provide the best possible version of charts on the Netdata Agent dashboard, as quickly as possible.:::

Netdata excels in collecting, storing, and organizing metrics in out-of-the-box dashboards. 
To make sense of all the metrics, Netdata offers an enhanced version of charts that update every second. 

These charts provide a lot of useful information, so that you can:

-   Enjoy the high-resolution, granular metrics collected by Netdata
-   Explore visualization with more options such as _line_, _stacked_ and _area_ types (other types like _bar_, _pie_ and _gauges_ are to be added shortly)
-   Examine all the metrics by hovering over them with your cursor
-   Use intuitive tooling and shortcuts to pan, zoom or highlight your charts
-   On highlight, ease access to [Metric Correlations](/docs/cloud/insights/metric-correlations) to see other metrics with similar patterns
-   Have the dimensions sorted based on name or value
-   View information about the chart, its plugin, context, and type
-   Get the chart status and possible errors. On top, reload functionality

These charts will available on [Overview tab](/docs/cloud/visualize/overview), Single Node view, and on your [Custom Dashboards](/docs/cloud/visualize/dashboards). 

With a quick glance at any composite chart, you have immediate information available at your disposal:

-   Chart title and units
-   Action bars
-   Chart area
-   Legend with dimensions

The following sections explain how you can interact with these composite charts in greater detail.

### Play, Pause and Reset

Your charts are controlled using the available [Time controls](/docs/dashboard/visualization-date-and-time-controls#time-controls). Besides these, when interacting with the chart you can also activate these controls by:

-   hovering over any chart to temporarily pause it - this momentarily switches time control to Pause, so that you can hover over a specific timeframe. When moving out of the chart time control will go back to Play (if it was it's previous state)
-   clicking on the chart to lock it - this enables the Pause option on the time controls, to the current timeframe. This is if you want to jump to a different chart to look for possible correlations. 
-   double clicking to release a previously locked chart - move the time control back to Play

 
| Interaction       | Keyboard/mouse | Touchpad/touchscreen | Time control          |
| :---------------- | :------------- | :------------------- | :-------------------- |
| **Pause** a chart | `hover`        | `n/a`                | Temporarily **Pause** |
| **Stop** a chart  | `click`        | `tap`                | **Pause**             |
| **Reset** a chart | `double click` | `n/a`                | **Play**              |

Note: These interactions are available when the default "Pan" action is used. Other actions are accessible via the [Exploration action bar](#exploration-action-bar).

### Title and chart action bar

When you start interacting with a chart, you'll notice valuable information on the top bar. You will see information from the chart title to a chart action bar.

The elements that you can find on this top bar are:

-   Netdata icon: this indicates that data is continuously being updated, this happens if [Time controls](/docs/dashboard/visualization-date-and-time-controls#time-controls) are in Play or Force Play mode
-   Chart status icon: indicates the status of the chart. Possible values are: Loading, Timeout, Error or No data
-   Chart title: on the chart title you can see the title together with the metric being displayed, as well as the unit of measurement
-   Chart action bar: here you'll have access to chart info, change chart types, enables fullscreen mode, and the ability to add the chart to a custom dashboard

![image.png](https://images.zenhubusercontent.com/60b4ebb03f4163193ec31819/c8f5f0bd-5f84-4812-970b-0e4340f4773b)

#### Chart action bar

On this bar you have access to immediate actions over the chart, the available actions are:

-   Chart info: you will be able to get more information relevant to the chart you are interacting with
-   Chart type: change the chart type from _line_, _stacked_ or _area_
-   Enter fullscreen mode: allows you expand the current chart to the full size of your screen
-   Add chart to dashboard: This allows you to add the chart to an existing custom dashboard or directly create a new one that includes the chart.

<img src="https://images.zenhubusercontent.com/60b4ebb03f4163193ec31819/65ac4fc8-3d8d-4617-8234-dbb9b31b4264" width="40%" height="40%" />

### Exploration action bar

When exploring the chart you will see a second action bar. This action bar is there to support you on this task. The available actions that you can see are:

-   Pan
-   Highlight
-   Horizontal and Vertical zooms
-   In-context zoom in and out

<img src="https://images.zenhubusercontent.com/60b4ebb03f4163193ec31819/0417ad66-fcf6-42d5-9a24-e9392ec51f87" width="40%" height="40%" />

#### Pan

Drag your mouse/finger to the right to pan backward through time, or drag to the left to pan forward in time. Think of it like pushing the current timeframe off the screen to see what came before or after.

| Interaction | Keyboard | Mouse          | Touchpad/touchscreen |
| :---------- | :------- | :------------- | :------------------- |
| **Pan**     | `n/a`    | `click + drag` | `touch drag`         |

#### Highlight

Selecting timeframes is useful when you see an interesting spike or change in a chart and want to investigate further, from looking at the same period of time on other charts/sections or triggering actions to help you troubleshoot with an in-context action bar to help you troubleshoot (currently only available on
 Single Node view). The available actions:

-   run [Metric Correlations](/docs/cloud/insights/metric-correlations)
-   zoom in on the selected timeframe

[Metric Correlations](/docs/cloud/insights/metric-correlations) will only be available if you respect the timeframe selection limitations. The selected duration pill together with the button state helps visualize this.

<img src="https://images.zenhubusercontent.com/60b4ebb03f4163193ec31819/2ffc157d-0f0f-402e-80bb-5ffa8a2091d5" width="50%" height="50%" />

<p/>

| Interaction                        | Keyboard/mouse                                           | Touchpad/touchscreen |
| :--------------------------------- | :------------------------------------------------------- | :------------------- |
| **Highlight** a specific timeframe | `Alt + mouse selection` or `⌘ + mouse selection` (macOS) | `n/a`                |

### Zoom

Zooming in helps you see metrics with maximum granularity, which is useful when you're trying to diagnose the root cause
of an anomaly or outage. Zooming out lets you see metrics within the larger context, such as the last hour, day, or
week, which is useful in understanding what "normal" looks like, or to identify long-term trends, like a slow creep in
memory usage.

The actions above are _normal_ vertical zoom actions. We also provide an horizontal zoom action that helps you focus on a 
specific Y-axis area to further investigate a spike or dive on your charts.

![Y5IESOjD3s.gif](https://images.zenhubusercontent.com/60b4ebb03f4163193ec31819/f8722ee8-e69b-426c-8bcb-6cb79897c177)

| Interaction                                | Keyboard/mouse                       | Touchpad/touchscreen                                 |
| :----------------------------------------- | :----------------------------------- | :--------------------------------------------------- |
| **Zoom** in or out                         | `Shift + mouse scrollwheel`          | `two-finger pinch` <br />`Shift + two-finger scroll` |
| **Zoom** to a specific timeframe           | `Shift + mouse vertical selection`   | `n/a`                                                |
| **Horizontal Zoom** a specific Y-axis area | `Shift + mouse horizontal selection` | `n/a`                                                |

You also have two direct action buttons on the exploration action bar for in-context `Zoom in` and `Zoom out`.

### Other interactions

#### Order dimensions legend

The bottom legend of the chart where you can see the dimensions of the chart can now be ordered by:

-   Dimension name (Ascending or Descending)
-   Dimension value (Ascending or Descending)

<img src="https://images.zenhubusercontent.com/60b4ebb03f4163193ec31819/d3031c35-37bc-46c1-bcf9-be29dea0b476" width="50%" height="50%" />

#### Show and hide dimensions

Hiding dimensions simplifies the chart and can help you better discover exactly which aspect of your system might be
behaving strangely.

| Interaction                            | Keyboard/mouse  | Touchpad/touchscreen |
| :------------------------------------- | :-------------- | :------------------- |
| **Show one** dimension and hide others | `click`         | `tap`                |
| **Toggle (show/hide)** one dimension   | `Shift + click` | `n/a`                |

#### Resize

To resize the chart, click-and-drag the icon on the bottom-right corner of any chart. To restore the chart to its original height,
double-click the same icon.

![AjqnkIHB9H.gif](https://images.zenhubusercontent.com/60b4ebb03f4163193ec31819/1bcc6a0a-a58e-457b-8a0c-e5d361a3083c)


## Kubernetes Dashboard

Netdata Cloud features enhanced visualizations for the resource utilization of Kubernetes (k8s) clusters, embedded in
the default [Overview](/docs/cloud/visualize/overview/) dashboard.

These visualizations include a health map for viewing the status of k8s pods/containers, in addition to composite charts
for viewing per-second CPU, memory, disk, and networking metrics from k8s nodes.

### Requirements

In order to use the Kubernetes visualizations in Netdata Cloud, you need:

- A Kubernetes cluster running Kubernetes v1.9 or newer.
- A Netdata deployment using the latest version of the [Helm chart](https://github.com/netdata/helmchart), which
  installs [v1.29.2](https://github.com/netdata/netdata/releases) or newer of the Netdata Agent.
- To connect your Kubernetes cluster to Netdata Cloud.
- To enable the feature flag described below.

See our [Kubernetes deployment instructions](/docs/agent/packaging/installer/methods/kubernetes/) for details on
installation and connecting to Netdata Cloud.

### Available Kubernetes metrics

Netdata Cloud organizes and visualizes the following metrics from your Kubernetes cluster from every container:

- `cpu_limit`: CPU utilization as a percentage of the limit defined by the [pod specification
  `spec.containers[].resources.limits.cpu`](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#resource-requests-and-limits-of-pod-and-container)
  or a [`LimitRange`
  object](https://kubernetes.io/docs/tasks/administer-cluster/manage-resources/cpu-default-namespace/#create-a-limitrange-and-a-pod).
- `cpu`: CPU utilization of the pod/container. 100% usage equals 1 fully-utilized core, 200% equals 2 fully-utilized
  cores, and so on.
- `cpu_per_core`: CPU utilization averaged across available cores.
- `mem_usage_limit`: Memory utilization, without cache, as a percentage of the limit defined by the [pod specification
  `spec.containers[].resources.limits.memory`](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#resource-requests-and-limits-of-pod-and-container)
  or a [`LimitRange`
  object](https://kubernetes.io/docs/tasks/administer-cluster/manage-resources/cpu-default-namespace/#create-a-limitrange-and-a-pod).
- `mem_usage`: Used memory, without cache.
- `mem`: The sum of `cache` and `rss` (resident set size) memory usage.
- `writeback`: The size of `dirty` and `writeback` cache.
- `mem_activity`: Sum of `in` and `out` bandwidth.
- `pgfaults`: Sum of page fault bandwidth, which are raised when the Kubernetes cluster tries accessing a memory page
  that is mapped into the virtual address space, but not actually loaded into main memory.
- `throttle_io`: Sum of `read` and `write` per second across all PVs/PVCs attached to the container.
- `throttle_serviced_ops`: Sum of the `read` and `write` operations per second across all PVs/PVCs attached to the
  container.
- `net.net`: Sum of `received` and `sent` bandwidth per second.
- `net.packets`: Sum of `multicast`, `received`, and `sent` packets.

When viewing the [health map](#health-map), Netdata Cloud shows the above metrics per container, or aggregated based on
their associated pods.

When viewing the [composite charts](#composite-charts), Netdata Cloud aggregates metrics from multiple nodes, pods, or
containers, depending on the grouping chosen. For example, if you group the `cpu_limit` composite chart by
`k8s_namespace`, the metrics shown will be the average of `cpu_limit` metrics from all nodes/pods/containers that are
part of that namespace.

### Health map

The health map places each container or pod as a single box, then varies the intensity of its color to visualize the
resource utilization of specific k8s pods/containers.

![The Kubernetes health map in Netdata
Cloud](https://user-images.githubusercontent.com/1153921/106964367-39f54100-66ff-11eb-888c-5a04f8abb3d0.png)

Change the health map's coloring, grouping, and displayed nodes to customize your experience and learn more about the
status of your k8s cluster.

#### Color by

Color the health map by choosing an aggregate function to apply to an [available Kubernetes
metric](#available-kubernetes-metrics), then whether you to display boxes for individual pods or containers. 

The default is the _average, of CPU within the configured limit, organized by container_.

#### Group by

Group the health map by the `k8s_cluster_id`, `k8s_controller_kind`, `k8s_controller_name`, `k8s_kind`, `k8s_namespace`,
and `k8s_node_name`. The default is `k8s_controller_name`.

#### Filtering

Filtering behaves identically to the [node filter in War Rooms](/docs/cloud/war-rooms#node-filter), with the ability to
filter pods/containers by `container_id` and `namespace`.

#### Detailed information

Hover over any of the pods/containers in the map to display a modal window, which contains contextual information
and real-time metrics from that resource.

![The modal containing additional information about a k8s
resource](https://user-images.githubusercontent.com/1153921/106964369-3a8dd780-66ff-11eb-8a8a-a5c8f0d5711f.png)

The **context** tab provides the following details about a container or pod:

- Cluster ID
- Node
- Controller Kind
- Controller Name
- Pod Name
- Container
- Kind
- Pod UID

This information helps orient you as to where the container/pod operates inside your cluster.

The **Metrics** tab contains charts visualizing the last 15 minutes of the same metrics available in the [color by
option](#color-by). Use these metrics along with the context, to identify which containers or pods are experiencing
problematic behavior to investigate further, troubleshoot, and remediate with `kubectl` or another tool.

### Composite charts

The Kubernetes composite charts show real-time and historical resource utilization metrics from nodes, pods, or
containers within your Kubernetes deployment.

See the [Overview](/docs/cloud/visualize/overview#definition-bar) doc for details on how composite charts work. These
work similarly, but in addition to visualizing _by dimension_ and _by node_, Kubernetes composite charts can also be
grouped by the following labels:

- `k8s_cluster_id`
- `k8s_container_id`
- `k8s_container_name`
- `k8s_controller_kind`
- `k8s_kind`
- `k8s_namespace`
- `k8s_node_name`
- `k8s_pod_name`
- `k8s_pod_uid`

![Composite charts of Kubernetes metrics in Netdata
Cloud](https://user-images.githubusercontent.com/1153921/106964370-3a8dd780-66ff-11eb-8858-05b2253b25c6.png)

In addition, when you hover over a composite chart, the colors in the heat map changes as well, so you can see how
certain pod/container-level metrics change over time.

### Caveats

There are some caveats and known issues with Kubernetes monitoring with Netdata Cloud.

- **No way to remove any nodes** you might have
  [drained](https://kubernetes.io/docs/tasks/administer-cluster/safely-drain-node/) from your Kubernetes cluster. These
  drained nodes will be marked "unreachable" and will show up in War Room management screens/dropdowns. The same applies
  for any ephemeral nodes created and destroyed during horizontal scaling.
  
## Agent dashboard

You can access the Netdata Agent dashboard by navigating to `http://NODE:19999` in your browser, replacing `NODE` with either
`localhost` or the hostname/IP address of a remote node.

![Animated GIF of navigating to the
dashboard](https://user-images.githubusercontent.com/1153921/80825153-abaec600-8b94-11ea-8b17-1b770a2abaa9.gif)

Many features of the internal web server that serves the dashboard are [configurable](/web/server/README.md), including
the listen port, enforced TLS, and even disabling the dashboard altogether.

## Sections and menus

As mentioned in the introduction, Netdata automatically organizes all the metrics it collects from your node, and places
them into **sections** of closely related charts.

The first section on any dashboard is the **System Overview**, followed by **CPUs**, **Memory**, and so on.

These sections populate the **menu**, which is on the right-hand side of the dashboard. Instead of manually scrolling up
and down to explore the dashboard, it's generally faster to click on the relevant menu item to jump to that position on
the dashboard.

Many menu items also contain a **submenu**, with links to additional categories. For example, the **Disks** section is often separated into multiple groups based on the number of disk drives/partitions on your node, which are also known as a family.

![Animated GIF of using Netdata's menus and
submenus](https://user-images.githubusercontent.com/1153921/80832425-7c528600-8ba1-11ea-8140-d0a17a62009b.gif)

## Charts

Every **chart** in the Netdata dashboard is [fully interactive](/docs/dashboard/interact-charts.mdx). Netdata
synchronizes your interactions to help you understand exactly how a node behaved in any timeframe, whether that's
seconds or days.

A chart is an individual, interactive, always-updating graphic displaying one or more collected/calculated metrics,
which are generated by [collectors](/docs/collect/how-collectors-work.md). 

![Animated GIF of the standard Netdata dashboard being manipulated and synchronizing
charts](https://user-images.githubusercontent.com/1153921/80839230-b034a800-8baf-11ea-9cb2-99c1e10f0f85.gif)

Hover over any chart to temporarily pause it and see the exact metrics values presented as different dimensions. Click
or tap to stop the chart from automatically updating with new metrics, thereby locking it to a single timeframe.
Double-click it to resume auto-updating.

Let's cover two of the most important ways to interact with charts: panning through time and zooming.

To pan through time, **click and hold** (or touch and hold) on any chart, then **drag your mouse** (or finger) to the
left or right. Drag to the right to pan backward through time, or drag to the left to pan forward in time. Think of it
like pushing the current timeframe off the screen to see what came before or after.

To zoom, press and hold `Shift`, then use your mouse's scroll wheel, or a two-finger pinch if you're using a touchpad.

See [interact with charts](/docs/dashboard/interact-charts.mdx) for all the possible ways to interact with the charts on
your dashboard.

## Custom Dashboards

You can:

-   create your own dashboards using simple HTML (no javascript is required for
    basic dashboards)
-   utilize any or all of the available chart libraries, on the same dashboard
-   use data from one or more Netdata servers, on the same dashboard
-   host your dashboard HTML page on any web server, anywhere

You can also add Netdata charts to existing web pages.

Check this **[very simple working example of a custom dashboard](http://netdata.firehol.org/demo.html)**.

You should also look at the [custom dashboard
template](https://my-netdata.io/dashboard.html), which contains samples of all
supported charts. The code is [here](http://netdata.firehol.org/dashboard.html).

If you plan to put the dashboard on TV, check out
[tv.html](http://netdata.firehol.org/tv.html). Here's is a screenshot of it,
monitoring two servers on the same page:

![image](https://cloud.githubusercontent.com/assets/2662304/14252187/d8d5f78e-fa8e-11e5-990d-99821d38c874.png)

--

### Web directory

All of the mentioned examples are available on your local Netdata installation
(e.g. `http://myhost:19999/dashboard.html`). The default web root directory with
the HTML and JS code is `/usr/share/netdata/web`. The main dashboard is also in
that directory and called `index.html`.\
Note: index.html has a different syntax. Don't use it as a template for simple
custom dashboards.

> Some operating systems will use `/opt/netdata/usr/share/netdata/web` as the web directory. If you're not sure where
> yours is, navigate to `http://NODE:19999/netdata.conf` in your browser, replacing `NODE` with the IP address or hostname
> of your node, and find the `# web files directory = ` setting. The value listed is the web directory for your system.

### Example empty dashboard

If you need to create a new dashboard on an empty page, we suggest the following
header:

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <title>Your dashboard</title>

  <meta http-equiv="Content-Type" content="text/html; charset=utf-8" />
  <meta charset="utf-8">
  <meta http-equiv="X-UA-Compatible" content="IE=edge,chrome=1">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="apple-mobile-web-app-capable" content="yes">
  <meta name="apple-mobile-web-app-status-bar-style" content="black-translucent">

  <!-- here we will add dashboard.js -->

</head>
<body>

<!-- here we will add charts -->

</body>
</html>
```

---

### dashboard.js

To add Netdata charts to any web page (dedicated to Netdata or not), you need to
include the `/dashboard.js` file of a Netdata server.

For example, if your Netdata server listens at `http://box:19999/`, you will
need to add the following to the `head` section of your web page:

```html
<script type="text/javascript" src="http://box:19999/dashboard.js"></script>
```

#### What does dashboard.js do?

`dashboard.js` will automatically load the following:

1.  `dashboard.css`, required for the Netdata charts

2.  `jquery.min.js`, (only if jQuery is not already loaded for this web page)

3.  `bootstrap.min.js` (only if Bootstrap is not already loaded) and
    `bootstrap.min.css`.

 You can disable this by adding the following before loading `dashboard.js`:

```html
<script>var netdataNoBootstrap = true;</script>
```

4.  `jquery.nanoscroller.min.js`, required for the scrollbar of the chart
    legends.

5.  `bootstrap-toggle.min.js` and `bootstrap-toggle.min.css`, required for the
    settings toggle buttons.

6.  `font-awesome.min.css`, for icons.

When `dashboard.js` loads will scan the page for elements that define charts
(see below) and immediately start refreshing them. Keep in mind more javascript
modules may be loaded (every chart library is a different javascript file, that
is loaded on first use).

#### Prevent dashboard.js from starting chart refreshes

If your web page is not static and you plan to add charts using JavaScript, you
can tell `dashboard.js` not to start processing charts immediately after loaded,
by adding this fragment before loading it:

```html
<script>var netdataDontStart = true;</script>
```

The above, will inform the `dashboard.js` to load everything, but not process the web page until you tell it to.
You can tell it to start processing the page, by running this javascript code:

```js
NETDATA.start();
```

Be careful not to call the `NETDATA.start()` multiple times. Each call to this
function will spawn a new thread that will start refreshing the charts.

If, after calling `NETDATA.start()` you need to update the page (or even get
your javascript code synchronized with `dashboard.js`), you can call (after you
loaded `dashboard.js`):

```js
NETDATA.pause(function() {
 // ok, it is paused

 // update the DOM as you wish

 // and then call this to let the charts refresh:
 NETDATA.unpause();
});
```

### The default Netdata server

`dashboard.js` will attempt to auto-detect the URL of the Netdata server it is
loaded from, and set this server as the default Netdata server for all charts.

If you need to set any other URL as the default Netdata server for all charts
that do not specify a Netdata server, add this before loading `dashboard.js`:

```html
<script type="text/javascript">var netdataServer = "http://your.netdata.server:19999";</script>
```

---

### Adding charts

To add charts, you need to add a `div` for each of them. Each of these `div`
elements accept a few `data-` attributes:

#### The chart unique ID

The unique ID of a chart is shown at the title of the chart of the default
Netdata dashboard. You can also find all the charts available at your Netdata
server with this URL: `http://your.netdata.server:19999/api/v1/charts`
([example](http://netdata.firehol.org/api/v1/charts)).

To specify the unique id, use this:

```html
<div data-netdata="unique.id"></div>
```

The above is enough for adding a chart. It most probably have the wrong visual
settings though. Keep reading...

#### The duration of the chart

You can specify the duration of the chart (how much time of data it will show)
using:

```html
<div data-netdata="unique.id"
 data-after="AFTER_SECONDS"
 data-before="BEFORE_SECONDS"
 ></div>
```

`AFTER_SECONDS` and `BEFORE_SECONDS` are numbers representing a time-frame in
seconds.

The can be either:

-   **absolute** unix timestamps (in javascript terms, they are `new
    Date().getTime() / 1000`. Using absolute timestamps you can have a chart
    showing always the same time-frame.

-   **relative** number of seconds to now. To show the last 10 minutes of data,
    `AFTER_SECONDS` must be `-600` (relative to now) and `BEFORE_SECONDS` must
    be `0` (meaning: now). If you want the chart to auto-refresh the current
    values, you need to specify **relative** values.

#### Chart sizes

You can set the size of the chart using this:

```html
<div data-netdata="unique.id"
 data-width="WIDTH"
 data-height="HEIGHT"
 ></div>
```

`WIDTH` and `HEIGHT` can be anything CSS accepts for width and height (e.g.
percentages, pixels, etc). Keep in mind that for certain chart libraries,
`dashboard.js` may apply an aspect ratio to these.

If you want `dashboard.js` to permanently remember (browser local storage) the
dimensions of the chart (the user may resize it), you can add: `data-id="
SETTINGS_ID"`, where `SETTINGS_ID` is anything that will be common for this
chart across user sessions.

#### Netdata server

Each chart can get data from a different Netdata server. You can specify the Netdata server to use for each chart using:

```html
<div data-netdata="unique.id"
 data-host="http://another.netdata.server:19999/"
 ></div>
```

If you have ephemeral monitoring setup ([More info here](/streaming/README.md#monitoring-ephemeral-nodes)) and have no
direct access to the nodes dashboards, you can use the following:

```html
<div data-netdata="unique.id"
 data-host="http://yournetdata.server:19999/host/reported-hostname"
 ></div>
```

#### Chart library

Netdata supports a number of chart libraries. The default chart library is
`dygraph`, but you can set a different chart library per chart using
`data-chart-library`:

```html
<div data-netdata="unique.id"
 data-chart-library="gauge"
 ></div>
```

Each chart library has a number of specific settings. To learn more about them,
you should investigate the documentation of the given chart library, or visit
the appropriate JavaScript file that defines the library's options. These files
are concatenated into the monolithic `dashboard.js` for deployment.

-   [Dygraph](https://github.com/netdata/netdata/blob/5b57fc441c40959514c4e2d0863be2e6a417e352/web/gui/dashboard.js#L2034)
-   [d3](https://github.com/netdata/netdata/blob/5b57fc441c40959514c4e2d0863be2e6a417e352/web/gui/dashboard.js#L4095)
-   [d3pie](https://github.com/netdata/netdata/blob/5b57fc441c40959514c4e2d0863be2e6a417e352/web/gui/dashboard.js#L3753)
-   [Gauge.js](https://github.com/netdata/netdata/blob/5b57fc441c40959514c4e2d0863be2e6a417e352/web/gui/dashboard.js#L3065)
-   [Google Charts](https://github.com/netdata/netdata/blob/5b57fc441c40959514c4e2d0863be2e6a417e352/web/gui/dashboard.js#L2936)
-   [EasyPieChart](https://github.com/netdata/netdata/blob/5b57fc441c40959514c4e2d0863be2e6a417e352/web/gui/dashboard.js#L3531)
-   [Peity](https://github.com/netdata/netdata/blob/5b57fc441c40959514c4e2d0863be2e6a417e352/web/gui/dashboard.js#L4137)
-   [Sparkline](https://github.com/netdata/netdata/blob/5b57fc441c40959514c4e2d0863be2e6a417e352/web/gui/dashboard.js#L2779)
-   [Text-only](https://github.com/netdata/netdata/blob/5b57fc441c40959514c4e2d0863be2e6a417e352/web/gui/dashboard.js#L4200)

#### Data points

For the time-frame requested, `dashboard.js` will use the chart dimensions and
the settings of the chart library to find out how many data points it can show.

For example, most line chart libraries are using 3 pixels per data point. If the
chart shows 10 minutes of data (600 seconds), its update frequency is 1 second,
and the chart width is 1800 pixels, then `dashboard.js` will request from the
Netdata server: 10 minutes of data, represented in 600 points, and the chart
will be refreshed per second. If the user resizes the window so that the chart
becomes 600 pixels wide, then `dashboard.js` will request the same 10 minutes of
data, represented in 200 points and the chart will be refreshed once every 3
seconds.

If you need the chart to show a fixed number of points, you can set the `data-points` option. Replace `DATA_POINTS` with the number of points you need:

```html
<div data-netdata="unique.id"
 data-points="DATA_POINTS"
 ></div>
```

You can also overwrite the pixels-per-point per chart using this:

```html
<div data-netdata="unique.id"
 data-pixels-per-point="PIXELS_PER_POINT"
 ></div>
```

Where `PIXELS_PER_POINT` is the number of pixels each data point should occupy.

#### Data grouping method

Netdata supports **average** (the default), **sum** and **max** grouping
methods. The grouping method is used when the Netdata server is requested to
return fewer points for a time-frame, compared to the number of points
available.

You can give it per chart, using:

```html
<div data-netdata="unique.id"
 data-method="max"
 ></div>
```

#### Changing rates

Netdata can change the rate of charts on the fly. So a charts that shows values
**per second** can be turned to **per minute** (or any other, e.g. **per 10
seconds**), with this:

```html
<div data-netdata="unique.id"
 data-method="average"
 data-gtime="60"
 data-units="per minute"
 ></div>
```

The above will provide the average rate per minute (60 seconds). Use 60 for
`/minute`, 3600 for `/hour`, 86400 for `/day` (provided you have that many
data).

-   The `data-gtime` setting does not change the units of the chart. You have to
    change them yourself with `data-units`.
-   This works only for `data-method="average"`.
-   Netdata may aggregate multiple points to satisfy the `data-points` setting.
    For example, you request `per minute` but the requested number of points to
    be returned are not enough to report every single minute. In this case
    Netdata will sum the `per second` raw data of the database to find the `per
    minute` for every single minute and then **average** them to find the
    **average per minute rate of every X minutes**. So, it works as if the data
    collection frequency was per minute.

#### Selecting dimensions

By default, `dashboard.js` will show all the dimensions of the chart. You can
select specific dimensions using this:

```html
<div data-netdata="unique.id"
 data-dimensions="dimension1,dimension2,dimension3,..."
 ></div>
```

Netdata supports coma (`,`) or pipe (`|`) separated [simple
patterns](/libnetdata/simple_pattern/README.md) for dimensions. By default it
searches for both dimension IDs and dimension NAMEs. You can control the target
of the match with: `data-append-options="match-ids"` or
`data-append-options="match-names"`. Spaces in `data-dimensions=""` are matched
in the dimension names and IDs.

#### Chart title

You can overwrite the title of the chart using this:

```html
<div data-netdata="unique.id"
 data-title="my super chart"
 ></div>
```

#### Chart units

You can overwrite the units of measurement of the dimensions of the chart, using
this:

```html
<div data-netdata="unique.id"
 data-units="words/second"
 ></div>
```

#### Chart colors

`dashboard.js` has an internal palette of colors for the dimensions of the
charts. You can prepend colors to it (so that your will be used first) using
this:

```html
<div data-netdata="unique.id"
 data-colors="#AABBCC #DDEEFF ..."
 ></div>
```

#### Extracting dimension values

`dashboard.js` can update the selected values of the chart at elements you
specify. For example, let's assume we have a chart that measures the bandwidth
of eth0, with 2 dimensions `in` and `out`. You can use this:

```html
<div data-netdata="net.eth0"
 data-show-value-of-in-at="eth0_in_value"
 data-show-value-of-out-at="eth0_out_value"
 ></div>

My eth0 interface, is receiving <span id="eth0_in_value"></span>
and transmitting <span id="eth0_out_value"></span>.
```

#### Hiding the legend of a chart

On charts that by default have a legend managed by `dashboard.js` you can remove
it, using this:

```html
<div data-netdata="unique.id"
 data-legend="no"
 ></div>
```

#### API options

You can append Netdata **[REST API v1](/web/api/README.md)** data options, using this:

```html
<div data-netdata="unique.id"
 data-append-options="absolute,percentage"
 ></div>
```

A few useful options are:

-   `absolute` to show all values are absolute (i.e. turn negative dimensions to
    positive)
-   `percentage` to express the values as a percentage of the chart total (so,
    the values of the dimensions are added, and the sum of them if expressed as
    a percentage of the sum of all dimensions)
-   `unaligned` to prevent Netdata from aligning the charts (e.g. when
    requesting 60 seconds aggregation per point, Netdata returns chart data
    aligned to XX:XX:00 to XX:XX:59 - similarly for hours, days, etc - the
    `unaligned` option disables this feature)
-   `match-ids` or `match-names` is used to control what `data-dimensions=` will
    match.

#### Chart library performance

`dashboard.js` measures the performance of the chart library when it renders the
charts. You can specify an element ID you want this information to be
visualized, using this:

```html
<div data-netdata="unique.id"
 data-dt-element-name="measurement1"
 ></div>

refreshed in <span id="measurement1"></span> milliseconds!
```

#### Syncing charts y-range

If you give the same `data-common-max="NAME"` to 2+ charts, then all of them
will share the same max value of their y-range. If one spikes, all of them will
be aligned to have the same scale. This is done for the cpu interrupts and and
cpu softnet charts at the dashboard and also for the `gauge` and `easypiecharts`
of the Netdata home page.

```html
<div data-netdata="chart1"
 data-common-max="chart-group-1"
 ></div>

<div data-netdata="chart2"
 data-common-max="chart-group-1"
 ></div>
```

The same functionality exists for `data-common-min`.

#### Syncing chart units

Netdata dashboards support auto-scaling of units. So, `MB` can become `KB`,
`GB`, etc dynamically, based on the value to be shown.

Giving the same `NAME` with `data-common-units= "NAME"`, 2+ charts can be forced
to always have the same units.

```html
<div data-netdata="chart1"
 data-common-units="chart-group-1"
 ></div>

<div data-netdata="chart2"
 data-common-units="chart-group-1"
 ></div>
```

### Setting desired units

Charts can be scaled to specific units with `data-desired-units=" UNITS"`. If
the dashboard can convert the units to the desired one, it will do.

```html
<div data-netdata="chart1"
 data-desired-units="GB"
 ></div>
```

## Chart library settings

### Dygraph

You can set the min and max values of the y-axis using
`data-dygraph-valuerange=" [MIN, MAX] "`.

### EasyPieChart

#### Value range

You can set the max value of the chart using the following snippet:

```html
<div data-netdata="unique.id"
 data-chart-library="easypiechart"
 data-easypiechart-max-value="40"
 ></div>
```

Be aware that values that exceed the max value will get expanded (e.g. "41" is
still 100%). Similar for the minimum:

```html
<div data-netdata="unique.id"
 data-chart-library="easypiechart"
 data-easypiechart-min-value="20"
 ></div>
```

If you specify both minimum and maximum, the rendering behavior changes. Instead
of displaying the `value` based from zero, it is now based on the range that is
provided by the snippet:

```html
<div data-netdata="unique.id"
 data-chart-library="easypiechart"
 data-easypiechart-min-value="20"
 data-easypiechart-max-value="40"
 ></div>
```

In the first example, a value of `30`, without specifying the minimum, fills the chart bar to '75 %` (100% / 40 * 30). However, in this example the range is now `20` (40 - 20 = 20). The value `30` will fill the chart to ** '50 %`**, since it's in the middle between 20 and 40.

This scenario is useful if you have metrics that change only within a specific range, e.g. temperatures that are very unlikely to fall out of range. In these cases it is more useful to have the chart render the values between the given min and max, to better highlight the changes within them.

#### Negative values

EasyPieCharts can render negative values with the following flag:
```html
<div data-netdata="unique.id"
 data-chart-library="easypiechart"
 data-override-options="signed"
 ></div>
```
Negative values are rendered counter-clockwise.

#### Full example with EasyPieChart

This is a chart that displays the hotwater temperature in the given range of 40
to 50.
```html
<div data-netdata="acme_system.hotwater.hotwatertemp"
 data-title="Hot Water Temperature"
 data-decimal-digits="1"
 data-chart-library="easypiechart"
 data-colors="#FE3912"
 data-width="55%"
 data-height="50%"
 data-points="1200"
 data-after="-1200"
 data-dimensions="actual"
 data-units="°C"
 data-easypiechart-max-value="50"
 data-easypiechart-min-value="40"
 data-common-max="netdata-hotwater-max"
 data-common-min="netdata-hotwater-min"
></div>
```

![hot water
chart](https://user-images.githubusercontent.com/12159026/28666665-a7d68ad2-72c8-11e7-9a96-f6bf9691b471.png)



## Related Topics

### **Related Concepts**
- [Spaces](https://github.com/netdata/learn/blob/master/docs/concepts/netdata-cloud/spaces.md)
- [Charts](https://github.com/netdata/learn/blob/master/docs/concepts/netdata-cloud/spaces.md)
- [Netdata Views](https://github.com/netdata/learn/blob/master/docs/concepts/netdata-cloud/netdata-views.md)
- [Rooms](https://github.com/netdata/learn/blob/rework-learn/docs/concepts/netdata-cloud/rooms.md)
- [From raw metrics to visualizations](https://github.com/netdata/learn/blob/rework-learn/docs/concepts/visualizations/from-raw-metrics-to-visualization.md)

### Related Tasks
- [Claiming an existing agent to Cloud](https://github.com/netdata/netdata/blob/rework-learn/docs/tasks/setup/claim-existing-agent-to-cloud.md)
- [Interact with charts](https://github.com/netdata/learn/blob/rework-learn/docs/tasks/interact-with-the-charts.md)
