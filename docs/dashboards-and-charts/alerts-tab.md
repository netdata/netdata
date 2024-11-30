# Alerts tab

Netdata comes with hundreds of pre-configured health alerts designed to notify you when an anomaly or performance issue affects your node or its applications.

## Active tab

From the Active tab, you can see all the active alerts in your Room. You will be presented with a table having information about each alert that is in warning or critical state.

You can always sort the table by a certain column by clicking on the name of that column, and using the gear icon on the top right to control which columns are visible at any given time.

### Filter alerts

From this tab, you can also filter alerts with the right-hand bar. More specifically, you can filter:

- **Alert status**: Filter based on the status of the alerts (e.g., Warning, Critical)
- **Alert class**: Filter based on the class of the alert (e.g., Latency, Utilization, Workload, etc.)
- **Alert type & component**: Filter based on the alert's type (e.g., System, Web Server) and component (e.g., CPU, Disk, Load)
- **Alert role**: Filter by the role that the alert is set to notify (e.g., Sysadmin, Webmaster etc.)
- **Host labels**: Filter based on the host labels that are configured for the nodes across the Room (e.g., `_cloud_instance_region` to match `us-east-1`)
- **Node status**: Filter by node availability status (e.g., Live or Offline)
- **Netdata version**: Filter by Netdata version (e.g., `v1.45.3`)
- **Nodes**: Filter the alerts based on the nodes of your Room.

### View alert details

By clicking on the name of an entry of the table, you can access that alert's details page, providing you with:

- Latest and Triggered time values
- The alert's description
- A link to the Netdata Advisor's page about this alert
- The chart at the time frame that the alert was triggered
- The alert's information: Node name, chart instance, type, component and class
- Configuration section
- Instance values - Node Instances

At the bottom of the panel you can click the green button "View alert page" to open a dynamic tab containing all the info for this alert in a tab format, where you can also run correlations and go to the node's chart that raised the particular alert.

### Silence an alert

From this tab, the "Silencing" column shows if there is any rule present for each alert, and from the "Actions" column you can create a new [silencing rule](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/centralized-cloud-notifications-reference.md#alert-notification-silencing-rules) for this alert, or get help and information about this alert from the [Netdata Assistant](/docs/netdata-assistant.md).

## Alert Configurations tab

From this tab, you can view all the configurations for all running alerts in your Room. Each row concerns one alert, and it provides information about it in the rest of the table columns.

By running alerts, we mean alerts that are related to some metric that is or was collected. Netdata may have more alerts pre-configured that aren't applicable to your monitoring use-cases.

You can control which columns are visible by using the gear icon on the right-hand side.

Similarly to the previous tab, you can see the silencing status of an alert, while also being able to dig deeper and show the configuration for the alert and ask the [Netdata Assistant](/docs/netdata-assistant.md) for help.

### See the configuration for an alert

From the actions column you can explore the alert's configuration, split by the different nodes that have this alert configured.

From there, you can click on any of the rows to get to the individual alert configurations for that node.

Click on an alert row to see the alert's page, with all the information about when it was last triggered, and what its configuration is.
