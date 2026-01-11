# 13.6 Scaling Alerting in Complex Topologies

Large deployments face different scaling challenges than single-node installations.

## Agent-Only Topologies

In agent-only topologies, every node runs alert evaluation locally. This model is simple and scales horizontally by adding more nodes.

Alert volume scales with node count. Use aggregate views in Netdata Cloud to monitor across nodes.

## Parent-Child Topologies

Parent-based topologies centralize some alert evaluation on parent nodes. Children focus on collection and streaming; parents handle aggregation and evaluation.

Design parent-based alerting to respect organizational boundaries. Alerts should fire on the node that owns the affected service.

## Cloud Integration

Netdata Cloud integration adds remote components to the alerting architecture. Cloud receives alert events from Agents and Parents, providing unified views.

Cloud-based alerts can be defined and managed remotely through the Cloud UI.

## Multi-Region Considerations

Deployments spanning multiple regions should consider alert evaluation latency. Design regional alerting to be self-contained.