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

// ProfileMetricType metric type used to override default type of the metric
// By default metric type is derived from the type of the SNMP value, for example Counter32/64 -> rate.
type ProfileMetricType string

const (
	// ProfileMetricTypeGauge is used to create a gauge metric
	ProfileMetricTypeGauge ProfileMetricType = "gauge"

	// ProfileMetricTypeMonotonicCount is used to create a monotonic_count metric
	ProfileMetricTypeMonotonicCount ProfileMetricType = "monotonic_count"

	// ProfileMetricTypeMonotonicCountAndRate is used to create a monotonic_count and rate metric
	ProfileMetricTypeMonotonicCountAndRate ProfileMetricType = "monotonic_count_and_rate"

	// ProfileMetricTypeRate is used to create a rate metric
	ProfileMetricTypeRate ProfileMetricType = "rate"

	// ProfileMetricTypeFlagStream is used to create metric based on a value that represent flags
	// See details in https://github.com/DataDog/integrations-core/pull/7072
	ProfileMetricTypeFlagStream ProfileMetricType = "flag_stream"

	// ProfileMetricTypeCounter is DEPRECATED
	// `counter` is deprecated in favour of `rate`
	ProfileMetricTypeCounter ProfileMetricType = "counter"

	// ProfileMetricTypePercent is DEPRECATED
	// `percent` is deprecated in favour of `scale_factor`
	ProfileMetricTypePercent ProfileMetricType = "percent"
)

// MetricsConfig holds configs for a metric
type MetricsConfig struct {
	// MIB the MIB used for this metric
	MIB string `yaml:"MIB,omitempty" json:"MIB,omitempty"`

	// Symbol configs
	Symbol SymbolConfig `yaml:"symbol,omitempty" json:"symbol,omitempty"`

	// Table the table OID
	Table SymbolConfig `yaml:"table,omitempty" json:"table,omitempty"`
	// Table configs
	Symbols []SymbolConfig `yaml:"symbols,omitempty" json:"symbols,omitempty"`

	// `static_tags` is not exposed as json at the moment since we need to evaluate if we want to expose it via UI
	StaticTags []StaticMetricTagConfig `yaml:"static_tags,omitempty" json:"-"`
	MetricTags MetricTagConfigList     `yaml:"metric_tags,omitempty" json:"metric_tags,omitempty"`

	Options MetricsConfigOption `yaml:"options,omitempty" json:"options,omitempty"`

	// DEPRECATED: Use .Symbol instead
	OID string `yaml:"OID,omitempty" json:"OID,omitempty" jsonschema:"-"`
	// DEPRECATED: Use .Symbol instead
	Name string `yaml:"name,omitempty" json:"name,omitempty" jsonschema:"-"`
	// DEPRECATED: use Symbol.MetricType instead.
	MetricType ProfileMetricType `yaml:"metric_type,omitempty" json:"metric_type,omitempty" jsonschema:"-"`
}

// Clone duplicates this MetricsConfig
func (m MetricsConfig) Clone() MetricsConfig {
	return MetricsConfig{
		MIB:        m.MIB,
		Table:      m.Table.Clone(),
		Symbol:     m.Symbol.Clone(),
		Symbols:    cloneSlice(m.Symbols),
		StaticTags: slices.Clone(m.StaticTags),
		MetricTags: cloneSlice(m.MetricTags),
		Options:    m.Options,

		OID:        m.OID,
		Name:       m.Name,
		MetricType: m.MetricType,
	}
}

// IsColumn returns true if the metrics config define columns metrics
func (m *MetricsConfig) IsColumn() bool {
	return len(m.Symbols) > 0
}

// IsScalar returns true if the metrics config define scalar metrics
func (m *MetricsConfig) IsScalar() bool {
	return m.Symbol.OID != "" && m.Symbol.Name != ""
}

// SymbolConfigCompat is used to deserialize string field or SymbolConfig.
// For OID/Name to Symbol harmonization:
// When users declare metric tag like:
//
//	metric_tags:
//	  - OID: 1.2.3
//	    symbol: aSymbol
//
// this will lead to OID stored as MetricTagConfig.OID  and name stored as MetricTagConfig.Symbol.Name
// When this happens, in validateEnrichMetricTags we harmonize by moving MetricTagConfig.OID to MetricTagConfig.Symbol.OID.
type SymbolConfigCompat SymbolConfig

// Clone creates a duplicate of this SymbolConfigCompat
func (s SymbolConfigCompat) Clone() SymbolConfigCompat {
	return SymbolConfigCompat(SymbolConfig(s).Clone())
}

