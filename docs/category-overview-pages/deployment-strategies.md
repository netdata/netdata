# Netdata Deployment Strategies

Netdata can be used to monitor all kinds of infrastructure, from stand-alone tiny IoT devices to complex hybrid setups 
combining on-premise and cloud infrastructure, mixing bare-metal servers, virtual machines and containers.

There are 3 components to structure your Netdata ecosystem:

1. **Netdata Agents**
   To monitor the physical or virtual nodes of your infrastructure, including all applications and containers running on them.
   
   Netdata Agents are Open-Source, licensed under GPL v3+.

2. **Netdata Parents**
   To create data centralization points within your infrastructure, to offload Netdata Agents functions from your production 
   systems, to provide high-availability of your data, increased data retention and isolation of your nodes. 
   
   Netdata Parents are implemented using the Netdata Agent software. Any Netdata Agent can be an Agent for a node and a Parent 
   for other Agents, at the same time.

   It is recommended to set up multiple Netdata Parents. They will all seamlessly be integrated by Netdata Cloud into one monitoring solution.


3. **Netdata Cloud**
   Our SaaS, combining all your infrastructure, all your Netdata Agents and Parents, into one uniform, distributed, infinitely 
   scalable, monitoring database, offering advanced data slicing and dicing capabilities, custom dashboards, advanced troubleshooting 
   tools, user management, centralized management of alerts, and more.


The Netdata Agent is a highly modular software piece, providing data collection via numerous plugins, an in-house crafted time-series 
database, a query engine, health monitoring and alerts, machine learning and anomaly detection, metrics exporting to third party systems.


To help our users have a complete experience of Netdata when they install it for the first time, a Netdata Agent with default configuration 
is a complete monitoring solution out of the box, having all these features enabled and available.

We suggest to do the following configuration changes for production deployments:

1. [Downsize Netdata Agents running on weak IoT devices](https://github.com/netdata/netdata/edit/master/docs/guides/configure/performance.md)
   To save valuable system resources.

2. Offload Netdata Agents running on production systems, using Netdata Parents to take over most of their functions
   To free system resources for the production applications running on them and to isolate production systems from the rest 
   of the world, while at the same time providing increased data retention, data high availability and security.
   
   See [streaming and replication](https://github.com/netdata/netdata/blob/master/streaming/README.md)

3. Use Netdata Cloud to access the dashboards
   For increased security, user management and access to our latest tools for advanced dashboarding and troubleshooting.
