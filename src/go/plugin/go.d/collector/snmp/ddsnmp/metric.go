package ddsnmp

import (
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

type ProfileMetrics struct {
	Source         string
	DeviceMetadata map[string]MetaTag
	Tags           map[string]string
	Metrics        []Metric
	Stats          CollectionStats
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

	IsTable bool
}

type MetaTag struct {
	Value        string
	IsExactMatch bool // whether this value is from an exact match context
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
	// VirtualMetrics is time spent computing derived/aggregated metrics.
	VirtualMetrics time.Duration
}

func (s TimingStats) Total() time.Duration {
	return s.Scalar + s.Table + s.VirtualMetrics
}

// SNMPOperationStats captures SNMP protocol-level operations.
type SNMPOperationStats struct {
	// GetRequests is the number of SNMP GET operations performed.
	GetRequests int
	// GetOIDs is the total number of OIDs requested across all GETs.
	GetOIDs int
	// WalkRequests is the number of SNMP Walk/BulkWalk operations.
	WalkRequests int
	// WalkPDUs is the total number of PDUs returned from all walks.
	WalkPDUs int
	// TablesWalked is the count of tables that required walking.
	TablesWalked int
	// TablesCached is the count of tables served from cache.
	TablesCached int
}

// MetricCountStats captures the number of metrics produced.
type MetricCountStats struct {
	// Scalar is the count of scalar (non-table) metrics.
	Scalar int
	// Table is the count of table metrics.
	Table int
	// Virtual is the count of computed/derived metrics.
	Virtual int
	// Tables is the count of unique tables with metrics.
	Tables int
	// Rows is the total number of table rows across all tables.
	Rows int
}

// TableCacheStats captures table cache performance.
type TableCacheStats struct {
	// Hits is the number of table configs served from cache.
	Hits int
	// Misses is the number of table configs that required walking.
	Misses int
}

// ErrorStats captures categorized error counts.
type ErrorStats struct {
	// SNMP is the count of SNMP-level errors (timeouts, network issues).
	SNMP int
	// Processing is the count of value conversion/transform errors.
	Processing struct {
		Scalar int
		Table  int
	}
	// MissingOIDs is the count of NoSuchObject/NoSuchName responses.
	MissingOIDs int
}
