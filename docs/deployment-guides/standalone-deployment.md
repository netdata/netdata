# Single Agent Deployment

The simplest way to use Netdata - install it, and you're monitoring. Each Agent works independently with zero configuration.

## Single Agents With Netdata Cloud (Recommended)

Get the best experience - one dashboard for all your systems, mobile alerts, and team collaboration. Your data stays on your servers.

### What You Get

| Feature                         | How it Works                             |
|---------------------------------|------------------------------------------|
| **Unified metrics dashboard**   | ✓ See all Agents in one place            |
| **Unified logs view**           | ✓ Access all logs from Cloud             |
| **Central alert configuration** | Each Agent still manages its own alerts  |
| **Central notifications**       | ✓ Cloud handles all notifications        |
| **Data stays on-premise**       | ✓ Cloud queries your Agents in real-time |

<details>
<summary><strong>Click to see visual representation of the architecture</strong></summary><br/>

```mermaid
flowchart TB
    NC("**Netdata Cloud**<br/>Unified dashboards<br/>Central notifications<br/>Access from anywhere")
    Users("**One Dashboard**<br/>for all your systems")
    Notifications("**Alert Notifications**<br/>Email, Slack, Mobile App")
    Users <--> NC
    NC --> Notifications

    subgraph infrastructure["Your Infrastructure"]
        direction TB
        Agents("**Netdata Agents**<br/>Agent 1, Agent 2,<br/>Agent 3")
        Data("**Your Metrics**<br/>Stay on your servers")
        Agents <--> Data
    end

    NC <--> Agents

    %% Style definitions
    classDef alert fill:#ffeb3b,stroke:#000000,stroke-width:3px,color:#000000,font-size:18px
    classDef neutral fill:#f9f9f9,stroke:#000000,stroke-width:3px,color:#000000,font-size:18px
    classDef complete fill:#4caf50,stroke:#000000,stroke-width:3px,color:#000000,font-size:18px
    classDef database fill:#2196F3,stroke:#000000,stroke-width:3px,color:#000000,font-size:18px

    %% Apply styles
    class Users,Agents alert
    class NC,Notifications neutral
    class Data complete
    class infrastructure database
```

</details>

### Setup

Getting started is simple:

1. **Sign up for Netdata Cloud** (it's free)
2. **Get your connection command** - Once logged in, you have three ways to get it:
    - Navigate to **Space Settings** → **Nodes** → Click **"+"**
    - Go to **Nodes** tab → Click **Add nodes**
    - Visit **Integrations** page → Select your OS

3. **Run the installation command** that includes your unique claim token and room information

**What happens next:**

- The command automatically detects your OS
- Installs the latest Netdata Agent
- Connects to your Cloud Space
- Your node appears live in seconds
- Charts start streaming real-time data immediately

:::tip

Get detailed instructions on how to connect Agents to Cloud in our [Connect Agent to Cloud Guide](https://github.com/netdata/netdata/blob/master/src/claim/README.md).

:::

### Optional Optimizations

- Disable local Agent notifications (Cloud handles them better)
- Restrict local dashboard access for security (use Cloud instead)

## Single Agents Without Cloud

You can also run Agents independently, though you'll miss out on unified dashboards and mobile alerts.

<details>
<summary><strong>Click to see visual representation of standalone architecture</strong></summary><br/>

```mermaid
flowchart TB
    subgraph infrastructure["Your Infrastructure"]
        direction TB
        A1("**Agent 1**<br/>Independent monitoring")
        A2("**Agent 2**<br/>Independent monitoring")
        A3("**Agent 3**<br/>Independent monitoring")
        D1("**Dashboard 1**<br/>:19999")
        D2("**Dashboard 2**<br/>:19999")
        D3("**Dashboard 3**<br/>:19999")
        N1("**Alerts**<br/>Local notifications")
        N2("**Alerts**<br/>Local notifications")
        N3("**Alerts**<br/>Local notifications")
        
        A1 --> D1
        A2 --> D2
        A3 --> D3
        A1 --> N1
        A2 --> N2
        A3 --> N3
    end

    %% Style definitions matching the reference
    classDef alert fill:#ffeb3b,stroke:#000000,stroke-width:3px,color:#000000,font-size:18px
    classDef neutral fill:#f9f9f9,stroke:#000000,stroke-width:3px,color:#000000,font-size:18px
    classDef complete fill:#4caf50,stroke:#000000,stroke-width:3px,color:#000000,font-size:18px
    classDef database fill:#2196F3,stroke:#000000,stroke-width:3px,color:#000000,font-size:18px

    %% Apply styles
    class A1,A2,A3 alert
    class D1,D2,D3 neutral
    class N1,N2,N3 complete
    class infrastructure database
```

</details>

### Setup

:::tip

Check the [Version & Platform](https://learn.netdata.cloud/docs/netdata-agent/versions-&-platforms) that's suitable for your needs and install the Agent.

:::

1. Install Netdata on each system
2. Access each dashboard at `http://agent-ip:19999`
3. Configure alerts individually on each Agent

## When to Use Each Approach

**Use Cloud-connected Agents when:**

- You want one dashboard for everything
- You need mobile alerts
- Multiple people need access
- You're managing more than one system

**Use standalone Agents only when:**

- You have strict air-gapped requirements
- You're testing on a single system
- Cloud connectivity is not possible

:::note

Without Netdata Cloud, each Agent operates independently - you'll need to check multiple dashboards, configure alerts on each system separately, and won't receive mobile notifications. Cloud connection is free and keeps your data on-premise while providing a unified view.

:::
