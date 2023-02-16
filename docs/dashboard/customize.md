<!--
title: "Customize the standard dashboard"
description: >-
    "Netdata's preconfigured dashboard offers many customization options, such as choosing when 
    charts are updated, your preferred theme, and custom text to document processes, and more."
type: "how-to"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/dashboard/customize.md"
sidebar_label: "Customize the standard dashboard"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Operations"
-->

# Customize the standard dashboard

While the [Netdata dashboard](https://github.com/netdata/netdata/blob/master/docs/dashboard/how-dashboard-works.md) comes preconfigured with hundreds of charts and
thousands of metrics, you may want to alter your experience based on a particular use case or preferences.

## Dashboard settings

To change dashboard settings, click the on the **settings** icon ![Import
icon](https://raw.githubusercontent.com/netdata/netdata-ui/98e31799c1ec0983f433537ff16d2ac2b0d994aa/src/components/icon/assets/gear.svg)
in the top panel.

These settings only affect how the dashboard behaves in your browser. They take effect immediately and are permanently
saved to browser local storage (except the refresh on focus / always option). Some settings are applied immediately, and
others are only reflected after the dashboard is refreshed, which happens automatically.

Here are a few popular settings:

### Change chart legend position

Find this setting under the **Visual** tab. By default, Netdata places the [legend of dimensions](https://github.com/netdata/netdata/blob/master/docs/dashboard/dimensions-contexts-families.md#dimension) _below_ charts. 
Click this toggle to move the legend to the _right_ of charts.


### Change theme

Find this setting under the **Visual** tab. Choose between Dark (the default) and White.

## Customize the standard dashboard info

Netdata stores information about individual charts in the `dashboard_info.js` file. This file includes section and
subsection headings, descriptions, colors, titles, tooltips, and other information for Netdata to render on the
dashboard.

One common use case for customizing the standard dashboard is adding internal "documentation" a section or specific
chart that can then be read by anyone with access to that dashboard.

For example, here is how `dashboard_info.js` defines the **System Overview** section.

```javascript
netdataDashboard.menu = {
  'system': {
    title: 'System Overview',
    icon: '<i class="fas fa-bookmark"></i>',
    info: 'Overview of the key system metrics.'
  },
```

If you want to customize this information, use the example `dashboard_info_custom_example.js` as a starting point.
First, navigate to the web server's directory. If you're on a Linux system, this should be at `/usr/share/netdata/web/`.
Copy the example file, then ensure that its permissions match the rest of the web server, which is `netdata:netdata` by
default.

```bash
cd /usr/share/netdata/web/
sudo cp dashboard_info_custom_example.js your_dashboard_info_file.js
sudo chown netdata:netdata your_dashboard_info_file.js
```

Edit the file with customizations to the `title`, `icon`, and `info` fields. Replace the string after `fas fa-` with any
icon from [Font Awesome](https://fontawesome.com/cheatsheet) to customize the icons that appear throughout the
dashboard.

Save the file, then navigate to your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md) to edit `netdata.conf`. Add
the following line to the `[web]` section to tell Netdata where to find your custom configuration.

```conf
[web]
    custom dashboard_info.js = your_dashboard_info_file.js
```

Reload your browser tab to see your custom configuration.

## What's next?

If you're keen on continuing to customize your Netdata experience, check out our docs on [building new custom
dashboards](https://github.com/netdata/netdata/blob/master/web/gui/custom/README.md) with HTML, CSS, and JavaScript.

### Further reading & related information

- Dashboard
  - [How the dashboard works](https://github.com/netdata/netdata/blob/master/docs/dashboard/how-dashboard-works.md)
  - [Interact with charts](https://github.com/netdata/netdata/blob/master/docs/dashboard/interact-charts.md)
  - [Chart dimensions, contexts, and families](https://github.com/netdata/netdata/blob/master/docs/dashboard/dimensions-contexts-families.md)
  - [Select timeframes to visualize](https://github.com/netdata/netdata/blob/master/docs/dashboard/visualization-date-and-time-controls.md)
  - [Import, export, and print a snapshot](https://github.com/netdata/netdata/blob/master/docs/dashboard/import-export-print-snapshot.md)
  - **[Customize the standard dashboard](https://github.com/netdata/netdata/blob/master/docs/dashboard/customize.md)**
