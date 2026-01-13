# 13.2 Configuration Layers

Netdata supports multiple configuration layers for health alerts. Understanding precedence rules helps in making modifications that take effect as intended.

:::important

Stock configuration files in `/usr/lib/netdata/conf.d/health.d/` are overwritten during upgrades. Always place custom alerts in `/etc/netdata/health.d/`.

:::

## 13.2.1 Stock Configuration Layer

Stock alerts are distributed with Netdata and reside in `/usr/lib/netdata/conf.d/health.d/`. These files are installed by the Netdata package and updated with each release. Modifying stock files is not recommended because changes are overwritten during upgrades.

Stock configurations define the default alert set. They are evaluated last in precedence, meaning custom configurations override stock configurations for the same alert.

## 13.2.2 Custom Configuration Layer

Custom alerts reside in `/etc/netdata/health.d/`. Files in this directory take precedence over stock configurations for the same alert names.

An alert defined in `/etc/netdata/health.d/` with the same name as a stock alert replaces the stock definition entirely.

## 13.2.3 Cloud Configuration Layer

Netdata Cloud can define alerts through the Alerts Configuration Manager. These Cloud-defined alerts take precedence over both stock and custom layers.

Cloud-defined alerts are stored remotely and synchronized to Agents on demand.

## 13.2.4 Configuration Precedence

| Layer | Location | Precedence | Persistence |
|-------|----------|------------|-------------|
| **Stock** | `/usr/lib/netdata/conf.d/health.d/` | Lowest | Package upgrades |
| **Custom** | `/etc/netdata/health.d/` | Medium | User-managed |
| **Cloud** | Remote synchronization | Highest | Cloud-synchronized |

## 13.2.5 Configuration File Merging

At startup and after configuration changes, Netdata merges all configuration layers into a single effective configuration.

Use the Alarms API to verify loaded configuration:

```bash
curl -s "http://localhost:19999/api/v1/alarms?all" | jq '.'
```

## Related Sections

- [13.1 Alert Lifecycle](2-alert-lifecycle.md) - How alerts transition states
- [13.3 Evaluation Architecture](1-evaluation-architecture.md) - Where alerts are evaluated
- [13.5 Scaling Topologies](5-scaling-topologies.md) - Behavior in distributed setups