// SPDX-License-Identifier: GPL-3.0-or-later

package pihole

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioDNSQueriesTotal = module.Priority + iota
	prioDNSQueriesBlockedPercent
	prioDNSQueriesByDestination
	prioDNSQueriesByType
	prioDNSQueriesByStatus
	prioDNSRepliesByType

	prioActiveClients

	prioGravityListBlockedDomains
	prioGravityListLastUpdateTimeAgo
)

var summaryCharts = module.Charts{
	chartDNSQueriesTotal.Copy(),
	chartDNSQueriesBlockedPercent.Copy(),
	chartDNSQueriesByDestination.Copy(),
	chartDNSQueriesByType.Copy(),
	chartDNSQueriesByStatus.Copy(),

	chartDNSRepliesByType.Copy(),

	chartActiveClients.Copy(),

	chartGravityListBlockedDomains.Copy(),
	chartGravityListLastUpdateTimeAgo.Copy(),
}

var (
	chartDNSQueriesTotal = module.Chart{
		ID:       "dns_queries_total",
		Title:    "Pi-hole DNS Queries Total (Cached, Blocked and Forwarded)",
		Units:    "queries/s",
		Fam:      "queries",
		Ctx:      "pihole.dns_queries_total",
		Priority: prioDNSQueriesTotal,
		Dims: module.Dims{
			{ID: "queries_total", Name: "queries", Algo: module.Incremental},
		},
	}
	chartDNSQueriesBlockedPercent = module.Chart{
		ID:       "dns_queries_blocked_percent",
		Title:    "Pi-hole DNS Queries Blocked Percent",
		Units:    "percent",
		Fam:      "queries",
		Ctx:      "pihole.dns_queries_blocked_percent",
		Priority: prioDNSQueriesBlockedPercent,
		Dims: module.Dims{
			{ID: "queries_percent_blocked", Name: "blocked", Div: precision},
		},
	}
	chartDNSQueriesByDestination = module.Chart{
		ID:       "dns_queries_by_destination",
		Title:    "Pi-hole DNS Queries by Destination",
		Units:    "queries/s",
		Fam:      "queries",
		Ctx:      "pihole.dns_queries_by_destination",
		Type:     module.Stacked,
		Priority: prioDNSQueriesByDestination,
		Dims: module.Dims{
			{ID: "queries_cached", Name: "cached", Algo: module.Incremental},
			{ID: "queries_blocked", Name: "blocked", Algo: module.Incremental},
			{ID: "queries_forwarded", Name: "forwarded", Algo: module.Incremental},
		},
	}
	chartDNSQueriesByType = module.Chart{
		ID:       "dns_queries_by_type",
		Title:    "Pi-hole DNS Queries by Type",
		Units:    "queries/s",
		Fam:      "queries",
		Ctx:      "pihole.dns_queries_by_type",
		Type:     module.Stacked,
		Priority: prioDNSQueriesByType,
		Dims: module.Dims{
			{ID: "queries_types_A", Name: "A", Algo: module.Incremental},
			{ID: "queries_types_AAAA", Name: "AAAA", Algo: module.Incremental},
			{ID: "queries_types_ANY", Name: "ANY", Algo: module.Incremental},
			{ID: "queries_types_SRV", Name: "SRV", Algo: module.Incremental},
			{ID: "queries_types_SOA", Name: "SOA", Algo: module.Incremental},
			{ID: "queries_types_PTR", Name: "PTR", Algo: module.Incremental},
			{ID: "queries_types_TXT", Name: "TXT", Algo: module.Incremental},
			{ID: "queries_types_NAPTR", Name: "NAPTR", Algo: module.Incremental},
			{ID: "queries_types_MX", Name: "MX", Algo: module.Incremental},
			{ID: "queries_types_DS", Name: "DS", Algo: module.Incremental},
			{ID: "queries_types_RRSIG", Name: "RRSIG", Algo: module.Incremental},
			{ID: "queries_types_DNSKEY", Name: "DNSKEY", Algo: module.Incremental},
			{ID: "queries_types_NS", Name: "NS", Algo: module.Incremental},
			{ID: "queries_types_SVCB", Name: "SVCB", Algo: module.Incremental},
			{ID: "queries_types_HTTPS", Name: "HTTPS", Algo: module.Incremental},
			{ID: "queries_types_OTHER", Name: "OTHER", Algo: module.Incremental},
		},
	}
	chartDNSQueriesByStatus = module.Chart{
		ID:       "dns_queries_by_status",
		Title:    "Pi-hole DNS Queries by Status",
		Units:    "queries/s",
		Fam:      "queries",
		Ctx:      "pihole.dns_queries_by_status",
		Type:     module.Stacked,
		Priority: prioDNSQueriesByStatus,
		Dims: module.Dims{
			{ID: "queries_status_UNKNOWN", Name: "UNKNOWN", Algo: module.Incremental},
			{ID: "queries_status_GRAVITY", Name: "GRAVITY", Algo: module.Incremental},
			{ID: "queries_status_FORWARDED", Name: "FORWARDED", Algo: module.Incremental},
			{ID: "queries_status_CACHE", Name: "CACHE", Algo: module.Incremental},
			{ID: "queries_status_REGEX", Name: "REGEX", Algo: module.Incremental},
			{ID: "queries_status_DENYLIST", Name: "DENYLIST", Algo: module.Incremental},
			{ID: "queries_status_EXTERNAL_BLOCKED_IP", Name: "EXTERNAL_BLOCKED_IP", Algo: module.Incremental},
			{ID: "queries_status_EXTERNAL_BLOCKED_NULL", Name: "EXTERNAL_BLOCKED_NULL", Algo: module.Incremental},
			{ID: "queries_status_EXTERNAL_BLOCKED_NXRA", Name: "EXTERNAL_BLOCKED_NXRA", Algo: module.Incremental},
			{ID: "queries_status_GRAVITY_CNAME", Name: "GRAVITY_CNAME", Algo: module.Incremental},
			{ID: "queries_status_REGEX_CNAME", Name: "REGEX_CNAME", Algo: module.Incremental},
			{ID: "queries_status_DENYLIST_CNAME", Name: "DENYLIST_CNAME", Algo: module.Incremental},
			{ID: "queries_status_RETRIED", Name: "RETRIED", Algo: module.Incremental},
			{ID: "queries_status_RETRIED_DNSSEC", Name: "RETRIED_DNSSEC", Algo: module.Incremental},
			{ID: "queries_status_IN_PROGRESS", Name: "IN_PROGRESS", Algo: module.Incremental},
			{ID: "queries_status_DBBUSY", Name: "DBBUSY", Algo: module.Incremental},
			{ID: "queries_status_SPECIAL_DOMAIN", Name: "SPECIAL_DOMAIN", Algo: module.Incremental},
			{ID: "queries_status_CACHE_STALE", Name: "CACHE_STALE", Algo: module.Incremental},
			{ID: "queries_status_EXTERNAL_BLOCKED_EDE15", Name: "EXTERNAL_BLOCKED_EDE15", Algo: module.Incremental},
		},
	}

	chartDNSRepliesByType = module.Chart{
		ID:       "dns_replies_by_type",
		Title:    "Pi-hole DNS Replies by Type",
		Units:    "replies/s",
		Fam:      "replies",
		Ctx:      "pihole.dns_replies_by_type",
		Type:     module.Stacked,
		Priority: prioDNSRepliesByType,
		Dims: module.Dims{
			{ID: "queries_replies_UNKNOWN", Name: "UNKNOWN", Algo: module.Incremental},
			{ID: "queries_replies_NODATA", Name: "NODATA", Algo: module.Incremental},
			{ID: "queries_replies_NXDOMAIN", Name: "NXDOMAIN", Algo: module.Incremental},
			{ID: "queries_replies_CNAME", Name: "CNAME", Algo: module.Incremental},
			{ID: "queries_replies_IP", Name: "IP", Algo: module.Incremental},
			{ID: "queries_replies_DOMAIN", Name: "DOMAIN", Algo: module.Incremental},
			{ID: "queries_replies_RRNAME", Name: "RRNAME", Algo: module.Incremental},
			{ID: "queries_replies_SERVFAIL", Name: "SERVFAIL", Algo: module.Incremental},
			{ID: "queries_replies_REFUSED", Name: "REFUSED", Algo: module.Incremental},
			{ID: "queries_replies_NOTIMP", Name: "NOTIMP", Algo: module.Incremental},
			{ID: "queries_replies_DNSSEC", Name: "DNSSEC", Algo: module.Incremental},
			{ID: "queries_replies_NONE", Name: "NONE", Algo: module.Incremental},
			{ID: "queries_replies_OTHER", Name: "OTHER", Algo: module.Incremental},
		},
	}
)

