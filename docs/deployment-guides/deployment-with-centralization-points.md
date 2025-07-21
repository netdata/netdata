# Parents: Your Centralization Points

***Parents*** are Netdata Agents that collect and store data from other Agents ("***Children***"). They act as centralization points for your observability data.

## How It Works

1. **You designate some Agents as Parents** - Configure them to receive streaming data
2. **Children stream their data to Parents** - They push metrics continuously
3. **Parents store and process everything** - All metrics and logs from all Children
4. **You access Parents for dashboards and alerts** - Centralized monitoring interface
5. **Cloud queries Parents when configured** - Reduces load on production systems

:::info

Parents give you centralized collection with distributed architecture benefits.

:::

## What Parents Do

Parents are specialized Netdata installations that you can configure to **receive, store, and process** observability data (metrics and logs) from multiple other systems in your infrastructure.

These Parents give you several core functions:

* **Receiving and storing** metrics and logs from multiple systems
* **Processing and analyzing** your collected data
* **Running health checks and alerts**
* Providing **unified dashboards** across all your systems
* **Replicating data** for your historical analysis

:::info

This **distributed yet centralized** approach gives you the benefits of both decentralized collection and centralized analysis.

:::

## Why Use Parents

| Use Case | Description | Benefits |
|----------|-------------|----------|
| **Ephemeral Systems** | Ideal for your Kubernetes nodes or temporary VMs that frequently go offline | You retain metrics and logs for analysis and troubleshooting even after node termination |
| **Limited Resources** | Offloads observability tasks from your systems with low disk space, CPU, RAM, or I/O bandwidth | Your production systems run efficiently without performance trade-offs |
| **Multi-Node Dashboards Without Netdata Cloud** | Aggregates data from all your nodes for centralized dashboards | You get Cloud-like functionality in environments that prefer or require on-premises solutions |
| **Restricted Netdata Cloud Access** | Acts as a bridge when your monitored systems can't connect to Netdata Cloud | You can still use Cloud features despite firewall restrictions or security policies |

## How Multiple Parents Work

<details>
<summary><strong>Click to see Parent architecture options</strong></summary><br/>

```mermaid
flowchart TB
    subgraph architectures["Parent Architecture Options"]
        direction TB
        
        subgraph single["Single Parent"]
            SP[SP]
            SC1[SC1]
            SC2[SC2]
            SC3[SC3]
            
            SP("**Parent**<br/>All data in one place")
            SC1("Child 1")
            SC2("Child 2")
            SC3("Child 3")
            
            SC1 --> SP
            SC2 --> SP
            SC3 --> SP
        end
        
        subgraph multiple["Multiple Parents"]
            MP1[MP1]
            MP2[MP2]
            MC1[MC1]
            MC2[MC2]
            MC3[MC3]
            MC4[MC4]
            
            MP1("**Parent 1**<br/>Region/Team A")
            MP2("**Parent 2**<br/>Region/Team B")
            MC1("Child 1")
            MC2("Child 2")
            MC3("Child 3")
            MC4("Child 4")
            
            MC1 --> MP1
            MC2 --> MP1
            MC3 --> MP2
            MC4 --> MP2
        end
        
        subgraph ha["High Availability"]
            HP1[HP1]
            HP2[HP2]
            HC1[HC1]
            HC2[HC2]
            
            HP1("**Parent 1**<br/>Active")
            HP2("**Parent 2**<br/>Active")
            HC1("Child 1")
            HC2("Child 2")
            
            HC1 --> HP1
            HC2 --> HP1
            HC1 -.-> HP2
            HC2 -.-> HP2
            HP1 <--> HP2
        end
    end
    
    classDef parent fill:#f3e8ff,stroke:#9b59b6,stroke-width:2px,color:#2c3e50,rx:10,ry:10
    classDef child fill:#e8f5e8,stroke:#27ae60,stroke-width:2px,color:#2c3e50,rx:10,ry:10
    classDef subgraphStyle fill:#f8f9fa,stroke:#6c757d,stroke-width:2px,color:#2c3e50,rx:15,ry:15
    classDef innerStyle fill:#f0f8ff,stroke:#87ceeb,stroke-width:2px,color:#2c3e50,rx:12,ry:12
    
    class SP,MP1,MP2,HP1,HP2 parent
    class SC1,SC2,SC3,MC1,MC2,MC3,MC4,HC1,HC2 child
    class architectures subgraphStyle
    class single,multiple,ha innerStyle
```

</details><br/>

| Scenario | Operation | Advantages |
|----------|-----------|------------|
| **With Netdata Cloud** | Queries all your Parents in parallel for a unified view | You get a seamless experience regardless of your underlying architecture |
| **Without Netdata Cloud** | Your Parents consolidate data from connected systems | You have a local view of metrics and logs without external dependencies |
| **High Availability Setup** | Your Parents share data with each other, forming a cluster | You won't lose data if one Parent fails |

## Technical Implementation

Parents consist of two major components you can deploy:

1. **Metrics Centralization** - Uses Netdata's streaming and replication features to centralize your metrics data
2. **Logs Centralization** - Uses systemd-journald methodologies to centralize your log data

You can configure your systems to connect to **multiple Parents** for redundancy. If a connection fails, they automatically switch to an available alternative.

In a **high-availability setup**, your Parents can form a cluster by sharing data with each other, ensuring all points have a complete copy of all your metrics and logs.

<details>
<summary><strong>Click to see how high availability works</strong></summary><br/>

```mermaid
flowchart TB
    NC[NC]
    
    NC("**Netdata Cloud**<br/>Queries available Parents")
    
    subgraph infrastructure["Your Infrastructure"]
        direction TB
        
        P1[P1]
        P2[P2]
        C1[C1]
        C2[C2]
        C3[C3]
        C4[C4]
        
        P1("**Parent 1**<br/>Active")
        P2("**Parent 2**<br/>Active")
        C1("Child 1")
        C2("Child 2")
        C3("Child 3")
        C4("Child 4")
        
        C1 -->|primary| P1
        C2 -->|primary| P1
        C3 -->|primary| P2
        C4 -->|primary| P2
        
        C1 -.->|failover| P2
        C2 -.->|failover| P2
        C3 -.->|failover| P1
        C4 -.->|failover| P1
        
        P1 <-->|sync| P2
    end
    
    NC <--> P1
    NC <--> P2
    
    classDef cloud fill:#e8f4fd,stroke:#4a90e2,stroke-width:2px,color:#2c3e50,rx:10,ry:10
    classDef parent fill:#f3e8ff,stroke:#9b59b6,stroke-width:2px,color:#2c3e50,rx:10,ry:10
    classDef child fill:#e8f5e8,stroke:#27ae60,stroke-width:2px,color:#2c3e50,rx:10,ry:10
    classDef subgraphStyle fill:#f8f9fa,stroke:#6c757d,stroke-width:2px,color:#2c3e50,rx:15,ry:15
    
    class NC cloud
    class P1,P2 parent
    class C1,C2,C3,C4 child
    class infrastructure subgraphStyle
```

</details><br/>

:::tip

Check out our [Parent-Child Deployment Guide](/docs/deployment-guides/parent-child-deployment.md) for step-by-step instructions.

:::