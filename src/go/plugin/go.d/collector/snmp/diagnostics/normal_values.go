// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostics

import (
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

type NormalMetric struct {
	IsVirtual   bool                                  `json:"is_virtual"`
	Profile     string                                `json:"profile"`
	Name        string                                `json:"name"`
	Description string                                `json:"description"`
	Family      string                                `json:"family"`
	Unit        string                                `json:"unit"`
	ChartType   string                                `json:"chart_type"`
	MetricType  ddprofiledefinition.ProfileMetricType `json:"metric_type"`
	StaticTags  map[string]string                     `json:"static_tags"`
	Tags        map[string]string                     `json:"tags"`
	Table       string                                `json:"table"`
	Value       int64                                 `json:"value"`
	MultiValue  map[string]int64                      `json:"multi_value"`
	IsTable     bool                                  `json:"is_table"`
}

type NormalLicense struct {
	ID                   string  `json:"id"`
	StructuralID         string  `json:"structural_id"`
	Source               string  `json:"source"`
	Table                string  `json:"table"`
	Name                 string  `json:"name"`
	Feature              string  `json:"feature"`
	Component            string  `json:"component"`
	Type                 string  `json:"type"`
	Impact               string  `json:"impact"`
	StateRaw             string  `json:"state_raw"`
	StateSeverity        int64   `json:"state_severity"`
	HasState             bool    `json:"has_state"`
	StateBucket          string  `json:"state_bucket"`
	ExpiryTS             int64   `json:"expiry_ts"`
	HasExpiry            bool    `json:"has_expiry"`
	AuthorizationExpiry  int64   `json:"authorization_expiry"`
	HasAuthorizationTime bool    `json:"has_authorization_time"`
	CertificateExpiry    int64   `json:"certificate_expiry"`
	HasCertificateTime   bool    `json:"has_certificate_time"`
	GraceExpiry          int64   `json:"grace_expiry"`
	HasGraceTime         bool    `json:"has_grace_time"`
	Usage                int64   `json:"usage"`
	HasUsage             bool    `json:"has_usage"`
	Capacity             int64   `json:"capacity"`
	HasCapacity          bool    `json:"has_capacity"`
	Available            int64   `json:"available"`
	HasAvailable         bool    `json:"has_available"`
	UsagePercent         float64 `json:"usage_percent"`
	HasUsagePct          bool    `json:"has_usage_pct"`
	IsUnlimited          bool    `json:"is_unlimited"`
	IsPerpetual          bool    `json:"is_perpetual"`
	ExpirySource         string  `json:"expiry_source"`
	AuthSource           string  `json:"auth_source"`
	CertSource           string  `json:"cert_source"`
	GraceSource          string  `json:"grace_source"`
}

type NormalBGPPeer struct {
	Key                  string            `json:"key"`
	Scope                string            `json:"scope"`
	Source               string            `json:"source"`
	Tags                 map[string]string `json:"tags"`
	Stale                bool              `json:"stale"`
	LastUpdate           time.Time         `json:"last_update"`
	LastFailure          time.Time         `json:"last_failure"`
	AdminStatus          string            `json:"admin_status"`
	State                string            `json:"state"`
	PreviousState        string            `json:"previous_state"`
	EstablishedUptime    *int64            `json:"established_uptime"`
	LastReceivedUpdate   *int64            `json:"last_received_update"`
	EstablishedCount     *int64            `json:"established_count"`
	DownTransitions      *int64            `json:"down_transitions"`
	UpTransitions        *int64            `json:"up_transitions"`
	Flaps                *int64            `json:"flaps"`
	LastErrorCode        *int64            `json:"last_error_code"`
	LastErrorSubcode     *int64            `json:"last_error_subcode"`
	LastErrorText        string            `json:"last_error_text"`
	LastDownReason       string            `json:"last_down_reason"`
	LastRecvNotify       string            `json:"last_recv_notify"`
	LastSentNotify       string            `json:"last_sent_notify"`
	GracefulRestart      string            `json:"graceful_restart"`
	UnavailabilityReason string            `json:"unavailability_reason"`
	UpdateCounts         map[string]int64  `json:"update_counts"`
	MessageCounts        map[string]int64  `json:"message_counts"`
	NotificationCounts   map[string]int64  `json:"notification_counts"`
	RouteRefreshCounts   map[string]int64  `json:"route_refresh_counts"`
	OpenCounts           map[string]int64  `json:"open_counts"`
	KeepaliveCounts      map[string]int64  `json:"keepalive_counts"`
	RouteCounts          map[string]int64  `json:"route_counts"`
	RouteTotals          map[string]int64  `json:"route_totals"`
	RouteLimits          map[string]int64  `json:"route_limits"`
	RouteLimitThresholds map[string]int64  `json:"route_limit_thresholds"`
}
