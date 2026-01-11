# 13. Alerts and Notifications Architecture

This chapter explains how Netdata's alerting system works under the hood, from evaluation to notification delivery.

## What You'll Find in This Chapter

| Section | What It Covers |
|---------|----------------|
| **13.1 Alert Lifecycle** | How alerts transition through states |
| **13.2 Configuration Layers** | Stock configs, user configs, and Cloud overrides |
| **13.3 Evaluation Architecture** | How and where alerts are evaluated |
| **13.4 Notification Dispatch** | How notifications are routed and delivered |
| **13.5 Scaling Topologies** | Single-node, streaming, and distributed setups |

## What's Next

- [13.1 Alert Lifecycle](alert-lifecycle.md) - State transitions
- [13.2 Configuration Layers](configuration-layers.md) - Configuration precedence
- [13.3 Evaluation Architecture](evaluation-architecture.md) - Evaluation process
- [13.4 Notification Dispatch](notification-dispatch.md) - Notification delivery
- [13.5 Scaling Topologies](scaling-topologies.md) - Multi-node architectures

## See Also

- [Alert Configuration Syntax](../alert-configuration-syntax/index.md) - Syntax reference
- [Receiving Notifications](../../receiving-notifications/index.md) - Notification configuration