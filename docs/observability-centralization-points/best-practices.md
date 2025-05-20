# Best Practices for Observability Centralization Points

## Critical factors to consider

When setting up Observability Centralization Points, consider the following:

| Factor                                      | Description                          | Impact                                                                                        |
|---------------------------------------------|--------------------------------------|-----------------------------------------------------------------------------------------------|
| **System Volume**                           | The number of monitored systems      | Larger infrastructures may need multiple centralization points to maintain performance        |
| **Data Transfer Costs**                     | Bandwidth usage between environments | Strategic placement reduces egress bandwidth costs in multi-cloud or hybrid environments      |
| **Usability Without Netdata Cloud**         | Standalone operation considerations  | Fewer centralization points simplifies access and management                                  |
| **Optimized Deployment with Netdata Cloud** | Cloud integration benefits           | Provides complete infrastructure view with optimized security, cost, and operational controls |

```mermaid
graph TD
    A[Optimized Deployment<br>with Netdata Cloud] --> B[Security]
    A --> C[Cost]
    A --> D[Operational Needs]
    
    B --> B1[Internet access controls]
    C --> C1[Bandwidth and<br>resource allocation]
    D --> D1[Regional, service, or<br>team-based isolation]
    
classDef default fill:#f9f9f9,stroke:#333,stroke-width:1px,color:#333;
classDef green fill:#4caf50,stroke:#333,stroke-width:1px,color:black;
class A default;
class B,C,D,B1,C1,D1 green;
```

## Cost Optimization Strategies

Netdata is designed to keep observability efficient and cost-effective. To manage costs:

| Strategy                                   | Description                                | Benefit                                                                                         |
|--------------------------------------------|--------------------------------------------|-------------------------------------------------------------------------------------------------|
| **Scale Out**                              | Use multiple smaller centralization points | Improves efficiency and performance across distributed systems                                  |
| **Use Existing Resources**                 | Leverage spare capacity                    | Minimize additional hardware costs by using available resources                                 |
| **Centralized or Separate Logs & Metrics** | Choose storage approach based on needs     | Optimize based on access patterns, retention policies, and compliance requirements              |
| **Flexible Configuration Management**      | Customize each centralization point        | Control costs with unique retention and alert settings tailored for different teams or services |

```mermaid
graph TD
    A[Cost Optimization<br>Strategies] --> B[Scale Out]
    A --> C[Use Existing<br>Resources]
    A --> D[Centralized or<br>Separate Logs & Metrics]
    A --> E[Flexible<br>Configuration Management]
    
    B --> B1[Multiple smaller<br>centralization points]
    C --> C1[Leverage spare capacity]
    D --> D1[Based on access needs,<br>retention policies,<br>and compliance]
    E --> E1[Unique settings for<br>different teams or services]
    
classDef default fill:#f9f9f9,stroke:#333,stroke-width:1px,color:#333;
classDef green fill:#4caf50,stroke:#333,stroke-width:1px,color:black;
class A default;
class B,C,D,E,B1,C1,D1,E1 green;
```

## Advantages of Netdata's Approach

Netdata provides several benefits over other observability solutions:

| Advantage                        | Description                                | Value                                                                 |
|----------------------------------|--------------------------------------------|-----------------------------------------------------------------------|
| **Scalability & Flexibility**    | Multiple independent centralization points | Customized observability by region, service, or team                  |
| **Resilience & Reliability**     | Built-in replication                       | Observability continues even if a centralization point fails          |
| **Optimized Cost & Performance** | Distributed workloads                      | Prevents bottlenecks and improves resource efficiency                 |
| **Ease of Use**                  | Minimal setup and maintenance              | Reduces complexity and operational overhead                           |
| **On-Prem Control**              | Data remains within your infrastructure    | Enhanced security and compliance, even when using Netdata Cloud       |
| **Comprehensive Observability**  | Segmented infrastructure with unified view | Deep visibility with tailored retention, alerts, and machine learning |

```mermaid
graph TD
    A[Advantages of<br>Netdata's Approach] --> B[Scalability & Flexibility]
    A --> C[Resilience & Reliability]
    A --> D[Optimized Cost &<br>Performance]
    A --> E[Ease of Use]
    A --> F[On-Prem Control]
    A --> G[Comprehensive<br>Observability]
    
    B --> B1[Customized observability<br>by region, service, or team]
    C --> C1[Observability continues<br> even if a centralization<br> point fails]
    D --> D1[Prevents bottlenecks<br>and improves<br>resource efficiency]
    E --> E1[Minimal setup and maintenance]
    F --> F1[Data remains within<br>your infrastructure]
    G --> G1[Unified view with<br>tailored segments]
    
classDef default fill:#f9f9f9,stroke:#333,stroke-width:1px,color:#333;
classDef green fill:#4caf50,stroke:#333,stroke-width:1px,color:black;
class A default;
class B,C,D,E,F,G,B1,C1,D1,E1,F1,G1 green;
```

Following these best practices helps you maintain a **cost-effective**, **high-performance** observability setup with Netdata.
