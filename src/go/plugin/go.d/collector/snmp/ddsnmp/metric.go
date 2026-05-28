package ddsnmp

import (
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

type ProfileMetrics struct {
	Source          string
	DeviceMetadata  map[string]MetaTag
	Tags            map[string]string
	Metrics         []Metric
	TopologyMetrics []Metric
	LicenseRows     []LicenseRow
	BGPRows         []BGPRow
	BGPCollectError error
	HiddenMetrics   []Metric
	Stats           CollectionStats
}

type Metric struct {
	Profile     *ProfileMetrics
	Name        string
	Description string
	Family      string
	Unit        string
	ChartType   string
	MetricType  ddprofiledefinition.ProfileMetricType
	StaticTags  map[string]string
	Tags        map[string]string
	Table       string
	Value       int64
	MultiValue  map[string]int64

	TopologyKind ddprofiledefinition.TopologyKind
	IsTable      bool
}

type MetaTag struct {
	Value        string
	IsExactMatch bool // whether this value is from an exact match context
}

type LicenseRow struct {
	OriginProfileID string
	TableOID        string
	Table           string
	RowKey          string
	StructuralID    string

	ID        string
	Name      string
	Feature   string
	Component string
	Type      string
	Impact    string

	IsPerpetual bool
	IsUnlimited bool

	State         LicenseState
	Expiry        LicenseTimer
	Authorization LicenseTimer
	Certificate   LicenseTimer
	Grace         LicenseTimer
	Usage         LicenseUsage

	Tags map[string]string
}

type LicenseState struct {
	Has       bool
	Severity  int64
	Raw       string
	Policy    ddprofiledefinition.LicenseStatePolicy
	SourceOID string
}

type LicenseTimer struct {
	Has              bool
	Timestamp        int64
	RemainingSeconds int64
	SourceOID        string
}

type LicenseUsage struct {
	HasUsed      bool
	Used         int64
	HasCapacity  bool
	Capacity     int64
	HasAvailable bool
	Available    int64
	HasPercent   bool
	Percent      int64
}

type BGPRow struct {
	OriginProfileID string
	Kind            ddprofiledefinition.BGPRowKind
	TableOID        string
	Table           string
	RowKey          string
	StructuralID    string

	Identity    BGPIdentity
	Descriptors BGPDescriptors
	Admin       BGPAdmin
	State       BGPState
	Previous    BGPState
	Connection  BGPConnection
	Traffic     BGPTraffic
	Transitions BGPTransitions
	Timers      BGPTimers
	LastError   BGPLastError
	LastNotify  BGPLastNotifications
	Reasons     BGPReasons
	Restart     BGPGracefulRestart
	Routes      BGPRoutes
	RouteLimits BGPRouteLimits
	Device      BGPDeviceCounts
	Tags        map[string]string
}

type BGPIdentity struct {
	RoutingInstance         string
	Neighbor                string
	RemoteAS                string
	AddressFamily           ddprofiledefinition.BGPAddressFamily
	SubsequentAddressFamily ddprofiledefinition.BGPSubsequentAddressFamily
}

type BGPDescriptors struct {
	LocalAddress    string
	LocalAS         string
	LocalIdentifier string
	PeerIdentifier  string
	PeerType        string
	BGPVersion      string
	Description     string
}

type BGPState struct {
	Has       bool
	State     ddprofiledefinition.BGPPeerState
	Raw       string
	SourceOID string
}

type BGPAdmin struct {
	Enabled BGPBool
}

type BGPInt64 struct {
	Has       bool
	Value     int64
	Raw       string
	SourceOID string
}

type BGPText struct {
	Has       bool
	Value     string
	Raw       string
	SourceOID string
}

type BGPBool struct {
	Has       bool
	Value     bool
	Raw       string
	SourceOID string
}

type BGPConnection struct {
	EstablishedUptime     BGPInt64
	LastReceivedUpdateAge BGPInt64
}

type BGPDirectional struct {
	Received BGPInt64
	Sent     BGPInt64
}

type BGPTraffic struct {
	Messages       BGPDirectional
	Updates        BGPDirectional
	Notifications  BGPDirectional
	RouteRefreshes BGPDirectional
	Opens          BGPDirectional
	Keepalives     BGPDirectional
}

type BGPTransitions struct {
	Established BGPInt64
	Down        BGPInt64
	Up          BGPInt64
	Flaps       BGPInt64
}

type BGPTimers struct {
	Negotiated BGPTimerPair
	Configured BGPTimerPair
}

type BGPTimerPair struct {
	ConnectRetry                  BGPInt64
	HoldTime                      BGPInt64
	KeepaliveTime                 BGPInt64
	MinASOriginationInterval      BGPInt64
	MinRouteAdvertisementInterval BGPInt64
}

type BGPLastError struct {
	Code    BGPInt64
	Subcode BGPInt64
	Text    string
}

type BGPLastNotification struct {
	Code    BGPInt64
	Subcode BGPInt64
	Reason  BGPText
}

type BGPLastNotifications struct {
	Received BGPLastNotification
	Sent     BGPLastNotification
}

type BGPReasons struct {
	LastDown       BGPText
	Unavailability BGPText
}

type BGPGracefulRestart struct {
	State BGPText
}

type BGPRoutes struct {
	Current BGPRouteCounters
	Total   BGPRouteCounters
}

type BGPRouteCounters struct {
	Received   BGPInt64
	Accepted   BGPInt64
	Rejected   BGPInt64
	Active     BGPInt64
	Advertised BGPInt64
	Suppressed BGPInt64
	Withdrawn  BGPInt64
}

type BGPRouteLimits struct {
	Limit          BGPInt64
	Threshold      BGPInt64
	ClearThreshold BGPInt64
}

type BGPDeviceCounts struct {
	Peers         BGPInt64
	InternalPeers BGPInt64
	ExternalPeers BGPInt64
	ByState       map[ddprofiledefinition.BGPPeerState]int64
	ByStateHas    bool
}

// CollectionStats contains statistics for a single profile collection cycle.
type CollectionStats struct {
	Timing     TimingStats
	SNMP       SNMPOperationStats
	Metrics    MetricCountStats
	TableCache TableCacheStats
	Errors     ErrorStats
}

// TimingStats captures duration of each collection phase.
type TimingStats struct {
	// Scalar is time spent collecting scalar (non-table) metrics.
	Scalar time.Duration
	// Table is time spent collecting table metrics.
	Table time.Duration
	// Licensing is time spent collecting typed licensing rows.
	Licensing time.Duration
	// BGP is time spent collecting typed BGP rows.
	BGP time.Duration
	// VirtualMetrics is time spent computing derived/aggregated metrics.
	VirtualMetrics time.Duration
}

func (s TimingStats) Total() time.Duration {
	return s.Scalar + s.Table + s.Licensing + s.BGP + s.VirtualMetrics
}

// SNMPOperationStats captures SNMP protocol-level operations.
type SNMPOperationStats struct {
	// GetRequests is the number of SNMP GET operations performed.
	GetRequests int64
	// GetOIDs is the total number of OIDs requested across all GETs.
	GetOIDs int64
	// WalkRequests is the number of SNMP Walk/BulkWalk operations.
	WalkRequests int64
	// WalkPDUs is the total number of PDUs returned from all walks.
	WalkPDUs int64
	// TablesWalked is the count of tables that required walking.
	TablesWalked int64
	// TablesCached is the count of tables served from cache.
	TablesCached int64
}

// MetricCountStats captures the number of metrics produced.
type MetricCountStats struct {
	// Scalar is the count of scalar (non-table) metrics.
	Scalar int64
	// Table is the count of table metrics.
	Table int64
	// Virtual is the count of computed/derived metrics.
	Virtual int64
	// Licensing is the count of typed licensing rows produced.
	Licensing int64
	// BGP is the count of typed BGP rows produced.
	BGP int64
	// Tables is the count of unique regular metric tables. Typed licensing
	// and BGP rows are counted separately.
	Tables int64
	// Rows is the total number of regular metric table rows. Typed licensing
	// and BGP rows are counted separately.
	Rows int64
}

// TableCacheStats captures table cache performance.
type TableCacheStats struct {
	// Hits is the number of table configs served from cache.
	Hits int64
	// Misses is the number of table configs that required walking.
	Misses int64
}

// ErrorStats captures categorized error counts.
type ErrorStats struct {
	// SNMP is the count of SNMP-level errors (timeouts, network issues).
	SNMP int64
	// Processing is the count of value conversion/transform errors.
	Processing struct {
		Scalar    int64
		Table     int64
		Licensing int64
		BGP       int64
	}
	// MissingOIDs is the count of NoSuchObject/NoSuchName responses.
	MissingOIDs int64
}
