// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"bytes"
	"fmt"
	"maps"
	"strconv"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

type metricBuilder struct {
	metric ddsnmp.Metric
}

func newMetricBuilder(name string, value int64) *metricBuilder {
	return &metricBuilder{
		metric: ddsnmp.Metric{
			Name:  name,
			Value: value,
		},
	}
}

func (mb *metricBuilder) withTags(tags map[string]string) *metricBuilder {
	if len(tags) > 0 {
		mb.metric.Tags = maps.Clone(tags)
	}
	return mb
}

func (mb *metricBuilder) withStaticTags(tags map[string]string) *metricBuilder {
	if len(tags) > 0 {
		mb.metric.StaticTags = maps.Clone(tags)
	}
	return mb
}

func (mb *metricBuilder) asTableMetric() *metricBuilder {
	mb.metric.IsTable = true
	return mb
}

func (mb *metricBuilder) fromSymbol(sym ddprofiledefinition.SymbolConfig, pdu gosnmp.SnmpPDU) *metricBuilder {
	mb.metric.Unit = sym.ChartMeta.Unit
	mb.metric.Description = sym.ChartMeta.Description
	mb.metric.Family = sym.ChartMeta.Family
	mb.metric.MetricType = ternary(sym.MetricType != "", sym.MetricType, getMetricTypeFromPDUType(pdu))
	mb.metric.Mappings = convMappingToNumeric(sym)
	return mb
}

func (mb *metricBuilder) build() ddsnmp.Metric {
	return mb.metric
}

func buildScalarMetric(cfg ddprofiledefinition.SymbolConfig, pdu gosnmp.SnmpPDU, value int64, staticTags map[string]string) (*ddsnmp.Metric, error) {
	metric := newMetricBuilder(cfg.Name, value).
		withStaticTags(staticTags).
		fromSymbol(cfg, pdu).
		build()

	if cfg.TransformCompiled != nil {
		if err := applyTransform(&metric, cfg); err != nil {
			return nil, err
		}
	}

	return &metric, nil
}

func buildTableMetric(cfg ddprofiledefinition.SymbolConfig, pdu gosnmp.SnmpPDU, value int64, tags, staticTags map[string]string) (*ddsnmp.Metric, error) {
	metric := newMetricBuilder(cfg.Name, value).
		withTags(tags).
		withStaticTags(staticTags).
		fromSymbol(cfg, pdu).
		asTableMetric().
		build()

	if cfg.TransformCompiled != nil {
		if err := applyTransform(&metric, cfg); err != nil {
			return nil, err
		}
	}

	return &metric, nil
}

func convMappingToNumeric(cfg ddprofiledefinition.SymbolConfig) map[int64]string {
	if len(cfg.Mapping) == 0 {
		return nil
	}

	isMappingKeysNumeric := func() bool {
		for k := range cfg.Mapping {
			if !isInt(k) {
				return false
			}
		}
		return true
	}()

	mappings := make(map[int64]string)

	if isMappingKeysNumeric {
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

func applyTransform(metric *ddsnmp.Metric, sym ddprofiledefinition.SymbolConfig) error {
	if sym.TransformCompiled == nil {
		return nil
	}

	ctx := struct{ Metric *ddsnmp.Metric }{Metric: metric}

	var buf bytes.Buffer
	if err := sym.TransformCompiled.Execute(&buf, ctx); err != nil {
		return fmt.Errorf("failed to execute transform template: %w", err)
	}

	return nil
}
