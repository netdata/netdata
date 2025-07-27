# Observability Centralization Points

## What Are Centralization Points?

Observability Centralization Points are specialized Netdata installations that you can configure to **receive, store, and process** observability data (metrics and logs) from multiple other systems in your infrastructure.

These centralization points give you several core functions:

* **Receiving and storing** metrics and logs from multiple systems
* **Processing and analyzing** your collected data
* **Running health checks and alerts**
* Providing **unified dashboards** across all your systems
* **Replicating data** for your historical analysis

This **distributed yet centralized** approach gives you the benefits of both decentralized collection and centralized analysis.

## Why Use Centralization Points?

| Use Case                                        | Description                                                                                    | Benefits                                                                                      |
|-------------------------------------------------|------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------| 
| **Ephemeral Systems**                           | Ideal for your Kubernetes nodes or temporary VMs that frequently go offline                    | You retain metrics and logs for analysis and troubleshooting even after node termination      |
| **Limited Resources**                           | Offloads observability tasks from your systems with low disk space, CPU, RAM, or I/O bandwidth | Your production systems run efficiently without performance trade-offs                        |
| **Multi-Node Dashboards Without Netdata Cloud** | Aggregates data from all your nodes for centralized dashboards                                 | You get Cloud-like functionality in environments that prefer or require on-premises solutions |
| **Restricted Netdata Cloud Access**             | Acts as a bridge when your monitored systems can't connect to Netdata Cloud                    | You can still use Cloud features despite firewall restrictions or security policies           |

## How Multiple Centralization Points Work

| Scenario                    | Operation                                                                | Advantages                                                               |
|-----------------------------|--------------------------------------------------------------------------|--------------------------------------------------------------------------|
| **With Netdata Cloud**      | Queries all your centralization points in parallel for a unified view    | You get a seamless experience regardless of your underlying architecture |
| **Without Netdata Cloud**   | Your centralization points consolidate data from connected systems       | You have a local view of metrics and logs without external dependencies  |
| **High Availability Setup** | Your centralization points share data with each other, forming a cluster | You won't lose data if one centralization point fails                    |

```mermaid
graph TD
    A[Centralization Points<br>Architecture] --> B[Single Centralization Point<br> Setup]
    A --> C[Multiple Independent<br>Centralization Points]
    A --> D[High Availability Cluster]
    
    B --> B1[All systems stream<br>to one centralization point]
    C --> C1[Systems divided<br>by region/service/team]
    D --> D1[Centralization points<br>share data with each other]
    
classDef default fill:#f9f9f9,stroke:#333,stroke-width:1px,color:#333;
classDef green fill:#4caf50,stroke:#333,stroke-width:1px,color:black;
class A default;
class B,C,D,B1,C1,D1 green;
```

## Technical Implementation

Observability Centralization Points consist of two major components you can deploy:

1. **Metrics Centralization** - Uses Netdata's streaming and replication features to centralize your metrics data
2. **Logs Centralization** - Uses systemd-journald methodologies to centralize your log data

You can configure your systems to connect to **multiple centralization points** for redundancy. If a connection fails, they automatically switch to an available alternative.

In a **high-availability setup**, your centralization points can form a cluster by sharing data with each other, ensuring all points have a complete copy of all your metrics and logs.

```mermaid
graph TD
    CP1[Centralization Point 1] --- CP2[Centralization Point 2]
    
    S1[System 1] --> CP1
    S2[System 2] --> CP1
    S3[System 3] --> CP2
    S4[System 4] --> CP2
    
    S1 -.-> CP2
    S2 -.-> CP2
    S3 -.-> CP1
    S4 -.-> CP1
    
classDef default fill:#f9f9f9,stroke:#333,stroke-width:1px,color:#333;
classDef green fill:#4caf50,stroke:#333,stroke-width:1px,color:black;
classDef blue fill:#2196F3,stroke:#333,stroke-width:1px,color:white;
class CP1,CP2 default;
class S1,S2,S3,S4 blue;
```
