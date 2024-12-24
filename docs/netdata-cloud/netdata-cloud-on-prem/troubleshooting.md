# Netdata Cloud On-Prem Troubleshooting

Netdata Cloud On-Prem is an enterprise-grade monitoring solution that relies on several infrastructure components:

- Databases: PostgreSQL, Redis, Elasticsearch
- Message Brokers: Pulsar, EMQX
- Traffic Controllers: Ingress, Traefik
- Kubernetes Cluster

These components should be monitored and managed according to your organization's established practices and requirements.

## Common Issues

### Timeout During Installation

If your installation fails with this error:

```
Installing netdata-cloud-onprem (or netdata-cloud-dependency) helm chart...
[...]
Error: client rate limiter Wait returned an error:  Context deadline exceeded.
```

This error typically indicates **insufficient cluster resources**. Here's how to diagnose and resolve the issue.

#### Diagnosis Steps

> **Important**
>
> - For full installation: Ensure you're in the correct cluster context.
> - For Light PoC: SSH into the Ubuntu VM with `kubectl` pre-configured.
> - For Light PoC, always perform a complete uninstallation before attempting a new installation.

1. Check for pods stuck in Pending state:

   ```shell
   kubectl get pods -n netdata-cloud | grep -v Running
   ```

2. If you find Pending pods, examine the resource constraints:

   ```shell
   kubectl describe pod <POD_NAME> -n netdata-cloud
   ```

   Review the Events section at the bottom of the output. Look for messages about:
    - Insufficient CPU
    - Insufficient Memory
    - Node capacity issues

3. View overall cluster resources:

   ```shell
   # Check resource allocation across nodes
   kubectl top nodes
   
   # View detailed node capacity
   kubectl describe nodes | grep -A 5 "Allocated resources"
   ```

#### Solution

1. Compare your available resources against the [minimum requirements](https://github.com/netdata/netdata/blob/master/docs/netdata-cloud/netdata-cloud-on-prem/installation.md#system-requirements).
2. Take one of these actions:
    - Add more resources to your cluster.
    - Free up existing resources.

### Login Issues After Installation

Installation may complete successfully, but login issues can occur due to configuration mismatches. This table provides a quick reference for troubleshooting common login issues after installation.

| Issue                         | Symptoms                                                | Cause                                                                                                                         | Solution                                                                                                                          |
|-------------------------------|---------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------|
| SSO Login Failure             | Unable to authenticate via SSO providers                | - Invalid callback URLs<br/>- Expired/invalid SSO tokens<br/>- Untrusted certificates<br/>- Incorrect FQDN in `global.public` | - Update SSO configuration in `values.yaml`<br/>- Verify certificates are valid and trusted<br/>- Ensure FQDN matches certificate |
| MailCatcher Login (Light PoC) | - Magic links not arriving<br/>- "Invalid token" errors | - Incorrect hostname during installation<br/>- Modified default MailCatcher values                                            | - Reinstall with correct FQDN<br/>- Restore default MailCatcher settings<br/>- Ensure hostname matches certificate                |
| Custom Mail Server Login      | Magic links not arriving                                | - Incorrect SMTP configuration<br/>- Network connectivity issues                                                              | - Update SMTP settings in `values.yaml`<br/>- Verify network allows SMTP traffic<br/>- Check mail server logs                     |
| Invalid Token Error           | "Something went wrong - invalid token" message          | - Mismatched `netdata-cloud-common` secret<br/>- Database hash mismatch<br/>- Namespace change without secret migration       | - Migrate secret before namespace change<br/>- Perform fresh installation<br/>- Contact support for data recovery                 |

> **Warning**
>
> If you're modifying the installation namespace, the `netdata-cloud-common` secret will be recreated.
>
> **Before proceeding**: Back up the existing `netdata-cloud-common` secret. Alternatively, wipe the PostgreSQL database to prevent data conflicts.

### Slow Chart Loading or Chart Errors

When charts take a long time to load or fail with errors, the issue typically stems from data collection challenges. The `charts` service must gather data from multiple Agents within a Room, requiring successful responses from all queried Agents.

| Issue                | Symptoms                                                                                                        | Cause                                                                        | Solution                                                                                                                                                                                  |
|----------------------|-----------------------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Agent Connectivity   | - Queries stall or timeout<br/>- Inconsistent chart loading                                                     | Slow Agents or unreliable network connections prevent timely data collection | Deploy additional [Parent](/docs/observability-centralization-points/README.md) nodes to provide reliable backends. The system will automatically prefer these for queries when available |
| Kubernetes Resources | - Service throttling<br/>- Slow data processing<br/>- Delayed dashboard updates                                 | Resource saturation at the node level or restrictive container limits        | Review and adjust container resource limits and node capacity as needed                                                                                                                   |
| Database Performance | - Slow query responses<br/>- Increased latency across services                                                  | PostgreSQL performance bottlenecks                                           | Monitor and optimize database resource utilization:<br/>- CPU usage<br/>- Memory allocation<br/>- Disk I/O performance                                                                    |
| Message Broker       | - Delayed node status updates (online/offline/stale)<br/>- Slow alert transitions<br/>- Dashboard update delays | Message accumulation in Pulsar due to processing bottlenecks                 | - Review Pulsar configuration<br/>- Adjust microservice resource allocation<br/>- Monitor message processing rates                                                                        |

## Need Help?

If issues persist:

1. Gather the following information:

    - Installation logs
    - Your cluster specifications

2. Contact support at `support@netdata.cloud`.
