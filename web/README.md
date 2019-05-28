# Web dashboards overview

The default port is 19999; for example, to access the dashboard on localhost, use: http://localhost:19999

To view Netdata collected data you access its **[REST API v1](api/)**.

For our convenience, Netdata provides 2 more layers:

1. The `dashboard.js` javascript library that allows us to design custom dashboards using plain HTML. For information on creating custom dashboards, see **[Custom Dashboards](gui/custom/)** and **[Atlassian Confluence Dashboards](gui/confluence/)**

2. Ready to be used web dashboards that render all the charts a Netdata server maintains.

## Customizing the standard dashboards

Charts information is stored at /usr/share/netdata/web/[dashboard_info.js](gui/dashboard_info.js). This file includes information that is rendered on the dashboard, controls chart colors, section and subsection heading, titles, etc.

If you change that file, your changes will be overwritten when Netdata is updated. You can preserve your settings by creating a new such file (there is /usr/share/netdata/web/[dashboard_info_custom_example.js](gui/dashboard_info_custom_example.js) you can use to start with).

You have to copy the example file under a new name, so that it will not be overwritten with Netdata updates.

To configure your info file set in netdata.conf:

```
[web]
   custom dashboard_info.js = your_file_name.js
```

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fweb%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