// SymbolConfig holds info for a single symbol/oid
type SymbolConfig struct {
	OID  string `yaml:"OID,omitempty" json:"OID,omitempty"`
	Name string `yaml:"name,omitempty" json:"name,omitempty"`

	ExtractValue         string         `yaml:"extract_value,omitempty" json:"extract_value,omitempty"`
	ExtractValueCompiled *regexp.Regexp `yaml:"-" json:"-"`

	MatchPattern         string         `yaml:"match_pattern,omitempty" json:"match_pattern,omitempty"`
	MatchValue           string         `yaml:"match_value,omitempty" json:"match_value,omitempty"`
	MatchPatternCompiled *regexp.Regexp `yaml:"-" json:"-"`

	ScaleFactor      float64 `yaml:"scale_factor,omitempty" json:"scale_factor,omitempty"`
	Format           string  `yaml:"format,omitempty" json:"format,omitempty"`
	ConstantValueOne bool    `yaml:"constant_value_one,omitempty" json:"constant_value_one,omitempty"`

	// `metric_type` is used for force the metric type
	//   When empty, by default, the metric type is derived from SNMP OID value type.
	//   Valid `metric_type` types: `gauge`, `rate`, `monotonic_count`, `monotonic_count_and_rate`
	//   Deprecated types: `counter` (use `rate` instead), percent (use `scale_factor` instead)
	MetricType ProfileMetricType `yaml:"metric_type,omitempty" json:"metric_type,omitempty"`

	ChartMeta ChartMeta `yaml:"chart_meta,omitempty" json:"chart_meta,omitempty"`

	Mapping           map[string]string  `yaml:"mapping,omitempty" json:"mapping,omitempty"`
	Transform         string             `yaml:"transform,omitempty" json:"transform,omitempty"`
	TransformCompiled *template.Template `yaml:"-" json:"-"`
}

// Clone creates a duplicate of this SymbolConfig
func (s SymbolConfig) Clone() SymbolConfig {
	ss := s
	ss.Mapping = maps.Clone(ss.Mapping)
	return ss
}

type ChartMeta struct {
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Family      string `yaml:"family,omitempty" json:"family,omitempty"`
	Unit        string `yaml:"unit,omitempty" json:"unit,omitempty"`
	Type        string `yaml:"type,omitempty" json:"type,omitempty"`
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
	OID string `yaml:"OID,omitempty" json:"-"  jsonschema:"-"`
	// Symbol records the OID to be parsed. Note that .Symbol.Name is ignored:
	// set .Tag to specify the tag name. If a serialized Symbol is a string
	// instead of an object, it will be treated like {name: <value>}; this use
	// pattern is deprecated
	Symbol SymbolConfigCompat `yaml:"symbol,omitempty" json:"symbol,omitempty"`

	IndexTransform []MetricIndexTransform `yaml:"index_transform,omitempty" json:"index_transform,omitempty"`

	MappingRef string            `yaml:"mapping_ref,omitempty" json:"mapping_ref,omitempty"`
	Mapping    map[string]string `yaml:"mapping,omitempty" json:"mapping,omitempty"`

	// Regex
	// Match/Tags are not exposed as json (UI) since ExtractValue can be used instead
	Match   string            `yaml:"match,omitempty" json:"-"`
	Tags    map[string]string `yaml:"tags,omitempty" json:"-"`
	Pattern *regexp.Regexp    `yaml:"-" json:"-"`

	SymbolTag string `yaml:"-" json:"-"`
}

// Clone duplicates this MetricTagConfig
func (m MetricTagConfig) Clone() MetricTagConfig {
	m2 := m // non-pointer assignment shallow-copies members
	// deep copy symbols and structures
	m2.Column = m.Column.Clone()
	m2.Symbol = m.Symbol.Clone()
	m2.IndexTransform = slices.Clone(m.IndexTransform)
	m2.Mapping = maps.Clone(m.Mapping)
	m2.Tags = maps.Clone(m.Tags)
	return m2
}

type StaticMetricTagConfig struct {
	Tag   string `yaml:"tag" json:"tag"`
	Value string `yaml:"value" json:"value"`
}

func (s StaticMetricTagConfig) Clone() StaticMetricTagConfig {
	return StaticMetricTagConfig{
		Tag:   s.Tag,
		Value: s.Value,
	}
}

// MetricTagConfigList holds configs for a list of metric tags
type MetricTagConfigList []MetricTagConfig

// MetricIndexTransform holds configs for metric index transform
type MetricIndexTransform struct {
	Start uint `yaml:"start" json:"start"`
	End   uint `yaml:"end" json:"end"`
}

// MetricsConfigOption holds config for metrics options
type MetricsConfigOption struct {
	Placement    uint   `yaml:"placement,omitempty" json:"placement,omitempty"`
	MetricSuffix string `yaml:"metric_suffix,omitempty" json:"metric_suffix,omitempty"`
}
