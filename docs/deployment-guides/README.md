# Deployment Guides

Get Netdata up and running in your infrastructure. Choose a deployment method that fits your needs.

## Quick Start

:::tip Getting Started

- **Testing Netdata?** → [Docker deployment](/packaging/docker/README.md) (2 minutes, easy cleanup)
- **Monitoring one server?** → [Standalone installation](/docs/deployment-guides/standalone-deployment.md) (1 minute, upgradeable)
- **Production ready?** → [Parent-Child setup](/docs/deployment-guides/deployment-with-centralization-points.md) (recommended)

:::

## Deployment Methods

### Standalone

Single Netdata Agent monitoring one system. Perfect for getting started or monitoring individual servers.

**Best for:** Testing or simple single-server monitoring
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
| **Testing/Development** | [Standalone](/docs/deployment-guides/standalone-deployment.md) or [Docker](/packaging/docker/README.md)                                                            | Quick setup, easy to remove                                         |
| **Single server**       | [Standalone](/docs/deployment-guides/standalone-deployment.md) (upgrade to [Parent-Child](/docs/deployment-guides/deployment-with-centralization-points.md) later) | Start simple, upgrade when ready for production                     |
| **Enterprise scale (1000+ nodes)** | [Parent-Child with clustering](/docs/deployment-guides/enterprise-deployment.md) | Multi-tier architecture, high availability, resource optimization |

:::warning Important Notes

- **Kubernetes**: Always use our Helm chart. Direct host installation won't have access to K8s API for pod metadata and service discovery.
- **Docker**: Requires specific privileges (SYS_PTRACE, SYS_ADMIN) and host mounts for full functionality. Rootless Docker has limited container network monitoring. Best for testing; production deployments should use host installation or Kubernetes Helm chart.
- **Production**: Parent-Child is recommended regardless of cluster size for better reliability and data persistence.
- **Enterprise Scale**: For deployments over 1000 nodes, see our [Enterprise Deployment Guide](/docs/deployment-guides/enterprise-deployment.md) for architecture patterns and resource planning.

:::

## Enterprise-Scale Deployments (1000+ Nodes)

Deploying Netdata at enterprise scale requires careful architecture planning and resource allocation. This section covers verified patterns and configurations for large-scale production environments.

### Quick Reference: Key Configuration Values

| Component | Parameter | Default | Production (500+ nodes) |
|-----------|-----------|---------|-------------------------|
| **Child Buffer** | `buffer size` | 10 MB | 20 MB |
| **Child Timeout** | `timeout` | 5m | 10m |
| **Parent Replication** | `replication threads` | CPUs/3 (min 4) | 32-64 |
| **Parent Replication** | `replication prefetch` | Auto | 100-200 |
| **Parent Replication** | `replication period` | 1d | 6h |
| **Parent Replication** | `replication step` | 1h | 5-10m |
| **Database Tier 0** | `retention time` | 14d | 3-7d |
| **Database Tier 0** | `retention size` | Auto | 100 GiB |

### Architecture Patterns

Netdata supports multiple streaming topologies for enterprise deployments:

#### Multi-Tier Parent Architecture

For large deployments, organize parents in clusters to distribute load and provide high availability:

```
Regional Parents (Tier 2)
    ↓
Local Parents (Tier 1)  
    ↓
Child Agents (Edge)
```

**Recommended cluster sizes:**
- 100-500 nodes: 2-3 parent nodes in active-active configuration
- 500-1000 nodes: 2 parent clusters (3 nodes each)
- 1000+ nodes: 3-4 parent clusters with geographic distribution

#### Active-Active Parent Clustering

Configure parents in a circular streaming topology where each parent streams to the others. This ensures all parents receive all data with no single point of failure:

```
Parent A ⟷ Parent B ⟷ Parent C
   ↑           ↑           ↑
Children    Children    Children
```

Children can connect to any parent, and failover happens automatically if one parent becomes unavailable.

#### Multi-Parent Failover

Configure children with multiple parent destinations for automatic failover. Children try parents in order and automatically reconnect with exponential backoff:

```ini
# In child's /etc/netdata/stream.conf
[stream]
    destination = parent1.example.com parent2.example.com parent3.example.com
```

### Kubernetes Child Pod Configuration

Child pods in Kubernetes deployments bind to `localhost:19999` only (not all interfaces) to avoid conflicts with any existing host-level Netdata agent:

```yaml
child:
  configs:
    netdata:
      data: |
        [web]
          bind to = localhost:19999  # Localhost only, no external access
```

This configuration means:
- No port conflict with host-level agent (which binds to `*:19999`)
- Child pod streams data to parent pod instead of serving dashboards
- Child pod dashboard not accessible from outside the node

**Note:** Running both a host-level Netdata agent and Kubernetes child pod on the same node will result in duplicate monitoring and wasted resources, even though there's no port conflict. Choose one approach per node.

### Resource Requirements

#### Memory per Parent Node

Memory requirements scale with the number of metrics and children connected:

**Per-child overhead:**
- Streaming buffer: 10-20 MB per child (default)
- Metrics registry: 150 bytes per metric
- ML models: ~18 KB per metric (if enabled)

**Database engine memory formula:**
```
Memory (KiB) = METRICS × (TIERS - 1) × 4 KiB × 2 + 32768 KiB
```

**Example calculations:**

| Metrics | Tiers | Estimated Memory |
|---------|-------|------------------|
| 100,000 | 3 | ~1.6 GB |
| 500,000 | 3 | ~8 GB |
| 1,000,000 | 3 | ~16 GB |
| 5,000,000 | 3 | ~80 GB |

#### CPU Requirements

CPU overhead scales with both child connections and metric throughput:

- Parent handling 100 children: ~2-4 cores
- Parent handling 500 children: ~8-12 cores  
- Parent handling 1M metrics/sec: ~10 cores

