# Netdata Cloud On-Prem Troubleshooting

Netdata Cloud On-Prem is an enterprise-grade monitoring solution that relies on several infrastructure components:

- Databases: PostgreSQL, Redis, Elasticsearch
- Message Brokers: Pulsar, EMQX
- Traffic Controllers: Ingress, Traefik
- Kubernetes Cluster

These components should be monitored and managed according to your organization's established practices and requirements.

## Common Issues

### Slow Chart Loading or Chart Errors

When charts take a long time to load or fail with errors, the issue typically stems from data collection challenges. The `charts` service must gather data from multiple Agents within a Room, requiring successful responses from all queried Agents.

| Issue                | Symptoms                                                                                                        | Cause                                                                        | Solution                                                                                                                                                                                  |
|----------------------|-----------------------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Agent Connectivity   | - Queries stall or timeout<br/>- Inconsistent chart loading                                                     | Slow Agents or unreliable network connections prevent timely data collection | Deploy additional [Parent](/docs/observability-centralization-points/README.md) nodes to provide reliable backends. The system will automatically prefer these for queries when available |
| Kubernetes Resources | - Service throttling<br/>- Slow data processing<br/>- Delayed dashboard updates                                 | Resource saturation at the node level or restrictive container limits        | Review and adjust container resource limits and node capacity as needed                                                                                                                   |
| Database Performance | - Slow query responses<br/>- Increased latency across services                                                  | PostgreSQL performance bottlenecks                                           | Monitor and optimize database resource utilization:<br/>- CPU usage<br/>- Memory allocation<br/>- Disk I/O performance                                                                    |
| Message Broker       | - Delayed node status updates (online/offline/stale)<br/>- Slow alert transitions<br/>- Dashboard update delays | Message accumulation in Pulsar due to processing bottlenecks                 | - Review Pulsar configuration<br/>- Adjust microservice resource allocation<br/>- Monitor message processing rates                                                                        |
