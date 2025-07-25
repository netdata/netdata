# Parent Configuration Best Practices

## Critical Factors to Consider

When setting up Parents, consider the following:

| Factor                                      | Description                          | Impact                                                                                        |
|---------------------------------------------|--------------------------------------|-----------------------------------------------------------------------------------------------|
| **System Volume**                           | The number of monitored systems      | Larger infrastructures may need multiple Parents to maintain performance                      |
| **Data Transfer Costs**                     | Bandwidth usage between environments | Strategic placement reduces egress bandwidth costs in multi-cloud or hybrid environments      |
| **Usability Without Netdata Cloud**         | Standalone operation considerations  | Fewer Parents simplifies access and management                                                |
| **Optimized Deployment with Netdata Cloud** | Cloud integration benefits           | Provides complete infrastructure view with optimized security, cost, and operational controls |

<details>
<summary><strong>Click to see deployment optimization factors</strong></summary><br/>

```mermaid
flowchart TB
    A[A]
    B[B]
    C[C]
    D[D]
    B1[B1]
    C1[C1]
    D1[D1]
    A("**Optimized Deployment**<br/>with Netdata Cloud")
    B("Security")
    C("Cost")
    D("Operational Needs")
    B1("Internet access controls")
    C1("Bandwidth and<br/>resource allocation")
    D1("Regional, service, or<br/>team-based isolation")
    A --> B
    A --> C
    A --> D
    B --> B1
    C --> C1
    D --> D1
    classDef default fill: #f9f9f9, stroke: #333, stroke-width: 2px, color: #2c3e50, rx: 10, ry: 10
    classDef factors fill: #e8f5e8, stroke: #27ae60, stroke-width: 2px, color: #2c3e50, rx: 10, ry: 10
    class A default
    class B factors
    class C factors
    class D factors
    class B1 factors
    class C1 factors
    class D1 factors
```

</details><br/>

## Cost Optimization Strategies

Netdata helps you keep observability efficient and cost-effective:

| Strategy                                   | Description                            | Benefit                                                                                         |
|--------------------------------------------|----------------------------------------|-------------------------------------------------------------------------------------------------|
| **Scale Out**                              | Use multiple smaller Parents           | Improves efficiency and performance across distributed systems                                  |
| **Use Existing Resources**                 | Leverage spare capacity                | Minimize additional hardware costs by using available resources                                 |
| **Centralized or Separate Logs & Metrics** | Choose storage approach based on needs | Optimize based on access patterns, retention policies, and compliance requirements              |
| **Flexible Configuration Management**      | Customize each Parent                  | Control costs with unique retention and alert settings tailored for different teams or services |

<details>
<summary><strong>Click to see cost optimization strategies</strong></summary><br/>

```mermaid
flowchart TB
    A[A]
    B[B]
    C[C]
    D[D]
    E[E]
    B1[B1]
    C1[C1]
    D1[D1]
    E1[E1]
    A("**Cost Optimization**<br/>Strategies")
    B("Scale Out")
    C("Use Existing<br/>Resources")
    D("Centralized or<br/>Separate Logs & Metrics")
    E("Flexible<br/>Configuration Management")
    B1("Multiple smaller<br/>Parents")
    C1("Leverage spare capacity")
    D1("Based on access needs,<br/>retention policies,<br/>and compliance")
    E1("Unique settings for<br/>different teams or services")
    A --> B
    A --> C
    A --> D
    A --> E
    B --> B1
    C --> C1
    D --> D1
    E --> E1
    classDef default fill: #f9f9f9, stroke: #333, stroke-width: 2px, color: #2c3e50, rx: 10, ry: 10
    classDef strategies fill: #e8f5e8, stroke: #27ae60, stroke-width: 2px, color: #2c3e50, rx: 10, ry: 10
    class A default
    class B strategies
    class C strategies
    class D strategies
    class E strategies
    class B1 strategies
    class C1 strategies
    class D1 strategies
    class E1 strategies
```

</details><br/>

## Advantages of Netdata's Approach

Netdata provides several benefits over other observability solutions:

| Advantage                        | Description                                | Value                                                                 |
|----------------------------------|--------------------------------------------|-----------------------------------------------------------------------|
| **Scalability & Flexibility**    | Multiple independent Parents               | Customized observability by region, service, or team                  |
| **Resilience & Reliability**     | Built-in replication                       | Observability continues even if a Parent fails                        |
| **Optimized Cost & Performance** | Distributed workloads                      | Prevents bottlenecks and improves resource efficiency                 |
| **Ease of Use**                  | Minimal setup and maintenance              | Reduces complexity and operational overhead                           |
| **On-Prem Control**              | Data remains within your infrastructure    | Enhanced security and compliance, even when using Netdata Cloud       |
| **Comprehensive Observability**  | Segmented infrastructure with unified view | Deep visibility with tailored retention, alerts, and machine learning |

:::tip

Following these best practices helps you maintain a **cost-effective**, **high-performance** observability setup with Netdata.

:::
