// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"slices"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

type vmetricsCollector struct {
	log *logger.Logger
}

func newVirtualMetricsCollector(log *logger.Logger) *vmetricsCollector {
	return &vmetricsCollector{
		log: log,
	}
}

func (p *vmetricsCollector) Collect(profDef *ddprofiledefinition.ProfileDefinition, collected []ddsnmp.Metric) []ddsnmp.Metric {
	if len(profDef.VirtualMetrics) == 0 {
		return nil
	}
	lookup, aggrs := p.buildAggregators(profDef)
	p.accumulate(lookup, collected)
	return p.emit(aggrs)
}

func (p *vmetricsCollector) accumulate(lookup map[vmetricsSourceKey][]vmetricsSink, collected []ddsnmp.Metric) {
	for _, m := range collected {
		sinks, ok := lookup[vmetricsSourceKey{metricName: m.Name, tableName: m.Table}]
		if !ok {
			continue
		}

		v, mv := vmCollapseMetricValue(m)

		type gke struct {
			key string
			ok  bool
		}
		var gkCache map[*vmetricsAggregator]gke

		for _, sink := range sinks {
			agg := sink.agg
			if agg == nil {
				continue
			}

			if agg.metricType == "" {
				agg.metricType = m.MetricType
			}

			if !agg.grouped {
				agg.accumulateTotal(sink, v, mv)
				continue
			}

			if gkCache == nil {
				gkCache = make(map[*vmetricsAggregator]gke, 2)
			}
			entry, found := gkCache[agg]
			if !found {
				k, ok := vmBuildGroupKey(m.Tags, agg)
				entry = gke{key: k, ok: ok}
				gkCache[agg] = entry
			}
			if !entry.ok {
				continue
			}
			agg.accumulateGroupedWithKey(sink, entry.key, v, m.Tags)
		}
	}
}

func (p *vmetricsCollector) emit(aggrs []*vmetricsAggregator) []ddsnmp.Metric {
	// small pre-alloc: 1 total or ~groups count; start conservative
	out := make([]ddsnmp.Metric, 0, len(aggrs))
	for _, agg := range aggrs {
		if !agg.hadData {
			p.log.Debugf("no source metrics found for virtual metric '%s'", agg.config.Name)
			continue
		}
		agg.emitInto(&out)
	}
	return out
}

type (
	// vmetricsAggregator holds accumulation state for a virtual metric
	vmetricsAggregator struct {
		config     ddprofiledefinition.VirtualMetricConfig
		metricType ddprofiledefinition.ProfileMetricType

		// --- grouping controls ---
		grouped    bool     // len(GroupBy) > 0
		perRow     bool     // from cfg.PerRow
		groupBy    []string // when perRow==true, groupBy are used as row-key hints
		groupTable string   // sources must share the same table
		perGroup   map[string]*vmetricsGroupBucket

		// --- dimensions (composite) ---
		dimNames []string

		// --- non-grouped accumulators (existing behavior) ---
		perDim   map[string]int64 // composite total (dim -> sum)
		sum      int64            // single-source total
		multiSum map[string]int64 // merged base MultiValue

		hadData bool
		keyBuf  strings.Builder
	}

	// vmetricsSourceKey identifies a metric source
	vmetricsSourceKey struct {
		metricName string
		tableName  string
	}

	// sink binding; dimIdx == -1 for non-composite
	vmetricsSink struct {
		agg    *vmetricsAggregator
		dimIdx int16
	}

	// per-group accumulator (emitted as one table row)
	vmetricsGroupBucket struct {
		vals     []int64 // len == dims.count when composite
		seen     []bool
		sum      int64             // single-source grouped case
		emitTags map[string]string // explicit group_by: tiny map; per-row "*": pointer to source Tags
	}
)

func (agg *vmetricsAggregator) accumulateTotal(sink vmetricsSink, v int64, mv map[string]int64) {
	agg.hadData = true

	if sink.dimIdx >= 0 {
		if agg.perDim == nil {
			agg.perDim = make(map[string]int64, len(agg.dimNames))
		}
		name := agg.dimNames[sink.dimIdx]
		agg.perDim[name] += v
		return
	}
	// Single-source path: preserve MultiValue if provided
	if mv != nil {
		if agg.multiSum == nil {
			agg.multiSum = make(map[string]int64, len(mv))
		}
		for k, x := range mv {
			agg.multiSum[k] += x
		}
		return
	}
	agg.sum += v
}

