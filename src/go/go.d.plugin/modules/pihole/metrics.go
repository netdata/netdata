// SPDX-License-Identifier: GPL-3.0-or-later

package pihole

type piholeMetrics struct {
	summary    *summaryRawMetrics   // ?summary
	queryTypes *queryTypesMetrics   // ?getQueryTypes
	forwarders *forwardDestinations // ?getForwardedDestinations
}

func (p piholeMetrics) hasSummary() bool {
	return p.summary != nil
}
func (p piholeMetrics) hasQueryTypes() bool {
	return p.queryTypes != nil
}
func (p piholeMetrics) hasForwarders() bool {
	return p.forwarders != nil && len(p.forwarders.Destinations) > 0
}

type piholeAPIVersion struct {
	Version int
}

type summaryRawMetrics struct {
	DomainsBeingBlocked int64   `json:"domains_being_blocked"`
	DNSQueriesToday     int64   `json:"dns_queries_today"`
	AdsBlockedToday     int64   `json:"ads_blocked_today"`
	AdsPercentageToday  float64 `json:"ads_percentage_today"`
	UniqueDomains       int64   `json:"unique_domains"`
	QueriesForwarded    int64   `json:"queries_forwarded"`
	QueriesCached       int64   `json:"queries_cached"`
	ClientsEverSeen     int64   `json:"clients_ever_seen"`
	UniqueClients       int64   `json:"unique_clients"`
	DNSQueriesAllTypes  int64   `json:"dns_queries_all_types"`
	ReplyNODATA         int64   `json:"reply_NODATA"`
	ReplyNXDOMAIN       int64   `json:"reply_NXDOMAIN"`
	ReplyCNAME          int64   `json:"reply_CNAME"`
	ReplyIP             int64   `json:"reply_IP"`
	PrivacyLevel        int64   `json:"privacy_level"`
	Status              string  `json:"status"`
	GravityLastUpdated  struct {
		// gravity.list has been removed (https://github.com/pi-hole/pi-hole/pull/2871#issuecomment-520251509)
		FileExists bool `json:"file_exists"`
		Absolute   *int64
	} `json:"gravity_last_updated"`
}

type queryTypesMetrics struct {
	Types struct {
		A    float64 `json:"A (IPv4)"`
		AAAA float64 `json:"AAAA (IPv6)"`
		ANY  float64
		SRV  float64
		SOA  float64
		PTR  float64
		TXT  float64
	} `json:"querytypes"`
}

// https://github.com/pi-hole/FTL/blob/6f69dd5b4ca60f925d68bfff3869350e934a7240/src/api/api.c#L474
type forwardDestinations struct {
	Destinations map[string]float64 `json:"forward_destinations"`
}

//type (
//	item map[string]int64
//
//	topClients struct {
//		Sources item `json:"top_sources"`
//	}
//	topItems struct {
//		TopQueries item `json:"top_queries"`
//		TopAds     item `json:"top_ads"`
//	}
//)
//
//func (i *item) UnmarshalJSON(data []byte) error {
//	if isEmptyArray(data) {
//		return nil
//	}
//	type plain *item
//	return json.Unmarshal(data, (plain)(i))
//}
