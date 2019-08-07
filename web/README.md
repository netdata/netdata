# Web dashboards overview

Because Netdata is a health monitoring and *performance troubleshooting* system, we put a lot of emphasis on real-time, meaningful, and context-aware charts.

We then leverage a number of dashboards, both configured by the Netdata community and customizeable by you, so you can easily see anomalies, jump between charts to find correlations, and generally make smarter decisions about your systems and applications.

There are two primary ways to view Netdata's dashboards:

1. The [Netdata web dashboard](#default-web-dashboard) that comes pre-configured with every Netdata installation and is accessed at `http://SERVER-IP:19999`, or `http://localhost:19999` on `localhost`.

2. The [`dashboard.js` JavaScript library](#dashboard-js) that helps you create custom dashboards using plain HTML.

You can also view all the data Netdata collects through the [REST API v1](api/).

## Netdata web dashboard



The default port is 19999; for example, to access the dashboard on localhost, use: http://localhost:19999

To view Netdata collected data you access its **[REST API v1](api/)**.

For our convenience, Netdata provides 2 more layers:

1.  The `dashboard.js` javascript library that allows us to design custom dashboards using plain HTML. For information on creating custom dashboards, see **[Custom Dashboards](gui/custom/)** and **[Atlassian Confluence Dashboards](gui/confluence/)**

2.  Ready to be used web dashboards that render all the charts a Netdata server maintains.


## Charts, contexts, families

Before configuring an alarm or writing a collector, it's important to understand how Netdata organizes collected metrics into charts. 

### Charts

Each chart that you see on the Netdata dashboard contains one or more dimensions, one for each collected or calculated metric. 

The chart name or chart id is what you see in parentheses at the top left corner of the chart you are interested in. For example, if you go to the system cpu chart: `http://your.netdata.ip:19999/#menu_system_submenu_cpu`, you will see at the top left of the chart the label "Total CPU utilization (system.cpu)". In this case, the chart name is `system.cpu`.  

### Dimensions

Most charts depict more than one dimensions. The dimensions of a chart are called "series" in some applications. You can see these dimensions on the right side of a chart, right under the date and time. For the system.cpu example we used, you will see the dimensions softirq, irq, user etc. Note that these are not always simple metrics (raw data). They could be calculated values (percentages, aggregates and more).

### Families

When you have several instances of a monitored hardware or software resource (e.g. network interfaces, mysql instances etc.), you need to be able to identify each one separately. Netdata uses "families" to identify such instances. For example, if I have the network interfaces `eth0` and `eth1`, `eth0` will be one family, and `eth1` will be another. 

The reasoning behind calling these instances "families" is that different charts for the same instance can and many times are related (relatives, family, you get it). The family of a chart is usually the name of the Netdata dashboard submenu that you see selected on the right navigation pane, when you are looking at a chart. For the example of the two network interfaces, you would see a submenu `eth0` and a submenu `eth1` under the "Network Interfaces" menu on the right navigation pane. 

### Contexts

A context is a grouping of identical charts, for each instance of the hardware or software monitored. For example, `health/health.d/net.conf` refers to four contexts: `net.drops`, `net.fifo`, `net.net`, `net.packets`. You can see the context of a chart if you hover over the date right above the dimensions of the chart.  The line that appears shows you two things: the collector that produces the chart and the chart context. 

For example, let's take the `net.packets` context. You will see on the dashboard as many charts with context net.packets as you have network interfaces (families). These charts will be named `net_packets.[family]`. For the example of the two interfaces `eth0` and `eth1`, you will see charts named `net_packets.eth0` and `net_packets.eth1`. Both of these charts show the exact same dimensions, but for different instances of a network interface.


## Customizing the standard dashboards

Charts information is stored at /usr/share/netdata/web/[dashboard_info.js](gui/dashboard_info.js). This file includes information that is rendered on the dashboard, controls chart colors, section and subsection heading, titles, etc.

If you change that file, your changes will be overwritten when Netdata is updated. You can preserve your settings by creating a new such file (there is /usr/share/netdata/web/[dashboard_info_custom_example.js](gui/dashboard_info_custom_example.js) you can use to start with).

You have to copy the example file under a new name, so that it will not be overwritten with Netdata updates.

To configure your info file set in `netdata.conf`:

```
[web]
   custom dashboard_info.js = your_file_name.js
```


For information on creating custom dashboards, see **[Custom Dashboards](gui/custom/)** and **[Atlassian Confluence Dashboards](gui/confluence/)**

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fweb%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
