// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostics

import (
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

const KindNormal = "normal"

// NormalDevice is a self-contained cut of one accepted runtime. Operation keys
// are local to this runtime; Producer.RunID and RuntimeID qualify them globally.
type NormalDevice struct {
	RegistrationID uint64                    `json:"registration_id"`
	RuntimeID      uint64                    `json:"runtime_id"`
	Hostname       string                    `json:"hostname"`
	CapturedAt     time.Time                 `json:"captured_at"`
	Initialization []SourceRef               `json:"initialization_sources,omitempty"`
	Device         DeviceInput               `json:"device"`
	ProfileContext ddsnmp.ProfileContextData `json:"profile_context"`
	Latest         *NormalAttempt            `json:"latest_attempt"`
	LastFailure    *NormalAttempt            `json:"last_failed_attempt,omitempty"`
	Sources        []*ddsnmp.SourceOperation `json:"source_operations,omitempty"`
}

type SourceRef struct {
	ContextID uint64 `json:"context_id"`
	Operation uint64 `json:"operation"`
}

type NormalAttempt struct {
	BGP            NormalBGP                                  `json:"bgp_cache"`
	Licensing      NormalLicensing                            `json:"licensing_cache"`
	ID             uint64                                     `json:"id"`
	Phase          string                                     `json:"phase"`
	StartedAt      time.Time                                  `json:"started_at"`
	CompletedAt    time.Time                                  `json:"completed_at"`
	Failed         bool                                       `json:"failed"`
	Failure        snmputils.Failure                          `json:"failure"`
	Failures       ddsnmp.CollectionFailures                  `json:"failures"`
	Sources        []SourceRef                                `json:"sources,omitempty"`
	Profiles       []NormalProfile                            `json:"profiles,omitempty"`
	Caches         []NormalCache                              `json:"cache_inputs,omitempty"`
	NegativeCauses []ddsnmpcollector.AcquisitionNegativeCause `json:"negative_causes,omitempty"`
	Metrics        []NormalMetricDecision                     `json:"metric_decisions,omitempty"`
	Samples        map[string]int64                           `json:"samples,omitempty"`
}

type NormalProfile struct {
	Source        string                                   `json:"source"`
	Acquisition   ddsnmpcollector.AcquisitionProfileReport `json:"acquisition"`
	Tags          map[string]string                        `json:"tags,omitempty"`
	Metadata      map[string]ddsnmp.MetaTag                `json:"metadata,omitempty"`
	Metrics       []NormalMetric                           `json:"metrics,omitempty"`
	HiddenMetrics []NormalMetric                           `json:"hidden_metrics,omitempty"`
	BGPRows       []ddsnmp.BGPRow                          `json:"bgp_rows,omitempty"`
	BGPFailed     bool                                     `json:"bgp_failed"`
	LicenseRows   []ddsnmp.LicenseRow                      `json:"license_rows,omitempty"`
}

// Cache content is the exact committed structure/tag generation. Its source
// references join the shared operation table rather than repeating raw PDUs.
type NormalCache struct {
	Kind      string                       `json:"kind"`
	Profile   string                       `json:"profile"`
	RootOID   string                       `json:"root_oid,omitempty"`
	ConfigID  string                       `json:"config_id,omitempty"`
	CreatedAt time.Time                    `json:"created_at"`
	ExpiresAt time.Time                    `json:"expires_at"`
	Marker    bool                         `json:"marker"`
	OIDs      map[string]map[string]string `json:"oids,omitempty"`
	TableTags map[string]map[string]string `json:"table_tags,omitempty"`
	Tags      map[string]string            `json:"tags,omitempty"`
	Metadata  map[string]ddsnmp.MetaTag    `json:"metadata,omitempty"`
	Sources   []SourceRef                  `json:"sources,omitempty"`
}

type NormalMetricDecision struct {
	Metric    NormalMetric `json:"metric"`
	Action    string       `json:"action"`
	SampleIDs []string     `json:"sample_ids,omitempty"`
}

type NormalBGP struct {
	Enabled         bool                   `json:"enabled"`
	LastUpdate      time.Time              `json:"last_update"`
	LastFailure     time.Time              `json:"last_failure"`
	StaleAfterNanos int64                  `json:"stale_after_ns"`
	Entries         []NormalBGPPeer        `json:"entries,omitempty"`
	Sources         map[string][]SourceRef `json:"sources,omitempty"`
}

type NormalLicensing struct {
	Enabled      bool            `json:"enabled"`
	NormalizedAt time.Time       `json:"normalized_at"`
	Rows         []NormalLicense `json:"rows,omitempty"`
	Sources      []SourceRef     `json:"sources,omitempty"`
}
