// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
)

const (
	metricIISCurrentAnonymousUsers    = "windows_iis_current_anonymous_users"
	metricIISCurrentNonAnonymousUsers = "windows_iis_current_non_anonymous_users"
	metricIISCurrentConnections       = "windows_iis_current_connections"
	metricIICurrentISAPIExtRequests   = "windows_iis_current_isapi_extension_requests"
	metricIISUptime                   = "windows_iis_service_uptime"

	metricIISReceivedBytesTotal            = "windows_iis_received_bytes_total"
	metricIISSentBytesTotal                = "windows_iis_sent_bytes_total"
	metricIISRequestsTotal                 = "windows_iis_requests_total"
	metricIISIPAPIExtRequestsTotal         = "windows_iis_ipapi_extension_requests_total"
	metricIISConnAttemptsAllInstancesTotal = "windows_iis_connection_attempts_all_instances_total"
	metricIISFilesReceivedTotal            = "windows_iis_files_received_total"
	metricIISFilesSentTotal                = "windows_iis_files_sent_total"
	metricIISLogonAttemptsTotal            = "windows_iis_logon_attempts_total"
	metricIISLockedErrorsTotal             = "windows_iis_locked_errors_total"
	metricIISNotFoundErrorsTotal           = "windows_iis_not_found_errors_total"
)

func (c *Collector) collectIIS(mx map[string]int64, pms prometheus.Series) {
	seen := make(map[string]bool)
	px := "iis_website_"
	for _, pm := range pms.FindByName(metricIISCurrentAnonymousUsers) {
		if name := cleanWebsiteName(pm.Labels.Get("site")); name != "" {
			seen[name] = true
			mx[px+name+"_current_anonymous_users"] += int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricIISCurrentNonAnonymousUsers) {
		if name := cleanWebsiteName(pm.Labels.Get("site")); name != "" {
			seen[name] = true
			mx[px+name+"_current_non_anonymous_users"] += int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricIISCurrentConnections) {
		if name := cleanWebsiteName(pm.Labels.Get("site")); name != "" {
			seen[name] = true
			mx[px+name+"_current_connections"] += int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricIICurrentISAPIExtRequests) {
		if name := cleanWebsiteName(pm.Labels.Get("site")); name != "" {
			seen[name] = true
			mx[px+name+"_current_isapi_extension_requests"] += int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricIISUptime) {
		if name := cleanWebsiteName(pm.Labels.Get("site")); name != "" {
			seen[name] = true
			mx[px+name+"_service_uptime"] += int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricIISReceivedBytesTotal) {
		if name := cleanWebsiteName(pm.Labels.Get("site")); name != "" {
			seen[name] = true
			mx[px+name+"_received_bytes_total"] += int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricIISSentBytesTotal) {
		if name := cleanWebsiteName(pm.Labels.Get("site")); name != "" {
			seen[name] = true
			mx[px+name+"_sent_bytes_total"] += int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricIISRequestsTotal) {
		if name := cleanWebsiteName(pm.Labels.Get("site")); name != "" {
			seen[name] = true
			mx[px+name+"_requests_total"] += int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricIISConnAttemptsAllInstancesTotal) {
		if name := cleanWebsiteName(pm.Labels.Get("site")); name != "" {
			seen[name] = true
			mx[px+name+"_connection_attempts_all_instances_total"] += int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricIISFilesReceivedTotal) {
		if name := cleanWebsiteName(pm.Labels.Get("site")); name != "" {
			seen[name] = true
			mx[px+name+"_files_received_total"] += int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricIISFilesSentTotal) {
		if name := cleanWebsiteName(pm.Labels.Get("site")); name != "" {
			seen[name] = true
			mx[px+name+"_files_sent_total"] += int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricIISIPAPIExtRequestsTotal) {
		if name := cleanWebsiteName(pm.Labels.Get("site")); name != "" {
			seen[name] = true
			mx[px+name+"_isapi_extension_requests_total"] += int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricIISLogonAttemptsTotal) {
		if name := cleanWebsiteName(pm.Labels.Get("site")); name != "" {
			seen[name] = true
			mx[px+name+"_logon_attempts_total"] += int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricIISLockedErrorsTotal) {
		if name := cleanWebsiteName(pm.Labels.Get("site")); name != "" {
			seen[name] = true
			mx[px+name+"_locked_errors_total"] += int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricIISNotFoundErrorsTotal) {
		if name := cleanWebsiteName(pm.Labels.Get("site")); name != "" {
			seen[name] = true
			mx[px+name+"_not_found_errors_total"] += int64(pm.Value)
		}
	}

	for site := range seen {
		if !c.cache.iis[site] {
			c.cache.iis[site] = true
			c.addIISWebsiteCharts(site)
		}
	}
	for site := range c.cache.iis {
		if !seen[site] {
			delete(c.cache.iis, site)
			c.removeIIWebsiteSCharts(site)
		}
	}
}

func cleanWebsiteName(name string) string {
	return strings.ReplaceAll(name, " ", "_")
}
