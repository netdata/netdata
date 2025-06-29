package ddsnmp

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

type ProfileMetrics struct {
	Source         string
	DeviceMetadata map[string]string
	Tags           map[string]string
	Metrics        []Metric
}

type Metric struct {
	Profile     *ProfileMetrics
	Name        string
	Description string
	Family      string
	Unit        string
	MetricType  ddprofiledefinition.ProfileMetricType
	StaticTags  map[string]string
	Tags        map[string]string
	Mappings    map[int64]string
	IsTable     bool
	Value       int64
}
