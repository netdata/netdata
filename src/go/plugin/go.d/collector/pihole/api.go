// SPDX-License-Identifier: GPL-3.0-or-later

package pihole

const (
	// Version 6 (https://ftl.pi-hole.net/master/docs/)
	urlPathAPIAuth         = "/api/auth"
	urlPathAPIStatsSummary = "/api/stats/summary"
)

type ftlAPIAuthResponse struct {
	Session struct {
		Valid    bool   `json:"valid"`
		Sid      string `json:"sid"`
		Csrf     string `json:"csrf"`
		Validity int64  `json:"validity"`
		Message  string `json:"message"`
	}
}

// https://github.com/pi-hole/FTL/blob/master/src/api/stats.c#L113
type ftlAPIStatsSummaryResponse struct {
	Queries struct {
		Total          int64   `json:"total" stm:"total"`                            // Total number of queries
		Blocked        float64 `json:"blocked" stm:"blocked"`                        // Number of blocked queries
		PercentBlocked float64 `json:"percent_blocked" stm:"percent_blocked,1000,1"` // Percent of blocked queries
		UniqueDomains  int64   `json:"unique_domains" stm:"unique_domains"`          // Number of unique domains FTL knows
		Forwarded      int64   `json:"forwarded" stm:"forwarded"`                    // Number of queries that have been forwarded upstream
		Cached         int64   `json:"cached" stm:"cached"`                          // Number of queries replied to from cache or local configuration
		Frequency      float64 `json:"frequency" stm:"frequency,1000,1"`             // Average number of queries per second
		Types          struct {
			A      int64 `json:"A" stm:"A"`           // Type A queries
			AAAA   int64 `json:"AAAA" stm:"AAAA"`     // Type AAAA queries
			ANY    int64 `json:"ANY" stm:"ANY"`       // Type ANY queries
			SRV    int64 `json:"SRV" stm:"SRV"`       // Type SRV queries
			SOA    int64 `json:"SOA" stm:"SOA"`       // Type SOA queries
			PTR    int64 `json:"PTR" stm:"PTR"`       // Type PTR queries
			TXT    int64 `json:"TXT" stm:"TXT"`       // Type TXT queries
			NAPTR  int64 `json:"NAPTR" stm:"NAPTR"`   // Type NAPTR queries
			MX     int64 `json:"MX" stm:"MX"`         // Type MX queries
			DS     int64 `json:"DS" stm:"DS"`         // Type DS queries
			RRSIG  int64 `json:"RRSIG" stm:"RRSIG"`   // Type RRSIG queries
			DNSKEY int64 `json:"DNSKEY" stm:"DNSKEY"` // Type DNSKEY queries
			NS     int64 `json:"NS" stm:"NS"`         // Type NS queries
			SVCB   int64 `json:"SVCB" stm:"SVCB"`     // Type SVCB queries
			HTTPS  int64 `json:"HTTPS" stm:"HTTPS"`   // Type HTTPS queries
			OTHER  int64 `json:"OTHER" stm:"OTHER"`   // Queries of remaining types
		} `json:"types" stm:"types"` // Number of individual queries
		Status struct {
			Unknown              int64 `json:"UNKNOWN" stm:"UNKNOWN"`                               // Type UNKNOWN queries
			Gravity              int64 `json:"GRAVITY" stm:"GRAVITY"`                               // Type GRAVITY queries
			Forwarded            int64 `json:"FORWARDED" stm:"FORWARDED"`                           // Type FORWARDED queries
			Cache                int64 `json:"CACHE" stm:"CACHE"`                                   // Type CACHE queries
			Regex                int64 `json:"REGEX" stm:"REGEX"`                                   // Type REGEX queries
			DenyList             int64 `json:"DENYLIST" stm:"DENYLIST"`                             // Type DENYLIST queries
			ExternalBlockedIP    int64 `json:"EXTERNAL_BLOCKED_IP" stm:"EXTERNAL_BLOCKED_IP"`       // Type EXTERNAL_BLOCKED_IP queries
			ExternalBlockedNull  int64 `json:"EXTERNAL_BLOCKED_NULL" stm:"EXTERNAL_BLOCKED_NULL"`   // Type EXTERNAL_BLOCKED_NULL queries
			ExternalBlockedNxra  int64 `json:"EXTERNAL_BLOCKED_NXRA" stm:"EXTERNAL_BLOCKED_NXRA"`   // Type EXTERNAL_BLOCKED_NXRA queries
			GravityCname         int64 `json:"GRAVITY_CNAME" stm:"GRAVITY_CNAME"`                   // Type GRAVITY_CNAME queries
			RegexCname           int64 `json:"REGEX_CNAME" stm:"REGEX_CNAME"`                       // Type REGEX_CNAME queries
			DenyListCname        int64 `json:"DENYLIST_CNAME" stm:"DENYLIST_CNAME"`                 // Type DENYLIST_CNAME queries
			Retried              int64 `json:"RETRIED" stm:"RETRIED"`                               // Type RETRIED queries
			RetriedDnssec        int64 `json:"RETRIED_DNSSEC" stm:"RETRIED_DNSSEC"`                 // Type RETRIED_DNSSEC queries
			InProgress           int64 `json:"IN_PROGRESS" stm:"IN_PROGRESS"`                       // Type IN_PROGRESS queries
			Dbbusy               int64 `json:"DBBUSY" stm:"DBBUSY"`                                 // Type DBBUSY queries
			SpecialDomain        int64 `json:"SPECIAL_DOMAIN" stm:"SPECIAL_DOMAIN"`                 // Type SPECIAL_DOMAIN queries
			CacheStale           int64 `json:"CACHE_STALE" stm:"CACHE_STALE"`                       // Type CACHE_STALE queries
			ExternalBlockedEde15 int64 `json:"EXTERNAL_BLOCKED_EDE15" stm:"EXTERNAL_BLOCKED_EDE15"` // Type EXTERNAL_BLOCKED_EDE15 queries
		} `json:"status" stm:"status"` // Number of individual queries (by status)
		Replies struct {
			UNKNOWN  int64 `json:"UNKNOWN" stm:"UNKNOWN"`   // Type UNKNOWN replies
			NODATA   int64 `json:"NODATA" stm:"NODATA"`     // Type NODATA replies
			NXDOMAIN int64 `json:"NXDOMAIN" stm:"NXDOMAIN"` // Type NXDOMAIN replies
			CNAME    int64 `json:"CNAME" stm:"CNAME"`       // Type CNAME replies
			IP       int64 `json:"IP" stm:"IP"`             // Type IP replies
			DOMAIN   int64 `json:"DOMAIN" stm:"DOMAIN"`     // Type DOMAIN replies
			RRNAME   int64 `json:"RRNAME" stm:"RRNAME"`     // Type RRNAME replies
			SERVFAIL int64 `json:"SERVFAIL" stm:"SERVFAIL"` // Type SERVFAIL replies
			REFUSED  int64 `json:"REFUSED" stm:"REFUSED"`   // Type REFUSED replies
			NOTIMP   int64 `json:"NOTIMP" stm:"NOTIMP"`     // Type NOTIMP replies
			OTHER    int64 `json:"OTHER" stm:"OTHER"`       // Type OTHER replies
			DNSSEC   int64 `json:"DNSSEC" stm:"DNSSEC"`     // Type DNSSEC replies
			NONE     int64 `json:"NONE" stm:"NONE"`         // Type NONE replies
			BLOB     int64 `json:"BLOB" stm:"BLOB"`         // Type BLOB replies
		} `json:"replies" stm:"replies"` // Number of individual replies
	} `json:"queries" stm:"queries"`
	Clients struct {
		Active int64 `json:"active" stm:"active"` // Number of active clients (seen in the last 24 hours)
		Total  int64 `json:"total" stm:"total"`   // Total number of clients seen by FTL
	} `json:"clients" stm:"clients"`
	Gravity struct {
		DomainsBeingBlocked int64 `json:"domains_being_blocked" stm:"domains_being_blocked"` // Number of domain on your Pi-hole's gravity list
		LastUpdate          int64 `json:"last_update" stm:"last_update"`                     // Unix timestamp of last gravity update (may be `0` if unknown)
	} `json:"gravity" stm:"gravity"`
	Took *float64 `json:"took"` // Time in seconds it took to process the request
}

type ftlErrorResponse struct {
	Error struct {
		Key     string `json:"key"`
		Message string `json:"message"`
	} `json:"error"`
	Took float64 `json:"took"`
}
