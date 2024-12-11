// SPDX-License-Identifier: GPL-3.0-or-later

package pihole

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioDNSQueriesTotal = module.Priority + iota
	prioDNSQueries
	prioDNSQueriesPerc
	prioUniqueClients
	prioDomainsOnBlocklist
	prioBlocklistLastUpdate
	prioUnwantedDomainsBlockingStatus

	prioDNSQueriesTypes
	prioDNSQueriesForwardedDestination
)

var baseCharts = module.Charts{
	chartDNSQueriesTotal.Copy(),
	chartDNSQueries.Copy(),
	chartDNSQueriesPerc.Copy(),
	chartUniqueClients.Copy(),
	chartDomainsOnBlocklist.Copy(),
	chartBlocklistLastUpdate.Copy(),
	chartUnwantedDomainsBlockingStatus.Copy(),
}

var (
	chartDNSQueriesTotal = module.Chart{
		ID:       "dns_queries_total",
		Title:    "DNS Queries Total (Cached, Blocked and Forwarded)",
		Units:    "queries",
		Fam:      "queries",
		Ctx:      "pihole.dns_queries_total",
		Priority: prioDNSQueriesTotal,
		Dims: module.Dims{
			{ID: "dns_queries_today", Name: "queries"},
		},
	}
	chartDNSQueries = module.Chart{
		ID:       "dns_queries",
		Title:    "DNS Queries",
		Units:    "queries",
		Fam:      "queries",
		Ctx:      "pihole.dns_queries",
		Type:     module.Stacked,
		Priority: prioDNSQueries,
		Dims: module.Dims{
			{ID: "queries_cached", Name: "cached"},
			{ID: "ads_blocked_today", Name: "blocked"},
			{ID: "queries_forwarded", Name: "forwarded"},
		},
	}
	chartDNSQueriesPerc = module.Chart{
		ID:       "dns_queries_percentage",
		Title:    "DNS Queries Percentage",
		Units:    "percentage",
		Fam:      "queries",
		Ctx:      "pihole.dns_queries_percentage",
		Type:     module.Stacked,
		Priority: prioDNSQueriesPerc,
		Dims: module.Dims{
			{ID: "queries_cached_perc", Name: "cached", Div: precision},
			{ID: "ads_blocked_today_perc", Name: "blocked", Div: precision},
			{ID: "queries_forwarded_perc", Name: "forwarded", Div: precision},
		},
	}
	chartUniqueClients = module.Chart{
		ID:       "unique_clients",
		Title:    "Unique Clients",
		Units:    "clients",
		Fam:      "clients",
		Ctx:      "pihole.unique_clients",
		Priority: prioUniqueClients,
		Dims: module.Dims{
			{ID: "unique_clients", Name: "unique"},
		},
	}
	chartDomainsOnBlocklist = module.Chart{
		ID:       "domains_on_blocklist",
		Title:    "Domains On Blocklist",
		Units:    "domains",
		Fam:      "blocklist",
		Ctx:      "pihole.domains_on_blocklist",
		Priority: prioDomainsOnBlocklist,
		Dims: module.Dims{
			{ID: "domains_being_blocked", Name: "blocklist"},
		},
	}
	chartBlocklistLastUpdate = module.Chart{
		ID:       "blocklist_last_update",
		Title:    "Blocklist Last Update",
		Units:    "seconds",
		Fam:      "blocklist",
		Ctx:      "pihole.blocklist_last_update",
		Priority: prioBlocklistLastUpdate,
		Dims: module.Dims{
			{ID: "blocklist_last_update", Name: "ago"},
		},
	}
	chartUnwantedDomainsBlockingStatus = module.Chart{
		ID:       "unwanted_domains_blocking_status",
		Title:    "Unwanted Domains Blocking Status",
		Units:    "status",
		Fam:      "status",
		Ctx:      "pihole.unwanted_domains_blocking_status",
		Priority: prioUnwantedDomainsBlockingStatus,
		Dims: module.Dims{
			{ID: "blocking_status_enabled", Name: "enabled"},
			{ID: "blocking_status_disabled", Name: "disabled"},
		},
	}
)

var (
	chartDNSQueriesTypes = module.Chart{
		ID:       "dns_queries_types",
		Title:    "DNS Queries Per Type",
		Units:    "percentage",
		Fam:      "doQuery types",
		Ctx:      "pihole.dns_queries_types",
		Type:     module.Stacked,
		Priority: prioDNSQueriesTypes,
		Dims: module.Dims{
			{ID: "A", Div: 100},
			{ID: "AAAA", Div: 100},
			{ID: "ANY", Div: 100},
			{ID: "PTR", Div: 100},
			{ID: "SOA", Div: 100},
			{ID: "SRV", Div: 100},
			{ID: "TXT", Div: 100},
		},
	}
	chartDNSQueriesForwardedDestination = module.Chart{
		ID:       "dns_queries_forwarded_destination",
		Title:    "DNS Queries Per Destination",
		Units:    "percentage",
		Fam:      "queries answered by",
		Ctx:      "pihole.dns_queries_forwarded_destination",
		Type:     module.Stacked,
		Priority: prioDNSQueriesForwardedDestination,
		Dims: module.Dims{
			{ID: "destination_cached", Name: "cached", Div: 100},
			{ID: "destination_blocked", Name: "blocked", Div: 100},
			{ID: "destination_other", Name: "other", Div: 100},
		},
	}
)

func (c *Collector) addChartDNSQueriesType() {
	chart := chartDNSQueriesTypes.Copy()
	if err := c.Charts().Add(chart); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addChartDNSQueriesForwardedDestinations() {
	chart := chartDNSQueriesForwardedDestination.Copy()
	if err := c.Charts().Add(chart); err != nil {
		c.Warning(err)
	}
}
