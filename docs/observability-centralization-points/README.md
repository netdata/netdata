# Observability Centralization Points

Netdata supports the creation of multiple independent **Observability Centralization Points**, aggregating metric samples, logs and metadata within an infrastructure.

Observability Centralization Points are crucial for ensuring comprehensive monitoring and observability across an infrastructure, particularly under the following conditions:

1. **Ephemeral Systems**: For systems like Kubernetes nodes or ephemeral VMs that may not be persistently available, centralization points ensure that metrics and logs are not lost when these systems go offline. This is essential for maintaining historical data for analysis and troubleshooting.

2. **Resource Constraints**: In scenarios where the monitored systems lack sufficient resources (disk space or I/O bandwidth, CPU, RAM) to handle observability tasks effectively, centralization points offload these responsibilities, ensuring that production systems can operate efficiently without compromise.

3. **Multi-node Dashboards without Netdata Cloud**: For environments requiring aggregated views across multiple nodes but without the use of Netdata Cloud, Netdata Parents can aggregate this data to provide comprehensive dashboards, similar to what Netdata Cloud offers.

4. **Netdata Cloud Access Restrictions**: In cases where monitored systems cannot connect to Netdata Cloud (due to a firewall policy), a Netdata Parent can serve as a bridge, aggregating data and interfacing with Netdata Cloud on behalf of these restricted systems.

When multiple independent centralization points are available:

- Netdata Cloud provides a unified infrastructure view by querying all points in parallel.
- Parent nodes without Cloud access provide consolidated views of their connected infrastructure's metrics and logs.
