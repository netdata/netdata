// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"maps"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

// MetricBuilder provides a fluent interface for building metrics
type MetricBuilder struct {
	metric ddsnmp.Metric
}

// NewMetricBuilder creates a new MetricBuilder with basic metric info
func NewMetricBuilder(name string, value int64) *MetricBuilder {
	return &MetricBuilder{
		metric: ddsnmp.Metric{
			Name:  name,
			Value: value,
		},
	}
}

// WithTags sets the metric tags
func (mb *MetricBuilder) WithTags(tags map[string]string) *MetricBuilder {
	if len(tags) > 0 {
		mb.metric.Tags = maps.Clone(tags)
	}
	return mb
}

// WithStaticTags sets the static tags
func (mb *MetricBuilder) WithStaticTags(tags map[string]string) *MetricBuilder {
	if len(tags) > 0 {
		mb.metric.StaticTags = maps.Clone(tags)
	}
	return mb
}

// WithUnit sets the metric unit
func (mb *MetricBuilder) WithUnit(unit string) *MetricBuilder {
	mb.metric.Unit = unit
	return mb
}

// WithDescription sets the metric description
func (mb *MetricBuilder) WithDescription(desc string) *MetricBuilder {
	mb.metric.Description = desc
	return mb
}

// WithFamily sets the metric family
func (mb *MetricBuilder) WithFamily(family string) *MetricBuilder {
	mb.metric.Family = family
	return mb
}

// WithMetricType sets the metric type
func (mb *MetricBuilder) WithMetricType(metricType ddprofiledefinition.ProfileMetricType) *MetricBuilder {
	mb.metric.MetricType = metricType
	return mb
}

// WithMappings sets the value mappings
func (mb *MetricBuilder) WithMappings(mappings map[int64]string) *MetricBuilder {
	if len(mappings) > 0 {
		mb.metric.Mappings = mappings
	}
	return mb
}

// AsTableMetric marks the metric as coming from a table
func (mb *MetricBuilder) AsTableMetric() *MetricBuilder {
	mb.metric.IsTable = true
	return mb
}

// FromSymbol applies symbol configuration to the metric
func (mb *MetricBuilder) FromSymbol(sym ddprofiledefinition.SymbolConfig, pdu gosnmp.SnmpPDU) *MetricBuilder {
	mb.metric.Unit = sym.Unit
	mb.metric.Description = sym.Description
	mb.metric.Family = sym.Family
	mb.metric.MetricType = getMetricType(sym, pdu)
	mb.metric.Mappings = convSymMappingToNumeric(sym)
	return mb
}

// Build returns the built metric
func (mb *MetricBuilder) Build() ddsnmp.Metric {
	return mb.metric
}

// buildScalarMetric builds a scalar metric using the builder
func buildScalarMetric(cfg ddprofiledefinition.SymbolConfig, pdu gosnmp.SnmpPDU, value int64, staticTags map[string]string) (*ddsnmp.Metric, error) {
	metric := NewMetricBuilder(cfg.Name, value).
		WithStaticTags(staticTags).
		FromSymbol(cfg, pdu).
		Build()

	if cfg.TransformCompiled != nil {
		if err := applyTransform(&metric, cfg); err != nil {
			return nil, err
		}
	}

	return &metric, nil
}

// buildTableMetric builds a table metric using the builder
func buildTableMetric(cfg ddprofiledefinition.SymbolConfig, pdu gosnmp.SnmpPDU, value int64, tags, staticTags map[string]string) (*ddsnmp.Metric, error) {
	metric := NewMetricBuilder(cfg.Name, value).
		WithTags(tags).
		WithStaticTags(staticTags).
		FromSymbol(cfg, pdu).
		AsTableMetric().
		Build()

	if cfg.TransformCompiled != nil {
		if err := applyTransform(&metric, cfg); err != nil {
			return nil, err
		}
	}

	return &metric, nil
}
