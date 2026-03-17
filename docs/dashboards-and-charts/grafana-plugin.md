# Grafana Plugin

Netdata integrates with Grafana through a dedicated data source plugin, enabling you to visualize Netdata metrics in Grafana dashboards.

## Installation

Install the Netdata data source plugin from the Grafana plugin marketplace:

1. Navigate to **Configuration** > **Data Sources** in Grafana
2. Click **Add data source**
3. Search for "Netdata"
4. Click **Install**

For detailed installation instructions, see the [Netdata Grafana data source plugin repository](https://github.com/netdata/netdata-grafana-datasource-plugin).

## Configuration

Configure the data source with your Netdata Agent URL:

1. Set the **URL** to your Netdata Agent endpoint (e.g., `http://localhost:19999`)
2. If using authentication, provide the necessary credentials
3. Click **Save & Test** to verify the connection

## Filter by Feature

### Overview

The "Filter by" option in Grafana visualizations allows you to filter metrics based on host labels. This feature enables you to create dynamic dashboards that automatically show data from specific nodes based on their labels.

### Requirements

The "Filter by" feature requires **host labels** to be configured on your Netdata Agent. These labels are set in the `[host labels]` section of your `netdata.conf` file.

Without configured host labels, the "Filter by" dropdown will be empty or may not function as expected.

### Configuring Host Labels

To enable the "Filter by" feature, configure host labels on your Netdata Agent:

1. Edit your Netdata configuration:

   ```bash
   cd /etc/netdata   # Replace with your Netdata config directory
   sudo ./edit-config netdata.conf
   ```

2. Add a `[host labels]` section with your custom labels:

   ```text
   [host labels]
       environment = production
       location = datacenter-east
       service_type = webserver
   ```

3. Reload the labels without restarting Netdata:

   ```bash
   netdatacli reload-labels
   ```

### Verifying Labels

To verify that your host labels are configured correctly, access the Netdata API endpoint:

```bash
curl http://localhost:19999/api/v1/info | grep -A 20 "host_labels"
```

Or open in your browser:

```
http://YOUR-NETDATA-HOST:19999/api/v1/info
```

You should see your configured labels in the JSON response:

```json
{
  "host_labels": {
    "environment": "production",
    "location": "datacenter-east",
    "service_type": "webserver"
  }
}
```

### Using Labels in Grafana

Once host labels are configured:

1. In your Grafana dashboard, add or edit a visualization
2. Configure the Netdata data source query
3. Use the "Filter by" option to select nodes based on their host labels
4. The visualization will automatically filter metrics from nodes matching the selected labels

### Additional Resources

For comprehensive information on configuring and using host labels for organizing your infrastructure:

- [Organize systems, metrics, and alerts](/docs/netdata-agent/configuration/organize-systems-metrics-and-alerts.md) - Learn about host labels, virtual nodes, and metric labels
- [Netdata Grafana data source plugin documentation](https://github.com/netdata/netdata-grafana-datasource-plugin) - Plugin-specific configuration and troubleshooting

## Troubleshooting

### Filter by option shows no labels

If the "Filter by" dropdown is empty:

1. Verify host labels are configured in `netdata.conf` under the `[host labels]` section
2. Ensure labels have been loaded using `netdatacli reload-labels`
3. Check that labels are visible at `/api/v1/info`
4. Verify the Grafana data source connection is working

### Labels not appearing in Grafana

If labels are configured but not appearing in Grafana:

1. Refresh the data source configuration in Grafana
2. Check the Netdata Agent logs for any errors
3. Verify the Netdata API is accessible from the Grafana server
4. Ensure labels don't start with underscore (`_`) - these are reserved for automatic labels
