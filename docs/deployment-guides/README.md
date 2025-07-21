# Deployment Guides

Netdata monitors everything from tiny IoT devices to massive cloud infrastructures. Deploy it on bare metal, VMs, or containers - it just works.

## Overview

Think of Netdata's individual components as three LEGO blocks that come together to create a robust architecture design of complete observability for your infrastructure.

## Components

### Netdata Agents
The foundation of every deployment. Install an Agent on each system you want to monitor.

**What they do:**
- Collect real-time metrics from systems, applications, and containers
- Store metrics locally with configurable retention
- Serve their own dashboard and API
- Run health checks and trigger alerts

**Key point:** Install an Agent, get instant monitoring. No configuration required.

### Netdata Parents
Scale your monitoring by designating some Agents as "Parents" that collect data from other Agents ("Children").

**Benefits:**
- **Resource optimization** - Children use minimal CPU and RAM
- **Data persistence** - Metrics survive even when systems are terminated
- **Centralized storage** - Organize data by region, environment, or team
- **High availability** - Easy redundancy with multiple Parents

**Key point:** A Parent is just a regular Netdata Agent with streaming enabled. Any Agent can become a Parent.

### Netdata Cloud
Unifies your entire infrastructure into a single interface. Optional but powerful.

**Features:**
- Unified dashboards across all Agents and Parents
- Mobile app alerts and notifications
- Team collaboration and user management
- Custom dashboards and data analysis
- Infrastructure overview and insights

**Key point:** Your data stays on your servers. Cloud provides the viewing layer.

## How They Work Together

```
Agents (collect) → Parents (centralize) → Cloud (unify)
```

:::info

Each component builds on the previous one. Start with Agents for immediate monitoring. Add Parents when you need centralization. Connect to Cloud when you want unified visibility.

:::