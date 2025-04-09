# Best Practices for Observability Centralization Points

## Critical factors to consider

When setting up Observability Centralization Points, consider the following:

1. **System Volume**: The number of monitored systems impacts scaling. Larger infrastructures may need multiple centralization points to maintain performance.
2. **Data Transfer Costs**: In multi-cloud or hybrid environments, placing centralization points strategically reduces egress bandwidth costs.
3. **Usability Without Netdata Cloud**: Using fewer centralization points simplifies access and management when Netdata Cloud is not in use.
4. **Optimized Deployment with Netdata Cloud**: Netdata Cloud provides a complete infrastructure view, allowing you to optimize based on:
    - **Security** (internet access controls)
    - **Cost** (bandwidth and resource allocation)
    - **Operational needs** (regional, service, or team-based isolation)

## Cost Optimization Strategies

Netdata is designed to keep observability efficient and cost-effective. To manage costs:

- **Scale Out**: Use multiple smaller centralization points to improve efficiency and performance.
- **Use Existing Resources**: Leverage spare capacity before dedicating new resources to observability.
- **Centralized or Separate Logs & Metrics**: Choose whether to store logs and metrics together or separately based on access needs, retention policies, and compliance.
- **Flexible Configuration Management**: Each centralization point can have unique retention and alert settings, helping to control costs and tailor observability for different teams or services.

## Advantages of Netdata's Approach

Netdata provides several benefits over other observability solutions:

- **Scalability & Flexibility**: Multiple independent centralization points allow for customized observability by region, service, or team.
- **Resilience & Reliability**: Built-in replication ensures that observability continues even if a centralization point fails.
- **Optimized Cost & Performance**: Distributing workloads prevents bottlenecks and improves resource efficiency.
- **Ease of Use**: Netdata Agents require minimal setup and maintenance, reducing complexity.
- **On-Prem Control**: Centralization points remain on-prem even when using Netdata Cloud, keeping data within your infrastructure.
- **Comprehensive Observability**: Netdata enables deep visibility by segmenting infrastructure into independent observability points with tailored retention, alerts, and machine learning, while Netdata Cloud provides a unified view.

Following these best practices helps maintain a **cost-effective**, **high-performance** observability setup with Netdata.
