package ddsnmp

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

type ProfileMetrics struct {
	Source         string
	DeviceMetadata map[string]MetaTag
	Tags           map[string]string
	Metrics        []Metric
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
