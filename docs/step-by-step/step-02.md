<!--
---
title: "Step 2. Get to know Netdata's dashboard"
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/step-by-step/step-02.md
---
-->

# Step 2. Get to know Netdata's dashboard

Welcome to Netdata proper! Now that you understand how Netdata works, how it's built, and why we built it, you can start
working with the dashboard directly.

This step-by-step guide assumes you've already installed Netdata on a system of yours. If you haven't yet, hop back over
to ["step 0"](step-00.md#before-we-get-started) for information about our one-line installer script. Or, view the
[installation docs](../../packaging/installer) to learn more. Once you have Netdata installed, you can hop back over
here and dig in.

## What you'll learn in this step

In this step of the Netdata guide, you'll learn how to:

-   [Visit and explore the dashboard](#visit-and-explore-the-dashboard)
-   [Explore available charts using menus](#explore-available-charts-using-menus)
-   [Read the descriptions accompanying charts](#read-the-descriptions-accompanying-charts)
-   [Interact with charts](#interact-with-charts)
-   [See raised alarms and the alarm log](#see-raised-alarms-and-the-alarm-log)

Let's get started!

## Visit and explore the dashboard

Netdata's dashboard is where you interact with your system's metrics. Time to open it up and start exploring. Open up
your browser of choice.

If you installed Netdata on the same system you're using to open your browser, navigate to `http://localhost:19999/`. 

If you installed Netdata on a remote system, navigate to `http://HOST:19999/` after replacing `HOST` with the IP address
of that system. To connect to a virtual private server (VPS), for example, you might navigate to
`http://203.0.113.0:19999`. We'll learn more on monitoring remote systems and [multiple systems](step-03.md)
later on.

> From here on out in this tutorial, we'll refer to the address you use to view your dashboard as `HOST`. Be sure to 
> replace it with either `localhost` or the IP address as needed.

Hit `Enter`. Welcome to Netdata!

![Animated GIF of navigating to the
dashboard](https://user-images.githubusercontent.com/1153921/63463901-fcb9c800-c412-11e9-8f67-8fe182e8b0d2.gif)

## Explore available charts using menus

**Menus** are located on the right-hand side of the Netdata dashboard. You can use these to navigate to the
charts you're interested in.

![Animated GIF of using the menus and
submenus](https://user-images.githubusercontent.com/1153921/63464031-3ee30980-c413-11e9-886a-44594f60e0a9.gif)

Netdata shows all its charts on a single page, so you can also scroll up and down using the mouse wheel, your
touchscreen/touchpad, or the scrollbar.

Both menus and the items displayed beneath them, called **submenus**, are populated automatically by Netdata based on
what it's collecting. If you run Netdata on many different systems using different OS types or versions, the
menus and submenus may look a little different for each one.

To learn more about menus, see our documentation about [navigating the standard
dashboard](../../web/gui/README.md#menus).

> ❗ By default, Netdata only creates and displays charts if the metrics are _not zero_. So, you may be missing some
> charts, menus, and submenus if those charts have zero metrics. You can change this by changing the **Which dimensions
> to show?** setting to **All**. In addition, if you start Netdata and immediately load the dashboard, not all
> charts/menus/submenus may be displayed, as some collectors can take a while to initialize.

## Read the descriptions accompanying charts

Many charts come with a short description of what dimensions the chart is displaying and why they matter.

For example, here's the description that accompanies the **swap** chart.

![Screenshot of the swap
description](https://user-images.githubusercontent.com/1153921/63452078-477b1600-c3fa-11e9-836b-2fc90fba8b4b.png)

If you're new to health monitoring and performance troubleshooting, we recommend you spend some time reading these
descriptions and learning more at the pages linked above.

## Understand charts, dimensions, families, and contexts

A **chart** is an interactive visualization of one or more collected/calculated metrics. You can see the name (also
known as its unique ID) of a chart by looking at the top-left corner of a chart and finding the parenthesized text. On a
Linux system, one of the first charts on the dashboard will be the system CPU chart, with the name `system.cpu`:

![Screenshot of the system CPU chart in the Netdata
dashboard](https://user-images.githubusercontent.com/1153921/67443082-43b16e80-f5b8-11e9-8d33-d6ee052c6678.png)

A **dimension** is any value that gets shown on a chart. The value can be raw data or calculated values, such as
percentages, aggregates, and more. Most charts will have more than one dimension, in which case it will display each in
a different color. Here, a `system.cpu` chart is showing many dimensions, such as `user`, `system`, `softirq`, `irq`,
and more.

![Screenshot of the dimensions shown in the system CPU chart in the Netdata
dashboard](https://user-images.githubusercontent.com/1153921/62721031-2bba4d80-b9c0-11e9-9dca-32403617ce72.png)

A **family** is _one_ instance of a monitored hardware or software resource that needs to be monitored and displayed
separately from similar instances. For example, if your system has multiple partitions, Netdata will create different
families for `/`, `/boot`, `/home`, and so on. Same goes for entire disks, network devices, and more.

![A number of families created for disk partitions](https://user-images.githubusercontent.com/1153921/67896952-a788e980-fb1a-11e9-880b-2dfb3945c8d6.png)

A **context** groups several charts based on the types of metrics being collected and displayed. For example, the
**Disk** section often has many contexts: `disk.io`, `disk.ops`, `disk.backlog`, `disk.util`, and so on. Netdata uses
this context to create individual charts and then groups them by family. You can always see the context of any chart by
looking at its name or hovering over the chart's date.

It's important to understand these differences, as Netdata uses charts, dimensions, families, and contexts to create
health alarms and configure collectors. To read even more about the differences between all these elements of the
dashboard, and how they affect other parts of Netdata, read our [dashboards
documentation](../../web/README.md#charts-contexts-families).

## Interact with charts

We built Netdata to be a big sandbox for learning more about your systems and applications. Time to play!

Netdata's charts are fully interactive. You can pan through historical metrics, zoom in and out, select specific
timeframes for further analysis, resize charts, and more.

Best of all, Whenever you use a chart in this way, Netdata synchronizes all the other charts to match it. This even
applies across different Netdata agents if you connect them using the [**My nodes** menu](../../registry/README.md)!

![Aniamted GIF of chart
synchronziation](https://user-images.githubusercontent.com/1153921/63464271-c03a9c00-c413-11e9-971d-245238926193.gif)

### Pan, zoom, highlight, and reset charts

You can change how charts show their metrics in a few different ways, each of which have a few methods:

| Change                                            | Method #1                           | Method #2                                                 | Method #3                                                  |
| ------------------------------------------------- | ----------------------------------- | --------------------------------------------------------- | ---------------------------------------------------------- |
| **Reset** charts to default auto-refreshing state | `double click`                      | `double tap` (touchpad/touchscreen)                       |                                                            |
| **Select** a certain timeframe                    | `ALT` + `mouse selection`           | `⌘` + `mouse selection` (macOS)                           |                                                            |
| **Pan** forward or back in time                   | `click and drag`                    | `touch and drag` (touchpad/touchscreen)                   |                                                            |
| **Zoom** to a specific timeframe                  | `SHIFT` + `mouse selection`         |                                                           |                                                            |
| **Zoom** in/out                                   | `SHIFT`/`ALT` + `mouse scrollwheel` | `SHIFT`/`ALT` + `two-finger pinch` (touchpad/touchscreen) | `SHIFT`/`ALT` + `two-finger scroll` (touchpad/touchscreen) |

These interactions can also be triggered using the icons on the bottom-right corner of every chart. They are,
respectively, `Pan Left`, `Reset`, `Pan Right`, `Zoom In`, and `Zoom Out`.

![Animated GIF of using the icons to interact with
charts](https://user-images.githubusercontent.com/1153921/65066637-9785c380-d939-11e9-8e26-6933ce78c172.gif)

### Show and hide dimensions

Each dimension can be hidden by clicking on it. Hiding dimensions simplifies the chart and can help you better discover
exactly which aspect of your system is behaving strangely.

### Resize charts

Additionally, resize charts by clicking-and-dragging the icon on the bottom-right corner of any chart. To restore the
chart to its original height, double-click the same icon.

![Animated GIF of resizing a chart and resetting it to the default
height](https://user-images.githubusercontent.com/1153921/65066675-aec4b100-d939-11e9-9b5d-cee7316428f6.gif)

To learn more about other options and chart interactivity, read our [dashboard documentation](../../web/README.md).

## See raised alarms and the alarm log

Aside from performance troubleshooting, Netdata is designed to help you monitor the health of your systems and
applications. That's why every Netdata installation comes with dozens of pre-configured alarms that trigger alerts when
your system starts acting strangely.

Find the **Alarms** button in the top navigation bring up a modal that shows currently raised alarms, all running
alarms, and the alarms log.

Here is an example of raised `disk_space._` and `disk_space._home` alarms, followed by the full list and alarm log:

![Animated GIF of looking at raised alarms and the alarm
log](https://user-images.githubusercontent.com/1153921/63468773-85d5fc80-c41d-11e9-8ef9-51bee0f91332.gif)

Let's look at one of those raised alarms a little more in-depth. Here is a static screenshot:

![Screenshot of a raised disk_space
alarm](https://user-images.githubusercontent.com/1153921/63468853-af8f2380-c41d-11e9-9cec-1b0cac5d5549.png)

The alarm itself is named **disk - /**, and its context is `disk_space._`. Beneath that is an auto-updating badge that
shows the latest metric: 28.4% disk space usage.

With the three icons beneath that and the **role** designation, you can **1)** scroll to the chart associated with this
raised alarm, **2)** copy a link to the badge to your clipboard, and **3)** copy the code to embed the badge onto
another web page using an `<embed>` element.

The table on the right-hand side displays information about the alarm's configuration.

In this example, Netdata triggers a warning alarm when any disk on the system is more than 20% full. Netdata triggers a
critical alarm when the disk is more than 30% full.

The `calculation` field is the equation used to calculate those percentages, and the `check every` field specifies how
often Netdata should be calculating these metrics to see if the alarm should remain triggered.

The `execute` field tells Netdata how to notify you about this alarm, and the `source` field lets you know where you can
find the configuration file, if you'd like to edit its configuration.

We'll cover alarm configuration in more detail later in the tutorial, so don't worry about it too much for now! Right
now, it's most important that you understand how to see alarms, and parse their details, if and when they appear on your
system.

## What's next?

In this step of the Netdata tutorial, you learned how to:

-   Visit the dashboard
-   Explore available charts (using the right-side menu)
-   Read the descriptions accompanying charts
-   Interact with charts
-   See raised alarms and the alarm log

Next, you'll learn how to monitor multiple nodes through the dashboard.

[Next: Monitor more than one system with Netdata →](step-03.md)
