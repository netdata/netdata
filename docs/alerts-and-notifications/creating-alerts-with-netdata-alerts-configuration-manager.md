# Creating Alerts with Netdata Alerts Configuration Manager

The Netdata Alerts Configuration Manager enables subscribers to easily set up Alerts directly from the Netdata Dashboard. More details on subscriptions can be found [here](https://www.netdata.cloud/pricing/).

## Using the Alerts Configuration Manager

1. Navigate to the **Metrics** tab and select the chart you want to configure for Alerts.
2. Click the **Alert icon** in the top right corner of the chart.
3. The Alert Configuration Manager will open, showing the default thresholds. Modify these thresholds as needed; the Alert definition on the right will update automatically.
4. For additional settings, toggle **Show advanced**.
5. After configuring the Alert, copy the generated Alert definition from the code box. Paste this into an existing or new custom health configuration file located at `<path to netdata install>/etc/netdata/health.d/` on a Parent Agent or a Standalone Child Agent. The guide to edit health configuration files is available [here](/src/health/REFERENCE.md#edit-health-configuration-files).
6. To activate the new Alert, run the command `<path to netdata install>/usr/sbin/netdatacli reload-health`.

## Alerts Configuration Manager Sections

### Alert Detection Method

An Alert is triggered whenever a metric crosses a threshold:

- **Standard Threshold**: Triggered when a metric crosses a predefined value.
- **Metric Variance**: Triggered based on the variance of the metric.
- **Anomaly Rate**: Triggered based on the anomaly rate of the metric.

### Metrics Lookup, Filtering, and Formula Section

You can read more about the different options in the [Alerts reference documentation](/src/health/REFERENCE.md).

- **Metrics Lookup**: Adjust the database lookup parameters directly in the UI, including method (`avg`, `sum`, `min`, `max`, etc.), computation style, dimensions, duration, and options like `absolute` or `percentage`.
- **Alert Filtering**: The **show advanced** checkbox allows filtering of Alert health checks for specific infrastructure components. Options include selecting hosts, nodes, instances, chart labels, and operating systems.
- **Formula / Calculation**: The **show advanced** checkbox allows defining a formula for the metric value, which is then used to set Alert thresholds.

### Alerting Conditions

- **Thresholds**: Set thresholds for warning and critical Alert states, specifying whether the Alert should trigger above or below these thresholds. Advanced settings allow for custom formulas.
- **Recovery Thresholds**: Set thresholds for downgrading the Alert from critical to warning or from warning to clear.
- **Check Interval**: Define how frequently the health check should run.
- **Delay Notifications**: Manage notification delays for Alert escalations or de-escalations.
- **Agent-Specific Options**: Options exclusive to the Netdata Agent, like repeat notification frequencies and notification recipients.
- **Custom Exec Script**: Define custom scripts to execute when an Alert triggers.

### Alert Name, Description, and Summary Section

- **Alert Template Name**: Provide a unique name for the Alert.
- **Alert Template Description**: Offer a brief explanation of what the Alert
