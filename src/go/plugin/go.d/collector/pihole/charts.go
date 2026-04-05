// SPDX-License-Identifier: GPL-3.0-or-later

package pihole

import (
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioDNSQueriesTotal = collectorapi.Priority + iota
	prioDNSQueriesBlockedPercent
	prioDNSQueriesByDestination
	prioDNSQueriesByType
	prioDNSQueriesByStatus
	prioDNSRepliesByType

	prioActiveClients

	prioGravityListBlockedDomains
	prioGravityListLastUpdateTimeAgo
)

var summaryCharts = collectorapi.Charts{
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
	chartDNSQueriesTotal = collectorapi.Chart{
		ID:       "dns_queries_total",
		Title:    "Pi-hole DNS Queries Total (Cached, Blocked and Forwarded)",
		Units:    "queries/s",
		Fam:      "queries",
		Ctx:      "pihole.dns_queries_total",
		Priority: prioDNSQueriesTotal,
		Dims: collectorapi.Dims{
			{ID: "queries_total", Name: "queries", Algo: collectorapi.Incremental},
		},
	}
	chartDNSQueriesBlockedPercent = collectorapi.Chart{
		ID:       "dns_queries_blocked_percent",
		Title:    "Pi-hole DNS Queries Blocked Percent",
		Units:    "percent",
		Fam:      "queries",
		Ctx:      "pihole.dns_queries_blocked_percent",
		Priority: prioDNSQueriesBlockedPercent,
		Dims: collectorapi.Dims{
			{ID: "queries_percent_blocked", Name: "blocked", Div: precision},
		},
	}
	chartDNSQueriesByDestination = collectorapi.Chart{
		ID:       "dns_queries_by_destination",
		Title:    "Pi-hole DNS Queries by Destination",
		Units:    "queries/s",
		Fam:      "queries",
		Ctx:      "pihole.dns_queries_by_destination",
		Type:     collectorapi.Stacked,
		Priority: prioDNSQueriesByDestination,
		Dims: collectorapi.Dims{
			{ID: "queries_cached", Name: "cached", Algo: collectorapi.Incremental},
			{ID: "queries_blocked", Name: "blocked", Algo: collectorapi.Incremental},
			{ID: "queries_forwarded", Name: "forwarded", Algo: collectorapi.Incremental},
		},
	}
	chartDNSQueriesByType = collectorapi.Chart{
		ID:       "dns_queries_by_type",
		Title:    "Pi-hole DNS Queries by Type",
		Units:    "queries/s",
		Fam:      "queries",
		Ctx:      "pihole.dns_queries_by_type",
		Type:     collectorapi.Stacked,
		Priority: prioDNSQueriesByType,
		Dims: collectorapi.Dims{
			{ID: "queries_types_A", Name: "A", Algo: collectorapi.Incremental},
			{ID: "queries_types_AAAA", Name: "AAAA", Algo: collectorapi.Incremental},
			{ID: "queries_types_ANY", Name: "ANY", Algo: collectorapi.Incremental},
			{ID: "queries_types_SRV", Name: "SRV", Algo: collectorapi.Incremental},
			{ID: "queries_types_SOA", Name: "SOA", Algo: collectorapi.Incremental},
			{ID: "queries_types_PTR", Name: "PTR", Algo: collectorapi.Incremental},
			{ID: "queries_types_TXT", Name: "TXT", Algo: collectorapi.Incremental},
			{ID: "queries_types_NAPTR", Name: "NAPTR", Algo: collectorapi.Incremental},
			{ID: "queries_types_MX", Name: "MX", Algo: collectorapi.Incremental},
			{ID: "queries_types_DS", Name: "DS", Algo: collectorapi.Incremental},
			{ID: "queries_types_RRSIG", Name: "RRSIG", Algo: collectorapi.Incremental},
			{ID: "queries_types_DNSKEY", Name: "DNSKEY", Algo: collectorapi.Incremental},
			{ID: "queries_types_NS", Name: "NS", Algo: collectorapi.Incremental},
			{ID: "queries_types_SVCB", Name: "SVCB", Algo: collectorapi.Incremental},
			{ID: "queries_types_HTTPS", Name: "HTTPS", Algo: collectorapi.Incremental},
			{ID: "queries_types_OTHER", Name: "OTHER", Algo: collectorapi.Incremental},
		},
	}
	chartDNSQueriesByStatus = collectorapi.Chart{
		ID:       "dns_queries_by_status",
		Title:    "Pi-hole DNS Queries by Status",
		Units:    "queries/s",
		Fam:      "queries",
		Ctx:      "pihole.dns_queries_by_status",
		Type:     collectorapi.Stacked,
		Priority: prioDNSQueriesByStatus,
		Dims: collectorapi.Dims{
			{ID: "queries_status_UNKNOWN", Name: "UNKNOWN", Algo: collectorapi.Incremental},
			{ID: "queries_status_GRAVITY", Name: "GRAVITY", Algo: collectorapi.Incremental},
			{ID: "queries_status_FORWARDED", Name: "FORWARDED", Algo: collectorapi.Incremental},
			{ID: "queries_status_CACHE", Name: "CACHE", Algo: collectorapi.Incremental},
			{ID: "queries_status_REGEX", Name: "REGEX", Algo: collectorapi.Incremental},
			{ID: "queries_status_DENYLIST", Name: "DENYLIST", Algo: collectorapi.Incremental},
			{ID: "queries_status_EXTERNAL_BLOCKED_IP", Name: "EXTERNAL_BLOCKED_IP", Algo: collectorapi.Incremental},
			{ID: "queries_status_EXTERNAL_BLOCKED_NULL", Name: "EXTERNAL_BLOCKED_NULL", Algo: collectorapi.Incremental},
			{ID: "queries_status_EXTERNAL_BLOCKED_NXRA", Name: "EXTERNAL_BLOCKED_NXRA", Algo: collectorapi.Incremental},
			{ID: "queries_status_GRAVITY_CNAME", Name: "GRAVITY_CNAME", Algo: collectorapi.Incremental},
			{ID: "queries_status_REGEX_CNAME", Name: "REGEX_CNAME", Algo: collectorapi.Incremental},
			{ID: "queries_status_DENYLIST_CNAME", Name: "DENYLIST_CNAME", Algo: collectorapi.Incremental},
			{ID: "queries_status_RETRIED", Name: "RETRIED", Algo: collectorapi.Incremental},
			{ID: "queries_status_RETRIED_DNSSEC", Name: "RETRIED_DNSSEC", Algo: collectorapi.Incremental},
			{ID: "queries_status_IN_PROGRESS", Name: "IN_PROGRESS", Algo: collectorapi.Incremental},
			{ID: "queries_status_DBBUSY", Name: "DBBUSY", Algo: collectorapi.Incremental},
			{ID: "queries_status_SPECIAL_DOMAIN", Name: "SPECIAL_DOMAIN", Algo: collectorapi.Incremental},
			{ID: "queries_status_CACHE_STALE", Name: "CACHE_STALE", Algo: collectorapi.Incremental},
			{ID: "queries_status_EXTERNAL_BLOCKED_EDE15", Name: "EXTERNAL_BLOCKED_EDE15", Algo: collectorapi.Incremental},
		},
	}

	chartDNSRepliesByType = collectorapi.Chart{
		ID:       "dns_replies_by_type",
		Title:    "Pi-hole DNS Replies by Type",
		Units:    "replies/s",
		Fam:      "replies",
		Ctx:      "pihole.dns_replies_by_type",
		Type:     collectorapi.Stacked,
		Priority: prioDNSRepliesByType,
		Dims: collectorapi.Dims{
			{ID: "queries_replies_UNKNOWN", Name: "UNKNOWN", Algo: collectorapi.Incremental},
			{ID: "queries_replies_NODATA", Name: "NODATA", Algo: collectorapi.Incremental},
			{ID: "queries_replies_NXDOMAIN", Name: "NXDOMAIN", Algo: collectorapi.Incremental},
			{ID: "queries_replies_CNAME", Name: "CNAME", Algo: collectorapi.Incremental},
			{ID: "queries_replies_IP", Name: "IP", Algo: collectorapi.Incremental},
			{ID: "queries_replies_DOMAIN", Name: "DOMAIN", Algo: collectorapi.Incremental},
			{ID: "queries_replies_RRNAME", Name: "RRNAME", Algo: collectorapi.Incremental},
			{ID: "queries_replies_SERVFAIL", Name: "SERVFAIL", Algo: collectorapi.Incremental},
			{ID: "queries_replies_REFUSED", Name: "REFUSED", Algo: collectorapi.Incremental},
			{ID: "queries_replies_NOTIMP", Name: "NOTIMP", Algo: collectorapi.Incremental},
			{ID: "queries_replies_DNSSEC", Name: "DNSSEC", Algo: collectorapi.Incremental},
			{ID: "queries_replies_NONE", Name: "NONE", Algo: collectorapi.Incremental},
			{ID: "queries_replies_OTHER", Name: "OTHER", Algo: collectorapi.Incremental},
		},
	}
)

