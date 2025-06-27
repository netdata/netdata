# WebSphere PMI Configuration Synchronization Analysis

This document analyzes the configuration synchronization between:
1. `collector.go` (Config struct)
2. `config_schema.json`
3. `metadata.yaml`

## Summary of Findings

### ❌ Missing Fields in config_schema.json

The following fields from the Config struct are NOT present in config_schema.json:
- `vnode` - Virtual node identifier
- `pmi_custom_stats_paths` - Array for custom PMI stat paths
- `custom_labels` - Map for custom labels
- HTTP client config fields (from embedded `web.HTTPConfig`):
  - `proxy_url`
  - `proxy_username`
  - `proxy_password`
  - `headers`
  - `body`
  - `method`
  - `tls_ca`
  - `tls_cert`
  - `tls_key`
  - `tls_skip_verify`
  - `not_follow_redirects`

### ❌ Missing Fields in metadata.yaml

The following fields from config_schema.json are NOT documented in metadata.yaml:
- `pmi_refresh_rate` - PMI refresh rate setting
- `pmi_custom_stats_paths` - Custom PMI statistics paths
- `cluster_name` - Cluster name for labeling
- `cell_name` - Cell name for labeling
- `node_name` - Node name for labeling
- `server_type` - Server type (app_server, dmgr, nodeagent)
- `custom_labels` - Custom labels map
- `collect_jca_metrics` - JCA metrics collection flag
- `collect_jms_metrics` - JMS metrics collection flag
- `collect_session_metrics` - Session metrics collection flag
- `collect_transaction_metrics` - Transaction metrics collection flag
- `collect_cluster_metrics` - Cluster metrics collection flag
- `collect_servlet_metrics` - Servlet metrics collection flag
- `collect_ejb_metrics` - EJB metrics collection flag
- `collect_jdbc_advanced` - Advanced JDBC metrics flag
- `max_jca_pools` - Maximum JCA pools limit
- `max_jms_destinations` - Maximum JMS destinations limit
- `max_servlets` - Maximum servlets limit
- `max_ejbs` - Maximum EJBs limit
- `collect_apps_matching` - Application filter pattern
- `collect_pools_matching` - Pool filter pattern
- `collect_jms_matching` - JMS filter pattern
- `collect_servlets_matching` - Servlet filter pattern
- `collect_ejbs_matching` - EJB filter pattern

### ✅ Type Consistency

All fields that exist in multiple files have consistent types:
- `update_every`: integer in all files
- `url`: string in all files
- `username`: string in all files
- `password`: string in all files
- `pmi_stats_type`: string with enum values in all files
- `timeout`: integer (Note: Go uses Duration type but JSON schema uses integer seconds)
- Boolean flags: all consistently boolean
- Cardinality limits: all consistently integer

### ⚠️ Default Value Discrepancies

1. **timeout**:
   - Go code: 5 seconds (via `confopt.Duration(time.Second * 5)`)
   - config_schema.json: 5 (consistent)
   - metadata.yaml: 5 (consistent)

2. **server_type**:
   - Go code: No default (empty string)
   - config_schema.json: "app_server"
   - metadata.yaml: Not documented

3. **APM collection flags** (servlet, ejb, jdbc_advanced):
   - Go code: false (not set in New())
   - config_schema.json: false (consistent)
   - metadata.yaml: Not documented

### 📋 Detailed Field Mapping

| Field | Go Type | JSON Schema | Metadata | Status |
|-------|---------|-------------|----------|---------|
| vnode | string | ❌ Missing | ❌ Missing | ⚠️ |
| update_every | int | ✅ integer | ✅ Documented | ✅ |
| url | string | ✅ string | ✅ Documented | ✅ |
| username | string | ✅ string | ✅ Documented | ✅ |
| password | string | ✅ string | ✅ Documented | ✅ |
| pmi_stats_type | string | ✅ string/enum | ✅ Documented | ✅ |
| pmi_refresh_rate | int | ✅ integer | ❌ Not documented | ⚠️ |
| pmi_custom_stats_paths | []string | ❌ Missing | ❌ Missing | ⚠️ |
| cluster_name | string | ✅ string | ❌ Not documented | ⚠️ |
| cell_name | string | ✅ string | ❌ Not documented | ⚠️ |
| node_name | string | ✅ string | ❌ Not documented | ⚠️ |
| server_type | string | ✅ string/enum | ❌ Not documented | ⚠️ |
| custom_labels | map[string]string | ❌ Missing | ❌ Missing | ⚠️ |
| collect_jvm_metrics | bool | ✅ boolean | ✅ Documented | ✅ |
| collect_threadpool_metrics | bool | ✅ boolean | ✅ Documented | ✅ |
| collect_jdbc_metrics | bool | ✅ boolean | ✅ Documented | ✅ |
| collect_jca_metrics | bool | ✅ boolean | ❌ Not documented | ⚠️ |
| collect_jms_metrics | bool | ✅ boolean | ❌ Not documented | ⚠️ |
| collect_webapp_metrics | bool | ✅ boolean | ✅ Documented | ✅ |
| collect_session_metrics | bool | ✅ boolean | ❌ Not documented | ⚠️ |
| collect_transaction_metrics | bool | ✅ boolean | ❌ Not documented | ⚠️ |
| collect_cluster_metrics | bool | ✅ boolean | ❌ Not documented | ⚠️ |
| collect_servlet_metrics | bool | ✅ boolean | ❌ Not documented | ⚠️ |
| collect_ejb_metrics | bool | ✅ boolean | ❌ Not documented | ⚠️ |
| collect_jdbc_advanced | bool | ✅ boolean | ❌ Not documented | ⚠️ |
| max_threadpools | int | ✅ integer | ✅ Documented | ✅ |
| max_jdbc_pools | int | ✅ integer | ✅ Documented | ✅ |
| max_jca_pools | int | ✅ integer | ❌ Not documented | ⚠️ |
| max_jms_destinations | int | ✅ integer | ❌ Not documented | ⚠️ |
| max_applications | int | ✅ integer | ✅ Documented | ✅ |
| max_servlets | int | ✅ integer | ❌ Not documented | ⚠️ |
| max_ejbs | int | ✅ integer | ❌ Not documented | ⚠️ |
| collect_apps_matching | string | ✅ string | ❌ Not documented | ⚠️ |
| collect_pools_matching | string | ✅ string | ❌ Not documented | ⚠️ |
| collect_jms_matching | string | ✅ string | ❌ Not documented | ⚠️ |
| collect_servlets_matching | string | ✅ string | ❌ Not documented | ⚠️ |
| collect_ejbs_matching | string | ✅ string | ❌ Not documented | ⚠️ |

## Recommendations

1. **Add missing fields to config_schema.json**:
   - Add `vnode` field
   - Add `pmi_custom_stats_paths` as array of strings
   - Add `custom_labels` as object/map
   - Consider adding common HTTP client fields (proxy, TLS, etc.)

2. **Update metadata.yaml documentation**:
   - Add documentation for all missing configuration options
   - Ensure all collection flags are documented
   - Add cardinality limit options
   - Add filter pattern options

3. **Fix default value inconsistencies**:
   - Align `server_type` default between Go code and schema
   - Consider setting APM flags defaults in Go code if needed

4. **Consider schema improvements**:
   - Add pattern validation for filter fields
   - Add format validation for URL fields
   - Add minimum/maximum constraints where appropriate