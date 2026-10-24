// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ddprofiledefinition

import (
	"maps"
	"regexp"
	"slices"
	"text/template"
)

// ProfileMetricType overrides the type inferred from an SNMP value.
// By default metric type is derived from the type of the SNMP value, for example Counter32/64 -> rate.
type ProfileMetricType string

const (
	// ProfileMetricTypeGauge is used to create a gauge metric
	ProfileMetricTypeGauge ProfileMetricType = "gauge"

	// ProfileMetricTypeMonotonicCount displays the cumulative value without calculating a rate.
	ProfileMetricTypeMonotonicCount ProfileMetricType = "monotonic_count"

	// ProfileMetricTypeMonotonicCountAndRate is a legacy spelling rendered as a cumulative value.
	ProfileMetricTypeMonotonicCountAndRate ProfileMetricType = "monotonic_count_and_rate"

	// ProfileMetricTypeRate is used to create a rate metric
	ProfileMetricTypeRate ProfileMetricType = "rate"
)

// MetricsConfig holds configs for a metric
type MetricsConfig struct {
	// MIB the MIB used for this metric
	MIB string `yaml:"MIB,omitempty" json:"MIB,omitempty"`

	// Symbol configs
	Symbol SymbolConfig `yaml:"symbol,omitempty" json:"symbol"`

	// Table the table OID
	Table SymbolConfig `yaml:"table,omitempty"   json:"table"`
	// Table configs
	Symbols []SymbolConfig `yaml:"symbols,omitempty" json:"symbols,omitempty"`

	StaticTags []StaticMetricTagConfig `yaml:"static_tags,omitempty" json:"-"`
	MetricTags []MetricTagConfig       `yaml:"metric_tags,omitempty" json:"metric_tags,omitempty"`

	// DEPRECATED: Use .Symbol instead
	OID string `yaml:"OID,omitempty"         json:"OID,omitempty"`
	// DEPRECATED: Use .Symbol instead
	Name string `yaml:"name,omitempty"        json:"name,omitempty"`
	// Deprecated: set the type on the symbol. This row default also applies to table columns.
	MetricType ProfileMetricType `yaml:"metric_type,omitempty" json:"metric_type,omitempty"`
}

// Clone duplicates this MetricsConfig
func (m MetricsConfig) Clone() MetricsConfig {
	m.Table = m.Table.Clone()
	m.Symbol = m.Symbol.Clone()
	m.Symbols = cloneSlice(m.Symbols)
	m.StaticTags = slices.Clone(m.StaticTags)
	m.MetricTags = cloneSlice(m.MetricTags)
	return m
}

// IsColumn reports whether the row defines table columns.
func (m *MetricsConfig) IsColumn() bool {
	return len(m.Symbols) > 0
}

// IsScalar reports whether the row defines a scalar symbol.
func (m *MetricsConfig) IsScalar() bool {
	return m.Symbol.OID != "" && m.Symbol.Name != ""
}

// SymbolConfigCompat accepts a symbol object or a legacy symbol-name string.
// validateEnrichMetricTag moves a legacy tag-level OID into this symbol.
type SymbolConfigCompat SymbolConfig

// Clone creates a duplicate of this SymbolConfigCompat
func (s SymbolConfigCompat) Clone() SymbolConfigCompat {
	return SymbolConfigCompat(SymbolConfig(s).Clone())
}

// SymbolConfig holds info for a single symbol/oid
type SymbolConfig struct {
	OID  string `yaml:"OID,omitempty"  json:"OID,omitempty"`
	Name string `yaml:"name,omitempty" json:"name,omitempty"`

	ExtractValue         string         `yaml:"extract_value,omitempty" json:"extract_value,omitempty"`
	ExtractValueCompiled *regexp.Regexp `yaml:"-"                       json:"-"`

	MatchPattern         string         `yaml:"match_pattern,omitempty" json:"match_pattern,omitempty"`
	MatchValue           string         `yaml:"match_value,omitempty"   json:"match_value,omitempty"`
	MatchPatternCompiled *regexp.Regexp `yaml:"-"                       json:"-"`

	ScaleFactor float64 `yaml:"scale_factor,omitempty" json:"scale_factor,omitempty"`
	Format      string  `yaml:"format,omitempty"       json:"format,omitempty"`

	// MetricType overrides the type derived from the SNMP PDU.
	// Gauge and the monotonic-count spellings render absolute values; rate renders changes per second.
	MetricType ProfileMetricType `yaml:"metric_type,omitempty" json:"metric_type,omitempty"`

	ChartMeta ChartMeta `yaml:"chart_meta,omitempty" json:"chart_meta"`

	Mapping           MappingConfig      `yaml:"mapping,omitempty"   json:"mapping"`
	Transform         string             `yaml:"transform,omitempty" json:"transform,omitempty"`
	TransformCompiled *template.Template `yaml:"-"                   json:"-"`
}

