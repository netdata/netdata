# 13.6 Scaling Alerting in Complex Topologies

Large deployments face different scaling challenges than single-node installations.

## 13.6.1 Agent-Only Topologies

In agent-only topologies, every node runs alert evaluation locally. This model is simple and scales horizontally by adding more nodes.

| Characteristic | Description |
|----------------|-------------|
| **Scalability** | Horizontal, add nodes to scale |
| **Complexity** | Minimal - single component |
| **Alert volume** | Proportional to node count |
| **Views** | Aggregate via Netdata Cloud |

Alert volume scales with node count. Use aggregate views in Netdata Cloud to monitor across nodes.

## 13.6.2 Parent-Child Topologies

Parent-based topologies centralize some alert evaluation on parent nodes. Children focus on collection and streaming; parents handle aggregation and evaluation.

| Node Type | Responsibilities |
|-----------|------------------|
| **Child** | Collect metrics, stream to parent |
| **Parent** | Aggregate, evaluate alerts, notify |
| **Both** | Can have local and aggregate alerts |

Design parent-based alerting to respect organizational boundaries. Alerts should fire on the node that owns the affected service.

## 13.6.3 Cloud Integration

Netdata Cloud integration adds remote components to the alerting architecture. Cloud receives alert events from Agents and Parents, providing unified views.

| Component | Purpose |
|-----------|---------|
| **Event collection** | Aggregate alerts from all nodes |
| **Unified views** | Single pane for all alerts |
| **Remote management** | Define alerts via Cloud UI |
| **Notification routing** | Cloud-managed delivery |

Cloud-based alerts can be defined and managed remotely through the Cloud UI.

## 13.6.4 Multi-Region Considerations

Deployments spanning multiple regions should consider alert evaluation latency. Design regional alerting to be self-contained.

| Challenge | Mitigation |
|-----------|------------|
| **Latency** | Evaluate alerts locally |
| **Cross-region dependencies** | Minimize or avoid |
| **Regional failures** | Independent regional alerting |
| **Data residency** | Regional Cloud endpoints |

## Related Sections

- [13.1 Evaluation Architecture](evaluation-architecture.md) - Alert evaluation process
- [13.2 Configuration Layers](configuration-layers.md) - How configurations work hierarchically
- [13.4 Notification Dispatch](notification-dispatch.md) - Delivery architecture