# Netdata Streaming Routing Architecture

## Overview

Netdata's streaming architecture implements an intelligent routing algorithm designed to optimize data consistency, computational load distribution, and fault tolerance in distributed monitoring environments. This document describes the technical implementation and operational characteristics of the streaming topology.

## Architecture Principles

### Core Design Goals

1. **Data Consistency**: Ensures complete metric collection and historical data preservation
2. **Load Distribution**: Balances computational workload across infrastructure nodes
3. **Fault Tolerance**: Maintains monitoring continuity during network disruptions
4. **Scalability**: Supports hierarchical deployments with automatic load balancing

### Routing Algorithm

The parent selection algorithm operates on the following priorities:

1. **Data Completeness Assessment**: Evaluates potential parents based on existing historical data coverage
2. **Load Distribution**: Implements randomized selection among equally qualified parents
3. **Failover Mechanism**: Maintains round-robin connection lists for rapid recovery

## Technical Implementation

### Parent Selection Logic

```
Priority Order:
1. Evaluate data completeness across available parents
2. Apply randomization for load distribution
3. Execute failover to first available parent during outages
```

### Workload Distribution Strategies

#### Machine Learning Processing

The system optimizes ML workload based on node configuration:

- **Child ML Disabled**: Parent nodes train models from scratch, requiring significant computational resources
- **Child ML Enabled**: Models trained locally and transferred to parents with pre-computed anomaly detection

#### Streaming Load Balance

Multi-tier deployments distribute processing load through:

- **Reception Layer**: Lightweight data ingestion
- **Proxy Layer**: Data forwarding with computational overhead
- **Storage Layer**: Final data persistence and analysis

### Multi-Tier Deployment Model

```
Child Nodes ──→ Parent Proxies ──→ Ultimate Parents
    │                 │                    │
    ├─ Data Collection├─ Data Relay       ├─ Data Storage
    └─ ML Training    └─ Load Balance     └─ Analysis
```

## Query Optimization

Netdata Cloud implements dual routing strategies based on query type:

### Historical Data Queries

- **Target**: Furthest parent in hierarchy
- **Benefit**: Automatic load distribution
- **Result**: Leverages computational resources at higher tiers

### Live Function Execution

- **Target**: Closest available node
- **Benefit**: Minimized latency
- **Result**: Real-time data extraction from source systems

## Operational Characteristics

### Network Topology Behavior

The routing algorithm prioritizes operational objectives over traditional network metrics:

| Traditional Networks | Netdata Streaming |
|---------------------|-------------------|
| Latency optimization | Data integrity |
| Geographic proximity | Historical completeness |
| Static routes | Dynamic load balancing |
| Path efficiency | Fault resilience |

### Failover Patterns

During network disruptions, the system may establish connections across geographically distant nodes. This behavior ensures:

- Continuous metric collection
- Preserved ML model accuracy
- Maintained historical context
- Prevented parent overload during recovery

## Deployment Considerations

### Infrastructure Planning

- Size parent nodes based on ML training requirements
- Design for regional failure scenarios
- Implement hierarchical structures for query optimization
- Consider computational distribution over network proximity

### Monitoring and Operations

- Monitor ML training distribution metrics
- Track data completeness across parents
- Analyze failover event patterns
- Leverage automatic query routing for performance

## Performance Characteristics

### Benefits

1. **Data Integrity**: Zero metric loss during complex failure scenarios
2. **Computational Efficiency**: Distributed processing prevents bottlenecks
3. **Automatic Optimization**: Self-balancing without manual intervention
4. **Resilient Architecture**: Maintains functionality during partial outages

### Trade-offs

The architecture optimizes for monitoring reliability over network efficiency. This design choice ensures:

- Complete data collection under adverse conditions
- Consistent ML model accuracy
- Scalable computational distribution
- Automatic recovery from failures

## Conclusion

Netdata's streaming routing architecture implements a data-centric approach to distributed monitoring. By prioritizing consistency, load distribution, and fault tolerance, the system delivers reliable observability infrastructure that automatically adapts to changing conditions while maintaining operational efficiency.