// Clone creates a duplicate of this SymbolConfig
func (s SymbolConfig) Clone() SymbolConfig {
	ss := s
	ss.Mapping = ss.Mapping.Clone()
	return ss
}

type ChartMeta struct {
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Family      string `yaml:"family,omitempty"      json:"family,omitempty"`
	Unit        string `yaml:"unit,omitempty"        json:"unit,omitempty"`
	Type        string `yaml:"type,omitempty"        json:"type,omitempty"`
}

// MetricTagConfig holds metric tag info
type MetricTagConfig struct {
	Tag string `yaml:"tag" json:"tag"`

	// Table config
	Index uint `yaml:"index,omitempty" json:"index,omitempty"`

	Table string `yaml:"table,omitempty" json:"table,omitempty"`

	// DEPRECATED: Use .Symbol instead
	Column SymbolConfig `yaml:"column,omitempty" json:"-"`

	// DEPRECATED: use .Symbol instead
	OID string `yaml:"OID,omitempty"    json:"-"`
	// Symbol records the OID to read. Its Name supplies the tag name when Tag is empty.
	// If a serialized Symbol is a string
	// instead of an object, it will be treated like {name: <value>}; this use
	// pattern is deprecated
	Symbol SymbolConfigCompat `yaml:"symbol,omitempty" json:"symbol"`

	// LookupSymbol optionally resolves cross-table tags by matching a value from the
	// current row index against a column in the referenced table, then reading Symbol
	// from the matched row in that table.
	LookupSymbol SymbolConfigCompat `yaml:"lookup_symbol,omitempty" json:"lookup_symbol"`

	IndexTransform []MetricIndexTransform `yaml:"index_transform,omitempty" json:"index_transform,omitempty"`

	MappingRef string        `yaml:"mapping_ref,omitempty" json:"mapping_ref,omitempty"`
	Mapping    MappingConfig `yaml:"mapping,omitempty"     json:"mapping"`

	// Regex
	Match   string            `yaml:"match,omitempty" json:"-"`
	Tags    map[string]string `yaml:"tags,omitempty"  json:"-"`
	Pattern *regexp.Regexp    `yaml:"-"               json:"-"`
}

// Clone duplicates this MetricTagConfig
func (m MetricTagConfig) Clone() MetricTagConfig {
	m2 := m // non-pointer assignment shallow-copies members
	// deep copy symbols and structures
	m2.Column = m.Column.Clone()
	m2.Symbol = m.Symbol.Clone()
	m2.LookupSymbol = m.LookupSymbol.Clone()
	m2.IndexTransform = slices.Clone(m.IndexTransform)
	m2.Mapping = m.Mapping.Clone()
	m2.Tags = maps.Clone(m.Tags)
	return m2
}

type GlobalMetricTagConfig struct {
	MetricTagConfig `            yaml:",inline"             json:",inline"`
	Consumers       ConsumerSet `yaml:"consumers,omitempty" json:"consumers,omitempty"`
}

func (m GlobalMetricTagConfig) Clone() GlobalMetricTagConfig {
	return GlobalMetricTagConfig{
		MetricTagConfig: m.MetricTagConfig.Clone(),
		Consumers:       m.Consumers.Clone(),
	}
}

type StaticMetricTagConfig struct {
	Tag   string `yaml:"tag"   json:"tag"`
	Value string `yaml:"value" json:"value"`
}

// MetricIndexTransform holds configs for metric index transform
type MetricIndexTransform struct {
	Start     uint `yaml:"start"                json:"start"`
	End       uint `yaml:"end"                  json:"end"`
	DropRight uint `yaml:"drop_right,omitempty" json:"drop_right,omitempty"`
}

// IsIndexTag reports whether a normalized tag reads the row index instead of a column.
func (tagCfg MetricTagConfig) IsIndexTag() bool {
	if tagCfg.Index != 0 {
		return true
	}

	if tagCfg.Table != "" {
		return false
	}

	if tagCfg.Symbol.OID != "" {
		return false
	}

	return len(tagCfg.IndexTransform) > 0 ||
		tagCfg.Symbol.Format != "" ||
		tagCfg.Symbol.ExtractValue != "" ||
		tagCfg.Symbol.MatchPattern != "" ||
		tagCfg.Mapping.HasItems()
}
