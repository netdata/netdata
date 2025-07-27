# Standalone Deployment

Netdata provides real-time monitoring out of the box. By default, each Netdata Agent functions as a standalone monitoring system with no additional configuration required.

## Standalone Agents Without Netdata Cloud

Each Netdata Agent operates independently and provides its own monitoring dashboard and alerting system.

### Features

| Feature | How it works |
|---------|-------------|
| **Infrastructure dashboards for metrics** | No, each Netdata Agent provides its own dashboard. |
| **Infrastructure dashboards for logs** | No, logs are only accessible per individual Netdata Agent. |
| **Centralized alert configuration** | No, each Netdata Agent has its own alert settings. |
| **Centralized alert notifications** | No, each Netdata Agent sends notifications independently. |
| **On-prem data retention** | Yes, all collected data remains on the monitored system. |

Each Netdata Agent is accessible via a unique URL: `http://agent-ip:19999`.

```mermaid
flowchart LR
    WEB["Multiple Independent Dashboards"]
    S1["Standalone Netdata 1"]
    S2["Standalone Netdata 2"]
    SN["Standalone Netdata N"]
    WEB -->|URL 1| S1
    WEB -->|URL 2| S2
    WEB -->|URL N| SN
```

Each agent also manages its own alert notifications:

```mermaid
flowchart LR
    S1["Standalone Netdata 1"]
    S2["Standalone Netdata 2"]
    SN["Standalone Netdata N"]
    EMAIL["Email notifications"]
    SLACK["Slack notifications"]
    OTHER["Other notifications"]
    S1 & S2 & SN .-> SLACK
    S1 & S2 & SN ---> EMAIL
    S1 & S2 & SN ==> OTHER
```

### Configuration Steps

- Install Netdata Agents on each system.
- Access each Agent individually via its URL (`http://agent-ip:19999`).

## Standalone Agents With Netdata Cloud

Connecting Netdata Agents to Netdata Cloud enables centralized monitoring while keeping collected data on-premise.

### Features

| Feature | Description |
|---------|-------------|
| **Infrastructure dashboards for metrics** | Yes, Netdata Cloud provides unified charts aggregating metrics from all systems. |
| **Infrastructure dashboards for logs** | Logs from all agents are accessible in Netdata Cloud (though not merged into a single view). |
| **Centralized alert configuration** | No, each Netdata Agent maintains its own alert settings. |
| **Centralized alert notifications** | Yes, Netdata Cloud manages and dispatches notifications. |
| **On-prem data retention** | Yes, Netdata Cloud queries Netdata Agents in real time. |

Connecting Netdata Agents to Netdata Cloud enables a unified monitoring view without requiring additional infrastructure setup.

```mermaid
flowchart LR
    WEB["Unified Dashboard for All Nodes"]
    NC["Netdata Cloud"]
    S1["Standalone Netdata 1"]
    S2["Standalone Netdata 2"]
    SN["Standalone Netdata N"]
    WEB -->|queries| NC
    NC -->|queries| S1 & S2 & SN
```

Alert notifications are managed centrally in Netdata Cloud:

```mermaid
flowchart LR
    EMAIL["Email notifications"]
    MOBILEAPP["Netdata Mobile App notifications"]
    SLACK["Slack notifications"]
    OTHER["Other notifications"]
    NC["Netdata Cloud"]
    S1["Standalone Netdata 1"]
    S2["Standalone Netdata 2"]
    SN["Standalone Netdata N"]
    NC -->|notification| EMAIL & MOBILEAPP & SLACK & OTHER
    S1 & S2 & SN -->|alert transition| NC
```

> **Note:** Alerts are still triggered by Netdata Agents. Netdata Cloud manages notifications.

### Configuration Steps

- Install Netdata Agents using the installation commands provided by Netdata Cloud to automatically link them to your Space.
- Alternatively, install Netdata Agents manually and connect them via the command line or dashboard.
- **Optional:** Disable direct dashboard access for security.
- **Optional:** Disable individual agent notifications to prevent duplicate alerts (Netdata Agents send email alerts by default if an MTA is detected).