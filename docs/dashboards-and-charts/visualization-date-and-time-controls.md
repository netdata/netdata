# Visualization date and time controls

Netdata's dashboard features powerful date visualization controls that include a time control, a timezone selector and a rich date and timeframe selector.

The controls come with useful defaults and rich customization, to help you narrow your focus when troubleshooting issues or anomalies.

## Time controls

The time control provides you the following options: **Play**, **Pause** and **Force Play**.

- **Play** - the content of the page will be automatically refreshed while this is in the foreground
- **Pause** - the content of the page isn't refreshed due to a manual request to pause it or, for example, when your investigating data on a chart (cursor is on top of a chart)
- **Force Play** - the content of the page will be automatically refreshed even if this is in the background

With this, we aim to bring more clarity and allow you to distinguish if the content you are looking at is live or historical and also allow you to always refresh the content of the page when the tabs are in the background.

Main use cases for **Force Play**:

- You use a terminal or deployment tools to do changes in your infra and want to see the effect immediately, Netdata is in the background, displaying the impact of these changes
- You want to have Netdata on the background, example displayed on a TV, to constantly see metrics through dashboards or to watch the alert status

![The time control with Play, Pause and Force Play](https://user-images.githubusercontent.com/70198089/225850250-1fe12477-23f8-4b4d-b497-79b416963e10.png)

## Date and time selector

The date and time selector allows you to change the visible timeframe and change the timezone used in the interface.

### Pick timeframes to visualize

While [panning through time and zooming in/out](https://github.com/netdata/netdata/blob/master/docs/dashboards-and-charts/netdata-charts.md) from charts it is helpful when you're looking a recent history, or want to do granular troubleshooting, what if you want to see metrics from 6 hours ago? Or 6 days?

Netdata's dashboard features a **timeframe selector** to help you visualize specific timeframes in a few helpful ways.
By default, it shows a certain number of minutes of historical metrics based on the your browser's viewport to ensure it's always showing per-second granularity.

#### Open the timeframe selector

To visualize a new timeframe, you need to open the picker, which appears just above the menu, near the top-right bar of the dashboard.

![Timeframe Selector](https://user-images.githubusercontent.com/70198089/225850611-728936d9-7ca4-49fa-8d37-1ce73dd6f76c.png)

The **Clear** button resets the dashboard back to its default state based on your browser viewport, and **Apply** closes
the picker and shifts all charts to the selected timeframe.

#### Use the pre-defined timeframes

Click any of the following options in the predefined timeframe column to choose between:

- Last 5 minutes
- Last 15 minutes
- Last 30 minutes
- Last hour
- Last 2 hours
- Last 6 hours
- Last 12 hours
- Last day
- Last 2 days
- Last 7 days

Click **Apply** to see metrics from your selected timeframe.

#### Choose a specific interval

Beneath the predefined timeframe columns is an input field and dropdown you use in combination to select a specific timeframe of
minutes, hours, days, or months. Enter a number and choose the appropriate unit of time, then click **Apply**.

#### Choose multiple days via the calendar

Use the calendar to select multiple days. Click on a date to begin the timeframe selection, then an ending date. The
timeframe begins at noon on the beginning and end dates. Click **Apply** to see your selected multi-day timeframe.

#### Caveats and considerations

**Longer timeframes will decrease metrics granularity**. At the default timeframe, based on your browser viewport, each
"tick" on charts represents one second. If you select a timeframe of 6 hours, each tick represents the _average_ value
across a larger period of time.

**You can only see metrics as far back in history as your metrics retention policy allows**. Netdata uses an internal
time-series database (TSDB) to store as many metrics as it can within a specific amount of disk space. The default
storage is 256 MiB, which should be enough for 1-3 days of historical metrics. If you navigate back to a timeframe
beyond stored historical metrics, you'll see this message:

![image](https://user-images.githubusercontent.com/70198089/225851033-43b95164-a651-48f2-8915-6aac9739ed93.png)

At any time, [configure the internal TSDB's storage capacity](https://github.com/netdata/netdata/blob/master/docs/store/change-metrics-storage.md) to expand your
depth of historical metrics.

### Timezone selector

The default timezone used in all date and time fields in Netdata Cloud comes from your browser. To change it, open the
date and time selector and use the control displayed here:

![Timezone selector](https://user-images.githubusercontent.com/43294513/216628390-c3bd1cd2-349d-4523-b8d3-c7e68395f670.png)
