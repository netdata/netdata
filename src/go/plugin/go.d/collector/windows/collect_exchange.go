// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
)

const (
	metricExchangeActiveSyncPingCmdsPending  = "windows_exchange_activesync_ping_cmds_pending"
	metricExchangeActiveSyncRequestsTotal    = "windows_exchange_activesync_requests_total"
	metricExchangeActiveSyncCMDsTotal        = "windows_exchange_activesync_sync_cmds_total"
	metricExchangeAutoDiscoverRequestsTotal  = "windows_exchange_autodiscover_requests_total"
	metricExchangeAvailServiceRequestsPerSec = "windows_exchange_avail_service_requests_per_sec"
	metricExchangeOWACurrentUniqueUsers      = "windows_exchange_owa_current_unique_users"
	metricExchangeOWARequestsTotal           = "windows_exchange_owa_requests_total"
	metricExchangeRPCActiveUserCount         = "windows_exchange_rpc_active_user_count"
	metricExchangeRPCAvgLatencySec           = "windows_exchange_rpc_avg_latency_sec"
	metricExchangeRPCConnectionCount         = "windows_exchange_rpc_connection_count"
	metricExchangeRPCOperationsTotal         = "windows_exchange_rpc_operations_total"
	metricExchangeRPCRequests                = "windows_exchange_rpc_requests"
	metricExchangeRPCUserCount               = "windows_exchange_rpc_user_count"

	metricExchangeTransportQueuesActiveMailboxDelivery        = "windows_exchange_transport_queues_active_mailbox_delivery"
	metricExchangeTransportQueuesExternalActiveRemoteDelivery = "windows_exchange_transport_queues_external_active_remote_delivery"
	metricExchangeTransportQueuesExternalLargestDelivery      = "windows_exchange_transport_queues_external_largest_delivery"
	metricExchangeTransportQueuesInternalActiveRemoteDelivery = "windows_exchange_transport_queues_internal_active_remote_delivery"
	metricExchangeTransportQueuesInternalLargestDelivery      = "windows_exchange_transport_queues_internal_largest_delivery"
	metricExchangeTransportQueuesPoison                       = "windows_exchange_transport_queues_poison"
	metricExchangeTransportQueuesRetryMailboxDelivery         = "windows_exchange_transport_queues_retry_mailbox_delivery"
	metricExchangeTransportQueuesUnreachable                  = "windows_exchange_transport_queues_unreachable"

	metricExchangeWorkloadActiveTasks    = "windows_exchange_workload_active_tasks"
	metricExchangeWorkloadCompletedTasks = "windows_exchange_workload_completed_tasks"
	metricExchangeWorkloadQueuedTasks    = "windows_exchange_workload_queued_tasks"
	metricExchangeWorkloadYieldedTasks   = "windows_exchange_workload_yielded_tasks"
	metricExchangeWorkloadIsActive       = "windows_exchange_workload_is_active"

	metricExchangeLDAPLongRunningOPSPerSec = "windows_exchange_ldap_long_running_ops_per_sec"
	metricExchangeLDAPReadTimeSec          = "windows_exchange_ldap_read_time_sec"
	metricExchangeLDAPSearchTmeSec         = "windows_exchange_ldap_search_time_sec"
	metricExchangeLDAPWriteTimeSec         = "windows_exchange_ldap_write_time_sec"
	metricExchangeLDAPTimeoutErrorsTotal   = "windows_exchange_ldap_timeout_errors_total"

	metricExchangeHTTPProxyAvgAuthLatency                    = "windows_exchange_http_proxy_avg_auth_latency"
	metricExchangeHTTPProxyAvgCASProcessingLatencySec        = "windows_exchange_http_proxy_avg_cas_proccessing_latency_sec"
	metricExchangeHTTPProxyMailboxProxyFailureRate           = "windows_exchange_http_proxy_mailbox_proxy_failure_rate"
	metricExchangeHTTPProxyMailboxServerLocatorAvgLatencySec = "windows_exchange_http_proxy_mailbox_server_locator_avg_latency_sec"
	metricExchangeHTTPProxyOutstandingProxyRequests          = "windows_exchange_http_proxy_outstanding_proxy_requests"
	metricExchangeHTTPProxyRequestsTotal                     = "windows_exchange_http_proxy_requests_total"
)

