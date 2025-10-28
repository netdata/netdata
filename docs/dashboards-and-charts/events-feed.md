# Events Tab

The **Events tab** provides a powerful feed that tracks key activities across your infrastructure and Space. It helps you investigate historical events, making it easier to correlate changes with anomalies or node behavior.

Use the Events feed to:

- Quickly identify what happened before or after a node went offline.
- Attribute sudden metric changes to specific environment events.
- Access a detailed history of alert transitions and node state changes.

:::note

Based on your Space plan, the time range available for querying past events may vary.

:::

## Available Event Domains

The Events feed provides visibility into the following event types:

| **Event Domain**                        | **Community** | **Homelab** | **Business** | **Enterprise On-Premise** |
|-----------------------------------------|---------------|-------------|--------------|---------------------------|
| **[Auditing events](#auditing-events)** | 4 hours       | 90 days     | 90 days      | User-dependent            |
| **[Topology events](#topology-events)** | 4 hours       | 14 days     | 14 days      | User-dependent            |
| **[Alert events](#alert-events)**       | 4 hours       | 90 days     | 90 days      | User-dependent            |

## Auditing Events

These events log user actions and Space configuration changes:

| **Event Name**          | **Description**                          | **Example**                                                                     |
|-------------------------|------------------------------------------|---------------------------------------------------------------------------------|
| Space Created           | A new Space was created.                 | Space `Acme Space` was **created**.                                             |
| Room Created            | A Room was added to the Space.           | Room `DB Servers` was **created** by `John Doe`.                                |
| Room Deleted            | A Room was removed from the Space.       | Room `DB Servers` was **deleted** by `John Doe`.                                |
| User Invited to Space   | A user was invited to join the Space.    | User `John Smith` was **invited** by `Alan Doe`.                                |
| User Removed from Space | A user was removed from the Space.       | User `John Smith` was **removed** by `Alan Doe`.                                |
| Silencing Rule Created  | A new silencing rule was added.          | Silencing rule `DB Servers schedule silencing` was **created** by `John Smith`. |
| Silencing Rule Changed  | An existing silencing rule was modified. | Silencing rule was **changed** by `John Doe`.                                   |
| Silencing Rule Deleted  | A silencing rule was removed.            | Silencing rule was **deleted** by `Alan Smith`.                                 |

## Topology Events

These events track changes to node connectivity and state:

| **Event Name**      | **Description**                                  | **Example**                                             |
|---------------------|--------------------------------------------------|---------------------------------------------------------|
| Node Became Live    | Node started streaming metrics to Cloud.         | Node `netdata-k8s-state-xyz` is **live**.               |
| Node Became Offline | Node stopped streaming metrics, fully offline.   | Node `ip-xyz.ec2.internal` is **offline**.              |
| Node Created        | Node was created but not yet seen by Cloud.      | Node `ip-xyz.ec2.internal` was **created**.             |
| Node Deleted        | Node was hard deleted from the Space.            | Node `ip-xyz.ec2.internal` was **deleted (hard)**.      |
| Agent Connected     | Agent connected to the Cloud server (MQTT link). | Agent `7d87bqs9-cv42-4823-8sd4-3614548850c7` connected. |
| Agent Disconnected  | Agent disconnected from the Cloud server.        | Agent disconnected due to **Connection Timeout**.       |

## Alert Events

These events log alert state transitions for node metrics:

| **Event Name**           | **Description**                                                                       | **Example**                                                                                                         |
|--------------------------|---------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------|
| Node Alert State Changed | Records state changes such as Cleared, Warning, Critical, Removed, Error, or Unknown. | Alert `httpcheck_web_service_bad_status` on node `netdata-parent-xyz` escalated to **CRITICAL** with value **25%**. |

## Who Can Access Events?

| **User Role**      | **Event Domains Accessible**                    |
|--------------------|-------------------------------------------------|
| Administrators     | All event domains (Auditing, Topology, Alerts). |
| Non-administrators | Topology and Alerts only.                       |

:::note

See the [Role-Based Access model](/docs/netdata-cloud/authentication-and-authorization/role-based-access-model.md) for details.

:::

## How to Use the Events Feed

1. Click the **Events** tab.
2. Define the timeframe using the [Date and Time selector](/docs/dashboards-and-charts/visualization-date-and-time-controls.md#date-and-time-selector).
3. Apply filters from the right-hand bar, such as **event domain**, **node**, **alert severity**, or **time range**, to focus on the data you need.

:::note

If your query exceeds the retention limits of your plan, an error will indicate that the requested data is outside your allowed timeframe.

:::
