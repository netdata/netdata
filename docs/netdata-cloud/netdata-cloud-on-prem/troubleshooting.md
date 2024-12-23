# Netdata Cloud On-Prem Troubleshooting

Netdata Cloud On-Prem is an enterprise-grade monitoring solution that relies on several infrastructure components:

- Databases: PostgreSQL, Redis, Elasticsearch
- Message Brokers: Pulsar, EMQX
- Traffic Controllers: Ingress, Traefik
- Kubernetes Cluster

These components should be monitored and managed according to your organization's established practices and requirements.

## Common Issues

### Installation cannot finish

If you are getting error like:

```
Installing netdata-cloud-onprem (or netdata-cloud-dependency) helm chart...
[...]
Error: client rate limiter Wait returned an error:  Context deadline exceeded.
```

There are probably not enough resources available. Fortunately, it is very easy to verify with the `kubectl` utility. In the case of a full installation, switch the context to the cluster where On-Prem is being installed. For the Light PoC installation, SSH into the Ubuntu VM where `kubectl` is already installed and configured.

To verify check if there are any `Pending` pods:

```shell
kubectl get pods -n netdata-cloud | grep -v Running
```

To check which resource is a limiting factor pick one of the `Pending` pods and issue command:

```shell
kubectl describe pod <POD_NAME> -n netdata-cloud
```

At the end in an `Events` section information about insufficient `CPU` or `Memory` on available nodes should appear.
Please check the minimum requirements for your on-prem installation type or contact our support - `support@netdata.cloud`.

> **Warning**
>
> In case of the Light PoC installations always uninstall before the next attempt.

### Installation finished but login does not work

It depends on the installation and login type, but the underlying problem is usually located in the `values.yaml` file. In the case of Light PoC installations, this is also true, but the installation script fills in the data for the user. We can split the problem into two variants:

1. SSO is not working - you need to check your tokens and callback URLs for a given provider. Equally important is the certificate - it needs to be trusted, and also hostname(s) under `global.public` section - make sure that FQDN is correct.
2. Mail login is not working:
   1. If you are using a Light PoC installation with MailCatcher, the problem usually appears if the wrong hostname was used during the installation. It needs to be a FQDN that matches the provided certificate. The usual error in such a case points to a invalid token.
   2. If the magic link is not arriving for MailCatcher, it's likely because the default values were changed. In the case of using your own mail server, check the `values.yaml` file in the `global.mail.smtp` section and your network settings.

If you are getting the error `Something went wrong - invalid token` and you are sure that it is not related to the hostname or the mail configuration as described above, it might be related to a dirty state of Netdata secrets. During the installation, a secret called `netdata-cloud-common` is created. By default, this secret should not be deleted by Helm and is created only if it does not exist. It stores a few strings that are mandatory for Netdata Cloud On-Prem's provisioning and continuous operation. Because they are used to hash the data in the PostgreSQL database, a mismatch will cause data corruption where the old data is not readable and the new data is hashed with the wrong string. Either a new installation is needed, or contact to our support to individually analyze the complexity of the problem.

> **Warning**
>
> If you are changing the installation namespace secret netdata-cloud-common will be created again. Make sure to transfer it beforehand or wipe postgres before new installation.

### Slow Chart Loading or Chart Errors

When charts take a long time to load or fail with errors, the issue typically stems from data collection challenges. The `charts` service must gather data from multiple Agents within a Room, requiring successful responses from all queried Agents.

| Issue                | Symptoms                                                                                                        | Cause                                                                        | Solution                                                                                                                                                                                  |
| -------------------- | --------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Agent Connectivity   | - Queries stall or timeout<br/>- Inconsistent chart loading                                                     | Slow Agents or unreliable network connections prevent timely data collection | Deploy additional [Parent](/docs/observability-centralization-points/README.md) nodes to provide reliable backends. The system will automatically prefer these for queries when available |
| Kubernetes Resources | - Service throttling<br/>- Slow data processing<br/>- Delayed dashboard updates                                 | Resource saturation at the node level or restrictive container limits        | Review and adjust container resource limits and node capacity as needed                                                                                                                   |
| Database Performance | - Slow query responses<br/>- Increased latency across services                                                  | PostgreSQL performance bottlenecks                                           | Monitor and optimize database resource utilization:<br/>- CPU usage<br/>- Memory allocation<br/>- Disk I/O performance                                                                    |
| Message Broker       | - Delayed node status updates (online/offline/stale)<br/>- Slow alert transitions<br/>- Dashboard update delays | Message accumulation in Pulsar due to processing bottlenecks                 | - Review Pulsar configuration<br/>- Adjust microservice resource allocation<br/>- Monitor message processing rates                                                                        |
