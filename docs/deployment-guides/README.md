# Deployment Guides

Netdata can be used to monitor all kinds of infrastructure, from tiny stand-alone IoT devices to complex hybrid setups combining on-premise and cloud infrastructure, mixing bare-metal servers, virtual machines and containers.

There are 3 components to structure your Netdata ecosystem:

1. **Netdata Agents**

   To monitor the physical or virtual nodes of your infrastructure, including all applications and containers running on them.

   Netdata Agents are Open-Source, licensed under GPL v3+.

2. **Netdata Parents**

   To create [observability centralization points](/docs/observability-centralization-points/README.md) within your infrastructure, to offload Netdata Agents functions from your production systems, to provide high-availability of your data, increased data retention and isolation of your nodes.

   Netdata Parents are implemented using the Netdata Agent software. Any Netdata Agent can be an Agent for a node and a Parent  for other Agents, at the same time.

   It is recommended to set up multiple Netdata Parents. They will all seamlessly be integrated by Netdata Cloud into one monitoring solution.

3. **Netdata Cloud**

   Our SaaS, combining all your infrastructure, all your Netdata Agents and Parents, into one uniform, distributed, scalable, monitoring database, offering advanced data slicing and dicing capabilities, custom dashboards, advanced troubleshooting tools, user management, centralized management of alerts, and more.

The Netdata Agent is a highly modular software piece, providing data collection via numerous plugins, an in-house crafted time-series database, a query engine, health monitoring and alerts, machine learning and anomaly detection, metrics exporting to third party systems.
