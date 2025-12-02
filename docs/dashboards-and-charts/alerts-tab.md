# Alerts Tab

Netdata provides hundreds of pre-configured health alerts to notify you when an anomaly or performance issue affects your node or its applications.

## Raised Alerts Tab

The **Raised Alerts** tab shows all current alerts in your Room that are in a **warning** or **critical** state.

### Alert Table Overview

The table provides key details about each active alert:

| Column               | Description                                                          |
|----------------------|----------------------------------------------------------------------|
| **Alert Name**       | The name of the alert; click to view alert details.                  |
| **Status**           | Current state: Warning or Critical.                                  |
| **Class**            | The alert's class (e.g., Latency, Utilization).                      |
| **Type & Component** | The system type and component involved.                              |
| **Role**             | The notification role assigned to the alert.                         |
| **Node Name**        | The node where the alert was triggered.                              |
| **Silencing Rule**   | Whether silencing rules are applied.                                 |
| **Actions**          | Options to create silencing rules or ask Netdata Assistant for help. |

Use the **gear icon** (top right) to control which columns are visible. Sort alerts by clicking on column headers.

## Filtering Alerts

Filter the alert list using the right-hand bar:

| Filter Option              | Purpose                                                                          |
|----------------------------|----------------------------------------------------------------------------------|
| **Alert Status**           | Filter by status (Warning, Critical).                                            |
| **Alert Class**            | Filter by class (e.g., Latency, Utilization).                                    |
| **Alert Type & Component** | Filter by alert type (e.g., System, Web Server) and component (e.g., CPU, Disk). |
| **Alert Role**             | Filter by the notification role (e.g., Sysadmin, Webmaster).                     |
| **Host Labels**            | Filter by host labels (e.g., `_cloud_instance_region=us-east-1`).                |
| **Node Status**            | Filter by node availability (Live, Offline).                                     |
| **Netdata Version**        | Filter by the Netdata version.                                                   |
| **Nodes**                  | Filter by specific nodes.                                                        |


## Viewing Alert Details

Click on an alert name to open the **alert details page**, which provides:

| Section                      | Description                                                  |
|------------------------------|--------------------------------------------------------------|
| **Latest / Triggered Time**  | Shows when the alert was last triggered.                     |
| **Description**              | Includes a detailed explanation of the alert.                |
| **Netdata Advisor Link**     | Links to related Netdata Advisor guidance.                   |
| **Triggered Chart Snapshot** | Visualizes the chart at the alert’s trigger time.            |
| **Alert Metadata**           | Shows node name, chart instance, type, component, and class. |
| **Configuration**            | Displays the alert's configuration parameters.               |
| **Instance Values**          | Provides node instance details.                              |

At the bottom of this page, click **View alert page** to open a dynamic view where you can:

- Run metric correlations.
- Navigate to the specific node’s chart that triggered the alert.

## Silencing Alerts

In the **Raised Alerts** tab:

- The **Silencing column** shows whether a silencing rule exists for an alert.
- The **Actions column** allows you to:
    - Create a new [silencing rule](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/centralized-cloud-notifications-reference.md#alert-notification-silencing-rules).
    - Ask AI to troubleshoot the alert.

## Alert Configurations Tab

The **Alert Configurations** tab shows the configuration of all running alerts in your Room.

:::note

"Running alerts" are alerts attached to metrics that are actively being collected. Pre-configured alerts that do not match your setup may not appear here.

:::

### Configuration Table Overview

| Column             | Description                                         |
|--------------------|-----------------------------------------------------|
| **Alert Name**     | The name of the alert; click to view configuration. |
| **Node Name**      | The node where this configuration applies.          |
| **Status**         | Whether the alert is active or silenced.            |
| **Silencing Rule** | Indicates if silencing rules are applied.           |
| **Actions**        | Explore configuration or ask the Netdata Assistant. |

Use the **gear icon** to adjust which columns are displayed.

## Viewing Alert Configuration

From the **Actions column**, click **Show Configuration** to:

| Action              | Outcome                                                      |
|---------------------|--------------------------------------------------------------|
| **Explore by Node** | View configurations split by node.                           |
| **View Alert Page** | See full alert details, including configuration and history. |

This allows you to investigate:

- When the alert last triggered.
- All configuration parameters per node.

## Alert Lifecycle Diagram

```mermaid
flowchart TD
    A("Metric Collection") --> B("Alert Evaluation")
    B --> C("Condition Met?")
    C -->|"Yes"| D("Trigger Alert")
    C -->|"No"| J("No Alert Triggered")
    D --> E("Alert in Raised Alerts Tab")
    E --> F("Details and Chart Snapshot")
    E --> G("Apply Silencing Rule")
    F --> H("Explore Metrics<br/>Run Correlations")
    G --> I("No Notification Sent")
    
    %% Style definitions
    classDef alert fill:#ffeb3b,stroke:#000000,stroke-width:3px,color:#000000,font-size:14px
    classDef neutral fill:#f9f9f9,stroke:#000000,stroke-width:3px,color:#000000,font-size:14px
    classDef complete fill:#4caf50,stroke:#000000,stroke-width:3px,color:#000000,font-size:14px
    classDef database fill:#2196F3,stroke:#000000,stroke-width:3px,color:#000000,font-size:14px

    %% Apply styles
    class A,D alert
    class B,C,E neutral
    class F,H,J complete
    class G,I database
```

:::tip

The diagram above illustrates the flow of alert detection and management, from metric collection to alert evaluation, triggering, and optional silencing.

:::