func (agg *vmetricsAggregator) accumulateGroupedWithKey(sink vmetricsSink, gkey string, v int64, tags map[string]string) {
	agg.hadData = true

	b := agg.perGroup[gkey]
	if b == nil {
		b = &vmetricsGroupBucket{emitTags: vmBuildEmitTags(tags, agg)}
		if len(agg.dimNames) > 0 {
			b.vals = make([]int64, len(agg.dimNames))
			b.seen = make([]bool, len(agg.dimNames))
		}
		agg.perGroup[gkey] = b
	}
	if sink.dimIdx >= 0 {
		b.vals[sink.dimIdx] += v
		b.seen[sink.dimIdx] = true
	} else {
		b.sum += v
	}
}

func (agg *vmetricsAggregator) emitInto(out *[]ddsnmp.Metric) {
	if agg.grouped {
		agg.emitGrouped(out)
	} else {
		agg.emitTotal(out)
	}
}

func (agg *vmetricsAggregator) emitGrouped(out *[]ddsnmp.Metric) {
	for _, b := range agg.perGroup {
		vm := ddsnmp.Metric{
			Name:        agg.config.Name,
			Description: agg.config.ChartMeta.Description,
			Family:      agg.config.ChartMeta.Family,
			Unit:        agg.config.ChartMeta.Unit,
			ChartType:   agg.config.ChartMeta.Type,
			MetricType:  agg.metricType,
			IsTable:     true,
			Table:       agg.groupTable,
			Tags:        b.emitTags,
		}
		if len(agg.dimNames) > 0 {
			mv := make(map[string]int64, len(agg.dimNames))
			for i, dn := range agg.dimNames {
				if b.seen[i] {
					mv[dn] = b.vals[i]
				}
			}
			vm.MultiValue = mv
		} else {
			vm.Value = b.sum
		}
		*out = append(*out, vm)
	}
}

func (agg *vmetricsAggregator) emitTotal(out *[]ddsnmp.Metric) {
	vm := ddsnmp.Metric{
		Name:        agg.config.Name,
		Description: agg.config.ChartMeta.Description,
		Family:      agg.config.ChartMeta.Family,
		Unit:        agg.config.ChartMeta.Unit,
		ChartType:   agg.config.ChartMeta.Type,
		MetricType:  agg.metricType,
	}
	switch {
	case len(agg.perDim) > 0:
		vm.MultiValue = agg.perDim
	case len(agg.multiSum) > 0:
		vm.MultiValue = agg.multiSum
	default:
		vm.Value = agg.sum
	}
	*out = append(*out, vm)
}

func (p *vmetricsCollector) buildAggregators(profDef *ddprofiledefinition.ProfileDefinition) (map[vmetricsSourceKey][]vmetricsSink, []*vmetricsAggregator) {
	sourceToSinks := make(map[vmetricsSourceKey][]vmetricsSink)
	aggregators := make([]*vmetricsAggregator, 0, len(profDef.VirtualMetrics))

	existingNames := p.getDefinedMetricNames(profDef.Metrics)

	for _, cfg := range profDef.VirtualMetrics {
		if existingNames[cfg.Name] {
			p.log.Warningf("virtual metric '%s' conflicts with existing metric, skipping", cfg.Name)
			continue
		}

		agg := &vmetricsAggregator{config: cfg}

		agg.grouped = cfg.PerRow || len(cfg.GroupBy) > 0

		if agg.grouped {
			// require all sources from the same table
			var table string
			same := true
			for i, s := range cfg.Sources {
				if i == 0 {
					table = s.Table
				} else if s.Table != table {
					same = false
					break
				}
			}
			if !same || table == "" {
				p.log.Warningf("virtual metric '%s' uses group_by but sources span tables or have no table; skipping (no joins yet)", cfg.Name)
				continue
			}

			agg.perRow = cfg.PerRow
			agg.groupBy = cfg.GroupBy
			agg.groupTable = table
			agg.perGroup = make(map[string]*vmetricsGroupBucket, 64)
		}

		// --- composite dims? (multiple sources and at least one 'as') ---
		isComposite := len(cfg.Sources) > 1 &&
			slices.ContainsFunc(cfg.Sources, func(s ddprofiledefinition.VirtualMetricSourceConfig) bool {
				return s.As != ""
			})

		var dimsIdxByName map[string]int

		if isComposite {
			dimsIdxByName = make(map[string]int, len(cfg.Sources))
			agg.dimNames = make([]string, 0, len(cfg.Sources))
			for _, s := range cfg.Sources {
				name := ternary(s.As != "", s.As, s.Metric)
				if _, dup := dimsIdxByName[name]; !dup {
					dimsIdxByName[name] = len(agg.dimNames)
					agg.dimNames = append(agg.dimNames, name)
				}
			}
		}

		// register sinks
		for _, src := range cfg.Sources {
			key := vmetricsSourceKey{metricName: src.Metric, tableName: src.Table}

			dimIdx := int16(-1)
			if isComposite {
				name := ternary(src.As != "", src.As, src.Metric)
				if idx, ok := dimsIdxByName[name]; ok {
					dimIdx = int16(idx)
				}
			}

			sourceToSinks[key] = append(sourceToSinks[key], vmetricsSink{
				agg:    agg,
				dimIdx: dimIdx,
			})
		}

		aggregators = append(aggregators, agg)
	}

	return sourceToSinks, aggregators
}