Monitor the `netdata.cpu` chart on parents to track actual overhead.

#### Disk I/O Patterns

The database engine uses append-only writes with batching:

- Standalone agent: ~5 KiB/s write
- Parent with 1M metrics/s: ~5 MB/s write, ~1 MB/s read (queries)
- Compression: LZ4 provides ~75% space savings

Use NVMe SSDs for parents handling more than 500 children for optimal performance.

### Configuration for Scale

#### Child Configuration Tuning

For large deployments, adjust these parameters in `/etc/netdata/stream.conf` on children:

```ini
[stream]
    enabled = yes
    destination = parent1:19999 parent2:19999 parent3:19999
    api key = YOUR-API-KEY
    buffer size = 20MiB          # Increase for high-latency networks
    timeout = 10m                # Longer timeout for large-scale deployments
    reconnect delay = 30s        # Prevent reconnection storms
    enable compression = yes
```

**Buffer sizing formula:**
```
buffer size = expected_latency_seconds × metrics_per_second × bytes_per_sample
```

Example: For 60s latency tolerance with 10,000 metrics/s, use at least 1 MB buffer (compressed).

#### Parent Configuration Tuning

Configure parents to handle hundreds of children in `/etc/netdata/stream.conf`:

```ini
[YOUR-API-KEY]
    type = api
    enabled = yes
    allow from = *
    db = dbengine
    health enabled = yes
    postpone alerts on connect = 5m     # Prevent alert storms on restart
    health log retention = 14d
    enable compression = yes
    enable replication = yes
    replication period = 6h              # Reduce for faster backfill
    replication step = 5m
```

#### Database Retention Tuning

Adjust retention in `/etc/netdata/netdata.conf` based on your scale:

```ini
[db]
    mode = dbengine
    storage tiers = 3                    # Consider reducing to 2 for 1000+ nodes
    
    # Tier 0 (1-second resolution)
    dbengine tier 0 retention size = 100GiB
    dbengine tier 0 retention time = 7d   # Reduce from default 14d for large scale
    
    # Tier 1 (1-minute resolution)  
    dbengine tier 1 retention size = 50GiB
    dbengine tier 1 retention time = 30d
    
    # Replication settings
    replication threads = 32              # Scale with CPU cores (formula: cores/3, min 4)
    replication prefetch = 100            # Increase for better parallelism
```

**For minimal child configuration** (offload processing to parent):

```ini
[db]
    mode = ram
    retention = 1200                      # 20 minutes in seconds (not entries)
[health]
    enabled = no
[ml]
    enabled = no
```

### High Availability Setup

#### Native Failover

Netdata provides built-in failover at the child level. Configure multiple parent destinations and children will automatically:

1. Try parents in the configured order
2. Temporarily block failed parents with randomized backoff
3. Reconnect automatically when parents recover
4. Maintain a minimum 5-second delay between reconnection attempts

No external load balancer required for basic failover.

#### External Load Balancing

For advanced load distribution, use HAProxy to distribute children across parent clusters:

```haproxy
frontend netdata_parents
    bind *:19999
    mode tcp
    default_backend netdata_parent_cluster

backend netdata_parent_cluster
    mode tcp
    balance leastconn
    server parent1 10.0.1.10:19999 check
    server parent2 10.0.1.11:19999 check
    server parent3 10.0.1.12:19999 check
```

### Performance Monitoring

Monitor these metrics on parent nodes to identify bottlenecks:

| Metric | Chart | Warning Threshold |
|--------|-------|-------------------|
| CPU Usage | `netdata.cpu` | >70% |
| Memory Usage | `netdata.memory` | >80% |
| Streaming Buffer | `netdata.streaming_buffer_usage` | >70% |
| Replication Lag | `netdata.replication_lag` | >300s |
| LibUV Workers | `netdata.libuv_workers` | >80% busy |
| DB Engine I/O | `netdata.dbengine_io` | >100 MB/s |

### Common Bottlenecks

**LibUV worker saturation:** Query latency increases when workers are fully utilized. Increase `libuv_worker_threads` in `netdata.conf` or add more parent nodes.

**Streaming buffer overflow:** Data gets dropped when buffers reach maximum capacity. Increase `buffer size` in child configuration or improve network bandwidth.

**Database memory pressure:** Frequent page evictions indicate insufficient memory. Allocate more RAM, reduce retention time, or reduce the number of tiers from 3 to 2.

### Deployment Verification

Use these commands to verify your deployment is working correctly:

**Check parent is receiving children:**
```bash
curl -s http://localhost:19999/api/v1/info | jq '.streaming.connected_children'
```

**Check child is connected:**
```bash
curl -s http://localhost:19999/api/v1/info | jq '.streaming.status'
```

**Monitor streaming buffer usage:**
```bash
curl -s http://localhost:19999/api/v1/data?chart=netdata.streaming_buffer_usage
```

**Check replication status:**
```bash
curl -s http://localhost:19999/api/v1/info | jq '.replication'
```

### Common Deployment Mistakes

**Running both OS agent and Kubernetes pod on same host:** This creates duplicate monitoring, wasted resources, and confusing dashboards. Choose one approach: use the Kubernetes Helm chart exclusively on K8s nodes (recommended), or run the OS agent on bare metal nodes only and exclude K8s nodes.

**Not increasing buffer size for high-latency networks:** Calculate buffer size based on expected latency using the formula: `buffer size = latency_seconds × metrics_per_second × bytes_per_sample`. Example: For 60s latency with 10K metrics/s, use at least 1-2 MB buffer.

**Using default retention on large parents:** Parent nodes can run out of disk space with default 14-day tier 0 retention. Reduce to 3-7 days for deployments with 1000+ nodes.
