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
		// --- Alternatives parent: choose first child with data ---
		if len(agg.alts) > 0 {
			i := slices.IndexFunc(agg.alts, func(alt *vmetricsAggregator) bool { return alt.hadData })
			if i == -1 {
				p.log.Debugf("no alternative had data for virtual metric '%s'", agg.config.Name)
				continue
			}
			winner := agg.alts[i]
			// Emit using the parent's name/meta, but the child's accumulated values.
			winner.emitIntoAs(&out, agg.config.Name, agg.config.ChartMeta)
			continue
		}

		// --- Plain VM
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
		// alts is non-empty only for an "alternatives parent" aggregator.
		// Children are plain aggregators (alts == nil) that actually receive samples.
		// At emit time, the parent picks the first child with hadData and emits that
		// child's accumulated values under the parent's (name, chart meta).
		alts []*vmetricsAggregator

		config ddprofiledefinition.VirtualMetricConfig
		// metricType is inferred from the first seen source sample for this aggregator.
		// If sources disagree, the first one wins (caller may log/debug mismatches).
		metricType ddprofiledefinition.ProfileMetricType

		// --- grouping controls ---
		// grouped is true when PerRow is set OR len(GroupBy) > 0.
		// Grouped aggregators produce table rows keyed by vmBuildGroupKey(...).
		grouped bool
		// perRow mirrors cfg.PerRow. When true, each unique tag-set becomes a row key.
		// groupBy acts as ordered hints for building the key; if any hint is missing,
		// we fall back to a stable key built from all tags (sorted "k=v").
		perRow bool
		// groupBy mirrors cfg.GroupBy. When perRow==false and groupBy!=nil, all listed
		// labels must be present to form the row key; otherwise the sample is skipped.
		groupBy []string
		// groupTable is the SNMP table name all sources must share for grouped VMs.
		// This constraint is enforced per-aggregator; for alternatives, it is enforced
		// independently for each child. (No cross-table joins.)
		groupTable string
		// perGroup accumulates rows by group key.
		perGroup map[string]*vmetricsGroupBucket

		// --- dimensions (composite) ---
		// dimNames defines the stable order of composite dimensions. It is built from
		// the 'as' names (or metric names when 'as' is empty) in first-seen order.
		// sink.dimIdx indexes into this slice.
		dimNames []string

		// --- non-grouped accumulators ---
		// perDim accumulates totals for composite (multi-source) non-grouped VMs:
		//   dim name -> sum of values assigned to that dim.
		perDim map[string]int64
		// sum accumulates totals for single-source non-grouped VMs (no MultiValue).
		sum int64
		// multiSum merges MultiValue maps for single-source non-grouped VMs that
		// provide MultiValue; keys are preserved and values are summed per key.
		multiSum map[string]int64

		// hadData is set to true once at least one sample was accumulated into this
		// aggregator (for grouped or non-grouped). Parents look at children.hadData
		// to decide which alternative to emit.
		hadData bool

		// keyBuf is a reusable scratch buffer for building group keys. Not goroutine-safe.
		keyBuf strings.Builder
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
	agg.emitIntoAs(out, agg.config.Name, agg.config.ChartMeta)
}

func (agg *vmetricsAggregator) emitGrouped(out *[]ddsnmp.Metric) {
	agg.emitGroupedAs(out, agg.config.Name, agg.config.ChartMeta)
}

func (agg *vmetricsAggregator) emitTotal(out *[]ddsnmp.Metric) {
	agg.emitTotalAs(out, agg.config.Name, agg.config.ChartMeta)
}

func (agg *vmetricsAggregator) emitIntoAs(out *[]ddsnmp.Metric, name string, meta ddprofiledefinition.ChartMeta) {
	if agg.grouped {
		agg.emitGroupedAs(out, name, meta)
	} else {
		agg.emitTotalAs(out, name, meta)
	}
}