func (p *vmetricsCollector) getDefinedMetricNames(profMetrics []ddprofiledefinition.MetricsConfig) map[string]bool {
	names := make(map[string]bool, len(profMetrics))
	for _, m := range profMetrics {
		switch {
		case m.IsScalar():
			names[m.Symbol.Name] = true
		case m.IsColumn():
			for _, sym := range m.Symbols {
				names[sym.Name] = true
			}
		}
	}
	return names
}

// vmBuildGroupKey returns a stable group key.
func vmBuildGroupKey(tags map[string]string, agg *vmetricsAggregator) (string, bool) {
	if !agg.grouped {
		return "", false
	}

	const (
		groupKeySep = '\x1F' // ASCII Unit Separator: safe delimiter between label values/pairs
		kvSep       = '='    // used only in per-row fallback "k=v"
	)

	if agg.perRow {
		if len(tags) == 0 {
			return "", false
		}

		if len(agg.groupBy) > 0 {
			agg.keyBuf.Reset()
			for i, l := range agg.groupBy {
				v := tags[l]
				if v == "" {
					// missing hint
					agg.keyBuf.Reset()
					goto perRowFallback
				}
				if i > 0 {
					agg.keyBuf.WriteByte(groupKeySep)
				}
				agg.keyBuf.WriteString(v)
			}
			return agg.keyBuf.String(), true
		}

	perRowFallback:
		// Fallback: stable key from all tags (sorted k=v)
		keys := make([]string, 0, len(tags))
		for k := range tags {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		agg.keyBuf.Reset()
		for i, k := range keys {
			if i > 0 {
				agg.keyBuf.WriteByte(groupKeySep)
			}
			agg.keyBuf.WriteString(k)
			agg.keyBuf.WriteByte(kvSep)
			agg.keyBuf.WriteString(tags[k])
		}
		return agg.keyBuf.String(), true
	}

	switch len(agg.groupBy) {
	case 0:
		return "", false
	case 1:
		v := tags[agg.groupBy[0]]
		return v, v != ""
	default:
		agg.keyBuf.Reset()
		for i, l := range agg.groupBy {
			v := tags[l]
			if v == "" {
				return "", false
			}
			if i > 0 {
				agg.keyBuf.WriteByte(groupKeySep)
			}
			agg.keyBuf.WriteString(v)
		}
		return agg.keyBuf.String(), true
	}
}

// vmBuildEmitTags captures labels to emit for a group (called once per new group)
func vmBuildEmitTags(tags map[string]string, agg *vmetricsAggregator) map[string]string {
	if agg.perRow {
		// per-row: reuse pointer; we never mutate it here
		return tags
	}
	out := make(map[string]string, len(agg.groupBy))
	for _, l := range agg.groupBy {
		if v := tags[l]; v != "" {
			out[l] = v
		}
	}
	return out
}

// vmCollapseMetricValue a metric to an int64 quickly; return mv if present for merge path
func vmCollapseMetricValue(m ddsnmp.Metric) (v int64, mv map[string]int64) {
	if len(m.MultiValue) == 0 {
		return m.Value, nil
	}
	var sum int64
	for _, x := range m.MultiValue {
		sum += x
	}
	return sum, m.MultiValue
}
