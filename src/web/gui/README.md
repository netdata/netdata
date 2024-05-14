# Legacy Agent dashboard

> ⚠️ You're checking the documentation for the legacy Agent dashboard. For the current version please check [Accessing Netdata Dashboards](https://github.com/netdata/netdata/blob/master/docs/dashboards-and-charts/README.md).


The local Netdata Agent dashboard is the heart of Netdata's performance troubleshooting toolkit. You've probably seen it
before:

![The Netdata dashboard in
action](https://user-images.githubusercontent.com/1153921/101513938-fae28380-3939-11eb-9434-8ad86a39be62.gif)

Learn more about how dashboards work and how they're populated using the `dashboards.js` file in our [web dashboards
overview](https://github.com/netdata/netdata/blob/master/src/web/README.md).

By default, Netdata starts a web server for its dashboard at port `19999`. Open up your web browser of choice and
navigate to `http://NODE:19999`, replacing `NODE` with the IP address or hostname of your Agent. If installed on localhost,
you can access it through `http://localhost:19999`.

Netdata uses an [internal, static-threaded web server](https://github.com/netdata/netdata/blob/master/src/web/server/README.md) to host the HTML, CSS, and JavaScript
files that make up the local Agent dashboard. You don't have to configure anything to access it, although you can adjust
[your settings](https://github.com/netdata/netdata/blob/master/src/web/server/README.md#other-netdataconf-web-section-options) in the `netdata.conf` file, or run Netdata
behind an [Nginx proxy](https://github.com/netdata/netdata/blob/master/docs/Running-behind-nginx.md), and so on.

## Navigating the local dashboard

Beyond charts, the local dashboard can be broken down into three key areas:

- [Local Agent dashboard](#local-agent-dashboard)
  - [Navigating the local dashboard](#navigating-the-local-dashboard)
    - [Sections](#sections)
    - [Time \& date picker](#time--date-picker)
    - [Metrics menus](#metrics-menus)
    - [Cloud menus (Spaces, War Rooms, and Visited nodes)](#cloud-menus-spaces-war-rooms-and-visited-nodes)
  - [Customizing the local dashboard](#customizing-the-local-dashboard)
  - [Custom dashboards](#custom-dashboards)

![Annotated screenshot of the local Agent
dashboard](https://user-images.githubusercontent.com/1153921/101509403-f7e59400-3935-11eb-9abd-cbecfa3ee49a.png)

### Sections

Netdata is broken up into multiple **sections**, such as **System Overview**,
**CPU**, **Disk**, and more. Inside each section you'll find a number of charts,
broken down into [contexts](https://github.com/netdata/netdata/blob/master/src/web/README.md#contexts) and
[families](https://github.com/netdata/netdata/blob/master/src/web/README.md#families).

An example of the **Memory** section on a Linux desktop system.

![Screenshot of the Memory section of the Netdata
dashboard](https://user-images.githubusercontent.com/1153921/101508423-e354cc00-3934-11eb-9b33-3ad57a5988b4.png)

All sections and their associated charts appear on a single page, so all you need to do to view different sections is
scroll up and down. But it's usually quicker to use the [menus](#metrics-menus).

### Time & date picker

The local dashboard features a time & date picker to help you visualize specific timeframes of historical metrics. The
picker chooses an appropriate default to always show per-second granularity based on the width of your browser's
viewport.

![The time & date picker in the local Agent
dashboard](https://user-images.githubusercontent.com/1153921/101507784-2c585080-3934-11eb-9d6e-eff30b8553e4.png)

Use the Quick Selector to show metrics from the last 5 minutes, 15 minutes, 30 minutes, 2 hours, 6 hours, or 12 hours.

Beneath the Quick Selector is an input field and dropdown you use in combination to select a specific timeframe of
minutes, hours, days, or months. Enter a number and choose the appropriate unit of time.

Use the calendar to select multiple days. Click on a date to begin the timeframe selection, then an ending date. 

Click **Apply** to re-render all visualizations with new metrics data, or **Clear** to restore the default timeframe.

[Increase the metrics retention policy](https://github.com/netdata/netdata/blob/master/docs/store/change-metrics-storage.md) for your node to see more historical
timeframes.

### Metrics menus

**Metrics menus** appears on the right-hand side of the local Agent dashboard. Netdata generates a menu for each
section, and menus link to the section they're associated with.

![A screenshot of metrics menus](https://user-images.githubusercontent.com/1153921/80834638-f08f2880-8ba5-11ea-99ae-f610b2885fd6.png)

Most metrics menu items will contain several **submenu** entries, which represent any
[families](https://github.com/netdata/netdata/blob/master/src/web/README.md#families) from that section. Netdata automatically
generates these submenu entries.

Here's a **Disks** menu with several submenu entries for each disk drive and
partition Netdata recognizes.

![Screenshot of some metrics
submenus](https://user-images.githubusercontent.com/1153921/80834697-11577e00-8ba6-11ea-979c-92fd19cdb480.png)

### Cloud menus (Spaces, War Rooms, and Visited nodes)

The dashboard also features a menu related to Netdata Cloud functionality. You can view your existing Spaces or create
new ones via the left vertical column of boxes. This menu also displays the name of your current Space, shows a list of
any War Rooms you've added you your Space, and lists any notes you recently visited via their Agent dashboards. Click on
a War Room's name to jump to the Netdata Cloud web interface.

![A screenshot of the Cloud
menus](https://user-images.githubusercontent.com/1153921/80837210-3f8b8c80-8bab-11ea-9c75-128c2d823ef8.png)

## Customizing the local dashboard

Netdata stores information about individual charts in the `dashboard_info.js`
file. This file includes section and subsection headings, descriptions, colors,
titles, tooltips, and other information for Netdata to render on the dashboard.

For example, here is how `dashboard_info.js` defines the **System Overview**
section.

```javascript
netdataDashboard.menu = {
  'system': {
    title: 'System Overview',
    icon: '<i class="fas fa-bookmark"></i>',
    info: 'Overview of the key system metrics.'
  },
```

If you want to customize this information, you should avoid editing
`dashboard_info.js` directly. These changes are not persistent; Netdata will
overwrite the file when it's updated. Instead, you should create a new file with
your customizations.

We created an example file at `dashboard_info_custom_example.js`. You can
copy this to a new file with a name of your choice in the `web/` directory. This
directory changes based on your operating system and installation method. If
you're on a Linux system, it should be at `/usr/share/netdata/web/`.

```shell
cd /usr/share/netdata/web/
sudo cp dashboard_info_custom_example.js your_dashboard_info_file.js
```

Edit the file with your customizations. For example:

```javascript
customDashboard.menu = {
  system: {
    title: "Testing, testing, 1 2 3",
    icon: '<i class="fas fa-thumbs-up"></i>',
    info: "This is overwritten info for the system overview section!"
  }
};
```

Finally, tell Netdata where you placed your customization file by replacing
`your_dashboard_info_file.js` below.

```conf
[web]
 custom dashboard_info.js = your_dashboard_info_file.js
```

Once you restart Netdata, refresh the dashboard to find your custom
configuration:

![Screenshot of overwritten text from dashboard_info.js
file](https://user-images.githubusercontent.com/1153921/62798924-570e6c80-ba94-11e9-9578-869753bec39c.png)

## Custom dashboards

For information on creating custom dashboards from scratch, see the [custom dashboards](https://github.com/netdata/netdata/blob/master/src/web/gui/custom/README.md) or
[Atlassian Confluence dashboards](https://github.com/netdata/netdata/blob/master/src/web/gui/confluence/README.md) guides.
