// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import "github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"

// Windows exporter:
// https://github.com/prometheus-community/windows_exporter/blob/master/docs/collector.ad.md
// Microsoft:
// https://learn.microsoft.com/en-us/previous-versions/ms803980(v=msdn.10)
const (
	metricADATQAverageRequestLatency                  = "windows_ad_atq_average_request_latency"
	metricADATQOutstandingRequests                    = "windows_ad_atq_outstanding_requests"
	metricADDatabaseOperationsTotal                   = "windows_ad_database_operations_total"
	metricADDirectoryOperationsTotal                  = "windows_ad_directory_operations_total"
	metricADReplicationInboundObjectsFilteringTotal   = "windows_ad_replication_inbound_objects_filtered_total"
	metricADReplicationInboundPropertiesFilteredTotal = "windows_ad_replication_inbound_properties_filtered_total"
	metricADReplicationInboundPropertiesUpdatedTotal  = "windows_ad_replication_inbound_properties_updated_total"
	metricADReplicationInboundSyncObjectsRemaining    = "windows_ad_replication_inbound_sync_objects_remaining"
	metricADReplicationDataInterSiteBytesTotal        = "windows_ad_replication_data_intersite_bytes_total"
	metricADReplicationDataIntraSiteBytesTotal        = "windows_ad_replication_data_intrasite_bytes_total"
	metricADReplicationPendingSyncs                   = "windows_ad_replication_pending_synchronizations"
	metricADReplicationSyncRequestsTotal              = "windows_ad_replication_sync_requests_total"
	metricADDirectoryServiceThreads                   = "windows_ad_directory_service_threads"
	metricADLDAPLastBindTimeSecondsTotal              = "windows_ad_ldap_last_bind_time_seconds"
	metricADBindsTotal                                = "windows_ad_binds_total"
	metricADLDAPSearchesTotal                         = "windows_ad_ldap_searches_total"
	metricADNameCacheLookupsTotal                     = "windows_ad_name_cache_lookups_total"
	metricADNameCacheHitsTotal                        = "windows_ad_name_cache_hits_total"
)

func (w *Windows) collectAD(mx map[string]int64, pms prometheus.Series) {
	if !w.cache.collection[collectorAD] {
		w.cache.collection[collectorAD] = true
		w.addADCharts()
	}

	if pm := pms.FindByName(metricADATQAverageRequestLatency); pm.Len() > 0 {
		mx["ad_atq_average_request_latency"] = int64(pm.Max() * precision)
	}
	if pm := pms.FindByName(metricADATQOutstandingRequests); pm.Len() > 0 {
		mx["ad_atq_outstanding_requests"] = int64(pm.Max())
	}
	for _, pm := range pms.FindByName(metricADDatabaseOperationsTotal) {
		if op := pm.Labels.Get("operation"); op != "" {
			mx["ad_database_operations_total_"+op] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricADDirectoryOperationsTotal) {
		if op := pm.Labels.Get("operation"); op != "" {
			mx["ad_directory_operations_total_"+op] += int64(pm.Value) // sum "origin"
		}
	}
	if pm := pms.FindByName(metricADReplicationInboundObjectsFilteringTotal); pm.Len() > 0 {
		mx["ad_replication_inbound_objects_filtered_total"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricADReplicationInboundPropertiesFilteredTotal); pm.Len() > 0 {
		mx["ad_replication_inbound_properties_filtered_total"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricADReplicationInboundPropertiesUpdatedTotal); pm.Len() > 0 {
		mx["ad_replication_inbound_properties_updated_total"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricADReplicationInboundSyncObjectsRemaining); pm.Len() > 0 {
		mx["ad_replication_inbound_sync_objects_remaining"] = int64(pm.Max())
	}
	for _, pm := range pms.FindByName(metricADReplicationDataInterSiteBytesTotal) {
		if name := pm.Labels.Get("direction"); name != "" {
			mx["ad_replication_data_intersite_bytes_total_"+name] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricADReplicationDataIntraSiteBytesTotal) {
		if name := pm.Labels.Get("direction"); name != "" {
			mx["ad_replication_data_intrasite_bytes_total_"+name] = int64(pm.Value)
		}
	}
	if pm := pms.FindByName(metricADReplicationPendingSyncs); pm.Len() > 0 {
		mx["ad_replication_pending_synchronizations"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricADReplicationSyncRequestsTotal); pm.Len() > 0 {
		mx["ad_replication_sync_requests_total"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricADDirectoryServiceThreads); pm.Len() > 0 {
		mx["ad_directory_service_threads"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricADLDAPLastBindTimeSecondsTotal); pm.Len() > 0 {
		mx["ad_ldap_last_bind_time_seconds"] = int64(pm.Max())
	}
	for _, pm := range pms.FindByName(metricADBindsTotal) {
		mx["ad_binds_total"] += int64(pm.Value) // sum "bind_method"'s
	}
	if pm := pms.FindByName(metricADLDAPSearchesTotal); pm.Len() > 0 {
		mx["ad_ldap_searches_total"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricADNameCacheLookupsTotal); pm.Len() > 0 {
		mx["ad_name_cache_lookups_total"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricADNameCacheHitsTotal); pm.Len() > 0 {
		mx["ad_name_cache_hits_total"] = int64(pm.Max())
	}
}