var (
	chartActiveClients = collectorapi.Chart{
		ID:       "active_clients",
		Title:    "Pi-hole Active Clients (Seen in the Last 24 Hours)",
		Units:    "clients",
		Fam:      "clients",
		Ctx:      "pihole.active_clients",
		Priority: prioActiveClients,
		Dims: collectorapi.Dims{
			{ID: "clients_active", Name: "active"},
		},
	}
)

var (
	chartGravityListBlockedDomains = collectorapi.Chart{
		ID:       "gravity_list_blocked_domains",
		Title:    "Pi-hole Gravity List Blocked Domains",
		Units:    "domains",
		Fam:      "blocklist",
		Ctx:      "pihole.gravity_list_blocked_domains",
		Priority: prioGravityListBlockedDomains,
		Dims: collectorapi.Dims{
			{ID: "gravity_domains_being_blocked", Name: "blocked"},
		},
	}
	chartGravityListLastUpdateTimeAgo = collectorapi.Chart{
		ID:       "gravity_list_last_update_time_ago",
		Title:    "Pi-hole Gravity List Time Since Last Update",
		Units:    "seconds",
		Fam:      "blocklist",
		Ctx:      "pihole.gravity_list_last_update_time_ago",
		Priority: prioGravityListLastUpdateTimeAgo,
		Dims: collectorapi.Dims{
			{ID: "gravity_last_update_seconds_ago", Name: "last_update_ago"},
		},
	}
)
