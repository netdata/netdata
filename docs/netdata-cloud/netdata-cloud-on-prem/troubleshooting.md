# Netdata Cloud On-Prem Troubleshooting

Netdata Cloud On-Prem is a sophisticated piece of software relying on in multiple infrastructure components for its operation.

We assume that your team already manages and monitors properly the components the On-Prem Cloud (OPC) depends upon, like the PostgreSQL, Redis and Elasticsearch databases, the Pulsar and EMQX message brokers, the traffic controllers (Ingress and Traefik) and of course the health of the Kubernetes cluster itself.

The following are questions that are usually asked by OPC operators.

## Loading charts takes a long time or ends with an error

The charts service is trying to collect data from the Agents involved in the query. In most of the cases, this microservice queries many Agents (depending on the Room), and all of them have to reply for the query to be satisfied.

One or more of the following may be the cause:

1. **Slow Agent or Agents with unreliable connections**

   If any of the queried Agents is slow or has an unreliable network connection, the query will stall and the OPC will have timeout before responding.

   When Agents are overloaded or have unreliable connections, we suggest to install more [Parents](/docs/observability-centralization-points/README.md) for providing reliable backends to the OPC. They will automatically be preferred for all queries, when available.

2. **Poor Kubernetes cluster management**

   Another common issue is poor management of the Kubernetes cluster. When a node of a Kubernetes cluster is saturated, or the limits set to its containers are small, the OPC's microservices get throttled by Kubernetes and does not get the resources required to process the responses of the Agents and aggregate the results for the dashboard.

   We recommend to review the throttling of the containers and increase the limits if required.

3. **Saturated Database**

   Slow responses may also indicate performance issues at the PostgreSQL database.

   Please review the resources utilization of the database server (CPU, Memory, and Disk I/O) and take action to improve the situation.

4. **Messages pilling up in Pulsar**

   Depending on the size of the infrastructure being monitored and the resources allocated to Pulsar and the microservices, messages may be pilling up. When this happens you may also experience that nodes status updates (online, offline, stale) are slow, or alerts transitions take time to appear on the dashboard.

   We recommend to review Pulsar configuration and the resources allocated of the microservices, to ensure that there is no saturation.
