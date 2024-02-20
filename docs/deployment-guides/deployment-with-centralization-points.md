# Deployment with Centralization Points

An observability centralization point can centralize both metrics and logs. The sending systems are called Children, while the receiving systems are called a Parents.

When metrics and logs are centralized, the Children are never queried for metrics and logs. The Netdata Parents have all the data needed to satisfy queries.

- **Metrics** are centralized by Netdata, with a feature we call **Streaming**. The Parents listen for incoming connections and permit access only to Children that connect to it with the right API key. Children are configured to push their metrics to the Parents and they initiate the connections to do so.

- **Logs** are centralized with methodologies provided by `systemd-journald`. This involves installing `systemd-journal-remote` on both the Parent and the Children, and configuring the keys required for this communication.

|                    Feature                    |                                                 How it works                                                  |
|:---------------------------------------------:|:-------------------------------------------------------------------------------------------------------------:|
| Unified infrastructure dashboards for metrics |                                             Yes, at Netdata Cloud                                             |
|  Unified infrastructure dashboards for logs   | All logs are accessible via the same dashboard at Netdata Cloud, although they are unified per Netdata Parent |
|          Centrally configured alerts          |                                            Yes, at Netdata Parents                                            |
|   Centrally dispatched alert notifications    |                                             Yes, at Netdata Cloud                                             |
|         Data are exclusively on-prem          |                                                 Yes, Netdata Cloud queries Netdata Agents to satisfy dashboard queries.                                                 |

A configuration with 2 observability centralization points, looks like this:

```mermaid
flowchart LR
    WEB[["One unified
        dashboard
        for all nodes"]]
    NC(["<b>Netdata Cloud</b>
        decides which agents
        need to be queried"])
    SA1["Netdata at AWS
         A1"]
    SA2["Netdata at AWS
         A2"]
    SAN["Netdata at AWS
         AN"]
    PA["<b>Netdata Parent A</b>
        at AWS
        having all metrics & logs
        for all Ax nodes"]
    SB1["Netdata On-Prem
         B1"]
    SB2["Netdata On-Prem
         B2"]
    SBN["Netdata On-Prem
         BN"]
    PB["<b>Netdata Parent B</b>
        On-Prem
        having all metrics & logs
        for all Bx nodes"]
    WEB -->|query| NC -->|query| PA & PB
    PA ---|stream| SA1 & SA2 & SAN
    PB ---|stream| SB1 & SB2 & SBN 
```

Netdata Cloud queries the Netdata Parents to provide aggregated dashboard views.

For alerts, the dispatch of notifications looks like in the following chart:

```mermaid
flowchart LR
    NC(["<b>Netdata Cloud</b>
        applies silencing
        & user settings"])
    SA1["Netdata at AWS
         A1"]
    SA2["Netdata at AWS
         A2"]
    SAN["Netdata at AWS
         AN"]
    PA["<b>Netdata Parent A</b>
        at AWS
        having all metrics & logs
        for all Ax nodes"]
    SB1["Netdata On-Prem
         B1"]
    SB2["Netdata On-Prem
         B2"]
    SBN["Netdata On-Prem
         BN"]
    PB["<b>Netdata Parent B</b>
        On-Prem
        having all metrics & logs
        for all Bx nodes"]
    EMAIL{{"<b>e-mail</b>
        notifications"}}
    MOBILEAPP{{"<b>Netdata Mobile App</b>
        notifications"}}
    SLACK{{"<b>Slack</b>
        notifications"}}
    OTHER{{"Other
        notifications"}}
    PA & PB -->|alert transitions| NC -->|notification| EMAIL & MOBILEAPP & SLACK & OTHER 
    SA1 & SA2 & SAN ---|stream| PA
    SB1 & SB2 & SBN ---|stream| PB 
```

### Configuration steps for deploying Netdata with Observability Centralization Points

For Metrics:

- Install Netdata agents on all systems and the Netdata Parents.

- Configure `stream.conf` at the Netdata Parents to enable streaming access with an API key.

- Configure `stream.conf` at the Netdata Children to enable streaming to the configured Netdata Parents.

For Logs:

- Install `systemd-journal-remote` on all systems and the Netdata Parents.

- Configure `systemd-journal-remote` at the Netdata Parents to enable logs reception.

- Configure `systemd-journal-upload` at the Netdata Children to enable transmission of their logs to the Netdata Parents.

Optionally:

- Disable ML, health checks and dashboard access at Netdata Children to save resources and avoid duplicate notifications.

When using Netdata Cloud:

- Optionally: disable dashboard access on all Netdata agents (including Netdata Parents).
- Optionally: disable alert notifications on all Netdata agents (including Netdata Parents).
