# Deployment Guides

Netdata provides real-time monitoring for various infrastructure types, from small IoT devices to complex hybrid environments that combine on-premise and cloud infrastructure. It supports bare-metal servers, virtual machines, and containers.

## Core Components of a Netdata Deployment

A Netdata deployment consists of three main components:

### 1. Netdata Agents

Netdata Agents collect real-time metrics from your infrastructure's physical or virtual nodes, including applications and containers running on them. They are open-source and licensed under GPL v3+.

### 2. Netdata Parents

Netdata Parents serve as central aggregation points for monitoring data. They help reduce the resource load on individual Netdata Agents, provide high availability for collected metrics, extend data retention, and enable better isolation of monitored nodes.

- Netdata Parents are built using the same Netdata Agent software.
- Any Netdata Agent can function as both an Agent for a node and a Parent for other Agents.
- Deploying multiple Netdata Parents ensures redundancy and seamless integration with Netdata Cloud.

### 3. Netdata Cloud

Netdata Cloud is a SaaS platform that unifies all Netdata Agents and Parents into a distributed, scalable monitoring solution. It provides:

- Centralized infrastructure monitoring
- Advanced data analysis and visualization tools
- Customizable dashboards
- User management features
- Alerting and anomaly detection capabilities

## Key Features of Netdata Agents

Netdata Agents offer a modular monitoring solution with capabilities that include:

- Extensive data collection through built-in plugins
- A high-performance time-series database optimized for real-time analytics
- A query engine for flexible data retrieval
- Integrated health monitoring and alerting
- Machine learning-based anomaly detection
- Exporting of metrics to third-party systems

This structured deployment allows for scalable, efficient monitoring of any infrastructure, ensuring optimal performance and proactive issue resolution.
