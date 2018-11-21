# Web Dashboards Overview

The default port is 19999; for example, to access the dashboard on localhost, use: http://localhost:19999

To view netdata collected data you access its **[REST API v1](api/)**.

For our convenience, netdata provides 2 more layers:

1. The `dashboard.js` javascript library that allows us to design custom dashboards using plain HTML. For information on creating custom dashboards, see **[Custom Dashboards](gui/custom/)** and **[Atlassian Confluence Dashboards](gui/confluence/)**

2. Ready to be used web dashboards that render all the charts a netdata server maintains.

## customizing the standard dashboards

Charts information is stored at /usr/share/netdata/web/[dashboard_info.js](gui/dashboard_info.js). This file includes information that is rendered on the dashboard, controls chart colors, section and subsection heading, titles, etc.

If you change that file, your changes will be overwritten when netdata is updated. You can preserve your settings by creating a new such file (there is /usr/share/netdata/web/[dashboard_info_custom.example.js](gui/dashboard_info_custom_example.js) you can use to start with).

You have to copy the example file under a new name, so that it will not be overwritten with netdata updates.

To configure your info file set in netdata.conf:

```
[web]
   custom dashboard_info.js = your_file_name.js
```
