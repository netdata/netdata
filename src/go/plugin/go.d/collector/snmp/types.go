package snmp

import (
	"regexp"

	"github.com/gosnmp/gosnmp"
)

type snmpPDU struct {
	value       interface{}
	oid         string
	metric_type gosnmp.Asn1BER
}

type Profile struct {
	Extends     []string     `yaml:"extends"`
	SysObjectID SysObjectIDs `yaml:"sysobjectid"`
	Metadata    Metadata     `yaml:"metadata"`
	Metrics     []Metric     `yaml:"metrics"`
}

type SysObjectIDs []string

type Metadata struct {
	Device DeviceMetadata `yaml:"device"`
}

type DeviceMetadata struct {
	Fields map[string]Symbol `yaml:"fields"`
}

type Symbol struct {
	OID          string `yaml:"OID,omitempty"`
	Name         string `yaml:"name,omitempty"`
	MatchPattern string `yaml:"match_pattern,omitempty"`
	MatchValue   string `yaml:"match_value,omitempty"`
	ExtractValue string `yaml:"extract_value,omitempty"`
}

// superset of OIDMetric, SymbolMetric and TableMetric
type Metric struct {
	Name string `yaml:"name,omitempty"`
	OID  string `yaml:"OID,omitempty"`
	//TODO check for only name existing in metric tag, as there is some case for that
	MetricTags []TableMetricTag `yaml:"metric_tags,omitempty"`
	MetricType string           `yaml:"metric_type,omitempty"`
	Options    map[string]string

	MIB    string `yaml:"MIB,omitempty"`
	Symbol Symbol `yaml:"symbol,omitempty"` //can be either string or Symbol

	Table   interface{} `yaml:"table,omitempty"` // can be either a string or Symbol
	Symbols []Symbol    `yaml:"symbols,omitempty"`
}

type TableMetricTag struct {
	Index   int            `yaml:"index"`
	Mapping map[int]string `yaml:"mapping"`

	Tag string `yaml:"tag"`

	MIB            string       `yaml:"mib"`
	Symbol         Symbol       `yaml:"symbol"`
	Table          string       `yaml:"table"`
	IndexTransform []IndexSlice `yaml:"index_transform"`
}

type oidMetric struct {
	name       string
	oid        string
	metricTags []string
	forcedType string
	options    map[string]string
}

type symbolMetric struct {
	mib        string
	symbol     interface{} //can be either string or Symbol
	forcedType string
	metricTags []string
	options    map[string]string
}

type tableMetric struct {
	mib        string
	table      interface{} // can be either a string or Symbol
	symbols    []Symbol
	forcedType string
	metricTags []TableMetricTag
	options    map[string]string
}

type parsedResult struct {
	oids           []string
	next_oids      []string
	bulk_oids      []string
	parsed_metrics []parsedMetric
}

type tableBatchKey struct {
	mib   string
	table string
}

type tableBatch struct {
	tableOID string
	oids     []string
}

type tableBatches map[tableBatchKey]tableBatch

type indexTag struct {
	parsedMetricTag parsedMetricTag
	index           int
}

type columnTag struct {
	parsedMetricTag parsedMetricTag
	column          string
	indexSlices     []IndexSlice
}

type indexMapping struct {
	tag     string
	index   int
	mapping map[int]string
}

type parsedSymbol struct {
	name                string
	oid                 string
	extractValuePattern *regexp.Regexp
	oidsToResolve       map[string]string
}

type parsedColumnMetricTag struct {
	oidsToResolve map[string]string
	tableBatches  tableBatches
	columnTags    []columnTag
}
type parsedIndexMetricTag struct {
	indexTags     []indexTag
	indexMappings map[int]map[string]string
}

type parsedTableMetricTag struct {
	oidsToResolve map[string]string
	tableBatches  tableBatches
	columnTags    []columnTag
	indexTags     []indexTag
	indexMappings map[int]map[int]string
}

type parsedSymbolMetric struct {
	name                string
	tags                []string
	forcedType          string
	enforceScalar       bool
	options             map[string]string
	extractValuePattern *regexp.Regexp
	baseoid             string //TODO consider changing this to OID, it will not have nested OIDs as it is a symbol
}

type parsedTableMetric struct {
	name                string
	indexTags           []indexTag
	columnTags          []columnTag
	forcedType          string
	options             map[string]string
	extractValuePattern *regexp.Regexp
	rowOID              string
	tableName           string
	tableOID            string
}

// union of two above
type parsedMetric any

// Not supported yet
/*type parsedSimpleMetricTag struct {
	name string
}

type parsedMatchMetricTag struct {
tags    []string
symbol  Symbol
pattern *regexp.Regexp
}

type symbolTag struct {
	parsedMetricTag parsedMetricTag
	symbol          string
}

type parsedSymbolTagsResult struct {
	oids             []string
	parsedSymbolTags []symbolTag
}
*/
type parsedMetricTag struct {
	name string

	tags    []string
	pattern *regexp.Regexp
	// symbol  Symbol not used yet
}

type metricParseResult struct {
	oidsToFetch   []string
	oidsToResolve map[string]string
	indexMappings []indexMapping
	tableBatches  tableBatches
	parsedMetrics []parsedMetric
}

type MetricTag struct {
	OID    string
	MIB    string
	Symbol Symbol
	// simple tag
	Tag string
	// regex matching
	Match string
	Tags  []string
}

type IndexSlice struct {
	Start int
	End   int
}

type processedMetric struct {
	oid         string
	name        string
	value       interface{}
	metric_type gosnmp.Asn1BER
	tableName   string
}
