# Deployment Guides

Get Netdata up and running in your infrastructure. Choose a deployment method that fits your needs.

## Quick Start

:::tip Getting Started

- **Evaluating Netdata?** → [Docker deployment](/packaging/docker/README.md) (2 minutes)
- **Starting with one system?** → [Standalone installation](/docs/deployment-guides/standalone-deployment.md) (1 minute, upgradeable)
- **Production ready?** → [Parent-Child setup](/docs/deployment-guides/deployment-with-centralization-points.md) (recommended)

:::

## Deployment Methods

### Standalone

A Netdata Agent that collects, stores, alerts and serves dashboards on its own, with or without a Parent. Every Agent is a complete observability node.

**Best for:** Evaluation, single systems, isolated sites, or any node that must keep working on its own

**Setup time:** < 1 minute

[→ Deploy Standalone Agent](/docs/deployment-guides/standalone-deployment.md)

### Parent-Child Streaming (Recommended)

The recommended production setup. Stream metrics from Child Agents to centralized Parent nodes for better data persistence and resource optimization.

**Best for:** Production environments of any size, high availability requirements

**Setup time:** 10-15 minutes

[→ Deploy Parent-Child Setup](/docs/deployment-guides/deployment-with-centralization-points.md)

### Kubernetes

Deploy Netdata across your Kubernetes clusters with our Helm chart. Required for proper Kubernetes monitoring.

**Best for:** Kubernetes environments (required for full K8s observability)

**Setup time:** 5-10 minutes

[→ Deploy on Kubernetes](https://github.com/netdata/helmchart#netdata-helm-chart-for-kubernetes-deployments)

### Docker

Run Netdata in containers for quick testing. Note: Some features are limited compared to host installation.

**Best for:** Quick testing, ephemeral environments

**Setup time:** 2-5 minutes

[→ Deploy with Docker](/packaging/docker/README.md)

## Which Deployment Should I Choose?

| Environment             | Recommended Method                                                                                                                                                 | Why                                                                 |
|-------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------------------|
| **Production servers**  | [Parent-Child](/docs/deployment-guides/deployment-with-centralization-points.md)                                                                                   | Best data persistence, resource optimization, and high availability |
| **Kubernetes**          | [Helm Chart](https://github.com/netdata/helmchart#netdata-helm-chart-for-kubernetes-deployments)                                                                   | Required for K8s API access and pod metadata collection             |
| **Testing/Development** | [Standalone](/docs/deployment-guides/standalone-deployment.md) or [Docker](/packaging/docker/README.md)                                                            | Quick to set up and remove                                          |
| **Single server**       | [Standalone](/docs/deployment-guides/standalone-deployment.md) (upgrade to [Parent-Child](/docs/deployment-guides/deployment-with-centralization-points.md) later) | Fully functional on its own; add a Parent for centralization        |

:::warning Important Notes

- **Kubernetes**: Always use our Helm chart. Direct host installation won't have access to K8s API for pod metadata and service discovery.
- **Docker**: Limited feature set compared to host installation. Best for testing, not recommended for production.
- **Production**: Parent-Child is recommended regardless of cluster size for better reliability and data persistence.

:::
