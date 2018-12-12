# Charts, contexts, families

Before configuring an alarm or writing a collector, it's important to understand how Netdata organizes collected metrics into charts. 

## Charts

Each chart that you see on the netdata dashboard contains one or more dimensions, one for each collected or calculated metric. 

The chart name or chart id is what you see in parentheses at the top left corner of the chart you are interested in. For example, if you go to the system cpu chart: `http://your.netdata.ip:19999/#menu_system_submenu_cpu`, you will see at the top left of the chart the label "Total CPU utilization (system.cpu)". In this case, the chart name is `system.cpu`.  

## Dimensions

Most charts depict more than one dimensions. The dimensions of a chart are called "series" in some applications. You can see these dimensions on the right side of a chart, right under the date and time. For the system.cpu example we used, you will see the dimensions softirq, irq, user etc. Note that these are not always simple metrics (raw data). They could be calculated values (percentages, aggregates and more).

## Families

When you have several instances of a monitored hardware or software (e.g. network interfaces, mysql instances etc.), you need to be able to identify each one separately. Netdata uses "families" to identify such instances. For example, if I have the network interfaces `eth0` and `eth1`, `eth0` will be one family, and `eth1` will be another. 

The reasoning behind calling these instances "families" is that different charts for the same instance can and many times are related (relatives, family, you get it). The family of a chart is usually the name of the netdata dashboard submenu that you see selected on the right navigation pane, when you are looking at a chart. For the example of the two network interfaces, you would see a submenu `eth0` and a submenu `eth1` under the "Network Interfaces" menu on the right navigation pane. 

## Contexts

A context is a grouping of identical charts, for each instance of the hardware or software monitored. For example, `health/health.d/net.conf` refers to four contexts: `net.drops`, `net.fifo`, `net.net`, `net.packets`. You can see the context of a chart if you hover over the date right above the dimensions of the chart.  The line that appears shows you two things: the collector that produces the chart and the chart context. 

For example, let's take the `net.packets` context. You will see on the dashboard as many charts with context net.packets as you have network interfaces (families). These charts will be named `net_packets.[family]`. For the example of the two interfaces `eth0` and `eth1`, you will see charts named `net_packets.eth0` and `net_packets.eth1`. Both of these charts show the exact same dimensions, but for different instances of a network interface.  

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2FCharts&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
