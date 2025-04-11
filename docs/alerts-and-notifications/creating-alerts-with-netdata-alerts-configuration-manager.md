# Creating Alerts with Netdata Alerts Configuration Manager

The **Netdata Alerts Configuration Manager** lets you create and fine-tune alerts directly from the Netdata Cloud Dashboard.  
To use this feature, you’ll need an active Netdata subscription. → [View subscription plans](https://www.netdata.cloud/pricing/)

---

## How to Create an Alert (Step-by-Step)

1. Go to the **Metrics** tab in your Netdata Cloud Dashboard.
2. Find the chart you want to monitor.
3. Click the **alert icon** in the top-right corner of the chart.
4. The **Alert Configuration Manager** will open.
5. Adjust the threshold values. The alert definition on the right updates live as you make changes.
6. (Optional) Click **Show advanced** for more detailed settings.
7. Copy the generated alert definition from the code box.
8. Paste it into a custom alert file on your Agent:

   ```
   <INSTALL_PREFIX>/etc/netdata/health.d/your-alert.conf
   ```
   → [How to edit alert configuration files](/src/health/REFERENCE.md#edit-health-configuration-files)

9. Apply your changes:

   ```
   <INSTALL_PREFIX>/usr/sbin/netdatacli reload-health
   ```
---

## Alert Detection Types

Netdata supports different ways of triggering alerts:

| Type              | Description                                                 |
|-------------------|-------------------------------------------------------------|
| **Standard**       | Fires when a metric crosses a set value                    |
| **Metric Variance**| Fires based on variation in values over time              |
| **Anomaly Rate**   | Fires when the anomaly rate exceeds a certain threshold   |

Choose the type that best suits the behavior you want to monitor.

---

## Metrics Lookup & Filters

Click **Show advanced** in the Alert Configuration Manager to access these options.

### Metrics Lookup

You can define how Netdata should query the data before triggering an Alert:

| Parameter   | Description                       |
|-------------|-----------------------------------|
| `method`    | How values are aggregated (`avg`, `min`, `max`, etc.) |
| `duration`  | Time window used for the check    |
| `dimensions`| Which metric dimensions to include|
| `options`   | Modify how values are interpreted (e.g., `percentage`, `absolute`) |

### Filtering Targets

Limit the alert to specific infrastructure components:

- Hosts
- Nodes
- Instances
- Operating systems
- Chart labels

### Formulas

Use a custom formula to manipulate values before comparing against thresholds.

Example:

```txt
(metric1 - metric2) / 100
```

---

## Defining Alert Conditions

You can control how and when alerts are triggered, escalated, or resolved.

| Setting              | Purpose                                                                 |
|----------------------|-------------------------------------------------------------------------|
| **Thresholds**        | Define values for `warning` and `critical` states                      |
| **Recovery thresholds**| Set when the alert should downgrade or clear                          |
| **Check interval**    | How often the alert check runs (e.g., every 10 seconds)                |
| **Notification delay**| Delay before sending notifications for state changes                   |
| **Repeat notifications**| How often to resend an alert if the issue persists (Agent only)     |
| **Notification recipients**| Define who gets alerted (Agent only)                              |
| **Custom exec script** | Run a custom shell script when an alert triggers                      |

---

## Naming and Documentation

| Field               | Description                                        |
|---------------------|----------------------------------------------------|
| **Alert Name**       | A unique name for the alert                       |
| **Description**      | What the alert does, in one or two sentences      |
| **Summary**          | Optional: a short summary for display in dashboards |

---

## Final Notes

- You can apply alert definitions to **Parent Agents** or **Standalone Child Agents**
- If you need help writing custom alerts, check the [full alert reference](/src/health/REFERENCE.md)
