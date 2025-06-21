// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"maps"
	"strconv"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

// metricBuilder provides a fluent interface for building metrics
type metricBuilder struct {
	metric ddsnmp.Metric
}

// newMetricBuilder creates a new metricBuilder with basic metric info
func newMetricBuilder(name string, value int64) *metricBuilder {
	return &metricBuilder{
		metric: ddsnmp.Metric{
			Name:  name,
			Value: value,
		},
	}
}

// withTags sets the metric tags
func (mb *metricBuilder) withTags(tags map[string]string) *metricBuilder {
	if len(tags) > 0 {
		mb.metric.Tags = maps.Clone(tags)
	}
	return mb
}

// withStaticTags sets the static tags
func (mb *metricBuilder) withStaticTags(tags map[string]string) *metricBuilder {
	if len(tags) > 0 {
		mb.metric.StaticTags = maps.Clone(tags)
	}
	return mb
}

// asTableMetric marks the metric as coming from a table
func (mb *metricBuilder) asTableMetric() *metricBuilder {
	mb.metric.IsTable = true
	return mb
}

// fromSymbol applies symbol configuration to the metric
func (mb *metricBuilder) fromSymbol(sym ddprofiledefinition.SymbolConfig, pdu gosnmp.SnmpPDU) *metricBuilder {
	mb.metric.Unit = sym.Unit
	mb.metric.Description = sym.Description
	mb.metric.Family = sym.Family
	mb.metric.MetricType = ternary(sym.MetricType != "", sym.MetricType, getMetricTypeFromPDUType(pdu))
	mb.metric.Mappings = convSymMappingToNumeric(sym)
	return mb
}

// Build returns the built metric
func (mb *metricBuilder) Build() ddsnmp.Metric {
	return mb.metric
}

// buildScalarMetric builds a scalar metric using the builder
func buildScalarMetric(cfg ddprofiledefinition.SymbolConfig, pdu gosnmp.SnmpPDU, value int64, staticTags map[string]string) (*ddsnmp.Metric, error) {
	metric := newMetricBuilder(cfg.Name, value).
		withStaticTags(staticTags).
		fromSymbol(cfg, pdu).
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
	metric := newMetricBuilder(cfg.Name, value).
		withTags(tags).
		withStaticTags(staticTags).
		fromSymbol(cfg, pdu).
		asTableMetric().
		Build()

	if cfg.TransformCompiled != nil {
		if err := applyTransform(&metric, cfg); err != nil {
			return nil, err
		}
	}

	return &metric, nil
}

func convSymMappingToNumeric(cfg ddprofiledefinition.SymbolConfig) map[int64]string {
	if len(cfg.Mapping) == 0 {
		return nil
	}

	mappings := make(map[int64]string)

	if isMappingKeysNumeric(cfg.Mapping) {
		for k, v := range cfg.Mapping {
			intKey, _ := strconv.ParseInt(k, 10, 64)
			mappings[intKey] = v
		}
	} else {
		for k, v := range cfg.Mapping {
			if intVal, err := strconv.ParseInt(v, 10, 64); err == nil {
				mappings[intVal] = k
			}
		}
	}

	return mappings
}
