# Observability Centralization Points (OCPs)

Netdata supports the creation of multiple independent **Observability Centralization Points**, aggregating metric samples, logs and metadata within an infrastructure.

> **Note**
>
> In our Documentation, an OCP is a node also referred to as a "Parent" that has "Children" nodes

OCPs are crucial for ensuring comprehensive monitoring and observability across an infrastructure, particularly under the following conditions:

1. **Ephemeral Systems**: For scenarios where Kubernetes nodes or ephemeral VMs that may not be persistently available, OCPs ensure that metrics and logs are not lost when these systems go offline. This is essential for maintaining historical data for analysis and troubleshooting.

2. **Resource Constraints**: In scenarios where the monitored systems lack sufficient resources (disk space or I/O bandwidth, CPU, RAM) to handle observability tasks effectively, OCPs offload these responsibilities, ensuring that production systems can operate efficiently without compromise.

3. **Multi-node Dashboards local dashboards**: For environments requiring aggregated views across multiple nodes locally, OCPs can aggregate this data to one dashboard.

4. **Netdata Cloud Access Restrictions**: In cases where monitored systems cannot connect to Netdata Cloud (due to a firewall policy), an OCP can serve as a bridge, aggregating data and interfacing with Netdata Cloud on behalf of these restricted systems.

When multiple independent OCPs are available:

- Netdata Cloud provides a unified infrastructure view by querying all points in parallel.
- OCPs without Cloud access provide consolidated views of their connected infrastructure's metrics and logs.

## Best Practices

When planning the deployment of OCPs on your infrastructure, the following factors need some consideration:

1. **Volume of Monitored Systems**: The number of systems being monitored dictates the scaling and number of OCPs required. Larger infrastructures may require multiple such nodes to manage the volume of data effectively and maintain performance.

2. **Cost of Data Transfer**: Particularly in multi-cloud or hybrid environments, the location of OCPs can significantly impact egress bandwidth costs. Strategically placing them in each data center or cloud region can minimize these costs by reducing the need for cross-network data transfer.
