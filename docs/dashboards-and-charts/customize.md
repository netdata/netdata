# Customize the standard dashboard

> ### Disclaimer
>
> This document is only applicable to the v1 version of the dashboard and doesn't affect the [Netdata Dashboard](https://github.com/netdata/netdata/blob/master/docs/dashboards-and-charts/README.md).

While the [Netdata dashboard](https://github.com/netdata/netdata/blob/master/src/web/gui/README.md) comes preconfigured with hundreds of charts and
thousands of metrics, you may want to alter your experience based on a particular use case or preferences.

## Dashboard settings

To change dashboard settings, click the on the **settings** icon 
![Import icon](https://raw.githubusercontent.com/netdata/netdata-ui/98e31799c1ec0983f433537ff16d2ac2b0d994aa/src/components/icon/assets/gear.svg)
in the top panel.

These settings only affect how the dashboard behaves in your browser. They take effect immediately and are permanently
saved to browser local storage (except the refresh on focus / always option). Some settings are applied immediately, and
others are only reflected after the dashboard is refreshed, which happens automatically.

Here are a few popular settings:

### Change chart legend position

Find this setting under the **Visual** tab. By default, Netdata places the 
[legend of dimensions](https://github.com/netdata/netdata/blob/master/docs/dashboards-and-charts/dimensions-contexts-families.md#dimension) _below_ charts. 
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

Save the file, then navigate to your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/netdata-agent/configuration/README.md) to edit `netdata.conf`. Add
the following line to the `[web]` section to tell Netdata where to find your custom configuration.

```conf
[web]
    custom dashboard_info.js = your_dashboard_info_file.js
```

Reload your browser tab to see your custom configuration.