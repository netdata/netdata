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
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
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

func (mb *metricBuilder) asTableMetric(table string) *metricBuilder {
	mb.metric.IsTable = true
	mb.metric.Table = table
	return mb
}

func (mb *metricBuilder) fromSymbol(sym ddprofiledefinition.SymbolConfig, pdu gosnmp.SnmpPDU) *metricBuilder {
	mb.metric.Unit = sym.ChartMeta.Unit
	mb.metric.Description = sym.ChartMeta.Description
	mb.metric.Family = sym.ChartMeta.Family
	mb.metric.ChartType = sym.ChartMeta.Type
	mb.metric.MetricType = ternary(sym.MetricType != "", sym.MetricType, getMetricTypeFromPDUType(pdu))
	mb.metric.MultiValue = buildMultiValue(mb.metric.Value, sym.Mapping)
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

func buildTableMetric(cfg ddprofiledefinition.SymbolConfig, pdu gosnmp.SnmpPDU, value int64, tags, staticTags map[string]string, tableName string) (*ddsnmp.Metric, error) {
	metric := newMetricBuilder(cfg.Name, value).
		withTags(tags).
		withStaticTags(staticTags).
		fromSymbol(cfg, pdu).
		asTableMetric(tableName).
		build()

	if cfg.TransformCompiled != nil {
		if err := applyTransform(&metric, cfg); err != nil {
			return nil, err
		}
	}

	return &metric, nil
}

func buildMultiValue(value int64, mappings map[string]string) map[string]int64 {
	if len(mappings) == 0 {
		return nil
	}

	// Check if this is an int→int mapping (value transformation)
	if isIntToIntMapping := func() bool {
		for k, v := range mappings {
			if !isInt(k) || !isInt(v) {
				return false
			}
		}
		return true
	}(); isIntToIntMapping {
		return nil
	}

	isMappingKeysNumeric := func() bool {
		for k := range mappings {
			if !isInt(k) {
				return false
			}
		}
		return true
	}()

	multiValue := make(map[string]int64)

	if isMappingKeysNumeric {
		// int→string mapping (e.g., 1→"up", 2→"down")
		for k, v := range mappings {
			intKey, _ := strconv.ParseInt(k, 10, 64)
			// Only set the value if:
			// 1. We haven't seen this state name before (!ok), OR
			// 2. The current value matches this key (value == intKey)
			// This ensures that if multiple keys map to the same state name,
			// we preserve the "1" (active) value if any key matches
			if _, ok := multiValue[v]; !ok || value == intKey {
				multiValue[v] = metrix.Bool(value == intKey)
			}
		}
	} else {
		// string→int mapping (e.g., "OK"→"0", "WARNING"→"1", "CRITICAL"→"2")
		// value has already been converted from string to int by the value processor
		// We need to find which original string maps to our current value
		for k, v := range mappings {
			if intVal, err := strconv.ParseInt(v, 10, 64); err == nil {
				multiValue[k] = metrix.Bool(value == intVal)
			}
		}
	}

	return multiValue
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