func (agg *vmetricsAggregator) emitGroupedAs(out *[]ddsnmp.Metric, name string, meta ddprofiledefinition.ChartMeta) {
	for _, b := range agg.perGroup {
		vm := ddsnmp.Metric{
			Name:        name,
			Description: meta.Description,
			Family:      meta.Family,
			Unit:        meta.Unit,
			ChartType:   meta.Type,
			MetricType:  agg.metricType,
			IsTable:     true,
			Table:       agg.groupTable, // winner's table (e.g., ifXTable or ifTable)
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

func (agg *vmetricsAggregator) emitTotalAs(out *[]ddsnmp.Metric, name string, meta ddprofiledefinition.ChartMeta) {
	vm := ddsnmp.Metric{
		Name:        name,
		Description: meta.Description,
		Family:      meta.Family,
		Unit:        meta.Unit,
		ChartType:   meta.Type,
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
	existingNames := p.getDefinedMetricNames(profDef.Metrics)
	builder := newAggregatorsBuilder(p.log, profDef, existingNames)
	return builder.build()
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

type aggregatorsBuilder struct {
	log           *logger.Logger
	prof          *ddprofiledefinition.ProfileDefinition
	existingNames map[string]bool

	sourceToSinks map[vmetricsSourceKey][]vmetricsSink
	aggregators   []*vmetricsAggregator
}

func newAggregatorsBuilder(
	log *logger.Logger, prof *ddprofiledefinition.ProfileDefinition, existingNames map[string]bool) *aggregatorsBuilder {
	return &aggregatorsBuilder{
		log:           log,
		prof:          prof,
		existingNames: existingNames,
		sourceToSinks: make(map[vmetricsSourceKey][]vmetricsSink),
		aggregators:   make([]*vmetricsAggregator, 0, len(prof.VirtualMetrics)),
	}
}

func (b *aggregatorsBuilder) build() (map[vmetricsSourceKey][]vmetricsSink, []*vmetricsAggregator) {
	for _, cfg := range b.prof.VirtualMetrics {
		b.buildOne(cfg)
	}
	return b.sourceToSinks, b.aggregators
}

func (b *aggregatorsBuilder) buildOne(cfg ddprofiledefinition.VirtualMetricConfig) {
	if b.existingNames[cfg.Name] {
		b.log.Warningf("virtual metric '%s' conflicts with existing metric, skipping", cfg.Name)
		return
	}
	if len(cfg.Sources) == 0 && len(cfg.Alternatives) == 0 {
		b.log.Warningf("virtual metric '%s' has no sources or alternatives; skipping", cfg.Name)
		return
	}
	if len(cfg.Sources) > 0 && len(cfg.Alternatives) > 0 {
		b.log.Warningf("virtual metric '%s' defines both 'sources' and 'alternatives'; using 'alternatives' only", cfg.Name)
	}
	if len(cfg.Alternatives) == 0 {
		b.buildPlain(cfg)
	} else {
		b.buildWithAlternatives(cfg)
	}
}

func (b *aggregatorsBuilder) buildPlain(cfg ddprofiledefinition.VirtualMetricConfig) {
	agg, isComposite, dimIdxByName, ok := b.prepareAggregator(cfg, cfg.Sources)
	if !ok {
		return
	}
	b.bindSinks(agg, cfg.Sources, dimIdxByName, isComposite)
	b.aggregators = append(b.aggregators, agg)
}

func (b *aggregatorsBuilder) buildWithAlternatives(cfg ddprofiledefinition.VirtualMetricConfig) {
	parent := &vmetricsAggregator{config: cfg}

	for _, alt := range cfg.Alternatives {
		childCfg := cfg.Clone()
		childCfg.Sources = slices.Clone(alt.Sources)
		childCfg.Alternatives = nil

		child, isComposite, dimIdxByName, ok := b.prepareAggregator(childCfg, childCfg.Sources)
		if !ok {
			continue
		}
		b.bindSinks(child, childCfg.Sources, dimIdxByName, isComposite)
		parent.alts = append(parent.alts, child)
	}

	if len(parent.alts) == 0 {
		b.log.Warningf("virtual metric '%s' has no valid alternatives; skipping", cfg.Name)
		return
	}
	b.aggregators = append(b.aggregators, parent)
}

func (b *aggregatorsBuilder) sameTableOrEmpty(sources []ddprofiledefinition.VirtualMetricSourceConfig) (table string, ok bool) {
	for i, s := range sources {
		if i == 0 {
			table = s.Table
			continue
		}
		if s.Table != table {
			return "", false
		}
	}
	return table, table != ""
}

func (b *aggregatorsBuilder) computeDimIndexMap(sources []ddprofiledefinition.VirtualMetricSourceConfig) (isComposite bool, dimIdxByName map[string]int, dimNames []string) {
	if len(sources) > 1 {
		for _, s := range sources {
			if s.As != "" {
				isComposite = true
				break
			}
		}
	}
	if !isComposite {
		return
	}
	dimIdxByName = make(map[string]int, len(sources))
	dimNames = make([]string, 0, len(sources))
	for _, s := range sources {
		name := s.As
		if name == "" {
			name = s.Metric
		}
		if _, exists := dimIdxByName[name]; !exists {
			dimIdxByName[name] = len(dimNames)
			dimNames = append(dimNames, name)
		}
	}
	return
}

func (b *aggregatorsBuilder) prepareAggregator(
	cfg ddprofiledefinition.VirtualMetricConfig,
	sources []ddprofiledefinition.VirtualMetricSourceConfig,
) (agg *vmetricsAggregator, isComposite bool, dimIdxByName map[string]int, ok bool) {
	agg = &vmetricsAggregator{config: cfg}

	agg.grouped = cfg.PerRow || len(cfg.GroupBy) > 0
	if agg.grouped {
		table, same := b.sameTableOrEmpty(sources)
		if !same {
			b.log.Warningf("virtual metric '%s' uses group_by/per_row but sources span tables or have no table; skipping (no joins yet)", cfg.Name)
			return nil, false, nil, false
		}
		agg.perRow = cfg.PerRow
		agg.groupBy = cfg.GroupBy
		agg.groupTable = table
		agg.perGroup = make(map[string]*vmetricsGroupBucket, 64)
	}

	isComposite, dimIdxByName, dimNames := b.computeDimIndexMap(sources)
	if isComposite {
		agg.dimNames = dimNames
	}
	return agg, isComposite, dimIdxByName, true
}

func (b *aggregatorsBuilder) bindSinks(
	agg *vmetricsAggregator,
	sources []ddprofiledefinition.VirtualMetricSourceConfig,
	dimIdxByName map[string]int, isComposite bool,
) {
	for _, src := range sources {
		key := vmetricsSourceKey{metricName: src.Metric, tableName: src.Table}
		dimIdx := int16(-1)
		if isComposite {
			name := src.As
			if name == "" {
				name = src.Metric
			}
			if idx, ok := dimIdxByName[name]; ok {
				dimIdx = int16(idx)
			}
		}
		b.sourceToSinks[key] = append(b.sourceToSinks[key], vmetricsSink{agg: agg, dimIdx: dimIdx})
	}
}
