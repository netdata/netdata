package snmp

import (
	"regexp"

	"github.com/gosnmp/gosnmp"
)

type snmpPDU struct {
	value      interface{}
	oid        string
	metricType gosnmp.Asn1BER
}

type SysObjectIDs []string

type parsedResult struct {
	OIDs          []string
	nextOIDs      []string
	bulkOIDs      []string
	parsedMetrics []parsedMetric
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
	unit                string
	description         string
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

type IndexSlice struct {
	Start int
	End   int
}

type processedMetric struct {
	oid         string
	name        string
	value       interface{}
	metricType  gosnmp.Asn1BER
	tableName   string
	unit        string
	description string
}
