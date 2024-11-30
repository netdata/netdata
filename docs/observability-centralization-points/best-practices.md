# Best Practices for Observability Centralization Points

When planning the deployment of Observability Centralization Points, the following factors need consideration:

1. **Volume of Monitored Systems**: The number of systems being monitored dictates the scaling and number of centralization points required. Larger infrastructures may require multiple centralization points to manage the volume of data effectively and maintain performance.

2. **Cost of Data Transfer**: Particularly in multi-cloud or hybrid environments, the location of centralization points can significantly impact egress bandwidth costs. Strategically placing centralization points in each data center or cloud region can minimize these costs by reducing the need for cross-network data transfer.

3. **Usability without Netdata Cloud**: When not using Netdata Cloud, observability with Netdata is simpler when there are fewer centralization points, making it easier to remember where observability is and how to access it.

4. Netdata Cloud provides infrastructure-wide views regardless of centralization points, allowing you to optimize your setup based on:
    - Security requirements (such as internet access controls)
    - Cost management (including bandwidth and resource allocation)
    - Operational needs (like regional, service, or team isolation)

## Cost Optimization

Netdata has been designed for observability cost optimization. For optimal cost, we recommend using Netdata Cloud and multiple independent observability centralization points:

- **Scale out**: add more, smaller centralization points to distribute the load. This strategy provides the least resource consumption per unit of workload, maintaining optimal performance and resource efficiency across your observability infrastructure.

- **Use existing infrastructure resources**: use spare capacities before allocating dedicated resources for observability. This approach minimizes additional costs and promotes an economically sustainable observability framework.

- **Unified or separate centralization for logs and metrics**: Netdata allows centralizing metrics and logs together or separately. Consider factors such as access frequency, data retention policies, and compliance requirements to enhance performance and reduce costs.

- **Decentralized configuration management**: each Netdata centralization point can have its own unique configuration for retention and alerts. This enables:
    - Finer control on infrastructure costs
    - Localized control for separate services or teams

## Pros and Cons

Compared to other observability solutions, the design of Netdata offers:

- **Enhanced Scalability and Flexibility**: Netdata's support for multiple independent observability centralization points allows for a more scalable and flexible architecture. This feature is particularly helpful in distributed and complex environments, enabling tailored observability strategies that can vary by region, service, or team requirements.

- **Resilience and Fault Tolerance**: The ability to deploy multiple centralization points also contributes to greater system resilience and fault tolerance. Replication is a native feature of Netdata centralization points, so in the event of a failure at one centralization point, others can continue to function, ensuring continuous observability.

- **Optimized Cost and Performance**: By distributing the load across multiple centralization points, Netdata can optimize both performance and cost. This distribution allows for the efficient use of resources and help mitigate the bottlenecks associated with a single centralization point.

- **Simplicity**: Netdata Agents (Children and Parents) require minimal configuration and maintenance, usually less than the configuration and maintenance required for the Agents and exporters of other monitoring solutions. This provides an observability pipeline that has less moving parts and is easier to manage and maintain.

- **Always On-Prem**: Netdata centralization points are always on-prem. Even when Netdata Cloud is used, Netdata Agents and parents are queried to provide the data required for the dashboards.

- **Bottom-Up Observability**: Netdata is designed to monitor systems, containers and applications bottom-up, aiming to provide the maximum resolution, visibility, depth and insights possible. Its ability to segment the infrastructure into multiple independent observability centralization points with customized retention, machine learning and alerts on each of them, while providing unified infrastructure level dashboards at Netdata Cloud, provides a flexible environment that can be tailored per service or team, while still being one unified infrastructure.