var (
	chartActiveClients = module.Chart{
		ID:       "active_clients",
		Title:    "Pi-hole Active Clients (Seen in the Last 24 Hours)",
		Units:    "clients",
		Fam:      "clients",
		Ctx:      "pihole.active_clients",
		Priority: prioActiveClients,
		Dims: module.Dims{
			{ID: "clients_active", Name: "active"},
		},
	}
)

var (
	chartGravityListBlockedDomains = module.Chart{
		ID:       "gravity_list_blocked_domains",
		Title:    "Pi-hole Gravity List Blocked Domains",
		Units:    "domains",
		Fam:      "blocklist",
		Ctx:      "pihole.gravity_list_blocked_domains",
		Priority: prioGravityListBlockedDomains,
		Dims: module.Dims{
			{ID: "gravity_domains_being_blocked", Name: "blocked"},
		},
	}
	chartGravityListLastUpdateTimeAgo = module.Chart{
		ID:       "gravity_list_last_update_time_ago",
		Title:    "Pi-hole Gravity List Time Since Last Update",
		Units:    "seconds",
		Fam:      "blocklist",
		Ctx:      "pihole.gravity_list_last_update_time_ago",
		Priority: prioGravityListLastUpdateTimeAgo,
		Dims: module.Dims{
			{ID: "gravity_last_update_seconds_ago", Name: "last_update_ago"},
		},
	}
)
