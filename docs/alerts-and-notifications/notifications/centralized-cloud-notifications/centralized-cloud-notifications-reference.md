# Centralized Cloud Notifications Reference

Netdata Cloud sends Alert notifications for nodes in warning, critical, or unreachable states, ensuring Alerts are managed centrally and efficiently.

## Benefits of Centralized Notifications

- Consolidate health status views across all infrastructure in one place
- Set up and [manage your Alert notifications easily](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/manage-notification-methods.md)
- Expedite troubleshooting with tools like [Metric Correlations](/docs/metric-correlations.md) and the [Anomaly Advisor](/docs/dashboards-and-charts/anomaly-advisor-tab.md)

:::info

To avoid notification overload, **flood protection** is triggered when a node frequently disconnects or sends excessive Alerts, highlighting potential issues. You can still access node details through Netdata Cloud, or directly via the local Agent dashboard.

:::

:::important

You must [enable Alert notifications](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/manage-notification-methods.md#manage-space-notification-settings) for your Space(s) as an administrator. All users can then customize their notification preferences through their [account menu](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/manage-notification-methods.md#manage-user-notification-settings).

:::

:::note

Centralized Alerts in Netdata Cloud are separate from the [Netdata Agent](/docs/alerts-and-notifications/notifications/README.md) notifications. You must [configure Agent Alerts individually](/src/health/REFERENCE.md) on each node.

:::

## Alert Notifications

You can send notifications via email or through third-party services like PagerDuty or Slack. You can manage notification settings for the entire Space as an administrator, while individual users can personalize settings in their profile.

### Service Level

```mermaid
%%{init: {'theme': 'dark', 'themeVariables': { 'primaryColor': '#2b2b2b', 'primaryTextColor': '#fff', 'primaryBorderColor': '#7C0000', 'lineColor': '#F8B229', 'secondaryColor': '#006100', 'tertiaryColor': '#333'}}}%%
flowchart LR
    ServiceLevel[Service Level] --> Personal[Personal]
    ServiceLevel --> System[System]
    
    Personal --> PersonalManaged[Managed By Users]
    System --> SystemManaged[Managed By Admins]
    
    PersonalManaged --> PersonalDest[User Specific destinations<br/>such as Email]
    SystemManaged --> SystemDest[General Targets<br/>like Slack Channels]
    
    style ServiceLevel fill:#ffeb3b,stroke:#555,color:#333,stroke-width:2px,rx:10,ry:10
    style Personal fill:#4caf50,stroke:#333,color:#fff,stroke-width:1px,rx:10,ry:10
    style System fill:#2196F3,stroke:#333,color:#fff,stroke-width:1px,rx:10,ry:10
    style PersonalManaged fill:#4caf50,stroke:#333,color:#fff,stroke-width:1px,rx:10,ry:10
    style SystemManaged fill:#2196F3,stroke:#333,color:#fff,stroke-width:1px,rx:10,ry:10
    style PersonalDest fill:#f9f9f9,stroke:#444,color:#333,stroke-width:1px,rx:10,ry:10
    style SystemDest fill:#f9f9f9,stroke:#444,color:#333,stroke-width:1px,rx:10,ry:10
```

### Service Classification

```mermaid
%%{init: {'theme': 'dark', 'themeVariables': { 'primaryColor': '#2b2b2b', 'primaryTextColor': '#fff', 'primaryBorderColor': '#7C0000', 'lineColor': '#F8B229', 'secondaryColor': '#006100', 'tertiaryColor': '#333'}}}%%
flowchart LR
    Classification[Service Classification] --> Community[Community]
    Classification --> Business[Business]
    
    Community --> CommunityType[Basic Methods]
    Business --> BusinessType[Advanced Methods]
    
    CommunityType --> CommunityMethods[e.g. Email & Discord]
    BusinessType --> BusinessMethods[e.g. PagerDuty & Slack]
    
    style Classification fill:#ffeb3b,stroke:#555,color:#333,stroke-width:2px,rx:10,ry:10
    style Community fill:#4caf50,stroke:#333,color:#fff,stroke-width:1px,rx:10,ry:10
    style Business fill:#2196F3,stroke:#333,color:#fff,stroke-width:1px,rx:10,ry:10
    style CommunityType fill:#4caf50,stroke:#333,color:#fff,stroke-width:1px,rx:10,ry:10
    style BusinessType fill:#2196F3,stroke:#333,color:#fff,stroke-width:1px,rx:10,ry:10
    style CommunityMethods fill:#f9f9f9,stroke:#444,color:#333,stroke-width:1px,rx:10,ry:10
    style BusinessMethods fill:#f9f9f9,stroke:#444,color:#333,stroke-width:1px,rx:10,ry:10
```

## Alert Notification Silencing Rules

Netdata Cloud offers a silencing rule engine to mute Alert notifications based on specific conditions related to nodes or Alert types. You can manage these settings [here](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/manage-alert-notification-silencing-rules.md).

You can set up silencing rules that apply to any combination of:
- Users, rooms, nodes, host labels
- Contexts (charts), alert name, alert role
- Optional starting and ending date/time for scheduled maintenance windows

## Disabling Notifications from Netdata Cloud UI

You can turn off all notifications for a user at the space or war room level at the beginning of any maintenance window. This disables all notifications from Netdata Cloud, though you'll still need additional steps to disable or mute alerts from the nodes/agents themselves.

[For more info click here](https://learn.netdata.cloud/docs/alerts-&-notifications/alert-configuration-reference#disable-or-silence-alerts)

## Anatomy of an Email Alert Notification

```mermaid
%%{init: {'theme': 'dark', 'themeVariables': { 'primaryColor': '#2b2b2b', 'primaryTextColor': '#fff', 'primaryBorderColor': '#7C0000', 'lineColor': '#F8B229', 'secondaryColor': '#006100', 'tertiaryColor': '#333'}}}%%
flowchart TD
   Alert[Alert Triggered] --> Email{Generate Email}
   style Alert fill:#f44336,stroke:#333,color:#fff,stroke-width:1px,rx:10,ry:10
   style Email fill:#2196F3,stroke:#333,color:#fff,stroke-width:1px,rx:10,ry:10
   
   Email --> Space[Space Name]
   Email --> Node[Node Name]
   Email --> Status[Alert Status]
   Email --> Previous[Previous Status]
   
   Email --> Time[Trigger Time]
   Email --> Context[Chart Context]
   Email --> AlertInfo[Alert Name & Info]
   Email --> Value[Alert Value]
   
   Email --> Count[Total Alerts Count]
   Email --> Threshold[Trigger Threshold]
   Email --> Calculation[Calculation Method]
   Email --> Source[Alert Source File]
   Email --> Link[Dashboard Link]
   
   style Space fill:#f9f9f9,stroke:#444,color:#333,stroke-width:1px,rx:10,ry:10
   style Node fill:#f9f9f9,stroke:#444,color:#333,stroke-width:1px,rx:10,ry:10
   style Status fill:#f9f9f9,stroke:#444,color:#333,stroke-width:1px,rx:10,ry:10
   style Previous fill:#f9f9f9,stroke:#444,color:#333,stroke-width:1px,rx:10,ry:10
   
   style Time fill:#4caf50,stroke:#333,color:#fff,stroke-width:1px,rx:10,ry:10
   style Context fill:#4caf50,stroke:#333,color:#fff,stroke-width:1px,rx:10,ry:10
   style AlertInfo fill:#4caf50,stroke:#333,color:#fff,stroke-width:1px,rx:10,ry:10
   style Value fill:#4caf50,stroke:#333,color:#fff,stroke-width:1px,rx:10,ry:10
   
   style Count fill:#ffeb3b,stroke:#555,color:#333,stroke-width:1px,rx:10,ry:10
   style Threshold fill:#ffeb3b,stroke:#555,color:#333,stroke-width:1px,rx:10,ry:10
   style Calculation fill:#ffeb3b,stroke:#555,color:#333,stroke-width:1px,rx:10,ry:10
   style Source fill:#ffeb3b,stroke:#555,color:#333,stroke-width:1px,rx:10,ry:10
   style Link fill:#ffeb3b,stroke:#555,color:#333,stroke-width:1px,rx:10,ry:10
```

Email notifications provide comprehensive details to help you understand and respond to alerts effectively.
