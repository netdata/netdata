# Azure Monitor

This collector monitors Azure services through the Azure Monitor Metrics batch API.

It discovers resources from Azure Resource Graph, groups requests by service profile and region,
and collects metrics in 1-minute or coarser native Azure time grains.

## Configuration

- File: `go.d/azure_monitor.conf`
- Multi-instance: yes (one job per subscription is recommended)

### Required options

- `subscription_id`: Azure subscription ID.
- `auth.mode`: `service_principal`, `managed_identity`, or `default` (defaults to `default` when omitted).

### Important options

- `profiles`: profile file keys to enable (defaults to `auto` — discovers resource types in the subscription and activates matching profiles).
- `resource_groups`: optional backend filter.
- `query_offset`: defaults to 180s to avoid partial Azure windows.
- `max_concurrency`: bounded concurrent batch calls.

## Authentication modes

### Service principal

```yaml
jobs:
  - name: azure-prod
    subscription_id: "<subscription-id>"
    auth:
      mode: service_principal
      tenant_id: "<tenant-id>"
      client_id: "<client-id>"
      client_secret: "<client-secret>"
```

### Managed identity

```yaml
jobs:
  - name: azure-prod
    subscription_id: "<subscription-id>"
    auth:
      mode: managed_identity
```

### Default credential chain

```yaml
jobs:
  - name: azure-prod
    subscription_id: "<subscription-id>"
    auth:
      mode: default
```

## Built-in profiles

38 stock profiles covering common Azure services:

`aks`, `api_management`, `app_service`, `application_gateway`, `application_insights`,
`cognitive_services`, `container_apps`, `container_instances`, `container_registry`,
`cosmos_db`, `data_explorer`, `data_factory`, `event_grid`, `event_hubs`,
`express_route_circuit`, `express_route_gateway`, `firewall`, `front_door`,
`iot_hub`, `key_vault`, `load_balancers`, `log_analytics`, `logic_apps`,
`machine_learning`, `mysql_flexible`, `nat_gateway`, `postgres_flexible`,
`redis_cache`, `service_bus`, `sql_database`, `sql_elastic_pool`,
`sql_managed_instance`, `storage_accounts`, `stream_analytics`, `synapse`,
`virtual_machines`, `vmss`, `vpn_gateway`

## Profile files

- Stock profiles (packaged with Netdata): `/usr/lib/netdata/conf.d/go.d/azure_monitor.profiles/default/`
- User profiles and overrides: `/etc/netdata/go.d/azure_monitor.profiles/`
- User files override stock files when they share the same filename key.
- Profiles are strict and chart-driven: each profile defines full chart metadata (`id`, `title`, `context`, `family`, `type`, `priority`, `dimensions`, `instances`).

## Notes

- Azure Monitor metrics are not per-second. Minimum granularity is typically `PT1M`.
- Gauge metrics (average, maximum, minimum) use absolute chart algorithms.
- Counter metrics (total, count) typically use incremental chart algorithms so Netdata computes accurate per-second rates. The chart algorithm determines whether a metric is accumulated, not the aggregation name alone (e.g. a percentage metric using Total aggregation may use an absolute algorithm).
