# Azure Monitor

This collector monitors Azure services through the Azure Monitor Metrics batch API.

It discovers resources from Azure Resource Graph, groups requests by service profile and region,
and collects metrics in 1-minute or coarser native Azure time grains.

## Configuration

- File: `go.d/azure_monitor.conf`
- Multi-instance: yes (one job per subscription is recommended)

### Required options

- `subscription_id`: Azure subscription ID.
- `auth.mode`: `service_principal`, `managed_identity`, or `default`.

### Important options

- `profiles`: profile file keys to enable (defaults to all stock profiles).
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

- `sql_managed_instance`
- `sql_database`
- `postgres_flexible`
- `cosmos_db`
- `logic_apps`
- `virtual_machines`
- `aks`
- `storage_accounts`
- `load_balancers`

## Profile files

- Stock profiles (packaged with Netdata): `/usr/lib/netdata/conf.d/go.d/azure_monitor.profiles/default/`
- User profiles and overrides: `/etc/netdata/go.d/azure_monitor.profiles/`
- User files override stock files when they share the same filename key.

## Notes

- Azure Monitor metrics are not per-second. Minimum granularity is typically `PT1M`.
- The collector uses absolute chart algorithms because Azure values are already pre-aggregated.
