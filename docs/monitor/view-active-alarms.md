# View active alerts

Netdata comes with hundreds of pre-configured health alerts designed to notify you when an anomaly or performance issue affects your node or its applications.

From the Alerts tab you can see all the active alerts in your War Room. You will be presented with a table having information about each alert that is in warning and critical state.
You can always sort the table by a certain column by clicking on the name of that column, and use the gear icon on the top right to control which columns are visible at any given time.

![image](https://user-images.githubusercontent.com/70198089/226340574-7e138dc7-5eab-4c47-a4a9-5f2640e38643.png)

## Filter alerts

From this tab, you can also filter alerts with the right hand bar. More specifically you can filter:

- Alert status
  - Filter based on the status of the alerts (e.g. Warning, Critical)
- Alert class
  - Filter based on the class of the alert (e.g. Latency, Utilization, Workload etc.)
- Alert type & component
  - Filter based on the alert's type (e.g. System, Web Server) and component (e.g. CPU, Disk, Load)
- Alert role
  - Filter by the role that the alert is set to notify (e.g. Sysadmin, Webmaster etc.)
- Nodes
  - Filter the alerts based on the nodes that are online, next to each node's name you can see how many alerts the node has, "critical" colored in red and "warning" colored in yellow

## View alert details

By clicking on the name of an entry of the table you can access that alert's details page, providing you with:

- Latest and Triggered time values
- The alert's description
- A link to the Community forum's alert page
- The chart at the time frame that the alert was triggered
- The alert's information: Node name, chart ID, type, component and class
- Configuration section
- Instance values - Node Instances

![image](https://user-images.githubusercontent.com/70198089/226339928-bae60140-0293-42cf-9713-ac4901708aba.png)

At the bottom of the panel you can click the green button "View dedicated alert page" to open a [dynamic tab](https://github.com/netdata/netdata/blob/master/docs/quickstart/infrastructure.md#dynamic-tabs) containing all the info for this alert in a tab format, where you can also run correlations and go to the node's chart that raised the particular alert.

![image](https://user-images.githubusercontent.com/70198089/226339794-61896c35-0b93-4ac9-92aa-07116fe63784.png)

<!-- 
## Local Netdata Agent dashboard

Find the alarms icon ![Alarms
icon](https://raw.githubusercontent.com/netdata/netdata-ui/98e31799c1ec0983f433537ff16d2ac2b0d994aa/src/components/icon/assets/alarm.svg)
in the top navigation to bring up a modal that shows currently raised alarms, all running alarms, and the alarms log.
Here is an example of a raised `system.cpu` alarm, followed by the full list and alarm log:

![Animated GIF of looking at raised alarms and the alarm
log](https://user-images.githubusercontent.com/1153921/80842482-8c289500-8bb6-11ea-9791-600cfdbe82ce.gif)

And a static screenshot of the raised CPU alarm: 

![Screenshot of a raised system CPU
alarm](https://user-images.githubusercontent.com/1153921/80842330-2dfbb200-8bb6-11ea-8147-3cd366eb0f37.png)

The alarm itself is named **system - cpu**, and its context is `system.cpu`. Beneath that is an auto-updating badge that
shows the latest value of the chart that triggered the alarm.

With the three icons beneath that and the **role** designation, you can:

1.  Scroll to the chart associated with this raised alarm.
2.  Copy a link to the badge to your clipboard.
3.  Copy the code to embed the badge onto another web page using an `<embed>` element.

The table on the right-hand side displays information about the health entity that triggered the alarm, which you can
use as a reference to [configure alarms](https://github.com/netdata/netdata/blob/master/health/REFERENCE.md).
 -->