func (c *Collector) collectExchange(mx map[string]int64, pms prometheus.Series) {
	if !c.cache.collection[collectorExchange] {
		c.cache.collection[collectorExchange] = true
		c.addExchangeCharts()
	}

	if pm := pms.FindByName(metricExchangeActiveSyncPingCmdsPending); pm.Len() > 0 {
		mx["exchange_activesync_ping_cmds_pending"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricExchangeActiveSyncRequestsTotal); pm.Len() > 0 {
		mx["exchange_activesync_requests_total"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricExchangeActiveSyncCMDsTotal); pm.Len() > 0 {
		mx["exchange_activesync_sync_cmds_total"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricExchangeAutoDiscoverRequestsTotal); pm.Len() > 0 {
		mx["exchange_autodiscover_requests_total"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricExchangeAvailServiceRequestsPerSec); pm.Len() > 0 {
		mx["exchange_avail_service_requests_per_sec"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricExchangeOWACurrentUniqueUsers); pm.Len() > 0 {
		mx["exchange_owa_current_unique_users"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricExchangeOWARequestsTotal); pm.Len() > 0 {
		mx["exchange_owa_requests_total"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricExchangeRPCActiveUserCount); pm.Len() > 0 {
		mx["exchange_rpc_active_user_count"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricExchangeRPCAvgLatencySec); pm.Len() > 0 {
		mx["exchange_rpc_avg_latency_sec"] = int64(pm.Max() * precision)
	}
	if pm := pms.FindByName(metricExchangeRPCConnectionCount); pm.Len() > 0 {
		mx["exchange_rpc_connection_count"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricExchangeRPCOperationsTotal); pm.Len() > 0 {
		mx["exchange_rpc_operations_total"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricExchangeRPCRequests); pm.Len() > 0 {
		mx["exchange_rpc_requests"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricExchangeRPCUserCount); pm.Len() > 0 {
		mx["exchange_rpc_user_count"] = int64(pm.Max())
	}

	c.collectExchangeAddTransportQueueMetric(mx, pms)
	c.collectExchangeAddWorkloadMetric(mx, pms)
	c.collectExchangeAddLDAPMetric(mx, pms)
	c.collectExchangeAddHTTPProxyMetric(mx, pms)
}

func (c *Collector) collectExchangeAddTransportQueueMetric(mx map[string]int64, pms prometheus.Series) {
	pms = pms.FindByNames(
		metricExchangeTransportQueuesActiveMailboxDelivery,
		metricExchangeTransportQueuesExternalActiveRemoteDelivery,
		metricExchangeTransportQueuesExternalLargestDelivery,
		metricExchangeTransportQueuesInternalActiveRemoteDelivery,
		metricExchangeTransportQueuesInternalLargestDelivery,
		metricExchangeTransportQueuesPoison,
		metricExchangeTransportQueuesRetryMailboxDelivery,
		metricExchangeTransportQueuesUnreachable,
	)

	for _, pm := range pms {
		if name := pm.Labels.Get("name"); name != "" && name != "total_excluding_priority_none" {
			metric := strings.TrimPrefix(pm.Name(), "windows_")
			mx[metric+"_"+name] += int64(pm.Value)
		}
	}
}

func (c *Collector) collectExchangeAddWorkloadMetric(mx map[string]int64, pms prometheus.Series) {
	seen := make(map[string]bool)

	for _, pm := range pms.FindByNames(
		metricExchangeWorkloadActiveTasks,
		metricExchangeWorkloadCompletedTasks,
		metricExchangeWorkloadQueuedTasks,
		metricExchangeWorkloadYieldedTasks,
	) {
		if name := pm.Labels.Get("name"); name != "" {
			seen[name] = true
			metric := strings.TrimPrefix(pm.Name(), "windows_exchange_workload_")
			mx["exchange_workload_"+name+"_"+metric] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricExchangeWorkloadIsActive) {
		if name := pm.Labels.Get("name"); name != "" {
			seen[name] = true
			mx["exchange_workload_"+name+"_is_active"] += metrix.Bool(pm.Value == 1)
			mx["exchange_workload_"+name+"_is_paused"] += metrix.Bool(pm.Value == 0)
		}
	}

	for name := range seen {
		if !c.cache.exchangeWorkload[name] {
			c.cache.exchangeWorkload[name] = true
			c.addExchangeWorkloadCharts(name)
		}
	}
	for name := range c.cache.exchangeWorkload {
		if !seen[name] {
			delete(c.cache.exchangeWorkload, name)
			c.removeExchangeWorkloadCharts(name)
		}
	}
}

func (c *Collector) collectExchangeAddLDAPMetric(mx map[string]int64, pms prometheus.Series) {
	seen := make(map[string]bool)

	for _, pm := range pms.FindByNames(
		metricExchangeLDAPLongRunningOPSPerSec,
		metricExchangeLDAPTimeoutErrorsTotal,
	) {
		if name := pm.Labels.Get("name"); name != "" {
			seen[name] = true
			metric := strings.TrimPrefix(pm.Name(), "windows_exchange_ldap_")
			mx["exchange_ldap_"+name+"_"+metric] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByNames(
		metricExchangeLDAPReadTimeSec,
		metricExchangeLDAPSearchTmeSec,
		metricExchangeLDAPWriteTimeSec,
	) {
		if name := pm.Labels.Get("name"); name != "" {
			seen[name] = true
			metric := strings.TrimPrefix(pm.Name(), "windows_exchange_ldap_")
			mx["exchange_ldap_"+name+"_"+metric] += int64(pm.Value * precision)
		}
	}

	for name := range seen {
		if !c.cache.exchangeLDAP[name] {
			c.cache.exchangeLDAP[name] = true
			c.addExchangeLDAPCharts(name)
		}
	}
	for name := range c.cache.exchangeLDAP {
		if !seen[name] {
			delete(c.cache.exchangeLDAP, name)
			c.removeExchangeLDAPCharts(name)
		}
	}
}

func (c *Collector) collectExchangeAddHTTPProxyMetric(mx map[string]int64, pms prometheus.Series) {
	seen := make(map[string]bool)

	for _, pm := range pms.FindByNames(
		metricExchangeHTTPProxyAvgAuthLatency,
		metricExchangeHTTPProxyOutstandingProxyRequests,
		metricExchangeHTTPProxyRequestsTotal,
	) {
		if name := pm.Labels.Get("name"); name != "" {
			seen[name] = true
			metric := strings.TrimPrefix(pm.Name(), "windows_exchange_http_proxy_")
			mx["exchange_http_proxy_"+name+"_"+metric] += int64(pm.Value)
		}
	}

	for _, pm := range pms.FindByNames(
		metricExchangeHTTPProxyAvgCASProcessingLatencySec,
		metricExchangeHTTPProxyMailboxProxyFailureRate,
		metricExchangeHTTPProxyMailboxServerLocatorAvgLatencySec,
	) {
		if name := pm.Labels.Get("name"); name != "" {
			seen[name] = true
			metric := strings.TrimPrefix(pm.Name(), "windows_exchange_http_proxy_")
			mx["exchange_http_proxy_"+name+"_"+metric] += int64(pm.Value * precision)
		}
	}

	for name := range seen {
		if !c.cache.exchangeHTTPProxy[name] {
			c.cache.exchangeHTTPProxy[name] = true
			c.addExchangeHTTPProxyCharts(name)
		}
	}
	for name := range c.cache.exchangeHTTPProxy {
		if !seen[name] {
			delete(c.cache.exchangeHTTPProxy, name)
			c.removeExchangeHTTPProxyCharts(name)
		}
	}
}
