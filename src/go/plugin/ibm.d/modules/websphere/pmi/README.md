# IBM WebSphere PMI collector

## Overview

Collects WebSphere Application Server performance metrics via the PerfServlet (PMI) interface,
covering JVM, thread pools, JDBC/JMS resources, applications, and clustering information.


This collector is part of the [Netdata](https://github.com/netdata/netdata) monitoring solution.

## Collected metrics

Metrics grouped by scope.

The scope defines the instance that the metric belongs to. An instance is uniquely identified by a set of labels.

### Per IBM WebSphere PMI instance


These metrics refer to the entire monitored IBM WebSphere PMI instance.

This scope has no labels.

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_pmi.jvm_heap_usage | used, free | bytes |
| websphere_pmi.jvm_heap_committed | committed | bytes |
| websphere_pmi.jvm_heap_max | limit | bytes |
| websphere_pmi.jvm_uptime | uptime | seconds |
| websphere_pmi.jvm_cpu | usage | percentage |
| websphere_pmi.jvm_gc_collections | collections | collections/s |
| websphere_pmi.jvm_gc_time | total | milliseconds |
| websphere_pmi.jvm_threads | daemon, other | threads |
| websphere_pmi.jvm_threads_peak | peak | threads |

These metrics refer to the entire monitored IBM WebSphere PMI instance.

This scope has no labels.

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_pmi.cpu_utilization | utilization | percentage |

These metrics refer to the entire monitored IBM WebSphere PMI instance.

This scope has no labels.

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_pmi.system_data_usage | cpu_since_last, free_memory | value |



### Per alarmmanager

These metrics refer to individual alarmmanager instances.

Labels:

| Label | Description |
|:------|:------------|
| node | Node identifier |
| server | Server identifier |
| manager | Manager identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_pmi.alarm_manager_events | created, cancelled, fired | events/s |

### Per dynamiccache

These metrics refer to individual dynamiccache instances.

Labels:

| Label | Description |
|:------|:------------|
| node | Node identifier |
| server | Server identifier |
| cache | Cache identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_pmi.dynamic_cache_in_memory | entries | entries |
| websphere_pmi.dynamic_cache_capacity | max_entries | entries |

### Per enterprisebeans

These metrics refer to individual enterprisebeans instances.

Labels:

| Label | Description |
|:------|:------------|
| node | Node identifier |
| server | Server identifier |
| bean | Bean identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_pmi.ejb_operations | create, remove, activate, passivate, instantiate, store, load | operations/s |
| websphere_pmi.ejb_messages | received, backout | messages/s |
| websphere_pmi.ejb_pool | ready, live, pooled, active_method, passive, server_session_pool, method_ready, async_queue | beans |
| websphere_pmi.ejb_time | activation, passivation, create, remove, load, store, method_response, wait, async_wait, read_lock, write_lock | milliseconds |

### Per extensionregistry

These metrics refer to individual extensionregistry instances.

Labels:

| Label | Description |
|:------|:------------|
| node | Node identifier |
| server | Server identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_pmi.extension_registry_requests | requests, hits, displacements | events/s |
| websphere_pmi.extension_registry_hit_rate | hit_rate | percentage |

### Per hamanager

These metrics refer to individual hamanager instances.

Labels:

| Label | Description |
|:------|:------------|
| node | Node identifier |
| server | Server identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_pmi.ha_manager_groups | local | groups |
| websphere_pmi.ha_manager_bulletin_board | subjects, subscriptions, local_subjects, local_subscriptions | items |
| websphere_pmi.ha_manager_rebuild_time | group_state, bulletin_board | milliseconds |

### Per jcapool

These metrics refer to individual jcapool instances.

Labels:

| Label | Description |
|:------|:------------|
| node | Node identifier |
| server | Server identifier |
| provider | Provider identifier |
| pool | Pool identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_pmi.jca_pool_operations | create, close, allocate, freed, faults | operations/s |
| websphere_pmi.jca_pool_managed | managed_connections, connection_handles | resources |
| websphere_pmi.jca_pool_utilization | percent_used, percent_maxed | percentage |
| websphere_pmi.jca_pool_waiting | waiting_threads | threads |

### Per jdbcpool

These metrics refer to individual jdbcpool instances.

Labels:

| Label | Description |
|:------|:------------|
| node | Node identifier |
| server | Server identifier |
| pool | Pool identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_pmi.jdbc_pool_usage | percent_used, percent_maxed | percentage |
| websphere_pmi.jdbc_pool_waiting | waiting_threads | threads |
| websphere_pmi.jdbc_pool_connections | managed, handles | connections |
| websphere_pmi.jdbc_pool_operations | created, closed, allocated, returned, faults, prep_stmt_cache_discard | operations/s |
| websphere_pmi.jdbc_pool_time | use, wait, jdbc | milliseconds |

### Per jmsqueue

These metrics refer to individual jmsqueue instances.

Labels:

| Label | Description |
|:------|:------------|
| node | Node identifier |
| server | Server identifier |
| engine | Engine identifier |
| destination | Destination identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_pmi.jms_queue_messages_produced | total, best_effort, express, reliable_nonpersistent, reliable_persistent, assured_persistent | messages/s |
| websphere_pmi.jms_queue_messages_consumed | total, best_effort, express, reliable_nonpersistent, reliable_persistent, assured_persistent, expired | messages/s |
| websphere_pmi.jms_queue_clients | local_producers, local_producer_attaches, local_consumers, local_consumer_attaches | clients |
| websphere_pmi.jms_queue_storage | available, unavailable, oldest_age | messages |
| websphere_pmi.jms_queue_wait_time | aggregate, local | milliseconds |

### Per jmsstore

These metrics refer to individual jmsstore instances.

Labels:

| Label | Description |
|:------|:------------|
| node | Node identifier |
| server | Server identifier |
| engine | Engine identifier |
| section | Section identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_pmi.jms_store_cache | add_stored, add_not_stored, stored_current, stored_bytes, not_stored_current, not_stored_bytes, discard_count, discard_bytes | events |
| websphere_pmi.jms_store_datastore | insert_batches, update_batches, delete_batches, insert_count, update_count, delete_count, open_count, abort_count, transaction_ms | events/s |
| websphere_pmi.jms_store_transactions | global_start, global_commit, global_abort, global_indoubt, local_start, local_commit, local_abort | transactions/s |
| websphere_pmi.jms_store_expiry | index_items | items |

### Per jmstopic

These metrics refer to individual jmstopic instances.

Labels:

| Label | Description |
|:------|:------------|
| node | Node identifier |
| server | Server identifier |
| engine | Engine identifier |
| destination | Destination identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_pmi.jms_topic_publications | assured, best_effort, express | messages/s |
| websphere_pmi.jms_topic_subscription_hits | assured, best_effort, express | events/s |
| websphere_pmi.jms_topic_subscriptions | durable_local | subscriptions |
| websphere_pmi.jms_topic_events | incomplete_publications, publisher_attaches, subscriber_attaches | events/s |
| websphere_pmi.jms_topic_age | local_oldest | milliseconds |

### Per orb

These metrics refer to individual orb instances.

Labels:

| Label | Description |
|:------|:------------|
| node | Node identifier |
| server | Server identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_pmi.orb_concurrent | concurrent_requests | requests |
| websphere_pmi.orb_requests | requests | requests/s |

### Per objectpool

These metrics refer to individual objectpool instances.

Labels:

| Label | Description |
|:------|:------------|
| node | Node identifier |
| server | Server identifier |
| pool | Pool identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_pmi.object_pool_operations | created | operations/s |
| websphere_pmi.object_pool_size | allocated, returned, idle | objects |

### Per pmiwebservicemodule

These metrics refer to individual pmiwebservicemodule instances.

Labels:

| Label | Description |
|:------|:------------|
| node | Node identifier |
| server | Server identifier |
| module | Module identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_pmi.pmi_web_service_module_services | loaded | services |

### Per portlet

These metrics refer to individual portlet instances.

Labels:

| Label | Description |
|:------|:------------|
| node | Node identifier |
| server | Server identifier |
| portlet | Portlet identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_pmi.portlet_requests | requests | requests/s |
| websphere_pmi.portlet_concurrent | concurrent | requests |
| websphere_pmi.portlet_errors | errors | errors/s |
| websphere_pmi.portlet_response_time | render, action, process_event, serve_resource | milliseconds |

### Per portletapplication

These metrics refer to individual portletapplication instances.

Labels:

| Label | Description |
|:------|:------------|
| node | Node identifier |
| server | Server identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_pmi.portlet_application_loaded | loaded | portlets |

### Per schedulers

These metrics refer to individual schedulers instances.

Labels:

| Label | Description |
|:------|:------------|
| node | Node identifier |
| server | Server identifier |
| scheduler | Scheduler identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_pmi.scheduler_activity | finished, failures, polls | events/s |

### Per securityauth

These metrics refer to individual securityauth instances.

Labels:

| Label | Description |
|:------|:------------|
| node | Node identifier |
| server | Server identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_pmi.security_auth_counts | web, tai, identity, basic, token, jaas_identity, jaas_basic, jaas_token, rmi | events/s |

### Per securityauthz

These metrics refer to individual securityauthz instances.

Labels:

| Label | Description |
|:------|:------------|
| node | Node identifier |
| server | Server identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_pmi.security_authz_time | web, ejb, admin, cwwja | milliseconds |

### Per sessionmanager

These metrics refer to individual sessionmanager instances.

Labels:

| Label | Description |
|:------|:------------|
| node | Node identifier |
| server | Server identifier |
| app | App identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_pmi.session_manager_active | active, live | sessions |
| websphere_pmi.session_manager_events | created, invalidated, timeout_invalidations, affinity_breaks, cache_discards, no_room, activate_non_exist | events/s |

### Per threadpool

These metrics refer to individual threadpool instances.

Labels:

| Label | Description |
|:------|:------------|
| name | Name identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_pmi.threadpool_usage | active, size | threads |

### Per transactionmanager

These metrics refer to individual transactionmanager instances.

Labels:

| Label | Description |
|:------|:------------|
| node | Node identifier |
| server | Server identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_pmi.transaction_counts | global_begun, global_committed, global_rolled_back, global_timeout, global_involved, optimizations, local_begun, local_committed, local_rolled_back, local_timeout | transactions/s |
| websphere_pmi.transaction_active | global, local | transactions |
| websphere_pmi.transaction_time | global_total, global_prepare, global_commit, global_before_completion, local_total, local_commit, local_before_completion | milliseconds |

### Per url

These metrics refer to individual url instances.

Labels:

| Label | Description |
|:------|:------------|
| node | Node identifier |
| server | Server identifier |
| url | Url identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_pmi.url_requests | requests | requests/s |
| websphere_pmi.url_time | service, async | milliseconds |

### Per webapp

These metrics refer to individual webapp instances.

Labels:

| Label | Description |
|:------|:------------|
| node | Node identifier |
| server | Server identifier |
| app | App identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_pmi.webapp_load | loaded_servlets, reloads | events |

### Per webservices

These metrics refer to individual webservices instances.

Labels:

| Label | Description |
|:------|:------------|
| node | Node identifier |
| server | Server identifier |
| service | Service identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_pmi.web_services_loaded | loaded | services |

### Per webservicesgateway

These metrics refer to individual webservicesgateway instances.

Labels:

| Label | Description |
|:------|:------------|
| node | Node identifier |
| server | Server identifier |
| gateway | Gateway identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_pmi.web_services_gateway_requests | synchronous, synchronous_responses, asynchronous, asynchronous_responses | requests/s |


## Configuration

### File

The configuration file name for this integration is `ibm.d/websphere_pmi.conf`.

You can edit the configuration file using the `edit-config` script from the
Netdata [config directory](https://github.com/netdata/netdata/blob/master/docs/netdata-agent/configuration.md#the-netdata-config-directory).

```bash
cd /etc/netdata 2>/dev/null || cd /opt/netdata/etc/netdata
sudo ./edit-config ibm.d/websphere_pmi.conf
```

### Options

The following options can be defined globally or per job.

| Name | Description | Default | Required | Min | Max |
|:-----|:------------|:--------|:---------|:----|:----|
| update_every | Data collection frequency | `1` | no | 1 | - |
| Vnode | Vnode allows binding the collector to a virtual node. | `` | no | - | - |
| PMIStatsType | PMIStatsType selects which PMI statistics tier to request (basic, extended, all). | `extended` | no | - | - |
| PMIRefreshRate | PMIRefreshRate overrides the PMI servlet refresh interval in seconds. | `60` | no | - | - |
| ClusterName | ClusterName appends a cluster label to every timeseries. | `` | no | - | - |
| CellName | CellName appends the cell label to every timeseries. | `` | no | - | - |
| NodeName | NodeName appends the node label to every timeseries. | `` | no | - | - |
| ServerType | ServerType annotates metrics with the WebSphere server type (e.g. app_server, dmgr). | `` | no | - | - |
| CollectJVMMetrics | CollectJVMMetrics toggles JVM runtime metrics. | `<no value>` | no | - | - |
| CollectThreadPoolMetrics | CollectThreadPoolMetrics toggles thread pool metrics. | `<no value>` | no | - | - |
| CollectJDBCMetrics | CollectJDBCMetrics toggles JDBC pool metrics. | `<no value>` | no | - | - |
| CollectJCAMetrics | CollectJCAMetrics toggles JCA resource adapter metrics. | `<no value>` | no | - | - |
| CollectJMSMetrics | CollectJMSMetrics toggles JMS destination metrics. | `<no value>` | no | - | - |
| CollectWebAppMetrics | CollectWebAppMetrics toggles Web application metrics. | `<no value>` | no | - | - |
| CollectSessionMetrics | CollectSessionMetrics toggles HTTP session manager metrics. | `<no value>` | no | - | - |
| CollectTransactionMetrics | CollectTransactionMetrics toggles transaction manager metrics. | `<no value>` | no | - | - |
| CollectClusterMetrics | CollectClusterMetrics toggles cluster health metrics. | `<no value>` | no | - | - |
| CollectServletMetrics | CollectServletMetrics toggles servlet response-time metrics. | `<no value>` | no | - | - |
| CollectEJBMetrics | CollectEJBMetrics toggles Enterprise Java Bean metrics. | `<no value>` | no | - | - |
| CollectJDBCAdvanced | CollectJDBCAdvanced toggles advanced JDBC timing metrics. | `<no value>` | no | - | - |
| MaxThreadPools | MaxThreadPools caps the number of thread pools charted per server. | `50` | no | - | - |
| MaxJDBCPools | MaxJDBCPools caps the number of JDBC pools charted. | `50` | no | - | - |
| MaxJCAPools | MaxJCAPools caps the number of JCA pools charted. | `50` | no | - | - |
| MaxJMSDestinations | MaxJMSDestinations caps the number of JMS destinations charted. | `50` | no | - | - |
| MaxApplications | MaxApplications caps the number of web applications charted. | `100` | no | - | - |
| MaxServlets | MaxServlets caps the number of servlets charted. | `50` | no | - | - |
| MaxEJBs | MaxEJBs caps the number of EJBs charted. | `50` | no | - | - |
| CollectAppsMatching | CollectAppsMatching filters applications by name using glob patterns. | `` | no | - | - |
| CollectPoolsMatching | CollectPoolsMatching filters pools (JDBC/JCA) by name using glob patterns. | `` | no | - | - |
| CollectJMSMatching | CollectJMSMatching filters JMS destinations by name using glob patterns. | `` | no | - | - |
| CollectServletsMatching | CollectServletsMatching filters servlets by name using glob patterns. | `` | no | - | - |
| CollectEJBsMatching | CollectEJBsMatching filters EJBs by name using glob patterns. | `` | no | - | - |

### Examples

#### Basic configuration

IBM WebSphere PMI monitoring with default settings.

<details>
<summary>Config</summary>

```yaml
jobs:
  - name: local
    endpoint: dummy://localhost
```

</details>

## Troubleshooting

### Debug Mode

To troubleshoot issues with the `websphere_pmi` collector, run the `ibm.d.plugin` with the debug option enabled.
The output should give you clues as to why the collector isn't working.

- Navigate to the `plugins.d` directory, usually at `/usr/libexec/netdata/plugins.d/`
- Switch to the `netdata` user
- Run the `ibm.d.plugin` to debug the collector:

```bash
sudo -u netdata ./ibm.d.plugin -d -m websphere_pmi
```

## Getting Logs

If you're encountering problems with the `websphere_pmi` collector, follow these steps to retrieve logs and identify potential issues:

- **Run the command** specific to your system (systemd, non-systemd, or Docker container).
- **Examine the output** for any warnings or error messages that might indicate issues. These messages will typically provide clues about the root cause of the problem.

### For systemd systems (most Linux distributions)

```bash
sudo journalctl -u netdata --reverse | grep websphere_pmi
```

### For non-systemd systems

```bash
sudo grep websphere_pmi /var/log/netdata/error.log
sudo grep websphere_pmi /var/log/netdata/collector.log
```

### For Docker containers

```bash
sudo docker logs netdata 2>&1 | grep websphere_pmi
```
